-- ICMP-Mon Initial Schema
-- Designed for PostgreSQL with TimescaleDB extension
--
-- Run with: psql -d icmpmon -f 001_initial_schema.sql

-- Enable TimescaleDB extension
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- =============================================================================
-- ENUM TYPES
-- =============================================================================

CREATE TYPE agent_status AS ENUM ('active', 'degraded', 'offline');
CREATE TYPE alert_severity AS ENUM ('critical', 'warning', 'info');
CREATE TYPE alert_status AS ENUM ('active', 'acknowledged', 'resolved', 'suppressed');

-- =============================================================================
-- TIERS
-- =============================================================================

-- Tier configuration (controls monitoring intensity)
CREATE TABLE tiers (
    name VARCHAR(50) PRIMARY KEY,
    display_name VARCHAR(100) NOT NULL,

    -- Probe timing
    probe_interval_ms INTEGER NOT NULL DEFAULT 30000,
    probe_timeout_ms INTEGER NOT NULL DEFAULT 5000,
    probe_retries INTEGER NOT NULL DEFAULT 0,

    -- Agent selection policy (stored as JSON for flexibility)
    agent_selection JSONB NOT NULL DEFAULT '{
        "strategy": "distributed",
        "count": 4
    }'::jsonb,

    -- Alert thresholds
    alert_thresholds JSONB DEFAULT '{
        "failure_threshold": 3,
        "recovery_threshold": 2,
        "consensus_failure_percent": 50
    }'::jsonb,

    -- Default expected outcome
    default_expected_outcome JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Insert default tiers
INSERT INTO tiers (name, display_name, probe_interval_ms, probe_timeout_ms, probe_retries, agent_selection) VALUES
    ('infrastructure', 'Pilot Infrastructure', 5000, 2000, 1, '{"strategy": "all"}'),
    ('vip', 'VIP Customers', 15000, 3000, 2, '{"strategy": "distributed", "count": 18, "diversity": {"min_regions": 4, "min_providers": 3}}'),
    ('standard', 'Standard Customers', 30000, 5000, 0, '{"strategy": "distributed", "count": 4, "diversity": {"min_regions": 2}}');

-- =============================================================================
-- AGENTS
-- =============================================================================

CREATE TABLE agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,

    -- Location metadata
    region VARCHAR(50),
    location VARCHAR(255),
    provider VARCHAR(50),

    -- Tags for selection
    tags JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- Network info
    public_ip INET,

    -- Capabilities
    executors TEXT[] NOT NULL DEFAULT '{}',
    max_targets INTEGER NOT NULL DEFAULT 10000,
    version VARCHAR(50),

    -- Status
    status agent_status NOT NULL DEFAULT 'active',
    last_heartbeat TIMESTAMPTZ,

    -- Metadata
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_agents_region ON agents(region);
CREATE INDEX idx_agents_tags ON agents USING GIN(tags);

-- =============================================================================
-- TARGETS
-- =============================================================================

CREATE TABLE targets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ip_address INET NOT NULL,
    tier VARCHAR(50) NOT NULL REFERENCES tiers(name),
    customer_id UUID,

    -- Tags for filtering and correlation
    tags JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- Expected outcome (for security testing)
    expected_outcome JSONB,

    -- Metadata
    display_name VARCHAR(255),
    notes TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(ip_address)
);

CREATE INDEX idx_targets_tier ON targets(tier);
CREATE INDEX idx_targets_customer ON targets(customer_id);
CREATE INDEX idx_targets_tags ON targets USING GIN(tags);
CREATE INDEX idx_targets_ip ON targets(ip_address);

-- =============================================================================
-- PROBE RESULTS (Time-series data)
-- =============================================================================

CREATE TABLE probe_results (
    time TIMESTAMPTZ NOT NULL,
    target_id UUID NOT NULL,
    agent_id UUID NOT NULL,

    -- Outcome
    success BOOLEAN NOT NULL,
    error_message TEXT,

    -- Metrics
    latency_ms REAL,
    packet_loss_pct REAL,

    -- Full payload (JSON)
    payload JSONB,

    PRIMARY KEY (time, target_id, agent_id)
);

-- Convert to TimescaleDB hypertable for efficient time-series storage
SELECT create_hypertable('probe_results', 'time');

-- Compression policy (compress data older than 1 day)
ALTER TABLE probe_results SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'target_id, agent_id'
);
SELECT add_compression_policy('probe_results', INTERVAL '1 day');

-- Retention policy (drop data older than 90 days)
SELECT add_retention_policy('probe_results', INTERVAL '90 days');

-- Indexes for common queries
CREATE INDEX idx_probe_results_target ON probe_results(target_id, time DESC);
CREATE INDEX idx_probe_results_agent ON probe_results(agent_id, time DESC);

-- =============================================================================
-- CONTINUOUS AGGREGATES (Pre-computed rollups)
-- =============================================================================

-- 1-minute rollup per target
CREATE MATERIALIZED VIEW probe_results_1m
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 minute', time) AS bucket,
    target_id,
    COUNT(*) AS probe_count,
    SUM(CASE WHEN success THEN 1 ELSE 0 END) AS success_count,
    AVG(latency_ms) FILTER (WHERE success) AS avg_latency_ms,
    MIN(latency_ms) FILTER (WHERE success) AS min_latency_ms,
    MAX(latency_ms) FILTER (WHERE success) AS max_latency_ms,
    AVG(packet_loss_pct) AS avg_packet_loss_pct
FROM probe_results
GROUP BY bucket, target_id
WITH NO DATA;

-- Refresh policy for continuous aggregate
SELECT add_continuous_aggregate_policy('probe_results_1m',
    start_offset => INTERVAL '10 minutes',
    end_offset => INTERVAL '1 minute',
    schedule_interval => INTERVAL '1 minute'
);

-- =============================================================================
-- TARGET STATE (Current computed state)
-- =============================================================================

CREATE TABLE target_state (
    target_id UUID PRIMARY KEY REFERENCES targets(id) ON DELETE CASCADE,

    -- Current consensus state
    consensus_state VARCHAR(20) NOT NULL DEFAULT 'unknown',
    reachable_agents INTEGER NOT NULL DEFAULT 0,
    total_agents INTEGER NOT NULL DEFAULT 0,

    -- Latest metrics
    avg_latency_ms REAL,
    avg_packet_loss_pct REAL,

    -- State timing
    state_since TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_updated TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_target_state_consensus ON target_state(consensus_state);

-- =============================================================================
-- SNAPSHOTS
-- =============================================================================

CREATE TABLE snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,

    -- Scope
    scope JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- Summary stats
    total_targets INTEGER NOT NULL DEFAULT 0,
    reachable_targets INTEGER NOT NULL DEFAULT 0,
    unreachable_targets INTEGER NOT NULL DEFAULT 0,

    -- Metadata
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_snapshots_created ON snapshots(created_at DESC);

-- Individual target states within a snapshot
CREATE TABLE snapshot_targets (
    snapshot_id UUID NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    target_id UUID NOT NULL,
    ip_address INET NOT NULL,

    -- State at snapshot time
    consensus_state VARCHAR(20) NOT NULL,
    reachable_from INTEGER NOT NULL,
    total_agents INTEGER NOT NULL,
    avg_latency_ms REAL,

    -- Tags at snapshot time (for correlation analysis)
    tags JSONB,

    PRIMARY KEY (snapshot_id, target_id)
);

-- =============================================================================
-- ALERTS
-- =============================================================================

CREATE TABLE alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Classification
    severity alert_severity NOT NULL,
    alert_type VARCHAR(50) NOT NULL,
    status alert_status NOT NULL DEFAULT 'active',

    -- Target info
    target_id UUID REFERENCES targets(id) ON DELETE SET NULL,
    target_ip INET,
    target_tier VARCHAR(50),
    target_tags JSONB,

    -- Details
    title VARCHAR(255) NOT NULL,
    message TEXT,
    details JSONB,

    -- Which agents observed this
    agent_ids UUID[],

    -- Lifecycle
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by VARCHAR(255),
    resolved_at TIMESTAMPTZ,

    -- For correlation
    related_alerts UUID[]
);

CREATE INDEX idx_alerts_status ON alerts(status, created_at DESC);
CREATE INDEX idx_alerts_target ON alerts(target_id, created_at DESC);
CREATE INDEX idx_alerts_severity ON alerts(severity, status);

-- =============================================================================
-- ALERT RULES
-- =============================================================================

CREATE TABLE alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    priority INTEGER NOT NULL DEFAULT 100,

    -- Match conditions
    match_config JSONB NOT NULL,

    -- Handlers
    handlers JSONB NOT NULL,

    -- Deduplication
    deduplication JSONB,
    delay_seconds INTEGER DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_alert_rules_enabled ON alert_rules(enabled, priority);

-- =============================================================================
-- COMMANDS (On-demand requests to agents)
-- =============================================================================

CREATE TABLE commands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    command_type VARCHAR(50) NOT NULL,

    -- Target (if applicable)
    target_ip INET,

    -- Parameters
    params JSONB,

    -- Routing
    agent_ids UUID[],  -- Specific agents, or NULL for all

    -- Lifecycle
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    requested_by VARCHAR(255),
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);

CREATE INDEX idx_commands_status ON commands(status, requested_at);

-- Command results (one per agent)
CREATE TABLE command_results (
    command_id UUID NOT NULL REFERENCES commands(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,

    success BOOLEAN NOT NULL,
    error_message TEXT,
    payload JSONB,

    duration_ms INTEGER,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (command_id, agent_id)
);

-- =============================================================================
-- HELPER FUNCTIONS
-- =============================================================================

-- Function to get current consensus state for a target
CREATE OR REPLACE FUNCTION get_target_consensus(p_target_id UUID, p_window INTERVAL DEFAULT '2 minutes')
RETURNS TABLE(
    consensus_state VARCHAR,
    reachable_agents INTEGER,
    total_agents INTEGER,
    avg_latency_ms REAL
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        CASE
            WHEN SUM(CASE WHEN success THEN 1 ELSE 0 END) = 0 THEN 'unreachable'
            WHEN SUM(CASE WHEN success THEN 1 ELSE 0 END) = COUNT(DISTINCT agent_id) THEN 'reachable'
            ELSE 'degraded'
        END::VARCHAR AS consensus_state,
        SUM(CASE WHEN success THEN 1 ELSE 0 END)::INTEGER AS reachable_agents,
        COUNT(DISTINCT agent_id)::INTEGER AS total_agents,
        AVG(latency_ms) FILTER (WHERE success)::REAL AS avg_latency_ms
    FROM probe_results
    WHERE target_id = p_target_id
      AND time > NOW() - p_window;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- VERSION TRACKING
-- =============================================================================

-- Track assignment version for delta sync
CREATE TABLE assignment_version (
    id INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),  -- Singleton row
    version BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO assignment_version (id, version) VALUES (1, 1);

-- Function to increment version
CREATE OR REPLACE FUNCTION increment_assignment_version()
RETURNS BIGINT AS $$
DECLARE
    new_version BIGINT;
BEGIN
    UPDATE assignment_version
    SET version = version + 1, updated_at = NOW()
    WHERE id = 1
    RETURNING version INTO new_version;
    RETURN new_version;
END;
$$ LANGUAGE plpgsql;

-- Trigger to increment version on target changes
CREATE OR REPLACE FUNCTION targets_changed_trigger()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM increment_assignment_version();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER targets_changed
AFTER INSERT OR UPDATE OR DELETE ON targets
FOR EACH STATEMENT
EXECUTE FUNCTION targets_changed_trigger();
