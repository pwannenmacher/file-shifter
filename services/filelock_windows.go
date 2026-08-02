//go:build windows

package services

import (
	"log/slog"
	"os"

	"golang.org/x/sys/windows"
)

// tryLockFileExclusively probes for exclusive access to an open file using a
// non-blocking LockFileEx. The lock is released immediately; only the probe
// result matters.
func tryLockFileExclusively(file *os.File) bool {
	handle := windows.Handle(file.Fd())
	overlapped := new(windows.Overlapped)

	err := windows.LockFileEx(handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, overlapped)
	if err != nil {
		return false
	}
	if err := windows.UnlockFileEx(handle, 0, 1, 0, overlapped); err != nil {
		slog.Error("Error unlocking file", "file", file.Name(), "error", err)
	}
	return true
}
