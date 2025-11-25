# ICMP-Mon: Distributed Network Reachability Monitoring System

## Overview

ICMP-Mon is a distributed network monitoring system designed for large-scale ISPs to monitor reachability of customer endpoints and internal infrastructure from multiple vantage points across the internet.

### Key Capabilities

- **100,000+ targets** monitored from **100+ agents**
- **Three execution modes**: Continuous probing, on-demand commands, point-in-time snapshots
- **Flexible tiers**: Control interval, agent count, region filtering, and diversity requirements
- **Extensible executors**: ICMP ping, MTR, TCP connect, HTTP checks, DNS resolution
- **Security validation**: Expected-failure testing for firewall policy verification
- **Maintenance workflows**: Before/after snapshots with automatic diff analysis

## Core Concepts

### Targets

A target is an IP address to monitor. Each target belongs to a tier and can have tags for correlation.

### Tiers

Tiers define the complete monitoring policy for a set of targets:

| Property | Description |
|----------|-------------|
| `probe_interval` | How often to probe (5s, 15s, 30s, etc.) |
| `probe_timeout` | How long to wait for response |
| `agent_selection.strategy` | "all" (every agent) or "distributed" (subset) |
| `agent_selection.count` | For distributed: how many agents per target |
| `agent_selection.regions` | Limit to specific regions (us-east, europe, etc.) |
| `agent_selection.require_tags` | Agent must have these tags |
| `agent_selection.diversity` | Spread requirements (min_regions, min_providers) |

### Agents

Lightweight processes deployed across the internet that:
- Register with control plane on startup
- Receive target assignments based on tier policies
- Continuously probe targets using executors
- Execute on-demand commands (MTR, diagnostics)
- Report results and health telemetry

### Executors

Plugin architecture for probe types:

| Executor | Purpose | Batching |
|----------|---------|----------|
| `icmp_ping` | Reachability + latency via fping | Yes |
| `mtr` | Full path trace | No |
| `tcp_connect` | Port accessibility | Yes |

### Snapshots

Point-in-time state captures for maintenance windows. Compare before/after to detect regressions.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         CONTROL PLANE                                    │
│                                                                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐ │
│  │  Agent   │  │  Target  │  │  Result  │  │ Snapshot │  │  Alert   │ │
│  │ Registry │  │ Assign.  │  │ Ingest   │  │ Service  │  │ Evaluator│ │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘  └──────────┘ │
│        │            │             │             │             │         │
│        └────────────┴─────────────┴─────────────┴─────────────┘         │
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

## Data Flow

### Continuous Probing

1. Agent syncs assignments from control plane
2. Agent groups targets by tier
3. For each tier, agent runs probe loop at tier's interval
4. Results batched and shipped to control plane
5. Control plane stores results, evaluates alerts

### On-Demand Commands

1. User requests MTR to target from UI
2. Control plane queues command for relevant agents
3. Agents poll for commands, execute, return results
4. Control plane aggregates and displays results

### Snapshots

1. User creates snapshot with scope (tags filter)
2. Control plane queries recent probe results for matching targets
3. Computes consensus state (reachable/unreachable) per target
4. Stores snapshot for later comparison

## Directory Structure

```
icmp-mon/
├── docs/                    # Documentation
├── pkg/                     # Shared Go packages
│   └── types/              # Core types (Target, Tier, Result, etc.)
├── agent/                   # Agent implementation
│   ├── cmd/                # CLI entrypoint
│   └── internal/
│       ├── executor/       # Probe executors (icmp, mtr, tcp)
│       ├── scheduler/      # Probe scheduling by tier
│       ├── shipper/        # Result batching and shipping
│       └── health/         # Agent health monitoring
├── control-plane/          # Control plane implementation
│   ├── cmd/                # CLI entrypoint
│   └── internal/
│       ├── api/           # HTTP handlers
│       ├── service/       # Business logic
│       └── store/         # Database access
├── db/
│   └── migrations/         # SQL migrations
├── ui/                     # Web dashboard
├── deploy/
│   └── docker-compose.yml  # Local development
├── scripts/                # Dev utilities
└── test/
    ├── integration/        # Integration tests
    └── fixtures/           # Test data
```

## Local Development

```bash
# Start everything
docker-compose up -d

# Run a test agent
go run ./agent/cmd --control-plane http://localhost:8080 --name test-01

# Add a target
curl -X POST localhost:8080/api/v1/targets -d '{"ip":"8.8.8.8","tier":"standard"}'

# View results
curl localhost:8080/api/v1/targets/8.8.8.8/results
```

## Security Considerations

- Agents authenticate with enrollment tokens, then receive credentials
- All agent↔control plane communication over TLS
- API authentication via JWT (users) or API keys (services)
- Role-based access control for UI

---

## Implementation Status

### Completed

| Component | Description |
|-----------|-------------|
| **Control Plane** | Full HTTP API server with agent registry, target management, result ingestion |
| **Agent** | Probe executor with fping batching, result shipping, health telemetry |
| **Database Schema** | TimescaleDB with hypertables, continuous aggregates (hourly/daily/monthly), baselines, incidents |
| **Web UI** | React dashboard with real-time updates |

#### API Endpoints (Implemented)
- `GET/POST /api/v1/targets` - Target CRUD
- `GET/PUT/DELETE /api/v1/targets/{id}` - Individual target operations
- `GET /api/v1/targets/{id}/status` - Real-time target status
- `GET /api/v1/targets/{id}/history` - Historical probe data
- `GET /api/v1/targets/{id}/live` - Live streaming probe results
- `POST /api/v1/targets/{id}/mtr` - Trigger MTR trace
- `GET/POST /api/v1/tiers` - Tier CRUD
- `GET/PUT/DELETE /api/v1/tiers/{name}` - Individual tier operations
- `GET /api/v1/agents` - List agents
- `GET /api/v1/agents/{id}/metrics` - Agent telemetry
- `GET/POST /api/v1/incidents` - Incident management
- `POST /api/v1/incidents/{id}/acknowledge` - Acknowledge incident
- `POST /api/v1/incidents/{id}/resolve` - Resolve incident
- `PUT /api/v1/incidents/{id}/notes` - Add notes
- `GET /api/v1/baselines/{agent}/{target}` - Get baseline for pair
- `POST /api/v1/baselines/recalculate` - Trigger baseline recalc
- `GET /api/v1/reports/targets/{id}` - Target performance report
- `GET/POST /api/v1/snapshots` - Snapshot management
- `GET /api/v1/snapshots/{id}/compare/{id2}` - Compare snapshots

#### UI Pages (Implemented)
- **Dashboard** - Fleet overview with health metrics
- **Targets** - Target list with status, detail panel, live streaming view with graph
- **Agents** - Agent list with health and metrics
- **Incidents** - Incident list with acknowledge/resolve/notes
- **Snapshots** - Before/after comparison
- **Alerts** - Alert rule list
- **Settings** - Tier management with full CRUD

### In Progress / Remaining

| Component | Description | Priority |
|-----------|-------------|----------|
| **Background Jobs** | Baseline calculator, state evaluator, correlation engine, incident manager | High |
| **Reporting UI** | Display target reports, SLA compliance views | Medium |
| **Baseline Visualization** | Show baseline data in target detail view | Medium |
| **Report Export** | JSON/CSV/PDF export for reports | Low |
| **Alert Rule Engine** | Configurable alert thresholds and notifications | Medium |
| **Notification Handlers** | Slack, PagerDuty, webhook integrations | Medium |

See [INCIDENTS_AND_REPORTING.md](./INCIDENTS_AND_REPORTING.md) for detailed design of the baseline detection and incident correlation system.
