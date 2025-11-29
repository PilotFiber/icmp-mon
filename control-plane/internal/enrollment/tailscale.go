package enrollment

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TailscaleConfig holds Tailscale configuration for enrollment.
type TailscaleConfig struct {
	// AuthKey is the Tailscale auth key for enrolling new machines.
	// Should be a reusable, non-ephemeral key for agents.
	AuthKey string

	// AcceptRoutes enables accepting advertised routes from other nodes.
	AcceptRoutes bool
}

// InstallTailscale installs Tailscale on the target system.
// If events is non-nil, keepalive events will be sent during the long-running install.
func InstallTailscale(ctx context.Context, client *SSHClient, info *SystemInfo, events chan<- Event) error {
	// Check if Tailscale is already installed
	_, err := client.Run(ctx, "which tailscale")
	if err == nil {
		// Tailscale already installed, check version
		version, _ := client.Run(ctx, "tailscale version")
		if strings.TrimSpace(version) != "" {
			return nil // Already installed
		}
	}

	// Use official Tailscale install script
	// This handles all distros (Ubuntu/Debian/CentOS/Fedora/etc)
	installCmd := "curl -fsSL https://tailscale.com/install.sh | sh"

	// Run with keepalive events every 3 seconds to prevent SSE timeout
	_, err = client.RunWithSudoKeepalive(ctx, fmt.Sprintf("bash -c '%s'", installCmd), events, "tailscale", 3*time.Second)
	if err != nil {
		return fmt.Errorf("installing Tailscale: %w", err)
	}

	return nil
}

// AuthenticateTailscale joins the machine to the tailnet.
func AuthenticateTailscale(ctx context.Context, client *SSHClient, config TailscaleConfig, hostname string) error {
	// Ensure tailscaled is running
	_, _ = client.RunWithSudo(ctx, "systemctl enable --now tailscaled")

	// Wait a moment for the daemon to start
	time.Sleep(2 * time.Second)

	// Build the tailscale up command
	args := []string{"tailscale", "up"}

	// Add auth key (required for non-interactive enrollment)
	args = append(args, fmt.Sprintf("--authkey=%s", config.AuthKey))

	// Set hostname to agent name for easy identification in Tailscale admin
	args = append(args, fmt.Sprintf("--hostname=%s", hostname))

	// Accept routes if configured (needed if control plane is behind subnet router)
	if config.AcceptRoutes {
		args = append(args, "--accept-routes")
	}

	// Force re-auth even if already connected (in case of stale state)
	args = append(args, "--force-reauth")

	cmd := strings.Join(args, " ")
	_, err := client.RunWithSudo(ctx, cmd)
	if err != nil {
		return fmt.Errorf("authenticating with Tailscale: %w", err)
	}

	return nil
}

// GetTailscaleIP waits for and returns the Tailscale IPv4 address.
func GetTailscaleIP(ctx context.Context, client *SSHClient) (string, error) {
	// Poll for IP assignment (up to 30 seconds)
	for i := 0; i < 30; i++ {
		output, err := client.Run(ctx, "tailscale ip -4")
		if err == nil {
			ip := strings.TrimSpace(output)
			if strings.HasPrefix(ip, "100.") {
				return ip, nil
			}
		}

		// Check context before sleeping
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
			// Continue polling
		}
	}

	return "", fmt.Errorf("timeout waiting for Tailscale IP assignment")
}

// GetTailscaleStatus returns the current Tailscale connection status.
func GetTailscaleStatus(ctx context.Context, client *SSHClient) (string, error) {
	output, err := client.Run(ctx, "tailscale status --json 2>/dev/null || tailscale status")
	if err != nil {
		return "", fmt.Errorf("getting Tailscale status: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// UninstallTailscale removes Tailscale from the system (for rollback).
func UninstallTailscale(ctx context.Context, client *SSHClient, info *SystemInfo) error {
	// Logout first to clean up auth state
	_, _ = client.RunWithSudo(ctx, "tailscale logout")

	// Stop and disable the service
	_, _ = client.RunWithSudo(ctx, "systemctl stop tailscaled")
	_, _ = client.RunWithSudo(ctx, "systemctl disable tailscaled")

	// Remove the package based on package manager
	var removeCmd string
	switch info.PackageManager {
	case "apt":
		removeCmd = "apt-get remove -y tailscale && apt-get autoremove -y"
	case "dnf":
		removeCmd = "dnf remove -y tailscale"
	case "yum":
		removeCmd = "yum remove -y tailscale"
	default:
		removeCmd = "apt-get remove -y tailscale 2>/dev/null || yum remove -y tailscale 2>/dev/null || dnf remove -y tailscale 2>/dev/null"
	}

	_, err := client.RunWithSudo(ctx, removeCmd)
	return err
}

// IsTailscaleInstalled checks if Tailscale is installed on the system.
func IsTailscaleInstalled(ctx context.Context, client *SSHClient) bool {
	_, err := client.Run(ctx, "which tailscale")
	return err == nil
}

// IsTailscaleConnected checks if Tailscale is connected to a tailnet.
func IsTailscaleConnected(ctx context.Context, client *SSHClient) bool {
	output, err := client.Run(ctx, "tailscale status --self --json 2>/dev/null | grep -q '\"Online\":true'")
	return err == nil && output == ""
}
