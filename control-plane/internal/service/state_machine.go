package service

import (
	"context"
	"time"

	"github.com/pilot-net/icmp-mon/pkg/types"
)

// StateProcessorConfig holds configuration for state transitions.
type StateProcessorConfig struct {
	// MaxDiscoveryAttempts is how many failed probes before UNKNOWN → UNRESPONSIVE.
	MaxDiscoveryAttempts int
}

// DefaultStateProcessorConfig returns sensible defaults.
func DefaultStateProcessorConfig() StateProcessorConfig {
	return StateProcessorConfig{
		MaxDiscoveryAttempts: 5, // After 5 failed discovery probes, mark UNRESPONSIVE
	}
}

// ProcessProbeResult handles state transitions based on a probe result.
// This is called for each probe result as it comes in.
func (s *Service) ProcessProbeResult(ctx context.Context, targetID string, success bool, probeTime time.Time) error {
	// Get current target state
	target, err := s.store.GetTarget(ctx, targetID)
	if err != nil || target == nil {
		return err // Target doesn't exist or error
	}

	// Skip archived targets
	if target.ArchivedAt != nil {
		return nil
	}

	if success {
		return s.handleSuccessfulProbe(ctx, target, probeTime)
	}
	return s.handleFailedProbe(ctx, target, probeTime)
}

// handleSuccessfulProbe processes state transitions on successful ICMP response.
func (s *Service) handleSuccessfulProbe(ctx context.Context, target *types.Target, probeTime time.Time) error {
	// Always update last_response_at
	if err := s.store.UpdateTargetLastResponse(ctx, target.ID, probeTime); err != nil {
		s.logger.Error("failed to update last_response_at", "target_id", target.ID, "error", err)
	}

	// Check for state transitions
	switch target.MonitoringState {
	case types.StateUnknown:
		// UNKNOWN → ACTIVE: First successful response
		return s.store.TransitionTargetState(ctx, target.ID, types.StateActive,
			"first successful probe response", "discovery")

	case types.StateUnresponsive:
		// UNRESPONSIVE → ACTIVE: Started responding
		return s.store.TransitionTargetState(ctx, target.ID, types.StateActive,
			"target started responding", "smart_recheck")

	case types.StateDown:
		// DOWN → ACTIVE: Recovery
		return s.store.TransitionTargetState(ctx, target.ID, types.StateActive,
			"target recovered", "system")

	case types.StateDegraded:
		// DEGRADED → ACTIVE: Performance recovered
		return s.store.TransitionTargetState(ctx, target.ID, types.StateActive,
			"performance recovered", "system")

	case types.StateExcluded:
		// EXCLUDED → ACTIVE: Came back online
		return s.store.TransitionTargetState(ctx, target.ID, types.StateActive,
			"target came back online", "smart_recheck")

	case types.StateInactive:
		// INACTIVE → ACTIVE: Unexpected response from supposedly inactive target
		return s.store.TransitionTargetState(ctx, target.ID, types.StateActive,
			"inactive target started responding", "system")
	}

	// ACTIVE stays ACTIVE - no transition needed
	return nil
}

// handleFailedProbe processes state transitions on failed ICMP probe.
// Note: ACTIVE → DOWN and DOWN → EXCLUDED are handled by the state_worker
// based on time thresholds, not individual probe failures.
func (s *Service) handleFailedProbe(ctx context.Context, target *types.Target, probeTime time.Time) error {
	switch target.MonitoringState {
	case types.StateUnknown:
		// Increment discovery attempts
		attempts, err := s.store.IncrementDiscoveryAttempts(ctx, target.ID)
		if err != nil {
			return err
		}

		// Check if max attempts reached
		config := DefaultStateProcessorConfig()
		if attempts >= config.MaxDiscoveryAttempts {
			// UNKNOWN → UNRESPONSIVE: Never responded to discovery
			return s.store.TransitionTargetState(ctx, target.ID, types.StateUnresponsive,
				"no response after discovery attempts", "discovery")
		}
	}

	// Other states: failed probes don't cause immediate transitions
	// The state_worker handles ACTIVE → DOWN and DOWN → EXCLUDED based on time
	return nil
}

// GetEffectiveTier returns the tier a target should use based on its monitoring state.
// This implements the virtual tier logic from the design doc.
func (s *Service) GetEffectiveTier(ctx context.Context, target *types.Target, tiers map[string]types.Tier) *types.Tier {
	// Skip archived targets entirely
	if target.ArchivedAt != nil {
		return nil
	}

	switch target.MonitoringState {
	case types.StateActive, types.StateDegraded, types.StateDown, "":
		// Empty state ("") is treated as active for legacy targets
		// Use the target's assigned tier for full monitoring
		if tier, ok := tiers[target.Tier]; ok {
			return &tier
		}
		// Fallback to standard if tier not found
		if tier, ok := tiers["standard"]; ok {
			return &tier
		}
		return nil

	case types.StateUnknown:
		// Use discovery tier (5 min interval)
		if tier, ok := tiers["discovery"]; ok {
			return &tier
		}
		return nil

	case types.StateInactive:
		// Use inactive re-check tier (1 hour interval)
		if tier, ok := tiers["inactive_recheck"]; ok {
			return &tier
		}
		return nil

	case types.StateUnresponsive, types.StateExcluded:
		// Smart re-check: only probe if subnet has no active coverage
		if target.SubnetID != nil {
			hasCoverage, err := s.store.SubnetHasActiveCoverage(ctx, *target.SubnetID)
			if err == nil && hasCoverage {
				return nil // Skip - subnet already has active targets
			}
		}
		// Subnet needs coverage, use smart_recheck tier
		if tier, ok := tiers["smart_recheck"]; ok {
			return &tier
		}
		return nil
	}

	return nil
}

// ProcessProbeResultsBatch processes state transitions for a batch of probe results.
// This is more efficient than processing one at a time.
func (s *Service) ProcessProbeResultsBatch(ctx context.Context, results []types.ProbeResult) error {
	for _, result := range results {
		if err := s.ProcessProbeResult(ctx, result.TargetID, result.Success, result.Timestamp); err != nil {
			s.logger.Error("failed to process probe result for state",
				"target_id", result.TargetID,
				"error", err,
			)
			// Continue processing other results
		}
	}
	return nil
}
