-- Migration: Service Status Integration
-- Adds service_status tracking to subnets for Flight Deck service lifecycle management

-- Add service_status to subnets table
ALTER TABLE subnets ADD COLUMN IF NOT EXISTS service_status TEXT;
ALTER TABLE subnets ADD COLUMN IF NOT EXISTS service_status_changed_at TIMESTAMPTZ;

-- Index for filtering by service status (partial index for efficiency)
CREATE INDEX IF NOT EXISTS idx_subnets_service_status
ON subnets(service_status)
WHERE service_status IS NOT NULL;

-- Index for finding cancelled services quickly
CREATE INDEX IF NOT EXISTS idx_subnets_service_cancelled
ON subnets(service_id)
WHERE service_status = 'cancelled';

COMMENT ON COLUMN subnets.service_status IS 'Service status from Flight Deck (e.g., active, cancelled)';
COMMENT ON COLUMN subnets.service_status_changed_at IS 'When the service status last changed';
