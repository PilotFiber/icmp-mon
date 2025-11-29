// Package worker provides background workers for the control plane.
package worker

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/pilot-net/icmp-mon/control-plane/internal/store"
)

// EvaluatorStore defines the storage interface for the evaluator worker.
type EvaluatorStore interface {
	// GetActiveAgentTargetPairs returns all active (agent_id, target_id) pairs
	// that have recent probe results and should be evaluated.
	GetActiveAgentTargetPairs(ctx context.Context, since time.Duration) ([]store.AgentTargetPair, error)

	// BulkGetRecentProbeStats retrieves probe stats for multiple pairs in a single query.
	BulkGetRecentProbeStats(ctx context.Context, pairs []store.AgentTargetPair, window time.Duration) (map[store.PairKey]*store.ProbeStats, error)

	// BulkGetBaselines retrieves baselines for multiple pairs in a single query.
	BulkGetBaselines(ctx context.Context, pairs []store.AgentTargetPair) (map[store.PairKey]*store.AgentTargetBaseline, error)

	// BulkGetAgentTargetStates retrieves states for multiple pairs in a single query.
	BulkGetAgentTargetStates(ctx context.Context, pairs []store.AgentTargetPair) (map[store.PairKey]*store.AgentTargetState, error)

	// BulkUpsertAgentTargetStates inserts or updates multiple agent-target states in bulk.
	BulkUpsertAgentTargetStates(ctx context.Context, states []*store.AgentTargetState) error

	// UpsertBaseline inserts or updates the agent_target_baseline row.
	UpsertBaseline(ctx context.Context, baseline *store.AgentTargetBaseline) error
}

// EvaluatorWorkerConfig holds configuration for the evaluator worker.
type EvaluatorWorkerConfig struct {
	// Interval between evaluation runs.
	Interval time.Duration

	// EvaluationWindow is how far back to look at probe results for evaluation.
	EvaluationWindow time.Duration

	// BaselineWindow is how far back to look for baseline calculation.
	BaselineWindow time.Duration

	// MinSamplesForBaseline is the minimum number of samples needed to establish a baseline.
	MinSamplesForBaseline int

	// ZScoreWarningThreshold is the z-score above which we consider latency degraded.
	ZScoreWarningThreshold float64

	// ZScoreCriticalThreshold is the z-score above which we consider latency critical.
	ZScoreCriticalThreshold float64

	// PacketLossWarningPct is the packet loss percentage above which we consider degraded.
	PacketLossWarningPct float64

	// PacketLossCriticalPct is the packet loss percentage above which we consider down.
	PacketLossCriticalPct float64

	// ConsecutiveFailuresForDown is how many consecutive failures before marking as down.
	ConsecutiveFailuresForDown int

	// ConsecutiveSuccessesForUp is how many consecutive successes before marking as up.
	ConsecutiveSuccessesForUp int
}

// DefaultEvaluatorWorkerConfig returns sensible defaults.
func DefaultEvaluatorWorkerConfig() EvaluatorWorkerConfig {
	return EvaluatorWorkerConfig{
		Interval:                   30 * time.Second,
		EvaluationWindow:           5 * time.Minute,
		BaselineWindow:             7 * 24 * time.Hour, // 7 days
		MinSamplesForBaseline:      100,
		ZScoreWarningThreshold:     3.0,
		ZScoreCriticalThreshold:    5.0,
		PacketLossWarningPct:       20.0,
		PacketLossCriticalPct:      20.0,
		ConsecutiveFailuresForDown: 3,
		ConsecutiveSuccessesForUp:  3,
	}
}

// EvaluatorWorker evaluates probe results against baselines and updates agent_target_state.
type EvaluatorWorker struct {
	store  EvaluatorStore
	config EvaluatorWorkerConfig
	logger *slog.Logger
	stopCh chan struct{}
}

// NewEvaluatorWorker creates a new evaluator worker.
func NewEvaluatorWorker(store EvaluatorStore, config EvaluatorWorkerConfig, logger *slog.Logger) *EvaluatorWorker {
	return &EvaluatorWorker{
		store:  store,
		config: config,
		logger: logger.With("component", "evaluator_worker"),
		stopCh: make(chan struct{}),
	}
}

// Start begins the evaluator worker in a goroutine.
func (w *EvaluatorWorker) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop signals the worker to stop.
func (w *EvaluatorWorker) Stop() {
	close(w.stopCh)
}

func (w *EvaluatorWorker) run(ctx context.Context) {
	w.logger.Info("evaluator worker started",
		"interval", w.config.Interval,
		"evaluation_window", w.config.EvaluationWindow,
		"z_score_warning", w.config.ZScoreWarningThreshold,
		"z_score_critical", w.config.ZScoreCriticalThreshold,
	)

	// Run immediately on start
	w.runOnce(ctx)

	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("evaluator worker stopping (context cancelled)")
			return
		case <-w.stopCh:
			w.logger.Info("evaluator worker stopping (stop signal)")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *EvaluatorWorker) runOnce(ctx context.Context) {
	start := time.Now()

	// Get all active agent-target pairs with recent probes
	pairs, err := w.store.GetActiveAgentTargetPairs(ctx, w.config.EvaluationWindow)
	if err != nil {
		w.logger.Error("failed to get active agent-target pairs", "error", err)
		return
	}

	if len(pairs) == 0 {
		w.logger.Debug("no active pairs to evaluate")
		return
	}

	// Fetch all data in bulk (3 queries instead of 3*N queries)
	fetchStart := time.Now()

	allStats, err := w.store.BulkGetRecentProbeStats(ctx, pairs, w.config.EvaluationWindow)
	if err != nil {
		w.logger.Error("failed to bulk get probe stats", "error", err)
		return
	}

	allBaselines, err := w.store.BulkGetBaselines(ctx, pairs)
	if err != nil {
		w.logger.Error("failed to bulk get baselines", "error", err)
		return
	}

	allStates, err := w.store.BulkGetAgentTargetStates(ctx, pairs)
	if err != nil {
		w.logger.Error("failed to bulk get states", "error", err)
		return
	}

	fetchDuration := time.Since(fetchStart)

	// Process all pairs using pre-fetched data
	evaluated := 0
	baselinesUpdated := 0
	stateChanges := 0
	var statesToUpdate []*store.AgentTargetState

	for _, pair := range pairs {
		key := store.PairKey{AgentID: pair.AgentID, TargetID: pair.TargetID}
		stats := allStats[key]
		baseline := allBaselines[key]
		currentState := allStates[key]

		newState, changed, baselineCreated := w.evaluatePairWithData(ctx, pair, stats, baseline, currentState)
		evaluated++
		if newState != nil {
			statesToUpdate = append(statesToUpdate, newState)
		}
		if changed {
			stateChanges++
		}
		if baselineCreated {
			baselinesUpdated++
		}
	}

	// Batch upsert all state updates
	if len(statesToUpdate) > 0 {
		if err := w.store.BulkUpsertAgentTargetStates(ctx, statesToUpdate); err != nil {
			w.logger.Error("failed to bulk upsert agent target states",
				"error", err,
				"count", len(statesToUpdate),
			)
		}
	}

	w.logger.Info("evaluator worker cycle complete",
		"duration", time.Since(start),
		"fetch_duration", fetchDuration,
		"pairs_evaluated", evaluated,
		"states_updated", len(statesToUpdate),
		"state_changes", stateChanges,
		"baselines_updated", baselinesUpdated,
	)
}

// evaluatePairWithData evaluates a single agent-target pair using pre-fetched data.
// Returns (newState, stateChanged, baselineCreated). newState may be nil if no stats available.
func (w *EvaluatorWorker) evaluatePairWithData(ctx context.Context, pair store.AgentTargetPair, stats *store.ProbeStats, baseline *store.AgentTargetBaseline, currentState *store.AgentTargetState) (*store.AgentTargetState, bool, bool) {
	if stats == nil || stats.TotalCount == 0 {
		return nil, false, false
	}

	baselineCreated := false

	// If no baseline exists and we have enough samples, create one
	if baseline == nil && stats.SuccessCount >= w.config.MinSamplesForBaseline {
		p50 := stats.P50LatencyMs
		p95 := stats.P95LatencyMs
		p99 := stats.MaxLatencyMs // Use max as P99 approximation
		stddev := stats.StddevMs
		baseline = &store.AgentTargetBaseline{
			AgentID:            pair.AgentID,
			TargetID:           pair.TargetID,
			LatencyP50:         &p50,
			LatencyP95:         &p95,
			LatencyP99:         &p99,
			LatencyStddev:      &stddev,
			PacketLossBaseline: stats.PacketLossPct,
			SampleCount:        stats.SuccessCount,
			FirstSeen:          time.Now(),
			LastUpdated:        time.Now(),
		}
		if err := w.store.UpsertBaseline(ctx, baseline); err != nil {
			w.logger.Error("failed to create baseline",
				"agent_id", pair.AgentID,
				"target_id", pair.TargetID,
				"error", err,
			)
		} else {
			baselineCreated = true
			w.logger.Debug("created new baseline",
				"agent_id", pair.AgentID,
				"target_id", pair.TargetID,
				"p50_latency", ptrFloat(baseline.LatencyP50),
				"stddev", ptrFloat(baseline.LatencyStddev),
			)
		}
	}

	// Calculate new state
	result := w.calculateState(stats, baseline, currentState)
	newState := result.State
	newState.AgentID = pair.AgentID
	newState.TargetID = pair.TargetID
	newState.LastProbeTime = &stats.LastProbeTime
	newState.LastEvaluated = time.Now()

	// Determine if state changed
	stateChanged := currentState == nil || currentState.Status != newState.Status

	// Track consecutive counters based on whether an anomaly was OBSERVED (not just the displayed status)
	// This allows us to track anomalies before they meet the threshold for state change
	if currentState != nil {
		if result.ObservedAnomaly {
			// This observation detected an anomaly - increment counter
			newState.ConsecutiveAnomalies = currentState.ConsecutiveAnomalies + 1
			newState.ConsecutiveSuccesses = 0
			if currentState.AnomalyStart == nil {
				now := time.Now()
				newState.AnomalyStart = &now
			} else {
				newState.AnomalyStart = currentState.AnomalyStart
			}
		} else {
			// Healthy observation - reset anomaly counter, increment success counter
			newState.ConsecutiveSuccesses = currentState.ConsecutiveSuccesses + 1
			newState.ConsecutiveAnomalies = 0
			newState.AnomalyStart = nil
		}
	} else {
		// No previous state
		if result.ObservedAnomaly {
			newState.ConsecutiveAnomalies = 1
			now := time.Now()
			newState.AnomalyStart = &now
		} else {
			newState.ConsecutiveSuccesses = 1
		}
	}

	// Update status_since if status changed
	if stateChanged {
		now := time.Now()
		newState.StatusSince = &now
	} else if currentState != nil {
		newState.StatusSince = currentState.StatusSince
	}

	if stateChanged {
		w.logger.Info("agent-target state changed",
			"agent_id", pair.AgentID,
			"target_id", pair.TargetID,
			"old_status", statusOrUnknown(currentState),
			"new_status", newState.Status,
			"z_score", ptrFloat(newState.CurrentZScore),
			"packet_loss", ptrFloat(newState.CurrentPacketLoss),
		)
	}

	return newState, stateChanged, baselineCreated
}

// stateResult contains both the calculated state and metadata about the observation.
type stateResult struct {
	State            *store.AgentTargetState
	ObservedAnomaly  bool // True if this observation detected an anomaly (even if not yet alerting)
	IsCriticalLevel  bool // True if the detected anomaly is critical-level
}

// calculateState determines the status based on probe stats and baseline.
// All anomaly conditions require consecutive observations before changing state.
// This prevents spurious alerts from single bad measurements.
func (w *EvaluatorWorker) calculateState(stats *store.ProbeStats, baseline *store.AgentTargetBaseline, current *store.AgentTargetState) stateResult {
	state := &store.AgentTargetState{}
	result := stateResult{State: state}

	// Set current metrics
	latency := stats.AvgLatencyMs
	state.CurrentLatencyMs = &latency
	packetLoss := stats.PacketLossPct
	state.CurrentPacketLoss = &packetLoss

	// Calculate z-score if we have a baseline
	var zScore float64
	hasZScore := false
	if baseline != nil && baseline.LatencyStddev != nil && *baseline.LatencyStddev > 0 && baseline.LatencyP50 != nil {
		zScore = (stats.AvgLatencyMs - *baseline.LatencyP50) / *baseline.LatencyStddev
		state.CurrentZScore = &zScore
		hasZScore = true
	}

	// Determine the "raw" anomaly level before considering consecutive observations
	// Priority: complete failure > critical packet loss > critical z-score > warning packet loss > warning z-score
	rawStatus := "up"
	isCritical := false

	// Check for complete failure (100% packet loss or all failures)
	if stats.SuccessCount == 0 || stats.PacketLossPct >= 100 {
		rawStatus = "down"
		isCritical = true
	} else if stats.PacketLossPct >= w.config.PacketLossCriticalPct {
		// Critical packet loss
		rawStatus = "down"
		isCritical = true
	} else if hasZScore && zScore >= w.config.ZScoreCriticalThreshold {
		// Critical latency deviation
		rawStatus = "down"
		isCritical = true
	} else if stats.PacketLossPct >= w.config.PacketLossWarningPct {
		// Warning packet loss
		rawStatus = "degraded"
	} else if hasZScore && zScore >= w.config.ZScoreWarningThreshold {
		// Warning latency deviation
		rawStatus = "degraded"
	}

	// Record whether we observed an anomaly (for counter tracking)
	result.ObservedAnomaly = rawStatus != "up"
	result.IsCriticalLevel = isCritical

	// If current observation shows an anomaly, check consecutive count
	if rawStatus != "up" {
		// Count consecutive anomalies including this observation
		consecutiveAnomalies := 1
		if current != nil && current.ConsecutiveAnomalies > 0 {
			// Previous observation also detected an anomaly
			consecutiveAnomalies = current.ConsecutiveAnomalies + 1
		}

		// Require at least 2 consecutive anomalous observations before changing state
		// For critical conditions (down), require ConsecutiveFailuresForDown observations
		// For warning conditions (degraded), require 2 observations
		requiredForDegraded := 2
		requiredForDown := w.config.ConsecutiveFailuresForDown

		if isCritical {
			// Critical anomaly detected
			if consecutiveAnomalies >= requiredForDown {
				state.Status = "down"
			} else if consecutiveAnomalies >= requiredForDegraded {
				// Enough for degraded but not yet for down
				state.Status = "degraded"
			} else {
				// First observation of critical condition - stay healthy but track
				state.Status = "up"
			}
		} else {
			// Warning-level anomaly (degraded)
			if consecutiveAnomalies >= requiredForDegraded {
				state.Status = "degraded"
			} else {
				// First observation of warning condition - stay healthy but track
				state.Status = "up"
			}
		}
		return result
	}

	// All healthy
	state.Status = "up"
	return result
}

func statusOrUnknown(state *store.AgentTargetState) string {
	if state == nil {
		return "unknown"
	}
	return state.Status
}

func ptrFloat(f *float64) float64 {
	if f == nil {
		return math.NaN()
	}
	return *f
}
