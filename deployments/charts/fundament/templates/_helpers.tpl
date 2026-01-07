{{/*
Common labels
*/}}
{{- define "fundament.labels" -}}
app.kubernetes.io/part-of: {{ $.Release.Name }}
app.kubernetes.io/managed-by: {{ $.Release.Service }}
{{- end }}

{{/*
Database name
*/}}
{{- define "fundament.db.name" -}}
{{ $.Release.Name }}-db
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
JWT Secret - used for signing and validating tokens across services
*/}}
{{- define "fundament.jwtSecret" -}}
{{- required "jwtSecret is required" .Values.jwtSecret -}}
{{- end }}

{{/*
Ingress controller service for internal access
*/}}
{{- define "fundament.ingressService" -}}
ingress-nginx-controller.ingress-nginx.svc.cluster.local
{{- end }}
