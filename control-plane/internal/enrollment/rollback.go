package enrollment

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// RollbackResult contains the results of a rollback operation.
type RollbackResult struct {
	TotalChanges     int
	RevertedChanges  int
	FailedChanges    int
	Errors           []string
}

// Rollback reverts changes made during a failed enrollment.
// It requires SSH access (either with password or key).
func (s *Service) Rollback(ctx context.Context, enrollmentID string, password string) (*RollbackResult, error) {
	enrollment, err := s.store.GetEnrollment(ctx, enrollmentID)
	if err != nil {
		return nil, err
	}
	if enrollment == nil {
		return nil, fmt.Errorf("enrollment not found: %s", enrollmentID)
	}

	if len(enrollment.Changes) == 0 {
		return &RollbackResult{TotalChanges: 0}, nil
	}

	// Try to connect (first try with key, then password)
	var client *SSHClient

	// Try key authentication first
	privateKey, err := s.keyStore.GetPrivateKey(ctx, "icmpmon-provisioning")
	if err == nil && len(privateKey) > 0 {
		client, err = ConnectSSH(ctx, SSHConfig{
			Host:       enrollment.TargetIP,
			Port:       enrollment.TargetPort,
			Username:   enrollment.Username,
			PrivateKey: privateKey,
			Timeout:    30 * time.Second,
		})
	}

	// Fall back to password if key auth fails
	if client == nil && password != "" {
		client, err = ConnectSSH(ctx, SSHConfig{
			Host:     enrollment.TargetIP,
			Port:     enrollment.TargetPort,
			Username: enrollment.Username,
			Password: password,
			Timeout:  30 * time.Second,
		})
		if err != nil {
			return nil, fmt.Errorf("cannot connect to target for rollback: %w", err)
		}
	}

	if client == nil {
		return nil, fmt.Errorf("cannot connect to target: no authentication method available")
	}
	defer client.Close()

	result := &RollbackResult{
		TotalChanges: len(enrollment.Changes),
	}

	// Revert changes in reverse order
	for i := len(enrollment.Changes) - 1; i >= 0; i-- {
		change := enrollment.Changes[i]

		if !change.Revertible {
			s.logger.Info("skipping non-revertible change", "type", change.Type)
			continue
		}

		if change.RevertCmd == "" {
			s.logger.Warn("change has no revert command", "type", change.Type)
			continue
		}

		s.logger.Info("reverting change",
			"type", change.Type,
			"description", change.Description,
			"cmd", change.RevertCmd)

		_, err := client.RunWithSudo(ctx, change.RevertCmd)
		if err != nil {
			s.logger.Error("failed to revert change",
				"type", change.Type,
				"error", err)
			result.FailedChanges++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", change.Type, err))
		} else {
			result.RevertedChanges++
		}
	}

	// Log rollback
	s.store.AddEnrollmentLog(ctx, enrollmentID, "rollback", "info",
		fmt.Sprintf("Rollback completed: %d/%d changes reverted", result.RevertedChanges, result.TotalChanges),
		result)

	return result, nil
}

// CleanupFailedEnrollment cleans up resources from a failed enrollment.
// This is a convenience method that combines rollback with database cleanup.
func (s *Service) CleanupFailedEnrollment(ctx context.Context, enrollmentID string, password string) error {
	enrollment, err := s.store.GetEnrollment(ctx, enrollmentID)
	if err != nil {
		return err
	}
	if enrollment == nil {
		return fmt.Errorf("enrollment not found: %s", enrollmentID)
	}

	// Only cleanup failed or cancelled enrollments
	if enrollment.State != StateFailed && enrollment.State != StateCancelled {
		return fmt.Errorf("can only cleanup failed or cancelled enrollments, current state: %s", enrollment.State)
	}

	// Attempt rollback
	result, err := s.Rollback(ctx, enrollmentID, password)
	if err != nil {
		s.logger.Warn("rollback failed during cleanup", "error", err)
	} else if result.FailedChanges > 0 {
		s.logger.Warn("some changes could not be reverted",
			"reverted", result.RevertedChanges,
			"failed", result.FailedChanges)
	}

	return nil
}

// UninstallAgent removes the agent from a host.
// This can be used to decommission an agent.
func UninstallAgent(ctx context.Context, ssh *SSHClient, logger *slog.Logger) error {
	paths := DefaultPaths()

	// Stop service
	logger.Info("stopping agent service")
	_, _ = ssh.RunWithSudo(ctx, "systemctl stop icmpmon-agent")

	// Disable service
	logger.Info("disabling agent service")
	_, _ = ssh.RunWithSudo(ctx, "systemctl disable icmpmon-agent")

	// Remove service file
	logger.Info("removing service file")
	_, _ = ssh.RunWithSudo(ctx, fmt.Sprintf("rm -f %s", paths.ServiceFile))
	_, _ = ssh.RunWithSudo(ctx, "systemctl daemon-reload")

	// Remove binary
	logger.Info("removing binary")
	_, _ = ssh.RunWithSudo(ctx, fmt.Sprintf("rm -f %s", paths.BinaryPath))

	// Remove config and data directories
	logger.Info("removing directories")
	_, _ = ssh.RunWithSudo(ctx, fmt.Sprintf("rm -rf %s %s %s", paths.ConfigDir, paths.DataDir, paths.LogDir))

	// Remove user
	logger.Info("removing icmpmon user")
	_, _ = ssh.RunWithSudo(ctx, "userdel icmpmon")

	return nil
}

// RemoveSSHKey removes the control plane's SSH key from authorized_keys.
func RemoveSSHKey(ctx context.Context, ssh *SSHClient) error {
	_, err := ssh.Run(ctx, "sed -i '/icmpmon-control-plane$/d' ~/.ssh/authorized_keys")
	return err
}

// ReenablePasswordAuth re-enables SSH password authentication.
func ReenablePasswordAuth(ctx context.Context, ssh *SSHClient) error {
	cmds := []string{
		"sed -i 's/^PasswordAuthentication no/PasswordAuthentication yes/' /etc/ssh/sshd_config",
		"systemctl reload sshd || systemctl reload ssh",
	}

	for _, cmd := range cmds {
		if _, err := ssh.RunWithSudo(ctx, cmd); err != nil {
			return err
		}
	}

	return nil
}
