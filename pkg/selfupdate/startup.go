package selfupdate

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fatih/color"
)

const (
	// CheckInterval is how often GitHub is asked for a new release.
	CheckInterval = 24 * time.Hour
	// checkRetry is how soon a failed check is retried, so being offline
	// doesn't add a network timeout to every command.
	checkRetry = time.Hour
	// checkTimeout bounds the one synchronous request made at startup.
	checkTimeout = 2 * time.Second
	// notesTimeout bounds fetching release notes after an upgrade.
	notesTimeout = 5 * time.Second
)

// Startup runs before a command in an interactive terminal: it shows the notes
// for a version the user upgraded to since the last run, checks for a new
// release at most once per CheckInterval, and offers to install it.
type Startup struct {
	Current   string
	StatePath string
	Client    *Client
	In        io.Reader
	Out       io.Writer
	Now       func() time.Time
	// Install upgrades to rel; it prints its own progress to Out.
	Install func(ctx context.Context, rel *Release) error
}

// Run returns true when it installed a new version, in which case the caller
// should re-run the command on the new binary.
func (s *Startup) Run(ctx context.Context) bool {
	st := LoadState(s.StatePath)
	loaded := st
	// The prompt can wait a long time for an answer. Merge onto what's on disk
	// then, so an answer given meanwhile in another shell isn't overwritten.
	defer func() { _ = LoadState(s.StatePath).merge(loaded, st).Save(s.StatePath) }()

	s.showNotesSinceLastRun(ctx, &st)

	now := s.Now()
	if now.Sub(st.LastChecked) >= CheckInterval {
		cctx, cancel := context.WithTimeout(ctx, checkTimeout)
		rel, err := s.Client.Latest(cctx, false)
		cancel()
		if err != nil {
			st.LastChecked = now.Add(checkRetry - CheckInterval)
		} else {
			st.LastChecked = now
			st.LatestVersion = rel.Version()
		}
	}

	latest := st.LatestVersion
	if !IsNewer(s.Current, latest) || latest == st.SkippedVersion || now.Before(st.SnoozedUntil) {
		return false
	}

	yellow := color.New(color.FgHiYellow)
	_, _ = fmt.Fprintln(s.Out)
	_, _ = yellow.Fprintf(s.Out, "A new version of shipyard is available: %s → %s\n", s.Current, latest)
	_, _ = fmt.Fprint(s.Out, "Upgrade now? [Y/n/s] (s = skip this version) ")

	switch readAnswer(s.In) {
	case "n", "no":
		st.SnoozedUntil = now.Add(CheckInterval)
		_, _ = fmt.Fprintln(s.Out, "OK. Run `shipyard upgrade` whenever you're ready.")
		_, _ = fmt.Fprintln(s.Out)
		return false
	case "s", "skip":
		st.SkippedVersion = latest
		_, _ = fmt.Fprintf(s.Out, "Skipping %s. You'll be asked again when a newer version is out.\n\n", latest)
		return false
	case "", "y", "yes":
	default:
		st.SnoozedUntil = now.Add(CheckInterval)
		_, _ = fmt.Fprintln(s.Out, "Not upgrading. Run `shipyard upgrade` whenever you're ready.")
		_, _ = fmt.Fprintln(s.Out)
		return false
	}

	_, _ = fmt.Fprintln(s.Out)
	rel, err := s.Client.ByTag(ctx, "v"+latest)
	if err == nil {
		err = s.Install(ctx, rel)
	}
	if err != nil {
		st.SnoozedUntil = now.Add(CheckInterval)
		_, _ = color.New(color.FgHiRed).Fprintf(s.Out, "Upgrade failed: %v\n", err)
		_, _ = fmt.Fprintln(s.Out, "Continuing with the current version. Run `shipyard upgrade` to try again.")
		_, _ = fmt.Fprintln(s.Out)
		return false
	}

	_, _ = color.New(color.FgHiGreen).Fprintf(s.Out, "✓ Upgraded to %s\n\n", latest)
	s.renderBetween(ctx, s.Current, latest)
	st.LastSeenVersion = latest
	return true
}

// showNotesSinceLastRun covers upgrades made outside the CLI, such as
// `brew upgrade` or the install script: the first run of a newer version
// prints the notes the user hasn't seen.
func (s *Startup) showNotesSinceLastRun(ctx context.Context, st *State) {
	if st.LastSeenVersion == "" {
		// A fresh install: nothing was missed, and its own notes would only
		// repeat what the user just chose to install.
		st.LastSeenVersion = s.Current
		return
	}
	if !IsNewer(st.LastSeenVersion, s.Current) {
		return
	}
	_, _ = color.New(color.FgHiGreen).Fprintf(s.Out, "shipyard was upgraded to %s\n\n", s.Current)
	s.renderBetween(ctx, st.LastSeenVersion, s.Current)
	st.LastSeenVersion = s.Current
}

func (s *Startup) renderBetween(ctx context.Context, from, to string) {
	nctx, cancel := context.WithTimeout(ctx, notesTimeout)
	defer cancel()
	releases, err := s.Client.Between(nctx, from, to)
	if err != nil || len(releases) == 0 {
		_, _ = fmt.Fprintf(s.Out, "Release notes: https://github.com/%s/%s/releases/tag/v%s\n\n", repoOwner, repoName, to)
		return
	}
	RenderNotes(s.Out, releases, DefaultNotesLines)
	_, _ = fmt.Fprintln(s.Out)
}

// readAnswer reads one line. Enter alone means yes, but end of input (Ctrl-D)
// means no: closing the prompt shouldn't install anything.
func readAnswer(in io.Reader) string {
	line, err := bufio.NewReader(in).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if err != nil && answer == "" {
		return "n"
	}
	return answer
}
