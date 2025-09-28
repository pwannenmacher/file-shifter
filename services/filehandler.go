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

// normaliseRemotePath converts Windows paths to Unix style for remote transfer
func normalizeRemotePath(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}

// parseRemotePath parses FTP/SFTP URLs and returns host, remotePath and default port
func parseRemotePath(targetPath, relPath string, defaultPort string) (host, remotePath string, err error) {
	u, err := url.Parse(targetPath)
	if err != nil {
		return "", "", fmt.Errorf("invalid remote path: %w", err)
	}

	host = u.Host
	remotePath = strings.TrimPrefix(u.Path, "/")
	if remotePath != "" {
		remotePath = filepath.Join(remotePath, relPath)
	} else {
		remotePath = relPath
	}

	// Set default port if not specified
	if !strings.Contains(host, ":") {
		host += ":" + defaultPort
	}

	return host, remotePath, nil
}

// createSSHConfig creates an SSH configuration for SFTP
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

// connectAndLoginFTP establishes an FTP connection and logs in
func connectAndLoginFTP(host string, ftpConfig config.FTPConfig) (*ftp.ServerConn, error) {
	client, err := ftp.Dial(host, ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		return nil, fmt.Errorf("FTP connection failed: %w", err)
	}

	if err := client.Login(ftpConfig.Username, ftpConfig.Password); err != nil {
		client.Quit()
		return nil, fmt.Errorf("FTP login failed: %w", err)
	}

	return client, nil
}

type s3PathInfo struct {
	bucketName string
	objectKey  string
}

// parseS3Path parses S3 URLs and creates object keys
func parseS3Path(targetPath, relPath string) (s3PathInfo, error) {
	u, err := url.Parse(targetPath)
	if err != nil {
		return s3PathInfo{}, fmt.Errorf("invalid S3 path: %w", err)
	}

	bucketName := u.Host
	prefix := strings.TrimPrefix(u.Path, "/")

	// Create S3 object key
	objectKey := relPath
	if prefix != "" {
		objectKey = filepath.Join(prefix, relPath)
	}
	// Always use Unix-style paths for S3
	objectKey = normalizeRemotePath(objectKey)

	return s3PathInfo{
		bucketName: bucketName,
		objectKey:  objectKey,
	}, nil
}

// calculateFileChecksum calculates the SHA256 checksum of a file
func (fh *FileHandler) calculateFileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("error opening file for checksum: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("error calculating checksum: %w", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (fh *FileHandler) ProcessFile(filePath, inputDir string) error {
	slog.Info("Process file", "file", filePath)

	// Calculate first checksum (immediately after finding the file)
	initialChecksum, err := fh.calculateFileChecksum(filePath)
	if err != nil {
		return fmt.Errorf("error calculating initial checksum: %w", err)
	}
	slog.Debug("Initial checksum calculated", "file", filePath, "checksum", initialChecksum)

	// Determine relative path
	relPath, err := filepath.Rel(inputDir, filePath)
	if err != nil {
		return fmt.Errorf("error determining relative path: %w", err)
	}

	// File info for attribute preservation
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("error reading file information: %w", err)
	}

	var transferErrors []error

	// Copy to all configured destinations
	for _, target := range fh.OutputTargets {
		switch target.Type {
		case "filesystem":
			if err := fh.copyToFilesystem(filePath, relPath, target.Path, fileInfo); err != nil {
				transferErrors = append(transferErrors, fmt.Errorf("file system transfer failed: %w", err))
				slog.Error("Filesystem-Transfer failed", "target", target.Path, "error", err)
			}
		case "s3":
			if err := fh.copyToS3(filePath, relPath, target); err != nil {
				transferErrors = append(transferErrors, fmt.Errorf("s3 transfer failed: %w", err))
				slog.Error("S3-Transfer failed", "target", target.Path, "error", err)
			}
		case "ftp":
			if err := fh.copyToFTP(filePath, relPath, target); err != nil {
				transferErrors = append(transferErrors, fmt.Errorf("FTP transfer failed: %w", err))
				slog.Error("FTP-Transfer failed", "target", target.Path, "error", err)
			}
		case "sftp":
			if err := fh.copyToSFTP(filePath, relPath, target); err != nil {
				transferErrors = append(transferErrors, fmt.Errorf("SFTP transfer failed: %w", err))
				slog.Error("SFTP-Transfer failed", "target", target.Path, "error", err)
			}
		default:
			transferErrors = append(transferErrors, fmt.Errorf("unknown target type: %s", target.Type))
		}
	}

	// If all transfers were successful, calculate the final checksum.
	if len(transferErrors) == 0 {
		// Calculate final checksum (immediately before deletion)
		finalChecksum, err := fh.calculateFileChecksum(filePath)
		if err != nil {
			slog.Error("Error calculating final checksum", "file", filePath, "error", err)
			// If there is an error in the checksum check: Delete target files
			err := fh.cleanupTargetFiles(relPath)
			if err != nil {
				return fmt.Errorf("error cleaning target files: %w", err)
			}
			return fmt.Errorf("error calculating the final checksum: %w", err)
		}

		// Prüfsummen vergleichen
		if initialChecksum != finalChecksum {
			slog.Warn("Prüfsummen stimmen nicht überein - Datei wurde während der Verarbeitung verändert",
				"file", filePath,
				"initial_checksum", initialChecksum,
				"final_checksum", finalChecksum)

			if err := fh.cleanupTargetFiles(relPath); err != nil {
				slog.Error("Error deleting target files", "file", relPath, "error", err)
			}

			slog.Info("Restart processing due to checksum mismatch", "file", filePath)
			return fh.ProcessFile(filePath, inputDir)
		}

		// Prüfsummen sind identisch - Originaldatei kann gelöscht werden
		if err := os.Remove(filePath); err != nil {
			slog.Error("Error deleting the original file", "file", filePath, "error", err)
			return fmt.Errorf("error deleting the original file: %w", err)
		}
		slog.Info("File successfully processed and removed", "file", relPath)
	} else {
		slog.Error("Not all transfers successful - original file retained", "file", relPath, "error", len(transferErrors))
		return fmt.Errorf("transfers failed: %v", transferErrors)
	}

	return nil
}

func (fh *FileHandler) copyToFilesystem(srcPath, relPath, targetBasePath string, fileInfo os.FileInfo) error {
	targetPath := filepath.Join(targetBasePath, relPath)
	targetDir := filepath.Dir(targetPath)

	// Create target directory
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("error creating the target directory: %w", err)
	}

	// Copy file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("error opening source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("error creating target file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("error copying the file: %w", err)
	}

	// Set file permissions and timestamps
	if err := os.Chmod(targetPath, fileInfo.Mode()); err != nil {
		slog.Warn("Could not set file permissions", "file", targetPath, "error", err)
	}

	if err := os.Chtimes(targetPath, fileInfo.ModTime(), fileInfo.ModTime()); err != nil {
		slog.Warn("Could not set timestamp", "file", targetPath, "error", err)
	}

	slog.Info("File successfully copied to file system", "source", relPath, "target", targetPath)
	return nil
}

func (fh *FileHandler) copyToS3(srcPath, relPath string, target config.OutputTarget) error {
	if fh.S3ClientManager == nil {
		return fmt.Errorf("s3ClientManager not initialised")
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
		slog.Warn("Konnte Remote-Verzeichnis nicht erstellen", "verzeichnis", remoteDir, "error", err)
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

	slog.Info("Datei erfolgreich über SFTP hochgeladen", "quelle", srcPath, "target", remotePath)
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

	slog.Info("Datei erfolgreich über FTP hochgeladen", "quelle", srcPath, "target", remotePath, "host", host)
	return nil
}

// cleanupTargetFiles löscht bereits übertragene Dateien in allen konfigurierten Zielen
func (fh *FileHandler) cleanupTargetFiles(relPath string) error {
	slog.Info("Lösche bereits übertragene Dateien", "file", relPath)
	var cleanupErrors []error

	for _, target := range fh.OutputTargets {
		switch target.Type {
		case "filesystem":
			if err := fh.deleteFromFilesystem(relPath, target.Path); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("filesystem-löschung fehlgeschlagen: %w", err))
				slog.Error("Filesystem-Löschung fehlgeschlagen", "target", target.Path, "error", err)
			}
		case "s3":
			if err := fh.deleteFromS3(relPath, target); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("s3-löschung fehlgeschlagen: %w", err))
				slog.Error("S3-Löschung fehlgeschlagen", "target", target.Path, "error", err)
			}
		case "ftp":
			if err := fh.deleteFromFTP(relPath, target); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("ftp-löschung fehlgeschlagen: %w", err))
				slog.Error("FTP-Löschung fehlgeschlagen", "target", target.Path, "error", err)
			}
		case "sftp":
			if err := fh.deleteFromSFTP(relPath, target); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("sftp-löschung fehlgeschlagen: %w", err))
				slog.Error("SFTP-Löschung fehlgeschlagen", "target", target.Path, "error", err)
			}
		}
	}

	if len(cleanupErrors) > 0 {
		return fmt.Errorf("cleanup-fehler: %v", cleanupErrors)
	}

	slog.Info("Alle Zieldateien erfolgreich gelöscht", "file", relPath)
	return nil
}

// deleteFromFilesystem löscht eine Datei vom Filesystem
func (fh *FileHandler) deleteFromFilesystem(relPath, targetBasePath string) error {
	targetPath := filepath.Join(targetBasePath, relPath)

	if err := os.Remove(targetPath); err != nil {
		if os.IsNotExist(err) {
			slog.Debug("Datei existiert nicht im Filesystem-Ziel", "path", targetPath)
			return nil // Datei existiert nicht - kein Fehler
		}
		return fmt.Errorf("fehler beim Löschen der Filesystem-Datei: %w", err)
	}

	slog.Debug("Datei erfolgreich vom Filesystem gelöscht", "path", targetPath)
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

	// Establish FTP connection and log in
	ftpConfig := target.GetFTPConfig()
	client, err := connectAndLoginFTP(host, ftpConfig)
	if err != nil {
		return err
	}
	defer client.Quit()

	// Use Unix-style path for FTP
	remotePath = normalizeRemotePath(remotePath)

	if err := client.Delete(remotePath); err != nil {
		// Check whether file exists (550 is the standard code for ‘file not found’)
		if strings.Contains(err.Error(), "550") {
			slog.Debug("File does not exist in FTP destination", "path", remotePath)
			return nil // File does not exist - no error
		}
		return fmt.Errorf("error during FTP deletion: %w", err)
	}

	slog.Debug("File successfully deleted from the FTP server", "path", remotePath, "host", host)
	return nil
}

// deleteFromSFTP deletes a file from the SFTP server
func (fh *FileHandler) deleteFromSFTP(relPath string, target config.OutputTarget) error {
	host, remotePath, err := parseRemotePath(target.Path, relPath, "22")
	if err != nil {
		return fmt.Errorf("fehler beim Parsen des SFTP-Pfads: %w", err)
	}

	ftpConfig := target.GetFTPConfig()
	config := createSSHConfig(ftpConfig)

	conn, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	defer conn.Close()

	client, err := sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("SFTP client creation failed: %w", err)
	}
	defer client.Close()

	// Datei löschen
	if err := client.Remove(remotePath); err != nil {
		if os.IsNotExist(err) {
			slog.Debug("File does not exist in SFTP destination", "path", remotePath)
			return nil // Datei existiert nicht - kein Fehler
		}
		return fmt.Errorf("fehler beim SFTP-Löschen: %w", err)
	}

	slog.Debug("File successfully deleted from the SFTP server", "path", remotePath)
	return nil
}
