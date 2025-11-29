// Package store - Assignment-related database operations
package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// =============================================================================
// TARGET ASSIGNMENTS
// =============================================================================

// CreateAssignment creates a new target-to-agent assignment.
func (s *Store) CreateAssignment(ctx context.Context, a *types.TargetAssignment) error {
	// If ID is provided, cast it to UUID; otherwise let the database generate one
	var err error
	if a.ID != "" {
		_, err = s.pool.Exec(ctx, `
			INSERT INTO target_assignments (id, target_id, agent_id, tier, assigned_at, assigned_by)
			VALUES ($1::uuid, $2, $3, $4, $5, $6)
			ON CONFLICT (target_id, agent_id) DO UPDATE SET
				tier = EXCLUDED.tier,
				assigned_at = EXCLUDED.assigned_at,
				assigned_by = EXCLUDED.assigned_by
		`, a.ID, a.TargetID, a.AgentID, a.Tier, time.Now(), a.AssignedBy)
	} else {
		_, err = s.pool.Exec(ctx, `
			INSERT INTO target_assignments (target_id, agent_id, tier, assigned_at, assigned_by)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (target_id, agent_id) DO UPDATE SET
				tier = EXCLUDED.tier,
				assigned_at = EXCLUDED.assigned_at,
				assigned_by = EXCLUDED.assigned_by
		`, a.TargetID, a.AgentID, a.Tier, time.Now(), a.AssignedBy)
	}
	return err
}

// DeleteAssignment removes an assignment by ID.
func (s *Store) DeleteAssignment(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM target_assignments WHERE id = $1`, id)
	return err
}

// DeleteAssignmentByTargetAgent removes an assignment by target and agent.
func (s *Store) DeleteAssignmentByTargetAgent(ctx context.Context, targetID, agentID string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM target_assignments WHERE target_id = $1 AND agent_id = $2
	`, targetID, agentID)
	return err
}

// DeleteAssignmentsByAgent removes all assignments for an agent.
func (s *Store) DeleteAssignmentsByAgent(ctx context.Context, agentID string) (int64, error) {
	result, err := s.pool.Exec(ctx, `DELETE FROM target_assignments WHERE agent_id = $1`, agentID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

// DeleteAllAssignments removes all assignments (for reinitialization).
func (s *Store) DeleteAllAssignments(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM target_assignments`)
	return err
}

// BulkCreateAssignments inserts multiple assignments in a single transaction.
// This is much faster than individual inserts for large numbers of assignments.
func (s *Store) BulkCreateAssignments(ctx context.Context, assignments []*types.TargetAssignment) (int, error) {
	if len(assignments) == 0 {
		return 0, nil
	}

	// Use COPY for maximum performance
	now := time.Now()
	rows := make([][]any, len(assignments))
	for i, a := range assignments {
		rows[i] = []any{a.TargetID, a.AgentID, a.Tier, now, a.AssignedBy}
	}

	copyCount, err := s.pool.CopyFrom(
		ctx,
		pgx.Identifier{"target_assignments"},
		[]string{"target_id", "agent_id", "tier", "assigned_at", "assigned_by"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return 0, err
	}

	return int(copyCount), nil
}

// GetAssignment retrieves an assignment by ID.
func (s *Store) GetAssignment(ctx context.Context, id string) (*types.TargetAssignment, error) {
	var a types.TargetAssignment
	err := s.pool.QueryRow(ctx, `
		SELECT id, target_id, agent_id, tier, assigned_at, assigned_by
		FROM target_assignments WHERE id = $1
	`, id).Scan(&a.ID, &a.TargetID, &a.AgentID, &a.Tier, &a.AssignedAt, &a.AssignedBy)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// GetAssignmentsByAgent retrieves all assignments for a specific agent.
func (s *Store) GetAssignmentsByAgent(ctx context.Context, agentID string) ([]types.TargetAssignment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, target_id, agent_id, tier, assigned_at, assigned_by
		FROM target_assignments WHERE agent_id = $1
		ORDER BY assigned_at
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []types.TargetAssignment
	for rows.Next() {
		var a types.TargetAssignment
		if err := rows.Scan(&a.ID, &a.TargetID, &a.AgentID, &a.Tier, &a.AssignedAt, &a.AssignedBy); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, nil
}

// GetAssignmentsByTarget retrieves all assignments for a specific target.
func (s *Store) GetAssignmentsByTarget(ctx context.Context, targetID string) ([]types.TargetAssignment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, target_id, agent_id, tier, assigned_at, assigned_by
		FROM target_assignments WHERE target_id = $1
		ORDER BY assigned_at
	`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []types.TargetAssignment
	for rows.Next() {
		var a types.TargetAssignment
		if err := rows.Scan(&a.ID, &a.TargetID, &a.AgentID, &a.Tier, &a.AssignedAt, &a.AssignedBy); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, nil
}

// GetActiveAssignmentsByTarget returns assignments where the agent is active.
func (s *Store) GetActiveAssignmentsByTarget(ctx context.Context, targetID string) ([]types.TargetAssignment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ta.id, ta.target_id, ta.agent_id, ta.tier, ta.assigned_at, ta.assigned_by
		FROM target_assignments ta
		JOIN agents a ON ta.agent_id = a.id
		WHERE ta.target_id = $1
		  AND a.last_heartbeat > NOW() - INTERVAL '60 seconds'
		ORDER BY ta.assigned_at
	`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []types.TargetAssignment
	for rows.Next() {
		var a types.TargetAssignment
		if err := rows.Scan(&a.ID, &a.TargetID, &a.AgentID, &a.Tier, &a.AssignedAt, &a.AssignedBy); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, nil
}

// GetAllAssignments retrieves all assignments.
func (s *Store) GetAllAssignments(ctx context.Context) ([]types.TargetAssignment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, target_id, agent_id, tier, assigned_at, assigned_by
		FROM target_assignments
		ORDER BY assigned_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []types.TargetAssignment
	for rows.Next() {
		var a types.TargetAssignment
		if err := rows.Scan(&a.ID, &a.TargetID, &a.AgentID, &a.Tier, &a.AssignedAt, &a.AssignedBy); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, nil
}

// CountAssignmentsByAgent returns the assignment count per agent.
func (s *Store) CountAssignmentsByAgent(ctx context.Context) (map[string]int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT agent_id, COUNT(*) as cnt
		FROM target_assignments
		GROUP BY agent_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var agentID string
		var count int
		if err := rows.Scan(&agentID, &count); err != nil {
			return nil, err
		}
		counts[agentID] = count
	}
	return counts, nil
}

// =============================================================================
// ASSIGNMENT HISTORY
// =============================================================================

// LogAssignmentHistory records an assignment change for audit purposes.
func (s *Store) LogAssignmentHistory(ctx context.Context, h *types.AssignmentHistory) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO assignment_history (target_id, agent_id, action, reason, old_agent_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, h.TargetID, h.AgentID, h.Action, h.Reason, nilIfEmpty(h.OldAgentID), time.Now())
	return err
}

// GetAssignmentHistory retrieves recent assignment history.
func (s *Store) GetAssignmentHistory(ctx context.Context, limit int) ([]types.AssignmentHistory, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, target_id, agent_id, action, COALESCE(reason, ''),
		       COALESCE(old_agent_id::text, ''), created_at
		FROM assignment_history
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []types.AssignmentHistory
	for rows.Next() {
		var h types.AssignmentHistory
		if err := rows.Scan(&h.ID, &h.TargetID, &h.AgentID, &h.Action, &h.Reason, &h.OldAgentID, &h.CreatedAt); err != nil {
			return nil, err
		}
		history = append(history, h)
	}
	return history, nil
}

// =============================================================================
// AGENT STATUS HELPERS
// =============================================================================

// AgentStatusTransition represents an agent whose status needs updating.
type AgentStatusTransition struct {
	AgentID        string
	AgentName      string
	CurrentStatus  types.AgentStatus
	ComputedStatus types.AgentStatus
}

// ListAgentsWithStatus returns ALL agents with their computed status.
// Unlike ListActiveAgents, this includes offline agents too.
func (s *Store) ListAgentsWithStatus(ctx context.Context) ([]types.Agent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, region, location, provider, tags, public_ip::text,
		       executors, max_targets, version,
			CASE
				WHEN last_heartbeat IS NULL OR last_heartbeat < NOW() - INTERVAL '60 seconds' THEN 'offline'
				WHEN last_heartbeat < NOW() - INTERVAL '30 seconds' THEN 'degraded'
				ELSE 'active'
			END as status,
			last_heartbeat, created_at
		FROM agents
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanAgentRows(rows)
}

// GetAgentsForStatusTransition returns agents whose stored status differs
// from what it should be based on heartbeat timestamps.
func (s *Store) GetAgentsForStatusTransition(ctx context.Context) ([]AgentStatusTransition, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, status,
			CASE
				WHEN last_heartbeat IS NULL OR last_heartbeat < NOW() - INTERVAL '60 seconds' THEN 'offline'
				WHEN last_heartbeat < NOW() - INTERVAL '30 seconds' THEN 'degraded'
				ELSE 'active'
			END as computed_status
		FROM agents
		WHERE status != CASE
			WHEN last_heartbeat IS NULL OR last_heartbeat < NOW() - INTERVAL '60 seconds' THEN 'offline'
			WHEN last_heartbeat < NOW() - INTERVAL '30 seconds' THEN 'degraded'
			ELSE 'active'
		END
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transitions []AgentStatusTransition
	for rows.Next() {
		var t AgentStatusTransition
		if err := rows.Scan(&t.AgentID, &t.AgentName, &t.CurrentStatus, &t.ComputedStatus); err != nil {
			return nil, err
		}
		transitions = append(transitions, t)
	}
	return transitions, nil
}

// UpdateAgentStatus updates an agent's stored status.
func (s *Store) UpdateAgentStatus(ctx context.Context, agentID string, status types.AgentStatus) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE agents SET status = $2, updated_at = NOW() WHERE id = $1
	`, agentID, status)
	return err
}

// IncrementAssignmentVersion bumps the global assignment version.
// Returns the new version number.
func (s *Store) IncrementAssignmentVersion(ctx context.Context) (int64, error) {
	var version int64
	err := s.pool.QueryRow(ctx, `SELECT increment_assignment_version()`).Scan(&version)
	return version, err
}

// =============================================================================
// HELPERS
// =============================================================================

// scanAgentRows scans agent rows into a slice.
func (s *Store) scanAgentRows(rows pgx.Rows) ([]types.Agent, error) {
	var agents []types.Agent
	for rows.Next() {
		var agent types.Agent
		var tagsJSON []byte
		if err := rows.Scan(
			&agent.ID, &agent.Name, &agent.Region, &agent.Location, &agent.Provider,
			&tagsJSON, &agent.PublicIP, &agent.Executors, &agent.MaxTargets, &agent.Version,
			&agent.Status, &agent.LastHeartbeat, &agent.CreatedAt,
		); err != nil {
			return nil, err
		}
		if tagsJSON != nil {
			_ = json.Unmarshal(tagsJSON, &agent.Tags)
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
