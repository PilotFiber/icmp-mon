// Package api - Assignment management endpoints
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/pilot-net/icmp-mon/control-plane/internal/service"
)

// AssignmentHandler handles assignment management endpoints.
type AssignmentHandler struct {
	rebalancer *service.Rebalancer
	logger     *slog.Logger

	// Track running materialization
	mu              sync.Mutex
	isRunning       bool
	lastStarted     time.Time
	lastCompleted   time.Time
	lastError       string
	lastAssignments int
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
	mux.HandleFunc("GET /api/v1/assignments/status", h.handleStatus)
}

// handleStatus returns the current status of assignment materialization.
func (h *AssignmentHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	status := map[string]any{
		"is_running":       h.isRunning,
		"last_started":     h.lastStarted,
		"last_completed":   h.lastCompleted,
		"last_error":       h.lastError,
		"last_assignments": h.lastAssignments,
	}
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleMaterialize computes and persists all target-to-agent assignments.
// This runs asynchronously and returns immediately.
func (h *AssignmentHandler) handleMaterialize(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	if h.isRunning {
		h.mu.Unlock()
		h.writeError(w, http.StatusConflict, "materialization already in progress")
		return
	}
	h.isRunning = true
	h.lastStarted = time.Now()
	h.lastError = ""
	h.mu.Unlock()

	h.logger.Info("starting assignment materialization (async)")

	// Run in background
	go func() {
		// Use a background context so it doesn't get cancelled when request ends
		ctx := context.Background()

		assignments, err := h.rebalancer.MaterializeAllAssignmentsWithCount(ctx)

		h.mu.Lock()
		h.isRunning = false
		h.lastCompleted = time.Now()
		h.lastAssignments = assignments
		if err != nil {
			h.lastError = err.Error()
			h.logger.Error("materialization failed", "error", err, "duration", time.Since(h.lastStarted))
		} else {
			h.logger.Info("materialization complete",
				"assignments", assignments,
				"duration", time.Since(h.lastStarted),
			)
		}
		h.mu.Unlock()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "started",
		"message": "assignment materialization started in background",
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
