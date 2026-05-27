package main

import (
	"file-shifter/config"
	"file-shifter/services"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type workerService interface {
	Start()
	Stop()
}

type healthService interface {
	Start()
	Stop()
}

type realWorkerService struct {
	worker *services.Worker
}

func (w *realWorkerService) Start() {
	w.worker.Start()
}

func (w *realWorkerService) Stop() {
	w.worker.Stop()
}

func newRealWorkerService(inputDir string, outputTargets []config.OutputTarget, cfg *config.EnvConfig) workerService {
	return &realWorkerService{worker: services.NewWorker(inputDir, outputTargets, cfg)}
}

func newRealHealthService(worker workerService, port string) healthService {
	realWorker, ok := worker.(*realWorkerService)
	if !ok {
		panic("invalid worker service implementation")
	}
	return services.NewHealthMonitor(realWorker.worker, port)
}

func loadEnvYaml() (*config.EnvConfig, error) {
	// Check which files are available
	yamlExists := fileExists("env.yaml")
	ymlExists := fileExists("env.yml")

	// Error if both files exist
	if yamlExists && ymlExists {
		return nil, fmt.Errorf("conflict: both env.yaml and env.yml are present, please use only one of the two files")
	}

	// Determine which file should be loaded
	var configFile string
	if yamlExists {
		configFile = "env.yaml"
	} else if ymlExists {
		configFile = "env.yml"
	} else {
		return nil, fmt.Errorf("no configuration file found (env.yaml or env.yml)")
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %w", configFile, err)
	}

	var cfg config.EnvConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", configFile, err)
	}

	return &cfg, nil
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func setupLogger(cfg *config.EnvConfig) {
	levelStr := cfg.GetLogLevel()
	var lvl slog.Level
	switch levelStr {
	case "DEBUG":
		lvl = slog.LevelDebug
	case "INFO":
		lvl = slog.LevelInfo
	case "WARN":
		lvl = slog.LevelWarn
	case "ERROR":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	logger := slog.New(handler)
	slog.SetDefault(logger)
}

func runApp(
	parseCLI func() *config.CLIConfig,
	loadEnvYamlFunc func() (*config.EnvConfig, error),
	loadDotEnv func() error,
	createWorker func(string, []config.OutputTarget, *config.EnvConfig) workerService,
	createHealthMonitor func(workerService, string) healthService,
	notifySignals func(chan<- os.Signal, ...os.Signal),
) int {
	cliCfg := parseCLI()

	// Validate CLI configuration
	if err := cliCfg.Validate(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Fehler in Kommandozeilen-Argumenten: %v\n", err)
		return 1
	}

	// 2. Configuration order:
	// - Load env.yaml or env.yml (if available)
	// - Load .env (if available)
	// - Load environment variables
	// - Apply CLI parameters (overrides everything else)

	cfg, err := loadEnvYamlFunc()
	if err != nil {
		fmt.Println("Konfigurationsdatei konnte nicht geladen werden:", err)
		cfg = &config.EnvConfig{} // leere Konfiguration
	}

	_ = loadDotEnv()

	// Set defaults
	cfg.SetDefaults()

	// Load environment variables (overwrites YAML and .env)
	err = cfg.LoadFromEnvironment()
	if err != nil {
		fmt.Println("Error loading environment variables:", err)
	}

	// Apply CLI parameters (highest priority)
	err = cliCfg.ApplyToCfg(cfg)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error applying CLI parameters: %v\n", err)
		return 1
	}

	// Logger configuration
	setupLogger(cfg)

	// Input Directory
	inputDir := cfg.Input

	// Output Targets
	outputTargets := cfg.Output

	// Standard default if no targets are configured
	if len(outputTargets) == 0 {
		outputTargets = []config.OutputTarget{
			{
				Path: "./output",
				Type: "filesystem",
			},
		}
		cfg.Output = outputTargets // Also set in cfg for validation
		slog.Info("No output configuration found - use standard default", "target", "./output")
	}

	// Validate configuration (after setting the default targets)
	if err := cfg.Validate(); err != nil {
		slog.Error("Invalid configuration", "error", err)
		return 1
	}

	// Initialise and start workers
	workerService := createWorker(inputDir, outputTargets, cfg)

	// Start Health-Monitor
	healthMonitor := createHealthMonitor(workerService, "8080")
	healthMonitor.Start()

	// Graceful Shutdown Handler
	sigChan := make(chan os.Signal, 1)
	notifySignals(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		slog.Info("Shutdown signal received...")
		healthMonitor.Stop()
		workerService.Stop()
	}()

	// Start worker (blocked until Stop is called)
	workerService.Start()
	return 0
}

func main() {
	code := runApp(
		config.ParseCLI,
		loadEnvYaml,
		func() error { return godotenv.Load() },
		newRealWorkerService,
		newRealHealthService,
		signal.Notify,
	)
	if code != 0 {
		os.Exit(code)
	}
}
