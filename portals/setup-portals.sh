#!/bin/bash

# Comprehensive portal setup and validation script
# Creates config.js files and validates configuration for all portals
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Test configuration values (can be overridden via environment variables)
TEST_API_URL="${TEST_API_URL:-http://localhost:3000}"
TEST_LOGS_URL="${TEST_LOGS_URL:-http://localhost:3001}"
TEST_CLIENT_ID="${TEST_CLIENT_ID:-test-client-id-123}"
TEST_BASE_URL="${TEST_BASE_URL:-https://api.asgardeo.io/t/test-org}"
TEST_SCOPE="${TEST_SCOPE:-openid profile}"
TEST_SIGN_IN_REDIRECT="${TEST_SIGN_IN_REDIRECT:-http://localhost:5173}"
TEST_SIGN_OUT_REDIRECT="${TEST_SIGN_OUT_REDIRECT:-http://localhost:5173}"
TEST_ADMIN_ROLE="${TEST_ADMIN_ROLE:-admin}"

# Function to get portal port (bash 3.2 compatible)
get_portal_port() {
    case "$1" in
        "admin-portal") echo "5174" ;;
        "consent-portal") echo "5175" ;;
        "member-portal") echo "5176" ;;
        *) echo "5173" ;;
    esac
}

echo -e "${GREEN}=== Portal Setup and Validation ===${NC}\n"

# Function to create config.js for a portal
create_config() {
    local portal_name=$1
    local config_path=$2
    local config_content=$3
    
    echo -e "${YELLOW}Creating config.js for ${portal_name}...${NC}"
    mkdir -p "$(dirname "$config_path")"
    echo "$config_content" > "$config_path"
    echo -e "${GREEN}✓ Created ${config_path}${NC}"
}

# Function to verify portal configuration
verify_portal() {
    local portal_name=$1
    local config_path=$2
    local html_path=$3
    local port=$4
    
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}Verifying ${portal_name}${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
    
    # Check if config.js exists
    if [ ! -f "$config_path" ]; then
        echo -e "${RED}✗ ERROR: ${config_path} not found${NC}\n"
        return 1
    fi
    echo -e "${GREEN}✓ Config file exists${NC}"
    
    # Check if HTML references config.js correctly
    if grep -q "config.js" "$html_path" 2>/dev/null; then
        echo -e "${GREEN}✓ HTML references config.js${NC}"
        local ref=$(grep "config.js" "$html_path" | head -1 | sed 's/^[[:space:]]*//')
        echo -e "  ${BLUE}→${NC} $ref"
    else
        echo -e "${RED}✗ ERROR: HTML does not reference config.js${NC}"
        return 1
    fi
    
    # Check if config.js is readable
    if [ -r "$config_path" ]; then
        echo -e "${GREEN}✓ Config file is readable${NC}"
    else
        echo -e "${RED}✗ ERROR: Config file is not readable${NC}"
        return 1
    fi
    
    # Check dependencies
    if [ ! -d "${portal_name}/node_modules" ]; then
        echo -e "${YELLOW}⚠ Dependencies not installed${NC}"
        echo -e "  Run: cd ${portal_name} && npm install"
    else
        echo -e "${GREEN}✓ Dependencies installed${NC}"
    fi
    
    # Display config content
    echo -e "${YELLOW}Config content:${NC}"
    cat "$config_path" | sed 's/^/  /'
    echo ""
    
    # Check for required variables based on portal
    case "$portal_name" in
        "admin-portal")
            required_vars=("VITE_API_URL" "VITE_LOGS_URL" "VITE_IDP_CLIENT_ID" "VITE_IDP_BASE_URL" "VITE_IDP_SCOPE" "VITE_IDP_ADMIN_ROLE")
            ;;
        "consent-portal")
            required_vars=("consentEngineUrl" "idpClientId" "idpBaseUrl" "idpScope" "idpSignInRedirectUrl" "idpSignOutRedirectUrl")
            ;;
        "member-portal")
            required_vars=("API_URL" "LOGS_URL" "CLIENT_ID" "BASE_URL" "SCOPE")
            ;;
    esac
    
    echo -e "${YELLOW}Checking required variables:${NC}"
    local all_present=true
    for var in "${required_vars[@]}"; do
        if grep -q "$var" "$config_path"; then
            echo -e "${GREEN}  ✓ ${var}${NC}"
        else
            echo -e "${RED}  ✗ ${var} (missing)${NC}"
            all_present=false
        fi
    done
    
    if [ "$all_present" = true ]; then
        echo -e "${GREEN}✓ All required variables present${NC}"
    else
        echo -e "${RED}✗ Some required variables are missing${NC}"
    fi
    
    echo ""
    echo -e "${YELLOW}To test this portal:${NC}"
    echo -e "  cd ${portal_name}"
    echo -e "  VITE_PORT=${port} npm run dev"
    echo -e "  Open http://localhost:${port}"
    echo -e "  Check browser console for 'Window configs:' log"
    echo ""
}

# 1. Admin Portal
ADMIN_CONFIG_PATH="admin-portal/public/config.js"
ADMIN_CONFIG_CONTENT="window.configs = {
  VITE_API_URL: '${TEST_API_URL}',
  VITE_LOGS_URL: '${TEST_LOGS_URL}',
  VITE_IDP_CLIENT_ID: '${TEST_CLIENT_ID}',
  VITE_IDP_BASE_URL: '${TEST_BASE_URL}',
  VITE_IDP_SCOPE: '${TEST_SCOPE}',
  VITE_IDP_ADMIN_ROLE: '${TEST_ADMIN_ROLE}',
  VITE_SIGN_IN_REDIRECT_URL: '${TEST_SIGN_IN_REDIRECT}',
  VITE_SIGN_OUT_REDIRECT_URL: '${TEST_SIGN_OUT_REDIRECT}'
};"

create_config "Admin Portal" "$ADMIN_CONFIG_PATH" "$ADMIN_CONFIG_CONTENT"
verify_portal "admin-portal" "$ADMIN_CONFIG_PATH" "admin-portal/index.html" "$(get_portal_port admin-portal)"

# 2. Consent Portal
CONSENT_CONFIG_PATH="consent-portal/public/config.js"
CONSENT_CONFIG_CONTENT="window.configs = {
  consentEngineUrl: '${TEST_API_URL}',
  idpClientId: '${TEST_CLIENT_ID}',
  idpBaseUrl: '${TEST_BASE_URL}',
  idpScope: '${TEST_SCOPE}',
  idpSignInRedirectUrl: '${TEST_SIGN_IN_REDIRECT}',
  idpSignOutRedirectUrl: '${TEST_SIGN_OUT_REDIRECT}'
};"

create_config "Consent Portal" "$CONSENT_CONFIG_PATH" "$CONSENT_CONFIG_CONTENT"
verify_portal "consent-portal" "$CONSENT_CONFIG_PATH" "consent-portal/index.html" "$(get_portal_port consent-portal)"

# 3. Member Portal
MEMBER_CONFIG_PATH="member-portal/public/config.js"
MEMBER_CONFIG_CONTENT="window.configs = {
  API_URL: '${TEST_API_URL}',
  LOGS_URL: '${TEST_LOGS_URL}',
  CLIENT_ID: '${TEST_CLIENT_ID}',
  BASE_URL: '${TEST_BASE_URL}',
  SCOPE: '${TEST_SCOPE}',
  SIGN_IN_REDIRECT_URL: '${TEST_SIGN_IN_REDIRECT}',
  SIGN_OUT_REDIRECT_URL: '${TEST_SIGN_OUT_REDIRECT}'
};"

create_config "Member Portal" "$MEMBER_CONFIG_PATH" "$MEMBER_CONFIG_CONTENT"
verify_portal "member-portal" "$MEMBER_CONFIG_PATH" "member-portal/index.html" "$(get_portal_port member-portal)"

# Summary
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}=== Setup Complete ===${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"

echo -e "${GREEN}✓ All config.js files created and validated!${NC}\n"

echo -e "${YELLOW}Quick Test Commands:${NC}\n"
echo -e "${BLUE}# Admin Portal${NC}"
echo -e "  cd admin-portal && VITE_PORT=5174 npm run dev\n"
echo -e "${BLUE}# Consent Portal${NC}"
echo -e "  cd consent-portal && VITE_PORT=5175 npm run dev\n"
echo -e "${BLUE}# Member Portal${NC}"
echo -e "  cd member-portal && VITE_PORT=5176 npm run dev\n"

echo -e "${YELLOW}Verification Checklist:${NC}"
echo -e "  [ ] All portals show 'Window configs:' in browser console"
echo -e "  [ ] All expected variables are present"
echo -e "  [ ] No 'undefined' values in config objects"
echo -e "  [ ] Auth configuration initializes correctly\n"

