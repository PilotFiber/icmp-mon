# ICMP-Mon

Distributed network reachability monitoring system for large-scale ISPs.

## Features

- **100,000+ targets** monitored from **100+ distributed agents**
- **Tiered monitoring**: Infrastructure (5s), VIP (15s), Standard (30s)
- **Flexible agent selection**: By region, provider, tags, with diversity requirements
- **ICMP ping + MTR**: Continuous probing with on-demand path tracing
- **Maintenance snapshots**: Before/after comparison for change validation
- **Security testing**: Expected-failure probes for firewall policy validation
- **Tag-based correlation**: Find common factors in outages

## Quick Start

### Prerequisites

- Go 1.23+
- Docker & Docker Compose
- fping (for local agent development)

### Start Services

```bash
# Start database and control plane
./scripts/dev.sh up

# In another terminal, start the UI
./scripts/dev.sh ui

# In another terminal, start the Docker test agent
docker-compose -f deploy/docker-compose.yml up -d agent
```

**Access Points:**
- UI Dashboard: http://localhost:3000
- Control Plane API: http://localhost:8081

### Add Test Targets

```bash
# Add targets via API
./scripts/dev.sh add-target 8.8.8.8 standard
./scripts/dev.sh add-target 1.1.1.1 vip

# List targets
./scripts/dev.sh list-targets

# List agents
./scripts/dev.sh list-agents
```

### API Examples

```bash
# Health check
curl http://localhost:8081/api/v1/health

# List tiers
curl http://localhost:8081/api/v1/tiers

# Create target with tags
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

# Create security test target (expect failure)
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

## Architecture

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

## Configuration

### Tiers

Tiers control monitoring intensity:

| Tier | Interval | Agents | Use Case |
|------|----------|--------|----------|
| `infrastructure` | 5s | All | Core network devices |
| `vip` | 15s | 18 (diverse) | Premium customers |
| `standard` | 30s | 4 | Standard customers |

### Agent Selection Policy

```yaml
agent_selection:
  strategy: distributed     # "all" or "distributed"
  count: 18                 # For distributed: agents per target
  regions:                  # Limit to these regions
    - us-east
    - us-west
    - europe
  diversity:
    min_regions: 4          # Spread across N regions
    min_providers: 3        # Spread across N providers
  require_tags:             # Agent must have these tags
    network_type: external
```

## Development

### Run Tests

```bash
go test ./...
```

### Project Structure

```
icmp-mon/
├── agent/                   # Agent implementation
│   ├── cmd/agent/          # CLI entrypoint
│   └── internal/
│       ├── executor/       # Probe executors (ICMP, MTR)
│       ├── scheduler/      # Probe scheduling
│       └── shipper/        # Result shipping
├── control-plane/          # Control plane
│   ├── cmd/server/         # CLI entrypoint
│   └── internal/
│       ├── api/           # HTTP handlers
│       ├── service/       # Business logic
│       └── store/         # Database access
├── pkg/types/              # Shared types
├── db/migrations/          # SQL migrations
├── deploy/                 # Docker configs
└── ui/                     # React dashboard
```

### Adding New Executors

1. Create a new file in `agent/internal/executor/`
2. Implement the `Executor` interface
3. Register in agent startup

```go
type MyExecutor struct{}

func (e *MyExecutor) Type() string { return "my_probe" }
func (e *MyExecutor) Capabilities() Capabilities { ... }
func (e *MyExecutor) Execute(ctx, target) (*Result, error) { ... }
func (e *MyExecutor) ExecuteBatch(ctx, targets) ([]*Result, error) { ... }
```

## Documentation

- [Development Guide](DEVELOPMENT.md) - Setup, configuration, and deployment
- [Architecture](docs/ARCHITECTURE.md) - Detailed system design

## License

Proprietary - Pilot Fiber
