package uri

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/viper"
)

const (
	defaultAPIBase = "https://shipyard.build/api/v1"
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
	if !strings.HasPrefix(path, "/api/v1") && !strings.HasPrefix(path, "/api/v2") {
		return "", fmt.Errorf("path must start with /api/v1 or /api/v2, got %q", path)
	}
	if strings.HasPrefix(path, "/api/application") || strings.HasPrefix(path, "/api/me") {
		return "", fmt.Errorf("browser session routes are not allowed via shipyard api")
	}

	parsed, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if parsed.Path == "" || parsed.Path == "/" {
		return "", fmt.Errorf("path must include a resource after /api/v1 or /api/v2")
	}

	q := parsed.Query()
	if org != "" && q.Get("org") == "" {
		q.Set("org", org)
	}
	parsed.RawQuery = q.Encode()

	return APIHost() + parsed.String(), nil
}
