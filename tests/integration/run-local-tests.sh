#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Lanka Software Foundation


# Integration Tests Local Runner
# This script mimics the GitHub Actions workflow for integration tests

set -e  # Exit on error

cd "$(dirname "$0")"

echo "========================================"
echo "Integration Tests Local Runner"
echo "========================================"
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check if Docker is running
echo "🔍 Checking Docker daemon..."
if ! docker info > /dev/null 2>&1; then
    echo -e "${RED}❌ Docker is not running. Please start Docker Desktop and try again.${NC}"
    exit 1
fi
echo -e "${GREEN}✅ Docker is running${NC}"
echo ""

# Check Go version
echo "🔍 Checking Go version..."
go version
echo ""

# Install test dependencies
echo "📦 Installing test dependencies..."
go mod download
go mod tidy
echo -e "${GREEN}✅ Dependencies installed${NC}"
echo ""

# Check for unclean Go Mod (skip if not in git repo or SKIP_GO_MOD_CHECK is set)
if [ -z "$SKIP_GO_MOD_CHECK" ] && git rev-parse --git-dir > /dev/null 2>&1; then
    echo "🔍 Checking for unclean Go mod files..."
    if ! git diff --exit-code go.mod go.sum > /dev/null 2>&1; then
        echo -e "${RED}❌ go.mod or go.sum files have uncommitted changes.${NC}"
        echo "Please run 'go mod tidy' and commit the changes."
        echo "Or set SKIP_GO_MOD_CHECK=1 to skip this check."
        git diff go.mod go.sum
        exit 1
    fi
    echo -e "${GREEN}✅ Go mod files are clean${NC}"
    echo ""
fi

# Start Docker Compose services
echo "🚀 Starting Docker Compose services..."
docker compose -f docker-compose.db.yml -f docker-compose.test.yml up -d --build
echo ""

echo "📋 Checking service status..."
docker compose -f docker-compose.db.yml -f docker-compose.test.yml ps
echo ""

# Wait for services to be healthy
echo "⏳ Waiting for all services to be healthy..."
echo ""

check_health() {
    local service_name=$1
    local health_url=$2
    local max_attempts=30
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        if curl -f -s $health_url > /dev/null 2>&1; then
            echo -e "${GREEN}✅ $service_name is healthy${NC}"
            return 0
        fi
        echo "  ⏳ Waiting for $service_name... (attempt $attempt/$max_attempts)"
        sleep 2
        attempt=$((attempt + 1))
    done
    
    echo -e "${RED}❌ $service_name failed to become healthy${NC}"
    echo "Service logs:"
    docker compose -f docker-compose.db.yml -f docker-compose.test.yml logs $service_name
    return 1
}

# Wait for databases first
echo "Waiting for databases to initialize..."
sleep 10
echo ""

# Check each service health endpoint
check_health "policy-decision-point" "http://localhost:8082/health" || exit 1
check_health "consent-engine" "http://localhost:8081/health" || exit 1
check_health "orchestration-engine" "http://localhost:4000/health" || exit 1

echo ""
echo -e "${GREEN}🎉 All services are healthy and ready!${NC}"
echo ""

# Run integration tests
echo "🧪 Running integration tests..."
echo "========================================"

if go test -v -race -timeout 30m ./...; then
    echo ""
    echo "========================================"
    echo -e "${GREEN}✅ All integration tests passed!${NC}"
    echo "========================================"
    TEST_RESULT=0
else
    echo ""
    echo "========================================"
    echo -e "${RED}❌ Integration tests failed${NC}"
    echo "========================================"
    echo ""
    echo "=== Docker Compose Services Status ==="
    docker compose -f docker-compose.db.yml -f docker-compose.test.yml ps
    echo ""
    echo "=== All Service Logs ==="
    docker compose -f docker-compose.db.yml -f docker-compose.test.yml logs
    TEST_RESULT=1
fi

# Cleanup
echo ""
echo "🧹 Cleaning up test infrastructure..."
docker compose -f docker-compose.db.yml -f docker-compose.test.yml down -v
echo -e "${GREEN}✅ Cleanup complete${NC}"

exit $TEST_RESULT
