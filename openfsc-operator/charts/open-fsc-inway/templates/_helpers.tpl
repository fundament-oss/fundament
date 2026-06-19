{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "open-fsc-inway.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "open-fsc-inway.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "open-fsc-inway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "open-fsc-inway.labels" -}}
helm.sh/chart: {{ include "open-fsc-inway.chart" . }}
{{ include "open-fsc-inway.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "open-fsc-inway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "open-fsc-inway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Create the name of the service account to use
*/}}
{{- define "open-fsc-inway.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
    {{ default (include "open-fsc-inway.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Return the image name for the OpenFSC inway
*/}}
{{- define "open-fsc-inway.image" -}}
{{- $registryName := default .Values.image.registry .Values.global.imageRegistry -}}
{{- $repositoryName := .Values.image.repository -}}
{{- $tag := default (printf "v%s" .Chart.AppVersion) (default .Values.image.tag .Values.global.imageTag) -}}

{{- printf "%s/%s:%s" $registryName $repositoryName $tag -}}
{{- end -}}

{{/*
Return the self address of the inway
*/}}
{{- define "open-fsc-inway.selfAddress" -}}
{{- if .Values.config.selfAddress -}}
  {{- .Values.config.selfAddress -}}
{{- else }}
  {{- printf "%s:%d" (include "open-fsc-inway.fullname" .) (.Values.service.port | int) -}}
{{- end -}}
{{- end -}}

{{/*
Return the image pull secrets for the inway
*/}}
{{- define "open-fsc-inway.imagePullSecrets" -}}
  {{- $imagePullSecrets := default .Values.image.pullSecrets .Values.global.imagePullSecrets -}}
  {{- toYaml $imagePullSecrets | nindent 8 }}
{{- end -}}
