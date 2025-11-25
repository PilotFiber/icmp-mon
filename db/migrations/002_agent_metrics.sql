-- Agent Health and Metrics Schema
-- Tracks detailed performance data for monitoring agent health

-- =============================================================================
-- AGENT METRICS (Time-series health data)
-- =============================================================================

CREATE TABLE agent_metrics (
    time TIMESTAMPTZ NOT NULL,
    agent_id UUID NOT NULL,

    -- Health status
    status agent_status NOT NULL,

    -- Resource utilization
    cpu_percent REAL,
    memory_mb REAL,
    memory_percent REAL,
    disk_used_gb REAL,
    disk_free_gb REAL,
    goroutine_count INTEGER,

    -- Network stats
    public_ip INET,
    network_rx_bytes BIGINT,
    network_tx_bytes BIGINT,

    -- Probe execution stats
    active_targets INTEGER,
    probes_executed BIGINT,
    probes_succeeded BIGINT,
    probes_failed BIGINT,
    probes_per_second REAL,

    -- Result shipping stats
    results_queued INTEGER,
    results_shipped BIGINT,
    results_failed BIGINT,
    results_bytes_shipped BIGINT,

    -- Assignment stats
    assignment_version BIGINT,
    targets_by_tier JSONB,  -- {"infrastructure": 1000, "vip": 1800, "standard": 4000}

    -- Executor stats
    executor_stats JSONB,  -- Per-executor performance data

    -- Health check results
    health_checks JSONB,  -- Array of health check outcomes

    -- Latency to control plane
    control_plane_latency_ms REAL,

    -- Errors in this period
    error_count INTEGER DEFAULT 0,
    last_error TEXT,

    PRIMARY KEY (time, agent_id)
);

-- Convert to hypertable
SELECT create_hypertable('agent_metrics', 'time');

-- Compression policy
ALTER TABLE agent_metrics SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'agent_id'
);
SELECT add_compression_policy('agent_metrics', INTERVAL '1 day');

-- Retention policy (keep agent metrics for 30 days)
SELECT add_retention_policy('agent_metrics', INTERVAL '30 days');

-- Indexes
CREATE INDEX idx_agent_metrics_agent ON agent_metrics(agent_id, time DESC);
CREATE INDEX idx_agent_metrics_status ON agent_metrics(status, time DESC);

-- =============================================================================
-- AGENT METRICS ROLLUP (Hourly aggregates)
-- =============================================================================

CREATE MATERIALIZED VIEW agent_metrics_1h
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', time) AS bucket,
    agent_id,

    -- Availability
    COUNT(*) AS sample_count,
    COUNT(*) FILTER (WHERE status = 'active') AS active_count,

    -- Resource averages
    AVG(cpu_percent) AS avg_cpu_percent,
    MAX(cpu_percent) AS max_cpu_percent,
    AVG(memory_mb) AS avg_memory_mb,
    MAX(memory_mb) AS max_memory_mb,

    -- Probing stats
    MAX(probes_executed) - MIN(probes_executed) AS probes_in_period,
    AVG(probes_per_second) AS avg_probes_per_second,
    MAX(probes_per_second) AS max_probes_per_second,

    -- Shipping stats
    MAX(results_shipped) - MIN(results_shipped) AS results_shipped_in_period,
    AVG(results_queued) AS avg_results_queued,
    MAX(results_queued) AS max_results_queued,

    -- Errors
    SUM(error_count) AS total_errors,

    -- Control plane latency
    AVG(control_plane_latency_ms) AS avg_cp_latency_ms,
    MAX(control_plane_latency_ms) AS max_cp_latency_ms

FROM agent_metrics
GROUP BY bucket, agent_id
WITH NO DATA;

SELECT add_continuous_aggregate_policy('agent_metrics_1h',
    start_offset => INTERVAL '2 hours',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour'
);

-- =============================================================================
-- AGENT HEALTH EVENTS
-- =============================================================================

-- Track significant health events (state changes, errors, etc.)
CREATE TABLE agent_health_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    event_type VARCHAR(50) NOT NULL,  -- 'status_change', 'error', 'recovered', 'degraded'
    severity alert_severity NOT NULL DEFAULT 'info',

    -- Event details
    message TEXT NOT NULL,
    details JSONB,

    -- Previous/new state for status changes
    previous_status agent_status,
    new_status agent_status,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_health_events_agent ON agent_health_events(agent_id, created_at DESC);
CREATE INDEX idx_agent_health_events_type ON agent_health_events(event_type, created_at DESC);

-- =============================================================================
-- AGENT PERFORMANCE THRESHOLDS
-- =============================================================================

-- Configurable thresholds for agent health alerting
CREATE TABLE agent_health_thresholds (
    id INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),  -- Singleton

    -- Resource thresholds
    cpu_warning_percent REAL DEFAULT 70,
    cpu_critical_percent REAL DEFAULT 90,
    memory_warning_percent REAL DEFAULT 70,
    memory_critical_percent REAL DEFAULT 90,
    disk_warning_percent REAL DEFAULT 80,
    disk_critical_percent REAL DEFAULT 95,

    -- Performance thresholds
    min_probes_per_second REAL DEFAULT 100,
    max_results_queued INTEGER DEFAULT 5000,
    max_control_plane_latency_ms REAL DEFAULT 5000,

    -- Heartbeat thresholds
    heartbeat_warning_seconds INTEGER DEFAULT 60,
    heartbeat_critical_seconds INTEGER DEFAULT 120,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO agent_health_thresholds (id) VALUES (1);

-- =============================================================================
-- HELPER FUNCTIONS
-- =============================================================================

-- Get current health summary for an agent
CREATE OR REPLACE FUNCTION get_agent_health_summary(p_agent_id UUID)
RETURNS TABLE(
    status agent_status,
    last_heartbeat TIMESTAMPTZ,
    seconds_since_heartbeat INTEGER,
    cpu_percent REAL,
    memory_mb REAL,
    probes_per_second REAL,
    results_queued INTEGER,
    active_targets INTEGER,
    health_score REAL  -- 0-100
) AS $$
BEGIN
    RETURN QUERY
    WITH latest AS (
        SELECT *
        FROM agent_metrics
        WHERE agent_id = p_agent_id
        ORDER BY time DESC
        LIMIT 1
    ),
    thresholds AS (
        SELECT * FROM agent_health_thresholds WHERE id = 1
    )
    SELECT
        l.status,
        l.time AS last_heartbeat,
        EXTRACT(EPOCH FROM (NOW() - l.time))::INTEGER AS seconds_since_heartbeat,
        l.cpu_percent,
        l.memory_mb,
        l.probes_per_second,
        l.results_queued,
        l.active_targets,
        -- Calculate health score (simplified)
        GREATEST(0, LEAST(100,
            100.0
            - (CASE WHEN l.cpu_percent > t.cpu_warning_percent THEN 20 ELSE 0 END)
            - (CASE WHEN l.memory_percent > t.memory_warning_percent THEN 20 ELSE 0 END)
            - (CASE WHEN l.results_queued > t.max_results_queued / 2 THEN 10 ELSE 0 END)
            - (CASE WHEN l.error_count > 0 THEN 10 ELSE 0 END)
        ))::REAL AS health_score
    FROM latest l, thresholds t;
END;
$$ LANGUAGE plpgsql;

-- Get fleet health overview
CREATE OR REPLACE FUNCTION get_fleet_health_overview()
RETURNS TABLE(
    total_agents INTEGER,
    active_agents INTEGER,
    degraded_agents INTEGER,
    offline_agents INTEGER,
    total_targets INTEGER,
    avg_probes_per_second REAL,
    total_results_queued INTEGER
) AS $$
BEGIN
    RETURN QUERY
    WITH agent_status_counts AS (
        SELECT
            COUNT(*) AS total,
            COUNT(*) FILTER (WHERE status = 'active') AS active,
            COUNT(*) FILTER (WHERE status = 'degraded') AS degraded,
            COUNT(*) FILTER (WHERE status = 'offline') AS offline
        FROM agents
    ),
    latest_metrics AS (
        SELECT DISTINCT ON (agent_id)
            agent_id,
            active_targets,
            probes_per_second,
            results_queued
        FROM agent_metrics
        ORDER BY agent_id, time DESC
    )
    SELECT
        asc.total::INTEGER,
        asc.active::INTEGER,
        asc.degraded::INTEGER,
        asc.offline::INTEGER,
        COALESCE(SUM(lm.active_targets), 0)::INTEGER,
        COALESCE(AVG(lm.probes_per_second), 0)::REAL,
        COALESCE(SUM(lm.results_queued), 0)::INTEGER
    FROM agent_status_counts asc
    LEFT JOIN latest_metrics lm ON true
    GROUP BY asc.total, asc.active, asc.degraded, asc.offline;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- TRIGGERS FOR STATUS CHANGES
-- =============================================================================

-- Update agent status based on heartbeat timeout
CREATE OR REPLACE FUNCTION update_agent_status()
RETURNS void AS $$
DECLARE
    t agent_health_thresholds%ROWTYPE;
    warning_threshold INTERVAL;
    critical_threshold INTERVAL;
BEGIN
    SELECT * INTO t FROM agent_health_thresholds WHERE id = 1;
    warning_threshold := make_interval(secs => t.heartbeat_warning_seconds);
    critical_threshold := make_interval(secs => t.heartbeat_critical_seconds);

    -- Mark agents as degraded if heartbeat is late
    UPDATE agents
    SET status = 'degraded', updated_at = NOW()
    WHERE status = 'active'
      AND last_heartbeat < NOW() - warning_threshold
      AND last_heartbeat >= NOW() - critical_threshold;

    -- Mark agents as offline if heartbeat is very late
    UPDATE agents
    SET status = 'offline', updated_at = NOW()
    WHERE status IN ('active', 'degraded')
      AND last_heartbeat < NOW() - critical_threshold;
END;
$$ LANGUAGE plpgsql;
