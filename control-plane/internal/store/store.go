// Package store provides database access for the control plane.
//
// # Design
//
// The store uses raw SQL with pgx for maximum performance with TimescaleDB.
// Complex queries are handled by database functions where appropriate.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// Store provides database operations.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new store with the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// NewStoreFromURL creates a new store by connecting to the given database URL.
func NewStoreFromURL(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close closes the database connection pool.
func (s *Store) Close() {
	s.pool.Close()
}

// Ping tests database connectivity.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// =============================================================================
// AGENTS
// =============================================================================

// CreateAgent registers a new agent.
func (s *Store) CreateAgent(ctx context.Context, agent *types.Agent) error {
	tagsJSON, _ := json.Marshal(agent.Tags)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO agents (id, name, region, location, provider, tags, public_ip, executors, max_targets, version, status, last_heartbeat)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`,
		agent.ID, agent.Name, agent.Region, agent.Location, agent.Provider,
		tagsJSON, agent.PublicIP, agent.Executors, agent.MaxTargets, agent.Version,
		agent.Status, time.Now(),
	)
	return err
}

// GetAgent retrieves an agent by ID.
func (s *Store) GetAgent(ctx context.Context, id string) (*types.Agent, error) {
	var agent types.Agent
	var tagsJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, region, location, provider, tags, public_ip::text, executors, max_targets, version,
			CASE
				WHEN last_heartbeat IS NULL OR last_heartbeat < NOW() - INTERVAL '60 seconds' THEN 'offline'
				WHEN last_heartbeat < NOW() - INTERVAL '30 seconds' THEN 'degraded'
				ELSE 'active'
			END as status,
			last_heartbeat, created_at
		FROM agents WHERE id = $1
	`, id).Scan(
		&agent.ID, &agent.Name, &agent.Region, &agent.Location, &agent.Provider,
		&tagsJSON, &agent.PublicIP, &agent.Executors, &agent.MaxTargets, &agent.Version,
		&agent.Status, &agent.LastHeartbeat, &agent.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(tagsJSON, &agent.Tags)
	return &agent, nil
}

// GetAgentByName retrieves an agent by name.
func (s *Store) GetAgentByName(ctx context.Context, name string) (*types.Agent, error) {
	var agent types.Agent
	var tagsJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, region, location, provider, tags, public_ip::text, executors, max_targets, version,
			CASE
				WHEN last_heartbeat IS NULL OR last_heartbeat < NOW() - INTERVAL '60 seconds' THEN 'offline'
				WHEN last_heartbeat < NOW() - INTERVAL '30 seconds' THEN 'degraded'
				ELSE 'active'
			END as status,
			last_heartbeat, created_at
		FROM agents WHERE name = $1
	`, name).Scan(
		&agent.ID, &agent.Name, &agent.Region, &agent.Location, &agent.Provider,
		&tagsJSON, &agent.PublicIP, &agent.Executors, &agent.MaxTargets, &agent.Version,
		&agent.Status, &agent.LastHeartbeat, &agent.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(tagsJSON, &agent.Tags)
	return &agent, nil
}

// ListAgents returns all agents with status computed from heartbeat age.
func (s *Store) ListAgents(ctx context.Context) ([]types.Agent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, region, location, provider, tags, public_ip::text, executors, max_targets, version,
			CASE
				WHEN last_heartbeat IS NULL OR last_heartbeat < NOW() - INTERVAL '60 seconds' THEN 'offline'
				WHEN last_heartbeat < NOW() - INTERVAL '30 seconds' THEN 'degraded'
				ELSE 'active'
			END as status,
			last_heartbeat, created_at
		FROM agents ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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
		json.Unmarshal(tagsJSON, &agent.Tags)
		agents = append(agents, agent)
	}
	return agents, nil
}

// ListActiveAgents returns agents with active status.
func (s *Store) ListActiveAgents(ctx context.Context) ([]types.Agent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, region, location, provider, tags, public_ip::text, executors, max_targets, version, status, last_heartbeat, created_at
		FROM agents WHERE status = 'active' ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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
		json.Unmarshal(tagsJSON, &agent.Tags)
		agents = append(agents, agent)
	}
	return agents, nil
}

// UpdateAgentHeartbeat updates the agent's last heartbeat time.
func (s *Store) UpdateAgentHeartbeat(ctx context.Context, agentID string, status types.AgentStatus) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE agents SET last_heartbeat = NOW(), status = $2, updated_at = NOW() WHERE id = $1
	`, agentID, status)
	return err
}

// UpdateAgent updates all fields of an existing agent.
func (s *Store) UpdateAgent(ctx context.Context, agent *types.Agent) error {
	tagsJSON, _ := json.Marshal(agent.Tags)

	_, err := s.pool.Exec(ctx, `
		UPDATE agents SET
			region = $2,
			location = $3,
			provider = $4,
			tags = $5,
			public_ip = $6,
			version = $7,
			executors = $8,
			max_targets = $9,
			status = $10,
			last_heartbeat = NOW(),
			updated_at = NOW()
		WHERE id = $1
	`, agent.ID, agent.Region, agent.Location, agent.Provider, tagsJSON,
		agent.PublicIP, agent.Version, agent.Executors, agent.MaxTargets, agent.Status)
	return err
}

// RecordAgentMetrics stores agent health metrics.
func (s *Store) RecordAgentMetrics(ctx context.Context, agentID string, heartbeat types.Heartbeat) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO agent_metrics (
			time, agent_id, status, cpu_percent, memory_mb, goroutine_count,
			public_ip, active_targets, probes_per_second, results_queued, results_shipped,
			assignment_version
		) VALUES (NOW(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`,
		agentID, heartbeat.Status, heartbeat.CPUPercent, heartbeat.MemoryMB, heartbeat.GoroutineCount,
		heartbeat.PublicIP, heartbeat.ActiveTargets, heartbeat.ProbesPerSecond, heartbeat.ResultsQueued, heartbeat.ResultsShipped,
		heartbeat.AssignmentVersion,
	)
	return err
}

// =============================================================================
// TARGETS
// =============================================================================

// CreateTarget creates a new target.
func (s *Store) CreateTarget(ctx context.Context, target *types.Target) error {
	tagsJSON, _ := json.Marshal(target.Tags)
	expectedJSON, _ := json.Marshal(target.ExpectedOutcome)

	// Handle empty subscriber_id (use NULL instead of empty string)
	var subscriberID interface{}
	if target.SubscriberID != "" {
		subscriberID = target.SubscriberID
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO targets (id, ip_address, tier, subscriber_id, tags, expected_outcome)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, target.ID, target.IP, target.Tier, subscriberID, tagsJSON, expectedJSON)
	return err
}

// AutoTargetParams contains parameters for auto-creating targets from subnets.
type AutoTargetParams struct {
	ID              string
	IP              string
	SubnetID        string
	IPType          types.IPType
	Tier            string
	Ownership       types.OwnershipType
	Origin          types.OriginType
	MonitoringState types.MonitoringState
	DisplayName     string
	Tags            map[string]string
}

// CreateAutoTarget creates a target with full auto-seeding parameters.
// Used when auto-creating targets from subnet definitions.
func (s *Store) CreateAutoTarget(ctx context.Context, params AutoTargetParams) error {
	tagsJSON, _ := json.Marshal(params.Tags)

	_, err := s.pool.Exec(ctx, `
		INSERT INTO targets (
			id, ip_address, subnet_id, ip_type, tier,
			ownership, origin, monitoring_state, display_name, tags
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (ip_address) DO NOTHING
	`, params.ID, params.IP, params.SubnetID, params.IPType, params.Tier,
		params.Ownership, params.Origin, params.MonitoringState, params.DisplayName, tagsJSON)
	return err
}

// GetTarget retrieves a target by ID.
func (s *Store) GetTarget(ctx context.Context, id string) (*types.Target, error) {
	var target types.Target
	var tagsJSON, expectedJSON []byte
	var subscriberID, subnetID *string
	err := s.pool.QueryRow(ctx, `
		SELECT id, host(ip_address), tier, subscriber_id, tags, expected_outcome,
			monitoring_state, archived_at, subnet_id,
			created_at, updated_at
		FROM targets WHERE id = $1
	`, id).Scan(
		&target.ID, &target.IP, &target.Tier, &subscriberID, &tagsJSON, &expectedJSON,
		&target.MonitoringState, &target.ArchivedAt, &subnetID,
		&target.CreatedAt, &target.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if subscriberID != nil {
		target.SubscriberID = *subscriberID
	}
	if subnetID != nil {
		target.SubnetID = subnetID
	}
	json.Unmarshal(tagsJSON, &target.Tags)
	json.Unmarshal(expectedJSON, &target.ExpectedOutcome)
	return &target, nil
}

// ListTargets returns all targets.
func (s *Store) ListTargets(ctx context.Context) ([]types.Target, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id, host(ip_address), tier, subscriber_id, tags, display_name, notes,
			subnet_id, ownership, origin, ip_type,
			monitoring_state, state_changed_at, needs_review, discovery_attempts, last_response_at,
			first_response_at, baseline_established_at,
			archived_at, archive_reason, expected_outcome, created_at, updated_at
		FROM targets ORDER BY ip_address
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanTargets(rows)
}

// ListTargetsByTier returns targets in a specific tier.
func (s *Store) ListTargetsByTier(ctx context.Context, tier string) ([]types.Target, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id, host(ip_address), tier, subscriber_id, tags, display_name, notes,
			subnet_id, ownership, origin, ip_type,
			monitoring_state, state_changed_at, needs_review, discovery_attempts, last_response_at,
			first_response_at, baseline_established_at,
			archived_at, archive_reason, expected_outcome, created_at, updated_at
		FROM targets WHERE tier = $1 ORDER BY ip_address
	`, tier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanTargets(rows)
}

// =============================================================================
// TIERS
// =============================================================================

// GetTier retrieves a tier configuration.
func (s *Store) GetTier(ctx context.Context, name string) (*types.Tier, error) {
	var tier types.Tier
	var agentSelectionJSON, alertThresholdsJSON, expectedJSON []byte
	var intervalMs, timeoutMs int

	err := s.pool.QueryRow(ctx, `
		SELECT name, display_name, probe_interval_ms, probe_timeout_ms, probe_retries,
		       agent_selection, alert_thresholds, default_expected_outcome
		FROM tiers WHERE name = $1
	`, name).Scan(
		&tier.Name, &tier.DisplayName, &intervalMs, &timeoutMs, &tier.ProbeRetries,
		&agentSelectionJSON, &alertThresholdsJSON, &expectedJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	tier.ProbeInterval = time.Duration(intervalMs) * time.Millisecond
	tier.ProbeTimeout = time.Duration(timeoutMs) * time.Millisecond
	json.Unmarshal(agentSelectionJSON, &tier.AgentSelection)
	json.Unmarshal(expectedJSON, &tier.DefaultExpectedOutcome)

	return &tier, nil
}

// ListTiers returns all tiers.
func (s *Store) ListTiers(ctx context.Context) ([]types.Tier, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT name, display_name, probe_interval_ms, probe_timeout_ms, probe_retries,
		       agent_selection, alert_thresholds, default_expected_outcome
		FROM tiers ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tiers []types.Tier
	for rows.Next() {
		var tier types.Tier
		var agentSelectionJSON, alertThresholdsJSON, expectedJSON []byte
		var intervalMs, timeoutMs int

		if err := rows.Scan(
			&tier.Name, &tier.DisplayName, &intervalMs, &timeoutMs, &tier.ProbeRetries,
			&agentSelectionJSON, &alertThresholdsJSON, &expectedJSON,
		); err != nil {
			return nil, err
		}

		tier.ProbeInterval = time.Duration(intervalMs) * time.Millisecond
		tier.ProbeTimeout = time.Duration(timeoutMs) * time.Millisecond
		json.Unmarshal(agentSelectionJSON, &tier.AgentSelection)
		json.Unmarshal(expectedJSON, &tier.DefaultExpectedOutcome)
		tiers = append(tiers, tier)
	}
	return tiers, nil
}

// CreateTier creates a new tier.
func (s *Store) CreateTier(ctx context.Context, tier *types.Tier) error {
	agentSelectionJSON, err := json.Marshal(tier.AgentSelection)
	if err != nil {
		return err
	}

	var expectedJSON []byte
	if tier.DefaultExpectedOutcome != nil {
		expectedJSON, err = json.Marshal(tier.DefaultExpectedOutcome)
		if err != nil {
			return err
		}
	}

	intervalMs := int(tier.ProbeInterval.Milliseconds())
	timeoutMs := int(tier.ProbeTimeout.Milliseconds())

	_, err = s.pool.Exec(ctx, `
		INSERT INTO tiers (name, display_name, probe_interval_ms, probe_timeout_ms, probe_retries,
		                   agent_selection, default_expected_outcome)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, tier.Name, tier.DisplayName, intervalMs, timeoutMs, tier.ProbeRetries,
		agentSelectionJSON, expectedJSON)

	return err
}

// UpdateTier updates an existing tier.
func (s *Store) UpdateTier(ctx context.Context, tier *types.Tier) error {
	agentSelectionJSON, err := json.Marshal(tier.AgentSelection)
	if err != nil {
		return err
	}

	var expectedJSON []byte
	if tier.DefaultExpectedOutcome != nil {
		expectedJSON, err = json.Marshal(tier.DefaultExpectedOutcome)
		if err != nil {
			return err
		}
	}

	intervalMs := int(tier.ProbeInterval.Milliseconds())
	timeoutMs := int(tier.ProbeTimeout.Milliseconds())

	result, err := s.pool.Exec(ctx, `
		UPDATE tiers
		SET display_name = $2, probe_interval_ms = $3, probe_timeout_ms = $4,
		    probe_retries = $5, agent_selection = $6, default_expected_outcome = $7
		WHERE name = $1
	`, tier.Name, tier.DisplayName, intervalMs, timeoutMs, tier.ProbeRetries,
		agentSelectionJSON, expectedJSON)

	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("tier not found: %s", tier.Name)
	}
	return nil
}

// DeleteTier deletes a tier by name.
func (s *Store) DeleteTier(ctx context.Context, name string) error {
	// Check if any targets use this tier
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM targets WHERE tier = $1`, name).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("cannot delete tier '%s': %d targets are using it", name, count)
	}

	result, err := s.pool.Exec(ctx, `DELETE FROM tiers WHERE name = $1`, name)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("tier not found: %s", name)
	}
	return nil
}

// =============================================================================
// PROBE RESULTS
// =============================================================================

// InsertProbeResults inserts a batch of probe results.
func (s *Store) InsertProbeResults(ctx context.Context, results []types.ProbeResult) error {
	if len(results) == 0 {
		return nil
	}

	// Use COPY for bulk insert (much faster)
	rows := make([][]any, len(results))
	for i, r := range results {
		rows[i] = []any{
			r.Timestamp, r.TargetID, r.AgentID, r.Success, r.Error,
			getLatency(r.Payload), getPacketLoss(r.Payload), r.Payload,
		}
	}

	_, err := s.pool.CopyFrom(ctx,
		pgx.Identifier{"probe_results"},
		[]string{"time", "target_id", "agent_id", "success", "error_message", "latency_ms", "packet_loss_pct", "payload"},
		pgx.CopyFromRows(rows),
	)
	return err
}

// GetRecentResults returns recent probe results for a target.
func (s *Store) GetRecentResults(ctx context.Context, targetID string, since time.Duration) ([]types.ProbeResult, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT time, target_id, agent_id, success, error_message, latency_ms, packet_loss_pct, payload
		FROM probe_results
		WHERE target_id = $1 AND time > NOW() - $2
		ORDER BY time DESC
	`, targetID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []types.ProbeResult
	for rows.Next() {
		var r types.ProbeResult
		var latency, packetLoss *float64
		if err := rows.Scan(&r.Timestamp, &r.TargetID, &r.AgentID, &r.Success, &r.Error, &latency, &packetLoss, &r.Payload); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// =============================================================================
// ASSIGNMENT VERSION
// =============================================================================

// GetAssignmentVersion returns the current assignment version.
func (s *Store) GetAssignmentVersion(ctx context.Context) (int64, error) {
	var version int64
	err := s.pool.QueryRow(ctx, `SELECT version FROM assignment_version WHERE id = 1`).Scan(&version)
	return version, err
}

// =============================================================================
// HELPERS
// =============================================================================

func getLatency(payload json.RawMessage) *float64 {
	var p struct {
		AvgMs float64 `json:"avg_ms"`
	}
	if err := json.Unmarshal(payload, &p); err == nil && p.AvgMs > 0 {
		return &p.AvgMs
	}
	return nil
}

func getPacketLoss(payload json.RawMessage) *float64 {
	var p struct {
		PacketLoss float64 `json:"packet_loss_pct"`
	}
	if err := json.Unmarshal(payload, &p); err == nil {
		return &p.PacketLoss
	}
	return nil
}

// =============================================================================
// TARGET STATUS & METRICS
// =============================================================================

// TargetStatus represents the current monitoring status of a target.
type TargetStatus struct {
	TargetID         string    `json:"target_id"`
	IP               string    `json:"ip"`
	Tier             string    `json:"tier"`
	Status           string    `json:"status"` // healthy, degraded, down, unknown
	AvgLatencyMs     *float64  `json:"avg_latency_ms"`
	MinLatencyMs     *float64  `json:"min_latency_ms"`
	MaxLatencyMs     *float64  `json:"max_latency_ms"`
	PacketLossPct    *float64  `json:"packet_loss_pct"`
	ReachableAgents  int       `json:"reachable_agents"`
	TotalAgents      int       `json:"total_agents"`
	LastProbe        time.Time `json:"last_probe"`
	ProbeCount       int       `json:"probe_count"`
}

// GetTargetStatus returns the current status for a single target.
func (s *Store) GetTargetStatus(ctx context.Context, targetID string, window time.Duration) (*TargetStatus, error) {
	var status TargetStatus
	status.TargetID = targetID
	cutoffTime := time.Now().Add(-window)

	err := s.pool.QueryRow(ctx, `
		SELECT
			host(t.ip_address),
			t.tier,
			COALESCE(COUNT(DISTINCT pr.agent_id), 0) as total_agents,
			COALESCE(SUM(CASE WHEN pr.success THEN 1 ELSE 0 END), 0) as reachable_agents,
			AVG(pr.latency_ms) FILTER (WHERE pr.success) as avg_latency_ms,
			MIN(pr.latency_ms) FILTER (WHERE pr.success) as min_latency_ms,
			MAX(pr.latency_ms) FILTER (WHERE pr.success) as max_latency_ms,
			AVG(pr.packet_loss_pct) as packet_loss_pct,
			MAX(pr.time) as last_probe,
			COUNT(*) as probe_count
		FROM targets t
		LEFT JOIN probe_results pr ON t.id = pr.target_id AND pr.time > $2
		WHERE t.id = $1
		GROUP BY t.id, t.ip_address, t.tier
	`, targetID, cutoffTime).Scan(
		&status.IP, &status.Tier,
		&status.TotalAgents, &status.ReachableAgents,
		&status.AvgLatencyMs, &status.MinLatencyMs, &status.MaxLatencyMs,
		&status.PacketLossPct, &status.LastProbe, &status.ProbeCount,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Determine status
	status.Status = calculateStatus(status.ReachableAgents, status.TotalAgents, status.PacketLossPct)
	return &status, nil
}

// GetAllTargetStatuses returns status for all targets.
func (s *Store) GetAllTargetStatuses(ctx context.Context, window time.Duration) ([]TargetStatus, error) {
	cutoffTime := time.Now().Add(-window)
	rows, err := s.pool.Query(ctx, `
		SELECT
			t.id,
			host(t.ip_address),
			t.tier,
			COUNT(DISTINCT pr.agent_id) FILTER (WHERE pr.time IS NOT NULL) as total_agents,
			COUNT(DISTINCT pr.agent_id) FILTER (WHERE pr.success) as reachable_agents,
			AVG(pr.latency_ms) FILTER (WHERE pr.success) as avg_latency_ms,
			MIN(pr.latency_ms) FILTER (WHERE pr.success) as min_latency_ms,
			MAX(pr.latency_ms) FILTER (WHERE pr.success) as max_latency_ms,
			AVG(pr.packet_loss_pct) as packet_loss_pct,
			MAX(pr.time) as last_probe,
			COUNT(pr.*) as probe_count
		FROM targets t
		LEFT JOIN probe_results pr ON t.id = pr.target_id AND pr.time > $1
		GROUP BY t.id, t.ip_address, t.tier
		ORDER BY t.ip_address
	`, cutoffTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var statuses []TargetStatus
	for rows.Next() {
		var status TargetStatus
		var lastProbe *time.Time
		if err := rows.Scan(
			&status.TargetID, &status.IP, &status.Tier,
			&status.TotalAgents, &status.ReachableAgents,
			&status.AvgLatencyMs, &status.MinLatencyMs, &status.MaxLatencyMs,
			&status.PacketLossPct, &lastProbe, &status.ProbeCount,
		); err != nil {
			return nil, err
		}
		if lastProbe != nil {
			status.LastProbe = *lastProbe
		}
		status.Status = calculateStatus(status.ReachableAgents, status.TotalAgents, status.PacketLossPct)
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// ProbeHistoryPoint represents a single data point in probe history.
type ProbeHistoryPoint struct {
	Time          time.Time `json:"time"`
	AvgLatencyMs  *float64  `json:"avg_latency_ms"`
	MinLatencyMs  *float64  `json:"min_latency_ms"`
	MaxLatencyMs  *float64  `json:"max_latency_ms"`
	PacketLossPct *float64  `json:"packet_loss_pct"`
	SuccessCount  int       `json:"success_count"`
	TotalCount    int       `json:"total_count"`
}

// GetTargetHistory returns historical probe data for charts.
func (s *Store) GetTargetHistory(ctx context.Context, targetID string, window time.Duration, bucketSize time.Duration) ([]ProbeHistoryPoint, error) {
	cutoffTime := time.Now().Add(-window)
	bucketInterval := fmt.Sprintf("%d seconds", int(bucketSize.Seconds()))
	rows, err := s.pool.Query(ctx, `
		SELECT
			time_bucket($3::interval, time) as bucket,
			AVG(latency_ms) FILTER (WHERE success) as avg_latency_ms,
			MIN(latency_ms) FILTER (WHERE success) as min_latency_ms,
			MAX(latency_ms) FILTER (WHERE success) as max_latency_ms,
			AVG(packet_loss_pct) as packet_loss_pct,
			SUM(CASE WHEN success THEN 1 ELSE 0 END) as success_count,
			COUNT(*) as total_count
		FROM probe_results
		WHERE target_id = $1 AND time > $2
		GROUP BY bucket
		ORDER BY bucket ASC
	`, targetID, cutoffTime, bucketInterval)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []ProbeHistoryPoint
	for rows.Next() {
		var point ProbeHistoryPoint
		if err := rows.Scan(
			&point.Time,
			&point.AvgLatencyMs, &point.MinLatencyMs, &point.MaxLatencyMs,
			&point.PacketLossPct, &point.SuccessCount, &point.TotalCount,
		); err != nil {
			return nil, err
		}
		history = append(history, point)
	}
	return history, nil
}

// AgentHistoryPoint represents a single data point with agent info.
type AgentHistoryPoint struct {
	Time          time.Time `json:"time"`
	AgentID       string    `json:"agent_id"`
	AgentName     string    `json:"agent_name"`
	AvgLatencyMs  *float64  `json:"avg_latency_ms"`
	MinLatencyMs  *float64  `json:"min_latency_ms"`
	MaxLatencyMs  *float64  `json:"max_latency_ms"`
	PacketLossPct *float64  `json:"packet_loss_pct"`
	SuccessCount  int       `json:"success_count"`
	TotalCount    int       `json:"total_count"`
}

// GetTargetHistoryByAgent returns per-agent probe history for a target.
// Uses continuous aggregates for efficiency on larger time windows.
func (s *Store) GetTargetHistoryByAgent(ctx context.Context, targetID string, window time.Duration, bucketSize time.Duration) ([]AgentHistoryPoint, error) {
	cutoffTime := time.Now().Add(-window)
	bucketInterval := fmt.Sprintf("%d seconds", int(bucketSize.Seconds()))

	// Choose the right source based on bucket size
	// < 1h: raw probe_results, 1h-24h: probe_hourly, > 24h: probe_daily
	var query string
	if bucketSize < time.Hour {
		query = `
			SELECT
				time_bucket($3::interval, pr.time) as bucket,
				pr.agent_id,
				COALESCE(a.name, pr.agent_id::text) as agent_name,
				AVG(pr.latency_ms) FILTER (WHERE pr.success) as avg_latency_ms,
				MIN(pr.latency_ms) FILTER (WHERE pr.success) as min_latency_ms,
				MAX(pr.latency_ms) FILTER (WHERE pr.success) as max_latency_ms,
				AVG(pr.packet_loss_pct) as packet_loss_pct,
				SUM(CASE WHEN pr.success THEN 1 ELSE 0 END) as success_count,
				COUNT(*) as total_count
			FROM probe_results pr
			LEFT JOIN agents a ON a.id = pr.agent_id
			WHERE pr.target_id = $1 AND pr.time > $2
			GROUP BY bucket, pr.agent_id, a.name
			ORDER BY bucket ASC, a.name
		`
	} else if window <= 24*time.Hour {
		query = `
			SELECT
				time_bucket($3::interval, ph.bucket) as bucket,
				ph.agent_id,
				COALESCE(a.name, ph.agent_id::text) as agent_name,
				AVG(ph.avg_latency) as avg_latency_ms,
				MIN(ph.min_latency) as min_latency_ms,
				MAX(ph.max_latency) as max_latency_ms,
				AVG(ph.avg_packet_loss) as packet_loss_pct,
				SUM(ph.success_count) as success_count,
				SUM(ph.probe_count) as total_count
			FROM probe_hourly ph
			LEFT JOIN agents a ON a.id = ph.agent_id
			WHERE ph.target_id = $1 AND ph.bucket > $2
			GROUP BY time_bucket($3::interval, ph.bucket), ph.agent_id, a.name
			ORDER BY bucket ASC, a.name
		`
	} else {
		query = `
			SELECT
				time_bucket($3::interval, pd.bucket) as bucket,
				pd.agent_id,
				COALESCE(a.name, pd.agent_id::text) as agent_name,
				AVG(pd.avg_latency) as avg_latency_ms,
				MIN(pd.min_latency) as min_latency_ms,
				MAX(pd.max_latency) as max_latency_ms,
				AVG(pd.avg_packet_loss) as packet_loss_pct,
				SUM(pd.success_count) as success_count,
				SUM(pd.probe_count) as total_count
			FROM probe_daily pd
			LEFT JOIN agents a ON a.id = pd.agent_id
			WHERE pd.target_id = $1 AND pd.bucket > $2
			GROUP BY time_bucket($3::interval, pd.bucket), pd.agent_id, a.name
			ORDER BY bucket ASC, a.name
		`
	}

	rows, err := s.pool.Query(ctx, query, targetID, cutoffTime, bucketInterval)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []AgentHistoryPoint
	for rows.Next() {
		var point AgentHistoryPoint
		if err := rows.Scan(
			&point.Time,
			&point.AgentID, &point.AgentName,
			&point.AvgLatencyMs, &point.MinLatencyMs, &point.MaxLatencyMs,
			&point.PacketLossPct, &point.SuccessCount, &point.TotalCount,
		); err != nil {
			return nil, err
		}
		history = append(history, point)
	}
	return history, nil
}

// LiveProbeResult represents a single raw probe result for live view.
type LiveProbeResult struct {
	Time          time.Time `json:"time"`
	AgentID       string    `json:"agent_id"`
	AgentName     string    `json:"agent_name"`
	AgentRegion   string    `json:"agent_region"`
	AgentProvider string    `json:"agent_provider"`
	LatencyMs     *float64  `json:"latency_ms"`
	PacketLossPct float64   `json:"packet_loss_pct"`
	Success       bool      `json:"success"`
}

// GetTargetLiveResults returns the most recent raw probe results for live monitoring.
func (s *Store) GetTargetLiveResults(ctx context.Context, targetID string, seconds int) ([]LiveProbeResult, error) {
	if seconds <= 0 {
		seconds = 60
	}
	if seconds > 300 {
		seconds = 300 // Cap at 5 minutes
	}

	cutoffTime := time.Now().Add(-time.Duration(seconds) * time.Second)

	rows, err := s.pool.Query(ctx, `
		SELECT
			pr.time,
			pr.agent_id,
			COALESCE(a.name, pr.agent_id::text) as agent_name,
			COALESCE(a.region, '') as agent_region,
			COALESCE(a.provider, '') as agent_provider,
			pr.latency_ms,
			pr.packet_loss_pct,
			pr.success
		FROM probe_results pr
		LEFT JOIN agents a ON a.id = pr.agent_id
		WHERE pr.target_id = $1 AND pr.time > $2
		ORDER BY pr.time DESC
		LIMIT 500
	`, targetID, cutoffTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []LiveProbeResult
	for rows.Next() {
		var r LiveProbeResult
		if err := rows.Scan(
			&r.Time,
			&r.AgentID, &r.AgentName, &r.AgentRegion, &r.AgentProvider,
			&r.LatencyMs, &r.PacketLossPct, &r.Success,
		); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// GetLatencyTrend returns aggregated latency data for the dashboard chart.
func (s *Store) GetLatencyTrend(ctx context.Context, window time.Duration, bucketSize time.Duration) ([]ProbeHistoryPoint, error) {
	cutoffTime := time.Now().Add(-window)
	bucketInterval := fmt.Sprintf("%d seconds", int(bucketSize.Seconds()))
	rows, err := s.pool.Query(ctx, `
		SELECT
			time_bucket($2::interval, time) as bucket,
			AVG(latency_ms) FILTER (WHERE success) as avg_latency_ms,
			MIN(latency_ms) FILTER (WHERE success) as min_latency_ms,
			MAX(latency_ms) FILTER (WHERE success) as max_latency_ms,
			percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE success) as p95_latency_ms,
			AVG(packet_loss_pct) as packet_loss_pct,
			SUM(CASE WHEN success THEN 1 ELSE 0 END) as success_count,
			COUNT(*) as total_count
		FROM probe_results
		WHERE time > $1
		GROUP BY bucket
		ORDER BY bucket ASC
	`, cutoffTime, bucketInterval)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []ProbeHistoryPoint
	for rows.Next() {
		var point ProbeHistoryPoint
		var p95 *float64
		if err := rows.Scan(
			&point.Time,
			&point.AvgLatencyMs, &point.MinLatencyMs, &point.MaxLatencyMs, &p95,
			&point.PacketLossPct, &point.SuccessCount, &point.TotalCount,
		); err != nil {
			return nil, err
		}
		// Store p95 in MaxLatencyMs for dashboard purposes
		if p95 != nil {
			point.MaxLatencyMs = p95
		}
		history = append(history, point)
	}
	return history, nil
}

// calculateStatus determines target status from metrics.
func calculateStatus(reachable, total int, packetLoss *float64) string {
	if total == 0 {
		return "unknown"
	}
	if reachable == 0 {
		return "down"
	}
	if reachable < total || (packetLoss != nil && *packetLoss > 5) {
		return "degraded"
	}
	return "healthy"
}

// =============================================================================
// COMMANDS (MTR, etc.)
// =============================================================================

// Command represents an on-demand command to agents.
type Command struct {
	ID          string            `json:"id"`
	CommandType string            `json:"command_type"`
	TargetIP    string            `json:"target_ip,omitempty"`
	Params      map[string]any    `json:"params,omitempty"`
	AgentIDs    []string          `json:"agent_ids,omitempty"`
	Status      string            `json:"status"`
	RequestedBy string            `json:"requested_by,omitempty"`
	RequestedAt time.Time         `json:"requested_at"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}

// CommandResult represents a result from an agent for a command.
type CommandResult struct {
	CommandID   string          `json:"command_id"`
	AgentID     string          `json:"agent_id"`
	AgentName   string          `json:"agent_name,omitempty"`
	Success     bool            `json:"success"`
	Error       string          `json:"error,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	DurationMs  int             `json:"duration_ms"`
	CompletedAt time.Time       `json:"completed_at"`
}

// CreateCommand creates a new command for agents.
func (s *Store) CreateCommand(ctx context.Context, cmd *Command) error {
	paramsJSON, _ := json.Marshal(cmd.Params)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO commands (id, command_type, target_ip, params, agent_ids, status, requested_by, requested_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, cmd.ID, cmd.CommandType, cmd.TargetIP, paramsJSON, cmd.AgentIDs, cmd.Status, cmd.RequestedBy, cmd.RequestedAt, cmd.ExpiresAt)
	return err
}

// GetCommand retrieves a command by ID.
func (s *Store) GetCommand(ctx context.Context, id string) (*Command, error) {
	var cmd Command
	var paramsJSON []byte
	var targetIP *string
	err := s.pool.QueryRow(ctx, `
		SELECT id, command_type, host(target_ip), params, agent_ids, status, requested_by, requested_at, expires_at
		FROM commands WHERE id = $1
	`, id).Scan(
		&cmd.ID, &cmd.CommandType, &targetIP, &paramsJSON, &cmd.AgentIDs,
		&cmd.Status, &cmd.RequestedBy, &cmd.RequestedAt, &cmd.ExpiresAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if targetIP != nil {
		cmd.TargetIP = *targetIP
	}
	json.Unmarshal(paramsJSON, &cmd.Params)
	return &cmd, nil
}

// GetPendingCommands returns pending commands for an agent.
func (s *Store) GetPendingCommands(ctx context.Context, agentID string) ([]Command, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, command_type, host(target_ip), params, agent_ids, status, requested_by, requested_at, expires_at
		FROM commands
		WHERE status = 'pending'
		  AND (agent_ids IS NULL OR cardinality(agent_ids) = 0 OR $1 = ANY(agent_ids))
		  AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY requested_at ASC
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commands []Command
	for rows.Next() {
		var cmd Command
		var paramsJSON []byte
		var targetIP *string
		if err := rows.Scan(
			&cmd.ID, &cmd.CommandType, &targetIP, &paramsJSON, &cmd.AgentIDs,
			&cmd.Status, &cmd.RequestedBy, &cmd.RequestedAt, &cmd.ExpiresAt,
		); err != nil {
			return nil, err
		}
		if targetIP != nil {
			cmd.TargetIP = *targetIP
		}
		json.Unmarshal(paramsJSON, &cmd.Params)
		commands = append(commands, cmd)
	}
	return commands, nil
}

// SaveCommandResult saves a command result from an agent.
func (s *Store) SaveCommandResult(ctx context.Context, result *CommandResult) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO command_results (command_id, agent_id, success, error_message, payload, duration_ms, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (command_id, agent_id) DO UPDATE SET
			success = EXCLUDED.success,
			error_message = EXCLUDED.error_message,
			payload = EXCLUDED.payload,
			duration_ms = EXCLUDED.duration_ms,
			completed_at = EXCLUDED.completed_at
	`, result.CommandID, result.AgentID, result.Success, result.Error, result.Payload, result.DurationMs, result.CompletedAt)
	return err
}

// GetCommandResults returns all results for a command.
func (s *Store) GetCommandResults(ctx context.Context, commandID string) ([]CommandResult, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT cr.command_id, cr.agent_id, a.name, cr.success, cr.error_message, cr.payload, cr.duration_ms, cr.completed_at
		FROM command_results cr
		JOIN agents a ON cr.agent_id = a.id
		WHERE cr.command_id = $1
		ORDER BY cr.completed_at ASC
	`, commandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CommandResult
	for rows.Next() {
		var r CommandResult
		var errMsg *string
		if err := rows.Scan(
			&r.CommandID, &r.AgentID, &r.AgentName, &r.Success, &errMsg, &r.Payload, &r.DurationMs, &r.CompletedAt,
		); err != nil {
			return nil, err
		}
		if errMsg != nil {
			r.Error = *errMsg
		}
		results = append(results, r)
	}
	return results, nil
}

// UpdateCommandStatus updates the status of a command.
func (s *Store) UpdateCommandStatus(ctx context.Context, commandID string, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE commands SET status = $2 WHERE id = $1`, commandID, status)
	return err
}

// =============================================================================
// BASELINES
// =============================================================================

// AgentTargetBaseline represents the baseline metrics for an agent-target pair.
type AgentTargetBaseline struct {
	AgentID            string    `json:"agent_id"`
	TargetID           string    `json:"target_id"`
	LatencyP50         *float64  `json:"latency_p50"`
	LatencyP95         *float64  `json:"latency_p95"`
	LatencyP99         *float64  `json:"latency_p99"`
	LatencyStddev      *float64  `json:"latency_stddev"`
	PacketLossBaseline float64   `json:"packet_loss_baseline"`
	SampleCount        int       `json:"sample_count"`
	FirstSeen          time.Time `json:"first_seen"`
	LastUpdated        time.Time `json:"last_updated"`
}

// GetBaseline retrieves the baseline for an agent-target pair.
func (s *Store) GetBaseline(ctx context.Context, agentID, targetID string) (*AgentTargetBaseline, error) {
	var b AgentTargetBaseline
	err := s.pool.QueryRow(ctx, `
		SELECT agent_id, target_id, latency_p50, latency_p95, latency_p99, latency_stddev,
		       packet_loss_baseline, sample_count, first_seen, last_updated
		FROM agent_target_baseline
		WHERE agent_id = $1 AND target_id = $2
	`, agentID, targetID).Scan(
		&b.AgentID, &b.TargetID, &b.LatencyP50, &b.LatencyP95, &b.LatencyP99, &b.LatencyStddev,
		&b.PacketLossBaseline, &b.SampleCount, &b.FirstSeen, &b.LastUpdated,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// GetBaselinesForTarget retrieves all baselines for a target (from all agents).
func (s *Store) GetBaselinesForTarget(ctx context.Context, targetID string) ([]AgentTargetBaseline, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT agent_id, target_id, latency_p50, latency_p95, latency_p99, latency_stddev,
		       packet_loss_baseline, sample_count, first_seen, last_updated
		FROM agent_target_baseline
		WHERE target_id = $1
		ORDER BY agent_id
	`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var baselines []AgentTargetBaseline
	for rows.Next() {
		var b AgentTargetBaseline
		if err := rows.Scan(
			&b.AgentID, &b.TargetID, &b.LatencyP50, &b.LatencyP95, &b.LatencyP99, &b.LatencyStddev,
			&b.PacketLossBaseline, &b.SampleCount, &b.FirstSeen, &b.LastUpdated,
		); err != nil {
			return nil, err
		}
		baselines = append(baselines, b)
	}
	return baselines, nil
}

// RecalculateBaseline triggers baseline recalculation for an agent-target pair.
func (s *Store) RecalculateBaseline(ctx context.Context, agentID, targetID string) error {
	_, err := s.pool.Exec(ctx, `SELECT calculate_baseline($1, $2)`, agentID, targetID)
	return err
}

// RecalculateAllBaselines triggers recalculation of all baselines.
func (s *Store) RecalculateAllBaselines(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT recalculate_all_baselines()`).Scan(&count)
	return count, err
}

// =============================================================================
// AGENT-TARGET STATE
// =============================================================================

// AgentTargetState represents the current state of an agent-target pair.
type AgentTargetState struct {
	AgentID              string     `json:"agent_id"`
	TargetID             string     `json:"target_id"`
	Status               string     `json:"status"` // healthy, degraded, down, unknown
	StatusSince          *time.Time `json:"status_since"`
	CurrentZScore        *float64   `json:"current_z_score"`
	CurrentPacketLoss    *float64   `json:"current_packet_loss"`
	CurrentLatencyMs     *float64   `json:"current_latency_ms"`
	AnomalyStart         *time.Time `json:"anomaly_start"`
	ConsecutiveAnomalies int        `json:"consecutive_anomalies"`
	LastProbeTime        *time.Time `json:"last_probe_time"`
	LastEvaluated        time.Time  `json:"last_evaluated"`
}

// GetAgentTargetState retrieves the state for an agent-target pair.
func (s *Store) GetAgentTargetState(ctx context.Context, agentID, targetID string) (*AgentTargetState, error) {
	var state AgentTargetState
	err := s.pool.QueryRow(ctx, `
		SELECT agent_id, target_id, status, status_since, current_z_score, current_packet_loss,
		       current_latency_ms, anomaly_start, consecutive_anomalies, last_probe_time, last_evaluated
		FROM agent_target_state
		WHERE agent_id = $1 AND target_id = $2
	`, agentID, targetID).Scan(
		&state.AgentID, &state.TargetID, &state.Status, &state.StatusSince, &state.CurrentZScore,
		&state.CurrentPacketLoss, &state.CurrentLatencyMs, &state.AnomalyStart,
		&state.ConsecutiveAnomalies, &state.LastProbeTime, &state.LastEvaluated,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// UpsertAgentTargetState creates or updates agent-target state.
func (s *Store) UpsertAgentTargetState(ctx context.Context, state *AgentTargetState) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO agent_target_state (agent_id, target_id, status, status_since, current_z_score,
		       current_packet_loss, current_latency_ms, anomaly_start, consecutive_anomalies,
		       last_probe_time, last_evaluated)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (agent_id, target_id) DO UPDATE SET
			status = EXCLUDED.status,
			status_since = CASE WHEN agent_target_state.status != EXCLUDED.status THEN NOW() ELSE agent_target_state.status_since END,
			current_z_score = EXCLUDED.current_z_score,
			current_packet_loss = EXCLUDED.current_packet_loss,
			current_latency_ms = EXCLUDED.current_latency_ms,
			anomaly_start = EXCLUDED.anomaly_start,
			consecutive_anomalies = EXCLUDED.consecutive_anomalies,
			last_probe_time = EXCLUDED.last_probe_time,
			last_evaluated = NOW()
	`, state.AgentID, state.TargetID, state.Status, state.StatusSince, state.CurrentZScore,
		state.CurrentPacketLoss, state.CurrentLatencyMs, state.AnomalyStart,
		state.ConsecutiveAnomalies, state.LastProbeTime)
	return err
}

// GetAnomalousStates returns all agent-target pairs currently in anomaly.
func (s *Store) GetAnomalousStates(ctx context.Context) ([]AgentTargetState, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT agent_id, target_id, status, status_since, current_z_score, current_packet_loss,
		       current_latency_ms, anomaly_start, consecutive_anomalies, last_probe_time, last_evaluated
		FROM agent_target_state
		WHERE anomaly_start IS NOT NULL
		ORDER BY anomaly_start ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []AgentTargetState
	for rows.Next() {
		var state AgentTargetState
		if err := rows.Scan(
			&state.AgentID, &state.TargetID, &state.Status, &state.StatusSince, &state.CurrentZScore,
			&state.CurrentPacketLoss, &state.CurrentLatencyMs, &state.AnomalyStart,
			&state.ConsecutiveAnomalies, &state.LastProbeTime, &state.LastEvaluated,
		); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

// AgentAnomalyCount represents anomaly counts per agent.
type AgentAnomalyCount struct {
	AgentID         string   `json:"agent_id"`
	AnomalyCount    int      `json:"anomaly_count"`
	AffectedTargets []string `json:"affected_targets"`
}

// GetAgentAnomalyCounts returns anomaly counts grouped by agent.
func (s *Store) GetAgentAnomalyCounts(ctx context.Context) ([]AgentAnomalyCount, error) {
	rows, err := s.pool.Query(ctx, `SELECT * FROM get_agent_anomaly_counts()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []AgentAnomalyCount
	for rows.Next() {
		var c AgentAnomalyCount
		if err := rows.Scan(&c.AgentID, &c.AnomalyCount, &c.AffectedTargets); err != nil {
			return nil, err
		}
		counts = append(counts, c)
	}
	return counts, nil
}

// TargetAnomalyCount represents anomaly counts per target.
type TargetAnomalyCount struct {
	TargetID       string   `json:"target_id"`
	AnomalyCount   int      `json:"anomaly_count"`
	AffectedAgents []string `json:"affected_agents"`
}

// GetTargetAnomalyCounts returns anomaly counts grouped by target.
func (s *Store) GetTargetAnomalyCounts(ctx context.Context) ([]TargetAnomalyCount, error) {
	rows, err := s.pool.Query(ctx, `SELECT * FROM get_target_anomaly_counts()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []TargetAnomalyCount
	for rows.Next() {
		var c TargetAnomalyCount
		if err := rows.Scan(&c.TargetID, &c.AnomalyCount, &c.AffectedAgents); err != nil {
			return nil, err
		}
		counts = append(counts, c)
	}
	return counts, nil
}

// =============================================================================
// INCIDENTS
// =============================================================================

// Incident represents a detected incident.
type Incident struct {
	ID                string          `json:"id"`
	IncidentType      string          `json:"incident_type"` // target, agent, regional, global
	Severity          string          `json:"severity"`      // low, medium, high, critical
	PrimaryEntityType string          `json:"primary_entity_type,omitempty"`
	PrimaryEntityID   string          `json:"primary_entity_id,omitempty"`
	AffectedTargetIDs []string        `json:"affected_target_ids,omitempty"`
	AffectedAgentIDs  []string        `json:"affected_agent_ids,omitempty"`
	DetectedAt        time.Time       `json:"detected_at"`
	ConfirmedAt       *time.Time      `json:"confirmed_at,omitempty"`
	ResolvedAt        *time.Time      `json:"resolved_at,omitempty"`
	PeakZScore        *float64        `json:"peak_z_score,omitempty"`
	PeakPacketLoss    *float64        `json:"peak_packet_loss,omitempty"`
	PeakLatencyMs     *float64        `json:"peak_latency_ms,omitempty"`
	BaselineSnapshot  json.RawMessage `json:"baseline_snapshot,omitempty"`
	AcknowledgedBy    string          `json:"acknowledged_by,omitempty"`
	AcknowledgedAt    *time.Time      `json:"acknowledged_at,omitempty"`
	Notes             string          `json:"notes,omitempty"`
	Status            string          `json:"status"` // pending, active, acknowledged, resolved
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// CreateIncident creates a new incident.
func (s *Store) CreateIncident(ctx context.Context, incident *Incident) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO incidents (id, incident_type, severity, primary_entity_type, primary_entity_id,
		       affected_target_ids, affected_agent_ids, detected_at, confirmed_at, status,
		       baseline_snapshot, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
	`, incident.ID, incident.IncidentType, incident.Severity, incident.PrimaryEntityType,
		incident.PrimaryEntityID, incident.AffectedTargetIDs, incident.AffectedAgentIDs,
		incident.DetectedAt, incident.ConfirmedAt, incident.Status, incident.BaselineSnapshot)
	return err
}

// GetIncident retrieves an incident by ID.
func (s *Store) GetIncident(ctx context.Context, id string) (*Incident, error) {
	var inc Incident
	err := s.pool.QueryRow(ctx, `
		SELECT id, incident_type, severity, primary_entity_type, primary_entity_id,
		       affected_target_ids, affected_agent_ids, detected_at, confirmed_at, resolved_at,
		       peak_z_score, peak_packet_loss, peak_latency_ms, baseline_snapshot,
		       acknowledged_by, acknowledged_at, notes, status, created_at, updated_at
		FROM incidents WHERE id = $1
	`, id).Scan(
		&inc.ID, &inc.IncidentType, &inc.Severity, &inc.PrimaryEntityType, &inc.PrimaryEntityID,
		&inc.AffectedTargetIDs, &inc.AffectedAgentIDs, &inc.DetectedAt, &inc.ConfirmedAt, &inc.ResolvedAt,
		&inc.PeakZScore, &inc.PeakPacketLoss, &inc.PeakLatencyMs, &inc.BaselineSnapshot,
		&inc.AcknowledgedBy, &inc.AcknowledgedAt, &inc.Notes, &inc.Status, &inc.CreatedAt, &inc.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inc, nil
}

// ListIncidents returns incidents filtered by status.
func (s *Store) ListIncidents(ctx context.Context, status string, limit int) ([]Incident, error) {
	query := `
		SELECT id, incident_type, severity, primary_entity_type, primary_entity_id,
		       affected_target_ids, affected_agent_ids, detected_at, confirmed_at, resolved_at,
		       peak_z_score, peak_packet_loss, peak_latency_ms, baseline_snapshot,
		       acknowledged_by, acknowledged_at, notes, status, created_at, updated_at
		FROM incidents
	`
	var args []any
	argNum := 1

	if status != "" {
		query += fmt.Sprintf(" WHERE status = $%d", argNum)
		args = append(args, status)
		argNum++
	}

	query += " ORDER BY detected_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var incidents []Incident
	for rows.Next() {
		var inc Incident
		if err := rows.Scan(
			&inc.ID, &inc.IncidentType, &inc.Severity, &inc.PrimaryEntityType, &inc.PrimaryEntityID,
			&inc.AffectedTargetIDs, &inc.AffectedAgentIDs, &inc.DetectedAt, &inc.ConfirmedAt, &inc.ResolvedAt,
			&inc.PeakZScore, &inc.PeakPacketLoss, &inc.PeakLatencyMs, &inc.BaselineSnapshot,
			&inc.AcknowledgedBy, &inc.AcknowledgedAt, &inc.Notes, &inc.Status, &inc.CreatedAt, &inc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		incidents = append(incidents, inc)
	}
	return incidents, nil
}

// GetActiveIncidents returns all non-resolved incidents.
func (s *Store) GetActiveIncidents(ctx context.Context) ([]Incident, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, incident_type, severity, primary_entity_type, primary_entity_id,
		       affected_target_ids, affected_agent_ids, detected_at, confirmed_at, resolved_at,
		       peak_z_score, peak_packet_loss, peak_latency_ms, baseline_snapshot,
		       acknowledged_by, acknowledged_at, notes, status, created_at, updated_at
		FROM incidents
		WHERE status NOT IN ('resolved')
		ORDER BY severity DESC, detected_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var incidents []Incident
	for rows.Next() {
		var inc Incident
		if err := rows.Scan(
			&inc.ID, &inc.IncidentType, &inc.Severity, &inc.PrimaryEntityType, &inc.PrimaryEntityID,
			&inc.AffectedTargetIDs, &inc.AffectedAgentIDs, &inc.DetectedAt, &inc.ConfirmedAt, &inc.ResolvedAt,
			&inc.PeakZScore, &inc.PeakPacketLoss, &inc.PeakLatencyMs, &inc.BaselineSnapshot,
			&inc.AcknowledgedBy, &inc.AcknowledgedAt, &inc.Notes, &inc.Status, &inc.CreatedAt, &inc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		incidents = append(incidents, inc)
	}
	return incidents, nil
}

// ConfirmIncident transitions an incident from pending to active.
func (s *Store) ConfirmIncident(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE incidents SET status = 'active', confirmed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'pending'
	`, id)
	return err
}

// AcknowledgeIncident marks an incident as acknowledged.
func (s *Store) AcknowledgeIncident(ctx context.Context, id string, acknowledgedBy string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE incidents SET status = 'acknowledged', acknowledged_by = $2, acknowledged_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ('pending', 'active')
	`, id, acknowledgedBy)
	return err
}

// ResolveIncident marks an incident as resolved.
func (s *Store) ResolveIncident(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE incidents SET status = 'resolved', resolved_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status != 'resolved'
	`, id)
	return err
}

// UpdateIncidentPeaks updates the peak metrics for an incident.
func (s *Store) UpdateIncidentPeaks(ctx context.Context, id string, zScore, packetLoss, latencyMs *float64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE incidents SET
			peak_z_score = GREATEST(COALESCE(peak_z_score, 0), COALESCE($2, 0)),
			peak_packet_loss = GREATEST(COALESCE(peak_packet_loss, 0), COALESCE($3, 0)),
			peak_latency_ms = GREATEST(COALESCE(peak_latency_ms, 0), COALESCE($4, 0)),
			updated_at = NOW()
		WHERE id = $1
	`, id, zScore, packetLoss, latencyMs)
	return err
}

// AddIncidentNote appends a note to an incident.
func (s *Store) AddIncidentNote(ctx context.Context, id string, note string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE incidents SET
			notes = CASE WHEN notes IS NULL OR notes = '' THEN $2 ELSE notes || E'\n---\n' || $2 END,
			updated_at = NOW()
		WHERE id = $1
	`, id, note)
	return err
}

// =============================================================================
// REPORTING
// =============================================================================

// TargetReport represents aggregated metrics for a target.
type TargetReport struct {
	AgentName    string   `json:"agent_name"`
	AgentRegion  string   `json:"agent_region"`
	AvgLatencyMs *float64 `json:"avg_latency_ms"`
	P95LatencyMs *float64 `json:"p95_latency_ms"`
	P99LatencyMs *float64 `json:"p99_latency_ms"`
	JitterMs     *float64 `json:"jitter_ms"`
	PacketLoss   *float64 `json:"packet_loss_pct"`
	UptimePct    *float64 `json:"uptime_pct"`
	TotalProbes  int64    `json:"total_probes"`
}

// GetTargetReport retrieves a report for a target over a time window.
func (s *Store) GetTargetReport(ctx context.Context, targetID string, windowDays int) ([]TargetReport, error) {
	rows, err := s.pool.Query(ctx, `SELECT * FROM get_target_report($1, $2)`, targetID, windowDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []TargetReport
	for rows.Next() {
		var r TargetReport
		if err := rows.Scan(
			&r.AgentName, &r.AgentRegion, &r.AvgLatencyMs, &r.P95LatencyMs, &r.P99LatencyMs,
			&r.JitterMs, &r.PacketLoss, &r.UptimePct, &r.TotalProbes,
		); err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, nil
}

// =============================================================================
// FLEXIBLE METRICS QUERY
// =============================================================================

// QueryMetrics executes a flexible metrics query with tag-based filtering.
// Designed for scale: filters agents/targets first, then queries aggregates.
func (s *Store) QueryMetrics(ctx context.Context, query *types.MetricsQuery) (*types.MetricsQueryResult, error) {
	startTime := time.Now()

	// Resolve time range
	window, err := query.TimeRange.GetWindowDuration()
	if err != nil {
		return nil, fmt.Errorf("invalid time range: %w", err)
	}

	cutoffTime := time.Now().Add(-window)
	if query.TimeRange.Start != nil {
		cutoffTime = *query.TimeRange.Start
	}

	// Auto-select bucket if not specified
	bucket := query.Bucket
	if bucket == "" {
		bucket = types.AutoSelectBucket(window)
	}
	bucketInterval := bucket

	// Select the right aggregate table
	aggTable := types.SelectAggregateTable(window, bucket)

	// Apply defaults
	metrics := query.Metrics
	if len(metrics) == 0 {
		metrics = []string{"avg_latency", "packet_loss"}
	}

	groupBy := query.GroupBy
	if len(groupBy) == 0 {
		groupBy = []string{"time"}
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 10000
	}

	// Build the query with CTEs for efficient filtering
	sql, args, err := s.buildMetricsQuery(query, cutoffTime, bucketInterval, aggTable, metrics, groupBy, limit)
	if err != nil {
		return nil, fmt.Errorf("building query: %w", err)
	}

	// Execute query
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("executing query: %w", err)
	}
	defer rows.Close()

	// Parse results into series
	seriesMap := make(map[string]*types.MetricsSeries)
	var totalPoints int

	for rows.Next() {
		point, groupKey, err := s.scanMetricsRow(rows, metrics, groupBy)
		if err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		series, exists := seriesMap[groupKey.Key]
		if !exists {
			series = &types.MetricsSeries{
				AgentID:       groupKey.AgentID,
				AgentName:     groupKey.AgentName,
				AgentRegion:   groupKey.AgentRegion,
				AgentProvider: groupKey.AgentProvider,
				TargetID:      groupKey.TargetID,
				TargetIP:      groupKey.TargetIP,
				TargetTier:    groupKey.TargetTier,
				Points:        make([]types.MetricsDataPoint, 0),
			}
			seriesMap[groupKey.Key] = series
		}
		series.Points = append(series.Points, point)
		totalPoints++
	}

	// Convert map to slice
	seriesList := make([]types.MetricsSeries, 0, len(seriesMap))
	for _, s := range seriesMap {
		seriesList = append(seriesList, *s)
	}

	// Get filter match counts (for transparency)
	matchedAgents, matchedTargets := s.countFilterMatches(ctx, query)

	return &types.MetricsQueryResult{
		Query:          *query,
		ExecutedAt:     startTime,
		ExecutionMs:    time.Since(startTime).Milliseconds(),
		AggregateTable: aggTable,
		MatchedAgents:  matchedAgents,
		MatchedTargets: matchedTargets,
		Series:         seriesList,
		TotalPoints:    totalPoints,
	}, nil
}

// buildMetricsQuery constructs the SQL query with CTEs for efficient filtering.
func (s *Store) buildMetricsQuery(
	query *types.MetricsQuery,
	cutoffTime time.Time,
	bucket string,
	aggTable string,
	metrics []string,
	groupBy []string,
	limit int,
) (string, []any, error) {
	args := []any{}
	argIdx := 1

	// Build agent filter CTE
	agentCTE, agentArgs, nextIdx := buildAgentFilterCTE(query.AgentFilter, argIdx)
	args = append(args, agentArgs...)
	argIdx = nextIdx

	// Build target filter CTE
	targetCTE, targetArgs, nextIdx := buildTargetFilterCTE(query.TargetFilter, argIdx)
	args = append(args, targetArgs...)
	argIdx = nextIdx

	// Time cutoff argument
	cutoffArg := fmt.Sprintf("$%d", argIdx)
	args = append(args, cutoffTime)
	argIdx++

	// Bucket argument
	bucketArg := fmt.Sprintf("$%d", argIdx)
	args = append(args, bucket)
	argIdx++

	// Limit argument
	limitArg := fmt.Sprintf("$%d", argIdx)
	args = append(args, limit)

	// Build SELECT clause based on requested metrics
	selectCols := buildMetricsSelectClause(metrics, aggTable)

	// Build GROUP BY clause
	groupByCols := buildGroupByClause(groupBy)

	// Build the main query
	var timeCol string
	switch aggTable {
	case "probe_results":
		timeCol = "pr.time"
	default:
		timeCol = "pr.bucket"
	}

	sql := fmt.Sprintf(`
		WITH filtered_agents AS (
			%s
		),
		filtered_targets AS (
			%s
		)
		SELECT
			time_bucket(%s::interval, %s) as bucket,
			%s
			%s
		FROM %s pr
		JOIN filtered_agents fa ON pr.agent_id = fa.id
		JOIN filtered_targets ft ON pr.target_id = ft.id
		LEFT JOIN agents a ON a.id = pr.agent_id
		LEFT JOIN targets t ON t.id = pr.target_id
		WHERE %s > %s
		GROUP BY time_bucket(%s::interval, %s)%s
		ORDER BY bucket ASC
		LIMIT %s
	`, agentCTE, targetCTE, bucketArg, timeCol, buildGroupBySelectClause(groupBy), selectCols,
		aggTable, timeCol, cutoffArg, bucketArg, timeCol, groupByCols, limitArg)

	return sql, args, nil
}

// buildAgentFilterCTE creates the CTE for filtering agents.
func buildAgentFilterCTE(filter *types.AgentFilter, startIdx int) (string, []any, int) {
	if filter == nil {
		return "SELECT id FROM agents LIMIT 1000", nil, startIdx
	}

	conditions := []string{}
	args := []any{}
	idx := startIdx

	// Filter by IDs
	if len(filter.IDs) > 0 {
		conditions = append(conditions, fmt.Sprintf("id = ANY($%d)", idx))
		args = append(args, filter.IDs)
		idx++
	}

	// Filter by regions
	if len(filter.Regions) > 0 {
		conditions = append(conditions, fmt.Sprintf("region = ANY($%d)", idx))
		args = append(args, filter.Regions)
		idx++
	}

	// Filter by providers
	if len(filter.Providers) > 0 {
		conditions = append(conditions, fmt.Sprintf("provider = ANY($%d)", idx))
		args = append(args, filter.Providers)
		idx++
	}

	// Filter by tags (all must match)
	if len(filter.Tags) > 0 {
		tagsJSON, _ := json.Marshal(filter.Tags)
		conditions = append(conditions, fmt.Sprintf("tags @> $%d", idx))
		args = append(args, tagsJSON)
		idx++
	}

	// Exclude by tags
	if len(filter.ExcludeTags) > 0 {
		tagsJSON, _ := json.Marshal(filter.ExcludeTags)
		conditions = append(conditions, fmt.Sprintf("NOT tags @> $%d", idx))
		args = append(args, tagsJSON)
		idx++
	}

	if len(conditions) == 0 {
		return "SELECT id FROM agents LIMIT 1000", args, idx
	}

	sql := fmt.Sprintf("SELECT id FROM agents WHERE %s LIMIT 1000",
		joinConditions(conditions, " AND "))
	return sql, args, idx
}

// buildTargetFilterCTE creates the CTE for filtering targets.
func buildTargetFilterCTE(filter *types.TargetFilter, startIdx int) (string, []any, int) {
	if filter == nil {
		return "SELECT id FROM targets LIMIT 10000", nil, startIdx
	}

	conditions := []string{}
	args := []any{}
	idx := startIdx

	// Filter by IDs
	if len(filter.IDs) > 0 {
		conditions = append(conditions, fmt.Sprintf("id = ANY($%d)", idx))
		args = append(args, filter.IDs)
		idx++
	}

	// Filter by tiers
	if len(filter.Tiers) > 0 {
		conditions = append(conditions, fmt.Sprintf("tier = ANY($%d)", idx))
		args = append(args, filter.Tiers)
		idx++
	}

	// Filter by tags (all must match)
	if len(filter.Tags) > 0 {
		tagsJSON, _ := json.Marshal(filter.Tags)
		conditions = append(conditions, fmt.Sprintf("tags @> $%d", idx))
		args = append(args, tagsJSON)
		idx++
	}

	// Exclude by tags
	if len(filter.ExcludeTags) > 0 {
		tagsJSON, _ := json.Marshal(filter.ExcludeTags)
		conditions = append(conditions, fmt.Sprintf("NOT tags @> $%d", idx))
		args = append(args, tagsJSON)
		idx++
	}

	if len(conditions) == 0 {
		return "SELECT id FROM targets LIMIT 10000", args, idx
	}

	sql := fmt.Sprintf("SELECT id FROM targets WHERE %s LIMIT 10000",
		joinConditions(conditions, " AND "))
	return sql, args, idx
}

// buildMetricsSelectClause creates SELECT columns for requested metrics.
func buildMetricsSelectClause(metrics []string, aggTable string) string {
	cols := []string{}

	// Map metric names to SQL expressions based on aggregate table
	for _, m := range metrics {
		switch m {
		case "avg_latency":
			if aggTable == "probe_results" {
				cols = append(cols, "AVG(pr.latency_ms) FILTER (WHERE pr.success) as avg_latency")
			} else {
				cols = append(cols, "AVG(pr.avg_latency) as avg_latency")
			}
		case "min_latency":
			if aggTable == "probe_results" {
				cols = append(cols, "MIN(pr.latency_ms) FILTER (WHERE pr.success) as min_latency")
			} else {
				cols = append(cols, "MIN(pr.min_latency) as min_latency")
			}
		case "max_latency":
			if aggTable == "probe_results" {
				cols = append(cols, "MAX(pr.latency_ms) FILTER (WHERE pr.success) as max_latency")
			} else {
				cols = append(cols, "MAX(pr.max_latency) as max_latency")
			}
		case "p50_latency":
			if aggTable == "probe_results" {
				cols = append(cols, "percentile_cont(0.5) WITHIN GROUP (ORDER BY pr.latency_ms) FILTER (WHERE pr.success) as p50_latency")
			} else {
				cols = append(cols, "AVG(pr.p50_latency) as p50_latency")
			}
		case "p95_latency":
			if aggTable == "probe_results" {
				cols = append(cols, "percentile_cont(0.95) WITHIN GROUP (ORDER BY pr.latency_ms) FILTER (WHERE pr.success) as p95_latency")
			} else {
				cols = append(cols, "AVG(pr.p95_latency) as p95_latency")
			}
		case "p99_latency":
			if aggTable == "probe_results" {
				cols = append(cols, "percentile_cont(0.99) WITHIN GROUP (ORDER BY pr.latency_ms) FILTER (WHERE pr.success) as p99_latency")
			} else {
				cols = append(cols, "AVG(pr.p99_latency) as p99_latency")
			}
		case "jitter":
			if aggTable == "probe_results" {
				cols = append(cols, "STDDEV(pr.latency_ms) FILTER (WHERE pr.success) as jitter")
			} else if aggTable == "probe_hourly" {
				cols = append(cols, "AVG(pr.latency_stddev) as jitter")
			} else {
				cols = append(cols, "AVG(pr.avg_jitter) as jitter")
			}
		case "packet_loss":
			if aggTable == "probe_results" {
				cols = append(cols, "AVG(pr.packet_loss_pct) as packet_loss")
			} else {
				cols = append(cols, "AVG(pr.avg_packet_loss) as packet_loss")
			}
		case "success_rate":
			if aggTable == "probe_results" {
				cols = append(cols, "SUM(CASE WHEN pr.success THEN 1 ELSE 0 END)::float / NULLIF(COUNT(*), 0) * 100 as success_rate")
			} else {
				cols = append(cols, "SUM(pr.success_count)::float / NULLIF(SUM(pr.probe_count), 0) * 100 as success_rate")
			}
		case "probe_count":
			if aggTable == "probe_results" {
				cols = append(cols, "COUNT(*) as probe_count")
			} else {
				cols = append(cols, "SUM(pr.probe_count) as probe_count")
			}
		}
	}

	return joinConditions(cols, ", ")
}

// buildGroupByClause creates the GROUP BY columns (excluding time bucket).
func buildGroupByClause(groupBy []string) string {
	cols := []string{}
	for _, g := range groupBy {
		switch g {
		case "agent":
			cols = append(cols, "pr.agent_id", "a.name")
		case "agent_region":
			cols = append(cols, "a.region")
		case "agent_provider":
			cols = append(cols, "a.provider")
		case "target":
			cols = append(cols, "pr.target_id", "t.ip_address", "t.tier")
		case "target_tier":
			cols = append(cols, "t.tier")
		// "time" is always included via bucket
		}
	}
	if len(cols) == 0 {
		return ""
	}
	return ", " + joinConditions(cols, ", ")
}

// buildGroupBySelectClause creates SELECT columns for grouping dimensions.
func buildGroupBySelectClause(groupBy []string) string {
	cols := []string{}
	for _, g := range groupBy {
		switch g {
		case "agent":
			cols = append(cols, "pr.agent_id", "COALESCE(a.name, pr.agent_id::text) as agent_name")
		case "agent_region":
			cols = append(cols, "COALESCE(a.region, '') as agent_region")
		case "agent_provider":
			cols = append(cols, "COALESCE(a.provider, '') as agent_provider")
		case "target":
			cols = append(cols, "pr.target_id", "host(t.ip_address) as target_ip", "t.tier as target_tier")
		case "target_tier":
			cols = append(cols, "COALESCE(t.tier, '') as target_tier")
		}
	}
	if len(cols) == 0 {
		return ""
	}
	return joinConditions(cols, ", ") + ","
}

// metricsGroupKey holds grouping dimension values for result parsing.
type metricsGroupKey struct {
	Key           string
	AgentID       string
	AgentName     string
	AgentRegion   string
	AgentProvider string
	TargetID      string
	TargetIP      string
	TargetTier    string
}

// scanMetricsRow scans a result row into a data point and group key.
func (s *Store) scanMetricsRow(rows pgx.Rows, metrics []string, groupBy []string) (types.MetricsDataPoint, metricsGroupKey, error) {
	var point types.MetricsDataPoint
	var key metricsGroupKey

	// Build scan destinations dynamically
	dests := []any{&point.Time}

	// Add group-by columns
	for _, g := range groupBy {
		switch g {
		case "agent":
			dests = append(dests, &key.AgentID, &key.AgentName)
		case "agent_region":
			dests = append(dests, &key.AgentRegion)
		case "agent_provider":
			dests = append(dests, &key.AgentProvider)
		case "target":
			dests = append(dests, &key.TargetID, &key.TargetIP, &key.TargetTier)
		case "target_tier":
			dests = append(dests, &key.TargetTier)
		}
	}

	// Add metric columns
	for _, m := range metrics {
		switch m {
		case "avg_latency":
			dests = append(dests, &point.AvgLatency)
		case "min_latency":
			dests = append(dests, &point.MinLatency)
		case "max_latency":
			dests = append(dests, &point.MaxLatency)
		case "p50_latency":
			dests = append(dests, &point.P50Latency)
		case "p95_latency":
			dests = append(dests, &point.P95Latency)
		case "p99_latency":
			dests = append(dests, &point.P99Latency)
		case "jitter":
			dests = append(dests, &point.Jitter)
		case "packet_loss":
			dests = append(dests, &point.PacketLoss)
		case "success_rate":
			dests = append(dests, &point.SuccessRate)
		case "probe_count":
			dests = append(dests, &point.ProbeCount)
		}
	}

	if err := rows.Scan(dests...); err != nil {
		return point, key, err
	}

	// Build composite key for grouping
	key.Key = fmt.Sprintf("%s|%s|%s|%s|%s|%s",
		key.AgentID, key.AgentRegion, key.AgentProvider,
		key.TargetID, key.TargetIP, key.TargetTier)

	return point, key, nil
}

// countFilterMatches returns the count of agents/targets matching filters.
func (s *Store) countFilterMatches(ctx context.Context, query *types.MetricsQuery) (int, int) {
	var agentCount, targetCount int

	// Count matched agents
	agentCTE, agentArgs, _ := buildAgentFilterCTE(query.AgentFilter, 1)
	agentSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s) sub", agentCTE)
	s.pool.QueryRow(ctx, agentSQL, agentArgs...).Scan(&agentCount)

	// Count matched targets
	targetCTE, targetArgs, _ := buildTargetFilterCTE(query.TargetFilter, 1)
	targetSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s) sub", targetCTE)
	s.pool.QueryRow(ctx, targetSQL, targetArgs...).Scan(&targetCount)

	return agentCount, targetCount
}

// joinConditions joins condition strings with a separator.
func joinConditions(conditions []string, sep string) string {
	if len(conditions) == 0 {
		return ""
	}
	result := conditions[0]
	for i := 1; i < len(conditions); i++ {
		result += sep + conditions[i]
	}
	return result
}
