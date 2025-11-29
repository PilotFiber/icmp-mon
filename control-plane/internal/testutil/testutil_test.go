package testutil

import (
	"testing"
	"time"

	"github.com/pilot-net/icmp-mon/pkg/types"
)

func TestFixtureAgent(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		agent := FixtureAgent()
		if agent.ID == "" {
			t.Error("expected agent to have ID")
		}
		if agent.Name == "" {
			t.Error("expected agent to have Name")
		}
		if agent.Status != types.AgentStatusActive {
			t.Errorf("expected status %s, got %s", types.AgentStatusActive, agent.Status)
		}
	})

	t.Run("with overrides", func(t *testing.T) {
		agent := FixtureAgent(func(a *types.Agent) {
			a.Name = "custom-agent"
			a.Region = "eu-west"
		})
		if agent.Name != "custom-agent" {
			t.Errorf("expected name 'custom-agent', got %s", agent.Name)
		}
		if agent.Region != "eu-west" {
			t.Errorf("expected region 'eu-west', got %s", agent.Region)
		}
	})

	t.Run("offline variant", func(t *testing.T) {
		agent := FixtureAgentOffline()
		if agent.Status != types.AgentStatusOffline {
			t.Errorf("expected status %s, got %s", types.AgentStatusOffline, agent.Status)
		}
		if time.Since(agent.LastHeartbeat) < 4*time.Minute {
			t.Error("expected old heartbeat for offline agent")
		}
	})

	t.Run("degraded variant", func(t *testing.T) {
		agent := FixtureAgentDegraded()
		if agent.Status != types.AgentStatusDegraded {
			t.Errorf("expected status %s, got %s", types.AgentStatusDegraded, agent.Status)
		}
	})
}

func TestFixtureTarget(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		target := FixtureTarget()
		if target.ID == "" {
			t.Error("expected target to have ID")
		}
		if target.IP == "" {
			t.Error("expected target to have IP")
		}
		if err := target.Validate(); err != nil {
			t.Errorf("expected valid target, got error: %v", err)
		}
	})

	t.Run("with specific IP", func(t *testing.T) {
		target := FixtureTargetWithIP("10.20.30.40")
		if target.IP != "10.20.30.40" {
			t.Errorf("expected IP '10.20.30.40', got %s", target.IP)
		}
	})

	t.Run("down variant", func(t *testing.T) {
		target := FixtureTargetDown()
		if target.MonitoringState != types.StateDown {
			t.Errorf("expected state %s, got %s", types.StateDown, target.MonitoringState)
		}
	})
}

func TestFixtureSubnet(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		subnet := FixtureSubnet()
		if subnet.ID == "" {
			t.Error("expected subnet to have ID")
		}
		if subnet.NetworkAddress == "" {
			t.Error("expected subnet to have NetworkAddress")
		}
		if err := subnet.Validate(); err != nil {
			t.Errorf("expected valid subnet, got error: %v", err)
		}
	})

	t.Run("with specific CIDR", func(t *testing.T) {
		subnet := FixtureSubnetWithCIDR("10.0.0.0/16", 16)
		if subnet.NetworkAddress != "10.0.0.0/16" {
			t.Errorf("expected CIDR '10.0.0.0/16', got %s", subnet.NetworkAddress)
		}
		if subnet.NetworkSize != 16 {
			t.Errorf("expected size 16, got %d", subnet.NetworkSize)
		}
	})
}

func TestFixtureTier(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		tier := FixtureTier()
		if tier.Name != "standard" {
			t.Errorf("expected name 'standard', got %s", tier.Name)
		}
		if err := tier.Validate(); err != nil {
			t.Errorf("expected valid tier, got error: %v", err)
		}
	})

	t.Run("infrastructure variant", func(t *testing.T) {
		tier := FixtureTierInfra()
		if tier.Name != "infrastructure" {
			t.Errorf("expected name 'infrastructure', got %s", tier.Name)
		}
		if tier.ProbeInterval != 5*time.Second {
			t.Errorf("expected interval 5s, got %v", tier.ProbeInterval)
		}
		if tier.AgentSelection.Strategy != "all" {
			t.Errorf("expected strategy 'all', got %s", tier.AgentSelection.Strategy)
		}
	})

	t.Run("vip variant", func(t *testing.T) {
		tier := FixtureTierVIP()
		if tier.Name != "vip" {
			t.Errorf("expected name 'vip', got %s", tier.Name)
		}
		if tier.AgentSelection.Diversity == nil {
			t.Error("expected diversity requirements for VIP tier")
		}
	})
}

func TestFixtureProbeResult(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		result := FixtureProbeResult("agent-1", "target-1")
		if !result.Success {
			t.Error("expected successful result")
		}
		if result.Duration == 0 {
			t.Error("expected non-zero duration for success")
		}
	})

	t.Run("failed", func(t *testing.T) {
		result := FixtureProbeResultFailed("agent-1", "target-1")
		if result.Success {
			t.Error("expected failed result")
		}
		if result.Error == "" {
			t.Error("expected error message")
		}
	})
}

func TestHelperFunctions(t *testing.T) {
	t.Run("Ptr", func(t *testing.T) {
		intPtr := Ptr(42)
		if *intPtr != 42 {
			t.Errorf("expected 42, got %d", *intPtr)
		}

		strPtr := Ptr("hello")
		if *strPtr != "hello" {
			t.Errorf("expected 'hello', got %s", *strPtr)
		}
	})

	t.Run("TimeAgo", func(t *testing.T) {
		past := TimeAgo(5 * time.Minute)
		expected := 5 * time.Minute
		actual := time.Since(past)
		if actual < expected-time.Second || actual > expected+time.Second {
			t.Errorf("expected ~%v ago, got %v ago", expected, actual)
		}
	})

	t.Run("TimeAgoPtr", func(t *testing.T) {
		past := TimeAgoPtr(10 * time.Minute)
		if past == nil {
			t.Error("expected non-nil pointer")
		}
		expected := 10 * time.Minute
		actual := time.Since(*past)
		if actual < expected-time.Second || actual > expected+time.Second {
			t.Errorf("expected ~%v ago, got %v ago", expected, actual)
		}
	})
}
