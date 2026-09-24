package requests

import (
	"bytes"
	"encoding/json"
	"regexp"
)

const redacted = "[REDACTED]"

var (
	bypassTokenRE = regexp.MustCompile(`(?i)("bypass_token"\s*:\s*")[^"]*(")`)
	kubeCADataRE  = regexp.MustCompile(`(?i)(certificate-authority-data:\s*)\S+`)
	kubeTokenRE   = regexp.MustCompile(`(?i)((?:^|\n)\s*token:\s*)\S+`)
)

// RedactAPIResponse masks secrets commonly present in public API payloads.
// Set includeSecrets to keep bypass tokens and kubeconfig credentials.
//
// JSON bodies are re-encoded only when a secret was found that the regexes
// could not mask in place, so secret-free responses come back byte-for-byte.
func RedactAPIResponse(body []byte, includeSecrets bool) []byte {
	if includeSecrets || len(body) == 0 {
		return body
	}

	out := bypassTokenRE.ReplaceAll(body, []byte(`${1}`+redacted+`${2}`))
	out = kubeCADataRE.ReplaceAll(out, []byte(`${1}`+redacted))
	out = kubeTokenRE.ReplaceAll(out, []byte(`${1}`+redacted))

	if !json.Valid(out) {
		return out
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil || !redactJSONSecrets(v) {
		return out
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return out
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// redactJSONSecrets masks secret-named fields in place and reports whether it
// changed anything.
func redactJSONSecrets(v any) bool {
	changed := false
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			switch k {
			case "bypass_token", "certificate-authority-data", "token":
				if child != redacted {
					t[k] = redacted
					changed = true
				}
			default:
				if redactJSONSecrets(child) {
					changed = true
				}
			}
		}
	case []any:
		for _, child := range t {
			if redactJSONSecrets(child) {
				changed = true
			}
		}
	}
	return changed
}
