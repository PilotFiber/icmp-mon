package config

import (
	"fmt"
	"testing"
	"time"
)

func TestAgentThresholds(t *testing.T) {
	// Verify that degraded threshold is less than offline threshold
	if AgentDegradedThreshold >= AgentOfflineThreshold {
		t.Errorf("AgentDegradedThreshold (%v) should be less than AgentOfflineThreshold (%v)",
			AgentDegradedThreshold, AgentOfflineThreshold)
	}

	// Verify SQL strings match Go durations
	tests := []struct {
		name        string
		duration    time.Duration
		sqlInterval string
	}{
		{"degraded", AgentDegradedThreshold, SQLAgentDegradedInterval},
		{"offline", AgentOfflineThreshold, SQLAgentOfflineInterval},
		{"offline_extended", AgentOfflineThresholdExtended, SQLAgentOfflineExtended},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the SQL interval and verify it matches the Go duration
			// Format: "30 seconds" -> 30 * time.Second
			t.Logf("SQL interval: %s, Go duration: %v", tt.sqlInterval, tt.duration)

			n, err := parseInterval(tt.sqlInterval)
			if err != nil {
				t.Fatalf("Failed to parse SQL interval %q: %v", tt.sqlInterval, err)
			}
			if n != tt.duration {
				t.Errorf("SQL interval %q (%v) does not match Go duration %v",
					tt.sqlInterval, n, tt.duration)
			}
		})
	}
}

// parseInterval parses a PostgreSQL interval string like "30 seconds"
func parseInterval(s string) (time.Duration, error) {
	var value int
	var unit string
	_, err := fmt.Sscanf(s, "%d %s", &value, &unit)
	if err != nil {
		return 0, err
	}

	switch unit {
	case "seconds", "second":
		return time.Duration(value) * time.Second, nil
	case "minutes", "minute":
		return time.Duration(value) * time.Minute, nil
	default:
		return 0, fmt.Errorf("unknown unit: %s", unit)
	}
}

func TestPaginationLimits(t *testing.T) {
	if DefaultPaginationLimit > MaxPaginationLimit {
		t.Errorf("DefaultPaginationLimit (%d) should not exceed MaxPaginationLimit (%d)",
			DefaultPaginationLimit, MaxPaginationLimit)
	}

	if DefaultPaginationLimit <= 0 {
		t.Error("DefaultPaginationLimit should be positive")
	}

	if MaxPaginationLimit <= 0 {
		t.Error("MaxPaginationLimit should be positive")
	}
}

func TestCacheTTLs(t *testing.T) {
	ttls := []struct {
		name string
		ttl  time.Duration
	}{
		{"FleetOverview", CacheTTLFleetOverview},
		{"TargetStatuses", CacheTTLTargetStatuses},
		{"InfraHealth", CacheTTLInfraHealth},
		{"LatencyMatrix", CacheTTLLatencyMatrix},
		{"InMarketLatency", CacheTTLInMarketLatency},
		{"LatencyTrend", CacheTTLLatencyTrend},
		{"TargetList", CacheTTLTargetList},
	}

	for _, tt := range ttls {
		t.Run(tt.name, func(t *testing.T) {
			if tt.ttl <= 0 {
				t.Errorf("Cache TTL for %s should be positive, got %v", tt.name, tt.ttl)
			}
			// Cache TTLs should generally be under 5 minutes to ensure freshness
			if tt.ttl > 5*time.Minute {
				t.Errorf("Cache TTL for %s (%v) seems too long", tt.name, tt.ttl)
			}
		})
	}
}
