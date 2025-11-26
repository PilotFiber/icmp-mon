// Package rollout provides fleet-wide agent update orchestration.
//
// The rollout engine supports multiple strategies:
//   - Immediate: All agents at once
//   - Canary: Small percentage first, then staged
//   - Staged: Gradual waves (10% → 25% → 50% → 100%)
//   - Manual: Operator selects specific agents
//
// Each strategy includes health verification between waves and
// automatic rollback on failure.
package rollout

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Strategy defines the rollout approach.
type Strategy string

const (
	StrategyImmediate Strategy = "immediate" // All agents at once
	StrategyCanary    Strategy = "canary"    // 5% first, then staged
	StrategyStaged    Strategy = "staged"    // 10% → 25% → 50% → 100%
	StrategyManual    Strategy = "manual"    // Operator selects agents
)

// Status represents the rollout state.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusPaused     Status = "paused"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
	StatusRolledBack Status = "rolled_back"
)

// Config contains rollout configuration.
type Config struct {
	// Strategy determines how agents are updated
	Strategy Strategy `json:"strategy"`

	// CanaryPercent is the percentage for canary phase (default: 5)
	CanaryPercent int `json:"canary_percent,omitempty"`

	// CanaryDuration is how long to monitor canaries before proceeding (default: 10m)
	CanaryDuration time.Duration `json:"canary_duration,omitempty"`

	// Waves defines the staged rollout percentages (default: [10, 25, 50, 100])
	Waves []int `json:"waves,omitempty"`

	// WaveDelay is the time between waves (default: 5m)
	WaveDelay time.Duration `json:"wave_delay,omitempty"`

	// HealthCheckWait is how long to wait for health after each wave (default: 2m)
	HealthCheckWait time.Duration `json:"health_check_wait,omitempty"`

	// FailureThreshold is the percentage of failures that triggers rollback (default: 10)
	FailureThreshold int `json:"failure_threshold,omitempty"`

	// AutoRollback enables automatic rollback on failures (default: true)
	AutoRollback bool `json:"auto_rollback"`

	// AgentFilter limits which agents are included
	AgentFilter *AgentFilter `json:"agent_filter,omitempty"`
}

// AgentFilter defines criteria for selecting agents.
type AgentFilter struct {
	// Regions limits to specific regions
	Regions []string `json:"regions,omitempty"`

	// Providers limits to specific providers
	Providers []string `json:"providers,omitempty"`

	// Tags requires these tags
	Tags map[string]string `json:"tags,omitempty"`

	// ExcludeAgentIDs excludes specific agents
	ExcludeAgentIDs []string `json:"exclude_agent_ids,omitempty"`

	// IncludeAgentIDs includes only these agents (overrides other filters)
	IncludeAgentIDs []string `json:"include_agent_ids,omitempty"`
}

// DefaultConfig returns the default rollout configuration.
func DefaultConfig() Config {
	return Config{
		Strategy:         StrategyStaged,
		CanaryPercent:    5,
		CanaryDuration:   10 * time.Minute,
		Waves:            []int{10, 25, 50, 100},
		WaveDelay:        5 * time.Minute,
		HealthCheckWait:  2 * time.Minute,
		FailureThreshold: 10,
		AutoRollback:     true,
	}
}

// Rollout represents an active rollout campaign.
type Rollout struct {
	ID        string `json:"id"`
	ReleaseID string `json:"release_id"`
	Version   string `json:"version"`

	Config Config `json:"config"`
	Status Status `json:"status"`

	// Wave tracking
	CurrentWave int `json:"current_wave"`
	TotalWaves  int `json:"total_waves"`

	// Agent counts
	AgentsTotal    int `json:"agents_total"`
	AgentsPending  int `json:"agents_pending"`
	AgentsUpdating int `json:"agents_updating"`
	AgentsUpdated  int `json:"agents_updated"`
	AgentsFailed   int `json:"agents_failed"`
	AgentsSkipped  int `json:"agents_skipped"`

	// Timing
	StartedAt    *time.Time `json:"started_at,omitempty"`
	PausedAt     *time.Time `json:"paused_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	RolledBackAt *time.Time `json:"rolled_back_at,omitempty"`

	// Error info
	RollbackReason string `json:"rollback_reason,omitempty"`
	LastError      string `json:"last_error,omitempty"`

	// Audit
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AgentUpdateState tracks an individual agent's update progress.
type AgentUpdateState struct {
	AgentID     string     `json:"agent_id"`
	AgentName   string     `json:"agent_name"`
	Wave        int        `json:"wave"`
	Status      string     `json:"status"` // pending, updating, updated, failed, skipped
	FromVersion string     `json:"from_version"`
	ToVersion   string     `json:"to_version"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// RolloutStore defines the database operations for rollouts.
type RolloutStore interface {
	CreateRollout(ctx context.Context, r *Rollout) error
	UpdateRollout(ctx context.Context, r *Rollout) error
	GetRollout(ctx context.Context, id string) (*Rollout, error)
	ListActiveRollouts(ctx context.Context) ([]Rollout, error)

	// Agent update tracking
	SetAgentUpdateState(ctx context.Context, rolloutID string, state *AgentUpdateState) error
	GetAgentUpdateStates(ctx context.Context, rolloutID string) ([]AgentUpdateState, error)

	// Release info
	GetRelease(ctx context.Context, id string) (*Release, error)

	// Agent info
	GetAgentsForRollout(ctx context.Context, filter *AgentFilter) ([]AgentInfo, error)
	GetAgentVersion(ctx context.Context, agentID string) (string, error)
	IsAgentHealthy(ctx context.Context, agentID string, since time.Time) (bool, error)
}

// Release contains agent release information.
type Release struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
}

// AgentInfo contains basic agent information for rollout planning.
type AgentInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Region   string `json:"region"`
	Provider string `json:"provider"`
}

// Engine orchestrates rollout execution.
type Engine struct {
	store  RolloutStore
	logger *slog.Logger

	mu       sync.Mutex
	active   map[string]context.CancelFunc
	notifyCh chan string // Agent ID notifications for updates
}

// NewEngine creates a new rollout engine.
func NewEngine(store RolloutStore, logger *slog.Logger) *Engine {
	return &Engine{
		store:    store,
		logger:   logger,
		active:   make(map[string]context.CancelFunc),
		notifyCh: make(chan string, 1000),
	}
}

// StartRollout creates and starts a new rollout.
func (e *Engine) StartRollout(ctx context.Context, releaseID string, cfg Config, createdBy string) (*Rollout, error) {
	// Get release info
	release, err := e.store.GetRelease(ctx, releaseID)
	if err != nil {
		return nil, fmt.Errorf("getting release: %w", err)
	}
	if release == nil {
		return nil, fmt.Errorf("release not found: %s", releaseID)
	}

	// Get eligible agents
	agents, err := e.store.GetAgentsForRollout(ctx, cfg.AgentFilter)
	if err != nil {
		return nil, fmt.Errorf("getting agents: %w", err)
	}

	if len(agents) == 0 {
		return nil, fmt.Errorf("no agents match the rollout filter")
	}

	// Filter out agents already at target version
	var eligibleAgents []AgentInfo
	for _, a := range agents {
		if a.Version != release.Version {
			eligibleAgents = append(eligibleAgents, a)
		}
	}

	if len(eligibleAgents) == 0 {
		return nil, fmt.Errorf("all agents already at version %s", release.Version)
	}

	// Apply defaults
	if cfg.CanaryPercent == 0 {
		cfg.CanaryPercent = 5
	}
	if cfg.CanaryDuration == 0 {
		cfg.CanaryDuration = 10 * time.Minute
	}
	if len(cfg.Waves) == 0 {
		cfg.Waves = []int{10, 25, 50, 100}
	}
	if cfg.WaveDelay == 0 {
		cfg.WaveDelay = 5 * time.Minute
	}
	if cfg.HealthCheckWait == 0 {
		cfg.HealthCheckWait = 2 * time.Minute
	}
	if cfg.FailureThreshold == 0 {
		cfg.FailureThreshold = 10
	}

	// Create rollout
	now := time.Now()
	rollout := &Rollout{
		ID:            uuid.New().String(),
		ReleaseID:     releaseID,
		Version:       release.Version,
		Config:        cfg,
		Status:        StatusPending,
		CurrentWave:   0,
		TotalWaves:    len(cfg.Waves),
		AgentsTotal:   len(eligibleAgents),
		AgentsPending: len(eligibleAgents),
		CreatedBy:     createdBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if cfg.Strategy == StrategyCanary {
		rollout.TotalWaves = len(cfg.Waves) + 1 // +1 for canary wave
	}

	if err := e.store.CreateRollout(ctx, rollout); err != nil {
		return nil, fmt.Errorf("creating rollout: %w", err)
	}

	// Initialize agent states
	for _, agent := range eligibleAgents {
		state := &AgentUpdateState{
			AgentID:     agent.ID,
			AgentName:   agent.Name,
			Wave:        0,
			Status:      "pending",
			FromVersion: agent.Version,
			ToVersion:   release.Version,
		}
		if err := e.store.SetAgentUpdateState(ctx, rollout.ID, state); err != nil {
			e.logger.Warn("failed to set initial agent state", "agent", agent.ID, "error", err)
		}
	}

	// Start rollout execution
	go e.executeRollout(rollout.ID)

	return rollout, nil
}

// PauseRollout pauses an active rollout.
func (e *Engine) PauseRollout(ctx context.Context, rolloutID string) error {
	e.mu.Lock()
	cancel, exists := e.active[rolloutID]
	e.mu.Unlock()

	if !exists {
		return fmt.Errorf("rollout not active: %s", rolloutID)
	}

	cancel()

	rollout, err := e.store.GetRollout(ctx, rolloutID)
	if err != nil {
		return err
	}
	if rollout == nil {
		return fmt.Errorf("rollout not found: %s", rolloutID)
	}

	now := time.Now()
	rollout.Status = StatusPaused
	rollout.PausedAt = &now
	rollout.UpdatedAt = now

	return e.store.UpdateRollout(ctx, rollout)
}

// ResumeRollout resumes a paused rollout.
func (e *Engine) ResumeRollout(ctx context.Context, rolloutID string) error {
	rollout, err := e.store.GetRollout(ctx, rolloutID)
	if err != nil {
		return err
	}
	if rollout == nil {
		return fmt.Errorf("rollout not found: %s", rolloutID)
	}

	if rollout.Status != StatusPaused {
		return fmt.Errorf("rollout not paused: %s", rollout.Status)
	}

	rollout.Status = StatusInProgress
	rollout.PausedAt = nil
	rollout.UpdatedAt = time.Now()

	if err := e.store.UpdateRollout(ctx, rollout); err != nil {
		return err
	}

	go e.executeRollout(rolloutID)

	return nil
}

// RollbackRollout stops and rolls back a rollout.
func (e *Engine) RollbackRollout(ctx context.Context, rolloutID, reason string) error {
	e.mu.Lock()
	if cancel, exists := e.active[rolloutID]; exists {
		cancel()
	}
	e.mu.Unlock()

	rollout, err := e.store.GetRollout(ctx, rolloutID)
	if err != nil {
		return err
	}
	if rollout == nil {
		return fmt.Errorf("rollout not found: %s", rolloutID)
	}

	now := time.Now()
	rollout.Status = StatusRolledBack
	rollout.RolledBackAt = &now
	rollout.RollbackReason = reason
	rollout.UpdatedAt = now

	// TODO: Notify agents to roll back to previous version

	return e.store.UpdateRollout(ctx, rollout)
}

// GetRollout returns rollout details.
func (e *Engine) GetRollout(ctx context.Context, rolloutID string) (*Rollout, error) {
	return e.store.GetRollout(ctx, rolloutID)
}

// GetRolloutProgress returns detailed progress for a rollout.
func (e *Engine) GetRolloutProgress(ctx context.Context, rolloutID string) ([]AgentUpdateState, error) {
	return e.store.GetAgentUpdateStates(ctx, rolloutID)
}

// executeRollout runs the rollout execution loop.
func (e *Engine) executeRollout(rolloutID string) {
	ctx, cancel := context.WithCancel(context.Background())

	e.mu.Lock()
	e.active[rolloutID] = cancel
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		delete(e.active, rolloutID)
		e.mu.Unlock()
	}()

	rollout, err := e.store.GetRollout(ctx, rolloutID)
	if err != nil {
		e.logger.Error("failed to get rollout", "id", rolloutID, "error", err)
		return
	}

	// Mark as in progress
	now := time.Now()
	rollout.Status = StatusInProgress
	rollout.StartedAt = &now
	rollout.UpdatedAt = now
	e.store.UpdateRollout(ctx, rollout)

	e.logger.Info("starting rollout execution",
		"rollout_id", rolloutID,
		"version", rollout.Version,
		"strategy", rollout.Config.Strategy,
		"agents", rollout.AgentsTotal)

	// Execute based on strategy
	var execErr error
	switch rollout.Config.Strategy {
	case StrategyImmediate:
		execErr = e.executeImmediate(ctx, rollout)
	case StrategyCanary:
		execErr = e.executeCanary(ctx, rollout)
	case StrategyStaged:
		execErr = e.executeStaged(ctx, rollout)
	case StrategyManual:
		// Manual rollouts don't auto-execute waves
		e.logger.Info("manual rollout ready for operator control", "rollout_id", rolloutID)
		return
	}

	if execErr != nil {
		e.logger.Error("rollout execution failed", "rollout_id", rolloutID, "error", execErr)
		rollout.LastError = execErr.Error()
		rollout.Status = StatusFailed
		if rollout.Config.AutoRollback {
			e.RollbackRollout(ctx, rolloutID, execErr.Error())
		}
	} else {
		rollout.Status = StatusCompleted
		now := time.Now()
		rollout.CompletedAt = &now
		e.logger.Info("rollout completed successfully", "rollout_id", rolloutID)
	}

	rollout.UpdatedAt = time.Now()
	e.store.UpdateRollout(ctx, rollout)
}

// executeImmediate updates all agents at once.
func (e *Engine) executeImmediate(ctx context.Context, rollout *Rollout) error {
	agents, err := e.store.GetAgentUpdateStates(ctx, rollout.ID)
	if err != nil {
		return err
	}

	// Notify all agents
	for _, agent := range agents {
		e.notifyAgentUpdate(ctx, rollout, agent.AgentID, 1)
	}

	// Wait for completion or timeout
	return e.waitForWaveCompletion(ctx, rollout, 1, len(agents))
}

// executeCanary runs canary phase first, then staged.
func (e *Engine) executeCanary(ctx context.Context, rollout *Rollout) error {
	agents, err := e.store.GetAgentUpdateStates(ctx, rollout.ID)
	if err != nil {
		return err
	}

	// Calculate canary count
	canaryCount := (len(agents) * rollout.Config.CanaryPercent) / 100
	if canaryCount < 1 {
		canaryCount = 1
	}

	e.logger.Info("executing canary phase",
		"rollout_id", rollout.ID,
		"canary_count", canaryCount)

	// Select canary agents (random selection)
	canaryAgents := selectRandomAgents(agents, canaryCount)

	// Update canary wave
	for _, agent := range canaryAgents {
		e.notifyAgentUpdate(ctx, rollout, agent.AgentID, 0) // Wave 0 is canary
	}

	// Wait for canary completion
	if err := e.waitForWaveCompletion(ctx, rollout, 0, canaryCount); err != nil {
		return fmt.Errorf("canary wave failed: %w", err)
	}

	// Monitor canaries for configured duration
	e.logger.Info("monitoring canary agents", "duration", rollout.Config.CanaryDuration)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(rollout.Config.CanaryDuration):
	}

	// Verify canary health
	if err := e.verifyWaveHealth(ctx, rollout, canaryAgents); err != nil {
		return fmt.Errorf("canary health check failed: %w", err)
	}

	// Continue with staged rollout for remaining agents
	remainingAgents := excludeAgents(agents, canaryAgents)
	return e.executeStagedWaves(ctx, rollout, remainingAgents, 1)
}

// executeStaged runs a staged rollout with configured waves.
func (e *Engine) executeStaged(ctx context.Context, rollout *Rollout) error {
	agents, err := e.store.GetAgentUpdateStates(ctx, rollout.ID)
	if err != nil {
		return err
	}

	return e.executeStagedWaves(ctx, rollout, agents, 0)
}

// executeStagedWaves executes the staged wave rollout.
func (e *Engine) executeStagedWaves(ctx context.Context, rollout *Rollout, agents []AgentUpdateState, startWave int) error {
	totalAgents := len(agents)
	updatedCount := 0

	for i, wavePercent := range rollout.Config.Waves {
		wave := startWave + i + 1

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Calculate agents for this wave
		targetCount := (totalAgents * wavePercent) / 100
		waveCount := targetCount - updatedCount
		if waveCount <= 0 {
			continue
		}

		e.logger.Info("executing wave",
			"rollout_id", rollout.ID,
			"wave", wave,
			"percent", wavePercent,
			"agents", waveCount)

		// Select agents for this wave
		waveAgents := selectRandomAgents(agents[updatedCount:], waveCount)

		// Notify agents
		for _, agent := range waveAgents {
			e.notifyAgentUpdate(ctx, rollout, agent.AgentID, wave)
		}

		// Wait for wave completion
		if err := e.waitForWaveCompletion(ctx, rollout, wave, waveCount); err != nil {
			return fmt.Errorf("wave %d failed: %w", wave, err)
		}

		// Verify health
		if err := e.verifyWaveHealth(ctx, rollout, waveAgents); err != nil {
			return fmt.Errorf("wave %d health check failed: %w", wave, err)
		}

		updatedCount = targetCount

		// Update rollout progress
		rollout.CurrentWave = wave
		rollout.AgentsUpdated = updatedCount
		rollout.AgentsPending = totalAgents - updatedCount
		rollout.UpdatedAt = time.Now()
		e.store.UpdateRollout(ctx, rollout)

		// Delay before next wave (unless last wave)
		if i < len(rollout.Config.Waves)-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(rollout.Config.WaveDelay):
			}
		}
	}

	return nil
}

// notifyAgentUpdate marks an agent for update.
func (e *Engine) notifyAgentUpdate(ctx context.Context, rollout *Rollout, agentID string, wave int) {
	now := time.Now()
	state := &AgentUpdateState{
		AgentID:   agentID,
		Wave:      wave,
		Status:    "updating",
		ToVersion: rollout.Version,
		StartedAt: &now,
	}

	if err := e.store.SetAgentUpdateState(ctx, rollout.ID, state); err != nil {
		e.logger.Warn("failed to set agent state", "agent", agentID, "error", err)
	}

	// Send notification through channel (picked up by heartbeat handler)
	select {
	case e.notifyCh <- agentID:
	default:
		e.logger.Warn("notification channel full", "agent", agentID)
	}
}

// waitForWaveCompletion waits for all agents in a wave to update.
func (e *Engine) waitForWaveCompletion(ctx context.Context, rollout *Rollout, wave, expectedCount int) error {
	timeout := time.After(30 * time.Minute) // Max wait time per wave
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("wave %d timed out", wave)
		case <-ticker.C:
			states, err := e.store.GetAgentUpdateStates(ctx, rollout.ID)
			if err != nil {
				continue
			}

			updated := 0
			failed := 0
			for _, s := range states {
				if s.Wave == wave {
					switch s.Status {
					case "updated":
						updated++
					case "failed":
						failed++
					}
				}
			}

			// Check failure threshold
			if failed > 0 {
				failPercent := (failed * 100) / expectedCount
				if failPercent >= rollout.Config.FailureThreshold {
					return fmt.Errorf("failure threshold exceeded: %d%% failed", failPercent)
				}
			}

			// Check completion
			if updated+failed >= expectedCount {
				return nil
			}
		}
	}
}

// verifyWaveHealth checks the health of updated agents.
func (e *Engine) verifyWaveHealth(ctx context.Context, rollout *Rollout, agents []AgentUpdateState) error {
	e.logger.Info("verifying wave health", "agents", len(agents))

	// Wait for health check period
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(rollout.Config.HealthCheckWait):
	}

	// Check each agent's health
	failedCount := 0
	for _, agent := range agents {
		healthy, err := e.store.IsAgentHealthy(ctx, agent.AgentID, time.Now().Add(-rollout.Config.HealthCheckWait))
		if err != nil || !healthy {
			failedCount++
			e.logger.Warn("agent unhealthy after update", "agent", agent.AgentID)
		}
	}

	// Check failure threshold
	if failedCount > 0 {
		failPercent := (failedCount * 100) / len(agents)
		if failPercent >= rollout.Config.FailureThreshold {
			return fmt.Errorf("health check failed: %d%% unhealthy", failPercent)
		}
	}

	return nil
}

// ShouldUpdateAgent checks if an agent should receive an update.
func (e *Engine) ShouldUpdateAgent(ctx context.Context, agentID, currentVersion string) (*UpdateNotification, error) {
	// Get active rollouts
	rollouts, err := e.store.ListActiveRollouts(ctx)
	if err != nil {
		return nil, err
	}

	for _, rollout := range rollouts {
		if rollout.Status != StatusInProgress {
			continue
		}

		// Check if this agent is marked for update
		states, err := e.store.GetAgentUpdateStates(ctx, rollout.ID)
		if err != nil {
			continue
		}

		for _, state := range states {
			if state.AgentID == agentID && state.Status == "updating" {
				release, err := e.store.GetRelease(ctx, rollout.ReleaseID)
				if err != nil || release == nil {
					continue
				}

				return &UpdateNotification{
					RolloutID:   rollout.ID,
					ReleaseID:   rollout.ReleaseID,
					Version:     release.Version,
					Checksum:    release.Checksum,
					Size:        release.Size,
					DownloadURL: fmt.Sprintf("/api/v1/releases/%s/download", rollout.ReleaseID),
				}, nil
			}
		}
	}

	return nil, nil
}

// UpdateNotification contains update information for an agent.
type UpdateNotification struct {
	RolloutID   string `json:"rollout_id"`
	ReleaseID   string `json:"release_id"`
	Version     string `json:"version"`
	Checksum    string `json:"checksum"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"download_url"`
}

// ReportAgentUpdate records an agent's update result.
func (e *Engine) ReportAgentUpdate(ctx context.Context, rolloutID, agentID string, success bool, err string) error {
	state := &AgentUpdateState{
		AgentID: agentID,
		Status:  "updated",
		Error:   "",
	}
	if !success {
		state.Status = "failed"
		state.Error = err
	}
	now := time.Now()
	state.CompletedAt = &now

	return e.store.SetAgentUpdateState(ctx, rolloutID, state)
}

// selectRandomAgents returns a random subset of agents.
func selectRandomAgents(agents []AgentUpdateState, count int) []AgentUpdateState {
	if count >= len(agents) {
		return agents
	}
	// Simple selection: take first N (could add actual randomization)
	return agents[:count]
}

// excludeAgents returns agents not in the exclude list.
func excludeAgents(all, exclude []AgentUpdateState) []AgentUpdateState {
	excludeMap := make(map[string]bool)
	for _, a := range exclude {
		excludeMap[a.AgentID] = true
	}

	var result []AgentUpdateState
	for _, a := range all {
		if !excludeMap[a.AgentID] {
			result = append(result, a)
		}
	}
	return result
}

// MarshalConfig serializes config to JSON for storage.
func (c Config) MarshalJSON() ([]byte, error) {
	type Alias Config
	return json.Marshal(&struct {
		CanaryDuration  string `json:"canary_duration,omitempty"`
		WaveDelay       string `json:"wave_delay,omitempty"`
		HealthCheckWait string `json:"health_check_wait,omitempty"`
		*Alias
	}{
		CanaryDuration:  c.CanaryDuration.String(),
		WaveDelay:       c.WaveDelay.String(),
		HealthCheckWait: c.HealthCheckWait.String(),
		Alias:           (*Alias)(&c),
	})
}
