-- Migration 021: Agent Status Function
-- Centralizes the agent status computation logic that was previously duplicated
-- across 6+ queries in the codebase.
--
-- The function computes agent status based on:
-- 1. Whether the agent is archived (always returns 'offline')
-- 2. Time since last heartbeat:
--    - No heartbeat or >60s: 'offline'
--    - 30-60s: 'degraded'
--    - <30s: 'active'

-- Create the agent status function
CREATE OR REPLACE FUNCTION get_agent_status(
    p_last_heartbeat TIMESTAMPTZ,
    p_archived_at TIMESTAMPTZ
) RETURNS TEXT AS $$
BEGIN
    -- Archived agents are always offline
    IF p_archived_at IS NOT NULL THEN
        RETURN 'offline';
    END IF;

    -- No heartbeat or heartbeat older than 60 seconds = offline
    IF p_last_heartbeat IS NULL OR
       p_last_heartbeat < NOW() - INTERVAL '60 seconds' THEN
        RETURN 'offline';
    END IF;

    -- Heartbeat between 30-60 seconds old = degraded
    IF p_last_heartbeat < NOW() - INTERVAL '30 seconds' THEN
        RETURN 'degraded';
    END IF;

    -- Recent heartbeat = active
    RETURN 'active';
END;
$$ LANGUAGE plpgsql STABLE;

-- Add a comment documenting the function
COMMENT ON FUNCTION get_agent_status(TIMESTAMPTZ, TIMESTAMPTZ) IS
    'Computes agent status based on heartbeat age and archive status. Returns: active, degraded, or offline.';

-- Create an index-friendly version that can be used in WHERE clauses
-- This is a helper for queries that need to filter by computed status
CREATE OR REPLACE FUNCTION is_agent_online(
    p_last_heartbeat TIMESTAMPTZ,
    p_archived_at TIMESTAMPTZ
) RETURNS BOOLEAN AS $$
BEGIN
    RETURN p_archived_at IS NULL
       AND p_last_heartbeat IS NOT NULL
       AND p_last_heartbeat >= NOW() - INTERVAL '60 seconds';
END;
$$ LANGUAGE plpgsql STABLE;

COMMENT ON FUNCTION is_agent_online(TIMESTAMPTZ, TIMESTAMPTZ) IS
    'Returns true if agent is online (not archived and has recent heartbeat). Useful for filtering.';
