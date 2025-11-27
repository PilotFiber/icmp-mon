// Package types - Alert configuration and routing
//
// # Alerting Design
//
// Alerts are routed based on configurable rules that match on:
// - Alert severity (critical, warning, info)
// - Target tier (infrastructure, vip, standard)
// - Target tags (device type, customer, etc.)
// - Probe type (icmp_ping, tcp_connect, security scan)
// - Alert type (availability, security_violation, latency_degradation)
//
// Each rule specifies handlers (Slack, PagerDuty, Splunk, webhook, etc.)
// with handler-specific configuration.
//
// # Example Configuration
//
//	alert_rules:
//	  - name: "Critical infrastructure to PagerDuty"
//	    match:
//	      severity: ["critical"]
//	      tier: ["infrastructure"]
//	    handlers:
//	      - type: pagerduty
//	        config:
//	          service_key: "xxx"
//	          urgency: high
//
//	  - name: "VIP customer issues to Slack + PagerDuty"
//	    match:
//	      severity: ["critical", "warning"]
//	      tier: ["vip"]
//	    handlers:
//	      - type: slack
//	        config:
//	          channel: "#noc-vip"
//	      - type: pagerduty
//	        when: severity == "critical"
//
//	  - name: "Security violations always page"
//	    match:
//	      alert_type: ["security_violation"]
//	    handlers:
//	      - type: pagerduty
//	      - type: slack
//	        config:
//	          channel: "#security-alerts"
package types

import (
	"encoding/json"
	"time"
)

// =============================================================================
// ALERT (Evolving)
// =============================================================================

// Alert represents a detected issue that evolves over time.
// Alerts track individual anomalies from detection through resolution,
// recording severity changes, metric evolution, and eventually linking to incidents.
type Alert struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`

	// What's affected
	TargetID string `json:"target_id"`
	TargetIP string `json:"target_ip"`
	AgentID  string `json:"agent_id,omitempty"` // Which agent detected (empty if consensus)

	// Classification
	AlertType AlertType     `json:"alert_type"`
	Severity  AlertSeverity `json:"severity"`
	Status    AlertStatus   `json:"status"`

	// Evolution tracking - how the alert changed over time
	InitialSeverity AlertSeverity `json:"initial_severity"`
	PeakSeverity    AlertSeverity `json:"peak_severity"`

	// Metrics at various points in the alert lifecycle
	InitialLatencyMs   *float64 `json:"initial_latency_ms,omitempty"`
	InitialPacketLoss  *float64 `json:"initial_packet_loss,omitempty"`
	PeakLatencyMs      *float64 `json:"peak_latency_ms,omitempty"`
	PeakPacketLoss     *float64 `json:"peak_packet_loss,omitempty"`
	CurrentLatencyMs   *float64 `json:"current_latency_ms,omitempty"`
	CurrentPacketLoss  *float64 `json:"current_packet_loss,omitempty"`

	// Human-readable info
	Title   string `json:"title"`
	Message string `json:"message,omitempty"`

	// Timeline
	DetectedAt     time.Time  `json:"detected_at"`
	LastUpdatedAt  time.Time  `json:"last_updated_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy string     `json:"acknowledged_by,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`

	// Correlation to incidents
	IncidentID     *string `json:"incident_id,omitempty"`
	CorrelationKey string  `json:"correlation_key,omitempty"` // e.g., "subnet:xxx", "target:xxx"

	// For API responses - populated by joins
	TargetName string `json:"target_name,omitempty"`
	AgentName  string `json:"agent_name,omitempty"`

	// Subnet/Pilot enriched metadata - for correlated alerting
	SubnetID         string `json:"subnet_id,omitempty"`
	SubscriberName   string `json:"subscriber_name,omitempty"`
	ServiceID        int    `json:"service_id,omitempty"`
	LocationID       int    `json:"location_id,omitempty"`
	LocationAddress  string `json:"location_address,omitempty"`
	City             string `json:"city,omitempty"`
	Region           string `json:"region,omitempty"`
	PopName          string `json:"pop_name,omitempty"`
	GatewayDevice    string `json:"gateway_device,omitempty"`

	// Legacy fields for backward compatibility with routing rules
	TargetTier string            `json:"target_tier,omitempty"`
	TargetTags map[string]string `json:"target_tags,omitempty"`
	ProbeType  string            `json:"probe_type,omitempty"`
	AgentIDs   []string          `json:"agent_ids,omitempty"` // For consensus alerts
	Details    map[string]any    `json:"details,omitempty"`
}

// AlertEvent represents a single change in an alert's history.
// Events are append-only and form the complete audit trail.
type AlertEvent struct {
	ID      int64     `json:"id"`
	AlertID string    `json:"alert_id"`
	EventType string  `json:"event_type"`
	// Event types:
	//   "created"            - Alert first detected
	//   "escalated"          - Severity increased
	//   "de_escalated"       - Severity decreased
	//   "acknowledged"       - Human acknowledged
	//   "unacknowledged"     - Acknowledgment reverted
	//   "linked_to_incident" - Alert joined an incident
	//   "metrics_updated"    - Current metrics changed significantly
	//   "resolved"           - Alert resolved
	//   "reopened"           - Alert reopened after resolution

	// What changed
	OldSeverity *AlertSeverity `json:"old_severity,omitempty"`
	NewSeverity *AlertSeverity `json:"new_severity,omitempty"`
	OldStatus   *AlertStatus   `json:"old_status,omitempty"`
	NewStatus   *AlertStatus   `json:"new_status,omitempty"`

	// Metrics at time of event
	LatencyMs   *float64 `json:"latency_ms,omitempty"`
	PacketLoss  *float64 `json:"packet_loss,omitempty"`

	// Context
	Description string         `json:"description,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
	TriggeredBy string         `json:"triggered_by"` // "system", "alert_worker", "user:xxx"

	CreatedAt time.Time `json:"created_at"`
}

// AlertWithEvents combines an alert with its event history.
type AlertWithEvents struct {
	Alert
	Events []AlertEvent `json:"events"`
}

// Anomaly represents a detected issue from agent_target_state.
// Used as input to the AlertWorker for creating/evolving alerts.
type Anomaly struct {
	TargetID            string  `json:"target_id"`
	TargetIP            string  `json:"target_ip"`
	AgentID             string  `json:"agent_id"`
	AnomalyType         string  `json:"anomaly_type"` // "availability", "latency", "packet_loss"
	Severity            string  `json:"severity"`     // "info", "warning", "critical"
	LatencyMs           float64 `json:"latency_ms"`
	PacketLoss          float64 `json:"packet_loss"`
	ZScore              float64 `json:"z_score"`
	SubnetID            string  `json:"subnet_id,omitempty"`
	ConsecutiveFailures int     `json:"consecutive_failures"`
}

// AlertFilter for listing alerts with filtering.
type AlertFilter struct {
	Status       *AlertStatus   `json:"status,omitempty"`
	Severity     *AlertSeverity `json:"severity,omitempty"`
	AlertType    *AlertType     `json:"alert_type,omitempty"`
	TargetID     *string        `json:"target_id,omitempty"`
	IncidentID   *string        `json:"incident_id,omitempty"`
	HasIncident  *bool          `json:"has_incident,omitempty"` // true = linked, false = unlinked
	Since        *time.Time     `json:"since,omitempty"`
	Limit        int            `json:"limit,omitempty"`
	Offset       int            `json:"offset,omitempty"`
}

// AlertStats provides aggregate statistics about alerts.
type AlertStats struct {
	ActiveCount            int      `json:"active_count"`
	CriticalCount          int      `json:"critical_count"`
	WarningCount           int      `json:"warning_count"`
	AcknowledgedCount      int      `json:"acknowledged_count"`
	ResolvedTodayCount     int      `json:"resolved_today_count"`
	TotalThisWeekCount     int      `json:"total_this_week_count"`
	AvgResolutionMinutes   *float64 `json:"avg_resolution_minutes,omitempty"`
}

// AlertConfig represents a configuration value from alert_config table.
type AlertConfig struct {
	Key         string    `json:"key"`
	Value       any       `json:"value"`
	Description string    `json:"description,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AlertCorrelation represents a common pattern across active alerts.
// Used to surface "hot spots" in the alert dashboard.
type AlertCorrelation struct {
	Key         string `json:"key"`           // e.g., "pop_name", "subscriber_name", "gateway_device"
	Value       string `json:"value"`         // e.g., "jfk00", "Family Management"
	AlertCount  int    `json:"alert_count"`   // Number of active alerts matching
	TargetCount int    `json:"target_count"`  // Unique targets affected
	AgentCount  int    `json:"agent_count"`   // Unique agents seeing issues
	Severity    string `json:"severity"`      // Most severe alert in this group
}

// AlertCorrelationSummary aggregates correlations by dimension.
type AlertCorrelationSummary struct {
	TotalActiveAlerts int                `json:"total_active_alerts"`
	ByPop             []AlertCorrelation `json:"by_pop"`
	ByGateway         []AlertCorrelation `json:"by_gateway"`
	BySubscriber      []AlertCorrelation `json:"by_subscriber"`
	ByLocation        []AlertCorrelation `json:"by_location"`
	ByRegion          []AlertCorrelation `json:"by_region"`
}

// SeverityLevel returns numeric level for comparison (higher = more severe).
func (s AlertSeverity) Level() int {
	switch s {
	case AlertSeverityCritical:
		return 3
	case AlertSeverityWarning:
		return 2
	case AlertSeverityInfo:
		return 1
	default:
		return 0
	}
}

// AlertSeverity indicates urgency level.
type AlertSeverity string

const (
	AlertSeverityCritical AlertSeverity = "critical" // Immediate action required
	AlertSeverityWarning  AlertSeverity = "warning"  // Attention needed
	AlertSeverityInfo     AlertSeverity = "info"     // Informational
)

// AlertType categorizes the nature of the alert.
type AlertType string

const (
	AlertTypeAvailability       AlertType = "availability"        // Target unreachable
	AlertTypeLatency            AlertType = "latency"             // Latency degradation
	AlertTypePacketLoss         AlertType = "packet_loss"         // Significant packet loss
	AlertTypeSecurityViolation  AlertType = "security_violation"  // Expected-failure succeeded
	AlertTypePathChange         AlertType = "path_change"         // Routing path changed
	AlertTypeAgentDown          AlertType = "agent_down"          // Monitoring agent offline
	AlertTypeFleetAnomaly       AlertType = "fleet_anomaly"       // Widespread issue detected
)

// AlertStatus tracks the alert lifecycle.
type AlertStatus string

const (
	AlertStatusActive       AlertStatus = "active"
	AlertStatusAcknowledged AlertStatus = "acknowledged"
	AlertStatusResolved     AlertStatus = "resolved"
	AlertStatusSuppressed   AlertStatus = "suppressed"
)

// =============================================================================
// ALERT RULES
// =============================================================================

// AlertRule defines when and how to route alerts.
type AlertRule struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	Priority int    `json:"priority"` // Lower = higher priority, evaluated first

	// Match conditions (all must match)
	Match AlertMatch `json:"match"`

	// Where to send matching alerts
	Handlers []AlertHandlerConfig `json:"handlers"`

	// Optional: suppress duplicate alerts
	Deduplication *DeduplicationConfig `json:"deduplication,omitempty"`

	// Optional: only alert after condition persists
	DelaySeconds int `json:"delay_seconds,omitempty"`
}

// AlertMatch defines criteria for matching alerts to rules.
type AlertMatch struct {
	// Match by severity (any of these)
	Severities []AlertSeverity `json:"severities,omitempty"`

	// Match by alert type (any of these)
	AlertTypes []AlertType `json:"alert_types,omitempty"`

	// Match by target tier (any of these)
	Tiers []string `json:"tiers,omitempty"`

	// Match by probe type (any of these)
	ProbeTypes []string `json:"probe_types,omitempty"`

	// Match by target tags (all must match)
	Tags map[string]string `json:"tags,omitempty"`

	// Match by tag presence (target must have these tag keys)
	HasTags []string `json:"has_tags,omitempty"`

	// Exclude targets with these tags
	ExcludeTags map[string]string `json:"exclude_tags,omitempty"`

	// Time-based matching (e.g., business hours only)
	TimeWindow *TimeWindow `json:"time_window,omitempty"`
}

// TimeWindow restricts when a rule is active.
type TimeWindow struct {
	// Timezone for evaluation (e.g., "America/Chicago")
	Timezone string `json:"timezone"`

	// Days of week (0=Sunday, 6=Saturday)
	DaysOfWeek []int `json:"days_of_week,omitempty"`

	// Time range (24h format)
	StartTime string `json:"start_time,omitempty"` // "08:00"
	EndTime   string `json:"end_time,omitempty"`   // "18:00"
}

// DeduplicationConfig prevents alert storms.
type DeduplicationConfig struct {
	// Key fields to group alerts (e.g., ["target_id", "alert_type"])
	GroupBy []string `json:"group_by"`

	// Time window for deduplication
	WindowSeconds int `json:"window_seconds"`

	// Max alerts per window before suppression
	MaxAlerts int `json:"max_alerts"`
}

// Matches checks if an alert matches this rule.
func (m *AlertMatch) Matches(alert *Alert) bool {
	// Check severities
	if len(m.Severities) > 0 && !containsSeverity(m.Severities, alert.Severity) {
		return false
	}

	// Check alert types
	if len(m.AlertTypes) > 0 && !containsAlertType(m.AlertTypes, alert.AlertType) {
		return false
	}

	// Check tiers
	if len(m.Tiers) > 0 && !containsString(m.Tiers, alert.TargetTier) {
		return false
	}

	// Check probe types
	if len(m.ProbeTypes) > 0 && !containsString(m.ProbeTypes, alert.ProbeType) {
		return false
	}

	// Check required tags
	for k, v := range m.Tags {
		if alert.TargetTags[k] != v {
			return false
		}
	}

	// Check tag presence
	for _, k := range m.HasTags {
		if _, ok := alert.TargetTags[k]; !ok {
			return false
		}
	}

	// Check excluded tags
	for k, v := range m.ExcludeTags {
		if alert.TargetTags[k] == v {
			return false
		}
	}

	return true
}

// =============================================================================
// ALERT HANDLERS
// =============================================================================

// AlertHandlerConfig defines how to send an alert to a destination.
type AlertHandlerConfig struct {
	// Handler type: "slack", "pagerduty", "splunk", "webhook", "email", "log"
	Type string `json:"type"`

	// Handler-specific configuration
	Config json.RawMessage `json:"config"`

	// Optional: conditional execution within a rule
	// Example: "severity == 'critical'" to only page for critical
	Condition string `json:"condition,omitempty"`
}

// SlackConfig configures Slack notifications.
type SlackConfig struct {
	WebhookURL string `json:"webhook_url,omitempty"` // Or use channel with bot token
	Channel    string `json:"channel"`               // "#noc-alerts"
	Username   string `json:"username,omitempty"`    // Bot display name
	IconEmoji  string `json:"icon_emoji,omitempty"`  // ":rotating_light:"

	// Message customization
	IncludeDetails bool `json:"include_details"` // Include full alert details
	MentionUsers   []string `json:"mention_users,omitempty"` // User IDs to mention
	MentionGroups  []string `json:"mention_groups,omitempty"` // Group IDs to mention
}

// PagerDutyConfig configures PagerDuty integration.
type PagerDutyConfig struct {
	RoutingKey string `json:"routing_key"` // Integration key

	// Severity mapping (our severity -> PD severity)
	SeverityMap map[AlertSeverity]string `json:"severity_map,omitempty"`

	// Custom fields
	Component string `json:"component,omitempty"` // PD component field
	Group     string `json:"group,omitempty"`     // PD group field
}

// SplunkConfig configures Splunk HEC (HTTP Event Collector).
type SplunkConfig struct {
	Endpoint   string `json:"endpoint"`    // HEC endpoint URL
	Token      string `json:"token"`       // HEC token
	Index      string `json:"index"`       // Target index
	Source     string `json:"source"`      // Event source
	SourceType string `json:"sourcetype"`  // Event sourcetype
}

// WebhookConfig configures generic webhook notifications.
type WebhookConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"` // Default: POST
	Headers map[string]string `json:"headers,omitempty"`

	// Authentication
	AuthType   string `json:"auth_type,omitempty"` // "basic", "bearer", "header"
	AuthConfig json.RawMessage `json:"auth_config,omitempty"`

	// Retry configuration
	RetryCount   int `json:"retry_count,omitempty"`
	RetryDelayMs int `json:"retry_delay_ms,omitempty"`
}

// EmailConfig configures email notifications.
type EmailConfig struct {
	To      []string `json:"to"`
	Cc      []string `json:"cc,omitempty"`
	Subject string   `json:"subject,omitempty"` // Template, default based on alert

	// SMTP settings (or use global config)
	SMTPHost string `json:"smtp_host,omitempty"`
	SMTPPort int    `json:"smtp_port,omitempty"`
	SMTPUser string `json:"smtp_user,omitempty"`
}

// LogConfig configures local logging of alerts (useful for debugging).
type LogConfig struct {
	Level  string `json:"level,omitempty"`  // "info", "warn", "error"
	Format string `json:"format,omitempty"` // "json", "text"
}

// =============================================================================
// ALERT THRESHOLDS (Per-Tier/ProbeType Configuration)
// =============================================================================

// AlertThresholds defines when to generate alerts.
// Can be configured per tier or globally.
type AlertThresholds struct {
	// Availability
	FailureThreshold int `json:"failure_threshold"` // Consecutive failures before alerting
	RecoveryThreshold int `json:"recovery_threshold"` // Consecutive successes before resolving

	// Consensus (for multi-agent monitoring)
	ConsensusFailurePercent float64 `json:"consensus_failure_percent"` // % of agents seeing failure

	// Latency
	LatencyWarningMs  float64 `json:"latency_warning_ms"`
	LatencyCriticalMs float64 `json:"latency_critical_ms"`
	LatencyBaselineMultiplier float64 `json:"latency_baseline_multiplier"` // Alert if > N*baseline

	// Packet loss
	PacketLossWarningPct  float64 `json:"packet_loss_warning_pct"`
	PacketLossCriticalPct float64 `json:"packet_loss_critical_pct"`
}

// DefaultAlertThresholds returns sensible defaults.
func DefaultAlertThresholds() AlertThresholds {
	return AlertThresholds{
		FailureThreshold:          3,
		RecoveryThreshold:         2,
		ConsensusFailurePercent:   50.0,
		LatencyWarningMs:          100.0,
		LatencyCriticalMs:         500.0,
		LatencyBaselineMultiplier: 2.0,
		PacketLossWarningPct:      5.0,
		PacketLossCriticalPct:     20.0,
	}
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

func containsSeverity(list []AlertSeverity, item AlertSeverity) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

func containsAlertType(list []AlertType, item AlertType) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

func containsString(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}
