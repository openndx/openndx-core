# OE configuration reference

Every setting the Orchestration Engine reads. The overview and the short version are in the [README](README.md). Working examples: [config.example.json](config.example.json) (native run) and [config.docker.json](config.docker.json) (Docker Compose).

## Where configuration comes from

1. A JSON file at `CONFIG_PATH`, default `./config.json` relative to the working directory. The service exits at startup if the file is missing or invalid. `cmd/oe/config.json` is git-ignored.
2. A few environment variables (below). The OE does not load `.env` files.

`make run orchestration-engine` runs from `cmd/oe`, so `./config.json` is `cmd/oe/config.json`. In Compose the file is mounted at `/app/config.json`.

## config.json

| Key | Type | Notes |
|---|---|---|
| `foundationalIdArgName` | string | **Required.** The query argument that carries the data owner's ID, matched by name (for example `nic`). Startup fails if empty. If a query has none the OE returns `MISSING_IDENTIFIER`; if it has several different values, `AMBIGUOUS_IDENTIFIER`. |
| `pdpUrl` | string | Policy Decision Point base URL. The OE appends `/api/v1/policy/decide`. If empty, the PDP and consent checks are skipped. `pdpConfig.clientUrl` overrides it. |
| `ceUrl` | string | Consent Engine URL, which must end in `/internal/api/v1` because the OE appends `/consents`. `ceConfig.clientUrl` overrides it. If the PDP requires consent and this is empty, the request fails with `CE_ERROR`. |
| `providers` | array | Data providers; see below and [PROVIDER_CONFIGURATION.md](PROVIDER_CONFIGURATION.md). Loaded once at startup, so changes need a restart. |
| `argMappings` | array | Argument mappings; see below. |
| `trustUpstream` | bool | `true` reads the token without verifying its signature. See [AUTHENTICATION.md](AUTHENTICATION.md). |
| `jwt.jwksUrl` | string | Signing keys. Required when `trustUpstream` is `false`. |
| `jwt.expectedIssuer` | string | The token's `iss` must equal this when set. |
| `jwt.validAudiences` | string array | The token's `aud` must contain one of these when set. |
| `environment` | string | `production` or `staging` enables the JWKS URL checks in [AUTHENTICATION.md](AUTHENTICATION.md#jwks-url-checks). Any other value disables them. |
| `schema` | string | Inline GraphQL SDL, used if there is no active schema in the database. |
| `auditConfig.serviceUrl` | string | Argus audit service URL. Overridden by `AUDIT_SERVICE_URL`. Auditing is off when this is empty. |
| `auditConfig.actorType`, `auditConfig.actorId` | string | Default `SERVICE` and `orchestration-engine`. |

Keys that are accepted but ignored: `server.port`, `log.level`, `services.pdp_url`, `sdl`.

### providers[]

| Key | Notes |
|---|---|
| `providerKey` | Unique name; must match `providerKey` in the schema's `@sourceInfo`. |
| `providerUrl` | The provider's GraphQL endpoint. The OE sends the sub-query as a `POST` to it. |
| `schemaId` | The provider schema's ID, as registered in Portal Backend and the PDP. `(providerKey, schemaId)` identifies a provider. |
| `auth.type` | `apiKey` or `oauth2`. Omit `auth` for an open provider. |
| `auth.apiKeyName`, `auth.apiKeyValue` | For `apiKey`: the header name and value sent with each request. |
| `auth.tokenUrl`, `auth.clientId`, `auth.clientSecret`, `auth.scopes` | For `oauth2`: client-credentials flow. `scopes` is a string array. |

### argMappings[]

Each entry maps an argument in the consumer's query to an argument of one provider's query.

| Key | Notes |
|---|---|
| `providerKey`, `schemaId` | The provider this mapping applies to. |
| `targetArgName` | The argument name the provider expects. |
| `sourceArgPath` | The consumer-side argument: a dash-separated path whose last segment is the argument name, for example `personInfo-nic`. |
| `targetArgPath` | Where the argument goes in the provider query: a leading part of the provider field path (the object the argument belongs to), for example `person` for the field `person.fullName`. |

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `4000` | |
| `CONFIG_PATH` | `./config.json` | |
| `DB_HOST`, `DB_PORT` | `localhost`, `5432` | |
| `DB_USERNAME` | `postgres` | |
| `DB_PASSWORD` | empty | A warning is logged when empty. |
| `DB_NAME` | `orchestration_engine` | |
| `DB_SSLMODE` | `disable` | |
| `AUDIT_SERVICE_URL` | | Overrides `auditConfig.serviceUrl`. |
| `ARGUS_API_KEY` | | Sent as `X-API-Key` to Argus. Falls back to `ARGUS_AUTH_TOKEN`. |
| `ENABLE_AUDIT` | on | Read by the audit client library. Audit is on when a URL is set and this is not `false`. |

[.env.example](.env.example) also lists `SERVICE_NAME`, `SERVICE_VERSION` and `OTEL_*`. The OE does not read them, and it does not serve `/metrics`. Compose passes some of them to every Go service.

If the database cannot be reached at startup the service keeps running, the `/sdl` routes return `503`, and queries fall back to the schema in `config.json` or `schema.graphql`.

## Schema resolution

For each query the OE picks the schema SDL in this order:

1. The **active schema** in the database (`unified_schemas` table, `is_active = true`).
2. The `schema` key in `config.json`.
3. A `schema.graphql` file in the working directory, or one or two directories above it.

If none is found the query fails with a "No active schema found" error. The OE creates its own tables at startup (`unified_schemas`; a `schema_versions` table is created but unused).

Leaf fields in the schema must carry `@sourceInfo(providerKey, schemaId, providerField)`. See [schema.graphql](schema.graphql) for a sample.

## Error codes

GraphQL errors carry the code in `errors[].extensions.code`. Federation errors use HTTP 200.

| Code | Meaning |
|---|---|
| `PDP_NOT_ALLOWED` | The application is not authorized for some field, or its access has expired. `unauthorizedFields` or `expiredFields` lists them. |
| `PDP_ERROR` | The PDP call failed, or the PDP asked for consent but listed no fields. A field with no policy metadata causes this. |
| `PDP_NO_RESPONSE` | The PDP returned nothing. |
| `CE_NOT_APPROVED` | Consent is pending or otherwise not approved. `consentPortalUrl` and `consentStatus` tell the consumer where and what. |
| `CE_ERROR` | The Consent Engine call failed, or consent is needed and no Consent Engine is configured. |
| `CE_NO_RESPONSE` | The Consent Engine returned nothing. |
| `MISSING_IDENTIFIER` | The query has no `foundationalIdArgName` argument. |
| `AMBIGUOUS_IDENTIFIER` | The query has that argument with different values. |
| `BAD_REQUEST` | The query could not be parsed. |

Some errors carry only a `message` and no code, for example `Only query operation is supported` (mutations and subscriptions are rejected) and `No active schema found`.

HTTP-level errors: `400` for an unreadable request body, `401` with the plain-text body `Unauthorized: invalid or expired token`.
