# Local dev sandbox: mock data providers

`oe` (the Orchestration Engine) federates queries out to real data providers.
This folder brings up mock member services locally so you can exercise the
full exchange flow.

The services are the DRP and RGD mocks from the
[opendif-farajaland](https://github.com/LSFLK/opendif-farajaland) reference
country implementation. We depend only on their **published Docker images**,
not on that repo being checked out next to this one, so this works for anyone
who clones just `openndx` — no sibling checkout, no path coupling.

## What's included

| Service           | Port | Protocol         | Role                                                                           |
|-------------------|------|------------------|--------------------------------------------------------------------------------|
| `drp-api`         | 9090 | REST (internal)  | Raw mock Digital Registration Provider API                                     |
| `drp-api-adapter` | 9091 | GraphQL          | Subgraph in front of `drp-api` — this is what `oe` calls as the `drp` provider |
| `rgd-api`         | 8080 | GraphQL + OAuth2 | Mock Registrar General's Department API — the `rgd` provider                   |

`cmd/oe/config.docker.json` (used by the root `compose.yml` stack) and
`cmd/oe/config.example.json` (used for `go run ./cmd/oe` outside Docker) are
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
cp .env.example .env   # if you haven't already
docker compose up --build
```

Or, if you're iterating on `oe` itself and don't want to rebuild its image on
every change, run it natively against the same providers instead of the
containerized `orchestration-engine` service:

```bash
CONFIG_PATH=cmd/oe/config.example.json go run ./cmd/oe
```

## Trying it

```bash
curl http://localhost:4000/health
```

OE's GraphQL endpoint requires a consumer access token — even with `trustUpstream: true`
(`cmd/oe/config.docker.json`), OE still requires *a* bearer token with a `client_id`
claim, it just skips verifying its signature itself (`internal/oe/auth/token.go`,
`GetConsumerJwtFromTokenWithValidator`). Get one for the seeded M2M data-consumer
client (`thunderid/bootstrap/application.yaml`, `DATA_CONSUMER`; secret is
`DATA_CONSUMER_CLIENT_SECRET` from `.env`, default `devsecret`):

```bash
TOKEN=$(curl -sk -u "DATA_CONSUMER:devsecret" \
  -X POST https://localhost:8090/oauth2/token \
  -d grant_type=client_credentials \
  -d resource=http://api.openndx.local \
  | jq -r .access_token)
```

`-k` skips TLS verification — ThunderID's local cert is self-signed (matches
`IDP_JWKS_INSECURE_SKIP_VERIFY=true` in `.env`). `resource=http://api.openndx.local`
is the OE resource server declared in `thunderid/bootstrap/resource.yaml`; the
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

Note this still goes through OE's normal authorization pipeline: PDP has to
allow the query and, for citizen-owned fields, CE has to have an approved
consent on file. A freshly started stack has no policies or consent records
yet, so expect `PDP_NOT_ALLOWED` / `CE_NOT_APPROVED` until those are set up.

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
`IDP_SUBJECT_CLAIM=nic` (`.env.example`) tells consent-engine to match that
claim against the consent record's owner id, so logging in as e.g. `suresh`
is recognized as the owner of NIC `748213` and can approve a request for it.

### Consent portal

`compose.yml` now builds and runs the consent portal too (`consent-portal`
service, `portals/apps/consent/Dockerfile`) — no separate `pnpm` step needed.
It's an nginx-served static build; `config.js` is generated at container
startup from env vars (`entrypoint.sh`), already wired to this stack's
ThunderID/consent-engine.

Open http://localhost:5173 (or `${PORT_CONSENT_PORTAL}` if you overrode it —
see `.env.example`). Since ThunderID's cert is self-signed, the browser will
otherwise reject the portal's requests to it — visit `https://localhost:8090`
directly first and accept the certificate warning, then reload the portal.

Don't use `portals/setup-portals.sh` for local testing here — it's a separate
helper for running all three portals via `pnpm dev` against a generic
(Asgardeo-shaped) IdP, and its default consent-portal port (`5175`) doesn't
match the `5173` baked into `CONSENT_PORTAL_URL`, ThunderID's registered
redirect URIs, and `thunderid/bootstrap/cors.yaml`'s allowed origins — it
isn't wired for this sandbox.

I haven't driven this login through an actual browser myself (no UI access
here), so treat it as the config that *should* work end-to-end, not a
verified one — if it doesn't, that's the first thing to check.

## Stopping

```bash
docker compose -f dev/compose.yml down
```

## Pinning versions

Image tags default to `v0.1.0` (matches `opendif-farajaland/members/run-member-services.sh`).
Override with env vars if needed:

```bash
IMAGE_TAG=v0.2.0 docker compose -f dev/compose.yml up -d
```
