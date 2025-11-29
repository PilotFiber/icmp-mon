package enrollment

import (
	"context"
	"fmt"
	"strings"
)

// SystemInfo holds detected system information.
type SystemInfo struct {
	OS             string // linux, darwin, freebsd
	OSVersion      string // e.g., "22.04", "14.0"
	Arch           string // amd64, arm64
	Hostname       string
	PackageManager string // apt, dnf, yum, apk
	InitSystem     string // systemd, sysvinit, openrc
	HasSudo        bool
	HasCurl        bool
	HasWget        bool
}

// Platform returns the platform string used for binary downloads (e.g., "linux-amd64").
func (s SystemInfo) Platform() string {
	return fmt.Sprintf("%s-%s", s.OS, s.Arch)
}

// DetectSystem detects the target system's OS, architecture, and capabilities.
func DetectSystem(ctx context.Context, ssh *SSHClient) (*SystemInfo, error) {
	info := &SystemInfo{}

	// Detect OS from /etc/os-release
	osRelease, err := ssh.Run(ctx, "cat /etc/os-release 2>/dev/null || cat /etc/redhat-release 2>/dev/null || uname -s")
	if err == nil {
		info.OS, info.OSVersion = parseOSRelease(osRelease)
	}

	// Fallback to uname for OS
	if info.OS == "" {
		unameS, err := ssh.Run(ctx, "uname -s")
		if err == nil {
			info.OS = normalizeOS(strings.TrimSpace(unameS))
		}
	}

	// Detect architecture
	unameM, err := ssh.Run(ctx, "uname -m")
	if err == nil {
		info.Arch = normalizeArch(strings.TrimSpace(unameM))
	}

	// Detect hostname
	hostname, err := ssh.Run(ctx, "hostname")
	if err == nil {
		info.Hostname = strings.TrimSpace(hostname)
	}

	// Detect package manager
	info.PackageManager = detectPackageManager(ctx, ssh)

	// Detect init system
	info.InitSystem = detectInitSystem(ctx, ssh)

	// Check for sudo
	_, err = ssh.Run(ctx, "which sudo")
	info.HasSudo = err == nil

	// Check for curl
	_, err = ssh.Run(ctx, "which curl")
	info.HasCurl = err == nil

	// Check for wget
	_, err = ssh.Run(ctx, "which wget")
	info.HasWget = err == nil

	return info, nil
}

// parseOSRelease parses /etc/os-release content.
func parseOSRelease(content string) (os, version string) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ID=") {
			os = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		}
		if strings.HasPrefix(line, "VERSION_ID=") {
			version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		}
	}
	os = normalizeOS(os)
	return
}

// normalizeOS normalizes OS names to standard values.
func normalizeOS(os string) string {
	os = strings.ToLower(strings.TrimSpace(os))
	switch os {
	case "linux", "ubuntu", "debian", "centos", "rhel", "fedora", "rocky", "alma", "amazon":
		return "linux"
	case "darwin":
		return "darwin"
	case "freebsd":
		return "freebsd"
	default:
		return os
	}
}

// normalizeArch normalizes architecture names.
func normalizeArch(arch string) string {
	arch = strings.ToLower(strings.TrimSpace(arch))
	switch arch {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "armv7l", "armhf":
		return "arm"
	case "i386", "i686":
		return "386"
	default:
		return arch
	}
}

// detectPackageManager detects the available package manager.
func detectPackageManager(ctx context.Context, ssh *SSHClient) string {
	managers := []struct {
		cmd  string
		name string
	}{
		{"which apt-get", "apt"},
		{"which dnf", "dnf"},
		{"which yum", "yum"},
		{"which apk", "apk"},
		{"which pacman", "pacman"},
		{"which zypper", "zypper"},
	}

	for _, m := range managers {
		_, err := ssh.Run(ctx, m.cmd)
		if err == nil {
			return m.name
		}
	}
	return ""
}

// detectInitSystem detects the init system.
func detectInitSystem(ctx context.Context, ssh *SSHClient) string {
	// Check for systemd
	_, err := ssh.Run(ctx, "pidof systemd")
	if err == nil {
		return "systemd"
	}

	// Check for systemctl
	_, err = ssh.Run(ctx, "which systemctl")
	if err == nil {
		return "systemd"
	}

	// Check for OpenRC
	_, err = ssh.Run(ctx, "which rc-service")
	if err == nil {
		return "openrc"
	}

	// Check for SysVinit
	exists, _ := ssh.FileExists(ctx, "/etc/init.d")
	if exists {
		return "sysvinit"
	}

	return "unknown"
}

// VerifySudoAccess verifies that the user has passwordless sudo access.
func VerifySudoAccess(ctx context.Context, ssh *SSHClient) error {
	// Try running sudo -n (non-interactive)
	_, err := ssh.Run(ctx, "sudo -n true")
	if err != nil {
		return fmt.Errorf("passwordless sudo not available: %w", err)
	}
	return nil
}

// VerifyConnectivity checks if the target can reach the control plane.
func VerifyConnectivity(ctx context.Context, ssh *SSHClient, controlPlaneURL string) error {
	// Extract host from URL
	host := controlPlaneURL
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if idx := strings.Index(host, "/"); idx > 0 {
		host = host[:idx]
	}
	if idx := strings.Index(host, ":"); idx > 0 {
		host = host[:idx]
	}

	// Try to resolve the hostname
	_, err := ssh.Run(ctx, fmt.Sprintf("getent hosts %s || nslookup %s", host, host))
	if err != nil {
		return fmt.Errorf("cannot resolve control plane hostname %s: %w", host, err)
	}

	// Try to connect to the control plane
	// Use -k to skip SSL verification (connectivity test only, not security-sensitive)
	cmd := fmt.Sprintf("curl -sSfk --connect-timeout 5 %s/health || wget -q --no-check-certificate --timeout=5 -O /dev/null %s/health",
		controlPlaneURL, controlPlaneURL)
	_, err = ssh.Run(ctx, cmd)
	if err != nil {
		return fmt.Errorf("cannot reach control plane at %s: %w", controlPlaneURL, err)
	}

	return nil
}
