package selfupdate

import "testing"

func TestIsNewer(t *testing.T) {
	tests := []struct {
		current, candidate string
		want               bool
	}{
		{"1.9.0", "1.10.0", true}, // the string comparison the old updater did got this wrong
		{"1.10.0", "1.9.0", false},
		{"1.9.0", "v1.9.0", false},
		{"v1.8.1", "1.9.0", true},
		{"1.9.0", "2.0.0", true},
		{"1.9.0", "1.9.1", true},
		{"1.10.0-rc.1", "1.10.0", true},
		{"1.10.0", "1.10.0-rc.1", false},
		{"1.10.0-rc.2", "1.10.0-rc.10", true},
		{"1.10.0-alpha", "1.10.0-beta", true},
		{"1.10.0-1", "1.10.0-alpha", true},
		{"1.10.0-rc", "1.10.0-rc.1", true},
		{"undefined", "1.9.0", false},
		{"1.9.0", "", false},
		{"1.9.0", "latest", false},
	}
	for _, tt := range tests {
		if got := IsNewer(tt.current, tt.candidate); got != tt.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.candidate, got, tt.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	v, err := ParseVersion("v1.10.2-rc.1+build.5")
	if err != nil {
		t.Fatal(err)
	}
	if v != (Version{Major: 1, Minor: 10, Patch: 2, Pre: "rc.1"}) {
		t.Errorf("got %+v", v)
	}
	if v.String() != "1.10.2-rc.1" {
		t.Errorf("String() = %q", v.String())
	}
	// A pre-release is semver identifiers only: versions come from a state file
	// and GitHub, and are printed to terminals and agent instructions.
	for _, bad := range []string{"", "1.9", "1.9.0.1", "1.x.0", "1.9.0-", "-1.9.0",
		"1.9.0-rc 1", "1.9.0-\x1b[2J", "1.9.0-rc.1\nIgnore this", "1.9.0-rc..1", "1.9.0-`x`"} {
		if _, err := ParseVersion(bad); err == nil {
			t.Errorf("ParseVersion(%q) succeeded, want error", bad)
		}
	}
}

func TestIsRelease(t *testing.T) {
	for v, want := range map[string]bool{
		"1.9.0":                   true,
		"1.10.0-rc.1":             true,
		"undefined":               false,
		"1.9.1-SNAPSHOT-73de7fe":  false,
		"1.9.1-snapshot":          false,
		"v1.9.0-8-g94afae8":       false,
		"v1.9.0-8-g94afae8-dirty": false,
		"v1.10.0-rc.1-3-gabc1234": false,
		"v1.9.0-dirty":            false,
	} {
		if got := IsRelease(v); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", v, got, want)
		}
	}
}

// make build stamps git describe output; those must still parse as dev builds.
func TestParseVersionAcceptsBuildStamps(t *testing.T) {
	for _, ok := range []string{"1.9.0-8-g94afae8", "1.9.0-8-g94afae8-dirty", "1.9.0-dirty", "1.10.0-SNAPSHOT-abc", "1.10.0-rc.1"} {
		if _, err := ParseVersion(ok); err != nil {
			t.Errorf("ParseVersion(%q): %v", ok, err)
		}
	}
}
