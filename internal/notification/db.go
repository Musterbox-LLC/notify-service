// internal/notification/db.go
package notification

import (
	"log"
	"notify-service/internal/config"
	"notify-service/pkg/models"
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var db *gorm.DB

func InitDB(cfg *config.Config) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Africa/Lagos",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPass, cfg.DBName, cfg.DBSSLMode,
	)

	var err error
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to DB: %v", err)
	}

	// Auto-migrate (safe in dev; use migrations in prod)
	err = db.AutoMigrate( &models.SyncConfig{}, &models.Notification{}, &models.NotificationRecipient{}, &models.User{}, &models.SystemNotificationTemplate{})
	if err != nil {
		log.Fatalf("❌ Failed to migrate: %v", err)
	}

	log.Println("✅ Notification DB connected & migrated")

	// ✅ Seed system templates after migration
	if err := seedSystemNotificationTemplates(db); err != nil {
		log.Printf("⚠️ Failed to seed system notification templates: %v", err)
	} else {
		log.Println("✅ System notification templates seeded")
	}
}

func GetDB() *gorm.DB {
	return db
}


