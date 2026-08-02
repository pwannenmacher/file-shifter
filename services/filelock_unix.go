//go:build !windows

package services

import (
	"log/slog"
	"os"
	"syscall"
)

// tryLockFileExclusively probes for exclusive access to an open file using a
// non-blocking flock. The lock is released immediately; only the probe result
// matters.
func tryLockFileExclusively(file *os.File) bool {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return false
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		slog.Error("Error unlocking file", "file", file.Name(), "error", err)
	}
	return true
}
