package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/pilot-net/icmp-mon/control-plane/internal/enrollment"
)

// writeJSON writes a JSON response. (Duplicated for EnrollmentHandler to avoid Server dependency)
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": message,
	})
}

// EnrollmentHandler handles agent enrollment API requests.
type EnrollmentHandler struct {
	service *enrollment.Service
	logger  *slog.Logger
}

// NewEnrollmentHandler creates a new enrollment handler.
func NewEnrollmentHandler(service *enrollment.Service, logger *slog.Logger) *EnrollmentHandler {
	return &EnrollmentHandler{
		service: service,
		logger:  logger,
	}
}

// RegisterRoutes registers enrollment routes on the given mux.
// Enrollment routes are under /api/v1/enrollments to avoid conflicts with /api/v1/agents/{id}/*
func (h *EnrollmentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/enrollments", h.handleEnroll)
	mux.HandleFunc("GET /api/v1/enrollments", h.handleListEnrollments)
	mux.HandleFunc("GET /api/v1/enrollments/{id}", h.handleGetEnrollment)
	mux.HandleFunc("POST /api/v1/enrollments/{id}/retry", h.handleRetryEnrollment)
	mux.HandleFunc("POST /api/v1/enrollments/{id}/resume", h.handleResumeEnrollment)
	mux.HandleFunc("POST /api/v1/enrollments/{id}/cancel", h.handleCancelEnrollment)
	mux.HandleFunc("DELETE /api/v1/enrollments/{id}", h.handleCancelEnrollment)
	mux.HandleFunc("GET /api/v1/enrollments/{id}/logs", h.handleGetEnrollmentLogs)
	mux.HandleFunc("POST /api/v1/enrollments/{id}/rollback", h.handleRollback)
}

// enrollRequest is the request body for starting an enrollment.
type enrollRequest struct {
	TargetIP    string            `json:"target_ip"`
	TargetPort  int               `json:"target_port,omitempty"`
	Username    string            `json:"username"`
	Password    string            `json:"password"`
	AgentName   string            `json:"agent_name,omitempty"`
	Region      string            `json:"region,omitempty"`
	Location    string            `json:"location,omitempty"`
	Provider    string            `json:"provider,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	TryKeyFirst bool              `json:"try_key_first,omitempty"` // For re-enrollment: try SSH key auth before password
}

// handleEnroll starts a new agent enrollment with SSE streaming.
// POST /api/v1/agents/enroll
func (h *EnrollmentHandler) handleEnroll(w http.ResponseWriter, r *http.Request) {
	var req enrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if req.TargetIP == "" {
		writeError(w, http.StatusBadRequest, "target_ip is required")
		return
	}
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}

	// Default port
	port := req.TargetPort
	if port == 0 {
		port = 22
	}

	// Create enrollment request
	enrollReq := enrollment.EnrollRequest{
		TargetIP:    req.TargetIP,
		TargetPort:  port,
		Username:    req.Username,
		Password:    req.Password,
		AgentName:   req.AgentName,
		Region:      req.Region,
		Location:    req.Location,
		Provider:    req.Provider,
		Tags:        req.Tags,
		TryKeyFirst: req.TryKeyFirst,
	}

	// Check if client wants SSE streaming
	acceptHeader := r.Header.Get("Accept")
	if strings.Contains(acceptHeader, "text/event-stream") {
		h.handleEnrollSSE(w, r, enrollReq)
		return
	}

	// Non-streaming: start enrollment and return immediately
	result, err := h.service.Enroll(r.Context(), enrollReq, nil)
	if err != nil {
		h.logger.Error("enrollment failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, result)
}

// handleEnrollSSE handles enrollment with Server-Sent Events streaming.
func (h *EnrollmentHandler) handleEnrollSSE(w http.ResponseWriter, r *http.Request, req enrollment.EnrollRequest) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Create event channel - service will close it
	events := make(chan enrollment.Event, 100)

	// Use a background context so enrollment continues even if client disconnects
	// The enrollment will complete in the background
	ctx := context.Background()

	// Start enrollment in background - service manages channel lifecycle
	go func() {
		_, err := h.service.Enroll(ctx, req, events)
		if err != nil {
			h.logger.Error("enrollment failed", "error", err)
		}
	}()

	// Stream events to client
	for {
		select {
		case event, ok := <-events:
			if !ok {
				// Channel closed by service, enrollment complete
				return
			}

			// Format as SSE
			data, err := json.Marshal(event)
			if err != nil {
				h.logger.Error("failed to marshal event", "error", err)
				continue
			}

			// Write SSE format: "event: <type>\ndata: <json>\n\n"
			fmt.Fprintf(w, "event: %s\n", event.Type)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

		case <-r.Context().Done():
			// Client disconnected - enrollment continues in background
			h.logger.Info("client disconnected, enrollment continues in background")
			return
		}
	}
}

// handleListEnrollments returns all enrollments.
// GET /api/v1/agents/enrollments
func (h *EnrollmentHandler) handleListEnrollments(w http.ResponseWriter, r *http.Request) {
	enrollments, err := h.service.ListEnrollments(r.Context())
	if err != nil {
		h.logger.Error("failed to list enrollments", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enrollments": enrollments,
		"count":       len(enrollments),
	})
}

// enrollmentResponse wraps enrollment with additional computed fields.
type enrollmentResponse struct {
	*enrollment.Enrollment
	CanResume bool `json:"can_resume"`
}

// handleGetEnrollment returns a specific enrollment.
// GET /api/v1/agents/enrollments/{id}
func (h *EnrollmentHandler) handleGetEnrollment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "enrollment id required")
		return
	}

	enroll, err := h.service.GetEnrollment(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get enrollment", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if enroll == nil {
		writeError(w, http.StatusNotFound, "enrollment not found")
		return
	}

	// Include can_resume flag
	resp := enrollmentResponse{
		Enrollment: enroll,
		CanResume:  h.service.CanResume(enroll),
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleRetryEnrollment retries a failed enrollment.
// POST /api/v1/agents/enrollments/{id}/retry
func (h *EnrollmentHandler) handleRetryEnrollment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "enrollment id required")
		return
	}

	// Optional: get password from body for retry
	var body struct {
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	// Check if client wants SSE streaming
	acceptHeader := r.Header.Get("Accept")
	if strings.Contains(acceptHeader, "text/event-stream") {
		h.handleRetrySSE(w, r, id, body.Password)
		return
	}

	result, err := h.service.Retry(r.Context(), id, body.Password, nil)
	if err != nil {
		h.logger.Error("retry failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, result)
}

// handleRetrySSE handles retry with SSE streaming.
func (h *EnrollmentHandler) handleRetrySSE(w http.ResponseWriter, r *http.Request, id, password string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	events := make(chan enrollment.Event, 100)

	go func() {
		defer close(events)
		_, err := h.service.Retry(r.Context(), id, password, events)
		if err != nil {
			h.logger.Error("retry failed", "id", id, "error", err)
		}
	}()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}

			data, err := json.Marshal(event)
			if err != nil {
				continue
			}

			fmt.Fprintf(w, "event: %s\n", event.Type)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}

// handleResumeEnrollment resumes a failed enrollment using SSH key auth.
// POST /api/v1/enrollments/{id}/resume
// This can only be used if the SSH key has already been installed.
func (h *EnrollmentHandler) handleResumeEnrollment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "enrollment id required")
		return
	}

	// Check if client wants SSE streaming
	acceptHeader := r.Header.Get("Accept")
	if strings.Contains(acceptHeader, "text/event-stream") {
		h.handleResumeSSE(w, r, id)
		return
	}

	result, err := h.service.Resume(r.Context(), id, nil)
	if err != nil {
		h.logger.Error("resume failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, result)
}

// handleResumeSSE handles resume with SSE streaming.
func (h *EnrollmentHandler) handleResumeSSE(w http.ResponseWriter, r *http.Request, id string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	events := make(chan enrollment.Event, 100)

	// Use background context so enrollment continues even if client disconnects
	ctx := context.Background()

	go func() {
		_, err := h.service.Resume(ctx, id, events)
		if err != nil {
			h.logger.Error("resume failed", "id", id, "error", err)
		}
	}()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}

			data, err := json.Marshal(event)
			if err != nil {
				continue
			}

			fmt.Fprintf(w, "event: %s\n", event.Type)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

		case <-r.Context().Done():
			h.logger.Info("client disconnected, resume continues in background")
			return
		}
	}
}

// handleCancelEnrollment cancels an in-progress enrollment.
// POST /api/v1/agents/enrollments/{id}/cancel
// DELETE /api/v1/agents/enrollments/{id}
func (h *EnrollmentHandler) handleCancelEnrollment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "enrollment id required")
		return
	}

	if err := h.service.Cancel(r.Context(), id); err != nil {
		h.logger.Error("cancel failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "cancelled",
		"message": "enrollment cancelled successfully",
	})
}

// handleGetEnrollmentLogs returns logs for an enrollment.
// GET /api/v1/agents/enrollments/{id}/logs
func (h *EnrollmentHandler) handleGetEnrollmentLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "enrollment id required")
		return
	}

	enrollment, err := h.service.GetEnrollment(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get enrollment", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if enrollment == nil {
		writeError(w, http.StatusNotFound, "enrollment not found")
		return
	}

	// Get logs from store (through service)
	logs, err := h.service.GetEnrollmentLogs(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get enrollment logs", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enrollment_id": id,
		"logs":          logs,
	})
}

// rollbackRequest is the request body for rollback.
type rollbackRequest struct {
	Password string `json:"password,omitempty"`
}

// handleRollback rolls back changes from a failed enrollment.
// POST /api/v1/agents/enrollments/{id}/rollback
func (h *EnrollmentHandler) handleRollback(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "enrollment id required")
		return
	}

	var req rollbackRequest
	json.NewDecoder(r.Body).Decode(&req)

	result, err := h.service.Rollback(r.Context(), id, req.Password)
	if err != nil {
		h.logger.Error("rollback failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "rollback_complete",
		"total_changes":    result.TotalChanges,
		"reverted_changes": result.RevertedChanges,
		"failed_changes":   result.FailedChanges,
		"errors":           result.Errors,
	})
}
