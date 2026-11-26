# Portal Backend (PB)

The management REST API of OpenNDX: members, provider schemas, consumer applications, and the submissions that create them. It validates callers' tokens, enforces scope-based access, and keeps the Policy Decision Point and the identity provider in step with what it stores.

> **Naming.** PB is planned to be renamed **Management Server**. Until then the code, image, service and environment names all say `portal-backend` or `pb`; the [rename checklist](#rename-note) lists every place the name appears.

## Overview

```mermaid
flowchart LR
    CLI[ondx CLI] -->|REST, Bearer token| PB
    PB --> DB[(PostgreSQL: portal_backend)]
    PB -->|field policies and access grants| PDP[Policy Decision Point]
    PB -->|create and delete app clients| IDP[ThunderID]
    PB -.->|verify tokens: JWKS| IDP
    PB -.->|audit events| A[Argus audit service]
```

PB remains an actively maintained management API, used through the [`ondx` CLI](../ondx/README.md) or direct authenticated API calls. The [Admin and Member Portals](../../portals/README.md) are paused, unsupported prototypes and do not provide a supported management interface. The Consent Portal does not use it. PB never calls the Orchestration Engine or Consent Engine.

## Quick start

With Docker Compose (from the repo root). PB needs the PDP at runtime, so start both:

```bash
docker compose up --build portal-backend policy-decision-point
curl http://localhost:8083/health
```

This also starts PostgreSQL and ThunderID, and registers PB's own client (`PORTAL_BACKEND`) in ThunderID.

Natively, while the rest of the stack runs in Docker. No container calls PB, so it can swap out on its own. Stop any other service you run natively first, since `make up` starts it in Docker on the same port (see [running the platform](../../CONTRIBUTING.md#running-the-platform)):

```bash
make up
docker compose stop portal-backend
cp cmd/pb/config.example.yaml cmd/pb/config.yaml    # once; config.yaml is git-ignored
export DB_PASSWORD=1234 IDP_CLIENT_SECRET=1234      # the Compose defaults
cd cmd/pb && go run .
```

[config.example.yaml](config.example.yaml) points at `localhost:5432` as user `postgres`, a PDP on `localhost:8082` and a ThunderID on `localhost:8090`, and reads the two secrets from the environment. The service does not create tables; Compose's PostgreSQL loads [schema.sql](schema.sql) on a fresh database. Schema changes go directly into `schema.sql`; there is no versioned upgrade path.

## Configuration

PB reads one YAML file: the `-config` flag, else `CONFIG_PATH`, else `config.yaml` in the working directory. There are no other flags or environment variables. The Docker image sets `CONFIG_PATH=/app/config/config.yaml` and has no config file of its own, so mount one there. Compose mounts [config.docker.yaml](config.docker.yaml).

A value written as `"{{env:NAME}}"` or `"{{file:/path}}"` is read from that environment variable or file at startup, so secrets stay out of the file. Required keys:

| Key | Notes |
|---|---|
| `idp.jwksURL`, `idp.issuer`, `idp.audience` | Validate incoming tokens. `audience` is PB's resource-server identifier. |
| `idp.management.baseURL`, `.clientID`, `.clientSecret` | PB's own client for ThunderID's management API. |
| `pdp.url` | Policy Decision Point. |
| `database.postgres.host`, `.user`, `.password` | |

Every key, its default and the environment variable it replaces: [CONFIGURATION.md](CONFIGURATION.md).

## API

All routes are under `/api/v1` and need `Authorization: Bearer <token>`. Full schemas: [openapi.yaml](openapi.yaml). Success responses are the bare resource, lists are `{items, count}`, and errors are `{"error": "<message>"}`.

| Resource | Endpoints | Scope |
|---|---|---|
| Members | `GET /members`, `GET /members/{memberId}`<br>`POST /members`<br>`PUT /members/{memberId}` | `member:read`<br>`member:write-all`<br>`member:write` |
| Schemas | `GET /schemas`, `GET /schemas/{schemaId}`<br>`POST /schemas`, `PUT /schemas/{schemaId}` | `schema:read`<br>`schema:write` |
| Schema field policies | `GET /schemas/{schemaId}/field-policies`<br>`PATCH`, `DELETE` on `.../field-policies/{id}`<br>`DELETE .../field-policies/{id}/allowlist/{applicationId}` | `schema:read`<br>`schema:write` |
| Schema submissions | `GET /schema-submissions`, `GET .../{submissionId}`<br>`POST /schema-submissions`, `PUT .../{submissionId}` | `schema-submission:read`<br>`schema-submission:write` |
| Applications | `GET /applications`, `GET /applications/{applicationId}`<br>`POST /applications`, `PUT /applications/{applicationId}`<br>`PUT /applications/{applicationId}/policy` | `application:read`<br>`application:write` |
| Application submissions | `GET /application-submissions`, `GET .../{submissionId}`<br>`POST /application-submissions`, `PUT .../{submissionId}` | `application-submission:read`<br>`application-submission:write` |
| Consent bypass grants | `GET /consent-bypass-grants`<br>`POST /applications/{applicationId}/consent-bypass-grants`, `POST /consent-bypass-grants/{grantId}/revoke` | `consent-bypass:read`<br>`consent-bypass:write` |

Outside `/api/v1`, with no authentication: `GET /health` (liveness only, it does not check the database). The authorization model, including the `-all` scopes and what each ThunderID role may do, is in [AUTHORIZATION.md](AUTHORIZATION.md).

## How it works

Creating an application touches three systems, so PB undoes earlier steps if a later one fails:

```mermaid
sequenceDiagram
    participant C as Caller
    participant PB
    participant IDP as ThunderID
    participant DB as PostgreSQL
    participant PDP
    C->>PB: POST /api/v1/applications
    PB->>IDP: create M2M client in the member's organization unit, assign role
    IDP-->>PB: client_id and client_secret
    PB->>DB: insert application
    PB->>PDP: grant client_id access to the fields (30 days)
    PB-->>C: application and idpClientSecret (shown once)
```

- PDP access grants are keyed by the IdP `client_id`, which is the identity the Orchestration Engine reads from consumer tokens. The `allowList` in field policy responses lists the applications with an active grant.
- `POST /schemas` stores the schema, then registers its field policies in the PDP, and removes the schema if that fails.
- A submission is a request that becomes a schema or application when its status is set to `approved` (`PUT .../{submissionId}`); the creation steps above then run, and the status goes back to `pending` if they fail. Who may set that status is described in [AUTHORIZATION.md](AUTHORIZATION.md#submission-approval).
- A caller reaches its own member's records. Reaching other members' records needs the matching `-all` scope as well.

## Security and deployment

> **Deployment security: network isolation.** `/health` is not authenticated. Expose only `/api/v1/*` beyond the internal network. PB also calls the PDP without authentication, so the PDP must be reachable only from PB and the Orchestration Engine.

- CORS defaults to `http://localhost:5173`, which Compose also uses; set explicit origins for any shared deployment.
- PB serves no `/metrics` endpoint and no `/openapi.yaml`; the spec is the file in this directory.

## Development

From the repository root:

```bash
make test PKG="./cmd/pb/... ./internal/pb/... ./internal/authn/..."    # tests with -race
make check                                                            # tidy, check license headers, lint and test the whole module
```

Tests run without external services; `internal/authn` holds the token validation code. Docker image: `docker build -t portal-backend -f cmd/pb/Dockerfile .` from the repo root.

Code is in [`internal/pb`](../../internal/pb): `member`, `schema`, `application` and `consentbypass` (one package per bounded context: a service and handler, and a model when it stores data in PB), `kernel` (types shared by those packages), `caller` (scopes and the caller's member), `policy` (PDP client), `idp/thunderid` (ThunderID client), `middleware` (audit), `database/dbtest` (test databases). The configuration is in [`config.go`](config.go). PB connects through the shared [`internal/database`](../../internal/database), which uses `OpenNSW/core/database`; CORS is [`OpenNSW/core/cors`](https://github.com/OpenNSW/core/tree/main/cors). Token validation is in [`internal/authn`](../../internal/authn/README.md).

## Rename note

When PB becomes Management Server, these identifiers embed the current name:

- Compose service, container and image: `portal-backend`, `pb-${ENVIRONMENT}`, `ghcr.io/openndx/portal-backend`; Makefile service name and CI workflows (`portal-backend-validate`, `portal-backend-publish`)
- Go packages `internal/pb` and `internal/cli/pbclient`; the `"portal-backend"` string in `/health`
- Environment variables `PB_PORT`, `DB_NAME_PB`, `PB_AUDIENCE`, and the `ondx` flag and variable `--pb-url` / `NDX_PB_URL`
- The token audience `http://pb.openndx.local`, which clients request as `resource=`
- ThunderID seed data in `thunderid/bootstrap`: the `PORTAL_BACKEND` client, the `PortalBackendSystem` role and the "Portal Backend Resource Server"

## More docs

- [AUTHORIZATION.md](AUTHORIZATION.md): scopes, ownership rules, roles, submission approval
- [CONFIGURATION.md](CONFIGURATION.md): every configuration key
- [openapi.yaml](openapi.yaml): API reference
- [`ondx` CLI](../ondx/README.md) and [Policy Decision Point](../pdp/README.md)
