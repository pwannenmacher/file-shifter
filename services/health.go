package services

import (
	"encoding/json"
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
		if err := hm.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
		hm.server.Close()
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
	queueSize := hm.worker.FileWatcher.GetQueueSize()
	queueCapacity := hm.worker.FileWatcher.GetQueueCapacity()
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

func (hm *HealthMonitor) healthHandler(w http.ResponseWriter, r *http.Request) {
	healthCheck := hm.getHealthStatus()

	w.Header().Set("Content-Type", "application/json")
	if healthCheck.Status != HealthStatusHealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	json.NewEncoder(w).Encode(healthCheck)
}

func (hm *HealthMonitor) livenessHandler(w http.ResponseWriter, r *http.Request) {
	// Liveness: Is the application still alive?
	// If we can respond here, the application is running
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "alive",
	})
}

func (hm *HealthMonitor) readinessHandler(w http.ResponseWriter, r *http.Request) {
	// Readiness: Is the application ready to do work?
	healthCheck := hm.getHealthStatus()

	w.Header().Set("Content-Type", "application/json")
	if healthCheck.Status != HealthStatusHealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(healthCheck)
}

func (hm *HealthMonitor) getHealthStatus() HealthCheck {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	components := make(map[string]ComponentHealth)
	overallStatus := HealthStatusHealthy

	// FileWatcher Status
	if hm.worker.FileWatcher != nil {
		queueSize := hm.worker.FileWatcher.GetQueueSize()
		queueCapacity := hm.worker.FileWatcher.GetQueueCapacity()
		fillPercentage := float64(queueSize) / float64(queueCapacity) * 100

		status := HealthStatusHealthy
		message := "FileWatcher is running normally"

		if fillPercentage > 90 {
			status = HealthStatusUnhealthy
			message = "FileQueue is critically full (>90%)"
			overallStatus = HealthStatusUnhealthy
		} else if fillPercentage > 80 {
			status = HealthStatusDegraded
			message = "FileQueue is heavily loaded (>80%)"
			if overallStatus == HealthStatusHealthy {
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
			Message:     fmt.Sprintf("%d workers active", hm.worker.FileWatcher.GetWorkerCount()),
		}
	}

	return HealthCheck{
		Status:     overallStatus,
		Timestamp:  time.Now(),
		Components: components,
	}
}
