package selfupdate

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Method is how the running binary was installed, which decides how to upgrade it.
type Method int

const (
	// MethodDirect is a binary placed on disk by the install script, a
	// download from the releases page, or a previous self-upgrade.
	MethodDirect Method = iota
	// MethodHomebrew is a binary in Homebrew's Cellar. Overwriting it would
	// leave Homebrew recording the old version, so upgrades go through brew.
	MethodHomebrew
)

func (m Method) String() string {
	if m == MethodHomebrew {
		return "Homebrew"
	}
	return "direct download"
}

// Executable returns the running binary's path with symlinks resolved, which
// is where a direct install has to write the new binary.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// DetectMethod reports how the binary at the resolved path exe was installed.
func DetectMethod(exe string) Method {
	if strings.Contains(filepath.ToSlash(exe), "/Cellar/shipyard/") {
		return MethodHomebrew
	}
	return MethodDirect
}

// AssetName is the release file for this platform, as .goreleaser.yaml names it.
func AssetName(goos, goarch string) string {
	name := fmt.Sprintf("shipyard-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// Installer upgrades the running binary to a release.
type Installer struct {
	HTTP   *http.Client
	Out    io.Writer
	Exe    string
	Method Method
	// Current is the running version; a Homebrew upgrade that leaves it in
	// place is reported as a failure, not a success.
	Current string
	// Brew runs a brew subcommand; tests replace it.
	Brew func(ctx context.Context, out io.Writer, args ...string) error
	// BrewVersion returns the version of shipyard Homebrew has installed.
	BrewVersion func(ctx context.Context) (string, error)
}

// downloadTimeout bounds a whole binary download, so a stalled connection
// can't hang the command the user actually ran.
const downloadTimeout = 5 * time.Minute

// NewInstaller returns an installer for the running binary, at version current.
func NewInstaller(out io.Writer, current string) (*Installer, error) {
	exe, err := Executable()
	if err != nil {
		return nil, fmt.Errorf("could not find the running binary: %w", err)
	}
	return &Installer{
		HTTP:        &http.Client{Timeout: downloadTimeout},
		Out:         out,
		Exe:         exe,
		Method:      DetectMethod(exe),
		Current:     current,
		Brew:        runBrew,
		BrewVersion: brewVersion,
	}, nil
}

// Install replaces the running binary with the release's.
func (in *Installer) Install(ctx context.Context, rel *Release) error {
	if in.Method == MethodHomebrew {
		if rel.Prerelease {
			return errors.New("pre-releases aren't published to Homebrew; install this one from " + rel.HTMLURL)
		}
		// brew only refreshes taps once a day on its own, so a release from
		// this morning is invisible to `brew upgrade` without an update first.
		_, _ = fmt.Fprintln(in.Out, "Updating Homebrew...")
		if err := in.Brew(ctx, in.Out, "update", "--quiet"); err != nil {
			return fmt.Errorf("brew update failed: %w", err)
		}
		_, _ = fmt.Fprintln(in.Out, "Upgrading with Homebrew...")
		if err := in.Brew(ctx, in.Out, "upgrade", "shipyard"); err != nil {
			return fmt.Errorf("brew upgrade shipyard failed: %w", err)
		}
		// brew exits 0 when there's nothing to upgrade, which happens when
		// GitHub has the release before the formula in the tap is updated.
		installed, err := in.BrewVersion(ctx)
		if err != nil {
			return fmt.Errorf("could not read the version Homebrew installed: %w", err)
		}
		if !IsNewer(in.Current, installed) {
			return fmt.Errorf("the Homebrew formula doesn't have %s yet (it has %s); try again later", rel.Version(), installed)
		}
		return nil
	}
	return in.installDirect(ctx, rel)
}

func (in *Installer) installDirect(ctx context.Context, rel *Release) error {
	name := AssetName(runtime.GOOS, runtime.GOARCH)
	asset, ok := rel.Asset(name)
	if !ok {
		return fmt.Errorf("release %s has no %s binary", rel.TagName, name)
	}
	sums, ok := rel.Asset("checksums.txt")
	if !ok {
		return fmt.Errorf("release %s has no checksums.txt, so the download can't be verified", rel.TagName)
	}
	want, err := in.checksum(ctx, sums.BrowserDownloadURL, name)
	if err != nil {
		return err
	}

	// Download next to the binary so the final step is a rename within one
	// filesystem. Rewriting the running file in place fails on Linux ("text
	// file busy") and can get the next launch killed on macOS, whose kernel
	// caches the old code signature.
	dir := filepath.Dir(in.Exe)
	tmp, err := os.CreateTemp(dir, ".shipyard-upgrade-*")
	if err != nil {
		return permissionHint(fmt.Errorf("could not write to %s: %w", dir, err))
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	_, _ = fmt.Fprintf(in.Out, "Downloading %s...\n", name)
	got, err := in.download(ctx, asset.BrowserDownloadURL, tmp)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if got != want {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", name, want, got)
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	return permissionHint(replace(in.Exe, tmp.Name()))
}

// checksum finds name's SHA-256 in the release's checksums.txt.
func (in *Installer) checksum(ctx context.Context, url, name string) (string, error) {
	body, err := in.fetch(ctx, url)
	if err != nil {
		return "", fmt.Errorf("could not download checksums.txt: %w", err)
	}
	defer func() { _ = body.Close() }()
	sc := bufio.NewScanner(body)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && fields[1] == name {
			return strings.ToLower(fields[0]), nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("checksums.txt has no entry for %s", name)
}

// download writes url's body to w and returns its SHA-256.
func (in *Installer) download(ctx context.Context, url string, w io.Writer) (string, error) {
	body, err := in.fetch(ctx, url)
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(w, h), body); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (in *Installer) fetch(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := in.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("GET %s returned status %d", url, resp.StatusCode)
	}
	return resp.Body, nil
}

// replace moves the new binary over the old one.
func replace(exe, newPath string) error {
	if runtime.GOOS != "windows" {
		return os.Rename(newPath, exe)
	}
	// Windows can't replace a running executable, but it can rename one.
	// The .old file is removed on the next run by CleanupOld.
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(newPath, exe); err != nil {
		_ = os.Rename(old, exe)
		return err
	}
	return nil
}

// CleanupOld removes what an earlier upgrade left next to the binary: the
// binary a Windows upgrade renamed out of the way, and downloads abandoned by
// Ctrl-C, which skips the deferred removal. Downloads under an hour old may
// belong to an upgrade running in another shell, so they're kept.
func CleanupOld() {
	exe, err := Executable()
	if err != nil {
		return
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(exe + ".old")
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".shipyard-upgrade-*"))
	for _, m := range matches {
		if fi, err := os.Stat(m); err == nil && time.Since(fi.ModTime()) > time.Hour {
			_ = os.Remove(m)
		}
	}
}

func permissionHint(err error) error {
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		return err
	}
	if runtime.GOOS == "windows" {
		return fmt.Errorf("%w\nRun the terminal as Administrator and try again", err)
	}
	return fmt.Errorf("%w\nThe binary's directory isn't writable by you. Try: sudo shipyard upgrade", err)
}

// brewVersion parses `brew list --versions shipyard`, e.g. "shipyard 1.9.0",
// whose last field is the newest installed version.
func brewVersion(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "brew", "list", "--versions", "shipyard").Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return "", fmt.Errorf("unexpected output %q", strings.TrimSpace(string(out)))
	}
	return fields[len(fields)-1], nil
}

func runBrew(ctx context.Context, out io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "brew", args...)
	cmd.Stdout, cmd.Stderr = out, out
	return cmd.Run()
}
