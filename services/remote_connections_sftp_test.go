package services

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"file-shifter/config"
)

// insecureFTPConfig returns an FTPConfig for the test server with host key
// verification disabled.
func insecureFTPConfig() config.FTPConfig {
	return config.FTPConfig{
		Username:                  sftpTestUser,
		Password:                  sftpTestPassword,
		InsecureSkipHostKeyVerify: true,
	}
}

func TestRemoteConnManager_GetSFTPClient_FreshConnection(t *testing.T) {
	srv := startSFTPTestServer(t)

	m := NewRemoteConnManager()
	defer m.Close()

	client, err := m.GetSFTPClient(srv.Addr, insecureFTPConfig())
	if err != nil {
		t.Fatalf("GetSFTPClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("GetSFTPClient() returned nil client")
	}
	if _, err := client.RealPath("."); err != nil {
		t.Errorf("RealPath() on fresh client failed: %v", err)
	}
}

func TestRemoteConnManager_GetSFTPClient_ReusesCachedClient(t *testing.T) {
	srv := startSFTPTestServer(t)

	m := NewRemoteConnManager()
	defer m.Close()

	cfg := insecureFTPConfig()
	client1, err := m.GetSFTPClient(srv.Addr, cfg)
	if err != nil {
		t.Fatalf("first GetSFTPClient() error = %v", err)
	}
	client2, err := m.GetSFTPClient(srv.Addr, cfg)
	if err != nil {
		t.Fatalf("second GetSFTPClient() error = %v", err)
	}
	if client1 != client2 {
		t.Error("expected the cached client to be reused for the same target")
	}
}

func TestRemoteConnManager_GetSFTPClient_ReconnectsAfterConnectionDied(t *testing.T) {
	srv := startSFTPTestServer(t)

	m := NewRemoteConnManager()
	defer m.Close()

	cfg := insecureFTPConfig()
	client1, err := m.GetSFTPClient(srv.Addr, cfg)
	if err != nil {
		t.Fatalf("first GetSFTPClient() error = %v", err)
	}

	// Kill the server-side connection so the cached client's liveness probe fails
	srv.CloseActiveConnections()

	// Wait until the cached client actually observes the dead connection
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := client1.RealPath("."); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cached client did not observe the dropped connection in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	client2, err := m.GetSFTPClient(srv.Addr, cfg)
	if err != nil {
		t.Fatalf("GetSFTPClient() after dropped connection error = %v", err)
	}
	if client2 == client1 {
		t.Error("expected a new client after the cached connection died")
	}
	if _, err := client2.RealPath("."); err != nil {
		t.Errorf("RealPath() on reconnected client failed: %v", err)
	}
}

func TestRemoteConnManager_GetSFTPClient_DialFailure(t *testing.T) {
	// Reserve a port and close it again so dialing it fails
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve port: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}

	m := NewRemoteConnManager()
	defer m.Close()

	_, err = m.GetSFTPClient(addr, insecureFTPConfig())
	if err == nil {
		t.Fatal("GetSFTPClient() should fail for a closed port")
	}
	if !strings.Contains(err.Error(), "SSH connection failed") {
		t.Errorf("error should mention SSH connection failure: %v", err)
	}
}

func TestRemoteConnManager_GetSFTPClient_SSHConfigError(t *testing.T) {
	m := NewRemoteConnManager()
	defer m.Close()

	cfg := config.FTPConfig{
		Username:   sftpTestUser,
		Password:   sftpTestPassword,
		KnownHosts: filepath.Join(t.TempDir(), "does-not-exist"),
	}

	if _, err := m.GetSFTPClient("127.0.0.1:22", cfg); err == nil {
		t.Fatal("GetSFTPClient() should fail when the configured known_hosts file is missing")
	}
}

func TestRemoteConnManager_InvalidateSFTPClient(t *testing.T) {
	srv := startSFTPTestServer(t)

	m := NewRemoteConnManager()
	defer m.Close()

	cfg := insecureFTPConfig()

	t.Run("without cached client", func(t *testing.T) {
		// Must be a no-op and not panic
		m.InvalidateSFTPClient(srv.Addr, cfg)
	})

	t.Run("with cached client", func(t *testing.T) {
		client, err := m.GetSFTPClient(srv.Addr, cfg)
		if err != nil {
			t.Fatalf("GetSFTPClient() error = %v", err)
		}

		m.InvalidateSFTPClient(srv.Addr, cfg)

		// The invalidated client must be closed
		if _, err := client.RealPath("."); err == nil {
			t.Error("invalidated client should be closed")
		}

		// A subsequent call must create a fresh connection
		client2, err := m.GetSFTPClient(srv.Addr, cfg)
		if err != nil {
			t.Fatalf("GetSFTPClient() after invalidation error = %v", err)
		}
		if client2 == client {
			t.Error("expected a new client after invalidation")
		}
	})
}

func TestRemoteConnManager_Close_WithCachedSFTPClients(t *testing.T) {
	srv := startSFTPTestServer(t)

	m := NewRemoteConnManager()

	client, err := m.GetSFTPClient(srv.Addr, insecureFTPConfig())
	if err != nil {
		t.Fatalf("GetSFTPClient() error = %v", err)
	}

	m.Close()

	if _, err := client.RealPath("."); err == nil {
		t.Error("client should be closed after RemoteConnManager.Close()")
	}
	if len(m.sftpClients) != 0 {
		t.Errorf("sftpClients map should be empty after Close(), has %d entries", len(m.sftpClients))
	}

	// Close must be idempotent
	m.Close()
}

// sftpOutputTarget builds an SFTP OutputTarget pointing at the test server.
func sftpOutputTarget(srv *sftpTestServer, basePath string) config.OutputTarget {
	return config.OutputTarget{
		Type:                      "sftp",
		Path:                      "sftp://" + srv.Addr + basePath,
		Username:                  sftpTestUser,
		Password:                  sftpTestPassword,
		InsecureSkipHostKeyVerify: true,
	}
}

func TestFileHandler_copyToSFTP_HappyPath(t *testing.T) {
	srv := startSFTPTestServer(t)

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "test.txt")
	testContent := "sftp upload content"
	if err := os.WriteFile(srcFile, []byte(testContent), 0o644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	fh := NewFileHandler(nil, NewS3ClientManager())
	defer fh.Close()

	target := sftpOutputTarget(srv, "/uploads")
	if err := fh.copyToSFTP(srcFile, "test.txt", target); err != nil {
		t.Fatalf("copyToSFTP() error = %v", err)
	}

	uploaded := filepath.Join(srv.RootDir, "uploads", "test.txt")
	content, err := os.ReadFile(uploaded)
	if err != nil {
		t.Fatalf("failed to read uploaded file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("uploaded content = %q, want %q", string(content), testContent)
	}

	info, err := os.Stat(uploaded)
	if err != nil {
		t.Fatalf("failed to stat uploaded file: %v", err)
	}
	if info.Size() != int64(len(testContent)) {
		t.Errorf("uploaded size = %d, want %d", info.Size(), len(testContent))
	}
}

func TestFileHandler_copyToSFTP_KnownHostsVerification(t *testing.T) {
	srv := startSFTPTestServer(t)

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "verified.txt")
	testContent := "verified upload"
	if err := os.WriteFile(srcFile, []byte(testContent), 0o644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	t.Run("host key in known_hosts", func(t *testing.T) {
		knownHosts := srv.writeKnownHostsFile(t, t.TempDir())

		fh := NewFileHandler(nil, NewS3ClientManager())
		defer fh.Close()

		target := sftpOutputTarget(srv, "/verified")
		target.InsecureSkipHostKeyVerify = false
		target.KnownHosts = knownHosts

		if err := fh.copyToSFTP(srcFile, "verified.txt", target); err != nil {
			t.Fatalf("copyToSFTP() with valid known_hosts error = %v", err)
		}

		content, err := os.ReadFile(filepath.Join(srv.RootDir, "verified", "verified.txt"))
		if err != nil {
			t.Fatalf("failed to read uploaded file: %v", err)
		}
		if string(content) != testContent {
			t.Errorf("uploaded content = %q, want %q", string(content), testContent)
		}
	})

	t.Run("host key missing from known_hosts fails closed", func(t *testing.T) {
		// known_hosts contains only an entry for an unrelated host
		knownHosts := filepath.Join(t.TempDir(), "known_hosts")
		entry := "example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB3BJDcBUzL3N0jXNbNsDbbmDKgWsRfkTxV5jVLpOAKU\n"
		if err := os.WriteFile(knownHosts, []byte(entry), 0o600); err != nil {
			t.Fatalf("failed to write known_hosts: %v", err)
		}

		fh := NewFileHandler(nil, NewS3ClientManager())
		defer fh.Close()

		target := sftpOutputTarget(srv, "/rejected")
		target.InsecureSkipHostKeyVerify = false
		target.KnownHosts = knownHosts

		if err := fh.copyToSFTP(srcFile, "rejected.txt", target); err == nil {
			t.Fatal("copyToSFTP() should fail when the host key is not in known_hosts")
		}

		if _, err := os.Stat(filepath.Join(srv.RootDir, "rejected", "rejected.txt")); !os.IsNotExist(err) {
			t.Error("no file must be uploaded when host key verification fails")
		}
	})
}

func TestResolveKnownHostsFile_NoFallbackFound(t *testing.T) {
	if _, err := os.Stat("/etc/ssh/ssh_known_hosts"); err == nil {
		t.Skip("/etc/ssh/ssh_known_hosts exists on this machine - fallback would succeed")
	}

	// Point HOME at an empty directory so ~/.ssh/known_hosts does not exist
	t.Setenv("HOME", t.TempDir())

	_, err := resolveKnownHostsFile("")
	if err == nil {
		t.Fatal("resolveKnownHostsFile() should fail when no known_hosts file exists")
	}
	if !strings.Contains(err.Error(), "no known_hosts file found") {
		t.Errorf("error should mention that no known_hosts file was found: %v", err)
	}
}

func TestFileHandler_deleteFromSFTP_HappyPath(t *testing.T) {
	srv := startSFTPTestServer(t)

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "delete_me.txt")
	if err := os.WriteFile(srcFile, []byte("to be deleted"), 0o644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	fh := NewFileHandler(nil, NewS3ClientManager())
	defer fh.Close()

	target := sftpOutputTarget(srv, "/uploads")

	if err := fh.copyToSFTP(srcFile, "delete_me.txt", target); err != nil {
		t.Fatalf("copyToSFTP() error = %v", err)
	}

	uploaded := filepath.Join(srv.RootDir, "uploads", "delete_me.txt")
	if _, err := os.Stat(uploaded); err != nil {
		t.Fatalf("uploaded file missing before delete: %v", err)
	}

	if err := fh.deleteFromSFTP("delete_me.txt", target); err != nil {
		t.Fatalf("deleteFromSFTP() error = %v", err)
	}

	if _, err := os.Stat(uploaded); !os.IsNotExist(err) {
		t.Error("file should have been deleted from the SFTP server")
	}

	t.Run("non-existent file is not an error", func(t *testing.T) {
		if err := fh.deleteFromSFTP("never_uploaded.txt", target); err != nil {
			t.Errorf("deleteFromSFTP() for non-existent file should succeed, got: %v", err)
		}
	})
}
