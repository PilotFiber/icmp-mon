package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/pilot-net/icmp-mon/control-plane/internal/rollout"
)

// RolloutHandler handles agent rollout API requests.
type RolloutHandler struct {
	engine *rollout.Engine
	logger *slog.Logger
}

// NewRolloutHandler creates a new rollout handler.
func NewRolloutHandler(engine *rollout.Engine, logger *slog.Logger) *RolloutHandler {
	return &RolloutHandler{
		engine: engine,
		logger: logger,
	}
}

// RegisterRoutes registers rollout routes on the given mux.
func (h *RolloutHandler) RegisterRoutes(mux *http.ServeMux) {
	// Releases
	mux.HandleFunc("GET /api/v1/releases", h.handleListReleases)
	mux.HandleFunc("POST /api/v1/releases", h.handleCreateRelease)
	mux.HandleFunc("GET /api/v1/releases/{id}", h.handleGetRelease)
	mux.HandleFunc("POST /api/v1/releases/{id}/publish", h.handlePublishRelease)
	mux.HandleFunc("GET /api/v1/releases/{id}/download", h.handleDownloadRelease)

	// Rollouts
	mux.HandleFunc("GET /api/v1/rollouts", h.handleListRollouts)
	mux.HandleFunc("POST /api/v1/rollouts", h.handleCreateRollout)
	mux.HandleFunc("GET /api/v1/rollouts/{id}", h.handleGetRollout)
	mux.HandleFunc("GET /api/v1/rollouts/{id}/progress", h.handleGetRolloutProgress)
	mux.HandleFunc("POST /api/v1/rollouts/{id}/pause", h.handlePauseRollout)
	mux.HandleFunc("POST /api/v1/rollouts/{id}/resume", h.handleResumeRollout)
	mux.HandleFunc("POST /api/v1/rollouts/{id}/rollback", h.handleRollbackRollout)

	// Fleet version info
	mux.HandleFunc("GET /api/v1/fleet/versions", h.handleFleetVersions)
}

// createRolloutRequest is the request body for starting a rollout.
type createRolloutRequest struct {
	ReleaseID string         `json:"release_id"`
	Strategy  string         `json:"strategy,omitempty"`
	Config    *rollout.Config `json:"config,omitempty"`
}

// handleListReleases returns all releases.
// GET /api/v1/releases
func (h *RolloutHandler) handleListReleases(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement with store
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"releases": []interface{}{},
	})
}

// handleCreateRelease creates a new release.
// POST /api/v1/releases
func (h *RolloutHandler) handleCreateRelease(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement with store and binary upload
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error": "release upload not yet implemented",
	})
}

// handleGetRelease returns release details.
// GET /api/v1/releases/{id}
func (h *RolloutHandler) handleGetRelease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "release id required")
		return
	}

	// TODO: Implement with store
	writeJSON(w, http.StatusNotFound, map[string]string{
		"error": "release not found",
	})
}

// handlePublishRelease publishes a release.
// POST /api/v1/releases/{id}/publish
func (h *RolloutHandler) handlePublishRelease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "release id required")
		return
	}

	// TODO: Implement with store
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "published",
	})
}

// handleDownloadRelease serves the release binary.
// GET /api/v1/releases/{id}/download
func (h *RolloutHandler) handleDownloadRelease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "release id required")
		return
	}

	// TODO: Implement binary serving
	writeError(w, http.StatusNotFound, "release not found")
}

// handleListRollouts returns all rollouts.
// GET /api/v1/rollouts
func (h *RolloutHandler) handleListRollouts(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement with store
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"rollouts": []interface{}{},
	})
}

// handleCreateRollout starts a new rollout.
// POST /api/v1/rollouts
func (h *RolloutHandler) handleCreateRollout(w http.ResponseWriter, r *http.Request) {
	var req createRolloutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.ReleaseID == "" {
		writeError(w, http.StatusBadRequest, "release_id is required")
		return
	}

	cfg := rollout.DefaultConfig()
	if req.Config != nil {
		cfg = *req.Config
	}
	if req.Strategy != "" {
		cfg.Strategy = rollout.Strategy(req.Strategy)
	}

	result, err := h.engine.StartRollout(r.Context(), req.ReleaseID, cfg, "api")
	if err != nil {
		h.logger.Error("failed to start rollout", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

// handleGetRollout returns rollout details.
// GET /api/v1/rollouts/{id}
func (h *RolloutHandler) handleGetRollout(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "rollout id required")
		return
	}

	result, err := h.engine.GetRollout(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get rollout", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if result == nil {
		writeError(w, http.StatusNotFound, "rollout not found")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleGetRolloutProgress returns detailed progress for a rollout.
// GET /api/v1/rollouts/{id}/progress
func (h *RolloutHandler) handleGetRolloutProgress(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "rollout id required")
		return
	}

	progress, err := h.engine.GetRolloutProgress(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get rollout progress", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"rollout_id": id,
		"agents":     progress,
	})
}

// handlePauseRollout pauses an active rollout.
// POST /api/v1/rollouts/{id}/pause
func (h *RolloutHandler) handlePauseRollout(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "rollout id required")
		return
	}

	if err := h.engine.PauseRollout(r.Context(), id); err != nil {
		h.logger.Error("failed to pause rollout", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "paused",
	})
}

// handleResumeRollout resumes a paused rollout.
// POST /api/v1/rollouts/{id}/resume
func (h *RolloutHandler) handleResumeRollout(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "rollout id required")
		return
	}

	if err := h.engine.ResumeRollout(r.Context(), id); err != nil {
		h.logger.Error("failed to resume rollout", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "resumed",
	})
}

// rolloutRollbackRequest is the request body for rollout rollback.
type rolloutRollbackRequest struct {
	Reason string `json:"reason,omitempty"`
}

// handleRollbackRollout stops and rolls back a rollout.
// POST /api/v1/rollouts/{id}/rollback
func (h *RolloutHandler) handleRollbackRollout(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "rollout id required")
		return
	}

	var req rolloutRollbackRequest
	json.NewDecoder(r.Body).Decode(&req)

	reason := req.Reason
	if reason == "" {
		reason = "manual rollback"
	}

	if err := h.engine.RollbackRollout(r.Context(), id, reason); err != nil {
		h.logger.Error("failed to rollback", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "rolled_back",
	})
}

// handleFleetVersions returns version distribution across the fleet.
// GET /api/v1/fleet/versions
func (h *RolloutHandler) handleFleetVersions(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement with store query
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"versions": map[string]int{},
		"total":    0,
	})
}
