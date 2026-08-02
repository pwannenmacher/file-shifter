package services

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"file-shifter/config"
)

// TestFileWatcher_BoundedGoroutinesUnderEventStorm verifies that an event
// storm does not spawn one goroutine per event (audit finding 3). Before the
// bounded event pipeline, every created file held its own goroutine sleeping
// through the stability check.
func TestFileWatcher_BoundedGoroutinesUnderEventStorm(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	s3Manager := NewS3ClientManager()
	defer s3Manager.Close()

	targets := []config.OutputTarget{{Type: "filesystem", Path: outputDir}}
	fileHandler := NewFileHandler(targets, s3Manager)

	const fileCount = 500

	// Long stability period so pending files stay in the pipeline while we
	// sample the goroutine count.
	watcher, err := NewFileWatcher(inputDir, fileHandler, 3, 50*time.Millisecond, 200*time.Millisecond, 4, 100)
	if err != nil {
		t.Fatalf("failed to create file watcher: %v", err)
	}

	go func() {
		if err := watcher.Start(); err != nil {
			t.Errorf("watcher start failed: %v", err)
		}
	}()
	defer watcher.Stop()

	// Give the watcher time to register before the storm
	time.Sleep(200 * time.Millisecond)

	baseline := runtime.NumGoroutine()

	for i := 0; i < fileCount; i++ {
		filePath := filepath.Join(inputDir, fmt.Sprintf("storm_%04d.txt", i))
		if err := os.WriteFile(filePath, []byte("storm content"), 0o644); err != nil {
			t.Fatalf("failed to create file %d: %v", i, err)
		}
	}

	// Sample the goroutine count while the storm is being processed
	maxGoroutines := 0
	for i := 0; i < 20; i++ {
		if n := runtime.NumGoroutine(); n > maxGoroutines {
			maxGoroutines = n
		}
		time.Sleep(50 * time.Millisecond)
	}

	// The old per-event-goroutine design would exceed the baseline by roughly
	// fileCount goroutines; the bounded pipeline stays within pools + margin.
	limit := baseline + 60
	if maxGoroutines > limit {
		t.Fatalf("goroutine count exploded under event storm: max %d, baseline %d, limit %d", maxGoroutines, baseline, limit)
	}

	t.Logf("goroutines: baseline=%d max=%d limit=%d files=%d", baseline, maxGoroutines, limit, fileCount)
}
