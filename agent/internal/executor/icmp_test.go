package executor

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"
)

func TestICMPExecutor_Type(t *testing.T) {
	e := NewICMPExecutor()
	if e.Type() != "icmp_ping" {
		t.Errorf("expected type 'icmp_ping', got '%s'", e.Type())
	}
}

func TestICMPExecutor_Capabilities(t *testing.T) {
	e := NewICMPExecutor()
	caps := e.Capabilities()

	if !caps.SupportsBatching {
		t.Error("expected batching support")
	}
	if caps.MaxBatchSize != 500 {
		t.Errorf("expected max batch size 500, got %d", caps.MaxBatchSize)
	}
	if caps.RequiresRoot {
		t.Error("should not require root")
	}
	if len(caps.Dependencies) != 1 || caps.Dependencies[0] != "fping" {
		t.Errorf("expected fping dependency, got %v", caps.Dependencies)
	}
}

func TestICMPExecutor_ParseRTTValues(t *testing.T) {
	e := NewICMPExecutor()

	tests := []struct {
		name           string
		input          string
		wantReachable  bool
		wantLoss       float64
		wantMinMs      float64
		wantMaxMs      float64
		wantAvgMs      float64
		wantPackets    int
		wantRecvd      int
	}{
		{
			name:          "all successful",
			input:         "12.45 13.22 11.80",
			wantReachable: true,
			wantLoss:      0.0,
			wantMinMs:     11.80,
			wantMaxMs:     13.22,
			wantPackets:   3,
			wantRecvd:     3,
		},
		{
			name:          "partial loss",
			input:         "12.45 - 11.80",
			wantReachable: true,
			wantLoss:      33.33333333333333,
			wantMinMs:     11.80,
			wantMaxMs:     12.45,
			wantPackets:   3,
			wantRecvd:     2,
		},
		{
			name:          "all failed",
			input:         "- - -",
			wantReachable: false,
			wantLoss:      100.0,
			wantPackets:   3,
			wantRecvd:     0,
		},
		{
			name:          "single success",
			input:         "5.5",
			wantReachable: true,
			wantLoss:      0.0,
			wantMinMs:     5.5,
			wantMaxMs:     5.5,
			wantPackets:   1,
			wantRecvd:     1,
		},
		{
			name:          "high latency",
			input:         "150.25 200.50 175.75",
			wantReachable: true,
			wantLoss:      0.0,
			wantMinMs:     150.25,
			wantMaxMs:     200.50,
			wantPackets:   3,
			wantRecvd:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := e.parseRTTValues(tt.input)

			if payload.Reachable != tt.wantReachable {
				t.Errorf("reachable: got %v, want %v", payload.Reachable, tt.wantReachable)
			}
			if payload.PacketsSent != tt.wantPackets {
				t.Errorf("packets sent: got %d, want %d", payload.PacketsSent, tt.wantPackets)
			}
			if payload.PacketsRecvd != tt.wantRecvd {
				t.Errorf("packets recvd: got %d, want %d", payload.PacketsRecvd, tt.wantRecvd)
			}
			if !floatClose(payload.PacketLoss, tt.wantLoss, 0.01) {
				t.Errorf("packet loss: got %f, want %f", payload.PacketLoss, tt.wantLoss)
			}
			if tt.wantReachable {
				if !floatClose(payload.MinMs, tt.wantMinMs, 0.01) {
					t.Errorf("min ms: got %f, want %f", payload.MinMs, tt.wantMinMs)
				}
				if !floatClose(payload.MaxMs, tt.wantMaxMs, 0.01) {
					t.Errorf("max ms: got %f, want %f", payload.MaxMs, tt.wantMaxMs)
				}
			}
		})
	}
}

func TestICMPExecutor_ParseOutput(t *testing.T) {
	e := NewICMPExecutor()

	ipToTarget := map[string]ProbeTarget{
		"8.8.8.8":   {ID: "google-dns", IP: "8.8.8.8"},
		"1.1.1.1":   {ID: "cloudflare", IP: "1.1.1.1"},
		"10.0.0.99": {ID: "internal", IP: "10.0.0.99"},
	}

	// Simulated fping output
	output := []byte(`8.8.8.8  : 12.45 13.22 11.80
1.1.1.1  : 5.5 - 6.2
10.0.0.99 : - - -
`)

	timestamp := time.Now()
	results := e.parseOutput(output, ipToTarget, timestamp)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Find results by target ID
	resultMap := make(map[string]*Result)
	for _, r := range results {
		resultMap[r.TargetID] = r
	}

	// Check google-dns (all success)
	r := resultMap["google-dns"]
	if r == nil {
		t.Fatal("missing google-dns result")
	}
	if !r.Success {
		t.Error("google-dns should be successful")
	}
	var payload ICMPPayload
	json.Unmarshal(r.Payload, &payload)
	if payload.PacketLoss != 0 {
		t.Errorf("google-dns packet loss: got %f, want 0", payload.PacketLoss)
	}

	// Check cloudflare (partial loss)
	r = resultMap["cloudflare"]
	if r == nil {
		t.Fatal("missing cloudflare result")
	}
	if !r.Success {
		t.Error("cloudflare should be successful (partial loss)")
	}
	json.Unmarshal(r.Payload, &payload)
	if payload.PacketLoss < 30 || payload.PacketLoss > 35 {
		t.Errorf("cloudflare packet loss: got %f, want ~33", payload.PacketLoss)
	}

	// Check internal (all failed)
	r = resultMap["internal"]
	if r == nil {
		t.Fatal("missing internal result")
	}
	if r.Success {
		t.Error("internal should not be successful")
	}
	json.Unmarshal(r.Payload, &payload)
	if payload.PacketLoss != 100 {
		t.Errorf("internal packet loss: got %f, want 100", payload.PacketLoss)
	}
}

func TestICMPExecutor_ParseParams(t *testing.T) {
	e := NewICMPExecutor()

	tests := []struct {
		name      string
		input     json.RawMessage
		wantCount int
		wantInt   int
	}{
		{
			name:      "nil params",
			input:     nil,
			wantCount: 0,
			wantInt:   0,
		},
		{
			name:      "empty params",
			input:     json.RawMessage(`{}`),
			wantCount: 0,
			wantInt:   0,
		},
		{
			name:      "count only",
			input:     json.RawMessage(`{"count": 5}`),
			wantCount: 5,
			wantInt:   0,
		},
		{
			name:      "interval only",
			input:     json.RawMessage(`{"interval_ms": 200}`),
			wantCount: 0,
			wantInt:   200,
		},
		{
			name:      "both params",
			input:     json.RawMessage(`{"count": 10, "interval_ms": 50}`),
			wantCount: 10,
			wantInt:   50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := e.parseParams(tt.input)
			if params.Count != tt.wantCount {
				t.Errorf("count: got %d, want %d", params.Count, tt.wantCount)
			}
			if params.IntervalMs != tt.wantInt {
				t.Errorf("interval: got %d, want %d", params.IntervalMs, tt.wantInt)
			}
		})
	}
}

func TestICMPExecutor_ErrorMessage(t *testing.T) {
	e := NewICMPExecutor()

	tests := []struct {
		name    string
		payload ICMPPayload
		want    string
	}{
		{
			name:    "reachable",
			payload: ICMPPayload{Reachable: true},
			want:    "",
		},
		{
			name:    "total loss",
			payload: ICMPPayload{Reachable: false, PacketsRecvd: 0, PacketsSent: 3},
			want:    "100% packet loss (3 packets sent)",
		},
		{
			name:    "partial loss",
			payload: ICMPPayload{Reachable: false, PacketLoss: 66.7, PacketsRecvd: 1},
			want:    "66.7% packet loss",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.errorMessage(tt.payload)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestICMPExecutor_ExecuteBatch_Empty(t *testing.T) {
	e := NewICMPExecutor()

	results, err := e.ExecuteBatch(context.Background(), []ProbeTarget{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// Integration test - requires fping to be installed
func TestICMPExecutor_Integration(t *testing.T) {
	// Skip if fping not available
	if _, err := exec.LookPath("fping"); err != nil {
		t.Skip("fping not installed, skipping integration test")
	}

	e := NewICMPExecutor()

	// Test against localhost (should always work)
	targets := []ProbeTarget{
		{
			ID:      "localhost",
			IP:      "127.0.0.1",
			Timeout: 2 * time.Second,
			Params:  json.RawMessage(`{"count": 2}`),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results, err := e.ExecuteBatch(ctx, targets)
	if err != nil {
		t.Fatalf("ExecuteBatch failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.TargetID != "localhost" {
		t.Errorf("wrong target ID: %s", r.TargetID)
	}
	if !r.Success {
		t.Errorf("localhost should be reachable: %s", r.Error)
	}

	var payload ICMPPayload
	if err := json.Unmarshal(r.Payload, &payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}

	if !payload.Reachable {
		t.Error("payload should show reachable")
	}
	if payload.PacketLoss != 0 {
		t.Errorf("expected 0%% packet loss, got %f%%", payload.PacketLoss)
	}
	if payload.AvgMs <= 0 {
		t.Error("expected positive latency")
	}

	t.Logf("Localhost latency: avg=%.2fms min=%.2fms max=%.2fms",
		payload.AvgMs, payload.MinMs, payload.MaxMs)
}

// Integration test with multiple targets
func TestICMPExecutor_Integration_MultipleTargets(t *testing.T) {
	if _, err := exec.LookPath("fping"); err != nil {
		t.Skip("fping not installed, skipping integration test")
	}

	e := NewICMPExecutor()

	targets := []ProbeTarget{
		{ID: "localhost", IP: "127.0.0.1", Timeout: 2 * time.Second},
		{ID: "google-dns", IP: "8.8.8.8", Timeout: 2 * time.Second},
		// Using a non-routable IP to test failure case
		{ID: "unreachable", IP: "192.0.2.1", Timeout: 1 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	results, err := e.ExecuteBatch(ctx, targets)
	if err != nil {
		t.Fatalf("ExecuteBatch failed: %v", err)
	}

	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	// Log results
	for _, r := range results {
		var payload ICMPPayload
		json.Unmarshal(r.Payload, &payload)
		t.Logf("Target %s: success=%v latency=%.2fms loss=%.1f%%",
			r.TargetID, r.Success, payload.AvgMs, payload.PacketLoss)
	}
}

// Helper function for float comparison
func floatClose(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}
