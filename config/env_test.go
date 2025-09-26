package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Test constants to reduce duplication
const (
	testInputPath     = "/test/input"
	testOutput1Path   = "/test/output1"
	testS3BucketPath  = "s3://bucket/path"
	testRemotePath    = "/remote/path"
	testJSONOutput1   = "/json/output1"
	testJSONS3Path    = "s3://json-bucket/path"
	testValidPath1    = "/test/path1"
	testValidPath2    = "/test/path2"
	testValidFilePath = "/valid/path"

	// Validation test constants
	testSomeInput   = "/some/input"
	testSomeOutput  = "/some/output"
	testCustomInput = "/custom/input"

	testMinioEndpoint = "minio.example.com"
	testFTPHost       = "ftp.example.com"
	testS3Endpoint    = "s3.amazonaws.com"
	testAccessKey     = "access123"
	testSecretKey     = "secret123"
	testUsername      = "user"
	testPassword      = "pass"
	testRegion        = "us-east-1"
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
				Input: testCustomInput,
			},
			expected: EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "DEBUG"},
				Input: testCustomInput,
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
				Input:  testSomeInput,
				Output: []OutputTarget{{Path: testSomeOutput, Type: "file"}},
			},
			wantError: false,
		},
		{
			name: "empty input",
			config: EnvConfig{
				Input:  "",
				Output: []OutputTarget{{Path: testSomeOutput, Type: "file"}},
			},
			wantError: true,
		},
		{
			name: "no output targets",
			config: EnvConfig{
				Input:  testSomeInput,
				Output: []OutputTarget{},
			},
			wantError: true,
		},
		{
			name: "nil output",
			config: EnvConfig{
				Input:  testSomeInput,
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
				"INPUT":     testInputPath,
			},
			expected: EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "DEBUG"},
				Input: testInputPath,
			},
		},
		{
			name: "flat output structure",
			envVars: map[string]string{
				"LOG_LEVEL":           "INFO",
				"INPUT":               testInputPath,
				"OUTPUT_1_PATH":       testOutput1Path,
				"OUTPUT_1_TYPE":       "file",
				"OUTPUT_2_PATH":       testS3BucketPath,
				"OUTPUT_2_TYPE":       "s3",
				"OUTPUT_2_ENDPOINT":   testMinioEndpoint,
				"OUTPUT_2_ACCESS_KEY": testAccessKey,
				"OUTPUT_2_SECRET_KEY": testSecretKey,
				"OUTPUT_2_SSL":        "true",
				"OUTPUT_2_REGION":     testRegion,
			},
			expected: EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "INFO"},
				Input: testInputPath,
				Output: []OutputTarget{
					{Path: testOutput1Path, Type: "file"},
					{
						Path:      testS3BucketPath,
						Type:      "s3",
						Endpoint:  testMinioEndpoint,
						AccessKey: testAccessKey,
						SecretKey: testSecretKey,
						SSL:       boolPtr(true),
						Region:    testRegion,
					},
				},
			},
		},
		{
			name: "FTP output configuration",
			envVars: map[string]string{
				"INPUT":             testInputPath,
				"OUTPUT_1_PATH":     testRemotePath,
				"OUTPUT_1_TYPE":     "ftp",
				"OUTPUT_1_HOST":     testFTPHost,
				"OUTPUT_1_USERNAME": testUsername,
				"OUTPUT_1_PASSWORD": testPassword,
			},
			expected: EnvConfig{
				Input: testInputPath,
				Output: []OutputTarget{
					{
						Path:     testRemotePath,
						Type:     "ftp",
						Host:     testFTPHost,
						Username: testUsername,
						Password: testPassword,
					},
				},
			},
		},
		{
			name: "SSL false configuration",
			envVars: map[string]string{
				"INPUT":         testInputPath,
				"OUTPUT_1_PATH": testS3BucketPath,
				"OUTPUT_1_TYPE": "s3",
				"OUTPUT_1_SSL":  "false",
			},
			expected: EnvConfig{
				Input: testInputPath,
				Output: []OutputTarget{
					{
						Path: testS3BucketPath,
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
		{Path: testJSONOutput1, Type: "file"},
		{Path: testJSONS3Path, Type: "s3", Endpoint: testS3Endpoint},
	}
	targetsJSON, _ := json.Marshal(targets)

	os.Setenv("INPUT", testInputPath)
	os.Setenv("OUTPUTS", string(targetsJSON))

	config := EnvConfig{}
	err := config.LoadFromEnvironment()
	if err != nil {
		t.Fatalf("LoadFromEnvironment() failed: %v", err)
	}

	if len(config.Output) != 2 {
		t.Errorf("Expected 2 output targets, got %d", len(config.Output))
	}

	if config.Output[0].Path != testJSONOutput1 {
		t.Errorf("First target path = %v, want %v", config.Output[0].Path, testJSONOutput1)
	}
}

func TestEnvConfig_LoadFromEnvironment_InvalidJSON(t *testing.T) {
	// Backup current environment
	originalEnv := backupEnvironment()
	defer restoreEnvironment(originalEnv)

	// Clear environment
	clearTestEnvironment()

	// Test invalid JSON - should not crash
	os.Setenv("INPUT", testInputPath)
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
				"OUTPUT_2_PATH": testValidFilePath,
				"OUTPUT_2_TYPE": "file",
			},
			expectedCount: 1,
		},
		{
			name: "malformed environment variables should be ignored",
			envVars: map[string]string{
				"OUTPUT_1_PATH":    testValidPath1,
				"OUTPUT_INVALID":   "should be ignored",
				"NOTOUTPUT_1_PATH": "should be ignored",
			},
			expectedCount: 1,
		},
		{
			name: "mixed valid and invalid SSL values",
			envVars: map[string]string{
				"OUTPUT_1_PATH": testValidPath1,
				"OUTPUT_1_SSL":  "true",
				"OUTPUT_2_PATH": testValidPath2,
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

	// Clear FILE_STABILITY_* pattern keys
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "FILE_STABILITY_") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) >= 1 {
				os.Unsetenv(parts[0])
			}
		}
	}
}

func TestEnvConfig_LoadFileStabilityFromEnv(t *testing.T) {
	// Clean environment before tests
	clearFileStabilityEnv()
	defer clearFileStabilityEnv()

	tests := []struct {
		name     string
		setupEnv func()
		expected struct {
			MaxRetries      int
			CheckInterval   int
			StabilityPeriod int
		}
		description string
	}{
		{
			name: "all file stability values set",
			setupEnv: func() {
				os.Setenv("FILE_STABILITY_MAX_RETRIES", "50")
				os.Setenv("FILE_STABILITY_CHECK_INTERVAL", "2")
				os.Setenv("FILE_STABILITY_PERIOD", "3")
			},
			expected: struct {
				MaxRetries      int
				CheckInterval   int
				StabilityPeriod int
			}{
				MaxRetries:      50,
				CheckInterval:   2,
				StabilityPeriod: 3,
			},
			description: "Should load all file stability values from environment",
		},
		{
			name: "partial file stability values set",
			setupEnv: func() {
				os.Setenv("FILE_STABILITY_MAX_RETRIES", "25")
				// Andere Werte nicht setzen
			},
			expected: struct {
				MaxRetries      int
				CheckInterval   int
				StabilityPeriod int
			}{
				MaxRetries:      25,
				CheckInterval:   0, // Default bleibt
				StabilityPeriod: 0, // Default bleibt
			},
			description: "Should load only set values, others remain default",
		},
		{
			name: "invalid values ignored",
			setupEnv: func() {
				os.Setenv("FILE_STABILITY_MAX_RETRIES", "invalid")
				os.Setenv("FILE_STABILITY_CHECK_INTERVAL", "-1")
				os.Setenv("FILE_STABILITY_PERIOD", "0")
			},
			expected: struct {
				MaxRetries      int
				CheckInterval   int
				StabilityPeriod int
			}{
				MaxRetries:      0, // Invalid ignored
				CheckInterval:   0, // Negative ignored
				StabilityPeriod: 0, // Zero ignored
			},
			description: "Should ignore invalid values",
		},
		{
			name: "empty environment",
			setupEnv: func() {
				// Keine Umgebungsvariablen setzen
			},
			expected: struct {
				MaxRetries      int
				CheckInterval   int
				StabilityPeriod int
			}{
				MaxRetries:      0,
				CheckInterval:   0,
				StabilityPeriod: 0,
			},
			description: "Should have zero values with empty environment",
		},
		{
			name: "boundary values",
			setupEnv: func() {
				os.Setenv("FILE_STABILITY_MAX_RETRIES", "1")
				os.Setenv("FILE_STABILITY_CHECK_INTERVAL", "1")
				os.Setenv("FILE_STABILITY_PERIOD", "1")
			},
			expected: struct {
				MaxRetries      int
				CheckInterval   int
				StabilityPeriod int
			}{
				MaxRetries:      1,
				CheckInterval:   1,
				StabilityPeriod: 1,
			},
			description: "Should handle boundary values correctly",
		},
		{
			name: "large valid values",
			setupEnv: func() {
				os.Setenv("FILE_STABILITY_MAX_RETRIES", "9999")
				os.Setenv("FILE_STABILITY_CHECK_INTERVAL", "300")
				os.Setenv("FILE_STABILITY_PERIOD", "600")
			},
			expected: struct {
				MaxRetries      int
				CheckInterval   int
				StabilityPeriod int
			}{
				MaxRetries:      9999,
				CheckInterval:   300,
				StabilityPeriod: 600,
			},
			description: "Should handle large valid values",
		},
		{
			name: "mixed valid and invalid values",
			setupEnv: func() {
				os.Setenv("FILE_STABILITY_MAX_RETRIES", "15")     // Valid
				os.Setenv("FILE_STABILITY_CHECK_INTERVAL", "abc") // Invalid
				os.Setenv("FILE_STABILITY_PERIOD", "5")           // Valid
			},
			expected: struct {
				MaxRetries      int
				CheckInterval   int
				StabilityPeriod int
			}{
				MaxRetries:      15, // Valid value loaded
				CheckInterval:   0,  // Invalid ignored
				StabilityPeriod: 5,  // Valid value loaded
			},
			description: "Should load valid values and ignore invalid ones",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment before each test
			clearFileStabilityEnv()

			// Setup environment for this test
			tt.setupEnv()

			// Create config and load from environment
			cfg := &EnvConfig{}
			cfg.loadFileStabilityFromEnv()

			// Verify results
			if cfg.FileStability.MaxRetries != tt.expected.MaxRetries {
				t.Errorf("MaxRetries mismatch. Expected: %d, Got: %d",
					tt.expected.MaxRetries, cfg.FileStability.MaxRetries)
			}

			if cfg.FileStability.CheckInterval != tt.expected.CheckInterval {
				t.Errorf("CheckInterval mismatch. Expected: %d, Got: %d",
					tt.expected.CheckInterval, cfg.FileStability.CheckInterval)
			}

			if cfg.FileStability.StabilityPeriod != tt.expected.StabilityPeriod {
				t.Errorf("StabilityPeriod mismatch. Expected: %d, Got: %d",
					tt.expected.StabilityPeriod, cfg.FileStability.StabilityPeriod)
			}
		})
	}
}

func TestEnvConfig_SetDefaults_FileStability(t *testing.T) {
	tests := []struct {
		name     string
		initial  EnvConfig
		expected struct {
			MaxRetries      int
			CheckInterval   int
			StabilityPeriod int
		}
		description string
	}{
		{
			name:    "empty config gets default file stability values",
			initial: EnvConfig{},
			expected: struct {
				MaxRetries      int
				CheckInterval   int
				StabilityPeriod int
			}{
				MaxRetries:      30,
				CheckInterval:   1,
				StabilityPeriod: 1,
			},
			description: "Should set default file stability values",
		},
		{
			name: "partial config preserves existing values",
			initial: EnvConfig{
				FileStability: struct {
					MaxRetries      int `yaml:"max-retries"`
					CheckInterval   int `yaml:"check-interval"`
					StabilityPeriod int `yaml:"stability-period"`
				}{
					MaxRetries:      50,
					CheckInterval:   0, // Will be defaulted
					StabilityPeriod: 5,
				},
			},
			expected: struct {
				MaxRetries      int
				CheckInterval   int
				StabilityPeriod int
			}{
				MaxRetries:      50, // Preserved
				CheckInterval:   1,  // Defaulted
				StabilityPeriod: 5,  // Preserved
			},
			description: "Should preserve existing non-zero values and default zero ones",
		},
		{
			name: "complete config preserves all values",
			initial: EnvConfig{
				FileStability: struct {
					MaxRetries      int `yaml:"max-retries"`
					CheckInterval   int `yaml:"check-interval"`
					StabilityPeriod int `yaml:"stability-period"`
				}{
					MaxRetries:      100,
					CheckInterval:   3,
					StabilityPeriod: 10,
				},
			},
			expected: struct {
				MaxRetries      int
				CheckInterval   int
				StabilityPeriod int
			}{
				MaxRetries:      100, // Preserved
				CheckInterval:   3,   // Preserved
				StabilityPeriod: 10,  // Preserved
			},
			description: "Should preserve all existing non-zero values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.initial
			cfg.SetDefaults()

			if cfg.FileStability.MaxRetries != tt.expected.MaxRetries {
				t.Errorf("MaxRetries mismatch. Expected: %d, Got: %d",
					tt.expected.MaxRetries, cfg.FileStability.MaxRetries)
			}

			if cfg.FileStability.CheckInterval != tt.expected.CheckInterval {
				t.Errorf("CheckInterval mismatch. Expected: %d, Got: %d",
					tt.expected.CheckInterval, cfg.FileStability.CheckInterval)
			}

			if cfg.FileStability.StabilityPeriod != tt.expected.StabilityPeriod {
				t.Errorf("StabilityPeriod mismatch. Expected: %d, Got: %d",
					tt.expected.StabilityPeriod, cfg.FileStability.StabilityPeriod)
			}
		})
	}
}

func TestEnvConfig_LoadFromEnvironment_WithFileStability(t *testing.T) {
	// Clear environment
	clearTestEnvironment()
	clearFileStabilityEnv()
	defer func() {
		clearTestEnvironment()
		clearFileStabilityEnv()
	}()

	// Setup environment
	os.Setenv("INPUT", "/test/input")
	os.Setenv("LOG_LEVEL", "DEBUG")
	os.Setenv("FILE_STABILITY_MAX_RETRIES", "75")
	os.Setenv("FILE_STABILITY_CHECK_INTERVAL", "3")
	os.Setenv("FILE_STABILITY_PERIOD", "5")

	cfg := &EnvConfig{}
	err := cfg.LoadFromEnvironment()

	if err != nil {
		t.Fatalf("LoadFromEnvironment should not fail: %v", err)
	}

	// Verify basic config loaded
	if cfg.Input != "/test/input" {
		t.Errorf("Input mismatch. Expected: /test/input, Got: %s", cfg.Input)
	}

	if cfg.Log.Level != "DEBUG" {
		t.Errorf("Log Level mismatch. Expected: DEBUG, Got: %s", cfg.Log.Level)
	}

	// Verify file stability config loaded
	if cfg.FileStability.MaxRetries != 75 {
		t.Errorf("MaxRetries mismatch. Expected: 75, Got: %d", cfg.FileStability.MaxRetries)
	}

	if cfg.FileStability.CheckInterval != 3 {
		t.Errorf("CheckInterval mismatch. Expected: 3, Got: %d", cfg.FileStability.CheckInterval)
	}

	if cfg.FileStability.StabilityPeriod != 5 {
		t.Errorf("StabilityPeriod mismatch. Expected: 5, Got: %d", cfg.FileStability.StabilityPeriod)
	}
}

func clearFileStabilityEnv() {
	fileStabilityKeys := []string{
		"FILE_STABILITY_MAX_RETRIES",
		"FILE_STABILITY_CHECK_INTERVAL",
		"FILE_STABILITY_PERIOD",
	}

	for _, key := range fileStabilityKeys {
		os.Unsetenv(key)
	}
}
