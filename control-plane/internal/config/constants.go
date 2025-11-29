// Package config provides configuration constants for the control plane.
//
// This package centralizes hardcoded values that were previously scattered
// throughout the codebase, making them easier to find, modify, and test.
package config

import "time"

// Agent health thresholds determine agent status based on heartbeat age.
const (
	// AgentDegradedThreshold - agent is considered degraded if no heartbeat
	// has been received within this duration.
	AgentDegradedThreshold = 30 * time.Second

	// AgentOfflineThreshold - agent is considered offline if no heartbeat
	// has been received within this duration.
	AgentOfflineThreshold = 60 * time.Second

	// AgentOfflineThresholdExtended is used for fleet overview metrics
	// where a slightly longer window is appropriate.
	AgentOfflineThresholdExtended = 120 * time.Second
)

// SQL interval strings for use in database queries.
// These must match the Go duration constants above.
const (
	SQLAgentDegradedInterval = "30 seconds"
	SQLAgentOfflineInterval  = "60 seconds"
	SQLAgentOfflineExtended  = "120 seconds"
)

// Batch processing configuration for result handling.
const (
	// DefaultResultBatchSize is the default number of results to batch
	// before shipping to the control plane (agent-side).
	DefaultResultBatchSize = 1000

	// BufferFlushBatchSize is the number of results to flush from the
	// Redis buffer to the database in a single operation.
	BufferFlushBatchSize = 20000

	// BufferFlushInterval is how often to flush the Redis buffer to database.
	BufferFlushInterval = 2 * time.Second
)

// Pagination defaults for API list endpoints.
const (
	// DefaultPaginationLimit is the default number of items returned
	// when no limit is specified.
	DefaultPaginationLimit = 50

	// MaxPaginationLimit is the maximum number of items that can be
	// requested in a single API call.
	MaxPaginationLimit = 500
)

// HTTP client timeouts.
const (
	// DefaultHTTPTimeout is the default timeout for HTTP client requests.
	DefaultHTTPTimeout = 30 * time.Second

	// DefaultBatchTimeout is the default max time before sending a batch.
	DefaultBatchTimeout = 5 * time.Second
)

// Cache TTLs for API response caching.
const (
	// CacheTTLFleetOverview is the TTL for fleet overview data.
	CacheTTLFleetOverview = 30 * time.Second

	// CacheTTLTargetStatuses is the TTL for target status data.
	CacheTTLTargetStatuses = 30 * time.Second

	// CacheTTLInfraHealth is the TTL for infrastructure health data.
	CacheTTLInfraHealth = 60 * time.Second

	// CacheTTLLatencyMatrix is the TTL for the latency matrix.
	CacheTTLLatencyMatrix = 60 * time.Second

	// CacheTTLInMarketLatency is the TTL for in-market latency data.
	CacheTTLInMarketLatency = 30 * time.Second

	// CacheTTLLatencyTrend is the TTL for latency trend data.
	CacheTTLLatencyTrend = 30 * time.Second

	// CacheTTLTargetList is the TTL for target list data.
	CacheTTLTargetList = 60 * time.Second
)

// Database connection configuration.
const (
	// DatabasePingTimeout is the timeout for database connectivity checks.
	DatabasePingTimeout = 5 * time.Second

	// RedisConnectionTimeout is the timeout for Redis connectivity checks.
	RedisConnectionTimeout = 5 * time.Second
)

// Agent polling and heartbeat intervals.
const (
	// AgentHeartbeatInterval is how often agents send heartbeats.
	AgentHeartbeatInterval = 30 * time.Second

	// CommandPollInterval is how often agents poll for commands.
	CommandPollInterval = 5 * time.Second
)
