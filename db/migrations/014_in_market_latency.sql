-- In-Market Latency & City-to-City Matrix Support
--
-- Adds denormalized region columns to probe_results for efficient aggregation:
-- - agent_region: the region of the agent that sent the probe
-- - target_region: the region of the target's subnet
-- - is_in_market: true when agent_region == target_region (primary SLA metric)
--
-- Also creates continuous aggregates for:
-- - In-market latency per target (for SLA compliance)
-- - City-to-city latency matrix (for network topology visualization)

-- =============================================================================
-- SCHEMA CHANGES
-- =============================================================================

-- Add denormalized region columns to probe_results for efficient aggregation
ALTER TABLE probe_results ADD COLUMN IF NOT EXISTS agent_region TEXT;
ALTER TABLE probe_results ADD COLUMN IF NOT EXISTS target_region TEXT;
ALTER TABLE probe_results ADD COLUMN IF NOT EXISTS is_in_market BOOLEAN;

-- Index for in-market queries (filtered to specific target)
CREATE INDEX IF NOT EXISTS idx_probe_results_in_market
    ON probe_results(target_id, time DESC)
    WHERE is_in_market = true;

-- Index for city-to-city matrix queries
CREATE INDEX IF NOT EXISTS idx_probe_results_regions
    ON probe_results(agent_region, target_region, time DESC)
    WHERE agent_region IS NOT NULL AND target_region IS NOT NULL;

-- Add in-market fields to target_state for quick lookups
ALTER TABLE target_state ADD COLUMN IF NOT EXISTS in_market_avg_latency_ms REAL;
ALTER TABLE target_state ADD COLUMN IF NOT EXISTS in_market_agent_count INTEGER DEFAULT 0;

-- =============================================================================
-- CONTINUOUS AGGREGATES
-- =============================================================================

-- 1. In-Market hourly aggregate (per target)
-- Used for: Dashboard in-market latency, target detail SLA compliance
CREATE MATERIALIZED VIEW IF NOT EXISTS probe_hourly_in_market
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', time) AS bucket,
    target_id,
    avg(latency_ms) FILTER (WHERE success) as avg_latency,
    min(latency_ms) FILTER (WHERE success) as min_latency,
    max(latency_ms) FILTER (WHERE success) as max_latency,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms)
        FILTER (WHERE success) as p95_latency,
    avg(packet_loss_pct) as avg_packet_loss,
    count(*) as probe_count,
    sum(case when success then 1 else 0 end) as success_count,
    count(distinct agent_id) as agent_count
FROM probe_results
WHERE is_in_market = true
GROUP BY bucket, target_id
WITH NO DATA;

-- Refresh policy for in-market aggregate
SELECT add_continuous_aggregate_policy('probe_hourly_in_market',
    start_offset => INTERVAL '3 hours',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists => true
);

-- Index for efficient target lookups
CREATE INDEX IF NOT EXISTS idx_probe_hourly_in_market_target
    ON probe_hourly_in_market(target_id, bucket DESC);

-- 2. City-to-City hourly aggregate (for latency matrix)
-- Used for: Latency matrix page showing region-to-region latency
CREATE MATERIALIZED VIEW IF NOT EXISTS probe_hourly_region_matrix
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', time) AS bucket,
    agent_region,
    target_region,
    avg(latency_ms) FILTER (WHERE success) as avg_latency,
    min(latency_ms) FILTER (WHERE success) as min_latency,
    max(latency_ms) FILTER (WHERE success) as max_latency,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms)
        FILTER (WHERE success) as p95_latency,
    avg(packet_loss_pct) as avg_packet_loss,
    count(*) as probe_count,
    sum(case when success then 1 else 0 end) as success_count,
    count(distinct agent_id) as agent_count,
    count(distinct target_id) as target_count
FROM probe_results
WHERE agent_region IS NOT NULL AND target_region IS NOT NULL
GROUP BY bucket, agent_region, target_region
WITH NO DATA;

-- Refresh policy for region matrix aggregate
SELECT add_continuous_aggregate_policy('probe_hourly_region_matrix',
    start_offset => INTERVAL '3 hours',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists => true
);

-- Index for efficient region pair lookups
CREATE INDEX IF NOT EXISTS idx_probe_hourly_region_matrix_lookup
    ON probe_hourly_region_matrix(agent_region, target_region, bucket DESC);

-- Index for time-based queries across all regions
CREATE INDEX IF NOT EXISTS idx_probe_hourly_region_matrix_time
    ON probe_hourly_region_matrix(bucket DESC);

-- =============================================================================
-- ACTIVITY LOG
-- =============================================================================

INSERT INTO activity_log (category, event_type, details, triggered_by, severity)
VALUES ('system', 'config_changed',
    '{"migration": "014_in_market_latency", "description": "Added in-market latency tracking and city-to-city matrix support"}',
    'system', 'info');
