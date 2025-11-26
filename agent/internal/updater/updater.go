// Package updater provides self-update functionality for the agent.
//
// The updater downloads new agent binaries from the control plane,
// verifies their checksums, and performs an atomic update using
// a symlink swap strategy for zero-downtime upgrades.
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/pilot-net/icmp-mon/pkg/types"
)

// Updater handles agent self-updates.
type Updater struct {
	installDir   string // Directory containing agent binaries
	binaryName   string // Name of the agent binary
	currentPath  string // Path to current binary (may be symlink)
	logger       *slog.Logger
	httpClient   *http.Client

	mu        sync.Mutex
	updating  bool
	lastError error
}

// Config contains updater configuration.
type Config struct {
	// InstallDir is the directory for agent binaries (default: /usr/local/bin)
	InstallDir string

	// BinaryName is the agent binary name (default: icmpmon-agent)
	BinaryName string

	// Logger for updater logs
	Logger *slog.Logger
}

// New creates a new updater.
func New(cfg Config) *Updater {
	installDir := cfg.InstallDir
	if installDir == "" {
		installDir = "/usr/local/bin"
	}

	binaryName := cfg.BinaryName
	if binaryName == "" {
		binaryName = "icmpmon-agent"
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Updater{
		installDir:  installDir,
		binaryName:  binaryName,
		currentPath: filepath.Join(installDir, binaryName),
		logger:      logger,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Binary downloads can take a while
		},
	}
}

// Update downloads and installs a new agent version.
// It returns nil if the update was successful.
// The caller should request a restart after a successful update.
func (u *Updater) Update(ctx context.Context, info *types.UpdateInfo) error {
	u.mu.Lock()
	if u.updating {
		u.mu.Unlock()
		return fmt.Errorf("update already in progress")
	}
	u.updating = true
	u.mu.Unlock()

	defer func() {
		u.mu.Lock()
		u.updating = false
		u.mu.Unlock()
	}()

	u.logger.Info("starting update",
		"version", info.Version,
		"url", info.DownloadURL,
		"size", info.Size)

	// Create temp directory for download
	tmpDir, err := os.MkdirTemp("", "icmpmon-update-*")
	if err != nil {
		u.lastError = fmt.Errorf("creating temp dir: %w", err)
		return u.lastError
	}
	defer os.RemoveAll(tmpDir)

	// Download new binary
	tmpPath := filepath.Join(tmpDir, u.binaryName)
	if err := u.downloadBinary(ctx, info.DownloadURL, tmpPath); err != nil {
		u.lastError = fmt.Errorf("downloading binary: %w", err)
		return u.lastError
	}

	// Verify checksum
	if err := u.verifyChecksum(tmpPath, info.Checksum); err != nil {
		u.lastError = fmt.Errorf("checksum verification: %w", err)
		return u.lastError
	}

	// Verify it's executable and has correct format
	if err := u.verifyBinary(ctx, tmpPath); err != nil {
		u.lastError = fmt.Errorf("binary verification: %w", err)
		return u.lastError
	}

	// Perform atomic update
	if err := u.atomicUpdate(tmpPath, info.Version); err != nil {
		u.lastError = fmt.Errorf("atomic update: %w", err)
		return u.lastError
	}

	u.logger.Info("update installed successfully",
		"version", info.Version)

	return nil
}

// downloadBinary downloads the binary from the given URL.
func (u *Updater) downloadBinary(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// Create destination file
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	// Download with progress logging
	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("writing binary: %w", err)
	}

	u.logger.Debug("download complete", "bytes", written)

	return nil
}

// verifyChecksum verifies the SHA256 checksum of the downloaded binary.
func (u *Updater) verifyChecksum(path, expectedChecksum string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actualChecksum := hex.EncodeToString(h.Sum(nil))

	// Handle "sha256:" prefix
	expected := strings.TrimPrefix(expectedChecksum, "sha256:")

	if !strings.EqualFold(actualChecksum, expected) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actualChecksum)
	}

	u.logger.Debug("checksum verified", "checksum", actualChecksum)

	return nil
}

// verifyBinary runs basic verification on the downloaded binary.
func (u *Updater) verifyBinary(ctx context.Context, path string) error {
	// Make sure it's executable
	if err := os.Chmod(path, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	// Try to run with --version to verify it's a valid binary
	cmd := exec.CommandContext(ctx, path, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("binary test failed: %w (output: %s)", err, string(output))
	}

	u.logger.Debug("binary verification passed", "output", strings.TrimSpace(string(output)))

	return nil
}

// atomicUpdate performs an atomic update using symlink swap.
// This ensures zero-downtime updates - the old binary continues running
// while the new binary is installed, and systemd will start the new
// binary on restart.
func (u *Updater) atomicUpdate(newBinaryPath, version string) error {
	// Versioned binary path: icmpmon-agent-v1.2.3
	versionedName := fmt.Sprintf("%s-v%s", u.binaryName, version)
	versionedPath := filepath.Join(u.installDir, versionedName)

	// Copy new binary to versioned path
	if err := u.copyFile(newBinaryPath, versionedPath); err != nil {
		return fmt.Errorf("copying to versioned path: %w", err)
	}

	// Set capabilities for ICMP (if setcap is available)
	if err := u.setCapabilities(versionedPath); err != nil {
		u.logger.Warn("failed to set capabilities, may need root for ICMP", "error", err)
	}

	// Check if current path is a symlink
	linkTarget, err := os.Readlink(u.currentPath)
	isSymlink := err == nil

	if isSymlink {
		// Already using symlinks - do atomic swap
		u.logger.Debug("using symlink swap", "current_target", linkTarget)

		// Create new symlink with temp name
		tmpLink := u.currentPath + ".new"
		os.Remove(tmpLink) // Remove if exists

		if err := os.Symlink(versionedPath, tmpLink); err != nil {
			return fmt.Errorf("creating new symlink: %w", err)
		}

		// Atomic rename
		if err := os.Rename(tmpLink, u.currentPath); err != nil {
			os.Remove(tmpLink)
			return fmt.Errorf("atomic rename: %w", err)
		}

		// Clean up old version (optional, keep for rollback)
		oldVersionedPath := linkTarget
		u.logger.Debug("old version preserved for rollback", "path", oldVersionedPath)

	} else {
		// First time or not using symlinks - need to replace directly
		u.logger.Debug("converting to symlink-based updates")

		// Backup current binary
		backupPath := u.currentPath + ".backup"
		if err := os.Rename(u.currentPath, backupPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("backing up current binary: %w", err)
		}

		// Create symlink to new version
		if err := os.Symlink(versionedPath, u.currentPath); err != nil {
			// Try to restore backup
			os.Rename(backupPath, u.currentPath)
			return fmt.Errorf("creating symlink: %w", err)
		}

		// Remove backup
		os.Remove(backupPath)
	}

	u.logger.Info("atomic update complete",
		"version", version,
		"binary", versionedPath)

	return nil
}

// copyFile copies a file from src to dst.
func (u *Updater) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// setCapabilities sets CAP_NET_RAW on the binary for ICMP ping.
func (u *Updater) setCapabilities(path string) error {
	if runtime.GOOS != "linux" {
		return nil // Only needed on Linux
	}

	cmd := exec.Command("setcap", "cap_net_raw=ep", path)
	return cmd.Run()
}

// RequestRestart signals systemd to restart the agent service.
func (u *Updater) RequestRestart() error {
	u.logger.Info("requesting service restart")

	// Try systemctl first (most common)
	cmd := exec.Command("systemctl", "restart", "icmpmon-agent")
	if err := cmd.Run(); err != nil {
		// Try self-restart via process signal
		u.logger.Warn("systemctl restart failed, agent will continue with old version until manual restart", "error", err)
		return err
	}

	return nil
}

// IsUpdating returns true if an update is currently in progress.
func (u *Updater) IsUpdating() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.updating
}

// LastError returns the last update error, if any.
func (u *Updater) LastError() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lastError
}

// Rollback reverts to the previous version by swapping the symlink.
func (u *Updater) Rollback(previousVersion string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	previousPath := filepath.Join(u.installDir, fmt.Sprintf("%s-v%s", u.binaryName, previousVersion))

	// Check if previous version exists
	if _, err := os.Stat(previousPath); os.IsNotExist(err) {
		return fmt.Errorf("previous version not found: %s", previousVersion)
	}

	// Create new symlink
	tmpLink := u.currentPath + ".rollback"
	os.Remove(tmpLink)

	if err := os.Symlink(previousPath, tmpLink); err != nil {
		return fmt.Errorf("creating rollback symlink: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpLink, u.currentPath); err != nil {
		os.Remove(tmpLink)
		return fmt.Errorf("rollback rename: %w", err)
	}

	u.logger.Info("rollback complete", "version", previousVersion)

	return nil
}
