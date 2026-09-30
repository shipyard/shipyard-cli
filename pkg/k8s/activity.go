package k8s

import (
	"context"
	"log"
	"net/http"
	"sync"
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
// for having no recent visit.
const ActivityHeartbeatInterval = 5 * time.Minute

// StartActivityHeartbeat reports activity on an environment every interval
// until ctx is done or stop is called. The first report goes out after one
// interval: fetching the kubeconfig already counts as the start of the session.
//
// A failed report is logged and skipped. It must never end the session.
func StartActivityHeartbeat(ctx context.Context, c client.Client, envID, source string, interval time.Duration) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := sendActivity(c, envID, source); err != nil {
					log.Printf("Failed to report %s activity for environment %s: %v", source, envID, err)
				}
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
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
