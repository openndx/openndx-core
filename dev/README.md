# Local dev sandbox: mock data providers

`oe` (the Orchestration Engine) federates queries out to real data providers.
This folder brings up mock member services locally so you can exercise the
full exchange flow.

For the complete flow, follow the **[local end-to-end walkthrough with `ondx`](WALKTHROUGH.md)**.
It covers starting the stack, operator login, registering members and both mock
schemas, granting application policy, citizen consent approval, and checking the
returned DRP/RGD data.

The services are the DRP and RGD mocks from the
[openndx-farajaland](https://github.com/openndx/openndx-farajaland) reference
country implementation. We depend only on their **published Docker images**,
not on that repo being checked out next to this one, so this works for anyone
who clones just `openndx` — no sibling checkout, no path coupling.

## What's included

| Service           | Port | Protocol         | Role                                                                           |
|-------------------|------|------------------|--------------------------------------------------------------------------------|
| `drp-api`         | 9090 | REST (internal)  | Raw mock Digital Registration Provider API                                     |
| `drp-api-adapter` | 9091 | GraphQL          | Subgraph in front of `drp-api` — this is what `oe` calls as the `drp` provider |
| `rgd-api`         | 8080 | GraphQL + OAuth2 | Mock Registrar General's Department API — the `rgd` provider                   |

`cmd/oe/config.docker.yaml` (used by the root `compose.yml` stack) and
`cmd/oe/config.example.yaml` (used when running `oe` outside Docker) are
already wired to these ports.

## Usage

```bash
# From the repo root
docker compose -f dev/compose.yml up -d
```

Then bring up the rest of the exchange stack (Postgres, ThunderID, PDP, CE,
and `oe` itself), which already knows how to reach the containers above via
`host.docker.internal`:

```bash
docker compose up -d --build postgres thunderid policy-decision-point \
  consent-engine consent-portal orchestration-engine portal-backend
```

Or, if you're iterating on `oe` itself and don't want to rebuild its image on
every change, run it natively against the same providers instead of the
containerized `orchestration-engine` service:

```bash
go run ./cmd/oe -config cmd/oe/config.example.yaml
```

The example config's `idp` section points at the local ThunderID, which OE uses
to verify consumer tokens; `idp.issuer` must match the `iss` ThunderID stamps,
its public URL.

## Trying it

```bash
curl http://localhost:4000/health
```

OE's GraphQL endpoint requires a consumer access token issued by ThunderID for OE's
audience. OE verifies its signature, issuer and audience with `internal/authn`
(`IDP_*` in `compose.yml`; see `cmd/oe/AUTHENTICATION.md`). Get one for the seeded M2M
data-consumer client (`thunderid/bootstrap/application.yaml`, `DATA_CONSUMER`; secret is
`DATA_CONSUMER_CLIENT_SECRET`, default `1234`):

```bash
TOKEN=$(curl -s -u "DATA_CONSUMER:1234" \
  -X POST http://localhost:8090/oauth2/token \
  -d grant_type=client_credentials \
  -d resource=http://api.openndx.local \
  | jq -r .access_token)
```

`resource=http://api.openndx.local` is the OE resource server declared in `thunderid/bootstrap/resource.yaml`; the
`getdata` scope comes from the `DataConsumerGetData` role, not a requested scope
param.

Then call OE with it:

```bash
curl -X POST http://localhost:4000/public/graphql \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query":"query { personInfo(nic: \"748213\") { fullName profession birthInfo { birthPlace district } } }"}'
```

`748213` (and a few other sample NICs — see
`dev/providers/drp-api/mock_data.json` and `dev/providers/rgd-api/mock_data.json`)
exists in both the DRP and RGD mock datasets, so a `personInfo` query can pull
fields from both providers in one request. The mock data uses the fictional
names (Suresh, Ramesh, Naresh, Gomesh) used for test accounts across our other
projects, not real people.

This request needs field policies, application grants, and citizen consent.
A freshly started stack has none of those. Follow the
[walkthrough](WALKTHROUGH.md#4-register-members-and-both-provider-schemas) to
register them with `ondx`, then use the client credentials printed for your
registered application. The seeded `DATA_CONSUMER` client above has no automatic
policy grants.

To approve the consent yourself, log in to the consent portal (see
[Consent portal](#consent-portal) below) as the matching seeded `Citizen`
account (`thunderid/bootstrap/users.yaml`), password `1234` for all of them:

| Username | `nic`    |
| -------- | -------- |
| `suresh` | `748213` |
| `ramesh` | `512346` |
| `naresh` | `398217` |
| `gomesh` | `674529` |

Each account's `nic` attribute is identical to its mock DRP/RGD record, and
`IDP_SUBJECT_CLAIM=nic` (the `compose.yml` default) tells consent-engine to match that
claim against the consent record's owner id, so logging in as e.g. `suresh`
is recognized as the owner of NIC `748213` and can approve a request for it.

### Consent portal

`compose.yml` now builds and runs the consent portal too (`consent-portal`
service, `portals/apps/consent/Dockerfile`) — no separate `pnpm` step needed.
It's an nginx-served static build; `config.js` is generated at container
startup from env vars (`entrypoint.sh`), already wired to this stack's
ThunderID/consent-engine.

Open http://localhost:5173 (or `${CONSENT_PORTAL_PORT}` if you overrode it —
see `.env.example`).

Don't use `portals/setup-portals.sh` for local testing here — it's a separate
helper for running all three portals via `pnpm dev` against a generic
(Asgardeo-shaped) IdP, and its default consent-portal port (`5175`) doesn't
match the `5173` baked into `CONSENT_PORTAL_URL`, ThunderID's registered
redirect URIs, and `thunderid/bootstrap/cors.yaml`'s allowed origins — it
isn't wired for this sandbox.

Use the exact consent URL returned by OE, including its `consentId` parameter,
and use a separate browser session from the management operator. The
[walkthrough](WALKTHROUGH.md#8-request-and-approve-citizen-consent) describes
approval and the required data checks after retrying the query.

## Stopping

```bash
docker compose down
docker compose -f dev/compose.yml down
```

## Pinning versions

Image tags default to `v0.1.0` (matches `openndx-farajaland/members/run-member-services.sh`).
Override with env vars if needed:

```bash
IMAGE_TAG=v0.2.0 docker compose -f dev/compose.yml up -d
```
