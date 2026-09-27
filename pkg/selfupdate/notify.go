package selfupdate

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/fatih/color"
)

const (
	// CheckInterval is how often GitHub is asked for a new release, and how
	// often the "new version available" message is shown.
	CheckInterval = 24 * time.Hour
	// checkRetry is how soon a failed check is retried.
	checkRetry = time.Hour
	// requestTimeout bounds each request the background check makes.
	requestTimeout = 5 * time.Second
)

// Notifier works out what to tell the user after a command: the notes for a
// version they upgraded to since the last run, and whether a newer release is
// out. It never asks anything, so it can't block a script or an agent.
type Notifier struct {
	Current   string
	StatePath string
	Client    *Client
	Now       func() time.Time
}

// Notice is the text to print after the command. Call Shown once it has been
// printed, so the same notes and message aren't printed again.
type Notice struct {
	Text      string
	statePath string
	lastSeen  string
	notified  time.Time
}

// Check makes the network requests and returns what to print, which may be
// empty. It saves what it learns from GitHub straight away; what the user has
// seen is only recorded by Notice.Shown.
func (n *Notifier) Check(ctx context.Context) *Notice {
	st := LoadState(n.StatePath)
	now := n.Now()
	checkDue := elapsed(now, st.LastChecked, CheckInterval)
	notesDue := st.LastSeenVersion != "" && IsNewer(st.LastSeenVersion, n.Current) &&
		elapsed(now, st.NotesTried, checkRetry)
	if checkDue || notesDue {
		// Record the attempts as failed before asking GitHub. The command
		// usually exits before a slow request finishes, and without this every
		// run would ask again and wait for it.
		attempt := st
		if checkDue {
			attempt.LastChecked = now.Add(checkRetry - CheckInterval)
		}
		if notesDue {
			attempt.NotesTried = now
		}
		if err := LoadState(n.StatePath).merge(st, attempt).Save(n.StatePath); err != nil {
			// Nothing can be remembered (e.g. ~/.shipyard isn't writable), so
			// checking would ask GitHub and print the notice on every run.
			return &Notice{}
		}
		st = attempt
	}
	loaded := st
	notice := &Notice{statePath: n.StatePath}
	var buf bytes.Buffer

	if st.LastSeenVersion == "" {
		// A fresh install: nothing was missed, and its own notes would only
		// repeat what the user just chose to install.
		st.LastSeenVersion = n.Current
	} else if notesDue {
		// Upgraded since the last run, by `shipyard upgrade` in another
		// shell, `brew upgrade`, or the install script. A failed fetch is
		// retried at most hourly.
		rctx, cancel := context.WithTimeout(ctx, requestTimeout)
		releases, err := n.Client.Between(rctx, st.LastSeenVersion, n.Current)
		cancel()
		if err == nil {
			_, _ = color.New(color.FgHiGreen).Fprintf(&buf, "shipyard was upgraded to %s\n\n", n.Current)
			if len(releases) > 0 {
				RenderNotes(&buf, releases, DefaultNotesLines)
			} else {
				_, _ = fmt.Fprintf(&buf, "Release notes: %s\n", releaseURL(n.Current))
			}
			notice.lastSeen = n.Current
		}
	}

	if checkDue {
		rctx, cancel := context.WithTimeout(ctx, requestTimeout)
		rel, err := n.Client.Latest(rctx, false)
		cancel()
		if err == nil {
			st.LastChecked = now
			st.LatestVersion = rel.Version()
		}
	}

	if IsNewer(n.Current, st.LatestVersion) && elapsed(now, st.NotifiedAt, CheckInterval) {
		if buf.Len() > 0 {
			_, _ = fmt.Fprintln(&buf)
		}
		_, _ = color.New(color.FgHiYellow).Fprintf(&buf, "A new version of shipyard is available: %s → %s\n", n.Current, st.LatestVersion)
		_, _ = fmt.Fprintf(&buf, "Run `shipyard upgrade` to install it. Release notes: %s\n", releaseURL(st.LatestVersion))
		notice.notified = now
	}

	_ = LoadState(n.StatePath).merge(loaded, st).Save(n.StatePath)
	notice.Text = buf.String()
	return notice
}

// Shown records that the notice was printed.
func (nt *Notice) Shown() {
	if nt.lastSeen == "" && nt.notified.IsZero() {
		return
	}
	st := LoadState(nt.statePath)
	if nt.lastSeen != "" {
		st.LastSeenVersion = nt.lastSeen
	}
	if !nt.notified.IsZero() {
		st.NotifiedAt = nt.notified
	}
	_ = st.Save(nt.statePath)
}

// elapsed reports whether interval has passed since t. A t in the future,
// left by a clock that was wrong or has since been set back, counts as
// elapsed rather than holding everything off until the clock catches up.
func elapsed(now, t time.Time, interval time.Duration) bool {
	d := now.Sub(t)
	return d >= interval || d < 0
}

func releaseURL(version string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/tag/v%s", repoOwner, repoName, version)
}
