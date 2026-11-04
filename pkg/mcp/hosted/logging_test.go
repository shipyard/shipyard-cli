package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
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
	wantLogLine(t, log, `MCP hosted: get_environments ok in \d+(\.\d+)?(µs|ms|s)$`)
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
	wantLogLine(t, log, `MCP hosted: get_environments failed status=404 in \d`)
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
	wantLogLine(t, log, `MCP hosted: get_environment failed status=none in \d`)
}

func TestAToolCallCoreCouldntAuthorizeLogsThatItDidntRun(t *testing.T) {
	core := newFakeCore(t)
	core.exchange = func(w http.ResponseWriter, _ map[string]string) { w.WriteHeader(http.StatusBadGateway) }
	srv := newHost(t, core)
	log := captureLog(t)
	callGetEnvironments(t, srv)
	wantLogLine(t, log, `MCP hosted: get_environments unavailable$`)
}

func TestRequestsThatAreNotToolCallsLogTheirMethodOnlyIfUseful(t *testing.T) {
	core := newFakeCore(t)
	srv := newHost(t, core)
	log := captureLog(t)
	initialize(t, srv, clientToken)
	wantLogLine(t, log, `MCP hosted: initialize$`)

	before := log.String()
	call(t, srv, clientToken, "tools/list", map[string]any{})
	call(t, srv, clientToken, "ping", map[string]any{})
	// A client names its own method, so an unknown one isn't written to the log.
	call(t, srv, clientToken, "made/up-method", map[string]any{})
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
	eventually(t, "the listen line", func() bool { return strings.Contains(log.String(), "MCP hosted: subscriptions/listen") })
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
