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
  {{- with .Values.consentPortal.config }}
  env:
    - {name: CONSENT_ENGINE_URL, value: {{ .consentEngineURL | quote }}}
    - {name: IDP_CLIENT_ID, value: {{ .idpClientId | quote }}}
    - {name: IDP_BASE_URL, value: {{ .idpBaseURL | quote }}}
    - {name: IDP_SCOPE, value: {{ .idpScope | quote }}}
    - {name: IDP_SIGN_IN_REDIRECT_URL, value: {{ .idpSignInRedirectURL | quote }}}
    - {name: IDP_SIGN_OUT_REDIRECT_URL, value: {{ .idpSignOutRedirectURL | quote }}}
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
