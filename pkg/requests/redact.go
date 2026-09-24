package requests

import (
	"encoding/json"
	"regexp"
)

var (
	bypassTokenRE = regexp.MustCompile(`(?i)("bypass_token"\s*:\s*")[^"]*(")`)
	kubeCADataRE  = regexp.MustCompile(`(?i)(certificate-authority-data:\s*)\S+`)
	kubeTokenRE   = regexp.MustCompile(`(?i)((?:^|\n)\s*token:\s*)\S+`)
)

// RedactAPIResponse masks secrets commonly present in public API payloads.
// Set includeSecrets to keep bypass tokens and kubeconfig credentials.
func RedactAPIResponse(body []byte, includeSecrets bool) []byte {
	if includeSecrets || len(body) == 0 {
		return body
	}

	out := bypassTokenRE.ReplaceAll(body, []byte(`${1}[REDACTED]${2}`))
	out = kubeCADataRE.ReplaceAll(out, []byte(`${1}[REDACTED]`))
	out = kubeTokenRE.ReplaceAll(out, []byte(`${1}[REDACTED]`))

	// Also redact JSON string fields named token when this looks like kubeconfig JSON.
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err == nil {
		redactJSONSecrets(obj)
		if b, err := json.Marshal(obj); err == nil {
			return b
		}
	}
	return out
}

func redactJSONSecrets(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			lk := k
			switch {
			case lk == "bypass_token", lk == "certificate-authority-data", lk == "token":
				t[k] = "[REDACTED]"
			default:
				redactJSONSecrets(child)
			}
		}
	case []any:
		for _, child := range t {
			redactJSONSecrets(child)
		}
	}
}
