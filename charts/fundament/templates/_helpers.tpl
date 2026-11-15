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
Pod topology spread for a Deployment with more than one replica: the pods go
to different nodes where the scheduler can place them. ScheduleAnyway keeps a
cluster with fewer nodes than replicas, or a node being drained, from leaving
pods Pending. pod-template-hash limits the count to the current ReplicaSet, so
a rolling update spreads the new pods too.
Renders at the pod spec's indentation, so call it without nindent.
Usage: {{- include "fundament.topologySpread" (dict "root" $ "name" "authn-api" "replicas" $.Values.authnApi.replicas) }}
*/}}
{{- define "fundament.topologySpread" -}}
{{- if gt (int .replicas) 1 }}
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: kubernetes.io/hostname
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels:
              {{- include "fundament.selectorLabels" . | nindent 14 }}
          matchLabelKeys:
            - pod-template-hash
{{- end }}
{{- end }}

{{/*
Keeps a stopping pod serving until Envoy has dropped it from its endpoints:
the endpoint removal and the stop signal start together, and a pod that exits
first turns the requests routed to it meanwhile into 503s. Rollouts, drains
and scale-downs all stop pods this way.
Usage: {{- include "fundament.preStop" . | nindent 10 }}
*/}}
{{- define "fundament.preStop" -}}
lifecycle:
  preStop:
    sleep:
      seconds: 5
{{- end }}

{{/*
PodDisruptionBudget for a Deployment with more than one replica: a node drain
evicts one pod at a time. A single replica gets none: with maxUnavailable 1 a
budget on one pod protects nothing. AlwaysAllow lets a drain evict a pod that
is not Ready, so a crash-looping pod does not hold up node maintenance.
Usage: include "fundament.podDisruptionBudget" (dict "root" $ "name" "authn-api" "component" "backend" "replicas" $.Values.authnApi.replicas)
*/}}
{{- define "fundament.podDisruptionBudget" -}}
{{- if gt (int .replicas) 1 }}
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ .name }}
  labels:
    {{- include "fundament.labels" . | nindent 4 }}
spec:
  maxUnavailable: 1
  unhealthyPodEvictionPolicy: AlwaysAllow
  selector:
    matchLabels:
      {{- include "fundament.selectorLabels" . | nindent 6 }}
{{- end }}
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
A service's connection pool as JSON: db.pool, with the service's own dbPool
over it. Rendering fails on a missing field, which pgx would refuse at start.

Usage: include "fundament.db.pool" (dict "root" $ "service" "clusterWorker") | fromJson
*/}}
{{- define "fundament.db.pool" -}}
{{- $own := (index .root.Values .service).dbPool | default dict -}}
{{- $pool := mergeOverwrite (deepCopy (.root.Values.db.pool | default dict)) (deepCopy $own) -}}
{{- if not (gt (int ($pool.maxConns | default 0)) 0) -}}
{{- fail (printf "db.pool.maxConns (or %s.dbPool.maxConns) must be at least 1" .service) -}}
{{- end -}}
{{- $_ := required (printf "db.pool.maxConnIdleTime (or %s.dbPool.maxConnIdleTime) is required" .service) $pool.maxConnIdleTime -}}
{{- toJson $pool -}}
{{- end }}

{{/*
A database role's URI. With a service, it carries that service's pool
settings; only the services read these URIs, through pgx, and psql and libpq
refuse pool_* parameters.

Usage: include "fundament.db.uri" (dict "root" $ "user" "fun_authn_api" "password" $p "service" "authnApi")
*/}}
{{- define "fundament.db.uri" -}}
postgresql://{{ .user }}:{{ .password }}@{{ include "fundament.db.host" .root }}:5432/fundament
{{- with .service -}}
{{- $pool := include "fundament.db.pool" (dict "root" $.root "service" .) | fromJson -}}
?pool_max_conns={{ int $pool.maxConns }}&pool_max_conn_idle_time={{ $pool.maxConnIdleTime }}
{{- end -}}
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
Name of the db-migrations Job for THIS release.

A Job's pod template is immutable, so the name carries the revision.

Usage: include "fundament.dbMigrationsJobName" $
*/}}
{{- define "fundament.dbMigrationsJobName" -}}
db-migrations-{{ .Release.Revision }}
{{- end }}

{{/*
Whether this release may destroy data: development.resetDatabasesInNamespace
must name the namespace being deployed to. A value naming a different namespace
is a values file that has travelled, so rendering fails rather than resetting
the wrong environment.

Usage: include "fundament.development.reset" $
*/}}
{{- define "fundament.development.reset" -}}
{{- $want := .Values.development.resetDatabasesInNamespace | default "" -}}
{{- if and $want (ne $want .Release.Namespace) -}}
{{- fail (printf "development.resetDatabasesInNamespace is %q but this release targets namespace %q. Destructive settings are bound to one namespace; remove the setting or correct it." $want .Release.Namespace) -}}
{{- end -}}
{{- if $want }}true{{ end }}
{{- end }}

{{/*
The OpenFGA store's name. Services resolve the id from it, because the id is
generated when the store is created.

Usage: include "fundament.openfga.storeName" $
*/}}
{{- define "fundament.openfga.storeName" -}}
fundament
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
livenessProbe rendered with Air-friendly cadence in hotreload mode and
production-tight cadence otherwise. In hotreload mode, failureThreshold *
periodSeconds gives ~5 minutes for any `go build` (initial *or* mid-life
rebuild after a source change) to finish before kubelet kills the pod —
a startupProbe wouldn't help here because it only covers the first start.

Usage: {{- include "fundament.livenessProbe" (dict "root" $ "port" "http") | nindent 10 }}
"path" overrides /livez, for the frontends, which serve no health endpoint.
*/}}
{{- define "fundament.livenessProbe" -}}
livenessProbe:
  httpGet:
    path: {{ .path | default "/livez" }}
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
"path" overrides /readyz, as for livenessProbe.
*/}}
{{- define "fundament.readinessProbe" -}}
readinessProbe:
  httpGet:
    path: {{ .path | default "/readyz" }}
    port: {{ .port }}
  initialDelaySeconds: 5
  periodSeconds: 5
  # Above the handlers' 5s dependency-ping budget; the kubelet default of 1s
  # would cancel a slow-but-successful ping and pull the pod from endpoints.
  timeoutSeconds: 6
{{- if .root.Values.hotreload.enabled }}
  failureThreshold: 60
{{- end }}
{{- end }}

{{/*
Readiness and liveness for a third-party server with nothing to ask over HTTP
that is independent of its dependencies: the port accepts connections.

Usage: {{- include "fundament.tcpProbes" (dict "port" "http") | nindent 10 }}
*/}}
{{- define "fundament.tcpProbes" -}}
readinessProbe:
  tcpSocket:
    port: {{ .port }}
  periodSeconds: 5
livenessProbe:
  tcpSocket:
    port: {{ .port }}
  initialDelaySeconds: 5
  periodSeconds: 10
{{- end }}

{{/*
CORS origins for a backend the browser calls with credentials: the operator's
externalUrls.corsAllowedOrigins when set, otherwise the frontends that call it
(those configured), so a frontend that gains a credentialed call to an API is
not CORS-blocked just because nobody listed its origin.
Usage: include "fundament.corsOrigins" (dict "root" $ "frontends" (list $.Values.externalUrls.console))
*/}}
{{- define "fundament.corsOrigins" -}}
{{- .root.Values.externalUrls.corsAllowedOrigins | default (compact .frontends | join ",") -}}
{{- end }}

{{/*
Where a frontend's test-environment banner links to: the same site on the PoC,
or "" when environmentBanner is off, which hides the banner.
Usage: include "fundament.pocUrl" (dict "root" $ "site" "console")
*/}}
{{- define "fundament.pocUrl" -}}
{{- if .root.Values.environmentBanner.enabled -}}
https://{{ .site }}.{{ .root.Values.environmentBanner.pocDomain }}
{{- end -}}
{{- end }}

{{/*
A container's standard resources block, indented to "indent"; renders nothing,
not even a blank line, when empty.

Usage: {{- include "fundament.resources" (dict "resources" $.Values.authnApi.resources "indent" 10) }}
*/}}
{{- define "fundament.resources" -}}
{{- with .resources }}
{{- printf "resources:\n%s" (toYaml . | indent 2) | nindent $.indent }}
{{- end }}
{{- end }}
