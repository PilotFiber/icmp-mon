// Package types defines the core domain types shared between agent and control plane.
//
// # Design Principles
//
// 1. Simplicity: Types represent the domain model directly, no ORM abstractions
// 2. Serialization: All types are JSON-serializable for API transport
// 3. Immutability: Prefer value types; mutations create new instances
// 4. Validation: Types include Validate() methods for business rule enforcement
package types

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// =============================================================================
// TARGET
// =============================================================================

// Target represents an IP address to monitor.
//
// Targets are assigned to tiers which define monitoring intensity.
// Tags provide flexible filtering for maintenance scoping and correlation analysis.
type Target struct {
	ID           string            `json:"id"`
	IP           string            `json:"ip"`
	Tier         string            `json:"tier"`
	SubscriberID string            `json:"subscriber_id,omitempty"`
	Tags         map[string]string `json:"tags,omitempty"`

	// ExpectedOutcome defines alerting behavior.
	// For traditional monitoring: ShouldSucceed=true (alert on failure)
	// For security testing: ShouldSucceed=false (alert on success)
	ExpectedOutcome *ExpectedOutcome `json:"expected_outcome,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Validate checks that the target has required fields and valid values.
func (t *Target) Validate() error {
	if t.IP == "" {
		return fmt.Errorf("target IP is required")
	}
	if ip := net.ParseIP(t.IP); ip == nil {
		return fmt.Errorf("invalid IP address: %s", t.IP)
	}
	if t.Tier == "" {
		return fmt.Errorf("target tier is required")
	}
	return nil
}

// ExpectedOutcome defines what result is expected and how to alert on violations.
//
// Traditional monitoring expects success (reachability), alerting on failure.
// Security validation expects failure (blocked access), alerting on success.
type ExpectedOutcome struct {
	// ShouldSucceed determines alert polarity.
	// true: alert when probe fails (traditional monitoring)
	// false: alert when probe succeeds (security validation)
	ShouldSucceed bool `json:"should_succeed"`

	// AlertSeverity for expectation violations: "critical", "warning", "info"
	AlertSeverity string `json:"alert_severity,omitempty"`

	// AlertMessage is a custom message for the alert.
	AlertMessage string `json:"alert_message,omitempty"`
}

// =============================================================================
// TIER
// =============================================================================

// Tier defines the monitoring policy for a class of targets.
//
// Tiers control:
// - Probe timing (interval, timeout, retries)
// - Agent selection (how many, from where, diversity requirements)
// - Default expected outcomes
type Tier struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`

	// Probe timing
	ProbeInterval time.Duration `json:"probe_interval"`
	ProbeTimeout  time.Duration `json:"probe_timeout"`
	ProbeRetries  int           `json:"probe_retries"`

	// Agent selection policy
	AgentSelection AgentSelectionPolicy `json:"agent_selection"`

	// Default expected outcome for targets in this tier (can be overridden per-target)
	DefaultExpectedOutcome *ExpectedOutcome `json:"default_expected_outcome,omitempty"`
}

// AgentSelectionPolicy defines which agents monitor targets in a tier.
type AgentSelectionPolicy struct {
	// Strategy: "all" (every agent) or "distributed" (subset of agents)
	Strategy string `json:"strategy"`

	// Count is the number of agents per target (for "distributed" strategy)
	Count int `json:"count,omitempty"`

	// Regions limits agents to these regions. Empty means any region.
	// Example: ["us-east", "us-west", "europe"]
	Regions []string `json:"regions,omitempty"`

	// ExcludeRegions removes these regions from consideration.
	ExcludeRegions []string `json:"exclude_regions,omitempty"`

	// Providers limits agents to these providers. Empty means any provider.
	// Example: ["aws", "vultr", "hetzner"]
	Providers []string `json:"providers,omitempty"`

	// RequireTags filters to agents that have ALL of these tags.
	// Example: {"network_type": "external"}
	RequireTags map[string]string `json:"require_tags,omitempty"`

	// ExcludeTags filters out agents that have ANY of these tags.
	ExcludeTags map[string]string `json:"exclude_tags,omitempty"`

	// Diversity requirements for distributed selection.
	Diversity *DiversityRequirement `json:"diversity,omitempty"`
}

// DiversityRequirement ensures agents are spread across regions/providers.
type DiversityRequirement struct {
	MinRegions   int `json:"min_regions,omitempty"`
	MinProviders int `json:"min_providers,omitempty"`
}

// Validate checks that the tier configuration is valid.
func (t *Tier) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("tier name is required")
	}
	if t.ProbeInterval <= 0 {
		return fmt.Errorf("probe_interval must be positive")
	}
	if t.ProbeTimeout <= 0 {
		return fmt.Errorf("probe_timeout must be positive")
	}
	if t.AgentSelection.Strategy != "all" && t.AgentSelection.Strategy != "distributed" {
		return fmt.Errorf("agent_selection.strategy must be 'all' or 'distributed'")
	}
	if t.AgentSelection.Strategy == "distributed" && t.AgentSelection.Count <= 0 {
		return fmt.Errorf("agent_selection.count must be positive for distributed strategy")
	}
	return nil
}

// =============================================================================
// AGENT
// =============================================================================

// Agent represents a monitoring agent in the fleet.
type Agent struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// Location metadata for selection
	Region   string `json:"region"`   // e.g., "us-east", "europe", "asia-pac"
	Location string `json:"location"` // Human-readable: "AWS us-east-1a"
	Provider string `json:"provider"` // e.g., "aws", "vultr", "hetzner"

	// Tags for flexible filtering
	Tags map[string]string `json:"tags,omitempty"`

	// Network info
	PublicIP string `json:"public_ip"`

	// Capabilities
	Executors  []string `json:"executors"`   // e.g., ["icmp_ping", "mtr", "tcp_connect"]
	MaxTargets int      `json:"max_targets"` // Capacity limit

	// Version info
	Version string `json:"version"`

	// Status
	Status        AgentStatus `json:"status"`
	LastHeartbeat time.Time   `json:"last_heartbeat"`

	CreatedAt time.Time `json:"created_at"`
}

// AgentStatus represents the health state of an agent.
type AgentStatus string

const (
	AgentStatusActive   AgentStatus = "active"
	AgentStatusDegraded AgentStatus = "degraded"
	AgentStatusOffline  AgentStatus = "offline"
)

// =============================================================================
// ASSIGNMENT
// =============================================================================

// Assignment tells an agent what to probe. Derived from Target + Tier.
type Assignment struct {
	TargetID string `json:"target_id"`
	IP       string `json:"ip"`
	Tier     string `json:"tier"`

	// Probe configuration (from tier)
	ProbeType     string        `json:"probe_type"` // e.g., "icmp_ping"
	ProbeInterval time.Duration `json:"probe_interval"`
	ProbeTimeout  time.Duration `json:"probe_timeout"`
	ProbeRetries  int           `json:"probe_retries"`

	// Probe-specific parameters
	ProbeParams json.RawMessage `json:"probe_params,omitempty"`

	// For correlation and alerting
	Tags            map[string]string `json:"tags,omitempty"`
	ExpectedOutcome *ExpectedOutcome  `json:"expected_outcome,omitempty"`
}

// AssignmentSet is a versioned collection of assignments for an agent.
type AssignmentSet struct {
	Version     int64        `json:"version"`
	Assignments []Assignment `json:"assignments"`
	GeneratedAt time.Time    `json:"generated_at"`
}

// AssignmentDelta represents changes since a previous version.
type AssignmentDelta struct {
	FromVersion int64              `json:"from_version"`
	ToVersion   int64              `json:"to_version"`
	Changes     []AssignmentChange `json:"changes"`
}

// AssignmentChange represents a single assignment modification.
type AssignmentChange struct {
	Action     string      `json:"action"` // "add", "update", "remove"
	Assignment *Assignment `json:"assignment,omitempty"`
	TargetID   string      `json:"target_id,omitempty"` // For "remove" action
}

// =============================================================================
// PROBE RESULT
// =============================================================================

// ProbeResult is the outcome of a single probe execution.
type ProbeResult struct {
	// Identity
	TargetID string `json:"target_id"`
	AgentID  string `json:"agent_id"`

	// Timing
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration"`

	// Outcome
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`

	// Type-specific payload (ICMPPingResult, MTRResult, etc.)
	ProbeType string          `json:"probe_type"`
	Payload   json.RawMessage `json:"payload"`
}

// ResultBatch is a collection of results shipped together.
type ResultBatch struct {
	AgentID   string        `json:"agent_id"`
	BatchID   string        `json:"batch_id"`
	Results   []ProbeResult `json:"results"`
	CreatedAt time.Time     `json:"created_at"`
}

// =============================================================================
// PROBE-SPECIFIC PAYLOADS
// =============================================================================

// ICMPPingPayload contains ICMP ping-specific results.
type ICMPPingPayload struct {
	Reachable    bool    `json:"reachable"`
	LatencyMs    float64 `json:"latency_ms"`
	MinMs        float64 `json:"min_ms"`
	MaxMs        float64 `json:"max_ms"`
	AvgMs        float64 `json:"avg_ms"`
	StdDevMs     float64 `json:"stddev_ms"`
	PacketLoss   float64 `json:"packet_loss_pct"`
	PacketsSent  int     `json:"packets_sent"`
	PacketsRecvd int     `json:"packets_recvd"`
}

// MTRPayload contains MTR trace results.
type MTRPayload struct {
	DestinationReached bool     `json:"destination_reached"`
	TotalHops          int      `json:"total_hops"`
	Hops               []MTRHop `json:"hops"`
}

// MTRHop represents a single hop in an MTR trace.
type MTRHop struct {
	Hop      int     `json:"hop"`
	IP       string  `json:"ip,omitempty"`
	Hostname string  `json:"hostname,omitempty"`
	ASN      int     `json:"asn,omitempty"`
	ASName   string  `json:"as_name,omitempty"`
	LossPct  float64 `json:"loss_pct"`
	SentCount int    `json:"sent"`
	AvgMs    float64 `json:"avg_ms"`
	BestMs   float64 `json:"best_ms"`
	WorstMs  float64 `json:"worst_ms"`
	StdDevMs float64 `json:"stddev_ms"`
}

// TCPConnectPayload contains TCP connection test results.
type TCPConnectPayload struct {
	Connected  bool    `json:"connected"`
	Port       int     `json:"port"`
	LatencyMs  float64 `json:"latency_ms"`
	Error      string  `json:"error,omitempty"`
	TLSVersion string  `json:"tls_version,omitempty"`
	TLSCipher  string  `json:"tls_cipher,omitempty"`
}

// =============================================================================
// COMMAND (On-Demand Execution)
// =============================================================================

// Command is a one-shot request from control plane to agent.
type Command struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "mtr", "tcp_connect", "diagnostic"

	// Target for the command
	TargetIP string `json:"target_ip,omitempty"`

	// Type-specific parameters
	Params json.RawMessage `json:"params,omitempty"`

	// Metadata
	RequestedBy string    `json:"requested_by"`
	RequestedAt time.Time `json:"requested_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// CommandResult is the response to a command.
type CommandResult struct {
	CommandID   string          `json:"command_id"`
	AgentID     string          `json:"agent_id"`
	Success     bool            `json:"success"`
	Error       string          `json:"error,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	CompletedAt time.Time       `json:"completed_at"`
	Duration    time.Duration   `json:"duration"`
}

// =============================================================================
// SNAPSHOT
// =============================================================================

// Snapshot captures point-in-time state for maintenance comparison.
type Snapshot struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Scope     SnapshotScope `json:"scope"`
	CreatedBy string        `json:"created_by"`
	CreatedAt time.Time     `json:"created_at"`

	// Summary stats
	TotalTargets      int `json:"total_targets"`
	ReachableTargets  int `json:"reachable_targets"`
	UnreachableTargets int `json:"unreachable_targets"`
}

// SnapshotScope defines which targets to include in a snapshot.
type SnapshotScope struct {
	// Filter by tags (all must match)
	Tags map[string]string `json:"tags,omitempty"`

	// Filter by tiers
	Tiers []string `json:"tiers,omitempty"`

	// Filter by specific target IDs
	TargetIDs []string `json:"target_ids,omitempty"`
}

// TargetState is a target's state at snapshot time.
type TargetState struct {
	TargetID string            `json:"target_id"`
	IP       string            `json:"ip"`
	Tags     map[string]string `json:"tags,omitempty"`

	// Consensus from all monitoring agents
	ConsensusState string  `json:"consensus_state"` // "reachable", "unreachable", "degraded"
	ReachableFrom  int     `json:"reachable_from"`  // Agent count that can reach
	TotalAgents    int     `json:"total_agents"`    // Total agents monitoring
	AvgLatencyMs   float64 `json:"avg_latency_ms"`

	// Per-agent breakdown
	AgentStates []AgentTargetState `json:"agent_states,omitempty"`
}

// AgentTargetState is one agent's view of a target.
type AgentTargetState struct {
	AgentID     string    `json:"agent_id"`
	AgentName   string    `json:"agent_name"`
	AgentRegion string    `json:"agent_region"`
	Reachable   bool      `json:"reachable"`
	LatencyMs   float64   `json:"latency_ms"`
	PacketLoss  float64   `json:"packet_loss_pct"`
	LastProbe   time.Time `json:"last_probe"`
}

// SnapshotComparison is the diff between two snapshots.
type SnapshotComparison struct {
	BeforeSnapshot string    `json:"before_snapshot"`
	AfterSnapshot  string    `json:"after_snapshot"`
	ComparedAt     time.Time `json:"compared_at"`

	// Changes
	BecameUnreachable []TargetChange `json:"became_unreachable"`
	BecameReachable   []TargetChange `json:"became_reachable"`
	LatencyIncreased  []TargetChange `json:"latency_increased"`
	LatencyDecreased  []TargetChange `json:"latency_decreased"`
	Unchanged         int            `json:"unchanged_count"`

	// Tag correlation analysis
	Correlations []TagCorrelation `json:"correlations,omitempty"`
}

// TargetChange describes a state change for one target.
type TargetChange struct {
	TargetID     string            `json:"target_id"`
	IP           string            `json:"ip"`
	Tags         map[string]string `json:"tags,omitempty"`
	BeforeState  string            `json:"before_state"`
	AfterState   string            `json:"after_state"`
	BeforeLatency float64          `json:"before_latency_ms,omitempty"`
	AfterLatency  float64          `json:"after_latency_ms,omitempty"`
}

// TagCorrelation identifies tags common among affected targets.
type TagCorrelation struct {
	TagKey           string  `json:"tag_key"`
	TagValue         string  `json:"tag_value"`
	AffectedCount    int     `json:"affected_count"`
	AffectedPercent  float64 `json:"affected_percent"`
	BaselinePercent  float64 `json:"baseline_percent"`
	SuspicionScore   float64 `json:"suspicion_score"`
}

// =============================================================================
// HEARTBEAT
// =============================================================================

// Heartbeat is the periodic health report from agent to control plane.
type Heartbeat struct {
	AgentID   string    `json:"agent_id"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`

	// Health status
	Status      AgentStatus  `json:"status"`
	HealthChecks []HealthCheck `json:"health_checks"`

	// Resource usage
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryMB       float64 `json:"memory_mb"`
	GoroutineCount int     `json:"goroutine_count"`

	// Task stats
	ActiveTargets   int   `json:"active_targets"`
	ProbesPerSecond int   `json:"probes_per_second"`
	ResultsQueued   int   `json:"results_queued"`
	ResultsShipped  int64 `json:"results_shipped_total"`

	// Assignment sync state
	AssignmentVersion int64 `json:"assignment_version"`

	// Network info
	PublicIP string `json:"public_ip"`
}

// HealthCheck is a single health check result.
type HealthCheck struct {
	Name    string            `json:"name"`
	Status  string            `json:"status"` // "healthy", "degraded", "unhealthy"
	Message string            `json:"message,omitempty"`
	Metrics map[string]float64 `json:"metrics,omitempty"`
}

// HeartbeatResponse is the control plane's response to a heartbeat.
type HeartbeatResponse struct {
	Acknowledged bool `json:"acknowledged"`

	// Hints for agent behavior
	AssignmentStale bool `json:"assignment_stale,omitempty"` // Should re-sync assignments

	// Pending commands to execute
	Commands []Command `json:"commands,omitempty"`
}
