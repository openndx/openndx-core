# OpenNDX Helm Chart

A single Helm chart for deploying the maintained OpenNDX services on Kubernetes.

## Components

All components are configured from [`values.yaml`](./values.yaml):

| Component | Key | Default | Description |
|---|---|---|---|
| **Orchestration Engine** | `orchestrationEngine` | *Always deployed* | Core GraphQL federation engine (`POST /public/graphql`) |
| **Policy Decision Point** | `policyDecisionPoint.enabled` | `true` | Field-level Rego policy evaluation (`internal/pdp`) |
| **Consent Engine** | `consentEngine.enabled` | `true` | Data-owner consent lifecycle service (`internal/ce`) |
| **Consent Portal** | `consentPortal.enabled` | `true` | Citizen-facing React UI for approving/rejecting consents |
| **Portal Backend** | `portalBackend.enabled` | `true` | Management API for members, schemas, applications, and submissions (`internal/pb`) |

> **Embedded identity provider:** OpenNDX requires ThunderID as its embedded IdP. It signs the tokens every service validates, and Portal Backend provisions application clients through its management API, so a generic OIDC provider is not a substitute. Install it from ThunderID's published chart before this one (see [Embedded identity provider (ThunderID)](#embedded-identity-provider-thunderid)).
>
> **External infrastructure:** PostgreSQL and the optional Audit Service (Argus) are outside the scope of this chart and should be deployed via their own Helm charts or managed cloud services. Each service's `config` points at them.

### Service configuration

Each Go service's `config` block in `values.yaml` **is** its `config.yaml`, with the same schema as `cmd/<svc>/config.example.yaml` (keys in `cmd/<svc>/README.md`). The chart renders it as-is into a ConfigMap mounted at `/app/config/config.yaml`. The only exception is the deployment's public URLs: when left empty, the chart fills them in (see [Required values](#required-values)). Extra files go under `configFiles` and are mounted beside it (the Orchestration Engine's `schema.graphql`).

Keep `config` non-secret. A secret goes in as a `"{{env:NAME}}"` placeholder, which the service resolves at startup from its environment (see [Secrets](#secrets)).

The default in-cluster URLs (`http://openndx-pdp:8082`, `http://openndx-ce:8081/internal/api/v1`) assume the release is named `openndx`. With another release name, or with a service disabled, update the URLs that point at it.

## Prerequisites

- Kubernetes 1.26+
- Helm 3.12+
- PostgreSQL 15+ with each enabled service's schema applied (`cmd/pdp/schema.sql`, `cmd/ce/schema.sql`, `cmd/pb/schema.sql`)
- ThunderID, the embedded identity provider, installed from `oci://ghcr.io/thunder-id/helm-charts/thunderid` (see [below](#embedded-identity-provider-thunderid))
- The Secrets your services' `env` points at, created before install (see below)

### Embedded identity provider (ThunderID)

ThunderID publishes its chart at `oci://ghcr.io/thunder-id/helm-charts/thunderid`. Install it in the same namespace as a release named `thunderid`: the chart then names its Service `thunderid-service`, and the default `idp` URLs in [`values.yaml`](./values.yaml) (`http://thunderid-service:8090`) work unchanged. With another release name the Service is `<release>-thunderid-service`; update every service's `config.idp.jwksURL` and Portal Backend's `config.idp.management.baseURL` to match.

OpenNDX needs ThunderID seeded with its resource server and clients (`CONSENT_PORTAL`, `PORTAL_BACKEND`, `NDX_CLI`) and its CORS origins. These are the bootstrap files in [`thunderid/bootstrap/`](../../../thunderid/bootstrap/), which Compose also uses. Load them into a ConfigMap and pass the values they template (`{{ .CONSENT_PORTAL_URL }}`, `{{ .PORTAL_BACKEND_CLIENT_SECRET }}`, ...) to the chart's setup job. Edit the files first for a shared deployment: `application.yaml` and `users.yaml` also seed a local-dev `DATA_CONSUMER` client and sample citizen user, and `cors.yaml` lists the local origins.

```bash
kubectl create configmap openndx-thunderid-bootstrap \
  --namespace openndx \
  --from-file=02-users.yaml=thunderid/bootstrap/users.yaml \
  --from-file=03-resource.yaml=thunderid/bootstrap/resource.yaml \
  --from-file=04-application.yaml=thunderid/bootstrap/application.yaml \
  --from-file=05-cors.yaml=thunderid/bootstrap/cors.yaml

helm upgrade --install thunderid oci://ghcr.io/thunder-id/helm-charts/thunderid \
  --version 1.0.1 \
  --namespace openndx \
  -f thunderid-values.yaml
```

```yaml
# thunderid-values.yaml
configuration:
  server:
    # The URL browsers use to reach ThunderID. It is also the token issuer, so
    # the OpenNDX chart's global.idpPublicURL must equal it.
    publicUrl: https://idp.example.org
    # Serve plain HTTP inside the cluster (TLS ends at the ingress), so the
    # in-cluster http:// URLs in the OpenNDX values work.
    httpOnly: true
bootstrap:
  configMap:
    name: openndx-thunderid-bootstrap
    # Listing the files adds them beside ThunderID's own defaults.
    files: [02-users.yaml, 03-resource.yaml, 04-application.yaml, 05-cors.yaml]
setup:
  env:
    - {name: CONSENT_PORTAL_URL, value: https://consent.example.org}
  secretEnv:
    # The same secret Portal Backend authenticates with (see Secrets below).
    - {name: PORTAL_BACKEND_CLIENT_SECRET, secretName: openndx-secrets, secretKey: pbClientSecret}
    - {name: DATA_CONSUMER_CLIENT_SECRET, secretName: openndx-secrets, secretKey: dataConsumerClientSecret}
```

ThunderID's database, ingress, admin credentials and other settings are configured through its own chart values: run `helm show values oci://ghcr.io/thunder-id/helm-charts/thunderid --version 1.0.1` for the full list.

### Secrets

The chart creates no Secret and assumes nothing about how yours are laid out. Each service has an `env` map (name to a plain value, or to a map such as `valueFrom: {secretKeyRef: {name, key}}`) and an `envFrom` list; they supply the environment variables its `config` references as `"{{env:NAME}}"`. Create the Secrets however you like (kubectl, External Secrets Operator, Sealed Secrets, ...), one shared or one per service, and point `env` at them. A variable a config references but `env` doesn't set stops that service at startup.

The defaults read from one Secret named `openndx-secrets`. An override file merges into them by name, so it only needs the entries it changes; set an entry to `null` to drop it.

| Env var | Secret key | Set for | Purpose |
|---|---|---|---|
| `DB_USER` | `dbUser` | PDP, Consent Engine, Portal Backend | PostgreSQL user |
| `DB_PASSWORD` | `dbPassword` | PDP, Consent Engine, Portal Backend | PostgreSQL password |
| `PORTAL_BACKEND_CLIENT_SECRET` | `pbClientSecret` | Portal Backend, ThunderID setup job | Client secret of `config.idp.management.clientID`; ThunderID seeds the client with it |
| `DATA_CONSUMER_CLIENT_SECRET` | `dataConsumerClientSecret` | ThunderID setup job | Secret of the local-dev `DATA_CONSUMER` client; drop it with that client from `application.yaml` |

Read the credentials from files rather than passing them on the command line, where they end up in shell history. Each file's bytes become the value exactly, so write it without a trailing newline (`read -rs` prompts without echoing; `printf '%s'` adds no newline), and delete the files once the Secret exists:

```bash
read -rs DB_PASSWORD && printf '%s' "$DB_PASSWORD" > db-password
read -rs PB_CLIENT_SECRET && printf '%s' "$PB_CLIENT_SECRET" > pb-client-secret

kubectl create namespace openndx
kubectl create secret generic openndx-secrets \
  --namespace openndx \
  --from-literal=dbUser=postgres \
  --from-file=dbPassword=db-password \
  --from-file=pbClientSecret=pb-client-secret

rm db-password pb-client-secret
```

To turn auditing on for the Orchestration Engine or Portal Backend, set its `config.audit.url` and `config.audit.apiKey: "{{env:ARGUS_API_KEY}}"`, and add `ARGUS_API_KEY` to its `env`:

```yaml
portalBackend:
  env:
    ARGUS_API_KEY:
      valueFrom:
        secretKeyRef: {name: openndx-secrets, key: argusApiKey}
```

## Required values

The chart ships no URLs for the deployment's public addresses or for anything it doesn't deploy, so the defaults alone don't install. Until the values below are set, `helm install` (and `helm template`) fails and lists every one that is missing. Only the enabled services' values are checked.

| Value | What it is |
|---|---|
| `global.idpPublicURL` | ThunderID's public URL (its `configuration.server.publicUrl`). Fills in every service's `config.idp.issuer` and the Consent Portal's `idpBaseURL`. |
| `orchestrationEngine.configFiles."schema.graphql"` | The consumer GraphQL schema. Pass it with `--set-file`. |
| `portalBackend.config.cors.allowedOrigins` | The browser origins allowed to call Portal Backend, which refuses to start without one. |
| `consentEngine.config.consentPortalURL` | The Consent Portal's URL, used in consent links. Derived from the Consent Portal's ingress. |
| `consentPortal.config.consentEngineURL` | The Consent Engine's public API (`.../api/v1`), called from the browser. Derived from the Consent Engine's ingress. |
| `consentPortal.config.idpSignInRedirectURL`, `idpSignOutRedirectURL` | Where ThunderID returns after sign-in and sign-out. Derived from the Consent Portal's ingress. |

A derived URL is the first host of that service's ingress, with `https` when the host is listed under the ingress's `tls`, and `http` otherwise. It is only derived when that service is enabled and its ingress is turned on; otherwise, set the value itself. A value you set always wins.

The chart deploys no data providers. List yours under `orchestrationEngine.config.providers`, one per schema ID the consumer schema's `@sourceInfo` uses ([PROVIDER_CONFIGURATION.md](../../../cmd/oe/PROVIDER_CONFIGURATION.md)), with any `argMappings` they need. With none listed, the Orchestration Engine starts, but every provider field comes back `null` with a `PROVIDER_ERROR`.

```yaml
# my-values.yaml
global:
  idpPublicURL: https://idp.example.org

orchestrationEngine:
  ingress:
    enabled: true
    hosts: [{host: api.example.org, paths: [{path: /, pathType: Prefix}]}]
    tls: [{hosts: [api.example.org], secretName: api-tls}]
  config:
    providers:
      - id: drp-person-v1
        url: https://drp.example.org/graphql
        auth:
          type: apiKey
          apiKey:
            key: X-API-Key
            value: "{{env:DRP_API_KEY}}"
  env:
    DRP_API_KEY:
      valueFrom:
        secretKeyRef: {name: openndx-secrets, key: drpApiKey}

consentEngine:
  ingress:
    enabled: true
    hosts: [{host: consent-api.example.org, paths: [{path: /, pathType: Prefix}]}]
    tls: [{hosts: [consent-api.example.org], secretName: consent-api-tls}]

consentPortal:
  ingress:
    enabled: true
    hosts: [{host: consent.example.org, paths: [{path: /, pathType: Prefix}]}]
    tls: [{hosts: [consent.example.org], secretName: consent-tls}]

portalBackend:
  config:
    cors:
      allowedOrigins: [https://admin.example.org]
```

`cmd/oe/schema.graphql` and the providers in `cmd/oe/config.example.yaml` are samples for the mock providers in [`dev/`](../../../dev/README.md), which this chart doesn't deploy.

## Usage

### Deploy the full OpenNDX stack

```bash
helm upgrade --install openndx ./deploy/helm/openndx \
  --namespace openndx --create-namespace \
  -f my-values.yaml \
  --set-file 'orchestrationEngine.configFiles.schema\.graphql=schema.graphql'
```

### Deploy only Orchestration Engine (minimal / stateless mode)

With the PDP and Consent Engine disabled, set the Orchestration Engine's `pdp.url` and `consentEngine.url` to `""` so it skips policy and consent checks. It still needs the IdP URL, the consumer schema and, to return any data, its providers:

```bash
helm upgrade --install openndx ./deploy/helm/openndx \
  --namespace openndx --create-namespace \
  -f my-providers.yaml \
  --set global.idpPublicURL=https://idp.example.org \
  --set-file 'orchestrationEngine.configFiles.schema\.graphql=schema.graphql' \
  --set policyDecisionPoint.enabled=false \
  --set consentEngine.enabled=false \
  --set consentPortal.enabled=false \
  --set portalBackend.enabled=false \
  --set orchestrationEngine.config.pdp.url="" \
  --set orchestrationEngine.config.consentEngine.url=""
```

### Validate templates locally

Pass the same values as the install, since the defaults alone fail on the required values:

```bash
helm lint ./deploy/helm/openndx -f my-values.yaml \
  --set-file 'orchestrationEngine.configFiles.schema\.graphql=schema.graphql'
helm template openndx ./deploy/helm/openndx -f my-values.yaml \
  --set-file 'orchestrationEngine.configFiles.schema\.graphql=schema.graphql'
```
