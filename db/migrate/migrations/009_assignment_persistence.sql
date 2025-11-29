-- =============================================================================
-- ASSIGNMENT PERSISTENCE
-- Tracks explicit target-to-agent assignments for visibility and redistribution
-- =============================================================================

-- Persisted assignments table
CREATE TABLE target_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    target_id UUID NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tier VARCHAR(50) NOT NULL REFERENCES tiers(name) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    assigned_by VARCHAR(100) NOT NULL, -- 'initial', 'rebalancer', 'failover', 'manual'

    -- Unique constraint: each target assigned once per agent
    UNIQUE(target_id, agent_id)
);

-- Indexes for common queries
CREATE INDEX idx_target_assignments_agent ON target_assignments(agent_id);
CREATE INDEX idx_target_assignments_target ON target_assignments(target_id);
CREATE INDEX idx_target_assignments_tier ON target_assignments(tier);

-- =============================================================================
-- ASSIGNMENT HISTORY (Audit Trail)
-- =============================================================================

CREATE TABLE assignment_history (
    id BIGSERIAL,
    target_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    action VARCHAR(20) NOT NULL, -- 'assigned', 'unassigned', 'reassigned'
    reason TEXT,
    old_agent_id UUID, -- For reassignments, the previous agent
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
);

-- Make it a hypertable for efficient time-series queries
SELECT create_hypertable('assignment_history', 'created_at');

-- Indexes
CREATE INDEX idx_assignment_history_target ON assignment_history(target_id, created_at DESC);
CREATE INDEX idx_assignment_history_agent ON assignment_history(agent_id, created_at DESC);

-- Retention: keep assignment history for 30 days
SELECT add_retention_policy('assignment_history', INTERVAL '30 days');

-- =============================================================================
-- TRIGGER: Bump assignment version when assignments change
-- =============================================================================

CREATE OR REPLACE FUNCTION assignment_changed_trigger()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM increment_assignment_version();
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER target_assignments_changed
AFTER INSERT OR UPDATE OR DELETE ON target_assignments
FOR EACH STATEMENT
EXECUTE FUNCTION assignment_changed_trigger();

-- =============================================================================
-- HELPER VIEWS
-- =============================================================================

-- View: Assignments with agent and target details
CREATE OR REPLACE VIEW assignment_details AS
SELECT
    ta.id,
    ta.target_id,
    t.ip_address,
    ta.agent_id,
    a.name as agent_name,
    a.region as agent_region,
    a.provider as agent_provider,
    a.status as agent_status,
    ta.tier,
    ta.assigned_at,
    ta.assigned_by
FROM target_assignments ta
JOIN targets t ON ta.target_id = t.id
JOIN agents a ON ta.agent_id = a.id;

-- View: Assignment counts per agent
CREATE OR REPLACE VIEW agent_assignment_counts AS
SELECT
    a.id as agent_id,
    a.name as agent_name,
    a.region,
    a.status,
    a.max_targets,
    COUNT(ta.id) as assignment_count,
    a.max_targets - COUNT(ta.id) as remaining_capacity
FROM agents a
LEFT JOIN target_assignments ta ON ta.agent_id = a.id
GROUP BY a.id, a.name, a.region, a.status, a.max_targets;

-- View: Targets missing assignments (under-assigned)
CREATE OR REPLACE VIEW underassigned_targets AS
SELECT
    t.id as target_id,
    t.ip_address,
    t.tier,
    ti.agent_selection->>'strategy' as strategy,
    COALESCE((ti.agent_selection->>'count')::int,
        CASE WHEN ti.agent_selection->>'strategy' = 'all' THEN 999 ELSE 4 END
    ) as required_agents,
    COUNT(ta.id) FILTER (WHERE a.status = 'active') as active_assignments,
    COUNT(ta.id) as total_assignments
FROM targets t
JOIN tiers ti ON t.tier = ti.name
LEFT JOIN target_assignments ta ON ta.target_id = t.id
LEFT JOIN agents a ON ta.agent_id = a.id
WHERE t.archived_at IS NULL
  AND t.monitoring_state IN ('active', 'degraded', 'down', 'unknown')
GROUP BY t.id, t.ip_address, t.tier, ti.agent_selection
HAVING COUNT(ta.id) FILTER (WHERE a.status = 'active') <
    COALESCE((ti.agent_selection->>'count')::int,
        CASE WHEN ti.agent_selection->>'strategy' = 'all' THEN 999 ELSE 4 END
    );
