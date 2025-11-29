-- =============================================================================
-- MIGRATION 015: Data Volume Management
-- =============================================================================
-- Implements comprehensive data volume management for scale:
-- 1. Compression policies for all hypertables
-- 2. Retention policies for continuous aggregates
-- 3. Database size monitoring infrastructure
-- 4. Forecasting capabilities
-- 5. Cleanup jobs for non-hypertable data
-- =============================================================================

-- =============================================================================
-- 1. COMPRESSION POLICIES (Missing)
-- =============================================================================

-- alert_events: Add compression (segment by alert_id for timeline queries)
-- Note: alert_events was created in 011_evolving_alerts.sql without compression
ALTER TABLE alert_events SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'alert_id'
);
SELECT add_compression_policy('alert_events', INTERVAL '1 day');

-- activity_log: Add compression (segment by category for filtering)
-- Note: activity_log was created in 006_pilot_monitoring.sql without compression
ALTER TABLE activity_log SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'category'
);
SELECT add_compression_policy('activity_log', INTERVAL '7 days');

-- =============================================================================
-- 2. CONTINUOUS AGGREGATE RETENTION POLICIES
-- =============================================================================

-- probe_results_1m: Keep 30 days (minute resolution rarely needed beyond this)
SELECT add_retention_policy('probe_results_1m', INTERVAL '30 days', if_not_exists => true);

-- probe_hourly: Keep 1 year
SELECT add_retention_policy('probe_hourly', INTERVAL '1 year', if_not_exists => true);

-- probe_daily: Keep 2 years
SELECT add_retention_policy('probe_daily', INTERVAL '2 years', if_not_exists => true);

-- probe_hourly_in_market: Keep 1 year (matches probe_hourly)
SELECT add_retention_policy('probe_hourly_in_market', INTERVAL '1 year', if_not_exists => true);

-- probe_hourly_region_matrix: Keep 1 year (matches probe_hourly)
SELECT add_retention_policy('probe_hourly_region_matrix', INTERVAL '1 year', if_not_exists => true);

-- agent_metrics_1h: Keep 90 days (agent metrics less critical long-term)
SELECT add_retention_policy('agent_metrics_1h', INTERVAL '90 days', if_not_exists => true);

-- alert_events: Extend to 180 days for better post-mortem analysis
SELECT remove_retention_policy('alert_events', if_exists => true);
SELECT add_retention_policy('alert_events', INTERVAL '180 days');

-- =============================================================================
-- 3. DATABASE SIZE MONITORING INFRASTRUCTURE
-- =============================================================================

-- Table to track historical size metrics
CREATE TABLE IF NOT EXISTS database_size_history (
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    table_name TEXT NOT NULL,
    total_bytes BIGINT NOT NULL,
    row_estimate BIGINT,
    compression_ratio REAL,
    chunk_count INTEGER,
    compressed_chunk_count INTEGER,
    PRIMARY KEY (recorded_at, table_name)
);

SELECT create_hypertable('database_size_history', 'recorded_at', if_not_exists => TRUE);
SELECT add_retention_policy('database_size_history', INTERVAL '2 years', if_not_exists => true);

-- Function to record current sizes (should be called daily via scheduler)
CREATE OR REPLACE FUNCTION record_database_sizes()
RETURNS INTEGER AS $$
DECLARE
    v_count INTEGER := 0;
BEGIN
    INSERT INTO database_size_history (
        table_name,
        total_bytes,
        row_estimate,
        compression_ratio,
        chunk_count,
        compressed_chunk_count
    )
    SELECT
        h.hypertable_name,
        hypertable_size(format('%I.%I', h.hypertable_schema, h.hypertable_name)::regclass),
        (SELECT reltuples::bigint FROM pg_class WHERE relname = h.hypertable_name),
        CASE WHEN cs.after_compression_total_bytes > 0
             THEN cs.before_compression_total_bytes::real / cs.after_compression_total_bytes
             ELSE NULL
        END,
        (SELECT COUNT(*)::integer FROM timescaledb_information.chunks c WHERE c.hypertable_name = h.hypertable_name),
        (SELECT COUNT(*)::integer FROM timescaledb_information.chunks c WHERE c.hypertable_name = h.hypertable_name AND c.is_compressed)
    FROM timescaledb_information.hypertables h
    LEFT JOIN LATERAL (
        SELECT
            SUM(before_compression_total_bytes) as before_compression_total_bytes,
            SUM(after_compression_total_bytes) as after_compression_total_bytes
        FROM timescaledb_information.compressed_chunk_stats ccs
        WHERE ccs.hypertable_name = h.hypertable_name
    ) cs ON true;

    GET DIAGNOSTICS v_count = ROW_COUNT;
    RETURN v_count;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- 4. DATABASE SIZE VIEWS
-- =============================================================================

-- Current size breakdown by table
CREATE OR REPLACE VIEW database_size_current AS
SELECT
    h.hypertable_name as table_name,
    pg_size_pretty(hypertable_size(format('%I.%I', h.hypertable_schema, h.hypertable_name)::regclass)) as total_size,
    hypertable_size(format('%I.%I', h.hypertable_schema, h.hypertable_name)::regclass) as total_bytes,
    (SELECT COUNT(*) FROM timescaledb_information.chunks c WHERE c.hypertable_name = h.hypertable_name) as chunk_count,
    (SELECT COUNT(*) FILTER (WHERE is_compressed) FROM timescaledb_information.chunks c WHERE c.hypertable_name = h.hypertable_name) as compressed_chunks,
    CASE
        WHEN cs.after_compression_total_bytes > 0
        THEN round(cs.before_compression_total_bytes::numeric / cs.after_compression_total_bytes, 1)
        ELSE NULL
    END as compression_ratio
FROM timescaledb_information.hypertables h
LEFT JOIN LATERAL (
    SELECT
        SUM(before_compression_total_bytes) as before_compression_total_bytes,
        SUM(after_compression_total_bytes) as after_compression_total_bytes
    FROM timescaledb_information.compressed_chunk_stats ccs
    WHERE ccs.hypertable_name = h.hypertable_name
) cs ON true
ORDER BY hypertable_size(format('%I.%I', h.hypertable_schema, h.hypertable_name)::regclass) DESC;

-- Growth trends over time
CREATE OR REPLACE VIEW database_growth_trends AS
WITH daily_sizes AS (
    SELECT
        date_trunc('day', recorded_at) as day,
        table_name,
        avg(total_bytes) as avg_bytes
    FROM database_size_history
    WHERE recorded_at > NOW() - INTERVAL '90 days'
    GROUP BY date_trunc('day', recorded_at), table_name
),
growth AS (
    SELECT
        table_name,
        day,
        avg_bytes,
        avg_bytes - lag(avg_bytes) OVER (PARTITION BY table_name ORDER BY day) as daily_delta
    FROM daily_sizes
)
SELECT
    table_name,
    pg_size_pretty(max(avg_bytes)::bigint) as current_size,
    pg_size_pretty(avg(daily_delta)::bigint) as avg_daily_growth,
    pg_size_pretty((avg(daily_delta) * 30)::bigint) as projected_30d_growth,
    pg_size_pretty((avg(daily_delta) * 365)::bigint) as projected_annual_growth,
    CASE
        WHEN avg(daily_delta) > 0
        THEN round(max(avg_bytes) / avg(daily_delta))::integer
        ELSE NULL
    END as days_to_double
FROM growth
WHERE daily_delta IS NOT NULL
GROUP BY table_name
ORDER BY max(avg_bytes) DESC;

-- =============================================================================
-- 5. FORECASTING FUNCTION
-- =============================================================================

CREATE OR REPLACE FUNCTION project_database_size(
    p_days_ahead INTEGER DEFAULT 90,
    p_lookback_days INTEGER DEFAULT 30
)
RETURNS TABLE(
    table_name TEXT,
    current_size TEXT,
    current_bytes BIGINT,
    projected_size TEXT,
    projected_bytes BIGINT,
    daily_growth TEXT,
    daily_growth_bytes BIGINT,
    growth_rate_pct NUMERIC,
    days_to_double INTEGER,
    alert_level TEXT
) AS $$
BEGIN
    RETURN QUERY
    WITH growth_rates AS (
        SELECT
            h.table_name,
            max(h.total_bytes) as current_bytes,
            CASE
                WHEN max(h.total_bytes) > min(h.total_bytes)
                THEN (max(h.total_bytes) - min(h.total_bytes))::float / p_lookback_days
                ELSE 0
            END as daily_growth
        FROM database_size_history h
        WHERE h.recorded_at > NOW() - make_interval(days => p_lookback_days)
        GROUP BY h.table_name
        HAVING COUNT(*) > 1
    )
    SELECT
        g.table_name,
        pg_size_pretty(g.current_bytes),
        g.current_bytes,
        pg_size_pretty((g.current_bytes + (g.daily_growth * p_days_ahead))::BIGINT),
        (g.current_bytes + (g.daily_growth * p_days_ahead))::BIGINT,
        pg_size_pretty(g.daily_growth::BIGINT) || '/day',
        g.daily_growth::BIGINT,
        CASE WHEN g.current_bytes > 0
             THEN round(100.0 * g.daily_growth * 30 / g.current_bytes, 2)
             ELSE 0
        END,
        CASE WHEN g.daily_growth > 0
             THEN (g.current_bytes / g.daily_growth)::INTEGER
             ELSE NULL
        END,
        CASE
            WHEN g.daily_growth > 0 AND g.current_bytes / g.daily_growth < 30 THEN 'critical'
            WHEN g.daily_growth > 0 AND g.current_bytes / g.daily_growth < 90 THEN 'warning'
            ELSE 'ok'
        END
    FROM growth_rates g
    ORDER BY g.current_bytes DESC;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- 6. CLEANUP FOR NON-HYPERTABLE DATA
-- =============================================================================

-- Function to clean up old resolved alerts (alerts table is not a hypertable)
CREATE OR REPLACE FUNCTION cleanup_old_alerts(p_retention_days INTEGER DEFAULT 365)
RETURNS INTEGER AS $$
DECLARE
    v_deleted INTEGER;
BEGIN
    DELETE FROM alerts
    WHERE status = 'resolved'
      AND resolved_at < NOW() - make_interval(days => p_retention_days);

    GET DIAGNOSTICS v_deleted = ROW_COUNT;

    -- Log the cleanup
    INSERT INTO activity_log (category, event_type, details, triggered_by, severity)
    VALUES ('system', 'retention_cleanup',
            jsonb_build_object('table', 'alerts', 'deleted_count', v_deleted, 'retention_days', p_retention_days),
            'system', 'info');

    RETURN v_deleted;
END;
$$ LANGUAGE plpgsql;

-- Function to clean up old resolved incidents
CREATE OR REPLACE FUNCTION cleanup_old_incidents(p_retention_days INTEGER DEFAULT 730)
RETURNS INTEGER AS $$
DECLARE
    v_deleted INTEGER;
BEGIN
    DELETE FROM incidents
    WHERE status = 'resolved'
      AND resolved_at < NOW() - make_interval(days => p_retention_days);

    GET DIAGNOSTICS v_deleted = ROW_COUNT;

    INSERT INTO activity_log (category, event_type, details, triggered_by, severity)
    VALUES ('system', 'retention_cleanup',
            jsonb_build_object('table', 'incidents', 'deleted_count', v_deleted, 'retention_days', p_retention_days),
            'system', 'info');

    RETURN v_deleted;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- 7. CONFIGURATION TABLE FOR RETENTION SETTINGS
-- =============================================================================

CREATE TABLE IF NOT EXISTS retention_config (
    table_name TEXT PRIMARY KEY,
    retention_interval INTERVAL NOT NULL,
    description TEXT,
    last_cleanup TIMESTAMPTZ,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO retention_config (table_name, retention_interval, description) VALUES
    ('probe_results', '90 days', 'Raw probe results'),
    ('probe_results_1m', '30 days', '1-minute aggregates'),
    ('probe_hourly', '1 year', 'Hourly aggregates'),
    ('probe_daily', '2 years', 'Daily aggregates'),
    ('probe_monthly', NULL, 'Monthly aggregates (kept forever)'),
    ('agent_metrics', '30 days', 'Agent health metrics'),
    ('agent_metrics_1h', '90 days', 'Hourly agent metrics'),
    ('alert_events', '180 days', 'Alert event timeline'),
    ('activity_log', '1 year', 'System activity log'),
    ('alerts', '1 year', 'Resolved alerts cleanup'),
    ('incidents', '2 years', 'Resolved incidents cleanup'),
    ('database_size_history', '2 years', 'Size tracking metrics')
ON CONFLICT (table_name) DO UPDATE SET
    retention_interval = EXCLUDED.retention_interval,
    description = EXCLUDED.description,
    updated_at = NOW();

-- =============================================================================
-- 8. COMPREHENSIVE MAINTENANCE FUNCTION
-- =============================================================================

-- Master function to run all maintenance tasks
CREATE OR REPLACE FUNCTION run_database_maintenance()
RETURNS TABLE(
    task TEXT,
    status TEXT,
    details JSONB
) AS $$
DECLARE
    v_count INTEGER;
BEGIN
    -- Record current sizes
    task := 'record_sizes';
    SELECT record_database_sizes() INTO v_count;
    status := 'success';
    details := jsonb_build_object('tables_recorded', v_count);
    RETURN NEXT;

    -- Clean up old alerts
    task := 'cleanup_alerts';
    SELECT cleanup_old_alerts(365) INTO v_count;
    status := 'success';
    details := jsonb_build_object('alerts_deleted', v_count);
    RETURN NEXT;

    -- Clean up old incidents
    task := 'cleanup_incidents';
    SELECT cleanup_old_incidents(730) INTO v_count;
    status := 'success';
    details := jsonb_build_object('incidents_deleted', v_count);
    RETURN NEXT;

    -- Analyze tables for query optimization
    task := 'analyze_tables';
    ANALYZE probe_results;
    ANALYZE agent_metrics;
    ANALYZE alerts;
    status := 'success';
    details := jsonb_build_object('tables_analyzed', 3);
    RETURN NEXT;

    RETURN;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- 9. HELPER FUNCTIONS FOR API
-- =============================================================================

-- Get total database size with breakdown
CREATE OR REPLACE FUNCTION get_database_size_breakdown()
RETURNS TABLE(
    table_name TEXT,
    size_bytes BIGINT,
    size_pretty TEXT,
    compression_ratio REAL,
    chunk_count INTEGER
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        h.hypertable_name::TEXT,
        hypertable_size(format('%I.%I', h.hypertable_schema, h.hypertable_name)::regclass),
        pg_size_pretty(hypertable_size(format('%I.%I', h.hypertable_schema, h.hypertable_name)::regclass)),
        CASE
            WHEN cs.after_compression_total_bytes > 0
            THEN (cs.before_compression_total_bytes::real / cs.after_compression_total_bytes)
            ELSE NULL
        END,
        (SELECT COUNT(*)::integer FROM timescaledb_information.chunks c WHERE c.hypertable_name = h.hypertable_name)
    FROM timescaledb_information.hypertables h
    LEFT JOIN LATERAL (
        SELECT
            SUM(before_compression_total_bytes) as before_compression_total_bytes,
            SUM(after_compression_total_bytes) as after_compression_total_bytes
        FROM timescaledb_information.compressed_chunk_stats ccs
        WHERE ccs.hypertable_name = h.hypertable_name
    ) cs ON true
    ORDER BY hypertable_size(format('%I.%I', h.hypertable_schema, h.hypertable_name)::regclass) DESC;
END;
$$ LANGUAGE plpgsql;

-- Get daily ingestion rate estimate
CREATE OR REPLACE FUNCTION get_daily_ingestion_rate()
RETURNS TABLE(
    table_name TEXT,
    daily_bytes BIGINT,
    daily_pretty TEXT
) AS $$
BEGIN
    RETURN QUERY
    WITH recent AS (
        SELECT
            h.table_name,
            h.total_bytes,
            h.recorded_at,
            lag(h.total_bytes) OVER (PARTITION BY h.table_name ORDER BY h.recorded_at) as prev_bytes,
            lag(h.recorded_at) OVER (PARTITION BY h.table_name ORDER BY h.recorded_at) as prev_time
        FROM database_size_history h
        WHERE h.recorded_at > NOW() - INTERVAL '7 days'
    ),
    daily_growth AS (
        SELECT
            table_name,
            AVG(
                (total_bytes - prev_bytes)::float /
                GREATEST(EXTRACT(EPOCH FROM (recorded_at - prev_time)) / 86400.0, 0.001)
            ) as avg_daily_bytes
        FROM recent
        WHERE prev_bytes IS NOT NULL AND total_bytes > prev_bytes
        GROUP BY table_name
    )
    SELECT
        dg.table_name,
        dg.avg_daily_bytes::BIGINT,
        pg_size_pretty(dg.avg_daily_bytes::BIGINT)
    FROM daily_growth dg
    ORDER BY dg.avg_daily_bytes DESC;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- 10. INITIAL SIZE RECORDING
-- =============================================================================

-- Record initial sizes
SELECT record_database_sizes();

-- =============================================================================
-- 11. LOG MIGRATION
-- =============================================================================

INSERT INTO activity_log (category, event_type, details, triggered_by, severity)
VALUES ('system', 'config_changed',
    jsonb_build_object(
        'migration', '015_data_volume_management',
        'description', 'Comprehensive data volume management: compression, retention, monitoring, forecasting',
        'changes', ARRAY[
            'Added compression to alert_events and activity_log',
            'Added retention policies to continuous aggregates',
            'Created database_size_history tracking table',
            'Created monitoring views and forecasting functions',
            'Added cleanup functions for non-hypertable data',
            'Created comprehensive maintenance function'
        ]
    ),
    'system', 'info');
