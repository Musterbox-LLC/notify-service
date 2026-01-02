// internal/middleware/sse_auth.go
package middleware

import (
	"log"
	"strings"

	"notify-service/internal/service" // Import the new services package
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// Context keys for user information (using string keys for Fiber Locals)
const (
	UserIDContextKey   = "userID"
	DeviceIDContextKey = "deviceID"
	// Add other keys as needed
)

// SSEAuthMiddleware validates accessToken & deviceID from query params via auth-service /validate.
// Expects:
//   ?token=abc123&device_id=dev_xyz
//
// On success:
//   - sets context: userID, deviceID
//   - continues
// On failure:
//   - returns 401
func SSEAuthMiddleware(authClient *service.AuthServiceClient) fiber.Handler { // Take client as dependency
	return func(c *fiber.Ctx) error {
		log.Printf("[SSEAuth] Processing auth for path: %s, RemoteAddr: %s", c.Path(), c.IP())
		log.Printf("  -> Query: %s", c.Request().URI().QueryString())

		// 🔍 Extract from query (EventSource can’t set headers)
		accessToken := strings.TrimSpace(c.Query("token"))
		deviceID := strings.TrimSpace(c.Query("device_id"))

		log.Printf("  -> Extracted Token (len): %d", len(accessToken))
		log.Printf("  -> Extracted DeviceID: '%s'", deviceID)

		if accessToken == "" || deviceID == "" {
			log.Printf("[SSEAuth] ❌ Missing query params: token='%s', device_id='%s'", accessToken, deviceID)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Missing token or device_id in query",
			})
		}

		// ✅ Call /validate on auth service using the pre-initialized client
		resp, err := authClient.ValidateToken(accessToken, deviceID)
		if err != nil {
			log.Printf("[SSEAuth] ❌ Validation failed for token (prefix: %s...), device %s: %v",
				accessToken[:min(10, len(accessToken))], deviceID, err)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized: invalid token or device",
			})
		}

		// Validate the user_id returned by auth service is a valid UUID
		parsedUserID, err := uuid.Parse(resp.UserID)
		if err != nil {
			log.Printf("[SSEAuth] ❌ Invalid user_id returned from auth service: %s, error: %v", resp.UserID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Internal server error during authentication",
			})
		}

		// ✅ Success: set locals (Fiber's way of passing data in context)
		c.Locals(UserIDContextKey, parsedUserID.String())
		c.Locals(DeviceIDContextKey, resp.DeviceID)

		log.Printf("[SSEAuth] ✅ Authenticated user %s (device %s)", parsedUserID.String(), resp.DeviceID)

		return c.Next()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Helper functions to retrieve values from context
func GetUserIDFromContext(c *fiber.Ctx) (string, bool) {
	value := c.Locals(UserIDContextKey)
	userID, ok := value.(string)
	if !ok {
		log.Printf("[SSEAuth] GetUserIDFromContext: FAILED to retrieve userID from context, ok=%t, value=%v", ok, value)
	}
	return userID, ok
}

func GetDeviceIDFromContext(c *fiber.Ctx) (string, bool) {
	value := c.Locals(DeviceIDContextKey)
	deviceID, ok := value.(string)
	if !ok {
		log.Printf("[SSEAuth] GetDeviceIDFromContext: FAILED to retrieve deviceID from context, ok=%t, value=%v", ok, value)
	}
	return deviceID, ok
}