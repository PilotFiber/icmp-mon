package api

import (
	"net/http"
	"strconv"

	"github.com/pilot-net/icmp-mon/pkg/types"
)

// =============================================================================
// ALERT ENDPOINTS
// =============================================================================

func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters into AlertFilter
	filter := types.AlertFilter{}

	if status := r.URL.Query().Get("status"); status != "" {
		st := types.AlertStatus(status)
		filter.Status = &st
	}
	if severity := r.URL.Query().Get("severity"); severity != "" {
		sev := types.AlertSeverity(severity)
		filter.Severity = &sev
	}
	if alertType := r.URL.Query().Get("type"); alertType != "" {
		at := types.AlertType(alertType)
		filter.AlertType = &at
	}
	if targetID := r.URL.Query().Get("target_id"); targetID != "" {
		filter.TargetID = &targetID
	}
	if incidentID := r.URL.Query().Get("incident_id"); incidentID != "" {
		filter.IncidentID = &incidentID
	}
	if hasIncident := r.URL.Query().Get("has_incident"); hasIncident != "" {
		hi := hasIncident == "true"
		filter.HasIncident = &hi
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			filter.Limit = l
		}
	}
	if offset := r.URL.Query().Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil {
			filter.Offset = o
		}
	}

	alerts, err := s.svc.ListAlerts(ctx, filter)
	if err != nil {
		s.logger.Error("list alerts failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list alerts")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"alerts": alerts,
		"count":  len(alerts),
	})
}

func (s *Server) handleGetAlert(w http.ResponseWriter, r *http.Request) {
	alertID := r.PathValue("id")

	alert, err := s.svc.GetAlertWithEvents(r.Context(), alertID)
	if err != nil {
		s.logger.Error("get alert failed", "alert_id", alertID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get alert")
		return
	}
	if alert == nil {
		s.writeError(w, http.StatusNotFound, "alert not found")
		return
	}

	s.writeJSON(w, http.StatusOK, alert)
}

func (s *Server) handleGetAlertStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.svc.GetAlertStats(r.Context())
	if err != nil {
		s.logger.Error("get alert stats failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get alert stats")
		return
	}

	s.writeJSON(w, http.StatusOK, stats)
}

type acknowledgeAlertRequest struct {
	AcknowledgedBy string `json:"acknowledged_by"`
}

func (s *Server) handleAcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	alertID := r.PathValue("id")

	var req acknowledgeAlertRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AcknowledgedBy == "" {
		req.AcknowledgedBy = "api_user"
	}

	if err := s.svc.AcknowledgeAlert(r.Context(), alertID, req.AcknowledgedBy); err != nil {
		s.logger.Error("acknowledge alert failed", "alert_id", alertID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to acknowledge alert")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "acknowledged",
		"message": "Alert acknowledged successfully",
	})
}

type resolveAlertRequest struct {
	Reason string `json:"reason,omitempty"`
}

func (s *Server) handleResolveAlert(w http.ResponseWriter, r *http.Request) {
	alertID := r.PathValue("id")

	var req resolveAlertRequest
	if err := s.readJSON(r, &req); err != nil {
		// Allow empty body
		req.Reason = "Manually resolved via API"
	}
	if req.Reason == "" {
		req.Reason = "Manually resolved via API"
	}

	if err := s.svc.ResolveAlert(r.Context(), alertID, req.Reason); err != nil {
		s.logger.Error("resolve alert failed", "alert_id", alertID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to resolve alert")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "resolved",
		"message": "Alert resolved successfully",
	})
}

func (s *Server) handleGetAlertEvents(w http.ResponseWriter, r *http.Request) {
	alertID := r.PathValue("id")

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	events, err := s.svc.ListAlertEvents(r.Context(), alertID, limit)
	if err != nil {
		s.logger.Error("get alert events failed", "alert_id", alertID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get alert events")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"count":  len(events),
	})
}

// =============================================================================
// ALERT CONFIG ENDPOINTS
// =============================================================================

func (s *Server) handleListAlertConfigs(w http.ResponseWriter, r *http.Request) {
	configs, err := s.svc.ListAlertConfigs(r.Context())
	if err != nil {
		s.logger.Error("list alert configs failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list alert configs")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"configs": configs,
		"count":   len(configs),
	})
}

type updateAlertConfigRequest struct {
	Value       any    `json:"value"`
	Description string `json:"description,omitempty"`
}

func (s *Server) handleUpdateAlertConfig(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	var req updateAlertConfigRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.svc.SetAlertConfig(r.Context(), key, req.Value, req.Description); err != nil {
		s.logger.Error("update alert config failed", "key", key, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to update alert config")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "updated",
		"message": "Config updated successfully",
	})
}

// =============================================================================
// ALERT CORRELATIONS
// =============================================================================

func (s *Server) handleGetAlertCorrelations(w http.ResponseWriter, r *http.Request) {
	correlations, err := s.svc.GetAlertCorrelations(r.Context())
	if err != nil {
		s.logger.Error("get alert correlations failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get alert correlations")
		return
	}

	s.writeJSON(w, http.StatusOK, correlations)
}
