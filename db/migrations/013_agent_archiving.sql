-- =============================================================================
-- MIGRATION 013: Agent Archiving Support
-- =============================================================================
-- Adds soft-delete (archive) capability for agents to preserve historical data
-- while removing them from operational queries.
--
-- Philosophy:
-- - Read queries (GetAgent, ListAgents) INCLUDE archived agents by default
--   so historical data displays correctly
-- - Operational queries (agent selection, heartbeats) EXCLUDE archived agents
-- =============================================================================

-- Add archive columns to agents table
ALTER TABLE agents ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS archive_reason TEXT;

-- Index for efficient filtering of active agents in operational queries
CREATE INDEX IF NOT EXISTS idx_agents_active ON agents(id) WHERE archived_at IS NULL;

-- Index for finding archived agents
CREATE INDEX IF NOT EXISTS idx_agents_archived ON agents(archived_at) WHERE archived_at IS NOT NULL;

-- =============================================================================
-- ACTIVITY LOG SUPPORT
-- =============================================================================
-- Ensure activity_log can track agent archival events

-- Add agent_name column if not exists (for when agent_id references archived agent)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'activity_log' AND column_name = 'agent_name'
    ) THEN
        ALTER TABLE activity_log ADD COLUMN agent_name VARCHAR(255);
    END IF;
END $$;

-- =============================================================================
-- HELPER FUNCTION: Check if agent is active (not archived)
-- =============================================================================
CREATE OR REPLACE FUNCTION is_agent_active(p_agent_id UUID)
RETURNS BOOLEAN AS $$
BEGIN
    RETURN EXISTS (
        SELECT 1 FROM agents
        WHERE id = p_agent_id AND archived_at IS NULL
    );
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- UPDATE get_current_anomalies TO EXCLUDE ARCHIVED AGENTS
-- =============================================================================
-- This function feeds the alert worker - we don't want to generate alerts
-- for archived agents.
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
    JOIN agents a ON ats.agent_id = a.id
    WHERE (ats.status IN ('down', 'degraded')
       OR ats.current_z_score > 3
       OR ats.current_packet_loss > 5)
      AND t.archived_at IS NULL  -- Exclude archived targets
      AND a.archived_at IS NULL; -- Exclude archived agents
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- COMMENTS
-- =============================================================================
COMMENT ON COLUMN agents.archived_at IS 'Timestamp when agent was archived (soft-deleted). NULL means active.';
COMMENT ON COLUMN agents.archive_reason IS 'Reason for archiving the agent (e.g., decommissioned, replaced, etc.)';
COMMENT ON FUNCTION is_agent_active(UUID) IS 'Returns true if the agent exists and is not archived';
