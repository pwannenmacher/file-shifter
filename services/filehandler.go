package services

import (
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"file-shifter/config"

	"github.com/jlaffaye/ftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type FileHandler struct {
	S3ClientManager *S3ClientManager
	RemoteConns     *RemoteConnManager
	OutputTargets   []config.OutputTarget
}

func NewFileHandler(targets []config.OutputTarget, s3ClientManager *S3ClientManager) *FileHandler {
	return &FileHandler{
		S3ClientManager: s3ClientManager,
		RemoteConns:     NewRemoteConnManager(),
		OutputTargets:   targets,
	}
}

// Close shuts down all pooled remote connections
func (fh *FileHandler) Close() {
	if fh.RemoteConns != nil {
		fh.RemoteConns.Close()
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
func createSSHConfig(ftpConfig config.FTPConfig) (*ssh.ClientConfig, error) {
	hostKeyCallback, err := createHostKeyCallback(ftpConfig)
	if err != nil {
		return nil, err
	}

	return &ssh.ClientConfig{
		User: ftpConfig.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(ftpConfig.Password),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         30 * time.Second,
	}, nil
}

// createHostKeyCallback builds the SSH host key verification callback.
// Verification is fail-closed: without a usable known_hosts file the
// connection is refused unless verification is explicitly disabled.
func createHostKeyCallback(ftpConfig config.FTPConfig) (ssh.HostKeyCallback, error) {
	if ftpConfig.InsecureSkipHostKeyVerify {
		slog.Warn("SFTP host key verification is DISABLED - connection is vulnerable to man-in-the-middle attacks")
		return ssh.InsecureIgnoreHostKey(), nil
	}

	knownHostsFile, err := resolveKnownHostsFile(ftpConfig.KnownHosts)
	if err != nil {
		return nil, err
	}

	callback, err := knownhosts.New(knownHostsFile)
	if err != nil {
		return nil, fmt.Errorf("error loading known_hosts file %s: %w", knownHostsFile, err)
	}

	return callback, nil
}

// resolveKnownHostsFile returns the known_hosts file to use: the configured
// path, or the first existing standard location as fallback.
func resolveKnownHostsFile(configured string) (string, error) {
	if configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("configured known_hosts file is not accessible: %w", err)
		}
		return configured, nil
	}

	var candidates []string
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".ssh", "known_hosts"))
	}
	candidates = append(candidates, "/etc/ssh/ssh_known_hosts")

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no known_hosts file found (checked: %s) - configure 'known-hosts' for the SFTP target or explicitly set 'insecure-skip-host-key-verification: true'", strings.Join(candidates, ", "))
}

// connectAndLoginFTP establishes an FTP connection and logs in.
// With TLS enabled the connection is upgraded via explicit FTPS (AUTH TLS)
// before credentials are sent.
func connectAndLoginFTP(host string, ftpConfig config.FTPConfig) (*ftp.ServerConn, error) {
	opts := []ftp.DialOption{ftp.DialWithTimeout(30 * time.Second)}

	if ftpConfig.TLS {
		serverName := host
		if h, _, err := net.SplitHostPort(host); err == nil {
			serverName = h
		}
		opts = append(opts, ftp.DialWithExplicitTLS(&tls.Config{
			ServerName: serverName,
			MinVersion: tls.VersionTLS12,
		}))
	}

	client, err := ftp.Dial(host, opts...)
	if err != nil {
		return nil, fmt.Errorf("FTP connection failed: %w", err)
	}

	if err := client.Login(ftpConfig.Username, ftpConfig.Password); err != nil {
		if quitErr := client.Quit(); quitErr != nil {
			slog.Debug("Error closing FTP connection after failed login", "error", quitErr)
		}
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
	const maxChecksumRetries = 5

	for attempt := 1; attempt <= maxChecksumRetries; attempt++ {
		retry, err := fh.processFileAttempt(filePath, inputDir, attempt, maxChecksumRetries)
		if err != nil {
			return err
		}
		if retry {
			continue
		}
		return nil
	}

	return fmt.Errorf("processing aborted after retries: %s", filePath)
}

func (fh *FileHandler) processFileAttempt(filePath, inputDir string, attempt, maxChecksumRetries int) (bool, error) {
	slog.Info("Process file", "file", filePath, "attempt", attempt, "max_attempts", maxChecksumRetries)

	initialChecksum, err := fh.calculateFileChecksum(filePath)
	if err != nil {
		return false, fmt.Errorf("error calculating initial checksum: %w", err)
	}
	slog.Debug("Initial checksum calculated", "file", filePath, "checksum", initialChecksum)

	relPath, err := filepath.Rel(inputDir, filePath)
	if err != nil {
		return false, fmt.Errorf("error determining relative path: %w", err)
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return false, fmt.Errorf("error reading file information: %w", err)
	}

	if err := fh.copyToAllTargets(filePath, relPath, fileInfo); err != nil {
		return false, err
	}

	return fh.finalizeProcessedFile(filePath, relPath, initialChecksum, attempt, maxChecksumRetries)
}

func (fh *FileHandler) copyToAllTargets(filePath, relPath string, fileInfo os.FileInfo) error {
	var transferErrors []error

	for _, target := range fh.OutputTargets {
		if err := fh.copyToTarget(filePath, relPath, target, fileInfo); err != nil {
			transferErrors = append(transferErrors, err)
		}
	}

	if len(transferErrors) > 0 {
		slog.Error("Not all transfers successful - original file retained", "file", relPath, "error", len(transferErrors))
		return fmt.Errorf("transfers failed: %w", errors.Join(transferErrors...))
	}

	return nil
}

func (fh *FileHandler) copyToTarget(filePath, relPath string, target config.OutputTarget, fileInfo os.FileInfo) error {
	switch target.Type {
	case "filesystem":
		if err := fh.copyToFilesystem(filePath, relPath, target.Path, fileInfo); err != nil {
			slog.Error("Filesystem-Transfer failed", "target", target.Path, "error", err)
			return fmt.Errorf("file system transfer failed: %w", err)
		}
	case "s3":
		if err := fh.copyToS3(filePath, relPath, target); err != nil {
			slog.Error("S3-Transfer failed", "target", target.Path, "error", err)
			return fmt.Errorf("s3 transfer failed: %w", err)
		}
	case "ftp":
		if err := fh.copyToFTP(filePath, relPath, target); err != nil {
			slog.Error("FTP-Transfer failed", "target", target.Path, "error", err)
			return fmt.Errorf("FTP transfer failed: %w", err)
		}
	case "sftp":
		if err := fh.copyToSFTP(filePath, relPath, target); err != nil {
			slog.Error("SFTP-Transfer failed", "target", target.Path, "error", err)
			return fmt.Errorf("SFTP transfer failed: %w", err)
		}
	default:
		return fmt.Errorf("unknown target type: %s", target.Type)
	}

	return nil
}

func (fh *FileHandler) finalizeProcessedFile(filePath, relPath, initialChecksum string, attempt, maxChecksumRetries int) (bool, error) {
	finalChecksum, checksumErr := fh.calculateFileChecksum(filePath)
	if checksumErr != nil {
		slog.Error("Error calculating final checksum", "file", filePath, "error", checksumErr)
		if cleanupErr := fh.cleanupTargetFiles(relPath); cleanupErr != nil {
			return false, fmt.Errorf("error cleaning target files: %w", cleanupErr)
		}
		return false, fmt.Errorf("error calculating the final checksum: %w", checksumErr)
	}

	if initialChecksum != finalChecksum {
		slog.Warn("Prüfsummen stimmen nicht überein - Datei wurde während der Verarbeitung verändert",
			"file", filePath,
			"initial_checksum", initialChecksum,
			"final_checksum", finalChecksum,
			"attempt", attempt,
			"max_attempts", maxChecksumRetries)

		if err := fh.cleanupTargetFiles(relPath); err != nil {
			slog.Error("Error deleting target files", "file", relPath, "error", err)
		}

		if attempt == maxChecksumRetries {
			return false, fmt.Errorf("checksum mismatch persists after %d attempts: %s", maxChecksumRetries, relPath)
		}

		return true, nil
	}

	if err := os.Remove(filePath); err != nil {
		slog.Error("Error deleting the original file", "file", filePath, "error", err)
		return false, fmt.Errorf("error deleting the original file: %w", err)
	}

	slog.Info("File successfully processed and removed", "file", relPath)
	return false, nil
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

	written, err := io.Copy(dstFile, srcFile)
	if err != nil {
		_ = dstFile.Close()
		return fmt.Errorf("error copying the file: %w", err)
	}

	// Sync and Close can surface write errors (e.g. ENOSPC, NFS flush) after a
	// successful io.Copy - they must be checked before the source file gets deleted.
	if err := dstFile.Sync(); err != nil {
		_ = dstFile.Close()
		return fmt.Errorf("error syncing target file: %w", err)
	}
	if err := dstFile.Close(); err != nil {
		return fmt.Errorf("error closing target file: %w", err)
	}

	if written != fileInfo.Size() {
		return fmt.Errorf("incomplete copy: wrote %d of %d bytes to %s", written, fileInfo.Size(), targetPath)
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

func (fh *FileHandler) copyToSFTPClient(srcPath, remotePath, host string, target config.OutputTarget) (err error) {
	// Gepoolten SFTP-Client holen (Verbindung wird wiederverwendet)
	ftpConfig := target.GetFTPConfig()
	client, err := fh.RemoteConns.GetSFTPClient(host, ftpConfig)
	if err != nil {
		return err
	}
	// Bei Fehlern die gecachte Verbindung verwerfen, damit der nächste
	// Versuch frisch verbindet
	defer func() {
		if err != nil {
			fh.RemoteConns.InvalidateSFTPClient(host, ftpConfig)
		}
	}()

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

	// Datei übertragen
	written, err := io.Copy(dstFile, srcFile)
	if err != nil {
		_ = dstFile.Close()
		return fmt.Errorf("fehler beim SFTP-Upload: %w", err)
	}

	// SFTP überträgt gepufferte Writes erst beim Close - ein Fehler hier
	// bedeutet eine unvollständige Remote-Datei.
	if err := dstFile.Close(); err != nil {
		return fmt.Errorf("fehler beim Abschließen der Remote-Datei: %w", err)
	}

	// Remote-Größe gegen die übertragenen Bytes verifizieren, bevor die Quelldatei gelöscht wird
	remoteInfo, err := client.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("fehler beim Verifizieren der Remote-Datei: %w", err)
	}
	if remoteInfo.Size() != written {
		return fmt.Errorf("unvollständiger SFTP-Upload: remote %d Bytes, erwartet %d (%s)", remoteInfo.Size(), written, remotePath)
	}

	slog.Info("Datei erfolgreich über SFTP hochgeladen", "quelle", srcPath, "target", remotePath)
	return nil
}

func (fh *FileHandler) copyToFTPRegular(srcPath, remotePath, host string, target config.OutputTarget) (err error) {
	// Gepoolte FTP-Verbindung holen (wird nach Erfolg wiederverwendet)
	ftpConfig := target.GetFTPConfig()
	client, err := fh.RemoteConns.GetFTPConn(host, ftpConfig)
	if err != nil {
		return err
	}
	defer func() {
		fh.RemoteConns.ReleaseFTPConn(host, ftpConfig, client, err == nil)
	}()

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

	// Gepoolte FTP-Verbindung holen
	ftpConfig := target.GetFTPConfig()
	client, err := fh.RemoteConns.GetFTPConn(host, ftpConfig)
	if err != nil {
		return err
	}
	healthy := true
	defer func() {
		fh.RemoteConns.ReleaseFTPConn(host, ftpConfig, client, healthy)
	}()

	// Use Unix-style path for FTP
	remotePath = normalizeRemotePath(remotePath)

	if err := client.Delete(remotePath); err != nil {
		// Check whether file exists (550 is the standard code for ‘file not found’)
		if strings.Contains(err.Error(), "550") {
			slog.Debug("File does not exist in FTP destination", "path", remotePath)
			return nil // File does not exist - no error
		}
		healthy = false
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
	client, err := fh.RemoteConns.GetSFTPClient(host, ftpConfig)
	if err != nil {
		return err
	}

	// Datei löschen
	if err := client.Remove(remotePath); err != nil {
		if os.IsNotExist(err) {
			slog.Debug("File does not exist in SFTP destination", "path", remotePath)
			return nil // Datei existiert nicht - kein Fehler
		}
		fh.RemoteConns.InvalidateSFTPClient(host, ftpConfig)
		return fmt.Errorf("fehler beim SFTP-Löschen: %w", err)
	}

	slog.Debug("File successfully deleted from the SFTP server", "path", remotePath)
	return nil
}
