package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"notify-service/internal/service"
	"notify-service/pkg/models"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotificationHandler struct {
	notifyService *service.NotifyService
}

func NewNotificationHandler(notifyService *service.NotifyService) *NotificationHandler {
	return &NotificationHandler{notifyService: notifyService}
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("⚠️ toJSON marshal error: %v", err)
		return "{}"
	}
	return string(b)
}

func (h *NotificationHandler) GetAllUsers(c *fiber.Ctx) error {
	users, err := h.notifyService.GetAllUsers(c.Context())
	if err != nil {
		log.Printf("❌ [GetAllUsers] Failed to fetch users: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch users"})
	}
	return c.JSON(fiber.Map{"users": users})
}

// CreateNotification — admin only
func (h *NotificationHandler) CreateNotification(c *fiber.Ctx) error {
	var req models.NotificationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.CreatorID == nil {
		creatorIDStr := c.Get("X-User-ID")
		if creatorIDStr == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "X-User-ID required for admin notification creation",
			})
		}
		creatorID, err := uuid.Parse(creatorIDStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid X-User-ID"})
		}
		req.CreatorID = &creatorID
	}
	if req.Heading == "" || req.Title == "" || req.Message == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "heading, title, and message are required"})
	}
	notification, err := h.notifyService.CreateNotification(c.Context(), &req)
	if err != nil {
		log.Printf("❌ CreateNotification failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"status":       "success",
		"message":      "notification draft created",
		"notification": notification,
	})
}

func (h *NotificationHandler) UpdateNotification(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid notification id"})
	}
	var req models.NotificationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Heading == "" || req.Title == "" || req.Message == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "heading, title, and message are required"})
	}
	notification, err := h.notifyService.UpdateNotification(c.Context(), id, &req)
	if err != nil {
		log.Printf("❌ UpdateNotification failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"status":       "success",
		"message":      "notification updated",
		"notification": notification,
	})
}

func (h *NotificationHandler) DeleteNotification(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid notification id"})
	}
	err = h.notifyService.DeleteNotification(c.Context(), id)
	if err != nil {
		log.Printf("❌ DeleteNotification failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "notification deleted",
	})
}

// PublishNotification — sends to targeted users (idempotent on recipients)
func (h *NotificationHandler) PublishNotification(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid notification id"})
	}
	var req struct {
		TargetUserIDs []uuid.UUID `json:"target_user_ids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := h.notifyService.PublishNotification(c.Context(), id, req.TargetUserIDs); err != nil {
		log.Printf("❌ PublishNotification failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "notification published to recipients",
	})
}

// ✅ GetAllDrafts
func (h *NotificationHandler) GetAllDrafts(c *fiber.Ctx) error {
	limit := getQueryInt(c, "limit", 20, 1, 100)
	offset := getQueryInt(c, "offset", 0, 0, 10000)
	creatorIDStr := c.Query("creator_id")
	var creatorID *uuid.UUID
	if creatorIDStr != "" {
		id, err := uuid.Parse(creatorIDStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid creator_id"})
		}
		creatorID = &id
	}
	drafts, err := h.notifyService.GetAllDrafts(c.Context(), limit, offset, creatorID)
	if err != nil {
		log.Printf("❌ GetAllDrafts: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch drafts"})
	}
	return c.JSON(fiber.Map{"drafts": drafts})
}

// ✅ GetNotificationReceipts
func (h *NotificationHandler) GetNotificationReceipts(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid notification id"})
	}
	receipts, err := h.notifyService.GetNotificationReceipts(c.Context(), id)
	if err != nil {
		log.Printf("❌ GetNotificationReceipts: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch receipts"})
	}
	return c.JSON(fiber.Map{"receipts": receipts})
}

// ✅ ConvertToDraft
func (h *NotificationHandler) ConvertToDraft(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid notification id"})
	}
	if err := h.notifyService.ConvertToDraft(c.Context(), id); err != nil {
		log.Printf("❌ ConvertToDraft: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "notification converted to draft",
	})
}

// ✅ GetNotificationHistory
func (h *NotificationHandler) GetNotificationHistory(c *fiber.Ctx) error {
	limit := getQueryInt(c, "limit", 20, 1, 100)
	offset := getQueryInt(c, "offset", 0, 0, 10000)
	creatorIDStr := c.Query("creator_id")
	var creatorID *uuid.UUID
	if creatorIDStr != "" {
		id, err := uuid.Parse(creatorIDStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid creator_id"})
		}
		creatorID = &id
	}
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	var startDate, endDate *time.Time
	if startDateStr != "" {
		t, err := time.Parse(time.RFC3339, startDateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid start_date (RFC3339)"})
		}
		startDate = &t
	}
	if endDateStr != "" {
		t, err := time.Parse(time.RFC3339, endDateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid end_date (RFC3339)"})
		}
		endDate = &t
	}
	result, err := h.notifyService.GetNotificationHistory(c.Context(), limit, offset, creatorID, "", startDate, endDate)
	if err != nil {
		log.Printf("❌ GetNotificationHistory: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch history"})
	}
	return c.JSON(fiber.Map{"notifications": result})
}

// ✅ BulkDeliverNotification — alias to Publish with target_all
func (h *NotificationHandler) BulkDeliverNotification(c *fiber.Ctx) error {
	var req struct {
		NotificationID uuid.UUID   `json:"notification_id"`
		UserIDs        []uuid.UUID `json:"user_ids"`
		TargetAll      bool        `json:"target_all"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}
	var targetUserIDs []uuid.UUID
	if req.TargetAll {
		all, err := h.notifyService.GetAllUsers(c.Context())
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch all users"})
		}
		for _, u := range all {
			if uid, err := uuid.Parse(u.ID); err == nil {
				targetUserIDs = append(targetUserIDs, uid)
			}
		}
	} else {
		targetUserIDs = req.UserIDs
		if len(targetUserIDs) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_ids required if target_all=false"})
		}
	}
	if err := h.notifyService.PublishNotification(c.Context(), req.NotificationID, targetUserIDs); err != nil {
		log.Printf("❌ BulkDeliverNotification: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "notification published",
	})
}

// Admin list: supports status filter
func (h *NotificationHandler) GetAllNotificationsAdmin(c *fiber.Ctx) error {
	limit := getQueryInt(c, "limit", 20, 1, 100)
	offset := getQueryInt(c, "offset", 0, 0, 10000)
	creatorIDStr := c.Query("creator_id")
	var creatorID *uuid.UUID
	if creatorIDStr != "" {
		id, err := uuid.Parse(creatorIDStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid creator_id"})
		}
		creatorID = &id
	}
	status := c.Query("status")
	notifications, err := h.notifyService.GetAllNotificationsAdmin(c.Context(), limit, offset, creatorID, status)
	if err != nil {
		log.Printf("❌ GetAllNotificationsAdmin: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch notifications"})
	}
	return c.JSON(fiber.Map{"notifications": notifications})
}

func (h *NotificationHandler) ScheduleNotification(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid notification id"})
	}
	var req struct {
		ScheduledAt   time.Time   `json:"scheduled_at"`
		TargetUserIDs []uuid.UUID `json:"target_user_ids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := h.notifyService.ScheduleNotificationWithTargets(c.Context(), id, req.ScheduledAt, req.TargetUserIDs); err != nil {
		log.Printf("❌ ScheduleNotification failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "notification scheduled",
	})
}

func (h *NotificationHandler) UnscheduleNotification(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid notification id"})
	}
	if err := h.notifyService.UnscheduleNotificationWithCleanup(c.Context(), id); err != nil {
		log.Printf("❌ UnscheduleNotification failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "notification unscheduled",
	})
}

// User-facing: via Gateway
func (h *NotificationHandler) GetUnread(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user_id"})
	}
	notifications, err := h.notifyService.GetUnreadNotifications(c.Context(), userID)
	if err != nil {
		log.Printf("❌ GetUnread: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch unread notifications"})
	}
	return c.JSON(fiber.Map{"notifications": notifications})
}

func (h *NotificationHandler) GetAll(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user_id"})
	}
	
	// Check for since parameter
	since := c.Query("since")
	var sinceTime *time.Time
	if since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid since parameter, must be RFC3339 format",
			})
		}
		sinceTime = &t
	}
	
	limit := getQueryInt(c, "limit", 20, 1, 100)
	offset := getQueryInt(c, "offset", 0, 0, 10000)
	
	// Use the new GetNotificationsSince method or modify GetAllNotifications to accept since
	var notifications []*models.Notification
	if sinceTime != nil {
		notifications, err = h.notifyService.GetNotificationsSince(c.Context(), userID, sinceTime)
	} else {
		notifications, err = h.notifyService.GetAllNotifications(c.Context(), userID, limit, offset, sinceTime)
	}
	
	if err != nil {
		log.Printf("❌ GetAll: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch notifications"})
	}
	
	return c.JSON(fiber.Map{"notifications": notifications})
}



func (h *NotificationHandler) MarkRead(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user_id"})
	}
	var req struct {
		NotificationIDs []uuid.UUID `json:"notification_ids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if len(req.NotificationIDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "notification_ids required"})
	}
	if err := h.notifyService.MarkNotificationsRead(c.Context(), userID, req.NotificationIDs); err != nil {
		log.Printf("❌ MarkRead: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to mark notifications as read"})
	}
	return c.JSON(fiber.Map{"status": "success", "message": "notifications marked as read"})
}

func (h *NotificationHandler) MarkAllRead(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user_id"})
	}
	if err := h.notifyService.MarkAllRead(c.Context(), userID); err != nil {
		log.Printf("❌ MarkAllRead: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to mark all as read"})
	}
	return c.JSON(fiber.Map{"status": "success", "message": "all notifications marked as read"})
}

// Helper
func getQueryInt(c *fiber.Ctx, key string, def, min, max int) int {
	s := c.Query(key)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func (h *NotificationHandler) UpdateSystemTemplate(c *fiber.Ctx) error {
	eventKey := c.Params("event_key")
	var req struct {
		Heading *string `json:"heading,omitempty"`
		Title   *string `json:"title,omitempty"`
		Message *string `json:"message,omitempty"`
		Type    *string `json:"type,omitempty"`
		Icon    *string `json:"icon,omitempty"`
		Enabled *bool   `json:"enabled,omitempty"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}
	updateFields := make(map[string]interface{})
	if req.Heading != nil {
		updateFields["heading"] = *req.Heading
	}
	if req.Title != nil {
		updateFields["title"] = *req.Title
	}
	if req.Message != nil {
		updateFields["message"] = *req.Message
	}
	if req.Type != nil {
		updateFields["type"] = *req.Type
	}
	if req.Icon != nil {
		updateFields["icon"] = *req.Icon
	}
	if req.Enabled != nil {
		updateFields["enabled"] = *req.Enabled
	}
	if len(updateFields) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no fields to update"})
	}

	db := h.notifyService.GetDB()
	result := db.Model(&models.SystemNotificationTemplate{}).
		Where("event_key = ?", eventKey).
		Updates(updateFields)

	if result.Error != nil {
		log.Printf("❌ UpdateSystemTemplate %s failed: %v", eventKey, result.Error)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "update failed"})
	}
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "template not found"})
	}

	var updated models.SystemNotificationTemplate
	err := db.Where("event_key = ?", eventKey).First(&updated).Error
	if err != nil {
		log.Printf("⚠️ Template %s updated but not retrievable: %v", eventKey, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "post-read failed"})
	}

	return c.JSON(fiber.Map{"template": updated})
}

func (h *NotificationHandler) TriggerSystemNotification(c *fiber.Ctx) error {
	var req struct {
		EventKey  string                 `json:"event_key" validate:"required"`
		UserID    uuid.UUID              `json:"user_id" validate:"required"`
		Variables map[string]interface{} `json:"variables" validate:"required"`
		DedupKey  *string                `json:"dedup_key,omitempty"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}

	db := h.notifyService.GetDB()

	// Fetch template
	var template models.SystemNotificationTemplate
	if err := db.Where("event_key = ? AND enabled = true", req.EventKey).First(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[TRIGGER] ⚠️ Ignored disabled/missing template: %s", req.EventKey)
			return c.Status(fiber.StatusNoContent).Send(nil)
		}
		log.Printf("[TRIGGER] DB error fetching template %s: %v", req.EventKey, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "template lookup failed"})
	}

	// Decode template.TemplateVars (JSONB) → []string
	var requiredVars []string
	if len(template.TemplateVars) > 0 {
		if err := json.Unmarshal(template.TemplateVars, &requiredVars); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "invalid template_vars format",
			})
		}
	}

	// Validate required variables
	for _, v := range requiredVars {
		if _, ok := req.Variables[v]; !ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("missing required variable: %s", v),
			})
		}
	}

	// Render
	renderedHeading := renderTemplateString(template.Heading, req.Variables)
	renderedTitle := renderTemplateString(template.Title, req.Variables)
	renderedMessage := renderTemplateString(template.Message, req.Variables)

	// Deduplication
	if req.DedupKey != nil {
		var count int64
		err := db.Model(&models.NotificationRecipient{}).
			Joins("JOIN notifications ON notifications.id = notification_recipients.notification_id").
			Where("notification_recipients.user_id = ? AND notifications.metadata->>'dedup_key' = ? AND notification_recipients.created_at > ?",
				req.UserID, *req.DedupKey, time.Now().Add(-24*time.Hour)).
			Count(&count)
		if err != nil {
			log.Printf("[DEDUP] DB error checking dedup: %v", err)
			// Proceed (fail-open for delivery)
		} else if count > 0 {
			log.Printf("[DEDUP] Skipped duplicate %s for user %s with key %s", req.EventKey, req.UserID, *req.DedupKey)
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
				"status":  "deduped",
				"message": "notification skipped (dedup)",
			})
		}
		// Inject dedup_key
		if req.Variables == nil {
			req.Variables = make(map[string]interface{})
		}
		req.Variables["dedup_key"] = *req.DedupKey
	}

	// Build request — note: NotificationRequest in models has no `SystemEventKey` or `RecipientUserID` (per current KB)
	// So we use CreatorID = nil (or &uuid.Nil), and pass UserID separately to service.
	notifReq := &models.NotificationRequest{
		CreatorID:       &uuid.Nil,
		Heading:         renderedHeading,
		Title:           renderedTitle,
		Message:         renderedMessage,
		Type:            template.Type,
		ContentImageURL: nil,
		ThumbnailURL:    nil,
		MediaURLs:       nil,
		ContentLink:     nil,
		Metadata:        req.Variables,
		// ScheduledAt, etc. — left nil
	}

	// Deliver
	notification, err := h.notifyService.CreateAndDeliverSystemNotification(c.Context(), notifReq, req.UserID)
	if err != nil {
		log.Printf("[TRIGGER] ❌ Failed to deliver %s to %s: %v", req.EventKey, req.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "delivery failed"})
	}

	return c.JSON(fiber.Map{
		"status":       "success",
		"notification": notification,
	})
}

// renderTemplateString replaces {{key}} with values (simple, non-HTML-escaped)
func renderTemplateString(template string, variables map[string]interface{}) string {
	result := template
	for key, value := range variables {
		placeholder := fmt.Sprintf("{{%s}}", key)
		var valueStr string
		switch v := value.(type) {
		case string:
			valueStr = v
		case nil:
			valueStr = ""
		case bool, int, int8, int16, int32, int64, float32, float64:
			valueStr = fmt.Sprintf("%v", v)
		default:
			if b, err := json.Marshal(v); err == nil {
				valueStr = string(b)
			} else {
				valueStr = fmt.Sprintf("%v", v)
			}
		}
		result = strings.ReplaceAll(result, placeholder, valueStr)
	}
	return result
}


// ✅ GetSystemTemplates — admin only
func (h *NotificationHandler) GetSystemTemplates(c *fiber.Ctx) error {
    db := h.notifyService.GetDB()
    var templates []models.SystemNotificationTemplate
    // Fetch all templates, ordered by event_key for consistency
    if err := db.Order("event_key ASC").Find(&templates).Error; err != nil {
        log.Printf("❌ GetSystemTemplates: %v", err)
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch templates"})
    }
    // Return the list of templates
    return c.JSON(fiber.Map{"templates": templates})
}


// StreamNotifications is removed as SSE is replaced by FCM
func (h *NotificationHandler) StreamNotifications(c *fiber.Ctx) error {
    return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
        "error": "SSE streaming is deprecated. Use FCM push notifications instead.",
    })
}

func (h *NotificationHandler) RegisterFCMToken(c *fiber.Ctx) error {
	userIDStr := c.Get("X-User-ID") // or from params/body
	if userIDStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "X-User-ID required"})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
	}

	var req struct {
		Token    string `json:"token" validate:"required"`
		DeviceID string `json:"device_id" validate:"required"`
		Platform string `json:"platform"` // e.g., "android", "ios", "web"
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}

	if req.Platform == "" {
		req.Platform = "unknown"
	}

	// Upsert: update if (user_id + device_id) exists
	token := models.FCMToken{
		UserID:   userID,
		DeviceID: req.DeviceID,
		Token:    req.Token,
		Platform: req.Platform,
	}

	result := h.notifyService.GetDB().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "device_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"token":      req.Token,
			"platform":   req.Platform,
			"updated_at": time.Now(),
		}),
	}).Create(&token)

	if result.Error != nil {
		log.Printf("❌ Failed to register FCM token: %v", result.Error)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "registration failed"})
	}

	return c.JSON(fiber.Map{
		"status": "success",
		"token_id": token.ID,
	})
}

func (h *NotificationHandler) UnregisterFCMToken(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Get("X-User-ID"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "X-User-ID invalid"})
	}

	var req struct {
		DeviceID string `json:"device_id" validate:"required"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "device_id required"})
	}

	err = h.notifyService.GetDB().Where("user_id = ? AND device_id = ?", userID, req.DeviceID).
		Update("deleted_at", time.Now()).Error

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "unregister failed"})
	}
	return c.JSON(fiber.Map{"status": "success"})
}


func (h *NotificationHandler) GetAllSince(c *fiber.Ctx) error {
	userID := c.Params("user_id")
	since := c.Query("since")
	
	if userID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "user_id is required",
		})
	}
	
	uid, err := uuid.Parse(userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid user_id",
		})
	}
	
	var sinceTime *time.Time
	if since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid since parameter, must be RFC3339 format",
			})
		}
		sinceTime = &t
	}
	
	// Get notifications from service - use h.notifyService instead of h.service
	notifications, err := h.notifyService.GetNotificationsSince(c.Context(), uid, sinceTime)
	if err != nil {
		log.Printf("❌ Failed to get notifications since %v: %v", sinceTime, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get notifications",
		})
	}
	
	return c.JSON(notifications)
}



// DeleteNotification - User deletes their notification
func (h *NotificationHandler) DeleteNotificationForUser(c *fiber.Ctx) error {
    userIDStr := c.Params("user_id")
    notificationIDStr := c.Params("notification_id")
    
    userID, err := uuid.Parse(userIDStr)
    if err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user_id"})
    }
    
    notificationID, err := uuid.Parse(notificationIDStr)
    if err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid notification_id"})
    }
    
    // Soft delete the notification recipient record
    err = h.notifyService.GetDB().Where("user_id = ? AND notification_id = ?", userID, notificationID).
        Delete(&models.NotificationRecipient{}).Error
    
    if err != nil {
        log.Printf("❌ DeleteNotificationForUser failed: %v", err)
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete notification"})
    }
    
    return c.JSON(fiber.Map{
        "status": "success",
        "message": "notification deleted",
    })
}

// ClearAllNotifications - User clears all their notifications
func (h *NotificationHandler) ClearAllNotifications(c *fiber.Ctx) error {
    userIDStr := c.Params("user_id")
    
    userID, err := uuid.Parse(userIDStr)
    if err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user_id"})
    }
    
    var req struct {
        NotificationIDs []uuid.UUID `json:"notification_ids"`
    }
    
    if err := c.BodyParser(&req); err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
    }
    
    // If specific IDs provided, delete only those
    if len(req.NotificationIDs) > 0 {
        err = h.notifyService.GetDB().
            Where("user_id = ? AND notification_id IN ?", userID, req.NotificationIDs).
            Delete(&models.NotificationRecipient{}).Error
    } else {
        // Delete all notifications for user
        err = h.notifyService.GetDB().
            Where("user_id = ?", userID).
            Delete(&models.NotificationRecipient{}).Error
    }
    
    if err != nil {
        log.Printf("❌ ClearAllNotifications failed: %v", err)
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to clear notifications"})
    }
    
    return c.JSON(fiber.Map{
        "status": "success",
        "message": "notifications cleared",
        "count": len(req.NotificationIDs),
    })
}

// HasUnreadNotifications - Minimal endpoint that returns true/false
func (h *NotificationHandler) HasUnreadNotifications(c *fiber.Ctx) error {
    userIDStr := c.Params("user_id")
    userID, err := uuid.Parse(userIDStr)
    if err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user_id"})
    }

    var hasUnread bool
    // Simple EXISTS query - very efficient
    err = h.notifyService.GetDB().Raw(`
        SELECT EXISTS(
            SELECT 1 
            FROM notification_recipients 
            WHERE user_id = ? 
            AND status = 'delivered'
            LIMIT 1
        )`, userID).Scan(&hasUnread).Error
    
    if err != nil {
        log.Printf("❌ HasUnreadNotifications failed: %v", err)
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to check"})
    }

    // Return minimal binary response
    return c.JSON(fiber.Map{
        "has_unread": hasUnread,
        "ts": time.Now().UTC().Unix(),
    })
}