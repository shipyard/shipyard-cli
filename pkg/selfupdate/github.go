package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"
)

const (
	repoOwner = "shipyard"
	repoName  = "shipyard-cli"
	userAgent = "shipyard-cli-updater"
)

// DefaultAPIBaseURL is the GitHub API origin releases are read from.
const DefaultAPIBaseURL = "https://api.github.com"

// Asset is a file attached to a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Release is the subset of a GitHub release the updater uses.
type Release struct {
	TagName    string  `json:"tag_name"`
	Name       string  `json:"name"`
	Body       string  `json:"body"`
	HTMLURL    string  `json:"html_url"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// Version returns the release's version without the leading "v".
func (r Release) Version() string {
	v, err := ParseVersion(r.TagName)
	if err != nil {
		return r.TagName
	}
	return v.String()
}

// Asset returns the attached file with exactly this name.
func (r Release) Asset(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// Client reads releases from the GitHub API.
type Client struct {
	HTTP    *http.Client
	BaseURL string
}

// NewClient returns a client whose requests give up after timeout.
func NewClient(timeout time.Duration) *Client {
	return &Client{HTTP: &http.Client{Timeout: timeout}, BaseURL: DefaultAPIBaseURL}
}

// Latest returns the newest stable release, or the newest release of any kind
// when prerelease is true.
func (c *Client) Latest(ctx context.Context, prerelease bool) (*Release, error) {
	if !prerelease {
		var r Release
		if err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/releases/latest", repoOwner, repoName), &r); err != nil {
			return nil, err
		}
		return &r, nil
	}
	releases, err := c.Releases(ctx)
	if err != nil {
		return nil, err
	}
	var newest *Release
	for i := range releases {
		r := &releases[i]
		if r.Draft {
			continue
		}
		if newest == nil || IsNewer(newest.TagName, r.TagName) {
			newest = r
		}
	}
	if newest == nil {
		return nil, fmt.Errorf("no releases found")
	}
	return newest, nil
}

// ByTag returns the release for a tag such as "v1.9.0".
func (c *Client) ByTag(ctx context.Context, tag string) (*Release, error) {
	var r Release
	if err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/releases/tags/%s", repoOwner, repoName, url.PathEscape(tag)), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Releases returns the most recent releases, newest first as GitHub orders them.
func (c *Client) Releases(ctx context.Context) ([]Release, error) {
	var rs []Release
	if err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/releases?per_page=50", repoOwner, repoName), &rs); err != nil {
		return nil, err
	}
	return rs, nil
}

// Between returns the releases newer than from and no newer than to, newest
// first: the notes a user moving from one version to the other has missed.
// Pre-releases are left out unless to is itself that pre-release.
func (c *Client) Between(ctx context.Context, from, to string) ([]Release, error) {
	all, err := c.Releases(ctx)
	if err != nil {
		return nil, err
	}
	return between(all, from, to), nil
}

func between(all []Release, from, to string) []Release {
	target, err := ParseVersion(to)
	if err != nil {
		return nil
	}
	var out []Release
	for _, r := range all {
		v, err := ParseVersion(r.TagName)
		if err != nil || r.Draft || !IsNewer(from, r.TagName) || v.Compare(target) > 0 {
			continue
		}
		if r.Prerelease && v.Compare(target) != 0 {
			continue
		}
		out = append(out, r)
	}
	// GitHub orders by creation date; a patch to an older line can land after
	// a newer release, so sort by version.
	sort.SliceStable(out, func(i, j int) bool { return IsNewer(out[j].TagName, out[i].TagName) })
	return out
}

func (c *Client) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned status %d for %s", resp.StatusCode, path)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
