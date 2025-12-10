// internal/utils/notification_upload.go
package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

type NotificationR2Config struct {
	AccountID       string
	AccessKeyID     string
	AccessKeySecret string
	BucketName      string
	PublicURL       string // ✅ Added: for constructing public URLs
}

type NotificationR2Client struct {
	client   *s3.Client
	config   NotificationR2Config // ✅ Store config to access PublicURL
}

func NewNotificationR2Client(cfg NotificationR2Config) (*NotificationR2Client, error) {
	if cfg.AccountID == "" || cfg.AccessKeyID == "" || cfg.AccessKeySecret == "" || cfg.BucketName == "" {
		return nil, fmt.Errorf("missing required R2 configuration parameters")
	}

	r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL: fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID),
		}, nil
	})

	awsCfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithEndpointResolverWithOptions(r2Resolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.AccessKeySecret,
			"",
		)),
		config.WithRetryer(func() aws.Retryer {
			return aws.NopRetryer{}
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	// Verify bucket exists and we have permissions
	_, err = client.HeadBucket(context.TODO(), &s3.HeadBucketInput{
		Bucket: aws.String(cfg.BucketName),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") {
			return nil, fmt.Errorf("bucket %s not found or you don't have permission to access it", cfg.BucketName)
		}
		return nil, fmt.Errorf("failed to access bucket: %w", err)
	}

	return &NotificationR2Client{
		client: client,
		config: cfg, // ✅ Store config
	}, nil
}

// ✅ NEW: Generic Upload method - FIXED
func (r *NotificationR2Client) Upload(ctx context.Context, key string, content []byte, contentType string) error {
	// Use bytes.NewReader instead of strings.NewReader to handle binary data correctly
	// This prevents potential issues with checksum calculation
	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.config.BucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(content), // Use bytes.NewReader for binary data
		ContentType: aws.String(contentType),
		// Explicitly disable automatic checksum calculation if it causes issues
		// ChecksumAlgorithm: types.ChecksumAlgorithmCrc32, // This might be the source of the error
		// Commenting out checksum for R2 compatibility if default behavior causes issues
	})
	if err != nil {
		return fmt.Errorf("failed to upload to R2: %w", err)
	}
	return nil
}

// ✅ NEW: PublicURL getter method
func (r *NotificationR2Client) GetPublicURL() string {
	return r.config.PublicURL
}

// UploadNotificationImage uploads a notification image to R2 under "notification_images/" folder
func (r *NotificationR2Client) UploadNotificationImage(ctx context.Context, file io.Reader, originalFileName string, userID uuid.UUID) (string, error) {
	if file == nil {
		return "", fmt.Errorf("file reader cannot be nil")
	}

	if originalFileName == "" {
		return "", fmt.Errorf("filename cannot be empty")
	}

	// Read the entire file content into memory
	content, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Generate unique filename with user context
	ext := filepath.Ext(originalFileName)
	uniqueName := fmt.Sprintf("notification_images/%s_%d%s", userID.String(), time.Now().Unix(), ext)

	// Use the fixed Upload method
	if err := r.Upload(ctx, uniqueName, content, getContentType(originalFileName)); err != nil {
		return "", err
	}

	// Return the public URL of the uploaded file
	return fmt.Sprintf("%s/%s", r.config.PublicURL, uniqueName), nil
}

// UploadNotificationVideo uploads a notification video to R2 under "notification_videos/" folder
func (r *NotificationR2Client) UploadNotificationVideo(ctx context.Context, file io.Reader, originalFileName string, userID uuid.UUID) (string, error) {
	if file == nil {
		return "", fmt.Errorf("file reader cannot be nil")
	}

	if originalFileName == "" {
		return "", fmt.Errorf("filename cannot be empty")
	}

	// Read the entire file content into memory
	content, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Generate unique filename with user context
	ext := filepath.Ext(originalFileName)
	uniqueName := fmt.Sprintf("notification_videos/%s_%d%s", userID.String(), time.Now().Unix(), ext)

	// Use the fixed Upload method
	if err := r.Upload(ctx, uniqueName, content, getContentType(originalFileName)); err != nil {
		return "", err
	}

	// Return the public URL of the uploaded file
	return fmt.Sprintf("%s/%s", r.config.PublicURL, uniqueName), nil
}

// UploadNotificationThumbnail uploads a notification thumbnail to R2 under "notification_thumbnails/" folder
func (r *NotificationR2Client) UploadNotificationThumbnail(ctx context.Context, file io.Reader, originalFileName string, userID uuid.UUID) (string, error) {
	if file == nil {
		return "", fmt.Errorf("file reader cannot be nil")
	}

	if originalFileName == "" {
		return "", fmt.Errorf("filename cannot be empty")
	}

	// Read the entire file content into memory
	content, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Generate unique filename with user context
	ext := filepath.Ext(originalFileName)
	uniqueName := fmt.Sprintf("notification_thumbnails/%s_%d%s", userID.String(), time.Now().Unix(), ext)

	// Use the fixed Upload method
	if err := r.Upload(ctx, uniqueName, content, getContentType(originalFileName)); err != nil {
		return "", err
	}

	// Return the public URL of the uploaded file
	return fmt.Sprintf("%s/%s", r.config.PublicURL, uniqueName), nil
}

// DeleteNotificationFile deletes a file from R2
func (r *NotificationR2Client) DeleteNotificationFile(ctx context.Context, fileName string) error {
	if fileName == "" {
		return fmt.Errorf("filename cannot be empty")
	}

	// Extract just the filename part if a full URL is provided
	if strings.Contains(fileName, "/") {
		fileName = fileName[strings.LastIndex(fileName, "/")+1:]
	}

	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.config.BucketName),
		Key:    aws.String(fileName),
	})
	if err != nil {
		return fmt.Errorf("failed to delete file from R2: %w", err)
	}

	return nil
}

// Helper functions
func getContentType(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".webm":
		return "video/webm"
	default:
		return "application/octet-stream"
	}
}