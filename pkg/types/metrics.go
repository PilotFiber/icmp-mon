// Package types - Flexible metrics querying with tag-based filtering
//
// # Metrics Query Design
//
// The metrics query system allows slicing probe data by agent and target attributes.
// Designed for scale (100k targets × 100 agents) using:
// - Pre-filtering by tags (GIN indexed JSONB) before joining probe data
// - Automatic aggregate table selection (hourly/daily/monthly based on window)
// - SQL-level aggregation to minimize data transfer
//
// # Example Queries
//
//   - "Latency from DigitalOcean agents in ORD to targets in NYC for last month"
//   - "Packet loss from all AWS agents to production targets today"
//   - "P95 latency by agent region for VIP tier targets over 90 days"
package types

import (
	"fmt"
	"time"
)

// =============================================================================
// METRICS QUERY
// =============================================================================

// MetricsQuery defines a flexible query for probe metrics.
type MetricsQuery struct {
	// Filters narrow down which agent-target pairs to include
	AgentFilter  *AgentFilter  `json:"agent_filter,omitempty"`
	TargetFilter *TargetFilter `json:"target_filter,omitempty"`

	// Time range for the query
	TimeRange TimeRange `json:"time_range"`

	// Bucket size for time-series aggregation
	// Examples: "5m", "1h", "1d"
	// If empty, auto-selected based on time range
	Bucket string `json:"bucket,omitempty"`

	// Which metrics to return
	// Options: "avg_latency", "min_latency", "max_latency", "p50_latency",
	//          "p95_latency", "p99_latency", "packet_loss", "success_rate", "probe_count"
	// Default: ["avg_latency", "packet_loss"]
	Metrics []string `json:"metrics,omitempty"`

	// How to group results
	// Options: "time", "agent", "agent_region", "agent_provider", "target", "target_tier"
	// Default: ["time"] - single aggregated time series
	// Example: ["time", "agent_region"] - one series per agent region
	GroupBy []string `json:"group_by,omitempty"`

	// Limit results (for cardinality control)
	// Default: 10000 data points
	Limit int `json:"limit,omitempty"`
}

// AgentFilter specifies which agents to include in the query.
type AgentFilter struct {
	// Filter by agent IDs (exact match)
	IDs []string `json:"ids,omitempty"`

	// Filter by region (exact match, any of these)
	Regions []string `json:"regions,omitempty"`

	// Filter by provider (exact match, any of these)
	Providers []string `json:"providers,omitempty"`

	// Filter by tags (all must match) - legacy simple format
	Tags map[string]string `json:"tags,omitempty"`

	// Exclude agents matching these tags - legacy simple format
	ExcludeTags map[string]string `json:"exclude_tags,omitempty"`

	// Advanced tag filters with operators
	// Filters with the same key are ORed, different keys are ANDed
	TagFilters []TagFilter `json:"tag_filters,omitempty"`
}

// TargetFilter specifies which targets to include in the query.
type TargetFilter struct {
	// Filter by target IDs (exact match)
	IDs []string `json:"ids,omitempty"`

	// Filter by tier (any of these)
	Tiers []string `json:"tiers,omitempty"`

	// Filter by region (any of these) - filters by subnet region
	Regions []string `json:"regions,omitempty"`

	// Filter by tags (all must match) - legacy simple format
	Tags map[string]string `json:"tags,omitempty"`

	// Exclude targets matching these tags - legacy simple format
	ExcludeTags map[string]string `json:"exclude_tags,omitempty"`

	// Advanced tag filters with operators
	// Filters with the same key are ORed, different keys are ANDed
	TagFilters []TagFilter `json:"tag_filters,omitempty"`
}

// TagFilter defines a single tag filter with an operator.
type TagFilter struct {
	Key      string `json:"key"`
	Operator string `json:"operator"` // equals, not_equals, contains, not_contains, starts_with, in, not_in, regex
	Value    string `json:"value"`
}

// TimeRange specifies the time window for the query.
type TimeRange struct {
	// Relative time range (takes precedence if set)
	// Examples: "1h", "24h", "7d", "30d", "90d"
	Window string `json:"window,omitempty"`

	// Absolute time range
	Start *time.Time `json:"start,omitempty"`
	End   *time.Time `json:"end,omitempty"`
}

// Validate checks that the query has valid configuration.
func (q *MetricsQuery) Validate() error {
	// Must have a time range
	if q.TimeRange.Window == "" && q.TimeRange.Start == nil {
		return fmt.Errorf("time_range.window or time_range.start is required")
	}

	// Validate metrics
	validMetrics := map[string]bool{
		"avg_latency":  true,
		"min_latency":  true,
		"max_latency":  true,
		"p50_latency":  true,
		"p95_latency":  true,
		"p99_latency":  true,
		"jitter":       true,
		"packet_loss":  true,
		"success_rate": true,
		"probe_count":  true,
	}
	for _, m := range q.Metrics {
		if !validMetrics[m] {
			return fmt.Errorf("invalid metric: %s", m)
		}
	}

	// Validate group_by
	validGroupBy := map[string]bool{
		"time":           true,
		"agent":          true,
		"agent_region":   true,
		"agent_provider": true,
		"target":         true,
		"target_tier":    true,
		"target_region":  true,
	}
	for _, g := range q.GroupBy {
		if !validGroupBy[g] {
			return fmt.Errorf("invalid group_by: %s", g)
		}
	}

	return nil
}

// GetWindowDuration parses the window string to a duration.
func (tr *TimeRange) GetWindowDuration() (time.Duration, error) {
	if tr.Window == "" {
		if tr.Start != nil && tr.End != nil {
			return tr.End.Sub(*tr.Start), nil
		}
		if tr.Start != nil {
			return time.Since(*tr.Start), nil
		}
		return 0, fmt.Errorf("no time range specified")
	}

	// Parse window string (e.g., "1h", "7d", "30d")
	return ParseDuration(tr.Window)
}

// ParseDuration parses duration strings including days.
func ParseDuration(s string) (time.Duration, error) {
	// Handle day suffix
	if len(s) > 0 && s[len(s)-1] == 'd' {
		var days int
		_, err := fmt.Sscanf(s, "%dd", &days)
		if err != nil {
			return 0, fmt.Errorf("invalid duration: %s", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	// Use standard parsing for hours/minutes/seconds
	return time.ParseDuration(s)
}

// =============================================================================
// METRICS QUERY RESPONSE
// =============================================================================

// MetricsQueryResult contains the response from a metrics query.
type MetricsQueryResult struct {
	// Query metadata
	Query          MetricsQuery `json:"query"`
	ExecutedAt     time.Time    `json:"executed_at"`
	ExecutionMs    int64        `json:"execution_ms"`
	AggregateTable string       `json:"aggregate_table"` // Which table was used

	// Filter resolution (for debugging/transparency)
	MatchedAgents  int `json:"matched_agents"`
	MatchedTargets int `json:"matched_targets"`

	// Results grouped by the group_by dimensions
	Series []MetricsSeries `json:"series"`

	// Total data points returned
	TotalPoints int `json:"total_points"`
}

// MetricsSeries is a single time series with optional grouping dimensions.
type MetricsSeries struct {
	// Grouping dimensions (present based on group_by)
	AgentID       string `json:"agent_id,omitempty"`
	AgentName     string `json:"agent_name,omitempty"`
	AgentRegion   string `json:"agent_region,omitempty"`
	AgentProvider string `json:"agent_provider,omitempty"`
	TargetID      string `json:"target_id,omitempty"`
	TargetIP      string `json:"target_ip,omitempty"`
	TargetTier    string `json:"target_tier,omitempty"`
	TargetRegion  string `json:"target_region,omitempty"`

	// Time-series data points
	Points []MetricsDataPoint `json:"points"`
}

// MetricsDataPoint is a single point in time with metric values.
type MetricsDataPoint struct {
	Time time.Time `json:"time"`

	// Metric values (present based on requested metrics)
	AvgLatency  *float64 `json:"avg_latency,omitempty"`
	MinLatency  *float64 `json:"min_latency,omitempty"`
	MaxLatency  *float64 `json:"max_latency,omitempty"`
	P50Latency  *float64 `json:"p50_latency,omitempty"`
	P95Latency  *float64 `json:"p95_latency,omitempty"`
	P99Latency  *float64 `json:"p99_latency,omitempty"`
	Jitter      *float64 `json:"jitter,omitempty"`
	PacketLoss  *float64 `json:"packet_loss,omitempty"`
	SuccessRate *float64 `json:"success_rate,omitempty"`
	ProbeCount  *int64   `json:"probe_count,omitempty"`
}

// =============================================================================
// HELPER - Auto bucket selection
// =============================================================================

// AutoSelectBucket chooses an appropriate bucket size based on the time window.
// Goal: Return ~100-500 data points for good visualization.
func AutoSelectBucket(window time.Duration) string {
	switch {
	case window <= 1*time.Hour:
		return "1m" // 60 points
	case window <= 6*time.Hour:
		return "5m" // 72 points
	case window <= 24*time.Hour:
		return "15m" // 96 points
	case window <= 7*24*time.Hour:
		return "1h" // 168 points
	case window <= 30*24*time.Hour:
		return "6h" // 120 points
	case window <= 90*24*time.Hour:
		return "1d" // 90 points
	default:
		return "1d" // cap at daily
	}
}

// SelectAggregateTable chooses the best pre-computed aggregate for the query.
// Returns: "probe_5min", "probe_hourly", "probe_daily", or "probe_monthly"
// Note: We no longer use "probe_results" directly due to compression making it slow.
func SelectAggregateTable(window time.Duration, bucket string) string {
	bucketDuration, err := ParseDuration(bucket)
	if err != nil {
		bucketDuration = time.Hour // fallback
	}

	// Use 5-minute aggregate for windows up to 24 hours with sub-hourly buckets
	// This provides good granularity while avoiding slow decompression of raw data
	// The probe_5min aggregate can efficiently serve any bucket >= 5 minutes
	if window <= 24*time.Hour && bucketDuration < 1*time.Hour {
		return "probe_5min"
	}

	// Use hourly aggregate for windows up to 7 days
	if window <= 7*24*time.Hour {
		return "probe_hourly"
	}

	// Use daily aggregate for windows up to 90 days
	if window <= 90*24*time.Hour {
		return "probe_daily"
	}

	// Use monthly aggregate for longer windows
	return "probe_monthly"
}
