package enrollment

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// sshSession holds the active SSH connection during enrollment.
// It's stored in the service to be reused across steps.
type sshSession struct {
	client     *SSHClient
	systemInfo *SystemInfo
}

// activeSessions tracks active SSH sessions by enrollment ID.
var activeSessions = make(map[string]*sshSession)

// stepConnect establishes the initial SSH connection.
// If TryKeyFirst is set on enrollment, it attempts SSH key auth before falling back to password.
func (s *Service) stepConnect(ctx context.Context, enrollment *Enrollment, password string, events chan<- Event) error {
	events <- Event{
		Type:      "log",
		Step:      "connecting",
		Message:   fmt.Sprintf("Connecting to %s:%d as %s", enrollment.TargetIP, enrollment.TargetPort, enrollment.Username),
		Timestamp: time.Now(),
	}

	var client *SSHClient
	var err error

	// For re-enrollment, try SSH key first (server may already be hardened)
	if enrollment.TryKeyFirst {
		events <- Event{
			Type:      "log",
			Step:      "connecting",
			Message:   "Attempting SSH key authentication (re-enrollment mode)",
			Timestamp: time.Now(),
		}

		// Get the provisioning key
		keyPair, keyErr := s.keyStore.GetOrCreateProvisioningKey(ctx)
		if keyErr == nil {
			privateKey, keyErr := s.keyStore.GetPrivateKey(ctx, keyPair.Name)
			if keyErr == nil {
				// Try key-based auth
				client, err = ConnectSSH(ctx, SSHConfig{
					Host:       enrollment.TargetIP,
					Port:       enrollment.TargetPort,
					Username:   enrollment.Username,
					PrivateKey: privateKey,
					Timeout:    30 * time.Second,
				})
				if err == nil {
					events <- Event{
						Type:      "log",
						Step:      "connecting",
						Message:   "SSH key authentication successful",
						Timestamp: time.Now(),
					}
					// Mark key_installing as already done (since key works, it's installed)
					if !contains(enrollment.StepsCompleted, "key_installing") {
						enrollment.StepsCompleted = append(enrollment.StepsCompleted, "key_installing")
					}
					// Also mark hardening as done (key-only auth means server is hardened)
					if !contains(enrollment.StepsCompleted, "hardening") {
						enrollment.StepsCompleted = append(enrollment.StepsCompleted, "hardening")
					}
				} else {
					events <- Event{
						Type:      "log",
						Step:      "connecting",
						Message:   fmt.Sprintf("SSH key auth failed (%v), falling back to password", err),
						Timestamp: time.Now(),
					}
				}
			}
		}
	}

	// If no client yet (key auth not tried or failed), use password
	if client == nil {
		client, err = ConnectSSH(ctx, SSHConfig{
			Host:     enrollment.TargetIP,
			Port:     enrollment.TargetPort,
			Username: enrollment.Username,
			Password: password,
			Timeout:  30 * time.Second,
		})
		if err != nil {
			return fmt.Errorf("SSH connection failed: %w", err)
		}
	}

	// Store session for subsequent steps
	activeSessions[enrollment.ID] = &sshSession{client: client}

	events <- Event{
		Type:      "log",
		Step:      "connecting",
		Message:   "SSH connection established",
		Timestamp: time.Now(),
	}

	// Log the connection event
	s.store.AddEnrollmentLog(ctx, enrollment.ID, "connecting", "info",
		fmt.Sprintf("Connected to %s:%d", enrollment.TargetIP, enrollment.TargetPort), nil)

	return nil
}

// stepDetect detects the system's OS, architecture, and capabilities.
func (s *Service) stepDetect(ctx context.Context, enrollment *Enrollment, password string, events chan<- Event) error {
	session, ok := activeSessions[enrollment.ID]
	if !ok {
		return fmt.Errorf("no active SSH session")
	}

	events <- Event{
		Type:      "log",
		Step:      "detecting",
		Message:   "Detecting system information",
		Timestamp: time.Now(),
	}

	// Detect system info
	info, err := DetectSystem(ctx, session.client)
	if err != nil {
		return fmt.Errorf("system detection failed: %w", err)
	}
	session.systemInfo = info

	// Validate detected info
	if info.OS == "" {
		return fmt.Errorf("could not detect operating system")
	}
	if info.Arch == "" {
		return fmt.Errorf("could not detect architecture")
	}

	// Update enrollment with detection results
	enrollment.DetectedOS = info.OS
	enrollment.DetectedOSVersion = info.OSVersion
	enrollment.DetectedArch = info.Arch
	enrollment.DetectedHostname = info.Hostname
	enrollment.DetectedPackageManager = info.PackageManager

	// Use hostname as agent name if not provided
	if enrollment.AgentName == "" || enrollment.AgentName == fmt.Sprintf("agent-%s", enrollment.TargetIP) {
		if info.Hostname != "" {
			enrollment.AgentName = info.Hostname
		}
	}

	// If a custom agent name was provided and it differs from the current hostname, set it
	if enrollment.AgentName != "" && enrollment.AgentName != info.Hostname {
		events <- Event{
			Type:      "log",
			Step:      "detecting",
			Message:   fmt.Sprintf("Setting hostname to %s", enrollment.AgentName),
			Timestamp: time.Now(),
		}

		// Set hostname using hostnamectl (works on systemd systems)
		_, err := session.client.RunWithSudo(ctx, fmt.Sprintf("hostnamectl set-hostname %s", enrollment.AgentName))
		if err != nil {
			// Try fallback method
			_, err = session.client.RunWithSudo(ctx, fmt.Sprintf("hostname %s && echo %s > /etc/hostname", enrollment.AgentName, enrollment.AgentName))
			if err != nil {
				events <- Event{
					Type:      "log",
					Step:      "detecting",
					Message:   fmt.Sprintf("Warning: could not set hostname: %v", err),
					Timestamp: time.Now(),
				}
			}
		}

		// Update /etc/hosts to include the new hostname
		_, _ = session.client.RunWithSudo(ctx, fmt.Sprintf("sed -i 's/127.0.1.1.*/127.0.1.1\\t%s/' /etc/hosts || echo '127.0.1.1\t%s' >> /etc/hosts", enrollment.AgentName, enrollment.AgentName))

		// Track this change for rollback
		enrollment.Changes = append(enrollment.Changes, Change{
			Type:        "hostname_changed",
			Description: fmt.Sprintf("Changed hostname from %s to %s", info.Hostname, enrollment.AgentName),
			Revertible:  true,
			RevertCmd:   fmt.Sprintf("hostnamectl set-hostname %s", info.Hostname),
			Timestamp:   time.Now(),
		})
	}

	events <- Event{
		Type: "log",
		Step: "detecting",
		Message: fmt.Sprintf("Detected: %s %s (%s), hostname: %s",
			info.OS, info.OSVersion, info.Arch, enrollment.AgentName),
		Details: map[string]string{
			"os":              info.OS,
			"os_version":      info.OSVersion,
			"arch":            info.Arch,
			"hostname":        info.Hostname,
			"package_manager": info.PackageManager,
			"init_system":     info.InitSystem,
		},
		Timestamp: time.Now(),
	}

	// Check for required capabilities
	if info.InitSystem != "systemd" {
		return fmt.Errorf("unsupported init system: %s (only systemd is supported)", info.InitSystem)
	}

	if !info.HasCurl && !info.HasWget {
		return fmt.Errorf("neither curl nor wget is available on the system")
	}

	// Verify sudo access
	if err := VerifySudoAccess(ctx, session.client); err != nil {
		// Try to set up passwordless sudo for the user
		events <- Event{
			Type:      "log",
			Step:      "detecting",
			Message:   "Setting up passwordless sudo for enrollment",
			Timestamp: time.Now(),
		}
	}

	// Verify connectivity to control plane
	// Skip if Tailscale is enabled - connectivity will be verified after Tailscale setup
	if !s.TailscaleEnabled() {
		events <- Event{
			Type:      "log",
			Step:      "detecting",
			Message:   "Verifying connectivity to control plane",
			Timestamp: time.Now(),
		}

		if err := VerifyConnectivity(ctx, session.client, enrollment.ControlPlaneURL); err != nil {
			return fmt.Errorf("control plane connectivity check failed: %w", err)
		}
	} else {
		events <- Event{
			Type:      "log",
			Step:      "detecting",
			Message:   "Skipping connectivity check (will verify after Tailscale setup)",
			Timestamp: time.Now(),
		}
	}

	s.store.AddEnrollmentLog(ctx, enrollment.ID, "detecting", "info",
		fmt.Sprintf("Detected %s %s (%s)", info.OS, info.OSVersion, info.Arch), nil)

	return nil
}

// stepInstallKey installs the control plane's SSH public key.
func (s *Service) stepInstallKey(ctx context.Context, enrollment *Enrollment, password string, events chan<- Event) error {
	session, ok := activeSessions[enrollment.ID]
	if !ok {
		return fmt.Errorf("no active SSH session")
	}

	events <- Event{
		Type:      "log",
		Step:      "key_installing",
		Message:   "Installing SSH public key",
		Timestamp: time.Now(),
	}

	// Get the provisioning key
	keyPair, err := s.keyStore.GetOrCreateProvisioningKey(ctx)
	if err != nil {
		return fmt.Errorf("getting provisioning key: %w", err)
	}

	enrollment.SSHKeyID = keyPair.ID

	// Check if key is already installed
	output, err := session.client.Run(ctx, "cat ~/.ssh/authorized_keys 2>/dev/null || echo ''")
	if err == nil && strings.Contains(output, strings.TrimSpace(keyPair.PublicKey)) {
		events <- Event{
			Type:      "log",
			Step:      "key_installing",
			Message:   "SSH key already installed",
			Timestamp: time.Now(),
		}
		return nil
	}

	// Create .ssh directory
	_, err = session.client.Run(ctx, "mkdir -p ~/.ssh && chmod 700 ~/.ssh")
	if err != nil {
		return fmt.Errorf("creating .ssh directory: %w", err)
	}

	// Append public key to authorized_keys
	pubKey := strings.TrimSpace(keyPair.PublicKey)
	if !strings.HasSuffix(pubKey, " icmpmon-control-plane") {
		pubKey = pubKey + " icmpmon-control-plane"
	}

	err = session.client.AppendToFile(ctx, "~/.ssh/authorized_keys", pubKey)
	if err != nil {
		return fmt.Errorf("adding key to authorized_keys: %w", err)
	}

	// Set permissions
	_, err = session.client.Run(ctx, "chmod 600 ~/.ssh/authorized_keys")
	if err != nil {
		return fmt.Errorf("setting authorized_keys permissions: %w", err)
	}

	// Track change for rollback
	enrollment.Changes = append(enrollment.Changes, Change{
		Type:        "ssh_key_added",
		Description: "Added control plane SSH key to authorized_keys",
		Revertible:  true,
		RevertCmd:   fmt.Sprintf("sed -i '/icmpmon-control-plane$/d' ~/.ssh/authorized_keys"),
		Timestamp:   time.Now(),
	})

	events <- Event{
		Type:      "log",
		Step:      "key_installing",
		Message:   fmt.Sprintf("SSH key installed (fingerprint: %s)", keyPair.Fingerprint),
		Timestamp: time.Now(),
	}

	// Verify key authentication works
	events <- Event{
		Type:      "log",
		Step:      "key_installing",
		Message:   "Verifying key authentication",
		Timestamp: time.Now(),
	}

	privateKey, err := s.keyStore.GetPrivateKey(ctx, keyPair.Name)
	if err != nil {
		return fmt.Errorf("getting private key: %w", err)
	}

	// Connect using key to verify
	keyClient, err := ConnectSSH(ctx, SSHConfig{
		Host:       enrollment.TargetIP,
		Port:       enrollment.TargetPort,
		Username:   enrollment.Username,
		PrivateKey: privateKey,
		Timeout:    10 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("key authentication verification failed: %w", err)
	}
	keyClient.Close()

	events <- Event{
		Type:      "log",
		Step:      "key_installing",
		Message:   "Key authentication verified",
		Timestamp: time.Now(),
	}

	s.store.AddEnrollmentLog(ctx, enrollment.ID, "key_installing", "info",
		fmt.Sprintf("SSH key installed, fingerprint: %s", keyPair.Fingerprint), nil)

	return nil
}

// stepHarden hardens SSH by disabling password authentication.
// Root login is allowed but only via SSH key authentication.
func (s *Service) stepHarden(ctx context.Context, enrollment *Enrollment, password string, events chan<- Event) error {
	session, ok := activeSessions[enrollment.ID]
	if !ok {
		return fmt.Errorf("no active SSH session")
	}

	events <- Event{
		Type:      "log",
		Step:      "hardening",
		Message:   "Hardening SSH configuration",
		Timestamp: time.Now(),
	}

	// Backup current sshd_config
	_, err := session.client.RunWithSudo(ctx, "cp /etc/ssh/sshd_config /etc/ssh/sshd_config.backup")
	if err != nil {
		s.logger.Warn("failed to backup sshd_config", "error", err)
	}

	// Use a more robust approach: remove existing settings and append new ones
	// This ensures settings are applied even if they don't exist in the original config
	hardenScript := `
# Remove any existing lines for these settings (commented or not)
sed -i '/^#*\s*PasswordAuthentication/d' /etc/ssh/sshd_config
sed -i '/^#*\s*ChallengeResponseAuthentication/d' /etc/ssh/sshd_config
sed -i '/^#*\s*KbdInteractiveAuthentication/d' /etc/ssh/sshd_config
sed -i '/^#*\s*PermitRootLogin/d' /etc/ssh/sshd_config
sed -i '/^#*\s*PubkeyAuthentication/d' /etc/ssh/sshd_config
sed -i '/^#*\s*UsePAM/d' /etc/ssh/sshd_config

# Append hardened settings at the end of the file
cat >> /etc/ssh/sshd_config << 'EOF'

# SSH Hardening by ICMP-Mon
PasswordAuthentication no
ChallengeResponseAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin prohibit-password
PubkeyAuthentication yes
UsePAM yes
EOF
`

	_, err = session.client.RunWithSudo(ctx, fmt.Sprintf("bash -c '%s'", strings.ReplaceAll(hardenScript, "'", "'\\''")))
	if err != nil {
		s.logger.Warn("ssh hardening script failed", "error", err)
		// Try individual commands as fallback
		fallbackCmds := []string{
			"sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config",
			"sed -i 's/^#*PermitRootLogin.*/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config",
			"grep -q '^PasswordAuthentication' /etc/ssh/sshd_config || echo 'PasswordAuthentication no' >> /etc/ssh/sshd_config",
			"grep -q '^PermitRootLogin' /etc/ssh/sshd_config || echo 'PermitRootLogin prohibit-password' >> /etc/ssh/sshd_config",
		}
		for _, cmd := range fallbackCmds {
			session.client.RunWithSudo(ctx, cmd)
		}
	}

	// Test sshd config
	_, err = session.client.RunWithSudo(ctx, "sshd -t")
	if err != nil {
		// Restore backup
		session.client.RunWithSudo(ctx, "cp /etc/ssh/sshd_config.backup /etc/ssh/sshd_config")
		return fmt.Errorf("SSH config test failed, restored backup: %w", err)
	}

	// Reload sshd (try both service names)
	_, err = session.client.RunWithSudo(ctx, "systemctl reload sshd 2>/dev/null || systemctl reload ssh 2>/dev/null || systemctl restart sshd 2>/dev/null || systemctl restart ssh")
	if err != nil {
		s.logger.Warn("failed to reload sshd", "error", err)
	}

	// Track change for rollback
	enrollment.Changes = append(enrollment.Changes, Change{
		Type:        "ssh_hardened",
		Description: "Disabled SSH password authentication, root login key-only",
		Revertible:  true,
		RevertCmd:   "cp /etc/ssh/sshd_config.backup /etc/ssh/sshd_config && (systemctl reload sshd || systemctl reload ssh)",
		Timestamp:   time.Now(),
	})

	events <- Event{
		Type:      "log",
		Step:      "hardening",
		Message:   "SSH hardening complete - password authentication disabled, root login key-only",
		Timestamp: time.Now(),
	}

	s.store.AddEnrollmentLog(ctx, enrollment.ID, "hardening", "info",
		"SSH password authentication disabled, root login key-only", nil)

	return nil
}

// stepInstallTailscale installs and configures Tailscale on the agent.
// This step is skipped if Tailscale is not configured.
func (s *Service) stepInstallTailscale(ctx context.Context, enrollment *Enrollment, password string, events chan<- Event) error {
	// Skip if Tailscale is not configured
	if !s.TailscaleEnabled() {
		events <- Event{
			Type:      "log",
			Step:      "tailscale",
			Message:   "Tailscale not configured, skipping",
			Timestamp: time.Now(),
		}
		return nil
	}

	session, ok := activeSessions[enrollment.ID]
	if !ok {
		return fmt.Errorf("no active SSH session")
	}

	events <- Event{
		Type:      "log",
		Step:      "tailscale",
		Message:   "Installing Tailscale",
		Timestamp: time.Now(),
	}

	// Install Tailscale with keepalive events to prevent SSE timeout
	if err := InstallTailscale(ctx, session.client, session.systemInfo, events); err != nil {
		return fmt.Errorf("installing Tailscale: %w", err)
	}

	events <- Event{
		Type:      "log",
		Step:      "tailscale",
		Message:   "Authenticating with Tailscale",
		Timestamp: time.Now(),
	}

	// Authenticate with Tailscale
	if err := AuthenticateTailscale(ctx, session.client, s.tailscaleConfig, enrollment.AgentName); err != nil {
		return fmt.Errorf("authenticating Tailscale: %w", err)
	}

	// Get assigned Tailscale IP
	tsIP, err := GetTailscaleIP(ctx, session.client)
	if err != nil {
		return fmt.Errorf("getting Tailscale IP: %w", err)
	}

	enrollment.TailscaleIP = tsIP

	events <- Event{
		Type:      "log",
		Step:      "tailscale",
		Message:   fmt.Sprintf("Tailscale IP assigned: %s", tsIP),
		Timestamp: time.Now(),
	}

	// Now verify connectivity to control plane via Tailscale
	events <- Event{
		Type:      "log",
		Step:      "tailscale",
		Message:   "Verifying connectivity to control plane via Tailscale",
		Timestamp: time.Now(),
	}

	if err := VerifyConnectivity(ctx, session.client, enrollment.ControlPlaneURL); err != nil {
		return fmt.Errorf("control plane connectivity check failed (via Tailscale): %w", err)
	}

	events <- Event{
		Type:      "log",
		Step:      "tailscale",
		Message:   "Control plane reachable via Tailscale",
		Timestamp: time.Now(),
	}

	// Track change for rollback
	enrollment.Changes = append(enrollment.Changes, Change{
		Type:        "tailscale_installed",
		Description: fmt.Sprintf("Installed Tailscale, IP: %s", tsIP),
		Revertible:  true,
		RevertCmd:   "tailscale logout && systemctl disable tailscaled && apt-get remove -y tailscale 2>/dev/null || yum remove -y tailscale 2>/dev/null",
		Timestamp:   time.Now(),
	})

	s.store.AddEnrollmentLog(ctx, enrollment.ID, "tailscale", "info",
		fmt.Sprintf("Tailscale installed, IP: %s", tsIP), nil)

	return nil
}

// stepInstallDependencies installs required packages (fping, mtr) on the target.
func (s *Service) stepInstallDependencies(ctx context.Context, enrollment *Enrollment, password string, events chan<- Event) error {
	session, ok := activeSessions[enrollment.ID]
	if !ok {
		return fmt.Errorf("no active SSH session")
	}

	events <- Event{
		Type:      "log",
		Step:      "dependencies",
		Message:   "Installing required packages (fping, mtr)",
		Timestamp: time.Now(),
	}

	// Determine package manager and install commands
	var installCmd string
	switch session.systemInfo.PackageManager {
	case "apt":
		// Update package list first, then install
		installCmd = "apt-get update -qq && apt-get install -y -qq fping mtr-tiny"
	case "dnf":
		installCmd = "dnf install -y fping mtr"
	case "yum":
		// EPEL may be needed for fping on older RHEL/CentOS
		installCmd = "yum install -y epel-release 2>/dev/null || true && yum install -y fping mtr"
	case "apk":
		installCmd = "apk add --no-cache fping mtr"
	case "pacman":
		installCmd = "pacman -Sy --noconfirm fping mtr"
	case "zypper":
		installCmd = "zypper install -y fping mtr"
	default:
		// Try apt as default for Debian-based systems
		installCmd = "apt-get update -qq && apt-get install -y -qq fping mtr-tiny"
	}

	events <- Event{
		Type:      "log",
		Step:      "dependencies",
		Message:   fmt.Sprintf("Using package manager: %s", session.systemInfo.PackageManager),
		Timestamp: time.Now(),
	}

	// Run the install command with keepalive events every 3 seconds
	// This prevents SSE connection timeouts during long apt-get operations
	output, err := session.client.RunWithSudoKeepalive(ctx, installCmd, events, "dependencies", 3*time.Second)
	if err != nil {
		return fmt.Errorf("installing dependencies: %w (output: %s)", err, output)
	}

	// Verify fping is installed and working
	_, err = session.client.Run(ctx, "which fping && fping --version")
	if err != nil {
		return fmt.Errorf("fping verification failed: %w", err)
	}

	// Verify mtr is installed (mtr-tiny on Debian/Ubuntu provides mtr)
	_, err = session.client.Run(ctx, "which mtr && mtr --version")
	if err != nil {
		// Try mtr-tiny specifically
		_, err = session.client.Run(ctx, "which mtr-tiny")
		if err != nil {
			s.logger.Warn("mtr not available, continuing anyway", "error", err)
		}
	}

	// Track change for rollback
	enrollment.Changes = append(enrollment.Changes, Change{
		Type:        "packages_installed",
		Description: "Installed fping and mtr packages",
		Revertible:  true,
		RevertCmd:   "apt-get remove -y fping mtr-tiny 2>/dev/null || yum remove -y fping mtr 2>/dev/null || true",
		Timestamp:   time.Now(),
	})

	events <- Event{
		Type:      "log",
		Step:      "dependencies",
		Message:   "Dependencies installed successfully",
		Timestamp: time.Now(),
	}

	s.store.AddEnrollmentLog(ctx, enrollment.ID, "dependencies", "info",
		"Installed fping and mtr packages", nil)

	return nil
}

// stepInstallAgent installs the agent binary and configuration.
func (s *Service) stepInstallAgent(ctx context.Context, enrollment *Enrollment, password string, events chan<- Event) error {
	session, ok := activeSessions[enrollment.ID]
	if !ok {
		return fmt.Errorf("no active SSH session")
	}

	paths := DefaultPaths()

	events <- Event{
		Type:      "log",
		Step:      "agent_installing",
		Message:   "Creating agent user and directories",
		Timestamp: time.Now(),
	}

	// Create agent user
	if err := CreateAgentUser(ctx, session.client); err != nil {
		return fmt.Errorf("creating agent user: %w", err)
	}

	enrollment.Changes = append(enrollment.Changes, Change{
		Type:        "user_created",
		Description: "Created icmpmon system user",
		Revertible:  true,
		RevertCmd:   "userdel icmpmon",
		Timestamp:   time.Now(),
	})

	// Create directories
	if err := CreateDirectories(ctx, session.client, paths); err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}

	enrollment.Changes = append(enrollment.Changes, Change{
		Type:        "directories_created",
		Description: fmt.Sprintf("Created directories: %s, %s, %s", paths.ConfigDir, paths.DataDir, paths.LogDir),
		Revertible:  true,
		RevertCmd:   fmt.Sprintf("rm -rf %s %s %s", paths.ConfigDir, paths.DataDir, paths.LogDir),
		Timestamp:   time.Now(),
	})

	events <- Event{
		Type:      "log",
		Step:      "agent_installing",
		Message:   fmt.Sprintf("Downloading agent binary for %s", session.systemInfo.Platform()),
		Timestamp: time.Now(),
	}

	// Download agent binary
	if err := DownloadAgentBinary(ctx, session.client, enrollment.ControlPlaneURL, session.systemInfo.Platform(), paths); err != nil {
		return fmt.Errorf("downloading agent binary: %w", err)
	}

	enrollment.Changes = append(enrollment.Changes, Change{
		Type:        "binary_installed",
		Description: fmt.Sprintf("Installed agent binary at %s", paths.BinaryPath),
		Revertible:  true,
		RevertCmd:   fmt.Sprintf("rm -f %s", paths.BinaryPath),
		Timestamp:   time.Now(),
	})

	events <- Event{
		Type:      "log",
		Step:      "agent_installing",
		Message:   "Generating API key",
		Timestamp: time.Now(),
	}

	// Generate API key for this agent
	// Use a temporary agent ID based on name (real ID assigned after registration)
	tempAgentID := fmt.Sprintf("%s-%d", enrollment.AgentName, time.Now().UnixNano())
	apiKey, apiKeyHash, err := GenerateAgentAPIKey(tempAgentID)
	if err != nil {
		return fmt.Errorf("generating API key: %w", err)
	}
	// Store plaintext key in enrollment for display to user (one-time only)
	enrollment.APIKey = apiKey

	events <- Event{
		Type:      "log",
		Step:      "agent_installing",
		Message:   "Writing agent configuration",
		Timestamp: time.Now(),
	}

	// Write config with API key
	cfg := AgentConfig{
		ControlPlaneURL:    enrollment.ControlPlaneURL,
		AgentName:          enrollment.AgentName,
		Region:             enrollment.Region,
		Location:           enrollment.Location,
		Provider:           enrollment.Provider,
		Tags:               enrollment.Tags,
		APIKey:             apiKey, // Include API key in config
		InsecureSkipVerify: true,   // Skip TLS verification (target may not have CA certs)
	}
	if err := WriteAgentConfig(ctx, session.client, cfg, paths); err != nil {
		return fmt.Errorf("writing agent config: %w", err)
	}

	// Store the API key hash in enrollment for later storage in DB
	enrollment.APIKeyHash = apiKeyHash

	enrollment.Changes = append(enrollment.Changes, Change{
		Type:        "api_key_generated",
		Description: "Generated API key for agent authentication",
		Revertible:  false, // Keys should be rotated, not reverted
		Timestamp:   time.Now(),
	})

	enrollment.Changes = append(enrollment.Changes, Change{
		Type:        "config_written",
		Description: fmt.Sprintf("Wrote config at %s", paths.ConfigFile),
		Revertible:  true,
		RevertCmd:   fmt.Sprintf("rm -f %s", paths.ConfigFile),
		Timestamp:   time.Now(),
	})

	events <- Event{
		Type:      "log",
		Step:      "agent_installing",
		Message:   "Installing systemd service",
		Timestamp: time.Now(),
	}

	// Install systemd service
	if err := InstallSystemdService(ctx, session.client, paths); err != nil {
		return fmt.Errorf("installing systemd service: %w", err)
	}

	enrollment.Changes = append(enrollment.Changes, Change{
		Type:        "service_installed",
		Description: "Installed and enabled systemd service",
		Revertible:  true,
		RevertCmd:   fmt.Sprintf("systemctl disable icmpmon-agent && rm -f %s && systemctl daemon-reload", paths.ServiceFile),
		Timestamp:   time.Now(),
	})

	events <- Event{
		Type:      "log",
		Step:      "agent_installing",
		Message:   "Agent installation complete",
		Timestamp: time.Now(),
	}

	s.store.AddEnrollmentLog(ctx, enrollment.ID, "agent_installing", "info",
		fmt.Sprintf("Agent installed at %s", paths.BinaryPath), nil)

	return nil
}

// stepStartAgent starts the agent service.
func (s *Service) stepStartAgent(ctx context.Context, enrollment *Enrollment, password string, events chan<- Event) error {
	session, ok := activeSessions[enrollment.ID]
	if !ok {
		return fmt.Errorf("no active SSH session")
	}

	events <- Event{
		Type:      "log",
		Step:      "starting",
		Message:   "Starting agent service",
		Timestamp: time.Now(),
	}

	if err := StartAgent(ctx, session.client); err != nil {
		return fmt.Errorf("starting agent: %w", err)
	}

	events <- Event{
		Type:      "log",
		Step:      "starting",
		Message:   "Agent service started successfully",
		Timestamp: time.Now(),
	}

	s.store.AddEnrollmentLog(ctx, enrollment.ID, "starting", "info",
		"Agent service started", nil)

	return nil
}

// stepVerifyRegistration waits for the agent to register with the control plane.
func (s *Service) stepVerifyRegistration(ctx context.Context, enrollment *Enrollment, password string, events chan<- Event) error {
	session, ok := activeSessions[enrollment.ID]
	if !ok {
		return fmt.Errorf("no active SSH session")
	}

	// Close SSH connection after this step
	defer func() {
		session.client.Close()
		delete(activeSessions, enrollment.ID)
	}()

	events <- Event{
		Type:      "log",
		Step:      "registering",
		Message:   "Waiting for agent to register with control plane",
		Timestamp: time.Now(),
	}

	// Wait for agent to appear
	if s.checker != nil {
		agentID, err := s.checker.WaitForAgent(ctx, enrollment.AgentName, 60*time.Second)
		if err != nil {
			// Get agent logs for debugging
			logs, _ := GetAgentLogs(ctx, session.client, 50)
			return fmt.Errorf("agent registration timeout: %w\nAgent logs:\n%s", err, logs)
		}
		enrollment.AgentID = agentID
	} else {
		// No checker available, just wait a bit
		time.Sleep(5 * time.Second)
	}

	events <- Event{
		Type:      "log",
		Step:      "registering",
		Message:   fmt.Sprintf("Agent registered successfully with ID: %s", enrollment.AgentID),
		Timestamp: time.Now(),
	}

	// Store API key hash in database now that we have the real agent ID
	if enrollment.AgentID != "" && enrollment.APIKeyHash != "" && s.checker != nil {
		events <- Event{
			Type:      "log",
			Step:      "registering",
			Message:   "Storing API key for agent",
			Timestamp: time.Now(),
		}

		if err := s.checker.SetAgentAPIKey(ctx, enrollment.AgentID, enrollment.APIKeyHash); err != nil {
			s.logger.Error("failed to store API key", "agent_id", enrollment.AgentID, "error", err)
			// Don't fail enrollment, key can be set manually later
		} else {
			s.store.AddEnrollmentLog(ctx, enrollment.ID, "registering", "info",
				"API key stored for agent", nil)
		}
	}

	// Store Tailscale IP if configured
	if enrollment.AgentID != "" && enrollment.TailscaleIP != "" && s.checker != nil {
		if err := s.checker.SetAgentTailscaleIP(ctx, enrollment.AgentID, enrollment.TailscaleIP); err != nil {
			s.logger.Error("failed to store Tailscale IP", "agent_id", enrollment.AgentID, "error", err)
		} else {
			s.store.AddEnrollmentLog(ctx, enrollment.ID, "registering", "info",
				fmt.Sprintf("Tailscale IP stored: %s", enrollment.TailscaleIP), nil)
		}
	}

	s.store.AddEnrollmentLog(ctx, enrollment.ID, "registering", "info",
		fmt.Sprintf("Agent registered with ID: %s", enrollment.AgentID), nil)

	return nil
}
