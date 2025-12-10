// internal/transport/http/email_handler.go
package http

import (
	"notify-service/internal/email"
	"github.com/gofiber/fiber/v2"
)

type EmailHandler struct {
	sender *email.Sender
}

func NewEmailHandler(sender *email.Sender) *EmailHandler {
	return &EmailHandler{sender: sender}
}

func (h *EmailHandler) SendEmail(c *fiber.Ctx) error {
	var req email.EmailRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := h.sender.SendEmail(c.Context(), &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "queued"})
}