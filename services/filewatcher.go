package services

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
}

func NewFileWatcher(inputDir string, fileHandler *FileHandler, maxRetries int, checkInterval, stabilityPeriod time.Duration) (*FileWatcher, error) {
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
	}

	// lsof-Verfügbarkeit prüfen
	fw.lsofAvailable = checkLsofAvailable()

	return fw, nil
}

func (fw *FileWatcher) Start() error {
	// Watcher für Input-Directory registrieren
	err := fw.addRecursiveWatcher(fw.inputDir)
	if err != nil {
		return err
	}

	slog.Info("File-Watcher gestartet", "directory", fw.inputDir)

	// Verarbeite bereits vorhandene Dateien beim Start
	go fw.processExistingFiles()

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
	fw.stopChan <- true
	fw.watcher.Close()
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

	if err := fw.fileHandler.ProcessFile(filePath, fw.inputDir); err != nil {
		slog.Error("Error processing file", "file", filePath, "error", err)
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
		// 1. Datei-Stabilitätsprüfung
		if !fw.isFileStable(filePath, fw.stabilityPeriod) {
			slog.Debug("File is not yet stable - please continue to wait", "file", filePath, "attempt", retry+1)
			continue
		}

		// 2. Exklusiver Zugriff Test
		if !fw.canOpenExclusively(filePath) {
			slog.Debug("File is still open in another process", "file", filePath, "attempt", retry+1)
			time.Sleep(fw.checkInterval)
			continue
		}

		// 3. lsof-Prüfung (nur Unix/macOS, wenn verfügbar)
		if runtime.GOOS != "windows" && fw.lsofAvailable && fw.isFileOpenByOtherProcess(filePath) {
			slog.Debug("File is still open according to lsof", "file", filePath, "attempt", retry+1)
			time.Sleep(fw.checkInterval)
			continue
		}

		// Alle Prüfungen bestanden
		slog.Info("File is complete and ready for processing", "file", filePath, "attempt", retry+1)
		return nil
	}

	return fmt.Errorf("file is still incomplete after %d attempts: %s", fw.maxRetries, filePath)
}

// isFileStable prüft ob sich Dateigröße und ModTime über checkDuration nicht ändern
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

// canOpenExclusively versucht exklusiven Zugriff auf die Datei zu bekommen
func (fw *FileWatcher) canOpenExclusively(filePath string) bool {
	var file *os.File
	var err error

	if runtime.GOOS == "windows" {
		// Windows: Versuche exklusiven Zugriff
		file, err = os.OpenFile(filePath, os.O_RDONLY, 0)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "being used by another process") {
				return false
			}
			// Anderer Fehler - könnte Berechtigung sein, als "verfügbar" behandeln
			return true
		}
	} else {
		// Unix/Linux/macOS: Versuche mit flock
		file, err = os.Open(filePath)
		if err != nil {
			return false
		}

		// Versuche ein non-blocking exclusive lock
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err != nil {
			file.Close()
			return false
		}
		// Lock wieder freigeben
		syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	}

	if file != nil {
		file.Close()
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
		if exitErr, ok := err.(*exec.ExitError); ok {
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
