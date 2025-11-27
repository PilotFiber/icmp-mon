#!/bin/bash
# ICMP-Mon Staging Deployment Script
# Run this on the staging server to deploy or update the application
#
# Usage: ./deploy-staging.sh [--build-ui] [--restart-only]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"
DEPLOY_DIR="$PROJECT_DIR/deploy"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

BUILD_UI=false
RESTART_ONLY=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --build-ui)
            BUILD_UI=true
            shift
            ;;
        --restart-only)
            RESTART_ONLY=true
            shift
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

cd "$PROJECT_DIR"

# Check for .env file
if [[ ! -f "$DEPLOY_DIR/.env" ]]; then
    log_error ".env file not found at $DEPLOY_DIR/.env"
    log_info "Copy .env.example to .env and fill in values"
    exit 1
fi

if [[ "$RESTART_ONLY" == "true" ]]; then
    log_info "Restarting services..."
    cd "$DEPLOY_DIR"
    docker compose -f docker-compose.staging.yml restart
    log_info "Services restarted"
    exit 0
fi

# Pull latest code
log_info "Pulling latest code..."
git pull origin main

# Build UI if requested or if dist doesn't exist
if [[ "$BUILD_UI" == "true" ]] || [[ ! -d "$PROJECT_DIR/ui/dist" ]]; then
    log_info "Building UI..."
    cd "$PROJECT_DIR/ui"
    npm install
    npm run build
    cd "$PROJECT_DIR"
fi

# Deploy with docker compose
log_info "Deploying services..."
cd "$DEPLOY_DIR"
docker compose -f docker-compose.staging.yml up -d --build

# Wait for services to be healthy
log_info "Waiting for services to be healthy..."
sleep 5

# Health check
log_info "Running health check..."
HEALTH_URL="http://localhost:8080/api/v1/health"
if curl -sf "$HEALTH_URL" > /dev/null; then
    log_info "Health check passed!"
else
    log_warn "Health check failed - check logs with: docker compose -f docker-compose.staging.yml logs"
fi

# Show status
log_info "Current container status:"
docker compose -f docker-compose.staging.yml ps

log_info "Deployment complete!"
