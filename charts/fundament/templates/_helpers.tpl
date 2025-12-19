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
Database secret name
*/}}
{{- define "fundament.db.secretName" -}}
{{ include "fundament.db.name" . }}-app
{{- end }}
