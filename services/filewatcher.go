package services

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

type FileWatcher struct {
	watcher     *fsnotify.Watcher
	inputDir    string
	fileHandler *FileHandler
	stopChan    chan bool
}

func NewFileWatcher(inputDir string, fileHandler *FileHandler) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	fw := &FileWatcher{
		watcher:     watcher,
		inputDir:    inputDir,
		fileHandler: fileHandler,
		stopChan:    make(chan bool),
	}

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
		// Kurz warten, um sicherzustellen, dass die Datei vollständig geschrieben wurde
		time.Sleep(100 * time.Millisecond)

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
	// Kurz warten, um sicherzustellen, dass die Datei nicht mehr geschrieben wird
	time.Sleep(500 * time.Millisecond)

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
