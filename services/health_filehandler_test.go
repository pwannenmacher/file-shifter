package services

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"file-shifter/config"
)

type failingResponseWriter struct {
	header http.Header
	status int
}

func (w *failingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func (w *failingResponseWriter) Write(_ []byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestHealthMonitor_PerformHealthCheckBranches(t *testing.T) {
	t.Run("nil file watcher sets unhealthy", func(t *testing.T) {
		hm := NewHealthMonitor(&Worker{}, "0")
		hm.performHealthCheck()

		if hm.isHealthy {
			t.Fatal("expected unhealthy status when FileWatcher is nil")
		}
		if hm.lastCheck.IsZero() {
			t.Fatal("expected lastCheck to be set")
		}
	})

	t.Run("over 90 percent queue sets unhealthy", func(t *testing.T) {
		fw := &FileWatcher{fileQueue: make(chan string, 10), queueCapacity: 10, workerCount: 2}
		for i := 0; i < 10; i++ {
			fw.fileQueue <- "f"
		}

		hm := NewHealthMonitor(&Worker{FileWatcher: fw}, "0")
		hm.performHealthCheck()
		if hm.isHealthy {
			t.Fatal("expected unhealthy status when queue fill is above 90%")
		}
	})
}

func TestHealthMonitor_HandlersAndStatusBranches(t *testing.T) {
	t.Run("readiness handler encode error path", func(t *testing.T) {
		hm := NewHealthMonitor(&Worker{}, "0")
		w := &failingResponseWriter{}
		hm.readinessHandler(w, nil)
		if w.status != http.StatusServiceUnavailable {
			t.Fatalf("expected %d, got %d", http.StatusServiceUnavailable, w.status)
		}
	})

	t.Run("health status degraded and zero capacity", func(t *testing.T) {
		fwDegraded := &FileWatcher{fileQueue: make(chan string, 10), queueCapacity: 10, workerCount: 3}
		for i := 0; i < 9; i++ {
			fwDegraded.fileQueue <- "f"
		}
		hmDegraded := NewHealthMonitor(&Worker{FileWatcher: fwDegraded, S3ClientManager: NewS3ClientManager()}, "0")
		status := hmDegraded.HealthStatus()
		if status.Status != HealthStatusDegraded {
			t.Fatalf("expected degraded status, got %s", status.Status)
		}

		fwZero := &FileWatcher{fileQueue: make(chan string, 1), queueCapacity: 0, workerCount: 1}
		hmZero := NewHealthMonitor(&Worker{FileWatcher: fwZero}, "0")
		zeroStatus := hmZero.HealthStatus()
		if zeroStatus.Status != HealthStatusUnhealthy {
			t.Fatalf("expected unhealthy status for zero capacity, got %s", zeroStatus.Status)
		}
	})
}

func TestFileHandler_TargetAndFinalizeBranches(t *testing.T) {
	tempDir := t.TempDir()
	inputFile := filepath.Join(tempDir, "in.txt")
	if err := os.WriteFile(inputFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to create input file: %v", err)
	}
	fi, err := os.Stat(inputFile)
	if err != nil {
		t.Fatalf("failed to stat input file: %v", err)
	}

	t.Run("copyToTarget covers switch branches", func(t *testing.T) {
		fh := NewFileHandler(nil, nil)
		outDir := filepath.Join(tempDir, "out")
		if err := fh.copyToTarget(inputFile, "in.txt", config.OutputTarget{Type: "filesystem", Path: outDir}, fi); err != nil {
			t.Fatalf("expected filesystem copy success, got: %v", err)
		}

		if _, err := os.Stat(filepath.Join(outDir, "in.txt")); err != nil {
			t.Fatalf("expected copied file, got: %v", err)
		}

		if err := fh.copyToTarget(inputFile, "in.txt", config.OutputTarget{Type: "unknown"}, fi); err == nil {
			t.Fatal("expected error for unknown target type")
		}

		if err := fh.copyToTarget(inputFile, "in.txt", config.OutputTarget{Type: "s3"}, fi); err == nil {
			t.Fatal("expected error for s3 target with nil manager")
		}

		if err := fh.copyToTarget(inputFile, "in.txt", config.OutputTarget{Type: "ftp", Path: "://bad"}, fi); err == nil {
			t.Fatal("expected error for invalid ftp target path")
		}

		if err := fh.copyToTarget(inputFile, "in.txt", config.OutputTarget{Type: "sftp", Path: "://bad"}, fi); err == nil {
			t.Fatal("expected error for invalid sftp target path")
		}
	})

	t.Run("copyToAllTargets returns joined error", func(t *testing.T) {
		fh := NewFileHandler([]config.OutputTarget{{Type: "unknown"}}, nil)
		err := fh.copyToAllTargets(inputFile, "in.txt", fi)
		if err == nil {
			t.Fatal("expected joined error for failing targets")
		}
		if !strings.Contains(err.Error(), "transfers failed") {
			t.Fatalf("expected transfer context in error, got: %v", err)
		}
	})

	t.Run("finalizeProcessedFile retry and max-retry paths", func(t *testing.T) {
		fh := NewFileHandler(nil, nil)
		fileRetry := filepath.Join(tempDir, "retry.txt")
		if err := os.WriteFile(fileRetry, []byte("retry"), 0o644); err != nil {
			t.Fatalf("failed to create retry file: %v", err)
		}

		retry, err := fh.finalizeProcessedFile(fileRetry, "retry.txt", "different", 1, 3)
		if err != nil {
			t.Fatalf("expected no error before max retries, got: %v", err)
		}
		if !retry {
			t.Fatal("expected retry=true when checksum mismatch and attempts remain")
		}

		_, err = fh.finalizeProcessedFile(fileRetry, "retry.txt", "different", 3, 3)
		if err == nil {
			t.Fatal("expected error when checksum mismatch reaches max retries")
		}
	})

	t.Run("finalizeProcessedFile checksum-error and success remove", func(t *testing.T) {
		fh := NewFileHandler(nil, nil)

		_, err := fh.finalizeProcessedFile(filepath.Join(tempDir, "missing.txt"), "missing.txt", "x", 1, 2)
		if err == nil {
			t.Fatal("expected final checksum error for missing file")
		}

		fileOK := filepath.Join(tempDir, "ok.txt")
		if err := os.WriteFile(fileOK, []byte("ok"), 0o644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
		checksum, err := fh.calculateFileChecksum(fileOK)
		if err != nil {
			t.Fatalf("failed to calculate checksum: %v", err)
		}

		retry, err := fh.finalizeProcessedFile(fileOK, "ok.txt", checksum, 1, 2)
		if err != nil {
			t.Fatalf("expected success remove, got: %v", err)
		}
		if retry {
			t.Fatal("expected retry=false on successful finalize")
		}
		if _, err := os.Stat(fileOK); !os.IsNotExist(err) {
			t.Fatal("expected file to be removed after successful finalize")
		}
	})

	t.Run("cleanupTargetFiles returns aggregate error", func(t *testing.T) {
		cleanupBase := filepath.Join(tempDir, "cleanup")
		if err := os.MkdirAll(cleanupBase, 0o755); err != nil {
			t.Fatalf("failed to create cleanup dir: %v", err)
		}
		cleanupFile := filepath.Join(cleanupBase, "x.txt")
		if err := os.WriteFile(cleanupFile, []byte("x"), 0o644); err != nil {
			t.Fatalf("failed to create cleanup file: %v", err)
		}

		fh := NewFileHandler([]config.OutputTarget{
			{Type: "filesystem", Path: cleanupBase},
			{Type: "s3", Path: "s3://bucket/prefix"},
		}, nil)

		err := fh.cleanupTargetFiles("x.txt")
		if err == nil {
			t.Fatal("expected cleanup error because s3 deletion cannot run without manager")
		}
	})

	t.Run("delete from ftp and sftp parse errors", func(t *testing.T) {
		fh := NewFileHandler(nil, nil)
		if err := fh.deleteFromFTP("file.txt", config.OutputTarget{Type: "ftp", Path: "://bad"}); err == nil {
			t.Fatal("expected parse error for invalid ftp delete path")
		}
		if err := fh.deleteFromSFTP("file.txt", config.OutputTarget{Type: "sftp", Path: "://bad"}); err == nil {
			t.Fatal("expected parse error for invalid sftp delete path")
		}
	})
}

func TestFileWatcher_QueueAndCompletionBranches(t *testing.T) {
	t.Run("enqueue handles stopping and stop channel", func(t *testing.T) {
		fw := &FileWatcher{
			stopChan:        make(chan bool),
			fileQueue:       make(chan string, 1),
			queueCapacity:   1,
			processingFiles: map[string]struct{}{"a": {}},
		}
		fw.stopping.Store(true)
		fw.enqueueFileWithMonitoring("a")
		if _, ok := fw.processingFiles["a"]; ok {
			t.Fatal("expected file to be unmarked when stopping")
		}

		fw2 := &FileWatcher{
			stopChan:        make(chan bool),
			fileQueue:       make(chan string, 1),
			queueCapacity:   1,
			processingFiles: map[string]struct{}{"b": {}},
		}
		fw2.fileQueue <- "already-full"
		close(fw2.stopChan)
		fw2.enqueueFileWithMonitoring("b")
		if len(fw2.fileQueue) != 1 {
			t.Fatal("expected queue length to remain unchanged when stop channel is closed")
		}
	})

	t.Run("enqueue and queue normalization", func(t *testing.T) {
		fw := &FileWatcher{
			stopChan:        make(chan bool),
			fileQueue:       make(chan string, 1),
			queueCapacity:   1,
			processingFiles: map[string]struct{}{},
		}
		fw.enqueueFileWithMonitoring("x")
		if len(fw.fileQueue) != 1 {
			t.Fatal("expected one item in queue")
		}
		if !fw.queueWarningLogged {
			t.Fatal("expected warning to be logged at 100% queue fill")
		}
		<-fw.fileQueue
		fw.checkQueueCapacity()
		if fw.queueWarningLogged {
			t.Fatal("expected warning state to reset after queue drains")
		}
	})

	t.Run("waitForCompleteFile success and timeout", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "stable.txt")
		if err := os.WriteFile(file, []byte("stable"), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		fw := &FileWatcher{maxRetries: 1, checkInterval: time.Millisecond, stabilityPeriod: time.Millisecond, lsofAvailable: false}
		if err := fw.waitForCompleteFile(file); err != nil {
			t.Fatalf("expected stable file to pass, got: %v", err)
		}

		fwFail := &FileWatcher{maxRetries: 1, checkInterval: time.Millisecond, stabilityPeriod: time.Millisecond, lsofAvailable: false}
		if err := fwFail.waitForCompleteFile(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
			t.Fatal("expected timeout error for missing file")
		}
	})
}

func TestWorker_validateFilesystemTarget_EmptyPath(t *testing.T) {
	worker := &Worker{}
	if err := worker.validateFilesystemTarget(config.OutputTarget{Type: "filesystem", Path: ""}); err == nil {
		t.Fatal("expected validation error for empty filesystem path")
	}
}
