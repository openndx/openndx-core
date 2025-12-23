#!/bin/bash
set -e

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${BLUE}==>${NC} $1"; }
error() { echo -e "${RED}❌ $1${NC}"; exit 1; }
success() { echo -e "${GREEN}✅ $1${NC}"; }
warn() { echo -e "${YELLOW}⚠️  $1${NC}"; }

echo "=========================================="
echo "  Observability Setup"
echo "=========================================="

# Check Docker
docker info > /dev/null 2>&1 || error "Docker is not running"

# Create network if needed
docker network create openndx-network 2>/dev/null || true

# Start observability stack
log "Starting Prometheus & Grafana..."
cd "$(dirname "$0")"
docker compose up -d

# Start exchange services
log "Starting exchange services..."
cd ../exchange
export DB_PASSWORD=${DB_PASSWORD:-password}
export DB_HOST=${DB_HOST:-localhost}  # Required for orchestration-engine

# Start databases
docker compose up -d pdp-db ce-db
log "Waiting for databases..."
sleep 5

# Start services
docker compose up -d orchestration-engine policy-decision-point consent-engine
log "Waiting for services to start..."
sleep 10

# Generate some traffic
log "Generating traffic..."
for i in {1..3}; do
    curl -s http://localhost:4000/health > /dev/null || true
    curl -s http://localhost:8081/health > /dev/null || true
    curl -s http://localhost:8082/health > /dev/null || true
    sleep 1
done

# Step 5: Verify Prometheus Scraping
log "Step 5: Checking Prometheus target health..."
if command -v jq > /dev/null 2>&1; then
    TARGETS=$(curl -s http://localhost:9091/api/v1/targets)
    # Filter for targets that are 'up'
    HEALTHY_COUNT=$(echo "$TARGETS" | jq '.data.activeTargets[] | select(.health=="up") | .health' | wc -l)
    TOTAL_COUNT=$(echo "$TARGETS" | jq '.data.activeTargets[] | .health' | wc -l)
    
    if [ "$HEALTHY_COUNT" -eq "$TOTAL_COUNT" ] && [ "$TOTAL_COUNT" -gt 0 ]; then
        success "Prometheus is successfully scraping all $TOTAL_COUNT targets."
    else
        warn "Prometheus targets: $HEALTHY_COUNT/$TOTAL_COUNT are UP."
        echo "$TARGETS" | jq -r '.data.activeTargets[] | "[\(.health)] \(.labels.job) -> \(.scrapeUrl)"'
    fi
else
    warn "Install 'jq' for detailed Prometheus target verification."
    curl -s http://localhost:9091/api/v1/targets | head -n 5
fi

echo ""
echo "=========================================="
success "Setup Complete!"
echo "=========================================="
echo ""
echo "Prometheus Targets: http://localhost:9091/targets"
echo "Prometheus Graph:   http://localhost:9091/graph"
echo "Grafana Dashboard:  http://localhost:3002/d/go-services-dashboard/go-services-metrics"
echo ""
