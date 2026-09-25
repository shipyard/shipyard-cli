package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"k8s.io/client-go/util/homedir"

	"github.com/shipyard/shipyard-cli/pkg/selfupdate"
	"github.com/shipyard/shipyard-cli/version"
)

func NewUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"update"},
		Short:   "Upgrade shipyard CLI to the latest version",
		Long: `Check GitHub for the latest release and install it if it is newer, then show
its release notes.

A Homebrew install is upgraded with 'brew upgrade shipyard'. Any other install
downloads the release binary, verifies it against the release's checksums.txt,
and replaces the running binary.`,
		Example: `  # Upgrade to the latest stable release
  shipyard upgrade

  # Include pre-releases
  shipyard upgrade --prerelease

  # Reinstall the latest release even if it's the running version
  shipyard upgrade --force`,
		RunE: runUpgrade,
	}

	cmd.Flags().BoolP("force", "f", false, "Reinstall even if already on the latest version")
	cmd.Flags().BoolP("prerelease", "p", false, "Include prerelease versions")

	return cmd
}

func runUpgrade(cmd *cobra.Command, _ []string) error {
	force, _ := cmd.Flags().GetBool("force")
	includePrerelease, _ := cmd.Flags().GetBool("prerelease")

	green := color.New(color.FgHiGreen)
	blue := color.New(color.FgHiBlue)
	out := cmd.OutOrStdout()

	current := version.Version
	if !selfupdate.IsRelease(current) {
		return fmt.Errorf("this is a development build (version %q); install a release to upgrade it", current)
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	client := selfupdate.NewClient(15 * time.Second)

	_, _ = blue.Fprintln(out, "Checking for updates...")
	latest, err := client.Latest(ctx, includePrerelease)
	if err != nil {
		return fmt.Errorf("failed to fetch latest release: %w", err)
	}
	_, _ = blue.Fprintf(out, "Current version: %s\n", current)
	_, _ = blue.Fprintf(out, "Latest version:  %s\n", latest.Version())

	if !force && !selfupdate.IsNewer(current, latest.TagName) {
		_, _ = green.Fprintln(out, "✓ You're already running the latest version!")
		return nil
	}

	installer, err := selfupdate.NewInstaller(out, current)
	if err != nil {
		return err
	}
	if installer.Method == selfupdate.MethodHomebrew && force && !selfupdate.IsNewer(current, latest.TagName) {
		return errors.New("installed with Homebrew: run 'brew reinstall shipyard' to reinstall")
	}
	if err := installer.Install(ctx, latest); err != nil {
		return err
	}
	_, _ = green.Fprintf(out, "✓ Upgraded to %s\n\n", latest.Version())

	notes, err := client.Between(ctx, current, latest.Version())
	if err != nil || len(notes) == 0 {
		// --force on the same version, or GitHub unreachable after the download.
		notes = []selfupdate.Release{*latest}
	}
	selfupdate.RenderNotes(out, notes, selfupdate.DefaultNotesLines)

	// The notes were just shown; don't show them again on the next run.
	if home := homedir.HomeDir(); home != "" {
		path := selfupdate.StatePath(home)
		st := selfupdate.LoadState(path)
		st.LastSeenVersion = latest.Version()
		st.LatestVersion = latest.Version()
		st.LastChecked = time.Now()
		if err := st.Save(path); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: could not save update state: %v\n", err)
		}
	}
	return nil
}
