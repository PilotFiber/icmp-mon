-- Agent Enrollment and Fleet Management Schema
-- Implements one-click agent enrollment, SSH key management, and rolling updates
--
-- Run with: psql -d icmpmon -f 005_agent_enrollment.sql

-- =============================================================================
-- ENUM TYPES
-- =============================================================================

CREATE TYPE enrollment_state AS ENUM (
    'pending',
    'connecting',
    'detecting',
    'key_installing',
    'hardening',
    'agent_installing',
    'starting',
    'registering',
    'complete',
    'failed',
    'cancelled'
);

CREATE TYPE ssh_key_status AS ENUM ('active', 'rotating', 'revoked');

CREATE TYPE release_status AS ENUM ('draft', 'published', 'deprecated');

CREATE TYPE rollout_strategy AS ENUM ('immediate', 'canary', 'staged', 'manual');

CREATE TYPE rollout_status AS ENUM ('pending', 'in_progress', 'paused', 'completed', 'failed', 'rolled_back');

CREATE TYPE update_status AS ENUM ('pending', 'downloading', 'installing', 'restarting', 'completed', 'failed', 'rolled_back');

-- =============================================================================
-- SSH KEYS
-- Metadata only - private key stored in 1Password
-- =============================================================================

CREATE TABLE ssh_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL UNIQUE,
    key_type VARCHAR(20) NOT NULL DEFAULT 'ed25519',
    public_key TEXT NOT NULL,
    fingerprint VARCHAR(64) NOT NULL UNIQUE,

    -- 1Password reference (NULL if key not stored in 1Password)
    onepassword_item_id VARCHAR(100),
    onepassword_vault_id VARCHAR(100),

    status ssh_key_status NOT NULL DEFAULT 'active',

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rotated_at TIMESTAMPTZ
);

CREATE INDEX idx_ssh_keys_status ON ssh_keys(status);
CREATE INDEX idx_ssh_keys_fingerprint ON ssh_keys(fingerprint);

-- =============================================================================
-- ENROLLMENTS
-- Track enrollment sessions with state machine
-- =============================================================================

CREATE TABLE enrollments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Target server info
    target_ip INET NOT NULL,
    target_port INTEGER NOT NULL DEFAULT 22,
    ssh_username VARCHAR(100) NOT NULL,
    -- Note: password is NEVER stored, only used during enrollment

    -- Desired agent configuration
    agent_name VARCHAR(255),
    region VARCHAR(50),
    location VARCHAR(255),
    provider VARCHAR(50),
    tags JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- State machine
    state enrollment_state NOT NULL DEFAULT 'pending',
    current_step VARCHAR(50),
    steps_completed TEXT[] NOT NULL DEFAULT '{}',

    -- Detection results (populated during enrollment)
    detected_os VARCHAR(50),
    detected_os_version VARCHAR(50),
    detected_arch VARCHAR(20),
    detected_hostname VARCHAR(255),
    detected_package_manager VARCHAR(20),  -- apt, dnf, yum, etc.

    -- Result
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    ssh_key_id UUID REFERENCES ssh_keys(id) ON DELETE SET NULL,

    -- Rollback tracking (what changes were made, for cleanup on failure)
    changes JSONB NOT NULL DEFAULT '[]'::jsonb,

    -- Error handling
    last_error TEXT,
    error_details JSONB,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,

    -- Timing
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    -- Audit
    requested_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_enrollments_state ON enrollments(state);
CREATE INDEX idx_enrollments_target_ip ON enrollments(target_ip);
CREATE INDEX idx_enrollments_agent ON enrollments(agent_id) WHERE agent_id IS NOT NULL;
CREATE INDEX idx_enrollments_created ON enrollments(created_at DESC);

-- =============================================================================
-- ENROLLMENT LOGS
-- Detailed logs for each enrollment step
-- =============================================================================

CREATE TABLE enrollment_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    enrollment_id UUID NOT NULL REFERENCES enrollments(id) ON DELETE CASCADE,

    step VARCHAR(50) NOT NULL,
    level VARCHAR(10) NOT NULL DEFAULT 'info',  -- debug, info, warn, error
    message TEXT NOT NULL,
    details JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_enrollment_logs_enrollment ON enrollment_logs(enrollment_id, created_at);
CREATE INDEX idx_enrollment_logs_level ON enrollment_logs(level) WHERE level IN ('warn', 'error');

-- =============================================================================
-- AGENT RELEASES
-- Track agent binary releases
-- =============================================================================

CREATE TABLE agent_releases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version VARCHAR(50) NOT NULL UNIQUE,

    -- Release info
    release_notes TEXT,
    changelog TEXT,

    -- Binary artifacts per platform
    -- Format: [{"platform": "linux-amd64", "url": "...", "checksum": "sha256:...", "size": 12345678}, ...]
    artifacts JSONB NOT NULL DEFAULT '[]'::jsonb,

    -- Supported platforms derived from artifacts
    platforms TEXT[] NOT NULL DEFAULT '{}',

    status release_status NOT NULL DEFAULT 'draft',

    -- Minimum compatible version (for rollback limits)
    min_compatible_version VARCHAR(50),

    published_at TIMESTAMPTZ,
    published_by VARCHAR(255),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255)
);

CREATE INDEX idx_agent_releases_status ON agent_releases(status);
CREATE INDEX idx_agent_releases_version ON agent_releases(version);
CREATE INDEX idx_agent_releases_published ON agent_releases(published_at DESC) WHERE status = 'published';

-- =============================================================================
-- ROLLOUTS
-- Track fleet-wide update campaigns
-- =============================================================================

CREATE TABLE rollouts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    release_id UUID NOT NULL REFERENCES agent_releases(id) ON DELETE RESTRICT,

    -- Strategy configuration
    strategy rollout_strategy NOT NULL,
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Example config for canary:
    -- {
    --   "canary_percent": 5,
    --   "canary_duration_minutes": 10,
    --   "waves": [10, 25, 50, 100],
    --   "wave_delay_minutes": 5,
    --   "health_check_wait_minutes": 2,
    --   "failure_threshold_percent": 10,
    --   "auto_rollback": true
    -- }

    -- Current state
    status rollout_status NOT NULL DEFAULT 'pending',
    current_wave INTEGER,

    -- Progress tracking
    agents_total INTEGER NOT NULL DEFAULT 0,
    agents_pending INTEGER NOT NULL DEFAULT 0,
    agents_updating INTEGER NOT NULL DEFAULT 0,
    agents_updated INTEGER NOT NULL DEFAULT 0,
    agents_failed INTEGER NOT NULL DEFAULT 0,
    agents_skipped INTEGER NOT NULL DEFAULT 0,

    -- Agent selection (NULL = all agents)
    agent_filter JSONB,  -- {"regions": ["us-west", "us-east"], "tags": {...}}
    selected_agent_ids UUID[],

    -- Timing
    started_at TIMESTAMPTZ,
    paused_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    -- Rollback info
    rolled_back_at TIMESTAMPTZ,
    rollback_reason TEXT,

    -- Audit
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_rollouts_status ON rollouts(status);
CREATE INDEX idx_rollouts_release ON rollouts(release_id);
CREATE INDEX idx_rollouts_active ON rollouts(status) WHERE status IN ('pending', 'in_progress', 'paused');
CREATE INDEX idx_rollouts_created ON rollouts(created_at DESC);

-- =============================================================================
-- ROLLOUT WAVES
-- Track individual waves within a staged rollout
-- =============================================================================

CREATE TABLE rollout_waves (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rollout_id UUID NOT NULL REFERENCES rollouts(id) ON DELETE CASCADE,

    wave_number INTEGER NOT NULL,
    target_percent INTEGER NOT NULL,  -- e.g., 10, 25, 50, 100

    status VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending, in_progress, completed, failed

    agents_in_wave INTEGER NOT NULL DEFAULT 0,
    agents_updated INTEGER NOT NULL DEFAULT 0,
    agents_failed INTEGER NOT NULL DEFAULT 0,

    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    health_check_passed BOOLEAN,
    health_check_at TIMESTAMPTZ,

    UNIQUE(rollout_id, wave_number)
);

CREATE INDEX idx_rollout_waves_rollout ON rollout_waves(rollout_id, wave_number);

-- =============================================================================
-- AGENT UPDATE HISTORY
-- Track individual agent updates
-- =============================================================================

CREATE TABLE agent_update_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    rollout_id UUID REFERENCES rollouts(id) ON DELETE SET NULL,
    wave_id UUID REFERENCES rollout_waves(id) ON DELETE SET NULL,

    from_version VARCHAR(50),
    to_version VARCHAR(50) NOT NULL,

    status update_status NOT NULL DEFAULT 'pending',

    -- Progress tracking
    download_started_at TIMESTAMPTZ,
    download_completed_at TIMESTAMPTZ,
    install_started_at TIMESTAMPTZ,
    install_completed_at TIMESTAMPTZ,

    -- Verification
    checksum_verified BOOLEAN,
    post_update_healthy BOOLEAN,

    -- Error info
    error_message TEXT,
    error_details JSONB,

    -- Timing
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_agent_update_history_agent ON agent_update_history(agent_id, started_at DESC);
CREATE INDEX idx_agent_update_history_rollout ON agent_update_history(rollout_id) WHERE rollout_id IS NOT NULL;
CREATE INDEX idx_agent_update_history_status ON agent_update_history(status) WHERE status NOT IN ('completed', 'failed');

-- =============================================================================
-- EXTEND AGENTS TABLE
-- Add enrollment and update tracking fields
-- =============================================================================

ALTER TABLE agents ADD COLUMN IF NOT EXISTS platform VARCHAR(50);
ALTER TABLE agents ADD COLUMN IF NOT EXISTS arch VARCHAR(20);
ALTER TABLE agents ADD COLUMN IF NOT EXISTS os_version VARCHAR(50);

ALTER TABLE agents ADD COLUMN IF NOT EXISTS enrolled_at TIMESTAMPTZ;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS enrolled_by VARCHAR(255);
ALTER TABLE agents ADD COLUMN IF NOT EXISTS enrollment_id UUID REFERENCES enrollments(id) ON DELETE SET NULL;

ALTER TABLE agents ADD COLUMN IF NOT EXISTS ssh_key_id UUID REFERENCES ssh_keys(id) ON DELETE SET NULL;

ALTER TABLE agents ADD COLUMN IF NOT EXISTS update_channel VARCHAR(50) DEFAULT 'stable';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS auto_update BOOLEAN DEFAULT true;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS last_update_check TIMESTAMPTZ;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS pending_update_id UUID REFERENCES agent_releases(id) ON DELETE SET NULL;

-- Index for finding agents pending updates
CREATE INDEX IF NOT EXISTS idx_agents_pending_update ON agents(pending_update_id) WHERE pending_update_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_agents_platform ON agents(platform) WHERE platform IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_agents_update_channel ON agents(update_channel);

-- =============================================================================
-- HELPER FUNCTIONS
-- =============================================================================

-- Get fleet version distribution
CREATE OR REPLACE FUNCTION get_fleet_version_distribution()
RETURNS TABLE(
    version VARCHAR(50),
    agent_count BIGINT,
    percentage NUMERIC(5,2)
) AS $$
BEGIN
    RETURN QUERY
    WITH totals AS (
        SELECT COUNT(*) AS total FROM agents WHERE status != 'offline'
    )
    SELECT
        COALESCE(a.version, 'unknown') AS version,
        COUNT(*) AS agent_count,
        ROUND((COUNT(*)::NUMERIC / NULLIF(t.total, 0) * 100), 2) AS percentage
    FROM agents a, totals t
    WHERE a.status != 'offline'
    GROUP BY a.version, t.total
    ORDER BY agent_count DESC;
END;
$$ LANGUAGE plpgsql;

-- Get enrollment summary
CREATE OR REPLACE FUNCTION get_enrollment_summary()
RETURNS TABLE(
    state enrollment_state,
    count BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT e.state, COUNT(*) as count
    FROM enrollments e
    WHERE e.created_at > NOW() - INTERVAL '24 hours'
    GROUP BY e.state
    ORDER BY count DESC;
END;
$$ LANGUAGE plpgsql;

-- Get rollout progress
CREATE OR REPLACE FUNCTION get_rollout_progress(p_rollout_id UUID)
RETURNS TABLE(
    status rollout_status,
    strategy rollout_strategy,
    current_wave INTEGER,
    agents_total INTEGER,
    agents_updated INTEGER,
    agents_failed INTEGER,
    progress_percent NUMERIC(5,2),
    estimated_completion TIMESTAMPTZ
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        r.status,
        r.strategy,
        r.current_wave,
        r.agents_total,
        r.agents_updated,
        r.agents_failed,
        ROUND((r.agents_updated::NUMERIC / NULLIF(r.agents_total, 0) * 100), 2) AS progress_percent,
        CASE
            WHEN r.agents_updated = 0 THEN NULL
            WHEN r.status = 'completed' THEN r.completed_at
            ELSE r.started_at + (
                (NOW() - r.started_at) * (r.agents_total::FLOAT / NULLIF(r.agents_updated, 0))
            )::INTERVAL
        END AS estimated_completion
    FROM rollouts r
    WHERE r.id = p_rollout_id;
END;
$$ LANGUAGE plpgsql;

-- Check if agent can be updated
CREATE OR REPLACE FUNCTION can_agent_update(p_agent_id UUID, p_release_id UUID)
RETURNS TABLE(
    can_update BOOLEAN,
    reason TEXT
) AS $$
DECLARE
    v_agent agents%ROWTYPE;
    v_release agent_releases%ROWTYPE;
    v_pending_updates INTEGER;
BEGIN
    SELECT * INTO v_agent FROM agents WHERE id = p_agent_id;
    SELECT * INTO v_release FROM agent_releases WHERE id = p_release_id;

    IF v_agent IS NULL THEN
        RETURN QUERY SELECT FALSE, 'Agent not found';
        RETURN;
    END IF;

    IF v_release IS NULL THEN
        RETURN QUERY SELECT FALSE, 'Release not found';
        RETURN;
    END IF;

    IF v_release.status != 'published' THEN
        RETURN QUERY SELECT FALSE, 'Release is not published';
        RETURN;
    END IF;

    IF v_agent.version = v_release.version THEN
        RETURN QUERY SELECT FALSE, 'Agent already on this version';
        RETURN;
    END IF;

    IF v_agent.status = 'offline' THEN
        RETURN QUERY SELECT FALSE, 'Agent is offline';
        RETURN;
    END IF;

    -- Check for pending updates
    SELECT COUNT(*) INTO v_pending_updates
    FROM agent_update_history
    WHERE agent_id = p_agent_id
      AND status NOT IN ('completed', 'failed', 'rolled_back');

    IF v_pending_updates > 0 THEN
        RETURN QUERY SELECT FALSE, 'Agent has pending update';
        RETURN;
    END IF;

    -- Check platform compatibility
    IF v_agent.platform IS NOT NULL AND NOT (v_agent.platform = ANY(v_release.platforms)) THEN
        RETURN QUERY SELECT FALSE, format('Platform %s not supported by release', v_agent.platform);
        RETURN;
    END IF;

    RETURN QUERY SELECT TRUE, NULL::TEXT;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- TRIGGERS
-- =============================================================================

-- Update timestamps
CREATE OR REPLACE FUNCTION update_enrollment_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER enrollments_updated
BEFORE UPDATE ON enrollments
FOR EACH ROW
EXECUTE FUNCTION update_enrollment_timestamp();

CREATE TRIGGER rollouts_updated
BEFORE UPDATE ON rollouts
FOR EACH ROW
EXECUTE FUNCTION update_enrollment_timestamp();

-- Log enrollment state changes
CREATE OR REPLACE FUNCTION log_enrollment_state_change()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.state IS DISTINCT FROM NEW.state THEN
        INSERT INTO enrollment_logs (enrollment_id, step, level, message, details)
        VALUES (
            NEW.id,
            'state_change',
            'info',
            format('State changed from %s to %s', OLD.state, NEW.state),
            jsonb_build_object('old_state', OLD.state, 'new_state', NEW.state)
        );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER enrollment_state_changed
AFTER UPDATE OF state ON enrollments
FOR EACH ROW
EXECUTE FUNCTION log_enrollment_state_change();
