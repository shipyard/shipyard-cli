package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/selfupdate"
	"github.com/shipyard/shipyard-cli/version"
)

// updateStatePath is where the CLI's update check keeps its state,
// ~/.shipyard/update-state.json, or "" when the CLI's opt-outs
// (SHIPYARD_NO_UPDATE_CHECK, CI, update_check: false) turn the check off.
func updateStatePath() string {
	if selfupdate.ChecksOff(viper.GetBool("update_check")) {
		return ""
	}
	home := selfupdate.HomeDir()
	if home == "" {
		return ""
	}
	return selfupdate.StatePath(home)
}

// cachedLatestVersion is the newest release the CLI's update check last saw.
// It never goes to the network: a client is waiting on initialize. The value
// is as fresh as the last terminal command or RefreshLatestVersion.
func cachedLatestVersion() string {
	path := updateStatePath()
	if path == "" {
		return ""
	}
	return selfupdate.LoadState(path).LatestVersion
}

// RefreshLatestVersion runs the CLI's update check in the background, so the
// next session's notice is current. Without it, someone who only uses the CLI
// through an assistant never gets one: the terminal check skips agents and
// never runs under `mcp serve`. It writes only the state file, never stdout,
// which carries the JSON-RPC stream. The check is due at most daily; if the
// process exits first, the check recorded the attempt and retries an hour later.
func RefreshLatestVersion(ctx context.Context) {
	refreshLatestVersion(ctx, selfupdate.NewClient(15*time.Second))
}

// refreshLatestVersion is RefreshLatestVersion with the GitHub client given.
// The returned channel closes when the check finishes, or at once when no
// check runs.
func refreshLatestVersion(ctx context.Context, client *selfupdate.Client) <-chan struct{} {
	done := make(chan struct{})
	path := updateStatePath()
	if path == "" || !selfupdate.IsRelease(version.Version) {
		close(done)
		return done
	}
	n := &selfupdate.Notifier{
		Current:    version.Version,
		StatePath:  path,
		Client:     client,
		Now:        time.Now,
		LatestOnly: true,
	}
	go func() {
		defer close(done)
		// main's recover doesn't cover this goroutine; a bug in the check
		// must not take down the server. stderr is safe: stdout is JSON-RPC.
		defer func() {
			if r := recover(); r != nil {
				log.Printf("update check failed: %v", r)
			}
		}()
		// The notice it returns is for a terminal; this session's agent got
		// its own in initialize. Not calling Shown leaves it for the terminal.
		n.Check(ctx)
	}()
	return done
}

// staleNotice goes before the server instructions when a newer release is
// known. The CLI's update notice goes to stderr after a command finishes, which
// `mcp serve` never does and clients don't show, so the agent is the only one
// who can pass it on.
//
// latest comes from a file and from GitHub and lands in the instructions the
// agent trusts most, so the notice carries only its parsed numbers. A cached
// pre-release (from `shipyard upgrade --prerelease`) is skipped: a plain
// `shipyard upgrade` wouldn't install it.
func staleNotice(current, latest string) string {
	l, err := selfupdate.ParseVersion(latest)
	if err != nil || l.Pre != "" || !selfupdate.IsRelease(current) || !selfupdate.IsNewer(current, l.String()) {
		return ""
	}
	return fmt.Sprintf("Shipyard CLI %s is out of date (latest %s): if a tool is missing or fails, "+
		"suggest `shipyard upgrade`.", current, l.String())
}
