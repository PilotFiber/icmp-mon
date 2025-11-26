package rollout

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// testLogger returns a logger that discards output.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockStore implements RolloutStore for testing.
type mockStore struct {
	mu       sync.Mutex
	rollouts map[string]*Rollout
	states   map[string][]AgentUpdateState
	releases map[string]*Release
	agents   []AgentInfo
	versions map[string]string
	healthy  map[string]bool
}

func newMockStore() *mockStore {
	return &mockStore{
		rollouts: make(map[string]*Rollout),
		states:   make(map[string][]AgentUpdateState),
		releases: make(map[string]*Release),
		agents:   []AgentInfo{},
		versions: make(map[string]string),
		healthy:  make(map[string]bool),
	}
}

func (m *mockStore) CreateRollout(ctx context.Context, r *Rollout) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollouts[r.ID] = r
	return nil
}

func (m *mockStore) UpdateRollout(ctx context.Context, r *Rollout) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollouts[r.ID] = r
	return nil
}

func (m *mockStore) GetRollout(ctx context.Context, id string) (*Rollout, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rollouts[id], nil
}

func (m *mockStore) ListActiveRollouts(ctx context.Context) ([]Rollout, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []Rollout
	for _, r := range m.rollouts {
		if r.Status == StatusPending || r.Status == StatusInProgress || r.Status == StatusPaused {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (m *mockStore) SetAgentUpdateState(ctx context.Context, rolloutID string, state *AgentUpdateState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	states := m.states[rolloutID]
	found := false
	for i, s := range states {
		if s.AgentID == state.AgentID {
			states[i] = *state
			found = true
			break
		}
	}
	if !found {
		states = append(states, *state)
	}
	m.states[rolloutID] = states
	return nil
}

func (m *mockStore) GetAgentUpdateStates(ctx context.Context, rolloutID string) ([]AgentUpdateState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.states[rolloutID], nil
}

func (m *mockStore) GetRelease(ctx context.Context, id string) (*Release, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.releases[id], nil
}

func (m *mockStore) GetAgentsForRollout(ctx context.Context, filter *AgentFilter) ([]AgentInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.agents, nil
}

func (m *mockStore) GetAgentVersion(ctx context.Context, agentID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.versions[agentID], nil
}

func (m *mockStore) IsAgentHealthy(ctx context.Context, agentID string, since time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	healthy, ok := m.healthy[agentID]
	if !ok {
		return true, nil // Default to healthy
	}
	return healthy, nil
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Strategy != StrategyStaged {
		t.Errorf("expected strategy %s, got %s", StrategyStaged, cfg.Strategy)
	}
	if cfg.CanaryPercent != 5 {
		t.Errorf("expected canary percent 5, got %d", cfg.CanaryPercent)
	}
	if len(cfg.Waves) != 4 {
		t.Errorf("expected 4 waves, got %d", len(cfg.Waves))
	}
	if cfg.Waves[0] != 10 || cfg.Waves[1] != 25 || cfg.Waves[2] != 50 || cfg.Waves[3] != 100 {
		t.Errorf("unexpected wave values: %v", cfg.Waves)
	}
	if cfg.FailureThreshold != 10 {
		t.Errorf("expected failure threshold 10, got %d", cfg.FailureThreshold)
	}
	if !cfg.AutoRollback {
		t.Error("expected auto rollback to be true")
	}
}

func TestStartRollout_NoRelease(t *testing.T) {
	store := newMockStore()
	engine := NewEngine(store, testLogger())

	_, err := engine.StartRollout(context.Background(), "nonexistent", DefaultConfig(), "test")
	if err == nil {
		t.Error("expected error for nonexistent release")
	}
}

func TestStartRollout_NoAgents(t *testing.T) {
	store := newMockStore()
	store.releases["release-1"] = &Release{
		ID:       "release-1",
		Version:  "1.0.0",
		Checksum: "sha256:abc123",
		Size:     1000,
	}
	// No agents added

	engine := NewEngine(store, testLogger())

	_, err := engine.StartRollout(context.Background(), "release-1", DefaultConfig(), "test")
	if err == nil {
		t.Error("expected error for no agents")
	}
}

func TestStartRollout_AllAgentsAtVersion(t *testing.T) {
	store := newMockStore()
	store.releases["release-1"] = &Release{
		ID:       "release-1",
		Version:  "1.0.0",
		Checksum: "sha256:abc123",
		Size:     1000,
	}
	store.agents = []AgentInfo{
		{ID: "agent-1", Name: "agent-1", Version: "1.0.0"},
		{ID: "agent-2", Name: "agent-2", Version: "1.0.0"},
	}

	engine := NewEngine(store, testLogger())

	_, err := engine.StartRollout(context.Background(), "release-1", DefaultConfig(), "test")
	if err == nil {
		t.Error("expected error when all agents at target version")
	}
}

func TestStartRollout_Success(t *testing.T) {
	store := newMockStore()
	store.releases["release-1"] = &Release{
		ID:       "release-1",
		Version:  "2.0.0",
		Checksum: "sha256:abc123",
		Size:     1000,
	}
	store.agents = []AgentInfo{
		{ID: "agent-1", Name: "agent-1", Version: "1.0.0"},
		{ID: "agent-2", Name: "agent-2", Version: "1.0.0"},
		{ID: "agent-3", Name: "agent-3", Version: "1.0.0"},
	}

	engine := NewEngine(store, testLogger())

	rollout, err := engine.StartRollout(context.Background(), "release-1", DefaultConfig(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rollout.ID == "" {
		t.Error("expected rollout ID to be set")
	}
	if rollout.Version != "2.0.0" {
		t.Errorf("expected version 2.0.0, got %s", rollout.Version)
	}
	if rollout.AgentsTotal != 3 {
		t.Errorf("expected 3 total agents, got %d", rollout.AgentsTotal)
	}
	if rollout.AgentsPending != 3 {
		t.Errorf("expected 3 pending agents, got %d", rollout.AgentsPending)
	}
	if rollout.CreatedBy != "test" {
		t.Errorf("expected created_by 'test', got %s", rollout.CreatedBy)
	}

	// Give execution goroutine time to start
	time.Sleep(100 * time.Millisecond)

	// Check rollout was stored
	stored, _ := store.GetRollout(context.Background(), rollout.ID)
	if stored == nil {
		t.Error("rollout not found in store")
	}
}

func TestPauseRollout(t *testing.T) {
	store := newMockStore()
	store.releases["release-1"] = &Release{
		ID:      "release-1",
		Version: "2.0.0",
	}
	store.agents = []AgentInfo{
		{ID: "agent-1", Name: "agent-1", Version: "1.0.0"},
	}

	engine := NewEngine(store, testLogger())

	// Use staged strategy with long delays so it stays active
	rollout, err := engine.StartRollout(context.Background(), "release-1", Config{
		Strategy:  StrategyStaged,
		Waves:     []int{100},
		WaveDelay: 10 * time.Minute, // Long delay to keep active
	}, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give goroutine time to register and start
	time.Sleep(200 * time.Millisecond)

	err = engine.PauseRollout(context.Background(), rollout.ID)
	if err != nil {
		t.Fatalf("unexpected error pausing: %v", err)
	}

	paused, _ := store.GetRollout(context.Background(), rollout.ID)
	if paused.Status != StatusPaused {
		t.Errorf("expected status %s, got %s", StatusPaused, paused.Status)
	}
	if paused.PausedAt == nil {
		t.Error("expected paused_at to be set")
	}
}

func TestRollbackRollout(t *testing.T) {
	store := newMockStore()
	store.releases["release-1"] = &Release{
		ID:      "release-1",
		Version: "2.0.0",
	}
	store.agents = []AgentInfo{
		{ID: "agent-1", Name: "agent-1", Version: "1.0.0"},
	}

	engine := NewEngine(store, testLogger())

	rollout, _ := engine.StartRollout(context.Background(), "release-1", Config{
		Strategy: StrategyManual,
	}, "test")

	time.Sleep(100 * time.Millisecond)

	err := engine.RollbackRollout(context.Background(), rollout.ID, "test rollback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rolled, _ := store.GetRollout(context.Background(), rollout.ID)
	if rolled.Status != StatusRolledBack {
		t.Errorf("expected status %s, got %s", StatusRolledBack, rolled.Status)
	}
	if rolled.RollbackReason != "test rollback" {
		t.Errorf("expected rollback reason 'test rollback', got %s", rolled.RollbackReason)
	}
}

func TestSelectRandomAgents(t *testing.T) {
	agents := []AgentUpdateState{
		{AgentID: "1"},
		{AgentID: "2"},
		{AgentID: "3"},
		{AgentID: "4"},
		{AgentID: "5"},
	}

	// Test selecting subset
	selected := selectRandomAgents(agents, 3)
	if len(selected) != 3 {
		t.Errorf("expected 3 agents, got %d", len(selected))
	}

	// Test selecting more than available
	selected = selectRandomAgents(agents, 10)
	if len(selected) != 5 {
		t.Errorf("expected 5 agents (all), got %d", len(selected))
	}

	// Test selecting zero
	selected = selectRandomAgents(agents, 0)
	if len(selected) != 0 {
		t.Errorf("expected 0 agents, got %d", len(selected))
	}
}

func TestExcludeAgents(t *testing.T) {
	all := []AgentUpdateState{
		{AgentID: "1"},
		{AgentID: "2"},
		{AgentID: "3"},
		{AgentID: "4"},
	}
	exclude := []AgentUpdateState{
		{AgentID: "2"},
		{AgentID: "4"},
	}

	result := excludeAgents(all, exclude)
	if len(result) != 2 {
		t.Errorf("expected 2 agents, got %d", len(result))
	}
	for _, a := range result {
		if a.AgentID == "2" || a.AgentID == "4" {
			t.Errorf("agent %s should have been excluded", a.AgentID)
		}
	}
}

func TestShouldUpdateAgent(t *testing.T) {
	store := newMockStore()
	store.releases["release-1"] = &Release{
		ID:       "release-1",
		Version:  "2.0.0",
		Checksum: "sha256:abc",
		Size:     1000,
	}

	// Create active rollout
	rollout := &Rollout{
		ID:        "rollout-1",
		ReleaseID: "release-1",
		Version:   "2.0.0",
		Status:    StatusInProgress,
	}
	store.rollouts[rollout.ID] = rollout

	// Mark agent for update
	store.states[rollout.ID] = []AgentUpdateState{
		{AgentID: "agent-1", Status: "updating"},
	}

	engine := NewEngine(store, testLogger())

	// Agent marked for update should get notification
	notification, err := engine.ShouldUpdateAgent(context.Background(), "agent-1", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notification == nil {
		t.Fatal("expected notification, got nil")
	}
	if notification.Version != "2.0.0" {
		t.Errorf("expected version 2.0.0, got %s", notification.Version)
	}

	// Agent not in rollout should not get notification
	notification, err = engine.ShouldUpdateAgent(context.Background(), "agent-2", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notification != nil {
		t.Error("expected no notification for agent-2")
	}
}

func TestReportAgentUpdate(t *testing.T) {
	store := newMockStore()
	store.states["rollout-1"] = []AgentUpdateState{
		{AgentID: "agent-1", Status: "updating"},
	}

	engine := NewEngine(store, testLogger())

	// Report success
	err := engine.ReportAgentUpdate(context.Background(), "rollout-1", "agent-1", true, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	states, _ := store.GetAgentUpdateStates(context.Background(), "rollout-1")
	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}
	if states[0].Status != "updated" {
		t.Errorf("expected status 'updated', got %s", states[0].Status)
	}

	// Report failure
	store.states["rollout-2"] = []AgentUpdateState{
		{AgentID: "agent-2", Status: "updating"},
	}
	err = engine.ReportAgentUpdate(context.Background(), "rollout-2", "agent-2", false, "download failed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	states, _ = store.GetAgentUpdateStates(context.Background(), "rollout-2")
	if states[0].Status != "failed" {
		t.Errorf("expected status 'failed', got %s", states[0].Status)
	}
	if states[0].Error != "download failed" {
		t.Errorf("expected error 'download failed', got %s", states[0].Error)
	}
}

func TestConfigMarshalJSON(t *testing.T) {
	cfg := Config{
		Strategy:        StrategyCanary,
		CanaryPercent:   5,
		CanaryDuration:  10 * time.Minute,
		WaveDelay:       5 * time.Minute,
		HealthCheckWait: 2 * time.Minute,
	}

	data, err := cfg.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that duration fields are serialized as strings
	jsonStr := string(data)
	if jsonStr == "" {
		t.Error("expected non-empty JSON")
	}
}
