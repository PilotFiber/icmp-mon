package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// =============================================================================
// SUBNETS
// =============================================================================

// CreateSubnet creates a new subnet.
func (s *Store) CreateSubnet(ctx context.Context, subnet *types.Subnet) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO subnets (
			id, pilot_subnet_id, network_address, network_size,
			gateway_address, first_usable_address, last_usable_address,
			vlan_id, service_id, service_status, subscriber_id, subscriber_name,
			location_id, location_address, city, region, pop_name,
			gateway_device, state
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
		)
	`,
		subnet.ID,
		subnet.PilotSubnetID,
		subnet.NetworkAddress,
		subnet.NetworkSize,
		subnet.GatewayAddress,
		subnet.FirstUsableAddress,
		subnet.LastUsableAddress,
		subnet.VLANID,
		subnet.ServiceID,
		subnet.ServiceStatus,
		subnet.SubscriberID,
		subnet.SubscriberName,
		subnet.LocationID,
		subnet.LocationAddress,
		subnet.City,
		subnet.Region,
		subnet.POPName,
		subnet.GatewayDevice,
		"active",
	)
	if err != nil {
		return err
	}

	// Log to activity log
	triggeredBy := "api"
	if subnet.PilotSubnetID != nil {
		triggeredBy = "sync"
	}
	detailsJSON, _ := json.Marshal(map[string]interface{}{
		"network_address":  subnet.NetworkAddress,
		"network_size":     subnet.NetworkSize,
		"pilot_subnet_id":  subnet.PilotSubnetID,
		"subscriber_name":  subnet.SubscriberName,
		"pop_name":         subnet.POPName,
	})
	_, _ = tx.Exec(ctx, `
		INSERT INTO activity_log (
			subnet_id, category, event_type, details, triggered_by, severity
		) VALUES ($1, 'subnet', 'created', $2, $3, 'info')
	`, subnet.ID, detailsJSON, triggeredBy)

	return tx.Commit(ctx)
}

// GetSubnet retrieves a subnet by ID.
func (s *Store) GetSubnet(ctx context.Context, id string) (*types.Subnet, error) {
	subnet := &types.Subnet{}
	err := s.pool.QueryRow(ctx, `
		SELECT
			id, pilot_subnet_id, network_address::text, network_size,
			host(gateway_address), host(first_usable_address), host(last_usable_address),
			vlan_id, service_id, service_status, subscriber_id, subscriber_name,
			location_id, location_address, city, region, pop_name,
			gateway_device, state, archived_at, archive_reason,
			created_at, updated_at
		FROM subnets WHERE id = $1
	`, id).Scan(
		&subnet.ID,
		&subnet.PilotSubnetID,
		&subnet.NetworkAddress,
		&subnet.NetworkSize,
		&subnet.GatewayAddress,
		&subnet.FirstUsableAddress,
		&subnet.LastUsableAddress,
		&subnet.VLANID,
		&subnet.ServiceID,
		&subnet.ServiceStatus,
		&subnet.SubscriberID,
		&subnet.SubscriberName,
		&subnet.LocationID,
		&subnet.LocationAddress,
		&subnet.City,
		&subnet.Region,
		&subnet.POPName,
		&subnet.GatewayDevice,
		&subnet.State,
		&subnet.ArchivedAt,
		&subnet.ArchiveReason,
		&subnet.CreatedAt,
		&subnet.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return subnet, nil
}

// GetSubnetByPilotID retrieves a subnet by its Pilot API ID.
func (s *Store) GetSubnetByPilotID(ctx context.Context, pilotID int) (*types.Subnet, error) {
	subnet := &types.Subnet{}
	err := s.pool.QueryRow(ctx, `
		SELECT
			id, pilot_subnet_id, network_address::text, network_size,
			host(gateway_address), host(first_usable_address), host(last_usable_address),
			vlan_id, service_id, service_status, subscriber_id, subscriber_name,
			location_id, location_address, city, region, pop_name,
			gateway_device, state, archived_at, archive_reason,
			created_at, updated_at
		FROM subnets WHERE pilot_subnet_id = $1
	`, pilotID).Scan(
		&subnet.ID,
		&subnet.PilotSubnetID,
		&subnet.NetworkAddress,
		&subnet.NetworkSize,
		&subnet.GatewayAddress,
		&subnet.FirstUsableAddress,
		&subnet.LastUsableAddress,
		&subnet.VLANID,
		&subnet.ServiceID,
		&subnet.ServiceStatus,
		&subnet.SubscriberID,
		&subnet.SubscriberName,
		&subnet.LocationID,
		&subnet.LocationAddress,
		&subnet.City,
		&subnet.Region,
		&subnet.POPName,
		&subnet.GatewayDevice,
		&subnet.State,
		&subnet.ArchivedAt,
		&subnet.ArchiveReason,
		&subnet.CreatedAt,
		&subnet.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return subnet, nil
}

// ListSubnets returns all active subnets.
func (s *Store) ListSubnets(ctx context.Context) ([]types.Subnet, error) {
	return s.listSubnetsWithFilter(ctx, "state = 'active'", nil)
}

// ListSubnetsIncludeArchived returns all subnets including archived.
func (s *Store) ListSubnetsIncludeArchived(ctx context.Context) ([]types.Subnet, error) {
	return s.listSubnetsWithFilter(ctx, "1=1", nil)
}

// ListSubnetsBySubscriber returns subnets for a specific subscriber.
func (s *Store) ListSubnetsBySubscriber(ctx context.Context, subscriberID int) ([]types.Subnet, error) {
	return s.listSubnetsWithFilter(ctx, "subscriber_id = $1 AND state = 'active'", []interface{}{subscriberID})
}

// ListSubnetsByPOP returns subnets at a specific POP.
func (s *Store) ListSubnetsByPOP(ctx context.Context, popName string) ([]types.Subnet, error) {
	return s.listSubnetsWithFilter(ctx, "pop_name = $1 AND state = 'active'", []interface{}{popName})
}

func (s *Store) listSubnetsWithFilter(ctx context.Context, where string, args []interface{}) ([]types.Subnet, error) {
	query := fmt.Sprintf(`
		SELECT
			id, pilot_subnet_id, network_address::text, network_size,
			host(gateway_address), host(first_usable_address), host(last_usable_address),
			vlan_id, service_id, service_status, subscriber_id, subscriber_name,
			location_id, location_address, city, region, pop_name,
			gateway_device, state, archived_at, archive_reason,
			created_at, updated_at
		FROM subnets
		WHERE %s
		ORDER BY network_address
	`, where)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subnets []types.Subnet
	for rows.Next() {
		var subnet types.Subnet
		if err := rows.Scan(
			&subnet.ID,
			&subnet.PilotSubnetID,
			&subnet.NetworkAddress,
			&subnet.NetworkSize,
			&subnet.GatewayAddress,
			&subnet.FirstUsableAddress,
			&subnet.LastUsableAddress,
			&subnet.VLANID,
			&subnet.ServiceID,
			&subnet.ServiceStatus,
			&subnet.SubscriberID,
			&subnet.SubscriberName,
			&subnet.LocationID,
			&subnet.LocationAddress,
			&subnet.City,
			&subnet.Region,
			&subnet.POPName,
			&subnet.GatewayDevice,
			&subnet.State,
			&subnet.ArchivedAt,
			&subnet.ArchiveReason,
			&subnet.CreatedAt,
			&subnet.UpdatedAt,
		); err != nil {
			return nil, err
		}
		subnets = append(subnets, subnet)
	}
	return subnets, rows.Err()
}

// SubnetListParams contains parameters for paginated subnet listing.
type SubnetListParams struct {
	Limit           int
	Offset          int
	POPName         string
	City            string
	Region          string
	SubscriberID    *int
	ServiceStatus   string // Filter by service status (e.g., "cancelled", "active")
	Search          string // Searches network address, subscriber name, location
	IncludeArchived bool
}

// SubnetListResult contains paginated subnet results.
type SubnetListResult struct {
	Subnets    []types.Subnet `json:"subnets"`
	TotalCount int            `json:"total_count"`
	Limit      int            `json:"limit"`
	Offset     int            `json:"offset"`
}

// ListSubnetsPaginated returns subnets with pagination and filtering.
func (s *Store) ListSubnetsPaginated(ctx context.Context, params SubnetListParams) (*SubnetListResult, error) {
	// Build WHERE clause
	conditions := []string{}
	args := []interface{}{}
	argNum := 1

	if !params.IncludeArchived {
		conditions = append(conditions, "state = 'active'")
	}
	if params.POPName != "" {
		conditions = append(conditions, fmt.Sprintf("pop_name = $%d", argNum))
		args = append(args, params.POPName)
		argNum++
	}
	if params.City != "" {
		conditions = append(conditions, fmt.Sprintf("city = $%d", argNum))
		args = append(args, params.City)
		argNum++
	}
	if params.Region != "" {
		conditions = append(conditions, fmt.Sprintf("region = $%d", argNum))
		args = append(args, params.Region)
		argNum++
	}
	if params.SubscriberID != nil {
		conditions = append(conditions, fmt.Sprintf("subscriber_id = $%d", argNum))
		args = append(args, *params.SubscriberID)
		argNum++
	}
	if params.ServiceStatus != "" {
		conditions = append(conditions, fmt.Sprintf("service_status = $%d", argNum))
		args = append(args, params.ServiceStatus)
		argNum++
	}
	if params.Search != "" {
		conditions = append(conditions, fmt.Sprintf(
			"(network_address::text ILIKE $%d OR subscriber_name ILIKE $%d OR location_address ILIKE $%d OR city ILIKE $%d)",
			argNum, argNum, argNum, argNum,
		))
		args = append(args, "%"+params.Search+"%")
		argNum++
	}

	whereClause := "1=1"
	if len(conditions) > 0 {
		whereClause = strings.Join(conditions, " AND ")
	}

	// Get total count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM subnets WHERE %s", whereClause)
	var totalCount int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("counting subnets: %w", err)
	}

	// Set defaults
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 500 {
		params.Limit = 500
	}

	// Build main query
	query := fmt.Sprintf(`
		SELECT
			id, pilot_subnet_id, network_address::text, network_size,
			host(gateway_address), host(first_usable_address), host(last_usable_address),
			vlan_id, service_id, service_status, subscriber_id, subscriber_name,
			location_id, location_address, city, region, pop_name,
			gateway_device, state, archived_at, archive_reason,
			created_at, updated_at
		FROM subnets
		WHERE %s
		ORDER BY network_address
		LIMIT $%d OFFSET $%d
	`, whereClause, argNum, argNum+1)

	args = append(args, params.Limit, params.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subnets []types.Subnet
	for rows.Next() {
		var subnet types.Subnet
		if err := rows.Scan(
			&subnet.ID,
			&subnet.PilotSubnetID,
			&subnet.NetworkAddress,
			&subnet.NetworkSize,
			&subnet.GatewayAddress,
			&subnet.FirstUsableAddress,
			&subnet.LastUsableAddress,
			&subnet.VLANID,
			&subnet.ServiceID,
			&subnet.ServiceStatus,
			&subnet.SubscriberID,
			&subnet.SubscriberName,
			&subnet.LocationID,
			&subnet.LocationAddress,
			&subnet.City,
			&subnet.Region,
			&subnet.POPName,
			&subnet.GatewayDevice,
			&subnet.State,
			&subnet.ArchivedAt,
			&subnet.ArchiveReason,
			&subnet.CreatedAt,
			&subnet.UpdatedAt,
		); err != nil {
			return nil, err
		}
		subnets = append(subnets, subnet)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &SubnetListResult{
		Subnets:    subnets,
		TotalCount: totalCount,
		Limit:      params.Limit,
		Offset:     params.Offset,
	}, nil
}

// UpdateSubnet updates a subnet's metadata.
func (s *Store) UpdateSubnet(ctx context.Context, subnet *types.Subnet) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE subnets SET
			pilot_subnet_id = $2,
			network_address = $3,
			network_size = $4,
			gateway_address = $5,
			first_usable_address = $6,
			last_usable_address = $7,
			vlan_id = $8,
			service_id = $9,
			service_status = $10,
			subscriber_id = $11,
			subscriber_name = $12,
			location_id = $13,
			location_address = $14,
			city = $15,
			region = $16,
			pop_name = $17,
			gateway_device = $18,
			updated_at = NOW()
		WHERE id = $1
	`,
		subnet.ID,
		subnet.PilotSubnetID,
		subnet.NetworkAddress,
		subnet.NetworkSize,
		subnet.GatewayAddress,
		subnet.FirstUsableAddress,
		subnet.LastUsableAddress,
		subnet.VLANID,
		subnet.ServiceID,
		subnet.ServiceStatus,
		subnet.SubscriberID,
		subnet.SubscriberName,
		subnet.LocationID,
		subnet.LocationAddress,
		subnet.City,
		subnet.Region,
		subnet.POPName,
		subnet.GatewayDevice,
	)
	return err
}

// ArchiveSubnet marks a subnet as archived and optionally archives its auto-owned targets.
func (s *Store) ArchiveSubnet(ctx context.Context, id string, reason string, archiveAutoTargets bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Get subnet info for logging
	var networkAddress string
	var networkSize int
	var pilotSubnetID *int
	_ = tx.QueryRow(ctx, `
		SELECT network_address::text, network_size, pilot_subnet_id FROM subnets WHERE id = $1
	`, id).Scan(&networkAddress, &networkSize, &pilotSubnetID)

	// Archive the subnet
	_, err = tx.Exec(ctx, `
		UPDATE subnets SET
			state = 'archived',
			archived_at = NOW(),
			archive_reason = $2,
			updated_at = NOW()
		WHERE id = $1
	`, id, reason)
	if err != nil {
		return err
	}

	archivedTargets := 0
	if archiveAutoTargets {
		// Archive auto-owned targets in this subnet
		result, err := tx.Exec(ctx, `
			UPDATE targets SET
				archived_at = NOW(),
				archive_reason = 'subnet_archived',
				updated_at = NOW()
			WHERE subnet_id = $1
			  AND ownership = 'auto'
			  AND archived_at IS NULL
		`, id)
		if err != nil {
			return err
		}
		archivedTargets = int(result.RowsAffected())

		// Orphan manual targets (set subnet_id to NULL but don't archive)
		_, err = tx.Exec(ctx, `
			UPDATE targets SET
				subnet_id = NULL,
				updated_at = NOW()
			WHERE subnet_id = $1
			  AND ownership = 'manual'
		`, id)
		if err != nil {
			return err
		}
	}

	// Log to activity log
	triggeredBy := "api"
	if reason == "removed_from_pilot" {
		triggeredBy = "sync"
	}
	detailsJSON, _ := json.Marshal(map[string]interface{}{
		"reason":           reason,
		"network_address":  networkAddress,
		"network_size":     networkSize,
		"pilot_subnet_id":  pilotSubnetID,
		"archived_targets": archivedTargets,
	})
	_, _ = tx.Exec(ctx, `
		INSERT INTO activity_log (
			subnet_id, category, event_type, details, triggered_by, severity
		) VALUES ($1, 'subnet', 'archived', $2, $3, 'warning')
	`, id, detailsJSON, triggeredBy)

	return tx.Commit(ctx)
}

// GetSubnetTargetCounts returns counts of targets by monitoring state for a subnet.
func (s *Store) GetSubnetTargetCounts(ctx context.Context, subnetID string) (map[string]int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT monitoring_state, COUNT(*)
		FROM targets
		WHERE subnet_id = $1 AND archived_at IS NULL
		GROUP BY monitoring_state
	`, subnetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, err
		}
		counts[state] = count
	}
	return counts, rows.Err()
}

// SubnetHasActiveCoverage checks if a subnet has any active customer targets.
func (s *Store) SubnetHasActiveCoverage(ctx context.Context, subnetID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM targets
			WHERE subnet_id = $1
			  AND archived_at IS NULL
			  AND monitoring_state = 'active'
			  AND ip_type = 'customer'
		)
	`, subnetID).Scan(&exists)
	return exists, err
}

// =============================================================================
// TARGETS BY SUBNET
// =============================================================================

// ListTargetsBySubnet returns all active targets in a subnet.
func (s *Store) ListTargetsBySubnet(ctx context.Context, subnetID string) ([]types.Target, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id, host(ip_address), tier, subscriber_id, tags, display_name, notes,
			subnet_id, ownership, origin, ip_type,
			monitoring_state, state_changed_at, needs_review, discovery_attempts, last_response_at,
			first_response_at, baseline_established_at,
			archived_at, archive_reason, expected_outcome, created_at, updated_at, is_representative
		FROM targets
		WHERE subnet_id = $1 AND archived_at IS NULL
		ORDER BY ip_address
	`, subnetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanTargets(rows)
}

// ListTargetsNeedingReview returns targets that need human review (EXCLUDED state).
func (s *Store) ListTargetsNeedingReview(ctx context.Context) ([]types.TargetEnriched, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			t.id, host(t.ip_address), t.tier, t.subscriber_id, t.tags, t.display_name, t.notes,
			t.subnet_id, t.ownership, t.origin, t.ip_type,
			t.monitoring_state, t.state_changed_at, t.needs_review, t.discovery_attempts, t.last_response_at,
			t.first_response_at, t.baseline_established_at,
			t.archived_at, t.archive_reason, t.expected_outcome, t.created_at, t.updated_at, t.is_representative,
			s.network_address::text, s.network_size, s.pilot_subnet_id,
			s.service_id, s.subscriber_id, s.subscriber_name,
			s.location_id, s.location_address, s.city, s.region, s.pop_name,
			s.gateway_device, host(s.gateway_address)
		FROM targets t
		LEFT JOIN subnets s ON t.subnet_id = s.id
		WHERE t.needs_review = true AND t.archived_at IS NULL
		ORDER BY t.state_changed_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanEnrichedTargets(rows)
}

// Helper to scan target rows into []types.Target
func (s *Store) scanTargets(rows pgx.Rows) ([]types.Target, error) {
	var targets []types.Target
	for rows.Next() {
		var target types.Target
		var tagsJSON, expectedJSON []byte
		var subscriberID, subnetID, displayName, notes, archiveReason *string
		var origin, ipType *string
		var ownership, monitoringState string

		if err := rows.Scan(
			&target.ID, &target.IP, &target.Tier, &subscriberID, &tagsJSON, &displayName, &notes,
			&subnetID, &ownership, &origin, &ipType,
			&monitoringState, &target.StateChangedAt, &target.NeedsReview, &target.DiscoveryAttempts, &target.LastResponseAt,
			&target.FirstResponseAt, &target.BaselineEstablishedAt,
			&target.ArchivedAt, &archiveReason, &expectedJSON, &target.CreatedAt, &target.UpdatedAt,
			&target.IsRepresentative,
		); err != nil {
			return nil, err
		}

		if subscriberID != nil {
			target.SubscriberID = *subscriberID
		}
		if subnetID != nil {
			target.SubnetID = subnetID
		}
		if displayName != nil {
			target.DisplayName = *displayName
		}
		if notes != nil {
			target.Notes = *notes
		}
		if archiveReason != nil {
			target.ArchiveReason = *archiveReason
		}
		if origin != nil {
			target.Origin = types.OriginType(*origin)
		}
		if ipType != nil {
			target.IPType = types.IPType(*ipType)
		}
		target.Ownership = types.OwnershipType(ownership)
		target.MonitoringState = types.MonitoringState(monitoringState)

		json.Unmarshal(tagsJSON, &target.Tags)
		json.Unmarshal(expectedJSON, &target.ExpectedOutcome)

		targets = append(targets, target)
	}
	return targets, rows.Err()
}

// Helper to scan enriched target rows
func (s *Store) scanEnrichedTargets(rows pgx.Rows) ([]types.TargetEnriched, error) {
	var targets []types.TargetEnriched
	for rows.Next() {
		var target types.TargetEnriched
		var tagsJSON, expectedJSON []byte
		var subscriberID, subnetID, displayName, notes, archiveReason *string
		var origin, ipType *string
		var ownership, monitoringState string

		if err := rows.Scan(
			&target.ID, &target.IP, &target.Tier, &subscriberID, &tagsJSON, &displayName, &notes,
			&subnetID, &ownership, &origin, &ipType,
			&monitoringState, &target.StateChangedAt, &target.NeedsReview, &target.DiscoveryAttempts, &target.LastResponseAt,
			&target.FirstResponseAt, &target.BaselineEstablishedAt,
			&target.ArchivedAt, &archiveReason, &expectedJSON, &target.CreatedAt, &target.UpdatedAt,
			&target.IsRepresentative,
			// Subnet fields
			&target.NetworkAddress, &target.NetworkSize, &target.PilotSubnetID,
			&target.ServiceID, &target.SubnetSubscriberID, &target.SubscriberName,
			&target.LocationID, &target.LocationAddress, &target.City, &target.Region, &target.POPName,
			&target.GatewayDevice, &target.GatewayAddress,
		); err != nil {
			return nil, err
		}

		if subscriberID != nil {
			target.SubscriberID = *subscriberID
		}
		if subnetID != nil {
			target.SubnetID = subnetID
		}
		if displayName != nil {
			target.DisplayName = *displayName
		}
		if notes != nil {
			target.Notes = *notes
		}
		if archiveReason != nil {
			target.ArchiveReason = *archiveReason
		}
		if origin != nil {
			target.Origin = types.OriginType(*origin)
		}
		if ipType != nil {
			target.IPType = types.IPType(*ipType)
		}
		target.Ownership = types.OwnershipType(ownership)
		target.MonitoringState = types.MonitoringState(monitoringState)

		json.Unmarshal(tagsJSON, &target.Tags)
		json.Unmarshal(expectedJSON, &target.ExpectedOutcome)

		targets = append(targets, target)
	}
	return targets, rows.Err()
}

// =============================================================================
// TARGET STATE TRANSITIONS
// =============================================================================

// TransitionTargetState changes a target's monitoring state with history logging.
func (s *Store) TransitionTargetState(ctx context.Context, targetID string, newState types.MonitoringState, reason, triggeredBy string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Get current state and IP for logging
	var oldState string
	var ip string
	var subnetID *string
	err = tx.QueryRow(ctx, `
		SELECT monitoring_state, host(ip_address), subnet_id FROM targets WHERE id = $1
	`, targetID).Scan(&oldState, &ip, &subnetID)
	if err != nil {
		return err
	}

	// Skip if no change
	if oldState == string(newState) {
		return nil
	}

	// Update target
	needsReview := newState == types.StateExcluded
	_, err = tx.Exec(ctx, `
		UPDATE targets SET
			monitoring_state = $2,
			state_changed_at = NOW(),
			needs_review = $3,
			updated_at = NOW()
		WHERE id = $1
	`, targetID, newState, needsReview)
	if err != nil {
		return err
	}

	// Record history
	_, err = tx.Exec(ctx, `
		INSERT INTO target_state_history (target_id, from_state, to_state, reason, triggered_by)
		VALUES ($1, $2, $3, $4, $5)
	`, targetID, oldState, newState, reason, triggeredBy)
	if err != nil {
		return err
	}

	// Determine severity based on state transition
	severity := "info"
	if newState == types.StateExcluded {
		severity = "warning"
	} else if newState == types.StateActive && oldState == string(types.StateExcluded) {
		severity = "info" // Recovery
	}

	// Log to activity log
	detailsJSON, _ := json.Marshal(map[string]interface{}{
		"from_state": oldState,
		"to_state":   string(newState),
		"reason":     reason,
	})
	var subnetIDVal interface{}
	if subnetID != nil {
		subnetIDVal = *subnetID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO activity_log (
			target_id, subnet_id, ip, category, event_type, details, triggered_by, severity
		) VALUES ($1, $2, $3, 'target', 'state_change', $4, $5, $6)
	`, targetID, subnetIDVal, ip, detailsJSON, triggeredBy, severity)
	if err != nil {
		// Log error but don't fail the transition
		// Activity logging is secondary to the actual state change
	}

	return tx.Commit(ctx)
}

// GetTargetStateHistory returns recent state transitions for a target.
func (s *Store) GetTargetStateHistory(ctx context.Context, targetID string, limit int) ([]types.TargetStateTransition, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, target_id, from_state, to_state, reason, triggered_by, created_at
		FROM target_state_history
		WHERE target_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, targetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []types.TargetStateTransition
	for rows.Next() {
		var h types.TargetStateTransition
		var fromState *string
		if err := rows.Scan(&h.ID, &h.TargetID, &fromState, &h.ToState, &h.Reason, &h.TriggeredBy, &h.CreatedAt); err != nil {
			return nil, err
		}
		if fromState != nil {
			h.FromState = types.MonitoringState(*fromState)
		}
		history = append(history, h)
	}
	return history, rows.Err()
}

// UpdateTargetLastResponse updates the last_response_at timestamp.
// Also sets first_response_at on the first successful response.
func (s *Store) UpdateTargetLastResponse(ctx context.Context, targetID string, responseTime time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE targets SET
			last_response_at = $2,
			first_response_at = COALESCE(first_response_at, $2),
			updated_at = NOW()
		WHERE id = $1
	`, targetID, responseTime)
	return err
}

// GetTargetsForBaselineCheck returns ACTIVE targets that have been responding
// for at least the threshold duration but don't have a baseline yet.
func (s *Store) GetTargetsForBaselineCheck(ctx context.Context, threshold time.Duration) ([]types.Target, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id, host(ip_address), tier, subscriber_id, tags, display_name, notes,
			subnet_id, ownership, origin, ip_type,
			monitoring_state, state_changed_at, needs_review, discovery_attempts, last_response_at,
			first_response_at, baseline_established_at,
			archived_at, archive_reason, expected_outcome, created_at, updated_at, is_representative
		FROM targets
		WHERE monitoring_state = 'active'
		  AND archived_at IS NULL
		  AND first_response_at IS NOT NULL
		  AND baseline_established_at IS NULL
		  AND first_response_at < NOW() - $1::interval
		ORDER BY first_response_at ASC
	`, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanTargets(rows)
}

// SetTargetBaseline marks a target as having an established baseline.
func (s *Store) SetTargetBaseline(ctx context.Context, targetID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE targets SET
			baseline_established_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
	`, targetID)
	return err
}

// IncrementDiscoveryAttempts increments the discovery attempt counter.
func (s *Store) IncrementDiscoveryAttempts(ctx context.Context, targetID string) (int, error) {
	var attempts int
	err := s.pool.QueryRow(ctx, `
		UPDATE targets SET
			discovery_attempts = discovery_attempts + 1,
			updated_at = NOW()
		WHERE id = $1
		RETURNING discovery_attempts
	`, targetID).Scan(&attempts)
	return attempts, err
}

// AcknowledgeTarget clears the needs_review flag and optionally transitions to INACTIVE.
func (s *Store) AcknowledgeTarget(ctx context.Context, targetID string, markInactive bool, triggeredBy string) error {
	if markInactive {
		return s.TransitionTargetState(ctx, targetID, types.StateInactive, "user acknowledged", triggeredBy)
	}

	// Just clear the review flag
	_, err := s.pool.Exec(ctx, `
		UPDATE targets SET
			needs_review = false,
			updated_at = NOW()
		WHERE id = $1
	`, targetID)
	return err
}

// =============================================================================
// STATE WORKER SUPPORT
// =============================================================================

// GetTargetsForDownTransition returns ACTIVE targets that haven't responded
// within the given threshold (should transition to DOWN).
// Only targets with an established baseline can transition to DOWN (alertable).
// Targets without a baseline go to UNRESPONSIVE instead.
func (s *Store) GetTargetsForDownTransition(ctx context.Context, threshold time.Duration) ([]types.Target, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id, host(ip_address), tier, subscriber_id, tags, display_name, notes,
			subnet_id, ownership, origin, ip_type,
			monitoring_state, state_changed_at, needs_review, discovery_attempts, last_response_at,
			first_response_at, baseline_established_at,
			archived_at, archive_reason, expected_outcome, created_at, updated_at, is_representative
		FROM targets
		WHERE monitoring_state = 'active'
		  AND archived_at IS NULL
		  AND last_response_at < NOW() - $1::interval
		  AND baseline_established_at IS NOT NULL  -- Only targets with baseline can be DOWN
		ORDER BY last_response_at ASC
	`, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanTargets(rows)
}

// GetTargetsForUnresponsiveTransition returns ACTIVE targets WITHOUT a baseline
// that have stopped responding. These should transition to UNRESPONSIVE (not alertable).
func (s *Store) GetTargetsForUnresponsiveTransition(ctx context.Context, threshold time.Duration) ([]types.Target, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id, host(ip_address), tier, subscriber_id, tags, display_name, notes,
			subnet_id, ownership, origin, ip_type,
			monitoring_state, state_changed_at, needs_review, discovery_attempts, last_response_at,
			first_response_at, baseline_established_at,
			archived_at, archive_reason, expected_outcome, created_at, updated_at, is_representative
		FROM targets
		WHERE monitoring_state = 'active'
		  AND archived_at IS NULL
		  AND last_response_at < NOW() - $1::interval
		  AND baseline_established_at IS NULL  -- No baseline = not alertable
		ORDER BY last_response_at ASC
	`, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanTargets(rows)
}

// GetTargetsForExcludedTransition returns DOWN targets that have been
// unresponsive for the given duration (should transition to EXCLUDED).
// Excludes infrastructure and gateway IPs which should stay DOWN.
func (s *Store) GetTargetsForExcludedTransition(ctx context.Context, threshold time.Duration) ([]types.Target, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id, host(ip_address), tier, subscriber_id, tags, display_name, notes,
			subnet_id, ownership, origin, ip_type,
			monitoring_state, state_changed_at, needs_review, discovery_attempts, last_response_at,
			first_response_at, baseline_established_at,
			archived_at, archive_reason, expected_outcome, created_at, updated_at, is_representative
		FROM targets
		WHERE monitoring_state = 'down'
		  AND archived_at IS NULL
		  AND state_changed_at < NOW() - $1::interval
		  AND (ip_type IS NULL OR ip_type = 'customer')  -- Infrastructure/gateway IPs stay down
		ORDER BY state_changed_at ASC
	`, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanTargets(rows)
}

// GetTargetsForSmartRecheck returns EXCLUDED or UNRESPONSIVE targets in
// subnets that have no active customer coverage. These should be re-probed.
func (s *Store) GetTargetsForSmartRecheck(ctx context.Context) ([]types.Target, error) {
	// Find targets that are EXCLUDED or UNRESPONSIVE in subnets without active coverage
	// We use a subquery to check if the subnet has any active customer targets
	rows, err := s.pool.Query(ctx, `
		SELECT
			t.id, host(t.ip_address), t.tier, t.subscriber_id, t.tags, t.display_name, t.notes,
			t.subnet_id, t.ownership, t.origin, t.ip_type,
			t.monitoring_state, t.state_changed_at, t.needs_review, t.discovery_attempts, t.last_response_at,
			t.first_response_at, t.baseline_established_at,
			t.archived_at, t.archive_reason, t.expected_outcome, t.created_at, t.updated_at
		FROM targets t
		WHERE t.monitoring_state IN ('excluded', 'unresponsive')
		  AND t.archived_at IS NULL
		  AND t.subnet_id IS NOT NULL
		  AND t.tier != 'smart_recheck'  -- Don't re-queue if already in smart_recheck tier
		  AND NOT EXISTS (
			  SELECT 1 FROM targets active
			  WHERE active.subnet_id = t.subnet_id
			    AND active.archived_at IS NULL
			    AND active.monitoring_state = 'active'
			    AND active.ip_type = 'customer'
		  )
		ORDER BY t.state_changed_at ASC
		LIMIT 1000  -- Batch limit to avoid overwhelming the system
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanTargets(rows)
}

// SetTargetTier changes a target's monitoring tier.
func (s *Store) SetTargetTier(ctx context.Context, targetID, tier string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE targets SET
			tier = $2,
			updated_at = NOW()
		WHERE id = $1
	`, targetID, tier)
	return err
}

// UpdateTarget updates a target's metadata fields.
func (s *Store) UpdateTarget(ctx context.Context, target *types.Target) error {
	tagsJSON, err := json.Marshal(target.Tags)
	if err != nil {
		tagsJSON = []byte("{}")
	}

	var expectedOutcomeJSON []byte
	if target.ExpectedOutcome != nil {
		expectedOutcomeJSON, _ = json.Marshal(target.ExpectedOutcome)
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE targets SET
			tier = $2,
			tags = $3,
			display_name = $4,
			notes = $5,
			expected_outcome = $6,
			updated_at = NOW()
		WHERE id = $1 AND archived_at IS NULL
	`,
		target.ID,
		target.Tier,
		tagsJSON,
		target.DisplayName,
		target.Notes,
		expectedOutcomeJSON,
	)
	return err
}

// UpdateTargetTagsBySubnet updates tags on all active targets in a subnet.
// Used when subnet metadata changes to keep target tags in sync.
func (s *Store) UpdateTargetTagsBySubnet(ctx context.Context, subnetID string, tags map[string]string) error {
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE targets SET
			tags = $2,
			updated_at = NOW()
		WHERE subnet_id = $1 AND archived_at IS NULL
	`, subnetID, tagsJSON)
	return err
}

// GetDistinctTargetTagKeys returns all unique tag keys from active targets.
func (s *Store) GetDistinctTargetTagKeys(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT jsonb_object_keys(tags) as key
		FROM targets
		WHERE archived_at IS NULL
		  AND tags IS NOT NULL
		  AND jsonb_typeof(tags) = 'object'
		  AND tags != '{}'::jsonb
		ORDER BY key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// ArchiveTarget soft-deletes a target with a reason.
func (s *Store) ArchiveTarget(ctx context.Context, targetID string, reason string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Get target info for activity log
	var ip string
	var subnetID *string
	err = tx.QueryRow(ctx, `
		SELECT host(ip_address), subnet_id FROM targets WHERE id = $1
	`, targetID).Scan(&ip, &subnetID)
	if err != nil {
		return err
	}

	// Archive the target
	_, err = tx.Exec(ctx, `
		UPDATE targets SET
			archived_at = NOW(),
			archive_reason = $2,
			updated_at = NOW()
		WHERE id = $1 AND archived_at IS NULL
	`, targetID, reason)
	if err != nil {
		return err
	}

	// Log the activity
	_, err = tx.Exec(ctx, `
		INSERT INTO activity_log (
			target_id, ip, category, event_type, details, triggered_by, severity
		) VALUES (
			$1, $2::inet, 'target', 'archived', $3, 'api', 'info'
		)
	`, targetID, ip, fmt.Sprintf(`{"reason": "%s"}`, reason))
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// =============================================================================
// SERVICE STATUS MANAGEMENT
// =============================================================================

// UpdateSubnetServiceStatus updates only the service_status field for a subnet.
// Returns true if the status changed, false otherwise.
func (s *Store) UpdateSubnetServiceStatus(ctx context.Context, subnetID string, newStatus *string) (bool, error) {
	result, err := s.pool.Exec(ctx, `
		UPDATE subnets SET
			service_status = $2,
			service_status_changed_at = CASE
				WHEN service_status IS DISTINCT FROM $2 THEN NOW()
				ELSE service_status_changed_at
			END,
			updated_at = NOW()
		WHERE id = $1
		  AND (service_status IS DISTINCT FROM $2)
	`, subnetID, newStatus)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() > 0, nil
}

// ListCancelledServiceSubnets returns all subnets with cancelled service status.
func (s *Store) ListCancelledServiceSubnets(ctx context.Context) ([]types.Subnet, error) {
	return s.listSubnetsWithFilter(ctx, "service_status = 'cancelled' AND state = 'active'", nil)
}

// GetSubnetsWithServiceStatusChange returns subnets where the service_status
// changed recently (for logging/alerting purposes).
func (s *Store) GetSubnetsWithServiceStatusChange(ctx context.Context, since time.Duration) ([]types.Subnet, error) {
	return s.listSubnetsWithFilter(ctx, "service_status_changed_at > NOW() - $1::interval", []interface{}{since})
}

// TransitionTargetsByServiceCancellation moves all active targets in a subnet
// to INACTIVE state when the service is cancelled. Returns the count of affected targets.
func (s *Store) TransitionTargetsByServiceCancellation(ctx context.Context, subnetID string) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// Get all active targets in this subnet
	rows, err := tx.Query(ctx, `
		SELECT id, monitoring_state
		FROM targets
		WHERE subnet_id = $1
		  AND archived_at IS NULL
		  AND monitoring_state NOT IN ('inactive', 'excluded')
	`, subnetID)
	if err != nil {
		return 0, err
	}

	var targetIDs []string
	var oldStates []string
	for rows.Next() {
		var id, state string
		if err := rows.Scan(&id, &state); err != nil {
			rows.Close()
			return 0, err
		}
		targetIDs = append(targetIDs, id)
		oldStates = append(oldStates, state)
	}
	rows.Close()

	if len(targetIDs) == 0 {
		return 0, nil
	}

	// Transition all targets to INACTIVE
	result, err := tx.Exec(ctx, `
		UPDATE targets SET
			monitoring_state = 'inactive',
			state_changed_at = NOW(),
			updated_at = NOW()
		WHERE subnet_id = $1
		  AND archived_at IS NULL
		  AND monitoring_state NOT IN ('inactive', 'excluded')
	`, subnetID)
	if err != nil {
		return 0, err
	}

	// Record state transitions in history
	for i, targetID := range targetIDs {
		_, err = tx.Exec(ctx, `
			INSERT INTO target_state_history (target_id, from_state, to_state, reason, triggered_by)
			VALUES ($1, $2, 'inactive', 'service_cancelled', 'sync')
		`, targetID, oldStates[i])
		if err != nil {
			return 0, err
		}
	}

	// Log activity
	detailsJSON, _ := json.Marshal(map[string]interface{}{
		"targets_affected": len(targetIDs),
		"reason":           "service_cancelled",
	})
	_, _ = tx.Exec(ctx, `
		INSERT INTO activity_log (
			subnet_id, category, event_type, details, triggered_by, severity
		) VALUES ($1, 'subnet', 'service_cancelled', $2, 'sync', 'warning')
	`, subnetID, detailsJSON)

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	return int(result.RowsAffected()), nil
}

// =============================================================================
// REPRESENTATIVE MONITORING
// =============================================================================

// GetSubnetRepresentative returns the current representative target for a subnet.
// Returns nil if no representative exists (subnet has no baseline-established customer IPs).
func (s *Store) GetSubnetRepresentative(ctx context.Context, subnetID string) (*types.Target, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id, host(ip_address), tier, subscriber_id, tags, display_name, notes,
			subnet_id, ownership, origin, ip_type,
			monitoring_state, state_changed_at, needs_review, discovery_attempts, last_response_at,
			first_response_at, baseline_established_at,
			archived_at, archive_reason, expected_outcome, created_at, updated_at, is_representative
		FROM targets
		WHERE subnet_id = $1
		  AND is_representative = true
		  AND ip_type = 'customer'
		  AND archived_at IS NULL
		LIMIT 1
	`, subnetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets, err := s.scanTargets(rows)
	if err != nil {
		return nil, err
	}

	if len(targets) == 0 {
		return nil, nil
	}
	return &targets[0], nil
}

// ElectRepresentative sets a target as the representative for its subnet.
// Also transitions the target to ACTIVE state if not already.
func (s *Store) ElectRepresentative(ctx context.Context, subnetID, targetID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Clear any existing representative for this subnet
	_, err = tx.Exec(ctx, `
		UPDATE targets
		SET is_representative = false, updated_at = NOW()
		WHERE subnet_id = $1 AND is_representative = true
	`, subnetID)
	if err != nil {
		return err
	}

	// Set the new representative
	_, err = tx.Exec(ctx, `
		UPDATE targets
		SET is_representative = true,
		    monitoring_state = 'active',
		    state_changed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`, targetID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetStandbyTargetsForSubnet returns all standby targets for a subnet, ordered by baseline age.
// These are potential failover candidates.
func (s *Store) GetStandbyTargetsForSubnet(ctx context.Context, subnetID string) ([]types.Target, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id, host(ip_address), tier, subscriber_id, tags, display_name, notes,
			subnet_id, ownership, origin, ip_type,
			monitoring_state, state_changed_at, needs_review, discovery_attempts, last_response_at,
			first_response_at, baseline_established_at,
			archived_at, archive_reason, expected_outcome, created_at, updated_at, is_representative
		FROM targets
		WHERE subnet_id = $1
		  AND monitoring_state = 'standby'
		  AND archived_at IS NULL
		ORDER BY baseline_established_at ASC, ip_address ASC
	`, subnetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanTargets(rows)
}

// PromoteStandbyToRepresentative promotes the oldest standby target to representative.
// Returns the promoted target, or nil if no standby targets exist.
func (s *Store) PromoteStandbyToRepresentative(ctx context.Context, subnetID string) (*types.Target, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Find the oldest standby target
	var targetID string
	err = tx.QueryRow(ctx, `
		SELECT id FROM targets
		WHERE subnet_id = $1
		  AND monitoring_state = 'standby'
		  AND archived_at IS NULL
		ORDER BY baseline_established_at ASC, ip_address ASC
		LIMIT 1
	`, subnetID).Scan(&targetID)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil // No standby targets available
		}
		return nil, err
	}

	// Promote to representative
	_, err = tx.Exec(ctx, `
		UPDATE targets
		SET is_representative = true,
		    monitoring_state = 'active',
		    state_changed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`, targetID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Fetch and return the promoted target
	return s.GetTarget(ctx, targetID)
}

// TransitionTargetToStandby moves a target to STANDBY state.
// Used when a second customer IP establishes baseline in a subnet that already has a representative.
func (s *Store) TransitionTargetToStandby(ctx context.Context, targetID, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE targets
		SET monitoring_state = 'standby',
		    state_changed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`, targetID)
	return err
}
