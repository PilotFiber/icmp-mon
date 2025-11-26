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

// stepConnect establishes the initial SSH connection using password authentication.
func (s *Service) stepConnect(ctx context.Context, enrollment *Enrollment, password string, events chan<- Event) error {
	events <- Event{
		Type:      "log",
		Step:      "connecting",
		Message:   fmt.Sprintf("Connecting to %s:%d as %s", enrollment.TargetIP, enrollment.TargetPort, enrollment.Username),
		Timestamp: time.Now(),
	}

	// Connect with password
	client, err := ConnectSSH(ctx, SSHConfig{
		Host:     enrollment.TargetIP,
		Port:     enrollment.TargetPort,
		Username: enrollment.Username,
		Password: password,
		Timeout:  30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
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

	events <- Event{
		Type: "log",
		Step: "detecting",
		Message: fmt.Sprintf("Detected: %s %s (%s), hostname: %s",
			info.OS, info.OSVersion, info.Arch, info.Hostname),
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
	events <- Event{
		Type:      "log",
		Step:      "detecting",
		Message:   "Verifying connectivity to control plane",
		Timestamp: time.Now(),
	}

	if err := VerifyConnectivity(ctx, session.client, enrollment.ControlPlaneURL); err != nil {
		return fmt.Errorf("control plane connectivity check failed: %w", err)
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

	// Disable password authentication
	hardenCmds := []string{
		"sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config",
		"sed -i 's/^#*ChallengeResponseAuthentication.*/ChallengeResponseAuthentication no/' /etc/ssh/sshd_config",
		"sed -i 's/^#*KbdInteractiveAuthentication.*/KbdInteractiveAuthentication no/' /etc/ssh/sshd_config",
	}

	for _, cmd := range hardenCmds {
		_, err := session.client.RunWithSudo(ctx, cmd)
		if err != nil {
			s.logger.Warn("ssh hardening command failed", "cmd", cmd, "error", err)
		}
	}

	// Test sshd config
	_, err = session.client.RunWithSudo(ctx, "sshd -t")
	if err != nil {
		// Restore backup
		session.client.RunWithSudo(ctx, "cp /etc/ssh/sshd_config.backup /etc/ssh/sshd_config")
		return fmt.Errorf("SSH config test failed, restored backup: %w", err)
	}

	// Reload sshd
	_, err = session.client.RunWithSudo(ctx, "systemctl reload sshd || systemctl reload ssh")
	if err != nil {
		s.logger.Warn("failed to reload sshd", "error", err)
	}

	// Track change for rollback
	enrollment.Changes = append(enrollment.Changes, Change{
		Type:        "ssh_hardened",
		Description: "Disabled SSH password authentication",
		Revertible:  true,
		RevertCmd:   "cp /etc/ssh/sshd_config.backup /etc/ssh/sshd_config && systemctl reload sshd",
		Timestamp:   time.Now(),
	})

	events <- Event{
		Type:      "log",
		Step:      "hardening",
		Message:   "SSH hardening complete - password authentication disabled",
		Timestamp: time.Now(),
	}

	s.store.AddEnrollmentLog(ctx, enrollment.ID, "hardening", "info",
		"SSH password authentication disabled", nil)

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
		Message:   "Writing agent configuration",
		Timestamp: time.Now(),
	}

	// Write config
	cfg := AgentConfig{
		ControlPlaneURL: enrollment.ControlPlaneURL,
		AgentName:       enrollment.AgentName,
		Region:          enrollment.Region,
		Location:        enrollment.Location,
		Provider:        enrollment.Provider,
		Tags:            enrollment.Tags,
	}
	if err := WriteAgentConfig(ctx, session.client, cfg, paths); err != nil {
		return fmt.Errorf("writing agent config: %w", err)
	}

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

	s.store.AddEnrollmentLog(ctx, enrollment.ID, "registering", "info",
		fmt.Sprintf("Agent registered with ID: %s", enrollment.AgentID), nil)

	return nil
}
