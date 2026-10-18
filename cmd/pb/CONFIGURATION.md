# PB configuration reference

Every environment variable the Portal Backend reads. The short version is in the [README](README.md#configuration); [.env.example](.env.example) is a template. PB loads no `.env` file, so export the variables or pass `--env-file` to Docker. Compose values come from [compose.yml](../../compose.yml).

"Code default" is what PB uses when the variable is unset. Several differ from the Compose values, so check which you are running with.

## Server

| Variable | Code default | Compose | Notes |
|---|---|---|---|
| `PORT` | `8083` | `8083` | |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:5173` | `*` | Comma-separated. Credentials are always allowed; set explicit origins outside local development. |

## Database

| Variable | Code default | Compose | Notes |
|---|---|---|---|
| `DB_HOST`, `DB_PORT` | `localhost`, `5432` | `postgres`, `5432` | |
| `DB_USERNAME` | `postgres` | `postgres` | |
| `DB_PASSWORD` | `password` | `1234` | |
| `DB_NAME` | `testdb2` | `portal_backend` | |
| `DB_SSLMODE` | `require` | `disable` | |
| `RUN_MIGRATION` | unset | `true` | Migrations run only when the value is exactly `true`. They first drop the legacy `members.idp_user_id` column, then auto-migrate `members`, `schemas`, `applications`, `schema_submissions` and `application_submissions`. There are no versioned migrations. |

The connection pool is fixed: 25 open and 5 idle connections, 1 hour lifetime, 30 minute idle time.

## Token validation

| Variable | Code default | Notes |
|---|---|---|
| `IDP_AUDIENCE` | none | **Required.** PB's resource-server identifier (`http://pb.openndx.local` in the bundled stack). |
| `IDP_BASE_URL` | none | Derives the two URLs below when they are not set. Required unless `IDP_JWKS_URL` and an issuer are set. |
| `IDP_JWKS_URL` | `IDP_BASE_URL` + `/oauth2/jwks` | Signing keys. |
| `IDP_ISSUER` | `IDP_TOKEN_URL`, else `IDP_BASE_URL` + `/oauth2/token` | Must equal the tokens' `iss`. Compose sets it to ThunderID's public URL. |
| `IDP_JWKS_INSECURE_SKIP_VERIFY` | `false` | Skip TLS verification when fetching the JWKS. Development only. |

## Identity provider management client

PB creates and deletes each application's OAuth2 client through ThunderID's management API.

| Variable | Code default | Notes |
|---|---|---|
| `IDP_CLIENT_ID`, `IDP_CLIENT_SECRET` | none | **Required.** PB's own client (`PORTAL_BACKEND` in the bundled stack). |
| `IDP_BASE_URL` | none | ThunderID base URL for management calls. If unset, falls back to `IDP_ISSUER` or `IDP_TOKEN_URL` when `IDP_JWKS_URL` is set. |
| `IDP_SCOPE` | none | Space-separated scopes for the management token (`system`). |
| `IDP_TOKEN_RESOURCE` | none | Resource server the `system` scope belongs to. ThunderID needs it for that scope. |
| `IDP_DATA_CONSUMER_ROLE_ID` | none | Role assigned to every created application client, which grants it `getdata`. If empty, role assignment is skipped. |
| `IDP_TOKEN_URL` | none | Only a fallback when deriving the issuer and base URL. |

## Policy Decision Point and audit

| Variable | Code default | Notes |
|---|---|---|
| `PDP_SERVICE_URL` | none | **Required.** Must start with `http://` or `https://`. PB calls the PDP without authentication. |
| `AUDIT_SERVICE_URL` | none | Argus URL. Auditing is off when empty. |
| `ARGUS_API_KEY` | none | Falls back to `ARGUS_AUTH_TOKEN`. |
| `ENABLE_AUDIT` | on | Read by the audit client library; `false` disables auditing. |

## Build metadata

The Dockerfile sets `main.Version`, `main.BuildTime` and `main.GitCommit` with linker flags from the `BUILD_VERSION`, `BUILD_TIME` and `GIT_COMMIT` build arguments. They appear in the startup log and in `GET /debug`.

## Startup failures

PB exits at startup if it cannot connect to the database, if `PDP_SERVICE_URL`, `IDP_CLIENT_ID`, `IDP_CLIENT_SECRET` or a usable IdP base URL is missing, or if `IDP_AUDIENCE`, the JWKS URL or the issuer is missing or not a valid URL.
