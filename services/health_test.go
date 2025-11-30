package services

import (
	"encoding/json"
	"file-shifter/config"
	"net/http"
	"testing"
	"time"
)

func TestHealthMonitor(t *testing.T) {
	// Create test configuration
	cfg := &config.EnvConfig{}
	cfg.SetDefaults()

	inputDir := t.TempDir()
	outputTargets := []config.OutputTarget{
		{
			Path: t.TempDir(),
			Type: "filesystem",
		},
	}

	// Create worker
	worker := NewWorker(inputDir, outputTargets, cfg)

	// Start worker in background
	go worker.Start()

	// Create health monitor
	healthMonitor := NewHealthMonitor(worker, "8081")
	healthMonitor.Start()
	defer healthMonitor.Stop()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test /health endpoint
	t.Run("Health endpoint", func(t *testing.T) {
		resp, err := http.Get("http://localhost:8081/health")
		if err != nil {
			t.Fatalf("Failed to call health endpoint: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var healthCheck HealthCheck
		if err := json.NewDecoder(resp.Body).Decode(&healthCheck); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if healthCheck.Status != HealthStatusHealthy {
			t.Errorf("Expected status healthy, got %s", healthCheck.Status)
		}

		if len(healthCheck.Components) == 0 {
			t.Error("Expected components in health check response")
		}
	})

	// Test /health/live endpoint
	t.Run("Liveness endpoint", func(t *testing.T) {
		resp, err := http.Get("http://localhost:8081/health/live")
		if err != nil {
			t.Fatalf("Failed to call liveness endpoint: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	// Test /health/ready endpoint
	t.Run("Readiness endpoint", func(t *testing.T) {
		resp, err := http.Get("http://localhost:8081/health/ready")
		if err != nil {
			t.Fatalf("Failed to call readiness endpoint: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	// Stop worker before cleanup
	worker.Stop()
}
