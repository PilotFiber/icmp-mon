// Package config handles agent configuration loading and validation.
//
// # Configuration Sources
//
// Configuration is loaded from (in order of precedence):
// 1. Command-line flags
// 2. Environment variables (ICMPMON_*)
// 3. Config file (YAML)
// 4. Defaults
//
// # Example Config File
//
//	control_plane:
//	  url: https://monitor.pilot.net
//	  token: pmon_xxx
//
//	agent:
//	  name: aws-us-east-01
//	  region: us-east
//	  location: AWS us-east-1a
//	  provider: aws
//	  tags:
//	    network_type: external
//	    datacenter: us-east-1a
//
//	probing:
//	  result_batch_size: 1000
//	  result_batch_timeout: 5s
//
//	health:
//	  heartbeat_interval: 30s
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the complete agent configuration.
type Config struct {
	ControlPlane ControlPlaneConfig `yaml:"control_plane"`
	Agent        AgentConfig        `yaml:"agent"`
	Probing      ProbingConfig      `yaml:"probing"`
	Health       HealthConfig       `yaml:"health"`
	Tiers        map[string]TierConfig `yaml:"tiers,omitempty"` // Optional local tier overrides
}

// ControlPlaneConfig defines how to connect to the control plane.
type ControlPlaneConfig struct {
	URL   string `yaml:"url"`   // e.g., https://monitor.pilot.net
	Token string `yaml:"token"` // Enrollment or auth token

	// TLS settings
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify,omitempty"`
	CACertFile         string `yaml:"ca_cert_file,omitempty"`

	// Timeouts
	ConnectTimeout time.Duration `yaml:"connect_timeout,omitempty"`
	RequestTimeout time.Duration `yaml:"request_timeout,omitempty"`
}

// AgentConfig defines agent identity and metadata.
type AgentConfig struct {
	Name     string            `yaml:"name"`     // Unique agent name
	Region   string            `yaml:"region"`   // Region identifier for selection
	Location string            `yaml:"location"` // Human-readable location
	Provider string            `yaml:"provider"` // Provider name (aws, vultr, etc.)
	Tags     map[string]string `yaml:"tags"`     // Custom tags for selection
}

// ProbingConfig defines probing behavior.
type ProbingConfig struct {
	// Result batching
	ResultBatchSize    int           `yaml:"result_batch_size"`
	ResultBatchTimeout time.Duration `yaml:"result_batch_timeout"`

	// Assignment sync
	AssignmentPollInterval time.Duration `yaml:"assignment_poll_interval"`

	// Command polling
	CommandPollInterval time.Duration `yaml:"command_poll_interval"`

	// Executor settings
	FpingPath string `yaml:"fping_path,omitempty"`
	MTRPath   string `yaml:"mtr_path,omitempty"`
}

// HealthConfig defines health monitoring behavior.
type HealthConfig struct {
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
}

// TierConfig allows local tier definition/override.
type TierConfig struct {
	ProbeInterval time.Duration `yaml:"probe_interval"`
	ProbeTimeout  time.Duration `yaml:"probe_timeout"`
	ProbeRetries  int           `yaml:"probe_retries"`
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		ControlPlane: ControlPlaneConfig{
			ConnectTimeout: 10 * time.Second,
			RequestTimeout: 30 * time.Second,
		},
		Agent: AgentConfig{
			Tags: make(map[string]string),
		},
		Probing: ProbingConfig{
			ResultBatchSize:        1000,
			ResultBatchTimeout:     5 * time.Second,
			AssignmentPollInterval: 30 * time.Second,
			CommandPollInterval:    5 * time.Second,
		},
		Health: HealthConfig{
			HeartbeatInterval: 30 * time.Second,
		},
	}
}

// LoadFromFile loads configuration from a YAML file.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return cfg, nil
}

// Validate checks that required configuration is present.
func (c *Config) Validate() error {
	if c.ControlPlane.URL == "" {
		return fmt.Errorf("control_plane.url is required")
	}
	if c.Agent.Name == "" {
		return fmt.Errorf("agent.name is required")
	}
	return nil
}

// ApplyEnvOverrides applies environment variable overrides.
// Environment variables use ICMPMON_ prefix:
// - ICMPMON_CONTROL_PLANE_URL
// - ICMPMON_CONTROL_PLANE_TOKEN
// - ICMPMON_AGENT_NAME
// - ICMPMON_AGENT_REGION
// - ICMPMON_AGENT_LOCATION
// - ICMPMON_AGENT_PROVIDER
// - ICMPMON_AGENT_TAGS (JSON object, e.g., '{"pilot_pop":"NYC1"}')
func (c *Config) ApplyEnvOverrides() {
	if v := os.Getenv("ICMPMON_CONTROL_PLANE_URL"); v != "" {
		c.ControlPlane.URL = v
	}
	if v := os.Getenv("ICMPMON_CONTROL_PLANE_TOKEN"); v != "" {
		c.ControlPlane.Token = v
	}
	if v := os.Getenv("ICMPMON_AGENT_NAME"); v != "" {
		c.Agent.Name = v
	}
	if v := os.Getenv("ICMPMON_AGENT_REGION"); v != "" {
		c.Agent.Region = v
	}
	if v := os.Getenv("ICMPMON_AGENT_LOCATION"); v != "" {
		c.Agent.Location = v
	}
	if v := os.Getenv("ICMPMON_AGENT_PROVIDER"); v != "" {
		c.Agent.Provider = v
	}
	if v := os.Getenv("ICMPMON_AGENT_TAGS"); v != "" {
		var tags map[string]string
		if err := json.Unmarshal([]byte(v), &tags); err == nil {
			if c.Agent.Tags == nil {
				c.Agent.Tags = make(map[string]string)
			}
			for k, val := range tags {
				c.Agent.Tags[k] = val
			}
		}
	}
}
