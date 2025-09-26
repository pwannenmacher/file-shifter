package services

import (
	"crypto/sha256"
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

// normalizeRemotePath konvertiert Windows-Pfade zu Unix-Style für Remote-Übertragung
func normalizeRemotePath(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}

// parseRemotePath parsed FTP/SFTP URLs und gibt Host, remotePath und Standard-Port zurück
func parseRemotePath(targetPath, relPath string, defaultPort string) (host, remotePath string, err error) {
	u, err := url.Parse(targetPath)
	if err != nil {
		return "", "", fmt.Errorf("ungültiger Remote-Pfad: %w", err)
	}

	host = u.Host
	remotePath = strings.TrimPrefix(u.Path, "/")
	if remotePath != "" {
		remotePath = filepath.Join(remotePath, relPath)
	} else {
		remotePath = relPath
	}

	// Standard-Port setzen falls nicht angegeben
	if !strings.Contains(host, ":") {
		host += ":" + defaultPort
	}

	return host, remotePath, nil
}

// createSSHConfig erstellt eine SSH-Konfiguration für SFTP
func createSSHConfig(ftpConfig config.FTPConfig) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User: ftpConfig.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(ftpConfig.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}
}

// connectAndLoginFTP stellt FTP-Verbindung her und meldet sich an
func connectAndLoginFTP(host string, ftpConfig config.FTPConfig) (*ftp.ServerConn, error) {
	client, err := ftp.Dial(host, ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		return nil, fmt.Errorf("FTP-Verbindung fehlgeschlagen: %w", err)
	}

	if err := client.Login(ftpConfig.Username, ftpConfig.Password); err != nil {
		client.Quit()
		return nil, fmt.Errorf("FTP-Anmeldung fehlgeschlagen: %w", err)
	}

	return client, nil
}

type s3PathInfo struct {
	bucketName string
	objectKey  string
}

// parseS3Path parsed S3-URLs und erstellt Object-Key
func parseS3Path(targetPath, relPath string) (s3PathInfo, error) {
	u, err := url.Parse(targetPath)
	if err != nil {
		return s3PathInfo{}, fmt.Errorf("ungültiger S3-Pfad: %w", err)
	}

	bucketName := u.Host
	prefix := strings.TrimPrefix(u.Path, "/")

	// S3-Objekt-Key erstellen
	objectKey := relPath
	if prefix != "" {
		objectKey = filepath.Join(prefix, relPath)
	}
	// Für S3 immer Unix-Style Pfade verwenden
	objectKey = normalizeRemotePath(objectKey)

	return s3PathInfo{
		bucketName: bucketName,
		objectKey:  objectKey,
	}, nil
}

// calculateFileChecksum berechnet die SHA256-Prüfsumme einer Datei
func (fh *FileHandler) calculateFileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("fehler beim Öffnen der Datei für Prüfsumme: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("fehler beim Berechnen der Prüfsumme: %w", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (fh *FileHandler) ProcessFile(filePath, inputDir string) error {
	slog.Info("Verarbeite Datei", "datei", filePath)

	// Erste Prüfsumme berechnen (direkt nach dem Finden der Datei)
	initialChecksum, err := fh.calculateFileChecksum(filePath)
	if err != nil {
		return fmt.Errorf("fehler beim Berechnen der initialen Prüfsumme: %w", err)
	}
	slog.Debug("Initiale Prüfsumme berechnet", "datei", filePath, "checksum", initialChecksum)

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

	// Wenn alle Transfers erfolgreich waren, finale Prüfsumme berechnen
	if len(transferErrors) == 0 {
		// Finale Prüfsumme berechnen (direkt vor dem Löschen)
		finalChecksum, err := fh.calculateFileChecksum(filePath)
		if err != nil {
			slog.Error("Fehler beim Berechnen der finalen Prüfsumme", "datei", filePath, "fehler", err)
			// Bei Fehler beim Prüfsummen-Check: Zieldateien löschen
			fh.cleanupTargetFiles(relPath)
			return fmt.Errorf("fehler beim Berechnen der finalen Prüfsumme: %w", err)
		}

		// Prüfsummen vergleichen
		if initialChecksum != finalChecksum {
			slog.Warn("Prüfsummen stimmen nicht überein - Datei wurde während der Verarbeitung verändert",
				"datei", filePath,
				"initial_checksum", initialChecksum,
				"final_checksum", finalChecksum)

			// Zieldateien löschen
			if err := fh.cleanupTargetFiles(relPath); err != nil {
				slog.Error("Fehler beim Löschen der Zieldateien", "datei", relPath, "fehler", err)
			}

			// Verarbeitung neu starten
			slog.Info("Starte Verarbeitung neu aufgrund von Prüfsummen-Mismatch", "datei", filePath)
			return fh.ProcessFile(filePath, inputDir)
		}

		// Prüfsummen sind identisch - Originaldatei kann gelöscht werden
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

	// S3-Pfad parsen
	s3Path, err := parseS3Path(target.Path, relPath)
	if err != nil {
		return fmt.Errorf("fehler beim Parsen des S3-Pfads: %w", err)
	}

	// Bucket-Name sanitarisieren
	bucketName := minioClient.SanitizeBucketName(s3Path.bucketName)

	// Bucket sicherstellen
	if err := minioClient.EnsureBucket(bucketName); err != nil {
		return fmt.Errorf("fehler beim Sicherstellen des Buckets: %w", err)
	}

	// Datei hochladen
	if _, err := minioClient.UploadFile(srcPath, bucketName, s3Path.objectKey); err != nil {
		return fmt.Errorf("fehler beim S3-Upload: %w", err)
	}

	slog.Info("Datei erfolgreich zu S3 hochgeladen",
		"quelle", relPath,
		"bucket", bucketName,
		"key", s3Path.objectKey,
		"endpoint", s3Config.Endpoint)
	return nil
}

func (fh *FileHandler) copyToFTP(srcPath, relPath string, target config.OutputTarget) error {
	host, remotePath, err := parseRemotePath(target.Path, relPath, "21")
	if err != nil {
		return fmt.Errorf("fehler beim Parsen des FTP-Pfads: %w", err)
	}

	return fh.copyToFTPRegular(srcPath, remotePath, host, target)
}

func (fh *FileHandler) copyToSFTP(srcPath, relPath string, target config.OutputTarget) error {
	host, remotePath, err := parseRemotePath(target.Path, relPath, "22")
	if err != nil {
		return fmt.Errorf("fehler beim Parsen des SFTP-Pfads: %w", err)
	}

	return fh.copyToSFTPClient(srcPath, remotePath, host, target)
}

func (fh *FileHandler) copyToSFTPClient(srcPath, remotePath, host string, target config.OutputTarget) error {
	// SSH-Verbindung aufbauen
	ftpConfig := target.GetFTPConfig()
	config := createSSHConfig(ftpConfig)

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
	// FTP-Verbindung aufbauen und anmelden
	ftpConfig := target.GetFTPConfig()
	client, err := connectAndLoginFTP(host, ftpConfig)
	if err != nil {
		return err
	}
	defer client.Quit()

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
			currentPath = normalizeRemotePath(currentPath)
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
	remotePath = normalizeRemotePath(remotePath)

	// Datei übertragen
	if err := client.Stor(remotePath, srcFile); err != nil {
		return fmt.Errorf("fehler beim FTP-Upload: %w", err)
	}

	slog.Info("Datei erfolgreich über FTP hochgeladen", "quelle", srcPath, "ziel", remotePath, "host", host)
	return nil
}

// cleanupTargetFiles löscht bereits übertragene Dateien in allen konfigurierten Zielen
func (fh *FileHandler) cleanupTargetFiles(relPath string) error {
	slog.Info("Lösche bereits übertragene Dateien", "datei", relPath)
	var cleanupErrors []error

	for _, target := range fh.OutputTargets {
		switch target.Type {
		case "filesystem":
			if err := fh.deleteFromFilesystem(relPath, target.Path); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("filesystem-löschung fehlgeschlagen: %w", err))
				slog.Error("Filesystem-Löschung fehlgeschlagen", "ziel", target.Path, "fehler", err)
			}
		case "s3":
			if err := fh.deleteFromS3(relPath, target); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("s3-löschung fehlgeschlagen: %w", err))
				slog.Error("S3-Löschung fehlgeschlagen", "ziel", target.Path, "fehler", err)
			}
		case "ftp":
			if err := fh.deleteFromFTP(relPath, target); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("ftp-löschung fehlgeschlagen: %w", err))
				slog.Error("FTP-Löschung fehlgeschlagen", "ziel", target.Path, "fehler", err)
			}
		case "sftp":
			if err := fh.deleteFromSFTP(relPath, target); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("sftp-löschung fehlgeschlagen: %w", err))
				slog.Error("SFTP-Löschung fehlgeschlagen", "ziel", target.Path, "fehler", err)
			}
		}
	}

	if len(cleanupErrors) > 0 {
		return fmt.Errorf("cleanup-fehler: %v", cleanupErrors)
	}

	slog.Info("Alle Zieldateien erfolgreich gelöscht", "datei", relPath)
	return nil
}

// deleteFromFilesystem löscht eine Datei vom Filesystem
func (fh *FileHandler) deleteFromFilesystem(relPath, targetBasePath string) error {
	targetPath := filepath.Join(targetBasePath, relPath)

	if err := os.Remove(targetPath); err != nil {
		if os.IsNotExist(err) {
			slog.Debug("Datei existiert nicht im Filesystem-Ziel", "pfad", targetPath)
			return nil // Datei existiert nicht - kein Fehler
		}
		return fmt.Errorf("fehler beim Löschen der Filesystem-Datei: %w", err)
	}

	slog.Debug("Datei erfolgreich vom Filesystem gelöscht", "pfad", targetPath)
	return nil
}

// deleteFromS3 löscht eine Datei von S3
func (fh *FileHandler) deleteFromS3(relPath string, target config.OutputTarget) error {
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

	// S3-Pfad parsen
	s3Path, err := parseS3Path(target.Path, relPath)
	if err != nil {
		return fmt.Errorf("fehler beim Parsen des S3-Pfads: %w", err)
	}

	// Bucket-Name sanitarisieren
	bucketName := minioClient.SanitizeBucketName(s3Path.bucketName)

	// Datei löschen
	if err := minioClient.DeleteFile(bucketName, s3Path.objectKey); err != nil {
		return fmt.Errorf("fehler beim S3-Löschen: %w", err)
	}

	slog.Debug("Datei erfolgreich von S3 gelöscht",
		"bucket", bucketName,
		"key", s3Path.objectKey,
		"endpoint", s3Config.Endpoint)
	return nil
}

// deleteFromFTP löscht eine Datei vom FTP-Server
func (fh *FileHandler) deleteFromFTP(relPath string, target config.OutputTarget) error {
	host, remotePath, err := parseRemotePath(target.Path, relPath, "21")
	if err != nil {
		return fmt.Errorf("fehler beim Parsen des FTP-Pfads: %w", err)
	}

	// FTP-Verbindung aufbauen und anmelden
	ftpConfig := target.GetFTPConfig()
	client, err := connectAndLoginFTP(host, ftpConfig)
	if err != nil {
		return err
	}
	defer client.Quit()

	// Unix-Style Pfad für FTP verwenden
	remotePath = normalizeRemotePath(remotePath)

	// Datei löschen
	if err := client.Delete(remotePath); err != nil {
		// Prüfen ob Datei existiert (550 ist der Standard-Code für "Datei nicht gefunden")
		if strings.Contains(err.Error(), "550") {
			slog.Debug("Datei existiert nicht im FTP-Ziel", "pfad", remotePath)
			return nil // Datei existiert nicht - kein Fehler
		}
		return fmt.Errorf("fehler beim FTP-Löschen: %w", err)
	}

	slog.Debug("Datei erfolgreich vom FTP-Server gelöscht", "pfad", remotePath, "host", host)
	return nil
}

// deleteFromSFTP löscht eine Datei vom SFTP-Server
func (fh *FileHandler) deleteFromSFTP(relPath string, target config.OutputTarget) error {
	host, remotePath, err := parseRemotePath(target.Path, relPath, "22")
	if err != nil {
		return fmt.Errorf("fehler beim Parsen des SFTP-Pfads: %w", err)
	}

	// SSH-Verbindung aufbauen
	ftpConfig := target.GetFTPConfig()
	config := createSSHConfig(ftpConfig)

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

	// Datei löschen
	if err := client.Remove(remotePath); err != nil {
		if os.IsNotExist(err) {
			slog.Debug("Datei existiert nicht im SFTP-Ziel", "pfad", remotePath)
			return nil // Datei existiert nicht - kein Fehler
		}
		return fmt.Errorf("fehler beim SFTP-Löschen: %w", err)
	}

	slog.Debug("Datei erfolgreich vom SFTP-Server gelöscht", "pfad", remotePath)
	return nil
}
