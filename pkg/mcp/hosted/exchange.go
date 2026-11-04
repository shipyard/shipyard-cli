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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	// maxRetryAfter caps how long a Retry-After from core pauses the exchanges.
	maxRetryAfter = time.Minute
	// maxTokenBytes is the longest token sent to core, URL-encoded as it is in
	// the form. Core gives this server its own rate limit only for token
	// requests under 8KB, measured encoded; a longer one counts against the
	// limit for everyone from this server's address.
	maxTokenBytes = 4096
	// refusedFor is how long a token core refused is answered with a 401
	// without asking core again: every exchange, refused or not, counts against
	// core's ceiling for this server.
	refusedFor = time.Minute
	// sweepEvery is how often a full cache is scanned for expired entries.
	// Between scans a full cache evicts a batch, so tokens expiring one at a
	// time can't make every insert scan all of it.
	sweepEvery = 10 * time.Second
	// maxRefusals is how many tokens core may refuse for one client address in
	// refusalWindow, on one replica, before that address's tokens that aren't
	// held by this replica stop reaching core until the window ends. Only refusals count here.
	// Core has two limits for this server: one counts only exchanges it grants,
	// and a ceiling counts every request. So a junk token costs core nothing but
	// the ceiling, and enough of them from anywhere pause every user's
	// exchanges. This stops one address, not many. A client that sends all its
	// users from a few shared addresses rarely sends a token core refuses: each
	// refused token is answered from memory for refusedFor after the first.
	maxRefusals   = 60
	refusalWindow = time.Minute
	// recheckEvery is how often /readyz asks core about this server's own
	// credentials while they aren't confirmed.
	recheckEvery = 30 * time.Second
	// configCheckToken is a subject token that can't exist. Core checks this
	// server's client and the API it asks for before the subject token, so it
	// refuses the token itself only when both are right.
	configCheckToken = "shipyard-mcp-host-config-check"
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
// couldn't be reached, failed, refused this server, or rate-limited it. Only it
// pauses the exchanges. Any other refusal is about the one request a client
// chose to send.
var errCoreDown = errors.New("core is down or refused this server")

// errThrottled is the part of errUnavailable that holds for one client address:
// core refused too many of its tokens lately (see maxRefusals).
var errThrottled = errors.New("core refused too many tokens from this address")

// retryAfter is how long core asked this server to wait, from a 429.
type retryAfter time.Duration

func (r retryAfter) Error() string { return "retry after " + time.Duration(r).String() }

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
	// refused holds, for tokens core refused, when it may be asked about them again.
	refused map[[sha256.Size]byte]time.Time
	// sweptAt is when the full cache was last scanned for expired entries.
	sweptAt time.Time
	// unavailableUntil is when core may be asked again after errCoreDown.
	unavailableUntil time.Time
	// refusals counts, per client address, the tokens core refused in the
	// address's current window.
	refusals map[string]refusalCount
	// checkedAt is when /readyz last asked core about this server's credentials.
	checkedAt time.Time

	// misconfigured is set while core refuses this server's own client or the
	// API it asks for. accepted is set once core has accepted them, and stays set.
	// checked is set once core, or the lack of it, has answered about them.
	misconfigured atomic.Bool
	accepted      atomic.Bool
	checked       atomic.Bool
}

type refusalCount struct {
	start time.Time
	count int
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
		now:      time.Now,
		cache:    make(map[[sha256.Size]byte]exchanged),
		refused:  make(map[[sha256.Size]byte]time.Time),
		refusals: make(map[string]refusalCount),
	}
}

// exchange returns a core API token for subject, sent from the client address
// addr. An error wraps auth.ErrInvalidToken when the subject token itself is no
// good, and errUnavailable otherwise.
func (e *exchanger) exchange(ctx context.Context, subject, addr string) (exchanged, error) {
	if len(url.QueryEscape(subject)) > maxTokenBytes {
		return exchanged{}, fmt.Errorf("%w: the token is too long", auth.ErrInvalidToken)
	}
	key := sha256.Sum256([]byte(subject))
	e.mu.Lock()
	hit, ok := e.cache[key]
	refusedUntil, refused := e.refused[key]
	backoff := e.now().Before(e.unavailableUntil)
	throttled := e.overRefusals(addr)
	e.mu.Unlock()
	if refused && e.now().Before(refusedUntil) {
		return exchanged{}, fmt.Errorf("%w: core refused the token moments ago", auth.ErrInvalidToken)
	}
	if ok && e.now().Before(hit.expires.Add(-expiryMargin)) {
		return hit, nil
	}
	// While core can't exchange again, a cached token is good until it expires.
	usable := ok && e.now().Before(hit.expires)
	// A token this replica holds isn't junk, so the throttle doesn't apply: an
	// address's refused tokens can't lock out its active users.
	unknown := throttled && !ok
	if backoff || unknown {
		if usable {
			return hit, nil
		}
		if unknown {
			return exchanged{}, fmt.Errorf("%w: %w", errUnavailable, errThrottled)
		}
		return exchanged{}, fmt.Errorf("%w: core was unavailable moments ago", errUnavailable)
	}

	// Requests carrying the same token share one exchange. It isn't tied to the
	// first request: that client going away mustn't fail the others.
	result, err, _ := e.group.Do(string(key[:]), func() (any, error) {
		return e.request(context.WithoutCancel(ctx), subject)
	})
	e.observe(err)
	if err != nil {
		if errors.Is(err, errCoreDown) {
			pause := unavailableBackoff
			var wait retryAfter
			if errors.As(err, &wait) {
				pause = max(pause, min(time.Duration(wait), maxRetryAfter))
			}
			e.mu.Lock()
			// A shorter pause from a request that ended later mustn't cut a longer one short.
			e.unavailableUntil = later(e.unavailableUntil, e.now().Add(pause))
			e.mu.Unlock()
		}
		if usable && !errors.Is(err, auth.ErrInvalidToken) {
			return hit, nil
		}
		// Only a refusal says anything about the token: after any other failure
		// the entry stays, lapsed, so the token stays known.
		if errors.Is(err, auth.ErrInvalidToken) {
			e.forget(subject)
			e.refuse(key, addr)
		}
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
	if now := e.now(); len(e.cache) >= maxCached && now.Sub(e.sweptAt) >= sweepEvery {
		e.sweptAt = now
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

// refuse answers key's token with a 401 for refusedFor without asking core,
// and counts the refusal against addr, once however many requests were waiting
// on core's answer. A full set is cleared: the worst case is asking core again.
func (e *exchanger) refuse(key [sha256.Size]byte, addr string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	if until, ok := e.refused[key]; ok && now.Before(until) {
		return
	}
	if len(e.refused) >= maxCached {
		clear(e.refused)
	}
	e.refused[key] = now.Add(refusedFor)

	c := e.refusals[addr]
	if now.Sub(c.start) >= refusalWindow {
		if len(e.refusals) >= maxCached {
			for a, old := range e.refusals {
				if now.Sub(old.start) >= refusalWindow {
					delete(e.refusals, a)
				}
			}
			// Still full: forgetting every count only lets addresses ask core again.
			if len(e.refusals) >= maxCached {
				clear(e.refusals)
			}
		}
		c = refusalCount{start: now}
	}
	c.count++
	e.refusals[addr] = c
	if c.count == maxRefusals {
		Logger.Printf("MCP hosted: core refused %d tokens from one address within %s; its new tokens wait for the rest of it",
			maxRefusals, refusalWindow)
	}
}

// overRefusals reports whether core refused maxRefusals of addr's tokens in its
// current window. Call it with e.mu held.
func (e *exchanger) overRefusals(addr string) bool {
	c, ok := e.refusals[addr]
	return ok && c.count >= maxRefusals && e.now().Sub(c.start) < refusalWindow
}

// observe records what an answer from core says about this server's own
// client and the API it asks for. Any other error says nothing about them.
func (e *exchanger) observe(err error) {
	defer e.checked.Store(true)
	switch {
	case err == nil, errors.Is(err, auth.ErrInvalidToken):
		e.misconfigured.Store(false)
		e.accepted.Store(true)
	case errors.Is(err, errMisconfigured):
		e.misconfigured.Store(true)
	}
}

// notReady reports whether the first check of this server's credentials is
// still running, or core refuses them and has never accepted them: the release
// has the wrong secret. Once accepted, a refusal (core's secret rotated first)
// leaves it ready, so the tokens it holds keep working until the release catches up.
func (e *exchanger) notReady() bool {
	return !e.accepted.Load() && (e.misconfigured.Load() || !e.checked.Load())
}

// recheck asks core about this server's credentials in the background, at most
// every recheckEvery, until core accepts them. Without it a server that core
// refused would never learn it was fixed: it gets no traffic while unready.
func (e *exchanger) recheck() {
	if e.accepted.Load() {
		return
	}
	e.mu.Lock()
	now := e.now()
	due := now.Sub(e.checkedAt) >= recheckEvery
	if due {
		e.checkedAt = now
	}
	e.mu.Unlock()
	if !due {
		return
	}
	go func() {
		_, err := e.request(context.Background(), configCheckToken)
		e.observe(err)
	}()
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// expire stops subject's cached token being used, so the next request exchanges
// again. The entry stays: it's what tells a throttled address's known tokens
// from new ones.
func (e *exchanger) expire(subject string) {
	key := sha256.Sum256([]byte(subject))
	e.mu.Lock()
	defer e.mu.Unlock()
	if got, ok := e.cache[key]; ok {
		got.expires = time.Time{}
		e.cache[key] = got
	}
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
		// Every request this server sends is under core's 8KB host limit, so a 429
		// is core's ceiling for all of them: asking again only adds to it.
		case resp.StatusCode == http.StatusTooManyRequests:
			seconds, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			return exchanged{}, fmt.Errorf("%w: %w: %w: core answered 429", errUnavailable, errCoreDown,
				retryAfter(time.Duration(max(seconds, 0))*time.Second))
		}
		// Anything else (a redirect, another refusal) is unavailable for this request only.
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
