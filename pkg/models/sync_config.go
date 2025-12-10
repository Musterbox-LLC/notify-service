// pkg/models/sync_config.go
package models


// SyncConfig stores synchronization metadata
type SyncConfig struct {
	Key   string `json:"key" gorm:"primaryKey;type:varchar(255)"`
	Value string `json:"value" gorm:"type:text"`
}

// TableName specifies the table name for SyncConfig
func (SyncConfig) TableName() string {
	return "sync_configs"
}