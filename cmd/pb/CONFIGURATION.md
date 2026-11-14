# PB configuration reference

The Portal Backend reads one YAML file: the `-config` flag, else `CONFIG_PATH`, else `config.yaml` in the working directory. The short version is in the [README](README.md#configuration). Templates:

- [config.example.yaml](config.example.yaml): native runs. Copy it to the git-ignored `config.yaml`.
- [config.docker.yaml](config.docker.yaml): mounted by [compose.yml](../../compose.yml), which passes in the shared values from [.env.example](../../.env.example).
- [config.test.yaml](config.test.yaml): mounted by [docker-compose.test.yml](docker-compose.test.yml).

A value written as `"{{env:NAME}}"` or `"{{file:/path}}"` is read from that environment variable or file at startup. The placeholder must be the whole value, and an unset or empty variable stops the service. Keys left out keep the defaults below; keys PB does not know are ignored.

The last column is the environment variable each key replaces, for upgrading from releases that configured PB through the environment.

## Server and logging

| Key | Default | Notes | Replaces |
|---|---|---|---|
| `server.port` | `8083` | Listen port. The server binds on all interfaces. | `PORT` |
| `server.timeout` | `15s` | Read and write timeout. | |
| `environment` | none | Deployment name (for example `local`, `staging`), added to every log line as `environment`. Left out of log lines when empty. | |
| `logging.level` | `info` | `debug`, `info`, `warn` or `error`. | |
| `logging.format` | `json` | `json` or `text`. Every line carries `service`, `version` and, when set, `environment`. | |

## CORS

Applies to `/api/v1/`. Uses [`OpenNSW/core/cors`](https://github.com/OpenNSW/core/tree/main/cors).

| Key | Default | Notes | Replaces |
|---|---|---|---|
| `cors.allowedOrigins` | `[http://localhost:5173]` | Browser origins of the member and admin portals. `*` stops the service at startup while `allowCredentials` is true. | `CORS_ALLOWED_ORIGINS` |
| `cors.allowedMethods`, `cors.allowedHeaders` | see [config.go](config.go) | A list in the file replaces the default list. | |
| `cors.allowCredentials`, `cors.maxAge` | `true`, `86400` | | |

## Token validation

| Key | Default | Notes | Replaces |
|---|---|---|---|
| `idp.jwksURL` | none | **Required.** Signing keys. | `IDP_JWKS_URL` |
| `idp.issuer` | none | **Required.** Must equal the tokens' `iss`: ThunderID's public URL, not `/oauth2/token`. | `IDP_ISSUER`, `IDP_TOKEN_URL` |
| `idp.audience` | none | **Required.** PB's resource-server identifier (`http://pb.openndx.local` in the bundled stack). | `IDP_AUDIENCE` |
| `idp.insecureSkipTLSVerify` | `false` | Skip TLS verification when fetching the JWKS. Development only. | `IDP_JWKS_INSECURE_SKIP_VERIFY` |

The JWKS URL and issuer are no longer derived from a base URL; set both.

## Identity provider management client

PB creates and deletes each application's OAuth2 client through ThunderID's management API.

| Key | Default | Notes | Replaces |
|---|---|---|---|
| `idp.management.baseURL` | none | **Required.** ThunderID base URL for management calls. | `IDP_BASE_URL` |
| `idp.management.clientID`, `.clientSecret` | none | **Required.** PB's own client (`PORTAL_BACKEND` in the bundled stack). | `IDP_CLIENT_ID`, `IDP_CLIENT_SECRET` |
| `idp.management.scopes` | empty | Scopes for the management token (`[system]`). | `IDP_SCOPE` (space-separated) |
| `idp.management.resource` | empty | Resource server the `system` scope belongs to. ThunderID needs it for that scope. | `IDP_TOKEN_RESOURCE` |
| `idp.management.dataConsumerRoleID` | empty | Role assigned to every created application client, which grants it `getdata`. If empty, role assignment is skipped. | `IDP_DATA_CONSUMER_ROLE_ID` |

## Policy Decision Point and audit

| Key | Default | Notes | Replaces |
|---|---|---|---|
| `pdp.url` | none | **Required.** `http` or `https` URL. PB calls the PDP without authentication. | `PDP_SERVICE_URL` |
| `audit.url` | empty | Argus URL. Auditing is off when empty. | `AUDIT_SERVICE_URL` |
| `audit.apiKey` | empty | | `ARGUS_API_KEY`, `ARGUS_AUTH_TOKEN` |

The audit client library still reads `ENABLE_AUDIT` itself; `false` disables auditing.

## Database

| Key | Default | Notes | Replaces |
|---|---|---|---|
| `database.driver` | `postgres` | Only `postgres` is supported. | |
| `database.postgres.host`, `.user`, `.password` | none | **Required.** | `DB_HOST`, `DB_USERNAME`, `DB_PASSWORD` |
| `database.postgres.port` | `5432` | | `DB_PORT` |
| `database.postgres.name` | `portal_backend` | | `DB_NAME` |
| `database.postgres.sslMode` | `require` | Use `disable` for a local PostgreSQL without TLS. | `DB_SSLMODE` |
| `database.postgres.pool.maxOpenConns`, `.maxIdleConns`, `.maxConnLifetimeSeconds` | `25`, `5`, `3600` | Connection pool. | |
| `database.idleTimeout` | `30m` | Closes connections idle this long. | |

PB does not create or migrate tables. Load [schema.sql](schema.sql) into the database before the first start; Compose does this through [init-db.sql](../../init-db.sql). The v0.6 `database.runMigration` key is no longer read.

A database that v0.6 migrated already matches `schema.sql` and needs no change. A database created before v0.5 keeps the legacy `members.idp_user_id` column unless v0.5 or v0.6 ran with `runMigration` on, and `schema.sql` does not remove it. Drop it once:

```sql
ALTER TABLE members DROP COLUMN IF EXISTS idp_user_id;
```

## Build metadata

The Dockerfile sets `main.Version`, `main.BuildTime` and `main.GitCommit` with linker flags from the `BUILD_VERSION`, `BUILD_TIME` and `GIT_COMMIT` build arguments. They appear in the startup log.

## Startup failures

PB checks the file at startup and exits with an error naming the bad key: a missing required key, an unset placeholder variable, a URL that is not `http` or `https`, a port outside 1–65535, `*` in `cors.allowedOrigins` with credentials allowed, or an inconsistent pool. It also exits if it cannot connect to the database within 10 seconds.
