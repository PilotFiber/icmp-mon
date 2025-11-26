// Package enrollment provides agent enrollment functionality.
//
// The enrollment service manages the full lifecycle of enrolling a new agent:
//  1. SSH connection with password authentication
//  2. OS/architecture detection
//  3. SSH key installation
//  4. SSH hardening (disable password auth)
//  5. Agent binary installation
//  6. Systemd service setup
//  7. Agent registration verification
//
// Each step is idempotent and the service supports resuming from any state.
package enrollment

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pilot-net/icmp-mon/control-plane/internal/secrets"
)

// State represents the enrollment state machine states.
type State string

const (
	StatePending         State = "pending"
	StateConnecting      State = "connecting"
	StateDetecting       State = "detecting"
	StateKeyInstalling   State = "key_installing"
	StateHardening       State = "hardening"
	StateAgentInstalling State = "agent_installing"
	StateStarting        State = "starting"
	StateRegistering     State = "registering"
	StateComplete        State = "complete"
	StateFailed          State = "failed"
	StateCancelled       State = "cancelled"
)

// Event represents an enrollment progress event for SSE streaming.
type Event struct {
	Type      string    `json:"type"`       // "step", "log", "error", "complete"
	Step      string    `json:"step,omitempty"`
	State     State     `json:"state,omitempty"`
	Message   string    `json:"message"`
	Details   any       `json:"details,omitempty"`
	Progress  int       `json:"progress,omitempty"` // 0-100
	Timestamp time.Time `json:"timestamp"`
}

// EnrollRequest contains the parameters for enrolling a new agent.
type EnrollRequest struct {
	// Connection details
	TargetIP   string `json:"target_ip"`
	TargetPort int    `json:"target_port"` // Default: 22
	Username   string `json:"username"`    // SSH username (must have sudo)
	Password   string `json:"password"`    // Used once, never stored

	// Agent configuration
	AgentName string            `json:"agent_name,omitempty"` // Auto-generated if empty
	Region    string            `json:"region,omitempty"`
	Location  string            `json:"location,omitempty"`
	Provider  string            `json:"provider,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`

	// Control plane URL (agent will connect to this)
	ControlPlaneURL string `json:"control_plane_url"`

	// Requested by (for audit)
	RequestedBy string `json:"requested_by,omitempty"`
}

// Enrollment represents an enrollment session.
type Enrollment struct {
	ID string `json:"id"`

	// Target server info
	TargetIP   string `json:"target_ip"`
	TargetPort int    `json:"target_port"`
	Username   string `json:"ssh_username"`

	// Agent configuration
	AgentName string            `json:"agent_name"`
	Region    string            `json:"region"`
	Location  string            `json:"location"`
	Provider  string            `json:"provider"`
	Tags      map[string]string `json:"tags"`

	// State machine
	State          State    `json:"state"`
	CurrentStep    string   `json:"current_step"`
	StepsCompleted []string `json:"steps_completed"`

	// Detection results
	DetectedOS             string `json:"detected_os,omitempty"`
	DetectedOSVersion      string `json:"detected_os_version,omitempty"`
	DetectedArch           string `json:"detected_arch,omitempty"`
	DetectedHostname       string `json:"detected_hostname,omitempty"`
	DetectedPackageManager string `json:"detected_package_manager,omitempty"`

	// Result
	AgentID  string `json:"agent_id,omitempty"`
	SSHKeyID string `json:"ssh_key_id,omitempty"`

	// Rollback tracking
	Changes []Change `json:"changes"`

	// Error handling
	LastError    string `json:"last_error,omitempty"`
	ErrorDetails any    `json:"error_details,omitempty"`
	RetryCount   int    `json:"retry_count"`
	MaxRetries   int    `json:"max_retries"`

	// Timing
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Audit
	RequestedBy     string `json:"requested_by,omitempty"`
	ControlPlaneURL string `json:"control_plane_url"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Change tracks a change made during enrollment for rollback.
type Change struct {
	Type        string    `json:"type"`        // "file_created", "ssh_key_added", "service_created", etc.
	Description string    `json:"description"`
	Revertible  bool      `json:"revertible"`
	RevertCmd   string    `json:"revert_cmd,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// EnrollmentLog represents a log entry for an enrollment.
type EnrollmentLog struct {
	ID           string    `json:"id"`
	EnrollmentID string    `json:"enrollment_id"`
	Step         string    `json:"step"`
	Level        string    `json:"level"` // "info", "warn", "error"
	Message      string    `json:"message"`
	Details      any       `json:"details,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// EnrollmentStore defines the database operations needed by the enrollment service.
type EnrollmentStore interface {
	CreateEnrollment(ctx context.Context, e *Enrollment) error
	UpdateEnrollment(ctx context.Context, e *Enrollment) error
	GetEnrollment(ctx context.Context, id string) (*Enrollment, error)
	ListEnrollments(ctx context.Context, limit int) ([]Enrollment, error)
	AddEnrollmentLog(ctx context.Context, enrollmentID, step, level, message string, details any) error
	GetEnrollmentLogs(ctx context.Context, enrollmentID string) ([]EnrollmentLog, error)
}

// AgentChecker verifies agent registration with the control plane.
type AgentChecker interface {
	WaitForAgent(ctx context.Context, name string, timeout time.Duration) (agentID string, err error)
}

// Service orchestrates agent enrollment.
type Service struct {
	store    EnrollmentStore
	keyStore secrets.KeyStore
	checker  AgentChecker
	logger   *slog.Logger

	// Active enrollments
	mu          sync.Mutex
	enrollments map[string]context.CancelFunc
}

// NewService creates a new enrollment service.
func NewService(store EnrollmentStore, keyStore secrets.KeyStore, checker AgentChecker, logger *slog.Logger) *Service {
	return &Service{
		store:       store,
		keyStore:    keyStore,
		checker:     checker,
		logger:      logger,
		enrollments: make(map[string]context.CancelFunc),
	}
}

// Enroll starts the enrollment process.
// If events is non-nil, progress events are sent to it.
// The password is used once for the initial SSH connection and never stored.
func (s *Service) Enroll(ctx context.Context, req EnrollRequest, events chan<- Event) (*Enrollment, error) {
	// Validate request
	if err := validateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Generate agent name if not provided
	agentName := req.AgentName
	if agentName == "" {
		agentName = fmt.Sprintf("agent-%s", req.TargetIP)
	}

	// Create enrollment record
	enrollment := &Enrollment{
		ID:              uuid.New().String(),
		TargetIP:        req.TargetIP,
		TargetPort:      req.TargetPort,
		Username:        req.Username,
		AgentName:       agentName,
		Region:          req.Region,
		Location:        req.Location,
		Provider:        req.Provider,
		Tags:            req.Tags,
		State:           StatePending,
		StepsCompleted:  []string{},
		Changes:         []Change{},
		MaxRetries:      3,
		ControlPlaneURL: req.ControlPlaneURL,
		RequestedBy:     req.RequestedBy,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if enrollment.TargetPort == 0 {
		enrollment.TargetPort = 22
	}

	// Save to database
	if err := s.store.CreateEnrollment(ctx, enrollment); err != nil {
		return nil, fmt.Errorf("saving enrollment: %w", err)
	}

	// Create cancellable context
	enrollCtx, cancel := context.WithCancel(ctx)

	// Track active enrollment
	s.mu.Lock()
	s.enrollments[enrollment.ID] = cancel
	s.mu.Unlock()

	// Start enrollment in background
	go s.runEnrollment(enrollCtx, enrollment, req.Password, events)

	return enrollment, nil
}

// Cancel cancels an in-progress enrollment.
func (s *Service) Cancel(ctx context.Context, enrollmentID string) error {
	s.mu.Lock()
	cancel, exists := s.enrollments[enrollmentID]
	s.mu.Unlock()

	if exists {
		cancel()
	}

	// Update state in database
	enrollment, err := s.store.GetEnrollment(ctx, enrollmentID)
	if err != nil {
		return err
	}
	if enrollment == nil {
		return fmt.Errorf("enrollment not found: %s", enrollmentID)
	}

	if enrollment.State != StateComplete && enrollment.State != StateFailed {
		enrollment.State = StateCancelled
		enrollment.UpdatedAt = time.Now()
		return s.store.UpdateEnrollment(ctx, enrollment)
	}

	return nil
}

// Retry retries a failed enrollment.
// If events is non-nil, progress events are sent to it.
func (s *Service) Retry(ctx context.Context, enrollmentID string, password string, events chan<- Event) (*Enrollment, error) {
	enrollment, err := s.store.GetEnrollment(ctx, enrollmentID)
	if err != nil {
		return nil, err
	}
	if enrollment == nil {
		return nil, fmt.Errorf("enrollment not found: %s", enrollmentID)
	}

	if enrollment.State != StateFailed {
		return nil, fmt.Errorf("can only retry failed enrollments, current state: %s", enrollment.State)
	}

	if enrollment.RetryCount >= enrollment.MaxRetries {
		return nil, fmt.Errorf("maximum retries exceeded (%d/%d)", enrollment.RetryCount, enrollment.MaxRetries)
	}

	// Reset state to resume from the failed step
	enrollment.State = StatePending
	enrollment.RetryCount++
	enrollment.LastError = ""
	enrollment.UpdatedAt = time.Now()

	if err := s.store.UpdateEnrollment(ctx, enrollment); err != nil {
		return nil, err
	}

	// Create cancellable context
	enrollCtx, cancel := context.WithCancel(ctx)

	// Track active enrollment
	s.mu.Lock()
	s.enrollments[enrollment.ID] = cancel
	s.mu.Unlock()

	// Resume enrollment
	go s.runEnrollment(enrollCtx, enrollment, password, events)

	return enrollment, nil
}

// GetEnrollment returns an enrollment by ID.
func (s *Service) GetEnrollment(ctx context.Context, id string) (*Enrollment, error) {
	return s.store.GetEnrollment(ctx, id)
}

// ListEnrollments returns recent enrollments.
func (s *Service) ListEnrollments(ctx context.Context) ([]Enrollment, error) {
	return s.store.ListEnrollments(ctx, 100) // Default limit
}

// GetEnrollmentLogs returns logs for an enrollment.
func (s *Service) GetEnrollmentLogs(ctx context.Context, enrollmentID string) ([]EnrollmentLog, error) {
	return s.store.GetEnrollmentLogs(ctx, enrollmentID)
}

// sendEvent sends an event to the channel if it's not nil.
// It safely handles the case where the channel is closed.
func sendEvent(events chan<- Event, event Event) {
	if events == nil {
		return
	}
	// Recover from panic if channel is closed
	defer func() {
		recover()
	}()
	events <- event
}

// safeClose safely closes a channel, recovering from panic if already closed.
func safeClose(events chan<- Event) {
	defer func() {
		recover()
	}()
	if events != nil {
		close(events)
	}
}

// runEnrollment executes the enrollment state machine.
func (s *Service) runEnrollment(ctx context.Context, enrollment *Enrollment, password string, events chan<- Event) {
	defer func() {
		safeClose(events)
		s.mu.Lock()
		delete(s.enrollments, enrollment.ID)
		s.mu.Unlock()
	}()

	now := time.Now()
	enrollment.StartedAt = &now

	// State machine steps
	steps := []struct {
		state State
		name  string
		fn    func(ctx context.Context, enrollment *Enrollment, password string, events chan<- Event) error
	}{
		{StateConnecting, "connecting", s.stepConnect},
		{StateDetecting, "detecting", s.stepDetect},
		{StateKeyInstalling, "key_installing", s.stepInstallKey},
		{StateHardening, "hardening", s.stepHarden},
		{StateAgentInstalling, "agent_installing", s.stepInstallAgent},
		{StateStarting, "starting", s.stepStartAgent},
		{StateRegistering, "registering", s.stepVerifyRegistration},
	}

	// Calculate progress per step
	progressPerStep := 100 / len(steps)

	// Find where to resume from (skip completed steps)
	startIdx := 0
	for i, step := range steps {
		if contains(enrollment.StepsCompleted, step.name) {
			startIdx = i + 1
		}
	}

	// Execute remaining steps
	for i := startIdx; i < len(steps); i++ {
		step := steps[i]

		// Check for cancellation
		select {
		case <-ctx.Done():
			s.handleError(ctx, enrollment, events, fmt.Errorf("enrollment cancelled"))
			return
		default:
		}

		// Update state
		enrollment.State = step.state
		enrollment.CurrentStep = step.name
		enrollment.UpdatedAt = time.Now()
		s.store.UpdateEnrollment(ctx, enrollment)

		// Send progress event
		progress := (i + 1) * progressPerStep
		sendEvent(events, Event{
			Type:      "step",
			Step:      step.name,
			State:     step.state,
			Message:   fmt.Sprintf("Starting: %s", step.name),
			Progress:  progress,
			Timestamp: time.Now(),
		})

		s.logger.Info("enrollment step starting",
			"enrollment_id", enrollment.ID,
			"step", step.name,
			"target", enrollment.TargetIP)

		// Execute step
		if err := step.fn(ctx, enrollment, password, events); err != nil {
			s.handleError(ctx, enrollment, events, err)
			return
		}

		// Mark step complete
		enrollment.StepsCompleted = append(enrollment.StepsCompleted, step.name)
		enrollment.UpdatedAt = time.Now()
		s.store.UpdateEnrollment(ctx, enrollment)

		sendEvent(events, Event{
			Type:      "log",
			Step:      step.name,
			Message:   fmt.Sprintf("Completed: %s", step.name),
			Progress:  progress,
			Timestamp: time.Now(),
		})
	}

	// Enrollment complete
	enrollment.State = StateComplete
	now = time.Now()
	enrollment.CompletedAt = &now
	enrollment.UpdatedAt = now
	s.store.UpdateEnrollment(ctx, enrollment)

	sendEvent(events, Event{
		Type:      "complete",
		State:     StateComplete,
		Message:   "Enrollment completed successfully",
		Progress:  100,
		Details:   map[string]string{"agent_id": enrollment.AgentID},
		Timestamp: time.Now(),
	})

	s.logger.Info("enrollment completed",
		"enrollment_id", enrollment.ID,
		"agent_id", enrollment.AgentID,
		"target", enrollment.TargetIP)
}

// handleError handles an enrollment error and triggers rollback if needed.
func (s *Service) handleError(ctx context.Context, enrollment *Enrollment, events chan<- Event, err error) {
	s.logger.Error("enrollment failed",
		"enrollment_id", enrollment.ID,
		"step", enrollment.CurrentStep,
		"error", err)

	enrollment.State = StateFailed
	enrollment.LastError = err.Error()
	enrollment.UpdatedAt = time.Now()
	s.store.UpdateEnrollment(ctx, enrollment)

	sendEvent(events, Event{
		Type:      "error",
		State:     StateFailed,
		Step:      enrollment.CurrentStep,
		Message:   fmt.Sprintf("Enrollment failed: %v", err),
		Timestamp: time.Now(),
	})

	// Store detailed log
	s.store.AddEnrollmentLog(ctx, enrollment.ID, enrollment.CurrentStep, "error", err.Error(), nil)
}

// Helper function to check if a slice contains a string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func validateRequest(req EnrollRequest) error {
	if req.TargetIP == "" {
		return fmt.Errorf("target IP address is required")
	}
	if req.Username == "" {
		return fmt.Errorf("username is required")
	}
	if req.Password == "" {
		return fmt.Errorf("password is required")
	}
	return nil
}
