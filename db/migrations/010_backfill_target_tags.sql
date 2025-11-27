-- Backfill target tags from subnet metadata
-- This migration populates tags on all targets that belong to subnets,
-- using the same logic as buildTargetTags() in pilot_sync.go
--
-- Tags enable filtering in the metrics explorer by:
-- - address: Location address
-- - location_id: Pilot location ID
-- - city: City name
-- - region: Region name
-- - subscriber: Subscriber name
-- - subscriber_id: Pilot subscriber ID
-- - service_id: Pilot service ID
-- - pop: POP name
-- - csw: Gateway device (CSW name)
-- - vlan_id: VLAN ID
-- - subnet: Network address
-- - auto_seeded: Indicates auto-created target
-- - pilot_sync: Indicates synced from Pilot

-- Build tags JSON for a target based on its subnet's metadata
-- Using a CTE to construct the tags JSON object
UPDATE targets t
SET tags = (
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'auto_seeded', 'true',
        'pilot_sync', 'true',
        'subnet', s.network_address::text,
        'address', NULLIF(s.location_address, ''),
        'location_id', CASE WHEN s.location_id IS NOT NULL THEN s.location_id::text END,
        'city', NULLIF(s.city, ''),
        'region', NULLIF(s.region, ''),
        'subscriber', NULLIF(s.subscriber_name, ''),
        'subscriber_id', CASE WHEN s.subscriber_id IS NOT NULL THEN s.subscriber_id::text END,
        'service_id', CASE WHEN s.service_id IS NOT NULL THEN s.service_id::text END,
        'pop', NULLIF(s.pop_name, ''),
        'csw', NULLIF(s.gateway_device, ''),
        'vlan_id', CASE WHEN s.vlan_id IS NOT NULL THEN s.vlan_id::text END
    ))
    FROM subnets s
    WHERE s.id = t.subnet_id
),
updated_at = NOW()
WHERE t.subnet_id IS NOT NULL
  AND t.archived_at IS NULL;

-- Log count of updated targets
DO $$
DECLARE
    updated_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO updated_count
    FROM targets
    WHERE subnet_id IS NOT NULL
      AND archived_at IS NULL
      AND tags != '{}'::jsonb;

    RAISE NOTICE 'Backfilled tags for % targets with subnet metadata', updated_count;
END $$;
