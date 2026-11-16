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
(<fullname>-<key>). Every template ranges over this list; read it with:
  include "openndx.components" . | fromYaml
It holds only fixed facts about each service, never user values, so the
fromYaml round trip can't be broken by a value. The templates read everything
else, enabled included, from the service's values block.
Each entry:
  values     key of the service's block in values.yaml
  component  app.kubernetes.io/component label and container name
  configMap  render <values>.config into config.yaml (configmap.yaml), mount
             it at /app/config and restart pods when it changes
  port       the port the image listens on, for an image that fixes it; the
             Go services' ports come from their config (openndx.containerPort)
  env        environment variables the chart always sets; secrets and
             anything else come from <values>.env and <values>.envFrom
*/}}
{{- define "openndx.components" -}}
oe:
  values: orchestrationEngine
  component: orchestration-engine
  configMap: true
  env:
    - {name: SERVICE_NAME, value: orchestration-engine}
pdp:
  values: policyDecisionPoint
  component: policy-decision-point
  configMap: true
ce:
  values: consentEngine
  component: consent-engine
  configMap: true
consent-portal:
  values: consentPortal
  component: consent-portal
  configMap: false
  port: 80
pb:
  values: portalBackend
  component: portal-backend
  configMap: true
{{- end }}

{{/*
The port a service's container listens on: the component's fixed port, else
config.server.port, else service.port. Takes (dict "root" $ "component" $c).
*/}}
{{- define "openndx.containerPort" -}}
{{- $v := index .root.Values .component.values }}
{{- .component.port | default (dig "server" "port" $v.service.port ($v.config | default dict)) }}
{{- end }}

{{/*
Fails, listing every missing required value at once, when an enabled service's
config can't be completed (see openndx.config). Renders nothing otherwise.
validate.yaml runs it, and openndx.config runs it before failing on its own.
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
URL that is left empty filled in from global.idpPublicURL or an ingress, each
URL of another chart service that is left out filled in from the release's
Service names (openndx.serviceURLInto), and the values that have no default
checked. configmap.yaml renders it,
deployment.yaml hashes it, and the Consent Portal's env reads it:
  include "openndx.config" (dict "root" $ "values" "consentEngine") | fromYaml
A value it can't fill in is added to "errs" ({list: [...]}) when one is passed;
without one, it fails through openndx.validate.
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
{{- /* pdp.url "" switches off authorization and consent, so the Consent Engine is only needed while it isn't. */}}
{{- $pdp := $cfg.pdp | default dict }}
{{- $authz := or (not (hasKey $pdp "url")) (not (empty $pdp.url)) }}
{{- $_ := set $cfg "pdp" (include "openndx.serviceURLInto" (dict "root" $root "block" $pdp "service" "policyDecisionPoint" "name" "pdp" "for" "orchestrationEngine.config.pdp.url" "disabled" "set it to an external PDP, or to \"\" to switch off authorization and consent" "errs" $errs) | fromYaml) }}
{{- if $authz }}
{{- $_ := set $cfg "consentEngine" (include "openndx.serviceURLInto" (dict "root" $root "block" ($cfg.consentEngine | default dict) "service" "consentEngine" "name" "ce" "path" "/internal/api/v1" "for" "orchestrationEngine.config.consentEngine.url" "disabled" "set it to an external Consent Engine's /internal/api/v1, or set orchestrationEngine.config.pdp.url to \"\" to switch off authorization and consent" "errs" $errs) | fromYaml) }}
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
{{- $pdp := $cfg.pdp | default dict }}
{{- if and (hasKey $pdp "url") (empty $pdp.url) }}
{{- $_ := set $errs "list" (append $errs.list "portalBackend.config.pdp.url: must not be empty, since the Portal Backend requires a PDP; leave it out to use the chart's PDP, or set an external PDP's URL") }}
{{- end }}
{{- $_ := set $cfg "pdp" (include "openndx.serviceURLInto" (dict "root" $root "block" $pdp "service" "policyDecisionPoint" "name" "pdp" "for" "portalBackend.config.pdp.url" "disabled" "the Portal Backend requires a PDP: enable policyDecisionPoint, or set it to an external PDP's URL" "errs" $errs) | fromYaml) }}
{{- end }}
{{- if and (not .errs) $errs.list }}
{{- /* Report every missing value, not only this service's. */}}
{{- include "openndx.validate" $root }}
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
A config block ("block", e.g. the OE's pdp) as YAML, with "url" set to the
in-cluster URL of the chart's own Service for "service" (its values key; "name"
is its component key, "path" an optional suffix) when "url" is left out. A url
that is set, "" included, is kept. When "url" is left out and the service is
disabled, adds "for" and the "disabled" hint to "errs".
*/}}
{{- define "openndx.serviceURLInto" -}}
{{- $block := deepCopy .block }}
{{- if not (hasKey $block "url") }}
{{- $v := index .root.Values .service }}
{{- if dig "enabled" true $v }}
{{- $_ := set $block "url" (printf "http://%s-%s:%v%s" (include "openndx.fullname" .root) .name $v.service.port (.path | default "")) }}
{{- else }}
{{- $_ := set .errs "list" (append .errs.list (printf "%s: %s is disabled, so the chart can't fill it in; %s" .for .service .disabled)) }}
{{- end }}
{{- end }}
{{- toYaml $block }}
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
