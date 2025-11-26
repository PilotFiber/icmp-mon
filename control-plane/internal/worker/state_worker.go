// Package worker provides background workers for the control plane.
package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/pilot-net/icmp-mon/pkg/types"
)

// StateStore defines the storage interface for the state worker.
type StateStore interface {
	// GetTargetsForDownTransition returns ACTIVE targets that haven't responded
	// within the given threshold (should transition to DOWN).
	// Only includes targets WITH an established baseline (alertable).
	GetTargetsForDownTransition(ctx context.Context, threshold time.Duration) ([]types.Target, error)

	// GetTargetsForUnresponsiveTransition returns ACTIVE targets WITHOUT a baseline
	// that have stopped responding. These should transition to UNRESPONSIVE (not alertable).
	GetTargetsForUnresponsiveTransition(ctx context.Context, threshold time.Duration) ([]types.Target, error)

	// GetTargetsForExcludedTransition returns DOWN targets that have been
	// unresponsive for the given duration (should transition to EXCLUDED).
	// Excludes infrastructure and gateway IPs which should stay DOWN.
	GetTargetsForExcludedTransition(ctx context.Context, threshold time.Duration) ([]types.Target, error)

	// GetTargetsForSmartRecheck returns EXCLUDED or UNRESPONSIVE targets in
	// subnets that have no active customer coverage. These should be re-probed.
	GetTargetsForSmartRecheck(ctx context.Context) ([]types.Target, error)

	// GetTargetsForBaselineCheck returns ACTIVE targets that have been responding
	// for at least the threshold duration but don't have a baseline yet.
	GetTargetsForBaselineCheck(ctx context.Context, threshold time.Duration) ([]types.Target, error)

	// TransitionTargetState changes a target's monitoring state with history.
	TransitionTargetState(ctx context.Context, targetID string, newState types.MonitoringState, reason, triggeredBy string) error

	// SetTargetTier changes a target's monitoring tier.
	SetTargetTier(ctx context.Context, targetID, tier string) error

	// SetTargetBaseline marks a target as having an established baseline.
	SetTargetBaseline(ctx context.Context, targetID string) error
}

// StateWorkerConfig holds configuration for the state worker.
type StateWorkerConfig struct {
	// Interval between state check runs.
	Interval time.Duration

	// BaselineThreshold is how long a target must respond before establishing a baseline.
	// Only targets with a baseline can transition to DOWN (alertable).
	BaselineThreshold time.Duration

	// DownThreshold is how long an ACTIVE target with baseline must be unresponsive
	// before transitioning to DOWN.
	DownThreshold time.Duration

	// UnresponsiveThreshold is how long an ACTIVE target WITHOUT baseline must be
	// unresponsive before transitioning to UNRESPONSIVE (not alertable).
	UnresponsiveThreshold time.Duration

	// ExcludedThreshold is how long a DOWN target must be unresponsive
	// before transitioning to EXCLUDED.
	ExcludedThreshold time.Duration

	// SmartRecheckEnabled enables the smart re-check feature for subnets
	// without active coverage.
	SmartRecheckEnabled bool
}

// DefaultStateWorkerConfig returns sensible defaults.
func DefaultStateWorkerConfig() StateWorkerConfig {
	return StateWorkerConfig{
		Interval:              5 * time.Minute,
		BaselineThreshold:     1 * time.Minute,  // 1 minute of responses = baseline established
		DownThreshold:         15 * time.Minute, // No response for 15 min = down (alertable)
		UnresponsiveThreshold: 15 * time.Minute, // No response for 15 min = unresponsive (not alertable)
		ExcludedThreshold:     24 * time.Hour,   // No response for 24h = excluded
		SmartRecheckEnabled:   true,
	}
}

// StateWorker monitors target states and handles automatic transitions.
type StateWorker struct {
	store  StateStore
	config StateWorkerConfig
	logger *slog.Logger
	stopCh chan struct{}
}

// NewStateWorker creates a new state worker.
func NewStateWorker(store StateStore, config StateWorkerConfig, logger *slog.Logger) *StateWorker {
	return &StateWorker{
		store:  store,
		config: config,
		logger: logger.With("component", "state_worker"),
		stopCh: make(chan struct{}),
	}
}

// Start begins the state worker in a goroutine.
func (w *StateWorker) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop signals the worker to stop.
func (w *StateWorker) Stop() {
	close(w.stopCh)
}

func (w *StateWorker) run(ctx context.Context) {
	w.logger.Info("state worker started",
		"interval", w.config.Interval,
		"baseline_threshold", w.config.BaselineThreshold,
		"down_threshold", w.config.DownThreshold,
		"unresponsive_threshold", w.config.UnresponsiveThreshold,
		"excluded_threshold", w.config.ExcludedThreshold,
	)

	// Run immediately on start
	w.runOnce(ctx)

	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("state worker stopping (context cancelled)")
			return
		case <-w.stopCh:
			w.logger.Info("state worker stopping (stop signal)")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *StateWorker) runOnce(ctx context.Context) {
	start := time.Now()

	// First, establish baselines for targets that have been stable long enough
	baselineCount := w.establishBaselines(ctx)

	// Transition targets WITH baseline that stopped responding to DOWN (alertable)
	downCount := w.transitionToDown(ctx)

	// Transition targets WITHOUT baseline that stopped responding to UNRESPONSIVE (not alertable)
	unresponsiveCount := w.transitionToUnresponsive(ctx)

	// Transition long-term down to excluded
	excludedCount := w.transitionToExcluded(ctx)

	// Re-check excluded/unresponsive targets in subnets without coverage
	recheckCount := 0
	if w.config.SmartRecheckEnabled {
		recheckCount = w.queueSmartRecheck(ctx)
	}

	w.logger.Info("state worker cycle complete",
		"duration", time.Since(start),
		"baselines_established", baselineCount,
		"down_transitions", downCount,
		"unresponsive_transitions", unresponsiveCount,
		"excluded_transitions", excludedCount,
		"smart_recheck_queued", recheckCount,
	)
}

// establishBaselines finds ACTIVE targets that have been responding long enough
// and marks them as having an established baseline (alertable on future outages).
func (w *StateWorker) establishBaselines(ctx context.Context) int {
	targets, err := w.store.GetTargetsForBaselineCheck(ctx, w.config.BaselineThreshold)
	if err != nil {
		w.logger.Error("failed to get targets for baseline check", "error", err)
		return 0
	}

	count := 0
	for _, t := range targets {
		if err := w.store.SetTargetBaseline(ctx, t.ID); err != nil {
			w.logger.Error("failed to set target baseline",
				"target_id", t.ID,
				"ip", t.IP,
				"error", err,
			)
			continue
		}
		w.logger.Info("baseline established for target",
			"target_id", t.ID,
			"ip", t.IP,
			"responding_since", t.FirstResponseAt,
		)
		count++
	}
	return count
}

// transitionToDown finds ACTIVE targets WITH baseline that haven't responded
// recently and transitions them to DOWN (alertable outage).
func (w *StateWorker) transitionToDown(ctx context.Context) int {
	targets, err := w.store.GetTargetsForDownTransition(ctx, w.config.DownThreshold)
	if err != nil {
		w.logger.Error("failed to get targets for down transition", "error", err)
		return 0
	}

	count := 0
	for _, t := range targets {
		reason := "no probe response for " + w.config.DownThreshold.String()
		if err := w.store.TransitionTargetState(ctx, t.ID, types.StateDown, reason, "state_worker"); err != nil {
			w.logger.Error("failed to transition target to down",
				"target_id", t.ID,
				"ip", t.IP,
				"error", err,
			)
			continue
		}
		w.logger.Info("target transitioned to down",
			"target_id", t.ID,
			"ip", t.IP,
		)
		count++
	}
	return count
}

// transitionToUnresponsive finds ACTIVE targets WITHOUT baseline that haven't
// responded recently and transitions them to UNRESPONSIVE (not alertable).
func (w *StateWorker) transitionToUnresponsive(ctx context.Context) int {
	targets, err := w.store.GetTargetsForUnresponsiveTransition(ctx, w.config.UnresponsiveThreshold)
	if err != nil {
		w.logger.Error("failed to get targets for unresponsive transition", "error", err)
		return 0
	}

	count := 0
	for _, t := range targets {
		reason := "no probe response for " + w.config.UnresponsiveThreshold.String() + " (no baseline established)"
		if err := w.store.TransitionTargetState(ctx, t.ID, types.StateUnresponsive, reason, "state_worker"); err != nil {
			w.logger.Error("failed to transition target to unresponsive",
				"target_id", t.ID,
				"ip", t.IP,
				"error", err,
			)
			continue
		}
		w.logger.Info("target transitioned to unresponsive (no baseline)",
			"target_id", t.ID,
			"ip", t.IP,
		)
		count++
	}
	return count
}

// transitionToExcluded finds DEGRADED targets that have been unresponsive
// for too long and transitions them to EXCLUDED (except infra/gateway IPs).
func (w *StateWorker) transitionToExcluded(ctx context.Context) int {
	targets, err := w.store.GetTargetsForExcludedTransition(ctx, w.config.ExcludedThreshold)
	if err != nil {
		w.logger.Error("failed to get targets for excluded transition", "error", err)
		return 0
	}

	count := 0
	for _, t := range targets {
		reason := "unresponsive for " + w.config.ExcludedThreshold.String()
		if err := w.store.TransitionTargetState(ctx, t.ID, types.StateExcluded, reason, "state_worker"); err != nil {
			w.logger.Error("failed to transition target to excluded",
				"target_id", t.ID,
				"ip", t.IP,
				"error", err,
			)
			continue
		}
		w.logger.Info("target transitioned to excluded",
			"target_id", t.ID,
			"ip", t.IP,
		)
		count++
	}
	return count
}

// queueSmartRecheck finds EXCLUDED/UNRESPONSIVE targets in subnets with no
// active coverage and moves them to the discovery tier for re-probing.
func (w *StateWorker) queueSmartRecheck(ctx context.Context) int {
	targets, err := w.store.GetTargetsForSmartRecheck(ctx)
	if err != nil {
		w.logger.Error("failed to get targets for smart recheck", "error", err)
		return 0
	}

	count := 0
	for _, t := range targets {
		// Move to discovery tier for re-probing
		if err := w.store.SetTargetTier(ctx, t.ID, "smart_recheck"); err != nil {
			w.logger.Error("failed to set target tier for smart recheck",
				"target_id", t.ID,
				"ip", t.IP,
				"error", err,
			)
			continue
		}
		w.logger.Debug("target queued for smart recheck",
			"target_id", t.ID,
			"ip", t.IP,
			"current_state", t.MonitoringState,
		)
		count++
	}
	return count
}
