package services

import (
	"crypto/md5"
	"file-shifter/config"
	"fmt"
	"log/slog"
	"sync"
)

// S3ClientManager verwaltet mehrere MinIO-Clients für verschiedene S3-Konfigurationen
type S3ClientManager struct {
	clients map[string]*MinIO
	mutex   sync.RWMutex
}

// NewS3ClientManager erstellt einen neuen S3ClientManager
func NewS3ClientManager() *S3ClientManager {
	return &S3ClientManager{
		clients: make(map[string]*MinIO),
	}
}

// getClientKey erstellt einen eindeutigen Schlüssel für eine S3-Konfiguration
func (scm *S3ClientManager) getClientKey(s3Config config.S3Config) string {
	// Erstelle einen Hash aus der Konfiguration
	data := fmt.Sprintf("%s:%s:%s:%t:%s",
		s3Config.Endpoint,
		s3Config.AccessKey,
		s3Config.SecretKey,
		s3Config.SSL,
		s3Config.Region)
	return fmt.Sprintf("%x", md5.Sum([]byte(data)))
}

// GetOrCreateClient gibt einen MinIO-Client für die gegebene S3-Konfiguration zurück
func (scm *S3ClientManager) GetOrCreateClient(s3Config config.S3Config) (*MinIO, error) {
	key := scm.getClientKey(s3Config)

	// Erst versuchen, einen bestehenden Client zu finden (Read-Lock)
	scm.mutex.RLock()
	if client, exists := scm.clients[key]; exists {
		scm.mutex.RUnlock()
		return client, nil
	}
	scm.mutex.RUnlock()

	// Client existiert nicht, also erstellen (Write-Lock)
	scm.mutex.Lock()
	defer scm.mutex.Unlock()

	// Nochmal prüfen, falls ein anderer Goroutine bereits erstellt hat
	if client, exists := scm.clients[key]; exists {
		return client, nil
	}

	// Neuen Client erstellen
	minioClient, err := NewMinIOConnection(
		s3Config.Endpoint,
		s3Config.AccessKey,
		s3Config.SecretKey,
		s3Config.SSL,
	)
	if err != nil {
		return nil, fmt.Errorf("fehler beim Erstellen des MinIO-Clients: %w", err)
	}

	// HealthCheck durchführen
	if err := minioClient.HealthCheck(); err != nil {
		return nil, fmt.Errorf("minIO-HealthCheck fehlgeschlagen: %w", err)
	}

	// Client im Cache speichern
	scm.clients[key] = minioClient

	slog.Info("Neuer MinIO-Client erstellt und gecacht",
		"endpoint", s3Config.Endpoint,
		"key", key[:8]) // Nur die ersten 8 Zeichen des Schlüssels anzeigen

	return minioClient, nil
}

// Close schließt alle MinIO-Clients (für cleanup)
func (scm *S3ClientManager) Close() {
	scm.mutex.Lock()
	defer scm.mutex.Unlock()

	for key, client := range scm.clients {
		if client != nil {
			// MinIO-Go-Client hat keine explizite Close-Methode
			// aber wir können ihn aus der Map entfernen
			delete(scm.clients, key)
		}
	}

	slog.Info("Alle MinIO-Clients geschlossen")
}

// GetActiveClientCount gibt die Anzahl der aktiven Clients zurück
func (scm *S3ClientManager) GetActiveClientCount() int {
	scm.mutex.RLock()
	defer scm.mutex.RUnlock()
	return len(scm.clients)
}
