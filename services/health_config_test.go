package services

import (
	"strings"
	"testing"
	"time"
)

// TestHealthMonitor_EventBacklogDegraded covers the event-backlog branch of
// HealthStatus: a backlog at or above the warn threshold degrades an otherwise
// healthy watcher and is appended to an already unhealthy status message.
func TestHealthMonitor_EventBacklogDegraded(t *testing.T) {
	t.Run("large backlog degrades healthy watcher", func(t *testing.T) {
		fw := &FileWatcher{
			fileQueue:            make(chan string, 10),
			queueCapacity:        10,
			workerCount:          2,
			backlogWarnThreshold: 3,
			pendingFiles:         []string{"a", "b", "c"},
		}

		hm := NewHealthMonitor(&Worker{FileWatcher: fw}, "0")
		status := hm.HealthStatus()

		if status.Status != HealthStatusDegraded {
			t.Fatalf("expected degraded overall status for large backlog, got %s", status.Status)
		}
		component, ok := status.Components["file_watcher"]
		if !ok {
			t.Fatal("expected file_watcher component in health status")
		}
		if component.Status != HealthStatusDegraded {
			t.Fatalf("expected degraded file_watcher status, got %s", component.Status)
		}
		if !strings.Contains(component.Message, "event backlog is large") {
			t.Fatalf("expected backlog message, got: %s", component.Message)
		}
	})

	t.Run("backlog message is appended to unhealthy queue status", func(t *testing.T) {
		fw := &FileWatcher{
			fileQueue:            make(chan string, 10),
			queueCapacity:        10,
			workerCount:          2,
			backlogWarnThreshold: 1,
			pendingFiles:         []string{"a"},
		}
		for i := 0; i < 10; i++ {
			fw.fileQueue <- "f"
		}

		hm := NewHealthMonitor(&Worker{FileWatcher: fw}, "0")
		status := hm.HealthStatus()

		if status.Status != HealthStatusUnhealthy {
			t.Fatalf("expected unhealthy overall status to be preserved, got %s", status.Status)
		}
		component := status.Components["file_watcher"]
		if component.Status != HealthStatusUnhealthy {
			t.Fatalf("expected unhealthy file_watcher status to be preserved, got %s", component.Status)
		}
		if !strings.Contains(component.Message, "critically full") {
			t.Fatalf("expected queue message to be retained, got: %s", component.Message)
		}
		if !strings.Contains(component.Message, "event backlog is large") {
			t.Fatalf("expected backlog message to be appended, got: %s", component.Message)
		}
	})

	t.Run("backlog below threshold stays healthy", func(t *testing.T) {
		fw := &FileWatcher{
			fileQueue:            make(chan string, 10),
			queueCapacity:        10,
			workerCount:          2,
			backlogWarnThreshold: 100,
			pendingFiles:         []string{"a"},
		}

		hm := NewHealthMonitor(&Worker{FileWatcher: fw}, "0")
		status := hm.HealthStatus()

		if status.Status != HealthStatusHealthy {
			t.Fatalf("expected healthy status for small backlog, got %s", status.Status)
		}
		if strings.Contains(status.Components["file_watcher"].Message, "event backlog is large") {
			t.Fatalf("did not expect backlog warning, got: %s", status.Components["file_watcher"].Message)
		}
	})
}

// TestHealthMonitor_ServerTimeoutConfiguration verifies that the health
// check HTTP server is configured with the expected timeouts so slow or
// malicious clients cannot exhaust connections.
func TestHealthMonitor_ServerTimeoutConfiguration(t *testing.T) {
	hm := NewHealthMonitor(&Worker{}, "0")
	hm.Start()
	defer hm.Stop()

	if hm.server == nil {
		t.Fatal("expected HTTP server to be created by Start")
	}

	if got, want := hm.server.ReadHeaderTimeout, 5*time.Second; got != want {
		t.Errorf("ReadHeaderTimeout = %v, want %v", got, want)
	}
	if got, want := hm.server.ReadTimeout, 10*time.Second; got != want {
		t.Errorf("ReadTimeout = %v, want %v", got, want)
	}
	if got, want := hm.server.WriteTimeout, 10*time.Second; got != want {
		t.Errorf("WriteTimeout = %v, want %v", got, want)
	}
	if got, want := hm.server.IdleTimeout, 60*time.Second; got != want {
		t.Errorf("IdleTimeout = %v, want %v", got, want)
	}
	if got, want := hm.server.Addr, ":0"; got != want {
		t.Errorf("Addr = %q, want %q", got, want)
	}
}
