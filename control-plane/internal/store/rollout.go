package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pilot-net/icmp-mon/control-plane/internal/rollout"
)

// =============================================================================
// ROLLOUT AGENT UPDATE STATES
// =============================================================================

// RolloutAgentState tracks an individual agent's update progress.
type RolloutAgentState struct {
	AgentID     string     `json:"agent_id"`
	AgentName   string     `json:"agent_name"`
	Wave        int        `json:"wave"`
	Status      string     `json:"status"`
	FromVersion string     `json:"from_version"`
	ToVersion   string     `json:"to_version"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Error       string     `json:"error"`
}

// SetRolloutAgentState creates or updates an agent's update state for a rollout.
func (s *Store) SetRolloutAgentState(ctx context.Context, rolloutID string, state *RolloutAgentState) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO rollout_agent_states (
			rollout_id, agent_id, agent_name, wave, status,
			from_version, to_version, started_at, completed_at, error
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (rollout_id, agent_id) DO UPDATE SET
			wave = EXCLUDED.wave,
			status = EXCLUDED.status,
			started_at = COALESCE(EXCLUDED.started_at, rollout_agent_states.started_at),
			completed_at = EXCLUDED.completed_at,
			error = EXCLUDED.error
	`,
		rolloutID, state.AgentID, state.AgentName, state.Wave, state.Status,
		state.FromVersion, state.ToVersion, state.StartedAt, state.CompletedAt, state.Error,
	)
	return err
}

// GetRolloutAgentStates returns all agent states for a rollout.
func (s *Store) GetRolloutAgentStates(ctx context.Context, rolloutID string) ([]RolloutAgentState, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT agent_id, agent_name, wave, status, from_version, to_version, started_at, completed_at, error
		FROM rollout_agent_states
		WHERE rollout_id = $1
		ORDER BY wave, agent_name
	`, rolloutID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []RolloutAgentState
	for rows.Next() {
		var st RolloutAgentState
		if err := rows.Scan(
			&st.AgentID, &st.AgentName, &st.Wave, &st.Status,
			&st.FromVersion, &st.ToVersion, &st.StartedAt, &st.CompletedAt, &st.Error,
		); err != nil {
			return nil, err
		}
		states = append(states, st)
	}
	return states, nil
}

// =============================================================================
// ROLLOUT AGENT SELECTION
// =============================================================================

// RolloutAgentInfo contains basic agent information for rollout planning.
type RolloutAgentInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Region   string `json:"region"`
	Provider string `json:"provider"`
}

// GetAgentsForRollout returns agents matching the rollout filter.
func (s *Store) GetAgentsForRollout(ctx context.Context, filter *rollout.AgentFilter) ([]RolloutAgentInfo, error) {
	query := `
		SELECT id, name, version, region, provider
		FROM agents
		WHERE status = 'active'
	`
	var args []any
	argNum := 1

	if filter != nil {
		// Include specific agents (overrides other filters)
		if len(filter.IncludeAgentIDs) > 0 {
			query += " AND id = ANY($" + itoa(argNum) + ")"
			args = append(args, filter.IncludeAgentIDs)
			argNum++
		} else {
			// Exclude specific agents
			if len(filter.ExcludeAgentIDs) > 0 {
				query += " AND id != ALL($" + itoa(argNum) + ")"
				args = append(args, filter.ExcludeAgentIDs)
				argNum++
			}

			// Filter by regions
			if len(filter.Regions) > 0 {
				query += " AND region = ANY($" + itoa(argNum) + ")"
				args = append(args, filter.Regions)
				argNum++
			}

			// Filter by providers
			if len(filter.Providers) > 0 {
				query += " AND provider = ANY($" + itoa(argNum) + ")"
				args = append(args, filter.Providers)
				argNum++
			}

			// Filter by tags (all tags must match)
			if len(filter.Tags) > 0 {
				for k, v := range filter.Tags {
					query += " AND tags->>'" + k + "' = $" + itoa(argNum)
					args = append(args, v)
					argNum++
				}
			}
		}
	}

	query += " ORDER BY name"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []RolloutAgentInfo
	for rows.Next() {
		var a RolloutAgentInfo
		if err := rows.Scan(&a.ID, &a.Name, &a.Version, &a.Region, &a.Provider); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, nil
}

// GetAgentVersion returns the current version of an agent.
func (s *Store) GetAgentVersion(ctx context.Context, agentID string) (string, error) {
	var version string
	err := s.pool.QueryRow(ctx, `SELECT version FROM agents WHERE id = $1`, agentID).Scan(&version)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return version, err
}

// IsAgentHealthy checks if an agent has been sending healthy heartbeats since the given time.
func (s *Store) IsAgentHealthy(ctx context.Context, agentID string, since time.Time) (bool, error) {
	var healthy bool
	err := s.pool.QueryRow(ctx, `
		SELECT
			status = 'active' AND last_heartbeat > $2
		FROM agents
		WHERE id = $1
	`, agentID, since).Scan(&healthy)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return healthy, err
}

// GetFleetVersions returns version distribution across the fleet.
func (s *Store) GetFleetVersions(ctx context.Context) (map[string]int, int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT version, COUNT(*) as count
		FROM agents
		WHERE status = 'active'
		GROUP BY version
		ORDER BY count DESC
	`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	versions := make(map[string]int)
	total := 0
	for rows.Next() {
		var version string
		var count int
		if err := rows.Scan(&version, &count); err != nil {
			return nil, 0, err
		}
		versions[version] = count
		total += count
	}
	return versions, total, nil
}

// GetReleaseSimple retrieves a release with minimal info for rollout.
func (s *Store) GetReleaseSimple(ctx context.Context, id string) (version, checksum string, size int64, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT version, COALESCE((artifacts->>'checksum')::text, ''), COALESCE((artifacts->>'size')::bigint, 0)
		FROM agent_releases WHERE id = $1
	`, id).Scan(&version, &checksum, &size)
	if err == pgx.ErrNoRows {
		return "", "", 0, nil
	}
	return
}

// =============================================================================
// HELPERS
// =============================================================================

// itoa converts an int to a string (simple helper to avoid strconv import)
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}
