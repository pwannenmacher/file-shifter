//go:build !windows

package services

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"file-shifter/config"
)

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
