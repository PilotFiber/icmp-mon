// Package executor - MTR executor for on-demand path tracing.
//
// # Why MTR?
//
// MTR (My Traceroute) combines the functionality of ping and traceroute:
//
// - Shows the complete network path to a target
// - Provides latency and packet loss statistics for each hop
// - Helps identify where network problems are occurring
//
// This executor is designed for on-demand use when investigating connectivity
// issues, not for continuous monitoring.
//
// # Output Parsing
//
// mtr --report-wide --report-cycles=10 --json outputs structured JSON data
// with information about each hop in the path.
//
// # Installation
//
//	Ubuntu/Debian: apt-get install mtr-tiny
//	RHEL/CentOS:   yum install mtr
//	macOS:         brew install mtr
package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// MTRExecutor runs MTR path traces.
type MTRExecutor struct {
	// MTRPath is the path to the mtr binary. Default: "mtr"
	MTRPath string

	// DefaultCycles is the number of probe cycles per hop. Default: 10
	DefaultCycles int
}

// NewMTRExecutor creates a new MTR executor with sensible defaults.
func NewMTRExecutor() *MTRExecutor {
	return &MTRExecutor{
		MTRPath:       "mtr",
		DefaultCycles: 10,
	}
}

// MTRParams are executor-specific parameters for MTR probes.
type MTRParams struct {
	Cycles int `json:"cycles,omitempty"` // Number of cycles per hop (default: 10)
}

// MTRPayload contains the results of an MTR probe.
type MTRPayload struct {
	Target     string    `json:"target"`
	Hops       []MTRHop  `json:"hops"`
	TotalHops  int       `json:"total_hops"`
	ReachedDst bool      `json:"reached_dst"`
	DstLatency float64   `json:"dst_latency_ms,omitempty"`
	RawOutput  string    `json:"raw_output,omitempty"`
}

// MTRHop contains statistics for a single hop.
type MTRHop struct {
	Number     int     `json:"number"`
	Host       string  `json:"host"`
	IP         string  `json:"ip,omitempty"`
	Loss       float64 `json:"loss_pct"`
	Sent       int     `json:"sent"`
	Received   int     `json:"received"`
	AvgLatency float64 `json:"avg_ms"`
	BestLatency float64 `json:"best_ms"`
	WorstLatency float64 `json:"worst_ms"`
	StdDev     float64 `json:"stddev_ms"`
}

// Type returns the executor type identifier.
func (e *MTRExecutor) Type() string {
	return "mtr"
}

// Capabilities returns what this executor can do.
func (e *MTRExecutor) Capabilities() Capabilities {
	return Capabilities{
		SupportsBatching: false, // MTR runs one target at a time
		MaxBatchSize:     1,
		RequiresRoot:     false,
		Dependencies:     []string{"mtr"},
	}
}

// Execute runs an MTR trace to a single target.
func (e *MTRExecutor) Execute(ctx context.Context, target ProbeTarget) (*Result, error) {
	params := e.parseParams(target.Params)
	timeout := target.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	start := time.Now()

	// Run mtr with JSON output
	output, err := e.runMTR(ctx, target.IP, params, timeout)
	duration := time.Since(start)

	if err != nil && len(output) == 0 {
		return &Result{
			TargetID:  target.ID,
			Timestamp: start,
			Duration:  duration,
			Success:   false,
			Error:     fmt.Sprintf("mtr failed: %v", err),
			Payload:   MarshalPayload(MTRPayload{Target: target.IP}),
		}, nil
	}

	// Parse the JSON output
	payload := e.parseOutput(target.IP, output)

	return &Result{
		TargetID:  target.ID,
		Timestamp: start,
		Duration:  duration,
		Success:   payload.ReachedDst,
		Error:     e.errorMessage(payload),
		Payload:   MarshalPayload(payload),
	}, nil
}

// ExecuteBatch runs MTR for multiple targets (one at a time).
func (e *MTRExecutor) ExecuteBatch(ctx context.Context, targets []ProbeTarget) ([]*Result, error) {
	results := make([]*Result, 0, len(targets))
	for _, target := range targets {
		result, err := e.Execute(ctx, target)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

// runMTR executes mtr and returns the raw output.
func (e *MTRExecutor) runMTR(ctx context.Context, ip string, params MTRParams, timeout time.Duration) ([]byte, error) {
	mtrPath := e.MTRPath
	if mtrPath == "" {
		mtrPath = "mtr"
	}

	cycles := params.Cycles
	if cycles <= 0 {
		cycles = e.DefaultCycles
	}

	// Build command arguments
	// --report-wide : Report mode with wide output
	// --report-cycles : Number of pings per hop
	// --json : Output in JSON format
	// --no-dns : Skip DNS resolution for faster execution (optional)
	args := []string{
		"--report-wide",
		fmt.Sprintf("--report-cycles=%d", cycles),
		"--json",
		ip,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, mtrPath, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Check if it's just a context timeout
		if ctx.Err() != nil {
			return nil, fmt.Errorf("mtr timed out after %v", timeout)
		}
		// mtr might have produced output anyway
		if stdout.Len() > 0 {
			return stdout.Bytes(), nil
		}
		return nil, fmt.Errorf("mtr error: %v, stderr: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// mtrJSONOutput represents the JSON output from mtr --json
type mtrJSONOutput struct {
	Report struct {
		MTR struct {
			Src string `json:"src"`
			Dst string `json:"dst"`
		} `json:"mtr"`
		Hubs []struct {
			Count int     `json:"count"`
			Host  string  `json:"host"`
			Loss  float64 `json:"Loss%"`
			Snt   int     `json:"Snt"`
			Last  float64 `json:"Last"`
			Avg   float64 `json:"Avg"`
			Best  float64 `json:"Best"`
			Wrst  float64 `json:"Wrst"`
			StDev float64 `json:"StDev"`
		} `json:"hubs"`
	} `json:"report"`
}

// parseOutput parses mtr JSON output and returns a structured payload.
func (e *MTRExecutor) parseOutput(targetIP string, output []byte) MTRPayload {
	payload := MTRPayload{
		Target: targetIP,
		Hops:   []MTRHop{},
	}

	// Try to parse JSON output
	var mtrOut mtrJSONOutput
	if err := json.Unmarshal(output, &mtrOut); err != nil {
		// If JSON parsing fails, store raw output
		payload.RawOutput = string(output)
		return payload
	}

	// Convert hubs to our hop format
	for i, hub := range mtrOut.Report.Hubs {
		hop := MTRHop{
			Number:       i + 1,
			Host:         hub.Host,
			Loss:         hub.Loss,
			Sent:         hub.Snt,
			Received:     hub.Snt - int(float64(hub.Snt)*hub.Loss/100),
			AvgLatency:   hub.Avg,
			BestLatency:  hub.Best,
			WorstLatency: hub.Wrst,
			StdDev:       hub.StDev,
		}

		// Check if this might be the destination
		if hub.Host == targetIP || hub.Host == mtrOut.Report.MTR.Dst {
			payload.ReachedDst = true
			payload.DstLatency = hub.Avg
		}

		payload.Hops = append(payload.Hops, hop)
	}

	payload.TotalHops = len(payload.Hops)

	// If last hop has low loss, consider destination reached
	if len(payload.Hops) > 0 {
		lastHop := payload.Hops[len(payload.Hops)-1]
		if lastHop.Loss < 50 && lastHop.Host != "???" {
			payload.ReachedDst = true
			payload.DstLatency = lastHop.AvgLatency
		}
	}

	return payload
}

// parseParams extracts MTR parameters from raw JSON.
func (e *MTRExecutor) parseParams(raw json.RawMessage) MTRParams {
	var params MTRParams
	if len(raw) > 0 {
		json.Unmarshal(raw, &params)
	}
	return params
}

// errorMessage generates an error message for failed traces.
func (e *MTRExecutor) errorMessage(payload MTRPayload) string {
	if payload.ReachedDst {
		return ""
	}
	if len(payload.Hops) == 0 {
		return "no route to host"
	}
	return fmt.Sprintf("destination unreachable after %d hops", payload.TotalHops)
}
