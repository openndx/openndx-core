# PB configuration reference

The Portal Backend reads one YAML file: the `-config` flag, else `CONFIG_PATH`, else `config.yaml` in the working directory. The short version is in the [README](README.md#configuration). Templates:

- [config.example.yaml](config.example.yaml): native runs. Copy it to the git-ignored `config.yaml`.
- [config.docker.yaml](config.docker.yaml): mounted by [compose.yml](../../compose.yml), which passes in the shared values from [.env.example](../../.env.example).

A value written as `"{{env:NAME}}"` or `"{{file:/path}}"` is read from that environment variable or file at startup. The placeholder must be the whole value, and an unset or empty variable stops the service. Keys left out keep the defaults below; keys PB does not know are ignored.

## Server and logging

| Key | Default | Notes |
|---|---|---|
| `server.port` | `8083` | Listen port. The server binds on all interfaces. |
| `server.timeout` | `15s` | Read and write timeout. |
| `environment` | none | Deployment name (for example `local`, `staging`), added to every log line as `environment`. Left out of log lines when empty. |
| `logging.level` | `info` | `debug`, `info`, `warn` or `error`. |
| `logging.format` | `json` | `json` or `text`. Every line carries `service`, `version` and, when set, `environment`. |

## CORS

Applies to `/api/v1/`. Uses [`OpenNSW/core/cors`](https://github.com/OpenNSW/core/tree/main/cors).

| Key | Default | Notes |
|---|---|---|
| `cors.allowedOrigins` | `[http://localhost:5173]` | Browser origins of the member and admin portals. `*` stops the service at startup while `allowCredentials` is true. |
| `cors.allowedMethods`, `cors.allowedHeaders` | see [config.go](config.go) | A list in the file replaces the default list. |
| `cors.allowCredentials`, `cors.maxAge` | `true`, `86400` | |

## Token validation

| Key | Default | Notes |
|---|---|---|
| `idp.jwksURL` | none | **Required.** Signing keys. |
| `idp.issuer` | none | **Required.** Must equal the tokens' `iss`: ThunderID's public URL, not `/oauth2/token`. |
| `idp.audience` | none | **Required.** PB's resource-server identifier (`http://pb.openndx.local` in the bundled stack). |
| `idp.insecureSkipTLSVerify` | `false` | Skip TLS verification when fetching the JWKS. Development only. |
| `idp.claims.email.required`, `.ouId.required`, `.ouHandle.required` | `false` | `true` rejects a token whose claim is absent or blank, client-credentials tokens included. |

## Identity provider management client

PB creates and deletes each application's OAuth2 client through ThunderID's management API.

| Key | Default | Notes |
|---|---|---|
| `idp.management.baseURL` | none | **Required.** ThunderID base URL for management calls. PB waits up to 15 seconds for each call. |
| `idp.management.clientID`, `.clientSecret` | none | **Required.** PB's own client (`PORTAL_BACKEND` in the bundled stack). |
| `idp.management.scopes` | empty | Scopes for the management token (`[system]`). |
| `idp.management.resource` | empty | Resource server the `system` scope belongs to. ThunderID needs it for that scope. The bundled stack sets `https://localhost:8090/mcp`, the fixed identifier of ThunderID's built-in System resource server. |
| `idp.management.dataConsumerRoleID` | empty | Role assigned to every created application client, which grants it `getdata`. If empty, role assignment is skipped. The bundled stack sets the ID of the `DataConsumerGetData` role in [resource.yaml](../../thunderid/bootstrap/resource.yaml). |

## Policy Decision Point

| Key | Default | Notes |
|---|---|---|
| `pdp.url` | none | **Required.** `http` or `https` URL. PB calls the PDP without authentication, and waits up to 10 seconds for each call. |

## OpenTelemetry and audit

OpenTelemetry export over OTLP/HTTP, through [`OpenNSW/core/telemetry`](https://github.com/OpenNSW/core/tree/main/telemetry). It carries PB's audit records, which go nowhere else. What the records hold, and what happens when export is off or fails, is in [internal/audit](../../internal/audit/README.md).

| Key | Default | Notes |
|---|---|---|
| `otel.endpoint` | empty | OTLP/HTTP base URL, for example `http://otel-collector:4318`. `/v1/logs` (and `/v1/traces`, `/v1/metrics`) is appended. Empty turns export off. |
| `otel.logs.endpoint` | empty | Full URL for logs alone, path included. Overrides `otel.endpoint` for logs. `otel.traces.endpoint` and `otel.metrics.endpoint` exist too; PB produces no traces or metrics. |
| `otel.serviceName` | `portal-backend` | The `service.name` resource attribute. |
| `otel.headers` | empty | Sent with every export, for example `Authorization: "{{env:OTEL_AUTH_HEADER}}"`. |

The commented `otel` blocks at the end of [config.example.yaml](config.example.yaml) and [config.docker.yaml](config.docker.yaml) point at the local OpenObserve that `make up OTEL=1` and `make deps OTEL=1` start; [Viewing audit records](../../CONTRIBUTING.md#viewing-audit-records) has the steps.

## Database

| Key | Default | Notes |
|---|---|---|
| `database.driver` | `postgres` | Only `postgres` is supported. |
| `database.postgres.host`, `.user`, `.password` | none | **Required.** |
| `database.postgres.port` | `5432` | |
| `database.postgres.name` | `portal_backend` | |
| `database.postgres.sslMode` | `require` | Use `disable` for a local PostgreSQL without TLS. |
| `database.postgres.pool.maxOpenConns`, `.maxIdleConns`, `.maxConnLifetimeSeconds` | `25`, `5`, `3600` | Connection pool. |
| `database.idleTimeout` | `30m` | Closes connections idle this long. |

PB does not create or migrate tables. Load [schema.sql](schema.sql) into the database before the first start; Compose does this through [init-db.sql](../../init-db.sql).

## Build metadata

The Dockerfile sets `main.Version`, `main.BuildTime` and `main.GitCommit` with linker flags from the `BUILD_VERSION`, `BUILD_TIME` and `GIT_COMMIT` build arguments. They appear in the startup log.

## Startup failures

PB checks the file at startup and exits with an error naming the bad key: a missing required key, an unset placeholder variable, a URL that is not `http` or `https`, a port outside 1–65535, `*` in `cors.allowedOrigins` with credentials allowed, or an inconsistent pool. It also exits if it cannot connect to the database within 10 seconds.
