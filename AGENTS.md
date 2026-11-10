# AGENTS.md

This file provides guidance to AI coding agents working with code in this repository.

## Commands

One Go module at the repo root (`github.com/openndx/openndx-core`) holds every service; `tests/integration` is a **separate module**. Each service's entrypoint is `cmd/<svc>` and its code is `internal/<svc>`, so test both paths:

| Service | Make name | Go packages |
|---|---|---|
| Orchestration Engine | `orchestration-engine` | `./cmd/oe/... ./internal/oe/...` |
| Policy Decision Point | `policy-decision-point` | `./cmd/pdp/... ./internal/pdp/...` |
| Consent Engine | `consent-engine` | `./cmd/ce/... ./internal/ce/... ./internal/database/...` |
| Portal Backend | `portal-backend` | `./cmd/pb/... ./internal/pb/...` |

```bash
make setup-all                          # git hooks, Go modules, portal deps
make validate-test <service>            # go test -race with coverage
make quality-check <service>            # gofumpt/goimports, vet, lint, gosec
make run <service>                      # run natively (needs cmd/<svc>/config.yaml)
go test ./internal/pdp/services -run TestPolicyMetadataService_PatchPolicyMetadata -count=1
golangci-lint run --new-from-rev=HEAD ./internal/... ./cmd/...   # only issues you introduced
docker compose up --build               # full local stack
```

- Unit tests need no database: PDP and PB tests use in-memory SQLite (PDP helpers in `internal/pdp/testhelpers`, PB in `internal/pb/database/dbtest`).
- OE federator golden tests pin end-to-end behaviour in `internal/oe/federator/testdata/golden`. After an intentional change: `go test ./internal/oe/federator -run TestFederateQueryGolden -update`.
- PDP policies are Rego with `_test.rego` files beside them in `internal/pdp/engine/policies`; `go test ./internal/pdp/engine` runs them.
- Integration tests run against live services on fixed ports (PDP 8082, CE 8081, OE 4000; PDP database on 5433 as `policy_db`): `cd tests/integration && ./run-local-tests.sh`, or bring up `docker-compose.test.yml` and `go test ./...`. Tests skip rather than fail when a dependency is unreachable. Some PB unit tests expect nothing to be listening on `localhost:8082`.
- `TestLooksLikeIDImprovedLogic` in `internal/monitoring` fails on `main`; it is not caused by your change.

## Architecture

A consumer sends one GraphQL query to the **Orchestration Engine**. For every requested field the OE asks the **Policy Decision Point** for a decision, asks the **Consent Engine** for the owner's consent when the decision requires it, then splits the query into sub-queries to the **data providers**, runs them in parallel and merges the result. The **Portal Backend** is the management API (members, schemas, applications, submissions); it keeps the PDP's policy metadata and application access and the identity provider's (ThunderID) clients in step with what it stores. The `ondx` CLI (`cmd/ondx`) drives PB. Each core service owns its own PostgreSQL database.

**OE (`internal/oe`)**: `federator` holds the pipeline: `schema` compiles the consumer schema, where `@sourceInfo(schemaId, providerField)` maps each field to a provider schema field; `plan` and `execute` build and run provider sub-queries; `merge` assembles the response; `access` applies PDP and consent results. `policy` and `consent` are HTTP clients for the PDP and CE. If `pdp.url` is empty the OE skips authorization and consent entirely.

**PDP (`internal/pdp`)**: decisions are Rego policies evaluated with the embedded OPA library (`engine`, policies embedded with `//go:embed`). The router `openndx.pdp` dispatches each field to `openndx.classes.<policyClass>`; built-in classes are `allowlist`, `allowlist_consent`, `consent` and `open`. Facts (field metadata, the application's access) stay in Postgres and are passed to Rego as `input` (`app`, `field`, `grant`, `now`), never loaded into OPA's store. Outcomes are a fixed set (`allow`, `deny`, `expired`, `consent_required`) plus optional obligations; an unknown class or evaluation error denies the field (fail closed). See `cmd/pdp/POLICY_MODEL.md`.

**PB (`internal/pb`)**: one package per bounded context (`member`, `schema`, `application`), each with model, service and handler; `kernel` is the shared kernel, `caller` resolves scopes and the caller's member, `policy` is the PDP client and owns the PDP DTOs PB depends on, `idp/thunderid` the ThunderID client. Contexts expose handlers only: **every route is registered explicitly in `cmd/pb/main.go`**. Multi-system writes (DB, IdP, PDP) are sagas with compensation on failure. PDP access is keyed by the IdP `client_id`, the identity the OE reads from consumer tokens.

**Shared code**: `internal/authn` (token validation), `internal/database` (connections, built on `OpenNSW/core/database`), `internal/logging`, `internal/monitoring`. Before writing a local helper, check whether an `OpenNSW/core` package (`configyaml`, `remote`, `secret`, `database`, `cors`, `authz`, `shared`) already covers it.

**Configuration**: each service reads one YAML file (`-config`, else `CONFIG_PATH`, else `config.yaml`), loaded with `OpenNSW/core/configyaml`. A value of `"{{env:NAME}}"` or `"{{file:/path}}"` is resolved at startup. Copy `cmd/<svc>/config.example.yaml` to the git-ignored `config.yaml`; Compose mounts `config.docker.yaml`. Each `cmd/<svc>/README.md` lists the keys.

**Database schemas**: `cmd/<svc>/schema.sql` is applied by hand (services don't create tables) and must stay safe to re-run. There are no versioned migrations; schema changes go straight into `schema.sql`, which the integration test containers also use.

**Portals** (`portals/apps`, pnpm/TypeScript): only the Consent Portal is maintained. The Admin and Member portals are paused prototypes, excluded from release images and default checks; don't change them.

## Conventions

- Commit and PR titles are conventional commits with a scope, `!` for breaking changes, e.g. `refactor(pdp,pb)!: ...`. PR bodies use `.github/pull_request_template.md` with every section kept.
- Branches are `<type>/<issue#>-<slug>`. Work happens on forks; PRs target `openndx/openndx-core` `main`.
- Each service has an `openapi.yaml` and README in `cmd/<svc>`; update them with API changes.
