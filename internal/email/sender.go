// internal/email/sender.go
package email

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"notify-service/internal/config"
	"notify-service/internal/email/templates" // Import the templates package

	"github.com/google/uuid"
	"gopkg.in/gomail.v2"
)

type Sender struct {
	cfg *config.Config
}

func NewSender(cfg *config.Config) *Sender {
	return &Sender{cfg: cfg}
}

func (s *Sender) Send(ctx context.Context, to, subject, body string) error {
	// Heavy logging — per your preference
	log.Printf("📧 [SEND] To: %s | Subject: %s", to, subject)

	m := gomail.NewMessage()
	m.SetHeader("From", fmt.Sprintf("%s <%s>", s.cfg.SMTPFromName, s.cfg.SMTPFrom))
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", body)

	dialer := gomail.NewDialer(s.cfg.SMTPHost, s.cfg.SMTPPort, s.cfg.SMTPUser, s.cfg.SMTPPass)

	// Exponential backoff: 1s, 2s, 4s → max 3 retries
	for attempt := 0; attempt < 3; attempt++ {
		if err := dialer.DialAndSend(m); err != nil {
			delay := time.Duration(1<<attempt) * time.Second // 1s, 2s, 4s
			log.Printf("❌ [ATTEMPT %d] Failed to send email to %s: %v → retrying in %v", attempt+1, to, err, delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return fmt.Errorf("email send cancelled: %w", ctx.Err())
			}
			continue
		}
		log.Printf("✅ [SUCCESS] Email sent to %s (Subject: %s)", to, subject)
		return nil
	}

	log.Printf("💥 [FAILED] All retries exhausted for %s", to)
	return fmt.Errorf("failed to send email to %s after 3 attempts", to)
}

// SendEmail processes the request, renders the appropriate template, and queues the email.
func (s *Sender) SendEmail(ctx context.Context, req *EmailRequest) error {
	// Log the incoming request type for debugging
	log.Printf("📧 [DEBUG] SendEmail received request. Type: '%s', To: %s, UserID: %s", req.Type, req.To, req.UserID)

	var subject, body string
	var err error

	// Normalize: trim *all* whitespace (including U+00A0, U+2028, U+2029), then lowercase
	cleanType := strings.Map(func(r rune) rune {
		if r <= ' ' || r == 0xA0 || r == 0x2028 || r == 0x2029 {
			return -1 // delete
		}
		return r
	}, req.Type)
	emailType := strings.ToLower(strings.TrimSpace(cleanType))
	log.Printf("📧 [DEBUG] Normalized email type: '%s' → '%s'", req.Type, emailType)

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
		// Validate OTP format if necessary (e.g., 6 digits)
		if len(code) != 6 {
			log.Printf("⚠️ [WARN] otp: OTP code length is %d, expected 6 for user %s", len(code), req.UserID)
		}
		body, err = templates.RenderOTPEmail(code)
		if err != nil {
			log.Printf("❌ [ERROR] otp: render failed for user %s: %v", req.UserID, err)
			return fmt.Errorf("render otp: %w", err)
		}
		subject = "Your MusterBox Login Code"
		log.Printf("📧 [DEBUG] otp template rendered successfully for user %s", req.UserID)

	case "new_login":
		log.Printf("📧 [DEBUG] Processing new_login email type for user %s", req.UserID)

		// Check if data exists in context
		data, ok := req.Context["data"].(map[string]interface{})
		if !ok {
			log.Printf("❌ [ERROR] new_login: missing 'data' in context for user %s. Context keys: %v",
				req.UserID, getContextKeys(req.Context))
			return fmt.Errorf("missing 'data' in context for new_login")
		}

		log.Printf("📧 [DEBUG] new_login: extracted data map with keys: %v", getMapKeys(data))

		d := templates.NewLoginData{
			UserName:         getString(data["user_name"]),
			Timestamp:        getString(data["timestamp"]),
			IPAddress:        getString(data["ip_address"]),
			DeviceOS:         getString(data["device_os"]),
			UserAgentSnippet: truncate(getString(data["user_agent_snippet"]), 40),
			LogoURL:          "", // defaults to musterbox.org/icon.png in template
			Year:             0,  // defaults to time.Now().Year() in template
		}

		// Log extracted data for debugging
		log.Printf("📧 [DEBUG] new_login: extracted data - UserName: '%s', Timestamp: '%s', IP: '%s', DeviceOS: '%s', UA: '%s'",
			d.UserName, d.Timestamp, d.IPAddress, d.DeviceOS, d.UserAgentSnippet)

		body, err = templates.RenderNewLoginEmail(d)
		if err != nil {
			log.Printf("❌ [ERROR] new_login: render failed for user %s: %v", req.UserID, err)
			return fmt.Errorf("render new_login: %w", err)
		}
		subject = "🔐 New Login to Your Account"
		log.Printf("📧 [DEBUG] new_login template rendered successfully for user %s", req.UserID)

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
		// Log the exact value of req.Type that caused the default case to trigger
		log.Printf("❌ [ERROR] SendEmail: unsupported email type received: '%s' (original), '%s' (normalized, len=%d). Available types: email_verification, password_reset, otp, new_login, deposit_detected, withdraw_completed, conversion_sol_to_fiat_completed, conversion_fiat_to_sol_completed",
			req.Type, emailType, len(emailType))
		log.Printf("❌ [ERROR] Request details - UserID: %s, To: %s, Context keys: %v",
			req.UserID, req.To, getContextKeys(req.Context))
		return fmt.Errorf("unsupported email type: %s", req.Type)
	}

	// Log the prepared email details before sending
	log.Printf("📧 [PREPARED] To: %s | Subject: %s | Type: %s (normalized: '%s') | UserID: %s",
		req.To, subject, req.Type, emailType, req.UserID)

	// Send the email asynchronously in a goroutine with a timeout
	go func() {
		log.Printf("📧 [ASYNC] Starting async email send for user %s, type: %s", req.UserID, emailType)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if sendErr := s.Send(ctx, req.To, subject, body); sendErr != nil {
			log.Printf("⚠️ [ERROR] Background email failed for user %s, type %s: %v", req.UserID, emailType, sendErr)
		} else {
			log.Printf("✅ [ASYNC SUCCESS] Email sent successfully for user %s, type: %s", req.UserID, emailType)
		}
	}()

	log.Printf("📧 [QUEUED] Email queued for async delivery to %s, type: %s", req.To, emailType)
	return nil
}

// getString is a helper to safely extract a string from an interface{}.
func getString(v interface{}) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	if v == nil {
		return ""
	}
	// Try to convert other types to string
	return fmt.Sprintf("%v", v)
}

// truncate is a helper to truncate a string to a maximum length, appending "…" if truncated.
func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// getContextKeys returns a slice of keys from the context map for debugging
func getContextKeys(ctx map[string]interface{}) []string {
	keys := make([]string, 0, len(ctx))
	for k := range ctx {
		keys = append(keys, k)
	}
	return keys
}

// getMapKeys returns a slice of keys from a map[string]interface{} for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
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

// EmailRequest represents the payload for triggering an email via the API.
// This struct is defined here as it's closely related to the sender's function.
type EmailRequest struct {
	UserID  uuid.UUID              `json:"user_id" validate:"required"`
	To      string                 `json:"to" validate:"required,email"` // Note: Consider making this optional if fetching from profile is desired
	Type    string                 `json:"type" validate:"required,oneof=email_verification password_reset otp new_login deposit_detected withdraw_completed conversion_sol_to_fiat_completed conversion_fiat_to_sol_completed"`
	Context map[string]interface{} `json:"context" validate:"required"`
}