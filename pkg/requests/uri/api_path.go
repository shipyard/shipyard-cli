package uri

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/viper"
)

const (
	defaultAPIBase   = "https://shipyard.build/api/v1"
	maxResponseBytes = 1 << 20 // 1 MiB
)

// MaxResponseBytes is the response body cap for shipyard api.
func MaxResponseBytes() int {
	return maxResponseBytes
}

// APIHost returns the API origin (scheme + host[+port]) derived from api_url.
// CreateResourceURI bases include a trailing /api/v1; this strips that suffix so
// callers can join full paths like /api/v1/environment or /api/v2/environment.
func APIHost() string {
	base := viper.GetString("api_url")
	if base == "" {
		base = defaultAPIBase
	}
	base = strings.TrimRight(base, "/")
	for _, suffix := range []string{"/api/v1", "/api/v2"} {
		if strings.HasSuffix(base, suffix) {
			return strings.TrimSuffix(base, suffix)
		}
	}
	return base
}

// ResolveAPIPath validates path (must start with /api/v1 or /api/v2) and returns
// an absolute URL against APIHost, merging org into the query when set and not
// already present on the path.
func ResolveAPIPath(path string, org string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required (example: /api/v1/environment)")
	}
	if strings.Contains(path, "://") || strings.HasPrefix(path, "//") {
		return "", fmt.Errorf("absolute URLs are not allowed; pass a path starting with /api/v1 or /api/v2")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	parsed, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	// parsed.Path is percent-decoded, so %2e%2e is caught here too.
	for _, seg := range strings.Split(parsed.Path, "/") {
		if seg == "." || seg == ".." {
			return "", fmt.Errorf("path must not contain . or .. segments, got %q", path)
		}
	}
	if !hasAPIPrefix(parsed.Path) {
		return "", fmt.Errorf("path must start with /api/v1/ or /api/v2/ followed by a resource, got %q", path)
	}

	q := parsed.Query()
	if org != "" && q.Get("org") == "" {
		q.Set("org", org)
	}
	parsed.RawQuery = q.Encode()

	return APIHost() + parsed.String(), nil
}

func hasAPIPrefix(p string) bool {
	for _, prefix := range []string{"/api/v1/", "/api/v2/"} {
		if strings.HasPrefix(p, prefix) && len(p) > len(prefix) {
			return true
		}
	}
	return false
}
