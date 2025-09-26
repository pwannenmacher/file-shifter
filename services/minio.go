package services

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIO struct {
	MinIOClient *minio.Client
}

func NewMinIOConnection(endpoint, accessKey, secretKey string, useSSL bool) (*MinIO, error) {
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}

	slog.Info("MinIO-Client erfolgreich initialisiert", "endpoint", endpoint)
	return &MinIO{MinIOClient: minioClient}, nil
}

func (m *MinIO) EnsureBucket(bucketName string) error {
	if m.MinIOClient == nil {
		return errors.New("MinIO client is not initialized")
	}

	ctx := context.Background()

	exists, err := m.MinIOClient.BucketExists(ctx, bucketName)
	if err != nil {
		return err
	}

	if !exists {
		err = m.MinIOClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return err
		}
		slog.Info("Bucket erfolgreich erstellt", "bucket", bucketName)
	}

	return nil
}

func (m *MinIO) UploadFile(filePath, bucketName, fileName string) (string, error) {
	if m.MinIOClient == nil {
		return "", errors.New("MinIO client is not initialized")
	}

	ctx := context.Background()

	// Bestimme Content-Type basierend auf Dateierweiterung
	contentType := "application/octet-stream"
	ext := filepath.Ext(fileName)
	switch ext {
	case ".txt":
		contentType = "text/plain"
	case ".json":
		contentType = "application/json"
	case ".pdf":
		contentType = "application/pdf"
	default:
		contentType = "application/octet-stream"
	}

	info, err := m.MinIOClient.FPutObject(ctx, bucketName, fileName, filePath,
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		slog.Warn("Fehler beim Hochladen der Datei", "datei", fileName, "err", err)
		return "", err
	}

	slog.Info("Datei erfolgreich hochgeladen", "datei", fileName, "größe", info.Size)
	return fileName, nil
}

func (m *MinIO) ObjectExists(bucket, key string) (bool, error) {
	if m.MinIOClient == nil {
		return false, errors.New("MinIO client is not initialized")
	}

	ctx := context.Background()
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
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func (m *MinIO) HealthCheck() error {
	if m.MinIOClient == nil {
		return errors.New("MinIO client not initialized")
	}
	_, err := m.MinIOClient.ListBuckets(context.Background())
	return err
}

func (m *MinIO) DeleteFile(bucketName, objectKey string) error {
	if m.MinIOClient == nil {
		return errors.New("MinIO client not initialized")
	}
	ctx := context.Background()
	err := m.MinIOClient.RemoveObject(ctx, bucketName, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		slog.Warn("Fehler beim Löschen der Datei", "bucket", bucketName, "key", objectKey, "err", err)
		return err
	}

	slog.Info("Datei erfolgreich gelöscht", "bucket", bucketName, "key", objectKey)
	return nil
}
