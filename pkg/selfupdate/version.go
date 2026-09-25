// Package selfupdate checks GitHub for new releases of the CLI, installs them,
// and renders their release notes in the terminal.
package selfupdate

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a parsed semantic version: MAJOR.MINOR.PATCH with an optional
// pre-release suffix such as "rc.1". Build metadata after "+" is ignored.
type Version struct {
	Major, Minor, Patch int
	Pre                 string
}

// ParseVersion accepts "1.9.0", "v1.9.0" and "1.10.0-rc.1".
func ParseVersion(s string) (Version, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v Version
	core := s
	if i := strings.IndexByte(s, '-'); i >= 0 {
		core, v.Pre = s[:i], s[i+1:]
		if v.Pre == "" {
			return Version{}, fmt.Errorf("invalid version %q: empty pre-release", s)
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version %q: want MAJOR.MINOR.PATCH", s)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("invalid version %q: %q is not a number", s, p)
		}
		nums[i] = n
	}
	v.Major, v.Minor, v.Patch = nums[0], nums[1], nums[2]
	return v, nil
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Compare returns -1, 0 or 1 as v is older than, equal to, or newer than o,
// following semver precedence: 1.9.0 < 1.10.0, and 1.10.0-rc.1 < 1.10.0.
func (v Version) Compare(o Version) int {
	for _, d := range []int{v.Major - o.Major, v.Minor - o.Minor, v.Patch - o.Patch} {
		if d != 0 {
			return sign(d)
		}
	}
	switch {
	case v.Pre == o.Pre:
		return 0
	case v.Pre == "":
		return 1
	case o.Pre == "":
		return -1
	}
	return comparePre(v.Pre, o.Pre)
}

// comparePre compares dot-separated pre-release identifiers: numeric ones
// numerically, others lexically, numeric before alphanumeric, and a shorter
// list before a longer one it prefixes.
func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aErr := strconv.Atoi(as[i])
		bn, bErr := strconv.Atoi(bs[i])
		switch {
		case aErr == nil && bErr == nil:
			if an != bn {
				return sign(an - bn)
			}
		case aErr == nil:
			return -1
		case bErr == nil:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	return sign(len(as) - len(bs))
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

// IsNewer reports whether candidate is a newer version than current. It is
// false when either fails to parse, so a dev build never prompts to upgrade.
func IsNewer(current, candidate string) bool {
	c, err := ParseVersion(current)
	if err != nil {
		return false
	}
	n, err := ParseVersion(candidate)
	if err != nil {
		return false
	}
	return n.Compare(c) > 0
}

// IsRelease reports whether v looks like a released version, as opposed to a
// local build ("undefined") or a goreleaser snapshot ("1.9.1-SNAPSHOT-abc").
func IsRelease(v string) bool {
	p, err := ParseVersion(v)
	return err == nil && !strings.Contains(strings.ToUpper(p.Pre), "SNAPSHOT")
}
