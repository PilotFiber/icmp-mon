// Command server runs the ICMP-Mon control plane.
//
// # Usage
//
//	server --database postgres://localhost/icmpmon --port 8080
//
// # Configuration
//
// The server can be configured via:
// - Command-line flags
// - Environment variables (ICMPMON_*)
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pilot-net/icmp-mon/control-plane/internal/api"
	"github.com/pilot-net/icmp-mon/control-plane/internal/enrollment"
	"github.com/pilot-net/icmp-mon/control-plane/internal/rollout"
	"github.com/pilot-net/icmp-mon/control-plane/internal/secrets"
	"github.com/pilot-net/icmp-mon/control-plane/internal/service"
	"github.com/pilot-net/icmp-mon/control-plane/internal/store"
	"github.com/pilot-net/icmp-mon/control-plane/internal/worker"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

func main() {
	var (
		port     = flag.Int("port", 8080, "HTTP server port")
		dbURL    = flag.String("database", "", "Database URL (postgres://...)")
		debug    = flag.Bool("debug", false, "Enable debug logging")
		version  = flag.Bool("version", false, "Print version and exit")
	)
	flag.Parse()

	if *version {
		fmt.Println("icmpmon-server v0.1.0")
		os.Exit(0)
	}

	// Set up logging
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Get database URL from env if not provided
	if *dbURL == "" {
		*dbURL = os.Getenv("ICMPMON_DATABASE_URL")
	}
	if *dbURL == "" {
		*dbURL = "postgres://localhost:5432/icmpmon?sslmode=disable"
	}

	// Connect to database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := store.NewStoreFromURL(ctx, *dbURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Verify database connection
	if err := db.Ping(ctx); err != nil {
		logger.Error("database ping failed", "error", err)
		os.Exit(1)
	}
	logger.Info("connected to database")

	// Create service and API
	svc := service.NewService(db, logger)
	apiServer := api.NewServer(svc, logger)

	// Initialize enrollment service (optional - only if secrets backend is configured)
	keyStore, err := secrets.NewKeyStore(secrets.ConfigFromEnv(), logger)
	if err != nil {
		logger.Warn("enrollment disabled - keystore initialization failed", "error", err)
	} else {
		// Create adapters
		enrollmentStore := &storeEnrollmentAdapter{db: db}
		agentChecker := &storeAgentChecker{db: db}

		// Create enrollment service
		enrollmentSvc := enrollment.NewService(enrollmentStore, keyStore, agentChecker, logger)

		// Register enrollment routes
		enrollmentHandler := api.NewEnrollmentHandler(enrollmentSvc, logger)
		enrollmentHandler.RegisterRoutes(apiServer.Mux())

		logger.Info("enrollment service enabled")
	}

	// Initialize rollout service
	rolloutStore := &storeRolloutAdapter{db: db}
	rolloutEngine := rollout.NewEngine(rolloutStore, logger)
	rolloutHandler := api.NewRolloutHandler(rolloutEngine, logger)
	rolloutHandler.RegisterRoutes(apiServer.Mux())
	logger.Info("rollout service enabled")

	// Initialize state worker for monitoring state transitions
	stateStoreAdapter := &storeStateAdapter{db: db}
	stateWorker := worker.NewStateWorker(stateStoreAdapter, worker.DefaultStateWorkerConfig(), logger)
	stateWorker.Start(context.Background())
	defer stateWorker.Stop()
	logger.Info("state worker started")

	// Initialize assignment worker for automatic redistribution
	rebalancer := service.NewRebalancer(db, logger)
	assignmentWorker := worker.NewAssignmentWorker(
		db,
		rebalancer,
		worker.DefaultAssignmentWorkerConfig(),
		logger,
	)
	assignmentWorker.Start(context.Background())
	defer assignmentWorker.Stop()
	logger.Info("assignment worker started")

	// Register assignment management routes
	assignmentHandler := api.NewAssignmentHandler(rebalancer, logger)
	assignmentHandler.RegisterRoutes(apiServer.Mux())
	logger.Info("assignment management enabled")

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      apiServer,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server
	go func() {
		logger.Info("starting server", "port", *port)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}

	logger.Info("shutdown complete")
}

// storeAgentChecker implements enrollment.AgentChecker using the store.
type storeAgentChecker struct {
	db *store.Store
}

func (c *storeAgentChecker) WaitForAgent(ctx context.Context, name string, timeout time.Duration) (string, error) {
	return c.db.WaitForAgentRegistration(ctx, name, timeout)
}

// storeEnrollmentAdapter implements enrollment.EnrollmentStore using store.Store.
type storeEnrollmentAdapter struct {
	db *store.Store
}

func (a *storeEnrollmentAdapter) CreateEnrollment(ctx context.Context, e *enrollment.Enrollment) error {
	se := toStoreEnrollment(e)
	return a.db.CreateEnrollment(ctx, se)
}

func (a *storeEnrollmentAdapter) UpdateEnrollment(ctx context.Context, e *enrollment.Enrollment) error {
	se := toStoreEnrollment(e)
	return a.db.UpdateEnrollment(ctx, se)
}

func (a *storeEnrollmentAdapter) GetEnrollment(ctx context.Context, id string) (*enrollment.Enrollment, error) {
	se, err := a.db.GetEnrollment(ctx, id)
	if err != nil {
		return nil, err
	}
	if se == nil {
		return nil, nil
	}
	return toEnrollmentEnrollment(se), nil
}

func (a *storeEnrollmentAdapter) ListEnrollments(ctx context.Context, limit int) ([]enrollment.Enrollment, error) {
	storeEnrollments, err := a.db.ListEnrollments(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]enrollment.Enrollment, len(storeEnrollments))
	for i, se := range storeEnrollments {
		result[i] = *toEnrollmentEnrollment(&se)
	}
	return result, nil
}

func (a *storeEnrollmentAdapter) AddEnrollmentLog(ctx context.Context, enrollmentID, step, level, message string, details any) error {
	return a.db.AddEnrollmentLog(ctx, enrollmentID, step, level, message, details)
}

func (a *storeEnrollmentAdapter) GetEnrollmentLogs(ctx context.Context, enrollmentID string) ([]enrollment.EnrollmentLog, error) {
	return a.db.GetEnrollmentLogs(ctx, enrollmentID)
}

// toStoreEnrollment converts enrollment.Enrollment to store.Enrollment.
func toStoreEnrollment(e *enrollment.Enrollment) *store.Enrollment {
	return &store.Enrollment{
		ID:                     e.ID,
		TargetIP:               e.TargetIP,
		TargetPort:             e.TargetPort,
		Username:               e.Username,
		AgentName:              e.AgentName,
		Region:                 e.Region,
		Location:               e.Location,
		Provider:               e.Provider,
		Tags:                   e.Tags,
		State:                  string(e.State),
		CurrentStep:            e.CurrentStep,
		StepsCompleted:         e.StepsCompleted,
		DetectedOS:             e.DetectedOS,
		DetectedOSVersion:      e.DetectedOSVersion,
		DetectedArch:           e.DetectedArch,
		DetectedHostname:       e.DetectedHostname,
		DetectedPackageManager: e.DetectedPackageManager,
		AgentID:                e.AgentID,
		SSHKeyID:               e.SSHKeyID,
		LastError:              e.LastError,
		RetryCount:             e.RetryCount,
		MaxRetries:             e.MaxRetries,
		StartedAt:              e.StartedAt,
		CompletedAt:            e.CompletedAt,
		RequestedBy:            e.RequestedBy,
		ControlPlaneURL:        e.ControlPlaneURL,
		CreatedAt:              e.CreatedAt,
		UpdatedAt:              e.UpdatedAt,
	}
}

// toEnrollmentEnrollment converts store.Enrollment to enrollment.Enrollment.
func toEnrollmentEnrollment(se *store.Enrollment) *enrollment.Enrollment {
	return &enrollment.Enrollment{
		ID:                     se.ID,
		TargetIP:               se.TargetIP,
		TargetPort:             se.TargetPort,
		Username:               se.Username,
		AgentName:              se.AgentName,
		Region:                 se.Region,
		Location:               se.Location,
		Provider:               se.Provider,
		Tags:                   se.Tags,
		State:                  enrollment.State(se.State),
		CurrentStep:            se.CurrentStep,
		StepsCompleted:         se.StepsCompleted,
		DetectedOS:             se.DetectedOS,
		DetectedOSVersion:      se.DetectedOSVersion,
		DetectedArch:           se.DetectedArch,
		DetectedHostname:       se.DetectedHostname,
		DetectedPackageManager: se.DetectedPackageManager,
		AgentID:                se.AgentID,
		SSHKeyID:               se.SSHKeyID,
		LastError:              se.LastError,
		RetryCount:             se.RetryCount,
		MaxRetries:             se.MaxRetries,
		StartedAt:              se.StartedAt,
		CompletedAt:            se.CompletedAt,
		RequestedBy:            se.RequestedBy,
		ControlPlaneURL:        se.ControlPlaneURL,
		CreatedAt:              se.CreatedAt,
		UpdatedAt:              se.UpdatedAt,
	}
}

// =============================================================================
// ROLLOUT STORE ADAPTER
// =============================================================================

// storeRolloutAdapter implements rollout.RolloutStore using store.Store.
type storeRolloutAdapter struct {
	db *store.Store
}

func (a *storeRolloutAdapter) CreateRollout(ctx context.Context, r *rollout.Rollout) error {
	sr := toStoreRollout(r)
	return a.db.CreateRollout(ctx, sr)
}

func (a *storeRolloutAdapter) UpdateRollout(ctx context.Context, r *rollout.Rollout) error {
	sr := toStoreRollout(r)
	return a.db.UpdateRollout(ctx, sr)
}

func (a *storeRolloutAdapter) GetRollout(ctx context.Context, id string) (*rollout.Rollout, error) {
	sr, err := a.db.GetRollout(ctx, id)
	if err != nil {
		return nil, err
	}
	if sr == nil {
		return nil, nil
	}
	return toRolloutRollout(sr), nil
}

func (a *storeRolloutAdapter) ListActiveRollouts(ctx context.Context) ([]rollout.Rollout, error) {
	srs, err := a.db.ListActiveRollouts(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]rollout.Rollout, len(srs))
	for i, sr := range srs {
		result[i] = *toRolloutRollout(&sr)
	}
	return result, nil
}

func (a *storeRolloutAdapter) SetAgentUpdateState(ctx context.Context, rolloutID string, state *rollout.AgentUpdateState) error {
	ss := &store.RolloutAgentState{
		AgentID:     state.AgentID,
		AgentName:   state.AgentName,
		Wave:        state.Wave,
		Status:      state.Status,
		FromVersion: state.FromVersion,
		ToVersion:   state.ToVersion,
		StartedAt:   state.StartedAt,
		CompletedAt: state.CompletedAt,
		Error:       state.Error,
	}
	return a.db.SetRolloutAgentState(ctx, rolloutID, ss)
}

func (a *storeRolloutAdapter) GetAgentUpdateStates(ctx context.Context, rolloutID string) ([]rollout.AgentUpdateState, error) {
	ss, err := a.db.GetRolloutAgentStates(ctx, rolloutID)
	if err != nil {
		return nil, err
	}
	result := make([]rollout.AgentUpdateState, len(ss))
	for i, s := range ss {
		result[i] = rollout.AgentUpdateState{
			AgentID:     s.AgentID,
			AgentName:   s.AgentName,
			Wave:        s.Wave,
			Status:      s.Status,
			FromVersion: s.FromVersion,
			ToVersion:   s.ToVersion,
			StartedAt:   s.StartedAt,
			CompletedAt: s.CompletedAt,
			Error:       s.Error,
		}
	}
	return result, nil
}

func (a *storeRolloutAdapter) GetRelease(ctx context.Context, id string) (*rollout.Release, error) {
	version, checksum, size, err := a.db.GetReleaseSimple(ctx, id)
	if err != nil {
		return nil, err
	}
	if version == "" {
		return nil, nil
	}
	return &rollout.Release{
		ID:       id,
		Version:  version,
		Checksum: checksum,
		Size:     size,
	}, nil
}

func (a *storeRolloutAdapter) GetAgentsForRollout(ctx context.Context, filter *rollout.AgentFilter) ([]rollout.AgentInfo, error) {
	agents, err := a.db.GetAgentsForRollout(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := make([]rollout.AgentInfo, len(agents))
	for i, a := range agents {
		result[i] = rollout.AgentInfo{
			ID:       a.ID,
			Name:     a.Name,
			Version:  a.Version,
			Region:   a.Region,
			Provider: a.Provider,
		}
	}
	return result, nil
}

func (a *storeRolloutAdapter) GetAgentVersion(ctx context.Context, agentID string) (string, error) {
	return a.db.GetAgentVersion(ctx, agentID)
}

func (a *storeRolloutAdapter) IsAgentHealthy(ctx context.Context, agentID string, since time.Time) (bool, error) {
	return a.db.IsAgentHealthy(ctx, agentID, since)
}

// toStoreRollout converts rollout.Rollout to store.Rollout.
func toStoreRollout(r *rollout.Rollout) *store.Rollout {
	var currentWave *int
	if r.CurrentWave > 0 {
		currentWave = &r.CurrentWave
	}
	return &store.Rollout{
		ID:             r.ID,
		ReleaseID:      r.ReleaseID,
		Strategy:       string(r.Config.Strategy),
		Status:         string(r.Status),
		CurrentWave:    currentWave,
		AgentsTotal:    r.AgentsTotal,
		AgentsPending:  r.AgentsPending,
		AgentsUpdating: r.AgentsUpdating,
		AgentsUpdated:  r.AgentsUpdated,
		AgentsFailed:   r.AgentsFailed,
		AgentsSkipped:  r.AgentsSkipped,
		StartedAt:      r.StartedAt,
		PausedAt:       r.PausedAt,
		CompletedAt:    r.CompletedAt,
		RolledBackAt:   r.RolledBackAt,
		RollbackReason: r.RollbackReason,
		CreatedBy:      r.CreatedBy,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}
}

// toRolloutRollout converts store.Rollout to rollout.Rollout.
func toRolloutRollout(sr *store.Rollout) *rollout.Rollout {
	currentWave := 0
	if sr.CurrentWave != nil {
		currentWave = *sr.CurrentWave
	}
	return &rollout.Rollout{
		ID:             sr.ID,
		ReleaseID:      sr.ReleaseID,
		Config:         rollout.Config{Strategy: rollout.Strategy(sr.Strategy)},
		Status:         rollout.Status(sr.Status),
		CurrentWave:    currentWave,
		AgentsTotal:    sr.AgentsTotal,
		AgentsPending:  sr.AgentsPending,
		AgentsUpdating: sr.AgentsUpdating,
		AgentsUpdated:  sr.AgentsUpdated,
		AgentsFailed:   sr.AgentsFailed,
		AgentsSkipped:  sr.AgentsSkipped,
		StartedAt:      sr.StartedAt,
		PausedAt:       sr.PausedAt,
		CompletedAt:    sr.CompletedAt,
		RolledBackAt:   sr.RolledBackAt,
		RollbackReason: sr.RollbackReason,
		CreatedBy:      sr.CreatedBy,
		CreatedAt:      sr.CreatedAt,
		UpdatedAt:      sr.UpdatedAt,
	}
}

// =============================================================================
// STATE WORKER STORE ADAPTER
// =============================================================================

// storeStateAdapter implements worker.StateStore using store.Store.
type storeStateAdapter struct {
	db *store.Store
}

func (a *storeStateAdapter) GetTargetsForDownTransition(ctx context.Context, threshold time.Duration) ([]types.Target, error) {
	return a.db.GetTargetsForDownTransition(ctx, threshold)
}

func (a *storeStateAdapter) GetTargetsForUnresponsiveTransition(ctx context.Context, threshold time.Duration) ([]types.Target, error) {
	return a.db.GetTargetsForUnresponsiveTransition(ctx, threshold)
}

func (a *storeStateAdapter) GetTargetsForExcludedTransition(ctx context.Context, threshold time.Duration) ([]types.Target, error) {
	return a.db.GetTargetsForExcludedTransition(ctx, threshold)
}

func (a *storeStateAdapter) GetTargetsForSmartRecheck(ctx context.Context) ([]types.Target, error) {
	return a.db.GetTargetsForSmartRecheck(ctx)
}

func (a *storeStateAdapter) GetTargetsForBaselineCheck(ctx context.Context, threshold time.Duration) ([]types.Target, error) {
	return a.db.GetTargetsForBaselineCheck(ctx, threshold)
}

func (a *storeStateAdapter) TransitionTargetState(ctx context.Context, targetID string, newState types.MonitoringState, reason, triggeredBy string) error {
	return a.db.TransitionTargetState(ctx, targetID, newState, reason, triggeredBy)
}

func (a *storeStateAdapter) SetTargetTier(ctx context.Context, targetID, tier string) error {
	return a.db.SetTargetTier(ctx, targetID, tier)
}

func (a *storeStateAdapter) SetTargetBaseline(ctx context.Context, targetID string) error {
	return a.db.SetTargetBaseline(ctx, targetID)
}
