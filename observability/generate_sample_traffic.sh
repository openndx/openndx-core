#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Lanka Software Foundation


# Script to generate sample traffic for Grafana dashboard
# This sends requests to various endpoints to populate metrics
# Targets: Orchestration Engine (port 4000) and Policy Decision Point (port 8082)

ORCHESTRATION_ENGINE_URL="${ORCHESTRATION_ENGINE_URL:-http://localhost:4000}"
POLICY_DECISION_POINT_URL="${POLICY_DECISION_POINT_URL:-http://localhost:8082}"
INTERVAL="${REQUEST_INTERVAL:-2}"  # seconds between requests
COUNT="${REQUEST_COUNT:-50}"        # number of requests per endpoint (0 = infinite)

echo "=========================================="
echo "Generating Sample Traffic for Grafana"
echo "=========================================="
echo "Orchestration Engine: $ORCHESTRATION_ENGINE_URL"
echo "Policy Decision Point: $POLICY_DECISION_POINT_URL"
echo "Interval: ${INTERVAL}s"
echo "Count: ${COUNT} (0 = infinite)"
echo ""
echo "Press Ctrl+C to stop"
echo "=========================================="
echo ""

# Function to send a request and show status
send_request() {
    local base_url=$1
    local method=$2
    local endpoint=$3
    local data=$4
    local description=$5
    
    if [ -n "$data" ]; then
        response=$(curl -s -w "\n%{http_code}" -X "$method" \
            -H "Content-Type: application/json" \
            -d "$data" \
            "$base_url$endpoint" 2>/dev/null)
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" \
            "$base_url$endpoint" 2>/dev/null)
    fi
    
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
        status="✓"
    elif [ "$http_code" -ge 400 ] && [ "$http_code" -lt 500 ]; then
        status="⚠"
    else
        status="✗"
    fi
    
    echo "[$status] $base_url$endpoint ($method) -> $http_code"
}

# Function to run requests in a loop
run_requests() {
    local i=0
    while [ $COUNT -eq 0 ] || [ $i -lt $COUNT ]; do
        echo ""
        echo "--- Batch $((i+1)) ---"
        
        # Orchestration Engine endpoints
        echo ">>> Orchestration Engine ($ORCHESTRATION_ENGINE_URL)"
        send_request "$ORCHESTRATION_ENGINE_URL" "GET" "/health" "" "Health check"
        sleep 0.5
        send_request "$ORCHESTRATION_ENGINE_URL" "GET" "/metrics" "" "Metrics endpoint"
        sleep 0.5
        
        # GraphQL endpoint (will generate errors without proper query, but useful for metrics)
        send_request "$ORCHESTRATION_ENGINE_URL" "POST" "/graphql" '{"query":"{ __typename }"}' "GraphQL query"
        sleep 0.5
        send_request "$ORCHESTRATION_ENGINE_URL" "POST" "/graphql" '{"invalid": "query"}' "Invalid GraphQL"
        sleep 0.5
        
        # Invalid endpoints (404s)
        send_request "$ORCHESTRATION_ENGINE_URL" "GET" "/unknown" "" "Unknown endpoint"
        sleep 0.5
        
        # Policy Decision Point endpoints
        echo ">>> Policy Decision Point ($POLICY_DECISION_POINT_URL)"
        send_request "$POLICY_DECISION_POINT_URL" "GET" "/health" "" "Health check"
        sleep 0.5
        send_request "$POLICY_DECISION_POINT_URL" "GET" "/metrics" "" "Metrics endpoint"
        sleep 0.5
        
        # Policy decision endpoint (will generate errors without proper data, but useful for metrics)
        send_request "$POLICY_DECISION_POINT_URL" "POST" "/api/v1/policy/decide" '{"consumer_id":"test-app","app_id":"test-app","request_id":"req_123","required_fields":["person.name"]}' "Policy decision"
        sleep 0.5
        send_request "$POLICY_DECISION_POINT_URL" "POST" "/api/v1/policy/fields" '{"field_name":"person.name","schema_id":"test-schema"}' "Field policies"
        sleep 0.5
        
        # Invalid endpoints (404s)
        send_request "$POLICY_DECISION_POINT_URL" "GET" "/api/v1/unknown" "" "Unknown endpoint"
        sleep 0.5
        
        # Invalid JSON (400s)
        send_request "$POLICY_DECISION_POINT_URL" "POST" "/api/v1/policy/decide" '{"invalid": }' "Invalid JSON"
        sleep 0.5
        
        i=$((i+1))
        
        if [ $COUNT -eq 0 ] || [ $i -lt $COUNT ]; then
            echo ""
            echo "Waiting ${INTERVAL}s before next batch..."
            sleep $INTERVAL
        fi
    done
}

# Check if services are accessible
echo "Checking if services are accessible..."

oe_accessible=false
pdp_accessible=false

if curl -s -f "$ORCHESTRATION_ENGINE_URL/health" > /dev/null 2>&1; then
    echo "✓ Orchestration Engine is accessible at $ORCHESTRATION_ENGINE_URL"
    oe_accessible=true
else
    echo "⚠ WARNING: Cannot reach Orchestration Engine at $ORCHESTRATION_ENGINE_URL"
    echo "   Try: curl $ORCHESTRATION_ENGINE_URL/health"
fi

if curl -s -f "$POLICY_DECISION_POINT_URL/health" > /dev/null 2>&1; then
    echo "✓ Policy Decision Point is accessible at $POLICY_DECISION_POINT_URL"
    pdp_accessible=true
else
    echo "⚠ WARNING: Cannot reach Policy Decision Point at $POLICY_DECISION_POINT_URL"
    echo "   Try: curl $POLICY_DECISION_POINT_URL/health"
fi

if [ "$oe_accessible" = false ] && [ "$pdp_accessible" = false ]; then
    echo ""
    echo "❌ ERROR: Neither service is accessible. Cannot generate traffic."
    echo "   Make sure at least one service is running and accessible."
    exit 1
fi

echo ""

# Start generating traffic
run_requests

echo ""
echo "=========================================="
echo "Traffic generation complete!"
echo "=========================================="
echo ""
echo "View metrics in Grafana:"
echo "  http://localhost:3002/d/go-services/go-services-metrics"
echo ""
echo "Or check Prometheus directly:"
echo "  http://localhost:9091"
echo ""

