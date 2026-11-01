{{- define "mcp-host.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "mcp-host.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "mcp-host.labels" -}}
app.kubernetes.io/name: {{ include "mcp-host.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Values.image.tag | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "mcp-host.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mcp-host.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* The image: by digest too when one is given, so a tag pushed again can't change what runs. */}}
{{- define "mcp-host.image" -}}
{{- $tag := required "image.tag is required: the CLI release the image was built from" .Values.image.tag -}}
{{- with .Values.image.digest -}}
{{- if not (regexMatch "^sha256:[0-9a-f]{64}$" .) -}}
{{- fail (printf "image.digest must be sha256:<64 hex digits>, got %q" .) -}}
{{- end -}}
{{- printf "%s:%s@%s" $.Values.image.repository $tag . -}}
{{- else -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}
{{- end -}}

{{/* The resource URL, checked: https, a host, and no query or fragment. */}}
{{- define "mcp-host.resourceUrl" -}}
{{- $raw := required "mcp.resourceUrl is required: this server's MCP URL, e.g. https://mcp.shipyard.build/mcp" .Values.mcp.resourceUrl -}}
{{- $u := urlParse $raw -}}
{{- if ne $u.scheme "https" -}}
{{- fail (printf "mcp.resourceUrl must be https, got %q" $raw) -}}
{{- end -}}
{{- if or (eq $u.host "") (contains ":" $u.host) -}}
{{- fail (printf "mcp.resourceUrl needs a host without a port, got %q" $raw) -}}
{{- end -}}
{{- if or $u.query $u.fragment -}}
{{- fail (printf "mcp.resourceUrl can't have a query or fragment, got %q" $raw) -}}
{{- end -}}
{{- $raw -}}
{{- end -}}

{{/* Shipyard's URL, checked: the server sends its client secret and users' tokens there. */}}
{{- define "mcp-host.siteUrl" -}}
{{- $raw := required "mcp.siteUrl is required" .Values.mcp.siteUrl -}}
{{- $u := urlParse $raw -}}
{{- if ne $u.scheme "https" -}}
{{- fail (printf "mcp.siteUrl must be https, got %q" $raw) -}}
{{- end -}}
{{- if eq $u.host "" -}}
{{- fail (printf "mcp.siteUrl needs a host, got %q" $raw) -}}
{{- end -}}
{{- $raw -}}
{{- end -}}

{{- define "mcp-host.host" -}}
{{- (urlParse (include "mcp-host.resourceUrl" .)).host -}}
{{- end -}}

{{- define "mcp-host.secretName" -}}
{{- if .Values.mcp.existingSecret -}}
{{- .Values.mcp.existingSecret -}}
{{- else -}}
{{- include "mcp-host.fullname" . -}}
{{- end -}}
{{- end -}}

{{- define "mcp-host.secretKey" -}}
{{- if .Values.mcp.existingSecret -}}
{{- required "mcp.existingSecretKey is required with mcp.existingSecret: the key that holds the client secret" .Values.mcp.existingSecretKey -}}
{{- else -}}
SHIPYARD_MCP_CLIENT_SECRET
{{- end -}}
{{- end -}}
