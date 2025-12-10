// pkg/models/models.go (or add to existing file)
package models

import (
	"time"
	"gorm.io/gorm"
)

// User represents the user data from profile service (stored locally in notification service)
type User struct {
	ID                string  `json:"id" gorm:"primaryKey;type:varchar(36)"` // UUID as string
	Username          string  `json:"username" gorm:"type:varchar(100);not null;index"`
	Email             string  `json:"email" gorm:"type:varchar(255);not null;index"`
	FirstName         *string `json:"first_name,omitempty" gorm:"type:varchar(100)"`
	LastName          *string `json:"last_name,omitempty" gorm:"type:varchar(100)"`
	ProfilePictureURL *string `json:"profile_picture_url,omitempty" gorm:"type:varchar(500)"`
	UpdatedAt         time.Time `json:"updated_at"`
	CreatedAt         time.Time `json:"created_at"`
	DeletedAt         gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

// TableName specifies the table name for User
func (User) TableName() string {
	return "users"
}