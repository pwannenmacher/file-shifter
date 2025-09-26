package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"file-shifter/config"
)

// Tests für Hilfsfunktionen
func TestNormalizeRemotePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"unix path unchanged", "path/to/file.txt", "path/to/file.txt"},
		{"windows backslashes converted", "path\\to\\file.txt", "path/to/file.txt"},
		{"mixed slashes normalized", "path/to\\file.txt", "path/to/file.txt"},
		{"single backslash", "file.txt\\", "file.txt/"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeRemotePath(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeRemotePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseRemotePath(t *testing.T) {
	tests := []struct {
		name         string
		targetPath   string
		relPath      string
		defaultPort  string
		expectedHost string
		expectedPath string
		wantErr      bool
	}{
		{
			name:         "ftp with port",
			targetPath:   "ftp://server.com:21/base/path",
			relPath:      "file.txt",
			defaultPort:  "21",
			expectedHost: "server.com:21",
			expectedPath: "base/path/file.txt",
			wantErr:      false,
		},
		{
			name:         "ftp without port",
			targetPath:   "ftp://server.com/base",
			relPath:      "subdir/file.txt",
			defaultPort:  "21",
			expectedHost: "server.com:21",
			expectedPath: "base/subdir/file.txt",
			wantErr:      false,
		},
		{
			name:         "sftp with custom port",
			targetPath:   "sftp://server.com:2222/uploads",
			relPath:      "data.txt",
			defaultPort:  "22",
			expectedHost: "server.com:2222",
			expectedPath: "uploads/data.txt",
			wantErr:      false,
		},
		{
			name:         "no base path",
			targetPath:   "ftp://server.com",
			relPath:      "file.txt",
			defaultPort:  "21",
			expectedHost: "server.com:21",
			expectedPath: "file.txt",
			wantErr:      false,
		},
		{
			name:         "invalid url",
			targetPath:   "://invalid-url-format",
			relPath:      "file.txt",
			defaultPort:  "21",
			expectedHost: "",
			expectedPath: "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, remotePath, err := parseRemotePath(tt.targetPath, tt.relPath, tt.defaultPort)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRemotePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if host != tt.expectedHost {
					t.Errorf("parseRemotePath() host = %q, want %q", host, tt.expectedHost)
				}
				if remotePath != tt.expectedPath {
					t.Errorf("parseRemotePath() remotePath = %q, want %q", remotePath, tt.expectedPath)
				}
			}
		})
	}
}

func TestParseS3Path(t *testing.T) {
	tests := []struct {
		name           string
		targetPath     string
		relPath        string
		expectedBucket string
		expectedKey    string
		wantErr        bool
	}{
		{
			name:           "s3 with prefix",
			targetPath:     "s3://my-bucket/uploads/data",
			relPath:        "file.txt",
			expectedBucket: "my-bucket",
			expectedKey:    "uploads/data/file.txt",
			wantErr:        false,
		},
		{
			name:           "s3 without prefix",
			targetPath:     "s3://my-bucket",
			relPath:        "file.txt",
			expectedBucket: "my-bucket",
			expectedKey:    "file.txt",
			wantErr:        false,
		},
		{
			name:           "s3 with nested relpath",
			targetPath:     "s3://bucket/base",
			relPath:        "subdir/nested/file.txt",
			expectedBucket: "bucket",
			expectedKey:    "base/subdir/nested/file.txt",
			wantErr:        false,
		},
		{
			name:           "windows paths normalized",
			targetPath:     "s3://bucket/path",
			relPath:        "subdir\\file.txt",
			expectedBucket: "bucket",
			expectedKey:    "path/subdir/file.txt",
			wantErr:        false,
		},
		{
			name:           "invalid url",
			targetPath:     "://invalid-s3-url",
			relPath:        "file.txt",
			expectedBucket: "",
			expectedKey:    "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s3Path, err := parseS3Path(tt.targetPath, tt.relPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseS3Path() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if s3Path.bucketName != tt.expectedBucket {
					t.Errorf("parseS3Path() bucketName = %q, want %q", s3Path.bucketName, tt.expectedBucket)
				}
				if s3Path.objectKey != tt.expectedKey {
					t.Errorf("parseS3Path() objectKey = %q, want %q", s3Path.objectKey, tt.expectedKey)
				}
			}
		})
	}
}

func TestCreateSSHConfig(t *testing.T) {
	ftpConfig := config.FTPConfig{
		Username: "testuser",
		Password: "testpass",
	}

	sshConfig := createSSHConfig(ftpConfig)

	if sshConfig == nil {
		t.Fatal("createSSHConfig() returned nil")
	}
	if sshConfig.User != "testuser" {
		t.Errorf("SSH config user = %q, want %q", sshConfig.User, "testuser")
	}
	if len(sshConfig.Auth) == 0 {
		t.Error("SSH config should have auth methods")
	}
	if sshConfig.Timeout.Seconds() != 30 {
		t.Errorf("SSH config timeout = %v, want 30s", sshConfig.Timeout)
	}
}

// Tests für calculateFileChecksum
func TestFileHandler_calculateFileChecksum_Extended(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "checksum_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	fh := NewFileHandler(nil, NewS3ClientManager())

	tests := []struct {
		name        string
		content     string
		expectedLen int // Prüfe nur die Länge der Prüfsumme
		wantErr     bool
	}{
		{"empty file", "", 64, false}, // SHA256 ist immer 64 Zeichen
		{"small file", "Hello World", 64, false},
		{"large content", strings.Repeat("test content ", 1000), 64, false},
		{"special characters", "äöüßε", 64, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFile := filepath.Join(tempDir, "test.txt")
			if err := os.WriteFile(testFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			checksum, err := fh.calculateFileChecksum(testFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("calculateFileChecksum() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(checksum) != tt.expectedLen {
					t.Errorf("calculateFileChecksum() checksum length = %d, want %d", len(checksum), tt.expectedLen)
				}
				// Prüfe ob es sich um einen gültigen Hex-String handelt
				for _, c := range checksum {
					if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
						t.Errorf("calculateFileChecksum() returned invalid hex character: %c", c)
						break
					}
				}
			}

			// Cleanup
			os.Remove(testFile)
		})
	}

	// Test für nicht-existierende Datei
	t.Run("non-existent file", func(t *testing.T) {
		_, err := fh.calculateFileChecksum("/non/existent/file.txt")
		if err == nil {
			t.Error("calculateFileChecksum() should return error for non-existent file")
		}
	})

	// Test für konsistente Prüfsummen
	t.Run("consistent checksums", func(t *testing.T) {
		testFile := filepath.Join(tempDir, "consistent.txt")
		testContent := "This content should produce the same checksum"
		if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		checksum1, err1 := fh.calculateFileChecksum(testFile)
		checksum2, err2 := fh.calculateFileChecksum(testFile)

		if err1 != nil || err2 != nil {
			t.Errorf("calculateFileChecksum() errors: %v, %v", err1, err2)
			return
		}

		if checksum1 != checksum2 {
			t.Errorf("calculateFileChecksum() inconsistent results: %q vs %q", checksum1, checksum2)
		}
	})
}

// Tests für Delete-Funktionen
func TestFileHandler_deleteFromFilesystem_Extended(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "delete_fs_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	fh := NewFileHandler(nil, NewS3ClientManager())

	// Test erfolgreiches Löschen
	t.Run("successful delete", func(t *testing.T) {
		testFile := filepath.Join(tempDir, "delete_me.txt")
		if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		err := fh.deleteFromFilesystem("delete_me.txt", tempDir)
		if err != nil {
			t.Errorf("deleteFromFilesystem() error = %v", err)
		}

		// Verify file was deleted
		if _, err := os.Stat(testFile); !os.IsNotExist(err) {
			t.Error("File should have been deleted")
		}
	})

	// Test Löschen einer nicht-existierenden Datei (sollte kein Fehler sein)
	t.Run("delete non-existent file", func(t *testing.T) {
		err := fh.deleteFromFilesystem("non_existent.txt", tempDir)
		if err != nil {
			t.Errorf("deleteFromFilesystem() should not error for non-existent file, got: %v", err)
		}
	})

	// Test mit verschachtelten Pfaden
	t.Run("nested path delete", func(t *testing.T) {
		nestedDir := filepath.Join(tempDir, "nested", "deep")
		if err := os.MkdirAll(nestedDir, 0755); err != nil {
			t.Fatalf("Failed to create nested dir: %v", err)
		}

		nestedFile := filepath.Join(nestedDir, "nested_file.txt")
		if err := os.WriteFile(nestedFile, []byte("nested content"), 0644); err != nil {
			t.Fatalf("Failed to create nested file: %v", err)
		}

		err := fh.deleteFromFilesystem("nested/deep/nested_file.txt", tempDir)
		if err != nil {
			t.Errorf("deleteFromFilesystem() error = %v", err)
		}

		if _, err := os.Stat(nestedFile); !os.IsNotExist(err) {
			t.Error("Nested file should have been deleted")
		}
	})
}

// Tests für cleanupTargetFiles
func TestFileHandler_cleanupTargetFiles_Extended(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cleanup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Setup: Create test files in multiple targets
	output1Dir := filepath.Join(tempDir, "output1")
	output2Dir := filepath.Join(tempDir, "output2")

	for _, dir := range []string{output1Dir, output2Dir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create output dir: %v", err)
		}
	}

	testFile1 := filepath.Join(output1Dir, "test.txt")
	testFile2 := filepath.Join(output2Dir, "test.txt")

	for _, file := range []string{testFile1, testFile2} {
		if err := os.WriteFile(file, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Create FileHandler with filesystem targets
	targets := []config.OutputTarget{
		{Path: output1Dir, Type: "filesystem"},
		{Path: output2Dir, Type: "filesystem"},
	}
	fh := NewFileHandler(targets, NewS3ClientManager())

	// Test cleanup
	err = fh.cleanupTargetFiles("test.txt")
	if err != nil {
		t.Errorf("cleanupTargetFiles() error = %v", err)
	}

	// Verify both files were deleted
	for _, file := range []string{testFile1, testFile2} {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Errorf("File %s should have been deleted", file)
		}
	}
}

// Tests für ProcessFile Edge-Cases
func TestFileHandler_ProcessFile_UnknownTargetType_Extended(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "unknown_target_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	inputDir := filepath.Join(tempDir, "input")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input dir: %v", err)
	}

	testFile := filepath.Join(inputDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create target with unknown type
	targets := []config.OutputTarget{
		{Path: "/tmp/output", Type: "unknown-type"},
	}
	fh := NewFileHandler(targets, NewS3ClientManager())

	err = fh.ProcessFile(testFile, inputDir)
	if err == nil {
		t.Error("ProcessFile() should return error for unknown target type")
	}

	// Verify original file still exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("Original file should be preserved when processing fails")
	}

	// Check error message
	if !strings.Contains(err.Error(), "unbekannter Zieltyp") {
		t.Errorf("Error message should mention unknown target type: %v", err)
	}
}

// Tests für copyTo* URL-Parsing (ohne echte Netzwerkverbindungen)
func TestFileHandler_copyToFTP_URLParsing_Extended(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ftp_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	fh := NewFileHandler(nil, NewS3ClientManager())

	tests := []struct {
		name    string
		target  config.OutputTarget
		wantErr bool
	}{
		{
			name: "invalid ftp url",
			target: config.OutputTarget{
				Path: "not-a-valid-url",
				Type: "ftp",
			},
			wantErr: true,
		},
		{
			name: "valid ftp url but connection fails",
			target: config.OutputTarget{
				Path: "ftp://nonexistent-server.example.com/path",
				Type: "ftp",
			},
			wantErr: true, // Connection will fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fh.copyToFTP(testFile, "test.txt", tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("copyToFTP() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFileHandler_copyToSFTP_URLParsing_Extended(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sftp_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	fh := NewFileHandler(nil, NewS3ClientManager())

	tests := []struct {
		name    string
		target  config.OutputTarget
		wantErr bool
	}{
		{
			name: "invalid sftp url",
			target: config.OutputTarget{
				Path: "invalid-url",
				Type: "sftp",
			},
			wantErr: true,
		},
		{
			name: "valid sftp url but connection fails",
			target: config.OutputTarget{
				Path: "sftp://nonexistent-server.example.com/uploads",
				Type: "sftp",
			},
			wantErr: true, // Connection will fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fh.copyToSFTP(testFile, "test.txt", tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("copyToSFTP() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test für S3 ohne echte S3-Verbindung (nur Struktur-Tests)
func TestFileHandler_copyToS3_Structure_Extended(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "s3_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("s3 test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test ohne S3ClientManager
	fh := NewFileHandler(nil, nil)
	target := config.OutputTarget{
		Path: "s3://test-bucket/uploads",
		Type: "s3",
	}

	err = fh.copyToS3(testFile, "test.txt", target)
	if err == nil {
		t.Error("copyToS3() should return error when S3ClientManager is nil")
	}

	if !strings.Contains(err.Error(), "s3ClientManager nicht initialisiert") {
		t.Errorf("Error should mention S3ClientManager not initialized: %v", err)
	}
}

// Zusätzliche Edge-Case Tests
func TestFileHandler_ProcessFile_EmptyTargets_Extended(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "empty_targets_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	inputDir := filepath.Join(tempDir, "input")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input dir: %v", err)
	}

	testFile := filepath.Join(inputDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// FileHandler ohne Targets
	fh := NewFileHandler([]config.OutputTarget{}, NewS3ClientManager())

	err = fh.ProcessFile(testFile, inputDir)
	if err != nil {
		t.Errorf("ProcessFile() with empty targets should succeed (no transfers to do): %v", err)
	}

	// Original file should be removed (no failed transfers)
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("Original file should be removed when no targets are configured")
	}
}

// Tests für die bestehenden Funktionen (aus der ursprünglichen Testdatei)
func TestNewFileHandler(t *testing.T) {
	targets := []config.OutputTarget{
		{Path: "./output1", Type: "filesystem"},
		{Path: "./output2", Type: "filesystem"},
	}
	s3Manager := NewS3ClientManager()

	fh := NewFileHandler(targets, s3Manager)

	if fh == nil {
		t.Fatal("NewFileHandler() returned nil")
	}
	if fh.S3ClientManager != s3Manager {
		t.Error("S3ClientManager not set correctly")
	}
	if len(fh.OutputTargets) != len(targets) {
		t.Errorf("OutputTargets length = %d, want %d", len(fh.OutputTargets), len(targets))
	}
	if fh.OutputTargets[0].Path != targets[0].Path {
		t.Errorf("OutputTargets[0].Path = %q, want %q", fh.OutputTargets[0].Path, targets[0].Path)
	}
}

func TestFileHandler_copyToFilesystem(t *testing.T) {
	// Create temporary directories for testing
	tempDir, err := os.MkdirTemp("", "filehandler_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test source file
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}

	srcFile := filepath.Join(srcDir, "testfile.txt")
	testContent := "This is a test file"
	if err := os.WriteFile(srcFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Get file info
	fileInfo, err := os.Stat(srcFile)
	if err != nil {
		t.Fatalf("Failed to get file info: %v", err)
	}

	// Create target directory
	targetDir := filepath.Join(tempDir, "target")

	// Create FileHandler
	targets := []config.OutputTarget{{Path: targetDir, Type: "filesystem"}}
	fh := NewFileHandler(targets, NewS3ClientManager())

	tests := []struct {
		name           string
		relPath        string
		expectedTarget string
		wantErr        bool
	}{
		{
			name:           "simple file copy",
			relPath:        "testfile.txt",
			expectedTarget: filepath.Join(targetDir, "testfile.txt"),
			wantErr:        false,
		},
		{
			name:           "nested directory copy",
			relPath:        "subdir/testfile.txt",
			expectedTarget: filepath.Join(targetDir, "subdir/testfile.txt"),
			wantErr:        false,
		},
		{
			name:           "deep nested path",
			relPath:        "a/b/c/d/testfile.txt",
			expectedTarget: filepath.Join(targetDir, "a/b/c/d/testfile.txt"),
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean target directory for each test
			os.RemoveAll(targetDir)

			err := fh.copyToFilesystem(srcFile, tt.relPath, targetDir, fileInfo)

			if (err != nil) != tt.wantErr {
				t.Errorf("copyToFilesystem() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify file was created
				if _, err := os.Stat(tt.expectedTarget); os.IsNotExist(err) {
					t.Errorf("Target file %s was not created", tt.expectedTarget)
					return
				}

				// Verify content
				content, err := os.ReadFile(tt.expectedTarget)
				if err != nil {
					t.Errorf("Failed to read target file: %v", err)
					return
				}
				if string(content) != testContent {
					t.Errorf("Content mismatch: got %q, want %q", string(content), testContent)
				}

				// Verify file permissions are preserved (approximately)
				targetInfo, err := os.Stat(tt.expectedTarget)
				if err != nil {
					t.Errorf("Failed to get target file info: %v", err)
					return
				}

				// Check that the file mode is reasonable (not exact due to umask)
				if targetInfo.Mode()&0600 == 0 {
					t.Errorf("Target file has no read/write permissions for owner")
				}
			}
		})
	}
}

func TestFileHandler_ProcessFile_FilesystemOnly(t *testing.T) {
	// Create temporary directories for testing
	tempDir, err := os.MkdirTemp("", "process_file_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create input directory structure
	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")

	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input dir: %v", err)
	}

	// Create test file in input directory
	testFile := filepath.Join(inputDir, "testfile.txt")
	testContent := "This is a test file for processing"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create FileHandler with filesystem target
	targets := []config.OutputTarget{{Path: outputDir, Type: "filesystem"}}
	fh := NewFileHandler(targets, NewS3ClientManager())

	// Process the file
	err = fh.ProcessFile(testFile, inputDir)
	if err != nil {
		t.Errorf("ProcessFile() error = %v", err)
		return
	}

	// Verify original file was removed
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("Original file should have been removed after successful processing")
	}

	// Verify file was copied to output
	outputFile := filepath.Join(outputDir, "testfile.txt")
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("File was not copied to output directory")
		return
	}

	// Verify content
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Errorf("Failed to read output file: %v", err)
		return
	}
	if string(content) != testContent {
		t.Errorf("Content mismatch: got %q, want %q", string(content), testContent)
	}
}

// Benchmark tests
func BenchmarkFileHandler_calculateFileChecksum(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			tempDir, err := os.MkdirTemp("", "checksum_bench_test")
			if err != nil {
				b.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			testFile := filepath.Join(tempDir, "bench.txt")
			testContent := strings.Repeat("A", size.size)
			if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
				b.Fatalf("Failed to create test file: %v", err)
			}

			fh := NewFileHandler(nil, NewS3ClientManager())

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := fh.calculateFileChecksum(testFile)
				if err != nil {
					b.Fatalf("calculateFileChecksum failed: %v", err)
				}
			}
		})
	}
}

func BenchmarkNormalizeRemotePath(b *testing.B) {
	testPaths := []string{
		"simple/path",
		"path\\with\\backslashes",
		"mixed/path\\with/both",
		"very/long/path/with/many/segments/and\\mixed\\slashes/throughout",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, path := range testPaths {
			normalizeRemotePath(path)
		}
	}
}

// Zusätzliche Tests für bessere Coverage

func TestFileHandler_copyToS3_MoreCoverage(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "s3_copy_test_*")
	defer cleanup()

	// Erstelle Testdatei
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content for S3"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Fehler beim Erstellen der Testdatei: %v", err)
	}

	s3Manager := NewS3ClientManager()
	defer s3Manager.Close()

	targets := []config.OutputTarget{
		{
			Type:      "s3",
			Path:      "s3://test-bucket/path/",
			AccessKey: "test-key",
			SecretKey: "test-secret",
			Endpoint:  "localhost:9000",
		},
	}

	fh := NewFileHandler(targets, s3Manager)

	tests := []struct {
		name      string
		target    config.OutputTarget
		expectErr bool
	}{
		{
			name: "S3 target with valid config",
			target: config.OutputTarget{
				Type:      "s3",
				Path:      "s3://bucket/prefix/",
				AccessKey: "key",
				SecretKey: "secret",
				Endpoint:  "localhost:9000",
			},
			expectErr: true, // Wird fehlschlagen da kein echter S3 Server
		},
		{
			name: "S3 target with invalid path",
			target: config.OutputTarget{
				Type:      "s3",
				Path:      "invalid-s3-path",
				AccessKey: "key",
				SecretKey: "secret",
				Endpoint:  "localhost:9000",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fh.copyToS3(testFile, tempDir, tt.target)

			if tt.expectErr && err == nil {
				t.Error("Erwartete einen Fehler, aber bekam keinen")
			}

			if !tt.expectErr && err != nil {
				t.Errorf("Unerwarteter Fehler: %v", err)
			}
		})
	}
}

func TestFileHandler_deleteFromS3_Coverage(t *testing.T) {
	s3Manager := NewS3ClientManager()
	defer s3Manager.Close()

	targets := []config.OutputTarget{
		{
			Type:      "s3",
			Path:      "s3://test-bucket/path/",
			AccessKey: "test-key",
			SecretKey: "test-secret",
			Endpoint:  "localhost:9000",
		},
	}

	fh := NewFileHandler(targets, s3Manager)

	tests := []struct {
		name      string
		target    config.OutputTarget
		fileName  string
		expectErr bool
	}{
		{
			name: "Delete from S3 with valid config",
			target: config.OutputTarget{
				Type:      "s3",
				Path:      "s3://bucket/prefix/",
				AccessKey: "key",
				SecretKey: "secret",
				Endpoint:  "localhost:9000",
			},
			fileName:  "test.txt",
			expectErr: true, // Wird fehlschlagen da kein echter S3 Server
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fh.deleteFromS3(tt.fileName, tt.target)

			if tt.expectErr && err == nil {
				t.Error("Erwartete einen Fehler, aber bekam keinen")
			}

			if !tt.expectErr && err != nil {
				t.Errorf("Unerwarteter Fehler: %v", err)
			}
		})
	}
}

func TestFileHandler_deleteFromFTP_Coverage(t *testing.T) {
	s3Manager := NewS3ClientManager()
	defer s3Manager.Close()

	targets := []config.OutputTarget{
		{
			Type:     "ftp",
			Path:     "ftp://test.example.com/path/",
			Username: "testuser",
			Password: "testpass",
		},
	}

	fh := NewFileHandler(targets, s3Manager)

	tests := []struct {
		name      string
		target    config.OutputTarget
		fileName  string
		expectErr bool
	}{
		{
			name: "Delete from FTP",
			target: config.OutputTarget{
				Type:     "ftp",
				Path:     "ftp://localhost/path/",
				Username: "user",
				Password: "pass",
			},
			fileName:  "test.txt",
			expectErr: true, // Wird fehlschlagen da kein FTP Server
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fh.deleteFromFTP(tt.fileName, tt.target)

			if tt.expectErr && err == nil {
				t.Error("Erwartete einen Fehler, aber bekam keinen")
			}

			if !tt.expectErr && err != nil {
				t.Errorf("Unerwarteter Fehler: %v", err)
			}
		})
	}
}

func TestFileHandler_deleteFromSFTP_Coverage(t *testing.T) {
	s3Manager := NewS3ClientManager()
	defer s3Manager.Close()

	targets := []config.OutputTarget{
		{
			Type:     "sftp",
			Path:     "sftp://test.example.com/path/",
			Username: "testuser",
			Password: "testpass",
		},
	}

	fh := NewFileHandler(targets, s3Manager)

	tests := []struct {
		name      string
		target    config.OutputTarget
		fileName  string
		expectErr bool
	}{
		{
			name: "Delete from SFTP",
			target: config.OutputTarget{
				Type:     "sftp",
				Path:     "sftp://localhost/path/",
				Username: "user",
				Password: "pass",
			},
			fileName:  "test.txt",
			expectErr: true, // Wird fehlschlagen da kein SFTP Server
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fh.deleteFromSFTP(tt.fileName, tt.target)

			if tt.expectErr && err == nil {
				t.Error("Erwartete einen Fehler, aber bekam keinen")
			}

			if !tt.expectErr && err != nil {
				t.Errorf("Unerwarteter Fehler: %v", err)
			}
		})
	}
}

func TestFileHandler_copyToSFTPClient_Coverage(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "sftp_copy_test_*")
	defer cleanup()

	// Erstelle Testdatei
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content for SFTP"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Fehler beim Erstellen der Testdatei: %v", err)
	}

	s3Manager := NewS3ClientManager()
	defer s3Manager.Close()

	targets := []config.OutputTarget{{Type: "filesystem", Path: "/tmp"}}
	fh := NewFileHandler(targets, s3Manager)

	// Test mit ungültigen Parametern
	target := config.OutputTarget{
		Type:     "sftp",
		Path:     "sftp://localhost/path/",
		Username: "user",
		Password: "pass",
	}

	err = fh.copyToSFTPClient(testFile, "/remote/path/test.txt", "localhost:22", target)
	if err == nil {
		t.Error("Erwartete einen Fehler bei SFTP Verbindung zu nicht existierendem Server")
	}
}

func TestFileHandler_copyToFTPRegular_Coverage(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "ftp_copy_test_*")
	defer cleanup()

	// Erstelle Testdatei
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content for FTP"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Fehler beim Erstellen der Testdatei: %v", err)
	}

	s3Manager := NewS3ClientManager()
	defer s3Manager.Close()

	targets := []config.OutputTarget{{Type: "filesystem", Path: "/tmp"}}
	fh := NewFileHandler(targets, s3Manager)

	// Test mit ungültigen Parametern
	target := config.OutputTarget{
		Type:     "ftp",
		Path:     "ftp://localhost/path/",
		Username: "user",
		Password: "pass",
	}

	err = fh.copyToFTPRegular(testFile, "/remote/path/test.txt", "localhost:21", target)
	if err == nil {
		t.Error("Erwartete einen Fehler bei FTP Verbindung zu nicht existierendem Server")
	}
}

func TestFileHandler_ProcessFile_MultipleTargets(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "process_multi_test_*")
	defer cleanup()

	// Erstelle Input- und Output-Verzeichnisse
	inputDir := filepath.Join(tempDir, "input")
	outputDir1 := filepath.Join(tempDir, "output1")
	outputDir2 := filepath.Join(tempDir, "output2")

	for _, dir := range []string{inputDir, outputDir1, outputDir2} {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("Fehler beim Erstellen des Verzeichnisses %s: %v", dir, err)
		}
	}

	// Erstelle Testdatei
	testFile := filepath.Join(inputDir, "test.txt")
	testContent := "test content for multiple targets"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Fehler beim Erstellen der Testdatei: %v", err)
	}

	s3Manager := NewS3ClientManager()
	defer s3Manager.Close()

	// Mehrere Targets definieren
	targets := []config.OutputTarget{
		{Type: "filesystem", Path: outputDir1},
		{Type: "filesystem", Path: outputDir2},
	}

	fh := NewFileHandler(targets, s3Manager)

	// Verarbeite die Datei
	err = fh.ProcessFile(testFile, inputDir)
	if err != nil {
		t.Errorf("ProcessFile sollte nicht fehlschlagen: %v", err)
	}

	// Überprüfe ob Datei in beide Zielverzeichnisse kopiert wurde
	for i, target := range targets {
		expectedPath := filepath.Join(target.Path, "test.txt")
		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			t.Errorf("Datei sollte in Target %d vorhanden sein: %s", i+1, expectedPath)
		} else {
			// Überprüfe Inhalt
			content, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Errorf("Fehler beim Lesen der kopierten Datei: %v", err)
			} else if string(content) != testContent {
				t.Errorf("Dateiinhalt stimmt nicht überein in Target %d", i+1)
			}
		}
	}
}
