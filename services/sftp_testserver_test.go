package services

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	sftpTestUser     = "testuser"
	sftpTestPassword = "testpass"
)

// sftpTestServer is a minimal in-process SSH+SFTP server for tests. It accepts
// password authentication (sftpTestUser/sftpTestPassword) and serves the SFTP
// subsystem rooted at RootDir.
type sftpTestServer struct {
	Addr        string        // host:port the server listens on
	HostPubKey  ssh.PublicKey // host public key for known_hosts entries
	RootDir     string        // working directory of the SFTP subsystem
	listener    net.Listener
	sshConfig   *ssh.ServerConfig
	mu          sync.Mutex
	activeConns []net.Conn
	closed      bool
	wg          sync.WaitGroup
}

// startSFTPTestServer starts an SSH server with an SFTP subsystem on
// 127.0.0.1:0. The server is stopped automatically via t.Cleanup.
func startSFTPTestServer(t *testing.T) *sftpTestServer {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("failed to create host key signer: %v", err)
	}
	hostPubKey, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("failed to convert host public key: %v", err)
	}

	sshConfig := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if conn.User() == sftpTestUser && string(password) == sftpTestPassword {
				return nil, nil
			}
			return nil, errors.New("invalid credentials")
		},
	}
	sshConfig.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	srv := &sftpTestServer{
		Addr:       listener.Addr().String(),
		HostPubKey: hostPubKey,
		RootDir:    t.TempDir(),
		listener:   listener,
		sshConfig:  sshConfig,
	}

	srv.wg.Add(1)
	go srv.acceptLoop()

	t.Cleanup(srv.Close)
	return srv
}

func (s *sftpTestServer) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return // listener closed
		}

		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			_ = conn.Close()
			return
		}
		s.activeConns = append(s.activeConns, conn)
		s.mu.Unlock()

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

func (s *sftpTestServer) handleConn(conn net.Conn) {
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.sshConfig)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "only session channels are supported")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleSession(channel, requests)
		}()
	}
}

func (s *sftpTestServer) handleSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()
	for req := range requests {
		// Subsystem request payload: uint32 length + name
		isSFTP := req.Type == "subsystem" && len(req.Payload) > 4 && string(req.Payload[4:]) == "sftp"
		if err := req.Reply(isSFTP, nil); err != nil {
			return
		}
		if !isSFTP {
			continue
		}

		server, err := sftp.NewServer(channel, sftp.WithServerWorkingDirectory(s.RootDir))
		if err != nil {
			return
		}
		_ = server.Serve()
		_ = server.Close()
		return
	}
}

// CloseActiveConnections kills all currently accepted TCP connections without
// stopping the listener, simulating a dropped connection.
func (s *sftpTestServer) CloseActiveConnections() {
	s.mu.Lock()
	conns := s.activeConns
	s.activeConns = nil
	s.mu.Unlock()

	for _, conn := range conns {
		_ = conn.Close()
	}
}

// Close stops the listener and terminates all active connections.
func (s *sftpTestServer) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	_ = s.listener.Close()
	s.CloseActiveConnections()
	s.wg.Wait()
}

// writeKnownHostsFile writes a known_hosts file in dir containing the server's
// host key and returns its path.
func (s *sftpTestServer) writeKnownHostsFile(t *testing.T, dir string) string {
	t.Helper()
	line := knownhosts.Line([]string{s.Addr}, s.HostPubKey)
	path := filepath.Join(dir, "known_hosts")
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write known_hosts file: %v", err)
	}
	return path
}
