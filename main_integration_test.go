package main

import (
	"os"
	"syscall"
	"testing"

	"file-shifter/config"
)

type fakeWorker struct {
	started bool
	stopped bool
	done    chan struct{}
}

func (w *fakeWorker) Start() {
	w.started = true
	<-w.done
}

func (w *fakeWorker) Stop() {
	if !w.stopped {
		w.stopped = true
		close(w.done)
	}
}

type fakeHealthMonitor struct {
	started bool
	stopped bool
}

func (h *fakeHealthMonitor) Start() {
	h.started = true
}

func (h *fakeHealthMonitor) Stop() {
	h.stopped = true
}

func TestRunApp_CLIValidationError(t *testing.T) {
	code := runApp(
		func() *config.CLIConfig { return &config.CLIConfig{LogLevel: "INVALID"} },
		func() (*config.EnvConfig, error) { return &config.EnvConfig{}, nil },
		func() error { return nil },
		func(string, []config.OutputTarget, *config.EnvConfig) workerService {
			return &fakeWorker{done: make(chan struct{})}
		},
		func(workerService, string) healthService { return &fakeHealthMonitor{} },
		func(chan<- os.Signal, ...os.Signal) {},
	)

	if code != 1 {
		t.Fatalf("expected exit code 1 for invalid CLI config, got %d", code)
	}
}

func TestRunApp_ApplyCLIError(t *testing.T) {
	code := runApp(
		func() *config.CLIConfig { return &config.CLIConfig{OutputsJSON: "{"} },
		func() (*config.EnvConfig, error) { return &config.EnvConfig{}, nil },
		func() error { return nil },
		func(string, []config.OutputTarget, *config.EnvConfig) workerService {
			return &fakeWorker{done: make(chan struct{})}
		},
		func(workerService, string) healthService { return &fakeHealthMonitor{} },
		func(chan<- os.Signal, ...os.Signal) {},
	)

	if code != 1 {
		t.Fatalf("expected exit code 1 for invalid outputs JSON, got %d", code)
	}
}

func TestRunApp_DefaultOutputsAndShutdownFlow(t *testing.T) {
	worker := &fakeWorker{done: make(chan struct{})}
	health := &fakeHealthMonitor{}

	var capturedTargets []config.OutputTarget
	var capturedInput string

	code := runApp(
		func() *config.CLIConfig { return &config.CLIConfig{} },
		func() (*config.EnvConfig, error) { return nil, os.ErrNotExist },
		func() error { return nil },
		func(input string, targets []config.OutputTarget, _ *config.EnvConfig) workerService {
			capturedInput = input
			capturedTargets = targets
			return worker
		},
		func(_ workerService, _ string) healthService { return health },
		func(ch chan<- os.Signal, _ ...os.Signal) {
			go func() { ch <- syscall.SIGTERM }()
		},
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !worker.started || !worker.stopped {
		t.Fatalf("expected worker start/stop to be called, started=%v stopped=%v", worker.started, worker.stopped)
	}
	if !health.started || !health.stopped {
		t.Fatalf("expected health monitor start/stop to be called, started=%v stopped=%v", health.started, health.stopped)
	}
	if capturedInput == "" {
		t.Fatal("expected input directory to be set by defaults")
	}
	if len(capturedTargets) != 1 || capturedTargets[0].Type != "filesystem" || capturedTargets[0].Path != "./output" {
		t.Fatalf("expected default filesystem output target, got %+v", capturedTargets)
	}
}

func TestRunApp_UsesConfiguredOutputsWithoutDefaultFallback(t *testing.T) {
	worker := &fakeWorker{done: make(chan struct{})}
	health := &fakeHealthMonitor{}

	configured := &config.EnvConfig{}
	configured.SetDefaults()
	configured.Output = []config.OutputTarget{{Type: "filesystem", Path: "/tmp/custom"}}

	var capturedTargets []config.OutputTarget

	code := runApp(
		func() *config.CLIConfig { return &config.CLIConfig{} },
		func() (*config.EnvConfig, error) { return configured, nil },
		func() error { return nil },
		func(_ string, targets []config.OutputTarget, _ *config.EnvConfig) workerService {
			capturedTargets = targets
			return worker
		},
		func(_ workerService, _ string) healthService { return health },
		func(ch chan<- os.Signal, _ ...os.Signal) {
			go func() { ch <- syscall.SIGINT }()
		},
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if len(capturedTargets) != 1 || capturedTargets[0].Path != "/tmp/custom" {
		t.Fatalf("expected configured outputs to be preserved, got %+v", capturedTargets)
	}
}
