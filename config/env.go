package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type EnvConfig struct {
	Log struct {
		Level string `yaml:"level"`
	} `yaml:"log"`
	Input         string       `yaml:"input"`
	Output        OutputConfig `yaml:"output"`
	FileStability struct {
		MaxRetries      int `yaml:"max-retries"`      // Maximum Anzahl Wiederholungen
		CheckInterval   int `yaml:"check-interval"`   // Prüf-Intervall in Sekunden
		StabilityPeriod int `yaml:"stability-period"` // Stabilität-Prüfung in Sekunden
	} `yaml:"file-stability"`
}

// LoadFromEnvironment lädt die Konfiguration aus Umgebungsvariablen
func (c *EnvConfig) LoadFromEnvironment() error {
	// Log Level - verschiedene Formate unterstützen
	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		c.Log.Level = logLevel
	} else if logLevel := os.Getenv("log.level"); logLevel != "" {
		c.Log.Level = logLevel
	}

	// Input Directory - verschiedene Formate unterstützen
	if inputDir := os.Getenv("INPUT"); inputDir != "" {
		c.Input = inputDir
	} else if inputDir := os.Getenv("input"); inputDir != "" {
		c.Input = inputDir
	}

	// File Stability Konfiguration - verschiedene Formate unterstützen
	c.loadFileStabilityFromEnv()

	// Output Targets - neue flache Struktur
	c.loadOutputTargetsFromEnv()

	// Output Targets - YAML-Struktur aus Umgebungsvariablen
	if len(c.Output) == 0 {
		c.loadOutputFromYAMLEnv()
	}

	// Output Targets - alte JSON/YAML-Struktur als Fallback
	if len(c.Output) == 0 {
		if outputTargetsStr := os.Getenv("OUTPUTS"); outputTargetsStr != "" {
			// Zuerst als JSON versuchen
			var targets []OutputTarget
			if err := json.Unmarshal([]byte(outputTargetsStr), &targets); err == nil {
				c.Output = targets
			} else {
				// Falls JSON fehlschlägt, als YAML versuchen
				if err := yaml.Unmarshal([]byte(outputTargetsStr), &targets); err == nil {
					c.Output = targets
				}
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

// loadFileStabilityFromEnv lädt File-Stability Konfiguration aus Umgebungsvariablen
func (c *EnvConfig) loadFileStabilityFromEnv() {
	// Alte Struktur (FILE_STABILITY_*)
	if maxRetries := os.Getenv("FILE_STABILITY_MAX_RETRIES"); maxRetries != "" {
		if val, err := strconv.Atoi(maxRetries); err == nil && val > 0 {
			c.FileStability.MaxRetries = val
		}
	}

	if checkInterval := os.Getenv("FILE_STABILITY_CHECK_INTERVAL"); checkInterval != "" {
		if val, err := strconv.Atoi(checkInterval); err == nil && val > 0 {
			c.FileStability.CheckInterval = val
		}
	}

	if stabilityPeriod := os.Getenv("FILE_STABILITY_PERIOD"); stabilityPeriod != "" {
		if val, err := strconv.Atoi(stabilityPeriod); err == nil && val > 0 {
			c.FileStability.StabilityPeriod = val
		}
	}

	// Neue Struktur (file_stability.*)
	if maxRetries := os.Getenv("file_stability.max_retries"); maxRetries != "" {
		if val, err := strconv.Atoi(maxRetries); err == nil && val > 0 {
			c.FileStability.MaxRetries = val
		}
	}

	if checkInterval := os.Getenv("file_stability.check_interval"); checkInterval != "" {
		if val, err := strconv.Atoi(checkInterval); err == nil && val > 0 {
			c.FileStability.CheckInterval = val
		}
	}

	if period := os.Getenv("file_stability.period"); period != "" {
		if val, err := strconv.Atoi(period); err == nil && val > 0 {
			c.FileStability.StabilityPeriod = val
		}
	}
}

// loadOutputFromYAMLEnv lädt Output-Targets aus YAML-strukturierten Umgebungsvariablen
func (c *EnvConfig) loadOutputFromYAMLEnv() {
	var targets []OutputTarget
	targetIndex := 0

	// Suche nach output.N.* Mustern
	for {
		pathKey := fmt.Sprintf("output.%d.path", targetIndex)
		typeKey := fmt.Sprintf("output.%d.type", targetIndex)

		path := os.Getenv(pathKey)
		targetType := os.Getenv(typeKey)

		if path == "" || targetType == "" {
			break // Keine weiteren Targets
		}

		target := OutputTarget{
			Path: path,
			Type: targetType,
		}

		// S3-spezifische Properties
		if endpoint := os.Getenv(fmt.Sprintf("output.%d.endpoint", targetIndex)); endpoint != "" {
			target.Endpoint = endpoint
		}
		if accessKey := os.Getenv(fmt.Sprintf("output.%d.access_key", targetIndex)); accessKey != "" {
			target.AccessKey = accessKey
		}
		if secretKey := os.Getenv(fmt.Sprintf("output.%d.secret_key", targetIndex)); secretKey != "" {
			target.SecretKey = secretKey
		}
		if sslStr := os.Getenv(fmt.Sprintf("output.%d.ssl", targetIndex)); sslStr != "" {
			ssl := strings.ToLower(sslStr) == "true"
			target.SSL = &ssl
		}
		if region := os.Getenv(fmt.Sprintf("output.%d.region", targetIndex)); region != "" {
			target.Region = region
		}

		// FTP/SFTP-spezifische Properties
		if host := os.Getenv(fmt.Sprintf("output.%d.host", targetIndex)); host != "" {
			target.Host = host
		}
		if username := os.Getenv(fmt.Sprintf("output.%d.username", targetIndex)); username != "" {
			target.Username = username
		}
		if password := os.Getenv(fmt.Sprintf("output.%d.password", targetIndex)); password != "" {
			target.Password = password
		}
		if portStr := os.Getenv(fmt.Sprintf("output.%d.port", targetIndex)); portStr != "" {
			if port, err := strconv.Atoi(portStr); err == nil {
				target.Port = port
			}
		}

		targets = append(targets, target)
		targetIndex++
	}

	if len(targets) > 0 {
		c.Output = targets
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
	// File Stability Defaults
	if c.FileStability.MaxRetries == 0 {
		c.FileStability.MaxRetries = 30 // 30 Versuche
	}
	if c.FileStability.CheckInterval == 0 {
		c.FileStability.CheckInterval = 1 // 1 Sekunde
	}
	if c.FileStability.StabilityPeriod == 0 {
		c.FileStability.StabilityPeriod = 1 // 1 Sekunde
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
