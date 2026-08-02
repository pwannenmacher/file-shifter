package services

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"file-shifter/config"

	"github.com/jlaffaye/ftp"
)

// fakeFTPServer is a minimal in-process FTP control-connection server that
// speaks just enough of the protocol for github.com/jlaffaye/ftp's
// Dial/Login/NoOp/Quit. It never opens data connections.
type fakeFTPServer struct {
	listener net.Listener
	password string // expected password; any other password is rejected with 530

	failNoOp   atomic.Bool // when true, NOOP is answered with 500 so liveness checks fail
	dropOnFeat atomic.Bool // when true, the connection is reset instead of answering FEAT

	mu    sync.Mutex
	conns []net.Conn
}

// startFakeFTPServer starts the server on an ephemeral localhost port.
// It is shut down automatically via t.Cleanup.
func startFakeFTPServer(t *testing.T, password string) *fakeFTPServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start fake FTP server: %v", err)
	}

	s := &fakeFTPServer{
		listener: listener,
		password: password,
	}

	go s.acceptLoop()
	t.Cleanup(s.Close)

	return s
}

// Addr returns the host:port the server listens on.
func (s *fakeFTPServer) Addr() string {
	return s.listener.Addr().String()
}

func (s *fakeFTPServer) Close() {
	if err := s.listener.Close(); err != nil {
		// Listener may already be closed; nothing to do in a test helper.
		_ = err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, conn := range s.conns {
		_ = conn.Close()
	}
	s.conns = nil
}

// closeActiveConnectionsAbruptly closes all server-side connections with
// SO_LINGER=0 so the peer receives a TCP RST. Subsequent client reads and
// writes on those connections fail, which makes both NoOp and Quit error out.
func (s *fakeFTPServer) closeActiveConnectionsAbruptly() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, conn := range s.conns {
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetLinger(0)
		}
		_ = conn.Close()
	}
	s.conns = nil
}

func (s *fakeFTPServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns = append(s.conns, conn)
		s.mu.Unlock()
		go s.handleConn(conn)
	}
}

func (s *fakeFTPServer) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	writer := bufio.NewWriter(conn)
	reply := func(line string) bool {
		if _, err := fmt.Fprintf(writer, "%s\r\n", line); err != nil {
			return false
		}
		return writer.Flush() == nil
	}

	if !reply("220 fake FTP server ready") {
		return
	}

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		verb := strings.ToUpper(fields[0])
		arg := ""
		if len(fields) > 1 {
			arg = fields[1]
		}

		var ok bool
		switch verb {
		case "USER":
			ok = reply("331 User name okay, need password")
		case "PASS":
			if arg == s.password {
				ok = reply("230 Login successful")
			} else {
				ok = reply("530 Login incorrect")
			}
		case "FEAT":
			if s.dropOnFeat.Load() {
				// Reset the connection mid-login: the client's read of the
				// FEAT response fails, and the follow-up QUIT write fails too.
				if tcpConn, isTCP := conn.(*net.TCPConn); isTCP {
					_ = tcpConn.SetLinger(0)
				}
				return
			}
			ok = reply("500 FEAT not supported")
		case "TYPE":
			ok = reply("200 Type set")
		case "OPTS":
			ok = reply("200 OK")
		case "NOOP":
			if s.failNoOp.Load() {
				ok = reply("500 NOOP disabled")
			} else {
				ok = reply("200 OK")
			}
		case "QUIT":
			reply("221 Goodbye")
			return
		default:
			ok = reply("500 Unknown command")
		}
		if !ok {
			return
		}
	}
}

// closedPortAddr returns a localhost address that is guaranteed to have no
// listener by opening and immediately closing an ephemeral port.
func closedPortAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve port: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}
	return addr
}

// waitForFTPConnDead polls the connection until the client has observed the
// broken transport, so that a following Quit deterministically fails too.
func waitForFTPConnDead(t *testing.T, conn *ftp.ServerConn) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := conn.NoOp(); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("connection did not become dead in time")
}

func fakeFTPConfig(password string) config.FTPConfig {
	return config.FTPConfig{
		Username: "testuser",
		Password: password,
	}
}

func TestConnectAndLoginFTP_Success(t *testing.T) {
	server := startFakeFTPServer(t, "secret")

	conn, err := connectAndLoginFTP(server.Addr(), fakeFTPConfig("secret"))
	if err != nil {
		t.Fatalf("connectAndLoginFTP() error = %v, want nil", err)
	}
	if conn == nil {
		t.Fatal("connectAndLoginFTP() returned nil connection")
	}
	if err := conn.Quit(); err != nil {
		t.Errorf("Quit() error = %v", err)
	}
}

func TestConnectAndLoginFTP_DialFailure(t *testing.T) {
	addr := closedPortAddr(t)

	conn, err := connectAndLoginFTP(addr, fakeFTPConfig("secret"))
	if err == nil {
		if conn != nil {
			_ = conn.Quit()
		}
		t.Fatal("connectAndLoginFTP() expected error for closed port, got nil")
	}
	if !strings.Contains(err.Error(), "FTP connection failed") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "FTP connection failed")
	}
}

func TestConnectAndLoginFTP_LoginFailure(t *testing.T) {
	server := startFakeFTPServer(t, "secret")

	conn, err := connectAndLoginFTP(server.Addr(), fakeFTPConfig("wrong-password"))
	if err == nil {
		if conn != nil {
			_ = conn.Quit()
		}
		t.Fatal("connectAndLoginFTP() expected error for bad password, got nil")
	}
	if !strings.Contains(err.Error(), "FTP login failed") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "FTP login failed")
	}
}

func TestConnectAndLoginFTP_LoginAbortQuitError(t *testing.T) {
	server := startFakeFTPServer(t, "secret")
	server.dropOnFeat.Store(true)

	conn, err := connectAndLoginFTP(server.Addr(), fakeFTPConfig("secret"))
	if err == nil {
		if conn != nil {
			_ = conn.Quit()
		}
		t.Fatal("connectAndLoginFTP() expected error for aborted login, got nil")
	}
	if !strings.Contains(err.Error(), "FTP login failed") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "FTP login failed")
	}
}

func TestConnectAndLoginFTP_TLSHandshakeFails(t *testing.T) {
	// The fake server does not support AUTH TLS, so the explicit-FTPS upgrade
	// fails. This still exercises the TLS option-building branch.
	server := startFakeFTPServer(t, "secret")

	cfg := fakeFTPConfig("secret")
	cfg.TLS = true

	conn, err := connectAndLoginFTP(server.Addr(), cfg)
	if err == nil {
		if conn != nil {
			_ = conn.Quit()
		}
		t.Fatal("connectAndLoginFTP() expected error for TLS against plain server, got nil")
	}
	if !strings.Contains(err.Error(), "FTP connection failed") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "FTP connection failed")
	}
}

func TestRemoteConnManager_GetFTPConn_NewConnection(t *testing.T) {
	server := startFakeFTPServer(t, "secret")
	manager := NewRemoteConnManager()
	defer manager.Close()

	conn, err := manager.GetFTPConn(server.Addr(), fakeFTPConfig("secret"))
	if err != nil {
		t.Fatalf("GetFTPConn() error = %v, want nil", err)
	}
	if conn == nil {
		t.Fatal("GetFTPConn() returned nil connection")
	}

	manager.ReleaseFTPConn(server.Addr(), fakeFTPConfig("secret"), conn, true)
}

func TestRemoteConnManager_GetFTPConn_ReusesIdleConnection(t *testing.T) {
	server := startFakeFTPServer(t, "secret")
	manager := NewRemoteConnManager()
	defer manager.Close()

	cfg := fakeFTPConfig("secret")

	first, err := manager.GetFTPConn(server.Addr(), cfg)
	if err != nil {
		t.Fatalf("GetFTPConn() error = %v, want nil", err)
	}
	manager.ReleaseFTPConn(server.Addr(), cfg, first, true)

	second, err := manager.GetFTPConn(server.Addr(), cfg)
	if err != nil {
		t.Fatalf("GetFTPConn() after release error = %v, want nil", err)
	}
	if second != first {
		t.Error("GetFTPConn() did not reuse the idle connection")
	}

	manager.ReleaseFTPConn(server.Addr(), cfg, second, true)
}

func TestRemoteConnManager_GetFTPConn_DiscardsDeadIdleConnection(t *testing.T) {
	server := startFakeFTPServer(t, "secret")
	manager := NewRemoteConnManager()
	defer manager.Close()

	cfg := fakeFTPConfig("secret")

	first, err := manager.GetFTPConn(server.Addr(), cfg)
	if err != nil {
		t.Fatalf("GetFTPConn() error = %v, want nil", err)
	}
	manager.ReleaseFTPConn(server.Addr(), cfg, first, true)

	// Make the pooled connection fail its NoOp liveness check. New
	// connections never send NOOP during dial/login, so they are unaffected.
	server.failNoOp.Store(true)

	second, err := manager.GetFTPConn(server.Addr(), cfg)
	if err != nil {
		t.Fatalf("GetFTPConn() with dead idle connection error = %v, want nil", err)
	}
	if second == first {
		t.Error("GetFTPConn() reused a connection that failed the liveness check")
	}

	server.failNoOp.Store(false)
	manager.ReleaseFTPConn(server.Addr(), cfg, second, true)
}

func TestRemoteConnManager_ReleaseFTPConn_NilConnection(t *testing.T) {
	manager := NewRemoteConnManager()
	defer manager.Close()

	// Must not panic
	manager.ReleaseFTPConn("127.0.0.1:21", fakeFTPConfig("secret"), nil, true)
}

func TestRemoteConnManager_ReleaseFTPConn_UnhealthyConnectionIsClosed(t *testing.T) {
	server := startFakeFTPServer(t, "secret")
	manager := NewRemoteConnManager()
	defer manager.Close()

	cfg := fakeFTPConfig("secret")

	conn, err := manager.GetFTPConn(server.Addr(), cfg)
	if err != nil {
		t.Fatalf("GetFTPConn() error = %v, want nil", err)
	}

	manager.ReleaseFTPConn(server.Addr(), cfg, conn, false)

	// The closed connection must not be handed out again
	next, err := manager.GetFTPConn(server.Addr(), cfg)
	if err != nil {
		t.Fatalf("GetFTPConn() after unhealthy release error = %v, want nil", err)
	}
	if next == conn {
		t.Error("GetFTPConn() returned a connection that was released as unhealthy")
	}
	manager.ReleaseFTPConn(server.Addr(), cfg, next, true)
}

func TestRemoteConnManager_ReleaseFTPConn_SurplusBeyondPoolLimitIsClosed(t *testing.T) {
	server := startFakeFTPServer(t, "secret")
	manager := NewRemoteConnManager()
	defer manager.Close()

	cfg := fakeFTPConfig("secret")

	// Open one connection more than the pool keeps idle
	conns := make([]*ftp.ServerConn, 0, maxIdleFTPConns+1)
	for i := 0; i < maxIdleFTPConns+1; i++ {
		conn, err := manager.GetFTPConn(server.Addr(), cfg)
		if err != nil {
			t.Fatalf("GetFTPConn() #%d error = %v, want nil", i, err)
		}
		conns = append(conns, conn)
	}

	// Releasing all of them fills the pool; the last release is surplus and
	// must be closed instead of pooled.
	for _, conn := range conns {
		manager.ReleaseFTPConn(server.Addr(), cfg, conn, true)
	}

	surplus := conns[len(conns)-1]
	if err := surplus.NoOp(); err == nil {
		t.Error("surplus connection beyond pool limit was not closed")
	}
}

func TestRemoteConnManager_GetFTPConn_DeadIdleConnQuitError(t *testing.T) {
	server := startFakeFTPServer(t, "secret")
	manager := NewRemoteConnManager()
	defer manager.Close()

	cfg := fakeFTPConfig("secret")

	first, err := manager.GetFTPConn(server.Addr(), cfg)
	if err != nil {
		t.Fatalf("GetFTPConn() error = %v, want nil", err)
	}
	manager.ReleaseFTPConn(server.Addr(), cfg, first, true)

	// Kill the pooled connection with a RST so that both the NoOp liveness
	// check and the follow-up Quit fail. The listener keeps accepting, so a
	// fresh connection can still be established afterwards.
	server.closeActiveConnectionsAbruptly()
	waitForFTPConnDead(t, first)

	second, err := manager.GetFTPConn(server.Addr(), cfg)
	if err != nil {
		t.Fatalf("GetFTPConn() with dead idle connection error = %v, want nil", err)
	}
	if second == first {
		t.Error("GetFTPConn() reused a dead connection")
	}
	manager.ReleaseFTPConn(server.Addr(), cfg, second, true)
}

func TestRemoteConnManager_ReleaseFTPConn_UnhealthyQuitError(t *testing.T) {
	server := startFakeFTPServer(t, "secret")
	manager := NewRemoteConnManager()
	defer manager.Close()

	cfg := fakeFTPConfig("secret")

	conn, err := manager.GetFTPConn(server.Addr(), cfg)
	if err != nil {
		t.Fatalf("GetFTPConn() error = %v, want nil", err)
	}

	server.closeActiveConnectionsAbruptly()
	waitForFTPConnDead(t, conn)

	// Must not panic even though Quit fails on the dead connection
	manager.ReleaseFTPConn(server.Addr(), cfg, conn, false)
}

func TestRemoteConnManager_ReleaseFTPConn_SurplusQuitError(t *testing.T) {
	server := startFakeFTPServer(t, "secret")
	manager := NewRemoteConnManager()
	defer manager.Close()

	cfg := fakeFTPConfig("secret")

	conns := make([]*ftp.ServerConn, 0, maxIdleFTPConns+1)
	for i := 0; i < maxIdleFTPConns+1; i++ {
		conn, err := manager.GetFTPConn(server.Addr(), cfg)
		if err != nil {
			t.Fatalf("GetFTPConn() #%d error = %v, want nil", i, err)
		}
		conns = append(conns, conn)
	}

	// Fill the pool with the first maxIdleFTPConns connections
	for _, conn := range conns[:maxIdleFTPConns] {
		manager.ReleaseFTPConn(server.Addr(), cfg, conn, true)
	}

	surplus := conns[maxIdleFTPConns]
	server.closeActiveConnectionsAbruptly()
	waitForFTPConnDead(t, surplus)

	// The pool is full, so the surplus connection is quit - which fails on
	// the dead transport. Must not panic.
	manager.ReleaseFTPConn(server.Addr(), cfg, surplus, true)
}

func TestRemoteConnManager_Close_QuitErrorOnDeadIdleConn(t *testing.T) {
	server := startFakeFTPServer(t, "secret")
	manager := NewRemoteConnManager()

	cfg := fakeFTPConfig("secret")

	conn, err := manager.GetFTPConn(server.Addr(), cfg)
	if err != nil {
		t.Fatalf("GetFTPConn() error = %v, want nil", err)
	}
	manager.ReleaseFTPConn(server.Addr(), cfg, conn, true)

	server.closeActiveConnectionsAbruptly()
	// We still hold a pointer to the pooled connection, so we can wait for
	// the client side to notice the broken transport.
	waitForFTPConnDead(t, conn)

	// Close must handle the failing Quit gracefully
	manager.Close()
}

func TestRemoteConnManager_Close_ClosesIdleFTPConnections(t *testing.T) {
	server := startFakeFTPServer(t, "secret")
	manager := NewRemoteConnManager()

	cfg := fakeFTPConfig("secret")

	conn, err := manager.GetFTPConn(server.Addr(), cfg)
	if err != nil {
		t.Fatalf("GetFTPConn() error = %v, want nil", err)
	}
	manager.ReleaseFTPConn(server.Addr(), cfg, conn, true)

	manager.Close()

	if err := conn.NoOp(); err == nil {
		t.Error("idle connection was not closed by Close()")
	}
}
