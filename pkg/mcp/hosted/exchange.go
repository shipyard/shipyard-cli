package hosted

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"golang.org/x/sync/singleflight"
)

const (
	grantTypeTokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"
	tokenTypeAccessToken   = "urn:ietf:params:oauth:token-type:access_token"

	// An exchanged token is reused until this long before it expires, so a
	// tool call never starts with a token that runs out partway through. The
	// longest, a log read that times out and retries, takes about 50 seconds.
	expiryMargin = 90 * time.Second
	// maxCached bounds the cache. Past it, expired entries go first, then the
	// tenth closest to expiry.
	maxCached = 10000
	// unavailableBackoff is how long a token that isn't cached waits for core
	// after core was down or refused this server, instead of every request
	// asking core again meanwhile.
	unavailableBackoff = 5 * time.Second
	// maxTokenBytes is the longest token sent to core. Core gives this server
	// its own rate limit only for token requests under 8KB; a longer one counts
	// against the limit for everyone from this server's address.
	maxTokenBytes = 4096
)

// errUnavailable means the exchange couldn't be completed for a reason that
// isn't the client's token: core is down, slow, or this server is misconfigured.
// It must never become a 401, which sends the client back through sign-in
// for something signing in again can't fix.
var errUnavailable = errors.New("token exchange unavailable")

// errMisconfigured is the part of errUnavailable this server has to fix: core
// refused its client credentials or the resource it asked for.
var errMisconfigured = errors.New("this server's OAuth client or URLs don't match core's")

// errCoreDown is the part of errUnavailable that holds for every token: core
// couldn't be reached, failed, or refused this server. Only it pauses the
// exchanges. Any other refusal is about the one request a client chose to send.
var errCoreDown = errors.New("core is down or refused this server")

// exchanged is a core API token obtained for one client token.
type exchanged struct {
	token   string
	scopes  []string
	expires time.Time
}

// exchanger trades a client's MCP token for a short-lived core API token
// (RFC 8693). The client's token is never sent anywhere but core's token endpoint.
type exchanger struct {
	tokenURL     string
	resource     string
	clientID     string
	clientSecret string
	http         *http.Client
	now          func() time.Time

	group singleflight.Group

	mu    sync.Mutex
	cache map[[sha256.Size]byte]exchanged
	// unavailableUntil is when core may be asked again after errUnavailable.
	unavailableUntil time.Time
}

func newExchanger(tokenURL, resource, clientID, clientSecret string) *exchanger {
	return &exchanger{
		tokenURL:     tokenURL,
		resource:     resource,
		clientID:     clientID,
		clientSecret: clientSecret,
		http: &http.Client{
			Timeout: 10 * time.Second,
			// A 307 or 308 would send the client secret and the user's token
			// again, to wherever it points.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		now:   time.Now,
		cache: make(map[[sha256.Size]byte]exchanged),
	}
}

// exchange returns a core API token for subject. An error wraps
// auth.ErrInvalidToken when the subject token itself is no good, and
// errUnavailable otherwise.
func (e *exchanger) exchange(ctx context.Context, subject string) (exchanged, error) {
	if len(subject) > maxTokenBytes {
		return exchanged{}, fmt.Errorf("%w: the token is too long", auth.ErrInvalidToken)
	}
	key := sha256.Sum256([]byte(subject))
	e.mu.Lock()
	hit, ok := e.cache[key]
	backoff := e.now().Before(e.unavailableUntil)
	e.mu.Unlock()
	if ok && e.now().Before(hit.expires.Add(-expiryMargin)) {
		return hit, nil
	}
	// While core can't exchange again, a cached token is good until it expires.
	usable := ok && e.now().Before(hit.expires)
	if backoff {
		if usable {
			return hit, nil
		}
		return exchanged{}, fmt.Errorf("%w: core was unavailable moments ago", errUnavailable)
	}

	// Requests carrying the same token share one exchange. It isn't tied to the
	// first request: that client going away mustn't fail the others.
	result, err, _ := e.group.Do(string(key[:]), func() (any, error) {
		return e.request(context.WithoutCancel(ctx), subject)
	})
	if err != nil {
		if errors.Is(err, errCoreDown) {
			e.mu.Lock()
			e.unavailableUntil = e.now().Add(unavailableBackoff)
			e.mu.Unlock()
		}
		if usable && !errors.Is(err, auth.ErrInvalidToken) {
			return hit, nil
		}
		e.forget(subject)
		return exchanged{}, err
	}
	got := result.(exchanged)

	e.mu.Lock()
	defer e.mu.Unlock()
	e.store(key, got)
	return got, nil
}

// store caches got under key, making room first when the cache is full. Call
// it with e.mu held.
func (e *exchanger) store(key [sha256.Size]byte, got exchanged) {
	if len(e.cache) >= maxCached {
		now := e.now()
		for k, v := range e.cache {
			if !now.Before(v.expires) {
				delete(e.cache, k)
			}
		}
	}
	if len(e.cache) >= maxCached {
		keys := make([][sha256.Size]byte, 0, len(e.cache))
		for k := range e.cache {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, b [sha256.Size]byte) int {
			return e.cache[a].expires.Compare(e.cache[b].expires)
		})
		for _, k := range keys[:len(keys)/10+1] {
			delete(e.cache, k)
		}
	}
	e.cache[key] = got
}

// forget drops subject's cached token, so the next request exchanges again.
func (e *exchanger) forget(subject string) {
	key := sha256.Sum256([]byte(subject))
	e.mu.Lock()
	delete(e.cache, key)
	e.mu.Unlock()
}

func (e *exchanger) request(ctx context.Context, subject string) (exchanged, error) {
	form := url.Values{
		"grant_type":         {grantTypeTokenExchange},
		"subject_token":      {subject},
		"subject_token_type": {tokenTypeAccessToken},
		"resource":           {e.resource},
		// client_secret_post: core accepts it, and it avoids the URL-encoding
		// rules RFC 6749 puts on credentials in a basic auth header.
		"client_id":     {e.clientID},
		"client_secret": {e.clientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return exchanged{}, fmt.Errorf("%w: %w: %v", errUnavailable, errCoreDown, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	start := e.now()
	resp, err := e.http.Do(req)
	if err != nil {
		return exchanged{}, fmt.Errorf("%w: %w: %v", errUnavailable, errCoreDown, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return exchanged{}, fmt.Errorf("%w: %w: reading the response: %v", errUnavailable, errCoreDown, err)
	}

	var payload struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
	}
	_ = json.Unmarshal(body, &payload)

	if resp.StatusCode != http.StatusOK {
		switch {
		// invalid_grant and invalid_scope are the answers about the subject token:
		// it is unknown, expired, revoked, for another audience, its user left the
		// org, or it has no scope to exchange. Signing in again fixes each.
		case resp.StatusCode == http.StatusBadRequest && (payload.Error == "invalid_grant" || payload.Error == "invalid_scope"):
			return exchanged{}, fmt.Errorf("%w: the token was refused (%s)", auth.ErrInvalidToken, payload.Error)
		// Core checks this server's client and resource before the subject token.
		case payload.Error == "invalid_client" || payload.Error == "unauthorized_client" || payload.Error == "invalid_target":
			return exchanged{}, fmt.Errorf("%w: %w: %w: core answered %d %s", errUnavailable, errCoreDown, errMisconfigured, resp.StatusCode, payload.Error)
		case resp.StatusCode >= http.StatusInternalServerError:
			return exchanged{}, fmt.Errorf("%w: %w: core answered %d", errUnavailable, errCoreDown, resp.StatusCode)
		}
		// Anything else (a 429, a redirect) is unavailable for this request only.
		return exchanged{}, fmt.Errorf("%w: core answered %d %s", errUnavailable, resp.StatusCode, payload.Error)
	}
	if payload.AccessToken == "" || payload.ExpiresIn <= 0 || !strings.EqualFold(payload.TokenType, "bearer") {
		return exchanged{}, fmt.Errorf("%w: core's response has no usable bearer token", errUnavailable)
	}
	return exchanged{
		token:   payload.AccessToken,
		scopes:  strings.Fields(payload.Scope),
		expires: start.Add(time.Duration(payload.ExpiresIn) * time.Second),
	}, nil
}
