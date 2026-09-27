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
	// onRequest, if set, runs before each request is served.
	onRequest func()
	// fail lists API paths that return 500.
	fail map[string]bool
}

func newFakeGitHub(t *testing.T, releases ...Release) (*fakeGitHub, *httptest.Server) {
	t.Helper()
	f := &fakeGitHub{releases: releases, files: map[string][]byte{}, hits: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits[r.URL.Path]++
		if f.onRequest != nil {
			f.onRequest()
		}
		if f.fail[r.URL.Path] {
			http.Error(w, "unavailable", http.StatusInternalServerError)
			return
		}
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

	if installed, err := in.Install(context.Background(), rel); err != nil || installed != "1.10.0" {
		t.Fatalf("installed %q, err %v", installed, err)
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
	// Windows keeps the replaced binary as .old until a later run removes it.
	want := 1
	if runtime.GOOS == "windows" {
		want = 2
		if b, _ := os.ReadFile(exe + ".old"); string(b) != "old binary" {
			t.Errorf(".old holds %q", b)
		}
	}
	if entries, _ := os.ReadDir(filepath.Dir(exe)); len(entries) != want {
		t.Errorf("left files behind: %v", entries)
	}
}

func TestInstallDirectRejectsChecksumMismatch(t *testing.T) {
	f, srv := newFakeGitHub(t)
	rel := releaseWithBinary(f, srv, "v1.10.0", []byte("tampered"), strings.Repeat("ab", 32))
	in, exe := directInstaller(t, srv)

	_, err := in.Install(context.Background(), rel)
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
	if _, err := in.Install(context.Background(), rel); err == nil || !strings.Contains(err.Error(), "checksums.txt") {
		t.Fatalf("got %v", err)
	}
}

func TestInstallDirectMissingPlatformAsset(t *testing.T) {
	_, srv := newFakeGitHub(t)
	in, _ := directInstaller(t, srv)
	_, err := in.Install(context.Background(), &Release{TagName: "v1.10.0"})
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
	if installed, err := in.Install(context.Background(), &Release{TagName: "v1.10.0"}); err != nil || installed != "1.10.0" {
		t.Fatalf("installed %q, err %v", installed, err)
	}
	if strings.Join(calls, "; ") != "update --quiet; upgrade shipyard" {
		t.Errorf("brew calls: %v", calls)
	}
	if _, err := in.Install(context.Background(), &Release{TagName: "v1.11.0-rc.1", Prerelease: true}); err == nil {
		t.Error("Homebrew pre-release install should fail")
	}
}

func TestInstallHomebrewNoOpIsAFailure(t *testing.T) {
	// The tap's formula lags the GitHub release: brew exits 0 with nothing done.
	var calls []string
	_, err := homebrewInstaller("1.9.0", &calls).Install(context.Background(), &Release{TagName: "v1.10.0"})
	if err == nil || !strings.Contains(err.Error(), "the Homebrew formula doesn't have 1.10.0 yet (it has 1.9.0)") {
		t.Fatalf("got %v", err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "update-state.json")
	if s := LoadState(path); s != (State{}) {
		t.Errorf("missing file: %+v", s)
	}
	want := State{LastChecked: time.Unix(1700000000, 0).UTC(), LatestVersion: "1.10.0", NotifiedAt: time.Unix(1700000500, 0).UTC(), LastSeenVersion: "1.9.0"}
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

// notifyFixture runs a Notifier against a fake GitHub with 1.9.0 installed
// and 1.10.0 released.
type notifyFixture struct {
	gh  *fakeGitHub
	n   *Notifier
	now time.Time
}

func newNotify(t *testing.T, st State) *notifyFixture {
	t.Helper()
	gh, srv := newFakeGitHub(t,
		Release{TagName: "v1.10.0", Body: "- new in 1.10"},
		Release{TagName: "v1.9.0", Body: "- new in 1.9"},
	)
	fx := &notifyFixture{gh: gh, now: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
	path := filepath.Join(t.TempDir(), "update-state.json")
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}
	fx.n = &Notifier{Current: "1.9.0", StatePath: path, Client: testClient(srv), Now: func() time.Time { return fx.now }}
	return fx
}

func (fx *notifyFixture) state() State { return LoadState(fx.n.StatePath) }

// checkedEarlierToday is a state whose last check, a few hours ago, found
// nothing newer, so Check makes no request for the latest release.
func checkedEarlierToday(lastSeen string) State {
	return State{LastSeenVersion: lastSeen, LastChecked: time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC), LatestVersion: "1.9.0"}
}

func TestNotifyNewVersion(t *testing.T) {
	fx := newNotify(t, State{LastSeenVersion: "1.9.0"})
	notice := fx.n.Check(context.Background())
	for _, want := range []string{
		"A new version of shipyard is available: 1.9.0 → 1.10.0",
		"Run `shipyard upgrade` to install it.",
		"releases/tag/v1.10.0",
	} {
		if !strings.Contains(notice.Text, want) {
			t.Errorf("notice missing %q:\n%s", want, notice.Text)
		}
	}
	if st := fx.state(); st.LatestVersion != "1.10.0" || !st.LastChecked.Equal(fx.now) || !st.NotifiedAt.IsZero() {
		t.Errorf("before Shown: %+v", st)
	}
	notice.Shown()
	if st := fx.state(); !st.NotifiedAt.Equal(fx.now) {
		t.Errorf("after Shown: %+v", st)
	}
}

func TestNotifyAtMostDaily(t *testing.T) {
	fx := newNotify(t, State{LastSeenVersion: "1.9.0"})
	fx.n.Check(context.Background()).Shown()

	fx.now = fx.now.Add(23 * time.Hour)
	fx.gh.hits = map[string]int{}
	if text := fx.n.Check(context.Background()).Text; text != "" || len(fx.gh.hits) != 0 {
		t.Errorf("within a day: %q, requests %v", text, fx.gh.hits)
	}

	fx.now = fx.now.Add(2 * time.Hour)
	if text := fx.n.Check(context.Background()).Text; !strings.Contains(text, "1.9.0 → 1.10.0") {
		t.Errorf("not shown again the next day: %q", text)
	}
}

func TestNotifyNotShownIsShownNextTime(t *testing.T) {
	// The command finished before the check did, so nothing was printed.
	fx := newNotify(t, State{LastSeenVersion: "1.9.0"})
	fx.n.Check(context.Background())
	fx.gh.hits = map[string]int{}
	if text := fx.n.Check(context.Background()).Text; !strings.Contains(text, "1.9.0 → 1.10.0") {
		t.Errorf("unshown notice lost: %q", text)
	}
	if len(fx.gh.hits) != 0 {
		t.Errorf("re-fetched a fresh result: %v", fx.gh.hits)
	}
}

func TestNotifyUpToDate(t *testing.T) {
	fx := newNotify(t, State{LastSeenVersion: "1.10.0"})
	fx.n.Current = "1.10.0"
	if text := fx.n.Check(context.Background()).Text; text != "" {
		t.Errorf("printed when up to date: %q", text)
	}
}

func TestNotifyOfflineRetriesInAnHour(t *testing.T) {
	fx := newNotify(t, State{LastSeenVersion: "1.9.0"})
	fx.n.Client.BaseURL = "http://127.0.0.1:1" // nothing listens here
	if text := fx.n.Check(context.Background()).Text; text != "" {
		t.Errorf("printed while offline: %q", text)
	}
	if next := fx.state().LastChecked.Add(CheckInterval); !next.Equal(fx.now.Add(time.Hour)) {
		t.Errorf("next check at %v, want an hour from now", next)
	}
}

func TestNotifyFreshInstallRecordsVersionQuietly(t *testing.T) {
	fx := newNotify(t, checkedEarlierToday(""))
	if text := fx.n.Check(context.Background()).Text; text != "" {
		t.Errorf("printed on first run: %q", text)
	}
	if st := fx.state(); st.LastSeenVersion != "1.9.0" {
		t.Errorf("state %+v", st)
	}
}

func TestNotifyNotesAfterUpgradeElsewhere(t *testing.T) {
	// e.g. `brew upgrade` from 1.8.1: the first run of 1.9.0 shows its notes once.
	fx := newNotify(t, checkedEarlierToday("1.8.1"))
	notice := fx.n.Check(context.Background())
	if !strings.Contains(notice.Text, "shipyard was upgraded to 1.9.0") || !strings.Contains(notice.Text, "new in 1.9") {
		t.Errorf("notice:\n%s", notice.Text)
	}
	if strings.Contains(notice.Text, "new in 1.10") {
		t.Errorf("showed notes for a version not installed:\n%s", notice.Text)
	}
	if st := fx.state(); st.LastSeenVersion != "1.8.1" {
		t.Errorf("recorded as seen before it was shown: %+v", st)
	}
	notice.Shown()
	if text := fx.n.Check(context.Background()).Text; text != "" {
		t.Errorf("notes shown twice:\n%s", text)
	}
}

func TestNotifyNotesAndNewVersionTogether(t *testing.T) {
	// Upgraded from 1.8.1 to 1.9.0 with 1.10.0 already out.
	fx := newNotify(t, State{LastSeenVersion: "1.8.1"})
	text := fx.n.Check(context.Background()).Text
	notes, available := strings.Index(text, "new in 1.9"), strings.Index(text, "1.9.0 → 1.10.0")
	if notes < 0 || available < 0 || notes > available {
		t.Errorf("want notes, then the new-version line:\n%s", text)
	}
}

func TestNotifyKeepsStateWrittenMeanwhile(t *testing.T) {
	// Another shell's `shipyard upgrade` records notes as seen during this check.
	fx := newNotify(t, State{LastSeenVersion: "1.9.0"})
	fx.gh.onRequest = func() {
		st := LoadState(fx.n.StatePath)
		st.LastSeenVersion = "1.10.0"
		_ = st.Save(fx.n.StatePath)
	}
	fx.n.Check(context.Background())
	if st := fx.state(); st.LastSeenVersion != "1.10.0" || st.LatestVersion != "1.10.0" {
		t.Errorf("lost a write: %+v", st)
	}
}

func TestInstallHomebrewFormulaBehindRelease(t *testing.T) {
	// GitHub has 1.10.0 but the formula only reached 1.9.5: report what's installed.
	var calls []string
	in := homebrewInstaller("1.9.5", &calls)
	installed, err := in.Install(context.Background(), &Release{TagName: "v1.10.0"})
	if err != nil || installed != "1.9.5" {
		t.Fatalf("installed %q, err %v", installed, err)
	}
}

func TestParseBrewVersions(t *testing.T) {
	for out, want := range map[string]string{
		"shipyard 1.9.0\n":          "1.9.0",
		"shipyard 1.10.0 1.9.0\n":   "1.10.0",
		"shipyard 1.9.0 1.10.0\n":   "1.10.0",
		"shipyard 1.10.0_1 1.9.0\n": "1.10.0",
	} {
		if got, err := parseBrewVersions(out); err != nil || got != want {
			t.Errorf("parseBrewVersions(%q) = %q, %v; want %q", out, got, err, want)
		}
	}
	for _, out := range []string{"", "shipyard\n", "shipyard HEAD\n"} {
		if got, err := parseBrewVersions(out); err == nil {
			t.Errorf("parseBrewVersions(%q) = %q, want an error", out, got)
		}
	}
}

func TestRenderNotesStripsControlCharacters(t *testing.T) {
	var buf bytes.Buffer
	RenderNotes(&buf, []Release{{TagName: "v1.10.0", Body: "- fixed\x1b]52;c;aGk=\x07 a bug\x1b[2J\tdone"}}, 0)
	if strings.ContainsAny(buf.String(), "\x1b\x07") {
		t.Errorf("control characters reached the output: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "\tdone") {
		t.Errorf("tab was stripped: %q", buf.String())
	}
}

func TestNotifyRecordsBackoffBeforeRequesting(t *testing.T) {
	// The command exits while the request is in flight, so the backoff must
	// already be saved by then or every run asks GitHub again.
	fx := newNotify(t, State{LastSeenVersion: "1.9.0"})
	var during State
	fx.gh.onRequest = func() { during = LoadState(fx.n.StatePath) }
	fx.n.Check(context.Background())
	if next := during.LastChecked.Add(CheckInterval); !next.Equal(fx.now.Add(time.Hour)) {
		t.Errorf("during the request, next check at %v, want an hour from now", next)
	}
	if st := fx.state(); !st.LastChecked.Equal(fx.now) || st.LatestVersion != "1.10.0" {
		t.Errorf("after a successful check: %+v", st)
	}
}

func TestNotifyNotesRetriedWhenFetchFails(t *testing.T) {
	fx := newNotify(t, checkedEarlierToday("1.8.1"))
	fx.gh.fail = map[string]bool{"/repos/shipyard/shipyard-cli/releases": true}
	notice := fx.n.Check(context.Background())
	if strings.Contains(notice.Text, "was upgraded") {
		t.Errorf("printed notes it couldn't fetch:\n%s", notice.Text)
	}
	notice.Shown()
	if st := fx.state(); st.LastSeenVersion != "1.8.1" {
		t.Errorf("notes marked seen after a failed fetch: %+v", st)
	}
	fx.gh.fail = nil
	fx.gh.hits = map[string]int{}
	if text := fx.n.Check(context.Background()).Text; text != "" || len(fx.gh.hits) != 0 {
		t.Errorf("retried within the hour: %q, requests %v", text, fx.gh.hits)
	}
	fx.now = fx.now.Add(time.Hour)
	if text := fx.n.Check(context.Background()).Text; !strings.Contains(text, "new in 1.9") {
		t.Errorf("notes not retried after an hour:\n%s", text)
	}
}

func TestNotifyFutureTimestampsCountAsElapsed(t *testing.T) {
	// A clock that was wrong left timestamps a year ahead.
	fx := newNotify(t, State{LastSeenVersion: "1.9.0"})
	ahead := fx.now.AddDate(1, 0, 0)
	st := fx.state()
	st.LastChecked, st.NotifiedAt, st.LatestVersion = ahead, ahead, "1.10.0"
	_ = st.Save(fx.n.StatePath)
	if text := fx.n.Check(context.Background()).Text; !strings.Contains(text, "1.9.0 → 1.10.0") {
		t.Errorf("held off by a future timestamp: %q", text)
	}
	if st := fx.state(); !st.LastChecked.Equal(fx.now) {
		t.Errorf("did not re-check: %+v", st)
	}
}

func TestBrewPath(t *testing.T) {
	prefix := t.TempDir()
	exe := filepath.Join(prefix, "Cellar", "shipyard", "1.9.0", "bin", "shipyard")
	if got := brewPath(exe); got != "brew" {
		t.Errorf("no brew in the prefix: got %q, want PATH lookup", got)
	}
	brew := filepath.Join(prefix, "bin", "brew")
	_ = os.MkdirAll(filepath.Dir(brew), 0o755)
	_ = os.WriteFile(brew, nil, 0o755)
	if got := brewPath(exe); got != brew {
		t.Errorf("got %q, want %q", got, brew)
	}
	if got := brewPath("/usr/local/bin/shipyard"); got != "brew" {
		t.Errorf("not a Cellar path: got %q", got)
	}
}

func TestInstallHomebrewUpgradesWhenUpdateFails(t *testing.T) {
	// An unrelated broken tap fails `brew update`; the upgrade still runs.
	var calls []string
	in := homebrewInstaller("1.10.0", &calls)
	in.Brew = func(_ context.Context, _ io.Writer, args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		if args[0] == "update" {
			return errors.New("exit status 1")
		}
		return nil
	}
	if installed, err := in.Install(context.Background(), &Release{TagName: "v1.10.0"}); err != nil || installed != "1.10.0" {
		t.Fatalf("installed %q, err %v", installed, err)
	}
	if strings.Join(calls, "; ") != "update --quiet; upgrade shipyard" {
		t.Errorf("brew calls: %v", calls)
	}
}

func TestReplaceRenamingOldWhenOldIsInUse(t *testing.T) {
	dir := t.TempDir()
	exe, newPath := filepath.Join(dir, "shipyard.exe"), filepath.Join(dir, "new")
	_ = os.WriteFile(exe, []byte("current"), 0o755)
	_ = os.WriteFile(newPath, []byte("new"), 0o755)
	// An .old that can't be removed, standing in for one that's still running.
	_ = os.MkdirAll(filepath.Join(exe+".old", "busy"), 0o755)

	if err := replaceRenamingOld(exe, newPath); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Errorf("exe holds %q", b)
	}
	olds, _ := filepath.Glob(exe + ".old-*")
	if len(olds) != 1 {
		t.Fatalf("want the current binary moved to a unique .old-*, got %v", olds)
	}
	if b, _ := os.ReadFile(olds[0]); string(b) != "current" {
		t.Errorf("moved-aside binary holds %q", b)
	}
}

func afterUpgradeFixture(t *testing.T) (*fakeGitHub, *Client, string) {
	t.Helper()
	gh, srv := newFakeGitHub(t,
		Release{TagName: "v1.10.0", Body: "- new in 1.10"},
		Release{TagName: "v1.9.5", Body: "- new in 1.9.5"},
		Release{TagName: "v1.9.0", Body: "- new in 1.9"},
	)
	path := filepath.Join(t.TempDir(), "update-state.json")
	_ = State{LastSeenVersion: "1.9.0"}.Save(path)
	return gh, testClient(srv), path
}

func TestAfterUpgrade(t *testing.T) {
	_, c, path := afterUpgradeFixture(t)
	var buf bytes.Buffer
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	if err := AfterUpgrade(context.Background(), &buf, c, path, "1.9.0", "1.10.0", &Release{TagName: "v1.10.0"}, now); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "✓ Upgraded to 1.10.0") || !strings.Contains(out, "new in 1.10") || !strings.Contains(out, "new in 1.9.5") {
		t.Errorf("output:\n%s", out)
	}
	if strings.Contains(out, "Homebrew doesn't have") {
		t.Errorf("lag message without lag:\n%s", out)
	}
	if st := LoadState(path); st.LastSeenVersion != "1.10.0" || st.LatestVersion != "1.10.0" || !st.LastChecked.Equal(now) {
		t.Errorf("state %+v", st)
	}
}

func TestAfterUpgradeHomebrewBehind(t *testing.T) {
	// Homebrew installed 1.9.5 while GitHub has 1.10.0.
	_, c, path := afterUpgradeFixture(t)
	var buf bytes.Buffer
	if err := AfterUpgrade(context.Background(), &buf, c, path, "1.9.0", "1.9.5", &Release{TagName: "v1.10.0", Body: "- new in 1.10"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "✓ Upgraded to 1.9.5") || !strings.Contains(out, "Homebrew doesn't have 1.10.0 yet") || !strings.Contains(out, "new in 1.9.5") {
		t.Errorf("output:\n%s", out)
	}
	if strings.Contains(out, "new in 1.10") {
		t.Errorf("showed notes for a version not installed:\n%s", out)
	}
	if st := LoadState(path); st.LastSeenVersion != "1.9.5" || st.LatestVersion != "1.10.0" {
		t.Errorf("state %+v", st)
	}
}

func TestAfterUpgradeHomebrewBehindAndNotesUnavailable(t *testing.T) {
	// No notes could be shown, so they mustn't be recorded as seen.
	gh, c, path := afterUpgradeFixture(t)
	gh.fail = map[string]bool{"/repos/shipyard/shipyard-cli/releases": true}
	var buf bytes.Buffer
	_ = AfterUpgrade(context.Background(), &buf, c, path, "1.9.0", "1.9.5", &Release{TagName: "v1.10.0"}, time.Now())
	if st := LoadState(path); st.LastSeenVersion != "1.9.0" {
		t.Errorf("notes marked seen without being shown: %+v", st)
	}
}

func TestAfterUpgradeForceSameVersion(t *testing.T) {
	// --force reinstalls 1.10.0 over 1.10.0: there's nothing in between, so
	// the release's own notes are shown.
	_, c, path := afterUpgradeFixture(t)
	var buf bytes.Buffer
	_ = AfterUpgrade(context.Background(), &buf, c, path, "1.10.0", "1.10.0", &Release{TagName: "v1.10.0", Body: "- new in 1.10"}, time.Now())
	if !strings.Contains(buf.String(), "new in 1.10") {
		t.Errorf("output:\n%s", buf.String())
	}
}

func TestIsOldBinary(t *testing.T) {
	for name, want := range map[string]bool{
		"shipyard.exe.old":         true,
		"shipyard.exe.old-1790000": true,
		"shipyard.exe":             false,
		"shipyard.exe.old.bak":     false,
		"shipyard.exe.older":       false,
		"shipyard.exe.old-":        false,
		"shipyard.exe.old-1a":      false,
		"other.exe.old":            false,
	} {
		if got := isOldBinary("shipyard.exe", name); got != want {
			t.Errorf("isOldBinary(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestNotifySkipsWhenStateCantBeSaved(t *testing.T) {
	fx := newNotify(t, State{LastSeenVersion: "1.9.0"})
	// A directory where the state file should be: nothing can be saved there.
	fx.n.StatePath = filepath.Join(t.TempDir(), "update-state.json")
	_ = os.MkdirAll(filepath.Join(fx.n.StatePath, "x"), 0o755)
	fx.gh.hits = map[string]int{}
	if text := fx.n.Check(context.Background()).Text; text != "" || len(fx.gh.hits) != 0 {
		t.Errorf("checked without being able to remember it: %q, requests %v", text, fx.gh.hits)
	}
}
