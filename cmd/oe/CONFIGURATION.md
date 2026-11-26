# OE configuration reference

Every setting the Orchestration Engine reads. The overview and the short version are in the [README](README.md). Working examples: [config.example.yaml](config.example.yaml) (native run) and [config.docker.yaml](config.docker.yaml) (Docker Compose).

## Where configuration comes from

All settings are in one YAML file, read once at startup. The OE looks for it at the `-config` flag, then `CONFIG_PATH`, then `config.yaml` in the working directory. It exits at startup if the file is missing or a setting is invalid. Keys left out keep the defaults below. `cmd/oe/config.yaml` is git-ignored.

Run natively from `cmd/oe` (`cd cmd/oe && go run .`), the default is `cmd/oe/config.yaml`. The image sets `CONFIG_PATH=/app/config/config.yaml`, and Compose mounts [config.docker.yaml](config.docker.yaml) there.

A value written as `"{{env:NAME}}"` or `"{{file:/path}}"` is read from that environment variable or file at startup, and an unset variable stops the OE. Use it for secrets, such as `apiKey: "{{env:ARGUS_API_KEY}}"`. The whole value must be the placeholder.

## config.yaml

| Key | Default | Notes |
|---|---|---|
| `server.port` | `4000` | |
| `server.timeout` | `30s` | Bounds reading a request and writing its response, so it must cover the PDP, consent and provider calls a query makes. |
| `environment` | none | Deployment name (for example `local`, `staging`), added to every log line as `environment`. Left out of log lines when empty. |
| `logging.level` | `info` | `debug`, `info`, `warn` or `error`. |
| `logging.format` | `json` | `json` or `text`. Every line carries `service`, `version` and, when set, `environment`. |
| `schemaPath` | `schema.graphql` | The consumer schema file. A relative path is resolved against the directory of the config file. See [Schema](#schema). |
| `idp` | | Consumer token validation. `idp.jwksURL`, `idp.issuer` and `idp.audience` are **required**. See [idp](#idp). |
| `pdp.url` | | Policy Decision Point base URL. The OE appends `/api/v1/policy/decide`. If empty, the PDP and consent checks are skipped. |
| `consentEngine.url` | | Consent Engine URL, which must end in `/internal/api/v1` because the OE appends `/consents`. If the PDP requires consent and this is empty, the request fails with `CE_ERROR`. |
| `consentEngine.ownerIdArgName` | | **Required when `consentEngine.url` is set.** The query argument that carries the ID of the data owner whose consent is asked, matched by name (for example `nic`) on every field of the query, aliases included. It is read only when the PDP requires consent, so a query that needs none can be keyed by any argument (for example a vehicle's `regNo`). When consent is required and the query has no non-empty string value for it, the OE returns `MISSING_IDENTIFIER`; if it has several different values, `AMBIGUOUS_IDENTIFIER`. |
| `audit.url` | | Argus audit service URL. Auditing is off when this is empty. |
| `audit.apiKey` | | Sent as `X-API-Key` to Argus. |
| `audit.actorType`, `audit.actorId` | `SERVICE`, `orchestration-engine` | The actor recorded on audit events. |
| `providers` | | Data providers; see below and [PROVIDER_CONFIGURATION.md](PROVIDER_CONFIGURATION.md). |
| `argMappings` | | Argument mappings; see below. |

Unknown keys are ignored, so check the spelling of a key whose setting doesn't seem to take effect.

### providers[]

Each entry is an [`OpenNSW/core/remote`](https://github.com/OpenNSW/core/tree/main/remote) service.

| Key | Notes |
|---|---|
| `id` | **Required.** The provider schema's ID, as registered in Portal Backend and the PDP. Unique; identifies the provider and must match `schemaId` in the schema's `@sourceInfo` and in `argMappings`. If one endpoint serves several schemas, add an entry for each. |
| `url` | **Required.** The provider's GraphQL endpoint, `http` or `https`. The OE sends the sub-query as a `POST` to it. |
| `timeout` | Bounds each call, as a Go duration. Default `10s`. |
| `auth.type` | `apiKey`, `bearer` or `oauth2`. Omit `auth` for an open provider. |
| `auth.apiKey.key`, `auth.apiKey.value` | For `apiKey`: the header name and value sent with each request. |
| `auth.bearer.token` | For `bearer`: sent as `Authorization: Bearer <token>`. |
| `auth.oauth2.tokenURL`, `auth.oauth2.clientID`, `auth.oauth2.clientSecret`, `auth.oauth2.scopes` | For `oauth2`: client-credentials flow, with the client credentials sent as HTTP Basic auth. `scopes` is a string list. |
| `auth.oauth2.endpointParams` | Extra parameters for the token request, as a map of name to a string or a list of strings, for example `resource`. `grant_type`, `scope`, `client_id` and `client_secret` are rejected at startup: the flow sets them. |
| `auth.oauth2.insecureSkipTLSVerify` | `true` skips TLS certificate checks on the token request. Local development only. |
| `tls.clientCertFile`, `tls.clientKeyFile` | Client certificate for mutual TLS. Both are required when `tls` is set. |

### argMappings[]

Each entry maps an argument in the consumer's query to an argument of one provider's query.

The first four keys are required.

| Key | Notes |
|---|---|
| `schemaId` | The provider this mapping applies to, its `id` in `providers`. The OE exits at startup if no provider has that `id`. |
| `targetArgName` | The argument name the provider expects. |
| `sourceArgPath` | The consumer-side argument: the consumer field path from the root, then the argument name, joined with dashes. `personInfo-nic` is the `nic` argument of the root field `personInfo`; an argument with the same name on another field doesn't match. |
| `targetArgPath` | The provider field the argument is attached to: a leading part of the provider field path, for example `person` for the field `person.fullName`. The argument is attached to that field only, not to the fields above it. |
| `targetArgType` | Optional. The type of the provider's argument in the provider's schema, written as in GraphQL, for example `ID!` or `[String!]`. Set it when the provider's type differs from the consumer's: a different scalar, enum or input type name. The OE exits at startup if it isn't a GraphQL type. |

A mapped argument is sent with every provider field under its `targetArgPath` that is selected below the consumer field carrying the argument. When aliases give the same mapped argument different values, the OE sends a separate provider query for each value.

The OE sends each mapped argument as a variable in the [GraphQL over HTTP](https://graphql.github.io/graphql-over-http/draft/) `variables` member, not written into the query text. The value is the consumer's, after variable defaults and schema defaults are applied, and it may be a list or an input object. For example, with the mapping above, the DRP provider receives:

```json
{
  "query": "query Querydrp_person_v1($nic: ID!) { person(nic: $nic) { fullName } }",
  "variables": {"nic": "748213"}
}
```

The variable is named after `targetArgName`, with a suffix such as `_2` when two mapped arguments in one query share a name. It is declared with `targetArgType` when set. Otherwise it is declared with the consumer argument's type, made non-null when the value isn't null. The provider rejects the query when that type doesn't fit its argument, so set `targetArgType` when the types differ. The example above is declared `ID!` because the DRP mapping sets `targetArgType: "ID!"`: the consumer schema declares `nic: String!`, and a `String!` variable can't be passed to the provider's `ID!` argument, even though an inlined string literal could.

### idp

How the OE authenticates data consumers on `POST /public/graphql`. The only other route, `/health`, is not authenticated. Tokens are validated by the shared [`internal/authn`](../../internal/authn) package, which the Portal Backend and the Consent Engine also use. Every token's signature is verified; no setting skips it.

| Key | Default | Notes |
|---|---|---|
| `idp.jwksURL` | | **Required.** Where the signing keys are fetched. Must be an `http` or `https` URL. |
| `idp.issuer` | | **Required.** The expected `iss`. Must be an `http` or `https` URL, equal to the `iss` the identity provider puts in its tokens. ThunderID's is its public URL (`http://localhost:8090` locally), not its `/oauth2/token` endpoint. |
| `idp.audience` | | **Required.** The expected `aud`: the OE's resource server identifier. |
| `idp.insecureSkipTLSVerify` | `false` | `true` skips TLS certificate checks when fetching the JWKS. Local development only. |
| `idp.claims.email.required`, `idp.claims.ouId.required`, `idp.claims.ouHandle.required` | `false` | `true` rejects a token whose claim is absent or blank, client-credentials tokens included, which usually carry none of them. |

The OE exits at startup if `jwksURL`, `issuer` or `audience` is missing or invalid. In Docker Compose, [config.docker.yaml](config.docker.yaml) fetches the keys from ThunderID's address on the Compose network (`http://thunderid:8090/oauth2/jwks`), and reads the issuer from `IDP_PUBLIC_URL` (ThunderID's public URL) and the audience from `IDP_AUDIENCE` (default `http://api.openndx.local`).

A consumer sends an access token in the `Authorization: Bearer <token>` header. In the bundled stack, consumers are machine clients that get a ThunderID `client_credentials` token requested with `resource=http://api.openndx.local`; see the example in [dev/README.md](../../dev/README.md). The token must pass these checks:

| Check | Requirement |
|---|---|
| Signature | `RS256`, `RS384` or `RS512`, signed by a key in the JWKS at `idp.jwksURL`, with a `kid` header that names the key. |
| `iss` | Equals `idp.issuer`. |
| `aud` | Contains `idp.audience`. |
| `exp` | Present and in the future, with 30 seconds of leeway. `nbf`, when present, must not be in the future, with the same leeway. |
| `sub` | Present. |
| `client_id` | Present. It becomes the application's identity (`applicationId`) for the PDP and the Consent Engine. |

A request without a valid token gets `401` with a JSON body: `{"error":"unauthorized","message":"authentication required"}` when there is no token, and `{"error":"unauthorized","message":"invalid authentication token"}` when the token fails a check. The reason is logged at debug level, not returned.

The signing keys are fetched on the first request, not at startup, so the OE starts even when the identity provider is not yet up. They are cached for 5 minutes. A token whose `kid` is not in the cached set triggers one refresh, so key rotation needs no restart.

In production:

- set `idp.jwksURL` and `idp.issuer` to the production identity provider over `https`, and `idp.audience` to the OE's resource server;
- leave `idp.insecureSkipTLSVerify` unset;
- terminate TLS in front of the OE, and expose only `POST /public/graphql`.

## Environment variables

Apart from the placeholders in the config file, the OE reads only `CONFIG_PATH`. The audit client library also reads `ENABLE_AUDIT`: auditing is on when `audit.url` is set and `ENABLE_AUDIT` isn't `false`. The OE does not load `.env` files. It records no metrics and serves no `/metrics`.

## Schema

The OE reads the consumer GraphQL schema from the file named by `schemaPath` once at startup. The default is `schema.graphql`, and a relative path is resolved against the directory that contains the config file, so by default the schema sits next to the config. An absolute path is used as is. The OE exits at startup if the file is missing or isn't a valid schema (see below). In Compose both files are mounted in `/app/config`.

The image contains only the binary, so the schema, like the config file, is mounted into the container. To change the schema, mount the new file (for example from a Kubernetes ConfigMap) and restart the OE or roll out the pods. Every OE instance then serves the same schema. There is no API for changing the schema at runtime, and the OE uses no database.

At startup the OE validates the schema against the GraphQL spec (with [gqlparser](https://github.com/vektah/gqlparser)) and compiles it once.

Leaf fields in the schema must carry `@sourceInfo(schemaId, providerField)`, with both set. The schema may declare the directive itself; if it doesn't, the OE adds `directive @sourceInfo(schemaId: String!, providerField: String!) on FIELD_DEFINITION`. See [schema.graphql](schema.graphql) for a sample.

A list field with `@sourceInfo` points at a list in the provider's response, and its item fields are read from each item. An item field's `providerField` is either the full path, starting with the list's path (`vehicle.getVehicleInfos.data.make`), or just the field's name in the item (`make`).

Every consumer query is validated against the schema before the OE calls the PDP: unknown fields, wrong argument types and missing required arguments are rejected with `BAD_REQUEST`.

### Queries

The OE follows the GraphQL spec's field collection: fragments and inline fragments are merged, `@skip` and `@include` are applied, and the response uses the query's aliases. Only the provider fields a query actually selects are sent to the PDP and fetched, each provider once per set of mapped argument values. `__typename` is answered by the OE.

Not supported, and rejected with `BAD_REQUEST`:

- selections on fields of interface or union type, because the OE can't tell which type a provider returned;
- selections on a list field without `@sourceInfo`, because there is no provider list to read the items from;
- introspection (`__schema`, `__type`).

## Error codes

GraphQL errors carry the code in `errors[].extensions.code`. Federation errors use HTTP 200.

| Code | Meaning |
|---|---|
| `PDP_NOT_ALLOWED` | The application is not authorized for some field, or its access has expired. `unauthorizedFields` or `expiredFields` lists them. |
| `PDP_ERROR` | The PDP call failed, or the PDP asked for consent but listed no fields. A field with no field policy causes this. |
| `PDP_NO_RESPONSE` | The PDP returned nothing. |
| `CE_NOT_APPROVED` | Consent is pending or otherwise not approved. `consentPortalUrl` and `consentStatus` tell the consumer where and what. |
| `CE_ERROR` | The Consent Engine call failed, or consent is needed and no Consent Engine is configured. |
| `CE_NO_RESPONSE` | The Consent Engine returned nothing. |
| `MISSING_IDENTIFIER` | The PDP requires the data owner's consent, and the query has no non-empty string value for the `consentEngine.ownerIdArgName` argument. When the schema declares that argument as required (for example `nic: String!`), a query without it fails validation with `BAD_REQUEST` instead. |
| `AMBIGUOUS_IDENTIFIER` | The PDP requires the data owner's consent, and the query has that argument with different values, for example on two aliased fields. The Consent Engine isn't called. |
| `BAD_REQUEST` | The query could not be parsed, failed validation against the schema, has invalid variables, or uses something the OE doesn't support (see [Queries](#queries)). Parse and validation errors carry `locations` in the query. |
| `PROVIDER_ERROR` | A provider call failed: the provider was unreachable or not configured, answered with a non-2xx HTTP status, or sent a body that isn't a GraphQL response. Also used for errors a provider returned in its response, with the message prefixed by the provider's schema ID. |

Mutations and subscriptions are rejected: with `BAD_REQUEST` when the schema has no `Mutation` or `Subscription` type, otherwise with the message `Only query operation is supported` and no code.

A provider failure doesn't fail the whole query. The fields that provider serves are `null`, each with a `PROVIDER_ERROR` at its `path`, and the other providers' data is returned. As in the GraphQL spec, a `null` in a non-null field (`String!`) makes its parent `null` instead, up to the nearest field that can be `null`; the error then reads `Cannot return null for non-nullable field …`. If that reaches a non-null root field, `data` is `null`.

HTTP-level errors: `400` for an unreadable request body, `401` with a JSON body such as `{"error":"unauthorized","message":"invalid authentication token"}` for a missing or invalid token.
