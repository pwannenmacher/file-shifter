package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

// CLIConfig holds command line argument configuration
type CLIConfig struct {
	LogLevel    string
	Input       string
	OutputsJSON string
	HealthPort  int
	ShowHelp    bool
}

// ParseCLI parses command line arguments and returns a CLIConfig
func ParseCLI() *CLIConfig {
	cfg := &CLIConfig{}

	// Define flags
	flag.StringVar(&cfg.LogLevel, "log-level", "", "Set log level (DEBUG, INFO, WARN, ERROR)")
	flag.StringVar(&cfg.Input, "input", "", "Set input directory")
	flag.StringVar(&cfg.OutputsJSON, "outputs", "", "Set output targets as JSON array")
	flag.IntVar(&cfg.HealthPort, "health-port", 0, "Set port for the health check HTTP server")
	flag.BoolVar(&cfg.ShowHelp, "help", false, "Show help message")

	// Also handle short forms and alternative help flags
	flag.BoolVar(&cfg.ShowHelp, "h", false, "Show help message")

	// Custom usage function
	flag.Usage = printUsage

	// Check for help flags before parsing
	for _, arg := range os.Args[1:] {
		if arg == "-h" || arg == "--help" {
			cfg.ShowHelp = true
			printUsage()
			os.Exit(0)
		}
	}

	// Parse flags
	flag.Parse()

	return cfg
}

// ApplyToCfg applies CLI configuration to EnvConfig
func (cli *CLIConfig) ApplyToCfg(cfg *EnvConfig) error {
	// Apply log level
	if cli.LogLevel != "" {
		cfg.Log.Level = cli.LogLevel
	}

	// Apply input directory
	if cli.Input != "" {
		cfg.Input = cli.Input
	}

	// Apply outputs JSON
	if cli.OutputsJSON != "" {
		var targets []OutputTarget
		if err := json.Unmarshal([]byte(cli.OutputsJSON), &targets); err != nil {
			return fmt.Errorf("error parsing --outputs JSON: %w", err)
		}
		cfg.Output = targets
	}

	// Apply health port
	if cli.HealthPort != 0 {
		cfg.Health.Port = cli.HealthPort
	}

	return nil
}

// printUsage prints the usage information
func printUsage() {
	_, err := fmt.Fprintf(os.Stderr, `File Shifter - Robuster File-Transfer-Service

USAGE:
    %s [OPTIONS]

OPTIONS:
    --log-level LEVEL    Set log level (DEBUG, INFO, WARN, ERROR)
                        Default: INFO
    
    --input DIRECTORY    Set input directory to watch for files
                        Default: ./input
    
    --outputs JSON       Set output targets as JSON array
                        Format: [{"path":"./output1","type":"filesystem"},...]
                        Supported types: filesystem, s3, sftp, ftp
                        
                        Filesystem example:
                        [{"path":"./backup","type":"filesystem"}]
                        
                        S3 example:
                        [{"path":"s3://bucket/prefix","type":"s3",
                          "endpoint":"s3.amazonaws.com","access-key":"KEY",
                          "secret-key":"SECRET","ssl":true,"region":"eu-central-1"}]
                        
                        SFTP example:
                        [{"path":"sftp://server/path","type":"sftp",
                          "host":"server.com","username":"user","password":"pass"}]

    --health-port PORT   Set port for the health check HTTP server
                        Default: 8080

    -h, --help           Show this help message

SECURITY:
    Credentials passed via --outputs are visible to all local users in the
    process list (ps). Prefer environment variables, a .env file or
    env.yaml for secrets (access keys, passwords).

EXAMPLES:
    # Basic filesystem backup
    %s --input ./data --outputs '[{"path":"./backup","type":"filesystem"}]'
    
    # Multi-target with S3 and filesystem
    %s --outputs '[{"path":"./local","type":"filesystem"},{"path":"s3://bucket/files","type":"s3","endpoint":"localhost:9000","access-key":"minioadmin","secret-key":"minioadmin","ssl":false,"region":"us-east-1"}]'
    
    # Debug mode with custom input
    %s --log-level DEBUG --input /data/incoming

CONFIGURATION PRIORITY:
    1. Command line arguments (highest)
    2. Environment variables
    3. env.yaml/env.yml file
    4. Default values (lowest)

ENVIRONMENT VARIABLES:
    LOG_LEVEL            Same as --log-level
    INPUT                Same as --input
    HEALTH_PORT          Same as --health-port
    OUTPUT_1_PATH        First output target path
    OUTPUT_1_TYPE        First output target type
    ...                  Additional OUTPUT_X_* variables
    
    Legacy format:
    INPUT_DIRECTORY      Legacy input directory
    OUTPUT_TARGETS       JSON array (legacy format)

For more configuration options, see the README.md or create an env.yaml file.

`, os.Args[0], os.Args[0], os.Args[0], os.Args[0])
	if err != nil {
		return
	}
}

// HasOutputsConfigured checks if outputs are configured via CLI
func (cli *CLIConfig) HasOutputsConfigured() bool {
	return cli.OutputsJSON != ""
}

// HasInlineCredentials reports whether the --outputs JSON appears to contain
// secrets, which are visible in the process list
func (cli *CLIConfig) HasInlineCredentials() bool {
	if cli.OutputsJSON == "" {
		return false
	}

	var targets []OutputTarget
	if err := json.Unmarshal([]byte(cli.OutputsJSON), &targets); err != nil {
		return false
	}

	for _, target := range targets {
		if target.SecretKey != "" || target.AccessKey != "" || target.Password != "" {
			return true
		}
	}
	return false
}

// Validate validates CLI configuration
func (cli *CLIConfig) Validate() error {
	if err := validateLogLevel(cli.LogLevel); err != nil {
		return err
	}

	if err := validateOutputsJSON(cli.OutputsJSON); err != nil {
		return err
	}

	if err := validateHealthPort(cli.HealthPort); err != nil {
		return err
	}

	return nil
}

func validateHealthPort(port int) error {
	if port == 0 {
		return nil
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid health port: %d (allowed: 1-65535)", port)
	}

	return nil
}

func validateLogLevel(level string) error {
	if level == "" {
		return nil
	}

	normalized := strings.ToUpper(level)
	if normalized != "DEBUG" && normalized != "INFO" && normalized != "WARN" && normalized != "ERROR" {
		return fmt.Errorf("invalid log level: %s (allowed: DEBUG, INFO, WARN, ERROR)", level)
	}

	return nil
}

func validateOutputsJSON(outputsJSON string) error {
	if outputsJSON == "" {
		return nil
	}

	var targets []OutputTarget
	if err := json.Unmarshal([]byte(outputsJSON), &targets); err != nil {
		return fmt.Errorf("invalid --outputs JSON format: %w", err)
	}

	for i, target := range targets {
		if err := validateOutputTarget(target, i); err != nil {
			return err
		}
	}

	return nil
}

func validateOutputTarget(target OutputTarget, index int) error {
	if target.Path == "" {
		return fmt.Errorf("output target %d: 'path' is required", index+1)
	}
	if target.Type == "" {
		return fmt.Errorf("output target %d: 'type' is required", index+1)
	}
	if target.Type != "filesystem" && target.Type != "s3" && target.Type != "sftp" && target.Type != "ftp" {
		return fmt.Errorf("output target %d: invalid type '%s' (allowed: filesystem, s3, sftp, ftp)", index+1, target.Type)
	}

	return nil
}
