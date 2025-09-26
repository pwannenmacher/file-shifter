package services

import (
	"file-shifter/config"
	"fmt"
	"log/slog"
	"os"
	"time"
)

type Worker struct {
	stopChan        chan bool
	InputDir        string
	OutputTargets   []config.OutputTarget
	S3ClientManager *S3ClientManager
	FileHandler     *FileHandler
	FileWatcher     *FileWatcher
}

func NewWorker(dir string, targets []config.OutputTarget, cfg *config.EnvConfig) *Worker {

	w := &Worker{
		stopChan:        make(chan bool),
		InputDir:        dir,
		OutputTargets:   targets,
		S3ClientManager: NewS3ClientManager(),
	}

	// Validierung: InputDir darf nicht leer sein
	if dir == "" {
		slog.Error("Input-Directory darf nicht leer sein")
		os.Exit(1)
	}

	// Sicherstellen, dass InputDir existiert
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			slog.Error("Fehler beim Erstellen des Input-Verzeichnisses", "inputDir", dir, "fehler", err)
			os.Exit(1)
		}
	}

	// Target-Konfigurationen validieren
	if err := w.validateTargets(targets); err != nil {
		slog.Error("Target-Validierung fehlgeschlagen", "fehler", err)
		os.Exit(1)
	}

	// FileHandler initialisieren
	w.FileHandler = NewFileHandler(targets, w.S3ClientManager)

	// FileWatcher initialisieren mit Konfiguration
	maxRetries := cfg.FileStability.MaxRetries
	checkInterval := time.Duration(cfg.FileStability.CheckInterval) * time.Second
	stabilityPeriod := time.Duration(cfg.FileStability.StabilityPeriod) * time.Second

	fileWatcher, err := NewFileWatcher(dir, w.FileHandler, maxRetries, checkInterval, stabilityPeriod)
	if err != nil {
		slog.Error("Fehler beim Initialisieren des File-Watchers", "err", err)
		os.Exit(1)
	}
	w.FileWatcher = fileWatcher

	return w
}

func (w *Worker) Start() {
	slog.Info("Worker gestartet - verarbeite eingehende Dateien")

	// File-Watcher in separater Goroutine starten
	go func() {
		if err := w.FileWatcher.Start(); err != nil {
			slog.Error("File-Watcher Fehler", "err", err)
		}
	}()

	// Auf Stop-Signal warten
	<-w.stopChan
	slog.Info("Worker gestoppt")
}

func (w *Worker) Stop() {
	if w.FileWatcher != nil {
		w.FileWatcher.Stop()
	}
	if w.S3ClientManager != nil {
		w.S3ClientManager.Close()
	}
	w.stopChan <- true
}

// validateTargets validiert die Target-Konfigurationen und erstellt S3-Clients
func (w *Worker) validateTargets(targets []config.OutputTarget) error {
	if len(targets) == 0 {
		slog.Info("Verwende Standard-Output-Konfiguration")
		return nil
	}

	for _, target := range targets {
		if err := w.validateSingleTarget(target); err != nil {
			return err
		}
	}

	slog.Info("Target-Konfigurationen validiert", "anzahl_targets", len(targets), "aktive_s3_clients", w.S3ClientManager.GetActiveClientCount())
	return nil
}

// validateSingleTarget validiert ein einzelnes Target
func (w *Worker) validateSingleTarget(target config.OutputTarget) error {
	switch target.Type {
	case "s3":
		return w.validateS3Target(target)
	case "ftp", "sftp":
		return w.validateFTPTarget(target)
	case "filesystem":
		return w.validateFilesystemTarget(target)
	default:
		slog.Error("Unbekannter Ausgabetyp in der Umgebungsdatei", "type", target.Type)
		return fmt.Errorf("unbekannter Ausgabetyp: %s", target.Type)
	}
}

// validateS3Target validiert S3-spezifische Konfiguration
func (w *Worker) validateS3Target(target config.OutputTarget) error {
	s3Config := target.GetS3Config()
	if s3Config.Endpoint == "" || s3Config.AccessKey == "" || s3Config.SecretKey == "" || s3Config.Region == "" {
		slog.Error("Ungültige S3-Konfiguration für Target", "path", target.Path)
		return fmt.Errorf("ungültige S3-Konfiguration für Target: %s", target.Path)
	}

	// S3-Client vorläufig erstellen und testen
	if _, err := w.S3ClientManager.GetOrCreateClient(s3Config); err != nil {
		slog.Error("S3-Client-Erstellung fehlgeschlagen", "endpoint", s3Config.Endpoint, "err", err)
		return fmt.Errorf("S3-Client-Erstellung fehlgeschlagen für %s: %w", s3Config.Endpoint, err)
	}

	return nil
}

// validateFTPTarget validiert FTP/SFTP-spezifische Konfiguration
func (w *Worker) validateFTPTarget(target config.OutputTarget) error {
	ftpConfig := target.GetFTPConfig()
	if ftpConfig.Host == "" || ftpConfig.Username == "" || ftpConfig.Password == "" {
		slog.Error("Ungültige FTP/SFTP-Konfiguration für Target", "path", target.Path, "type", target.Type)
		return fmt.Errorf("ungültige %s-Konfiguration für Target: %s", target.Type, target.Path)
	}
	return nil
}

// validateFilesystemTarget validiert Filesystem-spezifische Konfiguration
func (w *Worker) validateFilesystemTarget(target config.OutputTarget) error {
	if target.Path == "" {
		slog.Error("Ungültige Dateisystem-Konfiguration in der Umgebungsdatei")
		return fmt.Errorf("ungültige Dateisystem-Konfiguration: leerer Pfad")
	}
	return nil
}
