# AGENTS.md

This file provides guidance to AI coding agents working with code in this repository.

## Commands

One Go module at the repo root (`github.com/openndx/openndx-core`) holds every service; `tests/integration` is a **separate module**. Make targets work on the whole module. Each service's entrypoint is `cmd/<svc>` and its code is `internal/<svc>`, so test both paths:

| Service | Go packages |
|---|---|
| Orchestration Engine | `./cmd/oe/... ./internal/oe/...` |
| Policy Decision Point | `./cmd/pdp/... ./internal/pdp/...` |
| Consent Engine | `./cmd/ce/... ./internal/ce/... ./internal/database/...` |
| Portal Backend | `./cmd/pb/... ./internal/pb/...` |

```bash
make setup                              # git hook, Go modules, golangci-lint (version in the Makefile), portal deps
make test PKG=./internal/pdp/...        # go test -race; PKG defaults to ./...
make fmt                                # gofumpt and goimports
make lint                               # golangci-lint with .golangci.yml
make check                              # go mod tidy -diff, license-check, lint, test; the pre-commit hook runs it
make up / make down                     # full stack in Docker, with the mock providers
make deps                               # only the dependencies in Docker
make up OTEL=1                          # (or deps) also start a local OpenObserve for audit records
(cd cmd/oe && go run .)                 # run a service natively; reads cmd/oe/config.yaml
go test ./internal/pdp/fieldpolicy -run TestService_PatchPolicy -count=1
make lint LINT_FLAGS=--new-from-rev=HEAD   # only issues in uncommitted changes
```

- `make lint` and `make check` lint the whole module and fail on any issue; CI reports only issues on changed lines. `make lint LINT_FLAGS=--new-from-merge-base=main` shows what CI will report. The pre-commit hook can be skipped with `git commit --no-verify`.
- Unit tests need no database: PDP, PB and CE tests use in-memory SQLite (PDP helpers in `internal/pdp/pdptest`, PB in `internal/pb/database/dbtest`).
- OE federator golden tests pin end-to-end behaviour in `internal/oe/federator/testdata/golden`. After an intentional change: `go test ./internal/oe/federator -run TestFederateQueryGolden -update`.
- PDP decision rules are plain Go in `internal/pdp/decision`, with a table-driven test covering every flag and grant combination.
- Integration tests run against live services on fixed ports (PDP 8082, CE 8081, OE 4000, mock provider 8083; databases on 5433 as `policy_db` and 5434 as `consent_db`): `make test-integration`, or from `tests/integration` run `docker compose -f docker-compose.db.yml -f docker-compose.test.yml up -d --build` and then `go test ./...`. Tests skip rather than fail when a dependency is unreachable.

## Architecture

A consumer sends one GraphQL query to the **Orchestration Engine**. For every requested field the OE asks the **Policy Decision Point** for a decision, asks the **Consent Engine** for the owner's consent when the decision requires it, then splits the query into sub-queries to the **data providers**, runs them in parallel and merges the result. The **Portal Backend** is the management API (members, schemas, applications, submissions); it keeps the PDP's field policies and application access and the identity provider's (ThunderID) clients in step with what it stores. The `ondx` CLI (`cmd/ondx`) drives PB. The PDP, CE and PB each own a PostgreSQL database; the OE has none.

**OE (`internal/oe`)**: `federator` holds the pipeline: `schema` compiles the consumer schema, where `@sourceInfo(schemaId, providerField)` maps each field to a provider schema field; `plan` and `execute` build and run provider sub-queries; `merge` assembles the response; `access` applies PDP and consent results. `policy` and `consent` are HTTP clients for the PDP and CE. If `pdp.url` is empty the OE skips authorization and consent entirely.

**PDP (`internal/pdp`)**: each field's policy is two flags, `requiresAllowlist` and `requiresConsent`. `decision.Decide` combines them with the application's grant for the field (plain or consent bypass, with an expiry), loaded from Postgres. A consent bypass grant overrides consent. Outcomes are a fixed set (`allow`, `deny`, `expired`, `consent_required`) plus optional obligations (`audit.consent_bypassed`). A field policy missing either flag is rejected, never read as `false`. See `cmd/pdp/POLICY_MODEL.md`. The code is one package per bounded context, each with service and handler: `fieldpolicy` (field policies), `grant` (access grants) and `decision` (the rules and the decide endpoint); `pdptest` holds test helpers. `decision` reads fields and grants through `FieldReader` and `GrantReader`, ports it declares and `fieldpolicy` and `grant` implement, and imports neither. Every route is registered explicitly in `cmd/pdp/main.go`.

**PB (`internal/pb`)**: one package per bounded context (`member`, `schema`, `application`, `consentbypass`), each with service and handler, and a model when it stores data in PB; `kernel` is the shared kernel, `caller` resolves scopes and the caller's member, `policy` is the PDP client and owns the PDP DTOs PB depends on, `idp/thunderid` the ThunderID client. Contexts expose handlers only: **every route is registered explicitly in `cmd/pb/main.go`**. Multi-system writes (DB, IdP, PDP) are sagas with compensation on failure. PDP access is keyed by the IdP `client_id`, the identity the OE reads from consumer tokens.

**Shared code**: `internal/authn` (token validation), `internal/database` (connections, built on `OpenNSW/core/database`), `internal/logging`, `internal/monitoring`, `internal/audit` (audit records). Before writing a local helper, check whether an `OpenNSW/core` package (`configyaml`, `remote`, `secret`, `database`, `cors`, `authz`, `shared`, `telemetry`) already covers it.

**Audit**: the OE and PB write audit records through `internal/audit` as slog records with `category=audit`, to a logger whose only handler is the OTLP log exporter from `OpenNSW/core/telemetry` (the `otel` config section). Records never reach stdout and are dropped when log export is off. The record shape matches `OpenNSW/core/platform/audit`'s, so keep the attribute names in step with it. Audit metadata never carries query text, arguments or provider data.

**Configuration**: each service reads one YAML file (`-config`, else `CONFIG_PATH`, else `config.yaml`), loaded with `OpenNSW/core/configyaml`. A value of `"{{env:NAME}}"` or `"{{file:/path}}"` is resolved at startup. Copy `cmd/<svc>/config.example.yaml` to the git-ignored `config.yaml`; Compose mounts `config.docker.yaml`. Each `cmd/<svc>/README.md` lists the keys.

**Database schemas**: `cmd/<svc>/schema.sql` is applied by hand (services don't create tables) and must stay safe to re-run. There are no versioned migrations; schema changes go straight into `schema.sql`, which the integration test containers also use.

**Portals** (`portals/apps`, pnpm/TypeScript): only the Consent Portal is maintained. The Admin and Member portals are paused prototypes, excluded from release images and default checks; don't change them.

## Conventions

- Commit and PR titles are conventional commits with a scope (omitted for whole-repo changes), `!` for breaking changes, e.g. `refactor(pdp,pb)!: ...`. PR bodies use `.github/pull_request_template.md` with every section kept.
- Branches are `<type>/<issue#>-<slug>`. Work happens on forks; PRs target `openndx/openndx-core` `main`.
- Every source file starts with the SPDX header from `.github/license-header.tmpl`, in the file type's comment syntax (Helm templates use a `{{- /* */ -}}` template comment). `make license` adds it to new files; CI fails on a file without it. `scripts/license-headers.sh` lists the exclusions.
- Each service has an `openapi.yaml` and README in `cmd/<svc>`. Keep the `openapi.yaml` current: any change to a route, request or response body, query parameter, status code, error code, required scope or validation rule updates the spec in the same PR. The spec describes what the code does; where they disagree and the code is wrong, file an issue rather than documenting the intended behaviour as if it shipped.
- Comments and docs describe the code as it is now: what it does and why. Leave out its history (what it used to do, what was removed or renamed, which PR changed it); commit messages and release notes keep that.
