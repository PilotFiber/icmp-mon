package api

import (
	"net/http"
	"strconv"

	"github.com/pilot-net/icmp-mon/control-plane/internal/service"
	"github.com/pilot-net/icmp-mon/control-plane/internal/store"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// =============================================================================
// SUBNET ENDPOINTS
// =============================================================================

func (s *Server) handleListSubnets(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Check if pagination is requested
	limitStr := query.Get("limit")
	offsetStr := query.Get("offset")

	// If pagination params present, use paginated endpoint
	if limitStr != "" || offsetStr != "" {
		params := store.SubnetListParams{
			POPName:         query.Get("pop"),
			City:            query.Get("city"),
			Region:          query.Get("region"),
			Search:          query.Get("search"),
			IncludeArchived: query.Get("include_archived") == "true",
		}

		if limitStr != "" {
			if limit, err := strconv.Atoi(limitStr); err == nil {
				params.Limit = limit
			}
		}
		if offsetStr != "" {
			if offset, err := strconv.Atoi(offsetStr); err == nil {
				params.Offset = offset
			}
		}
		if subIDStr := query.Get("subscriber_id"); subIDStr != "" {
			if subID, err := strconv.Atoi(subIDStr); err == nil {
				params.SubscriberID = &subID
			}
		}

		result, err := s.svc.ListSubnetsPaginated(r.Context(), params)
		if err != nil {
			s.logger.Error("list subnets paginated failed", "error", err)
			s.writeError(w, http.StatusInternalServerError, "failed to list subnets")
			return
		}

		s.writeJSON(w, http.StatusOK, result)
		return
	}

	// Legacy: return all subnets
	includeArchived := query.Get("include_archived") == "true"

	var subnets []types.Subnet
	var err error
	if includeArchived {
		subnets, err = s.svc.ListSubnetsIncludeArchived(r.Context())
	} else {
		subnets, err = s.svc.ListSubnets(r.Context())
	}
	if err != nil {
		s.logger.Error("list subnets failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list subnets")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"subnets": subnets,
		"count":   len(subnets),
	})
}

type createSubnetRequest struct {
	PilotSubnetID      *int    `json:"pilot_subnet_id,omitempty"`
	NetworkAddress     string  `json:"network_address"`
	NetworkSize        int     `json:"network_size"`
	GatewayAddress     *string `json:"gateway_address,omitempty"`
	FirstUsableAddress *string `json:"first_usable_address,omitempty"`
	LastUsableAddress  *string `json:"last_usable_address,omitempty"`
	VLANID             *int    `json:"vlan_id,omitempty"`
	ServiceID          *int    `json:"service_id,omitempty"`
	SubscriberID       *int    `json:"subscriber_id,omitempty"`
	SubscriberName     *string `json:"subscriber_name,omitempty"`
	LocationID         *int    `json:"location_id,omitempty"`
	LocationAddress    *string `json:"location_address,omitempty"`
	City               *string `json:"city,omitempty"`
	Region             *string `json:"region,omitempty"`
	POPName            *string `json:"pop_name,omitempty"`
	GatewayDevice      *string `json:"gateway_device,omitempty"`
}

func (s *Server) handleCreateSubnet(w http.ResponseWriter, r *http.Request) {
	var req createSubnetRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.NetworkAddress == "" {
		s.writeError(w, http.StatusBadRequest, "network_address is required")
		return
	}
	if req.NetworkSize <= 0 || req.NetworkSize > 32 {
		s.writeError(w, http.StatusBadRequest, "network_size must be between 1 and 32")
		return
	}

	subnet, err := s.svc.CreateSubnet(r.Context(), service.CreateSubnetRequest{
		PilotSubnetID:      req.PilotSubnetID,
		NetworkAddress:     req.NetworkAddress,
		NetworkSize:        req.NetworkSize,
		GatewayAddress:     req.GatewayAddress,
		FirstUsableAddress: req.FirstUsableAddress,
		LastUsableAddress:  req.LastUsableAddress,
		VLANID:             req.VLANID,
		ServiceID:          req.ServiceID,
		SubscriberID:       req.SubscriberID,
		SubscriberName:     req.SubscriberName,
		LocationID:         req.LocationID,
		LocationAddress:    req.LocationAddress,
		City:               req.City,
		Region:             req.Region,
		POPName:            req.POPName,
		GatewayDevice:      req.GatewayDevice,
	})
	if err != nil {
		s.logger.Error("create subnet failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to create subnet")
		return
	}

	s.writeJSON(w, http.StatusCreated, subnet)
}

func (s *Server) handleGetSubnet(w http.ResponseWriter, r *http.Request) {
	subnetID := r.PathValue("id")
	if subnetID == "" {
		s.writeError(w, http.StatusBadRequest, "subnet ID required")
		return
	}

	subnet, err := s.svc.GetSubnet(r.Context(), subnetID)
	if err != nil {
		s.logger.Error("get subnet failed", "subnet", subnetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get subnet")
		return
	}
	if subnet == nil {
		s.writeError(w, http.StatusNotFound, "subnet not found")
		return
	}

	s.writeJSON(w, http.StatusOK, subnet)
}

type updateSubnetRequest struct {
	PilotSubnetID      *int    `json:"pilot_subnet_id,omitempty"`
	NetworkAddress     string  `json:"network_address"`
	NetworkSize        int     `json:"network_size"`
	GatewayAddress     *string `json:"gateway_address,omitempty"`
	FirstUsableAddress *string `json:"first_usable_address,omitempty"`
	LastUsableAddress  *string `json:"last_usable_address,omitempty"`
	VLANID             *int    `json:"vlan_id,omitempty"`
	ServiceID          *int    `json:"service_id,omitempty"`
	SubscriberID       *int    `json:"subscriber_id,omitempty"`
	SubscriberName     *string `json:"subscriber_name,omitempty"`
	LocationID         *int    `json:"location_id,omitempty"`
	LocationAddress    *string `json:"location_address,omitempty"`
	City               *string `json:"city,omitempty"`
	Region             *string `json:"region,omitempty"`
	POPName            *string `json:"pop_name,omitempty"`
	GatewayDevice      *string `json:"gateway_device,omitempty"`
}

func (s *Server) handleUpdateSubnet(w http.ResponseWriter, r *http.Request) {
	subnetID := r.PathValue("id")
	if subnetID == "" {
		s.writeError(w, http.StatusBadRequest, "subnet ID required")
		return
	}

	var req updateSubnetRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.NetworkAddress == "" {
		s.writeError(w, http.StatusBadRequest, "network_address is required")
		return
	}

	subnet, err := s.svc.UpdateSubnet(r.Context(), service.UpdateSubnetRequest{
		ID:                 subnetID,
		PilotSubnetID:      req.PilotSubnetID,
		NetworkAddress:     req.NetworkAddress,
		NetworkSize:        req.NetworkSize,
		GatewayAddress:     req.GatewayAddress,
		FirstUsableAddress: req.FirstUsableAddress,
		LastUsableAddress:  req.LastUsableAddress,
		VLANID:             req.VLANID,
		ServiceID:          req.ServiceID,
		SubscriberID:       req.SubscriberID,
		SubscriberName:     req.SubscriberName,
		LocationID:         req.LocationID,
		LocationAddress:    req.LocationAddress,
		City:               req.City,
		Region:             req.Region,
		POPName:            req.POPName,
		GatewayDevice:      req.GatewayDevice,
	})
	if err != nil {
		s.logger.Error("update subnet failed", "subnet", subnetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to update subnet")
		return
	}

	s.writeJSON(w, http.StatusOK, subnet)
}

func (s *Server) handleArchiveSubnet(w http.ResponseWriter, r *http.Request) {
	subnetID := r.PathValue("id")
	if subnetID == "" {
		s.writeError(w, http.StatusBadRequest, "subnet ID required")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := s.readJSON(r, &req); err != nil {
		req.Reason = "archived via API"
	}

	if err := s.svc.ArchiveSubnet(r.Context(), subnetID, req.Reason); err != nil {
		s.logger.Error("archive subnet failed", "subnet", subnetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to archive subnet")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "archived",
		"message": "subnet and auto-owned targets archived",
	})
}

func (s *Server) handleListSubnetTargets(w http.ResponseWriter, r *http.Request) {
	subnetID := r.PathValue("id")
	if subnetID == "" {
		s.writeError(w, http.StatusBadRequest, "subnet ID required")
		return
	}

	targets, err := s.svc.ListTargetsBySubnet(r.Context(), subnetID)
	if err != nil {
		s.logger.Error("list subnet targets failed", "subnet", subnetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list subnet targets")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"subnet_id": subnetID,
		"targets":   targets,
		"count":     len(targets),
	})
}

func (s *Server) handleGetSubnetStats(w http.ResponseWriter, r *http.Request) {
	subnetID := r.PathValue("id")
	if subnetID == "" {
		s.writeError(w, http.StatusBadRequest, "subnet ID required")
		return
	}

	counts, err := s.svc.GetSubnetTargetCounts(r.Context(), subnetID)
	if err != nil {
		s.logger.Error("get subnet stats failed", "subnet", subnetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get subnet stats")
		return
	}

	hasCoverage, err := s.svc.SubnetHasActiveCoverage(r.Context(), subnetID)
	if err != nil {
		s.logger.Error("check subnet coverage failed", "subnet", subnetID, "error", err)
		// Non-fatal, continue
		hasCoverage = false
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"subnet_id":         subnetID,
		"target_counts":     counts,
		"has_active_coverage": hasCoverage,
	})
}

func (s *Server) handleSeedSubnetTargets(w http.ResponseWriter, r *http.Request) {
	subnetID := r.PathValue("id")
	if subnetID == "" {
		s.writeError(w, http.StatusBadRequest, "subnet ID required")
		return
	}

	result, err := s.svc.SeedSubnetTargets(r.Context(), subnetID)
	if err != nil {
		s.logger.Error("seed subnet targets failed", "subnet", subnetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to seed subnet targets")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"subnet_id":               subnetID,
		"gateway_created":         result.GatewayCreated,
		"customer_targets_created": result.CustomerTargetsCreated,
		"errors":                  result.Errors,
	})
}

// =============================================================================
// TARGET STATE ENDPOINTS
// =============================================================================

func (s *Server) handleListTargetsNeedingReview(w http.ResponseWriter, r *http.Request) {
	targets, err := s.svc.ListTargetsNeedingReview(r.Context())
	if err != nil {
		s.logger.Error("list targets needing review failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list targets needing review")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"targets": targets,
		"count":   len(targets),
	})
}

func (s *Server) handleTransitionTargetState(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	var req struct {
		NewState    string `json:"new_state"`
		Reason      string `json:"reason"`
		TriggeredBy string `json:"triggered_by"`
	}
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.NewState == "" {
		s.writeError(w, http.StatusBadRequest, "new_state is required")
		return
	}

	// Validate state
	newState := types.MonitoringState(req.NewState)
	validStates := map[types.MonitoringState]bool{
		types.StateUnknown:      true,
		types.StateActive:       true,
		types.StateDegraded:     true,
		types.StateDown:         true,
		types.StateUnresponsive: true,
		types.StateExcluded:     true,
		types.StateInactive:     true,
	}
	if !validStates[newState] {
		s.writeError(w, http.StatusBadRequest, "invalid state: "+req.NewState)
		return
	}

	triggeredBy := req.TriggeredBy
	if triggeredBy == "" {
		triggeredBy = "api"
	}

	if err := s.svc.TransitionTargetState(r.Context(), targetID, newState, req.Reason, triggeredBy); err != nil {
		s.logger.Error("transition target state failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to transition target state")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":    "transitioned",
		"new_state": string(newState),
	})
}

func (s *Server) handleAcknowledgeTarget(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	var req struct {
		MarkInactive   bool   `json:"mark_inactive"`
		Notes          string `json:"notes,omitempty"`
		AcknowledgedBy string `json:"acknowledged_by,omitempty"`
	}
	if err := s.readJSON(r, &req); err != nil {
		// Use defaults
	}

	if err := s.svc.AcknowledgeTarget(r.Context(), service.AcknowledgeTargetRequest{
		TargetID:       targetID,
		MarkInactive:   req.MarkInactive,
		Notes:          req.Notes,
		AcknowledgedBy: req.AcknowledgedBy,
	}); err != nil {
		s.logger.Error("acknowledge target failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to acknowledge target")
		return
	}

	action := "cleared from review"
	if req.MarkInactive {
		action = "marked inactive"
	}
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "acknowledged",
		"message": "target " + action,
	})
}

func (s *Server) handleGetTargetStateHistory(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	history, err := s.svc.GetTargetStateHistory(r.Context(), targetID, limit)
	if err != nil {
		s.logger.Error("get target state history failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get target state history")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"target_id": targetID,
		"history":   history,
		"count":     len(history),
	})
}

// =============================================================================
// TARGET UPDATE/DELETE
// =============================================================================

type updateTargetRequest struct {
	Tier            string             `json:"tier,omitempty"`
	Tags            map[string]string  `json:"tags,omitempty"`
	DisplayName     string             `json:"display_name,omitempty"`
	Notes           string             `json:"notes,omitempty"`
	ExpectedOutcome *types.ExpectedOutcome `json:"expected_outcome,omitempty"`
}

func (s *Server) handleUpdateTarget(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	var req updateTargetRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	target, err := s.svc.UpdateTarget(r.Context(), service.UpdateTargetRequest{
		ID:              targetID,
		Tier:            req.Tier,
		Tags:            req.Tags,
		DisplayName:     req.DisplayName,
		Notes:           req.Notes,
		ExpectedOutcome: req.ExpectedOutcome,
	})
	if err != nil {
		s.logger.Error("update target failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to update target")
		return
	}

	s.writeJSON(w, http.StatusOK, target)
}

func (s *Server) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	var req struct {
		Reason string `json:"reason,omitempty"`
	}
	if err := s.readJSON(r, &req); err != nil {
		// Use default reason
	}

	if err := s.svc.DeleteTarget(r.Context(), targetID, req.Reason); err != nil {
		s.logger.Error("delete target failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to delete target")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "archived",
		"message": "target archived successfully",
	})
}

// =============================================================================
// TARGET TAG ENDPOINTS
// =============================================================================

func (s *Server) handleGetTargetTagKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.svc.GetTargetTagKeys(r.Context())
	if err != nil {
		s.logger.Error("get target tag keys failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get target tag keys")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"keys": keys,
	})
}

// =============================================================================
// ACTIVITY LOG ENDPOINTS
// =============================================================================

func (s *Server) handleListActivity(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	limitStr := query.Get("limit")
	limit := 100
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	activity, err := s.svc.ListActivity(r.Context(), service.ActivityFilter{
		TargetID: query.Get("target_id"),
		SubnetID: query.Get("subnet_id"),
		AgentID:  query.Get("agent_id"),
		IP:       query.Get("ip"),
		Category: query.Get("category"),
		Severity: query.Get("severity"),
		Limit:    limit,
	})
	if err != nil {
		s.logger.Error("list activity failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list activity")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"activity": activity,
		"count":    len(activity),
	})
}

func (s *Server) handleGetTargetActivity(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	activity, err := s.svc.GetTargetActivity(r.Context(), targetID, limit)
	if err != nil {
		s.logger.Error("get target activity failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get target activity")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"target_id": targetID,
		"activity":  activity,
		"count":     len(activity),
	})
}

func (s *Server) handleGetSubnetActivity(w http.ResponseWriter, r *http.Request) {
	subnetID := r.PathValue("id")
	if subnetID == "" {
		s.writeError(w, http.StatusBadRequest, "subnet ID required")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	activity, err := s.svc.GetSubnetActivity(r.Context(), subnetID, limit)
	if err != nil {
		s.logger.Error("get subnet activity failed", "subnet", subnetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get subnet activity")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"subnet_id": subnetID,
		"activity":  activity,
		"count":     len(activity),
	})
}
