# Incidents, Baselines, and Reporting System

## Overview

This document captures the design for:
1. **Baseline-relative anomaly detection** - detecting problems relative to what's "normal" for each agent→target pair
2. **Blast radius correlation** - determining if an issue is isolated or widespread
3. **Incident lifecycle management** - tracking problems from detection to resolution
4. **Reporting system** - generating 7/30/90-day and annual reports for customers

---

## Part 1: Baseline-Relative Detection

### Problem

Absolute thresholds don't work across diverse agent-target pairs:
- NYC-agent → NYC-target: 5ms is normal, 15ms is a problem
- NYC-agent → Ashburn-target: 10ms is normal
- London-agent → NYC-target: 75ms is normal, 150ms is a problem

### Solution: Per Agent-Target Baselines

Track what "normal" looks like for each specific (agent_id, target_id) pair.

#### Schema

```sql
CREATE TABLE agent_target_baseline (
    agent_id UUID NOT NULL,
    target_id UUID NOT NULL,

    -- Latency baselines
    latency_p50 DOUBLE PRECISION,      -- typical latency
    latency_p95 DOUBLE PRECISION,      -- expected worst case
    latency_p99 DOUBLE PRECISION,
    latency_stddev DOUBLE PRECISION,   -- for z-score calculation

    -- Packet loss baseline (usually ~0%)
    packet_loss_baseline DOUBLE PRECISION DEFAULT 0,

    -- Metadata
    sample_count INTEGER,
    first_seen TIMESTAMPTZ,
    last_updated TIMESTAMPTZ DEFAULT NOW(),

    PRIMARY KEY (agent_id, target_id),
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE,
    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE
);

CREATE INDEX idx_baseline_updated ON agent_target_baseline(last_updated);
```

#### Baseline Calculation

Background job runs daily (or more frequently initially):

```sql
INSERT INTO agent_target_baseline (agent_id, target_id, latency_p50, latency_p95, latency_p99, latency_stddev, packet_loss_baseline, sample_count, last_updated)
SELECT
    agent_id,
    target_id,
    percentile_cont(0.5) WITHIN GROUP (ORDER BY latency_ms) as latency_p50,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) as latency_p95,
    percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_ms) as latency_p99,
    stddev(latency_ms) as latency_stddev,
    avg(packet_loss_pct) as packet_loss_baseline,
    count(*) as sample_count,
    NOW()
FROM probe_results
WHERE time > NOW() - INTERVAL '7 days'
  AND success = true  -- only successful probes for latency baseline
GROUP BY agent_id, target_id
ON CONFLICT (agent_id, target_id) DO UPDATE SET
    latency_p50 = EXCLUDED.latency_p50,
    latency_p95 = EXCLUDED.latency_p95,
    latency_p99 = EXCLUDED.latency_p99,
    latency_stddev = EXCLUDED.latency_stddev,
    packet_loss_baseline = EXCLUDED.packet_loss_baseline,
    sample_count = EXCLUDED.sample_count,
    last_updated = NOW();
```

#### Anomaly Detection

For each probe result, calculate deviation:

```go
type AnomalyCheck struct {
    AgentID    string
    TargetID   string
    LatencyMs  float64
    PacketLoss float64
}

func (c *AnomalyCheck) IsAnomalous(baseline *AgentTargetBaseline) (bool, float64) {
    if baseline == nil || baseline.SampleCount < 100 {
        // Not enough data for baseline, use absolute thresholds
        return c.PacketLoss > 5 || c.LatencyMs > 500, 0
    }

    // Z-score: how many standard deviations from normal?
    zScore := (c.LatencyMs - baseline.LatencyP50) / baseline.LatencyStddev

    // Anomaly if:
    // - Z-score > 3 (3 sigma event)
    // - OR latency > p99 baseline
    // - OR packet loss > baseline + 1%
    isAnomalous := zScore > 3 ||
                   c.LatencyMs > baseline.LatencyP99 ||
                   c.PacketLoss > baseline.PacketLossBaseline + 1

    return isAnomalous, zScore
}
```

---

## Part 2: Blast Radius Correlation

### Problem

Not all anomalies are equal:
- One target failing from one agent = maybe transient
- One target failing from ALL agents = target is down
- ALL targets failing from one agent = that agent has upstream issues
- Many targets failing from many agents = major event

### Solution: Correlation Engine

#### Per Agent-Target State Tracking

```sql
CREATE TABLE agent_target_state (
    agent_id UUID NOT NULL,
    target_id UUID NOT NULL,

    status VARCHAR(20) DEFAULT 'unknown',  -- healthy, degraded, down, unknown
    status_since TIMESTAMPTZ,

    -- Current deviation from baseline
    current_z_score DOUBLE PRECISION,
    current_packet_loss DOUBLE PRECISION,

    -- Anomaly tracking
    anomaly_start TIMESTAMPTZ,  -- NULL if currently healthy
    consecutive_anomalies INTEGER DEFAULT 0,

    last_probe_time TIMESTAMPTZ,
    last_evaluated TIMESTAMPTZ DEFAULT NOW(),

    PRIMARY KEY (agent_id, target_id)
);
```

#### Correlation Logic

Run every 30 seconds:

```go
type CorrelationResult struct {
    Type            string   // "target", "agent", "regional", "global"
    Severity        string   // "low", "medium", "high", "critical"
    AffectedTargets []string
    AffectedAgents  []string
    WaitBeforeAlert time.Duration
}

func CorrelateAnomalies(anomalies []AgentTargetAnomaly) []CorrelationResult {
    // Count anomalies per target (how many agents see it?)
    targetAnomalyCount := make(map[string]int)
    // Count anomalies per agent (how many targets affected?)
    agentAnomalyCount := make(map[string]int)
    // Track which agents see which targets
    targetAgents := make(map[string][]string)
    agentTargets := make(map[string][]string)

    for _, a := range anomalies {
        targetAnomalyCount[a.TargetID]++
        agentAnomalyCount[a.AgentID]++
        targetAgents[a.TargetID] = append(targetAgents[a.TargetID], a.AgentID)
        agentTargets[a.AgentID] = append(agentTargets[a.AgentID], a.TargetID)
    }

    var results []CorrelationResult

    // Check for agent-wide issues (one agent, many targets)
    for agentID, targets := range agentTargets {
        if len(targets) > 5 { // Threshold: agent sees 5+ targets anomalous
            results = append(results, CorrelationResult{
                Type:            "agent",
                Severity:        "medium",
                AffectedAgents:  []string{agentID},
                AffectedTargets: targets,
                WaitBeforeAlert: 1 * time.Minute,
            })
        }
    }

    // Check for target-specific issues
    for targetID, agents := range targetAgents {
        agentCount := len(agents)
        totalAgentsForTarget := getTotalAgentsMonitoring(targetID)

        if agentCount == 1 && totalAgentsForTarget > 1 {
            // Only one agent sees it - could be transient
            results = append(results, CorrelationResult{
                Type:            "target",
                Severity:        "low",
                AffectedTargets: []string{targetID},
                AffectedAgents:  agents,
                WaitBeforeAlert: 5 * time.Minute,
            })
        } else if agentCount >= 2 {
            // Multiple agents see it - more likely real
            results = append(results, CorrelationResult{
                Type:            "target",
                Severity:        "medium",
                AffectedTargets: []string{targetID},
                AffectedAgents:  agents,
                WaitBeforeAlert: 2 * time.Minute,
            })
        }
    }

    // Check for global issues (many agents, many targets)
    if len(agentAnomalyCount) > 3 && len(targetAnomalyCount) > 10 {
        results = append(results, CorrelationResult{
            Type:            "global",
            Severity:        "critical",
            AffectedTargets: keys(targetAnomalyCount),
            AffectedAgents:  keys(agentAnomalyCount),
            WaitBeforeAlert: 0, // Immediate
        })
    }

    return results
}
```

#### Alert Timing Matrix

| Pattern | Agents Affected | Wait Time | Severity |
|---------|-----------------|-----------|----------|
| Single target | 1 | 5 min | low |
| Single target | 2+ | 2 min | medium |
| Multiple targets | 1 (agent issue) | 1 min | medium |
| Multiple targets | 2+ same region | 30 sec | high |
| Many targets | Many agents | immediate | critical |

---

## Part 3: Incident Lifecycle

### Incident Schema

```sql
CREATE TABLE incidents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Classification
    incident_type VARCHAR(20) NOT NULL,  -- target, agent, regional, global
    severity VARCHAR(20) NOT NULL,        -- low, medium, high, critical

    -- What's affected
    primary_entity_type VARCHAR(20),      -- target, agent, region
    primary_entity_id VARCHAR(255),       -- the main affected entity
    affected_target_ids UUID[],
    affected_agent_ids UUID[],

    -- Timeline
    detected_at TIMESTAMPTZ NOT NULL,     -- when anomaly first seen
    confirmed_at TIMESTAMPTZ,             -- when wait period elapsed, incident confirmed
    resolved_at TIMESTAMPTZ,              -- when recovery detected

    -- Observations during incident
    peak_z_score DOUBLE PRECISION,
    peak_packet_loss DOUBLE PRECISION,
    peak_latency_ms DOUBLE PRECISION,

    -- Baseline at time of incident (for context)
    baseline_snapshot JSONB,

    -- Human interaction
    acknowledged_by VARCHAR(255),
    acknowledged_at TIMESTAMPTZ,
    notes TEXT,

    -- Status
    status VARCHAR(20) DEFAULT 'active',  -- active, resolved, acknowledged

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_incidents_status ON incidents(status, detected_at);
CREATE INDEX idx_incidents_target ON incidents USING GIN(affected_target_ids);
CREATE INDEX idx_incidents_agent ON incidents USING GIN(affected_agent_ids);
CREATE INDEX idx_incidents_time ON incidents(detected_at DESC);
```

### Incident State Machine

```
                    ┌─────────────────┐
                    │    detected     │
                    │  (wait period)  │
                    └────────┬────────┘
                             │
              ┌──────────────┴──────────────┐
              │                             │
              ▼                             ▼
     ┌────────────────┐           ┌─────────────────┐
     │   confirmed    │           │    resolved     │
     │   (active)     │           │  (auto-cleared) │
     └───────┬────────┘           └─────────────────┘
             │                              ▲
             │                              │
             ▼                              │
     ┌────────────────┐                     │
     │  acknowledged  │─────────────────────┘
     └────────────────┘        (recovery)
```

### Incident Lifecycle Logic

```go
type IncidentManager struct {
    store *Store
    // Track pending incidents (in wait period)
    pending map[string]*PendingIncident
}

type PendingIncident struct {
    Correlation   CorrelationResult
    DetectedAt    time.Time
    AlertAfter    time.Time  // DetectedAt + WaitBeforeAlert
}

func (m *IncidentManager) ProcessCorrelations(correlations []CorrelationResult) {
    now := time.Now()

    for _, c := range correlations {
        key := correlationKey(c)

        if pending, exists := m.pending[key]; exists {
            // Already tracking this
            if now.After(pending.AlertAfter) {
                // Wait period elapsed - create confirmed incident
                m.createIncident(pending)
                delete(m.pending, key)
            }
            // Otherwise keep waiting
        } else {
            // New potential incident
            m.pending[key] = &PendingIncident{
                Correlation: c,
                DetectedAt:  now,
                AlertAfter:  now.Add(c.WaitBeforeAlert),
            }
        }
    }

    // Check for resolutions
    for key, pending := range m.pending {
        if !isStillAnomalous(pending.Correlation) {
            // Cleared before confirmation - delete without creating incident
            delete(m.pending, key)
        }
    }

    // Check active incidents for resolution
    m.checkResolutions()
}

func (m *IncidentManager) checkResolutions() {
    activeIncidents := m.store.GetActiveIncidents()

    for _, incident := range activeIncidents {
        if m.isResolved(incident) {
            m.store.ResolveIncident(incident.ID, time.Now())
        } else {
            // Update peak metrics
            m.updateIncidentMetrics(incident)
        }
    }
}
```

---

## Part 4: Continuous Aggregates for Reporting

### Hourly Aggregates

```sql
CREATE MATERIALIZED VIEW probe_hourly
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', time) AS bucket,
    agent_id,
    target_id,

    -- Latency metrics
    avg(latency_ms) as avg_latency,
    min(latency_ms) as min_latency,
    max(latency_ms) as max_latency,
    percentile_cont(0.5) WITHIN GROUP (ORDER BY latency_ms) as p50_latency,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) as p95_latency,
    percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_ms) as p99_latency,
    stddev(latency_ms) as latency_stddev,  -- jitter proxy

    -- Packet loss
    avg(packet_loss_pct) as avg_packet_loss,

    -- Counts
    count(*) as probe_count,
    sum(case when success then 1 else 0 end) as success_count,
    sum(case when not success then 1 else 0 end) as failure_count

FROM probe_results
GROUP BY bucket, agent_id, target_id
WITH NO DATA;

-- Refresh policy: keep last 90 days of hourly data
SELECT add_continuous_aggregate_policy('probe_hourly',
    start_offset => INTERVAL '90 days',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour');
```

### Daily Aggregates

```sql
CREATE MATERIALIZED VIEW probe_daily
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 day', bucket) AS bucket,
    agent_id,
    target_id,

    -- Aggregate from hourly
    avg(avg_latency) as avg_latency,
    min(min_latency) as min_latency,
    max(max_latency) as max_latency,
    avg(p50_latency) as p50_latency,
    avg(p95_latency) as p95_latency,
    avg(p99_latency) as p99_latency,
    avg(latency_stddev) as avg_jitter,

    avg(avg_packet_loss) as avg_packet_loss,

    sum(probe_count) as probe_count,
    sum(success_count) as success_count,
    sum(failure_count) as failure_count

FROM probe_hourly
GROUP BY time_bucket('1 day', bucket), agent_id, target_id
WITH NO DATA;

-- Refresh policy: keep 2 years of daily data
SELECT add_continuous_aggregate_policy('probe_daily',
    start_offset => INTERVAL '730 days',
    end_offset => INTERVAL '1 day',
    schedule_interval => INTERVAL '1 day');
```

### Monthly Aggregates

```sql
CREATE MATERIALIZED VIEW probe_monthly
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 month', bucket) AS bucket,
    agent_id,
    target_id,

    avg(avg_latency) as avg_latency,
    min(min_latency) as min_latency,
    max(max_latency) as max_latency,
    avg(p50_latency) as p50_latency,
    avg(p95_latency) as p95_latency,
    avg(p99_latency) as p99_latency,
    avg(avg_jitter) as avg_jitter,

    avg(avg_packet_loss) as avg_packet_loss,

    sum(probe_count) as probe_count,
    sum(success_count) as success_count,
    sum(failure_count) as failure_count

FROM probe_daily
GROUP BY time_bucket('1 month', bucket), agent_id, target_id
WITH NO DATA;

-- Refresh policy: keep forever
SELECT add_continuous_aggregate_policy('probe_monthly',
    start_offset => NULL,  -- no limit
    end_offset => INTERVAL '1 month',
    schedule_interval => INTERVAL '1 day');
```

### Report Query Examples

#### 90-Day Report for a Target

```sql
SELECT
    t.ip,
    a.name as agent_name,
    a.region as agent_region,

    avg(pd.avg_latency) as avg_latency_ms,
    avg(pd.p95_latency) as p95_latency_ms,
    avg(pd.p99_latency) as p99_latency_ms,
    avg(pd.avg_jitter) as jitter_ms,
    avg(pd.avg_packet_loss) as packet_loss_pct,

    sum(pd.success_count)::float / sum(pd.probe_count) * 100 as uptime_pct,
    sum(pd.probe_count) as total_probes

FROM probe_daily pd
JOIN targets t ON pd.target_id = t.id
JOIN agents a ON pd.agent_id = a.id
WHERE pd.target_id = $1
  AND pd.bucket >= NOW() - INTERVAL '90 days'
GROUP BY t.ip, a.name, a.region
ORDER BY a.region, a.name;
```

#### Customer Annual Report

```sql
SELECT
    t.id as target_id,
    host(t.ip_address) as ip,
    t.tags->>'name' as target_name,

    avg(pm.avg_latency) as avg_latency_ms,
    avg(pm.p95_latency) as p95_latency_ms,
    avg(pm.avg_packet_loss) as packet_loss_pct,
    sum(pm.success_count)::float / sum(pm.probe_count) * 100 as uptime_pct,

    -- Incident summary
    (SELECT count(*) FROM incidents i
     WHERE t.id = ANY(i.affected_target_ids)
       AND i.detected_at >= NOW() - INTERVAL '1 year') as incident_count,

    (SELECT sum(EXTRACT(EPOCH FROM (resolved_at - detected_at))/60)
     FROM incidents i
     WHERE t.id = ANY(i.affected_target_ids)
       AND i.detected_at >= NOW() - INTERVAL '1 year'
       AND i.resolved_at IS NOT NULL) as total_downtime_minutes

FROM probe_monthly pm
JOIN targets t ON pm.target_id = t.id
WHERE t.customer_id = $1
  AND pm.bucket >= NOW() - INTERVAL '1 year'
GROUP BY t.id, t.ip_address, t.tags
ORDER BY t.ip_address;
```

#### SLA Compliance Check

```sql
-- Check: 99.9% of probes under 50ms from US agents
SELECT
    sum(case when avg_latency < 50 then probe_count else 0 end)::float
    / sum(probe_count) * 100 as sla_compliance_pct
FROM probe_hourly ph
JOIN agents a ON ph.agent_id = a.id
WHERE ph.target_id = $1
  AND a.region LIKE 'us-%'
  AND ph.bucket >= NOW() - INTERVAL '30 days';
```

---

## Part 5: Implementation Order

### Phase 1: Continuous Aggregates (Foundation)
1. Create `probe_hourly`, `probe_daily`, `probe_monthly` materialized views
2. Set up refresh policies
3. Backfill from existing `probe_results`

### Phase 2: Baseline System
1. Create `agent_target_baseline` table
2. Implement baseline calculation job (runs daily)
3. Modify status calculation to use baseline-relative thresholds

### Phase 3: State Tracking
1. Create `agent_target_state` table
2. Implement state evaluation loop (runs every 30s)
3. Track anomalies per agent-target pair

### Phase 4: Correlation Engine
1. Implement blast radius detection
2. Count anomalies per agent / per target
3. Classify incident type (target, agent, regional, global)

### Phase 5: Incident Management
1. Create `incidents` table
2. Implement pending incident tracking (wait periods)
3. Incident creation, update, resolution logic
4. API endpoints for incident list, acknowledge, notes

### Phase 6: Reporting API
1. Implement report generation endpoints
2. Per-target reports (7/30/90/365 day)
3. Per-customer reports
4. SLA compliance calculations
5. Export formats (JSON, CSV, PDF)

---

## Part 6: Background Jobs Summary

| Job | Frequency | Purpose |
|-----|-----------|---------|
| Baseline Calculator | Daily | Update `agent_target_baseline` from last 7 days |
| State Evaluator | 30 seconds | Update `agent_target_state`, detect anomalies |
| Correlation Engine | 30 seconds | Analyze anomalies, determine blast radius |
| Incident Manager | 30 seconds | Create/update/resolve incidents |
| Continuous Aggregate Refresh | Hourly/Daily | Keep rollup tables current (handled by TimescaleDB) |

---

## Part 7: API Endpoints (New)

### Incidents
- `GET /api/v1/incidents` - List incidents (filter by status, time, target, agent)
- `GET /api/v1/incidents/{id}` - Get incident details
- `POST /api/v1/incidents/{id}/acknowledge` - Acknowledge incident
- `PUT /api/v1/incidents/{id}/notes` - Add notes to incident

### Reports
- `GET /api/v1/reports/targets/{id}?window=90d` - Target performance report
- `GET /api/v1/reports/customers/{id}?window=annual` - Customer report
- `GET /api/v1/reports/agents/{id}?window=30d` - Agent health report
- `GET /api/v1/reports/sla?customer={id}&window=30d` - SLA compliance

### Baselines
- `GET /api/v1/baselines/{agent_id}/{target_id}` - Get baseline for pair
- `GET /api/v1/baselines/target/{id}` - Get all baselines for a target
- `POST /api/v1/baselines/recalculate` - Trigger baseline recalculation

---

## Part 8: Expected Outcome Handling

For targets with `expected_outcome.should_succeed = false`:
- These are "negative tests" (e.g., IPs that should be blocked)
- Normal: probe fails (unreachable)
- Anomaly: probe succeeds (security concern!)
- Invert all detection logic for these targets
- Incidents should be "unexpected reachability" type

```go
func isAnomalous(target Target, probeResult ProbeResult) bool {
    if target.ExpectedOutcome != nil && !target.ExpectedOutcome.ShouldSucceed {
        // Inverted logic: success is bad, failure is good
        return probeResult.Success
    }
    // Normal logic
    return !probeResult.Success || exceedsBaseline(probeResult)
}
```

---

## Implementation Status

### Completed

| Phase | Component | Status |
|-------|-----------|--------|
| **Phase 1** | Continuous Aggregates | ✅ DB schema complete (`probe_hourly`, `probe_daily`, `probe_monthly`) |
| **Phase 2** | Baseline System | ✅ DB schema complete (`agent_target_baseline` table, `calculate_baseline()` function) |
| **Phase 3** | State Tracking | ✅ DB schema complete (`agent_target_state` table) |
| **Phase 4-5** | Incident Management | ✅ DB schema, API endpoints, and UI complete |
| **Phase 6** | Reporting API | ✅ `get_target_report()` function and `/reports/targets/{id}` endpoint |

### Backend API Endpoints (Implemented)

```
GET  /api/v1/incidents                    - List incidents with status filter
GET  /api/v1/incidents/{id}               - Get incident details
POST /api/v1/incidents/{id}/acknowledge   - Acknowledge incident
POST /api/v1/incidents/{id}/resolve       - Resolve incident
PUT  /api/v1/incidents/{id}/notes         - Add note to incident

GET  /api/v1/baselines/{agent}/{target}   - Get baseline for agent-target pair
GET  /api/v1/targets/{id}/baselines       - Get all baselines for a target
POST /api/v1/baselines/recalculate        - Trigger baseline recalculation

GET  /api/v1/reports/targets/{id}?window=90d  - Get target performance report
```

### UI (Implemented)

- **Incidents Page** (`/incidents`)
  - List view with severity/status/type filters
  - Detail panel with acknowledge/resolve actions
  - Notes/timeline support
  - Real-time refresh (15s polling)

### Remaining Work

| Component | Description | Notes |
|-----------|-------------|-------|
| **Baseline Calculator Job** | Background goroutine that runs `recalculate_all_baselines()` daily | Should run on control-plane startup |
| **State Evaluator Job** | 30-second loop that evaluates probe results against baselines, updates `agent_target_state` | Core anomaly detection |
| **Correlation Engine** | Analyzes `agent_target_state` anomalies to determine blast radius | Uses `get_agent_anomaly_counts()` and `get_target_anomaly_counts()` |
| **Incident Manager Job** | Creates/updates/resolves incidents based on correlation results | Manages pending→active→resolved lifecycle |
| **Reporting UI** | Display target reports in UI, SLA compliance dashboard | API exists, needs frontend |
| **Baseline Visualization** | Show baseline metrics in target detail view | API exists, needs frontend |
| **Report Export** | Generate PDF/CSV reports | Low priority |

### Background Job Architecture (To Implement)

```go
// In control-plane/internal/jobs/

type JobManager struct {
    store   *store.Store
    service *service.Service
    logger  *slog.Logger
}

func (m *JobManager) Start(ctx context.Context) {
    // Daily baseline recalculation
    go m.runBaselineCalculator(ctx)

    // 30-second evaluation loop
    go m.runStateEvaluator(ctx)

    // 30-second correlation + incident management
    go m.runIncidentManager(ctx)
}
```
