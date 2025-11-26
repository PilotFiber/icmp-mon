package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pilot-net/icmp-mon/pkg/types"
)

// =============================================================================
// ACTIVITY LOG
// =============================================================================

// LogActivity inserts an activity log entry.
func (s *Store) LogActivity(ctx context.Context, entry *types.ActivityLogEntry) error {
	detailsJSON, err := json.Marshal(entry.Details)
	if err != nil {
		detailsJSON = []byte("{}")
	}

	// Convert empty strings to nil for nullable fields
	var targetID, subnetID, agentID, ip interface{}
	if entry.TargetID != "" {
		targetID = entry.TargetID
	}
	if entry.SubnetID != "" {
		subnetID = entry.SubnetID
	}
	if entry.AgentID != "" {
		agentID = entry.AgentID
	}
	if entry.IP != "" {
		ip = entry.IP
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO activity_log (
			target_id, subnet_id, agent_id, ip,
			category, event_type, details, triggered_by, severity
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
	`,
		targetID,
		subnetID,
		agentID,
		ip,
		entry.Category,
		entry.EventType,
		detailsJSON,
		entry.TriggeredBy,
		entry.Severity,
	)
	return err
}

// LogTargetActivity logs an activity entry for a target.
func (s *Store) LogTargetActivity(ctx context.Context, targetID, ip, eventType, triggeredBy, severity string, details map[string]interface{}) error {
	return s.LogActivity(ctx, &types.ActivityLogEntry{
		TargetID:    targetID,
		IP:          ip,
		Category:    "target",
		EventType:   eventType,
		Details:     details,
		TriggeredBy: triggeredBy,
		Severity:    severity,
	})
}

// LogSubnetActivity logs an activity entry for a subnet.
func (s *Store) LogSubnetActivity(ctx context.Context, subnetID, eventType, triggeredBy, severity string, details map[string]interface{}) error {
	return s.LogActivity(ctx, &types.ActivityLogEntry{
		SubnetID:    subnetID,
		Category:    "subnet",
		EventType:   eventType,
		Details:     details,
		TriggeredBy: triggeredBy,
		Severity:    severity,
	})
}

// LogAgentActivity logs an activity entry for an agent.
func (s *Store) LogAgentActivity(ctx context.Context, agentID, eventType, triggeredBy, severity string, details map[string]interface{}) error {
	return s.LogActivity(ctx, &types.ActivityLogEntry{
		AgentID:     agentID,
		Category:    "agent",
		EventType:   eventType,
		Details:     details,
		TriggeredBy: triggeredBy,
		Severity:    severity,
	})
}

// LogSystemActivity logs a system-level activity entry.
func (s *Store) LogSystemActivity(ctx context.Context, eventType, triggeredBy, severity string, details map[string]interface{}) error {
	return s.LogActivity(ctx, &types.ActivityLogEntry{
		Category:    "system",
		EventType:   eventType,
		Details:     details,
		TriggeredBy: triggeredBy,
		Severity:    severity,
	})
}

// ListActivity returns recent activity log entries with optional filtering.
func (s *Store) ListActivity(ctx context.Context, filter ActivityFilter) ([]types.ActivityLogEntry, error) {
	// Build query dynamically based on filter
	where := "1=1"
	args := []interface{}{}
	argNum := 1

	if filter.TargetID != "" {
		where += fmt.Sprintf(" AND target_id = $%d", argNum)
		args = append(args, filter.TargetID)
		argNum++
	}
	if filter.SubnetID != "" {
		where += fmt.Sprintf(" AND subnet_id = $%d", argNum)
		args = append(args, filter.SubnetID)
		argNum++
	}
	if filter.AgentID != "" {
		where += fmt.Sprintf(" AND agent_id = $%d", argNum)
		args = append(args, filter.AgentID)
		argNum++
	}
	if filter.IP != "" {
		where += fmt.Sprintf(" AND ip = $%d", argNum)
		args = append(args, filter.IP)
		argNum++
	}
	if filter.Category != "" {
		where += fmt.Sprintf(" AND category = $%d", argNum)
		args = append(args, filter.Category)
		argNum++
	}
	if filter.Severity != "" {
		where += fmt.Sprintf(" AND severity = $%d", argNum)
		args = append(args, filter.Severity)
		argNum++
	}
	if !filter.Since.IsZero() {
		where += fmt.Sprintf(" AND created_at >= $%d", argNum)
		args = append(args, filter.Since)
		argNum++
	}

	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	query := fmt.Sprintf(`
		SELECT
			id, target_id, subnet_id, agent_id, host(ip),
			category, event_type, details, triggered_by, severity,
			created_at
		FROM activity_log
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d
	`, where, argNum)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []types.ActivityLogEntry
	for rows.Next() {
		var entry types.ActivityLogEntry
		var targetID, subnetID, agentID, ip *string
		var detailsJSON []byte

		if err := rows.Scan(
			&entry.ID,
			&targetID,
			&subnetID,
			&agentID,
			&ip,
			&entry.Category,
			&entry.EventType,
			&detailsJSON,
			&entry.TriggeredBy,
			&entry.Severity,
			&entry.CreatedAt,
		); err != nil {
			return nil, err
		}

		if targetID != nil {
			entry.TargetID = *targetID
		}
		if subnetID != nil {
			entry.SubnetID = *subnetID
		}
		if agentID != nil {
			entry.AgentID = *agentID
		}
		if ip != nil {
			entry.IP = *ip
		}

		if len(detailsJSON) > 0 {
			json.Unmarshal(detailsJSON, &entry.Details)
		}

		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// ActivityFilter for querying activity logs.
type ActivityFilter struct {
	TargetID string
	SubnetID string
	AgentID  string
	IP       string
	Category string
	Severity string
	Since    time.Time
	Limit    int
}

// GetRecentActivityForTarget returns recent activity for a specific target.
func (s *Store) GetRecentActivityForTarget(ctx context.Context, targetID string, limit int) ([]types.ActivityLogEntry, error) {
	return s.ListActivity(ctx, ActivityFilter{
		TargetID: targetID,
		Limit:    limit,
	})
}

// GetRecentActivityForSubnet returns recent activity for a subnet and its targets.
func (s *Store) GetRecentActivityForSubnet(ctx context.Context, subnetID string, limit int) ([]types.ActivityLogEntry, error) {
	// Get activity for the subnet directly or any target in the subnet
	query := `
		SELECT
			id, target_id, subnet_id, agent_id, host(ip),
			category, event_type, details, triggered_by, severity,
			created_at
		FROM activity_log
		WHERE subnet_id = $1
		   OR target_id IN (SELECT id FROM targets WHERE subnet_id = $1)
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := s.pool.Query(ctx, query, subnetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []types.ActivityLogEntry
	for rows.Next() {
		var entry types.ActivityLogEntry
		var targetID, snetID, agentID, ip *string
		var detailsJSON []byte

		if err := rows.Scan(
			&entry.ID,
			&targetID,
			&snetID,
			&agentID,
			&ip,
			&entry.Category,
			&entry.EventType,
			&detailsJSON,
			&entry.TriggeredBy,
			&entry.Severity,
			&entry.CreatedAt,
		); err != nil {
			return nil, err
		}

		if targetID != nil {
			entry.TargetID = *targetID
		}
		if snetID != nil {
			entry.SubnetID = *snetID
		}
		if agentID != nil {
			entry.AgentID = *agentID
		}
		if ip != nil {
			entry.IP = *ip
		}

		if len(detailsJSON) > 0 {
			json.Unmarshal(detailsJSON, &entry.Details)
		}

		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// GetActivityStats returns aggregate activity statistics.
func (s *Store) GetActivityStats(ctx context.Context, since time.Time) (map[string]int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT category || ':' || event_type, COUNT(*)
		FROM activity_log
		WHERE created_at >= $1
		GROUP BY category, event_type
		ORDER BY COUNT(*) DESC
	`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return nil, err
		}
		stats[key] = count
	}
	return stats, rows.Err()
}
