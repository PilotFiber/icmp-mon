// Package client provides the control plane API client for agents.
//
// # Operations
//
// - Register: Initial agent registration
// - Heartbeat: Periodic health reporting
// - GetAssignments: Fetch target assignments
// - GetCommands: Poll for on-demand commands
// - ReportCommandResult: Return command execution results
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pilot-net/icmp-mon/pkg/types"
)

// Client communicates with the control plane.
type Client struct {
	baseURL    string
	httpClient *http.Client
	agentID    string
	authToken  string
}

// Config for the client.
type Config struct {
	BaseURL            string
	AuthToken          string
	HTTPClient         *http.Client
	InsecureSkipVerify bool
}

// NewClient creates a new control plane client.
func NewClient(cfg Config) *Client {
	if cfg.HTTPClient == nil {
		transport := &http.Transport{}
		if cfg.InsecureSkipVerify {
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}
		cfg.HTTPClient = &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		}
	}
	return &Client{
		baseURL:    cfg.BaseURL,
		httpClient: cfg.HTTPClient,
		authToken:  cfg.AuthToken,
	}
}

// SetAgentID sets the agent ID after registration.
func (c *Client) SetAgentID(id string) {
	c.agentID = id
}

// AgentID returns the current agent ID.
func (c *Client) AgentID() string {
	return c.agentID
}

// RegisterRequest is sent to register a new agent.
type RegisterRequest struct {
	Name        string            `json:"name"`
	Region      string            `json:"region"`
	Location    string            `json:"location"`
	Provider    string            `json:"provider"`
	Tags        map[string]string `json:"tags,omitempty"`
	PublicIP    string            `json:"public_ip"`
	Version     string            `json:"version"`
	Executors   []string          `json:"executors"`
	MaxTargets  int               `json:"max_targets"`
}

// RegisterResponse is returned from agent registration.
type RegisterResponse struct {
	AgentID string `json:"agent_id"`
	Message string `json:"message,omitempty"`
}

// Register registers the agent with the control plane.
func (c *Client) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/v1/agents/register", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, c.readError(resp)
	}

	var result RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	c.agentID = result.AgentID
	return &result, nil
}

// Heartbeat sends a health report to the control plane.
func (c *Client) Heartbeat(ctx context.Context, heartbeat types.Heartbeat) (*types.HeartbeatResponse, error) {
	path := fmt.Sprintf("/api/v1/agents/%s/heartbeat", c.agentID)
	resp, err := c.doRequest(ctx, "POST", path, heartbeat)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}

	var result types.HeartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}

// GetAssignments fetches target assignments for this agent.
// If sinceVersion is provided, returns only changes since that version.
func (c *Client) GetAssignments(ctx context.Context, sinceVersion int64) (*types.AssignmentSet, error) {
	path := fmt.Sprintf("/api/v1/agents/%s/assignments", c.agentID)
	if sinceVersion > 0 {
		path = fmt.Sprintf("%s?since=%d", path, sinceVersion)
	}

	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}

	var result types.AssignmentSet
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}

// GetCommands polls for pending on-demand commands.
func (c *Client) GetCommands(ctx context.Context) ([]types.Command, error) {
	path := fmt.Sprintf("/api/v1/agents/%s/commands", c.agentID)
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}

	var result struct {
		Commands []types.Command `json:"commands"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return result.Commands, nil
}

// ReportCommandResult sends the result of an executed command.
func (c *Client) ReportCommandResult(ctx context.Context, result types.CommandResult) error {
	path := fmt.Sprintf("/api/v1/agents/%s/commands/%s/result", c.agentID, result.CommandID)
	resp, err := c.doRequest(ctx, "POST", path, result)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return c.readError(resp)
	}

	return nil
}

// Ping tests connectivity to the control plane.
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.doRequest(ctx, "GET", "/api/v1/health", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}

	return nil
}

// doRequest performs an HTTP request with standard headers.
func (c *Client) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "icmpmon-agent/1.0")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	if c.agentID != "" {
		req.Header.Set("X-Agent-ID", c.agentID)
	}

	return c.httpClient.Do(req)
}

// readError extracts an error message from a failed response.
func (c *Client) readError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
}
