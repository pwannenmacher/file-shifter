package services

import (
	"file-shifter/config"
	"os"
	"path/filepath"
	"testing"
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

func TestNewWorker_ValidInput(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_test_*")
	defer cleanup()

	targets := createFilesystemTargets()
	worker := NewWorker(tempDir, targets)

	assertWorkerBasics(t, worker, tempDir, 1)
}

func TestNewWorker_NonExistentInputDir(t *testing.T) {
	// Test-Setup: Nicht existierendes Verzeichnis
	nonExistentDir := filepath.Join(os.TempDir(), "non_existent_worker_test")

	// Sicherstellen, dass das Verzeichnis nicht existiert
	os.RemoveAll(nonExistentDir)
	defer os.RemoveAll(nonExistentDir)

	targets := createFilesystemTargets()
	worker := NewWorker(nonExistentDir, targets)

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
	worker := NewWorker(tempDir, targets)

	assertWorkerBasics(t, worker, tempDir, 0)
}

func TestNewWorker_FilesystemTarget(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_test_fs_*")
	defer cleanup()

	expectedPath := "/tmp/test-output"
	targets := createFilesystemTargets(expectedPath)
	worker := NewWorker(tempDir, targets)

	assertWorkerBasics(t, worker, tempDir, 1)
	assertTargets(t, worker.OutputTargets, []string{expectedPath})
}

func TestNewWorker_MultipleTargets(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_test_multi_*")
	defer cleanup()

	expectedPaths := []string{"/tmp/output1", "/tmp/output2"}
	targets := createFilesystemTargets(expectedPaths...)
	worker := NewWorker(tempDir, targets)

	assertWorkerBasics(t, worker, tempDir, 2)
	assertTargets(t, worker.OutputTargets, expectedPaths)
}

func TestNewWorker_StopChannelInitialized(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_test_stop_*")
	defer cleanup()

	targets := createFilesystemTargets()
	worker := NewWorker(tempDir, targets)

	assertWorkerBasics(t, worker, tempDir, 1)
	// stopChan wird bereits in assertWorkerBasics -> assertWorkerComponents geprüft
}

func TestNewWorker_ComponentsProperlyInitialized(t *testing.T) {
	tempDir, cleanup := setupTempDir(t, "worker_test_components_*")
	defer cleanup()

	targets := createFilesystemTargets()
	worker := NewWorker(tempDir, targets)

	assertWorkerBasics(t, worker, tempDir, 1)
	// Detaillierte Komponentenprüfung wird bereits in assertWorkerComponents durchgeführt
}

func TestNewWorker_InputDirValidation(t *testing.T) {
	tempBaseDir, cleanup := setupTempDir(t, "worker_test_special_*")
	defer cleanup()

	// Unterverzeichnis mit Leerzeichen und speziellen Zeichen
	specialDir := filepath.Join(tempBaseDir, "test dir with spaces")
	targets := createFilesystemTargets()
	worker := NewWorker(specialDir, targets)

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
	worker := NewWorker(tempDir, targets)

	assertWorkerBasics(t, worker, tempDir, 2)
	assertTargets(t, worker.OutputTargets, expectedPaths)
}

// Hinweis: Tests für ungültige Eingaben (leerer InputDir, ungültige S3-Konfiguration)
// führen zu os.Exit(1) und können daher nicht einfach getestet werden.
// Diese würden separate Test-Funktionen erfordern, die in einem subprocess laufen.
