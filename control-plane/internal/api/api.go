// Package api provides HTTP handlers for the control plane.
//
// # Endpoints
//
// Agent API:
//   - POST /api/v1/agents/register - Register new agent
//   - POST /api/v1/agents/{id}/heartbeat - Agent heartbeat
//   - GET  /api/v1/agents/{id}/assignments - Get assignments
//   - GET  /api/v1/agents/{id}/commands - Poll for commands
//   - POST /api/v1/agents/{id}/commands/{cmd}/result - Report command result
//
// Management API:
//   - GET  /api/v1/agents - List agents
//   - GET  /api/v1/agents/{id} - Get agent details
//   - PUT  /api/v1/agents/{id} - Update agent info
//   - GET  /api/v1/agents/{id}/metrics - Get agent metrics history
//   - GET  /api/v1/agents/{id}/stats - Get agent current stats
//   - POST /api/v1/agents/{id}/archive - Archive agent (soft-delete)
//   - POST /api/v1/agents/{id}/unarchive - Restore archived agent
//   - GET  /api/v1/fleet/overview - Get fleet overview stats
//   - GET  /api/v1/fleet/agents/stats - Get all agents current stats
//   - GET  /api/v1/targets - List targets
//   - POST /api/v1/targets - Create target
//   - GET  /api/v1/tiers - List tiers
//
// Subnet API:
//   - GET    /api/v1/subnets - List all subnets
//   - POST   /api/v1/subnets - Create subnet
//   - GET    /api/v1/subnets/{id} - Get subnet details
//   - PUT    /api/v1/subnets/{id} - Update subnet
//   - POST   /api/v1/subnets/{id}/archive - Archive subnet
//   - GET    /api/v1/subnets/{id}/targets - List targets in subnet
//   - GET    /api/v1/subnets/{id}/stats - Get subnet target counts
//
// Target State API:
//   - GET    /api/v1/targets/review - List targets needing review
//   - POST   /api/v1/targets/{id}/state - Transition target state
//   - POST   /api/v1/targets/{id}/acknowledge - Acknowledge target
//   - GET    /api/v1/targets/{id}/state-history - Get state transition history
//
// Results API:
//   - POST /api/v1/results - Ingest probe results
//
// Health:
//   - GET /api/v1/health - Health check
package api

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pilot-net/icmp-mon/control-plane/internal/cache"
	"github.com/pilot-net/icmp-mon/control-plane/internal/metrics"
	"github.com/pilot-net/icmp-mon/control-plane/internal/service"
	"github.com/pilot-net/icmp-mon/control-plane/internal/store"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// Cache TTLs for various endpoints
const (
	cacheTTLFleetOverview     = 30 * time.Second
	cacheTTLTargetStatuses    = 30 * time.Second
	cacheTTLInfraHealth       = 60 * time.Second
	cacheTTLLatencyMatrix     = 60 * time.Second
	cacheTTLInMarketLatency   = 30 * time.Second
	cacheTTLLatencyTrend      = 30 * time.Second
	cacheTTLTargetList        = 60 * time.Second
)

// Server is the HTTP API server.
type Server struct {
	svc              *service.Service
	metricsCollector *metrics.Collector
	cache            *cache.Cache
	logger           *slog.Logger
	mux              *http.ServeMux

	// Agent authentication (disabled by default for grace period)
	agentAuthEnabled bool
}

// NewServer creates a new API server.
func NewServer(svc *service.Service, metricsCollector *metrics.Collector, responseCache *cache.Cache, logger *slog.Logger) *Server {
	s := &Server{
		svc:              svc,
		metricsCollector: metricsCollector,
		cache:            responseCache,
		logger:           logger,
		mux:              http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Mux returns the underlying ServeMux for registering additional routes.
func (s *Server) Mux() *http.ServeMux {
	return s.mux
}

// EnableAgentAuth enables agent API key authentication enforcement.
// By default, auth is in grace period mode (logs but doesn't reject).
func (s *Server) EnableAgentAuth() {
	s.agentAuthEnabled = true
	s.logger.Info("agent API key authentication enabled")
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Agent-ID")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Log request
	start := time.Now()
	s.mux.ServeHTTP(w, r)
	s.logger.Debug("request",
		"method", r.Method,
		"path", r.URL.Path,
		"duration", time.Since(start))
}

func (s *Server) registerRoutes() {
	// Create the agent auth middleware (grace period by default, checks but doesn't reject)
	agentAuth := s.AgentAuthMiddleware(AgentAuthConfig{
		Enabled: s.agentAuthEnabled,
		Logger:  s.logger,
	})

	// Health
	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/v1/infrastructure/health", s.handleInfrastructureHealth)

	// Agent registration (open - no auth required, agents don't have keys yet)
	s.mux.HandleFunc("POST /api/v1/agents/register", s.handleAgentRegister)

	// Agent lifecycle (authenticated - these are agent-to-control-plane calls)
	s.mux.HandleFunc("POST /api/v1/agents/{id}/heartbeat", wrapHandler(s.handleAgentHeartbeat, agentAuth))
	s.mux.HandleFunc("GET /api/v1/agents/{id}/assignments", wrapHandler(s.handleAgentAssignments, agentAuth))
	s.mux.HandleFunc("GET /api/v1/agents/{id}/commands", wrapHandler(s.handleAgentCommands, agentAuth))
	s.mux.HandleFunc("POST /api/v1/agents/{id}/commands/{cmd}/result", wrapHandler(s.handleAgentCommandResult, agentAuth))

	// Agent management
	s.mux.HandleFunc("GET /api/v1/agents", s.handleListAgents)
	s.mux.HandleFunc("GET /api/v1/agents/{id}", s.handleGetAgent)
	s.mux.HandleFunc("PUT /api/v1/agents/{id}", s.handleUpdateAgent)
	s.mux.HandleFunc("GET /api/v1/agents/{id}/metrics", s.handleAgentMetrics)
	s.mux.HandleFunc("GET /api/v1/agents/{id}/stats", s.handleAgentStats)
	s.mux.HandleFunc("POST /api/v1/agents/{id}/archive", s.handleArchiveAgent)
	s.mux.HandleFunc("POST /api/v1/agents/{id}/unarchive", s.handleUnarchiveAgent)

	// Fleet overview
	s.mux.HandleFunc("GET /api/v1/fleet/overview", s.handleFleetOverview)
	s.mux.HandleFunc("GET /api/v1/fleet/agents/stats", s.handleAllAgentsStats)

	// Targets - static routes must come before wildcard {id} routes
	s.mux.HandleFunc("GET /api/v1/targets", s.handleListTargets)
	s.mux.HandleFunc("POST /api/v1/targets", s.handleCreateTarget)
	s.mux.HandleFunc("GET /api/v1/targets/status", s.handleGetAllTargetStatuses)
	s.mux.HandleFunc("GET /api/v1/targets/review", s.handleListTargetsNeedingReview)
	s.mux.HandleFunc("GET /api/v1/targets/tag-keys", s.handleGetTargetTagKeys)
	s.mux.HandleFunc("GET /api/v1/targets/{id}", s.handleGetTarget)
	s.mux.HandleFunc("GET /api/v1/targets/{id}/status", s.handleGetTargetStatus)
	s.mux.HandleFunc("GET /api/v1/targets/{id}/history", s.handleGetTargetHistory)
	s.mux.HandleFunc("GET /api/v1/targets/{id}/history/by-agent", s.handleGetTargetHistoryByAgent)
	s.mux.HandleFunc("GET /api/v1/targets/{id}/live", s.handleGetTargetLive)
	s.mux.HandleFunc("POST /api/v1/targets/{id}/mtr", s.handleTriggerMTR)
	s.mux.HandleFunc("GET /api/v1/targets/{id}/commands", s.handleGetTargetCommands)

	// Commands
	s.mux.HandleFunc("GET /api/v1/commands/{id}", s.handleGetCommand)

	// Metrics
	s.mux.HandleFunc("GET /api/v1/metrics/latency", s.handleGetLatencyTrend)
	s.mux.HandleFunc("GET /api/v1/metrics/latency/in-market", s.handleGetInMarketLatencyTrend)
	s.mux.HandleFunc("GET /api/v1/metrics/latency/matrix", s.handleGetLatencyMatrix)
	s.mux.HandleFunc("POST /api/v1/metrics/query", s.handleQueryMetrics)

	// Tiers
	s.mux.HandleFunc("GET /api/v1/tiers", s.handleListTiers)
	s.mux.HandleFunc("GET /api/v1/tiers/{name}", s.handleGetTier)
	s.mux.HandleFunc("POST /api/v1/tiers", s.handleCreateTier)
	s.mux.HandleFunc("PUT /api/v1/tiers/{name}", s.handleUpdateTier)
	s.mux.HandleFunc("DELETE /api/v1/tiers/{name}", s.handleDeleteTier)

	// Incidents
	s.mux.HandleFunc("GET /api/v1/incidents", s.handleListIncidents)
	s.mux.HandleFunc("GET /api/v1/incidents/{id}", s.handleGetIncident)
	s.mux.HandleFunc("POST /api/v1/incidents/{id}/acknowledge", s.handleAcknowledgeIncident)
	s.mux.HandleFunc("POST /api/v1/incidents/{id}/resolve", s.handleResolveIncident)
	s.mux.HandleFunc("PUT /api/v1/incidents/{id}/notes", s.handleAddIncidentNote)

	// Baselines
	s.mux.HandleFunc("GET /api/v1/baselines/{agent_id}/{target_id}", s.handleGetBaseline)
	s.mux.HandleFunc("GET /api/v1/targets/{id}/baselines", s.handleGetTargetBaselines)
	s.mux.HandleFunc("POST /api/v1/baselines/recalculate", s.handleRecalculateBaselines)

	// Reports
	s.mux.HandleFunc("GET /api/v1/reports/targets/{id}", s.handleGetTargetReport)

	// Results ingestion (authenticated - agents submit probe results)
	s.mux.HandleFunc("POST /api/v1/results", wrapHandler(s.handleIngestResults, agentAuth))

	// Subnets
	s.mux.HandleFunc("GET /api/v1/subnets", s.handleListSubnets)
	s.mux.HandleFunc("POST /api/v1/subnets", s.handleCreateSubnet)
	s.mux.HandleFunc("GET /api/v1/subnets/{id}", s.handleGetSubnet)
	s.mux.HandleFunc("PUT /api/v1/subnets/{id}", s.handleUpdateSubnet)
	s.mux.HandleFunc("POST /api/v1/subnets/{id}/archive", s.handleArchiveSubnet)
	s.mux.HandleFunc("GET /api/v1/subnets/{id}/targets", s.handleListSubnetTargets)
	s.mux.HandleFunc("GET /api/v1/subnets/{id}/stats", s.handleGetSubnetStats)
	s.mux.HandleFunc("POST /api/v1/subnets/{id}/seed", s.handleSeedSubnetTargets)

	// Target state management (dynamic routes already registered above)
	s.mux.HandleFunc("POST /api/v1/targets/{id}/state", s.handleTransitionTargetState)
	s.mux.HandleFunc("POST /api/v1/targets/{id}/acknowledge", s.handleAcknowledgeTarget)
	s.mux.HandleFunc("GET /api/v1/targets/{id}/state-history", s.handleGetTargetStateHistory)

	// Target update/delete
	s.mux.HandleFunc("PUT /api/v1/targets/{id}", s.handleUpdateTarget)
	s.mux.HandleFunc("DELETE /api/v1/targets/{id}", s.handleDeleteTarget)

	// Activity log
	s.mux.HandleFunc("GET /api/v1/activity", s.handleListActivity)
	s.mux.HandleFunc("GET /api/v1/targets/{id}/activity", s.handleGetTargetActivity)
	s.mux.HandleFunc("GET /api/v1/subnets/{id}/activity", s.handleGetSubnetActivity)

	// Alerts
	s.mux.HandleFunc("GET /api/v1/alerts", s.handleListAlerts)
	s.mux.HandleFunc("GET /api/v1/alerts/stats", s.handleGetAlertStats)
	s.mux.HandleFunc("GET /api/v1/alerts/correlations", s.handleGetAlertCorrelations)
	s.mux.HandleFunc("GET /api/v1/alerts/{id}", s.handleGetAlert)
	s.mux.HandleFunc("GET /api/v1/alerts/{id}/events", s.handleGetAlertEvents)
	s.mux.HandleFunc("POST /api/v1/alerts/{id}/acknowledge", s.handleAcknowledgeAlert)
	s.mux.HandleFunc("POST /api/v1/alerts/{id}/resolve", s.handleResolveAlert)

	// Alert configuration
	s.mux.HandleFunc("GET /api/v1/alerts/config", s.handleListAlertConfigs)
	s.mux.HandleFunc("PUT /api/v1/alerts/config/{key}", s.handleUpdateAlertConfig)

	// Agent binary packages (for enrollment)
	s.mux.HandleFunc("GET /api/v1/packages/{platform}", s.handleGetPackage)
}

// =============================================================================
// HEALTH
// =============================================================================

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleInfrastructureHealth(w http.ResponseWriter, r *http.Request) {
	if s.metricsCollector == nil {
		s.writeError(w, http.StatusServiceUnavailable, "metrics collector not initialized")
		return
	}

	const cacheKey = "infrastructure_health"

	// Try cache first
	if s.cache != nil {
		if data, err := s.cache.Get(r.Context(), cacheKey); err == nil && data != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			return
		}
	}

	health, err := s.metricsCollector.GetInfrastructureHealth(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to get infrastructure health: "+err.Error())
		return
	}

	// Cache the result
	if s.cache != nil {
		if err := s.cache.SetJSON(r.Context(), cacheKey, health, cacheTTLInfraHealth); err != nil {
			s.logger.Warn("failed to cache infrastructure health", "error", err)
		}
	}

	s.writeJSON(w, http.StatusOK, health)
}

// =============================================================================
// AGENT ENDPOINTS
// =============================================================================

type registerRequest struct {
	Name       string            `json:"name"`
	Region     string            `json:"region"`
	Location   string            `json:"location"`
	Provider   string            `json:"provider"`
	Tags       map[string]string `json:"tags"`
	PublicIP   string            `json:"public_ip"`
	Version    string            `json:"version"`
	Executors  []string          `json:"executors"`
	MaxTargets int               `json:"max_targets"`
}

func (s *Server) handleAgentRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	agent, err := s.svc.RegisterAgent(r.Context(), service.RegisterAgentRequest{
		Name:       req.Name,
		Region:     req.Region,
		Location:   req.Location,
		Provider:   req.Provider,
		Tags:       req.Tags,
		PublicIP:   req.PublicIP,
		Version:    req.Version,
		Executors:  req.Executors,
		MaxTargets: req.MaxTargets,
	})
	if err != nil {
		s.logger.Error("agent registration failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "registration failed")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"agent_id": agent.ID,
		"message":  "registered successfully",
	})
}

func (s *Server) handleAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		s.writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}

	var heartbeat types.Heartbeat
	if err := s.readJSON(r, &heartbeat); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	heartbeat.AgentID = agentID

	resp, err := s.svc.ProcessHeartbeat(r.Context(), heartbeat)
	if err != nil {
		s.logger.Error("heartbeat processing failed", "agent", agentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "heartbeat failed")
		return
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAgentAssignments(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		s.writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}

	// Get optional since parameter for delta sync
	// sinceStr := r.URL.Query().Get("since")

	assignments, err := s.svc.GetAssignments(r.Context(), agentID)
	if err != nil {
		s.logger.Error("get assignments failed", "agent", agentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get assignments")
		return
	}

	s.writeJSON(w, http.StatusOK, assignments)
}

func (s *Server) handleAgentCommands(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		s.writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}

	commands, err := s.svc.GetPendingCommands(r.Context(), agentID)
	if err != nil {
		s.logger.Error("get pending commands failed", "agent", agentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get commands")
		return
	}

	// Convert store.Command to types.Command for agent consumption
	agentCommands := make([]types.Command, len(commands))
	for i, cmd := range commands {
		var paramsJSON json.RawMessage
		if cmd.Params != nil {
			paramsJSON, _ = json.Marshal(cmd.Params)
		}
		expiresAt := time.Time{}
		if cmd.ExpiresAt != nil {
			expiresAt = *cmd.ExpiresAt
		}
		agentCommands[i] = types.Command{
			ID:          cmd.ID,
			Type:        cmd.CommandType,
			TargetIP:    cmd.TargetIP,
			Params:      paramsJSON,
			RequestedBy: cmd.RequestedBy,
			RequestedAt: cmd.RequestedAt,
			ExpiresAt:   expiresAt,
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"commands": agentCommands,
	})
}

func (s *Server) handleAgentCommandResult(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	commandID := r.PathValue("cmd")
	if agentID == "" || commandID == "" {
		s.writeError(w, http.StatusBadRequest, "agent ID and command ID required")
		return
	}

	var result struct {
		Success    bool            `json:"success"`
		Error      string          `json:"error,omitempty"`
		Payload    json.RawMessage `json:"payload,omitempty"`
		DurationMs int             `json:"duration_ms"`
	}
	if err := s.readJSON(r, &result); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cmdResult := &store.CommandResult{
		CommandID:   commandID,
		AgentID:     agentID,
		Success:     result.Success,
		Error:       result.Error,
		Payload:     result.Payload,
		DurationMs:  result.DurationMs,
		CompletedAt: time.Now(),
	}

	if err := s.svc.SaveCommandResult(r.Context(), cmdResult); err != nil {
		s.logger.Error("save command result failed", "command", commandID, "agent", agentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to save result")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "accepted",
	})
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.svc.ListAgents(r.Context())
	if err != nil {
		s.logger.Error("list agents failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"agents": agents,
		"count":  len(agents),
	})
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		s.writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}

	agent, err := s.svc.GetAgent(r.Context(), agentID)
	if err != nil {
		s.logger.Error("get agent failed", "agent", agentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}
	if agent == nil {
		s.writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	s.writeJSON(w, http.StatusOK, agent)
}

type updateAgentRequest struct {
	Name       string            `json:"name"`
	Region     string            `json:"region"`
	Location   string            `json:"location"`
	Provider   string            `json:"provider"`
	Tags       map[string]string `json:"tags"`
	MaxTargets int               `json:"max_targets"`
}

func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		s.writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}

	var req updateAgentRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	err := s.svc.UpdateAgentInfo(r.Context(), agentID, service.UpdateAgentInfoRequest{
		Name:       req.Name,
		Region:     req.Region,
		Location:   req.Location,
		Provider:   req.Provider,
		Tags:       req.Tags,
		MaxTargets: req.MaxTargets,
	})
	if err != nil {
		s.logger.Error("update agent failed", "agent", agentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to update agent")
		return
	}

	// Return the updated agent
	agent, err := s.svc.GetAgent(r.Context(), agentID)
	if err != nil {
		s.logger.Error("get agent after update failed", "agent", agentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "agent updated but failed to retrieve")
		return
	}

	s.writeJSON(w, http.StatusOK, agent)
}

func (s *Server) handleArchiveAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		s.writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := s.readJSON(r, &req); err != nil {
		req.Reason = "archived via API"
	}

	if err := s.svc.ArchiveAgent(r.Context(), agentID, req.Reason); err != nil {
		s.logger.Error("archive agent failed", "agent", agentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to archive agent")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "archived",
		"message": "agent archived successfully",
	})
}

func (s *Server) handleUnarchiveAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		s.writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}

	if err := s.svc.UnarchiveAgent(r.Context(), agentID); err != nil {
		s.logger.Error("unarchive agent failed", "agent", agentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to unarchive agent")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "unarchived",
		"message": "agent restored successfully",
	})
}

// =============================================================================
// AGENT METRICS ENDPOINTS
// =============================================================================

func (s *Server) handleAgentMetrics(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		s.writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}

	// Parse duration from query params (default 1h)
	durationStr := r.URL.Query().Get("duration")
	if durationStr == "" {
		durationStr = "1h"
	}
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid duration format")
		return
	}

	// Cap at 24h to prevent excessive data
	if duration > 24*time.Hour {
		duration = 24 * time.Hour
	}

	metrics, err := s.svc.GetAgentMetrics(r.Context(), agentID, duration)
	if err != nil {
		s.logger.Error("get agent metrics failed", "agent", agentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get agent metrics")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"agent_id": agentID,
		"duration": duration.String(),
		"metrics":  metrics,
		"count":    len(metrics),
	})
}

func (s *Server) handleAgentStats(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		s.writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}

	stats, err := s.svc.GetAgentCurrentStats(r.Context(), agentID)
	if err != nil {
		s.logger.Error("get agent stats failed", "agent", agentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get agent stats")
		return
	}

	if stats == nil {
		s.writeError(w, http.StatusNotFound, "no stats available for agent")
		return
	}

	s.writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleFleetOverview(w http.ResponseWriter, r *http.Request) {
	const cacheKey = "fleet_overview"

	// Try cache first
	if s.cache != nil {
		if data, err := s.cache.Get(r.Context(), cacheKey); err == nil && data != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			return
		}
	}

	overview, err := s.svc.GetFleetOverview(r.Context())
	if err != nil {
		s.logger.Error("get fleet overview failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get fleet overview")
		return
	}

	// Cache the result
	if s.cache != nil {
		if err := s.cache.SetJSON(r.Context(), cacheKey, overview, cacheTTLFleetOverview); err != nil {
			s.logger.Warn("failed to cache fleet overview", "error", err)
		}
	}

	s.writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleAllAgentsStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.svc.GetAllAgentsCurrentStats(r.Context())
	if err != nil {
		s.logger.Error("get all agents stats failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get agents stats")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"agents": stats,
		"count":  len(stats),
	})
}

// =============================================================================
// TARGET ENDPOINTS
// =============================================================================

func (s *Server) handleListTargets(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Check if pagination is requested
	limitStr := query.Get("limit")
	offsetStr := query.Get("offset")

	// If pagination params present, use paginated endpoint
	if limitStr != "" || offsetStr != "" {
		params := store.TargetListParams{
			Tier:            query.Get("tier"),
			MonitoringState: query.Get("state"),
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

		result, err := s.svc.ListTargetsPaginated(r.Context(), params)
		if err != nil {
			s.logger.Error("list targets paginated failed", "error", err)
			s.writeError(w, http.StatusInternalServerError, "failed to list targets")
			return
		}

		s.writeJSON(w, http.StatusOK, result)
		return
	}

	// Legacy: return all targets (with caching)
	const cacheKey = "target_list"

	// Try cache first
	if s.cache != nil {
		if data, err := s.cache.Get(r.Context(), cacheKey); err == nil && data != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			return
		}
	}

	targets, err := s.svc.ListTargets(r.Context())
	if err != nil {
		s.logger.Error("list targets failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list targets")
		return
	}

	response := map[string]any{
		"targets": targets,
		"count":   len(targets),
	}

	// Cache the result
	if s.cache != nil {
		if data, err := json.Marshal(response); err == nil {
			if err := s.cache.Set(r.Context(), cacheKey, data, cacheTTLTargetList); err != nil {
				s.logger.Warn("failed to cache target list", "error", err)
			}
		}
	}

	s.writeJSON(w, http.StatusOK, response)
}

type createTargetRequest struct {
	IP              string                 `json:"ip"`
	Tier            string                 `json:"tier"`
	SubscriberID    string                 `json:"subscriber_id,omitempty"`
	Tags            map[string]string      `json:"tags,omitempty"`
	ExpectedOutcome *types.ExpectedOutcome `json:"expected_outcome,omitempty"`
}

func (s *Server) handleCreateTarget(w http.ResponseWriter, r *http.Request) {
	var req createTargetRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.IP == "" {
		s.writeError(w, http.StatusBadRequest, "ip is required")
		return
	}
	if req.Tier == "" {
		req.Tier = "standard"
	}

	target, err := s.svc.CreateTarget(r.Context(), service.CreateTargetRequest{
		IP:              req.IP,
		Tier:            req.Tier,
		SubscriberID:    req.SubscriberID,
		Tags:            req.Tags,
		ExpectedOutcome: req.ExpectedOutcome,
	})
	if err != nil {
		s.logger.Error("create target failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to create target")
		return
	}

	s.writeJSON(w, http.StatusCreated, target)
}

func (s *Server) handleGetTarget(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	target, err := s.svc.GetTarget(r.Context(), targetID)
	if err != nil {
		s.logger.Error("get target failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get target")
		return
	}
	if target == nil {
		s.writeError(w, http.StatusNotFound, "target not found")
		return
	}

	s.writeJSON(w, http.StatusOK, target)
}

// =============================================================================
// TIER ENDPOINTS
// =============================================================================

func (s *Server) handleListTiers(w http.ResponseWriter, r *http.Request) {
	tiers, err := s.svc.ListTiers(r.Context())
	if err != nil {
		s.logger.Error("list tiers failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list tiers")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"tiers": tiers,
		"count": len(tiers),
	})
}

func (s *Server) handleGetTier(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tier, err := s.svc.GetTier(r.Context(), name)
	if err != nil {
		s.logger.Error("get tier failed", "error", err, "name", name)
		s.writeError(w, http.StatusInternalServerError, "failed to get tier")
		return
	}
	if tier == nil {
		s.writeError(w, http.StatusNotFound, "tier not found")
		return
	}
	s.writeJSON(w, http.StatusOK, tier)
}

func (s *Server) handleCreateTier(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string                    `json:"name"`
		DisplayName    string                    `json:"display_name"`
		ProbeIntervalS int                       `json:"probe_interval_seconds"`
		ProbeTimeoutS  int                       `json:"probe_timeout_seconds"`
		ProbeRetries   int                       `json:"probe_retries"`
		AgentSelection types.AgentSelectionPolicy `json:"agent_selection"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	tier := &types.Tier{
		Name:           req.Name,
		DisplayName:    req.DisplayName,
		ProbeInterval:  time.Duration(req.ProbeIntervalS) * time.Second,
		ProbeTimeout:   time.Duration(req.ProbeTimeoutS) * time.Second,
		ProbeRetries:   req.ProbeRetries,
		AgentSelection: req.AgentSelection,
	}

	if tier.DisplayName == "" {
		tier.DisplayName = tier.Name
	}
	if tier.ProbeInterval == 0 {
		tier.ProbeInterval = 30 * time.Second
	}
	if tier.ProbeTimeout == 0 {
		tier.ProbeTimeout = 5 * time.Second
	}
	if tier.AgentSelection.Strategy == "" {
		tier.AgentSelection.Strategy = "all"
	}

	if err := s.svc.CreateTier(r.Context(), tier); err != nil {
		s.logger.Error("create tier failed", "error", err, "name", req.Name)
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusCreated, tier)
}

func (s *Server) handleUpdateTier(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req struct {
		DisplayName    string                    `json:"display_name"`
		ProbeIntervalS int                       `json:"probe_interval_seconds"`
		ProbeTimeoutS  int                       `json:"probe_timeout_seconds"`
		ProbeRetries   int                       `json:"probe_retries"`
		AgentSelection types.AgentSelectionPolicy `json:"agent_selection"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tier := &types.Tier{
		Name:           name,
		DisplayName:    req.DisplayName,
		ProbeInterval:  time.Duration(req.ProbeIntervalS) * time.Second,
		ProbeTimeout:   time.Duration(req.ProbeTimeoutS) * time.Second,
		ProbeRetries:   req.ProbeRetries,
		AgentSelection: req.AgentSelection,
	}

	if err := s.svc.UpdateTier(r.Context(), tier); err != nil {
		s.logger.Error("update tier failed", "error", err, "name", name)
		if err.Error() == "tier not found: "+name {
			s.writeError(w, http.StatusNotFound, err.Error())
		} else {
			s.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	s.writeJSON(w, http.StatusOK, tier)
}

func (s *Server) handleDeleteTier(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := s.svc.DeleteTier(r.Context(), name); err != nil {
		s.logger.Error("delete tier failed", "error", err, "name", name)
		if strings.Contains(err.Error(), "targets are using it") {
			s.writeError(w, http.StatusConflict, err.Error())
		} else if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, err.Error())
		} else {
			s.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// RESULTS INGESTION
// =============================================================================

func (s *Server) handleIngestResults(w http.ResponseWriter, r *http.Request) {
	// Handle gzip compression
	var reader io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid gzip")
			return
		}
		defer gz.Close()
		reader = gz
	}

	var batch types.ResultBatch
	if err := json.NewDecoder(reader).Decode(&batch); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.svc.IngestResults(r.Context(), batch); err != nil {
		s.logger.Error("result ingestion failed",
			"agent", batch.AgentID,
			"count", len(batch.Results),
			"error", err)
		s.writeError(w, http.StatusInternalServerError, "ingestion failed")
		return
	}

	s.writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": len(batch.Results),
	})
}

// =============================================================================
// HELPERS
// =============================================================================

func (s *Server) readJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{
		"error": message,
	})
}

// getAgentID extracts agent ID from request header or path.
func getAgentID(r *http.Request) string {
	// Try header first
	if id := r.Header.Get("X-Agent-ID"); id != "" {
		return id
	}
	// Try path parameter
	parts := strings.Split(r.URL.Path, "/")
	for i, p := range parts {
		if p == "agents" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// =============================================================================
// TARGET STATUS ENDPOINTS
// =============================================================================

func (s *Server) handleGetAllTargetStatuses(w http.ResponseWriter, r *http.Request) {
	const cacheKey = "target_statuses"

	// Try cache first
	if s.cache != nil {
		if data, err := s.cache.Get(r.Context(), cacheKey); err == nil && data != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			return
		}
	}

	statuses, err := s.svc.GetAllTargetStatuses(r.Context())
	if err != nil {
		s.logger.Error("get target statuses failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get target statuses")
		return
	}

	response := map[string]any{
		"statuses": statuses,
		"count":    len(statuses),
	}

	// Cache the result
	if s.cache != nil {
		if err := s.cache.SetJSON(r.Context(), cacheKey, response, cacheTTLTargetStatuses); err != nil {
			s.logger.Warn("failed to cache target statuses", "error", err)
		}
	}

	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleGetTargetStatus(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	status, err := s.svc.GetTargetStatus(r.Context(), targetID)
	if err != nil {
		s.logger.Error("get target status failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get target status")
		return
	}
	if status == nil {
		s.writeError(w, http.StatusNotFound, "target not found")
		return
	}

	s.writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleGetTargetHistory(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	// Get window from query param, default to 1 hour
	windowStr := r.URL.Query().Get("window")
	window := time.Hour
	if windowStr != "" {
		if parsed, err := time.ParseDuration(windowStr); err == nil {
			window = parsed
		}
	}

	history, err := s.svc.GetTargetHistory(r.Context(), targetID, window)
	if err != nil {
		s.logger.Error("get target history failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get target history")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"target_id": targetID,
		"window":    window.String(),
		"history":   history,
	})
}

func (s *Server) handleGetTargetHistoryByAgent(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	// Get window from query param, default to 1 hour
	windowStr := r.URL.Query().Get("window")
	window := time.Hour
	if windowStr != "" {
		if parsed, err := time.ParseDuration(windowStr); err == nil {
			window = parsed
		}
	}

	history, err := s.svc.GetTargetHistoryByAgent(r.Context(), targetID, window)
	if err != nil {
		s.logger.Error("get target history by agent failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get target history by agent")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"target_id": targetID,
		"window":    window.String(),
		"history":   history,
	})
}

func (s *Server) handleGetTargetLive(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	// Get seconds from query param, default to 60
	secondsStr := r.URL.Query().Get("seconds")
	seconds := 60
	if secondsStr != "" {
		if parsed, err := strconv.Atoi(secondsStr); err == nil && parsed > 0 {
			seconds = parsed
		}
	}

	results, err := s.svc.GetTargetLiveResults(r.Context(), targetID, seconds)
	if err != nil {
		s.logger.Error("get target live results failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get live results")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"target_id": targetID,
		"seconds":   seconds,
		"count":     len(results),
		"results":   results,
	})
}

// =============================================================================
// METRICS ENDPOINTS
// =============================================================================

func (s *Server) handleGetLatencyTrend(w http.ResponseWriter, r *http.Request) {
	// Get window from query param, default to 24 hours
	windowStr := r.URL.Query().Get("window")
	window := 24 * time.Hour
	if windowStr != "" {
		if parsed, err := time.ParseDuration(windowStr); err == nil {
			window = parsed
		}
	}

	// Cache key includes window
	cacheKey := fmt.Sprintf("latency_trend_%s", window.String())

	// Try cache first
	if s.cache != nil {
		if data, err := s.cache.Get(r.Context(), cacheKey); err == nil && data != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			return
		}
	}

	history, err := s.svc.GetLatencyTrend(r.Context(), window)
	if err != nil {
		s.logger.Error("get latency trend failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get latency trend")
		return
	}

	response := map[string]any{
		"window":  window.String(),
		"history": history,
	}

	// Cache the result
	if s.cache != nil {
		if data, err := json.Marshal(response); err == nil {
			if err := s.cache.Set(r.Context(), cacheKey, data, cacheTTLLatencyTrend); err != nil {
				s.logger.Warn("failed to cache latency trend", "error", err)
			}
		}
	}

	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleGetInMarketLatencyTrend(w http.ResponseWriter, r *http.Request) {
	// Get window from query param, default to 24 hours
	windowStr := r.URL.Query().Get("window")
	window := 24 * time.Hour
	if windowStr != "" {
		if parsed, err := time.ParseDuration(windowStr); err == nil {
			window = parsed
		}
	}

	// Cache key includes window
	cacheKey := fmt.Sprintf("in_market_latency_%s", window.String())

	// Try cache first
	if s.cache != nil {
		if data, err := s.cache.Get(r.Context(), cacheKey); err == nil && data != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			return
		}
	}

	history, err := s.svc.GetInMarketLatencyTrend(r.Context(), window)
	if err != nil {
		s.logger.Error("get in-market latency trend failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get in-market latency trend")
		return
	}

	response := map[string]any{
		"window":    window.String(),
		"in_market": true,
		"history":   history,
	}

	// Cache the result
	if s.cache != nil {
		if err := s.cache.SetJSON(r.Context(), cacheKey, response, cacheTTLInMarketLatency); err != nil {
			s.logger.Warn("failed to cache in-market latency", "error", err)
		}
	}

	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleGetLatencyMatrix(w http.ResponseWriter, r *http.Request) {
	// Get window from query param, default to 24 hours
	windowStr := r.URL.Query().Get("window")
	window := 24 * time.Hour
	if windowStr != "" {
		if parsed, err := time.ParseDuration(windowStr); err == nil {
			window = parsed
		}
	}

	// Cache key includes window for different time ranges
	cacheKey := fmt.Sprintf("latency_matrix_%s", window.String())

	// Try cache first
	if s.cache != nil {
		if data, err := s.cache.Get(r.Context(), cacheKey); err == nil && data != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			return
		}
	}

	matrix, err := s.svc.GetRegionLatencyMatrix(r.Context(), window)
	if err != nil {
		s.logger.Error("get latency matrix failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get latency matrix")
		return
	}

	// Cache the result
	if s.cache != nil {
		if err := s.cache.SetJSON(r.Context(), cacheKey, matrix, cacheTTLLatencyMatrix); err != nil {
			s.logger.Warn("failed to cache latency matrix", "error", err)
		}
	}

	s.writeJSON(w, http.StatusOK, matrix)
}

// =============================================================================
// COMMAND ENDPOINTS
// =============================================================================

func (s *Server) handleTriggerMTR(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	// Get target to find IP
	target, err := s.svc.GetTarget(r.Context(), targetID)
	if err != nil {
		s.logger.Error("get target failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get target")
		return
	}
	if target == nil {
		s.writeError(w, http.StatusNotFound, "target not found")
		return
	}

	// Optional: specific agent IDs
	var req struct {
		AgentIDs []string `json:"agent_ids,omitempty"`
	}
	s.readJSON(r, &req) // Ignore error, use empty if not provided

	cmd, err := s.svc.CreateMTRCommand(r.Context(), targetID, target.IP, req.AgentIDs)
	if err != nil {
		s.logger.Error("create MTR command failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to create MTR command")
		return
	}

	s.writeJSON(w, http.StatusAccepted, map[string]any{
		"command_id":   cmd.ID,
		"command_type": cmd.CommandType,
		"target_id":    cmd.TargetID,
		"target_ip":    cmd.TargetIP,
		"status":       cmd.Status,
		"message":      "MTR command queued for agents",
	})
}

func (s *Server) handleGetTargetCommands(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	commands, err := s.svc.GetCommandsByTarget(r.Context(), targetID, limit)
	if err != nil {
		s.logger.Error("get target commands failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get commands")
		return
	}

	s.writeJSON(w, http.StatusOK, commands)
}

func (s *Server) handleGetCommand(w http.ResponseWriter, r *http.Request) {
	commandID := r.PathValue("id")
	if commandID == "" {
		s.writeError(w, http.StatusBadRequest, "command ID required")
		return
	}

	cmd, err := s.svc.GetCommand(r.Context(), commandID)
	if err != nil {
		s.logger.Error("get command failed", "command", commandID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get command")
		return
	}
	if cmd == nil {
		s.writeError(w, http.StatusNotFound, "command not found")
		return
	}

	results, err := s.svc.GetCommandResults(r.Context(), commandID)
	if err != nil {
		s.logger.Error("get command results failed", "command", commandID, "error", err)
		// Non-fatal, continue with empty results
		results = nil
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"command": cmd,
		"results": results,
	})
}

// =============================================================================
// INCIDENT ENDPOINTS
// =============================================================================

func (s *Server) handleListIncidents(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if parsed, err := parseInt(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	incidents, err := s.svc.ListIncidents(r.Context(), status, limit)
	if err != nil {
		s.logger.Error("list incidents failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list incidents")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"incidents": incidents,
		"count":     len(incidents),
	})
}

func (s *Server) handleGetIncident(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	if incidentID == "" {
		s.writeError(w, http.StatusBadRequest, "incident ID required")
		return
	}

	incident, err := s.svc.GetIncident(r.Context(), incidentID)
	if err != nil {
		s.logger.Error("get incident failed", "incident", incidentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get incident")
		return
	}
	if incident == nil {
		s.writeError(w, http.StatusNotFound, "incident not found")
		return
	}

	s.writeJSON(w, http.StatusOK, incident)
}

func (s *Server) handleAcknowledgeIncident(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	if incidentID == "" {
		s.writeError(w, http.StatusBadRequest, "incident ID required")
		return
	}

	var req struct {
		AcknowledgedBy string `json:"acknowledged_by"`
	}
	if err := s.readJSON(r, &req); err != nil {
		req.AcknowledgedBy = "api"
	}

	if err := s.svc.AcknowledgeIncident(r.Context(), incidentID, req.AcknowledgedBy); err != nil {
		s.logger.Error("acknowledge incident failed", "incident", incidentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to acknowledge incident")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "acknowledged",
		"message": "incident acknowledged",
	})
}

func (s *Server) handleResolveIncident(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	if incidentID == "" {
		s.writeError(w, http.StatusBadRequest, "incident ID required")
		return
	}

	if err := s.svc.ResolveIncident(r.Context(), incidentID); err != nil {
		s.logger.Error("resolve incident failed", "incident", incidentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to resolve incident")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "resolved",
		"message": "incident resolved",
	})
}

func (s *Server) handleAddIncidentNote(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	if incidentID == "" {
		s.writeError(w, http.StatusBadRequest, "incident ID required")
		return
	}

	var req struct {
		Note string `json:"note"`
	}
	if err := s.readJSON(r, &req); err != nil || req.Note == "" {
		s.writeError(w, http.StatusBadRequest, "note is required")
		return
	}

	if err := s.svc.AddIncidentNote(r.Context(), incidentID, req.Note); err != nil {
		s.logger.Error("add incident note failed", "incident", incidentID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to add note")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "added",
		"message": "note added to incident",
	})
}

// =============================================================================
// BASELINE ENDPOINTS
// =============================================================================

func (s *Server) handleGetTargetBaselines(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	baselines, err := s.svc.GetBaselinesForTarget(r.Context(), targetID)
	if err != nil {
		s.logger.Error("get baselines failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get baselines")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"target_id": targetID,
		"baselines": baselines,
		"count":     len(baselines),
	})
}

func (s *Server) handleGetBaseline(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agent_id")
	targetID := r.PathValue("target_id")
	if agentID == "" || targetID == "" {
		s.writeError(w, http.StatusBadRequest, "agent ID and target ID required")
		return
	}

	baseline, err := s.svc.GetBaseline(r.Context(), agentID, targetID)
	if err != nil {
		s.logger.Error("get baseline failed", "agent", agentID, "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get baseline")
		return
	}
	if baseline == nil {
		s.writeError(w, http.StatusNotFound, "baseline not found")
		return
	}

	s.writeJSON(w, http.StatusOK, baseline)
}

func (s *Server) handleRecalculateBaselines(w http.ResponseWriter, r *http.Request) {
	count, err := s.svc.RecalculateAllBaselines(r.Context())
	if err != nil {
		s.logger.Error("recalculate baselines failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to recalculate baselines")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"status":        "completed",
		"pairs_updated": count,
	})
}

// =============================================================================
// REPORT ENDPOINTS
// =============================================================================

func (s *Server) handleGetTargetReport(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if targetID == "" {
		s.writeError(w, http.StatusBadRequest, "target ID required")
		return
	}

	// Get window from query param, default to 90 days
	windowStr := r.URL.Query().Get("window")
	windowDays := 90
	switch windowStr {
	case "7d":
		windowDays = 7
	case "30d":
		windowDays = 30
	case "90d":
		windowDays = 90
	case "365d", "annual":
		windowDays = 365
	}

	report, err := s.svc.GetTargetReport(r.Context(), targetID, windowDays)
	if err != nil {
		s.logger.Error("get target report failed", "target", targetID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get target report")
		return
	}

	// Get target info
	target, _ := s.svc.GetTarget(r.Context(), targetID)

	s.writeJSON(w, http.StatusOK, map[string]any{
		"target_id":   targetID,
		"target_ip":   target.IP,
		"window_days": windowDays,
		"report":      report,
	})
}

// =============================================================================
// FLEXIBLE METRICS QUERY
// =============================================================================

func (s *Server) handleQueryMetrics(w http.ResponseWriter, r *http.Request) {
	var query types.MetricsQuery
	if err := s.readJSON(r, &query); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate the query
	if err := query.Validate(); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := s.svc.QueryMetrics(r.Context(), &query)
	if err != nil {
		s.logger.Error("metrics query failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to execute metrics query")
		return
	}

	s.writeJSON(w, http.StatusOK, result)
}

// parseInt parses a string to int, returning error if invalid.
func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid number")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
