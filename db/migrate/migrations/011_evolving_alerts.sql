-- Evolving Alerts System
-- Alerts track individual anomalies that evolve over time (escalate, de-escalate, resolve)
-- Alerts correlate into Incidents for blast radius tracking
--
-- Run with: psql -d icmpmon -f 011_evolving_alerts.sql

-- =============================================================================
-- ALERT TYPES & ENUMS
-- =============================================================================

CREATE TYPE alert_severity AS ENUM ('info', 'warning', 'critical');
CREATE TYPE alert_status AS ENUM ('active', 'acknowledged', 'resolved');
CREATE TYPE alert_type AS ENUM (
    'availability',      -- Target unreachable
    'latency',           -- Latency degradation
    'packet_loss',       -- Significant packet loss
    'agent_down',        -- Monitoring agent offline
    'security_violation' -- Expected-failure test passed (firewall breach)
);

-- =============================================================================
-- ALERTS TABLE
-- Tracks individual anomalies that evolve over time
-- =============================================================================

CREATE TABLE alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- What's affected
    target_id UUID REFERENCES targets(id) ON DELETE CASCADE,
    target_ip INET NOT NULL,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,  -- Which agent detected (NULL if consensus)

    -- Classification
    alert_type alert_type NOT NULL,
    severity alert_severity NOT NULL,
    status alert_status NOT NULL DEFAULT 'active',

    -- Evolution tracking
    initial_severity alert_severity NOT NULL,
    peak_severity alert_severity NOT NULL,

    -- Metrics at various points
    initial_latency_ms DOUBLE PRECISION,
    initial_packet_loss DOUBLE PRECISION,
    peak_latency_ms DOUBLE PRECISION,
    peak_packet_loss DOUBLE PRECISION,
    current_latency_ms DOUBLE PRECISION,
    current_packet_loss DOUBLE PRECISION,

    -- Human-readable
    title TEXT NOT NULL,
    message TEXT,

    -- Timeline
    detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by VARCHAR(255),
    resolved_at TIMESTAMPTZ,

    -- Correlation to incidents
    incident_id UUID REFERENCES incidents(id) ON DELETE SET NULL,
    correlation_key VARCHAR(255),  -- For grouping related alerts (subnet:xxx, target:xxx, agent:xxx)

    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for common queries
CREATE INDEX idx_alerts_status ON alerts(status) WHERE status != 'resolved';
CREATE INDEX idx_alerts_target ON alerts(target_id, status);
CREATE INDEX idx_alerts_agent ON alerts(agent_id) WHERE agent_id IS NOT NULL;
CREATE INDEX idx_alerts_incident ON alerts(incident_id) WHERE incident_id IS NOT NULL;
CREATE INDEX idx_alerts_detected ON alerts(detected_at DESC);
CREATE INDEX idx_alerts_correlation ON alerts(correlation_key) WHERE correlation_key IS NOT NULL;
CREATE INDEX idx_alerts_severity ON alerts(severity, status);
CREATE INDEX idx_alerts_type ON alerts(alert_type, status);

-- =============================================================================
-- ALERT EVENTS TABLE
-- Append-only timeline of changes to alerts
-- =============================================================================

CREATE TABLE alert_events (
    id BIGSERIAL,  -- Use BIGSERIAL for hypertable compatibility
    alert_id UUID NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,

    event_type VARCHAR(50) NOT NULL,
    -- Event types:
    --   'created'            - Alert first detected
    --   'escalated'          - Severity increased
    --   'de_escalated'       - Severity decreased
    --   'acknowledged'       - Human acknowledged
    --   'unacknowledged'     - Acknowledgment reverted (e.g., on re-escalation)
    --   'linked_to_incident' - Alert joined an incident
    --   'metrics_updated'    - Current metrics changed
    --   'resolved'           - Alert resolved
    --   'reopened'           - Alert reopened after resolution

    -- What changed
    old_severity alert_severity,
    new_severity alert_severity,
    old_status alert_status,
    new_status alert_status,

    -- Metrics at time of event
    latency_ms DOUBLE PRECISION,
    packet_loss DOUBLE PRECISION,

    -- Context
    description TEXT,
    details JSONB,
    triggered_by VARCHAR(255) NOT NULL,  -- 'system', 'alert_worker', 'user:xxx'

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Make alert_events a hypertable for efficient time-series queries and retention
SELECT create_hypertable('alert_events', 'created_at', if_not_exists => TRUE);

-- Indexes for alert_events
CREATE INDEX idx_alert_events_alert ON alert_events(alert_id, created_at DESC);
CREATE INDEX idx_alert_events_type ON alert_events(event_type, created_at DESC);

-- Retention: keep alert events for 90 days
SELECT add_retention_policy('alert_events', INTERVAL '90 days');

-- =============================================================================
-- ALERT CONFIGURATION TABLE
-- Configurable thresholds without code changes
-- =============================================================================

CREATE TABLE alert_config (
    key VARCHAR(100) PRIMARY KEY,
    value JSONB NOT NULL,
    description TEXT,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Insert default configuration
INSERT INTO alert_config (key, value, description) VALUES
    ('correlation_window_seconds', '300', 'How long to look back for NEW alert correlation (5 min default)'),
    ('resolution_probe_count', '3', 'Consecutive healthy probes before resolving an alert'),
    ('incident_creation_threshold', '2', 'Min correlated alerts before creating an incident'),
    ('escalation_latency_warning_ms', '100', 'Latency threshold for warning severity'),
    ('escalation_latency_critical_ms', '500', 'Latency threshold for critical severity'),
    ('escalation_packet_loss_warning_pct', '5', 'Packet loss threshold for warning severity'),
    ('escalation_packet_loss_critical_pct', '20', 'Packet loss threshold for critical severity'),
    ('alert_retention_days', '90', 'How long to keep resolved alerts'),
    ('incident_quiet_gap_minutes', '30', 'Gap without alerts before treating new alerts as separate incident'),
    ('incident_max_duration_hours', '24', 'After this, consider splitting into new incident even if related');

-- =============================================================================
-- ENHANCE INCIDENTS FOR ALERT AGGREGATION
-- =============================================================================

-- Add columns to track alert aggregation on incidents
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS alert_ids UUID[] DEFAULT '{}';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS alert_count INTEGER DEFAULT 0;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS last_alert_at TIMESTAMPTZ;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS correlation_key VARCHAR(255);

-- Track incident evolution in more detail
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS evolution_history JSONB DEFAULT '[]';
-- Example: [{"at": "...", "event": "alert_added", "alert_id": "...", "blast_radius": 5}]

-- Index for correlation queries
CREATE INDEX IF NOT EXISTS idx_incidents_correlation ON incidents(correlation_key) WHERE correlation_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_incidents_last_alert ON incidents(last_alert_at DESC) WHERE status != 'resolved';

-- =============================================================================
-- HELPER FUNCTIONS
-- =============================================================================

-- Get current anomalies from agent_target_state for alert generation
CREATE OR REPLACE FUNCTION get_current_anomalies(p_lookback INTERVAL DEFAULT '5 minutes')
RETURNS TABLE(
    target_id UUID,
    target_ip INET,
    agent_id UUID,
    anomaly_type VARCHAR,
    severity VARCHAR,
    latency_ms DOUBLE PRECISION,
    packet_loss DOUBLE PRECISION,
    z_score DOUBLE PRECISION,
    subnet_id UUID,
    consecutive_failures INTEGER
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        ats.target_id,
        t.ip_address AS target_ip,
        ats.agent_id,
        CASE
            WHEN ats.status = 'down' THEN 'availability'
            WHEN ats.current_z_score > 3 THEN 'latency'
            WHEN ats.current_packet_loss > 5 THEN 'packet_loss'
            ELSE 'unknown'
        END::VARCHAR AS anomaly_type,
        CASE
            WHEN ats.status = 'down' OR ats.current_packet_loss > 20 THEN 'critical'
            WHEN ats.current_z_score > 3 OR ats.current_packet_loss > 5 THEN 'warning'
            ELSE 'info'
        END::VARCHAR AS severity,
        COALESCE(
            (SELECT avg(pr.latency_ms) FROM probe_results pr
             WHERE pr.target_id = ats.target_id
               AND pr.agent_id = ats.agent_id
               AND pr.time > NOW() - p_lookback
               AND pr.success = true),
            0
        ) AS latency_ms,
        COALESCE(ats.current_packet_loss, 0) AS packet_loss,
        COALESCE(ats.current_z_score, 0) AS z_score,
        t.subnet_id,
        COALESCE(ats.consecutive_anomalies, 0) AS consecutive_failures
    FROM agent_target_state ats
    JOIN targets t ON ats.target_id = t.id
    WHERE ats.status IN ('down', 'degraded')
       OR ats.current_z_score > 3
       OR ats.current_packet_loss > 5;
END;
$$ LANGUAGE plpgsql;

-- Add an alert to an incident (updating both sides of the relationship)
CREATE OR REPLACE FUNCTION add_alert_to_incident(
    p_alert_id UUID,
    p_incident_id UUID
) RETURNS void AS $$
DECLARE
    v_alert_target_id UUID;
    v_alert_agent_id UUID;
BEGIN
    -- Get alert details
    SELECT target_id, agent_id INTO v_alert_target_id, v_alert_agent_id
    FROM alerts WHERE id = p_alert_id;

    -- Update alert
    UPDATE alerts
    SET incident_id = p_incident_id, last_updated_at = NOW()
    WHERE id = p_alert_id;

    -- Update incident
    UPDATE incidents
    SET
        alert_ids = array_append(COALESCE(alert_ids, '{}'), p_alert_id),
        alert_count = COALESCE(alert_count, 0) + 1,
        last_alert_at = NOW(),
        affected_target_ids = CASE
            WHEN NOT (v_alert_target_id = ANY(COALESCE(affected_target_ids, '{}')))
            THEN array_append(COALESCE(affected_target_ids, '{}'), v_alert_target_id)
            ELSE affected_target_ids
        END,
        affected_agent_ids = CASE
            WHEN v_alert_agent_id IS NOT NULL
                 AND NOT (v_alert_agent_id = ANY(COALESCE(affected_agent_ids, '{}')))
            THEN array_append(COALESCE(affected_agent_ids, '{}'), v_alert_agent_id)
            ELSE affected_agent_ids
        END,
        evolution_history = evolution_history || jsonb_build_object(
            'at', NOW(),
            'event', 'alert_added',
            'alert_id', p_alert_id,
            'target_id', v_alert_target_id,
            'blast_radius', COALESCE(array_length(affected_target_ids, 1), 0) + 1
        ),
        updated_at = NOW()
    WHERE id = p_incident_id;
END;
$$ LANGUAGE plpgsql;

-- Get alerts for an incident
CREATE OR REPLACE FUNCTION get_incident_alerts(p_incident_id UUID)
RETURNS TABLE(
    id UUID,
    target_id UUID,
    target_ip INET,
    agent_id UUID,
    alert_type alert_type,
    severity alert_severity,
    status alert_status,
    detected_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    peak_severity alert_severity
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        a.id, a.target_id, a.target_ip, a.agent_id,
        a.alert_type, a.severity, a.status,
        a.detected_at, a.resolved_at, a.peak_severity
    FROM alerts a
    WHERE a.incident_id = p_incident_id
    ORDER BY a.detected_at DESC;
END;
$$ LANGUAGE plpgsql;

-- Get alert config value (with default)
CREATE OR REPLACE FUNCTION get_alert_config(p_key VARCHAR, p_default JSONB DEFAULT NULL)
RETURNS JSONB AS $$
DECLARE
    v_value JSONB;
BEGIN
    SELECT value INTO v_value FROM alert_config WHERE key = p_key;
    RETURN COALESCE(v_value, p_default);
END;
$$ LANGUAGE plpgsql;

-- Find active incidents by correlation key
CREATE OR REPLACE FUNCTION find_active_incidents_by_correlation(p_correlation_key VARCHAR)
RETURNS TABLE(
    id UUID,
    incident_type incident_type,
    severity incident_severity,
    affected_target_ids UUID[],
    affected_agent_ids UUID[],
    detected_at TIMESTAMPTZ,
    last_alert_at TIMESTAMPTZ,
    alert_count INTEGER
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        i.id, i.incident_type, i.severity,
        i.affected_target_ids, i.affected_agent_ids,
        i.detected_at, i.last_alert_at, i.alert_count
    FROM incidents i
    WHERE i.status != 'resolved'
      AND i.correlation_key = p_correlation_key
    ORDER BY i.detected_at DESC;
END;
$$ LANGUAGE plpgsql;

-- Get unlinked alerts by correlation key (for incident creation)
CREATE OR REPLACE FUNCTION get_unlinked_alerts_by_correlation(
    p_correlation_key VARCHAR,
    p_window INTERVAL DEFAULT '5 minutes'
)
RETURNS TABLE(
    id UUID,
    target_id UUID,
    agent_id UUID,
    severity alert_severity,
    detected_at TIMESTAMPTZ
) AS $$
BEGIN
    RETURN QUERY
    SELECT a.id, a.target_id, a.agent_id, a.severity, a.detected_at
    FROM alerts a
    WHERE a.incident_id IS NULL
      AND a.status = 'active'
      AND a.correlation_key = p_correlation_key
      AND a.detected_at > NOW() - p_window
    ORDER BY a.detected_at;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- ALERT STATISTICS VIEW
-- =============================================================================

CREATE OR REPLACE VIEW alert_stats AS
SELECT
    COUNT(*) FILTER (WHERE status = 'active') AS active_count,
    COUNT(*) FILTER (WHERE status = 'active' AND severity = 'critical') AS critical_count,
    COUNT(*) FILTER (WHERE status = 'active' AND severity = 'warning') AS warning_count,
    COUNT(*) FILTER (WHERE status = 'acknowledged') AS acknowledged_count,
    COUNT(*) FILTER (WHERE status = 'resolved' AND resolved_at > NOW() - INTERVAL '24 hours') AS resolved_today,
    COUNT(*) FILTER (WHERE detected_at > NOW() - INTERVAL '7 days') AS total_this_week,
    AVG(EXTRACT(EPOCH FROM (resolved_at - detected_at))/60)
        FILTER (WHERE status = 'resolved' AND resolved_at > NOW() - INTERVAL '7 days') AS avg_resolution_minutes
FROM alerts;

-- =============================================================================
-- COMMENTS
-- =============================================================================

COMMENT ON TABLE alerts IS 'Individual anomaly detections that evolve over time';
COMMENT ON TABLE alert_events IS 'Append-only timeline of alert state changes';
COMMENT ON TABLE alert_config IS 'Configurable thresholds for alert behavior';
COMMENT ON COLUMN alerts.correlation_key IS 'Groups related alerts: subnet:xxx, target:xxx, or agent:xxx';
COMMENT ON COLUMN alerts.peak_severity IS 'Highest severity reached during alert lifecycle';
COMMENT ON COLUMN incidents.alert_ids IS 'Array of alert IDs that belong to this incident';
COMMENT ON COLUMN incidents.last_alert_at IS 'When the most recent alert was added (for gap detection)';
