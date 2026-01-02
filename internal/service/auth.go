// internal/services/auth.go
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type AuthServiceClient struct {
	BaseURL      string
	ServiceToken string // The token this service uses to call the auth service (e.g., MS_SERVICE_TOKEN)
	HTTPClient   *http.Client
}

type ValidateRequest struct {
	// Body is typically empty for /validate, data comes via headers
}

type ValidateResponse struct {
	Authenticated           bool   `json:"authenticated"`
	UserID                  string `json:"user_id"` // UUID string
	DeviceID                string `json:"device_id"`
	OTPNotRequiredForDevice bool   `json:"otp_not_required_for_device"` // Might be useful
	// Add other fields returned by your auth service if needed
}

func NewAuthServiceClient(baseURL, serviceToken string) *AuthServiceClient {
	return &AuthServiceClient{
		BaseURL:    baseURL,
		ServiceToken: serviceToken, // This is the token for calling auth service (e.g., MS_SERVICE_TOKEN)
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second, // Adjust timeout as needed
		},
	}
}

// ValidateToken calls POST /validate on auth service with X-Access-Token and X-Device-ID
func (c *AuthServiceClient) ValidateToken(accessToken, deviceID string) (*ValidateResponse, error) {
	url := fmt.Sprintf("%s/validate", strings.TrimSuffix(c.BaseURL, "/"))

	reqBody := ValidateRequest{} // Often empty
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.ServiceToken)              // This service's token to call auth service (e.g., MS_SERVICE_TOKEN)
	req.Header.Set("X-Access-Token", accessToken)                // The user's access token
	req.Header.Set("X-Device-ID", deviceID)                      // The user's device ID

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to auth service failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[AuthServiceClient] Validation failed: %d", resp.StatusCode)
		return nil, fmt.Errorf("auth service returned %d", resp.StatusCode)
	}

	var result ValidateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response from auth service: %w", err)
	}

	if !result.Authenticated {
		return nil, fmt.Errorf("token validation failed: not authenticated by auth service")
	}

	return &result, nil
}