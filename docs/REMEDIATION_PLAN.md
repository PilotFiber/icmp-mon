# ICMP-Mon Comprehensive Remediation Plan

## Overview

This plan transforms the ICMP monitoring codebase from a rapid prototype into production-ready, impressive code through systematic improvements to code organization, test coverage, and security.

**Goals:** Production-ready + Portfolio-impressive
**Scope:** Comprehensive remediation
**Priority:** Code organization (primary), with security and testing throughout

---

## Phase 1: Foundation & Configuration (Days 1-2)

### 1.1 Create Configuration Constants Package

**New file:** `control-plane/internal/config/constants.go`

Centralize hardcoded values scattered throughout the codebase:

```go
package config

import "time"

// Agent health thresholds
const (
    AgentDegradedThreshold = 30 * time.Second
    AgentOfflineThreshold  = 60 * time.Second
)

// SQL interval strings for queries
const (
    SQLAgentDegradedInterval = "30 seconds"
    SQLAgentOfflineInterval  = "60 seconds"
)

// Batch processing
const (
    DefaultResultBatchSize = 1000
    BufferFlushBatchSize   = 20000
    MaxPaginationLimit     = 500
    DefaultPaginationLimit = 50
)

// Cache TTLs
const (
    CacheTTLFleetOverview = 30 * time.Second
    CacheTTLTargetList    = 60 * time.Second
    // ... etc
)
```

**Files to update with constants:**
- `control-plane/internal/api/api.go` (lines 67-75)
- `control-plane/internal/buffer/buffer.go` (lines 22-27)
- `control-plane/internal/store/store.go` (multiple heartbeat intervals)

### 1.2 Create PostgreSQL Function for Agent Status

**New migration:** `db/migrations/021_agent_status_function.sql`

Extract duplicated agent status SQL (appears 6 times) into reusable function:

```sql
CREATE OR REPLACE FUNCTION get_agent_status(
    p_last_heartbeat TIMESTAMPTZ,
    p_archived_at TIMESTAMPTZ
) RETURNS TEXT AS $$
BEGIN
    IF p_archived_at IS NOT NULL THEN RETURN 'offline'; END IF;
    IF p_last_heartbeat IS NULL OR
       p_last_heartbeat < NOW() - INTERVAL '60 seconds' THEN RETURN 'offline'; END IF;
    IF p_last_heartbeat < NOW() - INTERVAL '30 seconds' THEN RETURN 'degraded'; END IF;
    RETURN 'active';
END;
$$ LANGUAGE plpgsql IMMUTABLE;
```

**Files to update:**
- `control-plane/internal/store/store.go` (lines 79-84, 108-113, 136-141)
- `control-plane/internal/store/store_assignments.go` (lines 289-294, 311-322)

### 1.3 Setup Test Infrastructure

**New file:** `control-plane/internal/testutil/testutil.go`

```go
package testutil

// Test helpers
func NewTestLogger() *slog.Logger
func NewTestServer(store Store) (*httptest.Server, *api.Server)

// Fixtures
func FixtureAgent(overrides ...func(*types.Agent)) *types.Agent
func FixtureTarget(overrides ...func(*types.Target)) *types.Target
func FixtureSubnet(overrides ...func(*types.Subnet)) *types.Subnet
```

**New file:** `control-plane/internal/testutil/mockstore.go`

Comprehensive mock implementing Store interface with:
- In-memory maps for all entity types
- Method call tracking for verification
- Error injection support for testing error paths

---

## Phase 2: Code Organization - Split Large Files (Days 3-6)

### 2.1 Split store.go (3,716 lines → ~10 files)

**Execution order** (each should compile and pass tests before proceeding):

| Order | New File | Content | ~Lines |
|-------|----------|---------|--------|
| 1 | `store_agents.go` | Agent CRUD, heartbeat, archive | 600 |
| 2 | `store_targets.go` | Target CRUD, listing, pagination | 400 |
| 3 | `store_tiers.go` | Tier configuration | 200 |
| 4 | `store_probes.go` | Probe result insertion, helpers | 350 |
| 5 | `store_metrics.go` | Target status, latency trends, matrix | 600 |
| 6 | `store_commands.go` | MTR and command handling | 250 |
| 7 | `store_baselines.go` | Baseline operations | 200 |
| 8 | `store_incidents.go` | Incident CRUD | 250 |
| 9 | `store_reporting.go` | Report generation | 150 |
| 10 | `store_query.go` | Flexible metrics query builder | 700 |

**Keep in `store.go`:** Store struct, constructor, pool access, ping (~350 lines)

### 2.2 Split api.go (1,721 lines → ~8 files)

| Order | New File | Content | ~Lines |
|-------|----------|---------|--------|
| 1 | `api_agents.go` | Agent endpoints, metrics endpoints | 350 |
| 2 | `api_targets.go` | Target endpoints, status endpoints | 250 |
| 3 | `api_tiers.go` | Tier endpoints | 150 |
| 4 | `api_results.go` | Results ingestion | 100 |
| 5 | `api_metrics.go` | Metrics endpoints | 200 |
| 6 | `api_commands.go` | Command endpoints | 100 |
| 7 | `api_incidents.go` | Incident endpoints | 150 |
| 8 | `api_baselines.go` | Baseline endpoints | 100 |

**Keep in `api.go`:** Server struct, NewServer(), registerRoutes(), ServeHTTP(), helpers (~300 lines)

### 2.3 Address Ignored Errors

**Category A - Add explanatory comments (acceptable):**
- Activity logging in `store_subnets.go` (lines 73, 519, 1271)
- Cleanup operations in `enrollment/rollback.go`, `enrollment/tailscale.go`

**Category B - Must fix:**
- `store_alerts.go:1334` - `rand.Read` error should be handled
- Any JSON marshal errors on user-provided data

### 2.4 Fix Global Mutable State

**File:** `control-plane/internal/enrollment/steps.go`

Move `activeSessions` map from package-level to Service struct:

```go
type Service struct {
    // ... existing fields ...
    sessionsMu sync.RWMutex
    sessions   map[string]*sshSession
}
```

Add thread-safe accessor methods: `getSession()`, `setSession()`, `deleteSession()`

---

## Phase 3: Security Hardening (Days 7-9)

### 3.1 CORS Configuration (HIGH - Quick Win)

**New file:** `control-plane/internal/api/cors.go`

```go
type CORSConfig struct {
    AllowedOrigins   []string
    AllowCredentials bool
    MaxAge           int
}

func CORSMiddleware(cfg CORSConfig) func(http.Handler) http.Handler
```

**Update:** `control-plane/internal/api/api.go` (lines 116-124)
- Remove `Access-Control-Allow-Origin: *`
- Use configurable middleware with environment variable: `CORS_ALLOWED_ORIGINS`

### 3.2 InsecureSkipVerify Warnings (MEDIUM)

**Update:** `agent/cmd/agent/main.go`

Add startup warning when `InsecureSkipVerify` is enabled:
```go
if cfg.ControlPlane.InsecureSkipVerify {
    logger.Warn("TLS VERIFICATION DISABLED - vulnerable to MITM attacks")
}
```

**Update:** `agent/internal/config/config.go`

Add `CACertFile` option as preferred alternative to `InsecureSkipVerify`

### 3.3 SSH Host Key Verification (HIGH - Requires Migration)

**New migration:** `db/migrations/022_ssh_host_keys.sql`

```sql
CREATE TABLE ssh_host_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_ip TEXT NOT NULL,
    host_port INTEGER NOT NULL DEFAULT 22,
    key_type TEXT NOT NULL,
    public_key_blob BYTEA NOT NULL,
    fingerprint TEXT NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    verified_by TEXT,
    UNIQUE(host_ip, host_port, key_type)
);
```

**New file:** `control-plane/internal/enrollment/hostkeys.go`

Implement Trust-On-First-Use (TOFU) pattern:
- Store host keys on first connection
- Verify on subsequent connections
- Alert on key change (potential MITM)

**Update:** `control-plane/internal/enrollment/ssh.go` (line 62)
- Replace `InsecureIgnoreHostKey()` with custom callback using host key store

### 3.4 Input Validation

**New file:** `pkg/validate/validate.go`

```go
func IP(s string) error
func CIDR(s string) error
func SafeName(s string) error  // Alphanumeric + dash/underscore
func Pagination(limit, offset, max int) (int, int, error)
func Tags(tags map[string]string) error
```

Apply validation to API handlers in `api.go` for user-provided input.

### 3.5 Sensitive Data Handling

**New file:** `pkg/types/sensitive.go`

```go
type Sensitive string

func (s Sensitive) String() string { return "[REDACTED]" }
func (s Sensitive) MarshalJSON() ([]byte, error) { return json.Marshal(nil) }
func (s Sensitive) LogValue() slog.Value { return slog.StringValue("[REDACTED]") }
func (s Sensitive) Value() string { return string(s) }
```

**Update:** `agent/internal/config/config.go`
- Change `Token string` to `Token types.Sensitive`

---

## Phase 4: Test Coverage (Days 10-16)

### 4.1 Test Infrastructure (Day 10)

**Docker Compose for integration tests:** `docker-compose.test.yml`

```yaml
services:
  postgres-test:
    image: timescale/timescaledb:latest-pg16
    environment:
      POSTGRES_DB: icmpmon_test
      POSTGRES_USER: test
      POSTGRES_PASSWORD: test
    ports:
      - "5433:5432"
  redis-test:
    image: redis:7-alpine
    ports:
      - "6380:6379"
```

### 4.2 API Handler Tests (Days 11-12) - Highest Impact

**New files:**
- `control-plane/internal/api/api_test.go` - Core endpoints
- `control-plane/internal/api/api_agents_test.go` - Agent endpoints
- `control-plane/internal/api/api_targets_test.go` - Target endpoints
- `control-plane/internal/api/middleware_test.go` - Auth middleware

**Priority endpoints to test:**
1. `POST /agents/register` - Agent registration
2. `POST /agents/{id}/heartbeat` - Heartbeat processing
3. `GET /agents/{id}/assignments` - Assignment retrieval
4. `POST /results` - Result ingestion
5. `GET /targets` - Target listing with pagination
6. `POST /targets` - Target creation with validation

### 4.3 Service Layer Tests (Days 13-14)

**New files:**
- `control-plane/internal/service/service_test.go`
- `control-plane/internal/service/state_machine_test.go`
- `control-plane/internal/service/rebalancer_test.go`

**Key tests:**
- Agent registration (new vs update)
- Assignment calculation (all vs distributed strategy)
- Result ingestion with buffering
- State transitions (unknown→active→down→excluded)

### 4.4 Database Integration Tests (Day 15)

**New files:**
- `control-plane/internal/store/store_test.go`
- `control-plane/internal/store/store_agents_test.go`
- `control-plane/internal/store/store_targets_test.go`

Use build tags to separate unit and integration tests:
```go
//go:build integration

package store_test
```

**Run commands:**
```bash
go test ./... -tags=unit          # Fast, mocks only
go test ./... -tags=integration   # Requires Docker
go test ./...                     # All tests
```

### 4.5 Worker Tests (Day 16)

**New files:**
- `control-plane/internal/worker/evaluator_worker_test.go`
- `control-plane/internal/worker/assignment_worker_test.go`
- `control-plane/internal/worker/alert_worker_test.go`

---

## Phase 5: Polish & Documentation (Days 17-18)

### 5.1 Resolve TODO Comments

Address the 7 TODO/FIXME comments found:
1. `agent/agent.go:497` - ProbeInterval for production (make configurable)
2. `agent/agent.go:208` - MaxTargets (add to config)
3. `agent/agent.go:446` - Report update failure (implement)
4. `control-plane/internal/buffer/flusher.go:106` - Dead-letter queue (add or document)
5. `control-plane/internal/service/service.go:433` - Consistent hashing (implement or document limitation)
6. `control-plane/internal/api/rollout.go` - Multiple unimplemented endpoints (implement or remove routes)

### 5.2 Update Documentation

- Update `DEVELOPMENT.md` with test commands
- Add security configuration section
- Document new environment variables

---

## Implementation Checklist

### Phase 1: Foundation (Days 1-2)
- [ ] Create `config/constants.go` with centralized values
- [ ] Create migration `021_agent_status_function.sql`
- [ ] Update store files to use `get_agent_status()` function
- [ ] Create `testutil/testutil.go` with helpers
- [ ] Create `testutil/mockstore.go` with mock implementation

### Phase 2: Code Organization (Days 3-6)
- [ ] Split `store.go` into 10 files (in order listed above)
- [ ] Split `api.go` into 8 files (in order listed above)
- [ ] Add comments to acceptable ignored errors
- [ ] Fix `rand.Read` error handling in `store_alerts.go`
- [ ] Move `activeSessions` to Service struct

### Phase 3: Security (Days 7-9)
- [ ] Create CORS middleware with configuration
- [ ] Add InsecureSkipVerify startup warning
- [ ] Add CACertFile config option for agents
- [ ] Create migration `022_ssh_host_keys.sql`
- [ ] Implement TOFU host key verification
- [ ] Create validation package
- [ ] Apply input validation to API handlers
- [ ] Create Sensitive type wrapper
- [ ] Update Token field to use Sensitive type

### Phase 4: Testing (Days 10-16)
- [ ] Create `docker-compose.test.yml`
- [ ] Create test fixtures and helpers
- [ ] Write API handler tests (~40 endpoints)
- [ ] Write service layer tests
- [ ] Write database integration tests
- [ ] Write worker tests

### Phase 5: Polish (Days 17-18)
- [ ] Address all TODO comments
- [ ] Update documentation
- [ ] Final code review pass

---

## Critical Files Reference

| File | Lines | Action |
|------|-------|--------|
| `control-plane/internal/store/store.go` | 3,716 | Split into 10 files |
| `control-plane/internal/api/api.go` | 1,721 | Split into 8 files |
| `control-plane/internal/store/store_assignments.go` | 436 | Update agent status SQL |
| `control-plane/internal/enrollment/ssh.go` | 127 | Fix host key verification |
| `control-plane/internal/enrollment/steps.go` | 901 | Fix global state |
| `agent/internal/config/config.go` | 199 | Add Sensitive type, CACertFile |

---

## Expected Outcomes

**Code Quality:**
- No file over 800 lines
- No duplicated SQL patterns
- Centralized configuration
- Thread-safe session management

**Security:**
- CORS restricted to configured origins
- SSH host key verification (TOFU)
- Input validation on all endpoints
- Sensitive data redacted from logs

**Test Coverage:**
- ~60-70% code coverage
- All API endpoints tested
- Database layer integration tests
- Worker behavior tests

**Impressiveness:**
- Clean, well-organized codebase
- Professional security practices
- Comprehensive test suite
- Production-ready configuration
