{{/*
Expand the name of the chart.
*/}}
{{- define "openndx.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "openndx.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "openndx.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "openndx.labels" -}}
helm.sh/chart: {{ include "openndx.chart" . }}
{{ include "openndx.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "openndx.selectorLabels" -}}
app.kubernetes.io/name: {{ include "openndx.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "openndx.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "openndx.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
The services this chart deploys, keyed by the suffix of their resource names
(<fullname>-<key>). configmap.yaml, deployment.yaml, service.yaml, ingress.yaml
and hpa.yaml range over this list; read it with:
  include "openndx.components" . | fromYaml
Every template reads it, so it also runs openndx.validate: a missing required
value fails the install before anything renders.
Each entry:
  values         key of the service's block in values.yaml
  component      app.kubernetes.io/component label and container name
  enabled        whether the service is deployed
  configMap      render <values>.config into config.yaml (configmap.yaml), mount
                 it at /app/config and restart pods when it changes
  containerPort  the port the container listens on: config.server.port for
                 the Go services, falling back to service.port
  env            environment variables the chart sets itself; secrets and
                 anything else come from <values>.env and <values>.envFrom
  liveness       liveness probe timings
*/}}
{{- define "openndx.components" -}}
{{- include "openndx.validate" . }}
oe:
  values: orchestrationEngine
  component: orchestration-engine
  enabled: true
  configMap: true
  containerPort: {{ dig "server" "port" .Values.orchestrationEngine.service.port .Values.orchestrationEngine.config }}
  env:
    - {name: SERVICE_NAME, value: orchestration-engine}
  liveness: {initialDelaySeconds: 10, periodSeconds: 30, timeoutSeconds: 10}
pdp:
  values: policyDecisionPoint
  component: policy-decision-point
  enabled: {{ .Values.policyDecisionPoint.enabled }}
  configMap: true
  containerPort: {{ dig "server" "port" .Values.policyDecisionPoint.service.port .Values.policyDecisionPoint.config }}
  liveness: {initialDelaySeconds: 10, periodSeconds: 30, timeoutSeconds: 5}
ce:
  values: consentEngine
  component: consent-engine
  enabled: {{ .Values.consentEngine.enabled }}
  configMap: true
  containerPort: {{ dig "server" "port" .Values.consentEngine.service.port .Values.consentEngine.config }}
  liveness: {initialDelaySeconds: 10, periodSeconds: 30, timeoutSeconds: 5}
consent-portal:
  values: consentPortal
  component: consent-portal
  enabled: {{ .Values.consentPortal.enabled }}
  configMap: false
  containerPort: 80
  {{- if .Values.consentPortal.enabled }}
  {{- with include "openndx.config" (dict "root" . "values" "consentPortal") | fromYaml }}
  env:
    - {name: CONSENT_ENGINE_URL, value: {{ .consentEngineURL | quote }}}
    - {name: IDP_CLIENT_ID, value: {{ .idpClientId | quote }}}
    - {name: IDP_BASE_URL, value: {{ .idpBaseURL | quote }}}
    - {name: IDP_SCOPE, value: {{ .idpScope | quote }}}
    - {name: IDP_SIGN_IN_REDIRECT_URL, value: {{ .idpSignInRedirectURL | quote }}}
    - {name: IDP_SIGN_OUT_REDIRECT_URL, value: {{ .idpSignOutRedirectURL | quote }}}
  {{- end }}
  {{- end }}
  liveness: {initialDelaySeconds: 5, periodSeconds: 15, timeoutSeconds: 5}
pb:
  values: portalBackend
  component: portal-backend
  enabled: {{ .Values.portalBackend.enabled }}
  configMap: true
  containerPort: {{ dig "server" "port" .Values.portalBackend.service.port .Values.portalBackend.config }}
  liveness: {initialDelaySeconds: 10, periodSeconds: 30, timeoutSeconds: 5}
{{- end }}

{{/*
Fails, listing every missing required value at once, when an enabled service's
config can't be completed (see openndx.config). Renders nothing otherwise.
*/}}
{{- define "openndx.validate" -}}
{{- $errs := dict "list" (list) }}
{{- range list "orchestrationEngine" "consentEngine" "consentPortal" "portalBackend" }}
{{- if dig "enabled" true (index $.Values .) }}
{{- $_ := include "openndx.config" (dict "root" $ "values" . "errs" $errs) }}
{{- end }}
{{- end }}
{{- if and .Values.networkPolicy.enabled (not .Values.networkPolicy.ingressFrom) }}
{{- $exposed := list }}
{{- range list "orchestrationEngine" "consentEngine" "consentPortal" "portalBackend" }}
{{- $v := index $.Values . }}
{{- if and (dig "enabled" true $v) (dig "ingress" "enabled" false $v) }}
{{- $exposed = append $exposed (printf "%s.ingress" .) }}
{{- end }}
{{- end }}
{{- with $exposed }}
{{- $_ := set $errs "list" (append $errs.list (printf "networkPolicy.ingressFrom: the ingress controller's pods as NetworkPolicy peers, needed while an ingress is enabled (%s); or set networkPolicy.enabled=false" (join ", " .))) }}
{{- end }}
{{- end }}
{{- with $errs.list | uniq }}
{{- fail (printf "missing required values (see the chart README, \"Required values\"):\n  - %s" (join "\n  - " .)) }}
{{- end }}
{{- end }}

{{/*
A service's config as the chart renders it: <values>.config, with each public
URL that is left empty filled in from global.idpPublicURL or an ingress, and
the values that have no default checked. configmap.yaml renders it,
deployment.yaml hashes it, and the Consent Portal's env reads it:
  include "openndx.config" (dict "root" $ "values" "consentEngine") | fromYaml
A value it can't fill in is added to "errs" ({list: [...]}) when one is passed;
without one, it fails itself.
*/}}
{{- define "openndx.config" -}}
{{- $root := .root }}
{{- $key := .values }}
{{- $errs := .errs | default (dict "list" (list)) }}
{{- $cfg := deepCopy ((index $root.Values $key).config | default dict) }}
{{- if has $key (list "orchestrationEngine" "consentEngine" "portalBackend") }}
{{- $idp := $cfg.idp | default dict }}
{{- if not $idp.issuer }}
{{- $_ := set $idp "issuer" (include "openndx.idpPublicURL" (dict "root" $root "errs" $errs)) }}
{{- end }}
{{- $_ := set $cfg "idp" $idp }}
{{- end }}
{{- if eq $key "orchestrationEngine" }}
{{- $schema := $cfg.schemaPath | default "schema.graphql" }}
{{- if and (not (isAbs $schema)) (not (index ($root.Values.orchestrationEngine.configFiles | default dict) $schema)) }}
{{- $_ := set $errs "list" (append $errs.list (printf "orchestrationEngine.configFiles.%s: the consumer GraphQL schema named by config.schemaPath, e.g. --set-file 'orchestrationEngine.configFiles.%s=schema.graphql'" $schema (replace "." "\\." $schema))) }}
{{- end }}
{{- end }}
{{- if eq $key "consentEngine" }}
{{- if not $cfg.consentPortalURL }}
{{- $_ := set $cfg "consentPortalURL" (include "openndx.publicURL" (dict "root" $root "service" "consentPortal" "for" "consentEngine.config.consentPortalURL" "errs" $errs)) }}
{{- end }}
{{- end }}
{{- if eq $key "consentPortal" }}
{{- if not $cfg.consentEngineURL }}
{{- with include "openndx.publicURL" (dict "root" $root "service" "consentEngine" "for" "consentPortal.config.consentEngineURL" "errs" $errs) }}
{{- $_ := set $cfg "consentEngineURL" (printf "%s/api/v1" .) }}
{{- end }}
{{- end }}
{{- if not $cfg.idpBaseURL }}
{{- $_ := set $cfg "idpBaseURL" (include "openndx.idpPublicURL" (dict "root" $root "errs" $errs)) }}
{{- end }}
{{- range list "idpSignInRedirectURL" "idpSignOutRedirectURL" }}
{{- if not (get $cfg .) }}
{{- $_ := set $cfg . (include "openndx.publicURL" (dict "root" $root "service" "consentPortal" "for" (printf "consentPortal.config.%s" .) "errs" $errs)) }}
{{- end }}
{{- end }}
{{- end }}
{{- if eq $key "portalBackend" }}
{{- if not (dig "cors" "allowedOrigins" (list) $cfg) }}
{{- $_ := set $errs "list" (append $errs.list "portalBackend.config.cors.allowedOrigins: the browser origins allowed to call Portal Backend, which refuses to start without one") }}
{{- end }}
{{- end }}
{{- if and (not .errs) $errs.list }}
{{- fail (printf "missing required values:\n  - %s" (join "\n  - " ($errs.list | uniq))) }}
{{- end }}
{{- toYaml $cfg }}
{{- end }}

{{/*
global.idpPublicURL without a trailing slash; when it is empty, renders
nothing and adds it to "errs".
*/}}
{{- define "openndx.idpPublicURL" -}}
{{- with .root.Values.global.idpPublicURL }}
{{- trimSuffix "/" . }}
{{- else }}
{{- $_ := set .errs "list" (append .errs.list "global.idpPublicURL: ThunderID's public URL (its configuration.server.publicUrl), which fills in every config.idp.issuer and the Consent Portal's idpBaseURL") }}
{{- end }}
{{- end }}

{{/*
The public URL of a deployed service, from its ingress (openndx.ingressURL).
When there is none, renders nothing and adds "for", the value that needed it,
to "errs".
*/}}
{{- define "openndx.publicURL" -}}
{{- $url := include "openndx.ingressURL" (index .root.Values .service) }}
{{- if $url }}
{{- $url }}
{{- else }}
{{- $_ := set .errs "list" (append .errs.list (printf "%s: set it, or deploy %s with its ingress enabled so it can be derived" .for .service)) }}
{{- end }}
{{- end }}

{{/*
A service's public URL from its ingress: the first host, https when that host
is listed under ingress.tls, else http. Empty when the service is disabled or
its ingress is off. Takes the service's values block.
*/}}
{{- define "openndx.ingressURL" -}}
{{- $v := . }}
{{- if and (dig "enabled" true $v) (dig "ingress" "enabled" false $v) }}
{{- with first ($v.ingress.hosts | default list) }}
{{- $host := .host }}
{{- $scheme := "http" }}
{{- range $v.ingress.tls }}
{{- if has $host (.hosts | default list) }}
{{- $scheme = "https" }}
{{- end }}
{{- end }}
{{- printf "%s://%s" $scheme $host }}
{{- end }}
{{- end }}
{{- end }}
