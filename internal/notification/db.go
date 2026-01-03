// internal/notification/db.go
package notification

import (
	"fmt"
	"log"
	"notify-service/internal/config"
	"notify-service/pkg/models"
	
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
	err = db.AutoMigrate(
		&models.SyncConfig{}, 
		&models.Notification{}, 
		&models.NotificationRecipient{}, 
		&models.User{}, 
		&models.SystemNotificationTemplate{}, 
		&models.FCMToken{},
	)
	if err != nil {
		log.Fatalf("❌ Failed to migrate: %v", err)
	}

	log.Println("✅ Notification DB connected & migrated")

	// ✅ After migration, check and create unique constraint if it doesn't exist
	if err := ensureFCMTokenConstraints(db); err != nil {
		log.Printf("⚠️ Failed to ensure FCM token constraints: %v", err)
	} else {
		log.Println("✅ FCM token constraints ensured")
	}

	// ✅ Seed system templates after migration
	if err := seedSystemNotificationTemplates(db); err != nil {
		log.Printf("⚠️ Failed to seed system notification templates: %v", err)
	} else {
		log.Println("✅ System notification templates seeded")
	}
}

func ensureFCMTokenConstraints(db *gorm.DB) error {
	// Check if the unique constraint already exists
	var count int64
	err := db.Raw(`
		SELECT COUNT(*)
		FROM information_schema.table_constraints 
		WHERE table_name = 'fcm_tokens' 
		AND constraint_name = 'fcm_tokens_user_id_device_id_key'
	`).Scan(&count).Error
	
	if err != nil {
		return fmt.Errorf("failed to check constraint: %w", err)
	}

	if count == 0 {
		// Create the unique constraint
		err = db.Exec(`
			ALTER TABLE fcm_tokens 
			ADD CONSTRAINT fcm_tokens_user_id_device_id_key 
			UNIQUE (user_id, device_id)
		`).Error
		
		if err != nil {
			return fmt.Errorf("failed to add constraint: %w", err)
		}
		log.Println("🛠️ Added unique constraint on fcm_tokens(user_id, device_id)")
	} else {
		log.Println("✅ Unique constraint already exists on fcm_tokens(user_id, device_id)")
	}

	return nil
}

func GetDB() *gorm.DB {
	return db
}