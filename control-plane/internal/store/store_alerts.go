package store

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// =============================================================================
// ALERTS - EVOLVING ALERTS WITH EVENT HISTORY
// =============================================================================

// CreateAlert inserts a new alert and its initial "created" event.
// It also looks up subnet metadata for the target IP and stores it for historical accuracy.
func (s *Store) CreateAlert(ctx context.Context, alert *types.Alert) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Convert optional fields to nullable
	var agentID, incidentID, correlationKey interface{}
	if alert.AgentID != "" {
		agentID = alert.AgentID
	}
	if alert.IncidentID != nil && *alert.IncidentID != "" {
		incidentID = *alert.IncidentID
	}
	if alert.CorrelationKey != "" {
		correlationKey = alert.CorrelationKey
	}

	// Lookup subnet metadata for the target IP (stored at creation time for historical accuracy)
	var subnetID, subscriberName, locationAddress, city, region, popName, gatewayDevice interface{}
	var serviceID, locationID interface{}
	err = tx.QueryRow(ctx, `
		SELECT id, subscriber_name, service_id, location_id, location_address, city, region, pop_name, gateway_device
		FROM subnets
		WHERE $1::inet << network_address::inet AND state = 'active'
		LIMIT 1
	`, alert.TargetIP).Scan(&subnetID, &subscriberName, &serviceID, &locationID, &locationAddress, &city, &region, &popName, &gatewayDevice)
	if err != nil && err != pgx.ErrNoRows {
		// Log error but don't fail alert creation - metadata is nice-to-have
		// Continue with nil values
	}

	// Insert the alert with subnet metadata
	_, err = tx.Exec(ctx, `
		INSERT INTO alerts (
			id, target_id, target_ip, agent_id,
			alert_type, severity, status,
			initial_severity, peak_severity,
			initial_latency_ms, initial_packet_loss,
			peak_latency_ms, peak_packet_loss,
			current_latency_ms, current_packet_loss,
			title, message,
			detected_at, last_updated_at,
			incident_id, correlation_key,
			subnet_id, subscriber_name, service_id, location_id, location_address, city, region, pop_name, gateway_device
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9,
			$10, $11,
			$12, $13,
			$14, $15,
			$16, $17,
			$18, $19,
			$20, $21,
			$22, $23, $24, $25, $26, $27, $28, $29, $30
		)
	`,
		alert.ID, alert.TargetID, alert.TargetIP, agentID,
		alert.AlertType, alert.Severity, alert.Status,
		alert.InitialSeverity, alert.PeakSeverity,
		alert.InitialLatencyMs, alert.InitialPacketLoss,
		alert.PeakLatencyMs, alert.PeakPacketLoss,
		alert.CurrentLatencyMs, alert.CurrentPacketLoss,
		alert.Title, alert.Message,
		alert.DetectedAt, alert.LastUpdatedAt,
		incidentID, correlationKey,
		subnetID, subscriberName, serviceID, locationID, locationAddress, city, region, popName, gatewayDevice,
	)
	if err != nil {
		return fmt.Errorf("insert alert: %w", err)
	}

	// Insert the "created" event
	_, err = tx.Exec(ctx, `
		INSERT INTO alert_events (
			alert_id, event_type, new_severity, new_status,
			latency_ms, packet_loss,
			description, triggered_by
		) VALUES (
			$1, 'created', $2, $3,
			$4, $5,
			$6, $7
		)
	`,
		alert.ID, alert.Severity, alert.Status,
		alert.CurrentLatencyMs, alert.CurrentPacketLoss,
		fmt.Sprintf("Alert created: %s", alert.Title),
		"alert_worker",
	)
	if err != nil {
		return fmt.Errorf("insert created event: %w", err)
	}

	return tx.Commit(ctx)
}

// GetAlert retrieves an alert by ID with optional target/agent name joins.
func (s *Store) GetAlert(ctx context.Context, id string) (*types.Alert, error) {
	var alert types.Alert
	var agentID, incidentID, correlationKey, acknowledgedBy *string
	var acknowledgedAt, resolvedAt *time.Time
	var targetName, agentName *string

	err := s.pool.QueryRow(ctx, `
		SELECT
			a.id, a.target_id, host(a.target_ip), a.agent_id,
			a.alert_type, a.severity, a.status,
			a.initial_severity, a.peak_severity,
			a.initial_latency_ms, a.initial_packet_loss,
			a.peak_latency_ms, a.peak_packet_loss,
			a.current_latency_ms, a.current_packet_loss,
			a.title, a.message,
			a.detected_at, a.last_updated_at,
			a.acknowledged_at, a.acknowledged_by,
			a.resolved_at,
			a.incident_id, a.correlation_key,
			a.created_at,
			t.ip_address::text as target_name,
			ag.name as agent_name
		FROM alerts a
		LEFT JOIN targets t ON a.target_id = t.id
		LEFT JOIN agents ag ON a.agent_id = ag.id
		WHERE a.id = $1
	`, id).Scan(
		&alert.ID, &alert.TargetID, &alert.TargetIP, &agentID,
		&alert.AlertType, &alert.Severity, &alert.Status,
		&alert.InitialSeverity, &alert.PeakSeverity,
		&alert.InitialLatencyMs, &alert.InitialPacketLoss,
		&alert.PeakLatencyMs, &alert.PeakPacketLoss,
		&alert.CurrentLatencyMs, &alert.CurrentPacketLoss,
		&alert.Title, &alert.Message,
		&alert.DetectedAt, &alert.LastUpdatedAt,
		&acknowledgedAt, &acknowledgedBy,
		&resolvedAt,
		&incidentID, &correlationKey,
		&alert.CreatedAt,
		&targetName, &agentName,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if agentID != nil {
		alert.AgentID = *agentID
	}
	if incidentID != nil {
		alert.IncidentID = incidentID
	}
	if correlationKey != nil {
		alert.CorrelationKey = *correlationKey
	}
	if acknowledgedAt != nil {
		alert.AcknowledgedAt = acknowledgedAt
	}
	if acknowledgedBy != nil {
		alert.AcknowledgedBy = *acknowledgedBy
	}
	if resolvedAt != nil {
		alert.ResolvedAt = resolvedAt
	}
	if targetName != nil {
		alert.TargetName = *targetName
	}
	if agentName != nil {
		alert.AgentName = *agentName
	}

	return &alert, nil
}

// GetAlertWithEvents retrieves an alert with its full event history.
func (s *Store) GetAlertWithEvents(ctx context.Context, id string) (*types.AlertWithEvents, error) {
	alert, err := s.GetAlert(ctx, id)
	if err != nil || alert == nil {
		return nil, err
	}

	events, err := s.ListAlertEvents(ctx, id, 1000)
	if err != nil {
		return nil, err
	}

	return &types.AlertWithEvents{
		Alert:  *alert,
		Events: events,
	}, nil
}

// ListAlerts returns alerts matching the given filter.
func (s *Store) ListAlerts(ctx context.Context, filter types.AlertFilter) ([]types.Alert, error) {
	where := "1=1"
	args := []interface{}{}
	argNum := 1

	if filter.Status != nil {
		where += fmt.Sprintf(" AND a.status = $%d", argNum)
		args = append(args, *filter.Status)
		argNum++
	}
	if filter.Severity != nil {
		where += fmt.Sprintf(" AND a.severity = $%d", argNum)
		args = append(args, *filter.Severity)
		argNum++
	}
	if filter.AlertType != nil {
		where += fmt.Sprintf(" AND a.alert_type = $%d", argNum)
		args = append(args, *filter.AlertType)
		argNum++
	}
	if filter.TargetID != nil {
		where += fmt.Sprintf(" AND a.target_id = $%d", argNum)
		args = append(args, *filter.TargetID)
		argNum++
	}
	if filter.IncidentID != nil {
		where += fmt.Sprintf(" AND a.incident_id = $%d", argNum)
		args = append(args, *filter.IncidentID)
		argNum++
	}
	if filter.HasIncident != nil {
		if *filter.HasIncident {
			where += " AND a.incident_id IS NOT NULL"
		} else {
			where += " AND a.incident_id IS NULL"
		}
	}
	if filter.Since != nil {
		where += fmt.Sprintf(" AND a.detected_at >= $%d", argNum)
		args = append(args, *filter.Since)
		argNum++
	}

	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	query := fmt.Sprintf(`
		SELECT
			a.id, a.target_id, host(a.target_ip), a.agent_id,
			a.alert_type, a.severity, a.status,
			a.initial_severity, a.peak_severity,
			a.initial_latency_ms, a.initial_packet_loss,
			a.peak_latency_ms, a.peak_packet_loss,
			a.current_latency_ms, a.current_packet_loss,
			a.title, a.message,
			a.detected_at, a.last_updated_at,
			a.acknowledged_at, a.acknowledged_by,
			a.resolved_at,
			a.incident_id, a.correlation_key,
			a.created_at,
			t.ip_address::text as target_name,
			ag.name as agent_name,
			a.subnet_id,
			a.subscriber_name,
			a.service_id,
			a.location_id,
			a.location_address,
			a.city,
			a.region,
			a.pop_name,
			a.gateway_device
		FROM alerts a
		LEFT JOIN targets t ON a.target_id = t.id
		LEFT JOIN agents ag ON a.agent_id = ag.id
		WHERE %s
		ORDER BY a.detected_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argNum, argNum+1)
	args = append(args, limit, filter.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []types.Alert
	for rows.Next() {
		var alert types.Alert
		var agentID, incidentID, correlationKey, acknowledgedBy *string
		var acknowledgedAt, resolvedAt *time.Time
		var targetName, agentName *string
		var message *string
		// Subnet metadata fields (stored on alert, nullable)
		var subnetID, subscriberName, locationAddress, city, region, popName, gatewayDevice *string
		var serviceID, locationID *int

		if err := rows.Scan(
			&alert.ID, &alert.TargetID, &alert.TargetIP, &agentID,
			&alert.AlertType, &alert.Severity, &alert.Status,
			&alert.InitialSeverity, &alert.PeakSeverity,
			&alert.InitialLatencyMs, &alert.InitialPacketLoss,
			&alert.PeakLatencyMs, &alert.PeakPacketLoss,
			&alert.CurrentLatencyMs, &alert.CurrentPacketLoss,
			&alert.Title, &message,
			&alert.DetectedAt, &alert.LastUpdatedAt,
			&acknowledgedAt, &acknowledgedBy,
			&resolvedAt,
			&incidentID, &correlationKey,
			&alert.CreatedAt,
			&targetName, &agentName,
			&subnetID, &subscriberName, &serviceID, &locationID, &locationAddress, &city, &region, &popName, &gatewayDevice,
		); err != nil {
			return nil, err
		}

		if agentID != nil {
			alert.AgentID = *agentID
		}
		if incidentID != nil {
			alert.IncidentID = incidentID
		}
		if correlationKey != nil {
			alert.CorrelationKey = *correlationKey
		}
		if acknowledgedAt != nil {
			alert.AcknowledgedAt = acknowledgedAt
		}
		if acknowledgedBy != nil {
			alert.AcknowledgedBy = *acknowledgedBy
		}
		if resolvedAt != nil {
			alert.ResolvedAt = resolvedAt
		}
		if targetName != nil {
			alert.TargetName = *targetName
		}
		if agentName != nil {
			alert.AgentName = *agentName
		}
		if message != nil {
			alert.Message = *message
		}
		// Subnet metadata
		if subnetID != nil {
			alert.SubnetID = *subnetID
		}
		if subscriberName != nil {
			alert.SubscriberName = *subscriberName
		}
		if serviceID != nil {
			alert.ServiceID = *serviceID
		}
		if locationID != nil {
			alert.LocationID = *locationID
		}
		if locationAddress != nil {
			alert.LocationAddress = *locationAddress
		}
		if city != nil {
			alert.City = *city
		}
		if region != nil {
			alert.Region = *region
		}
		if popName != nil {
			alert.PopName = *popName
		}
		if gatewayDevice != nil {
			alert.GatewayDevice = *gatewayDevice
		}

		alerts = append(alerts, alert)
	}

	return alerts, rows.Err()
}

// FindActiveAlertForTarget finds an existing active/acknowledged alert for a target+type combination.
// Used to determine if we should evolve an existing alert or create a new one.
func (s *Store) FindActiveAlertForTarget(ctx context.Context, targetID string, alertType types.AlertType, agentID string) (*types.Alert, error) {
	// For per-agent alerts, match on agent_id too
	// For consensus alerts (empty agentID), just match target+type
	where := "target_id = $1 AND alert_type = $2 AND status IN ('active', 'acknowledged')"
	args := []interface{}{targetID, alertType}

	if agentID != "" {
		where += " AND agent_id = $3"
		args = append(args, agentID)
	}

	var alert types.Alert
	var aID, incidentID, correlationKey, acknowledgedBy *string
	var acknowledgedAt, resolvedAt *time.Time

	err := s.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT
			id, target_id, host(target_ip), agent_id,
			alert_type, severity, status,
			initial_severity, peak_severity,
			initial_latency_ms, initial_packet_loss,
			peak_latency_ms, peak_packet_loss,
			current_latency_ms, current_packet_loss,
			title, message,
			detected_at, last_updated_at,
			acknowledged_at, acknowledged_by,
			resolved_at,
			incident_id, correlation_key,
			created_at
		FROM alerts
		WHERE %s
		ORDER BY detected_at DESC
		LIMIT 1
	`, where), args...).Scan(
		&alert.ID, &alert.TargetID, &alert.TargetIP, &aID,
		&alert.AlertType, &alert.Severity, &alert.Status,
		&alert.InitialSeverity, &alert.PeakSeverity,
		&alert.InitialLatencyMs, &alert.InitialPacketLoss,
		&alert.PeakLatencyMs, &alert.PeakPacketLoss,
		&alert.CurrentLatencyMs, &alert.CurrentPacketLoss,
		&alert.Title, &alert.Message,
		&alert.DetectedAt, &alert.LastUpdatedAt,
		&acknowledgedAt, &acknowledgedBy,
		&resolvedAt,
		&incidentID, &correlationKey,
		&alert.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if aID != nil {
		alert.AgentID = *aID
	}
	if incidentID != nil {
		alert.IncidentID = incidentID
	}
	if correlationKey != nil {
		alert.CorrelationKey = *correlationKey
	}
	if acknowledgedAt != nil {
		alert.AcknowledgedAt = acknowledgedAt
	}
	if acknowledgedBy != nil {
		alert.AcknowledgedBy = *acknowledgedBy
	}
	if resolvedAt != nil {
		alert.ResolvedAt = resolvedAt
	}

	return &alert, nil
}

// =============================================================================
// ALERT EVOLUTION - ESCALATION, DE-ESCALATION, RESOLUTION
// =============================================================================

// EscalateAlert increases severity and records the event.
func (s *Store) EscalateAlert(ctx context.Context, alertID string, newSeverity types.AlertSeverity, latencyMs, packetLoss *float64, description string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Get current alert state
	var oldSeverity types.AlertSeverity
	var peakSeverity types.AlertSeverity
	err = tx.QueryRow(ctx, `SELECT severity, peak_severity FROM alerts WHERE id = $1`, alertID).Scan(&oldSeverity, &peakSeverity)
	if err != nil {
		return err
	}

	// Update peak severity if new is higher
	updatePeak := newSeverity.Level() > peakSeverity.Level()
	peakLatency := latencyMs
	peakPacketLoss := packetLoss

	_, err = tx.Exec(ctx, `
		UPDATE alerts SET
			severity = $2,
			current_latency_ms = COALESCE($3, current_latency_ms),
			current_packet_loss = COALESCE($4, current_packet_loss),
			peak_severity = CASE WHEN $5 THEN $2 ELSE peak_severity END,
			peak_latency_ms = CASE WHEN $5 AND $3 IS NOT NULL THEN $3 ELSE peak_latency_ms END,
			peak_packet_loss = CASE WHEN $5 AND $4 IS NOT NULL THEN $4 ELSE peak_packet_loss END,
			last_updated_at = NOW()
		WHERE id = $1
	`, alertID, newSeverity, latencyMs, packetLoss, updatePeak)
	if err != nil {
		return err
	}

	// Record the escalation event
	detailsJSON, _ := json.Marshal(map[string]interface{}{
		"update_peak":      updatePeak,
		"peak_latency_ms":  peakLatency,
		"peak_packet_loss": peakPacketLoss,
	})
	_, err = tx.Exec(ctx, `
		INSERT INTO alert_events (
			alert_id, event_type,
			old_severity, new_severity,
			latency_ms, packet_loss,
			description, details, triggered_by
		) VALUES ($1, 'escalated', $2, $3, $4, $5, $6, $7, 'alert_worker')
	`, alertID, oldSeverity, newSeverity, latencyMs, packetLoss, description, detailsJSON)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// DeescalateAlert decreases severity and records the event.
func (s *Store) DeescalateAlert(ctx context.Context, alertID string, newSeverity types.AlertSeverity, latencyMs, packetLoss *float64, description string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var oldSeverity types.AlertSeverity
	err = tx.QueryRow(ctx, `SELECT severity FROM alerts WHERE id = $1`, alertID).Scan(&oldSeverity)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE alerts SET
			severity = $2,
			current_latency_ms = COALESCE($3, current_latency_ms),
			current_packet_loss = COALESCE($4, current_packet_loss),
			last_updated_at = NOW()
		WHERE id = $1
	`, alertID, newSeverity, latencyMs, packetLoss)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO alert_events (
			alert_id, event_type,
			old_severity, new_severity,
			latency_ms, packet_loss,
			description, triggered_by
		) VALUES ($1, 'de_escalated', $2, $3, $4, $5, $6, 'alert_worker')
	`, alertID, oldSeverity, newSeverity, latencyMs, packetLoss, description)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ResolveAlert marks an alert as resolved and records the event.
func (s *Store) ResolveAlert(ctx context.Context, alertID string, description string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var oldStatus types.AlertStatus
	err = tx.QueryRow(ctx, `SELECT status FROM alerts WHERE id = $1`, alertID).Scan(&oldStatus)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE alerts SET
			status = 'resolved',
			resolved_at = NOW(),
			last_updated_at = NOW()
		WHERE id = $1
	`, alertID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO alert_events (
			alert_id, event_type,
			old_status, new_status,
			description, triggered_by
		) VALUES ($1, 'resolved', $2, 'resolved', $3, 'alert_worker')
	`, alertID, oldStatus, description)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ReopenAlert reopens a previously resolved alert.
func (s *Store) ReopenAlert(ctx context.Context, alertID string, newSeverity types.AlertSeverity, latencyMs, packetLoss *float64, description string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE alerts SET
			status = 'active',
			severity = $2,
			current_latency_ms = COALESCE($3, current_latency_ms),
			current_packet_loss = COALESCE($4, current_packet_loss),
			resolved_at = NULL,
			last_updated_at = NOW()
		WHERE id = $1
	`, alertID, newSeverity, latencyMs, packetLoss)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO alert_events (
			alert_id, event_type,
			old_status, new_status, new_severity,
			latency_ms, packet_loss,
			description, triggered_by
		) VALUES ($1, 'reopened', 'resolved', 'active', $2, $3, $4, $5, 'alert_worker')
	`, alertID, newSeverity, latencyMs, packetLoss, description)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// AcknowledgeAlert marks an alert as acknowledged by a user.
func (s *Store) AcknowledgeAlert(ctx context.Context, alertID, acknowledgedBy string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var oldStatus types.AlertStatus
	err = tx.QueryRow(ctx, `SELECT status FROM alerts WHERE id = $1`, alertID).Scan(&oldStatus)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE alerts SET
			status = 'acknowledged',
			acknowledged_at = NOW(),
			acknowledged_by = $2,
			last_updated_at = NOW()
		WHERE id = $1
	`, alertID, acknowledgedBy)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO alert_events (
			alert_id, event_type,
			old_status, new_status,
			description, triggered_by
		) VALUES ($1, 'acknowledged', $2, 'acknowledged', $3, $4)
	`, alertID, oldStatus, fmt.Sprintf("Acknowledged by %s", acknowledgedBy), "user:"+acknowledgedBy)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// UpdateAlertMetrics updates current metrics without changing severity.
func (s *Store) UpdateAlertMetrics(ctx context.Context, alertID string, latencyMs, packetLoss *float64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE alerts SET
			current_latency_ms = COALESCE($2, current_latency_ms),
			current_packet_loss = COALESCE($3, current_packet_loss),
			last_updated_at = NOW()
		WHERE id = $1
	`, alertID, latencyMs, packetLoss)
	return err
}

// =============================================================================
// ALERT EVENTS
// =============================================================================

// ListAlertEvents returns events for an alert ordered by time (newest first).
func (s *Store) ListAlertEvents(ctx context.Context, alertID string, limit int) ([]types.AlertEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			id, alert_id, event_type,
			old_severity, new_severity,
			old_status, new_status,
			latency_ms, packet_loss,
			description, details, triggered_by,
			created_at
		FROM alert_events
		WHERE alert_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, alertID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []types.AlertEvent
	for rows.Next() {
		var event types.AlertEvent
		var oldSev, newSev *string
		var oldStatus, newStatus *string
		var detailsJSON []byte
		var description *string

		if err := rows.Scan(
			&event.ID, &event.AlertID, &event.EventType,
			&oldSev, &newSev,
			&oldStatus, &newStatus,
			&event.LatencyMs, &event.PacketLoss,
			&description, &detailsJSON, &event.TriggeredBy,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}

		if oldSev != nil {
			sev := types.AlertSeverity(*oldSev)
			event.OldSeverity = &sev
		}
		if newSev != nil {
			sev := types.AlertSeverity(*newSev)
			event.NewSeverity = &sev
		}
		if oldStatus != nil {
			st := types.AlertStatus(*oldStatus)
			event.OldStatus = &st
		}
		if newStatus != nil {
			st := types.AlertStatus(*newStatus)
			event.NewStatus = &st
		}
		if description != nil {
			event.Description = *description
		}
		if len(detailsJSON) > 0 {
			json.Unmarshal(detailsJSON, &event.Details)
		}

		events = append(events, event)
	}

	return events, rows.Err()
}

// =============================================================================
// INCIDENT CORRELATION
// =============================================================================

// LinkAlertToIncident links an alert to an incident and records the event.
func (s *Store) LinkAlertToIncident(ctx context.Context, alertID, incidentID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Use the DB function to handle both sides
	_, err = tx.Exec(ctx, `SELECT add_alert_to_incident($1, $2)`, alertID, incidentID)
	if err != nil {
		return err
	}

	// Record the event
	_, err = tx.Exec(ctx, `
		INSERT INTO alert_events (
			alert_id, event_type,
			description, details, triggered_by
		) VALUES ($1, 'linked_to_incident', $2, $3, 'alert_worker')
	`, alertID, fmt.Sprintf("Linked to incident %s", incidentID), fmt.Sprintf(`{"incident_id": "%s"}`, incidentID))
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetUnlinkedAlertsByCorrelation returns active alerts not yet linked to an incident.
func (s *Store) GetUnlinkedAlertsByCorrelation(ctx context.Context, correlationKey string, window time.Duration) ([]types.Alert, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, target_id, agent_id, severity, detected_at
		FROM alerts
		WHERE incident_id IS NULL
		  AND status = 'active'
		  AND correlation_key = $1
		  AND detected_at > NOW() - $2::interval
		ORDER BY detected_at
	`, correlationKey, window.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []types.Alert
	for rows.Next() {
		var alert types.Alert
		var agentID *string
		if err := rows.Scan(&alert.ID, &alert.TargetID, &agentID, &alert.Severity, &alert.DetectedAt); err != nil {
			return nil, err
		}
		if agentID != nil {
			alert.AgentID = *agentID
		}
		alerts = append(alerts, alert)
	}
	return alerts, rows.Err()
}

// =============================================================================
// ALERT STATISTICS
// =============================================================================

// GetAlertStats returns aggregate alert statistics.
func (s *Store) GetAlertStats(ctx context.Context) (*types.AlertStats, error) {
	var stats types.AlertStats
	err := s.pool.QueryRow(ctx, `
		SELECT
			active_count, critical_count, warning_count,
			acknowledged_count, resolved_today, total_this_week,
			avg_resolution_minutes
		FROM alert_stats
	`).Scan(
		&stats.ActiveCount, &stats.CriticalCount, &stats.WarningCount,
		&stats.AcknowledgedCount, &stats.ResolvedTodayCount, &stats.TotalThisWeekCount,
		&stats.AvgResolutionMinutes,
	)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

// =============================================================================
// ALERT CONFIGURATION
// =============================================================================

// GetAlertConfig retrieves a configuration value.
func (s *Store) GetAlertConfig(ctx context.Context, key string) (*types.AlertConfig, error) {
	var cfg types.AlertConfig
	var valueJSON []byte
	var description *string

	err := s.pool.QueryRow(ctx, `
		SELECT key, value, description, updated_at
		FROM alert_config WHERE key = $1
	`, key).Scan(&cfg.Key, &valueJSON, &description, &cfg.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal(valueJSON, &cfg.Value)
	if description != nil {
		cfg.Description = *description
	}
	return &cfg, nil
}

// GetAlertConfigInt retrieves a config value as int with default.
func (s *Store) GetAlertConfigInt(ctx context.Context, key string, defaultVal int) (int, error) {
	cfg, err := s.GetAlertConfig(ctx, key)
	if err != nil || cfg == nil {
		return defaultVal, err
	}
	// Handle JSON number/string conversion
	switch v := cfg.Value.(type) {
	case float64:
		return int(v), nil
	case string:
		var n int
		fmt.Sscanf(v, "%d", &n)
		return n, nil
	default:
		return defaultVal, nil
	}
}

// GetAlertConfigFloat retrieves a config value as float with default.
func (s *Store) GetAlertConfigFloat(ctx context.Context, key string, defaultVal float64) (float64, error) {
	cfg, err := s.GetAlertConfig(ctx, key)
	if err != nil || cfg == nil {
		return defaultVal, err
	}
	switch v := cfg.Value.(type) {
	case float64:
		return v, nil
	case string:
		var f float64
		fmt.Sscanf(v, "%f", &f)
		return f, nil
	default:
		return defaultVal, nil
	}
}

// SetAlertConfig sets a configuration value.
func (s *Store) SetAlertConfig(ctx context.Context, key string, value interface{}, description string) error {
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO alert_config (key, value, description, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			description = COALESCE(EXCLUDED.description, alert_config.description),
			updated_at = NOW()
	`, key, valueJSON, description)
	return err
}

// ListAlertConfigs returns all configuration values.
func (s *Store) ListAlertConfigs(ctx context.Context) ([]types.AlertConfig, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT key, value, description, updated_at
		FROM alert_config ORDER BY key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []types.AlertConfig
	for rows.Next() {
		var cfg types.AlertConfig
		var valueJSON []byte
		var description *string
		if err := rows.Scan(&cfg.Key, &valueJSON, &description, &cfg.UpdatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(valueJSON, &cfg.Value)
		if description != nil {
			cfg.Description = *description
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

// =============================================================================
// ANOMALY DETECTION
// =============================================================================

// GetCurrentAnomalies returns anomalies detected from agent_target_state.
func (s *Store) GetCurrentAnomalies(ctx context.Context, lookback time.Duration) ([]types.Anomaly, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			target_id, host(target_ip), agent_id, anomaly_type, severity,
			latency_ms, packet_loss, z_score, subnet_id, consecutive_failures
		FROM get_current_anomalies($1::interval)
	`, lookback.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var anomalies []types.Anomaly
	for rows.Next() {
		var a types.Anomaly
		var subnetID *string
		if err := rows.Scan(
			&a.TargetID, &a.TargetIP, &a.AgentID, &a.AnomalyType, &a.Severity,
			&a.LatencyMs, &a.PacketLoss, &a.ZScore, &subnetID, &a.ConsecutiveFailures,
		); err != nil {
			return nil, err
		}
		if subnetID != nil {
			a.SubnetID = *subnetID
		}
		anomalies = append(anomalies, a)
	}
	return anomalies, rows.Err()
}

// GetHealthyTargetsWithActiveAlerts finds targets that are now healthy but have active alerts.
// Used for auto-resolution. Uses the same tier-based logic as alert creation.
func (s *Store) GetHealthyTargetsWithActiveAlerts(ctx context.Context, requiredHealthyProbes int) ([]string, error) {
	// A target is considered healthy when enough agents see it as 'up' based on tier requirements
	// This mirrors the logic in get_current_anomalies() for symmetry
	rows, err := s.pool.Query(ctx, `
		WITH target_health AS (
			SELECT
				a.target_id,
				t.tier,
				COUNT(DISTINCT CASE WHEN ats.status = 'up' AND COALESCE(ats.consecutive_successes, 0) >= $1 THEN ats.agent_id END) AS healthy_agents,
				COUNT(DISTINCT ats.agent_id) AS total_agents,
				-- Minimum required healthy agents based on tier (matches get_current_anomalies)
				CASE t.tier
					WHEN 'pilot_infra' THEN COUNT(DISTINCT ats.agent_id)  -- All agents must be healthy
					WHEN 'infrastructure' THEN COUNT(DISTINCT ats.agent_id)  -- All agents must be healthy
					WHEN 'vlan_gateway' THEN LEAST(3, COUNT(DISTINCT ats.agent_id))
					ELSE LEAST(2, COUNT(DISTINCT ats.agent_id))  -- standard, vip, etc.
				END AS min_required
			FROM alerts a
			JOIN targets t ON t.id = a.target_id
			JOIN agent_target_state ats ON ats.target_id = a.target_id
			JOIN agents ag ON ag.id = ats.agent_id AND ag.archived_at IS NULL
			WHERE a.status IN ('active', 'acknowledged')
			  AND t.archived_at IS NULL
			GROUP BY a.target_id, t.tier
		)
		SELECT target_id
		FROM target_health
		WHERE healthy_agents >= min_required
	`, requiredHealthyProbes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targetIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		targetIDs = append(targetIDs, id)
	}
	return targetIDs, rows.Err()
}

// GetActiveAlertsForTarget returns all active/acknowledged alerts for a target.
func (s *Store) GetActiveAlertsForTarget(ctx context.Context, targetID string) ([]types.Alert, error) {
	status := types.AlertStatusActive
	return s.ListAlerts(ctx, types.AlertFilter{
		TargetID: &targetID,
		Status:   &status,
	})
}

// =============================================================================
// INCIDENT CORRELATION (for AlertWorker)
// =============================================================================

// FindActiveIncidentIDByCorrelation returns the ID of an active incident matching the correlation key.
// Returns empty string if no matching incident found.
func (s *Store) FindActiveIncidentIDByCorrelation(ctx context.Context, correlationKey string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM incidents
		WHERE status != 'resolved'
		  AND correlation_key = $1
		ORDER BY detected_at DESC
		LIMIT 1
	`, correlationKey).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

// CreateIncidentFromAlerts creates a new incident from a set of correlated alerts.
// Returns the new incident ID.
func (s *Store) CreateIncidentFromAlerts(ctx context.Context, correlationKey string, alertIDs []string, severity string) (string, error) {
	if len(alertIDs) == 0 {
		return "", fmt.Errorf("no alerts provided for incident creation")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	// Gather target IDs and agent IDs from alerts
	var affectedTargetIDs, affectedAgentIDs []string
	targetSet := make(map[string]bool)
	agentSet := make(map[string]bool)

	for _, alertID := range alertIDs {
		var targetID, agentID *string
		err := tx.QueryRow(ctx, `SELECT target_id, agent_id FROM alerts WHERE id = $1`, alertID).Scan(&targetID, &agentID)
		if err != nil {
			continue
		}
		if targetID != nil && !targetSet[*targetID] {
			affectedTargetIDs = append(affectedTargetIDs, *targetID)
			targetSet[*targetID] = true
		}
		if agentID != nil && !agentSet[*agentID] {
			affectedAgentIDs = append(affectedAgentIDs, *agentID)
			agentSet[*agentID] = true
		}
	}

	// Determine incident type based on correlation key
	incidentType := "target"
	if len(correlationKey) > 7 && correlationKey[:7] == "subnet:" {
		incidentType = "regional"
	}

	// Create the incident
	incidentID := fmt.Sprintf("%s", generateUUID())
	_, err = tx.Exec(ctx, `
		INSERT INTO incidents (
			id, incident_type, severity,
			affected_target_ids, affected_agent_ids,
			detected_at, status, correlation_key,
			alert_ids, alert_count, last_alert_at,
			created_at, updated_at
		) VALUES (
			$1, $2, $3,
			$4, $5,
			NOW(), 'active', $6,
			$7, $8, NOW(),
			NOW(), NOW()
		)
	`, incidentID, incidentType, severity,
		affectedTargetIDs, affectedAgentIDs,
		correlationKey,
		alertIDs, len(alertIDs),
	)
	if err != nil {
		return "", fmt.Errorf("create incident: %w", err)
	}

	// Link all alerts to the incident
	for _, alertID := range alertIDs {
		_, err = tx.Exec(ctx, `
			UPDATE alerts SET
				incident_id = $2,
				last_updated_at = NOW()
			WHERE id = $1
		`, alertID, incidentID)
		if err != nil {
			return "", fmt.Errorf("link alert %s: %w", alertID, err)
		}

		// Record the event
		_, err = tx.Exec(ctx, `
			INSERT INTO alert_events (
				alert_id, event_type,
				description, details, triggered_by
			) VALUES ($1, 'linked_to_incident', $2, $3, 'alert_worker')
		`, alertID,
			fmt.Sprintf("Linked to new incident %s", incidentID),
			fmt.Sprintf(`{"incident_id": "%s"}`, incidentID),
		)
		if err != nil {
			return "", fmt.Errorf("record link event for alert %s: %w", alertID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}

	return incidentID, nil
}

// =============================================================================
// ALERT CORRELATIONS
// =============================================================================

// GetAlertCorrelations returns a summary of active alerts grouped by common dimensions.
// This powers the "heat map" UI showing patterns like "500 alerts all have jfk00 in common".
func (s *Store) GetAlertCorrelations(ctx context.Context) (*types.AlertCorrelationSummary, error) {
	summary := &types.AlertCorrelationSummary{}

	// Get total active alert count
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM alerts WHERE status IN ('active', 'acknowledged')
	`).Scan(&summary.TotalActiveAlerts)
	if err != nil {
		return nil, fmt.Errorf("count active alerts: %w", err)
	}

	// Helper to get correlations by dimension
	getCorrelations := func(dimension, column string) ([]types.AlertCorrelation, error) {
		rows, err := s.pool.Query(ctx, fmt.Sprintf(`
			SELECT
				%s as value,
				COUNT(*) as alert_count,
				COUNT(DISTINCT target_id) as target_count,
				COUNT(DISTINCT agent_id) as agent_count,
				MAX(CASE severity
					WHEN 'critical' THEN 3
					WHEN 'warning' THEN 2
					ELSE 1
				END) as max_severity
			FROM alerts
			WHERE status IN ('active', 'acknowledged')
			  AND %s IS NOT NULL
			  AND %s != ''
			GROUP BY %s
			HAVING COUNT(*) > 1
			ORDER BY COUNT(*) DESC
			LIMIT 10
		`, column, column, column, column))
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var correlations []types.AlertCorrelation
		for rows.Next() {
			var c types.AlertCorrelation
			var maxSeverity int
			if err := rows.Scan(&c.Value, &c.AlertCount, &c.TargetCount, &c.AgentCount, &maxSeverity); err != nil {
				return nil, err
			}
			c.Key = dimension
			switch maxSeverity {
			case 3:
				c.Severity = "critical"
			case 2:
				c.Severity = "warning"
			default:
				c.Severity = "info"
			}
			correlations = append(correlations, c)
		}
		return correlations, rows.Err()
	}

	// Get correlations by each dimension
	summary.ByPop, err = getCorrelations("pop_name", "pop_name")
	if err != nil {
		return nil, fmt.Errorf("get pop correlations: %w", err)
	}

	summary.ByGateway, err = getCorrelations("gateway_device", "gateway_device")
	if err != nil {
		return nil, fmt.Errorf("get gateway correlations: %w", err)
	}

	summary.BySubscriber, err = getCorrelations("subscriber_name", "subscriber_name")
	if err != nil {
		return nil, fmt.Errorf("get subscriber correlations: %w", err)
	}

	// For location, we concatenate location_id with address for display
	rows, err := s.pool.Query(ctx, `
		SELECT
			COALESCE(location_address, location_id::text) as value,
			COUNT(*) as alert_count,
			COUNT(DISTINCT target_id) as target_count,
			COUNT(DISTINCT agent_id) as agent_count,
			MAX(CASE severity
				WHEN 'critical' THEN 3
				WHEN 'warning' THEN 2
				ELSE 1
			END) as max_severity
		FROM alerts
		WHERE status IN ('active', 'acknowledged')
		  AND location_id IS NOT NULL
		GROUP BY COALESCE(location_address, location_id::text)
		HAVING COUNT(*) > 1
		ORDER BY COUNT(*) DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, fmt.Errorf("get location correlations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var c types.AlertCorrelation
		var maxSeverity int
		if err := rows.Scan(&c.Value, &c.AlertCount, &c.TargetCount, &c.AgentCount, &maxSeverity); err != nil {
			return nil, err
		}
		c.Key = "location"
		switch maxSeverity {
		case 3:
			c.Severity = "critical"
		case 2:
			c.Severity = "warning"
		default:
			c.Severity = "info"
		}
		summary.ByLocation = append(summary.ByLocation, c)
	}

	summary.ByRegion, err = getCorrelations("region", "region")
	if err != nil {
		return nil, fmt.Errorf("get region correlations: %w", err)
	}

	return summary, nil
}

// helper for generating UUIDs (avoid import cycle)
func generateUUID() string {
	// Simple UUID v4 generation
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
