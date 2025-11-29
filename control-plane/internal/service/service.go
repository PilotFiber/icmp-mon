// Package service contains the business logic for the control plane.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/pilot-net/icmp-mon/control-plane/internal/buffer"
	"github.com/pilot-net/icmp-mon/control-plane/internal/store"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// Service provides business logic operations.
type Service struct {
	store        *store.Store
	logger       *slog.Logger
	resultBuffer *buffer.ResultBuffer // Optional Redis buffer for probe results
}

// NewService creates a new service.
func NewService(store *store.Store, logger *slog.Logger) *Service {
	return &Service{
		store:  store,
		logger: logger,
	}
}

// SetResultBuffer sets the Redis buffer for probe results.
// When set, IngestResults will push to Redis instead of writing directly to DB.
func (s *Service) SetResultBuffer(buf *buffer.ResultBuffer) {
	s.resultBuffer = buf
}

// Store returns the underlying store for direct access (used by middleware).
func (s *Service) Store() *store.Store {
	return s.store
}

// =============================================================================
// AGENT OPERATIONS
// =============================================================================

// RegisterAgentRequest contains parameters for agent registration.
type RegisterAgentRequest struct {
	Name       string
	Region     string
	Location   string
	Provider   string
	Tags       map[string]string
	PublicIP   string
	Version    string
	Executors  []string
	MaxTargets int
}

// RegisterAgent registers a new agent or updates an existing one.
func (s *Service) RegisterAgent(ctx context.Context, req RegisterAgentRequest) (*types.Agent, error) {
	// Check if agent already exists
	existing, err := s.store.GetAgentByName(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		// Update existing agent
		existing.Region = req.Region
		existing.Location = req.Location
		existing.Provider = req.Provider
		existing.Tags = req.Tags
		existing.PublicIP = req.PublicIP
		existing.Version = req.Version
		existing.Executors = req.Executors
		existing.MaxTargets = req.MaxTargets
		existing.Status = types.AgentStatusActive
		existing.LastHeartbeat = time.Now()

		// Save updated agent to store
		if err := s.store.UpdateAgent(ctx, existing); err != nil {
			return nil, fmt.Errorf("updating agent: %w", err)
		}
		s.logger.Info("agent re-registered", "name", req.Name, "id", existing.ID, "executors", req.Executors)
		return existing, nil
	}

	// Create new agent
	agent := &types.Agent{
		ID:            uuid.New().String(),
		Name:          req.Name,
		Region:        req.Region,
		Location:      req.Location,
		Provider:      req.Provider,
		Tags:          req.Tags,
		PublicIP:      req.PublicIP,
		Version:       req.Version,
		Executors:     req.Executors,
		MaxTargets:    req.MaxTargets,
		Status:        types.AgentStatusActive,
		LastHeartbeat: time.Now(),
		CreatedAt:     time.Now(),
	}

	if err := s.store.CreateAgent(ctx, agent); err != nil {
		return nil, err
	}

	s.logger.Info("agent registered", "name", req.Name, "id", agent.ID)
	return agent, nil
}

// ProcessHeartbeat handles an agent heartbeat.
func (s *Service) ProcessHeartbeat(ctx context.Context, heartbeat types.Heartbeat) (*types.HeartbeatResponse, error) {
	// Update agent status
	if err := s.store.UpdateAgentHeartbeat(ctx, heartbeat.AgentID, heartbeat.Status); err != nil {
		return nil, err
	}

	// Record metrics
	if err := s.store.RecordAgentMetrics(ctx, heartbeat.AgentID, heartbeat); err != nil {
		s.logger.Warn("failed to record agent metrics", "agent", heartbeat.AgentID, "error", err)
		// Non-fatal, continue
	}

	// Check if assignments are stale
	currentVersion, err := s.store.GetAssignmentVersion(ctx)
	if err != nil {
		s.logger.Warn("failed to get assignment version", "error", err)
	}

	assignmentStale := currentVersion > heartbeat.AssignmentVersion

	return &types.HeartbeatResponse{
		Acknowledged:    true,
		AssignmentStale: assignmentStale,
	}, nil
}

// GetAssignments returns assignments for an agent.
// Uses persisted assignments from the target_assignments table.
// Falls back to dynamic calculation if table is empty (for backward compatibility).
func (s *Service) GetAssignments(ctx context.Context, agentID string) (*types.AssignmentSet, error) {
	// Get agent to check it exists
	agent, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return &types.AssignmentSet{}, nil
	}

	// Get current version
	version, err := s.store.GetAssignmentVersion(ctx)
	if err != nil {
		version = 0
	}

	// Try to get assignments from persisted table first
	persistedAssignments, err := s.store.GetAssignmentsByAgent(ctx, agentID)
	if err != nil {
		s.logger.Warn("failed to get persisted assignments, falling back to dynamic",
			"agent_id", agentID,
			"error", err,
		)
		return s.getAssignmentsDynamic(ctx, agent, version)
	}

	// If no persisted assignments exist, fall back to dynamic calculation
	// This handles the case where assignments haven't been materialized yet
	if len(persistedAssignments) == 0 {
		return s.getAssignmentsDynamic(ctx, agent, version)
	}

	// Convert persisted assignments to full Assignment objects
	assignments, err := s.enrichAssignments(ctx, persistedAssignments)
	if err != nil {
		s.logger.Warn("failed to enrich assignments, falling back to dynamic",
			"agent_id", agentID,
			"error", err,
		)
		return s.getAssignmentsDynamic(ctx, agent, version)
	}

	return &types.AssignmentSet{
		Version:     version,
		Assignments: assignments,
		GeneratedAt: time.Now(),
	}, nil
}

// enrichAssignments converts persisted assignments to full Assignment objects.
func (s *Service) enrichAssignments(ctx context.Context, persisted []types.TargetAssignment) ([]types.Assignment, error) {
	// Get all tiers for probe configuration
	tiers, err := s.store.ListTiers(ctx)
	if err != nil {
		return nil, err
	}
	tierMap := make(map[string]types.Tier)
	for _, t := range tiers {
		tierMap[t.Name] = t
	}

	assignments := make([]types.Assignment, 0, len(persisted))

	for _, pa := range persisted {
		// Get target details
		target, err := s.store.GetTarget(ctx, pa.TargetID)
		if err != nil || target == nil {
			continue // Skip if target no longer exists
		}

		// Get effective tier for this target's state
		effectiveTier := s.GetEffectiveTier(ctx, target, tierMap)
		if effectiveTier == nil {
			continue
		}

		assignment := types.Assignment{
			TargetID:        target.ID,
			IP:              target.IP,
			Tier:            effectiveTier.Name,
			ProbeType:       "icmp_ping",
			ProbeInterval:   effectiveTier.ProbeInterval,
			ProbeTimeout:    effectiveTier.ProbeTimeout,
			ProbeRetries:    effectiveTier.ProbeRetries,
			Tags:            target.Tags,
			ExpectedOutcome: target.ExpectedOutcome,
		}

		if target.ExpectedOutcome == nil && effectiveTier.DefaultExpectedOutcome != nil {
			assignment.ExpectedOutcome = effectiveTier.DefaultExpectedOutcome
		}

		assignments = append(assignments, assignment)
	}

	return assignments, nil
}

// getAssignmentsDynamic calculates assignments dynamically (backward compatibility).
func (s *Service) getAssignmentsDynamic(ctx context.Context, agent *types.Agent, version int64) (*types.AssignmentSet, error) {
	// Get all tiers
	tiers, err := s.store.ListTiers(ctx)
	if err != nil {
		return nil, err
	}
	tierMap := make(map[string]types.Tier)
	for _, t := range tiers {
		tierMap[t.Name] = t
	}

	// Get all active agents for assignment calculation
	agents, err := s.store.ListActiveAgents(ctx)
	if err != nil {
		return nil, err
	}

	// Get all targets
	targets, err := s.store.ListTargets(ctx)
	if err != nil {
		return nil, err
	}

	// Calculate assignments for this agent
	assignments := s.calculateAssignments(agent, agents, targets, tierMap)

	return &types.AssignmentSet{
		Version:     version,
		Assignments: assignments,
		GeneratedAt: time.Now(),
	}, nil
}

// calculateAssignments determines which targets an agent should monitor.
// Uses state-based tier selection: UNKNOWN targets use discovery tier,
// UNRESPONSIVE/EXCLUDED use smart re-check, etc.
func (s *Service) calculateAssignments(
	agent *types.Agent,
	allAgents []types.Agent,
	targets []types.Target,
	tiers map[string]types.Tier,
) []types.Assignment {
	var assignments []types.Assignment

	// Create a background context for state-based tier lookups
	ctx := context.Background()

	for _, target := range targets {
		// Skip archived targets
		if target.ArchivedAt != nil {
			continue
		}

		// Get effective tier based on monitoring state
		effectiveTier := s.GetEffectiveTier(ctx, &target, tiers)
		if effectiveTier == nil {
			continue // Target shouldn't be monitored (e.g., EXCLUDED with active coverage)
		}

		// Check if this agent should monitor this target
		if !s.shouldAssign(agent, allAgents, target, *effectiveTier) {
			continue
		}

		assignments = append(assignments, types.Assignment{
			TargetID:        target.ID,
			IP:              target.IP,
			Tier:            effectiveTier.Name,
			ProbeType:       "icmp_ping",
			ProbeInterval:   effectiveTier.ProbeInterval,
			ProbeTimeout:    effectiveTier.ProbeTimeout,
			ProbeRetries:    effectiveTier.ProbeRetries,
			Tags:            target.Tags,
			ExpectedOutcome: target.ExpectedOutcome,
		})
	}

	return assignments
}

// shouldAssign determines if an agent should monitor a target based on tier policy.
func (s *Service) shouldAssign(
	agent *types.Agent,
	allAgents []types.Agent,
	target types.Target,
	tier types.Tier,
) bool {
	policy := tier.AgentSelection

	// Filter agents based on policy
	eligibleAgents := s.filterAgents(allAgents, policy)
	if len(eligibleAgents) == 0 {
		return false
	}

	// Strategy: all - every eligible agent gets every target
	if policy.Strategy == "all" {
		return s.isEligible(agent, policy)
	}

	// Strategy: distributed - use consistent hashing to select N agents
	selectedAgents := s.selectAgentsForTarget(target, eligibleAgents, policy.Count, policy.Diversity)

	for _, selected := range selectedAgents {
		if selected.ID == agent.ID {
			return true
		}
	}

	return false
}

// filterAgents returns agents matching the selection policy.
func (s *Service) filterAgents(agents []types.Agent, policy types.AgentSelectionPolicy) []types.Agent {
	var filtered []types.Agent

	for _, agent := range agents {
		if !s.isEligible(&agent, policy) {
			continue
		}
		filtered = append(filtered, agent)
	}

	return filtered
}

// isEligible checks if an agent matches selection criteria.
func (s *Service) isEligible(agent *types.Agent, policy types.AgentSelectionPolicy) bool {
	// Check region filter
	if len(policy.Regions) > 0 {
		found := false
		for _, r := range policy.Regions {
			if agent.Region == r {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check excluded regions
	for _, r := range policy.ExcludeRegions {
		if agent.Region == r {
			return false
		}
	}

	// Check provider filter
	if len(policy.Providers) > 0 {
		found := false
		for _, p := range policy.Providers {
			if agent.Provider == p {
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
func (s *Service) selectAgentsForTarget(
	target types.Target,
	eligibleAgents []types.Agent,
	count int,
	diversity *types.DiversityRequirement,
) []types.Agent {
	if len(eligibleAgents) <= count {
		return eligibleAgents
	}

	// Simple selection for now (TODO: implement proper consistent hashing with diversity)
	// Hash the target IP to get a deterministic starting point
	hash := simpleHash(target.IP)
	startIdx := int(hash) % len(eligibleAgents)

	selected := make([]types.Agent, 0, count)
	for i := 0; i < len(eligibleAgents) && len(selected) < count; i++ {
		idx := (startIdx + i) % len(eligibleAgents)
		selected = append(selected, eligibleAgents[idx])
	}

	return selected
}

// simpleHash generates a simple hash for consistent assignment.
func simpleHash(s string) uint32 {
	var h uint32
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return h
}

// ListAgents returns all agents.
func (s *Service) ListAgents(ctx context.Context) ([]types.Agent, error) {
	return s.store.ListAgents(ctx)
}

// GetAgent returns a single agent.
func (s *Service) GetAgent(ctx context.Context, id string) (*types.Agent, error) {
	return s.store.GetAgent(ctx, id)
}

// ArchiveAgent soft-deletes an agent by setting archived_at timestamp.
// Archived agents are excluded from operational queries but remain in read queries.
func (s *Service) ArchiveAgent(ctx context.Context, agentID, reason string) error {
	// Get agent name for logging
	agent, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		return fmt.Errorf("getting agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	if err := s.store.ArchiveAgent(ctx, agentID, reason); err != nil {
		return fmt.Errorf("archiving agent: %w", err)
	}

	s.logger.Info("agent archived", "id", agentID, "name", agent.Name, "reason", reason)
	return nil
}

// UnarchiveAgent restores an archived agent.
func (s *Service) UnarchiveAgent(ctx context.Context, agentID string) error {
	agent, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		return fmt.Errorf("getting agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	if err := s.store.UnarchiveAgent(ctx, agentID); err != nil {
		return fmt.Errorf("unarchiving agent: %w", err)
	}

	s.logger.Info("agent unarchived", "id", agentID, "name", agent.Name)
	return nil
}

// UpdateAgentInfoRequest contains parameters for updating agent info.
type UpdateAgentInfoRequest struct {
	Name       string
	Region     string
	Location   string
	Provider   string
	Tags       map[string]string
	MaxTargets int
}

// UpdateAgentInfo updates user-editable agent information.
func (s *Service) UpdateAgentInfo(ctx context.Context, agentID string, req UpdateAgentInfoRequest) error {
	agent, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		return fmt.Errorf("getting agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	if err := s.store.UpdateAgentInfo(ctx, agentID, req.Name, req.Region, req.Location, req.Provider, req.Tags, req.MaxTargets); err != nil {
		return fmt.Errorf("updating agent info: %w", err)
	}

	s.logger.Info("agent info updated", "id", agentID, "name", req.Name)
	return nil
}

// =============================================================================
// AGENT METRICS
// =============================================================================

// GetAgentMetrics returns time-series metrics for an agent.
func (s *Service) GetAgentMetrics(ctx context.Context, agentID string, duration time.Duration) ([]store.AgentMetricsPoint, error) {
	return s.store.GetAgentMetrics(ctx, agentID, duration)
}

// GetAgentCurrentStats returns the most recent metrics for an agent.
func (s *Service) GetAgentCurrentStats(ctx context.Context, agentID string) (*store.AgentCurrentStats, error) {
	return s.store.GetAgentCurrentStats(ctx, agentID)
}

// GetFleetOverview returns aggregated stats for all agents.
func (s *Service) GetFleetOverview(ctx context.Context) (*store.FleetOverview, error) {
	return s.store.GetFleetOverview(ctx)
}

// GetAllAgentsCurrentStats returns current stats for all active agents.
func (s *Service) GetAllAgentsCurrentStats(ctx context.Context) ([]store.AgentCurrentStats, error) {
	return s.store.GetAllAgentsCurrentStats(ctx)
}

// =============================================================================
// TARGET OPERATIONS
// =============================================================================

// CreateTargetRequest contains parameters for target creation.
type CreateTargetRequest struct {
	IP              string
	Tier            string
	SubscriberID    string
	Tags            map[string]string
	ExpectedOutcome *types.ExpectedOutcome
}

// CreateTarget creates a new target.
func (s *Service) CreateTarget(ctx context.Context, req CreateTargetRequest) (*types.Target, error) {
	target := &types.Target{
		ID:              uuid.New().String(),
		IP:              req.IP,
		Tier:            req.Tier,
		SubscriberID:    req.SubscriberID,
		Tags:            req.Tags,
		ExpectedOutcome: req.ExpectedOutcome,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := target.Validate(); err != nil {
		return nil, err
	}

	if err := s.store.CreateTarget(ctx, target); err != nil {
		return nil, err
	}

	s.logger.Info("target created", "ip", req.IP, "tier", req.Tier, "id", target.ID)
	return target, nil
}

// ListTargets returns all targets.
func (s *Service) ListTargets(ctx context.Context) ([]types.Target, error) {
	return s.store.ListTargets(ctx)
}

// ListTargetsPaginated returns targets with pagination and filtering.
func (s *Service) ListTargetsPaginated(ctx context.Context, params store.TargetListParams) (*store.TargetListResult, error) {
	return s.store.ListTargetsPaginated(ctx, params)
}

// GetTarget returns a single target.
func (s *Service) GetTarget(ctx context.Context, id string) (*types.Target, error) {
	return s.store.GetTarget(ctx, id)
}

// =============================================================================
// TIER OPERATIONS
// =============================================================================

// ListTiers returns all tiers.
func (s *Service) ListTiers(ctx context.Context) ([]types.Tier, error) {
	return s.store.ListTiers(ctx)
}

// GetTier returns a tier by name.
func (s *Service) GetTier(ctx context.Context, name string) (*types.Tier, error) {
	return s.store.GetTier(ctx, name)
}

// CreateTier creates a new tier.
func (s *Service) CreateTier(ctx context.Context, tier *types.Tier) error {
	return s.store.CreateTier(ctx, tier)
}

// UpdateTier updates an existing tier.
func (s *Service) UpdateTier(ctx context.Context, tier *types.Tier) error {
	return s.store.UpdateTier(ctx, tier)
}

// DeleteTier deletes a tier by name.
func (s *Service) DeleteTier(ctx context.Context, name string) error {
	return s.store.DeleteTier(ctx, name)
}

// =============================================================================
// RESULT INGESTION
// =============================================================================

// IngestResults stores probe results and processes state transitions.
func (s *Service) IngestResults(ctx context.Context, batch types.ResultBatch) error {
	if len(batch.Results) == 0 {
		return nil
	}

	s.logger.Debug("ingesting results",
		"agent", batch.AgentID,
		"count", len(batch.Results))

	// Set agent ID on all results
	for i := range batch.Results {
		batch.Results[i].AgentID = batch.AgentID
	}

	// Store probe results - use Redis buffer if available, otherwise direct DB write
	if s.resultBuffer != nil {
		// Push to Redis buffer for async DB write
		if err := s.resultBuffer.Push(ctx, batch.Results); err != nil {
			s.logger.Error("failed to push results to buffer", "error", err)
			// Fall back to direct DB write
			if err := s.store.InsertProbeResults(ctx, batch.Results); err != nil {
				return err
			}
		}
	} else {
		// Direct DB write (no Redis buffer configured)
		if err := s.store.InsertProbeResults(ctx, batch.Results); err != nil {
			return err
		}
	}

	// Process state transitions based on probe results
	// This runs asynchronously to not block result ingestion
	go func() {
		bgCtx := context.Background()
		if err := s.ProcessProbeResultsBatch(bgCtx, batch.Results); err != nil {
			s.logger.Error("failed to process state transitions", "error", err)
		}
	}()

	return nil
}

// =============================================================================
// TARGET STATUS
// =============================================================================

// GetTargetStatus returns the current monitoring status for a target.
func (s *Service) GetTargetStatus(ctx context.Context, targetID string) (*store.TargetStatus, error) {
	return s.store.GetTargetStatus(ctx, targetID, 2*time.Minute)
}

// GetAllTargetStatuses returns status for all targets.
func (s *Service) GetAllTargetStatuses(ctx context.Context) ([]store.TargetStatus, error) {
	return s.store.GetAllTargetStatuses(ctx, 2*time.Minute)
}

// GetTargetHistory returns historical probe data for a target.
func (s *Service) GetTargetHistory(ctx context.Context, targetID string, window time.Duration) ([]store.ProbeHistoryPoint, error) {
	// Use 1-minute buckets for windows under 2 hours, otherwise 5-minute buckets
	bucketSize := time.Minute
	if window > 2*time.Hour {
		bucketSize = 5 * time.Minute
	}
	return s.store.GetTargetHistory(ctx, targetID, window, bucketSize)
}

// GetTargetHistoryByAgent returns per-agent historical probe data for a target.
func (s *Service) GetTargetHistoryByAgent(ctx context.Context, targetID string, window time.Duration) ([]store.AgentHistoryPoint, error) {
	// Use appropriate bucket sizes based on window
	var bucketSize time.Duration
	switch {
	case window <= time.Hour:
		bucketSize = time.Minute
	case window <= 6*time.Hour:
		bucketSize = 5 * time.Minute
	case window <= 24*time.Hour:
		bucketSize = 15 * time.Minute
	case window <= 7*24*time.Hour:
		bucketSize = time.Hour
	default:
		bucketSize = 6 * time.Hour
	}
	return s.store.GetTargetHistoryByAgent(ctx, targetID, window, bucketSize)
}

// GetLatencyTrend returns overall latency trend for the dashboard.
func (s *Service) GetLatencyTrend(ctx context.Context, window time.Duration) ([]store.ProbeHistoryPoint, error) {
	bucketSize := time.Minute
	if window > 2*time.Hour {
		bucketSize = 5 * time.Minute
	}
	return s.store.GetLatencyTrend(ctx, window, bucketSize)
}

// GetTargetLiveResults returns recent raw probe results for live monitoring.
func (s *Service) GetTargetLiveResults(ctx context.Context, targetID string, seconds int) ([]store.LiveProbeResult, error) {
	return s.store.GetTargetLiveResults(ctx, targetID, seconds)
}

// GetInMarketLatencyTrend returns in-market latency trend for the dashboard.
func (s *Service) GetInMarketLatencyTrend(ctx context.Context, window time.Duration) ([]store.ProbeHistoryPoint, error) {
	bucketSize := time.Minute
	if window > 2*time.Hour {
		bucketSize = 5 * time.Minute
	}
	return s.store.GetInMarketLatencyTrend(ctx, window, bucketSize)
}

// GetRegionLatencyMatrix returns the city-to-city latency matrix.
func (s *Service) GetRegionLatencyMatrix(ctx context.Context, window time.Duration) (*store.RegionLatencyMatrix, error) {
	return s.store.GetRegionLatencyMatrix(ctx, window)
}

// GetTargetLatencyBreakdown returns in-market latency breakdown for a target.
func (s *Service) GetTargetLatencyBreakdown(ctx context.Context, targetID string, window time.Duration) (*store.LatencyBreakdown, error) {
	return s.store.GetTargetLatencyBreakdown(ctx, targetID, window)
}

// =============================================================================
// COMMANDS (MTR, etc.)
// =============================================================================

// CreateMTRCommand creates an MTR command for a target.
func (s *Service) CreateMTRCommand(ctx context.Context, targetID, targetIP string, agentIDs []string) (*store.Command, error) {
	cmd := &store.Command{
		ID:          uuid.New().String(),
		CommandType: "mtr",
		TargetID:    targetID,
		TargetIP:    targetIP,
		AgentIDs:    agentIDs,
		Status:      "pending",
		RequestedAt: time.Now(),
	}

	// Set expiration to 5 minutes from now
	expires := time.Now().Add(5 * time.Minute)
	cmd.ExpiresAt = &expires

	if err := s.store.CreateCommand(ctx, cmd); err != nil {
		return nil, err
	}

	s.logger.Info("MTR command created", "command_id", cmd.ID, "target_id", targetID, "target_ip", targetIP)
	return cmd, nil
}

// GetCommand returns a command by ID.
func (s *Service) GetCommand(ctx context.Context, commandID string) (*store.Command, error) {
	return s.store.GetCommand(ctx, commandID)
}

// GetCommandResults returns results for a command.
func (s *Service) GetCommandResults(ctx context.Context, commandID string) ([]store.CommandResult, error) {
	return s.store.GetCommandResults(ctx, commandID)
}

// GetCommandsByTarget returns commands for a specific target.
func (s *Service) GetCommandsByTarget(ctx context.Context, targetID string, limit int) ([]store.CommandWithResults, error) {
	return s.store.GetCommandsByTarget(ctx, targetID, limit)
}

// GetPendingCommands returns pending commands for an agent.
func (s *Service) GetPendingCommands(ctx context.Context, agentID string) ([]store.Command, error) {
	return s.store.GetPendingCommands(ctx, agentID)
}

// SaveCommandResult saves a command result from an agent.
func (s *Service) SaveCommandResult(ctx context.Context, result *store.CommandResult) error {
	if err := s.store.SaveCommandResult(ctx, result); err != nil {
		return err
	}

	// Check if all expected results are in, then mark command as complete
	cmd, err := s.store.GetCommand(ctx, result.CommandID)
	if err != nil || cmd == nil {
		return err
	}

	results, err := s.store.GetCommandResults(ctx, result.CommandID)
	if err != nil {
		return err
	}

	// If we have results from all targeted agents (or it was broadcast to all), mark complete
	if len(cmd.AgentIDs) == 0 || len(results) >= len(cmd.AgentIDs) {
		s.store.UpdateCommandStatus(ctx, result.CommandID, "completed")
	}

	return nil
}

// =============================================================================
// INCIDENTS
// =============================================================================

// ListIncidents returns incidents with optional status filter.
func (s *Service) ListIncidents(ctx context.Context, status string, limit int) ([]store.Incident, error) {
	return s.store.ListIncidents(ctx, status, limit)
}

// GetIncident returns a single incident.
func (s *Service) GetIncident(ctx context.Context, id string) (*store.Incident, error) {
	return s.store.GetIncident(ctx, id)
}

// AcknowledgeIncident marks an incident as acknowledged.
func (s *Service) AcknowledgeIncident(ctx context.Context, id string, acknowledgedBy string) error {
	return s.store.AcknowledgeIncident(ctx, id, acknowledgedBy)
}

// ResolveIncident marks an incident as resolved.
func (s *Service) ResolveIncident(ctx context.Context, id string) error {
	return s.store.ResolveIncident(ctx, id)
}

// AddIncidentNote adds a note to an incident.
func (s *Service) AddIncidentNote(ctx context.Context, id string, note string) error {
	return s.store.AddIncidentNote(ctx, id, note)
}

// =============================================================================
// BASELINES
// =============================================================================

// GetBaseline returns the baseline for an agent-target pair.
func (s *Service) GetBaseline(ctx context.Context, agentID, targetID string) (*store.AgentTargetBaseline, error) {
	return s.store.GetBaseline(ctx, agentID, targetID)
}

// GetBaselinesForTarget returns all baselines for a target.
func (s *Service) GetBaselinesForTarget(ctx context.Context, targetID string) ([]store.AgentTargetBaseline, error) {
	return s.store.GetBaselinesForTarget(ctx, targetID)
}

// RecalculateAllBaselines triggers recalculation of all baselines.
func (s *Service) RecalculateAllBaselines(ctx context.Context) (int, error) {
	return s.store.RecalculateAllBaselines(ctx)
}

// =============================================================================
// REPORTS
// =============================================================================

// GetTargetReport returns a report for a target over a time window.
func (s *Service) GetTargetReport(ctx context.Context, targetID string, windowDays int) ([]store.TargetReport, error) {
	return s.store.GetTargetReport(ctx, targetID, windowDays)
}

// =============================================================================
// FLEXIBLE METRICS QUERY
// =============================================================================

// QueryMetrics executes a flexible metrics query with tag-based filtering.
func (s *Service) QueryMetrics(ctx context.Context, query *types.MetricsQuery) (*types.MetricsQueryResult, error) {
	return s.store.QueryMetrics(ctx, query)
}
