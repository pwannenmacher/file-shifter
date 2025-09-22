package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestEnvConfig_SetDefaults(t *testing.T) {
	tests := []struct {
		name     string
		config   EnvConfig
		expected EnvConfig
	}{
		{
			name:   "empty config sets defaults",
			config: EnvConfig{},
			expected: EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "INFO"},
				Input: "./input",
			},
		},
		{
			name: "existing values are preserved",
			config: EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "DEBUG"},
				Input: "/custom/input",
			},
			expected: EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "DEBUG"},
				Input: "/custom/input",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.SetDefaults()
			if tt.config.Log.Level != tt.expected.Log.Level {
				t.Errorf("SetDefaults() Log.Level = %v, want %v", tt.config.Log.Level, tt.expected.Log.Level)
			}
			if tt.config.Input != tt.expected.Input {
				t.Errorf("SetDefaults() Input = %v, want %v", tt.config.Input, tt.expected.Input)
			}
		})
	}
}

func TestEnvConfig_GetLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
		expected string
	}{
		{"debug level", "debug", "DEBUG"},
		{"DEBUG level", "DEBUG", "DEBUG"},
		{"info level", "info", "INFO"},
		{"INFO level", "INFO", "INFO"},
		{"warn level", "warn", "WARN"},
		{"WARN level", "WARN", "WARN"},
		{"error level", "error", "ERROR"},
		{"ERROR level", "ERROR", "ERROR"},
		{"invalid level", "invalid", "INFO"},
		{"empty level", "", "INFO"},
		{"mixed case", "Debug", "DEBUG"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := EnvConfig{}
			config.Log.Level = tt.logLevel
			result := config.GetLogLevel()
			if result != tt.expected {
				t.Errorf("GetLogLevel() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEnvConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    EnvConfig
		wantError bool
	}{
		{
			name: "valid config",
			config: EnvConfig{
				Input:  "/some/input",
				Output: []OutputTarget{{Path: "/some/output", Type: "file"}},
			},
			wantError: false,
		},
		{
			name: "empty input",
			config: EnvConfig{
				Input:  "",
				Output: []OutputTarget{{Path: "/some/output", Type: "file"}},
			},
			wantError: true,
		},
		{
			name: "no output targets",
			config: EnvConfig{
				Input:  "/some/input",
				Output: []OutputTarget{},
			},
			wantError: true,
		},
		{
			name: "nil output",
			config: EnvConfig{
				Input:  "/some/input",
				Output: nil,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestEnvConfig_LoadFromEnvironment(t *testing.T) {
	// Backup current environment
	originalEnv := backupEnvironment()
	defer restoreEnvironment(originalEnv)

	tests := []struct {
		name     string
		envVars  map[string]string
		expected EnvConfig
	}{
		{
			name: "basic environment variables",
			envVars: map[string]string{
				"LOG_LEVEL": "DEBUG",
				"INPUT":     "/test/input",
			},
			expected: EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "DEBUG"},
				Input: "/test/input",
			},
		},
		{
			name: "flat output structure",
			envVars: map[string]string{
				"LOG_LEVEL":           "INFO",
				"INPUT":               "/test/input",
				"OUTPUT_1_PATH":       "/test/output1",
				"OUTPUT_1_TYPE":       "file",
				"OUTPUT_2_PATH":       "s3://bucket/path",
				"OUTPUT_2_TYPE":       "s3",
				"OUTPUT_2_ENDPOINT":   "minio.example.com",
				"OUTPUT_2_ACCESS_KEY": "access123",
				"OUTPUT_2_SECRET_KEY": "secret123",
				"OUTPUT_2_SSL":        "true",
				"OUTPUT_2_REGION":     "us-east-1",
			},
			expected: EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "INFO"},
				Input: "/test/input",
				Output: []OutputTarget{
					{Path: "/test/output1", Type: "file"},
					{
						Path:      "s3://bucket/path",
						Type:      "s3",
						Endpoint:  "minio.example.com",
						AccessKey: "access123",
						SecretKey: "secret123",
						SSL:       boolPtr(true),
						Region:    "us-east-1",
					},
				},
			},
		},
		{
			name: "FTP output configuration",
			envVars: map[string]string{
				"INPUT":             "/test/input",
				"OUTPUT_1_PATH":     "/remote/path",
				"OUTPUT_1_TYPE":     "ftp",
				"OUTPUT_1_HOST":     "ftp.example.com",
				"OUTPUT_1_USERNAME": "user",
				"OUTPUT_1_PASSWORD": "pass",
			},
			expected: EnvConfig{
				Input: "/test/input",
				Output: []OutputTarget{
					{
						Path:     "/remote/path",
						Type:     "ftp",
						Host:     "ftp.example.com",
						Username: "user",
						Password: "pass",
					},
				},
			},
		},
		{
			name: "SSL false configuration",
			envVars: map[string]string{
				"INPUT":         "/test/input",
				"OUTPUT_1_PATH": "s3://bucket/path",
				"OUTPUT_1_TYPE": "s3",
				"OUTPUT_1_SSL":  "false",
			},
			expected: EnvConfig{
				Input: "/test/input",
				Output: []OutputTarget{
					{
						Path: "s3://bucket/path",
						Type: "s3",
						SSL:  boolPtr(false),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			clearTestEnvironment()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			// Load configuration
			config := EnvConfig{}
			err := config.LoadFromEnvironment()
			if err != nil {
				t.Fatalf("LoadFromEnvironment() failed: %v", err)
			}

			// Compare results
			if config.Log.Level != tt.expected.Log.Level {
				t.Errorf("Log.Level = %v, want %v", config.Log.Level, tt.expected.Log.Level)
			}
			if config.Input != tt.expected.Input {
				t.Errorf("Input = %v, want %v", config.Input, tt.expected.Input)
			}

			// Compare output targets - order independent
			if len(config.Output) != len(tt.expected.Output) {
				t.Errorf("Output length = %v, want %v", len(config.Output), len(tt.expected.Output))
				return
			}

			// Create maps for comparison to ignore order
			actualTargets := make(map[string]OutputTarget)
			expectedTargets := make(map[string]OutputTarget)

			for _, target := range config.Output {
				actualTargets[target.Path] = target
			}
			for _, target := range tt.expected.Output {
				expectedTargets[target.Path] = target
			}

			// Compare each expected target
			for path, expectedTarget := range expectedTargets {
				actualTarget, exists := actualTargets[path]
				if !exists {
					t.Errorf("Missing output target with path: %s", path)
					continue
				}
				compareOutputTargetByPath(t, actualTarget, expectedTarget, path)
			}
		})
	}
}

func TestEnvConfig_LoadFromEnvironment_JSONFallback(t *testing.T) {
	// Backup current environment
	originalEnv := backupEnvironment()
	defer restoreEnvironment(originalEnv)

	// Clear environment
	clearTestEnvironment()

	// Test JSON fallback
	targets := []OutputTarget{
		{Path: "/json/output1", Type: "file"},
		{Path: "s3://json-bucket/path", Type: "s3", Endpoint: "s3.amazonaws.com"},
	}
	targetsJSON, _ := json.Marshal(targets)

	os.Setenv("INPUT", "/test/input")
	os.Setenv("OUTPUTS", string(targetsJSON))

	config := EnvConfig{}
	err := config.LoadFromEnvironment()
	if err != nil {
		t.Fatalf("LoadFromEnvironment() failed: %v", err)
	}

	if len(config.Output) != 2 {
		t.Errorf("Expected 2 output targets, got %d", len(config.Output))
	}

	if config.Output[0].Path != "/json/output1" {
		t.Errorf("First target path = %v, want %v", config.Output[0].Path, "/json/output1")
	}
}

func TestEnvConfig_LoadFromEnvironment_InvalidJSON(t *testing.T) {
	// Backup current environment
	originalEnv := backupEnvironment()
	defer restoreEnvironment(originalEnv)

	// Clear environment
	clearTestEnvironment()

	// Test invalid JSON - should not crash
	os.Setenv("INPUT", "/test/input")
	os.Setenv("OUTPUTS", "invalid json")

	config := EnvConfig{}
	err := config.LoadFromEnvironment()
	if err != nil {
		t.Fatalf("LoadFromEnvironment() failed: %v", err)
	}

	// Should have no output targets due to invalid JSON
	if len(config.Output) != 0 {
		t.Errorf("Expected 0 output targets due to invalid JSON, got %d", len(config.Output))
	}
}

func TestEnvConfig_LoadOutputTargetsEdgeCases(t *testing.T) {
	// Backup current environment
	originalEnv := backupEnvironment()
	defer restoreEnvironment(originalEnv)

	tests := []struct {
		name          string
		envVars       map[string]string
		expectedCount int
	}{
		{
			name: "target without path should be ignored",
			envVars: map[string]string{
				"OUTPUT_1_TYPE": "file",
				"OUTPUT_2_PATH": "/valid/path",
				"OUTPUT_2_TYPE": "file",
			},
			expectedCount: 1,
		},
		{
			name: "malformed environment variables should be ignored",
			envVars: map[string]string{
				"OUTPUT_1_PATH":    "/test/path",
				"OUTPUT_INVALID":   "should be ignored",
				"NOTOUTPUT_1_PATH": "should be ignored",
			},
			expectedCount: 1,
		},
		{
			name: "mixed valid and invalid SSL values",
			envVars: map[string]string{
				"OUTPUT_1_PATH": "/test/path1",
				"OUTPUT_1_SSL":  "true",
				"OUTPUT_2_PATH": "/test/path2",
				"OUTPUT_2_SSL":  "invalid",
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			clearTestEnvironment()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			config := EnvConfig{}
			err := config.LoadFromEnvironment()
			if err != nil {
				t.Fatalf("LoadFromEnvironment() failed: %v", err)
			}

			if len(config.Output) != tt.expectedCount {
				t.Errorf("Expected %d output targets, got %d", tt.expectedCount, len(config.Output))
			}
		})
	}
}

// Helper functions

func boolPtr(b bool) *bool {
	return &b
}

func compareOutputTargetByPath(t *testing.T, actual, expected OutputTarget, path string) {
	if actual.Path != expected.Path {
		t.Errorf("Output[%s].Path = %v, want %v", path, actual.Path, expected.Path)
	}
	if actual.Type != expected.Type {
		t.Errorf("Output[%s].Type = %v, want %v", path, actual.Type, expected.Type)
	}
	if actual.Endpoint != expected.Endpoint {
		t.Errorf("Output[%s].Endpoint = %v, want %v", path, actual.Endpoint, expected.Endpoint)
	}
	if actual.AccessKey != expected.AccessKey {
		t.Errorf("Output[%s].AccessKey = %v, want %v", path, actual.AccessKey, expected.AccessKey)
	}
	if actual.SecretKey != expected.SecretKey {
		t.Errorf("Output[%s].SecretKey = %v, want %v", path, actual.SecretKey, expected.SecretKey)
	}
	if (actual.SSL == nil) != (expected.SSL == nil) {
		t.Errorf("Output[%s].SSL nil mismatch: actual=%v, expected=%v", path, actual.SSL == nil, expected.SSL == nil)
	} else if actual.SSL != nil && expected.SSL != nil && *actual.SSL != *expected.SSL {
		t.Errorf("Output[%s].SSL = %v, want %v", path, *actual.SSL, *expected.SSL)
	}
	if actual.Region != expected.Region {
		t.Errorf("Output[%s].Region = %v, want %v", path, actual.Region, expected.Region)
	}
	if actual.Host != expected.Host {
		t.Errorf("Output[%s].Host = %v, want %v", path, actual.Host, expected.Host)
	}
	if actual.Username != expected.Username {
		t.Errorf("Output[%s].Username = %v, want %v", path, actual.Username, expected.Username)
	}
	if actual.Password != expected.Password {
		t.Errorf("Output[%s].Password = %v, want %v", path, actual.Password, expected.Password)
	}
}

func compareOutputTarget(t *testing.T, actual, expected OutputTarget, index int) {
	if actual.Path != expected.Path {
		t.Errorf("Output[%d].Path = %v, want %v", index, actual.Path, expected.Path)
	}
	if actual.Type != expected.Type {
		t.Errorf("Output[%d].Type = %v, want %v", index, actual.Type, expected.Type)
	}
	if actual.Endpoint != expected.Endpoint {
		t.Errorf("Output[%d].Endpoint = %v, want %v", index, actual.Endpoint, expected.Endpoint)
	}
	if actual.AccessKey != expected.AccessKey {
		t.Errorf("Output[%d].AccessKey = %v, want %v", index, actual.AccessKey, expected.AccessKey)
	}
	if actual.SecretKey != expected.SecretKey {
		t.Errorf("Output[%d].SecretKey = %v, want %v", index, actual.SecretKey, expected.SecretKey)
	}
	if (actual.SSL == nil) != (expected.SSL == nil) {
		t.Errorf("Output[%d].SSL nil mismatch: actual=%v, expected=%v", index, actual.SSL == nil, expected.SSL == nil)
	} else if actual.SSL != nil && expected.SSL != nil && *actual.SSL != *expected.SSL {
		t.Errorf("Output[%d].SSL = %v, want %v", index, *actual.SSL, *expected.SSL)
	}
	if actual.Region != expected.Region {
		t.Errorf("Output[%d].Region = %v, want %v", index, actual.Region, expected.Region)
	}
	if actual.Host != expected.Host {
		t.Errorf("Output[%d].Host = %v, want %v", index, actual.Host, expected.Host)
	}
	if actual.Username != expected.Username {
		t.Errorf("Output[%d].Username = %v, want %v", index, actual.Username, expected.Username)
	}
	if actual.Password != expected.Password {
		t.Errorf("Output[%d].Password = %v, want %v", index, actual.Password, expected.Password)
	}
}

func backupEnvironment() map[string]string {
	env := make(map[string]string)
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return env
}

func restoreEnvironment(env map[string]string) {
	// Clear all environment variables that start with our test prefixes
	clearTestEnvironment()

	// Restore original environment
	for key, value := range env {
		os.Setenv(key, value)
	}
}

func clearTestEnvironment() {
	testKeys := []string{
		"LOG_LEVEL", "INPUT", "OUTPUTS",
	}

	// Clear known test keys
	for _, key := range testKeys {
		os.Unsetenv(key)
	}

	// Clear OUTPUT_* pattern keys
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "OUTPUT_") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) >= 1 {
				os.Unsetenv(parts[0])
			}
		}
	}
}
