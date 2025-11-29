-- Migration 022: Reduce compression interval from 1 day to 3 hours
--
-- Probe results and agent metrics are write-once data that doesn't need
-- to remain uncompressed for long. 3 hours gives enough buffer for:
-- - Continuous aggregates (probe_5min) to process
-- - Dashboard queries on recent raw data
-- - Any debugging/investigation needs
--
-- This significantly reduces storage usage for high-volume probe data.

-- Remove existing compression policies
SELECT remove_compression_policy('probe_results', if_exists => true);
SELECT remove_compression_policy('agent_metrics', if_exists => true);

-- Add new 3-hour compression policies
SELECT add_compression_policy('probe_results', INTERVAL '3 hours');
SELECT add_compression_policy('agent_metrics', INTERVAL '3 hours');
