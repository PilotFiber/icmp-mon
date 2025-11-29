-- Pilot IP Pool Monitoring
-- Adds subnet support, monitoring state machine, and enriched metadata
--
-- See docs/PILOT_IP_MONITORING_DESIGN.md for full design details

-- =============================================================================
-- ENUM TYPES
-- =============================================================================

CREATE TYPE monitoring_state AS ENUM (
    'unknown',      -- Newly assigned IP, never probed
    'active',       -- Responds to ICMP, full monitoring
    'unresponsive', -- Never responded to discovery probes (no review needed)
    'inactive',     -- User-confirmed intentionally unreachable
    'degraded',     -- Was active, stopped responding (alerts fire)
    'excluded'      -- Was active, unreachable for 24h (needs review)
);

CREATE TYPE ip_type AS ENUM (
    'gateway',          -- Pilot-owned gateways (deprioritize ICMP)
    'infrastructure',   -- Pilot servers and network devices
    'customer'          -- Customer-facing addresses
);

CREATE TYPE ownership_type AS ENUM (
    'auto',    -- Follows subnet lifecycle, can be auto-archived
    'manual'   -- User explicitly wants this, never auto-archived
);

CREATE TYPE origin_type AS ENUM (
    'sync',       -- Created during Pilot API sync
    'discovery',  -- Found during probe sweep
    'user'        -- Manually created by user
);

-- =============================================================================
-- SUBNETS TABLE
-- =============================================================================

CREATE TABLE subnets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Pilot API fields
    pilot_subnet_id INTEGER UNIQUE,
    network_address INET NOT NULL,
    network_size INTEGER NOT NULL,
    gateway_address INET,
    first_usable_address INET,
    last_usable_address INET,

    -- Enriched metadata (from Pilot relationships)
    vlan_id INTEGER,
    service_id INTEGER,
    subscriber_id INTEGER,
    subscriber_name TEXT,

    -- Location metadata
    location_id INTEGER,
    location_address TEXT,
    city TEXT,
    region TEXT,
    pop_name TEXT,

    -- Network topology (CSW today, future-proof name)
    gateway_device TEXT,

    -- Lifecycle
    state TEXT NOT NULL DEFAULT 'active',  -- active | archived
    archived_at TIMESTAMPTZ,
    archive_reason TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Ensure network_address is a valid network (not host)
    CONSTRAINT valid_network CHECK (network_address = network(network_address))
);

CREATE INDEX idx_subnets_network ON subnets USING gist (network_address inet_ops);
CREATE INDEX idx_subnets_pilot_id ON subnets(pilot_subnet_id) WHERE pilot_subnet_id IS NOT NULL;
CREATE INDEX idx_subnets_subscriber ON subnets(subscriber_id) WHERE subscriber_id IS NOT NULL;
CREATE INDEX idx_subnets_service ON subnets(service_id) WHERE service_id IS NOT NULL;
CREATE INDEX idx_subnets_location ON subnets(location_id) WHERE location_id IS NOT NULL;
CREATE INDEX idx_subnets_pop ON subnets(pop_name) WHERE pop_name IS NOT NULL;
CREATE INDEX idx_subnets_active ON subnets(state) WHERE state = 'active';

-- =============================================================================
-- ALTER TARGETS TABLE
-- =============================================================================

-- Add subnet relationship
ALTER TABLE targets ADD COLUMN subnet_id UUID REFERENCES subnets(id);

-- Add ownership and origin tracking
ALTER TABLE targets ADD COLUMN ownership ownership_type NOT NULL DEFAULT 'manual';
ALTER TABLE targets ADD COLUMN origin origin_type;

-- Add IP classification
ALTER TABLE targets ADD COLUMN ip_type ip_type;

-- Add monitoring state machine fields
ALTER TABLE targets ADD COLUMN monitoring_state monitoring_state NOT NULL DEFAULT 'unknown';
ALTER TABLE targets ADD COLUMN state_changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE targets ADD COLUMN needs_review BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE targets ADD COLUMN discovery_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE targets ADD COLUMN last_response_at TIMESTAMPTZ;

-- Add archive fields (archived is NOT a monitoring state)
ALTER TABLE targets ADD COLUMN archived_at TIMESTAMPTZ;
ALTER TABLE targets ADD COLUMN archive_reason TEXT;

-- New indexes
CREATE INDEX idx_targets_subnet ON targets(subnet_id) WHERE subnet_id IS NOT NULL;
CREATE INDEX idx_targets_monitoring_state ON targets(monitoring_state);
CREATE INDEX idx_targets_needs_review ON targets(needs_review) WHERE needs_review = true;
CREATE INDEX idx_targets_active ON targets(archived_at) WHERE archived_at IS NULL;
CREATE INDEX idx_targets_ip_type ON targets(ip_type) WHERE ip_type IS NOT NULL;

-- =============================================================================
-- TARGET STATE HISTORY
-- =============================================================================

CREATE TABLE target_state_history (
    id BIGSERIAL PRIMARY KEY,
    target_id UUID NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    from_state monitoring_state,
    to_state monitoring_state NOT NULL,
    reason TEXT,
    triggered_by TEXT NOT NULL,  -- 'system', 'discovery', 'user:email@pilot.com'
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_state_history_target ON target_state_history(target_id);
CREATE INDEX idx_state_history_time ON target_state_history(created_at DESC);

-- =============================================================================
-- ACTIVITY LOG (Unified audit trail)
-- =============================================================================

CREATE TABLE activity_log (
    id BIGSERIAL PRIMARY KEY,

    -- What was affected (at least one should be set)
    target_id UUID REFERENCES targets(id) ON DELETE SET NULL,
    subnet_id UUID REFERENCES subnets(id) ON DELETE SET NULL,
    agent_id UUID,
    ip INET,  -- Denormalized for searching deleted targets

    -- Event classification
    category TEXT NOT NULL,
    -- 'target' | 'subnet' | 'agent' | 'sync' | 'user' | 'system'

    event_type TEXT NOT NULL,
    -- Target: 'state_change', 'discovered', 'created', 'archived', 'ownership_changed'
    -- Subnet: 'sync_created', 'sync_updated', 'sync_removed', 'archived'
    -- Agent: 'registered', 'heartbeat_missed', 'came_online', 'went_offline'
    -- Sync: 'sync_started', 'sync_completed', 'sync_failed'
    -- User: 'manual_create', 'manual_update', 'acknowledged', 'marked_inactive', 'pinned'
    -- System: 'retention_cleanup', 'config_changed'

    -- Event details (flexible JSON)
    details JSONB,

    -- Who/what triggered this
    triggered_by TEXT NOT NULL,
    -- 'system', 'sync', 'discovery', 'agent:agent-id', 'user:email@pilot.com'

    -- Severity for filtering
    severity TEXT NOT NULL DEFAULT 'info',  -- 'debug' | 'info' | 'warning' | 'error'

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Convert to hypertable for efficient time-series queries
SELECT create_hypertable('activity_log', 'created_at');

-- Indexes for common queries
CREATE INDEX idx_activity_target ON activity_log(target_id) WHERE target_id IS NOT NULL;
CREATE INDEX idx_activity_subnet ON activity_log(subnet_id) WHERE subnet_id IS NOT NULL;
CREATE INDEX idx_activity_agent ON activity_log(agent_id) WHERE agent_id IS NOT NULL;
CREATE INDEX idx_activity_ip ON activity_log(ip) WHERE ip IS NOT NULL;
CREATE INDEX idx_activity_category ON activity_log(category, event_type);
CREATE INDEX idx_activity_severity ON activity_log(severity) WHERE severity IN ('warning', 'error');

-- Retention: keep activity logs for 1 year
SELECT add_retention_policy('activity_log', INTERVAL '1 year');

-- =============================================================================
-- STATE-BASED TIERS
-- =============================================================================

-- Add tiers for different monitoring states
INSERT INTO tiers (name, display_name, probe_interval_ms, probe_timeout_ms, probe_retries, agent_selection) VALUES
    ('discovery', 'Discovery (New IPs)', 300000, 5000, 0, '{"strategy": "distributed", "count": 1}'),
    ('inactive_recheck', 'Inactive Re-check', 3600000, 5000, 0, '{"strategy": "distributed", "count": 1}'),
    ('smart_recheck', 'Smart Re-check (No Coverage)', 86400000, 5000, 0, '{"strategy": "distributed", "count": 1}')
ON CONFLICT (name) DO NOTHING;

-- =============================================================================
-- ENRICHED TARGETS VIEW
-- =============================================================================

CREATE VIEW targets_enriched AS
SELECT
    t.id,
    t.ip_address AS ip,
    t.display_name,
    t.tags,
    t.ip_type,
    t.tier,
    t.monitoring_state,
    t.state_changed_at,
    t.needs_review,
    t.last_response_at,
    t.ownership,
    t.origin,
    t.discovery_attempts,
    t.expected_outcome,
    t.notes,
    t.created_at,
    t.updated_at,
    -- Subnet metadata (denormalized via JOIN)
    s.id AS subnet_id,
    s.network_address,
    s.network_size,
    s.pilot_subnet_id,
    s.service_id,
    s.subscriber_id,
    s.subscriber_name,
    s.location_id,
    s.location_address,
    s.city,
    s.region,
    s.pop_name,
    s.gateway_device,
    s.gateway_address
FROM targets t
LEFT JOIN subnets s ON t.subnet_id = s.id
WHERE t.archived_at IS NULL;

-- =============================================================================
-- HELPER FUNCTIONS
-- =============================================================================

-- Function to check if a subnet has any active customer targets
CREATE OR REPLACE FUNCTION subnet_has_active_coverage(p_subnet_id UUID)
RETURNS BOOLEAN AS $$
BEGIN
    RETURN EXISTS (
        SELECT 1 FROM targets
        WHERE subnet_id = p_subnet_id
          AND archived_at IS NULL
          AND monitoring_state = 'active'
          AND ip_type = 'customer'
    );
END;
$$ LANGUAGE plpgsql;

-- Function to transition target state with history logging
CREATE OR REPLACE FUNCTION transition_target_state(
    p_target_id UUID,
    p_new_state monitoring_state,
    p_reason TEXT,
    p_triggered_by TEXT
) RETURNS VOID AS $$
DECLARE
    v_old_state monitoring_state;
BEGIN
    -- Get current state
    SELECT monitoring_state INTO v_old_state
    FROM targets WHERE id = p_target_id;

    -- Skip if no change
    IF v_old_state = p_new_state THEN
        RETURN;
    END IF;

    -- Update target
    UPDATE targets SET
        monitoring_state = p_new_state,
        state_changed_at = NOW(),
        needs_review = CASE
            WHEN p_new_state = 'excluded' THEN TRUE
            ELSE FALSE
        END,
        updated_at = NOW()
    WHERE id = p_target_id;

    -- Record history
    INSERT INTO target_state_history (target_id, from_state, to_state, reason, triggered_by)
    VALUES (p_target_id, v_old_state, p_new_state, p_reason, p_triggered_by);
END;
$$ LANGUAGE plpgsql;

-- Update assignment version trigger to also fire on monitoring_state changes
-- (existing trigger handles INSERT/UPDATE/DELETE, but we want state changes to trigger reassignment)

-- =============================================================================
-- MIGRATION OF EXISTING DATA
-- =============================================================================

-- Set existing targets to 'active' state (they were already being monitored)
UPDATE targets SET
    monitoring_state = 'active',
    state_changed_at = created_at,
    ownership = 'manual',
    origin = 'user'
WHERE monitoring_state = 'unknown';

-- Log migration
INSERT INTO activity_log (category, event_type, details, triggered_by, severity)
VALUES ('system', 'config_changed',
    '{"migration": "006_pilot_monitoring", "description": "Added subnet support and monitoring state machine"}',
    'system', 'info');
