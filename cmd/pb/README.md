# Portal Backend (PB)

The management REST API of OpenNDX: members, provider schemas, consumer applications, and the submissions that create them. It validates callers' tokens, enforces scope-based access, and keeps the Policy Decision Point and the identity provider in step with what it stores.

> **Naming.** PB is planned to be renamed **Management Server**. Until then the code, image, service and environment names all say `portal-backend` or `pb`; the [rename checklist](#rename-note) lists every place the name appears.

## Overview

```mermaid
flowchart LR
    AP[Admin and Member portals] -->|REST, Bearer token| PB
    CLI[ondx CLI] -->|REST, Bearer token| PB
    PB --> DB[(PostgreSQL: portal_backend)]
    PB -->|metadata and allow-list| PDP[Policy Decision Point]
    PB -->|create and delete app clients| IDP[ThunderID]
    PB -.->|verify tokens: JWKS| IDP
    PB -.->|audit events| A[Argus audit service]
```

PB is called by the [`ondx` CLI](../ondx/README.md) and by the admin and member portals. The Consent Portal does not use it. PB never calls the Orchestration Engine or Consent Engine.

## Quick start

With Docker Compose (from the repo root). PB needs the PDP at runtime, so start both:

```bash
docker compose up --build portal-backend policy-decision-point
curl http://localhost:8083/health
```

This also starts PostgreSQL and ThunderID, and registers PB's own client (`PORTAL_BACKEND`) in ThunderID.

Native, against your own PostgreSQL, PDP and identity provider. All of the variables marked required in [CONFIGURATION.md](CONFIGURATION.md) must be set, or the service exits at startup:

```bash
export DB_HOST=localhost DB_USERNAME=postgres DB_PASSWORD=<password> DB_NAME=portal_backend \
       DB_SSLMODE=disable RUN_MIGRATION=true \
       PDP_SERVICE_URL=http://localhost:8082 \
       IDP_BASE_URL=http://localhost:8090 IDP_AUDIENCE=http://pb.openndx.local \
       IDP_CLIENT_ID=PORTAL_BACKEND IDP_CLIENT_SECRET=<secret> IDP_SCOPE=system \
       IDP_TOKEN_RESOURCE=https://localhost:8090/mcp IDP_DATA_CONSUMER_ROLE_ID=<role-id>
make run portal-backend      # or: go run ./cmd/pb
```

No `.env` file is loaded; [.env.example](.env.example) is a template to source yourself (`set -a; source cmd/pb/.env; set +a`). Tables are created only when `RUN_MIGRATION` is exactly `true`.

## Configuration

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8083` | |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:5173` | Comma-separated origins; credentials are allowed. |
| `DB_HOST`, `DB_PORT`, `DB_USERNAME`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` | `localhost`, `5432`, `postgres`, `password`, `testdb2`, `require` | |
| `RUN_MIGRATION` | unset | Run table migrations at startup only when exactly `true`. |
| `PDP_SERVICE_URL` | none | **Required.** Must start with `http://` or `https://`. |
| `IDP_AUDIENCE` | none | **Required.** PB's resource-server identifier; tokens for any other audience are rejected. |
| `IDP_BASE_URL` | none | Required unless `IDP_JWKS_URL` and `IDP_ISSUER` are set. Derives the JWKS and issuer URLs. |
| `IDP_CLIENT_ID`, `IDP_CLIENT_SECRET` | none | **Required.** PB's own client for ThunderID's management API. |
| `IDP_SCOPE`, `IDP_TOKEN_RESOURCE`, `IDP_DATA_CONSUMER_ROLE_ID` | none | Management token scope and resource, and the role given to new application clients. |
| `AUDIT_SERVICE_URL`, `ARGUS_API_KEY` | none | Optional; auditing is off without a URL. |

All variables, with code defaults against the Compose values: [CONFIGURATION.md](CONFIGURATION.md).

## API

All routes are under `/api/v1` and need `Authorization: Bearer <token>`. Full schemas: [openapi.yaml](openapi.yaml). Success responses are the bare resource, lists are `{items, count}`, and errors are `{"error": "<message>"}`.

| Resource | Endpoints | Scope |
|---|---|---|
| Members | `GET /members`, `GET /members/{memberId}`<br>`POST /members`<br>`PUT /members/{memberId}` | `member:read`<br>`member:write-all`<br>`member:write` |
| Schemas | `GET /schemas`, `GET /schemas/{schemaId}`<br>`POST /schemas`, `PUT /schemas/{schemaId}` | `schema:read`<br>`schema:write` |
| Schema policy metadata | `GET /schemas/{schemaId}/policy-metadata`<br>`PATCH`, `DELETE` on `.../policy-metadata/{id}`<br>`DELETE .../policy-metadata/{id}/allowlist/{applicationId}` | `schema:read`<br>`schema:write` |
| Schema submissions | `GET /schema-submissions`, `GET .../{submissionId}`<br>`POST /schema-submissions`, `PUT .../{submissionId}` | `schema-submission:read`<br>`schema-submission:write` |
| Applications | `GET /applications`, `GET /applications/{applicationId}`<br>`POST /applications`, `PUT /applications/{applicationId}`<br>`PUT /applications/{applicationId}/policy` | `application:read`<br>`application:write` |
| Application submissions | `GET /application-submissions`, `GET .../{submissionId}`<br>`POST /application-submissions`, `PUT .../{submissionId}` | `application-submission:read`<br>`application-submission:write` |

Outside `/api/v1`, with no authentication: `GET /health` (`503` if the database is down), `GET /debug`, `GET /debug/db`, and `GET /internal/api/v1/applications?idpClientId=` (looks up an application by its IdP client ID). The authorization model, including the `-all` scopes and what each ThunderID role may do, is in [AUTHORIZATION.md](AUTHORIZATION.md).

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
    PB->>PDP: update allow-list for client_id (30 days)
    PB-->>C: application and idpClientSecret (shown once)
```

- The PDP allow-list is keyed by the IdP `client_id`, which is the identity the Orchestration Engine reads from consumer tokens.
- `POST /schemas` stores the schema, then registers its policy metadata in the PDP, and removes the schema if that fails.
- A submission is a request that becomes a schema or application when its status is set to `approved` (`PUT .../{submissionId}`); the creation steps above then run, and the status goes back to `pending` if they fail. Who may set that status is described in [AUTHORIZATION.md](AUTHORIZATION.md#submission-approval).
- A caller reaches its own member's records. Reaching other members' records needs the matching `-all` scope as well.

## Security and deployment

> **Deployment security: network isolation.** `GET /internal/api/v1/applications`, `/health`, `/debug` and `/debug/db` are not authenticated. Expose only `/api/v1/*` beyond the internal network. PB also calls the PDP without authentication, so the PDP must be reachable only from PB and the Orchestration Engine.

- CORS defaults to `http://localhost:5173`. Compose sets `*`; set explicit origins for any shared deployment.
- PB serves no `/metrics` endpoint and no `/openapi.yaml`; the spec is the file in this directory.

## Development

```bash
make validate-test portal-backend      # tests with -race and coverage
go test ./cmd/pb/... ./internal/pb/... -count=1
make quality-check portal-backend
```

Tests run without external services. `make validate-test` does not cover `internal/authn`; run `go test ./internal/authn/...` for the token validation code. Docker image: `docker build -t portal-backend -f cmd/pb/Dockerfile .` from the repo root.

Code is in [`internal/pb`](../../internal/pb): `member`, `schema` and `application` (one package per resource: model, service, handler), `caller` (scopes and the caller's member), `policy` (PDP client), `idp/thunderid` (ThunderID client), `middleware` (CORS and audit), `database`. Token validation is in [`internal/authn`](../../internal/authn/README.md).

## Rename note

When PB becomes Management Server, these identifiers embed the current name:

- Compose service, container and image: `portal-backend`, `pb-${ENVIRONMENT}`, `ghcr.io/openndx/portal-backend`; Makefile service name and CI workflows (`portal-backend-validate`, `portal-backend-publish`)
- Go packages `internal/pb` and `internal/cli/pbclient`; the `"portal-backend"` string in `/health` and `/debug`
- Environment variables `PB_PORT`, `DB_NAME_PB`, `PB_AUDIENCE`, and the `ondx` flag and variable `--pb-url` / `NDX_PB_URL`
- The token audience `http://pb.openndx.local`, which clients request as `resource=`
- ThunderID seed data in `thunderid/bootstrap`: the `PORTAL_BACKEND` client, the `PortalBackendSystem` role and the "Portal Backend Resource Server"

## More docs

- [AUTHORIZATION.md](AUTHORIZATION.md): scopes, ownership rules, roles, submission approval
- [CONFIGURATION.md](CONFIGURATION.md): every environment variable
- [openapi.yaml](openapi.yaml): API reference
- [`ondx` CLI](../ondx/README.md) and [Policy Decision Point](../pdp/README.md)
