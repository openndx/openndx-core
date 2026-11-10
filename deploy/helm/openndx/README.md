# OpenNDX Helm Chart

Single umbrella Helm chart for deploying the maintained OpenNDX services on Kubernetes.

## Components

All components are configured from [`values.yaml`](./values.yaml):

| Component | Key | Default | Description |
|---|---|---|---|
| **Orchestration Engine** | `orchestrationEngine` | *Always deployed* | Core GraphQL federation engine (`POST /public/graphql`) |
| **Policy Decision Point** | `policyDecisionPoint.enabled` | `true` | Field-level Rego policy evaluation (`internal/pdp`) |
| **Consent Engine** | `consentEngine.enabled` | `true` | Data-owner consent lifecycle service (`internal/ce`) |
| **Consent Portal** | `consentPortal.enabled` | `true` | Citizen-facing React UI for approving/rejecting consents |
| **Portal Backend** | `portalBackend.enabled` | `true` | Management API for members, schemas, applications, and submissions (`internal/pb`) |

> **External Infrastructure:** PostgreSQL, the Identity Provider (e.g. ThunderID), and the optional Audit Service (Argus) are outside the scope of this chart and should be deployed via their own Helm charts or managed cloud services. Each service's `config` points at them.

### Service configuration

Each Go service's `config` block in `values.yaml` **is** its `config.yaml`, with the same schema as `cmd/<svc>/config.example.yaml` (keys in `cmd/<svc>/README.md`). The chart renders it as-is into a ConfigMap mounted at `/app/config/config.yaml`; nothing is merged or filled in. Extra files go under `configFiles` and are mounted beside it (the Orchestration Engine's `schema.graphql`).

Keep `config` non-secret. A secret goes in as a `"{{env:NAME}}"` placeholder, which the service resolves at startup from its environment (see [Secrets](#secrets)).

The default in-cluster URLs (`http://openndx-pdp:8082`, `http://openndx-ce:8081/internal/api/v1`) assume the release is named `openndx`. With another release name, or with a service disabled, update the URLs that point at it.

## Prerequisites

- Kubernetes 1.26+
- Helm 3.12+
- PostgreSQL 15+ with each enabled service's schema applied (`cmd/pdp/schema.sql`, `cmd/ce/schema.sql`, `cmd/pb/schema.sql`)
- OIDC / OAuth2 Identity Provider (e.g. ThunderID)
- The Secrets your services' `env` points at, created before install (see below)

### Secrets

The chart creates no Secret and assumes nothing about how yours are laid out. Each service has an `env` map (name to a plain value, or to a map such as `valueFrom: {secretKeyRef: {name, key}}`) and an `envFrom` list; they supply the environment variables its `config` references as `"{{env:NAME}}"`. Create the Secrets however you like (kubectl, External Secrets Operator, Sealed Secrets, ...), one shared or one per service, and point `env` at them. A variable a config references but `env` doesn't set stops that service at startup.

The defaults read from one Secret named `openndx-secrets`. An override file merges into them by name, so it only needs the entries it changes; set an entry to `null` to drop it.

| Env var | Secret key | Set for | Purpose |
|---|---|---|---|
| `DB_USER` | `dbUser` | PDP, Consent Engine, Portal Backend | PostgreSQL user |
| `DB_PASSWORD` | `dbPassword` | PDP, Consent Engine, Portal Backend | PostgreSQL password |
| `PORTAL_BACKEND_CLIENT_SECRET` | `pbClientSecret` | Portal Backend | Client secret of `config.idp.management.clientID` |

```bash
kubectl create namespace openndx
kubectl create secret generic openndx-secrets \
  --namespace openndx \
  --from-literal=dbUser=postgres \
  --from-literal=dbPassword='<db-password>' \
  --from-literal=pbClientSecret='<pb-client-secret>'
```

To turn auditing on for the Orchestration Engine or Portal Backend, set its `config.audit.url` and `config.audit.apiKey: "{{env:ARGUS_API_KEY}}"`, and add `ARGUS_API_KEY` to its `env`:

```yaml
portalBackend:
  env:
    ARGUS_API_KEY:
      valueFrom:
        secretKeyRef: {name: openndx-secrets, key: argusApiKey}
```

## Usage

### Deploy the full OpenNDX stack

```bash
helm upgrade --install openndx ./deploy/helm/openndx \
  --namespace openndx --create-namespace \
  -f my-values.yaml
```

### Deploy only Orchestration Engine (minimal / stateless mode)

With the PDP and Consent Engine disabled, set the Orchestration Engine's `pdp.url` and `consentEngine.url` to `""` so it skips policy and consent checks:

```bash
helm upgrade --install openndx ./deploy/helm/openndx \
  --namespace openndx --create-namespace \
  --set policyDecisionPoint.enabled=false \
  --set consentEngine.enabled=false \
  --set consentPortal.enabled=false \
  --set portalBackend.enabled=false \
  --set orchestrationEngine.config.pdp.url="" \
  --set orchestrationEngine.config.consentEngine.url=""
```

### Validate templates locally

```bash
helm lint ./deploy/helm/openndx
helm template openndx ./deploy/helm/openndx
```
