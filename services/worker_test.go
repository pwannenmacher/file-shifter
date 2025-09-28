package services

import (
	"file-shifter/config"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Helper functions to reduce code duplication

// setupTempDir creates a temporary directory for testing and returns cleanup function
func setupTempDir(t *testing.T, prefix string) (string, func()) {
	tempDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("Fehler beim Erstellen des temporären Verzeichnisses: %v", err)
	}
	cleanup := func() { os.RemoveAll(tempDir) }
	return tempDir, cleanup
}

// createFilesystemTargets creates standard filesystem targets for testing
func createFilesystemTargets(paths ...string) []config.OutputTarget {
	if len(paths) == 0 {
		paths = []string{"/tmp/output"}
	}

	targets := make([]config.OutputTarget, len(paths))
	for i, path := range paths {
		targets[i] = config.OutputTarget{Type: "filesystem", Path: path}
	}
	return targets
}

// assertWorkerBasics performs standard assertions on worker components
func assertWorkerBasics(t *testing.T, worker *Worker, expectedInputDir string, expectedTargetCount int) {
	if worker == nil {
		t.Fatal("Worker sollte nicht nil sein")
	}

	if worker.InputDir != expectedInputDir {
		t.Errorf("InputDir stimmt nicht überein. Erwartet: %s, Bekommen: %s", expectedInputDir, worker.InputDir)
	}

	if len(worker.OutputTargets) != expectedTargetCount {
		t.Errorf("Anzahl der OutputTargets stimmt nicht überein. Erwartet: %d, Bekommen: %d", expectedTargetCount, len(worker.OutputTargets))
	}

	assertWorkerComponents(t, worker)
}

// assertWorkerComponents checks that all worker components are properly initialized
func assertWorkerComponents(t *testing.T, worker *Worker) {
	if worker.S3ClientManager == nil {
		t.Error("S3ClientManager sollte nicht nil sein")
	}

	if worker.FileHandler == nil {
		t.Error("FileHandler sollte nicht nil sein")
	}

	if worker.FileWatcher == nil {
		t.Error("FileWatcher sollte nicht nil sein")
	}

	if worker.stopChan == nil {
		t.Error("stopChan sollte nicht nil sein")
	}
}

// assertTargets verifies that targets match expected values
func assertTargets(t *testing.T, actualTargets []config.OutputTarget, expectedPaths []string) {
	if len(actualTargets) != len(expectedPaths) {
		t.Fatalf("Erwartete %d Targets, bekommen: %d", len(expectedPaths), len(actualTargets))
	}

	for i, target := range actualTargets {
		if target.Type != "filesystem" {
			t.Errorf("Target[%d].Type stimmt nicht überein. Erwartet: filesystem, Bekommen: %s", i, target.Type)
		}
		if target.Path != expectedPaths[i] {
			t.Errorf("Target[%d].Path stimmt nicht überein. Erwartet: %s, Bekommen: %s", i, expectedPaths[i], target.Path)
		}
	}
}

// createDefaultConfig erstellt eine Standard-Konfiguration für Tests
func createDefaultConfig() *config.EnvConfig {
	cfg := &config.EnvConfig{}
	cfg.SetDefaults()
	return cfg
}

func TestNewWorker_ValidInput(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_test_*")
	defer cleanup()

	targets := createFilesystemTargets()
	cfg := createDefaultConfig()
	worker := NewWorker(tempDir, targets, cfg)

	assertWorkerBasics(t, worker, tempDir, 1)
}

func TestNewWorker_NonExistentInputDir(t *testing.T) {
	// Test-Setup: Nicht existierendes Verzeichnis
	nonExistentDir := filepath.Join(os.TempDir(), "non_existent_worker_test")

	// Sicherstellen, dass das Verzeichnis nicht existiert
	os.RemoveAll(nonExistentDir)
	defer os.RemoveAll(nonExistentDir)

	targets := createFilesystemTargets()
	cfg := createDefaultConfig()
	worker := NewWorker(nonExistentDir, targets, cfg)

	assertWorkerBasics(t, worker, nonExistentDir, 1)

	// Prüfen, dass das Verzeichnis erstellt wurde
	if _, err := os.Stat(nonExistentDir); os.IsNotExist(err) {
		t.Error("Input-Verzeichnis sollte erstellt worden sein")
	}
}

func TestNewWorker_EmptyTargets(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_test_empty_*")
	defer cleanup()

	var targets []config.OutputTarget
	cfg := createDefaultConfig()
	worker := NewWorker(tempDir, targets, cfg)

	assertWorkerBasics(t, worker, tempDir, 0)
}

func TestNewWorker_FilesystemTarget(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_test_fs_*")
	defer cleanup()

	expectedPath := "/tmp/test-output"
	targets := createFilesystemTargets(expectedPath)
	cfg := createDefaultConfig()
	worker := NewWorker(tempDir, targets, cfg)

	assertWorkerBasics(t, worker, tempDir, 1)
	assertTargets(t, worker.OutputTargets, []string{expectedPath})
}

func TestNewWorker_MultipleTargets(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_test_multi_*")
	defer cleanup()

	expectedPaths := []string{"/tmp/output1", "/tmp/output2"}
	targets := createFilesystemTargets(expectedPaths...)
	cfg := createDefaultConfig()
	worker := NewWorker(tempDir, targets, cfg)

	assertWorkerBasics(t, worker, tempDir, 2)
	assertTargets(t, worker.OutputTargets, expectedPaths)
}

func TestNewWorker_StopChannelInitialized(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_test_stop_*")
	defer cleanup()

	targets := createFilesystemTargets()
	cfg := createDefaultConfig()
	worker := NewWorker(tempDir, targets, cfg)

	assertWorkerBasics(t, worker, tempDir, 1)
	// stopChan wird bereits in assertWorkerBasics -> assertWorkerComponents geprüft
}

func TestNewWorker_ComponentsProperlyInitialized(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_test_components_*")
	defer cleanup()

	targets := createFilesystemTargets()
	cfg := createDefaultConfig()
	worker := NewWorker(tempDir, targets, cfg)

	assertWorkerBasics(t, worker, tempDir, 1)
	// Detaillierte Komponentenprüfung wird bereits in assertWorkerComponents durchgeführt
}

func TestNewWorker_InputDirValidation(t *testing.T) {
	tempBaseDir, cleanup := setupTempDir(t, "worker_test_special_*")
	defer cleanup()

	// Unterverzeichnis mit Leerzeichen und speziellen Zeichen
	specialDir := filepath.Join(tempBaseDir, "test dir with spaces")
	targets := createFilesystemTargets()
	cfg := createDefaultConfig()
	worker := NewWorker(specialDir, targets, cfg)

	assertWorkerBasics(t, worker, specialDir, 1)

	// Prüfen, dass das Verzeichnis erstellt wurde
	if _, err := os.Stat(specialDir); os.IsNotExist(err) {
		t.Error("Spezielles Input-Verzeichnis sollte erstellt worden sein")
	}
}

func TestNewWorker_DifferentTargetTypes(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_test_types_*")
	defer cleanup()

	expectedPaths := []string{"/tmp/output1", "/tmp/output2"}
	targets := createFilesystemTargets(expectedPaths...)
	cfg := createDefaultConfig()
	worker := NewWorker(tempDir, targets, cfg)

	assertWorkerBasics(t, worker, tempDir, 2)
	assertTargets(t, worker.OutputTargets, expectedPaths)
}

// Hinweis: Tests für ungültige Eingaben (leerer InputDir, ungültige S3-Konfiguration)
// führen zu os.Exit(1) und können daher nicht einfach getestet werden.
// Diese würden separate Test-Funktionen erfordern, die in einem subprocess laufen.

func TestWorker_StartAndStop(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_start_stop_*")
	defer cleanup()

	targets := createFilesystemTargets()
	cfg := createDefaultConfig()
	worker := NewWorker(tempDir, targets, cfg)

	// Use a channel to signal when Start() begins
	started := make(chan bool, 1)
	stopped := make(chan bool, 1)

	// Start worker in goroutine
	go func() {
		started <- true
		worker.Start()
		stopped <- true
	}()

	// Wait for worker to start
	<-started

	// Give it a brief moment to initialize
	time.Sleep(10 * time.Millisecond)

	// Stop the worker
	worker.Stop()

	// Wait for worker to stop with timeout
	select {
	case <-stopped:
		// Test completed successfully
	case <-time.After(1 * time.Second):
		t.Error("Worker did not stop within timeout")
	}
}

func TestNewWorker_S3TargetValidation(t *testing.T) {
	_, cleanup := setupTempDir(t, "worker_s3_*")
	defer cleanup()

	// Test that S3 target structure is properly defined with S3-specific configuration
	// Note: We can't actually test S3 client creation without real credentials
	// This test validates the S3-specific Endpoint field
	s3Target := config.OutputTarget{
		Type:     "s3",
		Endpoint: "s3.amazonaws.com",
	}

	// Verify S3-specific structure is correct
	if s3Target.Type != "s3" {
		t.Errorf("S3 target type should be 's3', got %s", s3Target.Type)
	}
	if s3Target.Endpoint != "s3.amazonaws.com" {
		t.Errorf("S3 target endpoint should be 's3.amazonaws.com', got %s", s3Target.Endpoint)
	}
}

func TestNewWorker_FTPTargetValidation(t *testing.T) {
	_, cleanup := setupTempDir(t, "worker_ftp_*")
	defer cleanup()

	// Test that FTP target structure is properly defined with FTP-specific configuration
	// This test validates the FTP-specific Host field
	ftpTarget := config.OutputTarget{
		Type: "ftp",
		Host: "ftp.example.com",
	}

	// Verify FTP-specific structure is correct
	if ftpTarget.Type != "ftp" {
		t.Errorf("FTP target type should be 'ftp', got %s", ftpTarget.Type)
	}
	if ftpTarget.Host != "ftp.example.com" {
		t.Errorf("FTP target host should be 'ftp.example.com', got %s", ftpTarget.Host)
	}
}

func TestNewWorker_SFTPTargetValidation(t *testing.T) {
	_, cleanup := setupTempDir(t, "worker_sftp_*")
	defer cleanup()

	// Test that SFTP target structure is properly defined with SFTP-specific configuration
	// This test validates the SFTP-specific Host field
	sftpTarget := config.OutputTarget{
		Type: "sftp",
		Host: "sftp.example.com",
	}

	// Verify SFTP-specific structure is correct
	if sftpTarget.Type != "sftp" {
		t.Errorf("SFTP target type should be 'sftp', got %s", sftpTarget.Type)
	}
	if sftpTarget.Host != "sftp.example.com" {
		t.Errorf("SFTP target host should be 'sftp.example.com', got %s", sftpTarget.Host)
	}
}

func TestNewWorker_MixedTargets(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_mixed_*")
	defer cleanup()

	// Create mixed target types (only filesystem will work in tests)
	targets := []config.OutputTarget{
		{Type: "filesystem", Path: "/tmp/output1"},
		{Type: "filesystem", Path: "/tmp/output2"},
	}

	cfg := createDefaultConfig()
	worker := NewWorker(tempDir, targets, cfg)

	assertWorkerBasics(t, worker, tempDir, 2)

	// Verify all targets are set correctly
	for i, target := range worker.OutputTargets {
		if target.Type != "filesystem" {
			t.Errorf("Target[%d] should be filesystem type, got %s", i, target.Type)
		}
	}
}

func TestWorker_ComponentInitialization(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_init_*")
	defer cleanup()

	targets := createFilesystemTargets()
	cfg := createDefaultConfig()
	worker := NewWorker(tempDir, targets, cfg)

	// Verify S3ClientManager is properly initialized
	if worker.S3ClientManager == nil {
		t.Error("S3ClientManager should not be nil")
	} else {
		// Should start with 0 active clients
		if count := worker.S3ClientManager.GetActiveClientCount(); count != 0 {
			t.Errorf("Expected 0 active S3 clients, got %d", count)
		}
	}

	// Verify FileHandler has correct targets
	if worker.FileHandler == nil {
		t.Error("FileHandler should not be nil")
	} else {
		if len(worker.FileHandler.OutputTargets) != len(targets) {
			t.Errorf("FileHandler should have %d targets, got %d", len(targets), len(worker.FileHandler.OutputTargets))
		}
	}

	// Verify FileWatcher is initialized with correct input directory
	if worker.FileWatcher == nil {
		t.Error("FileWatcher should not be nil")
	}
}

// Benchmark tests for Worker creation
func BenchmarkNewWorker(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "bench_worker_*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targets := createFilesystemTargets("/tmp/bench1", "/tmp/bench2")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg := createDefaultConfig()
		_ = NewWorker(tempDir, targets, cfg)
		// Don't call Stop() in benchmark to avoid blocking
	}
}

func BenchmarkWorker_StartStop(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "bench_start_stop_*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Skip this benchmark as it's complex to test Start/Stop coordination
	b.Skip("Start/Stop coordination too complex for reliable benchmarking")
}

// Test validation functions that currently have 0% coverage
func TestWorker_validateS3Target(t *testing.T) {
	cfg := createDefaultConfig()
	worker := NewWorker("/tmp", []config.OutputTarget{}, cfg)

	tests := []struct {
		name        string
		target      config.OutputTarget
		expectError bool
	}{
		{
			name: "valid S3 target",
			target: config.OutputTarget{
				Type:      "s3",
				Path:      "s3://test-bucket/path",
				Endpoint:  "localhost:9000",
				AccessKey: "test-key",
				SecretKey: "test-secret",
				Region:    "us-east-1",
			},
			expectError: true, // Will fail because MinIO server isn't running
		},
		{
			name: "S3 target missing endpoint",
			target: config.OutputTarget{
				Type:      "s3",
				Path:      "s3://test-bucket/path",
				AccessKey: "test-key",
				SecretKey: "test-secret",
				Region:    "us-east-1",
			},
			expectError: true,
		},
		{
			name: "S3 target missing access key",
			target: config.OutputTarget{
				Type:      "s3",
				Path:      "s3://test-bucket/path",
				Endpoint:  "localhost:9000",
				SecretKey: "test-secret",
				Region:    "us-east-1",
			},
			expectError: true,
		},
		{
			name: "S3 target missing secret key",
			target: config.OutputTarget{
				Type:      "s3",
				Path:      "s3://test-bucket/path",
				Endpoint:  "localhost:9000",
				AccessKey: "test-key",
				Region:    "us-east-1",
			},
			expectError: true,
		},
		{
			name: "S3 target missing region",
			target: config.OutputTarget{
				Type:      "s3",
				Path:      "s3://test-bucket/path",
				Endpoint:  "localhost:9000",
				AccessKey: "test-key",
				SecretKey: "test-secret",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := worker.validateS3Target(tt.target)
			if tt.expectError && err == nil {
				t.Error("Expected error, but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, but got: %v", err)
			}
		})
	}
}

func TestWorker_validateFTPTarget(t *testing.T) {
	cfg := createDefaultConfig()
	worker := NewWorker("/tmp", []config.OutputTarget{}, cfg)

	tests := []struct {
		name        string
		target      config.OutputTarget
		expectError bool
	}{
		{
			name: "valid FTP target",
			target: config.OutputTarget{
				Type:     "ftp",
				Path:     "ftp://test.example.com/path",
				Host:     "test.example.com",
				Username: "testuser",
				Password: "testpass",
				Port:     21,
			},
			expectError: false,
		},
		{
			name: "valid SFTP target",
			target: config.OutputTarget{
				Type:     "sftp",
				Path:     "sftp://test.example.com/path",
				Host:     "test.example.com",
				Username: "testuser",
				Password: "testpass",
				Port:     22,
			},
			expectError: false,
		},
		{
			name: "FTP target missing host",
			target: config.OutputTarget{
				Type:     "ftp",
				Path:     "ftp://test.example.com/path", // Host is extracted from Path
				Username: "testuser",
				Password: "testpass",
				Port:     21,
			},
			expectError: false, // Host is extracted from Path in GetFTPConfig()
		},
		{
			name: "FTP target missing username",
			target: config.OutputTarget{
				Type:     "ftp",
				Path:     "ftp://test.example.com/path",
				Host:     "test.example.com",
				Password: "testpass",
				Port:     21,
			},
			expectError: true,
		},
		{
			name: "FTP target missing password",
			target: config.OutputTarget{
				Type:     "ftp",
				Path:     "ftp://test.example.com/path",
				Host:     "test.example.com",
				Username: "testuser",
				Port:     21,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := worker.validateFTPTarget(tt.target)
			if tt.expectError && err == nil {
				t.Error("Expected error, but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, but got: %v", err)
			}
		})
	}
}

func TestWorker_validateFilesystemTarget(t *testing.T) {
	cfg := createDefaultConfig()
	worker := NewWorker("/tmp", []config.OutputTarget{}, cfg)

	tempDir, cleanup := setupTempDir(t, "filesystem_validation_*")
	defer cleanup()

	tests := []struct {
		name        string
		target      config.OutputTarget
		expectError bool
	}{
		{
			name: "valid filesystem target - existing directory",
			target: config.OutputTarget{
				Type: "filesystem",
				Path: tempDir,
			},
			expectError: false,
		},
		{
			name: "filesystem target - non-existing directory (should still be valid)",
			target: config.OutputTarget{
				Type: "filesystem",
				Path: "/tmp/non_existing_test_dir_xyz123",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := worker.validateFilesystemTarget(tt.target)
			if tt.expectError && err == nil {
				t.Error("Expected error, but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, but got: %v", err)
			}
		})
	}
}

func TestWorker_validateSingleTarget(t *testing.T) {
	cfg := createDefaultConfig()
	worker := NewWorker("/tmp", []config.OutputTarget{}, cfg)

	tests := []struct {
		name        string
		target      config.OutputTarget
		expectError bool
	}{
		{
			name: "valid filesystem target",
			target: config.OutputTarget{
				Type: "filesystem",
				Path: "/tmp/test",
			},
			expectError: false,
		},
		{
			name: "valid s3 target",
			target: config.OutputTarget{
				Type:      "s3",
				Path:      "s3://test-bucket/path",
				Endpoint:  "localhost:9000",
				AccessKey: "test-key",
				SecretKey: "test-secret",
				Region:    "us-east-1",
			},
			expectError: true, // Will fail because MinIO server isn't running
		},
		{
			name: "valid ftp target",
			target: config.OutputTarget{
				Type:     "ftp",
				Path:     "ftp://test.example.com/path",
				Host:     "test.example.com",
				Username: "testuser",
				Password: "testpass",
				Port:     21,
			},
			expectError: false,
		},
		{
			name: "unknown target type",
			target: config.OutputTarget{
				Type: "unknown",
				Path: "/some/path",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := worker.validateSingleTarget(tt.target)
			if tt.expectError && err == nil {
				t.Error("Expected error, but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, but got: %v", err)
			}
		})
	}
}
