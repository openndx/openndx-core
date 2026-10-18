# Orchestration Engine (OE)

The data-exchange gateway. A data consumer sends one GraphQL query; the OE checks access with the Policy Decision Point, obtains the data owner's consent through the Consent Engine, fetches the data from the providers that hold it, and returns one merged response.

## Overview

```mermaid
flowchart LR
    C[Data consumer] -->|"POST /public/graphql<br/>Bearer token"| OE
    OE -->|decide| PDP[Policy Decision Point]
    OE -->|create consent| CE[Consent Engine]
    OE -->|sub-queries| P[Data providers]
    OE -.->|audit events| A[Argus audit service]
    OE --> DB[(PostgreSQL: orchestration_engine)]
```

The OE splits a consumer query into one sub-query per provider using `@sourceInfo` directives in its GraphQL schema, so consumers never see which provider holds which field. It stores versioned copies of that schema in PostgreSQL. The only callers are data consumers (machine clients).

## Quick start

With Docker Compose, the full stack including the OE (from the repo root):

```bash
docker compose up --build orchestration-engine
curl http://localhost:4000/health
```

Compose mounts `cmd/oe/config.docker.json` and `cmd/oe/schema.graphql` into the container. The sample providers are the mock APIs in [dev/](../../dev/README.md).

Native, from the repo root:

```bash
cp cmd/oe/config.example.json cmd/oe/config.json     # git-ignored
export DB_HOST=localhost DB_PASSWORD=<password>      # optional: see Configuration
make run orchestration-engine
```

Use `make run`, not `go run ./cmd/oe`. `make run` starts the service from `cmd/oe`, which is where `./config.json` and `schema.graphql` are looked up. To run from elsewhere, set `CONFIG_PATH` and make sure a schema is available (see [CONFIGURATION.md](CONFIGURATION.md#schema-resolution)).

To send a real query you need a consumer token, registered policy metadata and an application grant. Follow the [walkthrough](../../dev/WALKTHROUGH.md), or the token and curl example in [dev/README.md](../../dev/README.md).

## Configuration

The OE reads a JSON file (`CONFIG_PATH`, default `./config.json`) plus a few environment variables.

| `config.json` key | Meaning |
|---|---|
| `foundationalIdArgName` | **Required.** Name of the query argument that identifies the data owner, for example `nic`. The service exits at startup without it. |
| `pdpUrl` | PDP base URL. If empty, the OE skips authorization and consent checks (see Security). |
| `ceUrl` | Consent Engine URL **including** `/internal/api/v1`. |
| `providers[]` | Data providers: `providerKey`, `providerUrl`, `schemaId`, optional `auth`. See [PROVIDER_CONFIGURATION.md](PROVIDER_CONFIGURATION.md). |
| `argMappings[]` | How consumer arguments map to each provider's arguments. |
| `trustUpstream`, `jwt`, `environment` | Token validation. See [AUTHENTICATION.md](AUTHENTICATION.md). |
| `auditConfig` | Argus audit service URL and actor identity. |

| Environment variable | Default |
|---|---|
| `PORT` | `4000` |
| `CONFIG_PATH` | `./config.json` |
| `DB_HOST`, `DB_PORT`, `DB_USERNAME`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` | `localhost`, `5432`, `postgres`, empty, `orchestration_engine`, `disable` |
| `AUDIT_SERVICE_URL`, `ARGUS_API_KEY` | override `auditConfig.serviceUrl`; audit API key |

Every key and variable, with the error codes: [CONFIGURATION.md](CONFIGURATION.md). No `.env` file is loaded; export variables or use Docker `--env-file`.

## API

| Method and path | Auth | Purpose |
|---|---|---|
| `POST /public/graphql` | Bearer token | Federated query. Body `{query, variables, operationName}`; response `{data, errors}`. Only one `query` operation per request. |
| `GET /health` | none | `{"message": "OpenNDX Server is Healthy!"}` |
| `GET /sdl` | none | Active schema SDL |
| `POST /sdl` | none | Create a schema version (`{version, sdl, created_by}`) |
| `GET /sdl/versions` | none | List schema versions |
| `POST /sdl/validate` | none | Check SDL syntax |
| `POST /sdl/check-compatibility` | none | Check backward compatibility with the active schema |
| `POST /sdl/versions/{version}/activate` | none | Make a version the active schema |

[openapi.yaml](openapi.yaml) describes `/health` and the `/sdl` routes. It does not describe `/public/graphql`, and its global `bearerAuth` does not match the code, which checks a token only on `/public/graphql`. If the database is unreachable at startup the `/sdl` routes return `503` and the service keeps running.

## How it works

```mermaid
sequenceDiagram
    participant C as Consumer
    participant OE
    participant PDP
    participant CE
    participant P as Providers
    C->>OE: POST /public/graphql
    OE->>OE: validate token, split query by @sourceInfo
    OE->>PDP: POST /api/v1/policy/decide
    PDP-->>OE: authorized? consent needed?
    opt owner consent required
        OE->>CE: POST /internal/api/v1/consents
        CE-->>OE: pending or approved
    end
    OE->>P: one sub-query per provider, in parallel
    P-->>OE: provider data
    OE-->>C: merged response
```

- The consumer's identity is the token's `client_id`. It is sent to the PDP as `applicationId`.
- Each field's `providerField` and `schemaId` become the `(schemaId, fieldName)` the PDP looks up, so every field must have policy metadata registered.
- If consent is not yet approved, the response is HTTP 200 with `errors[0].extensions.code = CE_NOT_APPROVED`, plus `consentPortalUrl` and `consentStatus`. The citizen approves in the Consent Portal and the consumer repeats the query.
- Federation errors come back as HTTP 200 with `errors[].extensions.code` (`PDP_NOT_ALLOWED`, `CE_NOT_APPROVED`, `MISSING_IDENTIFIER`, and others; see [CONFIGURATION.md](CONFIGURATION.md#error-codes)). Only an unreadable body (`400`) or a bad token (`401`, plain text) use HTTP error statuses.
- A provider that fails is left out of the result, and the failure is audited. The response is still HTTP 200.

## Security and deployment

> **Deployment security: network isolation.** The `/sdl` routes and `/health` are not authenticated. Anyone who can reach them can create and activate schemas. Expose only `/public/graphql` to consumers.

- The sample configs set `trustUpstream: true`, which reads the token **without verifying its signature**. That is for local development only. Production must set `trustUpstream: false` and configure `jwt.jwksUrl` ([AUTHENTICATION.md](AUTHENTICATION.md)).
- With `pdpUrl` empty, the OE returns provider data without any authorization or consent check. Always set it.
- CORS allows every origin.
- Audit events (`DATA_REQUEST`, `POLICY_CHECK`, `CONSENT_CHECK`, `PROVIDER_FETCH`) go to Argus. If Argus is down, batches are lost.

## Development

```bash
make validate-test orchestration-engine        # tests with -race and coverage
go test ./cmd/oe/... ./internal/oe/... -count=1
make quality-check orchestration-engine
```

[HOW_TO_TEST.md](HOW_TO_TEST.md) covers unit and golden tests, schema management, and trying queries. Code is in [`internal/oe`](../../internal/oe): `server` (routes), `federator` (query splitting and merging), `auth`, `policy` and `consent` (clients), `provider`, `services` and `database` (schema versions), `configs`.

## More docs

- [CONFIGURATION.md](CONFIGURATION.md): all `config.json` keys, environment variables, error codes, schema resolution
- [AUTHENTICATION.md](AUTHENTICATION.md): token validation, `trustUpstream`, JWKS handling
- [PROVIDER_CONFIGURATION.md](PROVIDER_CONFIGURATION.md): adding a data provider
- [HOW_TO_TEST.md](HOW_TO_TEST.md): testing and manual checks
- [Policy Decision Point](../pdp/README.md) and [Consent Engine](../ce/README.md): the services the OE calls
