package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"file-shifter/config"
)

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

func TestFileHandler_copyToFilesystem_EdgeCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "filehandler_edge_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test source file
	srcFile := filepath.Join(tempDir, "testfile.txt")
	if err := os.WriteFile(srcFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	fileInfo, err := os.Stat(srcFile)
	if err != nil {
		t.Fatalf("Failed to get file info: %v", err)
	}

	fh := NewFileHandler(nil, NewS3ClientManager())

	tests := []struct {
		name       string
		srcPath    string
		relPath    string
		targetBase string
		fileInfo   os.FileInfo
		wantErr    bool
	}{
		{
			name:       "non-existent source file",
			srcPath:    "/non/existent/file.txt",
			relPath:    "file.txt",
			targetBase: tempDir,
			fileInfo:   fileInfo,
			wantErr:    true,
		},
		{
			name:       "invalid target path (read-only parent)",
			srcPath:    srcFile,
			relPath:    "file.txt",
			targetBase: "/root/readonly", // Assuming /root is not writable
			fileInfo:   fileInfo,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fh.copyToFilesystem(tt.srcPath, tt.relPath, tt.targetBase, tt.fileInfo)
			if (err != nil) != tt.wantErr {
				t.Errorf("copyToFilesystem() error = %v, wantErr %v", err, tt.wantErr)
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

func TestFileHandler_ProcessFile_MultipleTargets(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "multi_target_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create directories
	inputDir := filepath.Join(tempDir, "input")
	output1Dir := filepath.Join(tempDir, "output1")
	output2Dir := filepath.Join(tempDir, "output2")

	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input dir: %v", err)
	}

	// Create test file
	testFile := filepath.Join(inputDir, "multitest.txt")
	testContent := "Multi-target test file"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create FileHandler with multiple filesystem targets
	targets := []config.OutputTarget{
		{Path: output1Dir, Type: "filesystem"},
		{Path: output2Dir, Type: "filesystem"},
	}
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

	// Verify file was copied to both outputs
	output1File := filepath.Join(output1Dir, "multitest.txt")
	output2File := filepath.Join(output2Dir, "multitest.txt")

	for _, outputFile := range []string{output1File, output2File} {
		if _, err := os.Stat(outputFile); os.IsNotExist(err) {
			t.Errorf("File was not copied to %s", outputFile)
			continue
		}

		content, err := os.ReadFile(outputFile)
		if err != nil {
			t.Errorf("Failed to read output file %s: %v", outputFile, err)
			continue
		}
		if string(content) != testContent {
			t.Errorf("Content mismatch in %s: got %q, want %q", outputFile, string(content), testContent)
		}
	}
}

func TestFileHandler_ProcessFile_FailureHandling(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "failure_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	inputDir := filepath.Join(tempDir, "input")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input dir: %v", err)
	}

	// Create test file
	testFile := filepath.Join(inputDir, "failtest.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name    string
		targets []config.OutputTarget
		wantErr bool
	}{
		{
			name: "unknown target type",
			targets: []config.OutputTarget{
				{Path: "/tmp/output", Type: "unknown"},
			},
			wantErr: true,
		},
		{
			name: "mixed success and failure - all should fail",
			targets: []config.OutputTarget{
				{Path: filepath.Join(tempDir, "good"), Type: "filesystem"},
				{Path: "/root/readonly", Type: "filesystem"}, // Should fail
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Recreate test file for each test
			if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
				t.Fatalf("Failed to recreate test file: %v", err)
			}

			fh := NewFileHandler(tt.targets, NewS3ClientManager())
			err := fh.ProcessFile(testFile, inputDir)

			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessFile() error = %v, wantErr %v", err, tt.wantErr)
			}

			// If error expected, original file should still exist
			if tt.wantErr {
				if _, err := os.Stat(testFile); os.IsNotExist(err) {
					t.Error("Original file should be preserved when processing fails")
				}
			}
		})
	}
}

func TestFileHandler_ProcessFile_RelativePathHandling(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "relpath_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create nested input structure
	inputDir := filepath.Join(tempDir, "input")
	subDir := filepath.Join(inputDir, "subdir", "nested")
	outputDir := filepath.Join(tempDir, "output")

	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}

	// Create test file in nested directory
	testFile := filepath.Join(subDir, "nested_file.txt")
	testContent := "Nested file content"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create FileHandler
	targets := []config.OutputTarget{{Path: outputDir, Type: "filesystem"}}
	fh := NewFileHandler(targets, NewS3ClientManager())

	// Process the file
	err = fh.ProcessFile(testFile, inputDir)
	if err != nil {
		t.Errorf("ProcessFile() error = %v", err)
		return
	}

	// Verify file was copied with correct relative path
	expectedOutput := filepath.Join(outputDir, "subdir", "nested", "nested_file.txt")
	if _, err := os.Stat(expectedOutput); os.IsNotExist(err) {
		t.Error("File was not copied to correct relative path")
		return
	}

	// Verify content
	content, err := os.ReadFile(expectedOutput)
	if err != nil {
		t.Errorf("Failed to read output file: %v", err)
		return
	}
	if string(content) != testContent {
		t.Errorf("Content mismatch: got %q, want %q", string(content), testContent)
	}
}

// Test to verify error handling for non-existent files
func TestFileHandler_ProcessFile_NonExistentFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nonexistent_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input dir: %v", err)
	}

	nonExistentFile := filepath.Join(inputDir, "does_not_exist.txt")

	targets := []config.OutputTarget{{Path: outputDir, Type: "filesystem"}}
	fh := NewFileHandler(targets, NewS3ClientManager())

	err = fh.ProcessFile(nonExistentFile, inputDir)
	if err == nil {
		t.Error("ProcessFile() should return error for non-existent file")
	}

	// Check that error message contains relevant information
	if !strings.Contains(err.Error(), "datei-informationen") && !strings.Contains(err.Error(), "file") {
		t.Errorf("Error message should indicate file info problem: %v", err)
	}
}

// Benchmark tests
func BenchmarkFileHandler_copyToFilesystem(b *testing.B) {
	// Setup
	tempDir, err := os.MkdirTemp("", "bench_test")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	srcFile := filepath.Join(tempDir, "src.txt")
	testContent := strings.Repeat("This is benchmark test content. ", 1000) // ~32KB
	if err := os.WriteFile(srcFile, []byte(testContent), 0644); err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}

	fileInfo, _ := os.Stat(srcFile)
	targetDir := filepath.Join(tempDir, "target")

	fh := NewFileHandler(nil, NewS3ClientManager())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clean up from previous iteration
		os.RemoveAll(targetDir)

		err := fh.copyToFilesystem(srcFile, "src.txt", targetDir, fileInfo)
		if err != nil {
			b.Fatalf("copyToFilesystem failed: %v", err)
		}
	}
}

func BenchmarkFileHandler_ProcessFile(b *testing.B) {
	// Setup
	tempDir, err := os.MkdirTemp("", "process_bench_test")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		b.Fatalf("Failed to create input dir: %v", err)
	}

	testContent := strings.Repeat("Benchmark content ", 500) // ~16KB
	targets := []config.OutputTarget{{Path: outputDir, Type: "filesystem"}}
	fh := NewFileHandler(targets, NewS3ClientManager())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create fresh test file for each iteration
		testFile := filepath.Join(inputDir, "benchfile.txt")
		if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}

		err := fh.ProcessFile(testFile, inputDir)
		if err != nil {
			b.Fatalf("ProcessFile failed: %v", err)
		}

		// Clean up output for next iteration
		os.RemoveAll(outputDir)
	}
}
