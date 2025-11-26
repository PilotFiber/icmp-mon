// Package api - Assignment management endpoints
package api

import (
	"log/slog"
	"net/http"

	"github.com/pilot-net/icmp-mon/control-plane/internal/service"
)

// AssignmentHandler handles assignment management endpoints.
type AssignmentHandler struct {
	rebalancer *service.Rebalancer
	logger     *slog.Logger
}

// NewAssignmentHandler creates a new assignment handler.
func NewAssignmentHandler(rebalancer *service.Rebalancer, logger *slog.Logger) *AssignmentHandler {
	return &AssignmentHandler{
		rebalancer: rebalancer,
		logger:     logger.With("handler", "assignments"),
	}
}

// RegisterRoutes registers the assignment management routes.
func (h *AssignmentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/assignments/materialize", h.handleMaterialize)
}

// handleMaterialize computes and persists all target-to-agent assignments.
// This is typically used for initial population or full recomputation.
func (h *AssignmentHandler) handleMaterialize(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("materializing all assignments")

	if err := h.rebalancer.MaterializeAllAssignments(r.Context()); err != nil {
		h.logger.Error("materialization failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.logger.Info("assignment materialization complete")
	h.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "completed",
		"message": "all assignments have been materialized",
	})
}

func (h *AssignmentHandler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Simple JSON encoding
	if m, ok := v.(map[string]string); ok {
		w.Write([]byte(`{"status":"` + m["status"] + `","message":"` + m["message"] + `"}`))
	}
}

func (h *AssignmentHandler) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(`{"error":"` + message + `"}`))
}
