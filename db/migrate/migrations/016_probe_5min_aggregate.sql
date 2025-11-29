-- Migration 016: Create probe_5min continuous aggregate for fast metrics explorer queries
-- This aggregate provides 5-minute granularity with agent-level detail for real-time monitoring

-- Create 5-minute continuous aggregate with agent_id for flexible querying
-- This is the main aggregate for the Metrics Explorer on short-to-medium time ranges
CREATE MATERIALIZED VIEW IF NOT EXISTS probe_5min
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('5 minutes', time) AS bucket,
    agent_id,
    target_id,
    -- Latency stats (filtered to only successful probes)
    AVG(latency_ms) FILTER (WHERE success) AS avg_latency,
    MIN(latency_ms) FILTER (WHERE success) AS min_latency,
    MAX(latency_ms) FILTER (WHERE success) AS max_latency,
    -- Percentiles for latency analysis
    percentile_cont(0.50) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE success) AS p50_latency,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE success) AS p95_latency,
    percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE success) AS p99_latency,
    -- Jitter (standard deviation of latency)
    STDDEV(latency_ms) FILTER (WHERE success) AS latency_stddev,
    -- Packet loss
    AVG(packet_loss_pct) AS avg_packet_loss,
    -- Counts
    COUNT(*) AS probe_count,
    SUM(CASE WHEN success THEN 1 ELSE 0 END) AS success_count,
    SUM(CASE WHEN NOT success THEN 1 ELSE 0 END) AS failure_count
FROM probe_results
GROUP BY bucket, agent_id, target_id
WITH NO DATA;

-- Add refresh policy: refresh every 5 minutes, with 30 minute refresh window
-- start_offset=NULL means refresh from the beginning of time
-- end_offset='5 minutes' means don't refresh the most recent 5 minutes (still being written)
SELECT add_continuous_aggregate_policy('probe_5min',
    start_offset => INTERVAL '2 hours',  -- Refresh data up to 2 hours old (handles late-arriving data)
    end_offset => INTERVAL '5 minutes',  -- Don't refresh the current bucket being written to
    schedule_interval => INTERVAL '5 minutes'  -- Run every 5 minutes
);

-- Backfill historical data from existing probe_results
-- This will materialize the aggregate for existing data
CALL refresh_continuous_aggregate('probe_5min', NULL, NOW() - INTERVAL '5 minutes');

-- Create indexes for efficient querying
CREATE INDEX IF NOT EXISTS probe_5min_bucket_idx ON probe_5min (bucket DESC);
CREATE INDEX IF NOT EXISTS probe_5min_agent_id_idx ON probe_5min (agent_id, bucket DESC);
CREATE INDEX IF NOT EXISTS probe_5min_target_id_idx ON probe_5min (target_id, bucket DESC);
CREATE INDEX IF NOT EXISTS probe_5min_agent_target_idx ON probe_5min (agent_id, target_id, bucket DESC);

-- Add comment explaining the purpose
COMMENT ON MATERIALIZED VIEW probe_5min IS 'Pre-aggregated 5-minute metrics for fast Metrics Explorer queries on compressed data';
