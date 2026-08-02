package config

import (
	"strings"
	"testing"
)

// TestCLIConfig_ApplyToCfg_HealthPortOverride covers the health-port override
// in ApplyToCfg: a non-zero CLI value replaces the configured port, zero
// leaves it untouched.
func TestCLIConfig_ApplyToCfg_HealthPortOverride(t *testing.T) {
	t.Run("non-zero CLI port overrides config", func(t *testing.T) {
		cfg := &EnvConfig{}
		cfg.SetDefaults()

		cli := &CLIConfig{HealthPort: 9091}
		if err := cli.ApplyToCfg(cfg); err != nil {
			t.Fatalf("ApplyToCfg failed: %v", err)
		}
		if cfg.Health.Port != 9091 {
			t.Fatalf("expected health port 9091, got %d", cfg.Health.Port)
		}
	})

	t.Run("zero CLI port keeps configured value", func(t *testing.T) {
		cfg := &EnvConfig{}
		cfg.SetDefaults()
		configured := cfg.Health.Port

		cli := &CLIConfig{HealthPort: 0}
		if err := cli.ApplyToCfg(cfg); err != nil {
			t.Fatalf("ApplyToCfg failed: %v", err)
		}
		if cfg.Health.Port != configured {
			t.Fatalf("expected health port to stay %d, got %d", configured, cfg.Health.Port)
		}
	})
}

// TestCLIConfig_Validate_HealthPort covers validateHealthPort via
// CLIConfig.Validate for valid, unset and out-of-range values.
func TestCLIConfig_Validate_HealthPort(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{name: "zero means unset and is valid", port: 0, wantErr: false},
		{name: "lowest valid port", port: 1, wantErr: false},
		{name: "highest valid port", port: 65535, wantErr: false},
		{name: "typical port", port: 8080, wantErr: false},
		{name: "negative port is invalid", port: -1, wantErr: true},
		{name: "port above 65535 is invalid", port: 65536, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := &CLIConfig{HealthPort: tt.port}
			err := cli.Validate()

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected validation error for port %d", tt.port)
				}
				if !strings.Contains(err.Error(), "invalid health port") {
					t.Fatalf("expected health port error, got: %v", err)
				}
				if !strings.Contains(err.Error(), "allowed: 1-65535") {
					t.Fatalf("expected allowed range in error, got: %v", err)
				}
			} else if err != nil {
				t.Fatalf("expected port %d to be valid, got: %v", tt.port, err)
			}
		})
	}
}

// TestValidateHealthPort_Direct covers the validateHealthPort helper directly.
func TestValidateHealthPort_Direct(t *testing.T) {
	if err := validateHealthPort(0); err != nil {
		t.Fatalf("expected 0 (unset) to be valid, got: %v", err)
	}
	if err := validateHealthPort(443); err != nil {
		t.Fatalf("expected 443 to be valid, got: %v", err)
	}
	if err := validateHealthPort(-5); err == nil {
		t.Fatal("expected error for negative port")
	}
	if err := validateHealthPort(70000); err == nil {
		t.Fatal("expected error for port above 65535")
	}
}

// TestEnvConfig_Validate_HealthPortBounds covers the health port bounds check
// in EnvConfig.Validate including the exact error message and the
// zero-means-default behavior.
func TestEnvConfig_Validate_HealthPortBounds(t *testing.T) {
	newValidConfig := func(port int) *EnvConfig {
		cfg := &EnvConfig{}
		cfg.Input = "./input"
		cfg.Output = OutputConfig{{Path: "./output", Type: "filesystem"}}
		cfg.Health.Port = port
		return cfg
	}

	t.Run("negative port is rejected with full message", func(t *testing.T) {
		err := newValidConfig(-1).Validate()
		if err == nil {
			t.Fatal("expected validation error for negative health port")
		}
		if !strings.Contains(err.Error(), "(allowed: 1-65535, or 0 for the default)") {
			t.Fatalf("expected allowed-range message, got: %v", err)
		}
	})

	t.Run("port above 65535 is rejected", func(t *testing.T) {
		if err := newValidConfig(65536).Validate(); err == nil {
			t.Fatal("expected validation error for health port above 65535")
		}
	})

	t.Run("zero passes validation and defaults to 8080", func(t *testing.T) {
		cfg := newValidConfig(0)
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected zero health port to pass validation, got: %v", err)
		}
		cfg.SetDefaults()
		if cfg.Health.Port != 8080 {
			t.Fatalf("expected default health port 8080, got %d", cfg.Health.Port)
		}
	})

	t.Run("valid port passes validation", func(t *testing.T) {
		if err := newValidConfig(9090).Validate(); err != nil {
			t.Fatalf("expected valid health port to pass, got: %v", err)
		}
	})
}
