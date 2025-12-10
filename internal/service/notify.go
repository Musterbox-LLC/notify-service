package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"notify-service/internal/email"
	"notify-service/internal/email/templates"
	"notify-service/internal/notification"
	"notify-service/internal/sync"
	"notify-service/pkg/models"
	"notify-service/utils"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type NotifyService struct {
	emailSender     *email.Sender
	db              *gorm.DB
	r2Client        *utils.NotificationR2Client
	userSyncService *sync.UserSyncService
}

func NewNotifyService(emailSender *email.Sender, r2Client *utils.NotificationR2Client, userSyncService *sync.UserSyncService) *NotifyService {
	return &NotifyService{
		emailSender:     emailSender,
		db:              notification.GetDB(),
		r2Client:        r2Client,
		userSyncService: userSyncService,
	}
}

func (s *NotifyService) GetDB() *gorm.DB {
	return s.db
}

// --- User Sync & Helpers ---
func (s *NotifyService) GetAllUsers(ctx context.Context) ([]*models.User, error) {
	var users []*models.User
	err := s.db.WithContext(ctx).
		Order("username ASC").
		Find(&users).Error
	return users, err
}

// --- Email & generic notification helpers ---
func (s *NotifyService) SendEmail(ctx context.Context, req *models.EmailRequest) error {
	var subject, body string
	var err error

	// Normalize email type (trim whitespace, lowercase)
	emailType := strings.ToLower(strings.TrimSpace(req.Type))
	log.Printf("📧 [DEBUG] Processing email type: '%s' for user %s", emailType, req.UserID)

	switch emailType {
	case "email_verification":
		log.Printf("📧 [DEBUG] Processing email_verification for user %s", req.UserID)
		url, ok := req.Context["verify_url"].(string)
		if !ok {
			log.Printf("❌ [ERROR] email_verification: missing verify_url in context for user %s", req.UserID)
			return fmt.Errorf("missing verify_url in context")
		}
		body, err = templates.RenderEmailVerification(templates.VerificationData{
			VerifyURL: url,
		})
		if err != nil {
			log.Printf("❌ [ERROR] email_verification: render failed for user %s: %v", req.UserID, err)
			return fmt.Errorf("render verification: %w", err)
		}
		subject = "Verify Your Email Address"
		log.Printf("📧 [DEBUG] email_verification template rendered successfully for user %s", req.UserID)

	case "password_reset":
		log.Printf("📧 [DEBUG] Processing password_reset for user %s", req.UserID)
		resetLink, ok := req.Context["reset_link"].(string)
		if !ok {
			log.Printf("❌ [ERROR] password_reset: missing reset_link in context for user %s", req.UserID)
			return fmt.Errorf("missing reset_link in context")
		}
		body, err = templates.RenderPasswordResetEmail(templates.PasswordResetData{
			ResetLink: resetLink,
		})
		if err != nil {
			log.Printf("❌ [ERROR] password_reset: render failed for user %s: %v", req.UserID, err)
			return fmt.Errorf("render password_reset: %w", err)
		}
		subject = "Reset Your Password"
		log.Printf("📧 [DEBUG] password_reset template rendered successfully for user %s", req.UserID)

	case "otp":
		log.Printf("📧 [DEBUG] Processing otp for user %s", req.UserID)
		code, ok := req.Context["otp"].(string)
		if !ok {
			log.Printf("❌ [ERROR] otp: missing otp in context for user %s", req.UserID)
			return fmt.Errorf("missing otp in context")
		}
		if len(code) != 6 || !regexp.MustCompile(`^\d{6}$`).MatchString(code) {
			log.Printf("❌ [ERROR] otp: invalid OTP format for user %s: %s", req.UserID, code)
			return fmt.Errorf("invalid OTP format: expected 6-digit numeric")
		}
		body, err = templates.RenderOTPEmail(code)
		if err != nil {
			log.Printf("❌ [ERROR] otp: render failed for user %s: %v", req.UserID, err)
			return fmt.Errorf("render otp: %w", err)
		}
		subject = "Your MusterBox Login Code"
		log.Printf("📧 [DEBUG] otp template rendered successfully for user %s", req.UserID)

	case "new_login":
		log.Printf("📧 [DEBUG] Processing new_login for user %s", req.UserID)
		data, ok := req.Context["data"].(map[string]interface{})
		if !ok {
			log.Printf("❌ [ERROR] new_login: missing 'data' in context for user %s", req.UserID)
			return fmt.Errorf("missing 'data' in context for new_login")
		}

		d := templates.NewLoginData{
			UserName:         getString(data["user_name"]),
			Timestamp:        getString(data["timestamp"]),
			IPAddress:        getString(data["ip_address"]),
			DeviceOS:         getString(data["device_os"]),
			UserAgentSnippet: truncate(getString(data["user_agent_snippet"]), 40),
			LogoURL:          "",
			Year:             0,
		}

		body, err = templates.RenderNewLoginEmail(d)
		if err != nil {
			log.Printf("❌ [ERROR] new_login: render failed for user %s: %v", req.UserID, err)
			return fmt.Errorf("render new_login: %w", err)
		}
		subject = "🔐 New Login to Your Account"
		log.Printf("📧 [DEBUG] new_login template rendered successfully for user %s", req.UserID)

	case "pin_recovery":
		log.Printf("📧 [DEBUG] Processing pin_recovery for user %s", req.UserID)
		code, ok := req.Context["otp"].(string)
		if !ok {
			return fmt.Errorf("missing otp in context")
		}
		if len(code) != 6 || !regexp.MustCompile(`^\d{6}$`).MatchString(code) {
			return fmt.Errorf("invalid OTP format: expected 6-digit numeric")
		}

		// ✅ Compute subject FIRST (safe, reusable)
		otpData := templates.OTPData{
			OTP:     code,
			Purpose: "pin_recovery",
		}
		subject = templates.GetSubject(otpData.Purpose) // ← Extract as public helper
		body, err = templates.RenderOTPEmailWithData(otpData)
		if err != nil {
			return fmt.Errorf("render pin_recovery OTP: %w", err)
		}
		log.Printf("📧 [DEBUG] pin_recovery: subject='%s', user=%s", subject, req.UserID)

	// --- NEW CASES FOR TRANSACTIONAL EMAILS ---
	case "deposit_detected":
		log.Printf("📧 [DEBUG] Processing deposit_detected email type for user %s", req.UserID)

		data, ok := req.Context["data"].(map[string]interface{})
		if !ok {
			log.Printf("❌ [ERROR] deposit_detected: missing 'data' in context for user %s. Context keys: %v",
				req.UserID, getContextKeys(req.Context))
			return fmt.Errorf("missing 'data' in context for deposit_detected")
		}

		d := templates.DepositDetectedData{
			UserName:   getString(data["user_name"]),
			Amount:     getString(data["amount"]),
			Currency:   getString(data["currency"]),
			NewBalance: getString(data["new_balance"]),
			TxID:       getString(data["txid"]),
			Timestamp:  getString(data["timestamp"]),
			LogoURL:    getString(data["logo_url"]), // Optional, will default in renderer
			Year:       getYear(data["year"]),       // Optional, will default in renderer
		}

		log.Printf("📧 [DEBUG] deposit_detected: extracted data - UserName: '%s', Amount: '%s %s', NewBalance: '%s %s', TxID: '%s', Time: '%s'",
			d.UserName, d.Amount, d.Currency, d.NewBalance, d.Currency, d.TxID, d.Timestamp)

		body, err = templates.RenderDepositDetectedEmail(d)
		if err != nil {
			log.Printf("❌ [ERROR] deposit_detected: render failed for user %s: %v", req.UserID, err)
			return fmt.Errorf("render deposit_detected: %w", err)
		}
		subject = fmt.Sprintf("💰 Deposit of %s %s Confirmed", d.Amount, d.Currency)
		log.Printf("📧 [DEBUG] deposit_detected template rendered successfully for user %s", req.UserID)

	case "withdraw_completed":
		log.Printf("📧 [DEBUG] Processing withdraw_completed email type for user %s", req.UserID)

		data, ok := req.Context["data"].(map[string]interface{})
		if !ok {
			log.Printf("❌ [ERROR] withdraw_completed: missing 'data' in context for user %s. Context keys: %v",
				req.UserID, getContextKeys(req.Context))
			return fmt.Errorf("missing 'data' in context for withdraw_completed")
		}

		d := templates.WithdrawCompletedData{
			UserName:    getString(data["user_name"]),
			Amount:      getString(data["amount"]),
			Currency:    getString(data["currency"]),
			Destination: getString(data["destination"]),
			TxID:        getString(data["txid"]),
			FeeAmount:   getString(data["fee_amount"]),
			Timestamp:   getString(data["timestamp"]),
			LogoURL:     getString(data["logo_url"]),
			Year:        getYear(data["year"]),
		}

		log.Printf("📧 [DEBUG] withdraw_completed: extracted data - UserName: '%s', Amount: '%s %s', Dest: '%s', Fee: '%s %s', TxID: '%s', Time: '%s'",
			d.UserName, d.Amount, d.Currency, d.Destination, d.FeeAmount, d.Currency, d.TxID, d.Timestamp)

		body, err = templates.RenderWithdrawCompletedEmail(d)
		if err != nil {
			log.Printf("❌ [ERROR] withdraw_completed: render failed for user %s: %v", req.UserID, err)
			return fmt.Errorf("render withdraw_completed: %w", err)
		}
		subject = fmt.Sprintf("✅ Withdrawal of %s %s Completed", d.Amount, d.Currency)
		log.Printf("📧 [DEBUG] withdraw_completed template rendered successfully for user %s", req.UserID)

	case "conversion_sol_to_fiat_completed":
		log.Printf("📧 [DEBUG] Processing conversion_sol_to_fiat_completed email type for user %s", req.UserID)

		data, ok := req.Context["data"].(map[string]interface{})
		if !ok {
			log.Printf("❌ [ERROR] conversion_sol_to_fiat_completed: missing 'data' in context for user %s. Context keys: %v",
				req.UserID, getContextKeys(req.Context))
			return fmt.Errorf("missing 'data' in context for conversion_sol_to_fiat_completed")
		}

		d := templates.ConversionSolToFiatData{
			UserName:      getString(data["user_name"]),
			SOLAmount:     getString(data["sol_amount"]),
			FiatAmount:    getString(data["fiat_amount"]),
			FiatCurrency:  getString(data["fiat_currency"]),
			FeeAmountSOL:  getString(data["fee_amount_sol"]),
			ExchangeRate:  getString(data["exchange_rate"]),
			TxID:          getString(data["txid"]),
			Timestamp:     getString(data["timestamp"]),
			LogoURL:       getString(data["logo_url"]),
			Year:          getYear(data["year"]),
		}

		log.Printf("📧 [DEBUG] conversion_sol_to_fiat_completed: extracted data - UserName: '%s', %s SOL → %s %s, Fee: %s SOL, Rate: %s, TxID: '%s', Time: '%s'",
			d.UserName, d.SOLAmount, d.FiatAmount, d.FiatCurrency, d.FeeAmountSOL, d.ExchangeRate, d.TxID, d.Timestamp)

		body, err = templates.RenderConversionSolToFiatEmail(d)
		if err != nil {
			log.Printf("❌ [ERROR] conversion_sol_to_fiat_completed: render failed for user %s: %v", req.UserID, err)
			return fmt.Errorf("render conversion_sol_to_fiat_completed: %w", err)
		}
		subject = fmt.Sprintf("💱 SOL to %s Conversion Completed", d.FiatCurrency)
		log.Printf("📧 [DEBUG] conversion_sol_to_fiat_completed template rendered successfully for user %s", req.UserID)

	case "conversion_fiat_to_sol_completed":
		log.Printf("📧 [DEBUG] Processing conversion_fiat_to_sol_completed email type for user %s", req.UserID)

		data, ok := req.Context["data"].(map[string]interface{})
		if !ok {
			log.Printf("❌ [ERROR] conversion_fiat_to_sol_completed: missing 'data' in context for user %s. Context keys: %v",
				req.UserID, getContextKeys(req.Context))
			return fmt.Errorf("missing 'data' in context for conversion_fiat_to_sol_completed")
		}

		d := templates.ConversionFiatToSolData{
			UserName:       getString(data["user_name"]),
			FiatAmount:     getString(data["fiat_amount"]),
			FiatCurrency:   getString(data["fiat_currency"]),
			SOLAmount:      getString(data["sol_amount"]),
			FeeAmountFiat:  getString(data["fee_amount_fiat"]),
			ExchangeRate:   getString(data["exchange_rate"]),
			TxID:           getString(data["txid"]),
			Timestamp:      getString(data["timestamp"]),
			LogoURL:        getString(data["logo_url"]),
			Year:           getYear(data["year"]),
		}

		log.Printf("📧 [DEBUG] conversion_fiat_to_sol_completed: extracted data - UserName: '%s', %s %s → %s SOL, Fee: %s %s, Rate: %s, TxID: '%s', Time: '%s'",
			d.UserName, d.FiatAmount, d.FiatCurrency, d.SOLAmount, d.FeeAmountFiat, d.FiatCurrency, d.ExchangeRate, d.TxID, d.Timestamp)

		body, err = templates.RenderConversionFiatToSolEmail(d)
		if err != nil {
			log.Printf("❌ [ERROR] conversion_fiat_to_sol_completed: render failed for user %s: %v", req.UserID, err)
			return fmt.Errorf("render conversion_fiat_to_sol_completed: %w", err)
		}
		subject = fmt.Sprintf("💱 %s to SOL Conversion Completed", d.FiatCurrency)
		log.Printf("📧 [DEBUG] conversion_fiat_to_sol_completed template rendered successfully for user %s", req.UserID)

	// --- END NEW CASES ---

	default:
		log.Printf("❌ [ERROR] SendEmail: unsupported email type received: '%s' (normalized). Available types: email_verification, password_reset, otp, new_login, pin_recovery, deposit_detected, withdraw_completed, conversion_sol_to_fiat_completed, conversion_fiat_to_sol_completed",
			emailType)
		log.Printf("❌ [ERROR] Request details - UserID: %s, To: %s, Context keys: %v",
			req.UserID, req.To, getContextKeys(req.Context))
		return fmt.Errorf("unsupported email type: %s", req.Type)
	}

	// Log the prepared email details before sending
	log.Printf("📧 [PREPARED] To: %s | Subject: %s | Type: %s (normalized: '%s') | UserID: %s",
		req.To, subject, req.Type, emailType, req.UserID)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.emailSender.Send(ctx, req.To, subject, body); err != nil {
			log.Printf("⚠️ Background email failed for user %s, type %s: %v", req.UserID, emailType, err)
		} else {
			log.Printf("✅ [ASYNC SUCCESS] Email sent successfully for user %s, type: %s", req.UserID, emailType)
		}

		var actionLinks []models.ActionLink
		var contentLink *string
		switch emailType {
		case "email_verification":
			if url, ok := req.Context["verify_url"].(string); ok {
				actionLinks = []models.ActionLink{
					{Label: "Verify Email", URL: url, Style: "primary"},
				}
				contentLink = &url
			}
		case "password_reset":
			if link, ok := req.Context["reset_link"].(string); ok {
				actionLinks = []models.ActionLink{
					{Label: "Reset Password", URL: link, Style: "primary"},
				}
				contentLink = &link
			}
		case "new_login":
			// No action links for new login notifications
		case "pin_recovery":
			// No action links for PIN recovery (user enters code in app)
		case "deposit_detected", "withdraw_completed", "conversion_sol_to_fiat_completed", "conversion_fiat_to_sol_completed":
			// No action links for these transactional emails
		}

		actionsJSONBytes, _ := json.Marshal(actionLinks)
		actionsJSON := datatypes.JSON(actionsJSONBytes)
		metadataJSONBytes, _ := json.Marshal(map[string]string{"email_type": emailType})
		metadataJSON := datatypes.JSON(metadataJSONBytes)

		deliveredAt := time.Now()

		notif := &models.Notification{
			CreatorID:       req.UserID,
			Type:            models.NotificationTypeInfo,
			Heading:         getNotificationHeading(emailType),
			Title:           subject,
			Message:         "We've sent an email to your inbox. Please check your spam folder if you don't see it.",
			ContentImageURL: nil,
			ContentLink:     contentLink,
			ActionLinks:     actionsJSON,
			Metadata:        metadataJSON,
			IsDraft:         false,
			DeliveredAt:     &deliveredAt,
		}

		if err := s.db.Create(notif).Error; err != nil {
			log.Printf("⚠️ Failed to save email-triggered notification: %v", err)
			return
		}

		recipient := &models.NotificationRecipient{
			NotificationID: notif.ID,
			UserID:         req.UserID,
			Status:         models.RecipientStatusDelivered,
			DeliveredAt:    &deliveredAt,
		}

		if err := s.db.Create(recipient).Error; err != nil {
			log.Printf("⚠️ Failed to save recipient for email notification %s: %v", notif.ID, err)
		} else {
			log.Printf("✅ Email notification & recipient created: %s → user %s", notif.ID, req.UserID)
		}
	}()
	return nil
}

func getNotificationHeading(emailType string) string {
	switch emailType {
	case "email_verification":
		return "Email Verification Required"
	case "password_reset":
		return "Password Reset Requested"
	case "otp":
		return "Login Verification Code"
	case "new_login":
		return "New Login Activity"
	case "pin_recovery":
		return "PIN Recovery Code Sent" // ✅ Added
	case "deposit_detected":
		return "Deposit Confirmed"
	case "withdraw_completed":
		return "Withdrawal Completed"
	case "conversion_sol_to_fiat_completed":
		return "SOL to Fiat Conversion Completed"
	case "conversion_fiat_to_sol_completed":
		return "Fiat to SOL Conversion Completed"
	default:
		return "New Notification"
	}
}

func getString(v interface{}) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// getYear is a helper to safely extract an int year from an interface{}.
func getYear(v interface{}) int {
	if f, ok := v.(float64); ok { // JSON unmarshals numbers as float64
		return int(f)
	}
	if i, ok := v.(int); ok {
		return i
	}
	// Default to current year if not provided or invalid
	return 0 // This will be handled by the template renderer
}

// getContextKeys returns a slice of keys from the context map for debugging
func getContextKeys(ctx map[string]interface{}) []string {
	keys := make([]string, 0, len(ctx))
	for k := range ctx {
		keys = append(keys, k)
	}
	return keys
}

// --- User-facing: Get notifications + delivery status ---
func (s *NotifyService) GetUnreadNotifications(ctx context.Context, userID uuid.UUID) ([]*models.Notification, error) {
	var notifs []*models.Notification
	err := s.db.WithContext(ctx).
		Table("notifications").
		Joins("INNER JOIN notification_recipients nr ON notifications.id = nr.notification_id").
		Where("nr.user_id = ? AND nr.status = ?", userID, models.RecipientStatusDelivered).
		Order("nr.delivered_at DESC").
		Find(&notifs).Error
	return notifs, err
}

func (s *NotifyService) GetAllNotifications(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*models.Notification, error) {
	var notifs []*models.Notification
	err := s.db.WithContext(ctx).
		Table("notifications").
		Joins("INNER JOIN notification_recipients nr ON notifications.id = nr.notification_id").
		Where("nr.user_id = ?", userID).
		Order("nr.delivered_at DESC NULLS LAST, notifications.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&notifs).Error
	return notifs, err
}

func (s *NotifyService) MarkNotificationsRead(ctx context.Context, userID uuid.UUID, notificationIDs []uuid.UUID) error {
	now := time.Now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Model(&models.NotificationRecipient{}).
			Where("user_id = ? AND notification_id IN ?", userID, notificationIDs).
			Updates(map[string]interface{}{
				"status":     models.RecipientStatusRead,
				"read_at":    now,
				"updated_at": now,
			}).Error
	})
}

func (s *NotifyService) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	now := time.Now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Model(&models.NotificationRecipient{}).
			Where("user_id = ? AND status = ?", userID, models.RecipientStatusDelivered).
			Updates(map[string]interface{}{
				"status":     models.RecipientStatusRead,
				"read_at":    now,
				"updated_at": now,
			}).Error
	})
}

// --- Admin: CRUD on user-created notifications (drafts/templates) ---
func (s *NotifyService) CreateNotification(ctx context.Context, req *models.NotificationRequest) (*models.Notification, error) {
	actionsJSON, err := json.Marshal(req.ActionLinks)
	if err != nil {
		return nil, fmt.Errorf("invalid action_links: %w", err)
	}
	var metadataJSON datatypes.JSON
	if req.Metadata != nil {
		metaBytes, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("invalid metadata: %w", err)
		}
		metadataJSON = datatypes.JSON(metaBytes)
	}
	mediaURLsJSON, err := json.Marshal(req.MediaURLs)
	if err != nil {
		return nil, fmt.Errorf("invalid media_urls: %w", err)
	}
	notif := &models.Notification{
		CreatorID:       *req.CreatorID,
		Type:            models.NotificationType(req.Type),
		Heading:         req.Heading,
		Title:           req.Title,
		Message:         req.Message,
		ContentImageURL: req.ContentImageURL,
		ThumbnailURL:    req.ThumbnailURL,
		ContentLink:     req.ContentLink,
		ActionLinks:     datatypes.JSON(actionsJSON),
		Metadata:        metadataJSON,
		MediaURLs:       datatypes.JSON(mediaURLsJSON),
		ScheduledAt:     req.ScheduledAt,
		IsDraft:         true,
	}
	if err := s.db.WithContext(ctx).Create(notif).Error; err != nil {
		return nil, fmt.Errorf("DB create failed: %w", err)
	}
	log.Printf("✅ Draft notification created with ID %s: %s", notif.ID, req.Title)
	return notif, nil
}

func (s *NotifyService) UpdateNotification(ctx context.Context, id uuid.UUID, req *models.NotificationRequest) (*models.Notification, error) {
	var existing models.Notification
	if err := s.db.WithContext(ctx).Where("id = ? AND is_draft = true", id).First(&existing).Error; err != nil {
		return nil, fmt.Errorf("notification not found or not editable (must be draft): %w", err)
	}
	actionsJSON, err := json.Marshal(req.ActionLinks)
	if err != nil {
		return nil, fmt.Errorf("invalid action_links: %w", err)
	}
	var metadataJSON datatypes.JSON
	if req.Metadata != nil {
		metaBytes, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("invalid metadata: %w", err)
		}
		metadataJSON = datatypes.JSON(metaBytes)
	}
	mediaURLsJSON, err := json.Marshal(req.MediaURLs)
	if err != nil {
		return nil, fmt.Errorf("invalid media_urls: %w", err)
	}
	updates := map[string]interface{}{
		"heading":           req.Heading,
		"title":             req.Title,
		"message":           req.Message,
		"type":              models.NotificationType(req.Type),
		"content_image_url": req.ContentImageURL,
		"thumbnail_url":     req.ThumbnailURL,
		"content_link":      req.ContentLink,
		"action_links":      datatypes.JSON(actionsJSON),
		"metadata":          metadataJSON,
		"media_urls":        datatypes.JSON(mediaURLsJSON),
		"scheduled_at":      req.ScheduledAt,
	}
	if err := s.db.WithContext(ctx).Model(&existing).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&existing).Error; err != nil {
		return nil, err
	}
	return &existing, nil
}

func (s *NotifyService) DeleteNotification(ctx context.Context, id uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete template + all recipients
		if err := tx.Where("notification_id = ?", id).Delete(&models.NotificationRecipient{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Notification{}, id).Error
	})
}

// ✅ Publish: creates *only* recipients — no copies in notifications table
func (s *NotifyService) PublishNotification(ctx context.Context, id uuid.UUID, targetUserIDs []uuid.UUID) error {
	var template models.Notification
	if err := s.db.WithContext(ctx).Where("id = ? AND is_draft = true", id).First(&template).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("notification %s not found or not a draft", id)
		}
		return err
	}
	// If no targets, send to all
	if len(targetUserIDs) == 0 {
		var users []*models.User
		if err := s.db.WithContext(ctx).Find(&users).Error; err != nil {
			return fmt.Errorf("failed to fetch all users: %w", err)
		}
		for _, u := range users {
			if uid, err := uuid.Parse(u.ID); err == nil {
				targetUserIDs = append(targetUserIDs, uid)
			}
		}
	}
	now := time.Now()
	recipients := make([]*models.NotificationRecipient, 0, len(targetUserIDs))
	for _, userID := range targetUserIDs {
		recipients = append(recipients, &models.NotificationRecipient{
			NotificationID: id,
			UserID:         userID,
			Status:         models.RecipientStatusPending,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Bulk insert recipients
		if err := tx.CreateInBatches(recipients, 50).Error; err != nil {
			return fmt.Errorf("failed to create recipients: %w", err)
		}
		// Mark template as published
		if err := tx.Model(&template).
			Where("id = ?", id).
			Updates(map[string]interface{}{
				"is_draft":     false,
				"delivered_at": &now,
			}).Error; err != nil {
			return fmt.Errorf("failed to update template: %w", err)
		}
		log.Printf("✅ Published notification %s to %d users", id, len(targetUserIDs))
		return nil
	})
}

// ✅ GetAllDrafts — only drafts (is_draft = true AND scheduled_at IS NULL)
func (s *NotifyService) GetAllDrafts(ctx context.Context, limit, offset int, creatorID *uuid.UUID) ([]*models.Notification, error) {
	query := s.db.WithContext(ctx).
		Where("is_draft = true AND scheduled_at IS NULL").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset)
	if creatorID != nil {
		query = query.Where("creator_id = ?", *creatorID)
	}
	var drafts []*models.Notification
	err := query.Find(&drafts).Error
	return drafts, err
}

// ✅ GetAllNotificationsAdmin — supports filtering; returns templates only
func (s *NotifyService) GetAllNotificationsAdmin(ctx context.Context, limit, offset int, creatorID *uuid.UUID, status string) ([]*models.Notification, error) {
	query := s.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Offset(offset)
	if creatorID != nil {
		query = query.Where("creator_id = ?", *creatorID)
	}
	switch status {
	case "draft":
		query = query.Where("is_draft = true AND scheduled_at IS NULL")
	case "scheduled":
		query = query.Where("scheduled_at IS NOT NULL")
	case "delivered":
		query = query.Where("delivered_at IS NOT NULL AND is_draft = false")
	case "pending": // same as draft
		query = query.Where("is_draft = true AND scheduled_at IS NULL")
	}
	var notifs []*models.Notification
	err := query.Find(&notifs).Error
	return notifs, err
}

// ✅ GetNotificationHistory — templates that were delivered
func (s *NotifyService) GetNotificationHistory(
	ctx context.Context,
	limit, offset int,
	creatorID *uuid.UUID,
	status string,
	startDate, endDate *time.Time,
) ([]*models.Notification, error) {
	query := s.db.WithContext(ctx).
		Where("delivered_at IS NOT NULL AND is_draft = false").
		Order("delivered_at DESC").
		Limit(limit).
		Offset(offset)
	if creatorID != nil {
		query = query.Where("creator_id = ?", *creatorID)
	}
	if startDate != nil {
		query = query.Where("delivered_at >= ?", *startDate)
	}
	if endDate != nil {
		query = query.Where("delivered_at <= ?", *endDate)
	}
	var notifs []*models.Notification
	err := query.Find(&notifs).Error
	return notifs, err
}

// ✅ GetNotificationReceipts — returns ReceiptView with user info
func (s *NotifyService) GetNotificationReceipts(ctx context.Context, notifID uuid.UUID) ([]*models.ReceiptView, error) {
	var receipts []*models.NotificationRecipient
	if err := s.db.WithContext(ctx).
		Where("notification_id = ?", notifID).
		Order("delivered_at DESC NULLS LAST, created_at DESC").
		Find(&receipts).Error; err != nil {
		return nil, err
	}
	userIDs := make([]uuid.UUID, 0, len(receipts))
	for _, r := range receipts {
		userIDs = append(userIDs, r.UserID)
	}
	usersByID := make(map[uuid.UUID]*models.User)
	if len(userIDs) > 0 {
		var users []*models.User
		if err := s.db.WithContext(ctx).
			Where("id IN ?", userIDs).
			Find(&users).Error; err == nil {
			for _, u := range users {
				uid, _ := uuid.Parse(u.ID)
				usersByID[uid] = u
			}
		}
	}
	result := make([]*models.ReceiptView, 0, len(receipts))
	for _, r := range receipts {
		u := usersByID[r.UserID]
		username := "unknown"
		email := ""
		if u != nil {
			username = u.Username
			email = u.Email
		}
		result = append(result, &models.ReceiptView{
			UserID:      r.UserID,
			Username:    username,
			Email:       email,
			Status:      string(r.Status),
			DeliveredAt: r.DeliveredAt,
			ReadAt:      r.ReadAt,
		})
	}
	return result, nil
}

// ✅ ConvertToDraft — resets template & deletes recipients
func (s *NotifyService) ConvertToDraft(ctx context.Context, id uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Reset template
		if err := tx.Model(&models.Notification{}).
			Where("id = ?", id).
			Updates(map[string]interface{}{
				"is_draft":     true,
				"scheduled_at": nil,
				"delivered_at": nil,
			}).Error; err != nil {
			return err
		}
		// 2. Delete all recipients
		if err := tx.Where("notification_id = ?", id).Delete(&models.NotificationRecipient{}).Error; err != nil {
			log.Printf("⚠️ Failed to delete recipients for %s: %v", id, err)
			// Non-fatal; continue
		}
		log.Printf("🔄 Notification %s converted to draft", id)
		return nil
	})
}

// --- System Notification Template CRUD ---
func (s *NotifyService) GetAllSystemNotificationTemplates(ctx context.Context) ([]*models.SystemNotificationTemplate, error) {
	var templates []*models.SystemNotificationTemplate
	err := s.db.WithContext(ctx).Order("name ASC").Find(&templates).Error
	return templates, err
}

func (s *NotifyService) GetSystemNotificationTemplateByEventKey(ctx context.Context, eventKey string) (*models.SystemNotificationTemplate, error) {
	var template models.SystemNotificationTemplate
	err := s.db.WithContext(ctx).Where("event_key = ?", eventKey).First(&template).Error
	return &template, err
}

func (s *NotifyService) UpdateSystemNotificationTemplate(ctx context.Context, eventKey string, updates map[string]interface{}) error {
	allowedUpdates := make(map[string]interface{})
	for _, field := range []string{"heading", "title", "message", "type", "icon", "enabled"} {
		if val, ok := updates[field]; ok {
			allowedUpdates[field] = val
		}
	}
	if len(allowedUpdates) == 0 {
		return fmt.Errorf("no valid fields to update")
	}
	result := s.db.WithContext(ctx).Model(&models.SystemNotificationTemplate{}).Where("event_key = ?", eventKey).Updates(allowedUpdates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("template not found")
	}
	return nil
}

// --- System Notification Trigger Logic ---
func (s *NotifyService) CreateAndDeliverSystemNotification(
	ctx context.Context,
	req *models.NotificationRequest,
	userID uuid.UUID, // passed separately for clarity & type safety
) (*models.Notification, error) {
	now := time.Now()

	// Build final notification
	notification := &models.Notification{
		CreatorID:       uuid.Nil,
		Type:            models.NotificationType(req.Type),
		Heading:         req.Heading,
		Title:           req.Title,
		Message:         req.Message,
		ContentImageURL: req.ContentImageURL,
		ThumbnailURL:    req.ThumbnailURL,
		ContentLink:     req.ContentLink,
		IsDraft:         false,
		DeliveredAt:     &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	// Metadata: include original variables + audit
	meta := make(map[string]interface{})
	if req.Metadata != nil {
		if m, ok := req.Metadata.(map[string]interface{}); ok {
			for k, v := range m {
				meta[k] = v
			}
		}
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}
	notification.Metadata = datatypes.JSON(metaBytes)

	// Media & Actions
	mediaURLsJSON, _ := json.Marshal(req.MediaURLs)
	notification.MediaURLs = datatypes.JSON(mediaURLsJSON)

	actionsJSON, _ := json.Marshal(req.ActionLinks)
	notification.ActionLinks = datatypes.JSON(actionsJSON)

	// Save notification
	if err := s.db.WithContext(ctx).Create(notification).Error; err != nil {
		return nil, fmt.Errorf("DB create notification failed: %w", err)
	}

	// Create recipient
	recipient := &models.NotificationRecipient{
		NotificationID: notification.ID,
		UserID:         userID,
		Status:         models.RecipientStatusDelivered,
		DeliveredAt:    &now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.db.WithContext(ctx).Create(recipient).Error; err != nil {
		log.Printf("⚠️ Failed to create recipient for system notification %s: %v", notification.ID, err)
		// Do not fail — notification exists; delivery is async anyway
	}

	log.Printf("✅ System notification %s delivered to user %s", notification.ID, userID)
	return notification, nil
}

// --- R2 helpers ---
func (s *NotifyService) UploadFileToR2(ctx context.Context, key string, content []byte, contentType string) error {
	return s.r2Client.Upload(ctx, key, content, contentType)
}

func (s *NotifyService) GetPublicURL(key string) string {
	return fmt.Sprintf("%s/%s", s.r2Client.GetPublicURL(), key)
}

// ScheduleNotificationWithTargets — extends ScheduleNotification to accept target_user_ids
func (s *NotifyService) ScheduleNotificationWithTargets(ctx context.Context, id uuid.UUID, scheduledAt time.Time, targetUserIDs []uuid.UUID) error {
	var existing models.Notification
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&existing).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{
		"scheduled_at": scheduledAt,
	}
	if len(targetUserIDs) > 0 {
		existingMeta := make(map[string]interface{})
		if len(existing.Metadata) > 0 {
			_ = json.Unmarshal(existing.Metadata, &existingMeta)
		}
		targetUserIDsJSON, _ := json.Marshal(targetUserIDs)
		existingMeta["target_user_ids"] = json.RawMessage(targetUserIDsJSON)
		if metaJSON, err := json.Marshal(existingMeta); err == nil {
			updates["metadata"] = datatypes.JSON(metaJSON)
		}
	}
	return s.db.WithContext(ctx).Model(&existing).Updates(updates).Error
}

// UnscheduleNotificationWithCleanup — removes target_user_ids from metadata
func (s *NotifyService) UnscheduleNotificationWithCleanup(ctx context.Context, id uuid.UUID) error {
	var existing models.Notification
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&existing).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{
		"scheduled_at": nil,
	}
	if len(existing.Metadata) > 0 {
		var existingMeta map[string]interface{}
		if err := json.Unmarshal(existing.Metadata, &existingMeta); err == nil {
			delete(existingMeta, "target_user_ids")
			if metaJSON, err := json.Marshal(existingMeta); err == nil {
				updates["metadata"] = datatypes.JSON(metaJSON)
			}
		}
	}
	return s.db.WithContext(ctx).Model(&existing).Updates(updates).Error
}
