package config

import (
	"encoding/json"
	"os"
	"strings"
)

type EnvConfig struct {
	Log struct {
		Level string `yaml:"level"`
	} `yaml:"log"`
	Input  string       `yaml:"input"`
	Output OutputConfig `yaml:"output"`
}

// LoadFromEnvironment lädt die Konfiguration aus Umgebungsvariablen
func (c *EnvConfig) LoadFromEnvironment() error {
	// Log Level
	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		c.Log.Level = logLevel
	}

	// Input Directory - neue und alte Struktur unterstützen
	if inputDir := os.Getenv("INPUT"); inputDir != "" {
		c.Input = inputDir
	} else if inputDir := os.Getenv("INPUT"); inputDir != "" {
		c.Input = inputDir // Fallback für alte Struktur
	}

	// Output Targets - neue flache Struktur
	c.loadOutputTargetsFromEnv()

	// Output Targets - alte JSON-Struktur als Fallback
	if len(c.Output) == 0 {
		if outputTargetsJSON := os.Getenv("OUTPUTS"); outputTargetsJSON != "" {
			var targets []OutputTarget
			if err := json.Unmarshal([]byte(outputTargetsJSON), &targets); err == nil {
				c.Output = targets
			}
		}
	}

	return nil
}

// loadOutputTargetsFromEnv lädt Output-Targets aus der neuen flachen ENV-Struktur
func (c *EnvConfig) loadOutputTargetsFromEnv() {
	targetMap := make(map[string]*OutputTarget)

	// Iteriere durch alle Umgebungsvariablen und suche OUTPUT_X_* Pattern
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "OUTPUT_") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) != 2 {
				continue
			}

			key := parts[0]
			value := parts[1]

			// Parse OUTPUT_X_PATH Pattern
			if strings.HasSuffix(key, "_PATH") {
				// Extrahiere Index (z.B. "1" aus "OUTPUT_1_PATH")
				indexStr := strings.TrimPrefix(key, "OUTPUT_")
				indexStr = strings.TrimSuffix(indexStr, "_PATH")

				// Erstelle oder finde das entsprechende Target
				if targetMap[indexStr] == nil {
					targetMap[indexStr] = &OutputTarget{}
				}
				targetMap[indexStr].Path = value
			}
		}
	}

	// Lade zusätzliche Eigenschaften für jedes Target
	for index, target := range targetMap {
		c.loadTargetProperties(target, index)
	}

	// Konvertiere Map zu Slice
	var targets []OutputTarget
	for _, target := range targetMap {
		if target.Path != "" { // Nur Targets mit gesetztem Path hinzufügen
			targets = append(targets, *target)
		}
	}

	if len(targets) > 0 {
		c.Output = targets
	}
}

// loadTargetProperties lädt alle Eigenschaften für ein Target basierend auf seinem Index
func (c *EnvConfig) loadTargetProperties(target *OutputTarget, index string) {
	prefix := "OUTPUT_" + index + "_"

	// Grundlegende Eigenschaften
	if value := os.Getenv(prefix + "TYPE"); value != "" {
		target.Type = value
	}

	// S3-spezifische Eigenschaften
	if value := os.Getenv(prefix + "ENDPOINT"); value != "" {
		target.Endpoint = value
	}
	if value := os.Getenv(prefix + "ACCESS_KEY"); value != "" {
		target.AccessKey = value
	}
	if value := os.Getenv(prefix + "SECRET_KEY"); value != "" {
		target.SecretKey = value
	}
	if value := os.Getenv(prefix + "SSL"); value != "" {
		ssl := strings.ToLower(value) == "true"
		target.SSL = &ssl
	}
	if value := os.Getenv(prefix + "REGION"); value != "" {
		target.Region = value
	}

	// FTP/SFTP-spezifische Eigenschaften
	if value := os.Getenv(prefix + "HOST"); value != "" {
		target.Host = value
	}
	if value := os.Getenv(prefix + "USERNAME"); value != "" {
		target.Username = value
	}
	if value := os.Getenv(prefix + "PASSWORD"); value != "" {
		target.Password = value
	}
}

// SetDefaults setzt Standard-Werte für die Konfiguration
func (c *EnvConfig) SetDefaults() {
	if c.Log.Level == "" {
		c.Log.Level = "INFO"
	}
	if c.Input == "" {
		c.Input = "./input"
	}
}

// Validate überprüft die Konfiguration auf Vollständigkeit
func (c *EnvConfig) Validate() error {
	if c.Input == "" {
		return os.ErrInvalid
	}

	// Überprüfe ob mindestens ein Target konfiguriert ist
	if len(c.Output) == 0 {
		return os.ErrInvalid
	}

	return nil
}

// GetLogLevel gibt das konfigurierte Log-Level zurück
func (c *EnvConfig) GetLogLevel() string {
	level := strings.ToUpper(c.Log.Level)
	switch level {
	case "DEBUG", "INFO", "WARN", "ERROR":
		return level
	default:
		return "INFO"
	}
}
