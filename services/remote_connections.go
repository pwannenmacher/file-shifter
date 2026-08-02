package services

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sync"

	"file-shifter/config"

	"github.com/jlaffaye/ftp"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// maxIdleFTPConns limits the number of idle FTP connections kept per target
const maxIdleFTPConns = 4

// sftpConn bundles the SSH transport with its SFTP client
type sftpConn struct {
	sshClient  *ssh.Client
	sftpClient *sftp.Client
}

func (c *sftpConn) close() {
	if c.sftpClient != nil {
		if err := c.sftpClient.Close(); err != nil {
			slog.Debug("Error closing SFTP client", "error", err)
		}
	}
	if c.sshClient != nil {
		if err := c.sshClient.Close(); err != nil {
			slog.Debug("Error closing SSH connection", "error", err)
		}
	}
}

// RemoteConnManager reuses FTP and SFTP connections across file transfers so
// not every file pays the full connection/handshake cost.
//
// SFTP clients are multiplexing and safe for concurrent use, so one client is
// shared per target. FTP control connections are not concurrency-safe, so an
// idle pool hands each caller an exclusive connection.
type RemoteConnManager struct {
	mu          sync.Mutex
	sftpClients map[string]*sftpConn
	ftpIdle     map[string][]*ftp.ServerConn
}

func NewRemoteConnManager() *RemoteConnManager {
	return &RemoteConnManager{
		sftpClients: make(map[string]*sftpConn),
		ftpIdle:     make(map[string][]*ftp.ServerConn),
	}
}

// connKey builds a cache key covering everything that affects the connection
func connKey(scheme, host string, cfg config.FTPConfig) string {
	data := fmt.Sprintf("%s:%s:%s:%s:%t:%s:%t",
		scheme, host, cfg.Username, cfg.Password, cfg.TLS, cfg.KnownHosts, cfg.InsecureSkipHostKeyVerify)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
}

// GetSFTPClient returns a shared, validated SFTP client for the target,
// reconnecting transparently when the cached connection has died.
func (m *RemoteConnManager) GetSFTPClient(host string, cfg config.FTPConfig) (*sftp.Client, error) {
	key := connKey("sftp", host, cfg)

	m.mu.Lock()
	defer m.mu.Unlock()

	if cached, ok := m.sftpClients[key]; ok {
		// Cheap liveness probe; one round trip instead of a full handshake
		if _, err := cached.sftpClient.RealPath("."); err == nil {
			return cached.sftpClient, nil
		}
		slog.Debug("Cached SFTP connection no longer usable - reconnecting", "host", host)
		cached.close()
		delete(m.sftpClients, key)
	}

	sshConfig, err := createSSHConfig(cfg)
	if err != nil {
		return nil, err
	}

	sshClient, err := ssh.Dial("tcp", host, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("SSH connection failed: %w", err)
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		if closeErr := sshClient.Close(); closeErr != nil {
			slog.Debug("Error closing SSH connection", "error", closeErr)
		}
		return nil, fmt.Errorf("SFTP client creation failed: %w", err)
	}

	m.sftpClients[key] = &sftpConn{sshClient: sshClient, sftpClient: sftpClient}
	slog.Debug("New SFTP connection established and cached", "host", host)
	return sftpClient, nil
}

// InvalidateSFTPClient drops the cached client for a target, e.g. after a
// transfer error that suggests a broken connection.
func (m *RemoteConnManager) InvalidateSFTPClient(host string, cfg config.FTPConfig) {
	key := connKey("sftp", host, cfg)

	m.mu.Lock()
	defer m.mu.Unlock()

	if cached, ok := m.sftpClients[key]; ok {
		cached.close()
		delete(m.sftpClients, key)
	}
}

// GetFTPConn returns an exclusive FTP connection for the target, reusing an
// idle one when available. Callers must return it via ReleaseFTPConn.
func (m *RemoteConnManager) GetFTPConn(host string, cfg config.FTPConfig) (*ftp.ServerConn, error) {
	key := connKey("ftp", host, cfg)

	for {
		m.mu.Lock()
		idle := m.ftpIdle[key]
		if len(idle) == 0 {
			m.mu.Unlock()
			break
		}
		conn := idle[len(idle)-1]
		m.ftpIdle[key] = idle[:len(idle)-1]
		m.mu.Unlock()

		// Validate before reuse; discard dead connections
		if err := conn.NoOp(); err == nil {
			slog.Debug("Reusing idle FTP connection", "host", host)
			return conn, nil
		}
		if err := conn.Quit(); err != nil {
			slog.Debug("Error closing dead FTP connection", "error", err)
		}
	}

	return connectAndLoginFTP(host, cfg)
}

// ReleaseFTPConn returns a connection to the idle pool. Unhealthy connections
// (healthy=false, e.g. after a transfer error) are closed instead.
func (m *RemoteConnManager) ReleaseFTPConn(host string, cfg config.FTPConfig, conn *ftp.ServerConn, healthy bool) {
	if conn == nil {
		return
	}
	if !healthy {
		if err := conn.Quit(); err != nil {
			slog.Debug("Error closing FTP connection", "error", err)
		}
		return
	}

	key := connKey("ftp", host, cfg)

	m.mu.Lock()
	if len(m.ftpIdle[key]) < maxIdleFTPConns {
		m.ftpIdle[key] = append(m.ftpIdle[key], conn)
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	if err := conn.Quit(); err != nil {
		slog.Debug("Error closing surplus FTP connection", "error", err)
	}
}

// Close shuts down all cached connections
func (m *RemoteConnManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, cached := range m.sftpClients {
		cached.close()
		delete(m.sftpClients, key)
	}
	for key, idle := range m.ftpIdle {
		for _, conn := range idle {
			if err := conn.Quit(); err != nil {
				slog.Debug("Error closing FTP connection", "error", err)
			}
		}
		delete(m.ftpIdle, key)
	}

	slog.Debug("All remote connections closed")
}
