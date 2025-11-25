package executor

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// MockExecutor is a test executor for unit tests.
type MockExecutor struct {
	TypeName     string
	Caps         Capabilities
	ExecuteFunc  func(ctx context.Context, target ProbeTarget) (*Result, error)
	BatchFunc    func(ctx context.Context, targets []ProbeTarget) ([]*Result, error)
}

func (m *MockExecutor) Type() string {
	return m.TypeName
}

func (m *MockExecutor) Capabilities() Capabilities {
	return m.Caps
}

func (m *MockExecutor) Execute(ctx context.Context, target ProbeTarget) (*Result, error) {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, target)
	}
	return &Result{
		TargetID:  target.ID,
		Timestamp: time.Now(),
		Success:   true,
		Payload:   json.RawMessage(`{}`),
	}, nil
}

func (m *MockExecutor) ExecuteBatch(ctx context.Context, targets []ProbeTarget) ([]*Result, error) {
	if m.BatchFunc != nil {
		return m.BatchFunc(ctx, targets)
	}
	// Default: call Execute for each
	results := make([]*Result, len(targets))
	for i, t := range targets {
		r, err := m.Execute(ctx, t)
		if err != nil {
			return nil, err
		}
		results[i] = r
	}
	return results, nil
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	exec := &MockExecutor{
		TypeName: "test_ping",
		Caps: Capabilities{
			SupportsBatching: true,
			MaxBatchSize:     100,
		},
	}

	// First registration should succeed
	err := r.Register(exec)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Duplicate registration should fail
	err = r.Register(exec)
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()

	exec := &MockExecutor{TypeName: "test_ping"}
	r.Register(exec)

	// Should find registered executor
	found, ok := r.Get("test_ping")
	if !ok {
		t.Fatal("expected to find executor")
	}
	if found.Type() != "test_ping" {
		t.Fatalf("wrong executor type: %s", found.Type())
	}

	// Should not find unregistered executor
	_, ok = r.Get("nonexistent")
	if ok {
		t.Fatal("should not find nonexistent executor")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()

	r.Register(&MockExecutor{TypeName: "ping"})
	r.Register(&MockExecutor{TypeName: "mtr"})
	r.Register(&MockExecutor{TypeName: "tcp"})

	types := r.List()
	if len(types) != 3 {
		t.Fatalf("expected 3 executors, got %d", len(types))
	}

	// Check all types are present (order not guaranteed)
	typeSet := make(map[string]bool)
	for _, typ := range types {
		typeSet[typ] = true
	}
	for _, expected := range []string{"ping", "mtr", "tcp"} {
		if !typeSet[expected] {
			t.Errorf("missing executor type: %s", expected)
		}
	}
}

func TestRegistry_ListCapabilities(t *testing.T) {
	r := NewRegistry()

	r.Register(&MockExecutor{
		TypeName: "ping",
		Caps: Capabilities{
			SupportsBatching: true,
			MaxBatchSize:     500,
			Dependencies:     []string{},
		},
	})
	r.Register(&MockExecutor{
		TypeName: "mtr",
		Caps: Capabilities{
			SupportsBatching: false,
			Dependencies:     []string{},
		},
	})

	caps := r.ListCapabilities()
	if len(caps) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(caps))
	}

	pingCaps, ok := caps["ping"]
	if !ok {
		t.Fatal("missing ping capabilities")
	}
	if !pingCaps.SupportsBatching {
		t.Error("ping should support batching")
	}
	if pingCaps.MaxBatchSize != 500 {
		t.Errorf("wrong max batch size: %d", pingCaps.MaxBatchSize)
	}

	mtrCaps, ok := caps["mtr"]
	if !ok {
		t.Fatal("missing mtr capabilities")
	}
	if mtrCaps.SupportsBatching {
		t.Error("mtr should not support batching")
	}
}

func TestMockExecutor_Execute(t *testing.T) {
	exec := &MockExecutor{
		TypeName: "test",
		ExecuteFunc: func(ctx context.Context, target ProbeTarget) (*Result, error) {
			return &Result{
				TargetID:  target.ID,
				Timestamp: time.Now(),
				Success:   true,
				Payload:   MarshalPayload(map[string]float64{"latency_ms": 12.5}),
			}, nil
		},
	}

	result, err := exec.Execute(context.Background(), ProbeTarget{
		ID:      "test-target",
		IP:      "8.8.8.8",
		Timeout: time.Second,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TargetID != "test-target" {
		t.Errorf("wrong target ID: %s", result.TargetID)
	}
	if !result.Success {
		t.Error("expected success")
	}

	// Verify payload
	var payload map[string]float64
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if payload["latency_ms"] != 12.5 {
		t.Errorf("wrong latency: %f", payload["latency_ms"])
	}
}

func TestMockExecutor_ExecuteBatch(t *testing.T) {
	callCount := 0
	exec := &MockExecutor{
		TypeName: "test",
		BatchFunc: func(ctx context.Context, targets []ProbeTarget) ([]*Result, error) {
			callCount++
			results := make([]*Result, len(targets))
			for i, target := range targets {
				results[i] = &Result{
					TargetID:  target.ID,
					Timestamp: time.Now(),
					Success:   true,
					Payload:   json.RawMessage(`{}`),
				}
			}
			return results, nil
		},
	}

	targets := []ProbeTarget{
		{ID: "t1", IP: "1.1.1.1", Timeout: time.Second},
		{ID: "t2", IP: "8.8.8.8", Timeout: time.Second},
		{ID: "t3", IP: "9.9.9.9", Timeout: time.Second},
	}

	results, err := exec.ExecuteBatch(context.Background(), targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if callCount != 1 {
		t.Errorf("expected 1 batch call, got %d", callCount)
	}

	// Verify each result
	for i, result := range results {
		if result.TargetID != targets[i].ID {
			t.Errorf("result %d: wrong target ID", i)
		}
	}
}

func TestMarshalPayload(t *testing.T) {
	type TestPayload struct {
		LatencyMs float64 `json:"latency_ms"`
		Success   bool    `json:"success"`
	}

	payload := TestPayload{LatencyMs: 15.3, Success: true}
	raw := MarshalPayload(payload)

	// Should be valid JSON
	var decoded TestPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded.LatencyMs != 15.3 {
		t.Errorf("wrong latency: %f", decoded.LatencyMs)
	}
	if !decoded.Success {
		t.Error("expected success")
	}
}

func TestUnmarshalPayload(t *testing.T) {
	type TestPayload struct {
		LatencyMs float64 `json:"latency_ms"`
		Success   bool    `json:"success"`
	}

	raw := json.RawMessage(`{"latency_ms": 25.7, "success": false}`)

	payload, err := UnmarshalPayload[TestPayload](raw)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if payload.LatencyMs != 25.7 {
		t.Errorf("wrong latency: %f", payload.LatencyMs)
	}
	if payload.Success {
		t.Error("expected failure")
	}
}

func TestProbeTarget_Fields(t *testing.T) {
	target := ProbeTarget{
		ID:      "target-123",
		IP:      "192.168.1.1",
		Timeout: 5 * time.Second,
		Retries: 3,
		Params:  json.RawMessage(`{"port": 22}`),
	}

	if target.ID != "target-123" {
		t.Errorf("wrong ID: %s", target.ID)
	}
	if target.IP != "192.168.1.1" {
		t.Errorf("wrong IP: %s", target.IP)
	}
	if target.Timeout != 5*time.Second {
		t.Errorf("wrong timeout: %v", target.Timeout)
	}
	if target.Retries != 3 {
		t.Errorf("wrong retries: %d", target.Retries)
	}
}

func TestResult_Fields(t *testing.T) {
	now := time.Now()
	result := Result{
		TargetID:  "target-456",
		Timestamp: now,
		Duration:  150 * time.Millisecond,
		Success:   false,
		Error:     "connection timeout",
		Payload:   json.RawMessage(`{"details": "test"}`),
	}

	if result.TargetID != "target-456" {
		t.Errorf("wrong target ID: %s", result.TargetID)
	}
	if result.Timestamp != now {
		t.Error("wrong timestamp")
	}
	if result.Duration != 150*time.Millisecond {
		t.Errorf("wrong duration: %v", result.Duration)
	}
	if result.Success {
		t.Error("should not be successful")
	}
	if result.Error != "connection timeout" {
		t.Errorf("wrong error: %s", result.Error)
	}
}
