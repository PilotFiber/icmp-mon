# ICMP-Mon Development Guidelines

These guidelines ensure consistent, high-quality code throughout the ICMP monitoring system.

## Core Principles

### 1. DRY (Don't Repeat Yourself)

**Before writing new code:**
- Search for existing implementations that solve the same problem
- Check for similar patterns in the codebase that can be extracted into shared utilities
- If you find yourself copying code, stop and create an abstraction

**Specific patterns to watch for:**
- SQL fragments (use PostgreSQL functions or Go constants)
- Error handling patterns (create helper functions)
- HTTP response writing (use existing `writeJSON`, `writeError` helpers)
- Configuration values (use `config/constants.go`)

**Example - Bad:**
```go
// In store.go
CASE WHEN last_heartbeat < NOW() - INTERVAL '60 seconds' THEN 'offline' ...

// In store_assignments.go (same logic repeated)
CASE WHEN last_heartbeat < NOW() - INTERVAL '60 seconds' THEN 'offline' ...
```

**Example - Good:**
```go
// Use the PostgreSQL function
get_agent_status(last_heartbeat, archived_at) as status
```

### 2. Test-Driven Development

**Write tests as you go, not after:**
- When creating a new function, write at least one test immediately
- When fixing a bug, write a test that reproduces it first
- When refactoring, ensure existing tests pass before and after

**Test file naming:**
- Unit tests: `*_test.go` in the same package
- Integration tests: Use `//go:build integration` tag

**Test patterns to follow:**
```go
func TestFunctionName_Scenario(t *testing.T) {
    // Arrange
    // Act
    // Assert
}

// Table-driven tests for multiple cases
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {"valid input", "test", "TEST", false},
        {"empty input", "", "", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}
```

### 3. File Size Limits

**Keep files under 800 lines.** If a file grows larger:
- Split by domain/responsibility (e.g., `store.go` → `store_agents.go`, `store_targets.go`)
- Extract utilities into separate files
- Create sub-packages if appropriate

### 4. Error Handling

**Never ignore errors silently:**
```go
// Bad
_, _ = doSomething()

// Good - if error is truly ignorable, document why
_, _ = doSomething() // Best-effort logging, failure doesn't affect main operation

// Better - handle the error
if _, err := doSomething(); err != nil {
    return fmt.Errorf("doing something: %w", err)
}
```

### 5. Configuration

**No magic numbers in code:**
```go
// Bad
if time.Since(lastHeartbeat) > 60*time.Second {

// Good
if time.Since(lastHeartbeat) > config.AgentOfflineThreshold {
```

**Use environment variables for deployment-specific values:**
- Database URLs
- API keys
- Feature flags
- CORS origins

### 6. Security Practices

**Always validate input:**
```go
if err := validate.IP(req.IP); err != nil {
    return fmt.Errorf("invalid IP: %w", err)
}
```

**Never log sensitive data:**
```go
// Bad
logger.Info("connecting", "token", cfg.Token)

// Good - use Sensitive type
logger.Info("connecting", "token", cfg.Token) // Token is types.Sensitive, auto-redacts
```

**Use parameterized queries:**
```go
// Bad
query := fmt.Sprintf("SELECT * FROM agents WHERE name = '%s'", name)

// Good
query := "SELECT * FROM agents WHERE name = $1"
row := db.QueryRow(ctx, query, name)
```

### 7. Code Organization

**Package structure:**
```
control-plane/
  internal/
    api/          # HTTP handlers
    service/      # Business logic
    store/        # Database access
    worker/       # Background jobs
    config/       # Configuration
    testutil/     # Test helpers (not shipped)
```

**File naming conventions:**
- `store.go` - Core struct and constructor
- `store_agents.go` - Agent-specific operations
- `store_agents_test.go` - Tests for agent operations

### 8. Documentation

**Document public APIs:**
```go
// RegisterAgent registers a new agent or updates an existing one.
// It returns the agent's UUID and any error encountered.
func (s *Service) RegisterAgent(ctx context.Context, req RegisterRequest) (string, error)
```

**Don't over-document obvious code:**
```go
// Bad
// GetName returns the name
func (a *Agent) GetName() string { return a.Name }

// Good - no comment needed, the function is self-documenting
func (a *Agent) GetName() string { return a.Name }
```

## Workflow

### Before Starting Work

1. Read relevant existing code to understand patterns
2. Check `docs/REMEDIATION_PLAN.md` for context on ongoing improvements
3. Run existing tests: `go test ./...`

### While Working

1. Make small, focused commits
2. Write tests alongside implementation
3. Run tests frequently: `go test ./...`
4. Check for DRY violations before committing

### Before Committing

1. Run full test suite: `go test ./...`
2. Run linter: `go vet ./...`
3. Check for hardcoded values that should be constants
4. Verify no sensitive data in logs or error messages

## Common Patterns

### HTTP Handler
```go
func (s *Server) handleCreateThing(w http.ResponseWriter, r *http.Request) {
    var req CreateThingRequest
    if err := s.readJSON(r, &req); err != nil {
        s.writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    if err := validate.SafeName(req.Name); err != nil {
        s.writeError(w, http.StatusBadRequest, err.Error())
        return
    }

    thing, err := s.service.CreateThing(r.Context(), req)
    if err != nil {
        s.logger.Error("failed to create thing", "error", err)
        s.writeError(w, http.StatusInternalServerError, "failed to create thing")
        return
    }

    s.writeJSON(w, http.StatusCreated, thing)
}
```

### Database Query
```go
func (s *Store) GetThing(ctx context.Context, id string) (*types.Thing, error) {
    query := `
        SELECT id, name, created_at
        FROM things
        WHERE id = $1
    `

    var thing types.Thing
    err := s.pool.QueryRow(ctx, query, id).Scan(
        &thing.ID,
        &thing.Name,
        &thing.CreatedAt,
    )
    if err == pgx.ErrNoRows {
        return nil, ErrNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("querying thing: %w", err)
    }

    return &thing, nil
}
```

### Background Worker
```go
func (w *Worker) Run(ctx context.Context) error {
    ticker := time.NewTicker(w.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if err := w.runOnce(ctx); err != nil {
                w.logger.Error("worker iteration failed", "error", err)
                // Continue running, don't exit on transient errors
            }
        }
    }
}
```
