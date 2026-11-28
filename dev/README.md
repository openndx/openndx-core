# Local dev sandbox: sample data providers

The Orchestration Engine (OE) federates each query to data providers. This folder runs sample providers locally, so the whole exchange can be tried on one machine.

For the complete flow, follow the **[local end-to-end walkthrough with `ondx`](WALKTHROUGH.md)**. It covers starting the stack, operator login, registering members and both provider schemas, granting an application access, approving the citizen's consent, and checking the data each provider returns.

The providers are the mock member services of [openndx-farajaland](https://github.com/openndx/openndx-farajaland), a sample country implementation, run from its published images. Nothing else from that repository is needed.

## What's included

| Service                     | Port | Protocol        | Role                                                                                                   |
| --------------------------- | ---- | --------------- | ------------------------------------------------------------------------------------------------------ |
| `identity-registry-api`     | 9090 | REST            | The Identity Registry's person records, served only to requests with its API key                       |
| `identity-registry-adapter` | 9091 | GraphQL         | GraphQL service in front of `identity-registry-api`: the OE's `identity-registry-v1` provider          |
| `civil-registry-api`        | 8080 | GraphQL, OAuth2 | The Civil Registry's birth records: the OE's `civil-registry-v1` provider, which issues its own tokens |

The images are published as `ghcr.io/openndx/<service>` for linux/amd64 and linux/arm64, with the sample data built in; [compose.yml](compose.yml) pins their tags. The two Identity Registry services read their settings from [providers/](providers). [cmd/oe/config.docker.yaml](../cmd/oe/config.docker.yaml) (the OE in Compose) and [cmd/oe/config.example.yaml](../cmd/oe/config.example.yaml) (the OE run natively) point at these ports, with the providers' credentials.

## Sample data

Both providers hold the same six fictional people, each identified by a NIC, `FL-` and 10 digits: Suresh Fernando `FL-4829105736`, Ramesh Fernando `FL-7316048259`, Naresh Fernando `FL-2954817360`, Gomesh Fernando `FL-6081734925`, Dinesh Fernando `FL-3647209158` and Mahesh Fernando `FL-9172563804`. A query passes the NIC in the consumer schema's `nic` argument, so one `personInfo` query can read fields from both providers.

## Usage

`make up` and `make deps` start these with the rest of the stack. To start them on their own, from the repository root:

```bash
docker compose -f dev/compose.yml up -d
```

Then bring up the rest of the exchange stack (PostgreSQL, ThunderID, the PDP, the CE, the OE, Portal Backend and the Consent Portal). The OE in Compose reaches the providers through `host.docker.internal`:

```bash
docker compose up -d --build postgres thunderid policy-decision-point \
  consent-engine consent-portal orchestration-engine portal-backend
```

Or, if you're iterating on the OE itself and don't want to rebuild its image on every change, run it natively against the same providers instead of the containerized `orchestration-engine` service:

```bash
go run ./cmd/oe -config cmd/oe/config.example.yaml
```

The example config's `idp` section points at the local ThunderID, which the OE uses to verify consumer tokens; `idp.issuer` must match the `iss` ThunderID stamps, its public URL.

## Trying it

```bash
curl http://localhost:4000/health
```

The OE's GraphQL endpoint requires a consumer access token issued by ThunderID for the OE's audience. The OE verifies its signature, issuer and audience with `internal/authn` (`IDP_*` in `compose.yml`; see the `idp` section of [cmd/oe/CONFIGURATION.md](../cmd/oe/CONFIGURATION.md)). Get one for the sample `DATA_CONSUMER` client ([thunderid/README.md](../thunderid/README.md)); its secret is `DATA_CONSUMER_CLIENT_SECRET`, default `1234`:

```bash
TOKEN=$(curl -s -u "DATA_CONSUMER:1234" \
  -X POST http://localhost:8090/oauth2/token \
  -d grant_type=client_credentials \
  -d resource=http://api.openndx.local \
  | jq -r .access_token)
```

`resource=http://api.openndx.local` is the OE resource server declared in `thunderid/bootstrap/resource.yaml`; the `getdata` scope comes from the `DataConsumerGetData` role, not a requested scope parameter.

Then call the OE with it:

```bash
curl -X POST http://localhost:4000/public/graphql \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query":"query { personInfo(nic: \"FL-4829105736\") { fullName profession birthInfo { birthPlace district } } }"}'
```

This request needs field policies, application grants, and citizen consent. A freshly started stack has none of those. Follow the [walkthrough](WALKTHROUGH.md#4-register-members-and-both-provider-schemas) to register them with `ondx`, then use the client credentials printed for your registered application. The seeded `DATA_CONSUMER` client above has no automatic policy grants.

To approve the consent yourself, log in to the consent portal (see [Consent portal](#consent-portal) below) as the matching seeded `Citizen` account (`thunderid/bootstrap/users.yaml`), password `1234` for all of them:

| Username | NIC             |
| -------- | --------------- |
| `suresh` | `FL-4829105736` |
| `ramesh` | `FL-7316048259` |
| `naresh` | `FL-2954817360` |
| `gomesh` | `FL-6081734925` |

The OE takes a consent's owner ID from the query's `nic` argument, and the consent-engine matches it against the token claim named by `IDP_SUBJECT_CLAIM`. Compose sets that claim to `nic`, so logging in as `suresh` is recognized as the owner of `FL-4829105736` and can approve a request for it.

### Consent portal

`compose.yml` builds and runs the consent portal (`consent-portal` service, `portals/apps/consent/Dockerfile`), so no separate `pnpm` step is needed. It's an nginx-served static build; `config.js` is generated at container startup from environment variables (`entrypoint.sh`) and points at this stack's ThunderID and consent-engine.

Open <http://localhost:5173> (or `${CONSENT_PORTAL_PORT}` if you overrode it; see `.env.example`).

Don't use `portals/setup-portals.sh` for local testing here. It's a separate helper for running all three portals with `pnpm dev` against a generic identity provider, and its consent-portal port (`5175`) doesn't match the `5173` in `CONSENT_PORTAL_URL`, ThunderID's registered redirect URIs, and `thunderid/bootstrap/cors.yaml`'s allowed origins.

Use the exact consent URL returned by the OE, including its `consentId` parameter, and use a separate browser session from the management operator. The [walkthrough](WALKTHROUGH.md#8-request-and-approve-citizen-consent) describes approval and the required data checks after retrying the query.

## Stopping

```bash
docker compose down
docker compose -f dev/compose.yml down
```
