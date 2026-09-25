package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
)

func init() { color.NoColor = true }

// fakeGitHub serves the releases API and the release downloads.
type fakeGitHub struct {
	releases []Release
	files    map[string][]byte // download path -> body
	hits     map[string]int
}

func newFakeGitHub(t *testing.T, releases ...Release) (*fakeGitHub, *httptest.Server) {
	t.Helper()
	f := &fakeGitHub{releases: releases, files: map[string][]byte{}, hits: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits[r.URL.Path]++
		base := "/repos/shipyard/shipyard-cli/releases"
		switch {
		case r.URL.Path == base+"/latest":
			for _, rel := range f.releases {
				if !rel.Prerelease && !rel.Draft {
					_ = json.NewEncoder(w).Encode(rel)
					return
				}
			}
			http.NotFound(w, r)
		case strings.HasPrefix(r.URL.Path, base+"/tags/"):
			tag := strings.TrimPrefix(r.URL.Path, base+"/tags/")
			for _, rel := range f.releases {
				if rel.TagName == tag {
					_ = json.NewEncoder(w).Encode(rel)
					return
				}
			}
			http.NotFound(w, r)
		case r.URL.Path == base:
			_ = json.NewEncoder(w).Encode(f.releases)
		default:
			if b, ok := f.files[r.URL.Path]; ok {
				_, _ = w.Write(b)
				return
			}
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func testClient(srv *httptest.Server) *Client {
	return &Client{HTTP: srv.Client(), BaseURL: srv.URL}
}

func TestBetween(t *testing.T) {
	all := []Release{
		{TagName: "v1.8.2"}, // a patch published after 1.9.0
		{TagName: "v1.10.0-rc.1", Prerelease: true},
		{TagName: "v1.10.0"},
		{TagName: "v1.9.0"},
		{TagName: "v1.11.0", Draft: true},
		{TagName: "v1.8.1"},
		{TagName: "not-a-version"},
	}
	names := func(rs []Release) string {
		var s []string
		for _, r := range rs {
			s = append(s, r.TagName)
		}
		return strings.Join(s, ",")
	}
	if got := names(between(all, "1.8.1", "1.10.0")); got != "v1.10.0,v1.9.0,v1.8.2" {
		t.Errorf("1.8.1 -> 1.10.0: got %s", got)
	}
	if got := names(between(all, "1.9.0", "1.10.0-rc.1")); got != "v1.10.0-rc.1" {
		t.Errorf("1.9.0 -> 1.10.0-rc.1: got %s", got)
	}
	if got := names(between(all, "1.10.0", "1.10.0")); got != "" {
		t.Errorf("same version: got %s", got)
	}
}

func TestLatestPrereleasePicksHighestVersion(t *testing.T) {
	_, srv := newFakeGitHub(t,
		Release{TagName: "v1.9.1"}, // created after the rc, but lower
		Release{TagName: "v1.10.0-rc.1", Prerelease: true},
		Release{TagName: "v2.0.0", Draft: true},
	)
	rel, err := testClient(srv).Latest(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v1.10.0-rc.1" {
		t.Errorf("got %s", rel.TagName)
	}
}

func TestRenderNotes(t *testing.T) {
	var buf bytes.Buffer
	RenderNotes(&buf, []Release{{
		TagName: "v1.9.0",
		HTMLURL: "https://example.test/v1.9.0",
		Body:    "## `verify` **Highlights**\r\n\r\n\r\n- **`shipyard api`**: see [the guide](docs/mcp.md).\n  * nested item\n",
	}}, 0)
	want := "What's new in 1.9.0\n" +
		"verify Highlights\n" +
		"\n" +
		"  • shipyard api: see the guide.\n" +
		"    • nested item\n"
	if buf.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func TestRenderNotesTruncatesAndLinks(t *testing.T) {
	var buf bytes.Buffer
	RenderNotes(&buf, []Release{
		{TagName: "v1.10.0", HTMLURL: "https://example.test/v1.10.0", Body: "a\nb\nc\nd"},
		{TagName: "v1.9.0", HTMLURL: "https://example.test/v1.9.0"},
	}, 3)
	out := buf.String()
	if strings.Contains(out, "c\n") || !strings.Contains(out, "See the full release notes: https://example.test/v1.10.0") {
		t.Errorf("not truncated with a link:\n%s", out)
	}
}

func TestRenderNotesIssueReferenceIsNotAHeading(t *testing.T) {
	var buf bytes.Buffer
	RenderNotes(&buf, []Release{{TagName: "v1.9.0", Body: "#123 fixed\n### Real heading"}}, 0)
	want := "What's new in 1.9.0\n  #123 fixed\nReal heading\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

func TestRenderNotesEmptyBody(t *testing.T) {
	var buf bytes.Buffer
	RenderNotes(&buf, []Release{{TagName: "v1.9.0", HTMLURL: "https://example.test/v1.9.0"}}, 0)
	if !strings.Contains(buf.String(), "No release notes. See https://example.test/v1.9.0") {
		t.Errorf("got %q", buf.String())
	}
}

func TestDetectMethod(t *testing.T) {
	if DetectMethod("/opt/homebrew/Cellar/shipyard/1.8.1/bin/shipyard") != MethodHomebrew {
		t.Error("Cellar path should be Homebrew")
	}
	if DetectMethod("/home/linuxbrew/.linuxbrew/Cellar/shipyard/1.8.1/bin/shipyard") != MethodHomebrew {
		t.Error("Linuxbrew Cellar path should be Homebrew")
	}
	if DetectMethod("/usr/local/bin/shipyard") != MethodDirect {
		t.Error("/usr/local/bin should be direct")
	}
}

// releaseWithBinary publishes a release whose binary for this platform is body.
func releaseWithBinary(f *fakeGitHub, srv *httptest.Server, tag string, body []byte, sum string) *Release {
	name := AssetName(runtime.GOOS, runtime.GOARCH)
	if sum == "" {
		h := sha256.Sum256(body)
		sum = hex.EncodeToString(h[:])
	}
	f.files["/dl/"+tag+"/"+name] = body
	f.files["/dl/"+tag+"/checksums.txt"] = []byte(fmt.Sprintf("0000  shipyard-other-arch\n%s  %s\n", sum, name))
	return &Release{TagName: tag, Assets: []Asset{
		{Name: name, BrowserDownloadURL: srv.URL + "/dl/" + tag + "/" + name},
		{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/dl/" + tag + "/checksums.txt"},
	}}
}

func directInstaller(t *testing.T, srv *httptest.Server) (*Installer, string) {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "shipyard")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Installer{HTTP: srv.Client(), Out: io.Discard, Exe: exe, Method: MethodDirect}, exe
}

func TestInstallDirectReplacesBinary(t *testing.T) {
	f, srv := newFakeGitHub(t)
	rel := releaseWithBinary(f, srv, "v1.10.0", []byte("new binary"), "")
	in, exe := directInstaller(t, srv)

	if err := in.Install(context.Background(), rel); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "new binary" {
		t.Errorf("binary is %q", got)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(exe); fi.Mode().Perm() != 0o755 {
			t.Errorf("mode is %v", fi.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) != 1 {
		t.Errorf("left files behind: %v", entries)
	}
}

func TestInstallDirectRejectsChecksumMismatch(t *testing.T) {
	f, srv := newFakeGitHub(t)
	rel := releaseWithBinary(f, srv, "v1.10.0", []byte("tampered"), strings.Repeat("ab", 32))
	in, exe := directInstaller(t, srv)

	err := in.Install(context.Background(), rel)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("got %v, want checksum mismatch", err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "old binary" {
		t.Errorf("binary was replaced: %q", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) != 1 {
		t.Errorf("left files behind: %v", entries)
	}
}

func TestInstallDirectRequiresChecksums(t *testing.T) {
	f, srv := newFakeGitHub(t)
	rel := releaseWithBinary(f, srv, "v1.10.0", []byte("new"), "")
	rel.Assets = rel.Assets[:1]
	in, _ := directInstaller(t, srv)
	if err := in.Install(context.Background(), rel); err == nil || !strings.Contains(err.Error(), "checksums.txt") {
		t.Fatalf("got %v", err)
	}
}

func TestInstallDirectMissingPlatformAsset(t *testing.T) {
	_, srv := newFakeGitHub(t)
	in, _ := directInstaller(t, srv)
	err := in.Install(context.Background(), &Release{TagName: "v1.10.0"})
	if err == nil || !strings.Contains(err.Error(), "has no "+AssetName(runtime.GOOS, runtime.GOARCH)) {
		t.Fatalf("got %v", err)
	}
}

func homebrewInstaller(installed string, calls *[]string) *Installer {
	return &Installer{
		Out: io.Discard, Method: MethodHomebrew, Current: "1.9.0",
		Brew: func(_ context.Context, _ io.Writer, args ...string) error {
			*calls = append(*calls, strings.Join(args, " "))
			return nil
		},
		BrewVersion: func(context.Context) (string, error) { return installed, nil },
	}
}

func TestInstallHomebrew(t *testing.T) {
	var calls []string
	in := homebrewInstaller("1.10.0", &calls)
	if err := in.Install(context.Background(), &Release{TagName: "v1.10.0"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, "; ") != "update --quiet; upgrade shipyard" {
		t.Errorf("brew calls: %v", calls)
	}
	if err := in.Install(context.Background(), &Release{TagName: "v1.11.0-rc.1", Prerelease: true}); err == nil {
		t.Error("Homebrew pre-release install should fail")
	}
}

func TestInstallHomebrewNoOpIsAFailure(t *testing.T) {
	// The tap's formula lags the GitHub release: brew exits 0 with nothing done.
	var calls []string
	err := homebrewInstaller("1.9.0", &calls).Install(context.Background(), &Release{TagName: "v1.10.0"})
	if err == nil || !strings.Contains(err.Error(), "the Homebrew formula doesn't have 1.10.0 yet (it has 1.9.0)") {
		t.Fatalf("got %v", err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "update-state.json")
	if s := LoadState(path); s != (State{}) {
		t.Errorf("missing file: %+v", s)
	}
	want := State{LastChecked: time.Unix(1700000000, 0).UTC(), LatestVersion: "1.10.0", SkippedVersion: "1.9.5", LastSeenVersion: "1.9.0"}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	if got := LoadState(path); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if fi, _ := os.Stat(path); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o644 {
		t.Errorf("state file mode %v, want 0644 so a sudo-written file stays readable", fi.Mode().Perm())
	}
	_ = os.WriteFile(path, []byte("{not json"), 0o644)
	if s := LoadState(path); s != (State{}) {
		t.Errorf("corrupt file: %+v", s)
	}
}

// startupFixture runs Startup against a fake GitHub with 1.9.0 installed and
// 1.10.0 released.
type startupFixture struct {
	gh        *fakeGitHub
	s         *Startup
	out       bytes.Buffer
	installed []string
	now       time.Time
}

func newStartup(t *testing.T, answer string, st State) *startupFixture {
	t.Helper()
	gh, srv := newFakeGitHub(t,
		Release{TagName: "v1.10.0", Body: "- new in 1.10"},
		Release{TagName: "v1.9.0", Body: "- new in 1.9"},
	)
	fx := &startupFixture{gh: gh, now: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
	path := filepath.Join(t.TempDir(), "update-state.json")
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}
	fx.s = &Startup{
		Current:   "1.9.0",
		StatePath: path,
		Client:    testClient(srv),
		Ask:       AskFrom(strings.NewReader(answer)),
		Out:       &fx.out,
		Now:       func() time.Time { return fx.now },
		Install: func(_ context.Context, rel *Release) error {
			fx.installed = append(fx.installed, rel.TagName)
			return nil
		},
	}
	return fx
}

func (fx *startupFixture) state() State { return LoadState(fx.s.StatePath) }

func TestStartupYesInstallsAndShowsNotes(t *testing.T) {
	fx := newStartup(t, "\n", State{LastSeenVersion: "1.9.0"})
	if !fx.s.Run(context.Background()) {
		t.Fatal("Run returned false after installing")
	}
	if strings.Join(fx.installed, ",") != "v1.10.0" {
		t.Errorf("installed %v", fx.installed)
	}
	out := fx.out.String()
	for _, want := range []string{"1.9.0 → 1.10.0", "✓ Upgraded to 1.10.0", "What's new in 1.10.0", "new in 1.10"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "new in 1.9") {
		t.Errorf("showed notes the user already had:\n%s", out)
	}
	if st := fx.state(); st.LastSeenVersion != "1.10.0" || st.LatestVersion != "1.10.0" {
		t.Errorf("state %+v", st)
	}
}

func TestStartupNoSnoozesForADay(t *testing.T) {
	fx := newStartup(t, "n\n", State{LastSeenVersion: "1.9.0"})
	if fx.s.Run(context.Background()) {
		t.Fatal("installed after no")
	}
	if len(fx.installed) != 0 {
		t.Errorf("installed %v", fx.installed)
	}
	if st := fx.state(); !st.SnoozedUntil.Equal(fx.now.Add(CheckInterval)) {
		t.Errorf("snoozed until %v", st.SnoozedUntil)
	}

	// Within the day: no prompt, and no request to GitHub.
	fx.out.Reset()
	fx.now = fx.now.Add(23 * time.Hour)
	fx.gh.hits = map[string]int{}
	fx.s.Run(context.Background())
	if fx.out.Len() != 0 || len(fx.gh.hits) != 0 {
		t.Errorf("prompted or checked while snoozed: %q %v", fx.out.String(), fx.gh.hits)
	}

	// After it: asked again.
	fx.now = fx.now.Add(2 * time.Hour)
	fx.s.Ask = AskFrom(strings.NewReader("n\n"))
	fx.s.Run(context.Background())
	if !strings.Contains(fx.out.String(), "Upgrade now?") {
		t.Errorf("not asked again after the snooze:\n%s", fx.out.String())
	}
}

func TestStartupSkipIgnoresThatVersionOnly(t *testing.T) {
	fx := newStartup(t, "s\n", State{LastSeenVersion: "1.9.0"})
	fx.s.Run(context.Background())
	if st := fx.state(); st.SkippedVersion != "1.10.0" {
		t.Fatalf("state %+v", st)
	}

	fx.out.Reset()
	fx.now = fx.now.Add(48 * time.Hour)
	fx.s.Run(context.Background())
	if fx.out.Len() != 0 {
		t.Errorf("asked about a skipped version:\n%s", fx.out.String())
	}

	fx.gh.releases = append([]Release{{TagName: "v1.11.0"}}, fx.gh.releases...)
	fx.now = fx.now.Add(48 * time.Hour)
	fx.s.Ask = AskFrom(strings.NewReader("n\n"))
	fx.s.Run(context.Background())
	if !strings.Contains(fx.out.String(), "1.9.0 → 1.11.0") {
		t.Errorf("not asked about the next version:\n%s", fx.out.String())
	}
}

func TestStartupEOFDoesNotInstall(t *testing.T) {
	fx := newStartup(t, "", State{LastSeenVersion: "1.9.0"})
	if fx.s.Run(context.Background()) || len(fx.installed) != 0 {
		t.Error("installed on end of input")
	}
}

func TestStartupUnknownAnswerDoesNotInstall(t *testing.T) {
	fx := newStartup(t, "maybe\n", State{LastSeenVersion: "1.9.0"})
	if fx.s.Run(context.Background()) || len(fx.installed) != 0 {
		t.Error("installed on an unrecognized answer")
	}
}

func TestStartupNoAnswerContinuesWithoutInstalling(t *testing.T) {
	fx := newStartup(t, "", State{LastSeenVersion: "1.9.0"})
	var waited time.Duration
	fx.s.Ask = func(timeout time.Duration) (string, error) {
		waited = timeout
		return "", ErrNoAnswer
	}
	if fx.s.Run(context.Background()) || len(fx.installed) != 0 {
		t.Fatal("installed with no answer")
	}
	if waited != PromptTimeout {
		t.Errorf("asked with timeout %v, want %v", waited, PromptTimeout)
	}
	if out := fx.out.String(); !strings.Contains(out, "continuing in 10s") || !strings.Contains(out, "No answer, continuing") {
		t.Errorf("output:\n%s", out)
	}
	if st := fx.state(); !st.SnoozedUntil.Equal(fx.now.Add(CheckInterval)) {
		t.Errorf("not snoozed: %+v", st)
	}
}

func TestStartupInstallFailureContinues(t *testing.T) {
	fx := newStartup(t, "y\n", State{LastSeenVersion: "1.9.0"})
	fx.s.Install = func(context.Context, *Release) error { return errors.New("disk full") }
	if fx.s.Run(context.Background()) {
		t.Fatal("reported success after a failed install")
	}
	out := fx.out.String()
	if !strings.Contains(out, "Upgrade failed: disk full") || !strings.Contains(out, "shipyard upgrade") {
		t.Errorf("output:\n%s", out)
	}
	if st := fx.state(); st.LastSeenVersion != "1.9.0" || st.SnoozedUntil.IsZero() {
		t.Errorf("state %+v", st)
	}
}

func TestStartupKeepsAnswerFromAnotherShell(t *testing.T) {
	// This shell prompts; while it waits, another shell skips 1.10.0.
	fx := newStartup(t, "n\n", State{LastSeenVersion: "1.9.0"})
	fx.s.Ask = AskFrom(readerFunc(func(p []byte) (int, error) {
		other := LoadState(fx.s.StatePath)
		other.SkippedVersion = "1.10.0"
		if err := other.Save(fx.s.StatePath); err != nil {
			t.Fatal(err)
		}
		return copy(p, "n\n"), nil
	}))
	fx.s.Run(context.Background())
	st := fx.state()
	if st.SkippedVersion != "1.10.0" || st.SnoozedUntil.IsZero() {
		t.Errorf("lost one shell's answer: %+v", st)
	}
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

func TestStartupChecksAtMostDaily(t *testing.T) {
	fx := newStartup(t, "", State{LastSeenVersion: "1.9.0", LastChecked: time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC), LatestVersion: "1.9.0"})
	fx.s.Run(context.Background())
	if len(fx.gh.hits) != 0 || fx.out.Len() != 0 {
		t.Errorf("checked within the interval: %v %q", fx.gh.hits, fx.out.String())
	}
}

func TestStartupOfflineRetriesInAnHour(t *testing.T) {
	fx := newStartup(t, "", State{LastSeenVersion: "1.9.0"})
	fx.s.Client.BaseURL = "http://127.0.0.1:1" // nothing listens here
	fx.s.Run(context.Background())
	st := fx.state()
	if fx.out.Len() != 0 {
		t.Errorf("printed while offline: %q", fx.out.String())
	}
	if next := st.LastChecked.Add(CheckInterval); !next.Equal(fx.now.Add(time.Hour)) {
		t.Errorf("next check at %v, want an hour from now", next)
	}
}

func TestStartupFreshInstallRecordsVersionQuietly(t *testing.T) {
	fx := newStartup(t, "", State{LastChecked: time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC), LatestVersion: "1.9.0"})
	fx.s.Run(context.Background())
	if fx.out.Len() != 0 {
		t.Errorf("printed on first run: %q", fx.out.String())
	}
	if st := fx.state(); st.LastSeenVersion != "1.9.0" {
		t.Errorf("state %+v", st)
	}
}

func TestStartupShowsNotesAfterUpgradeElsewhere(t *testing.T) {
	// e.g. `brew upgrade` from 1.8.1: the first run of 1.9.0 shows its notes once.
	fx := newStartup(t, "", State{LastSeenVersion: "1.8.1", LastChecked: time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC), LatestVersion: "1.9.0"})
	fx.s.Run(context.Background())
	out := fx.out.String()
	if !strings.Contains(out, "shipyard was upgraded to 1.9.0") || !strings.Contains(out, "new in 1.9") {
		t.Errorf("output:\n%s", out)
	}
	if st := fx.state(); st.LastSeenVersion != "1.9.0" {
		t.Errorf("state %+v", st)
	}

	fx.out.Reset()
	fx.s.Run(context.Background())
	if fx.out.Len() != 0 {
		t.Errorf("notes shown twice:\n%s", fx.out.String())
	}
}
