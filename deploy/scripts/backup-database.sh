#!/bin/bash
# ICMP-Mon Database Backup Script
# Creates a compressed backup of the TimescaleDB database
#
# Usage: ./backup-database.sh
#
# Add to crontab for daily backups:
#   0 2 * * * /opt/icmp-mon/deploy/scripts/backup-database.sh >> /var/log/icmpmon-backup.log 2>&1

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(dirname "$SCRIPT_DIR")"
BACKUP_DIR="${BACKUP_DIR:-$DEPLOY_DIR/backups}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/icmpmon_${TIMESTAMP}.sql.gz"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "[$(date '+%Y-%m-%d %H:%M:%S')] ${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "[$(date '+%Y-%m-%d %H:%M:%S')] ${YELLOW}[WARN]${NC} $1"; }

# Ensure backup directory exists
mkdir -p "${BACKUP_DIR}"

log_info "Starting database backup..."

# Load environment variables
if [[ -f "$DEPLOY_DIR/.env" ]]; then
    source "$DEPLOY_DIR/.env"
fi

DB_USER="${DB_USER:-icmpmon}"
DB_NAME="${DB_NAME:-icmpmon}"

# Check if container is running
if ! docker ps --format '{{.Names}}' | grep -q 'icmpmon-db'; then
    log_warn "Database container (icmpmon-db) is not running"
    exit 1
fi

# Create backup using pg_dump
# Exclude TimescaleDB internal tables for cleaner restore
log_info "Creating backup: ${BACKUP_FILE}"
docker exec icmpmon-db pg_dump \
    -U "${DB_USER}" \
    -d "${DB_NAME}" \
    --no-owner \
    --no-acl \
    --exclude-table='_timescaledb_internal.*' \
    | gzip > "${BACKUP_FILE}"

# Get backup size
BACKUP_SIZE=$(du -h "${BACKUP_FILE}" | cut -f1)
log_info "Backup created: ${BACKUP_FILE} (${BACKUP_SIZE})"

# Cleanup old backups
log_info "Cleaning up backups older than ${RETENTION_DAYS} days..."
DELETED_COUNT=$(find "${BACKUP_DIR}" -name "icmpmon_*.sql.gz" -mtime "+${RETENTION_DAYS}" -delete -print | wc -l)
if [[ "$DELETED_COUNT" -gt 0 ]]; then
    log_info "Deleted ${DELETED_COUNT} old backup(s)"
fi

# Optional: Upload to S3 if AWS credentials are configured
if [[ -n "${AWS_ACCESS_KEY_ID:-}" ]] && [[ -n "${BACKUP_S3_BUCKET:-}" ]]; then
    log_info "Uploading to S3: s3://${BACKUP_S3_BUCKET}/$(basename ${BACKUP_FILE})"
    aws s3 cp "${BACKUP_FILE}" "s3://${BACKUP_S3_BUCKET}/$(basename ${BACKUP_FILE})"
    log_info "S3 upload complete"
fi

log_info "Backup complete!"
