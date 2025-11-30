package services

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

type FileWatcher struct {
	watcher         *fsnotify.Watcher
	inputDir        string
	fileHandler     *FileHandler
	stopChan        chan bool
	maxRetries      int
	checkInterval   time.Duration
	stabilityPeriod time.Duration
	lsofAvailable   bool
	// Worker pool for parallel processing
	fileQueue   chan string
	workerCount int
	workers     sync.WaitGroup
	// Queue monitoring
	queueCapacity      int
	queueWarningLogged bool
	queueMutex         sync.Mutex
}

func NewFileWatcher(inputDir string, fileHandler *FileHandler, maxRetries int, checkInterval, stabilityPeriod time.Duration, workerCount, queueSize int) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	fw := &FileWatcher{
		watcher:         watcher,
		inputDir:        inputDir,
		fileHandler:     fileHandler,
		stopChan:        make(chan bool),
		maxRetries:      maxRetries,
		checkInterval:   checkInterval,
		stabilityPeriod: stabilityPeriod,
		fileQueue:       make(chan string, queueSize), // Configurable queue size
		workerCount:     workerCount,                  // Configurable worker count
		queueCapacity:   queueSize,                    // Store capacity for monitoring
	}

	// Check lsof availability
	fw.lsofAvailable = checkLsofAvailable()

	return fw, nil
}

func (fw *FileWatcher) Start() error {
	// Register watcher for input directory
	err := fw.addRecursiveWatcher(fw.inputDir)
	if err != nil {
		return err
	}

	slog.Info("File-Watcher started", "directory", fw.inputDir)

	// Process existing files at startup
	go fw.processExistingFiles()

	// Start worker pool
	fw.startWorkers()

	// Event-Loop
	for {
		select {
		case <-fw.stopChan:
			slog.Info("File-Watcher stopped")
			return nil

		case event, ok := <-fw.watcher.Events:
			if !ok {
				return nil
			}
			fw.handleEvent(event)

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return nil
			}
			slog.Error("File-Watcher error", "error", err)
		}
	}
}

func (fw *FileWatcher) Stop() {
	close(fw.fileQueue) // Close the queue to terminate workers
	fw.workers.Wait()   // Wait until all workers have finished
	fw.stopChan <- true
	err := fw.watcher.Close()
	if err != nil {
		slog.Error("Error closing file watcher", "error", err)
	}
	slog.Info("File-Watcher completely stopped")
}

func (fw *FileWatcher) addRecursiveWatcher(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fw.watcher.Add(path)
		}
		return nil
	})
}

func (fw *FileWatcher) handleEvent(event fsnotify.Event) {
	slog.Debug("File-System event received", "event", event.Name, "op", event.Op)

	if fw.isRemoveOrRenameEvent(event) {
		fw.handleRemoveEvent(event)
		return
	}

	if fw.isModificationEvent(event) {
		fw.handleModificationEvent(event)
	}
}

// isRemoveOrRenameEvent checks if the event is a remove or rename operation
func (fw *FileWatcher) isRemoveOrRenameEvent(event fsnotify.Event) bool {
	return event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename
}

// isModificationEvent checks if the event is a create, write, or chmod operation
func (fw *FileWatcher) isModificationEvent(event fsnotify.Event) bool {
	return event.Op&fsnotify.Create == fsnotify.Create ||
		event.Op&fsnotify.Write == fsnotify.Write ||
		event.Op&fsnotify.Chmod == fsnotify.Chmod
}

// handleRemoveEvent handles file or directory removal/rename events
func (fw *FileWatcher) handleRemoveEvent(event fsnotify.Event) {
	slog.Info("Path removed or renamed", "path", event.Name, "op", event.Op)

	// Remove the watcher if it exists (will fail silently if not watched)
	// This is important for cleanup and memory management
	if err := fw.watcher.Remove(event.Name); err != nil {
		slog.Debug("Error removing watcher (may not have been watched)", "path", event.Name, "error", err)
	}
}

// handleModificationEvent handles file creation, modification, or permission change events
func (fw *FileWatcher) handleModificationEvent(event fsnotify.Event) {
	info, err := os.Stat(event.Name)
	if err != nil {
		slog.Debug("Error reading file info", "file", event.Name, "error", err)
		return
	}

	if info.IsDir() {
		fw.handleDirectoryCreation(event)
		return
	}

	fw.processFile(event.Name)
}

// handleDirectoryCreation handles new directory creation events
func (fw *FileWatcher) handleDirectoryCreation(event fsnotify.Event) {
	// Only add watcher for newly created directories
	if event.Op&fsnotify.Create != fsnotify.Create {
		return
	}

	if err := fw.watcher.Add(event.Name); err != nil {
		slog.Error("Error adding watcher for new directory", "directory", event.Name, "error", err)
	} else {
		slog.Debug("Watcher added for new directory", "directory", event.Name)
	}
}

func (fw *FileWatcher) processFile(filePath string) {
	// Check whether the file still exists (it may have been deleted in the meantime).
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		slog.Debug("File no longer exists", "file", filePath)
		return
	}

	fileName := filepath.Base(filePath)
	if fileName[0] == '.' || fileName[0] == '~' {
		slog.Debug("Ignore temporary/hidden file", "file", filePath)
		return
	}

	slog.Info("New file detected", "file", filePath)

	if err := fw.waitForCompleteFile(filePath); err != nil {
		slog.Error("File is not complete - processing skipped", "file", filePath, "error", err)
		return
	}

	// Enqueue file for processing with queue monitoring
	fw.enqueueFileWithMonitoring(filePath)
}

// enqueueFileWithMonitoring adds a file to the queue and monitors capacity
func (fw *FileWatcher) enqueueFileWithMonitoring(filePath string) {
	// Add file to queue
	fw.fileQueue <- filePath

	// Queue monitoring after adding
	fw.checkQueueCapacity()
}

// checkQueueCapacity monitors queue fill level and outputs warnings
func (fw *FileWatcher) checkQueueCapacity() {
	fw.queueMutex.Lock()
	defer fw.queueMutex.Unlock()

	currentSize := len(fw.fileQueue)
	capacity := fw.queueCapacity
	fillPercentage := float64(currentSize) / float64(capacity) * 100

	// 80% threshold for warning
	warningThreshold := 80.0

	if fillPercentage >= warningThreshold {
		// Output warning if not yet logged
		if !fw.queueWarningLogged {
			slog.Warn("FileQueue capacity critical",
				"current_size", currentSize,
				"capacity", capacity,
				"fill_percentage", fmt.Sprintf("%.1f%%", fillPercentage),
				"message", "The file queue is 80% or more full. Consider configuring more workers or increasing the queue size.")
			fw.queueWarningLogged = true
		}
	} else {
		// Output all-clear if warning was previously active
		if fw.queueWarningLogged {
			slog.Info("FileQueue capacity normalized",
				"current_size", currentSize,
				"capacity", capacity,
				"fill_percentage", fmt.Sprintf("%.1f%%", fillPercentage),
				"message", "The file queue is back below 80% capacity.")
			fw.queueWarningLogged = false
		}
	}
}

func (fw *FileWatcher) worker() {
	defer fw.workers.Done()

	for filePath := range fw.fileQueue {
		if err := fw.fileHandler.ProcessFile(filePath, fw.inputDir); err != nil {
			slog.Error("Error processing file", "file", filePath, "error", err)
		}

		// Queue monitoring after processing a file
		fw.checkQueueCapacity()
	}
}

func (fw *FileWatcher) startWorkers() {
	slog.Info("Starting worker pool", "count", fw.workerCount)
	fw.workers.Add(fw.workerCount)
	for i := 0; i < fw.workerCount; i++ {
		go fw.worker()
	}
}

func (fw *FileWatcher) processExistingFiles() {
	slog.Info("Search for existing files in the input directory")

	err := filepath.Walk(fw.inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only process files, not directories
		if !info.IsDir() {
			fw.processFile(path)
		}

		return nil
	})

	if err != nil {
		slog.Error("Error processing existing files", "error", err)
	}
}

// waitForCompleteFile waits until a file is complete (no more writing is taking place)
func (fw *FileWatcher) waitForCompleteFile(filePath string) error {
	slog.Debug("Check file completeness", "file", filePath)

	for retry := 0; retry < fw.maxRetries; retry++ {
		// 1. File stability check
		if !fw.isFileStable(filePath, fw.stabilityPeriod) {
			slog.Debug("File is not yet stable - please continue to wait", "file", filePath, "attempt", retry+1)
			continue
		}

		// 2. Exclusive access test
		if !fw.canOpenExclusively(filePath) {
			slog.Debug("File is still open in another process", "file", filePath, "attempt", retry+1)
			time.Sleep(fw.checkInterval)
			continue
		}

		// 3. lsof check (Unix/macOS only, if available)
		if runtime.GOOS != "windows" && fw.lsofAvailable && fw.isFileOpenByOtherProcess(filePath) {
			slog.Debug("File is still open according to lsof", "file", filePath, "attempt", retry+1)
			time.Sleep(fw.checkInterval)
			continue
		}

		slog.Info("File is complete and ready for processing", "file", filePath, "attempt", retry+1)
		return nil
	}

	return fmt.Errorf("file is still incomplete after %d attempts: %s", fw.maxRetries, filePath)
}

// isFileStable checks whether file size and ModTime do not change via checkDuration
func (fw *FileWatcher) isFileStable(filePath string, checkDuration time.Duration) bool {
	initialStat, err := os.Stat(filePath)
	if err != nil {
		slog.Debug("Error during initialisation", "file", filePath, "error", err)
		return false
	}

	time.Sleep(checkDuration)

	finalStat, err := os.Stat(filePath)
	if err != nil {
		slog.Debug("Error in the second stat", "file", filePath, "error", err)
		return false
	}

	stable := initialStat.Size() == finalStat.Size() &&
		initialStat.ModTime().Equal(finalStat.ModTime())

	if !stable {
		slog.Debug("File instability detected",
			"file", filePath,
			"size_old", initialStat.Size(),
			"size_new", finalStat.Size(),
			"timestamp_old", initialStat.ModTime(),
			"timestamp_new", finalStat.ModTime())
	}

	return stable
}

// safeCloseFile closes a file safely and logs errors
func (fw *FileWatcher) safeCloseFile(file *os.File, filePath string) {
	if err := file.Close(); err != nil {
		slog.Error("Error closing file", "file", filePath, "error", err)
	}
}

// canOpenExclusively attempts to gain exclusive access to the file
func (fw *FileWatcher) canOpenExclusively(filePath string) bool {
	var file *os.File
	var err error

	if runtime.GOOS == "windows" {
		// Windows: Attempt exclusive access
		file, err = os.OpenFile(filePath, os.O_RDONLY, 0)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "being used by another process") {
				return false
			}
			// Other error - could be permission, treat as "available"
			return true
		}
	} else {
		// Unix/Linux/macOS: Try using flock
		file, err = os.Open(filePath)
		if err != nil {
			return false
		}

		// Attempt a non-blocking exclusive lock
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err != nil {
			fw.safeCloseFile(file, filePath)
			return false
		}
		// Release exclusive lock
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
			slog.Error("Error unlocking file", "file", filePath, "error", err)
		}
	}

	if file != nil {
		fw.safeCloseFile(file, filePath)
	}
	return true
}

// isFileOpenByOtherProcess uses lsof to check whether the file is open by other processes
func (fw *FileWatcher) isFileOpenByOtherProcess(filePath string) bool {
	if runtime.GOOS == "windows" {
		return false // lsof is not available on Windows
	}

	output, err := fw.executeLsof(filePath)
	if err != nil {
		return false
	}

	return fw.hasRelevantProcesses(filePath, output)
}

// isHarmlessProcess checks whether a process can be classified as harmless.
func (fw *FileWatcher) isHarmlessProcess(processName string) bool {
	harmlessProcesses := []string{
		"mds", "mds_stores", "mdworker", "mdworker_shared", // macOS Spotlight
		"fsevents", "fseventsd", // Filesystem Events
		"Finder", "QuickLookSatellite", // macOS Finder
		"antivir", "avguard", "avscan", // Antivirus (read-only scans)
	}

	lowerProcessName := strings.ToLower(processName)
	for _, harmless := range harmlessProcesses {
		if strings.Contains(lowerProcessName, strings.ToLower(harmless)) {
			return true
		}
	}
	return false
}

// executeLsof executes the lsof command and handles errors
func (fw *FileWatcher) executeLsof(filePath string) (string, error) {
	cmd := exec.Command("lsof", filePath)
	output, err := cmd.Output()

	if err != nil {
		// lsof exit code 1 means ‘no open files found’ – that's good.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == 1 {
				return "", fmt.Errorf("no open files")
			}
		}
		// Other error (authorisation, etc.) - treat as an error
		slog.Debug("lsof error ignored", "file", filePath, "error", err)
		return "", err
	}

	return string(output), nil
}

// hasRelevantProcesses checks whether relevant processes have the file open
func (fw *FileWatcher) hasRelevantProcesses(filePath, lsofOutput string) bool {
	lines := strings.Split(strings.TrimSpace(lsofOutput), "\n")
	if len(lines) <= 1 {
		return false // Header only or empty
	}

	// Skip header and analyse processes
	for _, line := range lines[1:] {
		if fw.isRelevantProcess(filePath, line) {
			return true
		}
	}

	return false
}

// QueueSize returns the current size of the file queue
func (fw *FileWatcher) QueueSize() int {
	return len(fw.fileQueue)
}

// QueueCapacity returns the maximum capacity of the file queue
func (fw *FileWatcher) QueueCapacity() int {
	return fw.queueCapacity
}

// WorkerCount returns the number of workers
func (fw *FileWatcher) WorkerCount() int {
	return fw.workerCount
}

// isRelevantProcess checks whether a process in the lsof line is relevant
func (fw *FileWatcher) isRelevantProcess(filePath, line string) bool {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return false
	}

	processName := fields[0]
	pid := fields[1]

	// Ignore own process
	if pid == strconv.Itoa(os.Getpid()) {
		return false
	}

	// Ignore known harmless processes
	if fw.isHarmlessProcess(processName) {
		return false
	}

	slog.Debug("Active process detected", "file", filePath, "process", processName, "pid", pid)
	return true
}

// checkLsofAvailable checks if lsof command is available
func checkLsofAvailable() bool {
	if runtime.GOOS == "windows" {
		return false
	}

	_, err := exec.LookPath("lsof")
	if err != nil {
		slog.Debug("lsof command not available - lsof checks will be skipped", "error", err)
		return false
	}

	slog.Debug("lsof command available - advanced file checks enabled")
	return true
}
