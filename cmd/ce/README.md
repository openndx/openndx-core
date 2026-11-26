# Consent Engine (CE)

Records data-owner consent for data access requests and runs the consent lifecycle: a consumer asks, the citizen approves or rejects in the Consent Portal, and the grant expires or is revoked later.

## Overview

```mermaid
flowchart LR
    OE[Orchestration Engine] -->|create and check consent<br/>internal API, no auth| CE
    CP[Consent Portal] -->|list, view, decide<br/>portal API, JWT| CE
    CE --> DB[(PostgreSQL: consent_engine)]
    CE -.->|fetch signing keys| IDP[ThunderID]
```

| Caller | Uses |
|---|---|
| Orchestration Engine | Internal API: creates a consent when the PDP says the owner must consent, and reads its status |
| Consent Portal ([portals/apps/consent](../../portals/apps/consent/README.md)) | Portal API with the citizen's access token: list, view, approve, reject, revoke |

The only outbound call is the identity provider's JWKS endpoint, used to verify portal tokens. The CE does not emit audit events.

## Quick start

With Docker Compose (from the repo root; starts PostgreSQL and ThunderID first, and loads the schema on a fresh database):

```bash
docker compose up --build consent-engine
curl http://localhost:8081/health
```

Natively, with its dependencies in Docker (see [running the platform](../../CONTRIBUTING.md#running-the-platform)):

```bash
make deps                                           # PostgreSQL with schema.sql loaded, ThunderID and the rest
cp cmd/ce/config.example.yaml cmd/ce/config.yaml    # once; config.yaml is git-ignored
export DB_PASSWORD=1234                             # the Compose default
cd cmd/ce && go run .
```

The Orchestration Engine and the Consent Portal call the CE, so to use them with a native CE, run them natively too, each in another terminal (`cd cmd/oe && go run .`, `cd portals/apps/consent && pnpm dev`). [config.example.yaml](config.example.yaml) points at `localhost:5432` as user `postgres`, reads the password from `DB_PASSWORD`, and validates tokens from a ThunderID on `localhost:8090`. The service does not create tables; Compose's PostgreSQL loads `schema.sql` on a fresh database. Schema changes go directly into `schema.sql`; there is no versioned upgrade path.

## Configuration

The CE reads one YAML file: the `-config` flag, else `CONFIG_PATH`, else `config.yaml` in the working directory. There are no other flags. Besides `CONFIG_PATH` and the variables named by placeholders (below), it reads only the metrics settings, such as `SERVICE_NAME` and `OTEL_METRICS_EXPORTER`, listed in [observability/README.md](../../observability/README.md#settings). The Docker image sets `CONFIG_PATH=/app/config/config.yaml` and has no config file of its own, so mount one there. Compose mounts [config.docker.yaml](config.docker.yaml).

A value written as `"{{env:NAME}}"` or `"{{file:/path}}"` is read from that environment variable or file at startup, so secrets stay out of the file. The placeholder must be the whole value, and an unset or empty variable stops the service.

| Key                                                                               | Default                    | Notes                                                                                                                  |
|-----------------------------------------------------------------------------------|----------------------------|------------------------------------------------------------------------------------------------------------------------|
| `server.port`                                                                     | `8081`                     | Listen port. The server binds on all interfaces.                                                                       |
| `server.timeout`                                                                  | `10s`                      | Read and write timeout.                                                                                                |
| `environment`                                                                     | none                       | Deployment name (for example `local`, `staging`), added to every log line. Left out of log lines when empty.           |
| `logging.level`                                                                   | `info`                     | `debug`, `info`, `warn` or `error`.                                                                                    |
| `logging.format`                                                                  | `json`                     | `json` or `text`. Every line carries `service`, `version` and, when set, `environment`.                                |
| `consentPortalURL`                                                                | none                       | **Required.** Must include scheme and host. Consent links are `<url>?consentId=<uuid>`. Its origin is always allowed.  |
| `cors.allowedOrigins`                                                             | empty                      | Origins allowed besides `consentPortalURL`. `*` stops the service at startup while `allowCredentials` is true.         |
| `cors.allowedMethods`, `cors.allowedHeaders`                                      | see [config.go](config.go) | A list in the file replaces the default list.                                                                          |
| `cors.allowCredentials`, `cors.maxAge`                                            | `true`, `86400`            |                                                                                                                        |
| `idp.jwksURL`                                                                     | none                       | **Required.** Signing keys for portal tokens. If unreachable, every portal call returns 401.                           |
| `idp.issuer`, `idp.audience`                                                      | none                       | **Required.** Expected token `iss` and `aud`. Compose uses ThunderID's base URL and `CONSENT_PORTAL`.                  |
| `idp.subjectClaim`                                                                | `sub`                      | Token claim whose value must equal the consent's `ownerId`. Compose sets `nic`. A token without it gets `401`.         |
| `idp.insecureSkipTLSVerify`                                                       | `false`                    | Skips TLS verification for the JWKS fetch. Development only.                                                           |
| `idp.claims.email.required`, `.ouId.required`, `.ouHandle.required`               | `false`                    | `true` rejects a token whose claim is absent or blank.                                                                 |
| `database.driver`                                                                 | `postgres`                 | Only `postgres` is supported.                                                                                          |
| `database.postgres.host`, `.user`, `.password`                                    | none                       | **Required.**                                                                                                          |
| `database.postgres.port`                                                          | `5432`                     |                                                                                                                        |
| `database.postgres.name`                                                          | `consent_engine`           |                                                                                                                        |
| `database.postgres.sslMode`                                                       | `require`                  | Use `disable` for a local PostgreSQL without TLS.                                                                      |
| `database.postgres.pool.maxOpenConns`, `.maxIdleConns`, `.maxConnLifetimeSeconds` | `25`, `5`, `3600`          | Connection pool.                                                                                                       |
| `database.idleTimeout`                                                            | `30m`                      | Closes connections idle this long.                                                                                     |

The service checks the file at startup and exits with an error naming the bad key. Keys it does not know are ignored, so check the spelling of any key that seems to have no effect.

## API

Full schemas: [openapi.yaml](openapi.yaml). Errors are `{"error": {"code": "...", "message": "..."}}`, except a portal call with a missing or invalid token, which gets the shared authentication layer's `401` body, `{"error": "unauthorized", "message": "..."}`. Token checks: [CONSENT_LIFECYCLE.md](CONSENT_LIFECYCLE.md#who-may-decide).

**Internal API** (no authentication, for the Orchestration Engine):

| Method and path                                 | Purpose                                                                                                                                       |
|-------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| `POST /internal/api/v1/consents`                | Create a consent (`201`). Body: `appId`, `consentRequirement {owner, ownerId, fields[]}`, optional `appName`, `grantDuration`, `consentType`. |
| `GET /internal/api/v1/consents?ownerId=&appId=` | Latest consent for the pair. Both parameters are required (`400`); `404` if none.                                                             |

**Portal API** (Bearer access token; the token's subject claim must equal the consent's `ownerId`):

| Method and path                    | Purpose                                                                                                               |
|------------------------------------|-----------------------------------------------------------------------------------------------------------------------|
| `GET /api/v1/consents`             | The caller's consents, newest first. `status` (repeatable or comma-separated), `limit` (1-100, default 20), `offset`. |
| `GET /api/v1/consents/{consentId}` | One consent. `403` if it belongs to someone else.                                                                     |
| `PUT /api/v1/consents/{consentId}` | Body `{"action": "approve" \| "reject" \| "revoke"}`. `409` if the consent is not in a state that allows the action.  |

**System:** `GET /health` (`{"service": "consent-engine", "status": "healthy"}`) and `GET /metrics` (Prometheus), both unauthenticated.

## How it works

- A consent is `pending` until the owner approves or rejects it, or the request times out (1 hour for `realtime`, 24 hours for `offline`).
- Approving starts a grant of the requested duration (default 1 hour). A grant can be revoked.
- At most one `pending` or `approved` consent exists per `(ownerId, appId)`. Creating the same request again returns the existing one; creating it with different fields revokes the old one and issues a new one.
- Expiry is applied lazily, when a consent is read or acted on.

States, durations and the exact create rules: [CONSENT_LIFECYCLE.md](CONSENT_LIFECYCLE.md).

## Security and deployment

> **Deployment security: network isolation.** The `/internal/api/v1/*`, `/health` and `/metrics` routes are not authenticated. Anyone who can reach them can create consents and read consent status. Expose only the portal routes (`/api/v1/*`) to the Consent Portal's network, and keep the rest reachable only by the Orchestration Engine and your monitoring.

## Development

From the repository root:

```bash
make test PKG="./cmd/ce/... ./internal/ce/... ./internal/database/..."    # tests with -race
make check                                                                # tidy, check license headers, lint and test the whole module
```

Service tests use an in-memory SQLite database. The PostgreSQL connection and bootstrap tests are skipped unless `TEST_DATABASE_URL` points at a disposable database (the bootstrap test also needs `psql` and permission to create databases):

```bash
TEST_DATABASE_URL='postgres://postgres:password@localhost:5432/consent_test?sslmode=disable' \
  go test ./cmd/ce/... ./internal/database -count=1 -v
```

Docker image: `docker build -t consent-engine -f cmd/ce/Dockerfile .` from the repo root. The configuration is in [`config.go`](config.go). Routes are registered in [`main.go`](main.go); the code is in [`internal/ce`](../../internal/ce): `middleware`, `handlers`, `services`, `models`, `utils`. CORS is [`OpenNSW/core/cors`](https://github.com/OpenNSW/core/tree/main/cors).

## More docs

- [CONSENT_LIFECYCLE.md](CONSENT_LIFECYCLE.md): states, expiry, create rules, owner matching
- [openapi.yaml](openapi.yaml): API reference
- [Consent Portal](../../portals/apps/consent/README.md) and [Orchestration Engine](../oe/README.md): the two callers
