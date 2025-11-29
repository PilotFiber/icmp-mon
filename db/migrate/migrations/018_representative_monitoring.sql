-- Migration: 018_representative_monitoring.sql
-- Purpose: Limit monitoring to representative IPs per subnet to reduce scale
--
-- This migration adds support for "representative monitoring" where instead of
-- monitoring every IP in a subnet, we monitor a configurable subset of IPs
-- (representatives) plus the gateway. This reduces:
--   - Number of targets to monitor
--   - Probe volume and database storage
--   - Agent resource consumption
--
-- Representatives are selected to provide coverage across the IP range.

-- Add is_representative flag to targets
-- Representatives are actively monitored; non-representatives can be discovered on-demand
ALTER TABLE targets
ADD COLUMN is_representative BOOLEAN NOT NULL DEFAULT true;

-- Add index for efficient representative queries
CREATE INDEX idx_targets_representative ON targets (is_representative, subnet_id)
WHERE is_representative = true AND archived_at IS NULL;

-- Add representative count configuration to subnets
-- NULL means use the system default (from config)
ALTER TABLE subnets
ADD COLUMN max_representatives INTEGER;

-- Constraint: max_representatives must be positive or NULL
ALTER TABLE subnets
ADD CONSTRAINT valid_max_representatives CHECK (max_representatives IS NULL OR max_representatives > 0);

-- Comment for documentation
COMMENT ON COLUMN targets.is_representative IS
'Whether this target is a representative IP for the subnet. Representatives are actively monitored. Non-representatives may be discovered on demand.';

COMMENT ON COLUMN subnets.max_representatives IS
'Maximum number of representative IPs to monitor for this subnet. NULL uses system default. Excludes gateway which is always monitored.';

-- Update existing auto-seeded targets to mark representatives
-- For existing data: gateway is always representative, mark only some customer IPs
-- This uses a deterministic selection based on IP position in the subnet

-- First, mark all gateways as representatives (they're special)
UPDATE targets
SET is_representative = true
WHERE ip_type = 'gateway';

-- For customer IPs in each subnet, select representatives:
-- - Up to 3 IPs per subnet (configurable default)
-- - Spread evenly across the usable range: first, middle, last
--
-- We'll mark all customer IPs as non-representative first, then select representatives
UPDATE targets
SET is_representative = false
WHERE ip_type = 'customer' AND ownership = 'auto';

-- Select representatives using window functions
-- For each subnet, pick up to 3 customer IPs spread across the range
WITH ranked_targets AS (
    SELECT
        t.id,
        t.subnet_id,
        t.ip_address,
        ROW_NUMBER() OVER (PARTITION BY t.subnet_id ORDER BY t.ip_address) as rn,
        COUNT(*) OVER (PARTITION BY t.subnet_id) as total_in_subnet
    FROM targets t
    WHERE t.subnet_id IS NOT NULL
      AND t.ip_type = 'customer'
      AND t.ownership = 'auto'
      AND t.archived_at IS NULL
),
selected_representatives AS (
    SELECT id
    FROM ranked_targets
    WHERE
        -- For subnets with <= 3 IPs, all are representatives
        total_in_subnet <= 3
        OR
        -- For larger subnets, pick first, middle, and last
        rn = 1  -- First IP
        OR rn = total_in_subnet  -- Last IP
        OR rn = (total_in_subnet + 1) / 2  -- Middle IP
)
UPDATE targets
SET is_representative = true
WHERE id IN (SELECT id FROM selected_representatives);

-- Also mark any targets that have established baselines as representatives
-- These are responding IPs that we want to continue monitoring
UPDATE targets
SET is_representative = true
WHERE baseline_established_at IS NOT NULL
  AND is_representative = false;

-- Log the result
DO $$
DECLARE
    rep_count INTEGER;
    non_rep_count INTEGER;
    gateway_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO rep_count
    FROM targets
    WHERE is_representative = true AND ip_type = 'customer' AND archived_at IS NULL;

    SELECT COUNT(*) INTO non_rep_count
    FROM targets
    WHERE is_representative = false AND archived_at IS NULL;

    SELECT COUNT(*) INTO gateway_count
    FROM targets
    WHERE ip_type = 'gateway' AND archived_at IS NULL;

    RAISE NOTICE 'Representative monitoring setup complete:';
    RAISE NOTICE '  - Customer representatives: %', rep_count;
    RAISE NOTICE '  - Non-representatives: %', non_rep_count;
    RAISE NOTICE '  - Gateways (always monitored): %', gateway_count;
END $$;
