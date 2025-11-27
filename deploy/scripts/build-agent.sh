#!/bin/bash
# ICMP-Mon Agent Build Script
# Cross-compiles the agent binary for Linux AMD64
#
# Usage: ./build-agent.sh [--output DIR]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"
OUTPUT_DIR="${PROJECT_DIR}/build"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Colors for output
GREEN='\033[0;32m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }

cd "$PROJECT_DIR"

# Ensure output directory exists
mkdir -p "$OUTPUT_DIR"

# Get version info
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

log_info "Building ICMP-Mon Agent"
log_info "Version: ${VERSION}"
log_info "Commit: ${COMMIT}"
log_info "Output: ${OUTPUT_DIR}"

# Build for Linux AMD64
log_info "Building for linux/amd64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o "${OUTPUT_DIR}/icmpmon-agent-linux-amd64" \
    ./agent/cmd/agent

# Build for Linux ARM64 (optional, for Raspberry Pi, etc.)
log_info "Building for linux/arm64..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
    -ldflags "-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o "${OUTPUT_DIR}/icmpmon-agent-linux-arm64" \
    ./agent/cmd/agent

# Generate checksums
log_info "Generating checksums..."
cd "$OUTPUT_DIR"
sha256sum icmpmon-agent-* > checksums.txt

log_info "Build complete!"
log_info "Binaries:"
ls -lh icmpmon-agent-*
log_info ""
log_info "Checksums:"
cat checksums.txt
