// Package executor defines the plugin interface for probe types.
//
// # Design Principles
//
// 1. Interface Segregation: Small, focused interface that all probes implement
// 2. Batching Support: Executors can optimize for batch execution (fping)
// 3. Capability Declaration: Executors declare their requirements and limits
// 4. Graceful Degradation: Missing dependencies detected at registration, not runtime
//
// # Adding New Executors
//
// To add a new probe type:
//
//  1. Create a new file (e.g., http.go) implementing the Executor interface
//  2. Define parameter and result structs for your probe type
//  3. Register the executor in the registry
//
// Example:
//
//	type HTTPExecutor struct { /* ... */ }
//	func (e *HTTPExecutor) Type() string { return "http_check" }
//	func (e *HTTPExecutor) Execute(ctx, target) (*Result, error) { /* ... */ }
//
//	// In agent startup:
//	registry.Register(&HTTPExecutor{})
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// Executor is the interface all probe types implement.
//
// Executors are responsible for:
// - Executing probes against targets
// - Returning structured results
// - Declaring their capabilities and dependencies
type Executor interface {
	// Type returns the unique identifier for this executor (e.g., "icmp_ping")
	Type() string

	// Capabilities returns what this executor can do and what it needs
	Capabilities() Capabilities

	// Execute runs a single probe and returns the result
	Execute(ctx context.Context, target ProbeTarget) (*Result, error)

	// ExecuteBatch runs probes for multiple targets efficiently.
	// If SupportsBatching is false, this can delegate to individual Execute calls.
	ExecuteBatch(ctx context.Context, targets []ProbeTarget) ([]*Result, error)
}

// Capabilities describes an executor's requirements and limits.
type Capabilities struct {
	// SupportsBatching indicates the executor can efficiently probe many targets at once
	SupportsBatching bool

	// MaxBatchSize is the maximum targets per batch (if batching supported)
	MaxBatchSize int

	// RequiresRoot indicates the executor needs elevated privileges
	RequiresRoot bool

	// Dependencies lists external binaries required (e.g., ["fping", "mtr"])
	Dependencies []string
}

// ProbeTarget contains everything needed to probe a single target.
type ProbeTarget struct {
	ID       string          `json:"id"`
	IP       string          `json:"ip"`
	Timeout  time.Duration   `json:"timeout"`
	Retries  int             `json:"retries"`
	Params   json.RawMessage `json:"params,omitempty"` // Executor-specific params
}

// Result is the outcome of a probe execution.
type Result struct {
	TargetID  string          `json:"target_id"`
	Timestamp time.Time       `json:"timestamp"`
	Duration  time.Duration   `json:"duration"`
	Success   bool            `json:"success"`
	Error     string          `json:"error,omitempty"`
	Payload   json.RawMessage `json:"payload"` // Executor-specific result data
}

// =============================================================================
// REGISTRY
// =============================================================================

// Registry manages available executors.
type Registry struct {
	executors map[string]Executor
	mu        sync.RWMutex
}

// NewRegistry creates a new executor registry.
func NewRegistry() *Registry {
	return &Registry{
		executors: make(map[string]Executor),
	}
}

// Register adds an executor to the registry.
// Returns an error if dependencies are missing or executor already registered.
func (r *Registry) Register(e Executor) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	typ := e.Type()
	if _, exists := r.executors[typ]; exists {
		return fmt.Errorf("executor already registered: %s", typ)
	}

	// Verify dependencies are available
	caps := e.Capabilities()
	for _, dep := range caps.Dependencies {
		if _, err := exec.LookPath(dep); err != nil {
			return fmt.Errorf("executor %s missing dependency: %s", typ, dep)
		}
	}

	r.executors[typ] = e
	return nil
}

// Get returns an executor by type.
func (r *Registry) Get(typ string) (Executor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.executors[typ]
	return e, ok
}

// List returns all registered executor types.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]string, 0, len(r.executors))
	for t := range r.executors {
		types = append(types, t)
	}
	return types
}

// ListCapabilities returns capabilities for all registered executors.
func (r *Registry) ListCapabilities() map[string]Capabilities {
	r.mu.RLock()
	defer r.mu.RUnlock()
	caps := make(map[string]Capabilities, len(r.executors))
	for t, e := range r.executors {
		caps[t] = e.Capabilities()
	}
	return caps
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// MarshalPayload converts a typed payload to json.RawMessage.
// Use this to create Result.Payload from executor-specific structs.
func MarshalPayload(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		// This shouldn't happen with our types, but fallback to empty object
		return json.RawMessage(`{}`)
	}
	return data
}

// UnmarshalPayload extracts a typed payload from json.RawMessage.
func UnmarshalPayload[T any](data json.RawMessage) (T, error) {
	var v T
	err := json.Unmarshal(data, &v)
	return v, err
}
