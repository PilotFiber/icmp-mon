# Agent Enrollment and Fleet Management System

## Overview

One-click agent enrollment from the UI with automatic SSH key installation, security hardening, and self-updating agents.

**User Input Required**: IP address, SSH username (with sudo), password
**Automatic**: OS detection, agent installation, SSH hardening, updates

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                              UI                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  Enroll Agent Modal                                          │   │
│  │  [IP] [Username] [Password]  ───────▶  Progress Stream (SSE) │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         CONTROL PLANE                                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐  │
│  │  Enrollment  │  │   SSH Key    │  │   Rollout Engine         │  │
│  │  Service     │  │   Manager    │  │   (Canary/Staged/Manual) │  │
│  └──────────────┘  └──────────────┘  └──────────────────────────┘  │
│         │                 │                      │                   │
│         └─────────────────┴──────────────────────┘                   │
│                           │                                          │
│                  ┌────────────────┐                                  │
│                  │  PostgreSQL    │                                  │
│                  │  (Encrypted    │                                  │
│                  │   SSH Keys)    │                                  │
│                  └────────────────┘                                  │
└─────────────────────────────────────────────────────────────────────┘
                                │
                          SSH (one-time)
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         TARGET SERVER                                │
│  1. Connect with password (used once, never stored)                 │
│  2. Install control plane's SSH public key                          │
│  3. Harden SSH (disable password auth)                              │
│  4. Detect OS/arch, install agent binary                            │
│  5. Configure and start agent service                               │
│  6. Agent self-registers, receives assignments                      │
│  7. Agent self-updates via heartbeat signals                        │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Component Design

### 1. SSH Key Management (1Password)

**Single control-plane key pair** stored in 1Password via Connect API:
- Generated on first enrollment attempt (if not exists)
- Ed25519 key (modern, secure, small)
- Private key stored in 1Password vault
- Public key cached locally (not sensitive)
- Key rotation supported via 1Password

**1Password Connect Setup:**
- Deploy 1Password Connect Server (container or service)
- Create a vault for ICMP-Mon secrets
- Generate Connect credentials (token)
- Control plane uses `OP_CONNECT_HOST` and `OP_CONNECT_TOKEN` env vars

```go
// control-plane/internal/secrets/onepassword.go
type OnePasswordKeyStore struct {
    client    *connect.Client
    vaultUUID string
}

func (ks *OnePasswordKeyStore) GetOrCreateProvisioningKey(ctx context.Context) (*SSHKeyPair, error)
func (ks *OnePasswordKeyStore) RotateKey(ctx context.Context) error
func (ks *OnePasswordKeyStore) GetPrivateKey(ctx context.Context) ([]byte, error)
```

**1Password Item Structure:**
```
Item: "icmpmon-ssh-key"
Fields:
  - public_key: "ssh-ed25519 AAAA..."
  - private_key: "-----BEGIN OPENSSH PRIVATE KEY-----..."
  - fingerprint: "SHA256:..."
  - created_at: "2024-01-01T00:00:00Z"
```

### 2. Enrollment Service

**State machine with recovery** - enrollment can resume from any step:

```
States: pending → connecting → detecting → key_installing →
        hardening → agent_installing → starting → registering → complete

        Any state → failed (with rollback)
```

Each step is **idempotent** - safe to retry. Changes tracked for rollback.

```go
// control-plane/internal/enrollment/service.go
type EnrollmentService struct {
    store    *store.Store
    keyStore *secrets.KeyStore
    logger   *slog.Logger
}

func (s *EnrollmentService) Enroll(ctx context.Context, req EnrollRequest) (<-chan EnrollmentEvent, error)
func (s *EnrollmentService) Retry(ctx context.Context, enrollmentID string) error
func (s *EnrollmentService) Cancel(ctx context.Context, enrollmentID string) error
```

### 3. SSH Provisioner

Go SSH client for remote execution:

```go
// control-plane/internal/enrollment/ssh.go
type SSHProvisioner struct {
    keyStore *secrets.KeyStore
}

func (p *SSHProvisioner) ConnectWithPassword(ctx context.Context, host, user, password string) (*SSHSession, error)
func (p *SSHProvisioner) ConnectWithKey(ctx context.Context, host, user string) (*SSHSession, error)
func (p *SSHProvisioner) Execute(ctx context.Context, session *SSHSession, cmd string) (string, error)
func (p *SSHProvisioner) Transfer(ctx context.Context, session *SSHSession, content []byte, remotePath string) error
```

### 4. Agent Self-Updater

Agents update themselves via heartbeat signals (no SSH needed for updates):

```go
// agent/internal/updater/updater.go
type Updater struct {
    client      *client.Client
    installPath string
}

// Called when heartbeat response includes update_available
func (u *Updater) Update(ctx context.Context, update UpdateInfo) error {
    // 1. Download new binary to temp
    // 2. Verify SHA256 checksum
    // 3. Atomic symlink swap (zero downtime)
    // 4. Request systemd restart
}
```

### 5. Rollout Engine (Full: Canary + Staged)

Staged updates with health verification and automatic rollback:

```go
// control-plane/internal/rollout/engine.go
type RolloutEngine struct {
    store  *store.Store
    health *HealthChecker
}

type RolloutStrategy string
const (
    RolloutImmediate RolloutStrategy = "immediate"  // All at once
    RolloutCanary    RolloutStrategy = "canary"     // 5% first, then staged
    RolloutStaged    RolloutStrategy = "staged"     // 10% → 25% → 50% → 100%
    RolloutManual    RolloutStrategy = "manual"     // Operator selects agents
)

type RolloutConfig struct {
    Strategy          RolloutStrategy   `json:"strategy"`
    CanaryPercent     int               `json:"canary_percent"`      // e.g., 5
    CanaryDuration    time.Duration     `json:"canary_duration"`     // e.g., 10m
    Waves             []int             `json:"waves"`               // e.g., [10, 25, 50, 100]
    WaveDelay         time.Duration     `json:"wave_delay"`          // e.g., 5m between waves
    HealthCheckWait   time.Duration     `json:"health_check_wait"`   // e.g., 2m
    FailureThreshold  int               `json:"failure_threshold"`   // e.g., 10% triggers rollback
    AutoRollback      bool              `json:"auto_rollback"`       // default: true
}

// Health check verifies:
// - Agent heartbeat within 2 minutes
// - Agent reporting expected version
// - Probe success rate stable (no spike in errors)
```

**Rollout Flow (Canary):**
1. Select 5% of agents randomly
2. Push update notification via heartbeat
3. Wait for all canaries to update (or timeout)
4. Run health check for `canary_duration`
5. If healthy: proceed to staged waves
6. If unhealthy: auto-rollback canaries, halt rollout

**Automatic Rollback Triggers:**
- >10% of updated agents fail health check
- Updated agents stop sending heartbeats
- Agents report wrong version after timeout

---

## Database Schema

### Migration: `005_agent_enrollment.sql`

```sql
-- SSH keys metadata only (private key stored in 1Password)
CREATE TABLE ssh_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL UNIQUE,
    key_type VARCHAR(20) NOT NULL DEFAULT 'ed25519',
    public_key TEXT NOT NULL,
    fingerprint VARCHAR(64) NOT NULL,
    onepassword_item_id VARCHAR(100),  -- Reference to 1Password item
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    rotated_at TIMESTAMPTZ
);

-- Enrollment sessions
CREATE TABLE enrollments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    target_ip INET NOT NULL,
    target_port INTEGER DEFAULT 22,
    agent_name VARCHAR(255),
    region VARCHAR(50),
    tags JSONB DEFAULT '{}',

    -- State machine
    state VARCHAR(30) NOT NULL DEFAULT 'pending',
    current_step VARCHAR(50),
    steps_completed TEXT[],

    -- Detection results
    detected_os VARCHAR(50),
    detected_arch VARCHAR(20),
    detected_hostname VARCHAR(255),

    -- Result
    agent_id UUID REFERENCES agents(id),

    -- Rollback tracking
    changes JSONB DEFAULT '[]',

    -- Error handling
    last_error TEXT,
    retry_count INTEGER DEFAULT 0,

    started_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ,

    requested_by VARCHAR(255)
);

-- Agent releases
CREATE TABLE agent_releases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version VARCHAR(50) NOT NULL UNIQUE,
    release_notes TEXT,

    -- Binary info per platform
    artifacts JSONB NOT NULL,
    -- [{"platform": "linux-amd64", "checksum": "sha256:...", "size": 12345}]

    status VARCHAR(20) DEFAULT 'draft',
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Rollouts
CREATE TABLE rollouts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    release_id UUID REFERENCES agent_releases(id),
    strategy VARCHAR(20) NOT NULL,
    config JSONB,

    status VARCHAR(20) DEFAULT 'pending',

    agents_total INTEGER DEFAULT 0,
    agents_updated INTEGER DEFAULT 0,
    agents_failed INTEGER DEFAULT 0,

    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    created_by VARCHAR(255)
);

-- Agent update history
CREATE TABLE agent_update_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    rollout_id UUID REFERENCES rollouts(id),

    from_version VARCHAR(50),
    to_version VARCHAR(50),

    status VARCHAR(20) DEFAULT 'pending',
    started_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ,

    error_message TEXT
);

-- Extend agents table
ALTER TABLE agents ADD COLUMN IF NOT EXISTS platform VARCHAR(50);
ALTER TABLE agents ADD COLUMN IF NOT EXISTS enrolled_at TIMESTAMPTZ;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS enrolled_by VARCHAR(255);
ALTER TABLE agents ADD COLUMN IF NOT EXISTS ssh_key_id UUID REFERENCES ssh_keys(id);
```

---

## API Endpoints

### Enrollment

```
POST /api/v1/agents/enroll
  Body: { ip, port?, username, password, agent_name?, region?, tags? }
  Response: SSE stream of enrollment events

GET /api/v1/agents/enrollments
  List enrollment history

GET /api/v1/agents/enrollments/{id}
  Get enrollment details

POST /api/v1/agents/enrollments/{id}/retry
  Retry failed enrollment

DELETE /api/v1/agents/enrollments/{id}
  Cancel and cleanup enrollment
```

### Updates & Rollouts

```
POST /api/v1/releases
  Upload new agent release

GET /api/v1/releases
  List releases

POST /api/v1/releases/{id}/publish
  Publish release (make available for rollout)

POST /api/v1/rollouts
  Start a rollout { release_id, strategy, config? }

GET /api/v1/rollouts
  List rollouts

GET /api/v1/rollouts/{id}
  Get rollout status with per-agent breakdown

POST /api/v1/rollouts/{id}/pause
POST /api/v1/rollouts/{id}/resume
POST /api/v1/rollouts/{id}/rollback

GET /api/v1/fleet/versions
  Version distribution across fleet
```

---

## UI Components

### EnrollAgentModal.jsx

```jsx
// States: form → enrolling → success/failure

// Form state
<Modal title="Enroll New Agent">
  <Input label="Server IP" value={ip} />
  <Input label="SSH Username" value={username} />
  <Input label="Password" type="password" value={password} />
  <Input label="Agent Name (optional)" value={agentName} />
  <Input label="Region (optional)" value={region} />
  <Button onClick={handleEnroll}>Enroll Agent</Button>
</Modal>

// Enrolling state - real-time progress via SSE
<Modal title="Enrolling Agent">
  <ProgressBar percent={progress} />
  <StepList steps={steps} currentStep={current} />
  <LogOutput lines={logs} />
</Modal>
```

### FleetManagement.jsx (or extend Agents.jsx)

- Version distribution chart
- Active rollouts with progress
- "Update All" / "Start Rollout" actions
- Agent version table with bulk operations

---

## Enrollment Flow (Detailed)

### Step 1: Connect
```bash
# SSH with provided password
ssh -o StrictHostKeyChecking=accept-new user@host
```

### Step 2: Detect System
```bash
# Run detection script
cat /etc/os-release | grep -E "^ID=|^VERSION_ID="
uname -m
which apt-get || which dnf || which yum
```

### Step 3: Install SSH Key
```bash
mkdir -p ~/.ssh && chmod 700 ~/.ssh
echo 'ssh-ed25519 AAAA... icmpmon-control-plane' >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys
```

### Step 4: Verify Key Auth
```bash
# Reconnect using key (proves key works before hardening)
```

### Step 5: Harden SSH
```bash
sudo sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
sudo sed -i 's/^#*ChallengeResponseAuthentication.*/ChallengeResponseAuthentication no/' /etc/ssh/sshd_config
sudo sshd -t && sudo systemctl reload sshd
```

### Step 6: Install Agent
```bash
# Create user
sudo useradd -r -s /bin/false icmpmon || true

# Download and install binary
curl -o /tmp/icmpmon-agent https://control-plane/api/v1/packages/agent-linux-amd64
sudo mv /tmp/icmpmon-agent /usr/local/bin/
sudo chmod 755 /usr/local/bin/icmpmon-agent
sudo setcap cap_net_raw=ep /usr/local/bin/icmpmon-agent

# Install config
sudo mkdir -p /etc/icmpmon
cat <<EOF | sudo tee /etc/icmpmon/agent.yaml
control_plane:
  url: https://control-plane.example.com
agent:
  name: ${AGENT_NAME}
  region: ${REGION}
EOF

# Install systemd service
cat <<EOF | sudo tee /etc/systemd/system/icmpmon-agent.service
[Unit]
Description=ICMP-Mon Agent
After=network.target

[Service]
Type=simple
User=icmpmon
ExecStart=/usr/local/bin/icmpmon-agent --config /etc/icmpmon/agent.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable icmpmon-agent
sudo systemctl start icmpmon-agent
```

### Step 7: Verify Registration
```bash
# Wait for agent to appear in control plane (via heartbeat)
# Timeout after 60 seconds
```

---

## Update Flow (Agent Self-Update)

1. Control plane publishes new release
2. Operator starts rollout (immediate/canary/staged)
3. Selected agents receive `update_available` in heartbeat response
4. Agent downloads binary, verifies checksum
5. Agent performs atomic symlink swap:
   ```
   /usr/local/bin/icmpmon-agent -> icmpmon-agent-v1.2.3
   ```
6. Agent requests systemd restart
7. New agent starts, reports version in heartbeat
8. Control plane tracks rollout progress
9. If health checks fail, automatic rollback triggered

---

## Security Measures

1. **Password never stored** - used once for initial SSH, cleared from memory
2. **SSH key encrypted at rest** - AES-256-GCM with master key from env
3. **SSH hardening** - disable password auth after key installed
4. **Binary verification** - SHA256 checksum on all downloads
5. **Audit logging** - all enrollment and update actions logged
6. **Rollback capability** - can revert SSH config and remove agent

---

## File Structure

```
control-plane/internal/
├── enrollment/
│   ├── service.go        # Enrollment orchestration & state machine
│   ├── ssh.go            # SSH connection handling
│   ├── detection.go      # OS/arch detection scripts
│   ├── installer.go      # Binary installation & systemd setup
│   └── rollback.go       # Cleanup on failure
├── secrets/
│   └── onepassword.go    # 1Password Connect integration
├── rollout/
│   ├── engine.go         # Rollout orchestration with waves
│   ├── health.go         # Health verification
│   └── selector.go       # Agent selection for waves
└── api/
    ├── enrollment.go     # Enrollment endpoints (SSE streaming)
    └── rollout.go        # Rollout & release endpoints

agent/internal/
└── updater/
    └── updater.go        # Self-update with symlink swap

ui/src/
├── components/
│   ├── EnrollAgentModal.jsx    # Multi-step enrollment with progress
│   ├── RolloutProgress.jsx     # Wave progress visualization
│   └── VersionBadge.jsx        # Version display component
└── pages/
    └── Agents.jsx              # Add enroll button, version column, fleet stats

db/migrations/
└── 005_agent_enrollment.sql
```

### Dependencies to Add

```go
// go.mod additions
require (
    github.com/1Password/connect-sdk-go v1.5.3  // 1Password Connect
    golang.org/x/crypto v0.21.0                 // SSH client
)
```

---

## Implementation Phases

### Phase 1: Foundation
- [ ] Database migration (`005_agent_enrollment.sql`)
- [ ] 1Password Connect integration (`secrets/onepassword.go`)
- [ ] SSH key generation and storage in 1Password
- [ ] Basic SSH client wrapper (`enrollment/ssh.go`)

### Phase 2: Enrollment Service
- [ ] Enrollment state machine (`enrollment/service.go`)
- [ ] System detection scripts (`enrollment/detection.go`)
- [ ] SSH key installation and verification
- [ ] SSH hardening (always enabled)
- [ ] Agent binary transfer and installation
- [ ] Rollback on failure (`enrollment/rollback.go`)
- [ ] Enrollment API with SSE streaming (`api/enrollment.go`)

### Phase 3: Enrollment UI
- [ ] EnrollAgentModal component with form
- [ ] Progress tracking with step visualization
- [ ] Success/failure states
- [ ] "Enroll Agent" button in Agents page

### Phase 4: Agent Self-Update
- [ ] Updater module in agent (`agent/internal/updater/`)
- [ ] Extend HeartbeatResponse with `update_available`
- [ ] Binary download with checksum verification
- [ ] Atomic symlink swap
- [ ] Systemd restart request

### Phase 5: Rollout Engine
- [ ] Release management (upload, publish)
- [ ] Rollout engine with wave management
- [ ] Agent selection for canary/staged
- [ ] Health verification between waves
- [ ] Automatic rollback triggers
- [ ] Rollout API endpoints

### Phase 6: Fleet UI
- [ ] Version column in Agents table
- [ ] Fleet stats (version distribution)
- [ ] Rollout progress visualization
- [ ] "Update All" and rollout controls

---

## Critical Files to Modify

| File | Changes |
|------|---------|
| `control-plane/internal/api/api.go` | Add enrollment and rollout endpoints |
| `control-plane/internal/store/store.go` | Add enrollment, release, rollout queries |
| `control-plane/internal/service/service.go` | Wire up new services |
| `control-plane/cmd/server/main.go` | Initialize key store and enrollment service |
| `agent/agent.go` | Add update check in heartbeat loop |
| `agent/internal/client/client.go` | Handle update_available in HeartbeatResponse |
| `ui/src/pages/Agents.jsx` | Add Enroll button, version column |
| `ui/src/lib/api.js` | Add enrollment and rollout endpoints |
| `pkg/types/types.go` | Add UpdateInfo to HeartbeatResponse |
