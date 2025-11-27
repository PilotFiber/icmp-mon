// Package agent provides the main agent implementation.
//
// # Agent Lifecycle
//
//  1. Load configuration
//  2. Register with control plane
//  3. Fetch initial assignments
//  4. Start probe loops (one per tier)
//  5. Start result shipper
//  6. Start heartbeat loop
//  7. Run until shutdown signal
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/pilot-net/icmp-mon/agent/internal/client"
	"github.com/pilot-net/icmp-mon/agent/internal/config"
	"github.com/pilot-net/icmp-mon/agent/internal/executor"
	"github.com/pilot-net/icmp-mon/agent/internal/scheduler"
	"github.com/pilot-net/icmp-mon/agent/internal/shipper"
	"github.com/pilot-net/icmp-mon/agent/internal/updater"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// Version is set at build time.
var Version = "dev"

// Agent is the main monitoring agent.
type Agent struct {
	cfg       *config.Config
	client    *client.Client
	registry  *executor.Registry
	scheduler *scheduler.Scheduler
	shipper   *shipper.Shipper
	updater   *updater.Updater
	logger    *slog.Logger

	// State
	agentID           string
	assignmentVersion int64
	startTime         time.Time

	// Control
	mu sync.Mutex
}

// New creates a new agent with the given configuration.
func New(cfg *config.Config, logger *slog.Logger) (*Agent, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}

	// Create executor registry
	registry := executor.NewRegistry()

	// Register built-in executors
	icmpExec := executor.NewICMPExecutor()
	if cfg.Probing.FpingPath != "" {
		icmpExec.FpingPath = cfg.Probing.FpingPath
	}
	if err := registry.Register(icmpExec); err != nil {
		logger.Warn("failed to register ICMP executor", "error", err)
		// Continue without ICMP if fping not available
	} else {
		logger.Info("registered executor", "type", "icmp_ping")
	}

	// Register MTR executor for on-demand path tracing
	mtrExec := executor.NewMTRExecutor()
	if err := registry.Register(mtrExec); err != nil {
		logger.Warn("failed to register MTR executor", "error", err)
		// Continue without MTR if mtr not available
	} else {
		logger.Info("registered executor", "type", "mtr")
	}

	logger.Info("executor registry ready", "executors", registry.List())

	// Create control plane client
	cpClient := client.NewClient(client.Config{
		BaseURL:   cfg.ControlPlane.URL,
		AuthToken: cfg.ControlPlane.Token,
	})

	// Create updater for self-updates
	agentUpdater := updater.New(updater.Config{
		InstallDir: "/usr/local/bin",
		BinaryName: "icmpmon-agent",
		Logger:     logger,
	})

	a := &Agent{
		cfg:       cfg,
		client:    cpClient,
		registry:  registry,
		updater:   agentUpdater,
		logger:    logger,
		startTime: time.Now(),
	}

	return a, nil
}

// Run starts the agent and blocks until context is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	a.logger.Info("starting agent",
		"name", a.cfg.Agent.Name,
		"version", Version,
		"region", a.cfg.Agent.Region)

	// Register with control plane
	if err := a.register(ctx); err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	// Create shipper
	a.shipper = shipper.NewShipper(shipper.Config{
		Endpoint:     a.cfg.ControlPlane.URL + "/api/v1/results",
		AgentID:      a.agentID,
		BatchSize:    a.cfg.Probing.ResultBatchSize,
		BatchTimeout: a.cfg.Probing.ResultBatchTimeout,
		Logger:       a.logger,
	})

	// Create scheduler with result handler
	a.scheduler = scheduler.NewScheduler(
		a.registry,
		func(results []*executor.Result) {
			a.shipper.Add(results)
		},
		a.logger,
	)

	// Set default tiers (will be overridden by control plane)
	a.scheduler.SetTiers(defaultTiers())

	// Fetch initial assignments
	if err := a.syncAssignments(ctx); err != nil {
		a.logger.Warn("failed to fetch initial assignments", "error", err)
		// Continue anyway, will retry
	}

	// Run all loops concurrently
	errCh := make(chan error, 5)

	go func() {
		errCh <- a.runScheduler(ctx)
	}()

	go func() {
		errCh <- a.shipper.Run(ctx)
	}()

	go func() {
		errCh <- a.runHeartbeat(ctx)
	}()

	go func() {
		errCh <- a.runAssignmentSync(ctx)
	}()

	go func() {
		errCh <- a.runCommandPolling(ctx)
	}()

	// Wait for first error or context cancellation
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// register registers the agent with the control plane.
func (a *Agent) register(ctx context.Context) error {
	publicIP := getPublicIP()

	req := client.RegisterRequest{
		Name:       a.cfg.Agent.Name,
		Region:     a.cfg.Agent.Region,
		Location:   a.cfg.Agent.Location,
		Provider:   a.cfg.Agent.Provider,
		Tags:       a.cfg.Agent.Tags,
		PublicIP:   publicIP,
		Version:    Version,
		Executors:  a.registry.List(),
		MaxTargets: 10000, // TODO: Make configurable
	}

	resp, err := a.client.Register(ctx, req)
	if err != nil {
		return err
	}

	a.agentID = resp.AgentID
	a.logger.Info("registered with control plane",
		"agent_id", a.agentID,
		"public_ip", publicIP)

	return nil
}

// runScheduler runs the probe scheduler.
func (a *Agent) runScheduler(ctx context.Context) error {
	return a.scheduler.Run(ctx)
}

// runHeartbeat sends periodic heartbeats to the control plane.
func (a *Agent) runHeartbeat(ctx context.Context) error {
	ticker := time.NewTicker(a.cfg.Health.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := a.sendHeartbeat(ctx); err != nil {
				a.logger.Warn("heartbeat failed", "error", err)
			}
		}
	}
}

// sendHeartbeat sends a single heartbeat.
func (a *Agent) sendHeartbeat(ctx context.Context) error {
	stats := a.scheduler.Stats()
	shipperStats := a.shipper.Stats()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	heartbeat := types.Heartbeat{
		AgentID:           a.agentID,
		Timestamp:         time.Now(),
		Version:           Version,
		Status:            types.AgentStatusActive,
		ActiveTargets:     stats.TotalTargets,
		ResultsQueued:     shipperStats.Queued,
		ResultsShipped:    shipperStats.Shipped,
		MemoryMB:          float64(m.Alloc) / 1024 / 1024,
		GoroutineCount:    runtime.NumGoroutine(),
		AssignmentVersion: a.assignmentVersion,
		PublicIP:          getPublicIP(),
	}

	resp, err := a.client.Heartbeat(ctx, heartbeat)
	if err != nil {
		return err
	}

	// Check if we need to re-sync assignments
	if resp.AssignmentStale {
		a.logger.Info("assignment refresh requested")
		go a.syncAssignments(context.Background())
	}

	// Execute any commands from heartbeat response
	for _, cmd := range resp.Commands {
		go a.executeCommand(context.Background(), cmd)
	}

	// Check for available updates
	if resp.UpdateAvailable != nil {
		go a.handleUpdate(context.Background(), resp.UpdateAvailable)
	}

	return nil
}

// runAssignmentSync periodically syncs assignments from the control plane.
func (a *Agent) runAssignmentSync(ctx context.Context) error {
	ticker := time.NewTicker(a.cfg.Probing.AssignmentPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := a.syncAssignments(ctx); err != nil {
				a.logger.Warn("assignment sync failed", "error", err)
			}
		}
	}
}

// syncAssignments fetches and applies new assignments.
func (a *Agent) syncAssignments(ctx context.Context) error {
	assignSet, err := a.client.GetAssignments(ctx, a.assignmentVersion)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.assignmentVersion = assignSet.Version
	a.mu.Unlock()

	a.scheduler.UpdateAssignments(assignSet.Assignments)

	a.logger.Info("assignments synced",
		"version", assignSet.Version,
		"count", len(assignSet.Assignments))

	return nil
}

// runCommandPolling polls for and executes on-demand commands.
func (a *Agent) runCommandPolling(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second) // Poll every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			commands, err := a.client.GetCommands(ctx)
			if err != nil {
				a.logger.Debug("command poll failed", "error", err)
				continue
			}

			for _, cmd := range commands {
				go a.executeCommand(context.Background(), cmd)
			}
		}
	}
}

// executeCommand executes an on-demand command.
func (a *Agent) executeCommand(ctx context.Context, cmd types.Command) {
	a.logger.Info("executing command",
		"id", cmd.ID,
		"type", cmd.Type,
		"target", cmd.TargetIP)

	start := time.Now()
	var result types.CommandResult
	result.CommandID = cmd.ID
	result.AgentID = a.agentID

	switch cmd.Type {
	case "mtr":
		result = a.executeMTR(ctx, cmd)
	default:
		result.Success = false
		result.Error = fmt.Sprintf("unknown command type: %s", cmd.Type)
	}

	result.Duration = time.Since(start)
	result.CompletedAt = time.Now()

	if err := a.client.ReportCommandResult(ctx, result); err != nil {
		a.logger.Error("failed to report command result",
			"command", cmd.ID,
			"error", err)
	} else {
		a.logger.Info("command completed",
			"command", cmd.ID,
			"success", result.Success,
			"duration", result.Duration)
	}
}

// executeMTR runs an MTR trace for a command.
func (a *Agent) executeMTR(ctx context.Context, cmd types.Command) types.CommandResult {
	result := types.CommandResult{
		CommandID: cmd.ID,
		AgentID:   a.agentID,
	}

	// Get MTR executor
	exec, ok := a.registry.Get("mtr")
	if !ok {
		result.Success = false
		result.Error = "MTR executor not available"
		return result
	}

	// Build target
	target := executor.ProbeTarget{
		ID:      cmd.ID,
		IP:      cmd.TargetIP,
		Timeout: 30 * time.Second,
	}

	// Execute MTR
	mtrResult, err := exec.Execute(ctx, target)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return result
	}

	result.Success = mtrResult.Success
	result.Error = mtrResult.Error
	result.Payload = mtrResult.Payload

	return result
}

// handleUpdate handles an available update from the control plane.
func (a *Agent) handleUpdate(ctx context.Context, info *types.UpdateInfo) {
	// Skip if already updating
	if a.updater.IsUpdating() {
		a.logger.Debug("update already in progress, skipping")
		return
	}

	// Skip if we're already at this version
	if info.Version == Version {
		a.logger.Debug("already running target version", "version", Version)
		return
	}

	a.logger.Info("update available",
		"current_version", Version,
		"new_version", info.Version,
		"mandatory", info.Mandatory)

	// Perform the update
	if err := a.updater.Update(ctx, info); err != nil {
		a.logger.Error("update failed", "error", err)
		// TODO: Report update failure to control plane
		return
	}

	a.logger.Info("update installed, requesting restart")

	// Request restart via systemd
	if err := a.updater.RequestRestart(); err != nil {
		a.logger.Error("restart failed, manual restart required", "error", err)
	}
}

// defaultTiers returns default tier configurations.
// Includes both customer tiers (infrastructure, vip, standard) and
// monitoring state machine tiers (discovery, inactive_recheck, smart_recheck).
func defaultTiers() map[string]types.Tier {
	return map[string]types.Tier{
		// Customer monitoring tiers
		"infrastructure": {
			Name:          "infrastructure",
			DisplayName:   "Infrastructure",
			ProbeInterval: 5 * time.Second,
			ProbeTimeout:  2 * time.Second,
			ProbeRetries:  1,
		},
		"vip": {
			Name:          "vip",
			DisplayName:   "VIP",
			ProbeInterval: 15 * time.Second,
			ProbeTimeout:  3 * time.Second,
			ProbeRetries:  2,
		},
		"standard": {
			Name:          "standard",
			DisplayName:   "Standard",
			ProbeInterval: 30 * time.Second,
			ProbeTimeout:  5 * time.Second,
			ProbeRetries:  0,
		},
		// VLAN Gateway tier - monitors gateway addresses for subnets
		"vlan_gateway": {
			Name:          "vlan_gateway",
			DisplayName:   "VLAN Gateway",
			ProbeInterval: 30 * time.Second,
			ProbeTimeout:  3 * time.Second,
			ProbeRetries:  1,
		},
		// Monitoring state machine tiers
		"discovery": {
			Name:          "discovery",
			DisplayName:   "Discovery",
			ProbeInterval: 30 * time.Second, // TODO: Change to 5*time.Minute for production
			ProbeTimeout:  5 * time.Second,
			ProbeRetries:  0,
		},
		"inactive_recheck": {
			Name:          "inactive_recheck",
			DisplayName:   "Inactive Recheck",
			ProbeInterval: 1 * time.Hour,
			ProbeTimeout:  5 * time.Second,
			ProbeRetries:  0,
		},
		"smart_recheck": {
			Name:          "smart_recheck",
			DisplayName:   "Smart Recheck",
			ProbeInterval: 24 * time.Hour,
			ProbeTimeout:  5 * time.Second,
			ProbeRetries:  0,
		},
	}
}

// getPublicIP attempts to determine the agent's public IP.
func getPublicIP() string {
	// Try to get from environment first
	if ip := os.Getenv("ICMPMON_PUBLIC_IP"); ip != "" {
		return ip
	}

	// Try to detect by connecting to a known address
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
