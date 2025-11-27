package service

import (
	"context"

	"github.com/pilot-net/icmp-mon/pkg/types"
)

// =============================================================================
// ALERT OPERATIONS
// =============================================================================

// ListAlerts returns alerts matching the given filter.
func (s *Service) ListAlerts(ctx context.Context, filter types.AlertFilter) ([]types.Alert, error) {
	return s.store.ListAlerts(ctx, filter)
}

// GetAlert retrieves an alert by ID.
func (s *Service) GetAlert(ctx context.Context, id string) (*types.Alert, error) {
	return s.store.GetAlert(ctx, id)
}

// GetAlertWithEvents retrieves an alert with its full event history.
func (s *Service) GetAlertWithEvents(ctx context.Context, id string) (*types.AlertWithEvents, error) {
	return s.store.GetAlertWithEvents(ctx, id)
}

// GetAlertStats returns aggregate alert statistics.
func (s *Service) GetAlertStats(ctx context.Context) (*types.AlertStats, error) {
	return s.store.GetAlertStats(ctx)
}

// AcknowledgeAlert marks an alert as acknowledged by a user.
func (s *Service) AcknowledgeAlert(ctx context.Context, id, acknowledgedBy string) error {
	return s.store.AcknowledgeAlert(ctx, id, acknowledgedBy)
}

// ResolveAlert marks an alert as resolved.
func (s *Service) ResolveAlert(ctx context.Context, id, reason string) error {
	return s.store.ResolveAlert(ctx, id, reason)
}

// ListAlertEvents returns events for an alert.
func (s *Service) ListAlertEvents(ctx context.Context, alertID string, limit int) ([]types.AlertEvent, error) {
	return s.store.ListAlertEvents(ctx, alertID, limit)
}

// =============================================================================
// ALERT CONFIG OPERATIONS
// =============================================================================

// ListAlertConfigs returns all alert configuration values.
func (s *Service) ListAlertConfigs(ctx context.Context) ([]types.AlertConfig, error) {
	return s.store.ListAlertConfigs(ctx)
}

// SetAlertConfig sets a configuration value.
func (s *Service) SetAlertConfig(ctx context.Context, key string, value interface{}, description string) error {
	return s.store.SetAlertConfig(ctx, key, value, description)
}

// GetAlertCorrelations returns a summary of active alerts grouped by common dimensions.
func (s *Service) GetAlertCorrelations(ctx context.Context) (*types.AlertCorrelationSummary, error) {
	return s.store.GetAlertCorrelations(ctx)
}
