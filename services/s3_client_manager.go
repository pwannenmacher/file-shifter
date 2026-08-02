package services

import (
	"crypto/sha256"
	"file-shifter/config"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// S3ClientManager manages multiple MinIO clients for different S3 configurations
type S3ClientManager struct {
	clients          map[string]*MinIO
	mutex            sync.RWMutex
	operationTimeout time.Duration
	uploadTimeout    time.Duration
}

// NewS3ClientManager creates a new S3ClientManager
func NewS3ClientManager() *S3ClientManager {
	return &S3ClientManager{
		clients: make(map[string]*MinIO),
	}
}

// SetTimeouts configures the timeouts applied to newly created MinIO clients
func (scm *S3ClientManager) SetTimeouts(operationTimeout, uploadTimeout time.Duration) {
	scm.mutex.Lock()
	defer scm.mutex.Unlock()
	scm.operationTimeout = operationTimeout
	scm.uploadTimeout = uploadTimeout
}

// getClientKey creates a unique key for an S3 configuration
func (scm *S3ClientManager) getClientKey(s3Config config.S3Config) string {
	// Create a hash from the configuration
	data := fmt.Sprintf("%s:%s:%s:%t:%s",
		s3Config.Endpoint,
		s3Config.AccessKey,
		s3Config.SecretKey,
		s3Config.SSL,
		s3Config.Region)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
}

// GetOrCreateClient returns a MinIO client for the given S3 configuration
func (scm *S3ClientManager) GetOrCreateClient(s3Config config.S3Config) (*MinIO, error) {
	key := scm.getClientKey(s3Config)

	// First try to find an existing client (read lock)
	scm.mutex.RLock()
	if client, exists := scm.clients[key]; exists {
		scm.mutex.RUnlock()
		return client, nil
	}
	scm.mutex.RUnlock()

	// Client does not exist, so create (Write-Lock)
	scm.mutex.Lock()
	defer scm.mutex.Unlock()

	// Check again, in case another goroutine has already created it
	if client, exists := scm.clients[key]; exists {
		return client, nil
	}

	minioClient, err := NewMinIOConnection(
		s3Config.Endpoint,
		s3Config.AccessKey,
		s3Config.SecretKey,
		s3Config.SSL,
	)
	if err != nil {
		return nil, fmt.Errorf("error creating MinIO client: %w", err)
	}

	// Apply configured timeouts (keep client defaults when unset)
	if scm.operationTimeout > 0 {
		minioClient.OperationTimeout = scm.operationTimeout
	}
	if scm.uploadTimeout > 0 {
		minioClient.UploadTimeout = scm.uploadTimeout
	}

	// Perform health check
	if err := minioClient.HealthCheck(); err != nil {
		return nil, fmt.Errorf("minio health check failed: %w", err)
	}

	// Save client in cache
	scm.clients[key] = minioClient

	slog.Info("New MinIO client created and cached",
		"endpoint", s3Config.Endpoint,
		"key", key[:8]) // Only show first 8 characters of key

	return minioClient, nil
}

// Close closes all MinIO clients (for cleanup)
func (scm *S3ClientManager) Close() {
	scm.mutex.Lock()
	defer scm.mutex.Unlock()

	for key, client := range scm.clients {
		if client != nil {
			// MinIO Go client does not have an explicit close method, but we can remove it from the cache map
			delete(scm.clients, key)
		}
	}

	slog.Info("All MinIO clients closed")
}

// GetActiveClientCount returns the number of active clients
func (scm *S3ClientManager) GetActiveClientCount() int {
	scm.mutex.RLock()
	defer scm.mutex.RUnlock()
	return len(scm.clients)
}
