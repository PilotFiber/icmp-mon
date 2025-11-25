// Package scheduler manages probe execution loops by tier.
//
// # Design
//
// The scheduler runs one goroutine per tier, each executing at the tier's
// configured interval. This allows efficient batching within a tier while
// ensuring different tiers run at their appropriate frequencies.
//
// # Probe Loop
//
// For each tier:
//  1. Collect all assigned targets for this tier
//  2. Batch targets (respecting executor limits)
//  3. Execute batches concurrently (with limit)
//  4. Send results to shipper
//  5. Sleep until next interval
//
// # Graceful Handling
//
// - If probe execution takes longer than interval, next run starts immediately
// - Context cancellation stops all loops gracefully
// - Assignment updates are applied atomically between probe cycles
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/pilot-net/icmp-mon/agent/internal/executor"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// ResultHandler receives probe results for shipping.
type ResultHandler func(results []*executor.Result)

// Scheduler manages probe execution for all tiers.
type Scheduler struct {
	registry *executor.Registry
	handler  ResultHandler
	logger   *slog.Logger

	// Tier configurations (probe interval, timeout, etc.)
	tiers map[string]types.Tier
	tierMu sync.RWMutex

	// Current assignments grouped by tier
	assignments map[string][]types.Assignment // tier -> assignments
	assignMu    sync.RWMutex

	// Control
	wg sync.WaitGroup
}

// NewScheduler creates a new scheduler.
func NewScheduler(registry *executor.Registry, handler ResultHandler, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		registry:    registry,
		handler:     handler,
		logger:      logger,
		tiers:       make(map[string]types.Tier),
		assignments: make(map[string][]types.Assignment),
	}
}

// SetTiers updates the tier configurations.
func (s *Scheduler) SetTiers(tiers map[string]types.Tier) {
	s.tierMu.Lock()
	defer s.tierMu.Unlock()
	s.tiers = tiers
}

// UpdateAssignments replaces current assignments with new ones.
// Assignments are automatically grouped by tier.
func (s *Scheduler) UpdateAssignments(assignments []types.Assignment) {
	grouped := make(map[string][]types.Assignment)
	for _, a := range assignments {
		grouped[a.Tier] = append(grouped[a.Tier], a)
	}

	s.assignMu.Lock()
	s.assignments = grouped
	s.assignMu.Unlock()

	// Log assignment counts
	for tier, assigns := range grouped {
		s.logger.Info("assignments updated",
			"tier", tier,
			"count", len(assigns))
	}
}

// GetAssignmentCount returns the total number of assignments.
func (s *Scheduler) GetAssignmentCount() int {
	s.assignMu.RLock()
	defer s.assignMu.RUnlock()
	count := 0
	for _, assigns := range s.assignments {
		count += len(assigns)
	}
	return count
}

// GetAssignmentCountByTier returns assignment counts per tier.
func (s *Scheduler) GetAssignmentCountByTier() map[string]int {
	s.assignMu.RLock()
	defer s.assignMu.RUnlock()
	counts := make(map[string]int)
	for tier, assigns := range s.assignments {
		counts[tier] = len(assigns)
	}
	return counts
}

// Run starts probe loops for all configured tiers.
// Blocks until context is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	s.tierMu.RLock()
	tiers := make([]string, 0, len(s.tiers))
	for name := range s.tiers {
		tiers = append(tiers, name)
	}
	s.tierMu.RUnlock()

	// Start a loop for each tier
	for _, tier := range tiers {
		s.wg.Add(1)
		go func(tierName string) {
			defer s.wg.Done()
			s.runTierLoop(ctx, tierName)
		}(tier)
	}

	// Wait for all loops to finish
	s.wg.Wait()
	return ctx.Err()
}

// runTierLoop runs the probe loop for a single tier.
func (s *Scheduler) runTierLoop(ctx context.Context, tierName string) {
	s.tierMu.RLock()
	tier, ok := s.tiers[tierName]
	s.tierMu.RUnlock()

	if !ok {
		s.logger.Warn("tier not found", "tier", tierName)
		return
	}

	s.logger.Info("starting probe loop",
		"tier", tierName,
		"interval", tier.ProbeInterval)

	ticker := time.NewTicker(tier.ProbeInterval)
	defer ticker.Stop()

	// Run immediately on start
	s.executeTierProbes(ctx, tierName, tier)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("stopping probe loop", "tier", tierName)
			return
		case <-ticker.C:
			s.executeTierProbes(ctx, tierName, tier)
		}
	}
}

// executeTierProbes runs probes for all assignments in a tier.
func (s *Scheduler) executeTierProbes(ctx context.Context, tierName string, tier types.Tier) {
	// Get current assignments for this tier
	s.assignMu.RLock()
	assignments := s.assignments[tierName]
	s.assignMu.RUnlock()

	if len(assignments) == 0 {
		return
	}

	start := time.Now()

	// Get executor (default to icmp_ping)
	probeType := "icmp_ping"
	if len(assignments) > 0 && assignments[0].ProbeType != "" {
		probeType = assignments[0].ProbeType
	}

	exec, ok := s.registry.Get(probeType)
	if !ok {
		s.logger.Error("executor not found", "type", probeType)
		return
	}

	// Convert assignments to probe targets
	targets := make([]executor.ProbeTarget, len(assignments))
	for i, a := range assignments {
		targets[i] = executor.ProbeTarget{
			ID:      a.TargetID,
			IP:      a.IP,
			Timeout: tier.ProbeTimeout,
			Retries: tier.ProbeRetries,
			Params:  a.ProbeParams,
		}
	}

	// Execute in batches
	caps := exec.Capabilities()
	batchSize := caps.MaxBatchSize
	if batchSize <= 0 || !caps.SupportsBatching {
		batchSize = 1
	}

	var allResults []*executor.Result

	for i := 0; i < len(targets); i += batchSize {
		end := i + batchSize
		if end > len(targets) {
			end = len(targets)
		}
		batch := targets[i:end]

		results, err := exec.ExecuteBatch(ctx, batch)
		if err != nil {
			s.logger.Error("batch execution failed",
				"tier", tierName,
				"error", err,
				"batch_start", i,
				"batch_size", len(batch))
			continue
		}

		allResults = append(allResults, results...)
	}

	// Send results to handler
	if len(allResults) > 0 && s.handler != nil {
		s.handler(allResults)
	}

	elapsed := time.Since(start)
	s.logger.Debug("probe cycle complete",
		"tier", tierName,
		"targets", len(targets),
		"results", len(allResults),
		"elapsed", elapsed)
}

// Stats returns current scheduler statistics.
type Stats struct {
	TierCounts     map[string]int `json:"tier_counts"`
	TotalTargets   int            `json:"total_targets"`
	ActiveTiers    int            `json:"active_tiers"`
}

func (s *Scheduler) Stats() Stats {
	counts := s.GetAssignmentCountByTier()
	total := 0
	for _, c := range counts {
		total += c
	}
	return Stats{
		TierCounts:   counts,
		TotalTargets: total,
		ActiveTiers:  len(counts),
	}
}
