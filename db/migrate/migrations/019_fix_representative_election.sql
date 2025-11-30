-- Migration: 019_fix_representative_election.sql (Part 1: Add enum value)
--
-- PostgreSQL requires enum ADD VALUE to be in its own transaction
-- before the new value can be used. See 019b for data migration.

ALTER TYPE monitoring_state ADD VALUE IF NOT EXISTS 'standby';
