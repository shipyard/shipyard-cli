package requests_test

import (
	"strings"
	"testing"

	"github.com/shipyard/shipyard-cli/pkg/requests"
)

func TestRedactAPIResponse(t *testing.T) {
	in := []byte(`{"data":{"attributes":{"bypass_token":"secret-token","name":"env"}}}`)
	out := requests.RedactAPIResponse(in, false)
	if strings.Contains(string(out), "secret-token") {
		t.Fatalf("bypass_token not redacted: %s", out)
	}
	if !strings.Contains(string(out), "[REDACTED]") {
		t.Fatalf("expected redaction marker: %s", out)
	}

	kept := requests.RedactAPIResponse(in, true)
	if !strings.Contains(string(kept), "secret-token") {
		t.Fatalf("includeSecrets should keep token: %s", kept)
	}
}

func TestRedactAPIResponsePreservesSecretFreeBodies(t *testing.T) {
	in := []byte(`{"z":1, "id":9007199254740993, "url":"https://x?a=1&b=2"}`)
	out := requests.RedactAPIResponse(in, false)
	if string(out) != string(in) {
		t.Fatalf("secret-free body was rewritten:\n got %s\nwant %s", out, in)
	}
}

func TestRedactAPIResponseRedactsJSONTokenInArray(t *testing.T) {
	in := []byte(`[{"kubeconfig":{"users":[{"user":{"token":"kube-secret"}}]},"id":9007199254740993,"url":"https://x?a=1&b=2"}]`)
	out := string(requests.RedactAPIResponse(in, false))
	if strings.Contains(out, "kube-secret") {
		t.Fatalf("token not redacted: %s", out)
	}
	if !strings.Contains(out, "9007199254740993") || !strings.Contains(out, "a=1&b=2") {
		t.Fatalf("re-encoding lost precision or escaped HTML: %s", out)
	}
}
