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
curl http://localhost:8081/internal/api/v1/health
```

Native, against a PostgreSQL with an empty database. The service does not create tables, so load the schema first:

```bash
PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USERNAME" -d "$DB_NAME" \
  -v ON_ERROR_STOP=1 -f cmd/ce/schema.sql

export DB_HOST=localhost DB_PORT=5432 DB_USERNAME=postgres DB_PASSWORD=<password> \
       DB_NAME=consent_engine DB_SSLMODE=disable \
       IDP_ISSUER=<issuer> IDP_AUDIENCE=<audience> IDP_JWKS_URL=<jwks-url>
go run ./cmd/ce        # or: make run consent-engine
```

`DB_PASSWORD` must be non-empty, even for a PostgreSQL that uses trust authentication. No binary loads a `.env` file, so export the variables or pass `--env-file` to Docker. [.env.example](.env.example) is a template. Schema changes go directly into `schema.sql`; there is no versioned upgrade path.

## Configuration

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8081` | |
| `ENVIRONMENT` | `local` | |
| `CONSENT_PORTAL_URL` | `http://localhost:5173` | Must include scheme and host. Consent links are `<url>?consentId=<uuid>`. |
| `CORS_ALLOWED_ORIGINS` | empty | Comma-separated. `CONSENT_PORTAL_URL` is always allowed. A value of `*` stops the service at startup. |
| `IDP_JWKS_URL` | empty | Signing keys for portal tokens. If empty or unreachable, every portal call returns 401. |
| `IDP_ISSUER`, `IDP_AUDIENCE`, `IDP_CLIENT_ID` | empty | Each check applies only when set. With no issuer and audience, any token signed by a key in the JWKS is accepted. |
| `IDP_SUBJECT_CLAIM` | `sub` | Token claim whose value must equal the consent's `ownerId`. Compose sets `nic`. |
| `IDP_JWKS_INSECURE_SKIP_VERIFY` | `false` | Skips TLS verification for the JWKS fetch. Development only. |
| `DB_HOST`, `DB_PORT` | `localhost`, `5432` | |
| `DB_USERNAME`, `DB_PASSWORD` | `postgres`, none | The password is required. |
| `DB_NAME` | `consent_engine` | |
| `DB_SSLMODE` | `require` | |
| `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME_SECONDS`, `DB_CONN_MAX_IDLE_TIME` | `25`, `5`, `3600`, `30m` | Connection pool. |

Flags: `-env`, `-port`, `-timeout` (default `10s`), `-log-level`, `-log-format`. `-host`, `-cors` and `-rate-limit` are accepted but have no effect. Compose values are in [compose.yml](../../compose.yml).

## API

Full schemas: [openapi.yaml](openapi.yaml). Errors are `{"error": {"code": "...", "message": "..."}}`.

**Internal API** (no authentication, for the Orchestration Engine):

| Method and path | Purpose |
|---|---|
| `POST /internal/api/v1/consents` | Create a consent (`201`). Body: `appId`, `consentRequirement {owner, ownerId, fields[]}`, optional `appName`, `grantDuration`, `consentType`. |
| `GET /internal/api/v1/consents?ownerId=&appId=` | Latest consent for the pair. Both parameters are required (`400`); `404` if none. |
| `GET /internal/api/v1/health` | `{"status": "healthy"}` |

**Portal API** (Bearer access token; the token's subject claim must equal the consent's `ownerId`):

| Method and path | Purpose |
|---|---|
| `GET /api/v1/consents` | The caller's consents, newest first. `status` (repeatable or comma-separated), `limit` (1-100, default 20), `offset`. |
| `GET /api/v1/consents/{consentId}` | One consent. `403` if it belongs to someone else. |
| `PUT /api/v1/consents/{consentId}` | Body `{"action": "approve" \| "reject" \| "revoke"}`. `409` if the consent is not in a state that allows the action. |
| `GET /api/v1/health` | Public, no token needed. |

**System:** `GET /health` and `GET /metrics` (Prometheus), both unauthenticated.

## How it works

- A consent is `pending` until the owner approves or rejects it, or the request times out (1 hour for `realtime`, 24 hours for `offline`).
- Approving starts a grant of the requested duration (default 1 hour). A grant can be revoked.
- At most one `pending` or `approved` consent exists per `(ownerId, appId)`. Creating the same request again returns the existing one; creating it with different fields revokes the old one and issues a new one.
- Expiry is applied lazily, when a consent is read or acted on.

States, durations and the exact create rules: [CONSENT_LIFECYCLE.md](CONSENT_LIFECYCLE.md).

## Security and deployment

> **Deployment security: network isolation.** The `/internal/api/v1/*`, `/health` and `/metrics` routes are not authenticated. Anyone who can reach them can create consents and read consent status. Expose only the portal routes (`/api/v1/*`) to the Consent Portal's network, and keep the rest reachable only by the Orchestration Engine and your monitoring.

## Development

```bash
make validate-test consent-engine
go test ./cmd/ce/... ./internal/ce/... ./internal/database/... -count=1
make quality-check consent-engine
```

Service tests use an in-memory SQLite database. The PostgreSQL connection and bootstrap tests are skipped unless `TEST_DATABASE_URL` points at a disposable database (the bootstrap test also needs `psql` and permission to create databases):

```bash
TEST_DATABASE_URL='postgres://postgres:password@localhost:5432/consent_test?sslmode=disable' \
  go test ./cmd/ce/... ./internal/ce/config ./internal/database -count=1 -v
```

Docker image: `docker build -t consent-engine -f cmd/ce/Dockerfile .` from the repo root. Code is in [`internal/ce`](../../internal/ce): `router`, `middleware`, `auth`, `handlers`, `services`, `models`.

## More docs

- [CONSENT_LIFECYCLE.md](CONSENT_LIFECYCLE.md): states, expiry, create rules, owner matching
- [openapi.yaml](openapi.yaml): API reference
- [Consent Portal](../../portals/apps/consent/README.md) and [Orchestration Engine](../oe/README.md): the two callers
