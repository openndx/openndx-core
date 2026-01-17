# Integration Tests

End-to-end integration tests for the Data Exchange Platform covering GraphQL workflows, consent management, and policy decisions.

---

## Quick Start

### Option 1: Automated Script (Recommended)

Run the automated test script that mimics the CI/CD workflow:

```bash
cd tests/integration
./run-local-tests.sh
```

This script:
- Checks Docker and Go are available
- Installs dependencies
- Starts all services
- Waits for services to be healthy
- Runs tests with race detection
- Cleans up automatically

**Skip go.mod check for local development:**
```bash
SKIP_GO_MOD_CHECK=1 ./run-local-tests.sh
```

### Option 2: Manual Steps

```bash
cd tests/integration
docker compose -f docker-compose.test.yml up -d
go test -v ./...
docker compose -f docker-compose.test.yml down -v
```

**Prerequisites:** Docker and Docker Compose installed.

---

## Test Structure

### Directory Structure

```
tests/integration/
├── README.md                    # This file
├── docker-compose.test.yml     # Docker Compose configuration for tests
├── docker-compose.db.yml       # Database-only Docker Compose configuration
├── go.mod                      # Go module definition
├── go.sum                      # Go module checksums
├── run-local-tests.sh          # Automated test script
├── schema.graphql              # GraphQL schema for tests
├── config.json                 # Orchestration Engine config
├── init-consent-db.sql         # Consent database initialization script
├── init-stats.sh               # Statistics initialization script
├── graphql_flow_test.go        # GraphQL workflow tests
├── services_integration_test.go # Service health checks
├── audit_flow_integration_test.go # Audit flow integration tests
├── audit_traceid_test.go       # Audit trace ID tests
├── consent/                    # Consent Engine integration tests
│   └── consent_test.go
├── policy/                     # Policy Decision Point integration tests
│   └── policy_test.go
├── database/                  # Database connection integration tests
│   ├── consent_database_test.go # Consent Engine database tests
│   └── pdp_database_test.go    # Policy Decision Point database tests
├── testutils/                  # Test utilities
│   ├── db.go                   # Database helpers
│   └── http.go                 # HTTP client helpers
├── bin/                        # Compiled binaries (not committed)
│   └── consent-engine
└── mock-provider/              # Mock provider service
    ├── Dockerfile
    └── main.go
```

---

## Test Scenarios

### GraphQL Flow Tests

**`TestGraphQLFlow_SuccessPath`** - Complete success path:
- Creates policy metadata in PDP
- Adds application to allowlist
- Creates consent record
- Executes GraphQL query
- Verifies PDP and Consent Engine integration

**`TestGraphQLFlow_MissingPolicyMetadata`** - Tests behavior when field lacks policy metadata (expects authorization error)

**`TestGraphQLFlow_UnauthorizedApp`** - Tests behavior when app has no consent (expects consent error)

**`TestGraphQLFlow_ServiceTimeout`** - Tests resilience when PDP is unavailable

**`TestGraphQLFlow_InvalidQuery`** - Tests malformed GraphQL query handling

**`TestGraphQLFlow_MissingToken`** - Tests authentication failure (missing JWT)

### Service Health Tests

**`TestPortalBackend_Health`** - Verifies Portal Backend health endpoint

---

## Configuration

### Environment Variables

**Database Credentials:**
- `POSTGRES_PASSWORD` - Database password (default: `test-password-change-in-production` in `docker-compose.test.yml`; **override for security**)
- `POSTGRES_USER` - Database username (default: `postgres`)
- `POSTGRES_DB` - Database name (default: `postgres`)

> **Note:** The default `POSTGRES_PASSWORD` is for convenience in local testing only. Always set a strong password for production or CI environments.

**Service URLs (Optional):**
- `ORCHESTRATION_ENGINE_URL` - Default: `http://127.0.0.1:4000/public/graphql`
- `PDP_URL` - Default: `http://127.0.0.1:8082/api/v1/policy`
- `CONSENT_ENGINE_URL` - Default: `http://127.0.0.1:8081/consents`
- `PORTAL_BACKEND_URL` - Default: `http://127.0.0.1:3000`

**Setting Variables:**
```bash
# Export before running
export POSTGRES_PASSWORD=your-password
go test ./...

# Or inline
POSTGRES_PASSWORD=your-password go test ./...
```

**Security Notes:**
- Never commit `.env` files or credentials
- Test credentials should differ from production
- `docker-compose.test.yml` uses environment variable substitution

---

## Running Tests

### Run All Tests
```bash
docker compose -f docker-compose.test.yml up -d
go test -v ./...
docker compose -f docker-compose.test.yml down -v
```

### Run Specific Test
```bash
go test -v -run TestGraphQLFlow_SuccessPath
go test -v -run TestGraphQLFlow_MissingPolicyMetadata
```

### Run with Verbose Output
```bash
go test -v ./...
```

---

## Test Services

The `docker-compose.test.yml` starts:

- **PostgreSQL** (5432) - Shared database for all services
- **Policy Decision Point** (8082) - Policy evaluation service
- **Consent Engine** (8081) - Consent management service
- **Orchestration Engine** (4000) - GraphQL orchestration service
- **Portal Backend** (3000) - Admin portal backend

All services run on `test-network` Docker network.

---

## Test Data

Tests use unique identifiers (timestamp-based) for isolation:
- Schema IDs: `test-schema-123` (matches `schema.graphql`)
- App IDs: `test-consumer-app-{timestamp}`
- Test constants: `testNIC`, `testEmail`, `testOwnerID`

Tests automatically clean up created resources using `t.Cleanup()`.

---

## Troubleshooting

**Services not starting:**
```bash
# Check logs
docker compose -f docker-compose.test.yml logs

# Verify services are healthy
docker compose -f docker-compose.test.yml ps
```

**Database connection errors:**
- Verify `POSTGRES_PASSWORD` is set
- Check database is healthy: `docker compose -f docker-compose.test.yml ps shared-db`

**Port conflicts:**
```bash
# Check what's using ports
lsof -i :4000  # Orchestration Engine
lsof -i :8081  # Consent Engine
lsof -i :8082  # Policy Decision Point
lsof -i :5432  # PostgreSQL
```

**Test failures:**
- Ensure all services are healthy before running tests
- Check service logs for errors
- Verify GraphQL schema matches test expectations (`schema.graphql`)

---

## Test Coverage

Tests cover:
- ✅ Complete GraphQL request/response flow
- ✅ Policy metadata and allowlist management
- ✅ Consent creation and validation
- ✅ Authorization failures (missing metadata, unauthorized app)
- ✅ Service resilience (timeout scenarios)
- ✅ Invalid query handling
- ✅ Authentication (missing tokens)
- ✅ Service health checks

---

## Architecture

```
Test Runner (go test)
    ↓
Docker Compose Services
    ├── PostgreSQL (shared-db)
    ├── Policy Decision Point
    ├── Consent Engine
    ├── Orchestration Engine
    └── Portal Backend
    ↓
Test Utilities (testutils/)
    ├── HTTP client helpers
    └── Database helpers
```

---

## Contributing

When adding new tests:

1. **Use unique IDs** - Prevents test data conflicts
2. **Add cleanup** - Use `t.Cleanup()` for resource cleanup
3. **Use helpers** - Leverage `testutils` functions
4. **Document** - Add godoc comments to test functions
5. **Isolate** - Each test should be independent

---

## Resources

- [Go Testing Documentation](https://pkg.go.dev/testing)
- [Testify Documentation](https://github.com/stretchr/testify)
