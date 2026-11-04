package k8s

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shipyard/shipyard-cli/pkg/client"
)

type activityCall struct {
	method string
	uri    string
	body   any
}

// activityRequester records every request on a channel so a test can wait for
// ticks instead of sleeping.
type activityRequester struct {
	calls chan activityCall
	err   error
}

func (r *activityRequester) Do(method, uri, contentType string, body any) ([]byte, error) {
	r.calls <- activityCall{method: method, uri: uri, body: body}
	return nil, r.err
}

func newActivityClient(r *activityRequester, org string) client.Client {
	return client.Client{Requester: r, OrgLookupFn: func() string { return org }}
}

func waitForCall(t *testing.T, calls <-chan activityCall) activityCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a heartbeat")
		return activityCall{}
	}
}

func TestActivityHeartbeatSendsActivity(t *testing.T) {
	r := &activityRequester{calls: make(chan activityCall, 10)}
	stop := StartActivityHeartbeat(context.Background(), newActivityClient(r, "org-abc"), "env-123", ActivitySourcePortForward, 10*time.Millisecond)
	defer stop()

	call := waitForCall(t, r.calls)

	if call.method != "POST" {
		t.Errorf("expected method POST, got %s", call.method)
	}

	u, err := url.Parse(call.uri)
	if err != nil {
		t.Fatalf("invalid URI %q: %v", call.uri, err)
	}
	if !strings.HasSuffix(u.Path, "/environment/env-123/activity") {
		t.Errorf("unexpected path %s", u.Path)
	}
	if got := u.Query().Get("org"); got != "org-abc" {
		t.Errorf("expected org param org-abc, got %q", got)
	}

	body, err := json.Marshal(call.body)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"source":"port-forward"}`; string(body) != want {
		t.Errorf("expected body %s, got %s", want, body)
	}
}

func TestActivityHeartbeatOmitsEmptyOrg(t *testing.T) {
	r := &activityRequester{calls: make(chan activityCall, 10)}
	stop := StartActivityHeartbeat(context.Background(), newActivityClient(r, ""), "env-123", ActivitySourceExec, 10*time.Millisecond)
	defer stop()

	call := waitForCall(t, r.calls)
	if strings.Contains(call.uri, "org=") {
		t.Errorf("expected no org param, got %s", call.uri)
	}
}

// A failed heartbeat must not end the session, or stop the ones after it.
func TestActivityHeartbeatKeepsGoingAfterErrors(t *testing.T) {
	r := &activityRequester{calls: make(chan activityCall, 10), err: errors.New("environment not found")}
	stop := StartActivityHeartbeat(context.Background(), newActivityClient(r, ""), "env-123", ActivitySourceLogs, 10*time.Millisecond)
	defer stop()

	waitForCall(t, r.calls)
	waitForCall(t, r.calls)
}

// The first heartbeat waits a full interval: the kubeconfig fetch already
// recorded the start of the session.
func TestActivityHeartbeatWaitsForFirstInterval(t *testing.T) {
	r := &activityRequester{calls: make(chan activityCall, 10)}
	stop := StartActivityHeartbeat(context.Background(), newActivityClient(r, ""), "env-123", ActivitySourceExec, time.Hour)
	stop()

	if n := len(r.calls); n != 0 {
		t.Errorf("expected no heartbeat before the first interval, got %d", n)
	}
}

func TestActivityHeartbeatStops(t *testing.T) {
	tests := []struct {
		name string
		halt func(stop func(), cancel context.CancelFunc)
	}{
		{name: "stop", halt: func(stop func(), _ context.CancelFunc) { stop() }},
		{name: "context cancel", halt: func(stop func(), cancel context.CancelFunc) {
			cancel()
			// stop waits for the goroutine to exit, and is safe to call again.
			stop()
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &activityRequester{calls: make(chan activityCall, 100)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			stop := StartActivityHeartbeat(ctx, newActivityClient(r, ""), "env-123", ActivitySourceExec, 5*time.Millisecond)
			waitForCall(t, r.calls)

			tt.halt(stop, cancel)
			stop()

			// Drain anything sent before the halt, then make sure nothing follows.
			for len(r.calls) > 0 {
				<-r.calls
			}
			select {
			case <-r.calls:
				t.Fatal("heartbeat continued after being stopped")
			case <-time.After(50 * time.Millisecond):
			}
		})
	}
}

// Cancelling the parent context alone must end the heartbeat, without stop.
func TestActivityHeartbeatStopsOnParentCancel(t *testing.T) {
	r := &activityRequester{calls: make(chan activityCall, 100)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := StartActivityHeartbeat(ctx, newActivityClient(r, ""), "env-123", ActivitySourceExec, 5*time.Millisecond)
	defer stop()
	waitForCall(t, r.calls)

	cancel()
	waitForSilence(t, r.calls)
}

// waitForSilence fails unless calls goes quiet: a report already under way may
// still land, but none may follow it.
func waitForSilence(t *testing.T, calls <-chan activityCall) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-calls:
		case <-time.After(50 * time.Millisecond):
			return
		case <-deadline:
			t.Fatal("heartbeat kept going")
		}
	}
}

// blockingRequester holds every request until released, like a stalled API.
type blockingRequester struct {
	entered chan struct{}
	release chan struct{}
}

func (r *blockingRequester) Do(method, uri, contentType string, body any) ([]byte, error) {
	select {
	case r.entered <- struct{}{}:
	default:
	}
	<-r.release
	return nil, nil
}

// A request can't be cancelled, so a stalled one must not hold up the exit.
func TestActivityHeartbeatStopDoesNotWaitForStalledRequest(t *testing.T) {
	r := &blockingRequester{entered: make(chan struct{}, 1), release: make(chan struct{})}
	defer close(r.release)
	c := client.Client{Requester: r, OrgLookupFn: func() string { return "" }}

	stop := StartActivityHeartbeat(context.Background(), c, "env-123", ActivitySourceExec, time.Millisecond)
	<-r.entered

	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(stopWait + 2*time.Second):
		t.Fatal("stop blocked on a stalled request")
	}
}

// An exec session with no input must not keep the environment alive.
func TestInputHeartbeatSkipsIntervalsWithoutInput(t *testing.T) {
	r := &activityRequester{calls: make(chan activityCall, 10)}
	var input InputActivity
	stop := StartInputHeartbeat(context.Background(), newActivityClient(r, ""), "env-123", ActivitySourceExec, 5*time.Millisecond, &input)
	defer stop()

	select {
	case <-r.calls:
		t.Fatal("heartbeat sent for an interval with no input")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestInputHeartbeatReportsInputOnce(t *testing.T) {
	r := &activityRequester{calls: make(chan activityCall, 10)}
	var input InputActivity
	stop := StartInputHeartbeat(context.Background(), newActivityClient(r, ""), "env-123", ActivitySourceExec, 5*time.Millisecond, &input)
	defer stop()

	if _, err := io.ReadAll(input.Reader(strings.NewReader("ls\n"))); err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(waitForCall(t, r.calls).body)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"source":"exec"}`; string(body) != want {
		t.Errorf("expected body %s, got %s", want, body)
	}

	select {
	case <-r.calls:
		t.Fatal("one input was reported more than once")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestInputActivityIgnoresEmptyReads(t *testing.T) {
	var input InputActivity
	if _, err := io.ReadAll(input.Reader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	if input.seen.Load() {
		t.Error("a read with no data counted as input")
	}
}

// failingRequester records every request and fails those script marks true.
// Requests past the end of script succeed.
type failingRequester struct {
	activityRequester
	script []bool
	sent   atomic.Int32
}

func (r *failingRequester) Do(method, uri, contentType string, body any) ([]byte, error) {
	r.calls <- activityCall{method: method, uri: uri, body: body}
	i := int(r.sent.Add(1)) - 1
	if i < len(r.script) && r.script[i] {
		return nil, errors.New("\x1b]0;service unavailable")
	}
	return nil, nil
}

// A failed report must not lose the input it was meant to report: the next
// interval tries again, with no new input.
func TestInputHeartbeatRetriesAfterFailedReport(t *testing.T) {
	r := &failingRequester{activityRequester: activityRequester{calls: make(chan activityCall, 10)}, script: []bool{true}}
	c := client.Client{Requester: r, OrgLookupFn: func() string { return "" }}

	var input InputActivity
	stop := StartInputHeartbeat(context.Background(), c, "env-123", ActivitySourceExec, 5*time.Millisecond, &input)
	defer stop()

	if _, err := io.ReadAll(input.Reader(strings.NewReader("ls\n"))); err != nil {
		t.Fatal(err)
	}

	waitForCall(t, r.calls)
	waitForCall(t, r.calls)
}

func TestActivityHeartbeatEndsAtMaxSession(t *testing.T) {
	r := &activityRequester{calls: make(chan activityCall, 100)}
	h := newHeartbeat(newActivityClient(r, ""), "env-123", ActivitySourcePortForward, 5*time.Millisecond)
	h.maxSession = 30 * time.Millisecond

	stop := h.start(context.Background())
	defer stop()

	waitForCall(t, r.calls)
	waitForSilence(t, r.calls)
}

// syncBuffer lets a test read what the heartbeat goroutine wrote.
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

func runFailingHeartbeat(t *testing.T, script []bool, reports int) string {
	t.Helper()
	r := &failingRequester{activityRequester: activityRequester{calls: make(chan activityCall, 100)}, script: script}
	c := client.Client{Requester: r, OrgLookupFn: func() string { return "" }}

	var warnings syncBuffer
	h := newHeartbeat(c, "env-123", ActivitySourceLogs, time.Millisecond)
	h.warnings = &warnings

	stop := h.start(context.Background())
	for range reports {
		waitForCall(t, r.calls)
	}
	stop()
	return warnings.String()
}

func TestActivityHeartbeatWarnsOnceAfterRepeatedFailures(t *testing.T) {
	out := runFailingHeartbeat(t, []bool{true, true, true, true, true, true}, 10)

	if n := strings.Count(out, "Warning:"); n != 1 {
		t.Fatalf("expected one warning, got %d: %q", n, out)
	}
	if !strings.Contains(out, "env-123") {
		t.Errorf("expected the warning to name the environment, got %q", out)
	}
	if !strings.HasPrefix(out, "\r\n") || !strings.HasSuffix(out, "\r\n") {
		t.Errorf("expected \\r\\n line endings for a raw terminal, got %q", out)
	}
	// The error text comes from the server, and this goes to a live terminal.
	if strings.Contains(out, "service unavailable") || strings.Contains(out, "\x1b") {
		t.Errorf("expected the warning to leave out the server's error, got %q", out)
	}
}

func TestActivityHeartbeatDoesNotWarnBelowThreshold(t *testing.T) {
	if out := runFailingHeartbeat(t, []bool{true, true}, 10); out != "" {
		t.Errorf("expected no warning, got %q", out)
	}
}

// Only failures in a row count: a success in between starts the count again.
func TestActivityHeartbeatDoesNotWarnOnIntermittentFailures(t *testing.T) {
	script := []bool{true, true, false, true, true, false, true, true}
	if out := runFailingHeartbeat(t, script, 10); out != "" {
		t.Errorf("expected no warning, got %q", out)
	}
}
