# Testing the Orchestration Engine

Automated tests, and manual checks against a running OE. The service overview is in the [README](README.md).

## Automated tests

```bash
make validate-test orchestration-engine         # with -race and coverage
go test ./cmd/oe/... ./internal/oe/... -count=1 # plain run
```

All tests live under `internal/oe` and need no external database or service (they use local test servers). The federation tests in `internal/oe/federator` compare each result with a golden file in `testdata/golden/`. After an intended behavior change, regenerate them and review the diff:

```bash
go test ./internal/oe/federator -run TestFederateQueryGolden -update
```

The cross-service suite, which runs the OE against the PDP and Consent Engine, is a separate Go module in [tests/integration](../../tests/integration/README.md).

## Manual checks

Start the OE (see the [README](README.md#quick-start)) and, for the queries, the rest of the stack.

```bash
curl http://localhost:4000/health
# {"service":"orchestration-engine","status":"healthy"}
```

### Change the schema

The OE reads the consumer schema from `schemaPath` in `config.yaml` (default the `schema.graphql` next to it) at startup ([CONFIGURATION.md](CONFIGURATION.md#schema)). To try a different schema, edit the file, or point `schemaPath` at another one, and restart the OE. A file that is missing or isn't a valid schema stops it at startup with an error naming the path.

### Send a federated query

`POST /public/graphql` needs a consumer token, policy metadata, an application grant and, for restricted fields, the citizen's consent. The token and curl example are in [dev/README.md](../../dev/README.md), and the full setup is in the [walkthrough](../../dev/WALKTHROUGH.md). The expected results:

| Situation | Response |
|---|---|
| No or bad token | HTTP `401`, `{"error":"unauthorized",...}` |
| Application has no grant for a field | `errors[0].extensions.code` is `PDP_NOT_ALLOWED`, with `unauthorizedFields` |
| Field has no policy metadata | `PDP_ERROR` |
| No `nic` argument in the query | `BAD_REQUEST` (argument `nic` is required), or `MISSING_IDENTIFIER` if the schema declares `nic` as optional |
| Unknown field, or wrong argument type | `BAD_REQUEST`, with `locations` |
| Two aliases with different `nic` values | `AMBIGUOUS_IDENTIFIER` |
| A provider is down or returns a non-2xx status | `data` has that provider's fields as `null`, with a `PROVIDER_ERROR` at each field's `path` |
| Consent still pending | `CE_NOT_APPROVED` with `consentPortalUrl` (`<portal>?consentId=<uuid>`) and `consentStatus: pending` |
| Everything granted and approved | `data` holds the merged fields from all providers |

Open `consentPortalUrl` as the citizen to approve, then repeat the query.
