package services

import (
	"file-shifter/config"
	"os"
	"path/filepath"
	"testing"
)

func TestNewWorker_ValidInput(t *testing.T) {
	// Test-Setup: Temporäres Verzeichnis erstellen
	tempDir, err := os.MkdirTemp("", "worker_test_*")
	if err != nil {
		t.Fatalf("Fehler beim Erstellen des temporären Verzeichnisses: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Valid targets definieren
	targets := []config.OutputTarget{
		{Type: "filesystem", Path: "/tmp/output"},
	}

	// NewWorker aufrufen
	worker := NewWorker(tempDir, targets)

	// Assertions
	if worker == nil {
		t.Fatal("Worker sollte nicht nil sein")
	}

	if worker.InputDir != tempDir {
		t.Errorf("InputDir stimmt nicht überein. Erwartet: %s, Bekommen: %s", tempDir, worker.InputDir)
	}

	if len(worker.OutputTargets) != 1 {
		t.Errorf("Anzahl der OutputTargets stimmt nicht überein. Erwartet: 1, Bekommen: %d", len(worker.OutputTargets))
	}

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

func TestNewWorker_NonExistentInputDir(t *testing.T) {
	// Test-Setup: Nicht existierendes Verzeichnis
	nonExistentDir := filepath.Join(os.TempDir(), "non_existent_worker_test")

	// Sicherstellen, dass das Verzeichnis nicht existiert
	os.RemoveAll(nonExistentDir)
	defer os.RemoveAll(nonExistentDir)

	targets := []config.OutputTarget{
		{Type: "filesystem", Path: "/tmp/output"},
	}

	// NewWorker aufrufen - sollte das Verzeichnis erstellen
	worker := NewWorker(nonExistentDir, targets)

	// Assertions
	if worker == nil {
		t.Fatal("Worker sollte nicht nil sein")
	}

	// Prüfen, dass das Verzeichnis erstellt wurde
	if _, err := os.Stat(nonExistentDir); os.IsNotExist(err) {
		t.Error("Input-Verzeichnis sollte erstellt worden sein")
	}

	if worker.InputDir != nonExistentDir {
		t.Errorf("InputDir stimmt nicht überein. Erwartet: %s, Bekommen: %s", nonExistentDir, worker.InputDir)
	}
}

func TestNewWorker_EmptyTargets(t *testing.T) {
	// Test-Setup: Temporäres Verzeichnis erstellen
	tempDir, err := os.MkdirTemp("", "worker_test_empty_*")
	if err != nil {
		t.Fatalf("Fehler beim Erstellen des temporären Verzeichnisses: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Leere targets
	var targets []config.OutputTarget

	// NewWorker aufrufen
	worker := NewWorker(tempDir, targets)

	// Assertions
	if worker == nil {
		t.Fatal("Worker sollte nicht nil sein")
	}

	if len(worker.OutputTargets) != 0 {
		t.Errorf("Anzahl der OutputTargets sollte 0 sein. Bekommen: %d", len(worker.OutputTargets))
	}

	// Andere Komponenten sollten trotzdem initialisiert sein
	if worker.S3ClientManager == nil {
		t.Error("S3ClientManager sollte nicht nil sein")
	}

	if worker.FileHandler == nil {
		t.Error("FileHandler sollte nicht nil sein")
	}
}

func TestNewWorker_FilesystemTarget(t *testing.T) {
	// Test-Setup: Temporäres Verzeichnis erstellen
	tempDir, err := os.MkdirTemp("", "worker_test_fs_*")
	if err != nil {
		t.Fatalf("Fehler beim Erstellen des temporären Verzeichnisses: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Filesystem target
	targets := []config.OutputTarget{
		{Type: "filesystem", Path: "/tmp/test-output"},
	}

	// NewWorker aufrufen
	worker := NewWorker(tempDir, targets)

	// Assertions
	if worker == nil {
		t.Fatal("Worker sollte nicht nil sein")
	}

	if len(worker.OutputTargets) != 1 {
		t.Fatalf("Erwartete 1 OutputTarget, bekommen: %d", len(worker.OutputTargets))
	}

	target := worker.OutputTargets[0]
	if target.Type != "filesystem" {
		t.Errorf("Target.Type stimmt nicht überein. Erwartet: filesystem, Bekommen: %s", target.Type)
	}

	if target.Path != "/tmp/test-output" {
		t.Errorf("Target.Path stimmt nicht überein. Erwartet: /tmp/test-output, Bekommen: %s", target.Path)
	}
}

func TestNewWorker_MultipleTargets(t *testing.T) {
	// Test-Setup: Temporäres Verzeichnis erstellen
	tempDir, err := os.MkdirTemp("", "worker_test_multi_*")
	if err != nil {
		t.Fatalf("Fehler beim Erstellen des temporären Verzeichnisses: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mehrere targets
	targets := []config.OutputTarget{
		{Type: "filesystem", Path: "/tmp/output1"},
		{Type: "filesystem", Path: "/tmp/output2"},
	}

	// NewWorker aufrufen
	worker := NewWorker(tempDir, targets)

	// Assertions
	if worker == nil {
		t.Fatal("Worker sollte nicht nil sein")
	}

	if len(worker.OutputTargets) != 2 {
		t.Errorf("Anzahl der OutputTargets stimmt nicht überein. Erwartet: 2, Bekommen: %d", len(worker.OutputTargets))
	}

	// Prüfen, dass beide Targets korrekt gesetzt sind
	expectedPaths := []string{"/tmp/output1", "/tmp/output2"}
	for i, target := range worker.OutputTargets {
		if target.Type != "filesystem" {
			t.Errorf("Target[%d].Type stimmt nicht überein. Erwartet: filesystem, Bekommen: %s", i, target.Type)
		}
		if target.Path != expectedPaths[i] {
			t.Errorf("Target[%d].Path stimmt nicht überein. Erwartet: %s, Bekommen: %s", i, expectedPaths[i], target.Path)
		}
	}
}

func TestNewWorker_StopChannelInitialized(t *testing.T) {
	// Test-Setup: Temporäres Verzeichnis erstellen
	tempDir, err := os.MkdirTemp("", "worker_test_stop_*")
	if err != nil {
		t.Fatalf("Fehler beim Erstellen des temporären Verzeichnisses: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targets := []config.OutputTarget{
		{Type: "filesystem", Path: "/tmp/output"},
	}

	// NewWorker aufrufen
	worker := NewWorker(tempDir, targets)

	// Prüfen, dass stopChan nicht nil ist
	if worker.stopChan == nil {
		t.Fatal("stopChan sollte nicht nil sein")
	}

	// Test: stopChan sollte den richtigen Typ haben
	// Da es ein unbepufferter Channel ist, können wir nur prüfen ob er existiert
	// Ein Sende-/Empfangstest würde blockieren
}

func TestNewWorker_ComponentsProperlyInitialized(t *testing.T) {
	// Test-Setup: Temporäres Verzeichnis erstellen
	tempDir, err := os.MkdirTemp("", "worker_test_components_*")
	if err != nil {
		t.Fatalf("Fehler beim Erstellen des temporären Verzeichnisses: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targets := []config.OutputTarget{
		{Type: "filesystem", Path: "/tmp/output"},
	}

	// NewWorker aufrufen
	worker := NewWorker(tempDir, targets)

	// Detaillierte Prüfung der Komponenten
	if worker.S3ClientManager == nil {
		t.Fatal("S3ClientManager sollte nicht nil sein")
	}

	if worker.FileHandler == nil {
		t.Fatal("FileHandler sollte nicht nil sein")
	}

	if worker.FileWatcher == nil {
		t.Fatal("FileWatcher sollte nicht nil sein")
	}

	// Prüfen, dass FileHandler die korrekten Targets hat
	// (Direkter Zugriff auf Felder ist schwierig, daher nur Nil-Check)
	if worker.FileHandler == nil {
		t.Error("FileHandler sollte mit Targets initialisiert sein")
	}
}

func TestNewWorker_InputDirValidation(t *testing.T) {
	// Test-Setup: Verzeichnis mit speziellen Zeichen
	tempBaseDir, err := os.MkdirTemp("", "worker_test_special_*")
	if err != nil {
		t.Fatalf("Fehler beim Erstellen des temporären Verzeichnisses: %v", err)
	}
	defer os.RemoveAll(tempBaseDir)

	// Unterverzeichnis mit Leerzeichen und speziellen Zeichen
	specialDir := filepath.Join(tempBaseDir, "test dir with spaces")

	targets := []config.OutputTarget{
		{Type: "filesystem", Path: "/tmp/output"},
	}

	// NewWorker aufrufen
	worker := NewWorker(specialDir, targets)

	// Assertions
	if worker == nil {
		t.Fatal("Worker sollte nicht nil sein")
	}

	if worker.InputDir != specialDir {
		t.Errorf("InputDir stimmt nicht überein. Erwartet: %s, Bekommen: %s", specialDir, worker.InputDir)
	}

	// Prüfen, dass das Verzeichnis erstellt wurde
	if _, err := os.Stat(specialDir); os.IsNotExist(err) {
		t.Error("Spezielles Input-Verzeichnis sollte erstellt worden sein")
	}
}

func TestNewWorker_DifferentTargetTypes(t *testing.T) {
	// Test-Setup: Temporäres Verzeichnis erstellen
	tempDir, err := os.MkdirTemp("", "worker_test_types_*")
	if err != nil {
		t.Fatalf("Fehler beim Erstellen des temporären Verzeichnisses: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Verschiedene Target-Typen (nur gültige, da ungültige zu os.Exit führen)
	targets := []config.OutputTarget{
		{Type: "filesystem", Path: "/tmp/output1"},
		{Type: "filesystem", Path: "/tmp/output2"},
	}

	// NewWorker aufrufen
	worker := NewWorker(tempDir, targets)

	// Assertions
	if worker == nil {
		t.Fatal("Worker sollte nicht nil sein")
	}

	if len(worker.OutputTargets) != 2 {
		t.Errorf("Anzahl der OutputTargets stimmt nicht überein. Erwartet: 2, Bekommen: %d", len(worker.OutputTargets))
	}

	// Prüfen, dass alle Targets den Typ "filesystem" haben
	for i, target := range worker.OutputTargets {
		if target.Type != "filesystem" {
			t.Errorf("Target[%d] hat falschen Typ. Erwartet: filesystem, Bekommen: %s", i, target.Type)
		}
	}
}

// Hinweis: Tests für ungültige Eingaben (leerer InputDir, ungültige S3-Konfiguration)
// führen zu os.Exit(1) und können daher nicht einfach getestet werden.
// Diese würden separate Test-Funktionen erfordern, die in einem subprocess laufen.
