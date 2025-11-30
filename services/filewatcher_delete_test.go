package services

import (
	"file-shifter/config"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileWatcher_DirectoryDeletion(t *testing.T) {
	// Create test directories
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	// Create subdirectory in input
	subDir := filepath.Join(inputDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Create a file in the subdirectory
	testFile := filepath.Join(subDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg := &config.EnvConfig{}
	cfg.SetDefaults()

	outputTargets := []config.OutputTarget{
		{
			Path: outputDir,
			Type: "filesystem",
		},
	}

	fileHandler := NewFileHandler(outputTargets, NewS3ClientManager())
	fw, err := NewFileWatcher(inputDir, fileHandler, 30, 100*time.Millisecond, 200*time.Millisecond, 4, 100)
	if err != nil {
		t.Fatalf("Failed to create FileWatcher: %v", err)
	}

	// Start file watcher in background
	go func() {
		if err := fw.Start(); err != nil {
			t.Logf("FileWatcher stopped with error: %v", err)
		}
	}()
	defer fw.Stop()

	// Wait for watcher to initialize
	time.Sleep(500 * time.Millisecond)

	// Delete the subdirectory
	if err := os.RemoveAll(subDir); err != nil {
		t.Fatalf("Failed to delete subdirectory: %v", err)
	}

	// Wait for deletion event to be processed
	time.Sleep(500 * time.Millisecond)

	// Verify that the file watcher is still functional
	// Create a new file in the main input directory
	newFile := filepath.Join(inputDir, "newfile.txt")
	if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
		t.Fatalf("Failed to create new file: %v", err)
	}

	// Wait for file to be processed
	time.Sleep(1 * time.Second)

	// Check if new file was processed (should be deleted from input)
	if _, err := os.Stat(newFile); !os.IsNotExist(err) {
		t.Error("New file should have been processed and deleted, but still exists")
	}

	// Check if new file exists in output
	outputFile := filepath.Join(outputDir, "newfile.txt")
	if stat, err := os.Stat(outputFile); err != nil {
		if os.IsNotExist(err) {
			t.Error("New file should exist in output directory")
		} else {
			t.Errorf("Error checking output file: %v", err)
		}
	} else if stat.IsDir() {
		t.Error("Output file should be a file, not a directory")
	}
}

func TestFileWatcher_InputDirectoryDeletion(t *testing.T) {
	// Create test directories
	parentDir := t.TempDir()
	inputDir := filepath.Join(parentDir, "input")
	outputDir := t.TempDir()

	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	cfg := &config.EnvConfig{}
	cfg.SetDefaults()

	outputTargets := []config.OutputTarget{
		{
			Path: outputDir,
			Type: "filesystem",
		},
	}

	fileHandler := NewFileHandler(outputTargets, NewS3ClientManager())
	fw, err := NewFileWatcher(inputDir, fileHandler, 30, 100*time.Millisecond, 200*time.Millisecond, 4, 100)
	if err != nil {
		t.Fatalf("Failed to create FileWatcher: %v", err)
	}

	// Start file watcher in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- fw.Start()
	}()

	// Wait for watcher to initialize
	time.Sleep(500 * time.Millisecond)

	// Delete the entire input directory
	if err := os.RemoveAll(inputDir); err != nil {
		t.Fatalf("Failed to delete input directory: %v", err)
	}

	// Wait a bit to see if watcher handles this gracefully
	time.Sleep(1 * time.Second)

	// Stop the watcher
	fw.Stop()

	// Check if there was an error or if it handled gracefully
	select {
	case err := <-errChan:
		if err != nil {
			t.Logf("Watcher returned error after directory deletion: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Log("Watcher stopped gracefully")
	}
}

func TestFileWatcher_DirectoryRecreation(t *testing.T) {
	// Create test directories
	parentDir := t.TempDir()
	inputDir := filepath.Join(parentDir, "input")
	outputDir := t.TempDir()

	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("Failed to create input directory: %v", err)
	}

	cfg := &config.EnvConfig{}
	cfg.SetDefaults()

	outputTargets := []config.OutputTarget{
		{
			Path: outputDir,
			Type: "filesystem",
		},
	}

	fileHandler := NewFileHandler(outputTargets, NewS3ClientManager())
	fw, err := NewFileWatcher(inputDir, fileHandler, 30, 100*time.Millisecond, 200*time.Millisecond, 4, 100)
	if err != nil {
		t.Fatalf("Failed to create FileWatcher: %v", err)
	}

	// Start file watcher in background
	go func() {
		if err := fw.Start(); err != nil {
			t.Logf("FileWatcher stopped with error: %v", err)
		}
	}()
	defer fw.Stop()

	// Wait for watcher to initialize
	time.Sleep(500 * time.Millisecond)

	// Create a subdirectory
	subDir := filepath.Join(inputDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Delete the subdirectory
	if err := os.RemoveAll(subDir); err != nil {
		t.Fatalf("Failed to delete subdirectory: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Recreate the subdirectory
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to recreate subdirectory: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Create a file in the recreated subdirectory
	testFile := filepath.Join(subDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Wait for file to be processed
	time.Sleep(1 * time.Second)

	// Check if file was processed
	outputFile := filepath.Join(outputDir, "subdir", "test.txt")
	if stat, err := os.Stat(outputFile); err != nil {
		if os.IsNotExist(err) {
			t.Error("File in recreated directory should have been processed and exist in output")
		} else {
			t.Errorf("Error checking output file: %v", err)
		}
	} else if stat.IsDir() {
		t.Error("Output file should be a file, not a directory")
	}
}
