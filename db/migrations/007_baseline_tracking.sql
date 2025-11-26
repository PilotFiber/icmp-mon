-- Baseline Tracking for Alertable Outages
--
-- Distinguishes between:
-- - UNRESPONSIVE: Never responded to ICMP (not alertable - just not pingable)
-- - DOWN/DEGRADED: Was responding for a stable period, then stopped (ALERTABLE)
--
-- A target must establish a "baseline" (respond consistently for 1+ minute)
-- before outages are considered alertable. This prevents false alerts from:
-- - Targets that respond once then stop
-- - Transient responses during discovery
--
-- See state machine documentation for full flow details.

-- =============================================================================
-- ADD BASELINE TRACKING COLUMNS
-- =============================================================================

-- When this target first responded to any probe
ALTER TABLE targets ADD COLUMN first_response_at TIMESTAMPTZ;

-- When this target was confirmed stable (responding consistently for 1+ min)
-- Only targets with a baseline can transition to DEGRADED (alertable)
-- Targets without a baseline that stop responding go to UNRESPONSIVE
ALTER TABLE targets ADD COLUMN baseline_established_at TIMESTAMPTZ;

-- Index for finding targets that need baseline evaluation
CREATE INDEX idx_targets_baseline_pending ON targets(first_response_at)
    WHERE baseline_established_at IS NULL
    AND first_response_at IS NOT NULL
    AND monitoring_state = 'active';

-- =============================================================================
-- UPDATE EXISTING ACTIVE TARGETS
-- =============================================================================

-- For existing ACTIVE targets, assume they already have an established baseline
-- (they were stable enough for someone to add them as targets)
UPDATE targets SET
    first_response_at = COALESCE(last_response_at, state_changed_at),
    baseline_established_at = COALESCE(last_response_at, state_changed_at)
WHERE monitoring_state = 'active'
  AND baseline_established_at IS NULL;

-- =============================================================================
-- UPDATE targets_enriched VIEW
-- =============================================================================

DROP VIEW IF EXISTS targets_enriched;

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
    t.first_response_at,
    t.baseline_established_at,
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
-- LOG MIGRATION
-- =============================================================================

INSERT INTO activity_log (category, event_type, details, triggered_by, severity)
VALUES ('system', 'config_changed',
    '{"migration": "007_baseline_tracking", "description": "Added baseline tracking for alertable outages (first_response_at, baseline_established_at)"}',
    'system', 'info');
