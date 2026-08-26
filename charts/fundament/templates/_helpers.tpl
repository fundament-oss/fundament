{{/*
Selector labels for a component.
Usage: include "fundament.selectorLabels" (dict "root" $ "name" "authn-api")
*/}}
{{- define "fundament.selectorLabels" -}}
app.kubernetes.io/name: {{ .name }}
app.kubernetes.io/instance: {{ .root.Release.Name }}-{{ .name }}
{{- end }}

{{/*
Common labels applied to all resources.
Usage: include "fundament.labels" (dict "root" $ "name" "authn-api" "component" "backend")
*/}}
{{- define "fundament.labels" -}}
{{ include "fundament.selectorLabels" . }}
app.kubernetes.io/part-of: fundament
app.kubernetes.io/component: {{ .component }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
helm.sh/chart: {{ .root.Chart.Name }}-{{ .root.Chart.Version | replace "+" "_" }}
{{- end }}

{{/*
Database name
*/}}
{{- define "fundament.db.name" -}}
db
{{- end }}

{{/*
Database host (CNPG read-write service)
*/}}
{{- define "fundament.db.host" -}}
{{ include "fundament.db.name" . }}-rw
{{- end }}

{{/*
Database secret name (app user)
*/}}
{{- define "fundament.db.secretName" -}}
{{ include "fundament.db.name" . }}-app
{{- end }}

{{/*
Database superuser secret name
*/}}
{{- define "fundament.db.superuserSecretName" -}}
{{ include "fundament.db.name" . }}-superuser
{{- end }}

{{/*
JWT Secret environment variable - supports both direct value and secretRef
*/}}
{{- define "fundament.jwtSecretEnv" -}}
- name: JWT_SECRET
{{- if and .Values.jwtSecretRef .Values.jwtSecretRef.name .Values.jwtSecretRef.key }}
  valueFrom:
    secretKeyRef:
      name: {{ .Values.jwtSecretRef.name }}
      key: {{ .Values.jwtSecretRef.key }}
{{- else if .Values.jwtSecret }}
  value: {{ .Values.jwtSecret }}
{{- else }}
  {{- fail "Either jwtSecret or jwtSecretRef (with name and key) must be set" }}
{{- end }}
{{- end }}

{{/*
Subdomain infix for ingress hostnames (e.g., "pr123." for PR environments)
Inserted between service name and domain: service.pr123.domain
*/}}
{{- define "fundament.ingress.infix" -}}
{{ $.Values.ingress.subdomainInfix | default "" }}
{{- end }}

{{/*
Ingress controller service for internal access
*/}}
{{- define "fundament.ingressService" -}}
ingress-nginx-controller.ingress-nginx.svc.cluster.local
{{- end }}

{{/*
livenessProbe rendered with Air-friendly cadence in hotreload mode and
production-tight cadence otherwise. In hotreload mode, failureThreshold *
periodSeconds gives ~5 minutes for any `go build` (initial *or* mid-life
rebuild after a source change) to finish before kubelet kills the pod —
a startupProbe wouldn't help here because it only covers the first start.

Usage: {{- include "fundament.livenessProbe" (dict "root" $ "port" "http") | nindent 10 }}
*/}}
{{- define "fundament.livenessProbe" -}}
livenessProbe:
  httpGet:
    path: /livez
    port: {{ .port }}
  initialDelaySeconds: 5
{{- if .root.Values.hotreload.enabled }}
  periodSeconds: 5
  failureThreshold: 60
{{- else }}
  periodSeconds: 10
{{- end }}
{{- end }}

{{/*
readinessProbe rendered with the same hotreload tolerance as liveness so
the pod isn't yanked out of Service endpoints mid-rebuild either.

Usage: {{- include "fundament.readinessProbe" (dict "root" $ "port" "http") | nindent 10 }}
*/}}
{{- define "fundament.readinessProbe" -}}
readinessProbe:
  httpGet:
    path: /readyz
    port: {{ .port }}
  initialDelaySeconds: 5
  periodSeconds: 5
{{- if .root.Values.hotreload.enabled }}
  failureThreshold: 60
{{- end }}
{{- end }}

{{/*
Name of the OpenFGA store. Consumers resolve it to an id at runtime.

Constant across releases: a per-release name would exceed the 63-character label
cap on PR previews, where the release version is an image tag. Which release a
store belongs to is carried by fundament.openfga.generation instead.

Usage: include "fundament.openfga.storeName" $
*/}}
{{- define "fundament.openfga.storeName" -}}
fundament
{{- end }}

{{/*
Identifies the release whose datastore state is current. openfga's provision
sidecar publishes it, db-migrations waits for it, and the wipe runs when the
datastore carries a different one.

Only a resettable datastore has releases to tell apart, so elsewhere this is a
constant. A value that tracked Helm revisions would land in openfga's pod
template and roll the server on every reconcile, and hold the migration Job
behind that rollout.

deploymentVersion comes first because Release.Revision counts Helm operations
rather than builds: a drift correction or a retry bumps the revision and would
wipe a live environment mid-use. PR previews set deploymentVersion to the image
tag, which is unique per build.

Usage: include "fundament.openfga.generation" $
*/}}
{{- define "fundament.openfga.generation" -}}
{{- if .Values.db.reset -}}
{{ .Values.deploymentVersion | default .Release.Revision }}
{{- else -}}
stable
{{- end -}}
{{- end }}
