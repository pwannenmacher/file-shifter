package services

import (
	"context"
	"errors"
	"log/slog"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	ErrMinIOClientNotInitialized = "MinIO client is not initialized"

	defaultOperationTimeout = 30 * time.Second
	defaultUploadTimeout    = 10 * time.Minute
)

type MinIO struct {
	MinIOClient      *minio.Client
	OperationTimeout time.Duration // Timeout for metadata operations (bucket checks, stats, deletes)
	UploadTimeout    time.Duration // Timeout for uploads
}

func NewMinIOConnection(endpoint, accessKey, secretKey string, useSSL bool) (*MinIO, error) {
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}

	slog.Info("MinIO client initialized successfully", "endpoint", endpoint)
	return &MinIO{
		MinIOClient:      minioClient,
		OperationTimeout: defaultOperationTimeout,
		UploadTimeout:    defaultUploadTimeout,
	}, nil
}

// operationContext returns a context with the configured metadata operation timeout
func (m *MinIO) operationContext() (context.Context, context.CancelFunc) {
	timeout := m.OperationTimeout
	if timeout <= 0 {
		timeout = defaultOperationTimeout
	}
	return context.WithTimeout(context.Background(), timeout)
}

// uploadContext returns a context with the configured upload timeout
func (m *MinIO) uploadContext() (context.Context, context.CancelFunc) {
	timeout := m.UploadTimeout
	if timeout <= 0 {
		timeout = defaultUploadTimeout
	}
	return context.WithTimeout(context.Background(), timeout)
}

func (m *MinIO) EnsureBucket(bucketName string) error {
	if m.MinIOClient == nil {
		return errors.New(ErrMinIOClientNotInitialized)
	}

	ctx, cancel := m.operationContext()
	defer cancel()

	exists, err := m.MinIOClient.BucketExists(ctx, bucketName)
	if err != nil {
		return err
	}

	if !exists {
		err = m.MinIOClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return err
		}
		slog.Info("Bucket created successfully", "bucket", bucketName)
	}

	return nil
}

func (m *MinIO) UploadFile(filePath, bucketName, fileName string) (string, error) {
	if m.MinIOClient == nil {
		return "", errors.New(ErrMinIOClientNotInitialized)
	}

	ctx, cancel := m.uploadContext()
	defer cancel()

	// Determine content type based on file extension
	contentType := mime.TypeByExtension(filepath.Ext(fileName))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	info, err := m.MinIOClient.FPutObject(ctx, bucketName, fileName, filePath,
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		slog.Warn("Error uploading file", "file", fileName, "err", err)
		return "", err
	}

	slog.Info("File uploaded successfully", "file", fileName, "size", info.Size)
	return fileName, nil
}

func (m *MinIO) ObjectExists(bucket, key string) (bool, error) {
	if m.MinIOClient == nil {
		return false, errors.New(ErrMinIOClientNotInitialized)
	}

	ctx, cancel := m.operationContext()
	defer cancel()
	_, err := m.MinIOClient.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	if minio.ToErrorResponse(err).Code == "NoSuchKey" {
		return false, nil
	}
	return false, err
}

func (m *MinIO) SanitizeBucketName(name string) string {
	sanitized := strings.ToLower(name)
	sanitized = strings.ReplaceAll(sanitized, "_", "-")
	sanitized = strings.ReplaceAll(sanitized, " ", "-")
	var result strings.Builder
	for _, r := range sanitized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	if result.String() != name {
		slog.Warn("Bucket name was sanitized - files will be stored under the sanitized name",
			"original", name, "sanitized", result.String())
	}
	return result.String()
}

func (m *MinIO) HealthCheck() error {
	if m.MinIOClient == nil {
		return errors.New(ErrMinIOClientNotInitialized)
	}
	ctx, cancel := m.operationContext()
	defer cancel()
	_, err := m.MinIOClient.ListBuckets(ctx)
	return err
}

func (m *MinIO) DeleteFile(bucketName, objectKey string) error {
	if m.MinIOClient == nil {
		return errors.New(ErrMinIOClientNotInitialized)
	}
	ctx, cancel := m.operationContext()
	defer cancel()
	err := m.MinIOClient.RemoveObject(ctx, bucketName, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		slog.Warn("Error deleting file", "bucket", bucketName, "key", objectKey, "err", err)
		return err
	}

	slog.Info("File deleted successfully", "bucket", bucketName, "key", objectKey)
	return nil
}
