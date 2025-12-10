// internal/transport/http/file_upload.go
package http

import (
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"notify-service/pkg/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// UploadNotificationFiles handles multipart file uploads and optional JSON metadata
// Supports:
//   - `image` (image/*) — ✅ file upload
//   - `thumbnail` (image/*) — ✅ file upload
//   - `video` — ❌ NOT allowed as file upload (only via `content_link`)
//   - Optional JSON fields: heading, title, message, type, content_link, scheduled_at (RFC3339),
//     action_links (as JSON string), metadata (as JSON string)
func (h *NotificationHandler) UploadNotificationFiles(c *fiber.Ctx) error {
	// 🔐 Auth: Require X-User-ID (creator/admin), enforced by gatewayAuth middleware
	creatorIDStr := c.Get("X-User-ID")
	if creatorIDStr == "" {
		log.Printf("[UPLOAD] ❌ Missing X-User-ID header")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "X-User-ID header required (admin context)",
		})
	}
	creatorID, err := uuid.Parse(creatorIDStr)
	if err != nil {
		log.Printf("[UPLOAD] Invalid X-User-ID: %s", creatorIDStr)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid X-User-ID"})
	}

	// --- 1. Parse form fields (non-file) ---
	heading := strings.TrimSpace(c.FormValue("heading"))
	title := strings.TrimSpace(c.FormValue("title"))
	message := strings.TrimSpace(c.FormValue("message"))
	notifType := strings.TrimSpace(c.FormValue("type"))
	if notifType == "" {
		notifType = "info"
	}
	contentLink := strings.TrimSpace(c.FormValue("content_link"))

	// Optional: scheduled_at (RFC3339)
	var scheduledAt *time.Time
	if scheduledAtStr := strings.TrimSpace(c.FormValue("scheduled_at")); scheduledAtStr != "" {
		t, err := time.Parse(time.RFC3339, scheduledAtStr)
		if err != nil {
			log.Printf("[UPLOAD] Invalid scheduled_at: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid scheduled_at format — use RFC3339 (e.g., 2025-12-10T08:00:00Z)",
			})
		}
		scheduledAt = &t
	}

	// Validate required
	if heading == "" || title == "" || message == "" {
		log.Printf("[UPLOAD] Missing required fields from creator %s", creatorID)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "heading, title, and message are required",
		})
	}

	// Optional JSON fields
	var actionLinks []models.ActionLink
	if actionLinksStr := c.FormValue("action_links"); actionLinksStr != "" {
		if err := c.App().Config().JSONDecoder([]byte(actionLinksStr), &actionLinks); err != nil {
			log.Printf("[UPLOAD] Failed to parse action_links JSON: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid action_links JSON"})
		}
	}

	var metadata map[string]interface{}
	if metaStr := c.FormValue("metadata"); metaStr != "" {
		if err := c.App().Config().JSONDecoder([]byte(metaStr), &metadata); err != nil {
			log.Printf("[UPLOAD] Failed to parse metadata JSON: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid metadata JSON"})
		}
	}

	// --- 2. Handle file uploads ---
	ctx := c.Context()
	uploadResults := make(map[string]string)

	// Helper to upload a single image-type file
	uploadImageFile := func(fileHeader *multipart.FileHeader, prefix string) (string, error) {
		if fileHeader == nil {
			return "", nil
		}

		file, err := fileHeader.Open()
		if err != nil {
			return "", fmt.Errorf("failed to open file %s: %w", fileHeader.Filename, err)
		}
		defer file.Close()

		// Sanitize extension
		ext := filepath.Ext(fileHeader.Filename)
		allowedExts := map[string]bool{
			".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
		}
		if !allowedExts[strings.ToLower(ext)] {
			return "", fmt.Errorf("unsupported image extension: %s (allowed: .jpg, .png, .gif, .webp)", ext)
		}

		key := fmt.Sprintf("%s/%s%s", prefix, uuid.New().String(), ext)
		contentType := getContentType(ext)

		log.Printf("[UPLOAD] Uploading %s (%s, %d bytes) to R2 key: %s",
			fileHeader.Filename, contentType, fileHeader.Size, key)

		content, err := io.ReadAll(file)
		if err != nil {
			return "", fmt.Errorf("failed to read file %s: %w", fileHeader.Filename, err)
		}

		if err := h.notifyService.UploadFileToR2(ctx, key, content, contentType); err != nil {
			return "", fmt.Errorf("R2 upload failed for %s: %w", fileHeader.Filename, err)
		}

		publicURL := h.notifyService.GetPublicURL(key)
		log.Printf("[UPLOAD] ✅ Uploaded %s → %s", fileHeader.Filename, publicURL)
		return publicURL, nil
	}

	// ❌ Explicitly reject video file upload
	if _, err := c.FormFile("video"); err == nil {
		log.Printf("[UPLOAD] ❌ Video file upload attempted by creator %s", creatorID)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "video file upload is not allowed. Use 'content_link' for video URLs (e.g., YouTube, Vimeo).",
		})
	}

	// ✅ Upload image
	if imageHeader, err := c.FormFile("image"); err == nil && imageHeader != nil {
		url, err := uploadImageFile(imageHeader, "notifications/images")
		if err != nil {
			log.Printf("[UPLOAD] Image upload failed: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "image upload failed: " + err.Error()})
		}
		if url != "" {
			uploadResults["image_url"] = url
		}
	}

	// ✅ Upload thumbnail
	if thumbHeader, err := c.FormFile("thumbnail"); err == nil && thumbHeader != nil {
		url, err := uploadImageFile(thumbHeader, "notifications/thumbnails")
		if err != nil {
			log.Printf("[UPLOAD] Thumbnail upload failed: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "thumbnail upload failed: " + err.Error()})
		}
		if url != "" {
			uploadResults["thumbnail_url"] = url
		}
	}

	// --- 3. Build final notification request ---
	req := &models.NotificationRequest{
		CreatorID:       &creatorID,     // ✅ Track who created it
		Heading:         heading,
		Title:           title,
		Message:         message,
		Type:            notifType,
		ContentImageURL: getStrPtr(uploadResults["image_url"]),
		ThumbnailURL:    getStrPtr(uploadResults["thumbnail_url"]), // ✅ added
		// ❌ No ContentVideoURL — video via content_link only
		ContentLink: getStrPtr(contentLink),
		ActionLinks: actionLinks,
		Metadata:    metadata,
		ScheduledAt: scheduledAt,    // ✅ scheduling support
	}

	// Add uploaded media URLs to MediaURLs for rich notifications
	mediaURLs := []string{}
	if url, ok := uploadResults["image_url"]; ok && url != "" {
		mediaURLs = append(mediaURLs, url)
	}
	if url, ok := uploadResults["thumbnail_url"]; ok && url != "" {
		mediaURLs = append(mediaURLs, url)
	}
	req.MediaURLs = mediaURLs

	// --- 4. Create notification ---
	notification, err := h.notifyService.CreateNotification(c.Context(), req)
	if err != nil {
		log.Printf("[UPLOAD] CreateNotification failed for creator %s: %v", creatorID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create notification"})
	}

	log.Printf("[UPLOAD] ✅ Notification created with ID %s by creator %s", notification.ID, creatorID)

	// --- 5. Return response with upload URLs and notification ---
	response := fiber.Map{
		"status":       "success",
		"message":      "notification and files uploaded successfully",
		"notification": notification,
	}

	// Merge upload URLs
	for k, v := range uploadResults {
		response[k] = v
	}

	return c.Status(fiber.StatusCreated).JSON(response)
}

// Helper: get content type from extension
func getContentType(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

// Helper: safely return *string
func getStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}