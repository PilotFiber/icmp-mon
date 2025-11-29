-- Migration 020: Agent API Keys
-- Adds API key authentication for agents (defense in depth with Tailscale)

-- Store hashed API keys (never store plaintext)
-- Keys are bcrypt hashed and verified on each agent API call
ALTER TABLE agents ADD COLUMN api_key_hash TEXT;
ALTER TABLE agents ADD COLUMN api_key_created_at TIMESTAMPTZ;

-- Index for efficient lookup when validating keys
-- We look up by agent_id from header, then verify the hash
CREATE INDEX idx_agents_api_key_hash ON agents(id) WHERE api_key_hash IS NOT NULL;

-- Add Tailscale IP tracking for agents
ALTER TABLE agents ADD COLUMN tailscale_ip INET;

COMMENT ON COLUMN agents.api_key_hash IS 'Bcrypt hash of the agent API key';
COMMENT ON COLUMN agents.api_key_created_at IS 'When the API key was generated';
COMMENT ON COLUMN agents.tailscale_ip IS 'Tailscale IP assigned to the agent';
