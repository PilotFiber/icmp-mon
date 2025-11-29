// Package service - Rebalancer handles assignment redistribution
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pilot-net/icmp-mon/control-plane/internal/store"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// Rebalancer handles automatic redistribution of assignments.
type Rebalancer struct {
	store  *store.Store
	logger *slog.Logger
}

// NewRebalancer creates a new rebalancer.
func NewRebalancer(store *store.Store, logger *slog.Logger) *Rebalancer {
	return &Rebalancer{
		store:  store,
		logger: logger.With("component", "rebalancer"),
	}
}

// HandleAgentFailure redistributes assignments from a failed agent to other eligible agents.
func (r *Rebalancer) HandleAgentFailure(ctx context.Context, failedAgentID string) error {
	r.logger.Info("handling agent failure", "agent_id", failedAgentID)

	// Get assignments for the failed agent
	assignments, err := r.store.GetAssignmentsByAgent(ctx, failedAgentID)
	if err != nil {
		return fmt.Errorf("getting assignments for failed agent: %w", err)
	}

	if len(assignments) == 0 {
		r.logger.Info("no assignments to redistribute", "agent_id", failedAgentID)
		return nil
	}

	r.logger.Info("redistributing assignments",
		"agent_id", failedAgentID,
		"assignment_count", len(assignments),
	)

	// Get active agents (excluding the failed one)
	allAgents, err := r.store.ListAgentsWithStatus(ctx)
	if err != nil {
		return fmt.Errorf("listing agents: %w", err)
	}

	activeAgents := make([]types.Agent, 0)
	for _, agent := range allAgents {
		if agent.ID != failedAgentID && agent.Status == types.AgentStatusActive {
			activeAgents = append(activeAgents, agent)
		}
	}

	if len(activeAgents) == 0 {
		r.logger.Warn("no active agents available for redistribution")
		return fmt.Errorf("no active agents available")
	}

	// Get all tiers for policy lookup
	tiers, err := r.store.ListTiers(ctx)
	if err != nil {
		return fmt.Errorf("listing tiers: %w", err)
	}

	tierMap := make(map[string]types.Tier)
	for _, tier := range tiers {
		tierMap[tier.Name] = tier
	}

	// Redistribute each assignment
	reassigned := 0
	for _, assignment := range assignments {
		tier, ok := tierMap[assignment.Tier]
		if !ok {
			r.logger.Warn("tier not found for assignment",
				"tier", assignment.Tier,
				"target_id", assignment.TargetID,
			)
			continue
		}

		// Get target for IP (needed for consistent hashing)
		target, err := r.store.GetTarget(ctx, assignment.TargetID)
		if err != nil || target == nil {
			r.logger.Warn("target not found",
				"target_id", assignment.TargetID,
				"error", err,
			)
			continue
		}

		// Filter eligible agents based on tier policy
		eligibleAgents := r.filterAgents(activeAgents, tier.AgentSelection)
		if len(eligibleAgents) == 0 {
			r.logger.Warn("no eligible agents for target",
				"target_id", assignment.TargetID,
				"tier", tier.Name,
			)
			continue
		}

		// Select the best agent using consistent hashing
		selected := r.selectAgentsForTarget(*target, eligibleAgents, 1, tier.AgentSelection.Diversity)
		if len(selected) == 0 {
			r.logger.Warn("could not select agent for target",
				"target_id", assignment.TargetID,
			)
			continue
		}

		newAgent := selected[0]

		// Create new assignment
		newAssignment := &types.TargetAssignment{
			TargetID:   assignment.TargetID,
			AgentID:    newAgent.ID,
			Tier:       assignment.Tier,
			AssignedBy: types.AssignedByFailover,
		}

		if err := r.store.CreateAssignment(ctx, newAssignment); err != nil {
			r.logger.Error("failed to create assignment",
				"target_id", assignment.TargetID,
				"agent_id", newAgent.ID,
				"error", err,
			)
			continue
		}

		// Delete old assignment
		if err := r.store.DeleteAssignment(ctx, assignment.ID); err != nil {
			r.logger.Error("failed to delete old assignment",
				"assignment_id", assignment.ID,
				"error", err,
			)
		}

		// Log history
		r.store.LogAssignmentHistory(ctx, &types.AssignmentHistory{
			TargetID:   assignment.TargetID,
			AgentID:    newAgent.ID,
			Action:     types.AssignmentActionReassigned,
			Reason:     fmt.Sprintf("failover from agent %s", failedAgentID),
			OldAgentID: failedAgentID,
		})

		r.logger.Debug("reassigned target",
			"target_id", assignment.TargetID,
			"from_agent", failedAgentID,
			"to_agent", newAgent.ID,
		)

		reassigned++
	}

	r.logger.Info("failover complete",
		"agent_id", failedAgentID,
		"reassigned", reassigned,
		"total", len(assignments),
	)

	return nil
}

// HandleAgentRecovery rebalances assignments when a new agent joins or recovers.
func (r *Rebalancer) HandleAgentRecovery(ctx context.Context, agentID string) error {
	r.logger.Info("handling agent recovery/addition", "agent_id", agentID)

	// Get the agent
	agent, err := r.store.GetAgent(ctx, agentID)
	if err != nil || agent == nil {
		return fmt.Errorf("getting agent: %w", err)
	}

	// Get all tiers
	tiers, err := r.store.ListTiers(ctx)
	if err != nil {
		return fmt.Errorf("listing tiers: %w", err)
	}

	// Get all active agents
	allAgents, err := r.store.ListAgentsWithStatus(ctx)
	if err != nil {
		return fmt.Errorf("listing agents: %w", err)
	}

	activeAgents := make([]types.Agent, 0)
	for _, a := range allAgents {
		if a.Status == types.AgentStatusActive {
			activeAgents = append(activeAgents, a)
		}
	}

	assigned := 0

	// For each tier where this agent is eligible
	for _, tier := range tiers {
		if !r.isEligible(agent, tier.AgentSelection) {
			continue
		}

		// Get targets in this tier
		targets, err := r.store.ListTargetsByTier(ctx, tier.Name)
		if err != nil {
			r.logger.Error("failed to list targets by tier",
				"tier", tier.Name,
				"error", err,
			)
			continue
		}

		// For each target, check if it needs more agents or could benefit from this one
		for _, target := range targets {
			// Get current active assignments for this target
			currentAssignments, err := r.store.GetActiveAssignmentsByTarget(ctx, target.ID)
			if err != nil {
				continue
			}

			// Determine required agent count
			requiredCount := tier.AgentSelection.Count
			if tier.AgentSelection.Strategy == "all" {
				requiredCount = len(activeAgents)
			}

			// If target needs more agents
			if len(currentAssignments) < requiredCount {
				// Check if this agent is already assigned
				alreadyAssigned := false
				for _, a := range currentAssignments {
					if a.AgentID == agentID {
						alreadyAssigned = true
						break
					}
				}

				if !alreadyAssigned {
					// Add assignment
					newAssignment := &types.TargetAssignment{
						TargetID:   target.ID,
						AgentID:    agentID,
						Tier:       tier.Name,
						AssignedBy: types.AssignedByRebalancer,
					}

					if err := r.store.CreateAssignment(ctx, newAssignment); err != nil {
						r.logger.Error("failed to create assignment",
							"target_id", target.ID,
							"agent_id", agentID,
							"error", err,
						)
						continue
					}

					r.store.LogAssignmentHistory(ctx, &types.AssignmentHistory{
						TargetID: target.ID,
						AgentID:  agentID,
						Action:   types.AssignmentActionAssigned,
						Reason:   "agent recovery/addition",
					})

					assigned++
				}
			}
		}
	}

	r.logger.Info("agent recovery/addition complete",
		"agent_id", agentID,
		"assigned", assigned,
	)

	return nil
}

// MaterializeAllAssignments computes and stores all assignments.
// This is used for initial population of the assignments table.
func (r *Rebalancer) MaterializeAllAssignments(ctx context.Context) error {
	_, err := r.MaterializeAllAssignmentsWithCount(ctx)
	return err
}

// MaterializeAllAssignmentsWithCount computes and stores all assignments,
// returning the number of assignments created.
func (r *Rebalancer) MaterializeAllAssignmentsWithCount(ctx context.Context) (int, error) {
	r.logger.Info("materializing all assignments (batch mode)")

	// Clear existing assignments
	if err := r.store.DeleteAllAssignments(ctx); err != nil {
		return 0, fmt.Errorf("clearing assignments: %w", err)
	}

	// Get all data needed
	targets, err := r.store.ListTargets(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing targets: %w", err)
	}

	allAgents, err := r.store.ListAgentsWithStatus(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing agents: %w", err)
	}

	activeAgents := make([]types.Agent, 0)
	for _, a := range allAgents {
		if a.Status == types.AgentStatusActive {
			activeAgents = append(activeAgents, a)
		}
	}

	tiers, err := r.store.ListTiers(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing tiers: %w", err)
	}

	tierMap := make(map[string]types.Tier)
	for _, tier := range tiers {
		tierMap[tier.Name] = tier
	}

	r.logger.Info("computing assignments",
		"targets", len(targets),
		"active_agents", len(activeAgents),
		"tiers", len(tiers),
	)

	// Collect all assignments in memory first
	var allAssignments []*types.TargetAssignment
	skipped := 0

	for _, target := range targets {
		// Skip archived/inactive targets
		if target.ArchivedAt != nil {
			skipped++
			continue
		}

		tier, ok := tierMap[target.Tier]
		if !ok {
			skipped++
			continue
		}

		// Filter eligible agents
		eligibleAgents := r.filterAgents(activeAgents, tier.AgentSelection)
		if len(eligibleAgents) == 0 {
			continue
		}

		// Select agents based on strategy
		var selectedAgents []types.Agent
		if tier.AgentSelection.Strategy == "all" {
			selectedAgents = eligibleAgents
		} else {
			count := tier.AgentSelection.Count
			if count == 0 {
				count = 4 // Default
			}
			selectedAgents = r.selectAgentsForTarget(target, eligibleAgents, count, tier.AgentSelection.Diversity)
		}

		// Collect assignments
		for _, agent := range selectedAgents {
			allAssignments = append(allAssignments, &types.TargetAssignment{
				TargetID:   target.ID,
				AgentID:    agent.ID,
				Tier:       tier.Name,
				AssignedBy: types.AssignedByInitial,
			})
		}
	}

	r.logger.Info("inserting assignments",
		"total_assignments", len(allAssignments),
		"skipped_targets", skipped,
	)

	// Bulk insert in batches for reliability
	batchSize := 10000
	created := 0

	for i := 0; i < len(allAssignments); i += batchSize {
		end := i + batchSize
		if end > len(allAssignments) {
			end = len(allAssignments)
		}

		batch := allAssignments[i:end]
		count, err := r.store.BulkCreateAssignments(ctx, batch)
		if err != nil {
			r.logger.Error("batch insert failed",
				"batch_start", i,
				"batch_size", len(batch),
				"error", err,
			)
			return created, fmt.Errorf("batch insert failed at offset %d: %w", i, err)
		}
		created += count

		r.logger.Debug("batch inserted",
			"batch_start", i,
			"batch_count", count,
			"total_created", created,
		)
	}

	r.logger.Info("materialization complete",
		"targets", len(targets),
		"assignments_created", created,
	)

	return created, nil
}

// =============================================================================
// HELPER METHODS (duplicated from service.go for independence)
// =============================================================================

// filterAgents returns agents matching the selection policy.
func (r *Rebalancer) filterAgents(agents []types.Agent, policy types.AgentSelectionPolicy) []types.Agent {
	var filtered []types.Agent
	for _, agent := range agents {
		if r.isEligible(&agent, policy) {
			filtered = append(filtered, agent)
		}
	}
	return filtered
}

// isEligible checks if an agent matches selection criteria.
func (r *Rebalancer) isEligible(agent *types.Agent, policy types.AgentSelectionPolicy) bool {
	// Check region filter
	if len(policy.Regions) > 0 {
		found := false
		for _, region := range policy.Regions {
			if agent.Region == region {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check excluded regions
	for _, region := range policy.ExcludeRegions {
		if agent.Region == region {
			return false
		}
	}

	// Check provider filter
	if len(policy.Providers) > 0 {
		found := false
		for _, provider := range policy.Providers {
			if agent.Provider == provider {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check required tags
	for k, v := range policy.RequireTags {
		if agent.Tags[k] != v {
			return false
		}
	}

	// Check excluded tags
	for k, v := range policy.ExcludeTags {
		if agent.Tags[k] == v {
			return false
		}
	}

	return true
}

// selectAgentsForTarget uses consistent hashing to select N agents for a target.
func (r *Rebalancer) selectAgentsForTarget(
	target types.Target,
	eligibleAgents []types.Agent,
	count int,
	diversity *types.DiversityRequirement,
) []types.Agent {
	if len(eligibleAgents) <= count {
		return eligibleAgents
	}

	// Simple selection using consistent hashing
	hash := simpleHashString(target.IP)
	startIdx := int(hash) % len(eligibleAgents)

	selected := make([]types.Agent, 0, count)
	for i := 0; i < len(eligibleAgents) && len(selected) < count; i++ {
		idx := (startIdx + i) % len(eligibleAgents)
		selected = append(selected, eligibleAgents[idx])
	}

	return selected
}

// simpleHashString generates a simple hash for consistent assignment.
func simpleHashString(s string) uint32 {
	var h uint32
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return h
}
