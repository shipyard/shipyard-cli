package selfupdate

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"time"

	"k8s.io/client-go/util/homedir"
)

// State is what the updater remembers between runs. It lives in its own file,
// not config.yaml, because commands such as set_org rewrite config.yaml.
type State struct {
	// LastChecked is when GitHub was last asked for the latest release.
	LastChecked time.Time `json:"last_checked"`
	// LatestVersion is the newest release that check found.
	LatestVersion string `json:"latest_version,omitempty"`
	// NotifiedAt is when the "new version available" message was last shown;
	// it's shown at most once per CheckInterval.
	NotifiedAt time.Time `json:"notified_at,omitzero"`
	// LastSeenVersion is the version whose release notes the user has seen,
	// or the version they first ran. A newer running version means it was
	// upgraded since, by any method, and its notes are shown once.
	LastSeenVersion string `json:"last_seen_version,omitempty"`
	// NotesTried is when the notes for an upgrade made elsewhere were last
	// fetched; a failed fetch is retried at most hourly.
	NotesTried time.Time `json:"notes_tried,omitzero"`
}

// merge returns cur with the fields that changed from before to after applied.
func (cur State) merge(before, after State) State {
	if !after.LastChecked.Equal(before.LastChecked) {
		cur.LastChecked = after.LastChecked
	}
	if after.LatestVersion != before.LatestVersion {
		cur.LatestVersion = after.LatestVersion
	}
	if !after.NotifiedAt.Equal(before.NotifiedAt) {
		cur.NotifiedAt = after.NotifiedAt
	}
	if after.LastSeenVersion != before.LastSeenVersion {
		cur.LastSeenVersion = after.LastSeenVersion
	}
	if !after.NotesTried.Equal(before.NotesTried) {
		cur.NotesTried = after.NotesTried
	}
	return cur
}

// StatePath is where the state is kept: ~/.shipyard/update-state.json.
func StatePath(home string) string {
	return filepath.Join(home, ".shipyard", "update-state.json")
}

// LoadState reads the state, returning an empty one if the file is missing or
// unreadable: losing it only means checking again.
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
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		// Lchown: a directory we just made, never whatever a symlink points to.
		if uid, gid, ok := sudoUser(); ok {
			_ = os.Lchown(dir, uid, gid)
		}
	}
	tmp, err := os.CreateTemp(dir, ".update-state-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	// Permissions and ownership are set on the open file, before it's moved
	// into place, so they can't be redirected to another file by a symlink.
	// CreateTemp makes the file 0600; a root-owned 0600 file left by
	// `sudo shipyard upgrade` would be unreadable to the user afterwards.
	err = tmp.Chmod(0o644)
	if uid, gid, ok := sudoUser(); ok && err == nil {
		err = tmp.Chown(uid, gid)
	}
	if err == nil {
		_, err = tmp.Write(append(b, '\n'))
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// sudoUser returns the user who ran `sudo shipyard ...`, when running as root
// under sudo.
func sudoUser() (uid, gid int, ok bool) {
	uid, err1 := strconv.Atoi(os.Getenv("SUDO_UID"))
	gid, err2 := strconv.Atoi(os.Getenv("SUDO_GID"))
	return uid, gid, err1 == nil && err2 == nil && os.Geteuid() == 0
}

// HomeDir is the home directory whose state file to use: the invoking user's
// under sudo, which on Linux sets HOME to root's, so that notes recorded as
// seen by `sudo shipyard upgrade` aren't shown to the user again.
func HomeDir() string {
	if _, _, ok := sudoUser(); ok {
		if u, err := user.LookupId(os.Getenv("SUDO_UID")); err == nil && u.HomeDir != "" {
			return u.HomeDir
		}
	}
	return homedir.HomeDir()
}
