// Package executor - ICMP ping executor using fping.
//
// # Why fping?
//
// fping is designed for bulk pinging and is significantly more efficient than
// running individual ping processes:
//
// - Single process probes hundreds of targets in parallel
// - Configurable packet interval to avoid network congestion
// - Parseable output format for easy result extraction
// - Well-tested and widely available on Linux systems
//
// # Output Parsing
//
// fping -C (count) mode outputs:
//
//	192.168.1.1 : 12.45 13.22 - 11.80
//	192.168.1.2 : - - - -
//
// Where:
// - Each number is a round-trip time in milliseconds
// - "-" indicates a timeout/failure for that probe
//
// # Installation
//
//	Ubuntu/Debian: apt-get install fping
//	RHEL/CentOS:   yum install fping
//	macOS:         brew install fping
package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ICMPExecutor probes targets using fping.
type ICMPExecutor struct {
	// FpingPath is the path to the fping binary. Default: "fping"
	FpingPath string

	// DefaultCount is the number of pings per target. Default: 3
	DefaultCount int

	// DefaultInterval is the interval between pings in milliseconds. Default: 100
	DefaultIntervalMs int
}

// NewICMPExecutor creates a new ICMP executor with sensible defaults.
func NewICMPExecutor() *ICMPExecutor {
	return &ICMPExecutor{
		FpingPath:         "fping",
		DefaultCount:      3,
		DefaultIntervalMs: 100,
	}
}

// ICMPParams are executor-specific parameters for ICMP probes.
type ICMPParams struct {
	Count      int `json:"count,omitempty"`       // Pings per target (default: 3)
	IntervalMs int `json:"interval_ms,omitempty"` // Interval between pings (default: 100)
}

// ICMPPayload contains the results of an ICMP probe.
type ICMPPayload struct {
	Reachable    bool    `json:"reachable"`
	LatencyMs    float64 `json:"latency_ms"`     // Most recent RTT
	MinMs        float64 `json:"min_ms"`
	MaxMs        float64 `json:"max_ms"`
	AvgMs        float64 `json:"avg_ms"`
	StdDevMs     float64 `json:"stddev_ms"`
	PacketLoss   float64 `json:"packet_loss_pct"`
	PacketsSent  int     `json:"packets_sent"`
	PacketsRecvd int     `json:"packets_recvd"`
}

// Type returns the executor type identifier.
func (e *ICMPExecutor) Type() string {
	return "icmp_ping"
}

// Capabilities returns what this executor can do.
func (e *ICMPExecutor) Capabilities() Capabilities {
	return Capabilities{
		SupportsBatching: true,
		MaxBatchSize:     500, // fping handles this well
		RequiresRoot:     false,
		Dependencies:     []string{"fping"},
	}
}

// Execute runs a single ICMP probe.
func (e *ICMPExecutor) Execute(ctx context.Context, target ProbeTarget) (*Result, error) {
	results, err := e.ExecuteBatch(ctx, []ProbeTarget{target})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no results returned for target %s", target.IP)
	}
	return results[0], nil
}

// ExecuteBatch probes multiple targets efficiently using fping.
func (e *ICMPExecutor) ExecuteBatch(ctx context.Context, targets []ProbeTarget) ([]*Result, error) {
	if len(targets) == 0 {
		return []*Result{}, nil
	}

	// Parse params from first target (assume consistent within batch)
	params := e.parseParams(targets[0].Params)
	timeout := targets[0].Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	// Build target IP list
	ips := make([]string, len(targets))
	ipToTarget := make(map[string]ProbeTarget, len(targets))
	for i, t := range targets {
		ips[i] = t.IP
		ipToTarget[t.IP] = t
	}

	// Run fping
	start := time.Now()
	output, err := e.runFping(ctx, ips, params, timeout)
	if err != nil {
		// fping returns non-zero if any host is unreachable, which is normal
		// Only treat as error if we got no output at all
		if len(output) == 0 {
			return nil, fmt.Errorf("fping failed: %w", err)
		}
	}

	// Parse results
	results := e.parseOutput(output, ipToTarget, start)
	return results, nil
}

// runFping executes fping and returns the raw output.
func (e *ICMPExecutor) runFping(ctx context.Context, ips []string, params ICMPParams, timeout time.Duration) ([]byte, error) {
	fpingPath := e.FpingPath
	if fpingPath == "" {
		fpingPath = "fping"
	}

	count := params.Count
	if count <= 0 {
		count = e.DefaultCount
	}

	intervalMs := params.IntervalMs
	if intervalMs <= 0 {
		intervalMs = e.DefaultIntervalMs
	}

	// Build command arguments
	// -C n  : Send n pings to each target
	// -q    : Quiet mode (summary output only)
	// -t ms : Initial timeout in milliseconds
	// -p ms : Interval between pings to same target
	// -B 1  : Backoff multiplier (1 = no exponential backoff)
	args := []string{
		"-C", strconv.Itoa(count),
		"-q",
		"-t", strconv.FormatInt(timeout.Milliseconds(), 10),
		"-p", strconv.Itoa(intervalMs),
		"-B", "1",
	}
	args = append(args, ips...)

	cmd := exec.CommandContext(ctx, fpingPath, args...)

	// fping writes results to stderr (historical quirk)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Run command - ignore error because fping returns non-zero
	// when any host is unreachable (which is expected)
	_ = cmd.Run()

	return stderr.Bytes(), nil
}

// parseOutput parses fping output and returns results.
//
// fping -C output format:
//
//	192.168.1.1 : 12.45 13.22 - 11.80
//
// Where numbers are RTTs in ms and "-" indicates timeout.
func (e *ICMPExecutor) parseOutput(output []byte, ipToTarget map[string]ProbeTarget, timestamp time.Time) []*Result {
	results := make([]*Result, 0, len(ipToTarget))
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		// Parse line: "IP : value value value ..."
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		ip := strings.TrimSpace(parts[0])
		valuesStr := strings.TrimSpace(parts[1])

		target, ok := ipToTarget[ip]
		if !ok {
			continue
		}
		seen[ip] = true

		// Parse RTT values
		payload := e.parseRTTValues(valuesStr)

		results = append(results, &Result{
			TargetID:  target.ID,
			Timestamp: timestamp,
			Success:   payload.Reachable,
			Error:     e.errorMessage(payload),
			Payload:   MarshalPayload(payload),
		})
	}

	// Add results for any targets not in output (complete failure)
	for ip, target := range ipToTarget {
		if !seen[ip] {
			payload := ICMPPayload{
				Reachable:   false,
				PacketLoss:  100.0,
				PacketsSent: e.DefaultCount,
			}
			results = append(results, &Result{
				TargetID:  target.ID,
				Timestamp: timestamp,
				Success:   false,
				Error:     "no response from fping",
				Payload:   MarshalPayload(payload),
			})
		}
	}

	return results
}

// parseRTTValues parses the RTT values from fping output.
func (e *ICMPExecutor) parseRTTValues(valuesStr string) ICMPPayload {
	values := strings.Fields(valuesStr)

	var rtts []float64
	packetsSent := len(values)
	packetsRecvd := 0

	var lastRTT float64
	for _, v := range values {
		if v == "-" {
			continue
		}
		rtt, err := strconv.ParseFloat(v, 64)
		if err != nil {
			continue
		}
		rtts = append(rtts, rtt)
		lastRTT = rtt
		packetsRecvd++
	}

	payload := ICMPPayload{
		PacketsSent:  packetsSent,
		PacketsRecvd: packetsRecvd,
	}

	if packetsSent > 0 {
		payload.PacketLoss = float64(packetsSent-packetsRecvd) / float64(packetsSent) * 100.0
	}

	if len(rtts) == 0 {
		payload.Reachable = false
		return payload
	}

	payload.Reachable = true
	payload.LatencyMs = lastRTT

	// Calculate statistics
	payload.MinMs = rtts[0]
	payload.MaxMs = rtts[0]
	sum := 0.0
	for _, rtt := range rtts {
		sum += rtt
		if rtt < payload.MinMs {
			payload.MinMs = rtt
		}
		if rtt > payload.MaxMs {
			payload.MaxMs = rtt
		}
	}
	payload.AvgMs = sum / float64(len(rtts))

	// Standard deviation
	if len(rtts) > 1 {
		sumSquares := 0.0
		for _, rtt := range rtts {
			diff := rtt - payload.AvgMs
			sumSquares += diff * diff
		}
		payload.StdDevMs = math.Sqrt(sumSquares / float64(len(rtts)-1))
	}

	return payload
}

// parseParams extracts ICMP parameters from raw JSON.
func (e *ICMPExecutor) parseParams(raw json.RawMessage) ICMPParams {
	var params ICMPParams
	if len(raw) > 0 {
		json.Unmarshal(raw, &params)
	}
	return params
}

// errorMessage generates an error message for failed probes.
func (e *ICMPExecutor) errorMessage(payload ICMPPayload) string {
	if payload.Reachable {
		return ""
	}
	if payload.PacketsRecvd == 0 {
		return fmt.Sprintf("100%% packet loss (%d packets sent)", payload.PacketsSent)
	}
	return fmt.Sprintf("%.1f%% packet loss", payload.PacketLoss)
}
