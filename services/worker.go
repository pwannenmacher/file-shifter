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

	if dir == "" {
		slog.Error("Input directory must not be empty")
		os.Exit(1)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			slog.Error("Error creating input directory", "inputDir", dir, "error", err)
			os.Exit(1)
		}
	}

	if err := w.validateTargets(targets); err != nil {
		slog.Error("Target validation failed", "error", err)
		os.Exit(1)
	}

	w.FileHandler = NewFileHandler(targets, w.S3ClientManager)

	maxRetries := cfg.FileStability.MaxRetries
	checkInterval := time.Duration(cfg.FileStability.CheckInterval) * time.Second
	stabilityPeriod := time.Duration(cfg.FileStability.StabilityPeriod) * time.Second

	fileWatcher, err := NewFileWatcher(dir, w.FileHandler, maxRetries, checkInterval, stabilityPeriod)
	if err != nil {
		slog.Error("Error initializing file watcher", "err", err)
		os.Exit(1)
	}
	w.FileWatcher = fileWatcher

	return w
}

func (w *Worker) Start() {
	slog.Info("Worker started - process incoming files")

	// Start file watcher in separate goroutine
	go func() {
		if err := w.FileWatcher.Start(); err != nil {
			slog.Error("File-Watcher Fehler", "err", err)
		}
	}()

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

// validateTargets validates the target configurations and creates S3 clients
func (w *Worker) validateTargets(targets []config.OutputTarget) error {
	if len(targets) == 0 {
		slog.Info("Use standard output configuration")
		return nil
	}

	for _, target := range targets {
		if err := w.validateSingleTarget(target); err != nil {
			return err
		}
	}

	slog.Info("Target configurations validated", "number_of_targets", len(targets), "active_s3_clients", w.S3ClientManager.GetActiveClientCount())
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
		slog.Error("Unknown output type in the environment file", "type", target.Type)
		return fmt.Errorf("unknown output type: %s", target.Type)
	}
}

// validateS3Target validates S3-specific configuration
func (w *Worker) validateS3Target(target config.OutputTarget) error {
	s3Config := target.GetS3Config()
	if s3Config.Endpoint == "" || s3Config.AccessKey == "" || s3Config.SecretKey == "" || s3Config.Region == "" {
		slog.Error("Invalid S3 configuration for target", "path", target.Path)
		return fmt.Errorf("invalid S3 configuration for target: %s", target.Path)
	}

	// S3-Client vorläufig erstellen und testen
	if _, err := w.S3ClientManager.GetOrCreateClient(s3Config); err != nil {
		slog.Error("S3 client creation failed", "endpoint", s3Config.Endpoint, "err", err)
		return fmt.Errorf("S3 client creation failed for %s: %w", s3Config.Endpoint, err)
	}

	return nil
}

// validateFTPTarget validates FTP/SFTP-specific configuration
func (w *Worker) validateFTPTarget(target config.OutputTarget) error {
	ftpConfig := target.GetFTPConfig()
	if ftpConfig.Host == "" || ftpConfig.Username == "" || ftpConfig.Password == "" {
		slog.Error("Invalid FTP/SFTP configuration for target", "path", target.Path, "type", target.Type)
		return fmt.Errorf("invalid %s configuration for target: %s", target.Type, target.Path)
	}
	return nil
}

// validateFilesystemTarget validates filesystem-specific configuration
func (w *Worker) validateFilesystemTarget(target config.OutputTarget) error {
	if target.Path == "" {
		slog.Error("Invalid file system configuration in the environment file")
		return fmt.Errorf("invalid file system configuration: empty path")
	}
	return nil
}
