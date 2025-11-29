package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

// Binary storage location in container
const binariesDir = "/opt/icmpmon/binaries"

// Valid platform pattern: agent-linux-amd64, agent-linux-arm64
var validPlatformPattern = regexp.MustCompile(`^agent-linux-(amd64|arm64)$`)

// handleGetPackage serves agent binaries for download during enrollment.
// GET /api/v1/packages/{platform}
// where platform is "agent-linux-amd64" or "agent-linux-arm64"
func (s *Server) handleGetPackage(w http.ResponseWriter, r *http.Request) {
	platform := r.PathValue("platform")

	// Validate platform to prevent path traversal
	// Expected format: agent-linux-amd64 or agent-linux-arm64
	if !validPlatformPattern.MatchString(platform) {
		s.writeError(w, http.StatusBadRequest, "invalid platform: must be agent-linux-amd64 or agent-linux-arm64")
		return
	}

	// Binary filename is the platform value (agent-linux-amd64 -> agent-linux-amd64)
	binaryPath := filepath.Join(binariesDir, platform)

	// Check if binary exists
	info, err := os.Stat(binaryPath)
	if os.IsNotExist(err) {
		s.logger.Warn("agent binary not found", "platform", platform, "path", binaryPath)
		s.writeError(w, http.StatusNotFound, fmt.Sprintf("agent binary for platform %s not found", platform))
		return
	}
	if err != nil {
		s.logger.Error("failed to stat binary", "platform", platform, "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to access binary")
		return
	}

	// Set headers for binary download
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=icmpmon-agent-%s", platform))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

	// Serve the file
	http.ServeFile(w, r, binaryPath)

	s.logger.Info("served agent binary", "platform", platform, "size", info.Size())
}
