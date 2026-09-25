package selfupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// State is what the updater remembers between runs. It lives in its own file,
// not config.yaml, because commands such as set_org rewrite config.yaml.
type State struct {
	// LastChecked is when GitHub was last asked for the latest release.
	LastChecked time.Time `json:"last_checked"`
	// LatestVersion is the newest release that check found.
	LatestVersion string `json:"latest_version,omitempty"`
	// SkippedVersion is a release the user chose to skip; they aren't asked
	// about it again, only about releases after it.
	SkippedVersion string `json:"skipped_version,omitempty"`
	// SnoozedUntil holds off the prompt after the user answers "not now".
	SnoozedUntil time.Time `json:"snoozed_until,omitzero"`
	// LastSeenVersion is the version whose release notes the user has seen,
	// or the version they first ran. A newer running version means it was
	// upgraded since, by any method, and its notes are shown once.
	LastSeenVersion string `json:"last_seen_version,omitempty"`
}

// merge returns cur with the fields that changed from before to after applied.
func (cur State) merge(before, after State) State {
	if !after.LastChecked.Equal(before.LastChecked) {
		cur.LastChecked = after.LastChecked
	}
	if after.LatestVersion != before.LatestVersion {
		cur.LatestVersion = after.LatestVersion
	}
	if after.SkippedVersion != before.SkippedVersion {
		cur.SkippedVersion = after.SkippedVersion
	}
	if !after.SnoozedUntil.Equal(before.SnoozedUntil) {
		cur.SnoozedUntil = after.SnoozedUntil
	}
	if after.LastSeenVersion != before.LastSeenVersion {
		cur.LastSeenVersion = after.LastSeenVersion
	}
	return cur
}

// StatePath is where the state is kept: ~/.shipyard/update-state.json.
func StatePath(home string) string {
	return filepath.Join(home, ".shipyard", "update-state.json")
}

// LoadState reads the state, returning an empty one if the file is missing or
// unreadable: losing it only means asking again.
func LoadState(path string) State {
	var s State
	b, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(b, &s)
	return s
}

// Save writes the state atomically, so two shells starting at once can't leave
// a half-written file.
func (s State) Save(path string) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".update-state-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp makes the file 0600; a root-owned 0600 file left by
	// `sudo shipyard upgrade` would be unreadable to the user afterwards.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	chownToSudoUser(path)
	return nil
}

// chownToSudoUser gives a file written under sudo back to the invoking user,
// who keeps their HOME under sudo on macOS and so shares this state file.
func chownToSudoUser(path string) {
	uid, err1 := strconv.Atoi(os.Getenv("SUDO_UID"))
	gid, err2 := strconv.Atoi(os.Getenv("SUDO_GID"))
	if err1 != nil || err2 != nil || os.Geteuid() != 0 {
		return
	}
	_ = os.Chown(path, uid, gid)
}
