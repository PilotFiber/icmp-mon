-- State Rename: degraded → down (Part 2: Data migration)
--
-- The 'degraded' state was being used for targets that were completely
-- unresponsive (had baseline, now down). This should be called 'down'.
--
-- 'degraded' should instead mean: responding but with packet loss or latency issues.
--
-- State definitions after this migration:
-- - unknown: Not probed yet
-- - active: Responding normally
-- - degraded: Responding but with packet loss/latency issues (NEW meaning)
-- - down: Had baseline, now completely unresponsive (was 'degraded')
-- - unresponsive: Never established baseline (not alertable)
-- - excluded: Down 24h+, auto-stopped monitoring (needs review)
-- - inactive: User-disabled monitoring

-- =============================================================================
-- MIGRATE 'degraded' TO 'down' IN DATABASE
-- =============================================================================

-- Update all targets with 'degraded' state to 'down'
UPDATE targets
SET monitoring_state = 'down'
WHERE monitoring_state = 'degraded';

-- Update state history records
UPDATE target_state_history
SET from_state = 'down'
WHERE from_state = 'degraded';

UPDATE target_state_history
SET to_state = 'down'
WHERE to_state = 'degraded';

-- Update any activity log entries that reference the old state
UPDATE activity_log
SET details = jsonb_set(details, '{from_state}', '"down"')
WHERE details->>'from_state' = 'degraded';

UPDATE activity_log
SET details = jsonb_set(details, '{to_state}', '"down"')
WHERE details->>'to_state' = 'degraded';

-- =============================================================================
-- LOG MIGRATION
-- =============================================================================

INSERT INTO activity_log (category, event_type, details, triggered_by, severity)
VALUES ('system', 'config_changed',
    '{"migration": "008b_state_rename_data", "description": "Migrated degraded state to down. degraded now means packet loss/latency issues, down means complete outage."}',
    'system', 'info');
