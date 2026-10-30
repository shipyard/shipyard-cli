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

	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/mcp/tools"
)

const (
	clientToken = "client-mcp-token"
	coreToken   = "exchanged-core-token"
	secret      = "host-secret"
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
		if _, err := e.exchange(t.Context(), clientToken); err != nil {
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
	if _, err := e.exchange(t.Context(), clientToken); !errors.Is(err, errUnavailable) {
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
			if _, err := e.exchange(t.Context(), clientToken); err != nil {
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

func TestCoreIsNotAskedAgainRightAfterItWasUnavailable(t *testing.T) {
	core := newFakeCore(t)
	core.exchange = func(w http.ResponseWriter, _ map[string]string) { w.WriteHeader(http.StatusBadGateway) }
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	for _, subject := range []string{clientToken, "another-token"} {
		if _, err := e.exchange(t.Context(), subject); !errors.Is(err, errUnavailable) {
			t.Fatalf("err = %v, want errUnavailable", err)
		}
	}
	if core.exchanges != 1 {
		t.Errorf("exchanges during the backoff = %d, want 1", core.exchanges)
	}
	now = now.Add(unavailableBackoff)
	_, _ = e.exchange(t.Context(), clientToken)
	if core.exchanges != 2 {
		t.Errorf("exchanges after the backoff = %d, want 2", core.exchanges)
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
	if _, err := e.exchange(t.Context(), clientToken); err != nil {
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
	if _, err := e.exchange(t.Context(), clientToken); err != nil {
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

func TestARefusalForOneTokenDoesNotPauseOthers(t *testing.T) {
	// A 4xx is about the request a client chose to send, not about core: it
	// mustn't stop everyone else's exchanges.
	core := newFakeCore(t)
	exchange := core.exchange
	core.exchange = func(w http.ResponseWriter, form map[string]string) {
		if form["subject_token"] == "junk" {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		exchange(w, form)
	}
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	if _, err := e.exchange(t.Context(), "junk"); !errors.Is(err, errUnavailable) {
		t.Fatalf("err = %v, want errUnavailable", err)
	}
	if _, err := e.exchange(t.Context(), clientToken); err != nil {
		t.Errorf("a 429 for one token paused another: %v", err)
	}
}

func TestACachedTokenIsUsedUntilItExpiresWhileCoreIsDown(t *testing.T) {
	core := newFakeCore(t)
	e := newExchanger(core.server.URL+"/oauth/token", core.server.URL+"/api", "host", secret)
	now := time.Now()
	e.now = func() time.Time { return now }
	if _, err := e.exchange(t.Context(), clientToken); err != nil {
		t.Fatal(err)
	}
	core.exchange = func(w http.ResponseWriter, _ map[string]string) { w.WriteHeader(http.StatusBadGateway) }
	// Inside the margin, so it's exchanged again; core is down, and the token
	// still has a minute left.
	now = now.Add(4 * time.Minute)
	for range 2 { // the second is during the pause
		got, err := e.exchange(t.Context(), clientToken)
		if err != nil || got.token != coreToken {
			t.Fatalf("got %+v, %v; want the cached token", got, err)
		}
	}
	now = now.Add(time.Minute)
	if _, err := e.exchange(t.Context(), clientToken); !errors.Is(err, errUnavailable) {
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
