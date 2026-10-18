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
# {"message":"OpenNDX Server is Healthy!"}
```

### Manage schema versions

The `/sdl` routes need the database. Each should respond as shown.

```bash
# create a version (not active yet)
curl -X POST http://localhost:4000/sdl -H "Content-Type: application/json" \
  -d '{"version":"1.1.0","sdl":"type Query { hello: String }","created_by":"me"}'

# list versions, then activate one
curl http://localhost:4000/sdl/versions
curl -X POST http://localhost:4000/sdl/versions/1.1.0/activate

# the active schema
curl http://localhost:4000/sdl

# check syntax, and compatibility with the active schema
curl -X POST http://localhost:4000/sdl/validate -H "Content-Type: application/json" \
  -d '{"sdl":"type Query { hello: String }"}'                     # {"valid":true}
curl -X POST http://localhost:4000/sdl/check-compatibility -H "Content-Type: application/json" \
  -d '{"sdl":"type Query { hello: String }"}'                     # {"compatible":..., "reason":...}
```

Activating a version takes effect on the next query. Remember to activate a schema that has `@sourceInfo` directives before querying.

### Send a federated query

`POST /public/graphql` needs a consumer token, policy metadata, an application grant and, for restricted fields, the citizen's consent. The token and curl example are in [dev/README.md](../../dev/README.md), and the full setup is in the [walkthrough](../../dev/WALKTHROUGH.md). The expected results:

| Situation | Response |
|---|---|
| No or bad token | HTTP `401`, `Unauthorized: invalid or expired token` |
| Application has no grant for a field | `errors[0].extensions.code` is `PDP_NOT_ALLOWED`, with `unauthorizedFields` |
| Field has no policy metadata | `PDP_ERROR` |
| No `nic` argument in the query | `MISSING_IDENTIFIER` |
| Consent still pending | `CE_NOT_APPROVED` with `consentPortalUrl` (`<portal>?consentId=<uuid>`) and `consentStatus: pending` |
| Everything granted and approved | `data` holds the merged fields from all providers |

Open `consentPortalUrl` as the citizen to approve, then repeat the query.

## Inspecting the schema table

The OE keeps schema versions in the `unified_schemas` table of its database:

```sql
SELECT id, version, is_active, created_at, created_by FROM unified_schemas ORDER BY created_at DESC;
SELECT * FROM unified_schemas WHERE is_active = true;
DELETE FROM unified_schemas WHERE created_by = 'me';   -- clean up test versions
```

The table has `id`, `version` (unique), `sdl`, `status`, `description`, `created_at`, `updated_at`, `created_by`, `checksum` and `is_active`.
