package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"file-shifter/config"

	"github.com/fsnotify/fsnotify"
)

// newBacklogTestWatcher creates a FileWatcher for white-box event pipeline
// tests. The watcher is not started; cleanup closes the fsnotify watcher and
// the S3 client manager.
func newBacklogTestWatcher(t *testing.T, inputDir string, targets []config.OutputTarget, workerCount, queueSize int) *FileWatcher {
	t.Helper()

	s3Manager := NewS3ClientManager()
	fileHandler := NewFileHandler(targets, s3Manager)

	watcher, err := NewFileWatcher(inputDir, fileHandler, 2, 5*time.Millisecond, 10*time.Millisecond, workerCount, queueSize)
	if err != nil {
		t.Fatalf("failed to create file watcher: %v", err)
	}

	t.Cleanup(func() {
		if err := watcher.watcher.Close(); err != nil {
			t.Logf("error closing fsnotify watcher: %v", err)
		}
		s3Manager.Close()
	})

	return watcher
}

// backlogWaitUntil polls a condition with a deadline instead of sleeping a
// fixed duration, keeping the tests fast and non-flaky.
func backlogWaitUntil(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v: %s", timeout, msg)
}

// backlogRetryCount reads the retry counter for a path under the retry lock.
func backlogRetryCount(fw *FileWatcher, path string) int {
	fw.retryMu.Lock()
	defer fw.retryMu.Unlock()
	return fw.retryCounts[path]
}

// backlogWarnFlag reads the backlog warning flag under the pending lock.
func backlogWarnFlag(fw *FileWatcher) bool {
	fw.pendingMu.Lock()
	defer fw.pendingMu.Unlock()
	return fw.backlogWarnActive
}

func TestNewFileWatcher_EventPipelineSizing(t *testing.T) {
	tempDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: tempDir}}

	t.Run("minimum values enforced", func(t *testing.T) {
		fw := newBacklogTestWatcher(t, tempDir, targets, 0, 5)

		if fw.eventWorkerCount != 1 {
			t.Errorf("eventWorkerCount = %d, expected minimum of 1 for workerCount 0", fw.eventWorkerCount)
		}
		if fw.backlogWarnThreshold != 100 {
			t.Errorf("backlogWarnThreshold = %d, expected minimum of 100 for queueSize 5", fw.backlogWarnThreshold)
		}
	})

	t.Run("derived from configuration", func(t *testing.T) {
		fw := newBacklogTestWatcher(t, tempDir, targets, 3, 50)

		if fw.eventWorkerCount != 3 {
			t.Errorf("eventWorkerCount = %d, expected 3", fw.eventWorkerCount)
		}
		if fw.backlogWarnThreshold != 500 {
			t.Errorf("backlogWarnThreshold = %d, expected queueSize*10 = 500", fw.backlogWarnThreshold)
		}
		if got := fw.EventBacklogWarnThreshold(); got != 500 {
			t.Errorf("EventBacklogWarnThreshold() = %d, expected 500", got)
		}
	})
}

func TestFileWatcher_StartReturnsWhenAlreadyStopping(t *testing.T) {
	tempDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: tempDir}}
	fw := newBacklogTestWatcher(t, tempDir, targets, 1, 10)

	fw.stopping.Store(true)

	done := make(chan error, 1)
	go func() { done <- fw.Start() }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start() during shutdown should return nil, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start() did not return although shutdown is in progress")
	}
}

func TestFileWatcher_RegisterProducerDuringShutdown(t *testing.T) {
	tempDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: tempDir}}
	fw := newBacklogTestWatcher(t, tempDir, targets, 1, 10)

	if !fw.registerProducer() {
		t.Error("registerProducer() should return true while running")
	}
	fw.producersWG.Done() // balance the successful registration

	fw.stopping.Store(true)
	if fw.registerProducer() {
		t.Error("registerProducer() should return false during shutdown")
	}
	// Must return immediately: no producer may have been registered.
	fw.producersWG.Wait()
}

func TestFileWatcher_SubmitFileStoppingShortCircuit(t *testing.T) {
	tempDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: tempDir}}
	fw := newBacklogTestWatcher(t, tempDir, targets, 1, 10)

	fw.stopping.Store(true)
	fw.submitFile(filepath.Join(tempDir, "ignored.txt"))

	if got := fw.EventBacklog(); got != 0 {
		t.Errorf("backlog should stay empty during shutdown, got %d entries", got)
	}
	fw.processingMutex.Lock()
	marked := len(fw.processingFiles)
	fw.processingMutex.Unlock()
	if marked != 0 {
		t.Errorf("no file should be marked for processing during shutdown, got %d", marked)
	}
}

func TestFileWatcher_SubmitFileBacklogWarnThreshold(t *testing.T) {
	tempDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: tempDir}}
	// No event workers are started, so submitted files accumulate in the backlog.
	fw := newBacklogTestWatcher(t, tempDir, targets, 1, 10)
	fw.backlogWarnThreshold = 3

	fw.submitFile(filepath.Join(tempDir, "a.txt"))
	fw.submitFile(filepath.Join(tempDir, "b.txt"))
	if backlogWarnFlag(fw) {
		t.Error("warn flag must not be active below the threshold")
	}

	fw.submitFile(filepath.Join(tempDir, "c.txt"))
	if !backlogWarnFlag(fw) {
		t.Error("warn flag should activate once the backlog reaches the threshold")
	}
	if got := fw.EventBacklog(); got != 3 {
		t.Errorf("backlog size = %d, expected 3", got)
	}

	// Duplicate submissions are deduplicated - no file is queued twice.
	fw.submitFile(filepath.Join(tempDir, "a.txt"))
	if got := fw.EventBacklog(); got != 3 {
		t.Errorf("duplicate submission changed backlog size to %d, expected 3", got)
	}
}

func TestFileWatcher_NextPendingFileBacklogNormalization(t *testing.T) {
	tempDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: tempDir}}
	fw := newBacklogTestWatcher(t, tempDir, targets, 1, 10)

	fw.backlogWarnThreshold = 4
	fw.pendingMu.Lock()
	fw.backlogWarnActive = true
	fw.pendingFiles = []string{"/tmp/first.txt", "/tmp/second.txt", "/tmp/third.txt"}
	fw.pendingMu.Unlock()

	// First pop: backlog drops to 2, still >= threshold/2 - warning stays active.
	path, ok := fw.nextPendingFile()
	if !ok || path != "/tmp/first.txt" {
		t.Fatalf("nextPendingFile() = (%q, %v), expected (/tmp/first.txt, true)", path, ok)
	}
	if !backlogWarnFlag(fw) {
		t.Error("warn flag should still be active at backlog 2 with threshold 4")
	}

	// Second pop: backlog drops to 1 < threshold/2 - the all-clear is logged.
	path, ok = fw.nextPendingFile()
	if !ok || path != "/tmp/second.txt" {
		t.Fatalf("nextPendingFile() = (%q, %v), expected (/tmp/second.txt, true)", path, ok)
	}
	if backlogWarnFlag(fw) {
		t.Error("warn flag should reset once the backlog drained below half the threshold")
	}

	path, ok = fw.nextPendingFile()
	if !ok || path != "/tmp/third.txt" {
		t.Fatalf("nextPendingFile() = (%q, %v), expected (/tmp/third.txt, true)", path, ok)
	}
}

func TestFileWatcher_NextPendingFileStopping(t *testing.T) {
	tempDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: tempDir}}

	t.Run("empty backlog", func(t *testing.T) {
		fw := newBacklogTestWatcher(t, tempDir, targets, 1, 10)
		fw.stopping.Store(true)

		if path, ok := fw.nextPendingFile(); ok {
			t.Errorf("nextPendingFile() during shutdown should return false, got (%q, true)", path)
		}
	})

	t.Run("non-empty backlog", func(t *testing.T) {
		fw := newBacklogTestWatcher(t, tempDir, targets, 1, 10)
		fw.pendingMu.Lock()
		fw.pendingFiles = []string{"/tmp/pending.txt"}
		fw.pendingMu.Unlock()
		fw.stopping.Store(true)

		if path, ok := fw.nextPendingFile(); ok {
			t.Errorf("nextPendingFile() during shutdown should return false, got (%q, true)", path)
		}
	})
}

func TestFileWatcher_HandleModificationEventDirectory(t *testing.T) {
	inputDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: t.TempDir()}}
	fw := newBacklogTestWatcher(t, inputDir, targets, 1, 10)

	subDir := filepath.Join(inputDir, "newdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	innerFile := filepath.Join(subDir, "inner.txt")
	if err := os.WriteFile(innerFile, []byte("inner content"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	fw.handleModificationEvent(fsnotify.Event{Name: subDir, Op: fsnotify.Create})

	// The spawned producer walks the new directory and submits the contained
	// file. No event workers run, so it stays visible in the backlog.
	backlogWaitUntil(t, 5*time.Second, "file inside new directory submitted to backlog", func() bool {
		return fw.EventBacklog() == 1
	})
	fw.producersWG.Wait()

	fw.pendingMu.Lock()
	pending := append([]string(nil), fw.pendingFiles...)
	fw.pendingMu.Unlock()
	if len(pending) != 1 || pending[0] != innerFile {
		t.Errorf("pending backlog = %v, expected exactly [%s]", pending, innerFile)
	}
}

func TestFileWatcher_HandleModificationEventDirectoryDuringShutdown(t *testing.T) {
	inputDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: t.TempDir()}}
	fw := newBacklogTestWatcher(t, inputDir, targets, 1, 10)

	subDir := filepath.Join(inputDir, "stopdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	fw.stopping.Store(true)
	fw.handleModificationEvent(fsnotify.Event{Name: subDir, Op: fsnotify.Create})

	// registerProducer must have refused - Wait returns immediately.
	fw.producersWG.Wait()
	if got := fw.EventBacklog(); got != 0 {
		t.Errorf("backlog should stay empty during shutdown, got %d entries", got)
	}
}

func TestFileWatcher_ScheduleRetryStoppingShortCircuit(t *testing.T) {
	inputDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: t.TempDir()}}
	fw := newBacklogTestWatcher(t, inputDir, targets, 1, 10)

	filePath := filepath.Join(inputDir, "retry.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	fw.stopping.Store(true)
	fw.scheduleRetry(filePath)

	if got := backlogRetryCount(fw, filePath); got != 0 {
		t.Errorf("retry count = %d during shutdown, expected 0 (early return)", got)
	}
}

func TestFileWatcher_ScheduleRetryMissingFileClearsState(t *testing.T) {
	inputDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: t.TempDir()}}
	fw := newBacklogTestWatcher(t, inputDir, targets, 1, 10)

	missingPath := filepath.Join(inputDir, "vanished.txt")
	fw.retryMu.Lock()
	fw.retryCounts[missingPath] = 3
	fw.retryMu.Unlock()

	fw.scheduleRetry(missingPath)

	if got := backlogRetryCount(fw, missingPath); got != 0 {
		t.Errorf("retry state for a vanished file should be cleared, count = %d", got)
	}
}

func TestFileWatcher_ScheduleRetrySchedulesResubmission(t *testing.T) {
	inputDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: t.TempDir()}}
	fw := newBacklogTestWatcher(t, inputDir, targets, 1, 10)

	filePath := filepath.Join(inputDir, "retry.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	fw.scheduleRetry(filePath)
	if got := backlogRetryCount(fw, filePath); got != 1 {
		t.Errorf("retry count = %d after first scheduleRetry, expected 1", got)
	}

	fw.scheduleRetry(filePath)
	if got := backlogRetryCount(fw, filePath); got != 2 {
		t.Errorf("retry count = %d after second scheduleRetry, expected 2", got)
	}

	// Tear down the pending retry goroutines: closing stopChan makes them
	// exit without re-submitting the file.
	fw.stopping.Store(true)
	close(fw.stopChan)
	fw.producersWG.Wait()

	if got := fw.EventBacklog(); got != 0 {
		t.Errorf("retry goroutines must not submit during shutdown, backlog = %d", got)
	}
}

func TestRetryBackoffBoundaries(t *testing.T) {
	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{attempt: 1, expected: 5 * time.Second},
		{attempt: 2, expected: 10 * time.Second},
		{attempt: 3, expected: 20 * time.Second},
		{attempt: 6, expected: 160 * time.Second},
		{attempt: 7, expected: 5 * time.Minute},
		{attempt: 100, expected: 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			if got := retryBackoff(tt.attempt); got != tt.expected {
				t.Errorf("retryBackoff(%d) = %v, expected %v", tt.attempt, got, tt.expected)
			}
		})
	}
}

func TestFileWatcher_WorkerSchedulesRetryOnProcessingError(t *testing.T) {
	inputDir := t.TempDir()
	// An unknown target type makes every transfer fail deterministically.
	targets := []config.OutputTarget{{Type: "unsupported", Path: "/nonexistent"}}
	fw := newBacklogTestWatcher(t, inputDir, targets, 1, 4)

	filePath := filepath.Join(inputDir, "failing.txt")
	if err := os.WriteFile(filePath, []byte("content that cannot be transferred"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	fw.startWorkers()
	if !fw.tryMarkFileForProcessing(filePath) {
		t.Fatal("failed to mark file for processing")
	}
	fw.fileQueue <- filePath

	backlogWaitUntil(t, 10*time.Second, "worker scheduled a retry for the failed transfer", func() bool {
		return backlogRetryCount(fw, filePath) >= 1
	})

	// Per project principle the file must never be lost: after a failed
	// transfer it stays in the input directory.
	if _, err := os.Lstat(filePath); err != nil {
		t.Errorf("failed file must remain in the input directory: %v", err)
	}

	// Orderly teardown mirroring Stop(): set the stop flag under the
	// lifecycle lock, drain the worker pool first (the worker may still be
	// inside scheduleRetry and register a producer), then wait for any retry
	// goroutine it spawned.
	fw.lifecycleMu.Lock()
	fw.stopping.Store(true)
	fw.lifecycleMu.Unlock()
	close(fw.stopChan)
	close(fw.fileQueue)
	fw.workers.Wait()
	fw.producersWG.Wait()
}

func TestFileWatcher_WaitForCompleteFileShutdownAbort(t *testing.T) {
	inputDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: t.TempDir()}}
	fw := newBacklogTestWatcher(t, inputDir, targets, 1, 10)

	filePath := filepath.Join(inputDir, "complete.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	fw.stopping.Store(true)
	err := fw.waitForCompleteFile(filePath)
	if err == nil {
		t.Fatal("waitForCompleteFile() during shutdown should return an error")
	}
	if !strings.Contains(err.Error(), "shutdown in progress") {
		t.Errorf("unexpected error: %v, expected 'shutdown in progress'", err)
	}
}

func TestFileWatcher_CanOpenExclusivelyLockedFile(t *testing.T) {
	inputDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: t.TempDir()}}
	fw := newBacklogTestWatcher(t, inputDir, targets, 1, 10)

	filePath := filepath.Join(inputDir, "locked.txt")
	if err := os.WriteFile(filePath, []byte("locked content"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Hold an exclusive flock on a separate file descriptor - the watcher's
	// non-blocking lock attempt must fail.
	holder, err := os.OpenFile(filePath, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("failed to open lock holder: %v", err)
	}
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("failed to acquire exclusive lock: %v", err)
	}

	if fw.canOpenExclusively(filePath) {
		t.Error("canOpenExclusively() should fail while another descriptor holds an exclusive lock")
	}

	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatalf("failed to release lock: %v", err)
	}
	if err := holder.Close(); err != nil {
		t.Fatalf("failed to close lock holder: %v", err)
	}

	if !fw.canOpenExclusively(filePath) {
		t.Error("canOpenExclusively() should succeed after the lock is released")
	}
}

func TestFileWatcher_ProcessExistingFilesSubmitsToBacklog(t *testing.T) {
	inputDir := t.TempDir()
	targets := []config.OutputTarget{{Type: "filesystem", Path: t.TempDir()}}
	fw := newBacklogTestWatcher(t, inputDir, targets, 1, 10)

	subDir := filepath.Join(inputDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}
	for _, name := range []string{"one.txt", filepath.Join("sub", "two.txt")} {
		if err := os.WriteFile(filepath.Join(inputDir, name), []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to create file %s: %v", name, err)
		}
	}
	// Hidden files are filtered by submitFile and must not enter the backlog.
	if err := os.WriteFile(filepath.Join(inputDir, ".hidden.txt"), []byte("hidden"), 0o644); err != nil {
		t.Fatalf("failed to create hidden file: %v", err)
	}

	fw.processExistingFiles()

	if got := fw.EventBacklog(); got != 2 {
		t.Errorf("backlog = %d after startup scan, expected 2 (hidden file filtered)", got)
	}
}

func TestFileWatcher_ProcessExistingFilesMissingDirectory(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "does-not-exist")
	targets := []config.OutputTarget{{Type: "filesystem", Path: t.TempDir()}}
	fw := newBacklogTestWatcher(t, missingDir, targets, 1, 10)

	// Must log the walk error without panicking and submit nothing.
	fw.processExistingFiles()

	if got := fw.EventBacklog(); got != 0 {
		t.Errorf("backlog = %d for a missing input directory, expected 0", got)
	}
}
