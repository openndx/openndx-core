# Integration Tests

These tests run the Policy Decision Point, the Consent Engine and the Orchestration Engine together in Docker, and call them over HTTP. They are a separate Go module, so `make test` doesn't run them.

## Running the tests

From the repository root:

```bash
make test-integration
```

This runs [`run-local-tests.sh`](run-local-tests.sh), which:

1. checks that Docker is running, runs `go mod tidy`, and stops if this module's `go.mod` or `go.sum` then has uncommitted changes (`SKIP_GO_MOD_CHECK=1` skips that check);
2. starts the test stack with `docker compose -f docker-compose.db.yml -f docker-compose.test.yml up -d --build`, and waits until each service is healthy;
3. runs `go test -v -race -timeout 30m ./...`;
4. removes the containers and their volumes.

To run the steps yourself, from `tests/integration`:

```bash
docker compose -f docker-compose.db.yml -f docker-compose.test.yml up -d --build
go test -v ./...
docker compose -f docker-compose.db.yml -f docker-compose.test.yml down -v
```

`docker-compose.test.yml` isn't a valid project on its own: its services depend on the databases in `docker-compose.db.yml`.

Many tests skip, rather than fail, when a service they need doesn't answer, so check the skip messages.

## The test stack

| Service               | Host port | Notes                                                                                                                            |
| --------------------- | --------- | -------------------------------------------------------------------------------------------------------------------------------- |
| Policy Decision Point | 8082      | Configured by [`pdp-config.yaml`](pdp-config.yaml)                                                                               |
| Consent Engine        | 8081      | Configured by [`ce-config.yaml`](ce-config.yaml)                                                                                 |
| Orchestration Engine  | 4000      | Configured by [`oe-config.yaml`](oe-config.yaml), with the consumer schema [`schema.graphql`](schema.graphql)                    |
| Mock provider         | 8083      | [`mock-provider/`](mock-provider): a GraphQL provider, and the issuer of the tests' consumer tokens (`POST /token`, `GET /jwks`) |
| `pdp-db`              | 5433      | PostgreSQL database `policy_db`, user `postgres`, password `password`                                                            |
| `ce-db`               | 5434      | PostgreSQL database `consent_db`, user `postgres`, password `password`                                                           |

The databases load [`cmd/pdp/schema.sql`](../../cmd/pdp/schema.sql) and [`cmd/ce/schema.sql`](../../cmd/ce/schema.sql), the same files as the main stack. The Orchestration Engine verifies consumer tokens against the mock provider's keys, with issuer `http://mock-idp.test` and audience `http://api.openndx.local`. The Portal Backend isn't part of the stack, so `TestPortalBackend_Health` skips.

The stack uses some of the main stack's host ports, so stop that first with `make down`.

## Layout

- [`graphql_flow_test.go`](graphql_flow_test.go) and [`failure_paths_test.go`](failure_paths_test.go): queries through the Orchestration Engine, on the success path and when a field policy, grant, consent, token, service or provider is missing, expired, rejected or down.
- [`policy/`](policy) and [`consent/`](consent): the Policy Decision Point's and the Consent Engine's APIs, including their error paths.
- [`database/`](database): connections to the two test databases.
- [`services_integration_test.go`](services_integration_test.go): the Portal Backend health check.
- [`testutils/`](testutils): HTTP and database helpers.

## Settings

The tests find the services through these environment variables:

| Variable                   | Default                                |
| -------------------------- | -------------------------------------- |
| `ORCHESTRATION_ENGINE_URL` | `http://127.0.0.1:4000/public/graphql` |
| `PDP_URL`                  | `http://127.0.0.1:8082/api/v1/policy`  |
| `CONSENT_ENGINE_BASE_URL`  | `http://127.0.0.1:8081`                |
| `MOCK_IDP_URL`             | `http://127.0.0.1:8083`                |
| `PORTAL_BACKEND_URL`       | `http://127.0.0.1:3000`                |

The tests in `database/` connect to `pdp-db` and `ce-db` themselves. The database helper in [`testutils/db.go`](testutils/db.go) otherwise reads `TEST_DB_HOST`, `TEST_DB_PORT`, `TEST_DB_USERNAME`, `TEST_DB_DATABASE` and `TEST_DB_SSLMODE`, and the password from `TEST_DB_PASSWORD` or `POSTGRES_PASSWORD`.

Set `TEST_VERIFY_DB=true` to run `TestConsent_DatabaseVerification`, and `SKIP_DOCKER_COMPOSE_CHECK=true` to skip the Docker Compose check before the query tests.

## Troubleshooting

```bash
docker compose -f docker-compose.db.yml -f docker-compose.test.yml ps
docker compose -f docker-compose.db.yml -f docker-compose.test.yml logs <service>
```

If a port is taken, find what holds it with `lsof -i :<port>`, for 4000, 8081, 8082, 8083, 5433 and 5434.

## Writing tests

- Give test data unique IDs, so that tests don't collide.
- Clean up what a test creates with `t.Cleanup()`.
- Use the helpers in `testutils`.
- Keep each test independent of the others, and document it with a Go doc comment.
