package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"notify-service/internal/config"
	"notify-service/internal/email"
	"notify-service/internal/fcm"
	"notify-service/internal/notification"
	"notify-service/internal/service"
	"notify-service/internal/sync"
	"notify-service/internal/transport/http"
	"notify-service/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

var startTime time.Time

func main() {
	startTime = time.Now()
	cfg := config.Load()
	log.Printf("🔧 Service expected token: %s******", cfg.ServiceExpectedToken[:6])
	notification.InitDB(cfg)

	r2Config := utils.NotificationR2Config{
		AccountID:       cfg.R2AccountID,
		AccessKeyID:     cfg.R2AccessKeyID,
		AccessKeySecret: cfg.R2AccessKeySecret,
		BucketName:      cfg.R2BucketName,
		PublicURL:       cfg.R2PublicURL,
	}
	r2Client, err := utils.NewNotificationR2Client(r2Config)
	if err != nil {
		log.Fatalf("❌ [R2] Failed to initialize client: %v", err)
	}
	log.Println("✅ [R2] Notification R2 client initialized")

	userSyncService := sync.NewUserSyncService(notification.GetDB(), cfg.ProfileServiceURL, cfg.ServiceExpectedToken)
	log.Printf("🔄 [SYNC] User sync service initialized (ProfileServiceURL: %s)", cfg.ProfileServiceURL)

	emailSender := email.NewSender(cfg)

	// Initialize FCM client
	var fcmClient *fcm.FCMClient
	fcmCredsJSON := os.Getenv("FIREBASE_CREDENTIALS_JSON")
	if fcmCredsJSON != "" {
		client, err := fcm.NewFCMClient(context.Background(), []byte(fcmCredsJSON))
		if err != nil {
			log.Fatalf("❌ Failed to initialize FCM: %v", err)
		}
		fcmClient = client
		log.Println("✅ FCM client initialized")
	} else {
		log.Println("⚠️ FCM disabled (no FIREBASE_CREDENTIALS_JSON)")
	}

	notifyService := service.NewNotifyService(emailSender, r2Client, userSyncService, fcmClient)
	handler := http.NewHandler(notifyService)
	log.Println("✅ [SERVICE] NotifyService & Handler initialized")

	// NOTE: AuthServiceURL and MS_SERVICE_TOKEN are still loaded from config/env
	// but the authClient for SSE is no longer initialized or used.
	authServiceURL := os.Getenv("AUTH_SERVICE_URL")
	msServiceToken := os.Getenv("MS_SERVICE_TOKEN")
	if authServiceURL == "" || msServiceToken == "" {
		log.Println("⚠️ AUTH_SERVICE_URL and MS_SERVICE_TOKEN are missing. SSE auth was previously required, but SSE is now removed.")
		// No longer fatal if SSE is removed
	}

	app := fiber.New(fiber.Config{
		AppName:      "notify-service",
		ErrorHandler: customErrorHandler,
	})

	app.Use(recover.New())

	allowedOrigins := getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:3001")

	// CORS configuration:
	app.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS,PATCH,HEAD",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Requested-With,X-Device-ID,X-User-ID,X-User-Roles,X-Service-Token,X-Otp-Not-Required,Cache-Control",
		ExposeHeaders:    "X-Access-Token,X-Refresh-Token,X-New-Refresh-Token,X-Otp-Not-Required,Content-Type", // Added Content-Type
		AllowCredentials: true,
		MaxAge:           86400,
	}))

	app.Use(logger.New(logger.Config{
		Format: "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path} | ${ua}\n",
	}))

	// 1. User routes (via Gateway — secured)
	gatewayUserRoutes := app.Group("/v2", gatewayAuth())
	notifHandler := handler.GetNotificationHandler()
	gatewayUserRoutes.Get("/user/:user_id", notifHandler.GetAll)
	gatewayUserRoutes.Get("/user/:user_id/since", notifHandler.GetAllSince)
	gatewayUserRoutes.Get("/user/:user_id/unread", notifHandler.GetUnread)
	gatewayUserRoutes.Post("/user/:user_id/mark-read", notifHandler.MarkRead)
	gatewayUserRoutes.Post("/user/:user_id/mark-all-read", notifHandler.MarkAllRead)
	gatewayUserRoutes.Get("/user/:user_id/has-unread", notifHandler.HasUnreadNotifications)
	gatewayUserRoutes.Delete("/user/:user_id/notifications/:notification_id", notifHandler.DeleteNotificationForUser)
	gatewayUserRoutes.Post("/user/:user_id/clear-all", notifHandler.ClearAllNotifications)
	gatewayUserRoutes.Post("/user/:user_id/fcm-token", notifHandler.RegisterFCMToken)     // Add FCM token registration
	gatewayUserRoutes.Delete("/user/:user_id/fcm-token", notifHandler.UnregisterFCMToken) // Add FCM token unregistration
	log.Println("✅ [ROUTES] Registered user routes: /v1/notify/s/user/:user_id*")

	// 2. Admin routes (via Gateway + admin role)
	gatewayAdminRoutes := app.Group("/admin", gatewayAuth(), adminRoleAuth())
	gatewayAdminRoutes.Get("/users", notifHandler.GetAllUsers)
	gatewayAdminRoutes.Get("/notifications", notifHandler.GetAllNotificationsAdmin)
	gatewayAdminRoutes.Post("/notifications", notifHandler.CreateNotification)
	gatewayAdminRoutes.Post("/upload", notifHandler.UploadNotificationFiles)
	gatewayAdminRoutes.Put("/notifications/:id", notifHandler.UpdateNotification)
	gatewayAdminRoutes.Delete("/notifications/:id", notifHandler.DeleteNotification)
	gatewayAdminRoutes.Post("/notifications/:id/publish", notifHandler.PublishNotification)
	gatewayAdminRoutes.Post("/notifications/:id/schedule", notifHandler.ScheduleNotification)
	gatewayAdminRoutes.Post("/notifications/:id/unschedule", notifHandler.UnscheduleNotification)
	gatewayAdminRoutes.Get("/notifications/history", notifHandler.GetNotificationHistory)
	gatewayAdminRoutes.Post("/notifications/bulk", notifHandler.BulkDeliverNotification)
	gatewayAdminRoutes.Get("/notifications/:id/receipts", notifHandler.GetNotificationReceipts)
	gatewayAdminRoutes.Get("/system-templates/", notifHandler.GetSystemTemplates)
	gatewayAdminRoutes.Patch("/system-templates/:event_key", notifHandler.UpdateSystemTemplate)

	log.Println("✅ [ROUTES] Registered admin routes: /admin/*")

	// 3. Service-to-service routes
	serviceRoutes := app.Group("/svc/v1", serviceAuth(cfg))
	serviceRoutes.Post("/notify/email", handler.SendEmail)
	serviceRoutes.Post("/notifications/trigger", notifHandler.TriggerSystemNotification)
	serviceRoutes.Post("/notifications", notifHandler.CreateNotification)
	log.Println("✅ [ROUTES] Registered service routes: /svc/v1/notify/email, /notifications")

	// 4. Sync routes
	syncRoutes := app.Group("/svc/v1/sync", serviceAuth(cfg))
	syncRoutes.Get("/users", func(c *fiber.Ctx) error {
		sinceStr := c.Query("since")
		log.Printf("[SYNC] Request to sync users since: %q", sinceStr)
		if sinceStr == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Query parameter 'since' is required (format: RFC3339)",
			})
		}
		sinceTime, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("Invalid 'since' format. Expected RFC3339, got: %s", sinceStr),
			})
		}
		if err := userSyncService.SyncUsersSince(c.Context(), sinceTime); err != nil {
			log.Printf("[SYNC] ❌ Sync failed: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("Failed to sync users: %v", err),
			})
		}
		log.Println("[SYNC] ✅ Users synced successfully")
		return c.JSON(fiber.Map{
			"status":  "success",
			"message": "Users synced successfully",
		})
	})
	log.Println("✅ [ROUTES] Registered sync route: /svc/v1/sync/users")

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		uptime := time.Since(startTime).Round(time.Second)
		return c.JSON(fiber.Map{
			"status":      "ok",
			"service":     "notify-service",
			"uptime":      uptime.String(),
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
			"profile_url": cfg.ProfileServiceURL,
			"fcm_enabled": fcmClient != nil, // Show FCM status instead of SSE
		})
	})
	log.Println("✅ [ROUTES] Registered /health")

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-c
		log.Println("🛑 [SHUTDOWN] Graceful shutdown initiated...")
		if err := app.Shutdown(); err != nil {
			log.Printf("❌ [SHUTDOWN] Error: %v", err)
		}
	}()

	log.Printf("🚀 notify-service starting...")
	log.Printf("   🔗 Listening on port: %s", cfg.ServerPort)
	log.Printf("   🌐 CORS allowed origins: %s", allowedOrigins)
	log.Printf("   🌐 CORS with credentials: ENABLED")
	log.Printf("   📦 R2 bucket: %s", cfg.R2BucketName)
	log.Printf("   🔄 Profile sync URL: %s", cfg.ProfileServiceURL)
	log.Printf("   🛡️  Service token prefix: %s******", cfg.ServiceExpectedToken[:6])
	log.Println("✅ Server ready.")

	if err := app.Listen(":" + cfg.ServerPort); err != nil {
		log.Fatalf("❌ [STARTUP] Server failed to start: %v", err)
	}
}

func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	var errMsg string
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		errMsg = e.Message
	} else {
		errMsg = err.Error()
	}
	log.Printf("🔥 [ERROR] [%d] %s %s → %v | IP=%s | UA=%s",
		code,
		c.Method(),
		c.Path(),
		errMsg,
		c.IP(),
		c.Get("User-Agent"),
	)
	return c.Status(code).JSON(fiber.Map{
		"error":      "something went wrong",
		"request_id": c.Get("X-Request-ID"),
	})
}

func serviceAuth(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Get("X-Service-Token")
		if token == "" {
			authHeader := c.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}
		maskedToken := "<empty>"
		if token != "" {
			if len(token) > 6 {
				maskedToken = token[:6] + "..."
			} else {
				maskedToken = token
			}
		}
		if token != cfg.ServiceExpectedToken {
			log.Printf("[SERVICE-AUTH] ❌ REJECTED | IP=%s | Path=%s | Token=%s",
				c.IP(), c.Path(), maskedToken)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized: invalid or missing service token",
			})
		}
		log.Printf("[SERVICE-AUTH] ✅ ACCEPTED | IP=%s | Path=%s", c.IP(), c.Path())
		return c.Next()
	}
}

func gatewayAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Get("X-User-ID")
		deviceID := c.Get("X-Device-ID")
		if userID == "" || deviceID == "" {
			log.Printf("[GATEWAY-AUTH] ❌ REJECTED | IP=%s | Path=%s | UserID=%q | DeviceID=%q",
				c.IP(), c.Path(), userID, deviceID)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized: missing user/device context from Gateway",
			})
		}
		log.Printf("[GATEWAY-AUTH] ✅ ACCEPTED | UserID=%s | DeviceID=%s | IP=%s | Path=%s",
			userID, deviceID, c.IP(), c.Path())
		return c.Next()
	}
}

func adminRoleAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		userRolesHeader := c.Get("X-User-Roles")
		if userRolesHeader == "" {
			log.Printf("[ADMIN-AUTH] ❌ REJECTED (no roles) | Path=%s", c.Path())
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Forbidden: missing user roles from Gateway",
			})
		}
		userRoles := strings.Split(userRolesHeader, ",")
		hasAdminRole := false
		for _, role := range userRoles {
			if strings.ToLower(strings.TrimSpace(role)) == "admin" {
				hasAdminRole = true
				break
			}
		}
		if !hasAdminRole {
			log.Printf("[ADMIN-AUTH] ❌ REJECTED (no admin) | Roles=%v | Path=%s",
				userRoles, c.Path())
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Forbidden: admin role required",
			})
		}
		log.Printf("[ADMIN-AUTH] ✅ ACCEPTED | Roles=%v | Path=%s", userRoles, c.Path())
		return c.Next()
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}