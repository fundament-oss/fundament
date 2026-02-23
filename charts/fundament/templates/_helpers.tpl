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
Ingress controller service for internal access
*/}}
{{- define "fundament.ingressService" -}}
ingress-nginx-controller.ingress-nginx.svc.cluster.local
{{- end }}
