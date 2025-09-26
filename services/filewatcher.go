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
			slog.Error("File-Watcher Fehler", "fehler", err)
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

	// Bei CREATE, WRITE und CHMOD Events verarbeiten
	if event.Op&fsnotify.Create == fsnotify.Create ||
		event.Op&fsnotify.Write == fsnotify.Write ||
		event.Op&fsnotify.Chmod == fsnotify.Chmod {

		// Überprüfen, ob es eine Datei ist
		info, err := os.Stat(event.Name)
		if err != nil {
			slog.Debug("Fehler beim Lesen der Datei-Info", "datei", event.Name, "fehler", err)
			return
		}

		if info.IsDir() {
			// Neues Verzeichnis - Watcher hinzufügen
			if event.Op&fsnotify.Create == fsnotify.Create {
				if err := fw.watcher.Add(event.Name); err != nil {
					slog.Error("Fehler beim Hinzufügen des Watcher für neues Verzeichnis", "verzeichnis", event.Name, "fehler", err)
				}
			}
			return
		}

		// Datei verarbeiten
		fw.processFile(event.Name)
	}
}

func (fw *FileWatcher) processFile(filePath string) {
	// Überprüfen, ob Datei noch existiert (könnte zwischenzeitlich gelöscht worden sein)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		slog.Debug("Datei existiert nicht mehr", "datei", filePath)
		return
	}

	// Temporäre Dateien und versteckte Dateien ignorieren
	fileName := filepath.Base(filePath)
	if fileName[0] == '.' || fileName[0] == '~' {
		slog.Debug("Ignoriere temporäre/versteckte Datei", "datei", filePath)
		return
	}

	slog.Info("Neue Datei erkannt", "datei", filePath)

	// Warten bis Datei vollständig ist
	if err := fw.waitForCompleteFile(filePath); err != nil {
		slog.Error("Datei ist nicht vollständig - Verarbeitung übersprungen", "datei", filePath, "fehler", err)
		return
	}

	// Datei über FileHandler verarbeiten
	if err := fw.fileHandler.ProcessFile(filePath, fw.inputDir); err != nil {
		slog.Error("Fehler beim Verarbeiten der Datei", "datei", filePath, "fehler", err)
	}
}

func (fw *FileWatcher) processExistingFiles() {
	slog.Info("Suche nach bereits vorhandenen Dateien im Input-Directory")

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
		slog.Error("Fehler beim Verarbeiten vorhandener Dateien", "fehler", err)
	}
}

// waitForCompleteFile wartet bis eine Datei vollständig ist (nicht mehr geschrieben wird)
func (fw *FileWatcher) waitForCompleteFile(filePath string) error {
	slog.Debug("Prüfe Vollständigkeit der Datei", "datei", filePath)

	for retry := 0; retry < fw.maxRetries; retry++ {
		// 1. Datei-Stabilitätsprüfung
		if !fw.isFileStable(filePath, fw.stabilityPeriod) {
			slog.Debug("Datei ist noch nicht stabil - warte weiter", "datei", filePath, "versuch", retry+1)
			continue
		}

		// 2. Exklusiver Zugriff Test
		if !fw.canOpenExclusively(filePath) {
			slog.Debug("Datei ist noch von anderem Prozess geöffnet", "datei", filePath, "versuch", retry+1)
			time.Sleep(fw.checkInterval)
			continue
		}

		// 3. lsof-Prüfung (nur Unix/macOS, wenn verfügbar)
		if runtime.GOOS != "windows" && fw.lsofAvailable && fw.isFileOpenByOtherProcess(filePath) {
			slog.Debug("Datei ist laut lsof noch geöffnet", "datei", filePath, "versuch", retry+1)
			time.Sleep(fw.checkInterval)
			continue
		}

		// Alle Prüfungen bestanden
		slog.Info("Datei ist vollständig und bereit zur Verarbeitung", "datei", filePath, "versuche", retry+1)
		return nil
	}

	return fmt.Errorf("datei ist nach %d Versuchen noch nicht vollständig: %s", fw.maxRetries, filePath)
}

// isFileStable prüft ob sich Dateigröße und ModTime über checkDuration nicht ändern
func (fw *FileWatcher) isFileStable(filePath string, checkDuration time.Duration) bool {
	initialStat, err := os.Stat(filePath)
	if err != nil {
		slog.Debug("Fehler beim ersten Stat", "datei", filePath, "fehler", err)
		return false
	}

	time.Sleep(checkDuration)

	finalStat, err := os.Stat(filePath)
	if err != nil {
		slog.Debug("Fehler beim zweiten Stat", "datei", filePath, "fehler", err)
		return false
	}

	stable := initialStat.Size() == finalStat.Size() &&
		initialStat.ModTime().Equal(finalStat.ModTime())

	if !stable {
		slog.Debug("Datei-Instabilität erkannt",
			"datei", filePath,
			"größe_alt", initialStat.Size(),
			"größe_neu", finalStat.Size(),
			"zeit_alt", initialStat.ModTime(),
			"zeit_neu", finalStat.ModTime())
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

// isFileOpenByOtherProcess prüft mit lsof ob die Datei von anderen Prozessen geöffnet ist
func (fw *FileWatcher) isFileOpenByOtherProcess(filePath string) bool {
	if runtime.GOOS == "windows" {
		return false // lsof gibt es nicht unter Windows
	}

	output, err := fw.executeLsof(filePath)
	if err != nil {
		return false
	}

	return fw.hasRelevantProcesses(filePath, output)
}

// isHarmlessProcess prüft ob ein Prozess als harmlos eingestuft werden kann
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

// executeLsof führt lsof-Kommando aus und behandelt Fehler
func (fw *FileWatcher) executeLsof(filePath string) (string, error) {
	cmd := exec.Command("lsof", filePath)
	output, err := cmd.Output()

	if err != nil {
		// lsof exit code 1 bedeutet "keine offenen Files gefunden" - das ist gut
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return "", fmt.Errorf("no open files")
			}
		}
		// Anderer Fehler (Berechtigung, etc.) - als Fehler behandeln
		slog.Debug("lsof-Fehler ignoriert", "datei", filePath, "fehler", err)
		return "", err
	}

	return string(output), nil
}

// hasRelevantProcesses prüft ob relevante Prozesse die Datei offen haben
func (fw *FileWatcher) hasRelevantProcesses(filePath, lsofOutput string) bool {
	lines := strings.Split(strings.TrimSpace(lsofOutput), "\n")
	if len(lines) <= 1 {
		return false // Nur Header oder leer
	}

	// Header überspringen und Prozesse analysieren
	for _, line := range lines[1:] {
		if fw.isRelevantProcess(filePath, line) {
			return true
		}
	}

	return false
}

// isRelevantProcess prüft ob ein Prozess in der lsof-Zeile relevant ist
func (fw *FileWatcher) isRelevantProcess(filePath, line string) bool {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return false
	}

	processName := fields[0]
	pid := fields[1]

	// Eigenen Prozess ignorieren
	if pid == strconv.Itoa(os.Getpid()) {
		return false
	}

	// Bekannte harmlose Prozesse ignorieren
	if fw.isHarmlessProcess(processName) {
		return false
	}

	slog.Debug("Aktiver Prozess erkannt", "datei", filePath, "prozess", processName, "pid", pid)
	return true
}

// checkLsofAvailable prüft ob lsof-Kommando verfügbar ist
func checkLsofAvailable() bool {
	if runtime.GOOS == "windows" {
		return false
	}

	_, err := exec.LookPath("lsof")
	if err != nil {
		slog.Debug("lsof-Kommando nicht verfügbar - lsof-Prüfungen werden übersprungen", "fehler", err)
		return false
	}

	slog.Debug("lsof-Kommando verfügbar - erweiterte Datei-Prüfungen aktiviert")
	return true
}
