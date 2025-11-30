-- Migration: 019b_fix_representative_election_data.sql
-- Purpose: Fix representative election logic from migration 018
--
-- Migration 018 incorrectly pre-assigned representatives based on IP position.
-- The correct logic is:
--   1. All targets start with is_representative=false
--   2. Representative is elected AFTER baseline is established
--   3. First customer IP (by baseline_established_at) becomes representative
--   4. Other baseline-established customer IPs go to STANDBY state

-- Step 1: Reset all is_representative to false
UPDATE targets SET is_representative = false;

-- Step 2: Elect one representative per subnet
-- Pick the customer IP with the earliest baseline_established_at
WITH first_responders AS (
    SELECT DISTINCT ON (subnet_id) id, subnet_id
    FROM targets
    WHERE subnet_id IS NOT NULL
      AND ip_type = 'customer'
      AND baseline_established_at IS NOT NULL
      AND archived_at IS NULL
    ORDER BY subnet_id, baseline_established_at ASC, ip_address ASC
)
UPDATE targets t
SET is_representative = true
FROM first_responders fr
WHERE t.id = fr.id;

-- Step 3: Move other baseline-established customer IPs to STANDBY
-- These are valid failover candidates but not actively monitored at full rate
UPDATE targets
SET monitoring_state = 'standby'
WHERE subnet_id IS NOT NULL
  AND ip_type = 'customer'
  AND baseline_established_at IS NOT NULL
  AND is_representative = false
  AND archived_at IS NULL
  AND monitoring_state = 'active';

-- Step 4: Add index for standby targets (failover pool)
CREATE INDEX IF NOT EXISTS idx_targets_standby
ON targets(subnet_id, baseline_established_at)
WHERE monitoring_state = 'standby' AND archived_at IS NULL;

-- Step 5: Add standby_recheck tier for hourly verification
INSERT INTO tiers (name, display_name, probe_interval_ms, probe_timeout_ms, probe_retries, agent_selection)
VALUES ('standby_recheck', 'Standby Verification', 3600000, 5000, 1,
        '{"strategy": "distributed", "count": 1}')
ON CONFLICT (name) DO NOTHING;

-- Log the result
DO $$
DECLARE
    rep_count INTEGER;
    standby_count INTEGER;
    gateway_count INTEGER;
    unknown_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO rep_count
    FROM targets
    WHERE is_representative = true AND ip_type = 'customer' AND archived_at IS NULL;

    SELECT COUNT(*) INTO standby_count
    FROM targets
    WHERE monitoring_state = 'standby' AND archived_at IS NULL;

    SELECT COUNT(*) INTO gateway_count
    FROM targets
    WHERE ip_type = 'gateway' AND archived_at IS NULL;

    SELECT COUNT(*) INTO unknown_count
    FROM targets
    WHERE monitoring_state = 'unknown' AND archived_at IS NULL;

    RAISE NOTICE 'Representative election fix complete:';
    RAISE NOTICE '  - Representatives (customer): %', rep_count;
    RAISE NOTICE '  - Standby (failover pool): %', standby_count;
    RAISE NOTICE '  - Gateways (always monitored): %', gateway_count;
    RAISE NOTICE '  - Unknown (discovery pending): %', unknown_count;
END $$;
