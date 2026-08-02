package services

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"file-shifter/config"
)

// TestFileHandler_ProcessFile_BoundedRetryOnChecksumMismatch verifies the
// bounded retry loop in ProcessFile: a file that keeps changing during
// processing triggers a checksum mismatch on every attempt, the targets are
// cleaned up, and after the maximum number of attempts ProcessFile gives up
// with an error while the source file is retained.
func TestFileHandler_ProcessFile_BoundedRetryOnChecksumMismatch(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	srcFile := filepath.Join(inputDir, "mutating.txt")
	if err := os.WriteFile(srcFile, make([]byte, 128), 0o644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Continuously overwrite the first bytes of the file with a counter while
	// keeping the file size constant. Every ProcessFile attempt therefore sees
	// different content between its initial and final checksum, while the
	// filesystem transfer itself succeeds (written bytes == file size).
	stop := make(chan struct{})
	var writers sync.WaitGroup
	writers.Add(1)
	go func() {
		defer writers.Done()
		f, err := os.OpenFile(srcFile, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		defer f.Close()
		buf := make([]byte, 8)
		for counter := uint64(0); ; counter++ {
			select {
			case <-stop:
				return
			default:
			}
			binary.LittleEndian.PutUint64(buf, counter)
			if _, err := f.WriteAt(buf, 0); err != nil {
				return
			}
		}
	}()

	fh := NewFileHandler([]config.OutputTarget{{Type: "filesystem", Path: outputDir}}, nil)
	err := fh.ProcessFile(srcFile, inputDir)

	close(stop)
	writers.Wait()

	if err == nil {
		t.Fatal("expected checksum mismatch error after bounded retries, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch persists after 5 attempts") {
		t.Fatalf("expected bounded-retry checksum mismatch error, got: %v", err)
	}

	// The source file must never be deleted when the checksum mismatch persists.
	if _, statErr := os.Stat(srcFile); statErr != nil {
		t.Fatalf("expected source file to be retained after failed retries: %v", statErr)
	}

	// The partially transferred target file must be cleaned up after the final mismatch.
	if _, statErr := os.Stat(filepath.Join(outputDir, "mutating.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("expected target file to be cleaned up after final mismatch, stat err: %v", statErr)
	}
}

// TestFileHandler_CopyToFilesystem_CreateFileError covers the os.Create error
// branch: the target directory exists but is not writable.
func TestFileHandler_CopyToFilesystem_CreateFileError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root - directory permissions are not enforced")
	}

	tempDir := t.TempDir()
	srcFile := filepath.Join(tempDir, "src.txt")
	if err := os.WriteFile(srcFile, []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}
	fileInfo, err := os.Stat(srcFile)
	if err != nil {
		t.Fatalf("failed to stat source file: %v", err)
	}

	readOnlyDir := filepath.Join(tempDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0o755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}
	if err := os.Chmod(readOnlyDir, 0o500); err != nil {
		t.Fatalf("failed to chmod target dir: %v", err)
	}
	t.Cleanup(func() {
		// Restore permissions so t.TempDir cleanup can remove the directory.
		_ = os.Chmod(readOnlyDir, 0o755)
	})

	fh := NewFileHandler(nil, nil)
	err = fh.copyToFilesystem(srcFile, "src.txt", readOnlyDir, fileInfo)
	if err == nil {
		t.Fatal("expected error when creating a file in a read-only directory")
	}
	if !strings.Contains(err.Error(), "error creating target file") {
		t.Fatalf("expected target file creation error, got: %v", err)
	}
}

// TestFileHandler_CopyToFilesystem_MkdirAllError covers the os.MkdirAll error
// branch: a regular file blocks the creation of the target directory tree.
func TestFileHandler_CopyToFilesystem_MkdirAllError(t *testing.T) {
	tempDir := t.TempDir()
	srcFile := filepath.Join(tempDir, "src.txt")
	if err := os.WriteFile(srcFile, []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}
	fileInfo, err := os.Stat(srcFile)
	if err != nil {
		t.Fatalf("failed to stat source file: %v", err)
	}

	targetBase := filepath.Join(tempDir, "target")
	if err := os.MkdirAll(targetBase, 0o755); err != nil {
		t.Fatalf("failed to create target base dir: %v", err)
	}
	blocker := filepath.Join(targetBase, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("failed to create blocking file: %v", err)
	}

	fh := NewFileHandler(nil, nil)
	// targetDir becomes targetBase/blocker/sub which cannot be created because
	// "blocker" is a regular file.
	err = fh.copyToFilesystem(srcFile, filepath.Join("blocker", "sub", "src.txt"), targetBase, fileInfo)
	if err == nil {
		t.Fatal("expected error when a file blocks target directory creation")
	}
	if !strings.Contains(err.Error(), "error creating the target directory") {
		t.Fatalf("expected target directory creation error, got: %v", err)
	}
}

// TestFileHandler_CopyToFilesystem_SourceOpenError covers the os.Open error
// branch for a missing source file.
func TestFileHandler_CopyToFilesystem_SourceOpenError(t *testing.T) {
	tempDir := t.TempDir()
	srcFile := filepath.Join(tempDir, "exists.txt")
	if err := os.WriteFile(srcFile, []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}
	fileInfo, err := os.Stat(srcFile)
	if err != nil {
		t.Fatalf("failed to stat source file: %v", err)
	}

	fh := NewFileHandler(nil, nil)
	err = fh.copyToFilesystem(filepath.Join(tempDir, "missing.txt"), "missing.txt", filepath.Join(tempDir, "out"), fileInfo)
	if err == nil {
		t.Fatal("expected error when the source file does not exist")
	}
	if !strings.Contains(err.Error(), "error opening source file") {
		t.Fatalf("expected source open error, got: %v", err)
	}
}

// TestFileHandler_CopyToFilesystem_IncompleteCopy covers the written-bytes
// verification: a stale FileInfo (from a larger file) makes the size check
// fail after an otherwise successful copy.
func TestFileHandler_CopyToFilesystem_IncompleteCopy(t *testing.T) {
	tempDir := t.TempDir()

	smallFile := filepath.Join(tempDir, "small.txt")
	if err := os.WriteFile(smallFile, []byte("abc"), 0o644); err != nil {
		t.Fatalf("failed to create small file: %v", err)
	}
	largeFile := filepath.Join(tempDir, "large.txt")
	if err := os.WriteFile(largeFile, []byte("much longer content"), 0o644); err != nil {
		t.Fatalf("failed to create large file: %v", err)
	}
	staleInfo, err := os.Stat(largeFile)
	if err != nil {
		t.Fatalf("failed to stat large file: %v", err)
	}

	fh := NewFileHandler(nil, nil)
	err = fh.copyToFilesystem(smallFile, "small.txt", filepath.Join(tempDir, "out"), staleInfo)
	if err == nil {
		t.Fatal("expected incomplete copy error for size mismatch")
	}
	if !strings.Contains(err.Error(), "incomplete copy") {
		t.Fatalf("expected incomplete copy error, got: %v", err)
	}
}

// TestFileHandler_CopyToS3_InvalidPath covers the parseS3Path error branch in
// copyToS3 with a functioning (offline) S3 client manager.
func TestFileHandler_CopyToS3_InvalidPath(t *testing.T) {
	tempDir := t.TempDir()
	srcFile := filepath.Join(tempDir, "src.txt")
	if err := os.WriteFile(srcFile, []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	s3Manager := NewS3ClientManager()
	defer s3Manager.Close()

	fh := NewFileHandler(nil, s3Manager)
	target := config.OutputTarget{
		Type:      "s3",
		Path:      "://invalid-s3-url",
		Endpoint:  "localhost:9000",
		AccessKey: "key",
		SecretKey: "secret",
		Region:    "us-east-1",
	}

	// Pre-populate the client cache so GetOrCreateClient succeeds without a
	// network health check and copyToS3 proceeds to path parsing.
	cachedClient, err := NewMinIOConnection("localhost:9000", "key", "secret", true)
	if err != nil {
		t.Fatalf("failed to create offline MinIO client: %v", err)
	}
	s3Manager.clients[s3Manager.getClientKey(target.GetS3Config())] = cachedClient

	err = fh.copyToS3(srcFile, "src.txt", target)
	if err == nil {
		t.Fatal("expected error for unparsable S3 path")
	}
	if !strings.Contains(err.Error(), "error parsing S3 path") {
		t.Fatalf("expected S3 path parse error, got: %v", err)
	}
}
