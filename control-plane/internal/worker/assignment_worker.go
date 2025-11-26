// Package worker - Assignment worker monitors agent status and triggers redistribution
package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/pilot-net/icmp-mon/control-plane/internal/service"
	"github.com/pilot-net/icmp-mon/control-plane/internal/store"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// AssignmentWorkerConfig holds configuration for the assignment worker.
type AssignmentWorkerConfig struct {
	// Interval between status checks.
	Interval time.Duration

	// FlappingThreshold is the number of transitions allowed in FlappingWindow
	// before an agent is considered flapping.
	FlappingThreshold int

	// FlappingWindow is the time window for counting status transitions.
	FlappingWindow time.Duration
}

// DefaultAssignmentWorkerConfig returns sensible defaults.
func DefaultAssignmentWorkerConfig() AssignmentWorkerConfig {
	return AssignmentWorkerConfig{
		Interval:          30 * time.Second,
		FlappingThreshold: 3,
		FlappingWindow:    5 * time.Minute,
	}
}

// AssignmentWorker monitors agent status and triggers redistribution.
type AssignmentWorker struct {
	store      *store.Store
	rebalancer *service.Rebalancer
	config     AssignmentWorkerConfig
	logger     *slog.Logger
	stopCh     chan struct{}

	// Track last known states for change detection
	lastKnownStates map[string]types.AgentStatus
	statesMu        sync.RWMutex

	// Track transitions for flapping detection
	transitions   map[string][]time.Time
	transitionsMu sync.Mutex

	// Track if we've initialized (first run)
	initialized bool
}

// NewAssignmentWorker creates a new assignment worker.
func NewAssignmentWorker(
	store *store.Store,
	rebalancer *service.Rebalancer,
	config AssignmentWorkerConfig,
	logger *slog.Logger,
) *AssignmentWorker {
	return &AssignmentWorker{
		store:           store,
		rebalancer:      rebalancer,
		config:          config,
		logger:          logger.With("component", "assignment_worker"),
		stopCh:          make(chan struct{}),
		lastKnownStates: make(map[string]types.AgentStatus),
		transitions:     make(map[string][]time.Time),
	}
}

// Start begins the worker in a goroutine.
func (w *AssignmentWorker) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop signals the worker to stop.
func (w *AssignmentWorker) Stop() {
	close(w.stopCh)
}

func (w *AssignmentWorker) run(ctx context.Context) {
	w.logger.Info("assignment worker started",
		"interval", w.config.Interval,
		"flapping_threshold", w.config.FlappingThreshold,
		"flapping_window", w.config.FlappingWindow,
	)

	// Run immediately on start to initialize state
	w.runOnce(ctx)

	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("assignment worker stopping (context cancelled)")
			return
		case <-w.stopCh:
			w.logger.Info("assignment worker stopping (stop signal)")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *AssignmentWorker) runOnce(ctx context.Context) {
	// Get all agents with computed status
	agents, err := w.store.ListAgentsWithStatus(ctx)
	if err != nil {
		w.logger.Error("failed to list agents", "error", err)
		return
	}

	w.statesMu.Lock()
	defer w.statesMu.Unlock()

	// First run: just populate state, don't trigger any redistribution
	if !w.initialized {
		for _, agent := range agents {
			w.lastKnownStates[agent.ID] = agent.Status
		}
		w.initialized = true
		w.logger.Info("assignment worker initialized", "agent_count", len(agents))
		return
	}

	// Detect transitions
	var offlineAgents, recoveredAgents []types.Agent

	for _, agent := range agents {
		lastStatus, known := w.lastKnownStates[agent.ID]

		if !known {
			// New agent
			if agent.Status == types.AgentStatusActive {
				recoveredAgents = append(recoveredAgents, agent)
				w.logger.Info("new agent detected",
					"agent_id", agent.ID,
					"agent_name", agent.Name,
				)
			}
		} else {
			// Existing agent - check for transitions
			wasActive := lastStatus == types.AgentStatusActive || lastStatus == types.AgentStatusDegraded
			isOffline := agent.Status == types.AgentStatusOffline
			wasOffline := lastStatus == types.AgentStatusOffline
			isActive := agent.Status == types.AgentStatusActive

			if wasActive && isOffline {
				offlineAgents = append(offlineAgents, agent)
				w.recordTransition(agent.ID)
			} else if wasOffline && isActive {
				recoveredAgents = append(recoveredAgents, agent)
				w.recordTransition(agent.ID)
			}
		}

		w.lastKnownStates[agent.ID] = agent.Status
	}

	// Handle offline agents (failover)
	for _, agent := range offlineAgents {
		if w.isFlapping(agent.ID) {
			w.logger.Warn("agent is flapping, skipping redistribution",
				"agent_id", agent.ID,
				"agent_name", agent.Name,
			)
			continue
		}

		w.logger.Info("agent went offline, triggering failover",
			"agent_id", agent.ID,
			"agent_name", agent.Name,
			"region", agent.Region,
		)

		if err := w.rebalancer.HandleAgentFailure(ctx, agent.ID); err != nil {
			w.logger.Error("failover failed",
				"agent_id", agent.ID,
				"error", err,
			)
		}
	}

	// Handle recovered/new agents (rebalance)
	for _, agent := range recoveredAgents {
		if w.isFlapping(agent.ID) {
			w.logger.Warn("agent is flapping, skipping rebalance",
				"agent_id", agent.ID,
				"agent_name", agent.Name,
			)
			continue
		}

		w.logger.Info("agent recovered/added, triggering rebalance",
			"agent_id", agent.ID,
			"agent_name", agent.Name,
			"region", agent.Region,
		)

		if err := w.rebalancer.HandleAgentRecovery(ctx, agent.ID); err != nil {
			w.logger.Error("rebalance failed",
				"agent_id", agent.ID,
				"error", err,
			)
		}
	}
}

// recordTransition records a status transition for flapping detection.
func (w *AssignmentWorker) recordTransition(agentID string) {
	w.transitionsMu.Lock()
	defer w.transitionsMu.Unlock()

	now := time.Now()
	w.transitions[agentID] = append(w.transitions[agentID], now)

	// Prune old transitions outside the window
	cutoff := now.Add(-w.config.FlappingWindow)
	var recent []time.Time
	for _, t := range w.transitions[agentID] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	w.transitions[agentID] = recent
}

// isFlapping returns true if the agent has had too many transitions recently.
func (w *AssignmentWorker) isFlapping(agentID string) bool {
	w.transitionsMu.Lock()
	defer w.transitionsMu.Unlock()

	cutoff := time.Now().Add(-w.config.FlappingWindow)
	count := 0
	for _, t := range w.transitions[agentID] {
		if t.After(cutoff) {
			count++
		}
	}
	return count > w.config.FlappingThreshold
}
