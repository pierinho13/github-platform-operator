{{/*
Expand the name of the chart.
*/}}
{{- define "github-platform-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a fully qualified application name.
*/}}
{{- define "github-platform-operator.fullname" -}}
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
Chart name and version.
*/}}
{{- define "github-platform-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "github-platform-operator.labels" -}}
helm.sh/chart: {{ include "github-platform-operator.chart" . }}
{{ include "github-platform-operator.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "github-platform-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "github-platform-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: controller-manager
{{- end }}

{{/*
Service account name.
*/}}
{{- define "github-platform-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "github-platform-operator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- required "serviceAccount.name is required when serviceAccount.create is false" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Manager ClusterRole name.
*/}}
{{- define "github-platform-operator.managerRoleName" -}}
{{- printf "%s-manager" (include "github-platform-operator.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Leader election Role name.
*/}}
{{- define "github-platform-operator.leaderElectionRoleName" -}}
{{- printf "%s-leader-election" (include "github-platform-operator.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Metrics auth ClusterRole name.
*/}}
{{- define "github-platform-operator.metricsAuthRoleName" -}}
{{- printf "%s-metrics-auth" (include "github-platform-operator.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Metrics reader ClusterRole name.
*/}}
{{- define "github-platform-operator.metricsReaderRoleName" -}}
{{- printf "%s-metrics-reader" (include "github-platform-operator.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
