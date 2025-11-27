// Package worker provides background workers for the control plane.
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// AlertStore defines the storage interface for the alert worker.
type AlertStore interface {
	// Anomaly detection
	GetCurrentAnomalies(ctx context.Context, lookback time.Duration) ([]types.Anomaly, error)
	GetHealthyTargetsWithActiveAlerts(ctx context.Context, requiredHealthyProbes int) ([]string, error)

	// Alert CRUD
	CreateAlert(ctx context.Context, alert *types.Alert) error
	GetAlert(ctx context.Context, id string) (*types.Alert, error)
	ListAlerts(ctx context.Context, filter types.AlertFilter) ([]types.Alert, error)
	FindActiveAlertForTarget(ctx context.Context, targetID string, alertType types.AlertType, agentID string) (*types.Alert, error)
	GetActiveAlertsForTarget(ctx context.Context, targetID string) ([]types.Alert, error)

	// Alert evolution
	EscalateAlert(ctx context.Context, alertID string, newSeverity types.AlertSeverity, latencyMs, packetLoss *float64, description string) error
	DeescalateAlert(ctx context.Context, alertID string, newSeverity types.AlertSeverity, latencyMs, packetLoss *float64, description string) error
	ResolveAlert(ctx context.Context, alertID string, description string) error
	ReopenAlert(ctx context.Context, alertID string, newSeverity types.AlertSeverity, latencyMs, packetLoss *float64, description string) error
	UpdateAlertMetrics(ctx context.Context, alertID string, latencyMs, packetLoss *float64) error

	// Incident correlation
	LinkAlertToIncident(ctx context.Context, alertID, incidentID string) error
	GetUnlinkedAlertsByCorrelation(ctx context.Context, correlationKey string, window time.Duration) ([]types.Alert, error)

	// Configuration
	GetAlertConfigInt(ctx context.Context, key string, defaultVal int) (int, error)
	GetAlertConfigFloat(ctx context.Context, key string, defaultVal float64) (float64, error)
}

// IncidentStore defines incident operations needed by the alert worker.
// Note: Incidents are stored in the store package, so we use interface methods
// that don't expose the Incident type directly.
type IncidentStore interface {
	// FindActiveIncidentIDByCorrelation returns the ID of an active incident matching the correlation key, or empty string.
	FindActiveIncidentIDByCorrelation(ctx context.Context, correlationKey string) (string, error)
	// CreateIncidentFromAlerts creates a new incident from a set of correlated alerts.
	// Returns the incident ID.
	CreateIncidentFromAlerts(ctx context.Context, correlationKey string, alertIDs []string, severity string) (string, error)
}

// AlertWorkerConfig holds configuration for the alert worker.
type AlertWorkerConfig struct {
	// Interval between alert processing runs.
	Interval time.Duration

	// AnomalyLookback is how far back to look for anomalies in agent_target_state.
	AnomalyLookback time.Duration

	// CorrelationWindow is how long to look back when correlating alerts to incidents.
	CorrelationWindow time.Duration

	// ResolutionProbeCount is how many consecutive healthy probes before resolving.
	ResolutionProbeCount int

	// IncidentCreationThreshold is min correlated alerts before creating an incident.
	IncidentCreationThreshold int

	// Severity thresholds (defaults, can be overridden from DB config)
	LatencyWarningMs     float64
	LatencyCriticalMs    float64
	PacketLossWarningPct float64
	PacketLossCriticalPct float64
}

// DefaultAlertWorkerConfig returns sensible defaults.
func DefaultAlertWorkerConfig() AlertWorkerConfig {
	return AlertWorkerConfig{
		Interval:                  30 * time.Second,
		AnomalyLookback:           5 * time.Minute,
		CorrelationWindow:         5 * time.Minute,
		ResolutionProbeCount:      3,
		IncidentCreationThreshold: 2,
		LatencyWarningMs:          100,
		LatencyCriticalMs:         500,
		PacketLossWarningPct:      5,
		PacketLossCriticalPct:     20,
	}
}

// AlertWorker processes anomalies into evolving alerts and correlates them to incidents.
type AlertWorker struct {
	alertStore    AlertStore
	incidentStore IncidentStore
	config        AlertWorkerConfig
	logger        *slog.Logger
	stopCh        chan struct{}
}

// NewAlertWorker creates a new alert worker.
func NewAlertWorker(alertStore AlertStore, incidentStore IncidentStore, config AlertWorkerConfig, logger *slog.Logger) *AlertWorker {
	return &AlertWorker{
		alertStore:    alertStore,
		incidentStore: incidentStore,
		config:        config,
		logger:        logger.With("component", "alert_worker"),
		stopCh:        make(chan struct{}),
	}
}

// Start begins the alert worker in a goroutine.
func (w *AlertWorker) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop signals the worker to stop.
func (w *AlertWorker) Stop() {
	close(w.stopCh)
}

func (w *AlertWorker) run(ctx context.Context) {
	w.logger.Info("alert worker started",
		"interval", w.config.Interval,
		"anomaly_lookback", w.config.AnomalyLookback,
		"correlation_window", w.config.CorrelationWindow,
	)

	// Load config from DB on startup
	w.refreshConfig(ctx)

	// Run immediately on start
	w.runOnce(ctx)

	ticker := time.NewTicker(w.config.Interval)
	configTicker := time.NewTicker(5 * time.Minute) // Refresh config periodically
	defer ticker.Stop()
	defer configTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("alert worker stopping (context cancelled)")
			return
		case <-w.stopCh:
			w.logger.Info("alert worker stopping (stop signal)")
			return
		case <-configTicker.C:
			w.refreshConfig(ctx)
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

// refreshConfig loads threshold configuration from the database.
func (w *AlertWorker) refreshConfig(ctx context.Context) {
	if val, err := w.alertStore.GetAlertConfigFloat(ctx, "escalation_latency_warning_ms", w.config.LatencyWarningMs); err == nil {
		w.config.LatencyWarningMs = val
	}
	if val, err := w.alertStore.GetAlertConfigFloat(ctx, "escalation_latency_critical_ms", w.config.LatencyCriticalMs); err == nil {
		w.config.LatencyCriticalMs = val
	}
	if val, err := w.alertStore.GetAlertConfigFloat(ctx, "escalation_packet_loss_warning_pct", w.config.PacketLossWarningPct); err == nil {
		w.config.PacketLossWarningPct = val
	}
	if val, err := w.alertStore.GetAlertConfigFloat(ctx, "escalation_packet_loss_critical_pct", w.config.PacketLossCriticalPct); err == nil {
		w.config.PacketLossCriticalPct = val
	}
	if val, err := w.alertStore.GetAlertConfigInt(ctx, "resolution_probe_count", w.config.ResolutionProbeCount); err == nil {
		w.config.ResolutionProbeCount = val
	}
	if val, err := w.alertStore.GetAlertConfigInt(ctx, "incident_creation_threshold", w.config.IncidentCreationThreshold); err == nil {
		w.config.IncidentCreationThreshold = val
	}
}

func (w *AlertWorker) runOnce(ctx context.Context) {
	start := time.Now()

	// Phase 1: Process anomalies into alerts (create new or evolve existing)
	created, evolved := w.processAnomalies(ctx)

	// Phase 2: Check for alerts that should be resolved
	resolved := w.checkResolutions(ctx)

	// Phase 3: Correlate unlinked alerts to incidents
	linked, incidentsCreated := w.correlateToIncidents(ctx)

	w.logger.Info("alert worker cycle complete",
		"duration", time.Since(start),
		"alerts_created", created,
		"alerts_evolved", evolved,
		"alerts_resolved", resolved,
		"alerts_linked", linked,
		"incidents_created", incidentsCreated,
	)
}

// processAnomalies converts detected anomalies into alerts.
func (w *AlertWorker) processAnomalies(ctx context.Context) (created, evolved int) {
	anomalies, err := w.alertStore.GetCurrentAnomalies(ctx, w.config.AnomalyLookback)
	if err != nil {
		w.logger.Error("failed to get current anomalies", "error", err)
		return 0, 0
	}

	for _, anomaly := range anomalies {
		c, e := w.processAnomaly(ctx, anomaly)
		created += c
		evolved += e
	}

	return created, evolved
}

// processAnomaly handles a single anomaly - either creates a new alert or evolves an existing one.
func (w *AlertWorker) processAnomaly(ctx context.Context, anomaly types.Anomaly) (created, evolved int) {
	alertType := w.anomalyToAlertType(anomaly.AnomalyType)
	severity := w.calculateSeverity(anomaly)

	// Check if there's an existing active alert for this target+type (ignoring agent_id for target-level alerts)
	existing, err := w.alertStore.FindActiveAlertForTarget(ctx, anomaly.TargetID, alertType, "")
	if err != nil {
		w.logger.Error("failed to find active alert",
			"target_id", anomaly.TargetID,
			"error", err,
		)
		return 0, 0
	}

	latency := &anomaly.LatencyMs
	packetLoss := &anomaly.PacketLoss

	if existing != nil {
		// Evolve the existing alert
		return 0, w.evolveAlert(ctx, existing, severity, latency, packetLoss)
	}

	// Create a new alert (target-level, not per-agent)
	alert := &types.Alert{
		ID:              uuid.New().String(),
		TargetID:        anomaly.TargetID,
		TargetIP:        anomaly.TargetIP,
		AgentID:         "", // Empty for target-level alerts
		AlertType:       alertType,
		Severity:        severity,
		Status:          types.AlertStatusActive,
		InitialSeverity: severity,
		PeakSeverity:    severity,
		InitialLatencyMs:   latency,
		InitialPacketLoss:  packetLoss,
		PeakLatencyMs:      latency,
		PeakPacketLoss:     packetLoss,
		CurrentLatencyMs:   latency,
		CurrentPacketLoss:  packetLoss,
		Title:           w.generateTitle(alertType, anomaly.TargetIP, severity),
		Message:         w.generateMessage(alertType, anomaly),
		DetectedAt:      time.Now(),
		LastUpdatedAt:   time.Now(),
		CorrelationKey:  w.generateCorrelationKey(anomaly),
	}

	if err := w.alertStore.CreateAlert(ctx, alert); err != nil {
		w.logger.Error("failed to create alert",
			"target_id", anomaly.TargetID,
			"error", err,
		)
		return 0, 0
	}

	w.logger.Info("created new alert",
		"alert_id", alert.ID,
		"target_id", anomaly.TargetID,
		"target_ip", anomaly.TargetIP,
		"type", alertType,
		"severity", severity,
	)

	return 1, 0
}

// evolveAlert updates an existing alert based on new anomaly data.
func (w *AlertWorker) evolveAlert(ctx context.Context, alert *types.Alert, newSeverity types.AlertSeverity, latency, packetLoss *float64) int {
	oldLevel := alert.Severity.Level()
	newLevel := newSeverity.Level()

	if newLevel > oldLevel {
		// Escalate
		desc := fmt.Sprintf("Escalated from %s to %s", alert.Severity, newSeverity)
		if err := w.alertStore.EscalateAlert(ctx, alert.ID, newSeverity, latency, packetLoss, desc); err != nil {
			w.logger.Error("failed to escalate alert", "alert_id", alert.ID, "error", err)
			return 0
		}
		w.logger.Info("alert escalated",
			"alert_id", alert.ID,
			"old_severity", alert.Severity,
			"new_severity", newSeverity,
		)
		return 1
	} else if newLevel < oldLevel {
		// De-escalate
		desc := fmt.Sprintf("De-escalated from %s to %s", alert.Severity, newSeverity)
		if err := w.alertStore.DeescalateAlert(ctx, alert.ID, newSeverity, latency, packetLoss, desc); err != nil {
			w.logger.Error("failed to de-escalate alert", "alert_id", alert.ID, "error", err)
			return 0
		}
		w.logger.Info("alert de-escalated",
			"alert_id", alert.ID,
			"old_severity", alert.Severity,
			"new_severity", newSeverity,
		)
		return 1
	} else {
		// Same severity - just update metrics
		if err := w.alertStore.UpdateAlertMetrics(ctx, alert.ID, latency, packetLoss); err != nil {
			w.logger.Error("failed to update alert metrics", "alert_id", alert.ID, "error", err)
		}
		return 0
	}
}

// checkResolutions finds targets that are now healthy and resolves their alerts.
func (w *AlertWorker) checkResolutions(ctx context.Context) int {
	healthyTargets, err := w.alertStore.GetHealthyTargetsWithActiveAlerts(ctx, w.config.ResolutionProbeCount)
	if err != nil {
		w.logger.Error("failed to get healthy targets with active alerts", "error", err)
		return 0
	}

	resolved := 0
	for _, targetID := range healthyTargets {
		alerts, err := w.alertStore.GetActiveAlertsForTarget(ctx, targetID)
		if err != nil {
			w.logger.Error("failed to get active alerts for target", "target_id", targetID, "error", err)
			continue
		}

		for _, alert := range alerts {
			desc := fmt.Sprintf("Target recovered after %d consecutive healthy probes", w.config.ResolutionProbeCount)
			if err := w.alertStore.ResolveAlert(ctx, alert.ID, desc); err != nil {
				w.logger.Error("failed to resolve alert", "alert_id", alert.ID, "error", err)
				continue
			}
			w.logger.Info("alert resolved",
				"alert_id", alert.ID,
				"target_id", targetID,
			)
			resolved++
		}
	}

	return resolved
}

// correlateToIncidents links unlinked alerts to incidents, creating new incidents as needed.
func (w *AlertWorker) correlateToIncidents(ctx context.Context) (linked, incidentsCreated int) {
	// Get all unlinked active alerts with correlation keys
	unlinkedAlerts, err := w.getUnlinkedAlertsWithCorrelationKeys(ctx)
	if err != nil {
		w.logger.Error("failed to get unlinked alerts", "error", err)
		return 0, 0
	}

	// Group by correlation key
	alertsByCorrelation := make(map[string][]types.Alert)
	for _, alert := range unlinkedAlerts {
		if alert.CorrelationKey != "" {
			alertsByCorrelation[alert.CorrelationKey] = append(alertsByCorrelation[alert.CorrelationKey], alert)
		}
	}

	// Process each correlation group
	for correlationKey, alerts := range alertsByCorrelation {
		// Check if there's an existing active incident for this correlation key
		existingIncidentID, err := w.incidentStore.FindActiveIncidentIDByCorrelation(ctx, correlationKey)
		if err != nil {
			w.logger.Error("failed to find active incident", "correlation_key", correlationKey, "error", err)
			continue
		}

		if existingIncidentID != "" {
			// Link alerts to existing incident
			for _, alert := range alerts {
				if err := w.alertStore.LinkAlertToIncident(ctx, alert.ID, existingIncidentID); err != nil {
					w.logger.Error("failed to link alert to incident",
						"alert_id", alert.ID,
						"incident_id", existingIncidentID,
						"error", err,
					)
					continue
				}
				linked++
				w.logger.Info("linked alert to existing incident",
					"alert_id", alert.ID,
					"incident_id", existingIncidentID,
				)
			}
		} else if len(alerts) >= w.config.IncidentCreationThreshold {
			// Create new incident if we have enough correlated alerts
			alertIDs := make([]string, len(alerts))
			maxSeverity := types.AlertSeverityInfo
			for i, alert := range alerts {
				alertIDs[i] = alert.ID
				if alert.Severity.Level() > maxSeverity.Level() {
					maxSeverity = alert.Severity
				}
			}

			// Map alert severity to incident severity
			incidentSeverity := w.alertSeverityToIncidentSeverity(maxSeverity)

			incidentID, err := w.incidentStore.CreateIncidentFromAlerts(ctx, correlationKey, alertIDs, incidentSeverity)
			if err != nil {
				w.logger.Error("failed to create incident from alerts",
					"correlation_key", correlationKey,
					"alert_count", len(alertIDs),
					"error", err,
				)
				continue
			}

			incidentsCreated++
			linked += len(alerts)
			w.logger.Info("created new incident from correlated alerts",
				"incident_id", incidentID,
				"correlation_key", correlationKey,
				"alert_count", len(alerts),
				"severity", incidentSeverity,
			)
		}
	}

	return linked, incidentsCreated
}

// getUnlinkedAlertsWithCorrelationKeys returns all active alerts that aren't linked to an incident.
func (w *AlertWorker) getUnlinkedAlertsWithCorrelationKeys(ctx context.Context) ([]types.Alert, error) {
	hasIncident := false
	status := types.AlertStatusActive
	return w.alertStore.ListAlerts(ctx, types.AlertFilter{
		Status:      &status,
		HasIncident: &hasIncident,
		Limit:       1000,
	})
}

// alertSeverityToIncidentSeverity maps alert severity to incident severity.
func (w *AlertWorker) alertSeverityToIncidentSeverity(alertSev types.AlertSeverity) string {
	switch alertSev {
	case types.AlertSeverityCritical:
		return "critical"
	case types.AlertSeverityWarning:
		return "high"
	default:
		return "medium"
	}
}

// =============================================================================
// HELPER METHODS
// =============================================================================

func (w *AlertWorker) anomalyToAlertType(anomalyType string) types.AlertType {
	switch anomalyType {
	case "availability":
		return types.AlertTypeAvailability
	case "latency":
		return types.AlertTypeLatency
	case "packet_loss":
		return types.AlertTypePacketLoss
	default:
		return types.AlertTypeAvailability
	}
}

func (w *AlertWorker) calculateSeverity(anomaly types.Anomaly) types.AlertSeverity {
	// Critical conditions
	if anomaly.AnomalyType == "availability" {
		return types.AlertSeverityCritical
	}
	if anomaly.PacketLoss >= w.config.PacketLossCriticalPct {
		return types.AlertSeverityCritical
	}
	if anomaly.LatencyMs >= w.config.LatencyCriticalMs {
		return types.AlertSeverityCritical
	}

	// Warning conditions
	if anomaly.PacketLoss >= w.config.PacketLossWarningPct {
		return types.AlertSeverityWarning
	}
	if anomaly.LatencyMs >= w.config.LatencyWarningMs {
		return types.AlertSeverityWarning
	}

	return types.AlertSeverityInfo
}

func (w *AlertWorker) generateTitle(alertType types.AlertType, targetIP string, severity types.AlertSeverity) string {
	switch alertType {
	case types.AlertTypeAvailability:
		return fmt.Sprintf("%s unreachable - %s", targetIP, severity)
	case types.AlertTypeLatency:
		return fmt.Sprintf("%s high latency - %s", targetIP, severity)
	case types.AlertTypePacketLoss:
		return fmt.Sprintf("%s packet loss - %s", targetIP, severity)
	default:
		return fmt.Sprintf("%s issue detected - %s", targetIP, severity)
	}
}

func (w *AlertWorker) generateMessage(alertType types.AlertType, anomaly types.Anomaly) string {
	switch alertType {
	case types.AlertTypeAvailability:
		return fmt.Sprintf("Target %s is not responding. %d consecutive failures.",
			anomaly.TargetIP, anomaly.ConsecutiveFailures)
	case types.AlertTypeLatency:
		return fmt.Sprintf("Target %s latency is %.1fms (z-score: %.2f).",
			anomaly.TargetIP, anomaly.LatencyMs, anomaly.ZScore)
	case types.AlertTypePacketLoss:
		return fmt.Sprintf("Target %s has %.1f%% packet loss.",
			anomaly.TargetIP, anomaly.PacketLoss)
	default:
		return fmt.Sprintf("Issue detected on %s.", anomaly.TargetIP)
	}
}

func (w *AlertWorker) generateCorrelationKey(anomaly types.Anomaly) string {
	// Prefer subnet-based correlation for blast radius tracking
	if anomaly.SubnetID != "" {
		return "subnet:" + anomaly.SubnetID
	}
	// Fall back to target-based correlation
	return "target:" + anomaly.TargetID
}
