#!/bin/bash
# Development helper script for ICMP-Mon
#
# Usage:
#   ./scripts/dev.sh up        # Start services
#   ./scripts/dev.sh down      # Stop services
#   ./scripts/dev.sh logs      # View logs
#   ./scripts/dev.sh psql      # Connect to database
#   ./scripts/dev.sh agent     # Run agent locally (outside docker)
#   ./scripts/dev.sh server    # Run control plane locally
#   ./scripts/dev.sh test      # Run tests
#   ./scripts/dev.sh ui        # Run UI development server
#   ./scripts/dev.sh add-target <ip> [tier]  # Add a test target

set -e

cd "$(dirname "$0")/.."

case "$1" in
  up)
    echo "Starting services..."
    docker-compose -f deploy/docker-compose.yml up -d timescaledb
    echo "Waiting for database..."
    sleep 5
    echo "Starting control plane..."
    docker-compose -f deploy/docker-compose.yml up -d control-plane
    echo ""
    echo "Services started!"
    echo "  Control plane: http://localhost:8081"
    echo "  Database: localhost:5432"
    echo ""
    echo "To start a test agent: ./scripts/dev.sh agent"
    ;;

  down)
    echo "Stopping services..."
    docker-compose -f deploy/docker-compose.yml down
    ;;

  down-v)
    echo "Stopping services and removing volumes..."
    docker-compose -f deploy/docker-compose.yml down -v
    ;;

  logs)
    docker-compose -f deploy/docker-compose.yml logs -f
    ;;

  psql)
    docker-compose -f deploy/docker-compose.yml exec timescaledb psql -U icmpmon -d icmpmon
    ;;

  agent)
    echo "Running agent locally..."
    go run ./agent/cmd/agent \
      --control-plane http://localhost:8081 \
      --name "local-dev-agent" \
      --region "local" \
      --location "Local Development" \
      --provider "local" \
      --debug
    ;;

  server)
    echo "Running control plane locally..."
    go run ./control-plane/cmd/server \
      --database "postgres://icmpmon:icmpmon@localhost:5432/icmpmon?sslmode=disable" \
      --debug
    ;;

  test)
    echo "Running tests..."
    go test -v ./...
    ;;

  ui)
    echo "Starting UI development server..."
    cd ui && npm run dev
    ;;

  ui-build)
    echo "Building UI for production..."
    cd ui && npm run build
    ;;

  add-target)
    IP="${2:-8.8.8.8}"
    TIER="${3:-standard}"
    echo "Adding target: $IP (tier: $TIER)"
    curl -X POST http://localhost:8081/api/v1/targets \
      -H "Content-Type: application/json" \
      -d "{\"ip\": \"$IP\", \"tier\": \"$TIER\"}"
    echo ""
    ;;

  list-targets)
    echo "Listing targets..."
    curl -s http://localhost:8081/api/v1/targets | jq .
    ;;

  list-agents)
    echo "Listing agents..."
    curl -s http://localhost:8081/api/v1/agents | jq .
    ;;

  health)
    curl -s http://localhost:8081/api/v1/health | jq .
    ;;

  *)
    echo "ICMP-Mon Development Helper"
    echo ""
    echo "Usage: $0 <command>"
    echo ""
    echo "Commands:"
    echo "  up            Start database and control plane"
    echo "  down          Stop services"
    echo "  down-v        Stop services and remove volumes"
    echo "  logs          View service logs"
    echo "  psql          Connect to database"
    echo "  agent         Run agent locally"
    echo "  server        Run control plane locally"
    echo "  test          Run tests"
    echo "  ui            Run UI development server"
    echo "  ui-build      Build UI for production"
    echo "  add-target    Add a test target (usage: add-target <ip> [tier])"
    echo "  list-targets  List all targets"
    echo "  list-agents   List all agents"
    echo "  health        Check API health"
    ;;
esac
