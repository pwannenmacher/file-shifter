package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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
	worker *Worker
	port   string
	server *http.Server
}

func NewHealthMonitor(worker *Worker, port string) *HealthMonitor {
	return &HealthMonitor{
		worker: worker,
		port:   port,
	}
}

func (hm *HealthMonitor) Start() {
	// HTTP Server for Health-Check
	mux := http.NewServeMux()
	mux.HandleFunc("/health", hm.healthHandler)
	mux.HandleFunc("/health/live", hm.livenessHandler)
	mux.HandleFunc("/health/ready", hm.readinessHandler)

	hm.server = &http.Server{
		Addr:              ":" + hm.port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Start HTTP Server
	go func() {
		slog.Info("Health-Check server started", "port", hm.port)
		if err := hm.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Health-Check server error", "error", err)
		}
	}()
}

func (hm *HealthMonitor) Stop() {
	if hm.server != nil {
		if err := hm.server.Close(); err != nil {
			slog.Error("Error closing health check server", "error", err)
		}
	}
	slog.Info("Health-Check server stopped")
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

		// A large event backlog means files are processed with delay -
		// surface this as degraded (nothing is lost, only queued)
		backlog := hm.worker.FileWatcher.EventBacklog()
		if backlog >= hm.worker.FileWatcher.EventBacklogWarnThreshold() {
			message = fmt.Sprintf("%s; event backlog is large (%d files awaiting stability check)", message, backlog)
			if status == HealthStatusHealthy {
				status = HealthStatusDegraded
			}
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
			Message:     fmt.Sprintf("%d workers active", hm.worker.FileWatcher.WorkerCount()),
		}
	}

	return HealthCheck{
		Status:     overallStatus,
		Timestamp:  time.Now(),
		Components: components,
	}
}
