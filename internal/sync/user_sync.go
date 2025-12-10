// internal/sync/user_sync.go
package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
	"gorm.io/gorm"
)

// User represents the user data from profile service
type User struct {
	ID                string  `json:"id" gorm:"primaryKey"`
	Username          string  `json:"username"`
	Email             string  `json:"email"`
	FirstName         *string `json:"first_name,omitempty"`
	LastName          *string `json:"last_name,omitempty"`
	ProfilePictureURL *string `json:"profile_picture_url,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// SyncConfig stores synchronization metadata
type SyncConfig struct {
	Key   string `json:"key" gorm:"primaryKey;type:varchar(255)"`
	Value string `json:"value" gorm:"type:text"`
}

// UserSyncService handles user synchronization
type UserSyncService struct {
	db            *gorm.DB
	profileAPIURL string
	serviceToken  string // <--- Use service token
}

func NewUserSyncService(db *gorm.DB, profileAPIURL, serviceToken string) *UserSyncService {
	service := &UserSyncService{
		db:            db,
		profileAPIURL: profileAPIURL,
		serviceToken:  serviceToken, // <--- Use service token
	}
	
	// Auto migrate sync config table
	if err := db.AutoMigrate(&SyncConfig{}); err != nil {
		log.Printf("❌ Failed to migrate sync config table: %v", err)
	}
	
	// Start the sync scheduler
	go service.StartSyncScheduler()
	
	return service
}

// StartSyncScheduler starts the background sync processes
func (s *UserSyncService) StartSyncScheduler() {
	// Schedule lunch sync (12:00 PM daily)
	go s.scheduleLunchSync()
	
	// Schedule continuous 20-second updates
	go s.scheduleContinuousSync()
}

// scheduleLunchSync schedules a sync at lunch time (12:00 PM) daily
func (s *UserSyncService) scheduleLunchSync() {
	for {
		now := time.Now()
		lunchTime := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
		
		// If it's already past lunch today, schedule for tomorrow
		if now.After(lunchTime) {
			lunchTime = lunchTime.AddDate(0, 0, 1)
		}
		
		waitDuration := lunchTime.Sub(now)
		
		log.Printf("⏰ Scheduled lunch sync for: %s (in %v)", lunchTime.Format(time.RFC3339), waitDuration)
		
		// Wait until lunch time
		time.Sleep(waitDuration)
		
		// Perform lunch sync
		ctx := context.Background()
		log.Println("🍽️ Starting lunch time user sync...")
		if err := s.SyncUsersSince(ctx, time.Time{}); err != nil { // Sync all users since beginning of time
			log.Printf("❌ Lunch sync failed: %v", err)
		} else {
			log.Println("✅ Lunch sync completed successfully")
		}
		
		// Small delay to prevent multiple triggers
		time.Sleep(1 * time.Minute)
	}
}

// scheduleContinuousSync performs continuous 20-second updates
func (s *UserSyncService) scheduleContinuousSync() {
	ticker := time.NewTicker(10 * time.Second) // Changed to 10 seconds as requested
	defer ticker.Stop()
	
	for range ticker.C {
		ctx := context.Background()
		log.Println("🔄 Starting 10-second update sync...")
		
		// Get last sync time, if not exists, sync from beginning
		lastSyncTime, err := s.getLastSyncTime()
		if err != nil {
			log.Printf("⚠️ Could not get last sync time, syncing from beginning: %v", err)
			lastSyncTime = time.Time{} // Sync from beginning of time
		}
		
		if err := s.SyncUsersSince(ctx, lastSyncTime); err != nil {
			log.Printf("❌ 10-second sync failed: %v", err)
		} else {
			log.Println("✅ 10-second sync completed successfully")
		}
	}
}

// SyncUsersSince fetches and syncs users updated since a specific time
func (s *UserSyncService) SyncUsersSince(ctx context.Context, since time.Time) error {
	// Format time using RFC3339 which is the expected format
	// Convert to UTC to ensure consistent formatting
	var sinceFormatted string
	var isFullSync bool
	
	if since.IsZero() {
		// If since is zero time (beginning of time), don't include it in the query
		// This will fetch all users
		log.Printf("🔄 Starting full user sync (fetching all users)")
		isFullSync = true
	} else {
		sinceUTC := since.UTC()
		sinceFormatted = sinceUTC.Format(time.RFC3339)
		log.Printf("🔄 Starting user sync from: %s", sinceFormatted)
		isFullSync = false
	}

	// Call profile service API to get updated users
	users, err := s.fetchUsersFromProfileService(isFullSync, sinceFormatted)
	if err != nil {
		return fmt.Errorf("failed to fetch users from profile service: %w", err)
	}

	log.Printf("📥 Retrieved %d users from profile service", len(users))

	// Sync users to local DB
	for _, user := range users {
		if err := s.syncUserToDB(ctx, user); err != nil {
			log.Printf("⚠️ Failed to sync user %s: %v", user.ID, err)
			continue
		}
	}

	log.Printf("✅ User sync completed for %d users", len(users))
	
	// Update last sync time to now (only if this wasn't a full sync for the lunch sync)
	if !isFullSync || time.Since(since) > 24*time.Hour { // Update if not a lunch full sync
		if err := s.updateLastSyncTime(time.Now()); err != nil {
			log.Printf("⚠️ Failed to update last sync time: %v", err)
		} else {
			log.Printf("✅ Last sync time updated to: %s", time.Now().Format(time.RFC3339))
		}
	}

	return nil
}

// fetchUsersFromProfileService calls the profile sync API
func (s *UserSyncService) fetchUsersFromProfileService(isFullSync bool, since string) ([]User, error) {
	var url string
	if isFullSync {
		// Fetch all users (no since parameter) - use a very old date to get all users
		// This handles cases where the profile service requires the since parameter
		veryOldTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		url = fmt.Sprintf("%s/api/v1/public/profiles?since=%s", s.profileAPIURL, veryOldTime.Format(time.RFC3339))
	} else {
		// Fetch users updated since the specified time
		url = fmt.Sprintf("%s/api/v1/public/profiles?since=%s", s.profileAPIURL, since)
	}
	
	log.Printf("🌐 Fetching users from: %s", url)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	// Use service token for authentication with profile service
	req.Header.Set("X-Service-Token", s.serviceToken) // <--- Use service token
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		// Read the response body to see the error details
		body, _ := io.ReadAll(resp.Body)
		log.Printf("❌ Profile service error response: %s", string(body))
		return nil, fmt.Errorf("profile service returned status: %d, body: %s", resp.StatusCode, string(body))
	}
	
	// The response body is successful, so read it
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	log.Printf("✅ Profile service returned: %s", string(body))
	
	var response struct {
		Users []User `json:"users"`
	}
	
	if err := json.Unmarshal(body, &response); err != nil {
		log.Printf("❌ Failed to unmarshal JSON response: %v", err)
		log.Printf("Raw response: %s", string(body))
		return nil, fmt.Errorf("failed to unmarshal JSON response: %w", err)
	}
	
	return response.Users, nil
}

// syncUserToDB saves/updates user in local DB
func (s *UserSyncService) syncUserToDB(ctx context.Context, user User) error {
	// Check if user exists
	var existingUser User
	result := s.db.WithContext(ctx).Where("id = ?", user.ID).First(&existingUser)
	
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// Create new user
			return s.db.WithContext(ctx).Create(&user).Error
		}
		return result.Error
	}
	
	// Update existing user only if the record is newer
	if user.UpdatedAt.After(existingUser.UpdatedAt) {
		existingUser.Username = user.Username
		existingUser.Email = user.Email
		existingUser.FirstName = user.FirstName
		existingUser.LastName = user.LastName
		existingUser.ProfilePictureURL = user.ProfilePictureURL
		existingUser.UpdatedAt = user.UpdatedAt
		
		return s.db.WithContext(ctx).Save(&existingUser).Error
	}
	
	return nil // No update needed
}

// getLastSyncTime retrieves the last sync time from the database
func (s *UserSyncService) getLastSyncTime() (time.Time, error) {
	var config SyncConfig
	result := s.db.Where("key = ?", "last_user_sync_time").First(&config)
	
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// Return zero time if no record exists (meaning never synced before)
			log.Printf("⚠️ No last sync time found, will perform full sync")
			return time.Time{}, nil
		}
		return time.Time{}, result.Error
	}
	
	// Parse the stored time
	parsedTime, err := time.Parse(time.RFC3339, config.Value)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse sync time: %w", err)
	}
	
	return parsedTime, nil
}

// updateLastSyncTime updates the last sync time in the database
func (s *UserSyncService) updateLastSyncTime(syncTime time.Time) error {
	config := SyncConfig{
		Key:   "last_user_sync_time",
		Value: syncTime.UTC().Format(time.RFC3339),
	}
	
	// Use FirstOrCreate to handle the upsert properly
	var existingConfig SyncConfig
	result := s.db.Where("key = ?", "last_user_sync_time").First(&existingConfig)
	
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// Create new record
			log.Printf("📝 Creating new sync config record")
			return s.db.Create(&config).Error
		}
		log.Printf("❌ Error finding existing config: %v", result.Error)
		return result.Error
	}
	
	// Update existing record
	log.Printf("📝 Updating existing sync config record")
	return s.db.Model(&existingConfig).Update("value", config.Value).Error
}

// GetUserByID retrieves a user by ID from local DB
func (s *UserSyncService) GetUserByID(ctx context.Context, userID string) (*User, error) {
	var user User
	err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByUsername retrieves a user by username from local DB
func (s *UserSyncService) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := s.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}