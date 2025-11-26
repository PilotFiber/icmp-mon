package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pilot-net/icmp-mon/control-plane/internal/enrollment"
)

// =============================================================================
// ENROLLMENT TYPES
// =============================================================================

// Enrollment represents an agent enrollment session.
type Enrollment struct {
	ID         string            `json:"id"`
	TargetIP   string            `json:"target_ip"`
	TargetPort int               `json:"target_port"`
	Username   string            `json:"ssh_username"`
	AgentName  string            `json:"agent_name"`
	Region     string            `json:"region"`
	Location   string            `json:"location"`
	Provider   string            `json:"provider"`
	Tags       map[string]string `json:"tags"`

	State          string   `json:"state"`
	CurrentStep    string   `json:"current_step"`
	StepsCompleted []string `json:"steps_completed"`

	DetectedOS             string `json:"detected_os"`
	DetectedOSVersion      string `json:"detected_os_version"`
	DetectedArch           string `json:"detected_arch"`
	DetectedHostname       string `json:"detected_hostname"`
	DetectedPackageManager string `json:"detected_package_manager"`

	AgentID  string `json:"agent_id"`
	SSHKeyID string `json:"ssh_key_id"`

	Changes json.RawMessage `json:"changes"`

	LastError    string `json:"last_error"`
	ErrorDetails any    `json:"error_details"`
	RetryCount   int    `json:"retry_count"`
	MaxRetries   int    `json:"max_retries"`

	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	RequestedBy     string     `json:"requested_by"`
	ControlPlaneURL string     `json:"control_plane_url"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// EnrollmentLog represents a log entry for an enrollment.
type EnrollmentLog struct {
	ID           string          `json:"id"`
	EnrollmentID string          `json:"enrollment_id"`
	Step         string          `json:"step"`
	Level        string          `json:"level"`
	Message      string          `json:"message"`
	Details      json.RawMessage `json:"details"`
	CreatedAt    time.Time       `json:"created_at"`
}

// SSHKey represents SSH key metadata.
type SSHKey struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	KeyType             string     `json:"key_type"`
	PublicKey           string     `json:"public_key"`
	Fingerprint         string     `json:"fingerprint"`
	OnePasswordItemID   string     `json:"onepassword_item_id"`
	OnePasswordVaultID  string     `json:"onepassword_vault_id"`
	Status              string     `json:"status"`
	CreatedAt           time.Time  `json:"created_at"`
	RotatedAt           *time.Time `json:"rotated_at"`
}

// AgentRelease represents an agent binary release.
type AgentRelease struct {
	ID                   string          `json:"id"`
	Version              string          `json:"version"`
	ReleaseNotes         string          `json:"release_notes"`
	Changelog            string          `json:"changelog"`
	Artifacts            json.RawMessage `json:"artifacts"`
	Platforms            []string        `json:"platforms"`
	Status               string          `json:"status"`
	MinCompatibleVersion string          `json:"min_compatible_version"`
	PublishedAt          *time.Time      `json:"published_at"`
	PublishedBy          string          `json:"published_by"`
	CreatedAt            time.Time       `json:"created_at"`
	CreatedBy            string          `json:"created_by"`
}

// Rollout represents a fleet-wide update campaign.
type Rollout struct {
	ID             string          `json:"id"`
	ReleaseID      string          `json:"release_id"`
	Strategy       string          `json:"strategy"`
	Config         json.RawMessage `json:"config"`
	Status         string          `json:"status"`
	CurrentWave    *int            `json:"current_wave"`
	AgentsTotal    int             `json:"agents_total"`
	AgentsPending  int             `json:"agents_pending"`
	AgentsUpdating int             `json:"agents_updating"`
	AgentsUpdated  int             `json:"agents_updated"`
	AgentsFailed   int             `json:"agents_failed"`
	AgentsSkipped  int             `json:"agents_skipped"`
	AgentFilter    json.RawMessage `json:"agent_filter"`
	StartedAt      *time.Time      `json:"started_at"`
	PausedAt       *time.Time      `json:"paused_at"`
	CompletedAt    *time.Time      `json:"completed_at"`
	RolledBackAt   *time.Time      `json:"rolled_back_at"`
	RollbackReason string          `json:"rollback_reason"`
	CreatedBy      string          `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// =============================================================================
// ENROLLMENT OPERATIONS
// =============================================================================

// CreateEnrollment creates a new enrollment record.
func (s *Store) CreateEnrollment(ctx context.Context, e *Enrollment) error {
	tagsJSON, _ := json.Marshal(e.Tags)
	changesJSON, _ := json.Marshal(e.Changes)

	_, err := s.pool.Exec(ctx, `
		INSERT INTO enrollments (
			id, target_ip, target_port, ssh_username, agent_name, region, location, provider, tags,
			state, current_step, steps_completed, detected_os, detected_os_version, detected_arch,
			detected_hostname, detected_package_manager, agent_id, ssh_key_id, changes,
			last_error, retry_count, max_retries, started_at, completed_at, requested_by, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20,
			$21, $22, $23, $24, $25, $26, NOW(), NOW()
		)
	`,
		e.ID, e.TargetIP, e.TargetPort, e.Username, e.AgentName, e.Region, e.Location, e.Provider, tagsJSON,
		e.State, e.CurrentStep, e.StepsCompleted, e.DetectedOS, e.DetectedOSVersion, e.DetectedArch,
		e.DetectedHostname, e.DetectedPackageManager, nullIfEmpty(e.AgentID), nullIfEmpty(e.SSHKeyID), changesJSON,
		e.LastError, e.RetryCount, e.MaxRetries, e.StartedAt, e.CompletedAt, e.RequestedBy,
	)
	return err
}

// UpdateEnrollment updates an enrollment record.
func (s *Store) UpdateEnrollment(ctx context.Context, e *Enrollment) error {
	tagsJSON, _ := json.Marshal(e.Tags)
	changesJSON, _ := json.Marshal(e.Changes)

	_, err := s.pool.Exec(ctx, `
		UPDATE enrollments SET
			agent_name = $2, region = $3, location = $4, provider = $5, tags = $6,
			state = $7, current_step = $8, steps_completed = $9,
			detected_os = $10, detected_os_version = $11, detected_arch = $12,
			detected_hostname = $13, detected_package_manager = $14,
			agent_id = $15, ssh_key_id = $16, changes = $17,
			last_error = $18, retry_count = $19, started_at = $20, completed_at = $21,
			updated_at = NOW()
		WHERE id = $1
	`,
		e.ID, e.AgentName, e.Region, e.Location, e.Provider, tagsJSON,
		e.State, e.CurrentStep, e.StepsCompleted,
		e.DetectedOS, e.DetectedOSVersion, e.DetectedArch,
		e.DetectedHostname, e.DetectedPackageManager,
		nullIfEmpty(e.AgentID), nullIfEmpty(e.SSHKeyID), changesJSON,
		e.LastError, e.RetryCount, e.StartedAt, e.CompletedAt,
	)
	return err
}

// GetEnrollment retrieves an enrollment by ID.
func (s *Store) GetEnrollment(ctx context.Context, id string) (*Enrollment, error) {
	var e Enrollment
	var tagsJSON, changesJSON []byte
	var agentID, sshKeyID *string

	err := s.pool.QueryRow(ctx, `
		SELECT id, host(target_ip), target_port, ssh_username, agent_name, region, location, provider, tags,
			state, current_step, steps_completed, detected_os, detected_os_version, detected_arch,
			detected_hostname, detected_package_manager, agent_id, ssh_key_id, changes,
			last_error, retry_count, max_retries, started_at, completed_at, requested_by, created_at, updated_at
		FROM enrollments WHERE id = $1
	`, id).Scan(
		&e.ID, &e.TargetIP, &e.TargetPort, &e.Username, &e.AgentName, &e.Region, &e.Location, &e.Provider, &tagsJSON,
		&e.State, &e.CurrentStep, &e.StepsCompleted, &e.DetectedOS, &e.DetectedOSVersion, &e.DetectedArch,
		&e.DetectedHostname, &e.DetectedPackageManager, &agentID, &sshKeyID, &changesJSON,
		&e.LastError, &e.RetryCount, &e.MaxRetries, &e.StartedAt, &e.CompletedAt, &e.RequestedBy, &e.CreatedAt, &e.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal(tagsJSON, &e.Tags)
	e.Changes = changesJSON
	if agentID != nil {
		e.AgentID = *agentID
	}
	if sshKeyID != nil {
		e.SSHKeyID = *sshKeyID
	}

	return &e, nil
}

// ListEnrollments returns recent enrollments.
func (s *Store) ListEnrollments(ctx context.Context, limit int) ([]Enrollment, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, host(target_ip), target_port, ssh_username, agent_name, region, location, provider, tags,
			state, current_step, steps_completed, detected_os, detected_os_version, detected_arch,
			detected_hostname, detected_package_manager, agent_id, ssh_key_id, changes,
			last_error, retry_count, max_retries, started_at, completed_at, requested_by, created_at, updated_at
		FROM enrollments
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var enrollments []Enrollment
	for rows.Next() {
		var e Enrollment
		var tagsJSON, changesJSON []byte
		var agentID, sshKeyID *string

		if err := rows.Scan(
			&e.ID, &e.TargetIP, &e.TargetPort, &e.Username, &e.AgentName, &e.Region, &e.Location, &e.Provider, &tagsJSON,
			&e.State, &e.CurrentStep, &e.StepsCompleted, &e.DetectedOS, &e.DetectedOSVersion, &e.DetectedArch,
			&e.DetectedHostname, &e.DetectedPackageManager, &agentID, &sshKeyID, &changesJSON,
			&e.LastError, &e.RetryCount, &e.MaxRetries, &e.StartedAt, &e.CompletedAt, &e.RequestedBy, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, err
		}

		json.Unmarshal(tagsJSON, &e.Tags)
		e.Changes = changesJSON
		if agentID != nil {
			e.AgentID = *agentID
		}
		if sshKeyID != nil {
			e.SSHKeyID = *sshKeyID
		}

		enrollments = append(enrollments, e)
	}
	return enrollments, nil
}

// AddEnrollmentLog adds a log entry for an enrollment.
func (s *Store) AddEnrollmentLog(ctx context.Context, enrollmentID, step, level, message string, details any) error {
	var detailsJSON []byte
	if details != nil {
		detailsJSON, _ = json.Marshal(details)
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO enrollment_logs (enrollment_id, step, level, message, details)
		VALUES ($1, $2, $3, $4, $5)
	`, enrollmentID, step, level, message, detailsJSON)
	return err
}

// GetEnrollmentLogs retrieves logs for an enrollment.
// Returns enrollment.EnrollmentLog to satisfy the enrollment.EnrollmentStore interface.
func (s *Store) GetEnrollmentLogs(ctx context.Context, enrollmentID string) ([]enrollment.EnrollmentLog, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, enrollment_id, step, level, message, details, created_at
		FROM enrollment_logs
		WHERE enrollment_id = $1
		ORDER BY created_at ASC
	`, enrollmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []enrollment.EnrollmentLog
	for rows.Next() {
		var log enrollment.EnrollmentLog
		var details json.RawMessage
		if err := rows.Scan(
			&log.ID, &log.EnrollmentID, &log.Step, &log.Level, &log.Message, &details, &log.CreatedAt,
		); err != nil {
			return nil, err
		}
		// Convert json.RawMessage to any for the Details field
		if len(details) > 0 {
			var d any
			json.Unmarshal(details, &d)
			log.Details = d
		}
		logs = append(logs, log)
	}
	return logs, nil
}

// =============================================================================
// SSH KEY OPERATIONS
// =============================================================================

// CreateSSHKey creates a new SSH key record.
func (s *Store) CreateSSHKey(ctx context.Context, key *SSHKey) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ssh_keys (id, name, key_type, public_key, fingerprint, onepassword_item_id, onepassword_vault_id, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, key.ID, key.Name, key.KeyType, key.PublicKey, key.Fingerprint, nullIfEmpty(key.OnePasswordItemID), nullIfEmpty(key.OnePasswordVaultID), key.Status)
	return err
}

// GetSSHKeyByName retrieves an SSH key by name.
func (s *Store) GetSSHKeyByName(ctx context.Context, name string) (*SSHKey, error) {
	var key SSHKey
	var opItemID, opVaultID *string

	err := s.pool.QueryRow(ctx, `
		SELECT id, name, key_type, public_key, fingerprint, onepassword_item_id, onepassword_vault_id, status, created_at, rotated_at
		FROM ssh_keys WHERE name = $1 AND status = 'active'
	`, name).Scan(
		&key.ID, &key.Name, &key.KeyType, &key.PublicKey, &key.Fingerprint, &opItemID, &opVaultID, &key.Status, &key.CreatedAt, &key.RotatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if opItemID != nil {
		key.OnePasswordItemID = *opItemID
	}
	if opVaultID != nil {
		key.OnePasswordVaultID = *opVaultID
	}

	return &key, nil
}

// =============================================================================
// RELEASE OPERATIONS
// =============================================================================

// CreateRelease creates a new agent release record.
func (s *Store) CreateRelease(ctx context.Context, r *AgentRelease) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO agent_releases (id, version, release_notes, changelog, artifacts, platforms, status, min_compatible_version, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, r.ID, r.Version, r.ReleaseNotes, r.Changelog, r.Artifacts, r.Platforms, r.Status, nullIfEmpty(r.MinCompatibleVersion), r.CreatedBy)
	return err
}

// GetReleaseByVersion retrieves a release by version.
func (s *Store) GetReleaseByVersion(ctx context.Context, version string) (*AgentRelease, error) {
	var r AgentRelease
	var minCompat *string

	err := s.pool.QueryRow(ctx, `
		SELECT id, version, release_notes, changelog, artifacts, platforms, status, min_compatible_version,
			published_at, published_by, created_at, created_by
		FROM agent_releases WHERE version = $1
	`, version).Scan(
		&r.ID, &r.Version, &r.ReleaseNotes, &r.Changelog, &r.Artifacts, &r.Platforms, &r.Status, &minCompat,
		&r.PublishedAt, &r.PublishedBy, &r.CreatedAt, &r.CreatedBy,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if minCompat != nil {
		r.MinCompatibleVersion = *minCompat
	}

	return &r, nil
}

// GetLatestPublishedRelease returns the most recent published release.
func (s *Store) GetLatestPublishedRelease(ctx context.Context) (*AgentRelease, error) {
	var r AgentRelease
	var minCompat *string

	err := s.pool.QueryRow(ctx, `
		SELECT id, version, release_notes, changelog, artifacts, platforms, status, min_compatible_version,
			published_at, published_by, created_at, created_by
		FROM agent_releases
		WHERE status = 'published'
		ORDER BY published_at DESC
		LIMIT 1
	`).Scan(
		&r.ID, &r.Version, &r.ReleaseNotes, &r.Changelog, &r.Artifacts, &r.Platforms, &r.Status, &minCompat,
		&r.PublishedAt, &r.PublishedBy, &r.CreatedAt, &r.CreatedBy,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if minCompat != nil {
		r.MinCompatibleVersion = *minCompat
	}

	return &r, nil
}

// ListReleases returns all releases.
func (s *Store) ListReleases(ctx context.Context) ([]AgentRelease, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, version, release_notes, changelog, artifacts, platforms, status, min_compatible_version,
			published_at, published_by, created_at, created_by
		FROM agent_releases
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var releases []AgentRelease
	for rows.Next() {
		var r AgentRelease
		var minCompat *string
		if err := rows.Scan(
			&r.ID, &r.Version, &r.ReleaseNotes, &r.Changelog, &r.Artifacts, &r.Platforms, &r.Status, &minCompat,
			&r.PublishedAt, &r.PublishedBy, &r.CreatedAt, &r.CreatedBy,
		); err != nil {
			return nil, err
		}
		if minCompat != nil {
			r.MinCompatibleVersion = *minCompat
		}
		releases = append(releases, r)
	}
	return releases, nil
}

// PublishRelease marks a release as published.
func (s *Store) PublishRelease(ctx context.Context, id, publishedBy string) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE agent_releases SET status = 'published', published_at = NOW(), published_by = $2
		WHERE id = $1 AND status = 'draft'
	`, id, publishedBy)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("release not found or not in draft status")
	}
	return nil
}

// =============================================================================
// ROLLOUT OPERATIONS
// =============================================================================

// CreateRollout creates a new rollout record.
func (s *Store) CreateRollout(ctx context.Context, r *Rollout) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO rollouts (id, release_id, strategy, config, status, agents_total, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, r.ID, r.ReleaseID, r.Strategy, r.Config, r.Status, r.AgentsTotal, r.CreatedBy)
	return err
}

// GetRollout retrieves a rollout by ID.
func (s *Store) GetRollout(ctx context.Context, id string) (*Rollout, error) {
	var r Rollout
	err := s.pool.QueryRow(ctx, `
		SELECT id, release_id, strategy, config, status, current_wave,
			agents_total, agents_pending, agents_updating, agents_updated, agents_failed, agents_skipped,
			agent_filter, started_at, paused_at, completed_at, rolled_back_at, rollback_reason,
			created_by, created_at, updated_at
		FROM rollouts WHERE id = $1
	`, id).Scan(
		&r.ID, &r.ReleaseID, &r.Strategy, &r.Config, &r.Status, &r.CurrentWave,
		&r.AgentsTotal, &r.AgentsPending, &r.AgentsUpdating, &r.AgentsUpdated, &r.AgentsFailed, &r.AgentsSkipped,
		&r.AgentFilter, &r.StartedAt, &r.PausedAt, &r.CompletedAt, &r.RolledBackAt, &r.RollbackReason,
		&r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// UpdateRollout updates a rollout record.
func (s *Store) UpdateRollout(ctx context.Context, r *Rollout) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE rollouts SET
			status = $2, current_wave = $3,
			agents_pending = $4, agents_updating = $5, agents_updated = $6, agents_failed = $7, agents_skipped = $8,
			started_at = $9, paused_at = $10, completed_at = $11, rolled_back_at = $12, rollback_reason = $13,
			updated_at = NOW()
		WHERE id = $1
	`, r.ID, r.Status, r.CurrentWave,
		r.AgentsPending, r.AgentsUpdating, r.AgentsUpdated, r.AgentsFailed, r.AgentsSkipped,
		r.StartedAt, r.PausedAt, r.CompletedAt, r.RolledBackAt, r.RollbackReason,
	)
	return err
}

// ListActiveRollouts returns all active (non-completed) rollouts.
func (s *Store) ListActiveRollouts(ctx context.Context) ([]Rollout, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, release_id, strategy, config, status, current_wave,
			agents_total, agents_pending, agents_updating, agents_updated, agents_failed, agents_skipped,
			agent_filter, started_at, paused_at, completed_at, rolled_back_at, rollback_reason,
			created_by, created_at, updated_at
		FROM rollouts
		WHERE status IN ('pending', 'in_progress', 'paused')
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rollouts []Rollout
	for rows.Next() {
		var r Rollout
		if err := rows.Scan(
			&r.ID, &r.ReleaseID, &r.Strategy, &r.Config, &r.Status, &r.CurrentWave,
			&r.AgentsTotal, &r.AgentsPending, &r.AgentsUpdating, &r.AgentsUpdated, &r.AgentsFailed, &r.AgentsSkipped,
			&r.AgentFilter, &r.StartedAt, &r.PausedAt, &r.CompletedAt, &r.RolledBackAt, &r.RollbackReason,
			&r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rollouts = append(rollouts, r)
	}
	return rollouts, nil
}

// =============================================================================
// AGENT CHECKER (for enrollment verification)
// =============================================================================

// WaitForAgentRegistration waits for an agent to register by name.
func (s *Store) WaitForAgentRegistration(ctx context.Context, name string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			agent, err := s.GetAgentByName(ctx, name)
			if err != nil {
				return "", err
			}
			if agent != nil {
				return agent.ID, nil
			}
			if time.Now().After(deadline) {
				return "", fmt.Errorf("timeout waiting for agent %s to register", name)
			}
		}
	}
}

// =============================================================================
// HELPERS
// =============================================================================

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
