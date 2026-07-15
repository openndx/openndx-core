#!/bin/bash

# Script to test Docker builds for all services
# This validates that all Dockerfiles can build successfully with the current context

set -e  # Exit on first error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Build metadata
BUILD_VERSION="test"
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "unknown")

# Array to track results
declare -a PASSED_SERVICES
declare -a FAILED_SERVICES

echo "================================================"
echo "Testing Docker builds for all services"
echo "Build Time: $BUILD_TIME"
echo "Git Commit: $GIT_COMMIT"
echo "================================================"
echo ""

# Function to build a service
build_service() {
    local service_name=$1
    local dockerfile_path=$2
    local service_path=$3
    
    echo "----------------------------------------"
    echo -e "${YELLOW}Building: $service_name${NC}"
    echo "Dockerfile: $dockerfile_path"
    echo "Context: . (repository root)"
    echo "----------------------------------------"
    
    if docker build \
        --file "$dockerfile_path" \
        --build-arg SERVICE_PATH="$service_path" \
        --build-arg BUILD_VERSION="$BUILD_VERSION" \
        --build-arg BUILD_TIME="$BUILD_TIME" \
        --build-arg GIT_COMMIT="$GIT_COMMIT" \
        --tag "opendif-core/$service_name:test" \
        . ; then
        echo -e "${GREEN}✓ $service_name build succeeded${NC}"
        PASSED_SERVICES+=("$service_name")
        return 0
    else
        echo -e "${RED}✗ $service_name build failed${NC}"
        FAILED_SERVICES+=("$service_name")
        return 1
    fi
}

# Test all services
echo "Starting Docker build tests..."
echo ""

# Keep track of overall status
OVERALL_STATUS=0

# Build orchestration-engine
if ! build_service "orchestration-engine" "exchange/orchestration-engine/Dockerfile" "exchange/orchestration-engine"; then
    OVERALL_STATUS=1
fi
echo ""

# Build consent-engine
if ! build_service "consent-engine" "exchange/consent-engine/Dockerfile" "exchange/consent-engine"; then
    OVERALL_STATUS=1
fi
echo ""

# Build policy-decision-point
if ! build_service "policy-decision-point" "exchange/policy-decision-point/Dockerfile" "exchange/policy-decision-point"; then
    OVERALL_STATUS=1
fi
echo ""

# Build portal-backend
if ! build_service "portal-backend" "portal-backend/Dockerfile" "portal-backend"; then
    OVERALL_STATUS=1
fi
echo ""

# Print summary
echo "================================================"
echo "BUILD SUMMARY"
echo "================================================"

if [ ${#PASSED_SERVICES[@]} -gt 0 ]; then
    echo -e "${GREEN}✓ Passed (${#PASSED_SERVICES[@]}):${NC}"
    for service in "${PASSED_SERVICES[@]}"; do
        echo "  - $service"
    done
    echo ""
fi

if [ ${#FAILED_SERVICES[@]} -gt 0 ]; then
    echo -e "${RED}✗ Failed (${#FAILED_SERVICES[@]}):${NC}"
    for service in "${FAILED_SERVICES[@]}"; do
        echo "  - $service"
    done
    echo ""
fi

echo "Total: $((${#PASSED_SERVICES[@]} + ${#FAILED_SERVICES[@]})) services tested"
echo "Passed: ${#PASSED_SERVICES[@]}"
echo "Failed: ${#FAILED_SERVICES[@]}"
echo "================================================"

# Exit with appropriate status
if [ $OVERALL_STATUS -eq 0 ]; then
    echo -e "${GREEN}All Docker builds succeeded!${NC}"
    exit 0
else
    echo -e "${RED}Some Docker builds failed!${NC}"
    exit 1
fi
