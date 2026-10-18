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
  --schema-id drp-schema-v1 --name "DRP Person Registry" \
  --endpoint http://localhost:9091 --member-id "$MEMBER_ID" \
  --field person.fullName:public:primary \
  --field person.profession:restricted:primary
```

The `--schema-id` becomes the `schemaId`, and each `--field` name becomes the PDP `fieldName`. See [the walkthrough](../../dev/WALKTHROUGH.md#4-register-members-and-both-provider-schemas) for the full sequence.

## 2. Add the provider to `config.json`

Add an entry to `providers`. `providerKey` and `schemaId` together identify the provider.

```json
{
  "providerKey": "drp",
  "providerUrl": "http://localhost:9091",
  "schemaId": "drp-schema-v1",
  "auth": { "type": "apiKey", "apiKeyName": "Test-Key", "apiKeyValue": "<key>" }
}
```

For OAuth2 use `"auth": { "type": "oauth2", "tokenUrl": "...", "clientId": "...", "clientSecret": "...", "scopes": ["..."] }`. Leave out `auth` for a provider that needs none.

## 3. Map the arguments

`argMappings` tells the OE how the consumer's argument becomes the provider's. Say the consumer asks:

```graphql
query { personInfo(nic: "748213") { fullName } }
```

and the provider expects `person(nic: "...") { fullName }`. The mapping is:

```json
{
  "providerKey": "drp",
  "schemaId": "drp-schema-v1",
  "targetArgName": "nic",
  "sourceArgPath": "personInfo-nic",
  "targetArgPath": "person"
}
```

`sourceArgPath` is the consumer-side argument (dash-separated path ending in the argument name). `targetArgPath` is the leading part of the provider field path the argument belongs to: `person` for `person.fullName`. Add one mapping per provider argument.

## 4. Declare the fields in the GraphQL schema

Each leaf field the consumer can query carries `@sourceInfo`:

```graphql
type PersonInfo {
  fullName: String @sourceInfo(providerKey: "drp", schemaId: "drp-schema-v1", providerField: "person.fullName")
}
```

- `providerKey` and `schemaId` must match the provider entry in `config.json`.
- `providerField` is the path in the provider's schema and must match the `fieldName` registered in step 1.

The checked-in sample is [schema.graphql](schema.graphql). To change the schema the OE serves, create a new version and activate it with the `/sdl` routes (see [HOW_TO_TEST.md](HOW_TO_TEST.md#manage-schema-versions)), or edit `schema.graphql` when the database holds no active schema.

## 5. Restart and check

Providers are read once at startup, so restart the OE after editing `config.json`. Then run a query as in [dev/README.md](../../dev/README.md). If it fails:

| Error | Likely cause |
|---|---|
| `PDP_ERROR` | A queried field has no policy metadata for that `schemaId` (step 1). |
| `PDP_NOT_ALLOWED` | The consumer application has no grant for the field. |
| `CE_NOT_APPROVED` | The owner has not approved consent yet. |
| Field missing from the result, no error | The provider call failed, so its fields are omitted. Check the OE log and the provider's URL and credentials. |
