package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type NotificationType string

const (
	NotificationTypeGeneric        NotificationType = "generic"
	NotificationTypeActionRequired NotificationType = "action_required"
	NotificationTypeSuccess        NotificationType = "success"
	NotificationTypeWarning        NotificationType = "warning"
	NotificationTypeInfo           NotificationType = "info"
	NotificationTypePromotional    NotificationType = "promotional"
	NotificationTypeSecurity       NotificationType = "security"
	NotificationTypeVideo          NotificationType = "video"
)

// Notification is the template/draft/published notification — *one per campaign*.
type Notification struct {
	ID        uuid.UUID        `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CreatorID uuid.UUID        `json:"creator_id" gorm:"type:uuid;index;not null"` // admin/gamer who created it
	Type      NotificationType `json:"type" gorm:"type:varchar(30);not null;default:'info'"`
	Heading   string           `json:"heading" gorm:"type:varchar(100);not null"`
	Title     string           `json:"title" gorm:"type:varchar(100);not null"`
	Message   string           `json:"message" gorm:"type:text;not null"`
	// Media
	ContentImageURL *string        `json:"content_image_url,omitempty" gorm:"type:varchar(500)"` // external
	ThumbnailURL    *string        `json:"thumbnail_url,omitempty" gorm:"type:varchar(500)"`     // uploaded thumbnail
	MediaURLs       datatypes.JSON `json:"media_urls,omitempty" gorm:"type:jsonb"`               // []string (R2 URLs)
	// Interaction
	ContentLink  *string        `json:"content_link,omitempty" gorm:"type:varchar(500)"`
	ActionLinks  datatypes.JSON `json:"action_links,omitempty" gorm:"type:jsonb"` // []ActionLink
	Metadata     datatypes.JSON `json:"metadata,omitempty" gorm:"type:jsonb"`
	// Lifecycle
	IsDraft     bool       `json:"is_draft" gorm:"not null;default:true"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty" gorm:"index"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"` // when *first* sent (or nil if draft/scheduled)
	// Timestamps
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

type ActionLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	Style string `json:"style"` // "primary", "secondary", etc.
}

// EmailRequest — unchanged
type EmailRequest struct {
	UserID  uuid.UUID              `json:"user_id" validate:"required"`
	To      string                 `json:"to" validate:"required,email"`
	Type    string                 `json:"type" validate:"required,oneof=email_verification password_reset otp developer_app_received developer_app_approved developer_app_rejected developer_profile_update"`
	Context map[string]interface{} `json:"context" validate:"required"`
}

// NotificationRequest — unchanged (API input)
type NotificationRequest struct {
	Heading         string       `json:"heading" validate:"required"`
	Title           string       `json:"title" validate:"required"`
	Message         string       `json:"message" validate:"required"`
	Type            string       `json:"type,omitempty"`
	CreatorID       *uuid.UUID   `json:"creator_id,omitempty"`
	UserID          *uuid.UUID   `json:"user_id,omitempty"` // DEPRECATED in new logic (for backward compat only)
	ContentLink     *string      `json:"content_link,omitempty"`
	ActionLinks     []ActionLink `json:"action_links,omitempty"`
	Metadata        interface{}  `json:"metadata,omitempty"`
	ContentImageURL *string      `json:"content_image_url,omitempty"`
	ThumbnailURL    *string      `json:"thumbnail_url,omitempty"`
	MediaURLs       []string     `json:"media_urls,omitempty"`
	ScheduledAt     *time.Time   `json:"scheduled_at,omitempty"`
}

// ✅ Renamed & enhanced: per-user delivery state
type NotificationRecipientStatus string

const (
	RecipientStatusPending   NotificationRecipientStatus = "pending"
	RecipientStatusDelivered NotificationRecipientStatus = "delivered"
	RecipientStatusRead      NotificationRecipientStatus = "read"
	RecipientStatusFailed    NotificationRecipientStatus = "failed"
)

type NotificationRecipient struct {
	ID             uuid.UUID                   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	NotificationID uuid.UUID                   `gorm:"type:uuid;not null;index" json:"notification_id"`
	UserID         uuid.UUID                   `gorm:"type:uuid;not null;index" json:"user_id"`
	Status         NotificationRecipientStatus `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	DeliveredAt    *time.Time                  `gorm:"type:timestamptz" json:"delivered_at,omitempty"`
	ReadAt         *time.Time                  `gorm:"type:timestamptz" json:"read_at,omitempty"`
	ErrorMessage   *string                     `gorm:"type:text" json:"error_message,omitempty"`
	DeviceID       *string                     `gorm:"type:varchar(100)" json:"device_id,omitempty"`
	CreatedAt      time.Time                   `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time                   `gorm:"not null" json:"updated_at"`
}

// ✅ View model: enriched receipt for admin
type ReceiptView struct {
	UserID      uuid.UUID  `json:"user_id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	Status      string     `json:"status"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
}


type SystemNotificationTemplate struct {
    ID           uuid.UUID      `json:"id" gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    EventKey     string         `json:"event_key" gorm:"uniqueIndex;not null"`
    Name         string         `json:"name" gorm:"not null"`
    Enabled      bool           `json:"enabled" gorm:"not null;default:true"`
    Heading      string         `json:"heading"`
    Title        string         `json:"title"`
    Message      string         `json:"message"`
    Type         string         `json:"type"`
    Icon         string         `json:"icon"`
    TemplateVars datatypes.JSON `json:"template_vars" gorm:"type:jsonb"`
    CreatedAt    time.Time      `json:"created_at"`
    UpdatedAt    time.Time      `json:"updated_at"`
}
