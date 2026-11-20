# Deploy OpenNDX

This is how an operator runs OpenNDX from a checkout. It is the same Compose stack as local development, pointed at the operator's URLs, secrets, and data providers. `compose.yml` builds the maintained services from the source tree. For Kubernetes, use the published images and the [Helm chart](../deploy/helm/openndx/README.md).

For a throwaway local run with the default passwords, use the [README quick start](../README.md#quick-start) and then the [walkthrough](../dev/WALKTHROUGH.md). This page is the same mechanism with the choices a real deployment has to make.

## What you need

- Docker with Compose v2
- A checkout of the version you intend to run (a release tag, `main`, or your own fork)
- Hostnames and ports for the identity provider, the Consent Portal, and the APIs your users will open
- The GraphQL services that hold the data (providers). The mocks under [dev/](../dev/README.md) are for local trials

Go and Node are not required to deploy. They are required to work on the source; see [CONTRIBUTING.md](../CONTRIBUTING.md).

## 1. Choose the version

```bash
git clone https://github.com/openndx/openndx-core.git
cd openndx-core
git checkout v0.7.0   # a published release tag, or stay on main
```

A downstream fork uses the fork's URL instead, and the same commands after that. Patches committed on that fork are included because Compose builds from the working tree. See [Upstream and downstream](../CONTRIBUTING.md#upstream-and-downstream).

Published container images and the `ondx` CLI for an upstream release are listed in [RELEASE.md](RELEASE.md). Those images match upstream tags. A patch that exists only in your fork is in the running system when you build from that fork.

## 2. Set secrets and public URLs

```bash
cp .env.example .env
```

`.env` is gitignored. Change every secret (they default to `1234`) before anyone but you can reach the host. Then set the URLs clients actually use:

| Variable | Why it has to match the outside world |
|---|---|
| `IDP_PUBLIC_URL` | Tokens carry this as `iss`. It must be the identity provider URL browsers and applications call, and the same value as `server.public_url` in [thunderid/deployment.yaml](../thunderid/deployment.yaml). The services fetch its keys from inside Compose at `http://thunderid:8090/oauth2/jwks`. |
| `CONSENT_PORTAL_URL` | Link the Consent Engine puts in a consent request, and the portal's sign-in redirect. |

Host ports (`OE_PORT`, `CE_PORT`, `PDP_PORT`, `PB_PORT`, `CONSENT_PORTAL_PORT`, `DB_PORT`) can change. The Consent Portal reads `CE_PORT` into a URL the browser loads, so the published Consent Engine port has to be reachable from the user's machine.

The Compose file talks HTTP and binds the services on the host. Put TLS in front of anything exposed beyond a local machine, and use those HTTPS URLs in the variables above.

## 3. Point the gateway at real providers

The Orchestration Engine reads [cmd/oe/config.docker.yaml](../cmd/oe/config.docker.yaml) and the consumer schema [cmd/oe/schema.graphql](../cmd/oe/schema.graphql), both mounted into the container. Replace the sample provider list and schema with the systems this deployment federates. Field names in the schema are the fields the Policy Decision Point must have a field policy for. Provider settings are described in [cmd/oe/PROVIDER_CONFIGURATION.md](../cmd/oe/PROVIDER_CONFIGURATION.md).

Leave `pdp.url` set. An empty policy URL makes the gateway return provider data with no access check.

## 4. Start

The defaults need host ports 5432, 8090, 8082, 8081, 5173, 4000 and 8083 free. ThunderID stays on 8090. If another program is using one of these, Compose stops with `port is already allocated` and leaves the stack only partly started. Free the port, then run the command again.

```bash
docker compose up --build -d
```

On a fresh database volume, Postgres runs [init-db.sql](../init-db.sql) once: it creates the Policy Decision Point, Consent Engine and Portal Backend databases and loads each service schema. Services do not create their own tables. A later schema change is applied by an operator to the existing database; the init script does not run again on a volume that already exists.

Audit records are dropped until an OpenTelemetry endpoint is set. [Viewing audit records](../CONTRIBUTING.md#viewing-audit-records) shows the local OpenObserve option.

The process is up when these return HTTP 200. They do not check the database. ThunderID's own check is `http://localhost:8090/health/readiness`.

| Service | Check |
|---|---|
| Orchestration Engine | `curl -f http://localhost:4000/health` |
| Consent Engine | `curl -f http://localhost:8081/health` |
| Policy Decision Point | `curl -f http://localhost:8082/health` |
| Portal Backend | `curl -f http://localhost:8083/health` |
| Consent Portal | `curl -f http://localhost:5173/health` |

## 5. Use the deployment

Install `ondx` from the [CLI README](../cmd/ondx/README.md#install), sign in, and register members, schemas, and applications. The [walkthrough](../dev/WALKTHROUGH.md) is the worked example against the mock providers; the same commands apply to a deployment whose providers and accounts you have configured. Citizen and consumer steps are in the Consent Engine and Orchestration Engine READMEs linked from there.

## Update

```bash
git fetch origin
git checkout v0.7.0          # or merge a newer upstream tag into your fork
docker compose up --build -d
```

Apply any new statements in `cmd/<svc>/schema.sql` to the existing databases yourself. Those files are written to be safe to run again; the Postgres entrypoint will not re-run them.

## Stop

```bash
docker compose down          # keeps the database volume
docker compose down -v       # also deletes the database and identity-provider volumes
```
