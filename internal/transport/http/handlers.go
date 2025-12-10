// internal/transport/http/handlers.go
package http

import (
	"log"
	"notify-service/internal/service"
	"notify-service/pkg/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type Handler struct {
	notifyService *service.NotifyService
}

func NewHandler(notifyService *service.NotifyService) *Handler {
	return &Handler{notifyService: notifyService}
}

func (h *Handler) GetNotificationHandler() *NotificationHandler {
	return NewNotificationHandler(h.notifyService)
}

func (h *Handler) SendEmail(c *fiber.Ctx) error {
	var req models.EmailRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}

	if req.UserID == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_id required"})
	}

	log.Printf("📬 [EMAIL REQUEST] From: %s | User: %s | Type: %s", c.Locals("device_id"), req.UserID, req.Type)

	err := h.notifyService.SendEmail(c.Context(), &req)
	if err != nil {
		log.Printf("❌ SendEmail failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to queue email"})
	}

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"status":  "queued",
		"message": "Email queued for delivery",
	})
}