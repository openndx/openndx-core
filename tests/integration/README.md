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
docker compose -f docker-compose.db.yml -f docker-compose.test.yml up -d --build
go test -v ./...
docker compose -f docker-compose.db.yml -f docker-compose.test.yml down -v
```

`docker-compose.test.yml` alone is not a valid project. The database services it depends on are in `docker-compose.db.yml`.

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
├── oe-config.yaml              # Orchestration Engine config
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
└── mock-provider/              # Mock provider service and consumer token issuer
    ├── Dockerfile
    └── main.go
```

---

## Test Scenarios

The PDP and CE database containers initialize their schemas from
`../../cmd/pdp/schema.sql` and `../../cmd/ce/schema.sql`, the same files used by
the root development stack.

### GraphQL Flow Tests

**`TestGraphQLFlow_SuccessPath`** - Complete success path:
- Creates a field policy in PDP
- Adds application to allowlist
- Creates consent record
- Executes GraphQL query
- Verifies PDP and Consent Engine integration

**`TestGraphQLFlow_MissingFieldPolicy`** - Tests behavior when field lacks a field policy (expects authorization error)

**`TestGraphQLFlow_UnauthorizedApp`** - Tests behavior when app has no consent (expects consent error)

**`TestGraphQLFlow_ServiceTimeout`** - Tests resilience when PDP is unavailable

**`TestGraphQLFlow_InvalidQuery`** - Tests malformed GraphQL query handling

**`TestGraphQLFlow_MissingToken`** - Tests authentication failure (missing JWT)

**`TestGraphQLFlow_RejectedTokens`** - Tests that the Orchestration Engine rejects unsigned, expired, wrong-issuer and wrong-audience tokens with `401`

Consumer tokens are signed by the mock provider (`POST /token`), which also serves the JWKS the Orchestration Engine verifies them against (`GET /jwks`). The Orchestration Engine's `idp` settings in `oe-config.yaml` point `jwksURL` at it, with issuer `http://mock-idp.test` and audience `http://api.openndx.local`.

### Service Health Tests

**`TestPortalBackend_Health`** - Verifies Portal Backend health endpoint

---

## Configuration

### Environment Variables

**Database credentials:**

`docker-compose.db.yml` sets these directly. They are not read from the environment.

| Database | Host port | Name | User | Password |
|---|---|---|---|---|
| `pdp-db` | 5433 | `policy_db` | `postgres` | `password` |
| `ce-db` | 5434 | `consent_db` | `postgres` | `password` |

Tests that open a database themselves need `POSTGRES_PASSWORD=password`. For the Policy Decision Point database also set `TEST_DB_PORT=5433`.

**Service URLs (Optional):**
- `ORCHESTRATION_ENGINE_URL` - Default: `http://127.0.0.1:4000/public/graphql`
- `PDP_URL` - Default: `http://127.0.0.1:8082/api/v1/policy`
- `CONSENT_ENGINE_URL` - Default: `http://127.0.0.1:8081/consents`
- `PORTAL_BACKEND_URL` - Default: `http://127.0.0.1:3000`

**Setting Variables:**
```bash
export POSTGRES_PASSWORD=password
export TEST_DB_PORT=5433
go test ./...
```

**Security notes:**
- Never commit `.env` files or credentials
- These database passwords are for the local test databases only

---

## Running Tests

### Run All Tests
```bash
docker compose -f docker-compose.db.yml -f docker-compose.test.yml up -d --build
go test -v ./...
docker compose -f docker-compose.db.yml -f docker-compose.test.yml down -v
```

### Run Specific Test
```bash
go test -v -run TestGraphQLFlow_SuccessPath
go test -v -run TestGraphQLFlow_MissingFieldPolicy
```

### Run with Verbose Output
```bash
go test -v ./...
```

---

## Test Services

`docker-compose.db.yml` and `docker-compose.test.yml` together start:

- **pdp-db** (host port 5433) — PostgreSQL for the Policy Decision Point, database `policy_db`
- **ce-db** (host port 5434) — PostgreSQL for the Consent Engine, database `consent_db`
- **Policy Decision Point** (8082)
- **Consent Engine** (8081)
- **Orchestration Engine** (4000)
- **Mock provider** (8083) — sample data, and the token issuer for the tests

There is no Portal Backend in this stack. The mock provider uses host port 8083.

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
docker compose -f docker-compose.db.yml -f docker-compose.test.yml logs

# Verify services are healthy
docker compose -f docker-compose.db.yml -f docker-compose.test.yml ps
```

**Database connection errors:**
- Set `POSTGRES_PASSWORD=password` to match `docker-compose.db.yml`
- For the Policy Decision Point database, set `TEST_DB_PORT=5433`
- Check the databases: `docker compose -f docker-compose.db.yml -f docker-compose.test.yml ps pdp-db ce-db`

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
- ✅ Field policy and allowlist management
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
