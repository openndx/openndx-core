# OpenNDX Helm Chart

A single Helm chart for deploying the maintained OpenNDX services on Kubernetes.

## Components

All components are configured from [`values.yaml`](./values.yaml):

| Component                 | Key                           | Default           | Description                                                                         |
| ------------------------- | ----------------------------- | ----------------- | ----------------------------------------------------------------------------------- |
| **Orchestration Engine**  | `orchestrationEngine`         | _Always deployed_ | Core GraphQL federation engine (`POST /public/graphql`)                             |
| **Policy Decision Point** | `policyDecisionPoint.enabled` | `true`            | Field-level access decisions from field policies and access grants (`internal/pdp`) |
| **Consent Engine**        | `consentEngine.enabled`       | `true`            | Data-owner consent lifecycle service (`internal/ce`)                                |
| **Consent Portal**        | `consentPortal.enabled`       | `true`            | Citizen-facing React UI for approving/rejecting consents                            |
| **Portal Backend**        | `portalBackend.enabled`       | `true`            | Management API for members, schemas, applications, and submissions (`internal/pb`)  |

> **Embedded identity provider:** OpenNDX requires ThunderID as its embedded IdP. It signs the tokens every service validates, and Portal Backend provisions application clients through its management API, so a generic OIDC provider is not a substitute. Install it from ThunderID's published chart before this one (see [Embedded identity provider (ThunderID)](#embedded-identity-provider-thunderid)).
>
> **External infrastructure:** PostgreSQL and the OpenTelemetry collector or backend that receives audit records are outside the scope of this chart and should be deployed via their own Helm charts or managed cloud services. Each service's `config` points at them.

### Service configuration

Each Go service's `config` block in `values.yaml` **is** its `config.yaml`, with the same schema as `cmd/<svc>/config.example.yaml` (keys in `cmd/<svc>/README.md`, and in `CONFIGURATION.md` for the OE and PB). The chart renders it as-is into a ConfigMap mounted at `/app/config/config.yaml`. The only exceptions are the deployment's public URLs, which the chart fills in when left empty, and the URLs one service uses to call another, which it fills in when left out (see [Required values](#required-values)). Extra files go under `configFiles` and are mounted beside it (the Orchestration Engine's `schema.graphql`).

Keep `config` non-secret. A secret goes in as a `"{{env:NAME}}"` placeholder, which the service resolves at startup from its environment (see [Secrets](#secrets)).

## Prerequisites

- Kubernetes 1.26+
- Helm 3.12+
- PostgreSQL 15+ with each enabled service's schema applied (`cmd/pdp/schema.sql`, `cmd/ce/schema.sql`, `cmd/pb/schema.sql`)
- ThunderID, the embedded identity provider, installed from `oci://ghcr.io/thunder-id/helm-charts/thunderid` (see [below](#embedded-identity-provider-thunderid))
- The Secrets your services' `env` points at, created before install (see below)

### Embedded identity provider (ThunderID)

ThunderID publishes its chart at `oci://ghcr.io/thunder-id/helm-charts/thunderid`. Install it in the same namespace as a release named `thunderid`: the chart then names its Service `thunderid-service`, and the default `idp` URLs in [`values.yaml`](./values.yaml) (`http://thunderid-service:8090`) work unchanged. With another release name the Service is `<release>-thunderid-service`; update every service's `config.idp.jwksURL` and Portal Backend's `config.idp.management.baseURL` to match.

OpenNDX needs ThunderID seeded with its resource servers and roles, the `CONSENT_PORTAL` and `PORTAL_BACKEND` clients, and its CORS origins. These are the bootstrap files in [`thunderid/bootstrap/`](../../../thunderid/bootstrap/), which Compose also uses. Load them into a ConfigMap and pass the values they template (`{{ .CONSENT_PORTAL_URL }}`, `{{ .PORTAL_BACKEND_CLIENT_SECRET }}`, ...) to the chart's setup job. Edit the files first for a shared deployment: they also seed entries for local use only, such as the sample users, the `DATA_CONSUMER` and `NDX_CLI` clients and the local CORS origins, which [thunderid/README.md](../../../thunderid/README.md#entries-for-local-use-only) lists.

```bash
kubectl create configmap openndx-thunderid-bootstrap \
  --namespace openndx \
  --from-file=02-users.yaml=thunderid/bootstrap/users.yaml \
  --from-file=03-resource.yaml=thunderid/bootstrap/resource.yaml \
  --from-file=04-application.yaml=thunderid/bootstrap/application.yaml \
  --from-file=05-cors.yaml=thunderid/bootstrap/cors.yaml

helm upgrade --install thunderid oci://ghcr.io/thunder-id/helm-charts/thunderid \
  --version 1.1.1 \
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

ThunderID's database, ingress, admin credentials and other settings are configured through its own chart values: run `helm show values oci://ghcr.io/thunder-id/helm-charts/thunderid --version 1.1.1` for the full list.

#### Refuse self-service attribute changes

Any signed-in ThunderID user can change their own attributes with `PUT /users/me`, and ThunderID has no setting to turn the route off. That includes the claim the Consent Engine matches against a consent's owner (`consentEngine.config.idp.subjectClaim`, `nic` by default): a citizen could set it to the ID of someone without an account and decide on that person's consents. OpenNDX doesn't call the route, so refuse it at the ingress or gateway that exposes ThunderID. In nginx, for example, beside the `location /` that passes everything else to ThunderID:

```nginx
location ~ ^/users/me/?$ {
    limit_except GET {
        deny all;
    }
    proxy_pass http://thunderid-service:8090;
}
```

`curl -i -X PUT https://idp.example.org/users/me` must then return `403` from the gateway; ThunderID itself answers `401`.

### Secrets

The chart creates no Secret and assumes nothing about how yours are laid out. Each service has an `env` map (name to a plain value, or to a map such as `valueFrom: {secretKeyRef: {name, key}}`) and an `envFrom` list; they supply the environment variables its `config` references as `"{{env:NAME}}"`. Create the Secrets however you like (kubectl, External Secrets Operator, Sealed Secrets, ...), one shared or one per service, and point `env` at them. A variable a config references but `env` doesn't set stops that service at startup.

The defaults read from one Secret named `openndx-secrets`. An override file merges into them by name, so it only needs the entries it changes; set an entry to `null` to drop it.

| Env var                        | Secret key                 | Set for                             | Purpose                                                                                          |
| ------------------------------ | -------------------------- | ----------------------------------- | ------------------------------------------------------------------------------------------------ |
| `DB_USER`                      | `dbUser`                   | PDP, Consent Engine, Portal Backend | PostgreSQL user                                                                                  |
| `DB_PASSWORD`                  | `dbPassword`               | PDP, Consent Engine, Portal Backend | PostgreSQL password                                                                              |
| `PORTAL_BACKEND_CLIENT_SECRET` | `pbClientSecret`           | Portal Backend, ThunderID setup job | Client secret of `config.idp.management.clientID`; ThunderID seeds the client with it            |
| `DATA_CONSUMER_CLIENT_SECRET`  | `dataConsumerClientSecret` | ThunderID setup job                 | Secret of the local-dev `DATA_CONSUMER` client; drop it with that client from `application.yaml` |

Read the credentials from files rather than passing them on the command line, where they end up in shell history. Each file's bytes become the value exactly, so write it without a trailing newline (`read -rs` prompts without echoing; `printf '%s'` adds no newline), and delete the files once the Secret exists:

```bash
read -rs DB_PASSWORD && printf '%s' "$DB_PASSWORD" > db-password
read -rs PB_CLIENT_SECRET && printf '%s' "$PB_CLIENT_SECRET" > pb-client-secret
read -rs DATA_CONSUMER_CLIENT_SECRET && printf '%s' "$DATA_CONSUMER_CLIENT_SECRET" > data-consumer-client-secret

kubectl create namespace openndx
kubectl create secret generic openndx-secrets \
  --namespace openndx \
  --from-literal=dbUser=postgres \
  --from-file=dbPassword=db-password \
  --from-file=pbClientSecret=pb-client-secret \
  --from-file=dataConsumerClientSecret=data-consumer-client-secret

rm db-password pb-client-secret data-consumer-client-secret
```

The Orchestration Engine and Portal Backend send [audit records](../../../internal/audit/README.md) only to the OpenTelemetry endpoint in `config.otel`, and drop them while it is empty. To keep them, point each service's `config.otel` at your collector or backend, with any credentials taken from a Secret through its `env`:

```yaml
portalBackend:
  env:
    OTEL_AUTH_HEADER:
      valueFrom:
        secretKeyRef: {name: openndx-secrets, key: otelAuthHeader}
  config:
    otel:
      endpoint: http://otel-collector:4318
      headers:
        Authorization: "{{env:OTEL_AUTH_HEADER}}"
```

The records carry `category=audit`, so the backend can route them to their own stream or index.

## Required values

The chart ships no URLs for the deployment's public addresses or for anything it doesn't deploy, so the defaults alone don't install. Until the values below are set, `helm install` (and `helm template`) fails and lists every one that is missing. Only the enabled services' values are checked.

| Value                                                                | What it is                                                                                                                                         |
| -------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| `global.idpPublicURL`                                                | ThunderID's public URL (its `configuration.server.publicUrl`). Fills in every service's `config.idp.issuer` and the Consent Portal's `idpBaseURL`. |
| `orchestrationEngine.configFiles."schema.graphql"`                   | The consumer GraphQL schema. Pass it with `--set-file`.                                                                                            |
| `portalBackend.config.cors.allowedOrigins`                           | The browser origins allowed to call Portal Backend, which refuses to start without one.                                                            |
| `consentEngine.config.consentPortalURL`                              | The Consent Portal's URL, used in consent links. Derived from the Consent Portal's ingress.                                                        |
| `consentPortal.config.consentEngineURL`                              | The Consent Engine's public API (`.../api/v1`), called from the browser. Derived from the Consent Engine's ingress.                                |
| `consentPortal.config.idpSignInRedirectURL`, `idpSignOutRedirectURL` | Where ThunderID returns after sign-in and sign-out. Derived from the Consent Portal's ingress.                                                     |
| `networkPolicy.ingressFrom`                                          | Where the ingress controller's pods run. Required while any ingress is enabled (see [Network policies](#network-policies)).                        |

A derived URL is the first host of that service's ingress, with `https` when the host is listed under the ingress's `tls`, and `http` otherwise. It is only derived when that service is enabled and its ingress is turned on; otherwise, set the value itself. A value you set always wins.

The URLs one service uses to call another are left out of `values.yaml`, and the chart fills each in with the release's own Service while that service is enabled, whatever the release is named:

| Value                                          | Filled in with                              | When its service is disabled                                                                         |
| ---------------------------------------------- | ------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `orchestrationEngine.config.pdp.url`           | `http://<fullname>-pdp:8082`                | Set it to an external PDP, or to `""` to switch off authorization and consent.                       |
| `orchestrationEngine.config.consentEngine.url` | `http://<fullname>-ce:8081/internal/api/v1` | Set it to an external Consent Engine. Not needed while the Orchestration Engine's `pdp.url` is `""`. |
| `portalBackend.config.pdp.url`                 | `http://<fullname>-pdp:8082`                | Set it to an external PDP. The Portal Backend requires one, so `""` fails too.                       |

`<fullname>` is the prefix of every resource the chart creates: `fullnameOverride` when set, otherwise `<release>-openndx`, or just the release name when it already contains `openndx`. The port is the service's `service.port`. A value you set, `""` included, is used as-is. Left out with its service disabled, the install fails rather than calling a Service that doesn't exist, so switching off authorization is always an explicit `pdp.url: ""`.

The chart deploys no data providers. List yours under `orchestrationEngine.config.providers`, one per schema ID the consumer schema's `@sourceInfo` uses ([PROVIDER_CONFIGURATION.md](../../../cmd/oe/PROVIDER_CONFIGURATION.md)), with any `argMappings` they need. With none listed, the Orchestration Engine starts, but every provider field comes back `null` with a `PROVIDER_ERROR`.

```yaml
# my-values.yaml
global:
  idpPublicURL: https://idp.example.org

networkPolicy:
  ingressFrom:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: ingress-nginx

orchestrationEngine:
  ingress:
    enabled: true
    hosts: [{host: api.example.org, paths: [{path: /, pathType: Prefix}]}]
    tls: [{hosts: [api.example.org], secretName: api-tls}]
  config:
    providers:
      - id: identity-registry-v1
        url: https://identity-registry.example.org/graphql
        auth:
          type: apiKey
          apiKey:
            key: X-API-Key
            value: "{{env:IDENTITY_REGISTRY_API_KEY}}"
  env:
    IDENTITY_REGISTRY_API_KEY:
      valueFrom:
        secretKeyRef: {name: openndx-secrets, key: identityRegistryApiKey}

consentEngine:
  ingress:
    enabled: true
    hosts: [{host: consent-api.example.org, paths: [{path: /api/v1, pathType: Prefix}]}]
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

## Network policies

The PDP's API (including `POST /api/v1/policy/grants`) and the Consent Engine's `/internal/api/v1` have no authentication. They rely on the network, so the chart ships a NetworkPolicy for each service, on by default (`networkPolicy.enabled`). Each one limits inbound traffic to the service's port:

| Service                                              | Accepts traffic from                                                                                       |
| ---------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| Policy Decision Point                                | The Orchestration Engine and Portal Backend                                                                |
| Consent Engine                                       | The Orchestration Engine, plus `networkPolicy.ingressFrom` while its ingress is enabled                    |
| Orchestration Engine, Portal Backend, Consent Portal | `networkPolicy.ingressFrom` while its ingress is enabled; anyone when it has no ingress and no `extraFrom` |

Each service also accepts the peers in its own `<service>.networkPolicy.extraFrom`.

`networkPolicy.ingressFrom` is a list of NetworkPolicy `from` peers for the ingress controller, typically a `namespaceSelector` for its namespace. It is required while any ingress is enabled, and added only to the policies of the services whose ingress is enabled. Keep it to the ingress controller: whatever it lists can reach every exposed service, the Consent Engine's unauthenticated `/internal` API included.

Any other caller, such as a monitoring scraper, an in-cluster consumer of a service that also has an ingress, or a `LoadBalancer` Service, goes in that service's `networkPolicy.extraFrom`, which lets it into that service alone. For example, to let a consumer application in the cluster's `consumers` namespace call the Orchestration Engine:

```yaml
orchestrationEngine:
  networkPolicy:
    extraFrom:
      - namespaceSelector:
          matchLabels:
            kubernetes.io/metadata.name: consumers
```

A service with an ingress accepts only the ingress controller and its `extraFrom`, so list its in-cluster clients there. The PDP's and Consent Engine's `extraFrom` open APIs that have no authentication; add to them only what must call those APIs.

The policies only limit inbound traffic. Egress stays open, since PostgreSQL, ThunderID, the providers and the OpenTelemetry endpoint are all outside the chart. NetworkPolicies take effect only when the cluster's network plugin enforces them (Calico, Cilium and most managed Kubernetes do); otherwise they are ignored. Set `networkPolicy.enabled=false` when the cluster manages network policy another way.

A NetworkPolicy works per port, not per URL path. Anything allowed to reach the Consent Engine, the ingress controller and its `extraFrom` included, can reach its `/internal` API, so keep both as narrow as they can be.

From outside the cluster, the `/internal` API is kept off by the ingress itself: by default, the Consent Engine's ingress routes only the public API, `/api/v1`, which is all the Consent Portal calls. Avoid widening its paths. A path of `/` would also route `/internal/api/v1`, which has no authentication, and `/metrics` to the internet.

## Pod security

The Orchestration Engine, PDP, Consent Engine and Portal Backend meet the Kubernetes Pod Security Standards "restricted" profile by default: they run as a non-root user, with privilege escalation off, every capability dropped, and the `RuntimeDefault` seccomp profile.

The Consent Portal doesn't yet. Its image runs nginx as root on port 80 and writes `config.js` into its own filesystem at startup, so the chart sets only `seccompProfile: RuntimeDefault` for it. In a namespace that enforces "restricted", its pod is rejected; enforce `baseline` there instead (`pod-security.kubernetes.io/enforce: baseline`), which it meets. This lasts until the image runs unprivileged ([#767](https://github.com/openndx/openndx-core/issues/767)).

## Scaling

Each component runs `replicaCount` pods (default 1). To scale one on CPU instead, set its `autoscaling.enabled: true`, with `minReplicas`, `maxReplicas` and `targetCPUUtilizationPercentage`. The chart then creates a HorizontalPodAutoscaler and leaves the replica count to it. `values.yaml` lists these keys under `orchestrationEngine`, and every component reads them.

## Usage

### Deploy the full OpenNDX stack

```bash
helm upgrade --install openndx ./deploy/helm/openndx \
  --namespace openndx --create-namespace \
  -f my-values.yaml \
  --set-file 'orchestrationEngine.configFiles.schema\.graphql=schema.graphql'
```

### Deploy only Orchestration Engine (minimal / stateless mode)

With the PDP and Consent Engine disabled, set the Orchestration Engine's `pdp.url` to `""` so it skips policy and consent checks; the chart then needs no `consentEngine.url`. The Portal Backend requires a PDP, so it is disabled too. It still needs the IdP URL, the consumer schema and, to return any data, its providers:

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
  --set orchestrationEngine.config.pdp.url=""
```

### Validate templates locally

Pass the same values as the install, since the defaults alone fail on the required values:

```bash
helm lint ./deploy/helm/openndx -f my-values.yaml \
  --set-file 'orchestrationEngine.configFiles.schema\.graphql=schema.graphql'
helm template openndx ./deploy/helm/openndx -f my-values.yaml \
  --set-file 'orchestrationEngine.configFiles.schema\.graphql=schema.graphql'
```
