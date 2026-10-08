package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

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

	if upToDate, err := checkUpgradeTarget(current, latest, force); err != nil {
		return err
	} else if upToDate {
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
	installed, err := installer.Install(ctx, latest)
	if err != nil {
		return err
	}
	var statePath string
	if home := selfupdate.HomeDir(); home != "" {
		statePath = selfupdate.StatePath(home)
	}
	if err := selfupdate.AfterUpgrade(ctx, out, client, statePath, current, installed, latest, time.Now()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: could not save update state: %v\n", err)
	}
	return nil
}

// checkUpgradeTarget decides whether to install latest over current: not when
// current is already up to date, unless force. It never installs an older
// version, which --force would otherwise do to a pre-release build.
func checkUpgradeTarget(current string, latest *selfupdate.Release, force bool) (upToDate bool, err error) {
	if _, err := selfupdate.ParseVersion(latest.TagName); err != nil {
		return false, fmt.Errorf("the latest release's tag %q isn't a version: %w", latest.TagName, err)
	}
	if force && selfupdate.IsNewer(latest.TagName, current) {
		err := fmt.Errorf("the latest release, %s, is older than this version (%s)", latest.Version(), current)
		if v, perr := selfupdate.ParseVersion(current); perr == nil && v.Pre != "" {
			return false, fmt.Errorf("%w; add --prerelease to include pre-releases", err)
		}
		// This release was withdrawn; going back to an older one is a deliberate reinstall.
		return false, fmt.Errorf("%w; to go back to it, download it from %s", err, latest.HTMLURL)
	}
	return !force && !selfupdate.IsNewer(current, latest.TagName), nil
}
