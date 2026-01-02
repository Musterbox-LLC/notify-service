package http

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"notify-service/internal/middleware"
	"notify-service/internal/service"
	"notify-service/internal/sse"
	"notify-service/pkg/models"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
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
	limit := getQueryInt(c, "limit", 20, 1, 100)
	offset := getQueryInt(c, "offset", 0, 0, 10000)
	notifications, err := h.notifyService.GetAllNotifications(c.Context(), userID, limit, offset)
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


func (h *NotificationHandler) StreamNotifications(c *fiber.Ctx) error {
    // ------------------------------------------------------------
    // 1. Retrieve authenticated context (set by SSEAuthMiddleware)
    // ------------------------------------------------------------
    userIDStr, ok := middleware.GetUserIDFromContext(c)
    if !ok {
        return c.Status(fiber.StatusInternalServerError).
            JSON(fiber.Map{"error": "User ID not found in context"})
    }
    userID, err := uuid.Parse(userIDStr)
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).
            JSON(fiber.Map{"error": "Invalid user ID in context"})
    }
    deviceID, _ := middleware.GetDeviceIDFromContext(c)

    // 🕒 Capture connection start time
    connStart := time.Now()
    log.Printf("✅ [SSE] 🟢 Connection STARTED for user=%s, device=%s at %s", userID, deviceID, connStart.Format(time.RFC3339Nano))

    // ------------------------------------------------------------
    // 2. CRITICAL: Set SSE headers BEFORE SetBodyStreamWriter
    // ------------------------------------------------------------
    c.Set("Content-Type", "text/event-stream")
    c.Set("Cache-Control", "no-cache")
    c.Set("Connection", "keep-alive")
    c.Set("X-Accel-Buffering", "no")
    c.Set("Transfer-Encoding", "chunked")
    
    // IMPORTANT: Set CORS headers
    origin := c.Get("Origin")
    if origin != "" {
        c.Set("Access-Control-Allow-Origin", origin)
        c.Set("Access-Control-Allow-Credentials", "true")
    }

    // ------------------------------------------------------------
    // 3. Register SSE client BEFORE streaming
    // ------------------------------------------------------------
    broker := h.notifyService.GetSSEBroker()
    clientChan := make(chan sse.Event, 10)
    broker.Register(userID, clientChan)
    
    // Defer cleanup
    defer func() {
        broker.Unregister(userID, clientChan)
        close(clientChan)
        duration := time.Since(connStart)
        log.Printf("🔌 [SSE] 🔴 Connection CLOSED for user=%s after %v", userID, duration)
    }()

    // ------------------------------------------------------------
    // 4. Use Fiber's SetBodyStreamWriter with proper implementation
    // ------------------------------------------------------------
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		flusher, ok := any(w).(interface{ Flush() error })
		if !ok {
			log.Printf("⚠️ [SSE] Writer doesn't support Flush() for user=%s", userID)
			return
		}

        log.Printf("📡 [SSE] Starting stream writer for user=%s", userID)

        // ------------------------------------------------------------
        // 5. Initial snapshot
        // ------------------------------------------------------------
        initial, err := h.notifyService.GetAllNotifications(c.Context(), userID, 50, 0)
        if err != nil {
            log.Printf("⚠️ [SSE] Failed to fetch initial notifications for %s: %v", userID, err)
        } else {
            log.Printf("📥 [SSE] Sending %d initial notifications for user=%s", len(initial), userID)

            // Send notifications in chronological order (oldest to newest)
            for i := len(initial) - 1; i >= 0; i-- {
                n := initial[i]
                
                // Format SSE message properly
                message := fmt.Sprintf("event: notification.created\ndata: %s\n\n", toJSON(n))
                
                // Write and flush
                if _, err := w.Write([]byte(message)); err != nil {
                    log.Printf("⚠️ [SSE] Failed to write initial notification %s: %v", n.ID, err)
                    return
                }
                
                if err := flusher.Flush(); err != nil {
                    log.Printf("⚠️ [SSE] Failed to flush initial notification %s: %v", n.ID, err)
                    return
                }
                
                // Small delay to prevent overwhelming client
                time.Sleep(10 * time.Millisecond)
            }
        }

        // ------------------------------------------------------------
        // 6. Send 'ready' event
        // ------------------------------------------------------------
        readyPayload := map[string]interface{}{
            "status":  "ready",
            "at":      time.Now().Format(time.RFC3339Nano),
            "message": "SSE connection established successfully",
            "user_id": userID.String(),
        }
        readyJSON, _ := json.Marshal(readyPayload)
        readyMessage := fmt.Sprintf("event: ready\ndata: %s\n\n", readyJSON)
        
        log.Printf("✅ [SSE] → user=%s | event=ready", userID)
        
        if _, err := w.Write([]byte(readyMessage)); err != nil {
            log.Printf("⚠️ [SSE] Failed to write ready event: %v", err)
            return
        }
        
        if err := flusher.Flush(); err != nil {
            log.Printf("⚠️ [SSE] Failed to flush ready event: %v", err)
            return
        }

        // ------------------------------------------------------------
        // 7. Stream loop with proper error handling
        // ------------------------------------------------------------
        done := c.Context().Done()
        heartbeat := time.NewTicker(30 * time.Second)
        defer heartbeat.Stop()

        for {
            select {
            case <-done:
                log.Printf("🔌 [SSE] Context done for user=%s", userID)
                return

            case event := <-clientChan:
                if event.Data == nil {
                    continue
                }
                
                eventJSON, err := json.Marshal(event.Data)
                if err != nil {
                    log.Printf("⚠️ [SSE] Failed to marshal event data: %v", err)
                    continue
                }
                
                // Format SSE message
                message := fmt.Sprintf("event: %s\ndata: %s\n\n", event.Type, eventJSON)
                
                // Log for debugging
                log.Printf("📡 [SSE] → user=%s | event=%s | data_length=%d", 
                    userID, event.Type, len(eventJSON))

                // Write event
                if _, err := w.Write([]byte(message)); err != nil {
                    log.Printf("⚠️ [SSE] Write error for user=%s: %v", userID, err)
                    return
                }
                
                // Flush immediately
                if err := flusher.Flush(); err != nil {
                    log.Printf("⚠️ [SSE] Flush error for user=%s: %v", userID, err)
                    return
                }

            case <-heartbeat.C:
                log.Printf("💓 [SSE] → user=%s | sending heartbeat", userID)
                
                // Send heartbeat as comment (not an event)
                heartbeatMsg := ": heartbeat\n\n"
                if _, err := w.Write([]byte(heartbeatMsg)); err != nil {
                    log.Printf("⚠️ [SSE] Failed to write heartbeat: %v", err)
                    return
                }
                
                if err := flusher.Flush(); err != nil {
                    log.Printf("⚠️ [SSE] Failed to flush heartbeat: %v", err)
                    return
                }
            }
        }
    })

    return nil
}