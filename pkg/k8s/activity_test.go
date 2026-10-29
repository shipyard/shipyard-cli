package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
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
