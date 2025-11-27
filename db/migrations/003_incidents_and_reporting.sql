-- Incidents, Baselines, and Reporting System
-- Implements baseline-relative detection, blast radius correlation, and reporting rollups
--
-- Run with: psql -d icmpmon -f 003_incidents_and_reporting.sql

-- =============================================================================
-- PHASE 1: CONTINUOUS AGGREGATES FOR REPORTING
-- Pre-computed rollups per (agent_id, target_id) pair
-- =============================================================================

-- Hourly aggregates per agent-target pair
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

-- Indexes for common queries
CREATE INDEX idx_probe_hourly_target ON probe_hourly(target_id, bucket DESC);
CREATE INDEX idx_probe_hourly_agent ON probe_hourly(agent_id, bucket DESC);
CREATE INDEX idx_probe_hourly_pair ON probe_hourly(agent_id, target_id, bucket DESC);

-- Daily aggregates (aggregates from hourly)
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

CREATE INDEX idx_probe_daily_target ON probe_daily(target_id, bucket DESC);
CREATE INDEX idx_probe_daily_agent ON probe_daily(agent_id, bucket DESC);
CREATE INDEX idx_probe_daily_pair ON probe_daily(agent_id, target_id, bucket DESC);

-- Monthly aggregates (aggregates from daily)
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

CREATE INDEX idx_probe_monthly_target ON probe_monthly(target_id, bucket DESC);
CREATE INDEX idx_probe_monthly_agent ON probe_monthly(agent_id, bucket DESC);
CREATE INDEX idx_probe_monthly_pair ON probe_monthly(agent_id, target_id, bucket DESC);

-- =============================================================================
-- PHASE 2: BASELINE SYSTEM
-- Per agent-target pair baselines for anomaly detection
-- =============================================================================

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
CREATE INDEX idx_baseline_target ON agent_target_baseline(target_id);
CREATE INDEX idx_baseline_agent ON agent_target_baseline(agent_id);

-- =============================================================================
-- PHASE 3: STATE TRACKING
-- Per agent-target pair current state for anomaly tracking
-- =============================================================================

CREATE TABLE agent_target_state (
    agent_id UUID NOT NULL,
    target_id UUID NOT NULL,

    status VARCHAR(20) DEFAULT 'unknown',  -- healthy, degraded, down, unknown
    status_since TIMESTAMPTZ,

    -- Current deviation from baseline
    current_z_score DOUBLE PRECISION,
    current_packet_loss DOUBLE PRECISION,
    current_latency_ms DOUBLE PRECISION,

    -- Anomaly tracking
    anomaly_start TIMESTAMPTZ,  -- NULL if currently healthy
    consecutive_anomalies INTEGER DEFAULT 0,
    consecutive_successes INTEGER DEFAULT 0,  -- For auto-resolution tracking

    last_probe_time TIMESTAMPTZ,
    last_evaluated TIMESTAMPTZ DEFAULT NOW(),

    PRIMARY KEY (agent_id, target_id),
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE,
    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE
);

CREATE INDEX idx_state_status ON agent_target_state(status);
CREATE INDEX idx_state_anomaly ON agent_target_state(anomaly_start) WHERE anomaly_start IS NOT NULL;
CREATE INDEX idx_state_target ON agent_target_state(target_id);
CREATE INDEX idx_state_agent ON agent_target_state(agent_id);

-- =============================================================================
-- PHASE 4-5: INCIDENT MANAGEMENT
-- Track incidents from detection through resolution
-- =============================================================================

CREATE TYPE incident_type AS ENUM ('target', 'agent', 'regional', 'global');
CREATE TYPE incident_severity AS ENUM ('low', 'medium', 'high', 'critical');
CREATE TYPE incident_status AS ENUM ('pending', 'active', 'acknowledged', 'resolved');

CREATE TABLE incidents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Classification
    incident_type incident_type NOT NULL,
    severity incident_severity NOT NULL,

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
    status incident_status NOT NULL DEFAULT 'pending',

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_incidents_status ON incidents(status, detected_at);
CREATE INDEX idx_incidents_target ON incidents USING GIN(affected_target_ids);
CREATE INDEX idx_incidents_agent ON incidents USING GIN(affected_agent_ids);
CREATE INDEX idx_incidents_time ON incidents(detected_at DESC);
CREATE INDEX idx_incidents_type ON incidents(incident_type, status);
CREATE INDEX idx_incidents_severity ON incidents(severity, status);

-- Incident timeline events for detailed tracking
CREATE TABLE incident_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,

    event_type VARCHAR(50) NOT NULL,  -- 'detected', 'confirmed', 'escalated', 'acknowledged', 'note_added', 'resolved'
    description TEXT,
    details JSONB,

    created_by VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_incident_events_incident ON incident_events(incident_id, created_at);

-- =============================================================================
-- HELPER FUNCTIONS
-- =============================================================================

-- Calculate baseline for an agent-target pair
CREATE OR REPLACE FUNCTION calculate_baseline(p_agent_id UUID, p_target_id UUID)
RETURNS void AS $$
BEGIN
    INSERT INTO agent_target_baseline (agent_id, target_id, latency_p50, latency_p95, latency_p99, latency_stddev, packet_loss_baseline, sample_count, first_seen, last_updated)
    SELECT
        agent_id,
        target_id,
        percentile_cont(0.5) WITHIN GROUP (ORDER BY latency_ms) as latency_p50,
        percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) as latency_p95,
        percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_ms) as latency_p99,
        stddev(latency_ms) as latency_stddev,
        avg(packet_loss_pct) as packet_loss_baseline,
        count(*) as sample_count,
        min(time) as first_seen,
        NOW()
    FROM probe_results
    WHERE agent_id = p_agent_id
      AND target_id = p_target_id
      AND time > NOW() - INTERVAL '7 days'
      AND success = true
    GROUP BY agent_id, target_id
    ON CONFLICT (agent_id, target_id) DO UPDATE SET
        latency_p50 = EXCLUDED.latency_p50,
        latency_p95 = EXCLUDED.latency_p95,
        latency_p99 = EXCLUDED.latency_p99,
        latency_stddev = EXCLUDED.latency_stddev,
        packet_loss_baseline = EXCLUDED.packet_loss_baseline,
        sample_count = EXCLUDED.sample_count,
        last_updated = NOW();
END;
$$ LANGUAGE plpgsql;

-- Recalculate all baselines (run daily)
CREATE OR REPLACE FUNCTION recalculate_all_baselines()
RETURNS INTEGER AS $$
DECLARE
    pairs_updated INTEGER := 0;
    pair RECORD;
BEGIN
    FOR pair IN
        SELECT DISTINCT agent_id, target_id
        FROM probe_results
        WHERE time > NOW() - INTERVAL '7 days'
    LOOP
        PERFORM calculate_baseline(pair.agent_id, pair.target_id);
        pairs_updated := pairs_updated + 1;
    END LOOP;

    RETURN pairs_updated;
END;
$$ LANGUAGE plpgsql;

-- Get report data for a target over a time window
CREATE OR REPLACE FUNCTION get_target_report(
    p_target_id UUID,
    p_window_days INTEGER DEFAULT 90
)
RETURNS TABLE(
    agent_name VARCHAR,
    agent_region VARCHAR,
    avg_latency_ms DOUBLE PRECISION,
    p95_latency_ms DOUBLE PRECISION,
    p99_latency_ms DOUBLE PRECISION,
    jitter_ms DOUBLE PRECISION,
    packet_loss_pct DOUBLE PRECISION,
    uptime_pct DOUBLE PRECISION,
    total_probes BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        a.name as agent_name,
        a.region as agent_region,
        avg(pd.avg_latency)::DOUBLE PRECISION as avg_latency_ms,
        avg(pd.p95_latency)::DOUBLE PRECISION as p95_latency_ms,
        avg(pd.p99_latency)::DOUBLE PRECISION as p99_latency_ms,
        avg(pd.avg_jitter)::DOUBLE PRECISION as jitter_ms,
        avg(pd.avg_packet_loss)::DOUBLE PRECISION as packet_loss_pct,
        (sum(pd.success_count)::float / NULLIF(sum(pd.probe_count), 0) * 100)::DOUBLE PRECISION as uptime_pct,
        sum(pd.probe_count)::BIGINT as total_probes
    FROM probe_daily pd
    JOIN agents a ON pd.agent_id = a.id
    WHERE pd.target_id = p_target_id
      AND pd.bucket >= NOW() - make_interval(days => p_window_days)
    GROUP BY a.name, a.region
    ORDER BY a.region, a.name;
END;
$$ LANGUAGE plpgsql;

-- Get anomaly count by agent (for blast radius detection)
CREATE OR REPLACE FUNCTION get_agent_anomaly_counts()
RETURNS TABLE(
    agent_id UUID,
    anomaly_count BIGINT,
    affected_targets UUID[]
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        s.agent_id,
        count(*) as anomaly_count,
        array_agg(s.target_id) as affected_targets
    FROM agent_target_state s
    WHERE s.anomaly_start IS NOT NULL
    GROUP BY s.agent_id;
END;
$$ LANGUAGE plpgsql;

-- Get anomaly count by target (for blast radius detection)
CREATE OR REPLACE FUNCTION get_target_anomaly_counts()
RETURNS TABLE(
    target_id UUID,
    anomaly_count BIGINT,
    affected_agents UUID[]
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        s.target_id,
        count(*) as anomaly_count,
        array_agg(s.agent_id) as affected_agents
    FROM agent_target_state s
    WHERE s.anomaly_start IS NOT NULL
    GROUP BY s.target_id;
END;
$$ LANGUAGE plpgsql;
