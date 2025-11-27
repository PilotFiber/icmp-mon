-- Alert Subnet Metadata
-- Stores subnet metadata at alert creation time for historical accuracy
-- Tags can change over time, so we capture them when the alert fires
--
-- Run with: psql -d icmpmon -f 012_alert_subnet_metadata.sql

-- =============================================================================
-- ADD SUBNET METADATA COLUMNS TO ALERTS
-- =============================================================================

-- Subnet identification
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS subnet_id UUID;

-- Subscriber/Customer info
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS subscriber_name VARCHAR(255);
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS service_id INTEGER;

-- Location info
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS location_id INTEGER;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS location_address TEXT;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS city VARCHAR(255);
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS region VARCHAR(255);

-- Network info
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS pop_name VARCHAR(255);
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS gateway_device VARCHAR(255);

-- =============================================================================
-- INDEXES FOR CORRELATION QUERIES
-- These support the correlation summary feature that groups alerts by common tags
-- =============================================================================

-- Index for grouping by POP (common outage pattern)
CREATE INDEX IF NOT EXISTS idx_alerts_pop_name ON alerts(pop_name) WHERE status IN ('active', 'acknowledged');

-- Index for grouping by gateway device (common outage pattern)
CREATE INDEX IF NOT EXISTS idx_alerts_gateway_device ON alerts(gateway_device) WHERE status IN ('active', 'acknowledged');

-- Index for grouping by subscriber
CREATE INDEX IF NOT EXISTS idx_alerts_subscriber ON alerts(subscriber_name) WHERE status IN ('active', 'acknowledged');

-- Index for grouping by location
CREATE INDEX IF NOT EXISTS idx_alerts_location ON alerts(location_id) WHERE status IN ('active', 'acknowledged');

-- Index for grouping by city/region
CREATE INDEX IF NOT EXISTS idx_alerts_city ON alerts(city) WHERE status IN ('active', 'acknowledged');
CREATE INDEX IF NOT EXISTS idx_alerts_region ON alerts(region) WHERE status IN ('active', 'acknowledged');

-- Composite index for subnet correlation
CREATE INDEX IF NOT EXISTS idx_alerts_subnet_id ON alerts(subnet_id) WHERE status IN ('active', 'acknowledged');

-- =============================================================================
-- BACKFILL EXISTING ALERTS
-- Populate metadata for existing alerts by looking up current subnet data
-- Note: This captures current metadata, not historical (unavoidable for existing data)
-- =============================================================================

UPDATE alerts a
SET
    subnet_id = s.id,
    subscriber_name = s.subscriber_name,
    service_id = s.service_id,
    location_id = s.location_id,
    location_address = s.location_address,
    city = s.city,
    region = s.region,
    pop_name = s.pop_name,
    gateway_device = s.gateway_device
FROM subnets s
WHERE a.target_ip << s.network_address::inet
  AND s.state = 'active'
  AND a.subnet_id IS NULL;

-- =============================================================================
-- COMMENT ON PURPOSE
-- =============================================================================

COMMENT ON COLUMN alerts.subnet_id IS 'Subnet ID at time of alert creation (for historical reference)';
COMMENT ON COLUMN alerts.subscriber_name IS 'Subscriber name at time of alert creation';
COMMENT ON COLUMN alerts.service_id IS 'Service ID at time of alert creation';
COMMENT ON COLUMN alerts.location_id IS 'Location ID at time of alert creation';
COMMENT ON COLUMN alerts.location_address IS 'Location address at time of alert creation';
COMMENT ON COLUMN alerts.city IS 'City at time of alert creation';
COMMENT ON COLUMN alerts.region IS 'Region at time of alert creation';
COMMENT ON COLUMN alerts.pop_name IS 'POP name at time of alert creation';
COMMENT ON COLUMN alerts.gateway_device IS 'Gateway device at time of alert creation';
