package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusDegraded  HealthStatus = "degraded"
)

const (
	contentTypeHeader = "Content-Type"
	contentTypeJSON   = "application/json"
)

type ComponentHealth struct {
	Status      HealthStatus `json:"status"`
	LastChecked time.Time    `json:"last_checked"`
	Message     string       `json:"message,omitempty"`
}

type HealthCheck struct {
	Status     HealthStatus               `json:"status"`
	Timestamp  time.Time                  `json:"timestamp"`
	Components map[string]ComponentHealth `json:"components"`
}

type HealthMonitor struct {
	worker      *Worker
	port        string
	server      *http.Server
	mu          sync.RWMutex
	lastCheck   time.Time
	isHealthy   bool
	stopChan    chan bool
	checkTicker *time.Ticker
}

func NewHealthMonitor(worker *Worker, port string) *HealthMonitor {
	return &HealthMonitor{
		worker:    worker,
		port:      port,
		stopChan:  make(chan bool),
		isHealthy: true,
	}
}

func (hm *HealthMonitor) Start() {
	// HTTP Server for Health-Check
	mux := http.NewServeMux()
	mux.HandleFunc("/health", hm.healthHandler)
	mux.HandleFunc("/health/live", hm.livenessHandler)
	mux.HandleFunc("/health/ready", hm.readinessHandler)

	hm.server = &http.Server{
		Addr:    ":" + hm.port,
		Handler: mux,
	}

	// Periodic Health-Checks
	hm.checkTicker = time.NewTicker(10 * time.Second)
	go hm.periodicHealthCheck()

	// Start HTTP Server
	go func() {
		slog.Info("Health-Check server started", "port", hm.port)
		if err := hm.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Health-Check server error", "error", err)
		}
	}()
}

func (hm *HealthMonitor) Stop() {
	if hm.checkTicker != nil {
		hm.checkTicker.Stop()
	}
	close(hm.stopChan)
	if hm.server != nil {
		if err := hm.server.Close(); err != nil {
			slog.Error("Error closing health check server", "error", err)
		}
	}
	slog.Info("Health-Check server stopped")
}

func (hm *HealthMonitor) periodicHealthCheck() {
	for {
		select {
		case <-hm.stopChan:
			return
		case <-hm.checkTicker.C:
			hm.performHealthCheck()
		}
	}
}

func (hm *HealthMonitor) performHealthCheck() {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.lastCheck = time.Now()
	hm.isHealthy = true

	// Check FileWatcher status
	if hm.worker.FileWatcher == nil {
		slog.Warn("Health-Check: FileWatcher is not initialized")
		hm.isHealthy = false
		return
	}

	// Check if the file queue is too full (over 90%)
	queueSize := hm.worker.FileWatcher.QueueSize()
	queueCapacity := hm.worker.FileWatcher.QueueCapacity()
	if queueCapacity > 0 {
		fillPercentage := float64(queueSize) / float64(queueCapacity) * 100
		if fillPercentage > 90 {
			slog.Warn("Health-Check: FileQueue is critically full",
				"fill_percentage", fillPercentage,
				"queue_size", queueSize,
				"capacity", queueCapacity)
			hm.isHealthy = false
		}
	}
}

func (hm *HealthMonitor) healthHandler(w http.ResponseWriter, _ *http.Request) {
	healthCheck := hm.HealthStatus()

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	if healthCheck.Status != HealthStatusHealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	if err := json.NewEncoder(w).Encode(healthCheck); err != nil {
		slog.Error("Failed to encode health check response", "error", err)
	}
}

func (hm *HealthMonitor) livenessHandler(w http.ResponseWriter, _ *http.Request) {
	// Liveness: Is the application still alive?
	// If we can respond here, the application is running
	w.Header().Set(contentTypeHeader, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "alive",
	}); err != nil {
		slog.Error("Failed to encode liveness response", "error", err)
	}
}

func (hm *HealthMonitor) readinessHandler(w http.ResponseWriter, _ *http.Request) {
	// Readiness: Is the application ready to do work?
	healthCheck := hm.HealthStatus()

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	if healthCheck.Status != HealthStatusHealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if err := json.NewEncoder(w).Encode(healthCheck); err != nil {
		slog.Error("Failed to encode readiness response", "error", err)
	}
}

func (hm *HealthMonitor) HealthStatus() HealthCheck {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	components := make(map[string]ComponentHealth)
	overallStatus := HealthStatusHealthy

	// FileWatcher Status
	if hm.worker.FileWatcher != nil {
		queueSize := hm.worker.FileWatcher.QueueSize()
		queueCapacity := hm.worker.FileWatcher.QueueCapacity()
		var fillPercentage float64
		status := HealthStatusHealthy
		message := "FileWatcher is running normally"

		if queueCapacity == 0 {
			fillPercentage = 0
			status = HealthStatusUnhealthy
			message = "FileWatcher queue capacity is zero (misconfiguration)"
			overallStatus = HealthStatusUnhealthy
		} else {
			fillPercentage = float64(queueSize) / float64(queueCapacity) * 100
			if fillPercentage > 90 {
				status = HealthStatusUnhealthy
				message = "FileQueue is critically full (>90%)"
				overallStatus = HealthStatusUnhealthy
			} else if fillPercentage > 80 {
				status = HealthStatusDegraded
				message = "FileQueue is heavily loaded (>80%)"
				overallStatus = HealthStatusDegraded
			}
		}

		components["file_watcher"] = ComponentHealth{
			Status:      status,
			LastChecked: time.Now(),
			Message:     message,
		}
	} else {
		components["file_watcher"] = ComponentHealth{
			Status:      HealthStatusUnhealthy,
			LastChecked: time.Now(),
			Message:     "FileWatcher not initialized",
		}
		overallStatus = HealthStatusUnhealthy
	}

	// S3 Client Manager Status
	if hm.worker.S3ClientManager != nil {
		activeClients := hm.worker.S3ClientManager.GetActiveClientCount()
		components["s3_clients"] = ComponentHealth{
			Status:      HealthStatusHealthy,
			LastChecked: time.Now(),
			Message:     fmt.Sprintf("%d active S3 clients", activeClients),
		}
	}

	// Worker Pool Status
	if hm.worker.FileWatcher != nil {
		components["worker_pool"] = ComponentHealth{
			Status:      HealthStatusHealthy,
			LastChecked: time.Now(),
			Message:     fmt.Sprintf("%d workers active", hm.worker.FileWatcher.WorkerCount()),
		}
	}

	return HealthCheck{
		Status:     overallStatus,
		Timestamp:  time.Now(),
		Components: components,
	}
}
