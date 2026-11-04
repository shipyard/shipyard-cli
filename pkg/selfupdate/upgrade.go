package selfupdate

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/fatih/color"
)

// AfterUpgrade reports an upgrade from current to installed, where latest is
// the release that was asked for, prints the notes for the versions in
// between, and records them as seen in the state at statePath (skipped when
// statePath is empty). installed differs from latest when Homebrew's formula
// is behind the GitHub release.
func AfterUpgrade(ctx context.Context, w io.Writer, c *Client, statePath, current, installed string, latest *Release, now time.Time) error {
	_, _ = color.New(color.FgHiGreen).Fprintf(w, "✓ Upgraded to %s\n", installed)
	if installed != latest.Version() {
		_, _ = fmt.Fprintf(w, "Homebrew doesn't have %s yet; run 'shipyard upgrade' again later to get it.\n", latest.Version())
	}
	_, _ = fmt.Fprintln(w)

	notes, err := c.Between(ctx, current, installed)
	if (err != nil || len(notes) == 0) && installed == latest.Version() {
		// --force on the same version, or GitHub unreachable after the download.
		notes = []Release{*latest}
	}
	RenderNotes(w, notes, DefaultNotesLines)

	if statePath == "" {
		return nil
	}
	st := LoadState(statePath)
	if len(notes) > 0 {
		// The notes were just shown; don't show them again on the next run.
		st.LastSeenVersion = installed
	}
	st.LatestVersion = latest.Version()
	st.LastChecked = now
	return st.Save(statePath)
}
