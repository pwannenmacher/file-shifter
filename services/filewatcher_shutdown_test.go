package services

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"file-shifter/config"
)

// shutdownFileExists reports whether a regular file exists at the given path.
func shutdownFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// TestFileWatcher_ShutdownRaceWithConcurrentProducers closes the TODO test
// gap "shutdown race resilience": Stop() is called while new files are still
// being produced concurrently. The watcher must neither panic nor deadlock,
// and per project principle no file may ever be lost - every produced file
// must end up either transferred to the output directory or still be present
// in the input directory.
func TestFileWatcher_ShutdownRaceWithConcurrentProducers(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	s3Manager := NewS3ClientManager()
	defer s3Manager.Close()

	targets := []config.OutputTarget{{Type: "filesystem", Path: outputDir}}
	fileHandler := NewFileHandler(targets, s3Manager)

	watcher, err := NewFileWatcher(inputDir, fileHandler, 3, 10*time.Millisecond, 20*time.Millisecond, 4, 32)
	if err != nil {
		t.Fatalf("failed to create file watcher: %v", err)
	}

	startErr := make(chan error, 1)
	go func() { startErr <- watcher.Start() }()

	// Wait until the pipeline is fully operational: a sentinel file must be
	// picked up and transferred before we begin the race.
	sentinel := filepath.Join(inputDir, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("sentinel content"), 0o644); err != nil {
		t.Fatalf("failed to create sentinel file: %v", err)
	}
	waitDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(waitDeadline) {
		if shutdownFileExists(filepath.Join(outputDir, "sentinel.txt")) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !shutdownFileExists(filepath.Join(outputDir, "sentinel.txt")) {
		t.Fatal("watcher did not process the sentinel file - pipeline not operational")
	}

	// Produce files continuously; once a third has been written, Stop() is
	// invoked concurrently while production continues.
	const totalFiles = 120
	const stopAfter = 40
	reachedStopPoint := make(chan struct{})
	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		for i := 0; i < totalFiles; i++ {
			path := filepath.Join(inputDir, fmt.Sprintf("race_%03d.txt", i))
			if err := os.WriteFile(path, []byte("shutdown race content"), 0o644); err != nil {
				t.Errorf("failed to write file %d: %v", i, err)
				return
			}
			if i == stopAfter {
				close(reachedStopPoint)
			}
		}
	}()

	<-reachedStopPoint
	stopped := make(chan struct{})
	go func() {
		watcher.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(30 * time.Second):
		t.Fatal("Stop() did not complete within 30s - possible shutdown deadlock")
	}
	<-producerDone

	// Start() must have returned cleanly after Stop().
	select {
	case err := <-startErr:
		if err != nil {
			t.Errorf("Start() returned an error after Stop(): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Start() did not return after Stop()")
	}

	// No-file-loss invariant: each produced file is either still in the input
	// directory (not yet transferred) or in the output directory (transferred).
	var lost []string
	checkFile := func(name string) {
		inInput := shutdownFileExists(filepath.Join(inputDir, name))
		inOutput := shutdownFileExists(filepath.Join(outputDir, name))
		if !inInput && !inOutput {
			lost = append(lost, name)
		}
	}
	checkFile("sentinel.txt")
	for i := 0; i < totalFiles; i++ {
		checkFile(fmt.Sprintf("race_%03d.txt", i))
	}
	if len(lost) > 0 {
		t.Fatalf("%d files were lost during the shutdown race (neither in input nor output): %v", len(lost), lost)
	}
}

// TestFileWatcher_StopWithoutStart verifies that Stop() on a watcher that was
// never started completes without panicking or hanging.
func TestFileWatcher_StopWithoutStart(t *testing.T) {
	inputDir := t.TempDir()

	s3Manager := NewS3ClientManager()
	defer s3Manager.Close()

	targets := []config.OutputTarget{{Type: "filesystem", Path: t.TempDir()}}
	fileHandler := NewFileHandler(targets, s3Manager)

	watcher, err := NewFileWatcher(inputDir, fileHandler, 3, 10*time.Millisecond, 20*time.Millisecond, 2, 8)
	if err != nil {
		t.Fatalf("failed to create file watcher: %v", err)
	}

	stopped := make(chan struct{})
	go func() {
		watcher.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop() without Start() did not complete - possible deadlock")
	}
}

// TestFileWatcher_StopIsIdempotent verifies that calling Stop() multiple
// times (also concurrently) is safe.
func TestFileWatcher_StopIsIdempotent(t *testing.T) {
	inputDir := t.TempDir()

	s3Manager := NewS3ClientManager()
	defer s3Manager.Close()

	targets := []config.OutputTarget{{Type: "filesystem", Path: t.TempDir()}}
	fileHandler := NewFileHandler(targets, s3Manager)

	watcher, err := NewFileWatcher(inputDir, fileHandler, 3, 10*time.Millisecond, 20*time.Millisecond, 2, 8)
	if err != nil {
		t.Fatalf("failed to create file watcher: %v", err)
	}

	done := make(chan struct{}, 3)
	for i := 0; i < 3; i++ {
		go func() {
			watcher.Stop()
			done <- struct{}{}
		}()
	}
	deadline := time.After(15 * time.Second)
	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-deadline:
			t.Fatal("concurrent Stop() calls did not all complete - possible deadlock")
		}
	}
}
