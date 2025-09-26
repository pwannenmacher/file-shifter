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

func loadEnvYaml() (*config.EnvConfig, error) {
	// Prüfe welche Dateien vorhanden sind
	yamlExists := fileExists("env.yaml")
	ymlExists := fileExists("env.yml")

	// Fehler wenn beide Dateien vorhanden sind
	if yamlExists && ymlExists {
		return nil, fmt.Errorf("konflikt: sowohl env.yaml als auch env.yml sind vorhanden, bitte verwende nur eine der beiden Dateien")
	}

	// Bestimme welche Datei geladen werden soll
	var configFile string
	if yamlExists {
		configFile = "env.yaml"
	} else if ymlExists {
		configFile = "env.yml"
	} else {
		return nil, fmt.Errorf("keine Konfigurationsdatei gefunden (env.yaml oder env.yml)")
	}

	// Lade die Datei
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("fehler beim Lesen von %s: %w", configFile, err)
	}

	var cfg config.EnvConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("fehler beim Parsen von %s: %w", configFile, err)
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

func main() {
	// 1. Command Line Arguments parsen
	cliCfg := config.ParseCLI()

	// Validiere CLI-Konfiguration
	if err := cliCfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Fehler in Kommandozeilen-Argumenten: %v\n", err)
		os.Exit(1)
	}

	// 2. Reihenfolge der Konfiguration:
	// - env.yaml oder env.yml laden (falls vorhanden)
	// - .env laden (falls vorhanden)
	// - Umgebungsvariablen laden
	// - CLI-Parameter anwenden (überschreibt alles andere)

	cfg, err := loadEnvYaml()
	if err != nil {
		fmt.Println("Konfigurationsdatei konnte nicht geladen werden:", err)
		cfg = &config.EnvConfig{} // leere Konfiguration
	}

	// .env laden (optional)
	_ = godotenv.Load()

	// Defaults setzen
	cfg.SetDefaults()

	// Umgebungsvariablen laden (überschreibt YAML und .env)
	err = cfg.LoadFromEnvironment()
	if err != nil {
		fmt.Println("Fehler beim Laden der Umgebungsvariablen:", err)
	}

	// CLI-Parameter anwenden (höchste Priorität)
	err = cliCfg.ApplyToCfg(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fehler beim Anwenden der CLI-Parameter: %v\n", err)
		os.Exit(1)
	}

	// Logger-Konfiguration
	setupLogger(cfg)

	// Input Directory
	inputDir := cfg.Input

	// Output Targets
	outputTargets := cfg.Output

	// Standard-Default falls keine Targets konfiguriert
	if len(outputTargets) == 0 {
		outputTargets = []config.OutputTarget{
			{
				Path: "./output",
				Type: "filesystem",
			},
		}
		cfg.Output = outputTargets // Auch in cfg setzen für Validierung
		slog.Info("Keine Output-Konfiguration gefunden - verwende Standard-Default", "target", "./output")
	}

	// Konfiguration validieren (nach dem Setzen der Standard-Targets)
	if err := cfg.Validate(); err != nil {
		slog.Error("Ungültige Konfiguration", "error", err)
		os.Exit(1)
	}

	// Worker initialisieren und starten
	workerService := services.NewWorker(inputDir, outputTargets, cfg)

	// Graceful Shutdown Handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		slog.Info("Shutdown-Signal empfangen...")
		workerService.Stop()
	}()

	// Worker starten (blockiert bis Stop aufgerufen wird)
	workerService.Start()
}
