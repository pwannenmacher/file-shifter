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
		MaxRetries      int `yaml:"max-retries"`      // Maximum number of repetitions in case of file instability
		CheckInterval   int `yaml:"check-interval"`   // Check interval in milliseconds
		StabilityPeriod int `yaml:"stability-period"` // Period during which a file must remain stable in milliseconds
	} `yaml:"file-stability"`
	WorkerPool struct {
		Workers   int `yaml:"workers"`    // Number of parallel workers
		QueueSize int `yaml:"queue-size"` // Size of the file queue
	} `yaml:"worker-pool"`
}

// LoadFromEnvironment loads the configuration from environment variables
func (c *EnvConfig) LoadFromEnvironment() error {
	if logLevel := firstNonEmptyEnv("LOG_LEVEL", "log.level"); logLevel != "" {
		c.Log.Level = logLevel
	}

	if inputDir := firstNonEmptyEnv("INPUT", "input"); inputDir != "" {
		c.Input = inputDir
	}

	// File Stability Configuration - support different formats
	c.loadFileStabilityFromEnv()

	// Worker Pool Configuration - support different formats
	c.loadWorkerPoolFromEnv()

	// Output Targets - flat structure
	c.loadOutputTargetsFromEnv()

	// Output Targets - YAML-structure from env
	if len(c.Output) == 0 {
		c.loadOutputFromYAMLEnv()
	}

	// Output Targets - JSON/YAML structure as fallback
	if len(c.Output) == 0 {
		if targets := parseOutputTargetsEnv("OUTPUTS"); len(targets) > 0 {
			c.Output = targets
		}
	}

	return nil
}

// loadOutputTargetsFromEnv loads output targets from the new flat ENV structure
func (c *EnvConfig) loadOutputTargetsFromEnv() {
	targetMap := make(map[string]*OutputTarget)

	for _, env := range os.Environ() {
		key, value, ok := splitEnvVar(env)
		if !ok {
			continue
		}
		indexStr, ok := outputPathIndex(key)
		if !ok {
			continue
		}
		if targetMap[indexStr] == nil {
			targetMap[indexStr] = &OutputTarget{}
		}
		targetMap[indexStr].Path = value
	}

	// Load additional properties for each target
	for index, target := range targetMap {
		c.loadTargetProperties(target, index)
	}

	if targets := outputTargetsFromMap(targetMap); len(targets) > 0 {
		c.Output = targets
	}
}

// loadTargetProperties loads all properties for a target based on its index
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
		target.SSL = toBoolPtr(strings.ToLower(value) == "true")
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
	c.FileStability.MaxRetries = readPositiveIntEnv(c.FileStability.MaxRetries, "FILE_STABILITY_MAX_RETRIES", "file_stability.max_retries")
	c.FileStability.CheckInterval = readPositiveIntEnv(c.FileStability.CheckInterval, "FILE_STABILITY_CHECK_INTERVAL", "file_stability.check_interval")
	c.FileStability.StabilityPeriod = readPositiveIntEnv(c.FileStability.StabilityPeriod, "FILE_STABILITY_PERIOD", "file_stability.period")
}

// loadWorkerPoolFromEnv lädt die Worker-Pool-Konfiguration aus Umgebungsvariablen
func (c *EnvConfig) loadWorkerPoolFromEnv() {
	c.WorkerPool.Workers = readPositiveIntEnv(c.WorkerPool.Workers, "WORKER_POOL_WORKERS", "worker_pool.workers")
	c.WorkerPool.QueueSize = readPositiveIntEnv(c.WorkerPool.QueueSize, "WORKER_POOL_QUEUE_SIZE", "worker_pool.queue_size")
}

// loadOutputFromYAMLEnv lädt Output-Targets aus YAML-strukturierten Umgebungsvariablen
func (c *EnvConfig) loadOutputFromYAMLEnv() {
	var targets []OutputTarget
	for targetIndex := 0; ; targetIndex++ {
		target, ok := readYAMLOutputTarget(targetIndex)
		if !ok {
			break
		}
		targets = append(targets, target)
	}

	if len(targets) > 0 {
		c.Output = targets
	}
}

func splitEnvVar(env string) (string, string, bool) {
	parts := strings.SplitN(env, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func outputPathIndex(key string) (string, bool) {
	if !strings.HasPrefix(key, "OUTPUT_") || !strings.HasSuffix(key, "_PATH") {
		return "", false
	}
	indexStr := strings.TrimPrefix(key, "OUTPUT_")
	indexStr = strings.TrimSuffix(indexStr, "_PATH")
	if indexStr == "" {
		return "", false
	}
	return indexStr, true
}

func outputTargetsFromMap(targetMap map[string]*OutputTarget) []OutputTarget {
	var targets []OutputTarget
	for _, target := range targetMap {
		if target.Path != "" {
			targets = append(targets, *target)
		}
	}
	return targets
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func parseOutputTargetsEnv(key string) []OutputTarget {
	outputTargetsStr := os.Getenv(key)
	if outputTargetsStr == "" {
		return nil
	}

	var targets []OutputTarget
	if err := json.Unmarshal([]byte(outputTargetsStr), &targets); err == nil {
		return targets
	}
	if err := yaml.Unmarshal([]byte(outputTargetsStr), &targets); err == nil {
		return targets
	}
	return nil
}

func readPositiveIntEnv(defaultValue int, keys ...string) int {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
				defaultValue = parsed
			}
		}
	}
	return defaultValue
}

func readYAMLOutputTarget(index int) (OutputTarget, bool) {
	path := os.Getenv(fmt.Sprintf("output.%d.path", index))
	targetType := os.Getenv(fmt.Sprintf("output.%d.type", index))
	if path == "" || targetType == "" {
		return OutputTarget{}, false
	}

	target := OutputTarget{
		Path: path,
		Type: targetType,
	}

	target.Endpoint = os.Getenv(fmt.Sprintf("output.%d.endpoint", index))
	target.AccessKey = os.Getenv(fmt.Sprintf("output.%d.access_key", index))
	target.SecretKey = os.Getenv(fmt.Sprintf("output.%d.secret_key", index))
	target.Region = os.Getenv(fmt.Sprintf("output.%d.region", index))
	target.Host = os.Getenv(fmt.Sprintf("output.%d.host", index))
	target.Username = os.Getenv(fmt.Sprintf("output.%d.username", index))
	target.Password = os.Getenv(fmt.Sprintf("output.%d.password", index))

	if sslStr := os.Getenv(fmt.Sprintf("output.%d.ssl", index)); sslStr != "" {
		target.SSL = toBoolPtr(strings.ToLower(sslStr) == "true")
	}
	if portStr := os.Getenv(fmt.Sprintf("output.%d.port", index)); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			target.Port = port
		}
	}

	return target, true
}

func toBoolPtr(value bool) *bool {
	return &value
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
		c.FileStability.CheckInterval = 1000 // 1000ms = 1 Sekunde
	}
	if c.FileStability.StabilityPeriod == 0 {
		c.FileStability.StabilityPeriod = 1000 // 1000ms = 1 Sekunde
	}
	// Worker Pool Defaults
	if c.WorkerPool.Workers == 0 {
		c.WorkerPool.Workers = 4 // 4 parallele Worker
	}
	if c.WorkerPool.QueueSize == 0 {
		c.WorkerPool.QueueSize = 100 // 100 Dateien in der Warteschlange
	}
}

// Validate checks the configuration for completeness.
func (c *EnvConfig) Validate() error {
	if c.Input == "" {
		return os.ErrInvalid
	}

	// Check that at least one target is configured.
	if len(c.Output) == 0 {
		return os.ErrInvalid
	}

	return nil
}

// GetLogLevel returns the configured log level.
func (c *EnvConfig) GetLogLevel() string {
	level := strings.ToUpper(c.Log.Level)
	switch level {
	case "DEBUG", "INFO", "WARN", "ERROR":
		return level
	default:
		return "INFO"
	}
}
