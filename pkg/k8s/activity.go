package k8s

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
)

// Activity sources the backend accepts for a heartbeat.
const (
	ActivitySourceExec        = "exec"
	ActivitySourceLogs        = "logs"
	ActivitySourcePortForward = "port-forward"
)

// ActivityHeartbeatInterval is how often a long-running session tells the
// backend the environment is still in use, so it is not stopped mid-session
// for having no recent visit. It is short so that one failed report can't end a
// session under a short timeout. The backend records one audit event per five
// minutes however often it hears from a session.
const ActivityHeartbeatInterval = 2 * time.Minute

// activityMaxSession is how long one session can keep reporting activity. Past
// it the session keeps running, but the environment can be stopped as idle
// again. The backend has no limit of its own, so without this a forgotten session
// or a stalled connection would hold an environment for as long as the process lives.
const activityMaxSession = 12 * time.Hour

// activityFailuresBeforeWarning is how many reports in a row must fail before
// the user is told, once, that the environment may be stopped.
const activityFailuresBeforeWarning = 3

// stopWait is how long stop waits for a report already in flight.
const stopWait = time.Second

// InputActivity records whether a session has had input since the last report.
// An exec session counts as activity only while someone is using it, as in the
// web terminal, so a forgotten shell does not keep an environment running.
type InputActivity struct {
	seen atomic.Bool
}

// Reader returns r, noting each read that returns data.
func (a *InputActivity) Reader(r io.Reader) io.Reader {
	return inputReader{r: r, activity: a}
}

type inputReader struct {
	r        io.Reader
	activity *InputActivity
}

func (r inputReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		r.activity.seen.Store(true)
	}
	return n, err
}

// StartActivityHeartbeat reports activity on an environment every interval
// until ctx is done, stop is called or activityMaxSession passes. The first report
// goes out after one interval: fetching the kubeconfig already counts as the
// start of the session.
//
// A failed report must never end the session. It is logged, and repeated failures
// warn the user once. A report can't be cancelled, so stop waits for one in flight
// only briefly: the session is over by then, and it must not be held up by a
// stalled API.
func StartActivityHeartbeat(ctx context.Context, c client.Client, envID, source string, interval time.Duration) (stop func()) {
	return newHeartbeat(c, envID, source, interval).start(ctx)
}

// StartInputHeartbeat is StartActivityHeartbeat for a session that only counts
// as activity while it has input: it skips an interval that had none.
func StartInputHeartbeat(ctx context.Context, c client.Client, envID, source string, interval time.Duration, input *InputActivity) (stop func()) {
	h := newHeartbeat(c, envID, source, interval)
	h.due = func() bool { return input.seen.Swap(false) }
	h.failed = func() { input.seen.Store(true) }
	return h.start(ctx)
}

type heartbeat struct {
	client     client.Client
	envID      string
	source     string
	interval   time.Duration
	maxSession time.Duration
	warnings   io.Writer
	// due says whether this interval has anything to report.
	due func() bool
	// failed runs after a report fails, so the next interval can try again.
	failed func()
}

func newHeartbeat(c client.Client, envID, source string, interval time.Duration) heartbeat {
	return heartbeat{
		client:     c,
		envID:      envID,
		source:     source,
		interval:   interval,
		maxSession: activityMaxSession,
		warnings:   os.Stderr,
		due:        func() bool { return true },
		failed:     func() {},
	}
}

func (h heartbeat) start(ctx context.Context) (stop func()) {
	ctx, cancel := context.WithTimeout(ctx, h.maxSession)
	done := make(chan struct{})

	go func() {
		defer close(done)

		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()

		failures := 0
		warned := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !h.due() {
					continue
				}
				err := sendActivity(h.client, h.envID, h.source)
				if err == nil {
					failures = 0
					continue
				}
				log.Printf("Failed to report %s activity for environment %s: %v", h.source, h.envID, err)
				h.failed()
				if failures++; failures >= activityFailuresBeforeWarning && !warned {
					warned = true
					// \r\n: during exec the terminal is in raw mode. The error stays in
					// the log: its text comes from the server, and this is a live terminal.
					_, _ = fmt.Fprintf(h.warnings, "\r\nWarning: could not report activity for environment %s, so it may be stopped as idle\r\n", h.envID)
				}
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(stopWait):
			}
		})
	}
}

// sendActivity records one heartbeat for the environment.
func sendActivity(c client.Client, envID, source string) error {
	params := make(map[string]string)
	if org := c.OrgLookupFn(); org != "" {
		params["org"] = org
	}

	requestURI := uri.CreateResourceURI("", "environment", envID, "activity", params)
	body := map[string]string{"source": source}
	_, err := c.Requester.Do(http.MethodPost, requestURI, "application/json", body)
	return err
}
