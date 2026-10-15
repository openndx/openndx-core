# Consent Engine

Manages data owner consent workflows for data access requests. Provides JWT-authenticated endpoints for user interactions and internal APIs for service-to-service communication.

## Quick Start

```bash
# Initialize the database as described below, then run from the repo root.
go run ./cmd/ce

# Or build and run
go build -o ce ./cmd/ce && ./ce
```

Service runs on port **8081** by default.

## Configuration

### Environment Variables

| Variable             | Description                                                             | Default                 |
|----------------------|-------------------------------------------------------------------------|-------------------------|
| `PORT`               | Service port                                                            | `8081`                  |
| `ENVIRONMENT`        | `production` or `local`                                                 | `local`                 |
| `CONSENT_PORTAL_URL` | Consent Portal URL                                                      | `http://localhost:5173` |
| `IDP_ORG_NAME`       | IDP organization name                                                   | -                       |
| `IDP_ISSUER`         | JWT issuer URL                                                          | -                       |
| `IDP_AUDIENCE`       | JWT audience                                                            | -                       |
| `IDP_JWKS_URL`       | JWKS endpoint URL                                                       | -                       |
| `IDP_SUBJECT_CLAIM`  | Token claim carrying the owner UID (matched against consent `owner_id`) | `sub`                   |
| `DB_HOST`            | Database host                                                           | `localhost`             |
| `DB_PORT`            | Database port                                                           | `5432`                  |
| `DB_USERNAME`        | Database username                                                       | `postgres`              |
| `DB_PASSWORD`        | Database password                                                       | -                       |
| `DB_NAME`            | Database name                                                           | `consent_engine`        |
| `DB_SSLMODE`         | SSL mode                                                                | `require`               |
| `DB_MAX_OPEN_CONNS` | Maximum open database connections                                       | `25`                    |
| `DB_MAX_IDLE_CONNS` | Maximum idle database connections                                       | `5`                     |
| `DB_CONN_MAX_LIFETIME_SECONDS` | Maximum database connection lifetime, in seconds                | `3600`                  |
| `DB_CONN_MAX_IDLE_TIME` | Maximum database connection idle time, as a Go duration              | `30m`                   |

### Database setup

CE's global configuration contains `github.com/OpenNSW/core/database.Config` directly.
The loader reads environment variables and flags, then calls `Config.Validate()`
before startup. Existing `DB_*` settings continue to apply.
`DB_PORT` must be an integer from 1 to 65535, and `DB_PASSWORD` must be non-empty
(the factory validates it even for PostgreSQL installations using trust authentication).
`internal/database` owns driver registration, OpenNSW connection setup, the
10-second connection deadline, and idle-timeout configuration. Startup returns an
error if the connection fails; a deployment's restart policy can retry startup.
The pool defaults to 25 open connections, 5 idle connections, a one-hour connection
lifetime, and a 30-minute idle timeout; the pool environment variables can override them.
CE makes one `database.OpenGORM(ctx, cfg.Database, cfg.DatabaseIdleTimeout)` call
to obtain the GORM handle and its SQL pool for the existing consent service.
Raw SQL callers use `database.Open(ctx, cfg.Database, cfg.DatabaseIdleTimeout)`.
Both initializers use the same pool setup; GORM wraps that pool without opening
or pinging another one. The application closes the returned SQL pool on shutdown,
and the package closes it if GORM initialization fails.

`cmd/ce/schema.sql` defines the initial PostgreSQL schema. Initialize the CE database
before starting the service, using the same `DB_*` settings:

```bash
# From the repository root, after creating the database.
PGPASSWORD="$DB_PASSWORD" psql \
  -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USERNAME" -d "$DB_NAME" \
  -v ON_ERROR_STOP=1 -f cmd/ce/schema.sql
```

The root Docker Compose stack and integration-test database container use this same
file when initializing a fresh PostgreSQL data directory. The service opens the pool
and expects the schema to exist; it does not run SQL schema setup during startup.
The root stack passes `DB_NAME_CE` to both PostgreSQL bootstrap and CE, so custom
database names are created and initialized consistently. An unset or empty value
uses `consent_engine`.

While CE is pre-stable, schema changes are made directly in this initial schema file.
There is no versioned upgrade path for existing databases. PB and PDP retain their
own `RUN_MIGRATION` setting.

Consent operations temporarily use a GORM adapter over this same SQL pool until
issue #631 integrates the raw SQL repository.

## API Endpoints

### Internal APIs (No Authentication)

| Method | Endpoint                    | Description               |
|--------|-----------------------------|---------------------------|
| GET    | `/internal/api/v1/health`   | Health check              |
| GET    | `/internal/api/v1/consents` | Get consent by session ID |
| POST   | `/internal/api/v1/consents` | Create new consent        |

### Portal APIs (JWT Authentication)

| Method | Endpoint                       | Description                                                                     |
|--------|--------------------------------|---------------------------------------------------------------------------------|
| GET    | `/api/v1/health`               | Health check                                                                    |
| GET    | `/api/v1/consents`             | List the signed-in owner's consents (`?status=pending&limit=20&offset=0`)       |
| GET    | `/api/v1/consents/{consentId}` | Get consent details                                                             |
| PUT    | `/api/v1/consents/{consentId}` | Approve/reject a pending consent or revoke an approved one (`409` otherwise)    |

### System Endpoints

| Method | Endpoint   | Description         |
|--------|------------|---------------------|
| GET    | `/health`  | Legacy health check |
| GET    | `/metrics` | Prometheus metrics  |

## Testing

```bash
# From the repo root
go test ./cmd/ce/... ./internal/ce/... ./internal/database/... -count=1
```

PostgreSQL connection and bootstrap tests use a disposable PostgreSQL instance.
Bootstrap tests require `psql` and a database user with permission to create databases.
These tests are skipped when
`TEST_DATABASE_URL` is unset:

```bash
TEST_DATABASE_URL='postgres://postgres:password@localhost:5432/consent_test?sslmode=disable' \
  go test ./cmd/ce/... ./internal/ce/config ./internal/database -count=1 -v
```

## Docker

```bash
# Build from the repo root
docker build -t consent-engine -f cmd/ce/Dockerfile .

# Run
docker run -p 8081:8081 --env-file .env consent-engine
```
