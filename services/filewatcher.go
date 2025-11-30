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

	slog.Info("File-Watcher gestartet", "directory", fw.inputDir)

	// Process existing files at startup
	go fw.processExistingFiles()

	// Start worker pool
	fw.startWorkers()

	// Event-Loop
	for {
		select {
		case <-fw.stopChan:
			slog.Info("File-Watcher gestoppt")
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
			slog.Error("File-Watcher Fehler", "error", err)
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
	slog.Info("File-Watcher vollständig gestoppt")
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
	slog.Debug("File-System Event empfangen", "event", event.Name, "op", event.Op)

	// Process CREATE, WRITE, and CHMOD events
	if event.Op&fsnotify.Create == fsnotify.Create ||
		event.Op&fsnotify.Write == fsnotify.Write ||
		event.Op&fsnotify.Chmod == fsnotify.Chmod {

		// Check whether it is a file
		info, err := os.Stat(event.Name)
		if err != nil {
			slog.Debug("Error reading file info", "file", event.Name, "error", err)
			return
		}

		if info.IsDir() {
			// New directory - Add watcher
			if event.Op&fsnotify.Create == fsnotify.Create {
				if err := fw.watcher.Add(event.Name); err != nil {
					slog.Error("Error adding watcher for new directory", "directory", event.Name, "error", err)
				}
			}
			return
		}

		fw.processFile(event.Name)
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

	slog.Info("Neue Datei erkannt", "file", filePath)

	if err := fw.waitForCompleteFile(filePath); err != nil {
		slog.Error("Datei ist nicht vollständig - Verarbeitung übersprungen", "file", filePath, "error", err)
		return
	}

	// Enqueue file for processing with queue monitoring
	fw.enqueueFileWithMonitoring(filePath)
}

// enqueueFileWithMonitoring fügt eine Datei zur Warteschlange hinzu und überwacht die Kapazität
func (fw *FileWatcher) enqueueFileWithMonitoring(filePath string) {
	// Datei zur Warteschlange hinzufügen
	fw.fileQueue <- filePath

	// Queue-Monitoring nach dem Hinzufügen
	fw.checkQueueCapacity()
}

// checkQueueCapacity überwacht die Queue-Füllung und gibt Warnungen aus
func (fw *FileWatcher) checkQueueCapacity() {
	fw.queueMutex.Lock()
	defer fw.queueMutex.Unlock()

	currentSize := len(fw.fileQueue)
	capacity := fw.queueCapacity
	fillPercentage := float64(currentSize) / float64(capacity) * 100

	// 80% Schwellwert für Warnung
	warningThreshold := 80.0

	if fillPercentage >= warningThreshold {
		// Warnung ausgeben, wenn noch nicht geloggt
		if !fw.queueWarningLogged {
			slog.Warn("FileQueue-Kapazität kritisch",
				"current_size", currentSize,
				"capacity", capacity,
				"fill_percentage", fmt.Sprintf("%.1f%%", fillPercentage),
				"message", "Die Datei-Warteschlange ist zu 80% oder mehr gefüllt. Erwägen Sie, mehr Worker zu konfigurieren oder die Queue-Größe zu erhöhen.")
			fw.queueWarningLogged = true
		}
	} else {
		// Entwarnung ausgeben, wenn Warnung vorher aktiv war
		if fw.queueWarningLogged {
			slog.Info("FileQueue-Kapazität normalisiert",
				"current_size", currentSize,
				"capacity", capacity,
				"fill_percentage", fmt.Sprintf("%.1f%%", fillPercentage),
				"message", "Die Datei-Warteschlange ist wieder unter 80% Kapazität.")
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

		// Queue-Monitoring nach dem Verarbeiten einer Datei
		fw.checkQueueCapacity()
	}
}

func (fw *FileWatcher) startWorkers() {
	slog.Info("Starte Worker-Pool", "anzahl", fw.workerCount)
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

		// Nur Dateien verarbeiten, keine Verzeichnisse
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
			// Anderer Fehler - könnte Berechtigung sein, als "verfügbar" behandeln
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

// GetQueueSize returns the current size of the file queue
func (fw *FileWatcher) GetQueueSize() int {
	return len(fw.fileQueue)
}

// GetQueueCapacity returns the maximum capacity of the file queue
func (fw *FileWatcher) GetQueueCapacity() int {
	return fw.queueCapacity
}

// GetWorkerCount returns the number of workers
func (fw *FileWatcher) GetWorkerCount() int {
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

	slog.Debug("Active process detected", "file", filePath, "prozess", processName, "pid", pid)
	return true
}

// checkLsofAvailable prüft ob lsof-Kommando verfügbar ist
func checkLsofAvailable() bool {
	if runtime.GOOS == "windows" {
		return false
	}

	_, err := exec.LookPath("lsof")
	if err != nil {
		slog.Debug("lsof-Kommando nicht verfügbar - lsof-Prüfungen werden übersprungen", "error", err)
		return false
	}

	slog.Debug("lsof command available - advanced file checks enabled")
	return true
}
