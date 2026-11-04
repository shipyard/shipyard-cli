package hosted

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/mcp/tools"
	"github.com/shipyard/shipyard-cli/pkg/requests"
)

const (
	clientToken = "client-mcp-token"
	coreToken   = "exchanged-core-token"
	secret      = "host-secret"
	testAddr    = "192.0.2.1"
)

// fakeCore is core's token endpoint and API.
type fakeCore struct {
	t      *testing.T
	server *httptest.Server

	mu         sync.Mutex
	exchanges  int
	exchange   func(w http.ResponseWriter, form map[string]string)
	apiStatus  int
	apiTokens  []string
	apiHeaders []http.Header
}

func newFakeCore(t *testing.T) *fakeCore {
	f := &fakeCore{t: t, apiStatus: http.StatusOK}
	f.exchange = func(w http.ResponseWriter, form map[string]string) {
		if form["subject_token"] != clientToken {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":300,"scope":%q,"issued_token_type":%q}`,
			coreToken, strings.Join(tools.KnownScopes, " "), tokenTypeAccessToken)
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path == "/oauth/token" {
			f.exchanges++
			_ = r.ParseForm()
			form := map[string]string{}
			for k := range r.PostForm {
				form[k] = r.PostForm.Get(k)
			}
			for k, want := range map[string]string{
				"grant_type":         grantTypeTokenExchange,
				"subject_token_type": tokenTypeAccessToken,
				"resource":           f.server.URL + "/api",
				"client_id":          "host",
				"client_secret":      secret,
			} {
				if form[k] != want {
					t.Errorf("exchange %s = %q, want %q", k, form[k], want)
				}
			}
			f.exchange(w, form)
			return
		}
		f.apiTokens = append(f.apiTokens, r.Header.Get("Authorization"))
		f.apiHeaders = append(f.apiHeaders, r.Header.Clone())
		w.WriteHeader(f.apiStatus)
		if f.apiStatus == http.StatusOK {
			_, _ = w.Write([]byte(`{"data":[],"links":{},"meta":{"pagination":{"page":1,"pages":1}}}`))
		} else {
			_, _ = w.Write([]byte(`{"errors":[{"title":"Invalid token"}]}`))
		}
	}))
	t.Cleanup(f.server.Close)
	viper.Set("api_url", f.server.URL+"/api/v1")
	t.Cleanup(viper.Reset)
	return f
}

func newHost(t *testing.T, core *fakeCore) *httptest.Server {
	handler, err := Handler(Config{
		ResourceURL:  "https://mcp.example.test/mcp",
		SiteURL:      core.server.URL,
		ClientID:     "host",
		ClientSecret: secret,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

type rpcResult struct {
	status int
	header http.Header
	body   map[string]any
}

func call(t *testing.T, srv *httptest.Server, token, method string, params any) rpcResult {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	out := rpcResult{status: resp.StatusCode, header: resp.Header}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func initialize(t *testing.T, srv *httptest.Server, token string) rpcResult {
	return call(t, srv, token, "initialize", map[string]any{
		"protocolVersion": "2025-11-25",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "1"},
	})
}

func toolNames(t *testing.T, res rpcResult) []string {
	t.Helper()
	result, _ := res.body["result"].(map[string]any)
	list, _ := result["tools"].([]any)
	names := []string{}
	for _, item := range list {
		names = append(names, item.(map[string]any)["name"].(string))
	}
	slices.Sort(names)
	return names
}

func toolText(t *testing.T, res rpcResult) (string, bool) {
	t.Helper()
	result, ok := res.body["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", res.body)
	}
	content := result["content"].([]any)
	isError, _ := result["isError"].(bool)
	return content[0].(map[string]any)["text"].(string), isError
}

func TestNoTokenIsChallengedWithResourceMetadata(t *testing.T) {
	srv := newHost(t, newFakeCore(t))
	res := initialize(t, srv, "")
	if res.status != http.StatusUnauthorized {
		t.Fatalf("status = %d", res.status)
	}
	want := `Bearer resource_metadata="https://mcp.example.test/.well-known/oauth-protected-resource/mcp"`
	if got := res.header.Get("WWW-Authenticate"); got != want {
		t.Errorf("WWW-Authenticate = %q, want %q", got, want)
	}
}

func TestProtectedResourceMetadata(t *testing.T) {
	core := newFakeCore(t)
	srv := newHost(t, core)
	for _, path := range []string{"/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-protected-resource"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		var meta struct {
			Resource             string   `json:"resource"`
			AuthorizationServers []string `json:"authorization_servers"`
			ScopesSupported      []string `json:"scopes_supported"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&meta)
		_ = resp.Body.Close()
		if meta.Resource != "https://mcp.example.test/mcp" {
			t.Errorf("%s: resource = %q", path, meta.Resource)
		}
		if !slices.Equal(meta.AuthorizationServers, []string{core.server.URL}) {
			t.Errorf("%s: authorization_servers = %v", path, meta.AuthorizationServers)
		}
		if !slices.Equal(meta.ScopesSupported, tools.KnownScopes) {
			t.Errorf("%s: scopes_supported = %v", path, meta.ScopesSupported)
		}
	}
}

func TestATokenCoreRefusesIs401(t *testing.T) {
	srv := newHost(t, newFakeCore(t))
	res := initialize(t, srv, "some-other-token")
	if res.status != http.StatusUnauthorized {
		t.Fatalf("status = %d", res.status)
	}
	if !strings.Contains(res.header.Get("WWW-Authenticate"), "resource_metadata=") {
		t.Errorf("WWW-Authenticate = %q", res.header.Get("WWW-Authenticate"))
	}
}

func TestToolsAreTheHostedSet(t *testing.T) {
	srv := newHost(t, newFakeCore(t))
	if res := initialize(t, srv, clientToken); res.status != http.StatusOK {
		t.Fatalf("initialize status = %d: %v", res.status, res.body)
	}
	want := []string{}
	for name := range tools.HostedToolScopes {
		want = append(want, name)
	}
	slices.Sort(want)
	if got := toolNames(t, call(t, srv, clientToken, "tools/list", map[string]any{})); !slices.Equal(got, want) {
		t.Errorf("tools = %v, want %v", got, want)
	}
}

func TestToolsFollowTheExchangedScopes(t *testing.T) {
	core := newFakeCore(t)
	core.exchange = func(w http.ResponseWriter, _ map[string]string) {
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":300,"scope":"envs:read"}`, coreToken)
	}
	srv := newHost(t, core)
	got := toolNames(t, call(t, srv, clientToken, "tools/list", map[string]any{}))
	want := []string{"get_build_history", "get_build_logs", "get_environment", "get_environments", "get_failure_details"}
	if !slices.Equal(got, want) {
		t.Errorf("tools = %v, want %v", got, want)
	}
	res := call(t, srv, clientToken, "tools/call", map[string]any{"name": "create_application", "arguments": map[string]any{}})
	if _, ok := res.body["error"]; !ok {
		t.Errorf("a tool outside the token's scopes ran: %v", res.body)
	}
}

func TestToolCallsUseTheExchangedToken(t *testing.T) {
	core := newFakeCore(t)
	srv := newHost(t, core)
	res := call(t, srv, clientToken, "tools/call", map[string]any{"name": "get_environments", "arguments": map[string]any{}})
	if text, isError := toolText(t, res); isError {
		t.Fatalf("tool error: %s", text)
	}
	if len(core.apiTokens) == 0 {
		t.Fatal("the tool made no API call")
	}
	for i, token := range core.apiTokens {
		if token != "Bearer "+coreToken {
			t.Errorf("API call sent Authorization %q", token)
		}
		if got := core.apiHeaders[i].Get("x-api-token"); got != "" {
			t.Errorf("API call sent x-api-token %q", got)
		}
	}
}

func TestExchangesAreCachedUntilNearExpiry(t *testing.T) {
	core := newFakeCore(t)
	srv := newHost(t, core)
	for range 3 {
		call(t, srv, clientToken, "tools/list", map[string]any{})
	}
	if core.exchanges != 1 {
		t.Errorf("exchanges = %d, want 1", core.exchanges)
	}
}

func TestCachedTokenIsExchangedAgainBeforeItExpires(t *testing.T) {
	core := newFakeCore(t)
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	for _, step := range []time.Duration{0, 3 * time.Minute, 30 * time.Second} {
		now = now.Add(step)
		if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
			t.Fatal(err)
		}
	}
	// The second call has 2 minutes of the token left; the third has 90 seconds,
	// less than a tool call that retries a slow log read can take.
	if core.exchanges != 2 {
		t.Errorf("exchanges = %d, want 2", core.exchanges)
	}
}

func TestCoreUnavailableIsAToolErrorNotA401(t *testing.T) {
	for name, respond := range map[string]func(http.ResponseWriter, map[string]string){
		"5xx": func(w http.ResponseWriter, _ map[string]string) { w.WriteHeader(http.StatusBadGateway) },
		"invalid_client": func(w http.ResponseWriter, _ map[string]string) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
		},
		"no token": func(w http.ResponseWriter, _ map[string]string) { _, _ = w.Write([]byte(`{}`)) },
		"no expiry": func(w http.ResponseWriter, _ map[string]string) {
			_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":0,"scope":"envs:read"}`, coreToken)
		},
		"not a bearer token": func(w http.ResponseWriter, _ map[string]string) {
			_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"mac","expires_in":300,"scope":"envs:read"}`, coreToken)
		},
	} {
		t.Run(name, func(t *testing.T) {
			core := newFakeCore(t)
			core.exchange = respond
			srv := newHost(t, core)
			if res := initialize(t, srv, clientToken); res.status != http.StatusOK {
				t.Fatalf("initialize status = %d", res.status)
			}
			res := call(t, srv, clientToken, "tools/call", map[string]any{"name": "get_environments", "arguments": map[string]any{}})
			if res.status != http.StatusOK {
				t.Fatalf("status = %d", res.status)
			}
			text, isError := toolText(t, res)
			if !isError || text != unavailableMessage {
				t.Errorf("result = %q (isError %v)", text, isError)
			}
			if len(core.apiTokens) != 0 {
				t.Error("a tool called the API without an exchanged token")
			}
		})
	}
}

func TestAPIRefusingTheExchangedTokenAsksToReconnect(t *testing.T) {
	core := newFakeCore(t)
	core.apiStatus = http.StatusUnauthorized
	srv := newHost(t, core)
	res := call(t, srv, clientToken, "tools/call", map[string]any{"name": "get_environments", "arguments": map[string]any{}})
	if text, isError := toolText(t, res); !isError || text != reconnectMessage {
		t.Errorf("result = %q (isError %v)", text, isError)
	}
	// The cached token was dropped, so the next request exchanges again.
	call(t, srv, clientToken, "tools/list", map[string]any{})
	if core.exchanges != 2 {
		t.Errorf("exchanges = %d, want 2", core.exchanges)
	}
}

func TestConfigIsValidated(t *testing.T) {
	good := Config{ResourceURL: "https://mcp.example.test/mcp", SiteURL: "https://example.test", ClientID: "host", ClientSecret: secret}
	if _, err := Handler(good); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Config){
		"no resource":  func(c *Config) { c.ResourceURL = "" },
		"no site":      func(c *Config) { c.SiteURL = "example.test" },
		"no secret":    func(c *Config) { c.ClientSecret = "" },
		"no client id": func(c *Config) { c.ClientID = "" },
	} {
		cfg := good
		mutate(&cfg)
		if _, err := Handler(cfg); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestInvalidScopeIs401(t *testing.T) {
	// Core refuses a token with no scope to exchange. Signing in again is what fixes it.
	core := newFakeCore(t)
	core.exchange = func(w http.ResponseWriter, _ map[string]string) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_scope"}`))
	}
	srv := newHost(t, core)
	if res := initialize(t, srv, clientToken); res.status != http.StatusUnauthorized {
		t.Fatalf("status = %d", res.status)
	}
}

func TestARedirectFromCoreIsNotFollowed(t *testing.T) {
	// The exchange's body holds the client secret and the user's token; a 307 or
	// 308 would send it again to wherever the redirect points.
	var leaked bool
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		leaked = r.PostForm.Get("client_secret") != "" || r.PostForm.Get("subject_token") != ""
	}))
	t.Cleanup(elsewhere.Close)
	core := newFakeCore(t)
	core.exchange = func(w http.ResponseWriter, _ map[string]string) {
		w.Header().Set("Location", elsewhere.URL+"/oauth/token")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	if _, err := e.exchange(t.Context(), clientToken, testAddr); !errors.Is(err, errUnavailable) {
		t.Errorf("err = %v, want errUnavailable", err)
	}
	if leaked {
		t.Error("the exchange was sent on to the redirect's target")
	}
}

func TestConcurrentExchangesForOneTokenAreCoalesced(t *testing.T) {
	// Core holds every exchange until the test releases it, so all callers
	// overlap: shared, they make one exchange; not shared, one each.
	var arrived atomic.Int32
	release := make(chan struct{})
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		arrived.Add(1)
		<-release
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":300,"scope":"envs:read"}`, coreToken)
	}))
	t.Cleanup(core.Close)
	e := newExchanger(core.URL+"/oauth/token", core.URL+"/api", "host", secret)

	const callers = 10
	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
				t.Error(err)
			}
		})
	}
	// Wait for every caller's exchange to reach core, or long enough for all of
	// them to have joined the first one.
	for deadline := time.Now().Add(500 * time.Millisecond); arrived.Load() < callers && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	close(release)
	wg.Wait()
	if n := arrived.Load(); n != 1 {
		t.Errorf("exchanges = %d, want 1", n)
	}
}

// countingTransport counts the requests it sends; fail makes each one a
// transport error, as when core can't be reached.
type countingTransport struct {
	n    atomic.Int64
	fail bool
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.n.Add(1)
	if c.fail {
		return nil, errors.New("connection refused")
	}
	return http.DefaultTransport.RoundTrip(req)
}

func TestCoreIsNotAskedAgainRightAfterItWasUnavailable(t *testing.T) {
	// Each of these holds for every token, so none is worth asking core again
	// for during the pause.
	for name, tc := range map[string]struct {
		respond     func(http.ResponseWriter, map[string]string)
		unreachable bool
		pause       time.Duration
	}{
		"502": {respond: func(w http.ResponseWriter, _ map[string]string) { w.WriteHeader(http.StatusBadGateway) }},
		"invalid_client": {respond: func(w http.ResponseWriter, _ map[string]string) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
		}},
		"invalid_target": {respond: func(w http.ResponseWriter, _ map[string]string) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_target"}`))
		}},
		"unreachable": {unreachable: true},
		// Every request from this server is under core's 8KB host limit, so a 429
		// is core's ceiling for all of them, not one client's.
		"429": {respond: func(w http.ResponseWriter, _ map[string]string) { w.WriteHeader(http.StatusTooManyRequests) }},
		"429 with Retry-After": {pause: 30 * time.Second, respond: func(w http.ResponseWriter, _ map[string]string) {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
		}},
	} {
		t.Run(name, func(t *testing.T) {
			core := newFakeCore(t)
			core.exchange = tc.respond
			e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
			transport := &countingTransport{fail: tc.unreachable}
			e.http.Transport = transport
			now := time.Now()
			e.now = func() time.Time { return now }
			pause := tc.pause
			if pause == 0 {
				pause = unavailableBackoff
			}
			for _, subject := range []string{clientToken, "another-token"} {
				if _, err := e.exchange(t.Context(), subject, testAddr); !errors.Is(err, errUnavailable) {
					t.Fatalf("err = %v, want errUnavailable", err)
				}
			}
			if n := transport.n.Load(); n != 1 {
				t.Errorf("exchanges during the backoff = %d, want 1", n)
			}
			now = now.Add(pause - time.Second)
			_, _ = e.exchange(t.Context(), clientToken, testAddr)
			if n := transport.n.Load(); n != 1 {
				t.Errorf("exchanges a second before the pause ends = %d, want 1", n)
			}
			now = now.Add(time.Second)
			_, _ = e.exchange(t.Context(), clientToken, testAddr)
			if n := transport.n.Load(); n != 2 {
				t.Errorf("exchanges after the backoff = %d, want 2", n)
			}
		})
	}
}

func TestARevokedTokenIsNotServedFromCacheAtRefresh(t *testing.T) {
	// The cached core token still has a minute left, but core now refuses the
	// client's token: the connection was revoked, so it's a 401, not the cache.
	core := newFakeCore(t)
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
		t.Fatal(err)
	}
	core.exchange = func(w http.ResponseWriter, _ map[string]string) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}
	now = now.Add(4 * time.Minute)
	if got, err := e.exchange(t.Context(), clientToken, testAddr); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("got %+v, %v; want ErrInvalidToken", got, err)
	}
	if _, ok := e.cache[sha256.Sum256([]byte(clientToken))]; ok {
		t.Error("the revoked token's core token is still cached")
	}
}

func TestARefusedTokenIsNotExchangedAgainForAMinute(t *testing.T) {
	// Junk tokens count against core's ceiling for this server; a client
	// resending one mustn't spend it again.
	core := newFakeCore(t)
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	for range 3 {
		if _, err := e.exchange(t.Context(), "junk", testAddr); !errors.Is(err, auth.ErrInvalidToken) {
			t.Fatalf("err = %v, want ErrInvalidToken", err)
		}
	}
	if core.exchanges != 1 {
		t.Errorf("exchanges for a refused token = %d, want 1", core.exchanges)
	}
	now = now.Add(refusedFor)
	_, _ = e.exchange(t.Context(), "junk", testAddr)
	if core.exchanges != 2 {
		t.Errorf("exchanges after %s = %d, want 2", refusedFor, core.exchanges)
	}
}

func TestAFullCacheIsNotScannedOnEveryInsert(t *testing.T) {
	// With tokens expiring one at a time, each scan of a full cache frees one
	// entry: the next insert would scan all of it again. Within sweepEvery it
	// evicts a batch instead.
	core := newFakeCore(t)
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	e.cache[sha256.Sum256([]byte("first"))] = exchanged{expires: now.Add(-time.Second)}
	e.cache[sha256.Sum256([]byte("second"))] = exchanged{expires: now.Add(time.Second)}
	fillCache(e, now)
	e.store(sha256.Sum256([]byte("a")), exchanged{expires: now.Add(time.Hour)})
	now = now.Add(2 * time.Second)
	e.store(sha256.Sum256([]byte("b")), exchanged{expires: now.Add(time.Hour)})
	if len(e.cache) > maxCached*9/10+1 {
		t.Errorf("cache size = %d after two inserts into a full cache: it was scanned again instead of evicting a batch", len(e.cache))
	}
}

func TestAFullCacheEvictsExpiredEntriesFirst(t *testing.T) {
	core := newFakeCore(t)
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	expired := sha256.Sum256([]byte("expired"))
	soonest := sha256.Sum256([]byte("soonest"))
	e.cache[expired] = exchanged{expires: now.Add(-time.Second)}
	e.cache[soonest] = exchanged{expires: now.Add(time.Minute)}
	fillCache(e, now)
	if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.cache[expired]; ok {
		t.Error("an expired entry was kept")
	}
	if _, ok := e.cache[soonest]; !ok {
		t.Error("a live entry was dropped while an expired one made room")
	}
}

func TestAFullCacheOfLiveTokensEvictsTheSoonestToExpire(t *testing.T) {
	core := newFakeCore(t)
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	soonest := sha256.Sum256([]byte("soonest"))
	latest := sha256.Sum256([]byte("latest"))
	e.cache[soonest] = exchanged{expires: now.Add(time.Minute)}
	e.cache[latest] = exchanged{expires: now.Add(time.Hour)}
	fillCache(e, now)
	if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.cache[soonest]; ok {
		t.Error("the entry closest to expiry was kept")
	}
	if _, ok := e.cache[latest]; !ok {
		t.Error("the entry furthest from expiry was dropped")
	}
	if len(e.cache) >= maxCached {
		t.Errorf("cache size = %d, want under %d", len(e.cache), maxCached)
	}
}

// fillCache fills e's cache to maxCached with tokens that expire in 2 minutes.
func fillCache(e *exchanger, now time.Time) {
	for i := len(e.cache); i < maxCached; i++ {
		e.cache[sha256.Sum256(fmt.Appendf(nil, "live-%d", i))] = exchanged{expires: now.Add(2 * time.Minute)}
	}
}

func TestOtherAPIErrorsKeepTheConnection(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			core := newFakeCore(t)
			core.apiStatus = status
			srv := newHost(t, core)
			res := call(t, srv, clientToken, "tools/call", map[string]any{"name": "get_environments", "arguments": map[string]any{}})
			text, isError := toolText(t, res)
			if !isError || text == reconnectMessage || text == unavailableMessage {
				t.Errorf("result = %q (isError %v)", text, isError)
			}
			call(t, srv, clientToken, "tools/list", map[string]any{})
			if core.exchanges != 1 {
				t.Errorf("exchanges = %d, want 1: the cached token was dropped", core.exchanges)
			}
		})
	}
}

func TestTheServerIsBuiltOncePerRequest(t *testing.T) {
	core := newFakeCore(t)
	srv := newHost(t, core)
	before := serversBuilt.Load()
	call(t, srv, clientToken, "tools/list", map[string]any{})
	if built := serversBuilt.Load() - before; built != 1 {
		t.Errorf("servers built = %d, want 1", built)
	}
}

func TestCheckConfig(t *testing.T) {
	for name, tc := range map[string]struct {
		respond func(http.ResponseWriter, map[string]string)
		wantErr bool
	}{
		// Core checks the client and the resource before the subject token, so
		// refusing a token that can't exist means both are right.
		"invalid_grant": {respond: func(w http.ResponseWriter, _ map[string]string) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		}},
		"invalid_client": {wantErr: true, respond: func(w http.ResponseWriter, _ map[string]string) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
		}},
		"invalid_target": {wantErr: true, respond: func(w http.ResponseWriter, _ map[string]string) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_target"}`))
		}},
		// Core being down mustn't stop the server: it answers with a retry hint until core is back.
		"core down": {respond: func(w http.ResponseWriter, _ map[string]string) { w.WriteHeader(http.StatusBadGateway) }},
	} {
		t.Run(name, func(t *testing.T) {
			core := newFakeCore(t)
			core.exchange = tc.respond
			err := CheckConfig(t.Context(), Config{
				ResourceURL:  "https://mcp.example.test/mcp",
				SiteURL:      core.server.URL,
				ClientID:     "host",
				ClientSecret: secret,
			})
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, want error %v", err, tc.wantErr)
			}
		})
	}
}

func TestAnOversizedTokenIs401WithoutAskingCore(t *testing.T) {
	// Core rate-limits a token request over 8KB as if it came from anyone, so a
	// junk token that size must never reach it.
	core := newFakeCore(t)
	srv := newHost(t, core)
	if res := initialize(t, srv, strings.Repeat("x", maxTokenBytes+1)); res.status != http.StatusUnauthorized {
		t.Fatalf("status = %d", res.status)
	}
	if core.exchanges != 0 {
		t.Errorf("exchanges = %d, want 0", core.exchanges)
	}
}

func TestATokenThatEncodesPast4KBIs401WithoutAskingCore(t *testing.T) {
	// Core measures the encoded form against its 8KB host limit, and '/' encodes
	// to 3 bytes: 2000 of them are 6000 bytes on the wire.
	core := newFakeCore(t)
	srv := newHost(t, core)
	if res := initialize(t, srv, strings.Repeat("/", 2000)); res.status != http.StatusUnauthorized {
		t.Fatalf("status = %d", res.status)
	}
	if core.exchanges != 0 {
		t.Errorf("exchanges = %d, want 0", core.exchanges)
	}
}

func TestARefusalForOneTokenDoesNotPauseOthers(t *testing.T) {
	// A refusal about the request a client chose to send, not about core,
	// mustn't stop everyone else's exchanges.
	core := newFakeCore(t)
	exchange := core.exchange
	core.exchange = func(w http.ResponseWriter, form map[string]string) {
		if form["subject_token"] == "junk" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_request"}`))
			return
		}
		exchange(w, form)
	}
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	if _, err := e.exchange(t.Context(), "junk", testAddr); !errors.Is(err, errUnavailable) {
		t.Fatalf("err = %v, want errUnavailable", err)
	}
	if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
		t.Errorf("a 429 for one token paused another: %v", err)
	}
}

func TestACachedTokenIsUsedUntilItExpiresWhileCoreIsDown(t *testing.T) {
	core := newFakeCore(t)
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
		t.Fatal(err)
	}
	core.exchange = func(w http.ResponseWriter, _ map[string]string) { w.WriteHeader(http.StatusBadGateway) }
	// Inside the margin, so it's exchanged again; core is down, and the token
	// still has a minute left.
	now = now.Add(4 * time.Minute)
	for range 2 { // the second is during the pause
		got, err := e.exchange(t.Context(), clientToken, testAddr)
		if err != nil || got.token != coreToken {
			t.Fatalf("got %+v, %v; want the cached token", got, err)
		}
	}
	now = now.Add(time.Minute)
	if _, err := e.exchange(t.Context(), clientToken, testAddr); !errors.Is(err, errUnavailable) {
		t.Errorf("err = %v after the token expired, want errUnavailable", err)
	}
}

func TestOnlyAPOSTBuildsTheServer(t *testing.T) {
	// Stateless Streamable HTTP answers anything but a POST with 405. GET is a
	// client opening a stream, often on every reconnect.
	core := newFakeCore(t)
	srv := newHost(t, core)
	before := serversBuilt.Load()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
	if built := serversBuilt.Load() - before; built != 0 {
		t.Errorf("servers built for a GET = %d, want 0", built)
	}
}

func TestAPublicHostOverLoopbackIsServed(t *testing.T) {
	// Behind a proxy on the same machine (a sidecar, a tunnel) requests arrive on
	// loopback with the public Host. Every request needs a bearer token, so the
	// SDK's DNS-rebinding guard for local servers mustn't refuse them.
	core := newFakeCore(t)
	srv := newHost(t, core)
	payload := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(payload))
	req.Host = "mcp.example.test"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+clientToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestResourceURLPaths(t *testing.T) {
	for _, tc := range []struct{ resource, endpoint, metadata string }{
		{"https://mcp.example.test/", "/", "https://mcp.example.test/.well-known/oauth-protected-resource"},
		{"https://mcp.example.test/mcp", "/mcp", "https://mcp.example.test/.well-known/oauth-protected-resource/mcp"},
		{"https://mcp.example.test/mcp/", "/mcp/", "https://mcp.example.test/.well-known/oauth-protected-resource/mcp"},
	} {
		t.Run(tc.resource, func(t *testing.T) {
			core := newFakeCore(t)
			handler, err := Handler(Config{ResourceURL: tc.resource, SiteURL: core.server.URL, ClientID: "host", ClientSecret: secret})
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(handler)
			t.Cleanup(srv.Close)

			req, _ := http.NewRequest(http.MethodPost, srv.URL+tc.endpoint, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if want := `Bearer resource_metadata="` + tc.metadata + `"`; resp.Header.Get("WWW-Authenticate") != want {
				t.Errorf("WWW-Authenticate = %q, want %q", resp.Header.Get("WWW-Authenticate"), want)
			}
			metaPath := strings.TrimPrefix(tc.metadata, "https://mcp.example.test")
			resp, err = http.Get(srv.URL + metaPath)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("GET %s = %d", metaPath, resp.StatusCode)
			}
		})
	}
}

func TestTokensCoreRefusesFromOneAddressStopReachingCore(t *testing.T) {
	core := newFakeCore(t)
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }

	for i := range maxRefusals {
		if _, err := e.exchange(t.Context(), fmt.Sprintf("junk-%d", i), testAddr); !errors.Is(err, auth.ErrInvalidToken) {
			t.Fatalf("junk %d: err = %v, want invalid token", i, err)
		}
	}
	if core.exchanges != maxRefusals {
		t.Fatalf("core exchanges = %d, want %d", core.exchanges, maxRefusals)
	}
	// Past the limit, nothing new from that address reaches core, a good token included.
	for _, subject := range []string{"junk-next", clientToken} {
		if _, err := e.exchange(t.Context(), subject, testAddr); !errors.Is(err, errThrottled) || errors.Is(err, errCoreDown) {
			t.Errorf("%s: err = %v, want throttled without pausing everyone", subject, err)
		}
	}
	if core.exchanges != maxRefusals {
		t.Errorf("core exchanges = %d after the limit, want %d", core.exchanges, maxRefusals)
	}
	// Other addresses aren't held back.
	if _, err := e.exchange(t.Context(), clientToken, "192.0.2.2"); err != nil {
		t.Fatalf("another address: %v", err)
	}
	// And the address may ask again once its window ends.
	now = now.Add(refusalWindow)
	if _, err := e.exchange(t.Context(), "junk-later", testAddr); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("after the window: err = %v, want invalid token", err)
	}
}

func TestATokenRefusedAgainFromMemoryDoesNotCount(t *testing.T) {
	core := newFakeCore(t)
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	for range maxRefusals * 2 {
		_, _ = e.exchange(t.Context(), "junk", testAddr)
	}
	if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
		t.Errorf("one refused token sent again and again throttled its address: %v", err)
	}
}

func TestAThrottledAddressKeepsItsCachedTokens(t *testing.T) {
	core := newFakeCore(t)
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
		t.Fatal(err)
	}
	for i := range maxRefusals {
		_, _ = e.exchange(t.Context(), fmt.Sprintf("junk-%d", i), testAddr)
	}
	// Due for a new exchange, but still unexpired: served from the cache.
	now = now.Add(5*time.Minute - expiryMargin)
	if got, err := e.exchange(t.Context(), clientToken, testAddr); err != nil || got.token != coreToken {
		t.Errorf("got %q, %v; want the cached token", got.token, err)
	}
}

// Not a 401, which sends the client through sign-in for nothing, and not a pass
// through bearer auth: a token core never accepted reaches no MCP server.
func TestAThrottledRequestIsA503BeforeAnyServerIsBuilt(t *testing.T) {
	core := newFakeCore(t)
	srv := newHost(t, core)
	for i := range maxRefusals {
		if res := initialize(t, srv, fmt.Sprintf("junk-%d", i)); res.status != http.StatusUnauthorized {
			t.Fatalf("junk %d: status = %d", i, res.status)
		}
	}
	built := serversBuilt.Load()
	for _, token := range []string{"junk-next", clientToken} {
		res := initialize(t, srv, token)
		if res.status != http.StatusServiceUnavailable {
			t.Fatalf("%s: status = %d, want 503", token, res.status)
		}
		if res.header.Get("Retry-After") != "60" {
			t.Errorf("%s: Retry-After = %q", token, res.header.Get("Retry-After"))
		}
	}
	if n := serversBuilt.Load() - built; n != 0 {
		t.Errorf("%d servers built for throttled requests", n)
	}
}

func TestClientAddress(t *testing.T) {
	for name, tc := range map[string]struct {
		behindProxy bool
		forwarded   []string
		want        string
	}{
		"direct":                              {want: "192.0.2.9"},
		"direct ignores X-Forwarded-For":      {forwarded: []string{"198.51.100.1"}, want: "192.0.2.9"},
		"behind a proxy":                      {behindProxy: true, forwarded: []string{"198.51.100.1"}, want: "198.51.100.1"},
		"the proxy's entry, not the client's": {behindProxy: true, forwarded: []string{"203.0.113.5, 198.51.100.1"}, want: "198.51.100.1"},
		"the last header":                     {behindProxy: true, forwarded: []string{"203.0.113.5", "198.51.100.1"}, want: "198.51.100.1"},
		"behind a proxy without the header":   {behindProxy: true, want: "192.0.2.9"},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			req.RemoteAddr = "192.0.2.9:41234"
			for _, v := range tc.forwarded {
				req.Header.Add("X-Forwarded-For", v)
			}
			if got := (&host{behindProxy: tc.behindProxy}).clientAddress(req); got != tc.want {
				t.Errorf("clientAddress = %q, want %q", got, tc.want)
			}
		})
	}
}

func get(t *testing.T, srv *httptest.Server, path string) int {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// eventually polls cond for up to two seconds: /readyz checks with core in the background.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestNotReadyWhileCoreRefusesTheServer(t *testing.T) {
	core := newFakeCore(t)
	refuse := func(w http.ResponseWriter, _ map[string]string) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}
	core.mu.Lock()
	core.exchange = refuse
	core.mu.Unlock()
	srv := newHost(t, core)

	// No traffic needed: /readyz asks core itself. It stays unready once core
	// has answered, not only while the first check runs.
	_ = get(t, srv, "/readyz")
	eventually(t, "core to be asked", func() bool { core.mu.Lock(); defer core.mu.Unlock(); return core.exchanges > 0 })
	for deadline := time.Now().Add(200 * time.Millisecond); time.Now().Before(deadline); {
		if status := get(t, srv, "/readyz"); status != http.StatusServiceUnavailable {
			t.Fatalf("/readyz = %d while core refuses the server", status)
		}
	}
	if status := get(t, srv, "/healthz"); status != http.StatusOK {
		t.Errorf("/healthz = %d: a restart can't fix the secret", status)
	}
}

func TestReadyAgainOnceCoreAcceptsTheServer(t *testing.T) {
	core := newFakeCore(t)
	good := core.exchange
	core.exchange = func(w http.ResponseWriter, _ map[string]string) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	var mu sync.Mutex
	now := time.Now()
	e.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }

	_, _ = e.exchange(t.Context(), clientToken, testAddr)
	if !e.notReady() {
		t.Fatal("an invalid_client answer left the server ready")
	}
	core.mu.Lock()
	core.exchange = good
	core.mu.Unlock()

	e.recheck()
	eventually(t, "the recheck", func() bool { return !e.notReady() })
	core.mu.Lock()
	asked := core.exchanges
	core.mu.Unlock()
	// Confirmed: no more checks.
	mu.Lock()
	now = now.Add(recheckEvery)
	mu.Unlock()
	e.recheck()
	time.Sleep(50 * time.Millisecond)
	core.mu.Lock()
	defer core.mu.Unlock()
	if core.exchanges != asked {
		t.Errorf("checked again after core accepted the server")
	}
}

func TestRechecksAreSpacedOut(t *testing.T) {
	core := newFakeCore(t)
	core.exchange = func(w http.ResponseWriter, _ map[string]string) { w.WriteHeader(http.StatusBadGateway) }
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	count := func() int { core.mu.Lock(); defer core.mu.Unlock(); return core.exchanges }

	e.recheck()
	eventually(t, "the first check", func() bool { return count() == 1 })
	e.recheck()
	e.recheck()
	time.Sleep(50 * time.Millisecond)
	if n := count(); n != 1 {
		t.Fatalf("core asked %d times within %s", n, recheckEvery)
	}
	if e.notReady() {
		t.Error("core being down made the server unready")
	}
}

// Core's secret rotated before the release: the pods core accepted stay ready,
// so the tokens they hold keep working until the release catches up.
func TestAServerCoreAcceptedStaysReadyWhenCoreLaterRefusesIt(t *testing.T) {
	core := newFakeCore(t)
	srv := newHost(t, core)
	if res := initialize(t, srv, clientToken); res.status != http.StatusOK {
		t.Fatalf("initialize status = %d", res.status)
	}
	core.mu.Lock()
	core.exchange = func(w http.ResponseWriter, _ map[string]string) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}
	core.mu.Unlock()
	_ = initialize(t, srv, "another-token")
	if status := get(t, srv, "/readyz"); status != http.StatusOK {
		t.Errorf("/readyz = %d after core refused a server it had accepted", status)
	}
}

// A burst of one token is one answer from core, so one refusal.
func TestConcurrentRefusalsOfOneTokenCountOnce(t *testing.T) {
	core := newFakeCore(t)
	release := make(chan struct{})
	refuse := core.exchange
	core.exchange = func(w http.ResponseWriter, form map[string]string) {
		<-release
		refuse(w, form)
	}
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	var wg sync.WaitGroup
	for range maxRefusals {
		wg.Go(func() { _, _ = e.exchange(t.Context(), "junk", testAddr) })
	}
	time.Sleep(100 * time.Millisecond) // gstack-timing-ok: lets callers join the flight; a slow one is refused from memory, which doesn't count either
	close(release)
	wg.Wait()
	e.mu.Lock()
	defer e.mu.Unlock()
	if got := e.refusals[testAddr].count; got != 1 {
		t.Errorf("refusals = %d for one token core refused once, want 1", got)
	}
}

func TestAThrottledAddressStillExchangesATokenCoreAcceptedBefore(t *testing.T) {
	core := newFakeCore(t)
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5*time.Minute + time.Second) // the exchanged token has lapsed
	for i := range maxRefusals {
		_, _ = e.exchange(t.Context(), fmt.Sprintf("junk-%d", i), testAddr)
	}
	if !e.overRefusals(testAddr) {
		t.Fatal("the address isn't throttled")
	}
	if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
		t.Errorf("a token core accepted before was throttled: %v", err)
	}
}

func TestBehindAProxyTheLimitIsPerForwardedAddress(t *testing.T) {
	core := newFakeCore(t)
	handler, err := Handler(Config{
		ResourceURL: "https://mcp.example.test/mcp", SiteURL: core.server.URL,
		ClientID: "host", ClientSecret: secret, BehindProxy: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	send := func(token, forwarded string) int {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Forwarded-For", forwarded)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	for i := range maxRefusals {
		send(fmt.Sprintf("junk-%d", i), "198.51.100.1")
	}
	if status := send("junk-next", "198.51.100.1"); status != http.StatusServiceUnavailable {
		t.Errorf("the address that reached the limit: status = %d, want 503", status)
	}
	if status := send("junk-next", "198.51.100.2"); status != http.StatusUnauthorized {
		t.Errorf("another address: status = %d, want 401", status)
	}
}

func TestNotReadyUntilTheFirstCheckAnswers(t *testing.T) {
	core := newFakeCore(t)
	release := make(chan struct{})
	answer := core.exchange
	core.exchange = func(w http.ResponseWriter, form map[string]string) {
		<-release
		answer(w, form)
	}
	srv := newHost(t, core)
	if status := get(t, srv, "/readyz"); status != http.StatusServiceUnavailable {
		t.Errorf("/readyz = %d before core answered the first check", status)
	}
	close(release)
	eventually(t, "/readyz to pass", func() bool { return get(t, srv, "/readyz") == http.StatusOK })
}

func TestRechecksRunAgainAfterTheInterval(t *testing.T) {
	core := newFakeCore(t)
	good := core.exchange
	core.exchange = func(w http.ResponseWriter, _ map[string]string) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	var mu sync.Mutex
	now := time.Now()
	e.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	count := func() int { core.mu.Lock(); defer core.mu.Unlock(); return core.exchanges }

	e.recheck()
	eventually(t, "the first check", func() bool { return count() == 1 })
	core.mu.Lock()
	core.exchange = good
	core.mu.Unlock()
	mu.Lock()
	now = now.Add(recheckEvery)
	mu.Unlock()
	e.recheck()
	eventually(t, "the second check", func() bool { return !e.notReady() })
}

func TestAFullRefusalsMapStaysBounded(t *testing.T) {
	e := newExchanger("http://core.invalid/oauth/token", "http://core.invalid/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	fill := func(start time.Time) {
		clear(e.refusals)
		for i := range maxCached {
			e.refusals[fmt.Sprintf("addr-%d", i)] = refusalCount{start: start, count: 1}
		}
	}

	fill(now.Add(-2 * refusalWindow))
	e.refuse(sha256.Sum256([]byte("a")), "new-address")
	if len(e.refusals) != 1 {
		t.Errorf("expired windows left %d entries, want the new one", len(e.refusals))
	}
	fill(now)
	e.refuse(sha256.Sum256([]byte("b")), "another-address")
	if len(e.refusals) != 1 {
		t.Errorf("a map full of live windows left %d entries, want the new one", len(e.refusals))
	}
}

// throttledWithLapsedToken leaves clientToken cached and lapsed, on an address
// over its limit.
func throttledWithLapsedToken(t *testing.T, core *fakeCore) (*exchanger, *time.Time) {
	t.Helper()
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5*time.Minute + time.Second)
	for i := range maxRefusals {
		_, _ = e.exchange(t.Context(), fmt.Sprintf("junk-%d", i), testAddr)
	}
	if !e.overRefusals(testAddr) {
		t.Fatal("the address isn't throttled")
	}
	return e, &now
}

// Core failing says nothing about the token: it stays known.
func TestACoreOutageDoesNotForgetAThrottledAddressesKnownToken(t *testing.T) {
	core := newFakeCore(t)
	e, now := throttledWithLapsedToken(t, core)
	good := core.exchange
	core.exchange = func(w http.ResponseWriter, _ map[string]string) { w.WriteHeader(http.StatusBadGateway) }
	if _, err := e.exchange(t.Context(), clientToken, testAddr); !errors.Is(err, errCoreDown) {
		t.Fatalf("err = %v, want core down", err)
	}
	core.exchange = good
	*now = now.Add(unavailableBackoff)
	if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
		t.Errorf("after the outage: %v", err)
	}
}

// After an API 401 the next request has to reach core, for the 401 that sends
// the client to sign in, throttled or not.
func TestAnExchangedTokenTheAPIRefusedStillReachesCoreWhenThrottled(t *testing.T) {
	core := newFakeCore(t)
	e, _ := throttledWithLapsedToken(t, core)
	if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
		t.Fatal(err)
	}
	(&host{exchanger: e}).toolError("get_environments", clientToken, &requests.APIError{StatusCode: http.StatusUnauthorized})
	core.exchange = func(w http.ResponseWriter, _ map[string]string) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}
	if _, err := e.exchange(t.Context(), clientToken, testAddr); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("err = %v, want the refusal that sends the client to sign in", err)
	}
}

func TestAThrottledAddressStillRefreshesATokenNearExpiry(t *testing.T) {
	core := newFakeCore(t)
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5*time.Minute - expiryMargin + time.Second)
	for i := range maxRefusals {
		_, _ = e.exchange(t.Context(), fmt.Sprintf("junk-%d", i), testAddr)
	}
	asked := core.exchanges
	if _, err := e.exchange(t.Context(), clientToken, testAddr); err != nil {
		t.Fatal(err)
	}
	if core.exchanges != asked+1 {
		t.Errorf("exchanges = %d, want a refresh after %d", core.exchanges, asked)
	}
}
