package services

import (
	"file-shifter/config"
	"log/slog"
	"os"
)

type Worker struct {
	stopChan        chan bool
	InputDir        string
	OutputTargets   []config.OutputTarget
	S3ClientManager *S3ClientManager
	FileHandler     *FileHandler
	FileWatcher     *FileWatcher
}

func NewWorker(dir string, targets []config.OutputTarget) *Worker {

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
	if len(targets) > 0 {
		for _, target := range targets {
			switch target.Type {
			case "s3":
				s3Config := target.GetS3Config()
				if s3Config.Endpoint == "" || s3Config.AccessKey == "" || s3Config.SecretKey == "" || s3Config.Region == "" {
					slog.Error("Ungültige S3-Konfiguration für Target", "path", target.Path)
					os.Exit(1)
				}
				// S3-Client vorläufig erstellen und testen
				if _, err := w.S3ClientManager.GetOrCreateClient(s3Config); err != nil {
					slog.Error("S3-Client-Erstellung fehlgeschlagen", "endpoint", s3Config.Endpoint, "err", err)
					os.Exit(1)
				}
			case "ftp", "sftp":
				ftpConfig := target.GetFTPConfig()
				if ftpConfig.Host == "" || ftpConfig.Username == "" || ftpConfig.Password == "" {
					slog.Error("Ungültige FTP/SFTP-Konfiguration für Target", "path", target.Path, "type", target.Type)
					os.Exit(1)
				}
			case "filesystem":
				if target.Path == "" {
					slog.Error("Ungültige Dateisystem-Konfiguration in der Umgebungsdatei")
					os.Exit(1)
				}
			default:
				slog.Error("Unbekannter Ausgabetyp in der Umgebungsdatei", "type", target.Type)
				os.Exit(1)
			}
		}

		slog.Info("Target-Konfigurationen validiert", "anzahl_targets", len(targets), "aktive_s3_clients", w.S3ClientManager.GetActiveClientCount())
	} else {
		// Falls keine Targets definiert sind, verwende die Standard-Defaults die in main.go gesetzt wurden
		slog.Info("Verwende Standard-Output-Konfiguration")
	}

	// FileHandler initialisieren
	w.FileHandler = NewFileHandler(targets, w.S3ClientManager)

	// FileWatcher initialisieren
	fileWatcher, err := NewFileWatcher(dir, w.FileHandler)
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
