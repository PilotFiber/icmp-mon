// Package testutil provides testing utilities and fixtures for the control plane.
//
// This package contains:
//   - Test helper functions (loggers, servers, assertions)
//   - Fixture factories for domain types (agents, targets, subnets, tiers)
//   - Common test patterns and utilities
//
// # Usage
//
// Fixtures use functional options for customization:
//
//	agent := testutil.FixtureAgent()
//	agent := testutil.FixtureAgent(func(a *types.Agent) {
//		a.Name = "custom-agent"
//		a.Region = "us-west"
//	})
package testutil

import (
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/pilot-net/icmp-mon/pkg/types"
)

// NewTestLogger returns a logger that discards all output.
// Use for tests where logging output is not needed.
func NewTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// NewVerboseTestLogger returns a logger that writes to stderr.
// Use for debugging test failures.
func NewVerboseTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// =============================================================================
// AGENT FIXTURES
// =============================================================================

// FixtureAgent creates a test agent with sensible defaults.
// Use overrides to customize specific fields.
func FixtureAgent(overrides ...func(*types.Agent)) *types.Agent {
	agent := &types.Agent{
		ID:            uuid.New().String(),
		Name:          "test-agent-" + uuid.New().String()[:8],
		Region:        "us-east",
		Location:      "AWS us-east-1a",
		Provider:      "aws",
		Tags:          map[string]string{"env": "test"},
		PublicIP:      "10.0.0.1",
		Executors:     []string{"icmp_ping", "mtr"},
		MaxTargets:    10000,
		Version:       "1.0.0",
		Status:        types.AgentStatusActive,
		LastHeartbeat: time.Now(),
		CreatedAt:     time.Now(),
	}

	for _, override := range overrides {
		override(agent)
	}

	return agent
}

// FixtureAgentOffline creates an offline agent (no recent heartbeat).
func FixtureAgentOffline(overrides ...func(*types.Agent)) *types.Agent {
	return FixtureAgent(append([]func(*types.Agent){
		func(a *types.Agent) {
			a.Status = types.AgentStatusOffline
			a.LastHeartbeat = time.Now().Add(-5 * time.Minute)
		},
	}, overrides...)...)
}

// FixtureAgentDegraded creates a degraded agent (stale heartbeat).
func FixtureAgentDegraded(overrides ...func(*types.Agent)) *types.Agent {
	return FixtureAgent(append([]func(*types.Agent){
		func(a *types.Agent) {
			a.Status = types.AgentStatusDegraded
			a.LastHeartbeat = time.Now().Add(-45 * time.Second)
		},
	}, overrides...)...)
}

// =============================================================================
// TARGET FIXTURES
// =============================================================================

// FixtureTarget creates a test target with sensible defaults.
func FixtureTarget(overrides ...func(*types.Target)) *types.Target {
	target := &types.Target{
		ID:              uuid.New().String(),
		IP:              "192.168.1.100",
		Tier:            "standard",
		Tags:            map[string]string{"env": "test"},
		Ownership:       types.OwnershipManual,
		MonitoringState: types.StateActive,
		StateChangedAt:  time.Now(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	for _, override := range overrides {
		override(target)
	}

	return target
}

// FixtureTargetWithIP creates a target with a specific IP.
func FixtureTargetWithIP(ip string, overrides ...func(*types.Target)) *types.Target {
	return FixtureTarget(append([]func(*types.Target){
		func(t *types.Target) {
			t.IP = ip
		},
	}, overrides...)...)
}

// FixtureTargetDown creates a target in DOWN state.
func FixtureTargetDown(overrides ...func(*types.Target)) *types.Target {
	return FixtureTarget(append([]func(*types.Target){
		func(t *types.Target) {
			t.MonitoringState = types.StateDown
			t.StateChangedAt = time.Now().Add(-10 * time.Minute)
		},
	}, overrides...)...)
}

// =============================================================================
// SUBNET FIXTURES
// =============================================================================

// FixtureSubnet creates a test subnet with sensible defaults.
func FixtureSubnet(overrides ...func(*types.Subnet)) *types.Subnet {
	subnet := &types.Subnet{
		ID:             uuid.New().String(),
		NetworkAddress: "192.168.1.0/24",
		NetworkSize:    24,
		State:          "active",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	for _, override := range overrides {
		override(subnet)
	}

	return subnet
}

// FixtureSubnetWithCIDR creates a subnet with a specific CIDR.
func FixtureSubnetWithCIDR(cidr string, size int, overrides ...func(*types.Subnet)) *types.Subnet {
	return FixtureSubnet(append([]func(*types.Subnet){
		func(s *types.Subnet) {
			s.NetworkAddress = cidr
			s.NetworkSize = size
		},
	}, overrides...)...)
}

// =============================================================================
// TIER FIXTURES
// =============================================================================

// FixtureTier creates a test tier with sensible defaults.
func FixtureTier(overrides ...func(*types.Tier)) *types.Tier {
	tier := &types.Tier{
		Name:          "standard",
		DisplayName:   "Standard",
		ProbeInterval: 30 * time.Second,
		ProbeTimeout:  5 * time.Second,
		ProbeRetries:  3,
		AgentSelection: types.AgentSelectionPolicy{
			Strategy: "distributed",
			Count:    4,
		},
	}

	for _, override := range overrides {
		override(tier)
	}

	return tier
}

// FixtureTierInfra creates an infrastructure tier (high frequency).
func FixtureTierInfra(overrides ...func(*types.Tier)) *types.Tier {
	return FixtureTier(append([]func(*types.Tier){
		func(t *types.Tier) {
			t.Name = "infrastructure"
			t.DisplayName = "Infrastructure"
			t.ProbeInterval = 5 * time.Second
			t.AgentSelection = types.AgentSelectionPolicy{
				Strategy: "all",
			}
		},
	}, overrides...)...)
}

// FixtureTierVIP creates a VIP tier (medium frequency, high diversity).
func FixtureTierVIP(overrides ...func(*types.Tier)) *types.Tier {
	return FixtureTier(append([]func(*types.Tier){
		func(t *types.Tier) {
			t.Name = "vip"
			t.DisplayName = "VIP"
			t.ProbeInterval = 15 * time.Second
			t.AgentSelection = types.AgentSelectionPolicy{
				Strategy: "distributed",
				Count:    18,
				Diversity: &types.DiversityRequirement{
					MinRegions:   4,
					MinProviders: 3,
				},
			}
		},
	}, overrides...)...)
}

// =============================================================================
// PROBE RESULT FIXTURES
// =============================================================================

// FixtureProbeResult creates a successful probe result.
func FixtureProbeResult(agentID, targetID string, overrides ...func(*types.ProbeResult)) *types.ProbeResult {
	result := &types.ProbeResult{
		AgentID:   agentID,
		TargetID:  targetID,
		Success:   true,
		ProbeType: "icmp",
		Timestamp: time.Now(),
		Duration:  15 * time.Millisecond,
	}

	for _, override := range overrides {
		override(result)
	}

	return result
}

// FixtureProbeResultFailed creates a failed probe result.
func FixtureProbeResultFailed(agentID, targetID string, overrides ...func(*types.ProbeResult)) *types.ProbeResult {
	return FixtureProbeResult(agentID, targetID, append([]func(*types.ProbeResult){
		func(r *types.ProbeResult) {
			r.Success = false
			r.Duration = 0
			r.Error = "request timeout"
		},
	}, overrides...)...)
}

// =============================================================================
// ASSIGNMENT FIXTURES
// =============================================================================

// FixtureAssignment creates a target assignment.
func FixtureAssignment(targetID, agentID string, overrides ...func(*types.TargetAssignment)) *types.TargetAssignment {
	assignment := &types.TargetAssignment{
		ID:         uuid.New().String(),
		TargetID:   targetID,
		AgentID:    agentID,
		Tier:       "standard",
		AssignedAt: time.Now(),
		AssignedBy: "system",
	}

	for _, override := range overrides {
		override(assignment)
	}

	return assignment
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// Ptr returns a pointer to the given value.
// Useful for setting optional fields in fixtures.
func Ptr[T any](v T) *T {
	return &v
}

// TimeAgo returns a time in the past by the given duration.
func TimeAgo(d time.Duration) time.Time {
	return time.Now().Add(-d)
}

// TimeAgoPtr returns a pointer to a time in the past.
func TimeAgoPtr(d time.Duration) *time.Time {
	t := time.Now().Add(-d)
	return &t
}
