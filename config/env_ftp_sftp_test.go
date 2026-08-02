package config

import (
	"os"
	"testing"
)

// yamlEnvSecurityKeys lists the lowercase YAML-style environment variables
// used by these tests so they can be cleared reliably.
var yamlEnvSecurityKeys = []string{
	"output.0.path", "output.0.type", "output.0.host", "output.0.username",
	"output.0.password", "output.0.tls", "output.0.known_hosts",
	"output.0.insecure_skip_host_key_verification", "output.0.port", "output.0.ssl",
}

func clearYAMLEnvSecurityKeys() {
	for _, key := range yamlEnvSecurityKeys {
		os.Unsetenv(key)
	}
}

// TestEnvConfig_LoadTargetProperties_SFTPSecurityEnv covers loading of TLS,
// KNOWN_HOSTS and INSECURE_SKIP_HOST_KEY_VERIFICATION from the flat
// OUTPUT_N_* environment structure.
func TestEnvConfig_LoadTargetProperties_SFTPSecurityEnv(t *testing.T) {
	originalEnv := backupEnvironment()
	defer restoreEnvironment(originalEnv)
	clearTestEnvironment()
	clearYAMLEnvSecurityKeys()

	setEnv := func(key, value string) {
		if err := os.Setenv(key, value); err != nil {
			t.Fatalf("failed to set %s: %v", key, err)
		}
	}

	setEnv("OUTPUT_1_PATH", "sftp://sftp.example.com/upload")
	setEnv("OUTPUT_1_TYPE", "sftp")
	setEnv("OUTPUT_1_HOST", "sftp.example.com")
	setEnv("OUTPUT_1_USERNAME", "user")
	setEnv("OUTPUT_1_PASSWORD", "secret")
	setEnv("OUTPUT_1_TLS", "TRUE") // parsed case-insensitively
	setEnv("OUTPUT_1_KNOWN_HOSTS", "/etc/ssh/test_known_hosts")
	setEnv("OUTPUT_1_INSECURE_SKIP_HOST_KEY_VERIFICATION", "True")

	cfg := &EnvConfig{}
	if err := cfg.LoadFromEnvironment(); err != nil {
		t.Fatalf("LoadFromEnvironment failed: %v", err)
	}

	if len(cfg.Output) != 1 {
		t.Fatalf("expected 1 output target, got %d", len(cfg.Output))
	}
	target := cfg.Output[0]
	if target.Type != "sftp" {
		t.Errorf("Type = %q, want %q", target.Type, "sftp")
	}
	if !target.TLS {
		t.Error("expected TLS to be true")
	}
	if target.KnownHosts != "/etc/ssh/test_known_hosts" {
		t.Errorf("KnownHosts = %q, want %q", target.KnownHosts, "/etc/ssh/test_known_hosts")
	}
	if !target.InsecureSkipHostKeyVerify {
		t.Error("expected InsecureSkipHostKeyVerify to be true")
	}
}

// TestEnvConfig_LoadTargetProperties_SecurityFlagsFalse covers the non-"true"
// values of the boolean security flags in the flat environment structure.
func TestEnvConfig_LoadTargetProperties_SecurityFlagsFalse(t *testing.T) {
	originalEnv := backupEnvironment()
	defer restoreEnvironment(originalEnv)
	clearTestEnvironment()
	clearYAMLEnvSecurityKeys()

	setEnv := func(key, value string) {
		if err := os.Setenv(key, value); err != nil {
			t.Fatalf("failed to set %s: %v", key, err)
		}
	}

	setEnv("OUTPUT_1_PATH", "ftp://ftp.example.com/upload")
	setEnv("OUTPUT_1_TYPE", "ftp")
	setEnv("OUTPUT_1_TLS", "false")
	setEnv("OUTPUT_1_INSECURE_SKIP_HOST_KEY_VERIFICATION", "no")

	cfg := &EnvConfig{}
	if err := cfg.LoadFromEnvironment(); err != nil {
		t.Fatalf("LoadFromEnvironment failed: %v", err)
	}

	if len(cfg.Output) != 1 {
		t.Fatalf("expected 1 output target, got %d", len(cfg.Output))
	}
	target := cfg.Output[0]
	if target.TLS {
		t.Error("expected TLS to be false")
	}
	if target.InsecureSkipHostKeyVerify {
		t.Error("expected InsecureSkipHostKeyVerify to be false for non-true value")
	}
	if target.KnownHosts != "" {
		t.Errorf("expected empty KnownHosts, got %q", target.KnownHosts)
	}
}

// TestEnvConfig_ReadYAMLOutputTarget_SecurityFields covers the lowercase
// YAML-style environment variant (output.N.tls, output.N.known_hosts,
// output.N.insecure_skip_host_key_verification).
func TestEnvConfig_ReadYAMLOutputTarget_SecurityFields(t *testing.T) {
	originalEnv := backupEnvironment()
	defer restoreEnvironment(originalEnv)
	defer clearYAMLEnvSecurityKeys()
	clearTestEnvironment()
	clearYAMLEnvSecurityKeys()

	yamlEnv := map[string]string{
		"output.0.path":        "sftp://sftp.example.com/drop",
		"output.0.type":        "sftp",
		"output.0.host":        "sftp.example.com",
		"output.0.username":    "yamluser",
		"output.0.password":    "yamlpass",
		"output.0.tls":         "True",
		"output.0.known_hosts": "/home/user/.ssh/known_hosts",
		"output.0.insecure_skip_host_key_verification": "TRUE",
		"output.0.port": "2222",
	}
	for key, value := range yamlEnv {
		if err := os.Setenv(key, value); err != nil {
			t.Fatalf("failed to set %s: %v", key, err)
		}
	}

	cfg := &EnvConfig{}
	if err := cfg.LoadFromEnvironment(); err != nil {
		t.Fatalf("LoadFromEnvironment failed: %v", err)
	}

	if len(cfg.Output) != 1 {
		t.Fatalf("expected 1 output target, got %d", len(cfg.Output))
	}
	target := cfg.Output[0]
	if target.Path != "sftp://sftp.example.com/drop" {
		t.Errorf("Path = %q, want %q", target.Path, "sftp://sftp.example.com/drop")
	}
	if !target.TLS {
		t.Error("expected TLS to be true from lowercase env variant")
	}
	if target.KnownHosts != "/home/user/.ssh/known_hosts" {
		t.Errorf("KnownHosts = %q, want %q", target.KnownHosts, "/home/user/.ssh/known_hosts")
	}
	if !target.InsecureSkipHostKeyVerify {
		t.Error("expected InsecureSkipHostKeyVerify to be true from lowercase env variant")
	}
	if target.Port != 2222 {
		t.Errorf("Port = %d, want 2222", target.Port)
	}

	// A second run with the boolean flags set to non-true values must yield
	// false (covers the false branches of the lowercase variant).
	if err := os.Setenv("output.0.tls", "off"); err != nil {
		t.Fatalf("failed to update output.0.tls: %v", err)
	}
	if err := os.Setenv("output.0.insecure_skip_host_key_verification", "0"); err != nil {
		t.Fatalf("failed to update output.0.insecure_skip_host_key_verification: %v", err)
	}

	cfgFalse := &EnvConfig{}
	if err := cfgFalse.LoadFromEnvironment(); err != nil {
		t.Fatalf("LoadFromEnvironment failed: %v", err)
	}
	if len(cfgFalse.Output) != 1 {
		t.Fatalf("expected 1 output target, got %d", len(cfgFalse.Output))
	}
	if cfgFalse.Output[0].TLS {
		t.Error("expected TLS to be false for non-true lowercase value")
	}
	if cfgFalse.Output[0].InsecureSkipHostKeyVerify {
		t.Error("expected InsecureSkipHostKeyVerify to be false for non-true lowercase value")
	}
}

// TestEnvConfig_HealthPortFromEnv covers loading HEALTH_PORT from the
// environment including rejection of non-positive values.
func TestEnvConfig_HealthPortFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected int
	}{
		{name: "valid port is applied", value: "9095", expected: 9095},
		{name: "non-numeric value is ignored", value: "not-a-port", expected: 0},
		{name: "negative value is ignored", value: "-2", expected: 0},
		{name: "zero is ignored", value: "0", expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalEnv := backupEnvironment()
			defer restoreEnvironment(originalEnv)
			clearTestEnvironment()
			if err := os.Setenv("HEALTH_PORT", tt.value); err != nil {
				t.Fatalf("failed to set HEALTH_PORT: %v", err)
			}
			defer os.Unsetenv("HEALTH_PORT")

			cfg := &EnvConfig{}
			if err := cfg.LoadFromEnvironment(); err != nil {
				t.Fatalf("LoadFromEnvironment failed: %v", err)
			}
			if cfg.Health.Port != tt.expected {
				t.Fatalf("Health.Port = %d, want %d", cfg.Health.Port, tt.expected)
			}
		})
	}
}
