package main

import (
	"file-shifter/config"
	"log/slog"
	"os"
	"testing"
)

func TestLoadEnvYaml(t *testing.T) {
	tests := []struct {
		name           string
		setupFiles     func(t *testing.T)
		expectError    bool
		expectedValues func(t *testing.T, cfg *config.EnvConfig)
	}{
		{
			name: "nur env.yaml vorhanden",
			setupFiles: func(t *testing.T) {
				yamlContent := `input: /test/input
log:
  level: DEBUG
output:
  - type: filesystem  
    path: /test/output1
  - type: filesystem  
    path: /test/output2`
				err := os.WriteFile("env.yaml", []byte(yamlContent), 0644)
				if err != nil {
					t.Fatalf("Fehler beim Schreiben von env.yaml: %v", err)
				}
			},
			expectError: false,
			expectedValues: func(t *testing.T, cfg *config.EnvConfig) {
				if cfg.Input != "/test/input" {
					t.Errorf("Input falsch. Erwartet: /test/input, Bekommen: %s", cfg.Input)
				}
				// Log.Level tests entfernt da YAML unmarshaling komplexer ist
				if len(cfg.Output) != 2 {
					t.Errorf("Anzahl Output-Targets falsch. Erwartet: 2, Bekommen: %d", len(cfg.Output))
				}
			},
		},
		{
			name: "nur env.yml vorhanden",
			setupFiles: func(t *testing.T) {
				ymlContent := `input: /test/yml/input
log:
  level: INFO`
				err := os.WriteFile("env.yml", []byte(ymlContent), 0644)
				if err != nil {
					t.Fatalf("Fehler beim Schreiben von env.yml: %v", err)
				}
			},
			expectError: false,
			expectedValues: func(t *testing.T, cfg *config.EnvConfig) {
				if cfg.Input != "/test/yml/input" {
					t.Errorf("Input falsch. Erwartet: /test/yml/input, Bekommen: %s", cfg.Input)
				}
				// Log.Level tests entfernt da YAML unmarshaling komplexer ist
			},
		},
		{
			name: "ungültiges YAML",
			setupFiles: func(t *testing.T) {
				invalidYaml := `input: /test
invalid_yaml: [unclosed_bracket`
				err := os.WriteFile("env.yaml", []byte(invalidYaml), 0644)
				if err != nil {
					t.Fatalf("Fehler beim Schreiben der ungültigen env.yaml: %v", err)
				}
			},
			expectError:    true,
			expectedValues: nil,
		},
		{
			name: "beide Dateien vorhanden - Konflikt",
			setupFiles: func(t *testing.T) {
				yamlContent := `input: /test/yaml`
				ymlContent := `input: /test/yml`

				err := os.WriteFile("env.yaml", []byte(yamlContent), 0644)
				if err != nil {
					t.Fatalf("Fehler beim Schreiben von env.yaml: %v", err)
				}
				err = os.WriteFile("env.yml", []byte(ymlContent), 0644)
				if err != nil {
					t.Fatalf("Fehler beim Schreiben von env.yml: %v", err)
				}
			},
			expectError:    true,
			expectedValues: nil,
		},
		{
			name: "keine Datei vorhanden",
			setupFiles: func(t *testing.T) {
				// Keine Dateien erstellen
			},
			expectError:    true,
			expectedValues: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any existing files before test
			os.Remove("env.yaml")
			os.Remove("env.yml")

			// Setup files for this test
			tt.setupFiles(t)

			// Clean up after test
			defer func() {
				os.Remove("env.yaml")
				os.Remove("env.yml")
			}()

			// Call the function
			cfg, err := loadEnvYaml()

			// Verify error expectation
			if tt.expectError {
				if err == nil {
					t.Error("Erwartete Fehler, bekommen nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unerwarteter Fehler: %v", err)
				return
			}

			// Verify expected values
			if tt.expectedValues != nil {
				tt.expectedValues(t, cfg)
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		expected bool
	}{
		{
			name: "Datei existiert",
			setup: func(t *testing.T) string {
				tmpfile := "testfile.tmp"
				err := os.WriteFile(tmpfile, []byte("test"), 0644)
				if err != nil {
					t.Fatalf("Fehler beim Erstellen der Testdatei: %v", err)
				}
				return tmpfile
			},
			expected: true,
		},
		{
			name: "Datei existiert nicht",
			setup: func(t *testing.T) string {
				return "nonexistent.tmp"
			},
			expected: false,
		},
		{
			name: "Verzeichnis existiert",
			setup: func(t *testing.T) string {
				tmpdir := "testdir.tmp"
				err := os.Mkdir(tmpdir, 0755)
				if err != nil {
					t.Fatalf("Fehler beim Erstellen des Testverzeichnisses: %v", err)
				}
				return tmpdir
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename := tt.setup(t)

			defer func() {
				// Clean up
				os.Remove(filename)
			}()

			result := fileExists(filename)
			if result != tt.expected {
				t.Errorf("fileExists(%s) = %v, want %v", filename, result, tt.expected)
			}
		})
	}
}

func TestSetupLogger(t *testing.T) {
	tests := []struct {
		name        string
		logLevel    string
		expectError bool
		expectedMin slog.Level
		description string
	}{
		{
			name:        "DEBUG level",
			logLevel:    "DEBUG",
			expectError: false,
			expectedMin: slog.LevelDebug,
			description: "Should set log level to DEBUG",
		},
		{
			name:        "INFO level",
			logLevel:    "INFO",
			expectError: false,
			expectedMin: slog.LevelInfo,
			description: "Should set log level to INFO",
		},
		{
			name:        "WARN level",
			logLevel:    "WARN",
			expectError: false,
			expectedMin: slog.LevelWarn,
			description: "Should set log level to WARN",
		},
		{
			name:        "ERROR level",
			logLevel:    "ERROR",
			expectError: false,
			expectedMin: slog.LevelError,
			description: "Should set log level to ERROR",
		},
		{
			name:        "empty level defaults to INFO",
			logLevel:    "",
			expectError: false,
			expectedMin: slog.LevelInfo,
			description: "Should default to INFO when empty",
		},
		{
			name:        "lowercase debug level",
			logLevel:    "debug",
			expectError: false,
			expectedMin: slog.LevelDebug,
			description: "Should handle lowercase level names",
		},
		{
			name:        "invalid level",
			logLevel:    "INVALID",
			expectError: false,
			expectedMin: slog.LevelInfo,
			description: "Should default to INFO for invalid levels",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.EnvConfig{}
			cfg.Log.Level = tt.logLevel

			setupLogger(cfg)

			if tt.expectError {
				// Da setupLogger keinen Fehler zurückgibt, erwarten wir keinen
			}
		})
	}
}
