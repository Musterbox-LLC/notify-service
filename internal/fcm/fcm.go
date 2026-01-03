// internal/fcm/fcm.go
package fcm

import (
	"context"
	"fmt"
	"log"
	"strconv"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

type FCMClient struct {
	client *messaging.Client
}

func NewFCMClient(ctx context.Context, credentialsJSON []byte) (*FCMClient, error) {
	conf := &firebase.Config{}
	app, err := firebase.NewApp(ctx, conf, option.WithCredentialsJSON(credentialsJSON))
	if err != nil {
		return nil, fmt.Errorf("firebase init failed: %w", err)
	}

	messagingClient, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("messaging client init failed: %w", err)
	}

	return &FCMClient{client: messagingClient}, nil
}

// convertDataToStringMap safely converts map[string]interface{} → map[string]string
func convertDataToStringMap(data map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range data {
		switch val := v.(type) {
		case string:
			result[k] = val
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			result[k] = fmt.Sprintf("%d", val)
		case float32, float64:
			result[k] = fmt.Sprintf("%f", val)
		case bool:
			result[k] = strconv.FormatBool(val)
		default:
			result[k] = fmt.Sprintf("%v", val) // fallback to string representation
		}
	}
	return result
}

func intPtr(i int) *int {
	return &i
}

func (f *FCMClient) SendToToken(ctx context.Context, token string, title, body string, data map[string]interface{}) error {
	stringData := convertDataToStringMap(data)

	badge := intPtr(1) // ✅ *int

	message := &messaging.Message{
		Token: token,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: stringData, // ✅ map[string]string
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound: "default",
					Badge: badge, // ✅ *int
				},
			},
		},
		Android: &messaging.AndroidConfig{
			Notification: &messaging.AndroidNotification{
				Sound: "default",
			},
			Priority: "high",
		},
	}

	resp, err := f.client.Send(ctx, message)
	if err != nil {
		return fmt.Errorf("FCM send failed: %w", err)
	}
	log.Printf("✅ FCM sent to %s → msg ID: %s", maskToken(token), resp)
	return nil
}

func (f *FCMClient) SendToMultipleTokens(ctx context.Context, tokens []string, title, body string, data map[string]interface{}) error {
	if len(tokens) == 0 {
		return nil
	}

	stringData := convertDataToStringMap(data)
	badge := intPtr(1)

	var messages []*messaging.Message
	for _, token := range tokens {
		messages = append(messages, &messaging.Message{
			Token: token,
			Notification: &messaging.Notification{
				Title: title,
				Body:  body,
			},
			Data: stringData, // ✅ reused (immutable safe)
			APNS: &messaging.APNSConfig{
				Payload: &messaging.APNSPayload{
					Aps: &messaging.Aps{
						Sound: "default",
						Badge: badge,
					},
				},
			},
			Android: &messaging.AndroidConfig{
				Notification: &messaging.AndroidNotification{
					Sound: "default",
				},
				Priority: "high",
			},
		})
	}

	// Send in batches of up to 500 (FCM SendEach limit)
	const batchSize = 500
	for i := 0; i < len(messages); i += batchSize {
		end := i + batchSize
		if end > len(messages) {
			end = len(messages)
		}

		batch := messages[i:end]
		resp, err := f.client.SendEach(ctx, batch)
		if err != nil {
			return fmt.Errorf("FCM batch[%d:%d] failed: %w", i, end, err)
		}

		for j, r := range resp.Responses {
			if !r.Success {
				log.Printf("⚠️ FCM token %s (idx %d in batch %d) failed: %v",
					maskToken(tokens[i+j]), j, i, r.Error)
			}
		}
	}

	return nil
}

// maskToken hides all but last 6 chars for logging safety
func maskToken(token string) string {
	if len(token) <= 6 {
		return token
	}
	return "..." + token[len(token)-6:]
}