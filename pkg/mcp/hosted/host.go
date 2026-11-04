// Package hosted serves Shipyard's MCP tools over stateless Streamable HTTP for
// clients that connect with OAuth, instead of over stdio with the CLI's own token.
//
// Each request carries the user's MCP token. The server trades it for a
// short-lived core API token (RFC 8693) and builds that request's tools with it;
// the client's token is never sent to the API.
package hosted

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/shipyard/shipyard-cli/pkg/client"
	mcperrors "github.com/shipyard/shipyard-cli/pkg/mcp/errors"
	"github.com/shipyard/shipyard-cli/pkg/mcp/tools"
	"github.com/shipyard/shipyard-cli/pkg/requests"
	"github.com/shipyard/shipyard-cli/version"
)

//go:embed instructions.md
var instructions string

// serversBuilt counts the MCP servers built, one per request.
var serversBuilt atomic.Int64

// Logger is the hosted server's own log, for its operator: one line per tool
// call, with its outcome, duration and org, and none of the user's data. The
// tools' request logs follow --verbose: they carry users' params and the API's
// error bodies.
var Logger = log.New(os.Stderr, "", log.LstdFlags|log.LUTC)

const (
	protectedResourcePath = "/.well-known/oauth-protected-resource"

	// Keys in auth.TokenInfo.Extra.
	extraCoreToken   = "core_token"
	extraSubject     = "subject_token"
	extraOrg         = "org"
	extraUnavailable = "exchange_unavailable"
	extraThrottled   = "exchange_throttled"

	unavailableMessage = "Shipyard is temporarily unavailable. Try again in a minute."
	reconnectMessage   = "This Shipyard connection is no longer valid. Reconnect Shipyard in your agent, then try again."
)

// Config is what a hosted server needs. All of it comes from the deployment.
type Config struct {
	// ResourceURL is this server's MCP endpoint, the audience of the tokens it
	// accepts, e.g. https://mcp.example.com/mcp. Core's OAUTH_MCP_RESOURCE must match.
	ResourceURL string
	// SiteURL is core's base URL: the authorization server, and /api is the API.
	SiteURL string
	// ClientID and ClientSecret are this server's confidential client at core
	// (OAUTH_MCP_HOST_CLIENT_ID / OAUTH_MCP_HOST_CLIENT_SECRET).
	ClientID     string
	ClientSecret string
	// BehindProxy takes each request's client address from the last entry of
	// X-Forwarded-For, which the proxy in front (the ingress) appends. Set it
	// only behind one: without it a client could name any address it likes.
	BehindProxy bool
}

func (c Config) validate() error {
	resource, err := url.Parse(c.ResourceURL)
	if err != nil || resource.Scheme == "" || resource.Host == "" {
		return fmt.Errorf("the resource URL %q is not an absolute URL", c.ResourceURL)
	}
	site, err := url.Parse(c.SiteURL)
	if err != nil || site.Scheme == "" || site.Host == "" {
		return fmt.Errorf("the site URL %q is not an absolute URL", c.SiteURL)
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		return errors.New("the client id and secret are required")
	}
	return nil
}

// Handler serves the MCP endpoint at the resource URL's path, its protected
// resource metadata (RFC 9728), /healthz and /readyz.
func Handler(cfg Config) (http.Handler, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	resource, _ := url.Parse(cfg.ResourceURL)
	site := strings.TrimSuffix(cfg.SiteURL, "/")
	mcpPath := resource.Path
	if mcpPath == "" {
		mcpPath = "/"
	}

	h := &host{
		exchanger:   newExchanger(site+"/oauth/token", site+"/api", cfg.ClientID, cfg.ClientSecret),
		behindProxy: cfg.BehindProxy,
	}

	// RFC 9728 puts the metadata for a resource with a path under that path. Some
	// clients only look at the root, so both answer.
	metadataURL := resource.Scheme + "://" + resource.Host + protectedResourcePath + strings.TrimSuffix(resource.Path, "/")
	metadata := auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource:               cfg.ResourceURL,
		AuthorizationServers:   []string{site},
		ScopesSupported:        tools.KnownScopes,
		BearerMethodsSupported: []string{"header"},
	})

	streamable := mcp.NewStreamableHTTPHandler(requestServer, &mcp.StreamableHTTPOptions{
		Stateless:           true,
		JSONResponse:        true,
		MaxRequestBodyBytes: 1 << 20,
		// The guard is for local servers without auth, against DNS rebinding.
		// Every request here needs a bearer token, and behind a proxy on the same
		// machine (a sidecar, a tunnel) requests arrive on loopback with the
		// public Host, which the guard would refuse with a 403.
		DisableLocalhostProtection: true,
	})

	mux := http.NewServeMux()
	mux.Handle(protectedResourcePath, metadata)
	if mcpPath != "/" {
		mux.Handle(protectedResourcePath+strings.TrimSuffix(mcpPath, "/"), metadata)
	}
	// Liveness: the process answers. Restarting can't fix anything /readyz reports.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	// Readiness: not until the first check of this server's client and site URL
	// has answered, nor while core refuses them, so a release with the wrong
	// secret fails its rollout. Core being down is still ready: every replica
	// would be down with it, and tool calls answer with a retry hint meanwhile.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		h.exchanger.recheck()
		if h.exchanger.notReady() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle(mcpPath, auth.RequireBearerToken(h.verify, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: metadataURL,
	})(h.withServer(streamable)))
	return mux, nil
}

// CheckConfig asks core to exchange a token that can't exist. Core checks this
// server's client and the API it asks for (the site URL) before the subject
// token, so it refuses the token itself only when both are right. That doesn't
// cover core's OAUTH_MCP_RESOURCE. Core being unreachable isn't an error:
// the server answers with a retry hint until it's back.
func CheckConfig(ctx context.Context, cfg Config) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	site := strings.TrimSuffix(cfg.SiteURL, "/")
	_, err := newExchanger(site+"/oauth/token", site+"/api", cfg.ClientID, cfg.ClientSecret).
		request(ctx, configCheckToken)
	switch {
	case err == nil, errors.Is(err, auth.ErrInvalidToken):
		return nil
	case errors.Is(err, errMisconfigured):
		return err
	}
	Logger.Printf("MCP hosted: couldn't check the configuration with core: %v", err)
	return nil
}

type serverKey struct{}

// withServer builds the request's MCP server once. The SDK asks for it more
// than once per request, and each build makes every tool again. Only a POST
// needs one: stateless, the SDK answers anything else with 405.
func (h *host) withServer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// No MCP server for a token this replica doesn't hold: a throttled request
		// hasn't been authenticated, and answering 401 would send it through
		// sign-in for nothing.
		if info := auth.TokenInfoFromContext(req.Context()); info != nil && info.Extra[extraThrottled] == true {
			w.Header().Set("Retry-After", strconv.Itoa(int(refusalWindow.Seconds())))
			http.Error(w, unavailableMessage, http.StatusServiceUnavailable)
			return
		}
		if req.Method != http.MethodPost {
			next.ServeHTTP(w, req)
			return
		}
		if server := h.server(req); server != nil {
			req = req.WithContext(context.WithValue(req.Context(), serverKey{}, server))
		}
		next.ServeHTTP(w, req)
	})
}

func requestServer(req *http.Request) *mcp.Server {
	server, _ := req.Context().Value(serverKey{}).(*mcp.Server)
	return server
}

type host struct {
	exchanger   *exchanger
	behindProxy bool
}

// clientAddress is the address a request came from, for the refusal limit.
func (h *host) clientAddress(req *http.Request) string {
	if h.behindProxy {
		if forwarded := req.Header.Values("X-Forwarded-For"); len(forwarded) > 0 {
			hops := strings.Split(forwarded[len(forwarded)-1], ",")
			if last := strings.TrimSpace(hops[len(hops)-1]); last != "" {
				return last
			}
		}
	}
	addr, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return addr
}

// verify exchanges the client's token. Only a token core refuses is a 401: when
// core can't be reached the request goes on, and every tool call answers with
// a retry hint instead.
func (h *host) verify(ctx context.Context, token string, req *http.Request) (*auth.TokenInfo, error) {
	got, err := h.exchanger.exchange(ctx, token, h.clientAddress(req))
	switch {
	case errors.Is(err, auth.ErrInvalidToken):
		return nil, err
	case errors.Is(err, errThrottled):
		// Logged once, when the address reached the limit.
		return &auth.TokenInfo{Expiration: time.Now().Add(time.Minute), Extra: map[string]any{extraThrottled: true}}, nil
	case errors.Is(err, errMisconfigured):
		Logger.Printf("MCP hosted: misconfigured, every request will fail until it's fixed: %v", err)
		return unavailableInfo(), nil
	case err != nil:
		Logger.Printf("MCP hosted: %v", err)
		return unavailableInfo(), nil
	}
	return &auth.TokenInfo{
		Scopes:     got.scopes,
		Expiration: got.expires,
		Extra:      map[string]any{extraCoreToken: got.token, extraSubject: token, extraOrg: got.org},
	}, nil
}

// unavailableInfo lets a request through when the exchange couldn't be done,
// so its tool calls answer with a retry hint rather than a 401.
func unavailableInfo() *auth.TokenInfo {
	return &auth.TokenInfo{
		Scopes:     tools.KnownScopes,
		Expiration: time.Now().Add(time.Minute),
		Extra:      map[string]any{extraUnavailable: true},
	}
}

// server builds the MCP server for one request: the hosted tools the token's
// scopes allow, each calling the API with that request's exchanged token.
func (h *host) server(req *http.Request) *mcp.Server {
	info := auth.TokenInfoFromContext(req.Context())
	if info == nil {
		return nil
	}
	serversBuilt.Add(1)
	server := mcp.NewServer(&mcp.Implementation{Name: "shipyard-mcp-server", Version: version.Version},
		&mcp.ServerOptions{
			Instructions: strings.TrimSpace(instructions),
			// Every request builds its own server, so the tool list can't change
			// under a client. Advertising that it can has clients hold a stream open
			// that never carries anything, until the ingress cuts it. This also drops
			// the SDK's default logging capability: nothing here sends client logs.
			Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
		})

	coreToken, _ := info.Extra[extraCoreToken].(string)
	subject, _ := info.Extra[extraSubject].(string)
	org, _ := info.Extra[extraOrg].(string)
	if org == "" {
		org = "unknown"
	}
	server.AddReceivingMiddleware(logRequest(org))
	unavailable, _ := info.Extra[extraUnavailable].(bool)
	// A hosted token is bound to one org, and core reads it from the token.
	api := client.New(requests.NewWithBearerToken("mcp-hosted", coreToken), func() string { return "" })

	names := make([]string, 0, len(tools.HostedToolScopes))
	for name, scope := range tools.HostedToolScopes {
		if slices.Contains(info.Scopes, scope) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		tool := tools.NewHostedTool(api, name)
		server.AddTool(sdkTool(tools.Annotate(tool.Definition())), func(ctx context.Context, call *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if unavailable {
				Logger.Printf("MCP hosted: %s unavailable org=%q", name, org)
				return errorResult(unavailableMessage), nil
			}
			start := time.Now()
			out, err := tool.Execute(ctx, call.Params.Arguments)
			elapsed := time.Since(start).Round(time.Millisecond)
			if err != nil {
				return errorResult(h.toolError(name, subject, org, err, elapsed)), nil
			}
			Logger.Printf("MCP hosted: %s ok in %s org=%q", name, elapsed, org)
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out}}}, nil
		})
	}
	return server
}

// toolError is the message a failed tool call answers with. The response body
// isn't logged: it can hold the user's data.
func (h *host) toolError(name, subject, org string, err error, elapsed time.Duration) string {
	var apiErr *requests.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
		// The exchanged token stopped working, most likely because the user
		// revoked the connection. The next request exchanges again, and gets the
		// 401 that sends the client to sign in.
		h.exchanger.expire(subject)
		Logger.Printf("MCP hosted: %s: the API refused the exchanged token in %s org=%q", name, elapsed, org)
		return reconnectMessage
	}
	Logger.Printf("MCP hosted: %s failed status=%s in %s org=%q", name, failureStatus(err), elapsed, org)
	var mcpErr *mcperrors.MCPError
	if !errors.As(err, &mcpErr) {
		mcpErr = mcperrors.ParseHTTPError(name, err, "")
	}
	return mcpErr.Error()
}

// failureStatus is the HTTP status behind a failed tool call, or "none" when it
// never got an answer from the API, as with a bad argument or a timeout.
func failureStatus(err error) string {
	var apiErr *requests.APIError
	if errors.As(err, &apiErr) {
		return strconv.Itoa(apiErr.StatusCode)
	}
	var mcpErr *mcperrors.MCPError
	if errors.As(err, &mcpErr) && mcpErr.StatusCode != 0 {
		return strconv.Itoa(mcpErr.StatusCode)
	}
	return "none"
}

// logRequest logs the requests that say how clients connect. Tool calls log
// their own line, and the rest would only add noise. It names the methods it
// logs because the method comes from the client: a made-up one isn't written.
func logRequest(org string) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			switch method {
			case "initialize", "server/discover", "subscriptions/listen":
				Logger.Printf("MCP hosted: %s org=%q", method, org)
			}
			return next(ctx, method, req)
		}
	}
}

func errorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
}

func sdkTool(def tools.ToolDefinition) *mcp.Tool {
	tool := &mcp.Tool{Name: def.Name, Title: def.Title, Description: def.Description, InputSchema: def.InputSchema}
	if a := def.Annotations; a != nil {
		tool.Annotations = &mcp.ToolAnnotations{
			Title:           a.Title,
			ReadOnlyHint:    a.ReadOnlyHint,
			DestructiveHint: &a.DestructiveHint,
			IdempotentHint:  a.IdempotentHint,
			OpenWorldHint:   &a.OpenWorldHint,
		}
	}
	return tool
}
