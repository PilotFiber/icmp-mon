// Command agent runs the ICMP monitoring agent.
//
// # Usage
//
//	agent --control-plane https://monitor.pilot.net --name my-agent-01
//
// # Configuration
//
// Configuration can be provided via:
// - Command-line flags
// - Environment variables (ICMPMON_*)
// - Config file (--config)
//
// # Examples
//
// Run with flags:
//
//	agent --control-plane https://monitor.pilot.net \
//	      --name aws-us-east-01 \
//	      --region us-east \
//	      --location "AWS us-east-1a" \
//	      --provider aws
//
// Run with config file:
//
//	agent --config /etc/icmpmon/agent.yaml
//
// Run with environment variables:
//
//	ICMPMON_CONTROL_PLANE_URL=https://monitor.pilot.net \
//	ICMPMON_AGENT_NAME=my-agent \
//	agent
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/pilot-net/icmp-mon/agent"
	"github.com/pilot-net/icmp-mon/agent/internal/config"
)

func main() {
	// Parse flags
	var (
		configFile   = flag.String("config", "", "Path to config file")
		controlPlane = flag.String("control-plane", "", "Control plane URL")
		token        = flag.String("token", "", "Authentication token")
		name         = flag.String("name", "", "Agent name")
		region       = flag.String("region", "", "Agent region")
		location     = flag.String("location", "", "Agent location (human-readable)")
		provider     = flag.String("provider", "", "Provider name (aws, vultr, etc.)")
		debug        = flag.Bool("debug", false, "Enable debug logging")
		version      = flag.Bool("version", false, "Print version and exit")
	)
	flag.Parse()

	// Print version
	if *version {
		fmt.Printf("icmpmon-agent %s\n", agent.Version)
		os.Exit(0)
	}

	// Set up logging
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Load configuration
	cfg := config.DefaultConfig()

	// Load from file if specified
	if *configFile != "" {
		fileCfg, err := config.LoadFromFile(*configFile)
		if err != nil {
			logger.Error("failed to load config file", "error", err)
			os.Exit(1)
		}
		cfg = fileCfg
	}

	// Apply environment overrides
	cfg.ApplyEnvOverrides()

	// Apply flag overrides
	if *controlPlane != "" {
		cfg.ControlPlane.URL = *controlPlane
	}
	if *token != "" {
		cfg.ControlPlane.Token = *token
	}
	if *name != "" {
		cfg.Agent.Name = *name
	}
	if *region != "" {
		cfg.Agent.Region = *region
	}
	if *location != "" {
		cfg.Agent.Location = *location
	}
	if *provider != "" {
		cfg.Agent.Provider = *provider
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	// Create agent
	a, err := agent.New(cfg, logger)
	if err != nil {
		logger.Error("failed to create agent", "error", err)
		os.Exit(1)
	}

	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received shutdown signal", "signal", sig)
		cancel()
	}()

	// Run agent
	logger.Info("starting icmpmon agent",
		"name", cfg.Agent.Name,
		"control_plane", cfg.ControlPlane.URL)

	if err := a.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("agent exited with error", "error", err)
		os.Exit(1)
	}

	logger.Info("agent shutdown complete")
}
