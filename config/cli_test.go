package config

import (
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"
)

func TestParseCLI(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected *CLIConfig
	}{
		{
			name: "no arguments",
			args: []string{},
			expected: &CLIConfig{
				LogLevel:    "",
				Input:       "",
				OutputsJSON: "",
				ShowHelp:    false,
			},
		},
		{
			name: "log level argument",
			args: []string{"--log-level", "DEBUG"},
			expected: &CLIConfig{
				LogLevel:    "DEBUG",
				Input:       "",
				OutputsJSON: "",
				ShowHelp:    false,
			},
		},
		{
			name: "input directory argument",
			args: []string{"--input", "/test/input"},
			expected: &CLIConfig{
				LogLevel:    "",
				Input:       "/test/input",
				OutputsJSON: "",
				ShowHelp:    false,
			},
		},
		{
			name: "outputs JSON argument",
			args: []string{"--outputs", `[{"path":"./output","type":"filesystem"}]`},
			expected: &CLIConfig{
				LogLevel:    "",
				Input:       "",
				OutputsJSON: `[{"path":"./output","type":"filesystem"}]`,
				ShowHelp:    false,
			},
		},
		{
			name: "all arguments combined",
			args: []string{"--log-level", "WARN", "--input", "/data", "--outputs", `[{"path":"s3://bucket","type":"s3"}]`},
			expected: &CLIConfig{
				LogLevel:    "WARN",
				Input:       "/data",
				OutputsJSON: `[{"path":"s3://bucket","type":"s3"}]`,
				ShowHelp:    false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset command line flags for each test
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

			// Backup original os.Args
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()

			// Set test arguments
			os.Args = append([]string{"test"}, tt.args...)

			// Parse CLI
			result := ParseCLI()

			// Compare results
			if result.LogLevel != tt.expected.LogLevel {
				t.Errorf("LogLevel = %q, want %q", result.LogLevel, tt.expected.LogLevel)
			}
			if result.Input != tt.expected.Input {
				t.Errorf("Input = %q, want %q", result.Input, tt.expected.Input)
			}
			if result.OutputsJSON != tt.expected.OutputsJSON {
				t.Errorf("OutputsJSON = %q, want %q", result.OutputsJSON, tt.expected.OutputsJSON)
			}
			if result.ShowHelp != tt.expected.ShowHelp {
				t.Errorf("ShowHelp = %v, want %v", result.ShowHelp, tt.expected.ShowHelp)
			}
		})
	}
}

func TestCLIConfig_ApplyToCfg(t *testing.T) {
	tests := []struct {
		name     string
		cli      *CLIConfig
		initial  *EnvConfig
		expected *EnvConfig
		wantErr  bool
	}{
		{
			name: "empty CLI config doesn't change EnvConfig",
			cli: &CLIConfig{
				LogLevel:    "",
				Input:       "",
				OutputsJSON: "",
			},
			initial: &EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "INFO"},
				Input:  "./input",
				Output: []OutputTarget{},
			},
			expected: &EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "INFO"},
				Input:  "./input",
				Output: []OutputTarget{},
			},
			wantErr: false,
		},
		{
			name: "CLI overrides log level",
			cli: &CLIConfig{
				LogLevel: "DEBUG",
			},
			initial: &EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "INFO"},
				Input: "./input",
			},
			expected: &EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "DEBUG"},
				Input: "./input",
			},
			wantErr: false,
		},
		{
			name: "CLI overrides input directory",
			cli: &CLIConfig{
				Input: "/custom/input",
			},
			initial: &EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "INFO"},
				Input: "./input",
			},
			expected: &EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "INFO"},
				Input: "/custom/input",
			},
			wantErr: false,
		},
		{
			name: "CLI sets outputs from JSON",
			cli: &CLIConfig{
				OutputsJSON: `[{"path":"./backup","type":"filesystem"},{"path":"s3://bucket","type":"s3"}]`,
			},
			initial: &EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "INFO"},
				Input:  "./input",
				Output: []OutputTarget{},
			},
			expected: &EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "INFO"},
				Input: "./input",
				Output: []OutputTarget{
					{Path: "./backup", Type: "filesystem"},
					{Path: "s3://bucket", Type: "s3"},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid JSON returns error",
			cli: &CLIConfig{
				OutputsJSON: `invalid json`,
			},
			initial: &EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "INFO"},
				Input:  "./input",
				Output: []OutputTarget{},
			},
			expected: nil,
			wantErr:  true,
		},
		{
			name: "all CLI options applied",
			cli: &CLIConfig{
				LogLevel:    "ERROR",
				Input:       "/data/source",
				OutputsJSON: `[{"path":"sftp://server/path","type":"sftp","host":"server.com","username":"user","password":"pass"}]`,
			},
			initial: &EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "INFO"},
				Input:  "./input",
				Output: []OutputTarget{},
			},
			expected: &EnvConfig{
				Log: struct {
					Level string `yaml:"level"`
				}{Level: "ERROR"},
				Input: "/data/source",
				Output: []OutputTarget{
					{
						Path:     "sftp://server/path",
						Type:     "sftp",
						Host:     "server.com",
						Username: "user",
						Password: "pass",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cli.ApplyToCfg(tt.initial)

			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyToCfg() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return // Skip further checks if we expected an error
			}

			// Compare results
			if tt.initial.Log.Level != tt.expected.Log.Level {
				t.Errorf("Log.Level = %q, want %q", tt.initial.Log.Level, tt.expected.Log.Level)
			}
			if tt.initial.Input != tt.expected.Input {
				t.Errorf("Input = %q, want %q", tt.initial.Input, tt.expected.Input)
			}
			if len(tt.initial.Output) != len(tt.expected.Output) {
				t.Errorf("Output length = %d, want %d", len(tt.initial.Output), len(tt.expected.Output))
				return
			}

			// Compare output targets
			for i, got := range tt.initial.Output {
				expected := tt.expected.Output[i]
				if got.Path != expected.Path {
					t.Errorf("Output[%d].Path = %q, want %q", i, got.Path, expected.Path)
				}
				if got.Type != expected.Type {
					t.Errorf("Output[%d].Type = %q, want %q", i, got.Type, expected.Type)
				}
				if got.Host != expected.Host {
					t.Errorf("Output[%d].Host = %q, want %q", i, got.Host, expected.Host)
				}
				if got.Username != expected.Username {
					t.Errorf("Output[%d].Username = %q, want %q", i, got.Username, expected.Username)
				}
				if got.Password != expected.Password {
					t.Errorf("Output[%d].Password = %q, want %q", i, got.Password, expected.Password)
				}
			}
		})
	}
}

func TestCLIConfig_HasOutputsConfigured(t *testing.T) {
	tests := []struct {
		name     string
		cli      *CLIConfig
		expected bool
	}{
		{
			name: "no outputs configured",
			cli: &CLIConfig{
				OutputsJSON: "",
			},
			expected: false,
		},
		{
			name: "outputs configured",
			cli: &CLIConfig{
				OutputsJSON: `[{"path":"./output","type":"filesystem"}]`,
			},
			expected: true,
		},
		{
			name: "empty JSON array still counts as configured",
			cli: &CLIConfig{
				OutputsJSON: "[]",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cli.HasOutputsConfigured()
			if result != tt.expected {
				t.Errorf("HasOutputsConfigured() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCLIConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cli     *CLIConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid empty config",
			cli: &CLIConfig{
				LogLevel:    "",
				Input:       "",
				OutputsJSON: "",
			},
			wantErr: false,
		},
		{
			name: "valid log level DEBUG",
			cli: &CLIConfig{
				LogLevel: "DEBUG",
			},
			wantErr: false,
		},
		{
			name: "valid log level INFO",
			cli: &CLIConfig{
				LogLevel: "INFO",
			},
			wantErr: false,
		},
		{
			name: "valid log level WARN",
			cli: &CLIConfig{
				LogLevel: "WARN",
			},
			wantErr: false,
		},
		{
			name: "valid log level ERROR",
			cli: &CLIConfig{
				LogLevel: "ERROR",
			},
			wantErr: false,
		},
		{
			name: "valid log level case insensitive",
			cli: &CLIConfig{
				LogLevel: "debug",
			},
			wantErr: false,
		},
		{
			name: "invalid log level",
			cli: &CLIConfig{
				LogLevel: "INVALID",
			},
			wantErr: true,
			errMsg:  "invalid log level",
		},
		{
			name: "valid outputs JSON",
			cli: &CLIConfig{
				OutputsJSON: `[{"path":"./output","type":"filesystem"}]`,
			},
			wantErr: false,
		},
		{
			name: "valid multiple outputs JSON",
			cli: &CLIConfig{
				OutputsJSON: `[{"path":"./output1","type":"filesystem"},{"path":"s3://bucket","type":"s3"}]`,
			},
			wantErr: false,
		},
		{
			name: "invalid JSON format",
			cli: &CLIConfig{
				OutputsJSON: `invalid json`,
			},
			wantErr: true,
			errMsg:  "invalid --outputs JSON format",
		},
		{
			name: "output without path",
			cli: &CLIConfig{
				OutputsJSON: `[{"type":"filesystem"}]`,
			},
			wantErr: true,
			errMsg:  "'path' is required",
		},
		{
			name: "output without type",
			cli: &CLIConfig{
				OutputsJSON: `[{"path":"./output"}]`,
			},
			wantErr: true,
			errMsg:  "'type' is required",
		},
		{
			name: "invalid output type",
			cli: &CLIConfig{
				OutputsJSON: `[{"path":"./output","type":"invalid"}]`,
			},
			wantErr: true,
			errMsg:  "invalid type",
		},
		{
			name: "valid filesystem type",
			cli: &CLIConfig{
				OutputsJSON: `[{"path":"./output","type":"filesystem"}]`,
			},
			wantErr: false,
		},
		{
			name: "valid s3 type",
			cli: &CLIConfig{
				OutputsJSON: `[{"path":"s3://bucket","type":"s3"}]`,
			},
			wantErr: false,
		},
		{
			name: "valid sftp type",
			cli: &CLIConfig{
				OutputsJSON: `[{"path":"sftp://server/path","type":"sftp"}]`,
			},
			wantErr: false,
		},
		{
			name: "valid ftp type",
			cli: &CLIConfig{
				OutputsJSON: `[{"path":"ftp://server/path","type":"ftp"}]`,
			},
			wantErr: false,
		},
		{
			name: "complete valid config",
			cli: &CLIConfig{
				LogLevel:    "DEBUG",
				Input:       "/data/input",
				OutputsJSON: `[{"path":"./backup","type":"filesystem"},{"path":"s3://bucket/prefix","type":"s3","endpoint":"s3.amazonaws.com","access-key":"key","secret-key":"secret","ssl":true,"region":"eu-central-1"}]`,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cli.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error message = %v, should contain %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestCLIConfig_ValidateComplexOutputs(t *testing.T) {
	tests := []struct {
		name        string
		outputsJSON string
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "S3 output with all fields",
			outputsJSON: `[{"path":"s3://bucket/prefix","type":"s3","endpoint":"s3.amazonaws.com","access-key":"AKIAIOSFODNN7EXAMPLE","secret-key":"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY","ssl":true,"region":"eu-central-1"}]`,
			wantErr:     false,
		},
		{
			name:        "SFTP output with authentication",
			outputsJSON: `[{"path":"sftp://server/path","type":"sftp","host":"server.com","username":"user","password":"secret"}]`,
			wantErr:     false,
		},
		{
			name:        "FTP output with port",
			outputsJSON: `[{"path":"ftp://server/path","type":"ftp","host":"ftp.server.com","username":"ftpuser","password":"ftppass","port":2121}]`,
			wantErr:     false,
		},
		{
			name:        "mixed output types",
			outputsJSON: `[{"path":"./local","type":"filesystem"},{"path":"s3://bucket","type":"s3","endpoint":"localhost:9000","access-key":"minioadmin","secret-key":"minioadmin","ssl":false},{"path":"sftp://backup/files","type":"sftp","host":"backup.com","username":"backup","password":"backuppw"}]`,
			wantErr:     false,
		},
		{
			name:        "empty array",
			outputsJSON: `[]`,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := &CLIConfig{
				OutputsJSON: tt.outputsJSON,
			}

			err := cli.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error message = %v, should contain %q", err, tt.errMsg)
				}
			}

			// Additional check: verify JSON can be parsed into OutputTarget slice
			if !tt.wantErr && tt.outputsJSON != "" {
				var targets []OutputTarget
				parseErr := json.Unmarshal([]byte(tt.outputsJSON), &targets)
				if parseErr != nil {
					t.Errorf("Test JSON should be valid but failed to parse: %v", parseErr)
				}
			}
		})
	}
}

// TestPrintUsage tests that printUsage doesn't panic and runs without error
func TestPrintUsage(t *testing.T) {
	// Backup original os.Args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set test program name
	os.Args = []string{"file-shifter"}

	// This should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("printUsage() panicked: %v", r)
		}
	}()

	printUsage()
	// If we reach here, the function didn't panic
}

// Benchmark tests for performance
func BenchmarkParseCLI(b *testing.B) {
	// Backup original os.Args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"test", "--log-level", "DEBUG", "--input", "/test", "--outputs", `[{"path":"./out","type":"filesystem"}]`}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset flags for each iteration
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		ParseCLI()
	}
}

func BenchmarkValidate(b *testing.B) {
	cli := &CLIConfig{
		LogLevel:    "DEBUG",
		Input:       "/test/input",
		OutputsJSON: `[{"path":"./backup","type":"filesystem"},{"path":"s3://bucket","type":"s3"}]`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cli.Validate()
	}
}

func BenchmarkApplyToCfg(b *testing.B) {
	cli := &CLIConfig{
		LogLevel:    "DEBUG",
		Input:       "/test/input",
		OutputsJSON: `[{"path":"./backup","type":"filesystem"}]`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create fresh config for each iteration
		testCfg := &EnvConfig{
			Log: struct {
				Level string `yaml:"level"`
			}{Level: "INFO"},
			Input:  "./input",
			Output: []OutputTarget{},
		}
		cli.ApplyToCfg(testCfg)
	}
}
