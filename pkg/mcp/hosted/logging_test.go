package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	mcperrors "github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/requests"
)

// syncBuffer is a log destination the test can read while the server runs.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureLog(t *testing.T) *syncBuffer {
	t.Helper()
	out := &syncBuffer{}
	previous := Logger.Writer()
	Logger.SetOutput(out)
	t.Cleanup(func() { Logger.SetOutput(previous) })
	return out
}

func wantLogLine(t *testing.T, log *syncBuffer, pattern string) {
	t.Helper()
	if !regexp.MustCompile(`(?m)` + pattern).MatchString(log.String()) {
		t.Errorf("no log line matches %q in:\n%s", pattern, log.String())
	}
}

func callGetEnvironments(t *testing.T, srv *httptest.Server) {
	t.Helper()
	call(t, srv, clientToken, "tools/call", map[string]any{"name": "get_environments", "arguments": map[string]any{"name": "private-app"}})
}

func TestASuccessfulToolCallLogsOneLine(t *testing.T) {
	core := newFakeCore(t)
	srv := newHost(t, core)
	log := captureLog(t)
	callGetEnvironments(t, srv)
	wantLogLine(t, log, `MCP hosted: get_environments ok in \d+(\.\d+)?(µs|ms|s) org="test-org"$`)
	if n := strings.Count(log.String(), "get_environments"); n != 1 {
		t.Errorf("%d lines for one call:\n%s", n, log.String())
	}
}

func TestAFailedToolCallLogsItsStatusButNotTheBody(t *testing.T) {
	core := newFakeCore(t)
	core.apiStatus = http.StatusNotFound
	srv := newHost(t, core)
	log := captureLog(t)
	callGetEnvironments(t, srv)
	wantLogLine(t, log, `MCP hosted: get_environments failed status=404 in \S+ org="test-org"$`)
	// The fake API's error body, and the call's own arguments.
	for _, private := range []string{"Invalid token", "private-app"} {
		if strings.Contains(log.String(), private) {
			t.Errorf("the log holds %q:\n%s", private, log.String())
		}
	}
}

func TestAFailureThatNeverReachedTheAPILogsNoStatus(t *testing.T) {
	core := newFakeCore(t)
	srv := newHost(t, core)
	log := captureLog(t)
	// A required argument is missing, so the tool stops before any API call.
	call(t, srv, clientToken, "tools/call", map[string]any{"name": "get_environment", "arguments": map[string]any{}})
	wantLogLine(t, log, `MCP hosted: get_environment failed status=none in \S+ org="test-org"$`)
}

func TestAToolCallCoreCouldntAuthorizeLogsThatItDidntRun(t *testing.T) {
	core := newFakeCore(t)
	core.exchange = func(w http.ResponseWriter, _ map[string]string) { w.WriteHeader(http.StatusBadGateway) }
	srv := newHost(t, core)
	log := captureLog(t)
	callGetEnvironments(t, srv)
	wantLogLine(t, log, `MCP hosted: get_environments unavailable org="unknown"$`)
}

func TestRequestsThatAreNotToolCallsLogTheirMethodOnlyIfUseful(t *testing.T) {
	core := newFakeCore(t)
	srv := newHost(t, core)
	log := captureLog(t)
	initialize(t, srv, clientToken)
	wantLogLine(t, log, `MCP hosted: initialize org="test-org"$`)

	before := log.String()
	call(t, srv, clientToken, "tools/list", map[string]any{})
	call(t, srv, clientToken, "ping", map[string]any{})
	if got := strings.TrimPrefix(log.String(), before); got != "" {
		t.Errorf("logged:\n%s", got)
	}
}

func TestAListenStreamIsLoggedWhenItOpens(t *testing.T) {
	core := newFakeCore(t)
	srv := newHost(t, core)
	log := captureLog(t)
	payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 9, "method": "subscriptions/listen", "params": map[string]any{
		"_meta": map[string]any{
			"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
			"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "test", "version": "1"},
		},
		"notifications": map[string]any{"toolsListChanged": true},
	}})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/mcp", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+clientToken)
	req.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	req.Header.Set("Mcp-Method", "subscriptions/listen")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	// The stream stays open, so the line has to come when it opens, not when it ends.
	eventually(t, "the listen line", func() bool { return strings.Contains(log.String(), `MCP hosted: subscriptions/listen org="test-org"`) })
}

func TestAToolCallTheAPIRefusedLogsItsOrg(t *testing.T) {
	core := newFakeCore(t)
	core.apiStatus = http.StatusUnauthorized
	srv := newHost(t, core)
	log := captureLog(t)
	callGetEnvironments(t, srv)
	wantLogLine(t, log, `MCP hosted: get_environments: the API refused the exchanged token in \S+ org="test-org"$`)
}

// Core sends the org from one release on, so an older core has to keep working.
func TestWithoutAnOrgFromCoreTheOrgIsUnknown(t *testing.T) {
	core := newFakeCore(t)
	core.exchange = func(w http.ResponseWriter, _ map[string]string) {
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":300,"scope":"envs:read"}`, coreToken)
	}
	srv := newHost(t, core)
	log := captureLog(t)
	callGetEnvironments(t, srv)
	wantLogLine(t, log, `MCP hosted: get_environments ok in \S+ org="unknown"$`)
}

func TestACachedTokenKeepsItsOrg(t *testing.T) {
	core := newFakeCore(t)
	srv := newHost(t, core)
	callGetEnvironments(t, srv)
	log := captureLog(t)
	callGetEnvironments(t, srv)
	if core.exchanges != 1 {
		t.Fatalf("%d exchanges, want the second call to use the cached token", core.exchanges)
	}
	wantLogLine(t, log, `MCP hosted: get_environments ok in \S+ org="test-org"$`)
}

// Core doesn't limit what an org name holds, and a line break would forge a log line.
func TestAnOrgNameCantBreakTheLogLine(t *testing.T) {
	core := newFakeCore(t)
	core.exchange = func(w http.ResponseWriter, _ map[string]string) {
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":300,"scope":"envs:read","org":"a b\nMCP hosted: forged"}`, coreToken)
	}
	srv := newHost(t, core)
	log := captureLog(t)
	callGetEnvironments(t, srv)
	if lines := strings.Count(strings.TrimSpace(log.String()), "\n") + 1; lines != 1 {
		t.Errorf("%d lines:\n%s", lines, log.String())
	}
	wantLogLine(t, log, `org="a b\\nMCP hosted: forged"$`)
}

// A client opens a long-lived stream only to hear about changes. Each request
// builds its own server, so there's never a change to send, and nginx cuts the
// idle stream at its read timeout.
func TestTheServerDoesNotPromiseToolListChanges(t *testing.T) {
	core := newFakeCore(t)
	srv := newHost(t, core)
	result, _ := initialize(t, srv, clientToken).body["result"].(map[string]any)
	capabilities, _ := result["capabilities"].(map[string]any)
	toolCaps, ok := capabilities["tools"].(map[string]any)
	if !ok {
		t.Fatalf("no tools capability: %v", capabilities)
	}
	if changed, _ := toolCaps["listChanged"].(bool); changed {
		t.Errorf("tools capability = %v", toolCaps)
	}
	if _, ok := capabilities["logging"]; ok {
		t.Errorf("advertises logging, which nothing here sends: %v", capabilities)
	}
}

func TestOnlyTheNamedMethodsAreLogged(t *testing.T) {
	log := captureLog(t)
	handle := logRequest("acme", nil)(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return nil, nil
	})
	for _, method := range []string{"initialize", "server/discover", "subscriptions/listen"} {
		if _, err := handle(t.Context(), method, nil); err != nil {
			t.Fatal(err)
		}
		wantLogLine(t, log, `MCP hosted: `+method+` org="acme"$`)
	}
	before := log.String()
	// A client names its own method, so an unknown one isn't written to the log.
	for _, method := range []string{"made/up-method", "resources/list", "tools/list", "ping"} {
		if _, err := handle(t.Context(), method, nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.TrimPrefix(log.String(), before); got != "" {
		t.Errorf("logged:\n%s", got)
	}
}

func TestACallOutsideTheTokensScopesLogsThatItWasRefused(t *testing.T) {
	core := newFakeCore(t)
	core.exchange = func(w http.ResponseWriter, _ map[string]string) {
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":300,"scope":"envs:read","org":"test-org"}`, coreToken)
	}
	srv := newHost(t, core)
	log := captureLog(t)
	call(t, srv, clientToken, "tools/call", map[string]any{"name": "create_application", "arguments": map[string]any{}})
	wantLogLine(t, log, `MCP hosted: create_application not offered org="test-org"$`)
}

func TestACallToNoToolLogsWithoutTheClientsName(t *testing.T) {
	core := newFakeCore(t)
	srv := newHost(t, core)
	log := captureLog(t)
	call(t, srv, clientToken, "tools/call", map[string]any{"name": "client-named-this", "arguments": map[string]any{}})
	wantLogLine(t, log, `MCP hosted: unknown tool org="test-org"$`)
	if strings.Contains(log.String(), "client-named-this") {
		t.Errorf("the log holds the client's name:\n%s", log.String())
	}
}

// Core can answer a create or update with what to do next, and nothing changed.
func TestAnApplicationCallCoreAnsweredWithAnActionDoesntLogOK(t *testing.T) {
	core := newFakeCore(t)
	core.apiStatus = http.StatusConflict
	core.apiBody = `{"status":"needs_action","message":"private-detail"}`
	srv := newHost(t, core)
	log := captureLog(t)
	call(t, srv, clientToken, "tools/call", map[string]any{"name": "update_application", "arguments": map[string]any{
		"application_id": "11111111-1111-1111-1111-111111111111",
		"projects":       []map[string]any{{"repo": "app", "branch": "main"}},
	}})
	wantLogLine(t, log, `MCP hosted: update_application action in \S+ org="test-org"$`)
	if strings.Contains(log.String(), "private-detail") {
		t.Errorf("the log holds the body:\n%s", log.String())
	}
}

func TestCallOutcome(t *testing.T) {
	const action = `{"status":"needs_action"}`
	tests := []struct{ name, out, want string }{
		{"plan_application", action, "action"},
		{"create_application", action, "action"},
		{"update_application", action, "action"},
		{"update_application", `{"status":"updated"}`, "ok"},
		// Only the application tools answer this way: an environment has its own status.
		{"get_environment", `{"status":"creating"}`, "ok"},
	}
	for _, tt := range tests {
		if got := callOutcome(tt.name, tt.out); got != tt.want {
			t.Errorf("callOutcome(%s, %s) = %s, want %s", tt.name, tt.out, got, tt.want)
		}
	}
}

func TestAnUpdateOfAMissingAppLogsTheAPIsStatus(t *testing.T) {
	core := newFakeCore(t)
	core.apiStatus = http.StatusNotFound
	srv := newHost(t, core)
	log := captureLog(t)
	call(t, srv, clientToken, "tools/call", map[string]any{"name": "update_application", "arguments": map[string]any{
		"application_id": "11111111-1111-1111-1111-111111111111",
		"projects":       []map[string]any{{"repo": "app", "branch": "main"}},
	}})
	wantLogLine(t, log, `MCP hosted: update_application failed status=404 in \S+ org="test-org"$`)
}

func TestTheLoggedStatusIsTheAPIsNotOneGuessedFromItsWords(t *testing.T) {
	core := newFakeCore(t)
	core.apiStatus = http.StatusBadRequest
	core.apiBody = `{"errors":[{"title":"Branch not found"}]}`
	srv := newHost(t, core)
	log := captureLog(t)
	callGetEnvironments(t, srv)
	wantLogLine(t, log, `MCP hosted: get_environments failed status=400 in \S+ org="test-org"$`)
}

func TestARefusedTokenIsSeenWhateverTheAPISays(t *testing.T) {
	core := newFakeCore(t)
	core.apiStatus = http.StatusUnauthorized
	core.apiBody = `{"errors":[{"title":"Unauthorized"}]}`
	srv := newHost(t, core)
	log := captureLog(t)
	callGetEnvironments(t, srv)
	wantLogLine(t, log, `MCP hosted: get_environments: the API refused the exchanged token in \S+ org="test-org"$`)
}

func TestFailureStatus(t *testing.T) {
	apiErr := requests.NewAPIError(http.StatusBadRequest, []byte(`{"errors":[{"title":"Branch not found"}]}`))
	// The address of a request that never got an answer can hold any digits.
	network := errors.New(`error sending API request: Get "https://example.test/environment/a404b": EOF`)
	tests := map[string]struct {
		err  error
		want string
	}{
		"an API error":             {apiErr, "400"},
		"a converted API error":    {mcperrors.ParseHTTPError("get_environment", apiErr, ""), "400"},
		"no answer":                {network, "none"},
		"no answer, converted":     {mcperrors.ParseHTTPError("get_environment", network, ""), "none"},
		"a guess with no response": {&mcperrors.MCPError{StatusCode: http.StatusForbidden}, "none"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := failureStatus(tt.err); got != tt.want {
				t.Errorf("failureStatus = %s, want %s", got, tt.want)
			}
		})
	}
}
