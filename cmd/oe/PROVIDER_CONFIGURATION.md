# Adding a data provider to the OE

How to make the Orchestration Engine fetch data from a new provider. The examples use the two mock providers from [dev/](../../dev/README.md): `drp` (API-key auth) and `rgd` (OAuth2). Every key is described in [CONFIGURATION.md](CONFIGURATION.md).

## What you need from the provider

- The provider's GraphQL endpoint URL.
- Its authentication details: an API key header, or OAuth2 client-credentials (token URL, client ID, secret, scopes).
- Its GraphQL schema: the query fields and their paths, for example `person.fullName`.

## 1. Register the schema and its policy metadata

The Policy Decision Point only answers for fields it has metadata for. A field with no metadata makes the whole query fail with `PDP_ERROR`. Register the provider's schema through Portal Backend, which creates the metadata in the PDP:

```bash
ondx schemas create --profile local \
  --schema-id drp-person-v1 --name "DRP Person Registry" \
  --endpoint http://localhost:9091 --member-id "$MEMBER_ID" \
  --field person.fullName:allowlist:primary \
  --field person.profession:allowlist_consent:primary
```

The `--schema-id` becomes the `schemaId`, and each `--field` name becomes the PDP `fieldName`. See [the walkthrough](../../dev/WALKTHROUGH.md#4-register-members-and-both-provider-schemas) for the full sequence.

## 2. Add the provider to `config.yaml`

Add an entry to `providers`. Its `id` is the schema ID from step 1, which identifies the provider. If one endpoint serves several schemas, add an entry for each.

```yaml
providers:
  - id: drp-person-v1
    url: http://localhost:9091
    auth:
      type: apiKey
      apiKey:
        key: Test-Key
        value: "{{env:DRP_API_KEY}}"
```

For OAuth2 client credentials use:

```yaml
    auth:
      type: oauth2
      oauth2:
        tokenURL: https://provider.example/oauth/token
        clientID: openndx
        clientSecret: "{{env:PROVIDER_CLIENT_SECRET}}"
        scopes: [read]
```

Leave out `auth` for a provider that needs none. `timeout` (default `10s`) and `tls` are also available; see [CONFIGURATION.md](CONFIGURATION.md#providers).

## 3. Map the arguments

`argMappings` tells the OE how the consumer's argument becomes the provider's. Say the consumer asks:

```graphql
query { personInfo(nic: "748213") { fullName } }
```

and the provider expects `person(nic: "...") { fullName }`. The mapping is:

```yaml
argMappings:
  - schemaId: drp-person-v1
    targetArgName: nic
    sourceArgPath: personInfo-nic
    targetArgPath: person
```

`sourceArgPath` is the consumer-side argument: the consumer field path from the root, then the argument name, joined with dashes (`personInfo-nic`). `targetArgPath` is the provider field the argument is attached to, a leading part of the provider field path: `person` for `person.fullName`. The argument goes on that field only. Add one mapping per provider argument.

The OE sends the value as a GraphQL variable, `person(nic: $nic)` with `{"variables": {"nic": "748213"}}`, declared with the consumer argument's type (`String!` here). If the provider's argument has a different type, set `targetArgType` on the mapping (see [CONFIGURATION.md](CONFIGURATION.md#argmappings)). The provider validates the variable's type strictly: a `String!` variable is rejected for an `ID!` argument, though an inlined string was accepted. The DRP and RGD mocks declare `nic: ID!`, so their mappings set:

```yaml
    targetArgType: "ID!"
```

## 4. Declare the fields in the GraphQL schema

Each leaf field the consumer can query carries `@sourceInfo`:

```graphql
type PersonInfo {
  fullName: String @sourceInfo(schemaId: "drp-person-v1", providerField: "person.fullName")
}
```

- `schemaId` must match the provider's `id` in `config.yaml`.
- `providerField` is the path in the provider's schema and must match the `fieldName` registered in step 1.

The checked-in sample is [schema.graphql](schema.graphql). To change the schema the OE serves, edit the file named by `schemaPath` in `config.yaml` (default the `schema.graphql` next to it) and restart the OE (see [CONFIGURATION.md](CONFIGURATION.md#schema)).

## 5. Restart and check

Providers are read once at startup, so restart the OE after editing `config.yaml`. Then run a query as in [dev/README.md](../../dev/README.md). If it fails:

| Error | Likely cause |
|---|---|
| `PDP_ERROR` | A queried field has no policy metadata for that `schemaId` (step 1). |
| `PDP_NOT_ALLOWED` | The consumer application has no grant for the field. |
| `CE_NOT_APPROVED` | The owner has not approved consent yet. |
| Field is `null` with a `PROVIDER_ERROR` at its `path` | The provider call failed: the provider was unreachable or not configured, answered with a non-2xx status, or sent a body that isn't a GraphQL response. Check the OE log and the provider's URL and credentials. |
