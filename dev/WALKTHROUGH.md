# Local end-to-end walkthrough with `ondx`

Use the checked-in DRP/RGD mocks to register members and schemas, grant an
application access through `ondx`, approve citizen consent, and retrieve data
through OE. Management calls go through Portal Backend; no SQL edits or direct
PDP writes are needed.

This is an interactive walkthrough: operator login and citizen approval use a
browser. The final checks require actual provider data and reject GraphQL errors.
The separate backend integration suite is described in
[tests/integration](../tests/integration/README.md).

## 1. Prepare a local checkout

Use Bash, Docker with Compose v2, Go 1.26+, `curl`, `jq`, and a browser. Run every
terminal command from the repository root in the same Bash session. Build the CLI
from this checkout so its commands match the running services:

```bash
go build -o ondx ./cmd/ondx
./ondx version
```

The commands below use the default local ports and a fresh database. No `.env`
file is required. If you already have one, check its overrides against
[.env.example](../.env.example). Ports 4000, 5173, 5432, 8080–8083, 8090, 8765,
9090, and 9091 must be available. Do not run the integration-test stack at the
same time; several ports overlap.

The seeded accounts below all use password `1234` in this local sandbox:

| Account     | Purpose                                                            |
|-------------|--------------------------------------------------------------------|
| `ndx-admin` | Operator who registers members, schemas, and application policies  |
| `suresh`    | Citizen whose NIC is `748213`; approves sharing their mock records |

Use separate browser profiles or a private window for the citizen. An existing
operator login at ThunderID can otherwise be reused when the portal needs a
citizen login.

## 2. Start the providers and platform

```bash
docker compose -f dev/compose.yml up -d
docker compose up -d --build postgres thunderid policy-decision-point \
  consent-engine consent-portal orchestration-engine portal-backend

docker compose -f dev/compose.yml ps
docker compose ps -a
```

The provider images are published separately; their configuration and mock data
are mounted from `dev/providers/`. The root stack builds OpenNDX from this
checkout and initializes PostgreSQL and ThunderID. Audit is optional and is
omitted from these commands.

Wait for startup, especially the one-time `thunderid-setup` service. It must exit
successfully. Check the endpoints before continuing:

```bash
curl --fail --silent --show-error http://localhost:8090/health/readiness
curl --fail --silent --show-error http://localhost:8083/health
curl --fail --silent --show-error http://localhost:8082/health
curl --fail --silent --show-error http://localhost:8081/health
curl --fail --silent --show-error http://localhost:4000/health
curl --fail --silent --show-error http://localhost:5173/health
```

These are readiness checks; they do not prove that a provider query works. The
checks in steps 6–9 do that.

## 3. Log in as the operator

```bash
./ondx login --profile local --extra resource=http://pb.openndx.local
```

Sign in as **`ndx-admin` / `1234`** in the browser. The built-in local profile
uses ThunderID at `http://localhost:8090`, client `NDX_CLI`, callback
`http://127.0.0.1:8765/callback`, resource `http://pb.openndx.local`, and Portal
Backend at `http://localhost:8083`. The explicit `--extra resource=...` also
handles a saved local profile created before it included the PB resource.
Each management command below explicitly selects that profile. If you edited
the local profile previously, compare `./ondx profile list` with these values.

## 4. Register members and both provider schemas

The bootstrap already creates the DRP and DIE organization units in ThunderID.
Register the corresponding members in Portal Backend:

```bash
./ondx members create --profile local \
  --name "DRP Demo Provider" --email drp@openndx.local \
  --ou-id 01900000-0000-7000-8000-0000000000c0

./ondx members create --profile local \
  --name "DIE Demo Consumer" --email die@openndx.local \
  --ou-id 01900000-0000-7000-8000-0000000000c1
```

Each command prints `Created member <id> (...)`. Copy those two IDs into shell
variables, replacing the placeholder text:

```bash
DRP_MEMBER_ID='paste-the-DRP-member-id'
DIE_MEMBER_ID='paste-the-DIE-member-id'
```

For this sandbox, register both schemas under the DRP demo member. The seeded
organization units include DRP and DIE; a separate RGD member is not needed to
exercise the two providers. Schema ownership in Portal Backend and provider
routing in OE are separate: OE still calls DRP and RGD independently.

```bash
./ondx schemas create --profile local \
  --schema-id drp-person-v1 --name "DRP Person Registry" \
  --endpoint http://host.docker.internal:9091 \
  --member-id "$DRP_MEMBER_ID" \
  --field person.fullName:allowlist:primary \
  --field person.profession:allowlist_consent:primary

./ondx schemas create --profile local \
  --schema-id rgd-person-v1 --name "RGD Birth Registry" \
  --endpoint http://host.docker.internal:8080/graphql \
  --member-id "$DRP_MEMBER_ID" \
  --field getPersonInfo.birthPlace:allowlist_consent:primary \
  --field getPersonInfo.district:allowlist_consent:primary
```

Keep the exact schema IDs and field names: they match `@sourceInfo` in
[`cmd/oe/schema.graphql`](../cmd/oe/schema.graphql). `schemas create` registers
the policy metadata in PDP through Portal Backend. It does not change OE's
checked-in provider configuration. The endpoints above are reachable from OE's
Docker container; native OE uses the localhost endpoints in `config.example.yaml`.

The middle part of each `--field` is the field's PDP policy class. `allowlist`
fields require an application grant. `allowlist_consent` fields also require
the citizen's consent. Here `person.fullName` needs only the grant; the
profession and birth fields exercise consent.

## 5. Create the consumer application

Start with permission for only the full name:

```bash
./ondx applications create --profile local \
  --name "Local Passport Demo" --member-id "$DIE_MEMBER_ID" \
  --field drp-person-v1:person.fullName
```

This provisions an M2M client in ThunderID and grants it the data-consumer role.
Copy the three printed values into these variables. The secret is shown once:

```bash
APP_ID='paste-Created-application-id'
CLIENT_ID='paste-IdP-Client'
CLIENT_SECRET='paste-IdP-Secret'
```

`APP_ID` identifies the Portal Backend application and is used by CLI management
commands. `CLIENT_ID` is the OAuth client identity in the consumer token; Portal
Backend uses it when writing PDP grants, and OE uses it for policy and consent
checks. Keep these IDs distinct.

Request a consumer access token:

```bash
TOKEN_RESPONSE=$(curl --fail --silent --show-error \
  --user "$CLIENT_ID:$CLIENT_SECRET" \
  --data-urlencode grant_type=client_credentials \
  --data-urlencode resource=http://api.openndx.local \
  http://localhost:8090/oauth2/token)
TOKEN=$(printf '%s' "$TOKEN_RESPONSE" | jq -er '.access_token | strings | select(length > 0)')
```

Stop if token retrieval fails. The `resource` selects OE's resource server;
the data-consumer role supplies its `getdata` permission. This token is for
consumer requests. Citizen approval uses the portal's own browser login.

## 6. Verify a granted field that needs no consent

Define a helper for submitting GraphQL queries. It fails on HTTP errors; each
step also checks GraphQL errors because those can arrive with HTTP 200:

```bash
oe_query() {
  curl --fail --silent --show-error http://localhost:4000/public/graphql \
    -H "Authorization: Bearer $TOKEN" \
    -H 'Content-Type: application/json' \
    --data-binary "$1"
}

PUBLIC_REQUEST='{"query":"query { personInfo(nic: \"748213\") { fullName } }"}'
PUBLIC_RESPONSE=$(oe_query "$PUBLIC_REQUEST")
printf '%s\n' "$PUBLIC_RESPONSE" | jq .
printf '%s' "$PUBLIC_RESPONSE" | jq -e '
  ((.errors // []) | length == 0) and
  .data.personInfo.fullName == "Suresh"
'
```

The assertion must print `true` and exit successfully. This proves the token,
PDP grant, OE mapping, and DRP provider are working before adding consent.

## 7. Grant the consent fields with `ondx policy update`

Prepare a query spanning DRP and RGD:

```bash
FULL_REQUEST='{"query":"query { personInfo(nic: \"748213\") { fullName profession birthInfo { birthPlace district } } }"}'
DENIED_RESPONSE=$(oe_query "$FULL_REQUEST")
printf '%s\n' "$DENIED_RESPONSE" | jq .
printf '%s' "$DENIED_RESPONSE" | jq -e '
  any(.errors[]?; .extensions.code == "PDP_NOT_ALLOWED")
'
```

PDP should deny this request because the application has not been granted the
consent fields yet. Add them through the CLI:

```bash
./ondx policy update --profile local --app-id "$APP_ID" \
  --grant-duration 30d \
  --field drp-person-v1:person.fullName \
  --field drp-person-v1:person.profession \
  --field rgd-person-v1:getPersonInfo.birthPlace \
  --field rgd-person-v1:getPersonInfo.district

./ondx applications get --profile local --app-id "$APP_ID"
```

Keep `person.fullName` in the command, along with every other field the
application should retain. The CLI output should now show all four fields.
Portal Backend saves the supplied field list and adds or refreshes the matching
PDP grants. Currently, omitting a previously granted field does **not** revoke
its existing PDP grant; do not use omission as an access-revocation mechanism.

## 8. Request and approve citizen consent

Submit the same full query after granting the policy:

```bash
PENDING_RESPONSE=$(oe_query "$FULL_REQUEST")
printf '%s\n' "$PENDING_RESPONSE" | jq .
printf '%s' "$PENDING_RESPONSE" | jq -e '
  any(.errors[]?; .extensions.code == "CE_NOT_APPROVED" and
                  .extensions.consentStatus == "pending")
'
CONSENT_URL=$(printf '%s' "$PENDING_RESPONSE" | jq -er '
  first(.errors[]? | select(.extensions.code == "CE_NOT_APPROVED") |
        .extensions.consentPortalUrl | strings | select(length > 0))
')
printf 'Open in the citizen browser: %s\n' "$CONSENT_URL"
```

Expected: `CE_NOT_APPROVED`, status `pending`, and a URL such as
`http://localhost:5173?consentId=<uuid>`. This means application authorization
succeeded, and CE created the citizen approval request. `CE_ERROR` is a failure;
do not treat it as a pending consent.

1. Open the exact printed URL in the separate citizen browser/private window.
2. Click **Sign In**, then log in as **`suresh` / `1234`**.
3. Review the requested profession, birth place, and district fields.
4. Click **Approve** and wait for the success page.

Use the returned URL, including `consentId`; opening the bare portal does not
select a request. The citizen's token contains `nic: "748213"`, which CE matches
to the request's owner ID using the stack's `IDP_SUBJECT_CLAIM=nic` setting.
`ndx-admin` is the management operator and cannot approve Suresh's consent.

## 9. Retry and verify the complete response

```bash
APPROVED_RESPONSE=$(oe_query "$FULL_REQUEST")
printf '%s\n' "$APPROVED_RESPONSE" | jq .
printf '%s' "$APPROVED_RESPONSE" | jq -e '
  ((.errors // []) | length == 0) and
  .data.personInfo.fullName == "Suresh" and
  .data.personInfo.profession == "Software Engineer" and
  .data.personInfo.birthInfo.birthPlace == "Rivertown General Hospital" and
  .data.personInfo.birthInfo.district == "Rivertown"
'
```

Expected data:

```json
{
  "data": {
    "personInfo": {
      "fullName": "Suresh",
      "profession": "Software Engineer",
      "birthInfo": {
        "birthPlace": "Rivertown General Hospital",
        "district": "Rivertown"
      }
    }
  }
}
```

Only count the walkthrough as successful when the assertion exits successfully.
It checks DRP data, RGD data, and absence of GraphQL errors. HTTP 200 alone is
insufficient. If the consumer token expires, repeat the token request in step 5.

## Troubleshooting

| Symptom                                             | Check                                                                                                                                                                                                                                                                                                                                                                                    |
|-----------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Port already allocated                              | Another stack is using a required port. Stop only the conflicting services you own, or configure a consistent set of alternate URLs/ports. ThunderID's published port is fixed in root Compose.                                                                                                                                                                                          |
| Login fails or redirects to the wrong account       | Use `--profile local`, callback port 8765, and the correct seeded account. Use a separate browser session for citizen approval.                                                                                                                                                                                                                                                          |
| Management request returns 401/403                  | Repeat step 3 as `ndx-admin`, requesting `resource=http://pb.openndx.local`. PB rejects old tokens audienced to `NDX_CLI`, and returns 403 when the token lacks the route's scope: log in again with the `local` profile so the PB scopes are requested, as a user whose role grants them (`OpenNDX_Admin` for member management). Check `docker compose logs portal-backend thunderid`. |
| Schema/member creation reports an existing ID or OU | The setup commands create records, not upserts. Reuse IDs saved from the previous run and skip creation. If necessary, inspect members/schemas through the management API or use a fresh disposable stack.                                                                                                                                                                               |
| `policy metadata not found` / `PDP_ERROR`           | Register both schemas first. Field names must match `person.profession` / `getPersonInfo.birthPlace`, not the unified GraphQL names.                                                                                                                                                                                                                                                     |
| Unexpected `PDP_NOT_ALLOWED`                        | Check all four grants with `applications get`. Use `APP_ID` for CLI updates and the token for `CLIENT_ID` from that same application.                                                                                                                                                                                                                                                    |
| `MISSING_IDENTIFIER` despite passing `nic`          | Check OE's `argMappings`: DRP's `targetArgPath` must be `person`, and RGD's must be `getPersonInfo`, without provider-name prefixes. Restart OE after changing its config; it loads config at startup.                                                                                                                                                                                   |
| `CE_ERROR`                                          | Check `docker compose logs orchestration-engine consent-engine`. OE's `consentEngine.url` must include `/internal/api/v1`; its client appends `/consents`. A fresh database must contain `cmd/ce/schema.sql`.                                                                                                                                                                                        |
| Consent portal returns 401                          | Check CE's issuer, JWKS URL, `CONSENT_ENGINE_AUDIENCE=CONSENT_PORTAL`, and that the token carries the `IDP_SUBJECT_CLAIM` claim (`nic`). The current portal does not request an OAuth `resource`, so ThunderID audiences its token to the client ID.                                                                                                                                                 |
| Consent portal returns 403                          | The logged-in citizen's NIC must match the requested NIC. Use `suresh` for `748213`, not the operator or another citizen.                                                                                                                                                                                                                                                                |
| Consent remains pending or expired                  | Open the newest returned consent URL and approve it. Pending realtime requests expire after one hour. Changing the requested restricted fields can replace an existing consent.                                                                                                                                                                                                          |
| Provider errors / missing data                      | Check `docker compose -f dev/compose.yml logs drp-api drp-api-adapter rgd-api`. OE uses `host.docker.internal:9091` and `:8080`, with a host-gateway entry on Linux. Verify both provider images are running.                                                                                                                                                                            |

For service logs and configuration details:

```bash
docker compose logs --tail=100 portal-backend policy-decision-point \
  consent-engine orchestration-engine thunderid
docker compose -f dev/compose.yml logs --tail=100
```

## Stop and repeat

```bash
docker compose down
docker compose -f dev/compose.yml down
unset TOKEN TOKEN_RESPONSE CLIENT_SECRET
```

Normal shutdown preserves the platform's named volumes, so members, schemas,
applications, and consents remain available. Keep the printed IDs and client
secret if you want to resume; do not rerun creation commands blindly.

For a completely fresh run, **only on a disposable local stack**, replace
`docker compose down` with `docker compose down --volumes`. This deletes the
stack's PostgreSQL and ThunderID data, including application clients and key
material. Start again at step 2 and run `ondx login` again; cached tokens from
the previous identity database are no longer valid.
