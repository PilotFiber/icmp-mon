# ICMP-Mon Development Guide

This document provides a comprehensive overview of the ICMP-Mon distributed network monitoring system, including architecture, setup instructions, and configuration details.

## Quick Start

### Prerequisites

- **Go 1.23+** - For building the agent and control plane
- **Docker & Docker Compose** - For running services locally
- **Node.js 18+** - For the UI development server
- **fping** (optional) - For running the agent outside Docker on macOS/Linux

### Start Everything

```bash
# 1. Start the database and control plane
./scripts/dev.sh up

# 2. Start the UI development server (in a separate terminal)
./scripts/dev.sh ui

# 3. Start a test agent (in a separate terminal)
docker-compose -f deploy/docker-compose.yml up -d agent
```

**Access Points:**
- **UI Dashboard**: http://localhost:3000
- **Control Plane API**: http://localhost:8081
- **Database**: localhost:5432 (user: icmpmon, password: icmpmon)

### Add Test Targets

```bash
./scripts/dev.sh add-target 8.8.8.8 standard
./scripts/dev.sh add-target 1.1.1.1 vip
./scripts/dev.sh add-target 9.9.9.9 infrastructure
```

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         CONTROL PLANE                                    │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐ │
│  │  Agent   │  │  Target  │  │  Result  │  │ Snapshot │  │  Alert   │ │
│  │ Registry │  │ Assign.  │  │ Ingest   │  │ Service  │  │ Evaluator│ │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘  └──────────┘ │
│                                   │                                      │
│                          ┌────────────────┐                             │
│                          │  TimescaleDB   │                             │
│                          └────────────────┘                             │
└─────────────────────────────────────────────────────────────────────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              │                    │                    │
              ▼                    ▼                    ▼
       ┌────────────┐       ┌────────────┐       ┌────────────┐
       │   Agent    │       │   Agent    │       │   Agent    │
       │ us-east-01 │       │ europe-01  │       │  asia-01   │
       └────────────┘       └────────────┘       └────────────┘
```

### Components

#### Control Plane (`/control-plane`)
The central orchestration service that:
- Registers and monitors agents
- Manages target configurations and tier policies
- Calculates agent-to-target assignments
- Ingests and stores probe results in TimescaleDB
- Evaluates alert conditions

#### Agent (`/agent`)
Distributed monitoring agents that:
- Register with the control plane on startup
- Receive target assignments based on tier policies
- Execute ICMP probes using fping for efficiency
- Report results back to the control plane
- Send periodic heartbeats with health metrics

#### UI Dashboard (`/ui`)
React-based monitoring dashboard featuring:
- Fleet overview with real-time metrics
- Agent management and health monitoring
- Target configuration and status tracking
- Alert management
- Maintenance snapshot workflows

---

## Project Structure

```
icmp-mon/
├── agent/                      # Agent implementation
│   ├── cmd/agent/              # CLI entrypoint
│   │   └── main.go
│   ├── internal/
│   │   ├── config/             # Agent configuration
│   │   ├── client/             # Control plane API client
│   │   ├── executor/           # Probe executors (ICMP, MTR)
│   │   │   ├── executor.go     # Executor interface
│   │   │   ├── icmp.go         # fping-based ICMP executor
│   │   │   └── executor_test.go
│   │   ├── scheduler/          # Probe scheduling by tier
│   │   └── shipper/            # Result batching and shipping
│   └── agent.go                # Main agent struct
│
├── control-plane/              # Control plane implementation
│   ├── cmd/server/             # CLI entrypoint
│   │   └── main.go
│   └── internal/
│       ├── api/                # HTTP handlers
│       ├── service/            # Business logic
│       └── store/              # Database access (pgx)
│
├── pkg/types/                  # Shared types
│   ├── types.go                # Core domain types
│   └── alerts.go               # Alert configuration types
│
├── db/migrations/              # SQL migrations
│   ├── 001_initial_schema.sql  # Core schema + TimescaleDB
│   └── 002_agent_metrics.sql   # Agent health tracking
│
├── deploy/                     # Docker configs
│   ├── docker-compose.yml      # Local development stack
│   ├── Dockerfile.agent        # Agent container
│   └── Dockerfile.control-plane
│
├── ui/                         # React dashboard
│   ├── src/
│   │   ├── components/         # Reusable UI components
│   │   ├── pages/              # Route pages
│   │   ├── hooks/              # React hooks
│   │   └── lib/                # Utilities and API client
│   └── vite.config.js
│
├── scripts/
│   └── dev.sh                  # Development helper script
│
└── docs/
    └── ARCHITECTURE.md         # Detailed architecture docs
```

---

## Monitoring Tiers

Tiers control monitoring intensity for different classes of targets:

| Tier | Interval | Agents | Use Case |
|------|----------|--------|----------|
| `infrastructure` | 5s | All agents | Pilot's core network devices |
| `vip` | 15s | 18 (with diversity) | Premium customers |
| `standard` | 30s | 4 | Standard customers |

### Tier Configuration

Tiers are defined in the database and control:
- **Probe timing**: interval, timeout, retries
- **Agent selection**: how many agents, from which regions/providers
- **Diversity requirements**: minimum regions and providers for redundancy

```sql
-- Example tier configuration
INSERT INTO tiers (name, display_name, probe_interval_ms, agent_selection) VALUES
  ('vip', 'VIP Customers', 15000, '{
    "strategy": "distributed",
    "count": 18,
    "diversity": {
      "min_regions": 4,
      "min_providers": 3
    }
  }');
```

---

## Agent Configuration

### Running in Docker (Recommended for Testing)

The Docker agent is pre-configured in `docker-compose.yml`:

```bash
docker-compose -f deploy/docker-compose.yml up -d agent
```

### Running Locally

For development, run the agent directly:

```bash
# Install fping (required)
brew install fping  # macOS
apt install fping   # Ubuntu/Debian

# Run the agent
./scripts/dev.sh agent

# Or manually:
go run ./agent/cmd/agent \
  --control-plane http://localhost:8081 \
  --name "local-dev-agent" \
  --region "local" \
  --location "Local Development" \
  --provider "local" \
  --debug
```

### Running on a Remote Server

To deploy an agent to a remote server:

1. **Build the agent binary:**
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o icmpmon-agent ./agent/cmd/agent
```

2. **Copy to remote server:**
```bash
scp icmpmon-agent user@remote-server:/usr/local/bin/
```

3. **Install fping on the remote server:**
```bash
apt install fping
```

4. **Create systemd service** (`/etc/systemd/system/icmpmon-agent.service`):
```ini
[Unit]
Description=ICMP-Mon Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/icmpmon-agent \
  --control-plane https://your-control-plane.example.com \
  --name "us-east-01" \
  --region "us-east" \
  --location "New York, NY" \
  --provider "aws"
Restart=always
RestartSec=10

# Required for ICMP
AmbientCapabilities=CAP_NET_RAW
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

5. **Start the service:**
```bash
systemctl daemon-reload
systemctl enable icmpmon-agent
systemctl start icmpmon-agent
```

### Agent Environment Variables

| Variable | Description |
|----------|-------------|
| `ICMPMON_CONTROL_PLANE_URL` | Control plane URL |
| `ICMPMON_AGENT_NAME` | Unique agent name |
| `ICMPMON_AGENT_REGION` | Geographic region |
| `ICMPMON_AGENT_LOCATION` | Human-readable location |
| `ICMPMON_AGENT_PROVIDER` | Hosting provider (aws, gcp, etc.) |

---

## API Reference

### Health Check
```bash
curl http://localhost:8081/api/v1/health
```

### List Agents
```bash
curl http://localhost:8081/api/v1/agents
```

### List Targets
```bash
curl http://localhost:8081/api/v1/targets
```

### Create Target
```bash
curl -X POST http://localhost:8081/api/v1/targets \
  -H "Content-Type: application/json" \
  -d '{
    "ip": "10.0.0.1",
    "tier": "infrastructure",
    "tags": {
      "device": "core-router-chi-01",
      "pop": "chicago"
    }
  }'
```

### Create Security Test Target
```bash
curl -X POST http://localhost:8081/api/v1/targets \
  -H "Content-Type: application/json" \
  -d '{
    "ip": "10.0.0.1",
    "tier": "standard",
    "expected_outcome": {
      "should_succeed": false,
      "alert_severity": "critical",
      "alert_message": "SSH accessible from external network!"
    }
  }'
```

### List Tiers
```bash
curl http://localhost:8081/api/v1/tiers
```

---

## Database Schema

The system uses TimescaleDB for efficient time-series storage.

### Key Tables

- **`agents`** - Registered monitoring agents
- **`targets`** - IPs to monitor with tier assignments
- **`tiers`** - Tier configurations (intervals, agent selection)
- **`probe_results`** - Time-series probe data (hypertable)
- **`agent_metrics`** - Agent health metrics (hypertable)
- **`alerts`** - Generated alerts

### Connect to Database

```bash
./scripts/dev.sh psql

# Or directly:
docker exec -it icmpmon-db psql -U icmpmon -d icmpmon
```

### Useful Queries

```sql
-- Recent probe results
SELECT * FROM probe_results
WHERE time > NOW() - INTERVAL '5 minutes'
ORDER BY time DESC
LIMIT 100;

-- Agent health
SELECT name, status, last_heartbeat,
       NOW() - last_heartbeat as time_since_heartbeat
FROM agents;

-- Targets by tier
SELECT tier, COUNT(*) FROM targets GROUP BY tier;
```

---

## UI Development

The UI is built with React, Vite, and Tailwind CSS using Pilot's design system.

### Start Development Server

```bash
./scripts/dev.sh ui
# Or:
cd ui && npm run dev
```

The UI runs on http://localhost:3000 and proxies API requests to the control plane at http://localhost:8081.

### Build for Production

```bash
./scripts/dev.sh ui-build
# Or:
cd ui && npm run build
```

### Design System

The UI uses Pilot Fiber's brand colors and Maax font:

- **Primary Yellow**: `#FFE200`
- **Navy**: `#18284F`
- **Cyan**: `#6EDBE0`
- **Red**: `#FC534E`

---

## Development Commands

```bash
# Start database + control plane
./scripts/dev.sh up

# Stop everything
./scripts/dev.sh down

# Stop and remove data
./scripts/dev.sh down-v

# View logs
./scripts/dev.sh logs

# Connect to database
./scripts/dev.sh psql

# Run local agent
./scripts/dev.sh agent

# Run local control plane (without Docker)
./scripts/dev.sh server

# Run tests
./scripts/dev.sh test

# Start UI
./scripts/dev.sh ui

# Build UI
./scripts/dev.sh ui-build

# Add target
./scripts/dev.sh add-target 8.8.8.8 standard

# List targets
./scripts/dev.sh list-targets

# List agents
./scripts/dev.sh list-agents

# Health check
./scripts/dev.sh health
```

---

## Troubleshooting

### Port 8081 Already in Use

The control plane runs on port 8081 (changed from 8080 to avoid conflicts).

```bash
lsof -i :8081
```

### Agent Not Registering

1. Check control plane is running: `./scripts/dev.sh health`
2. Check agent logs: `docker logs icmpmon-agent-test`
3. Verify network connectivity between agent and control plane

### No Probe Results

1. Ensure agent has assigned targets: check agent logs for "assignments synced"
2. Verify fping is installed (for local agents)
3. Check that the agent container has `CAP_NET_RAW` capability

### Database Connection Issues

```bash
# Check database is healthy
docker ps | grep icmpmon-db

# Restart database
docker-compose -f deploy/docker-compose.yml restart timescaledb
```

---

## Testing

### Run All Tests

```bash
go test ./...
```

### Run with Verbose Output

```bash
go test -v ./...
```

### Integration Tests

The ICMP executor has integration tests that require fping:

```bash
go test -v ./agent/internal/executor/...
```

---

## Next Steps

### Planned Features

1. **MTR Executor** - On-demand path tracing
2. **Maintenance Snapshots** - Before/after comparison
3. **Alert Routing** - Slack, PagerDuty, webhooks
4. **Real-time UI Updates** - WebSocket or SSE
5. **Tag-based Correlation** - Find common factors in outages

### Contributing

1. Fork the repository
2. Create a feature branch
3. Write tests for new functionality
4. Submit a pull request
