{{/*
Expand the name of the chart.
*/}}
{{- define "candela-sidecar.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "candela-sidecar.fullname" -}}
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
Common labels
*/}}
{{- define "candela-sidecar.labels" -}}
helm.sh/chart: {{ include "candela-sidecar.name" . }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/name: {{ include "candela-sidecar.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "candela-sidecar.selectorLabels" -}}
app.kubernetes.io/name: {{ include "candela-sidecar.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Build comma-separated provider names for PROVIDERS env var.
*/}}
{{- define "candela-sidecar.providerNames" -}}
{{- $names := list }}
{{- range .Values.providers }}
{{- $names = append $names .name }}
{{- end }}
{{- join "," $names }}
{{- end }}
