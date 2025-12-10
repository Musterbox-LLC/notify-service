// internal/config/config.go
package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort string

	// SMTP
	SMTPUser     string
	SMTPPass     string
	SMTPFrom     string
	SMTPHost     string
	SMTPPort     int
	SMTPFromName string

	// DB
	DBHost     string
	DBPort     string
	DBUser     string
	DBPass     string
	DBName     string
	DBSSLMode  string

	// Auth
	ServiceExpectedToken string

	// R2 Storage
	R2AccountID       string
	R2AccessKeyID     string
	R2AccessKeySecret string
	R2BucketName      string
	R2PublicURL       string

	// CORS
	AllowedOrigins string

	// User Sync
	ProfileServiceURL string // <--- Keep profile service URL
	// Remove: ProfileServiceToken (we'll use ServiceExpectedToken instead)
}

func Load() *Config {
	if os.Getenv("ENV") != "production" {
		_ = godotenv.Load() // optional .env for local
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8085"
	}

	smtpPort, err := strconv.Atoi(os.Getenv("SMTP_PORT"))
	if err != nil {
		log.Fatalf("❌ Invalid SMTP_PORT: %v", err)
	}

	return &Config{
		ServerPort:      port,
		SMTPUser:        os.Getenv("SMTP_USER"),
		SMTPPass:        os.Getenv("SMTP_PASS"),
		SMTPFrom:        os.Getenv("SMTP_FROM"),
		SMTPHost:        os.Getenv("SMTP_HOST"),
		SMTPPort:        smtpPort,
		SMTPFromName:    "MusterBox Secure",

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPass:     getEnv("DB_PASS", "postgres"),
		DBName:     getEnv("DB_NAME", "notify_db"),
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),

		ServiceExpectedToken: getEnv("SERVICE_TOKEN", "your-secret-service-token"),

		// R2 Configuration
		R2AccountID:       os.Getenv("R2_ACCOUNT_ID"),
		R2AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
		R2AccessKeySecret: os.Getenv("R2_ACCESS_KEY_SECRET"),
		R2BucketName:      os.Getenv("R2_BUCKET_NAME"),
		R2PublicURL:       os.Getenv("R2_PUBLIC_URL"),

		// CORS Configuration
		AllowedOrigins: getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:3001"),

		// User Sync Configuration
		ProfileServiceURL: getEnv("PROFILE_SERVICE_URL", "http://localhost:3000"), // <--- Keep profile service URL
		// Remove: ProfileServiceToken
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}