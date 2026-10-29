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
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

const (
	grantTypeTokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"
	tokenTypeAccessToken   = "urn:ietf:params:oauth:token-type:access_token"

	// An exchanged token is reused until this long before it expires, so a
	// tool call never starts with a token that runs out partway through.
	expiryMargin = 30 * time.Second
	// maxCached bounds the cache. Past it, expired entries go first, then everything.
	maxCached = 10000
)

// errUnavailable means the exchange couldn't be completed for a reason that
// isn't the client's token: core is down, slow, or this server is misconfigured.
// It must never become a 401, which sends the client back through sign-in
// for something signing in again can't fix.
var errUnavailable = errors.New("token exchange unavailable")

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

	mu    sync.Mutex
	cache map[[sha256.Size]byte]exchanged
}

func newExchanger(tokenURL, resource, clientID, clientSecret string) *exchanger {
	return &exchanger{
		tokenURL:     tokenURL,
		resource:     resource,
		clientID:     clientID,
		clientSecret: clientSecret,
		http:         &http.Client{Timeout: 10 * time.Second},
		now:          time.Now,
		cache:        make(map[[sha256.Size]byte]exchanged),
	}
}

// exchange returns a core API token for subject. An error wraps
// auth.ErrInvalidToken when the subject token itself is no good, and
// errUnavailable otherwise.
func (e *exchanger) exchange(ctx context.Context, subject string) (exchanged, error) {
	key := sha256.Sum256([]byte(subject))
	e.mu.Lock()
	hit, ok := e.cache[key]
	e.mu.Unlock()
	if ok && e.now().Before(hit.expires.Add(-expiryMargin)) {
		return hit, nil
	}

	got, err := e.request(ctx, subject)
	if err != nil {
		e.forget(subject)
		return exchanged{}, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.cache) >= maxCached {
		now := e.now()
		for k, v := range e.cache {
			if !now.Before(v.expires) {
				delete(e.cache, k)
			}
		}
		if len(e.cache) >= maxCached {
			clear(e.cache)
		}
	}
	e.cache[key] = got
	return got, nil
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
		return exchanged{}, fmt.Errorf("%w: %v", errUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	start := e.now()
	resp, err := e.http.Do(req)
	if err != nil {
		return exchanged{}, fmt.Errorf("%w: %v", errUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return exchanged{}, fmt.Errorf("%w: reading the response: %v", errUnavailable, err)
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
		// invalid_grant is the one answer about the subject token: it is unknown,
		// expired, revoked, for another audience, or its user left the org.
		// Anything else (invalid_client, invalid_target, a 5xx) is ours to fix.
		if resp.StatusCode == http.StatusBadRequest && payload.Error == "invalid_grant" {
			return exchanged{}, fmt.Errorf("%w: the token was refused", auth.ErrInvalidToken)
		}
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
