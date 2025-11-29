// Package api provides HTTP handlers for the control plane.
package api

import (
	"log/slog"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// AgentAuthConfig controls agent authentication behavior.
type AgentAuthConfig struct {
	// Enabled controls whether authentication is enforced.
	// When false, authentication is checked but not required (grace period mode).
	Enabled bool

	// Logger for authentication events.
	Logger *slog.Logger
}

// AgentAuthMiddleware creates middleware that validates agent API keys.
// During the grace period (Enabled=false), it logs but doesn't reject unauthenticated requests.
func (s *Server) AgentAuthMiddleware(config AgentAuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for registration endpoint - agents don't have keys yet
			if r.URL.Path == "/api/v1/agents/register" {
				next.ServeHTTP(w, r)
				return
			}

			// Extract agent ID and API key
			agentID := r.Header.Get("X-Agent-ID")
			authHeader := r.Header.Get("Authorization")

			// Check if auth headers are present
			if agentID == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				if config.Enabled {
					config.Logger.Warn("agent auth failed: missing credentials",
						"path", r.URL.Path,
						"agent_id", agentID,
						"has_auth_header", authHeader != "",
					)
					http.Error(w, "unauthorized: missing credentials", http.StatusUnauthorized)
					return
				}
				// Grace period: log but allow
				config.Logger.Debug("agent auth: missing credentials (grace period)",
					"path", r.URL.Path,
					"agent_id", agentID,
				)
				next.ServeHTTP(w, r)
				return
			}

			apiKey := strings.TrimPrefix(authHeader, "Bearer ")

			// Look up expected hash from database
			expectedHash, err := s.svc.Store().GetAgentAPIKeyHash(r.Context(), agentID)
			if err != nil {
				config.Logger.Error("agent auth failed: database error",
					"agent_id", agentID,
					"error", err,
				)
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}

			// No key set for this agent
			if expectedHash == "" {
				if config.Enabled {
					config.Logger.Warn("agent auth failed: no API key configured",
						"agent_id", agentID,
						"path", r.URL.Path,
					)
					http.Error(w, "unauthorized: no API key configured", http.StatusUnauthorized)
					return
				}
				// Grace period: log but allow
				config.Logger.Debug("agent auth: no API key configured (grace period)",
					"agent_id", agentID,
				)
				next.ServeHTTP(w, r)
				return
			}

			// Verify the key hash
			if err := bcrypt.CompareHashAndPassword([]byte(expectedHash), []byte(apiKey)); err != nil {
				if config.Enabled {
					config.Logger.Warn("agent auth failed: invalid API key",
						"agent_id", agentID,
						"path", r.URL.Path,
					)
					http.Error(w, "unauthorized: invalid API key", http.StatusUnauthorized)
					return
				}
				// Grace period: log but allow
				config.Logger.Warn("agent auth: invalid API key (grace period - would reject)",
					"agent_id", agentID,
				)
				next.ServeHTTP(w, r)
				return
			}

			// Authentication successful
			config.Logger.Debug("agent auth successful",
				"agent_id", agentID,
				"path", r.URL.Path,
			)
			next.ServeHTTP(w, r)
		})
	}
}

// wrapHandler converts an http.HandlerFunc to use middleware.
func wrapHandler(h http.HandlerFunc, middleware func(http.Handler) http.Handler) http.HandlerFunc {
	return middleware(h).ServeHTTP
}
