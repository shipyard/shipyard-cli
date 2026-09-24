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
