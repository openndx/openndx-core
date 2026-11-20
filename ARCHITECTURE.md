# Architecture

OpenNDX is a data exchange. A consumer sends one GraphQL query; the platform decides each field, asks the person when the field requires consent, fetches the allowed fields from the organizations that hold them, and returns one answer. Who it is for, and the local ports, are in the [README](README.md).

## System context

```mermaid
flowchart LR
    Consumer[Data consumer app] -->|GraphQL| OE[Orchestration Engine]
    OE -->|access decision| PDP[Policy Decision Point]
    OE -->|consent| CE[Consent Engine]
    OE -->|sub-queries| PR[Data providers]
    Citizen --> CP[Consent Portal]
    CP --> CE
    Operator[Operators and member staff] --> CLI[ondx CLI]
    CLI --> PB[Portal Backend]
    PB -->|policy metadata, allow-lists| PDP
    PB -->|application clients| IDP[Identity provider]
    CP -.->|sign in| IDP
    CLI -.->|sign in| IDP
    Consumer -.->|client credentials| IDP
    CE -.->|verify tokens| IDP
    PB -.->|verify tokens| IDP
    OE -.->|audit events| AU[Audit service]
    PB -.->|audit events| AU
```

The identity provider in Docker Compose is ThunderID. The audit service is [Argus](https://github.com/LSFLK/argus), and it is optional. Data providers are systems the operator connects. Sample providers for local use are under [dev/](dev/README.md).

## Request flow

```mermaid
sequenceDiagram
    participant C as Consumer
    participant OE as Orchestration Engine
    participant PDP as Policy Decision Point
    participant CE as Consent Engine
    participant P as Providers
    C->>OE: POST /public/graphql
    OE->>OE: validate token, plan one sub-query per provider
    OE->>PDP: POST /api/v1/policy/decide
    PDP-->>OE: allow, deny, expired, or consent_required
    opt consent required
        OE->>CE: POST /internal/api/v1/consents
        CE-->>OE: pending or approved
    end
    OE->>P: sub-queries, in parallel
    P-->>OE: provider data
    OE-->>C: merged response
```

Field policy is in [cmd/pdp/POLICY_MODEL.md](cmd/pdp/POLICY_MODEL.md). Consent states are in [cmd/ce/CONSENT_LIFECYCLE.md](cmd/ce/CONSENT_LIFECYCLE.md).

## Functional requirements

| ID | Requirement |
|---|---|
| FR-1 | A consumer sends one GraphQL query to the Orchestration Engine and receives one response. |
| FR-2 | The engine validates the caller's token against the identity provider's JWKS, issuer, and audience before it runs the query. |
| FR-3 | Every requested field is decided by the Policy Decision Point. A field missing either `requiresAllowlist` or `requiresConsent` is rejected at registration. |
| FR-4 | When the decision is `consent_required` and no approved consent exists, the Consent Engine records a pending consent and the response includes a consent link. The engine does not fetch the field. |
| FR-5 | Only the person whose token subject matches the consent's `ownerId` can approve, reject, or revoke it. |
| FR-6 | Approved consent and access grants expire. Revocation takes effect on the next read. |
| FR-7 | A consent bypass is stored only with a non-empty legal basis, and the decision reports the `audit.consent_bypassed` obligation. |
| FR-8 | The engine fetches the allowed fields from the providers in parallel and merges them. A provider failure nulls that provider's fields and still returns the others. |
| FR-9 | The Orchestration Engine has no database and does not persist provider payloads. |
| FR-10 | Members, schemas, applications, policy, and grants are managed through the Portal Backend, which keeps the Policy Decision Point and the identity provider in step with what it stores. |

## Technology stack

| Layer | Choice | Version in this repo |
|---|---|---|
| Services and `ondx` | Go | 1.26.4 (`go.mod`) |
| Consent Portal | TypeScript, React, Vite | React 19, Vite 7 |
| Portal builds | Node.js, pnpm | Node 24+ |
| Databases | PostgreSQL, one per service that stores data | 16 (`postgres:16-alpine`) |
| Consumer API | GraphQL (`gqlparser` v2) | |
| Management APIs | HTTP JSON, OpenAPI 3.0.3 | `cmd/*/openapi.yaml` |
| Caller authentication | OAuth 2.0 access tokens, JWT, checked via JWKS | |
| Operator login | OAuth 2.0 authorization code with PKCE | `ondx` |
| Reference identity provider | ThunderID | `ghcr.io/thunder-id/thunderid:1.0.1` |
| Shared libraries | `github.com/OpenNSW/core` | Apache-2.0 |
| Metrics | OpenTelemetry, Prometheus exposition | optional, [observability/](observability/README.md) |
| Audit | Argus | optional |

Unit tests for the Policy Decision Point and Portal Backend use SQLite in memory. The services that store data use PostgreSQL when deployed.

## Where to go next

| Task | Guide |
|---|---|
| Install and run locally | [README](README.md#quick-start) |
| Run one service, run tests, fork, upstream, downstream | [Development guide](docs/contributing/development.md) |
| Use a running system | [Walkthrough](dev/WALKTHROUGH.md) |
| Deploy a release tag or a fork | [Deploy](docs/DEPLOY.md) |
| Release notes, images, and the CLI | [Release guide](docs/RELEASE.md) |
