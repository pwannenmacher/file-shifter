package services

import (
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"file-shifter/config"

	"github.com/jlaffaye/ftp"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type FileHandler struct {
	S3ClientManager *S3ClientManager
	OutputTargets   []config.OutputTarget
}

func NewFileHandler(targets []config.OutputTarget, s3ClientManager *S3ClientManager) *FileHandler {
	return &FileHandler{
		S3ClientManager: s3ClientManager,
		OutputTargets:   targets,
	}
}

func (fh *FileHandler) ProcessFile(filePath, inputDir string) error {
	slog.Info("Verarbeite Datei", "datei", filePath)

	// Relative Pfad bestimmen
	relPath, err := filepath.Rel(inputDir, filePath)
	if err != nil {
		return fmt.Errorf("fehler beim Bestimmen des relativen Pfads: %w", err)
	}

	// Datei-Info für Attribute-Erhaltung
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("fehler beim Lesen der Datei-Informationen: %w", err)
	}

	var transferErrors []error

	// Zu allen konfigurierten Zielen kopieren
	for _, target := range fh.OutputTargets {
		switch target.Type {
		case "filesystem":
			if err := fh.copyToFilesystem(filePath, relPath, target.Path, fileInfo); err != nil {
				transferErrors = append(transferErrors, fmt.Errorf("filesystem-transfer fehlgeschlagen: %w", err))
				slog.Error("Filesystem-Transfer fehlgeschlagen", "ziel", target.Path, "fehler", err)
			}
		case "s3":
			if err := fh.copyToS3(filePath, relPath, target); err != nil {
				transferErrors = append(transferErrors, fmt.Errorf("s3-transfer fehlgeschlagen: %w", err))
				slog.Error("S3-Transfer fehlgeschlagen", "ziel", target.Path, "fehler", err)
			}
		case "ftp":
			if err := fh.copyToFTP(filePath, relPath, target); err != nil {
				transferErrors = append(transferErrors, fmt.Errorf("ftp-transfer fehlgeschlagen: %w", err))
				slog.Error("FTP-Transfer fehlgeschlagen", "ziel", target.Path, "fehler", err)
			}
		case "sftp":
			if err := fh.copyToSFTP(filePath, relPath, target); err != nil {
				transferErrors = append(transferErrors, fmt.Errorf("sftp-transfer fehlgeschlagen: %w", err))
				slog.Error("SFTP-Transfer fehlgeschlagen", "ziel", target.Path, "fehler", err)
			}
		default:
			transferErrors = append(transferErrors, fmt.Errorf("unbekannter Zieltyp: %s", target.Type))
		}
	}

	// Wenn alle Transfers erfolgreich waren, Originaldatei löschen
	if len(transferErrors) == 0 {
		if err := os.Remove(filePath); err != nil {
			slog.Error("Fehler beim Löschen der Originaldatei", "datei", filePath, "fehler", err)
			return fmt.Errorf("fehler beim Löschen der Originaldatei: %w", err)
		}
		slog.Info("Datei erfolgreich verarbeitet und entfernt", "datei", relPath)
	} else {
		slog.Error("Nicht alle Transfers erfolgreich - Originaldatei wird beibehalten", "datei", relPath, "fehler", len(transferErrors))
		return fmt.Errorf("transfers fehlgeschlagen: %v", transferErrors)
	}

	return nil
}

func (fh *FileHandler) copyToFilesystem(srcPath, relPath, targetBasePath string, fileInfo os.FileInfo) error {
	targetPath := filepath.Join(targetBasePath, relPath)
	targetDir := filepath.Dir(targetPath)

	// Zielverzeichnis erstellen
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("fehler beim Erstellen des Zielverzeichnisses: %w", err)
	}

	// Datei kopieren
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("fehler beim Öffnen der Quelldatei: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("fehler beim Erstellen der Zieldatei: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("fehler beim Kopieren der Datei: %w", err)
	}

	// Dateiberechtigungen und Zeitstempel setzen
	if err := os.Chmod(targetPath, fileInfo.Mode()); err != nil {
		slog.Warn("Konnte Dateiberechtigungen nicht setzen", "datei", targetPath, "fehler", err)
	}

	if err := os.Chtimes(targetPath, fileInfo.ModTime(), fileInfo.ModTime()); err != nil {
		slog.Warn("Konnte Zeitstempel nicht setzen", "datei", targetPath, "fehler", err)
	}

	slog.Info("Datei erfolgreich zu Filesystem kopiert", "quelle", relPath, "ziel", targetPath)
	return nil
}

func (fh *FileHandler) copyToS3(srcPath, relPath string, target config.OutputTarget) error {
	if fh.S3ClientManager == nil {
		return fmt.Errorf("s3ClientManager nicht initialisiert")
	}

	// S3-Konfiguration aus dem Target extrahieren
	s3Config := target.GetS3Config()

	// Den entsprechenden MinIO-Client für diese Konfiguration holen
	minioClient, err := fh.S3ClientManager.GetOrCreateClient(s3Config)
	if err != nil {
		return fmt.Errorf("fehler beim Abrufen des S3-Clients: %w", err)
	}

	// S3-Pfad parsen (s3://bucket/prefix)
	u, err := url.Parse(target.Path)
	if err != nil {
		return fmt.Errorf("ungültiger S3-Pfad: %w", err)
	}

	bucketName := u.Host
	prefix := strings.TrimPrefix(u.Path, "/")

	// Bucket-Name sanitarisieren
	bucketName = minioClient.SanitizeBucketName(bucketName)

	// Bucket sicherstellen
	if err := minioClient.EnsureBucket(bucketName); err != nil {
		return fmt.Errorf("fehler beim Sicherstellen des Buckets: %w", err)
	}

	// S3-Objekt-Key erstellen
	objectKey := relPath
	if prefix != "" {
		objectKey = filepath.Join(prefix, relPath)
	}
	// Für S3 immer Unix-Style Pfade verwenden
	objectKey = strings.ReplaceAll(objectKey, "\\", "/")

	// Datei hochladen
	if _, err := minioClient.UploadFile(srcPath, bucketName, objectKey); err != nil {
		return fmt.Errorf("fehler beim S3-Upload: %w", err)
	}

	slog.Info("Datei erfolgreich zu S3 hochgeladen",
		"quelle", relPath,
		"bucket", bucketName,
		"key", objectKey,
		"endpoint", s3Config.Endpoint)
	return nil
}

func (fh *FileHandler) copyToFTP(srcPath, relPath string, target config.OutputTarget) error {
	// FTP-Pfad parsen (ftp://server/path)
	u, err := url.Parse(target.Path)
	if err != nil {
		return fmt.Errorf("ungültiger FTP-Pfad: %w", err)
	}

	host := u.Host
	remotePath := strings.TrimPrefix(u.Path, "/")
	if remotePath != "" {
		remotePath = filepath.Join(remotePath, relPath)
	} else {
		remotePath = relPath
	}

	// Standard-Port setzen falls nicht angegeben
	if !strings.Contains(host, ":") {
		host += ":21"
	}

	return fh.copyToFTPRegular(srcPath, remotePath, host, target)
}

func (fh *FileHandler) copyToSFTP(srcPath, relPath string, target config.OutputTarget) error {
	// SFTP-Pfad parsen (sftp://server/path)
	u, err := url.Parse(target.Path)
	if err != nil {
		return fmt.Errorf("ungültiger SFTP-Pfad: %w", err)
	}

	host := u.Host
	remotePath := strings.TrimPrefix(u.Path, "/")
	if remotePath != "" {
		remotePath = filepath.Join(remotePath, relPath)
	} else {
		remotePath = relPath
	}

	// Standard-Port setzen falls nicht angegeben
	if !strings.Contains(host, ":") {
		host += ":22"
	}

	return fh.copyToSFTPClient(srcPath, remotePath, host, target)
}

func (fh *FileHandler) copyToSFTPClient(srcPath, remotePath, host string, target config.OutputTarget) error {
	// SSH-Verbindung aufbauen
	ftpConfig := target.GetFTPConfig()
	config := &ssh.ClientConfig{
		User: ftpConfig.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(ftpConfig.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // In Produktion sollte hier eine ordentliche Verifikation stehen
		Timeout:         30 * time.Second,
	}

	conn, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return fmt.Errorf("SSH-Verbindung fehlgeschlagen: %w", err)
	}
	defer conn.Close()

	// SFTP-Client erstellen
	client, err := sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("SFTP-Client-Erstellung fehlgeschlagen: %w", err)
	}
	defer client.Close()

	// Remote-Verzeichnis erstellen
	remoteDir := filepath.Dir(remotePath)
	if err := client.MkdirAll(remoteDir); err != nil {
		slog.Warn("Konnte Remote-Verzeichnis nicht erstellen", "verzeichnis", remoteDir, "fehler", err)
	}

	// Quelldatei öffnen
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("fehler beim Öffnen der Quelldatei: %w", err)
	}
	defer srcFile.Close()

	// Remote-Datei erstellen
	dstFile, err := client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("fehler beim Erstellen der Remote-Datei: %w", err)
	}
	defer dstFile.Close()

	// Datei übertragen
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("fehler beim SFTP-Upload: %w", err)
	}

	slog.Info("Datei erfolgreich über SFTP hochgeladen", "quelle", srcPath, "ziel", remotePath)
	return nil
}

func (fh *FileHandler) copyToFTPRegular(srcPath, remotePath, host string, target config.OutputTarget) error {
	// FTP-Verbindung aufbauen
	client, err := ftp.Dial(host, ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		return fmt.Errorf("FTP-Verbindung fehlgeschlagen: %w", err)
	}
	defer client.Quit()

	// Anmelden
	ftpConfig := target.GetFTPConfig()
	if err := client.Login(ftpConfig.Username, ftpConfig.Password); err != nil {
		return fmt.Errorf("FTP-Anmeldung fehlgeschlagen: %w", err)
	}

	// Remote-Verzeichnis erstellen (falls nötig)
	remoteDir := filepath.Dir(remotePath)
	if remoteDir != "." && remoteDir != "/" {
		// Verzeichnisse schrittweise erstellen
		dirs := strings.Split(remoteDir, "/")
		currentPath := ""
		for _, dir := range dirs {
			if dir == "" {
				continue
			}
			currentPath = filepath.Join(currentPath, dir)
			// Unix-Style Pfad für FTP
			currentPath = strings.ReplaceAll(currentPath, "\\", "/")
			if err := client.MakeDir(currentPath); err != nil {
				// Fehler ignorieren falls Verzeichnis bereits existiert
				slog.Debug("Verzeichnis existiert möglicherweise bereits", "verzeichnis", currentPath)
			}
		}
	}

	// Quelldatei öffnen
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("fehler beim Öffnen der Quelldatei: %w", err)
	}
	defer srcFile.Close()

	// Unix-Style Pfad für FTP verwenden
	remotePath = strings.ReplaceAll(remotePath, "\\", "/")

	// Datei übertragen
	if err := client.Stor(remotePath, srcFile); err != nil {
		return fmt.Errorf("fehler beim FTP-Upload: %w", err)
	}

	slog.Info("Datei erfolgreich über FTP hochgeladen", "quelle", srcPath, "ziel", remotePath, "host", host)
	return nil
}
