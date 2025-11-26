# Pilot IP Pool Monitoring Design

**Status:** Draft / Discussion
**Date:** 2025-01-25
**Authors:** Design discussion captured from planning session

## Problem Statement

Pilot directly owns/operates 50k+ IPv4 addresses that constantly rotate between new and cancelled customers. We need to:

1. **See which IPs are responding to ping at any time**
2. **Classify IPs by type:**
   - **Gateway IPs** - Pilot-owned gateways that deprioritize ICMP (unreliable for latency measurements)
   - **Infrastructure IPs** - Pilot servers and network devices
   - **Customer IPs** - Customer-facing addresses with variable reachability
3. **Handle customer IP variability:**
   - Some customers have firewalls that never respond (this is OK/expected)
   - Detect when previously-unreachable IPs become reachable (start monitoring)
   - Detect when reachable IPs stop responding (possibly intentionally)
   - Auto-exclude intentionally-unreachable IPs from active monitoring

## Current Architecture Context

The existing icmp-mon system has:

- **Tag system**: Flexible key-value metadata on targets
- **Tier system**: Different monitoring intensities (infrastructure/vip/standard)
- **Expected outcomes**: Support for "expect failure" scenarios
- **Scalable probe infrastructure**: fping-based, handles thousands of probes/sec

**Gap:** The current system assumes targets are manually curated and always actively monitored. There's no concept of:
- Discovery vs active monitoring
- State-based monitoring intensity
- Automatic state transitions based on reachability

## Proposed Solution: Monitoring State Machine

### IP Classifications (via Tags)

```json
{
  "ip_type": "gateway|infrastructure|customer",
  "customer_id": "cust-123",
  "pop": "NYC1",
  "subnet": "192.0.2.0/24",
  "assignment_date": "2025-01-15"
}
```

### Monitoring States for Customer IPs

```
                    ┌─────────────┐
                    │   UNKNOWN   │  IP assigned, never probed
                    └──────┬──────┘
                           │ first probe
                           ▼
         ┌─────────────────┴─────────────────┐
         │                                   │
         ▼ responds                          ▼ no response after
┌─────────────────┐                 ┌─────────────────┐  N discovery probes
│     ACTIVE      │                 │  UNRESPONSIVE   │
│                 │                 │ (never replied) │
│ Full monitoring │                 │                 │
│ 15-30s interval │                 │ Re-check only if│
└────────┬────────┘                 │ subnet has no   │
         │                          │ active targets  │
         │ stops responding         └────────┬────────┘
         │ for N minutes                     │
         ▼                                   │ periodic re-check
┌─────────────────┐                          │ suddenly responds!
│    DEGRADED     │                          │
│ (investigating) │                          │
│                 │                          │
│ Still monitoring│                          │
│ at full rate    │                          │
└────────┬────────┘                          │
         │                                   │
         │ confirmed down                    │
         │ for N hours                       │
         ▼                                   │
┌─────────────────┐                          │
│    EXCLUDED     │                          │
│ (needs review)  │                          │
│                 │◀─────────────────────────┘
│ Re-check only if│  can transition back
│ subnet has no   │  if suddenly responds
│ active targets  │
└─────────────────┘

Note: INACTIVE is a user-confirmed state (not shown in flow)
      User can manually mark any IP as INACTIVE at any time
```

### State Definitions

| State | Description | Probe Interval | Alerting | Needs Review |
|-------|-------------|----------------|----------|--------------|
| **UNKNOWN** | Newly assigned IP, never probed | 5 min (discovery) | No | No |
| **ACTIVE** | Responds to ICMP, full monitoring | 15-30s (tier-based) | Yes | No |
| **UNRESPONSIVE** | Never responded to discovery probes | Smart re-check* | No | No |
| **INACTIVE** | User-confirmed intentionally unreachable | 1 hour | No | No |
| **DEGRADED** | Was active, stopped responding | 15-30s (maintain rate) | Yes | No |
| **EXCLUDED** | Was active, unreachable for 24h | Smart re-check* | No | **Yes** |

*Smart re-check: Only probe if subnet has no ACTIVE customer targets (see below)

**Archived targets:** Tracked via `archived_at` timestamp, not a monitoring state. Archived targets are removed from Pilot API sync but retained for historical queries.

**Key distinctions:**
- **UNRESPONSIVE**: Auto-transitioned after discovery fails - no review needed (probably firewalled)
- **INACTIVE**: User manually confirmed as "expected to not respond"
- **EXCLUDED**: System auto-transitioned after 24h unreachable from ACTIVE - **requires acknowledgment**

### State Transitions

| From | To | Trigger | Notes |
|------|-----|---------|-------|
| UNKNOWN | ACTIVE | First successful probe response | |
| UNKNOWN | UNRESPONSIVE | N failed discovery probes | No review needed |
| UNKNOWN | INACTIVE | User marks as "expected unreachable" | Manual action |
| ACTIVE | DEGRADED | No response for 5 min | Alerts fire |
| DEGRADED | ACTIVE | Response received | Auto-recovery |
| DEGRADED | EXCLUDED | No response for 24 hours | **Adds to review queue** |
| DEGRADED | INACTIVE | User marks as "expected unreachable" | Clears alerts, no review needed |
| UNRESPONSIVE | ACTIVE | Response received during smart re-check | |
| UNRESPONSIVE | INACTIVE | User marks as "expected unreachable" | Manual action |
| INACTIVE | ACTIVE | Response received during re-check | |
| EXCLUDED | ACTIVE | Response received during smart re-check | Clears from review queue |
| EXCLUDED | INACTIVE | User acknowledges in review queue | Clears from review queue |
| * | archived | Target removed from Pilot API sync | Sets `archived_at` timestamp |

### Needs Review Queue (Hybrid Alerting Model)

When an IP auto-transitions to EXCLUDED after 24h:
1. Alerting stops (no more noise)
2. Target is added to **Needs Review** queue
3. Periodic sweeps continue (daily)
4. Target stays in queue until:
   - **User acknowledges** → moves to INACTIVE (confirmed intentional)
   - **IP responds during sweep** → moves back to ACTIVE (auto-clears)

**UI/API for Review Queue:**
```
GET /api/v1/targets/needs-review
[
  {
    "target_id": "uuid",
    "ip": "192.168.1.5",
    "subscriber": "Acme Corp",
    "pop": "JFK00",
    "excluded_at": "2025-01-25T10:00:00Z",
    "last_seen_active": "2025-01-24T10:00:00Z",
    "reason": "No response for 24 hours"
  }
]

POST /api/v1/targets/{id}/acknowledge
{
  "action": "mark_inactive",  // or "re-enable"
  "notes": "Customer confirmed firewall blocks ICMP"
}
```

### Smart Re-check Logic for Non-Responding IPs

To minimize probe overhead while ensuring coverage, UNRESPONSIVE and EXCLUDED IPs use conditional re-checking:

| Subnet Coverage | Re-check Behavior |
|-----------------|-------------------|
| Subnet has ACTIVE customer targets | **Skip re-checks** - we already have visibility |
| Subnet has NO active customer targets | **Re-check periodically** - need to find a working probe point |

**Rationale:** If we already have an ACTIVE target in a subnet, we have visibility into that customer's network. Probing additional non-responding IPs provides no value. But if a subnet has zero responding IPs, we need to periodically try to find one.

**Manual override:** Users can always manually trigger discovery or mark specific IPs for monitoring regardless of this logic.

### Differentiated Monitoring by IP Type

| IP Type | State Machine | Alerting | Notes |
|---------|---------------|----------|-------|
| **Gateway** | ACTIVE ↔ DEGRADED only | On degradation (not latency) | Never EXCLUDED; deprioritize ICMP |
| **Infrastructure** | ACTIVE ↔ DEGRADED only | Strict, any degradation | Never EXCLUDED; always alert |
| **Customer** | Full state machine | On ACTIVE→DEGRADED | Variable reachability expected |

**Gateway/Infrastructure never reach EXCLUDED:** These are Pilot-owned and should always be monitored. If they stop responding, that's a real problem requiring immediate attention, not a firewall.

## Scale Estimates

Assuming 50k IPs with typical distribution:

| State | Est. Count | Interval | Probes/sec |
|-------|------------|----------|------------|
| Active | ~10,000 | 30s | 333 |
| Discovery (UNKNOWN) | ~5,000 | 5 min | 17 |
| Inactive | ~5,000 | 1 hour | 1.4 |
| Unresponsive | ~25,000 | Smart re-check* | ~1 |
| Excluded | ~5,000 | Smart re-check* | ~0.5 |

*Smart re-check: Only probed if subnet has no ACTIVE customer targets

**Total: ~350-400 probes/sec** - Well within fping capabilities (can do 10k+/sec)

## Data Model

### Subnets (First-Class Entity)

Subnets are synced from Pilot API and own targets:

```sql
CREATE TABLE subnets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Pilot API fields
    pilot_subnet_id INTEGER UNIQUE,
    network_address INET NOT NULL,
    network_size INTEGER NOT NULL,
    gateway_address INET,
    first_usable_address INET,
    last_usable_address INET,

    -- Enriched metadata (from Pilot relationships)
    vlan_id INTEGER,
    service_id INTEGER,
    subscriber_id INTEGER,
    subscriber_name TEXT,

    -- Location metadata
    location_id INTEGER,
    location_address TEXT,
    city TEXT,
    region TEXT,
    pop_name TEXT,

    -- Network topology (CSW today, but field name is future-proof)
    gateway_device TEXT,  -- Core Switch or other gateway device identifier

    -- Lifecycle
    state TEXT DEFAULT 'active',  -- active | archived
    archived_at TIMESTAMPTZ,
    archive_reason TEXT,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_subnets_network ON subnets USING gist (network_address inet_ops);
CREATE INDEX idx_subnets_pilot_id ON subnets(pilot_subnet_id);
CREATE INDEX idx_subnets_subscriber ON subnets(subscriber_id);
CREATE INDEX idx_subnets_service ON subnets(service_id);
CREATE INDEX idx_subnets_location ON subnets(location_id);
CREATE INDEX idx_subnets_pop ON subnets(pop_name);
```

### Targets (IP Addresses)

```sql
CREATE TABLE targets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ip INET UNIQUE NOT NULL,           -- Only one target per IP globally

    -- Relationship to subnet (nullable for orphaned/external targets)
    subnet_id UUID REFERENCES subnets(id),

    -- User-editable fields
    display_name TEXT,                        -- Friendly name, e.g. "Cloudflare DNS Primary"
    tags JSONB DEFAULT '{}',                  -- Flexible user-defined tags

    -- Ownership & Origin
    ownership TEXT NOT NULL DEFAULT 'auto',  -- 'auto' | 'manual'
    origin TEXT,                              -- 'sync' | 'discovery' | 'user' (audit)

    -- Classification
    ip_type TEXT,  -- 'gateway' | 'customer_probe' | 'customer'
    tier TEXT,

    -- Monitoring state machine
    monitoring_state TEXT DEFAULT 'unknown',  -- unknown|active|unresponsive|inactive|degraded|excluded
    state_changed_at TIMESTAMPTZ DEFAULT NOW(),
    needs_review BOOLEAN DEFAULT FALSE,       -- For EXCLUDED IPs only (hybrid alerting model)
    discovery_attempts INTEGER DEFAULT 0,
    last_response_at TIMESTAMPTZ,

    -- Lifecycle (archived is NOT a monitoring state - use this timestamp)
    archived_at TIMESTAMPTZ,
    archive_reason TEXT,  -- 'removed_from_source' | 'subnet_archived' | 'manual'

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_targets_ip ON targets(ip);
CREATE INDEX idx_targets_subnet ON targets(subnet_id);
CREATE INDEX idx_targets_state ON targets(monitoring_state);
CREATE INDEX idx_targets_needs_review ON targets(needs_review) WHERE needs_review = true;
CREATE INDEX idx_targets_active ON targets(archived_at) WHERE archived_at IS NULL;
CREATE INDEX idx_targets_tags ON targets USING gin(tags);
```

**Example tags:**
```json
{
  "environment": "production",
  "team": "noc",
  "notes": "Customer prefers email alerts only",
  "contract_type": "enterprise"
}
```

### Target State History

Track all state transitions for audit and debugging:

```sql
CREATE TABLE target_state_history (
    id BIGSERIAL PRIMARY KEY,
    target_id UUID REFERENCES targets(id),
    from_state TEXT,
    to_state TEXT NOT NULL,
    reason TEXT,
    triggered_by TEXT NOT NULL,  -- 'system', 'discovery', 'user:email@pilot.com'
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_state_history_target ON target_state_history(target_id);
CREATE INDEX idx_state_history_time ON target_state_history(created_at DESC);
```

### Enriched Targets View

Convenience view that joins target with subnet metadata for common queries:

```sql
CREATE VIEW targets_enriched AS
SELECT
    t.id,
    t.ip,
    t.display_name,
    t.tags,
    t.ip_type,
    t.tier,
    t.monitoring_state,
    t.state_changed_at,
    t.needs_review,
    t.last_response_at,
    t.ownership,
    t.origin,
    -- Subnet metadata (denormalized via JOIN)
    s.id AS subnet_id,
    s.network_address,
    s.network_size,
    s.service_id,
    s.subscriber_id,
    s.subscriber_name,
    s.location_id,
    s.location_address,
    s.city,
    s.region,
    s.pop_name,
    s.gateway_device,
    s.gateway_address
FROM targets t
LEFT JOIN subnets s ON t.subnet_id = s.id
WHERE t.archived_at IS NULL;
```

**Query examples:**
```sql
-- Find all targets for a subscriber
SELECT * FROM targets_enriched WHERE subscriber_id = 123;

-- Find all targets in a city
SELECT * FROM targets_enriched WHERE city = 'New York';

-- Find targets by custom tag
SELECT * FROM targets_enriched WHERE tags->>'team' = 'noc';

-- Search by display_name
SELECT * FROM targets_enriched WHERE display_name ILIKE '%cloudflare%';
```

### Ownership Model

| ownership | Behavior | Created by |
|-----------|----------|------------|
| `auto` | Follows subnet lifecycle, can be auto-archived | Sync or discovery |
| `manual` | User explicitly wants this, never auto-archived | User or "pinned" |

**Origin field (audit trail):**
- `sync` - Created during Pilot API sync
- `discovery` - Found during probe sweep
- `user` - Manually created by user

**Converting auto → manual:** User clicks "Pin" → ownership becomes `manual`, origin unchanged.

### Conflict Resolution

| Scenario | Action |
|----------|--------|
| Sync creates IP that exists as `manual` | Link to subnet, **keep ownership=manual** |
| Sync creates IP that exists as `auto` | Update metadata, keep as auto |
| User adds IP in existing subnet | Create with ownership=`manual`, link to subnet |
| User adds IP not in any subnet | Create with ownership=`manual`, subnet_id=NULL |
| Subnet archived with `auto` targets | Archive targets: set `archived_at`, keep `subnet_id` for history |
| Subnet archived with `manual` targets | Targets survive: set `subnet_id=NULL` (orphaned), NOT archived |

### Subnet Lifecycle & IP Churn

When subnets are removed from Pilot API (customer cancellation, reallocation):

1. **Archive old data** - Set `archived_at` on subnet and its `auto` targets
2. **Preserve history** - Keep `subnet_id` on archived targets for historical queries
3. **Handle reallocation** - If same IP range returns (e.g., /29 → two /30s):
   - Old subnet/targets remain archived (queryable)
   - New subnets created as fresh entries
   - New IPs start in UNKNOWN state

**Example:** Customer cancels a /29, it returns to pool, later issued as two /30s to new customers:
- Original /29 and its targets: `archived_at = NOW()`, queryable for 1 year
- New /30s: Created fresh with new UUIDs, targets start as UNKNOWN

### Unified Activity Log

Single source of truth for all events - queryable by IP, subnet, agent, or user:

```sql
CREATE TABLE activity_log (
    id BIGSERIAL PRIMARY KEY,

    -- What was affected (all nullable, at least one should be set)
    target_id UUID REFERENCES targets(id),
    subnet_id UUID REFERENCES subnets(id),
    agent_id TEXT,
    ip INET,  -- Denormalized for searching deleted targets

    -- Event classification
    category TEXT NOT NULL,
    -- 'target' | 'subnet' | 'agent' | 'sync' | 'user' | 'system'

    event_type TEXT NOT NULL,
    -- Target events:
    --   'state_change', 'discovered', 'created', 'archived', 'ownership_changed'
    -- Subnet events:
    --   'sync_created', 'sync_updated', 'sync_removed', 'archived'
    -- Agent events:
    --   'registered', 'heartbeat_missed', 'came_online', 'went_offline',
    --   'assignment_updated', 'probe_error'
    -- Sync events:
    --   'sync_started', 'sync_completed', 'sync_failed'
    -- User events:
    --   'manual_create', 'manual_update', 'manual_delete', 'acknowledged',
    --   'marked_inactive', 'pinned', 'triggered_discovery'
    -- System events:
    --   'retention_cleanup', 'config_changed'

    -- Event details (flexible JSON)
    details JSONB,
    -- Examples:
    -- {"from_state": "active", "to_state": "degraded", "reason": "no response 5min"}
    -- {"field": "subscriber_name", "old": "Acme", "new": "Acme Corp"}
    -- {"subnets_processed": 5521, "created": 12, "updated": 45, "archived": 3}
    -- {"error": "connection timeout", "retry_count": 3}

    -- Who/what triggered this
    triggered_by TEXT NOT NULL,
    -- 'system', 'sync', 'discovery', 'agent:agent-id', 'user:email@pilot.com'

    -- Severity for filtering
    severity TEXT DEFAULT 'info',  -- 'debug' | 'info' | 'warning' | 'error'

    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for common queries
CREATE INDEX idx_activity_target ON activity_log(target_id) WHERE target_id IS NOT NULL;
CREATE INDEX idx_activity_subnet ON activity_log(subnet_id) WHERE subnet_id IS NOT NULL;
CREATE INDEX idx_activity_agent ON activity_log(agent_id) WHERE agent_id IS NOT NULL;
CREATE INDEX idx_activity_ip ON activity_log(ip) WHERE ip IS NOT NULL;
CREATE INDEX idx_activity_time ON activity_log(created_at DESC);
CREATE INDEX idx_activity_category ON activity_log(category, event_type);
CREATE INDEX idx_activity_severity ON activity_log(severity) WHERE severity IN ('warning', 'error');

-- Retention: keep activity logs for 1 year
SELECT add_retention_policy('activity_log', INTERVAL '1 year');
```

**Query examples:**

```sql
-- All activity for an IP (even if target deleted)
SELECT * FROM activity_log WHERE ip = '1.2.3.4' ORDER BY created_at DESC;

-- All activity for a subnet and its targets
SELECT * FROM activity_log
WHERE subnet_id = ? OR target_id IN (SELECT id FROM targets WHERE subnet_id = ?)
ORDER BY created_at DESC;

-- Agent activity timeline
SELECT * FROM activity_log WHERE agent_id = 'agent-nyc-01' ORDER BY created_at DESC;

-- Recent sync events
SELECT * FROM activity_log WHERE category = 'sync' ORDER BY created_at DESC LIMIT 50;

-- All user actions
SELECT * FROM activity_log WHERE triggered_by LIKE 'user:%' ORDER BY created_at DESC;

-- Errors in last 24h
SELECT * FROM activity_log
WHERE severity = 'error' AND created_at > NOW() - INTERVAL '24 hours';
```

**UI Endpoints:**

```
GET /api/v1/activity?ip=1.2.3.4
GET /api/v1/activity?subnet=1.2.3.0/29
GET /api/v1/activity?subnet_id=uuid
GET /api/v1/activity?agent_id=agent-nyc-01
GET /api/v1/activity?category=sync&since=2025-01-25T00:00:00Z
GET /api/v1/activity?triggered_by=user:*
GET /api/v1/activity?severity=error&last=24h
```

### Virtual Tiers Based on State

```go
func getTierForTarget(t *Target) *Tier {
    // Skip archived targets entirely
    if t.ArchivedAt != nil {
        return nil
    }

    switch t.MonitoringState {
    case StateActive:
        return getConfiguredTier(t.Tier) // Use assigned tier
    case StateUnknown:
        return DiscoveryTier // 5 min interval, single agent
    case StateUnresponsive:
        return SmartRecheckTier(t) // Only if subnet needs coverage
    case StateInactive:
        return InactiveTier  // 1 hour interval, single agent
    case StateDegraded:
        return getConfiguredTier(t.Tier) // Maintain full monitoring
    case StateExcluded:
        return SmartRecheckTier(t) // Only if subnet needs coverage
    }
}

// SmartRecheckTier returns nil if subnet already has active targets
func SmartRecheckTier(t *Target) *Tier {
    if subnetHasActiveCoverge(t.SubnetID) {
        return nil // Skip - subnet already covered
    }
    return RecheckTier // Periodic re-check
}
```

## Data Retention & Archiving

### Archived Targets

When a subnet/IP is removed from the Pilot API sync:
1. **Don't delete** - set `archived_at` timestamp
2. **Record the event** in `target_state_history` and `activity_log`
3. **Keep time series data** for extended period
4. **Hide from active views** but allow historical queries

**Canonical archived check:** `WHERE archived_at IS NULL` for active targets

### Retention Policy

| Data Type | Retention | Notes |
|-----------|-----------|-------|
| Probe results (raw) | 1 year | All targets, active and archived |
| Aggregated metrics | 1 year | |
| State history | Forever | Audit trail |
| Target/Subnet metadata | Forever | Historical queries |
| Activity log | 1 year | See activity_log retention policy |

**Implementation:**
```sql
-- Simple uniform retention for probe_results
SELECT add_retention_policy('probe_results', INTERVAL '1 year');

-- Activity log retention (already defined in activity_log schema)
SELECT add_retention_policy('activity_log', INTERVAL '1 year');
```

**Note:** TimescaleDB retention policies apply uniformly to all data in a hypertable. To query archived data, filter by target's `archived_at` timestamp, not by a separate retention window.

### Historical Queries

Allow querying archived data for analysis:
```
GET /api/v1/targets?include_archived=true&archived_after=2024-01-01

GET /api/v1/metrics?target_id=uuid&include_archived=true
  → Returns data even for archived targets
```

---

## Integration Points

### Subnet API Integration

Pilot has an API to pull all subnets. Integration approach:

1. **Periodic sync job** polls subnet API
2. **Diff against current targets** - find new IPs, removed IPs
3. **Auto-create targets** for new IPs in UNKNOWN state
4. **Handle removed IPs** - mark as cancelled? delete? archive?

```go
type SubnetSyncResult struct {
    NewIPs      []string  // Create as UNKNOWN
    RemovedIPs  []string  // Handle based on policy
    UpdatedIPs  []string  // Metadata changed
}
```

### Questions for API Integration

- What metadata comes with subnets? (customer ID, IP type, POP?)
- How often does the data change?
- Is there a webhook for changes, or do we poll?
- How do we determine IP type from the API data?

## Open Questions

### Thresholds and Timing

1. **Discovery period**: How many probes before marking UNRESPONSIVE?
   - Proposal: 5 probes over 25 minutes
   - **Decision:** UNKNOWN → UNRESPONSIVE (no review needed)

2. **Degraded threshold**: How long without response before investigating?
   - Proposal: 5 minutes (10 missed probes at 30s interval)

3. **Excluded threshold**: How long in DEGRADED before excluding?
   - Proposal: 24 hours
   - **Decision:** DEGRADED → EXCLUDED (adds to review queue)

4. **Re-check frequency for INACTIVE**: How often to look for newly-responsive IPs?
   - Proposal: 1 hour

5. **Re-check frequency for UNRESPONSIVE/EXCLUDED**:
   - **Decision:** Smart re-check - only probe if subnet has no ACTIVE targets

### Operational Questions (Resolved)

1. **Manual override**: ✅ Yes - users can mark any IP as INACTIVE at any time

2. **Alerting scope**: ✅ Only alert on ACTIVE→DEGRADED, not discovery failures
   - UNKNOWN→UNRESPONSIVE generates no alerts or review items

3. **IP churn handling**: ✅ Archive old, create fresh
   - Old subnet/targets: `archived_at` set, queryable for 1 year
   - New assignment: Fresh entries starting as UNKNOWN

4. **Gateway IP handling**: ✅ ACTIVE ↔ DEGRADED only, never EXCLUDED
   - Always alert on degradation (these are Pilot-owned)

5. **Bulk operations**: TBD - Design API for subnet-level actions

## Next Steps

1. [x] Review this design document
2. [x] Answer open questions on thresholds and state machine
3. [ ] Investigate Pilot subnet API - what data is available?
4. [ ] Design the sync job for subnet API integration
5. [ ] Prototype database schema changes (subnets, targets, target_state_history)
6. [ ] Implement state machine logic in control plane
7. [ ] Implement smart re-check logic for UNRESPONSIVE/EXCLUDED
8. [ ] Update agent assignment logic for state-based tiers
9. [ ] Build UI for viewing IP states, review queue, and manual overrides

## Related Documents

- [ARCHITECTURE.md](./ARCHITECTURE.md) - Core system architecture
- [AGENT_ENROLLMENT_PLAN.md](./AGENT_ENROLLMENT_PLAN.md) - Agent deployment strategy
