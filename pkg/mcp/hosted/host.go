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
	"net/http"
	"net/url"
	"os"
	"slices"
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

// Logger is the hosted server's own log, for its operator. The tools' request
// logs follow --verbose: they carry users' params and the API's error bodies.
var Logger = log.New(os.Stderr, "", log.LstdFlags|log.LUTC)

const (
	protectedResourcePath = "/.well-known/oauth-protected-resource"

	// Keys in auth.TokenInfo.Extra.
	extraCoreToken   = "core_token"
	extraSubject     = "subject_token"
	extraUnavailable = "exchange_unavailable"

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
// resource metadata (RFC 9728), and /healthz.
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

	h := &host{exchanger: newExchanger(site+"/oauth/token", site+"/api", cfg.ClientID, cfg.ClientSecret)}

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
	})

	mux := http.NewServeMux()
	mux.Handle(protectedResourcePath, metadata)
	if mcpPath != "/" {
		mux.Handle(protectedResourcePath+strings.TrimSuffix(mcpPath, "/"), metadata)
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle(mcpPath, auth.RequireBearerToken(h.verify, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: metadataURL,
	})(h.withServer(streamable)))
	return mux, nil
}

// CheckConfig asks core to exchange a token that can't exist. Core checks this
// server's client and resource before the subject token, so it refuses the
// token itself only when both are right. Core being unreachable isn't an error:
// the server answers with a retry hint until it's back.
func CheckConfig(ctx context.Context, cfg Config) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	site := strings.TrimSuffix(cfg.SiteURL, "/")
	_, err := newExchanger(site+"/oauth/token", site+"/api", cfg.ClientID, cfg.ClientSecret).
		request(ctx, "shipyard-mcp-host-config-check")
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
	exchanger *exchanger
}

// verify exchanges the client's token. Only a token core refuses is a 401: when
// core can't be reached the request goes on, and every tool call answers with
// a retry hint instead.
func (h *host) verify(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	got, err := h.exchanger.exchange(ctx, token)
	switch {
	case errors.Is(err, auth.ErrInvalidToken):
		return nil, err
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
		Extra:      map[string]any{extraCoreToken: got.token, extraSubject: token},
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
		&mcp.ServerOptions{Instructions: strings.TrimSpace(instructions)})

	coreToken, _ := info.Extra[extraCoreToken].(string)
	subject, _ := info.Extra[extraSubject].(string)
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
				return errorResult(unavailableMessage), nil
			}
			out, err := tool.Execute(ctx, call.Params.Arguments)
			if err != nil {
				return errorResult(h.toolError(name, subject, err)), nil
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out}}}, nil
		})
	}
	return server
}

// toolError is the message a failed tool call answers with. The response body
// isn't logged: it can hold the user's data.
func (h *host) toolError(name, subject string, err error) string {
	var apiErr *requests.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
		// The exchanged token stopped working, most likely because the user
		// revoked the connection. The next request exchanges again, and gets the
		// 401 that sends the client to sign in.
		h.exchanger.forget(subject)
		Logger.Printf("MCP hosted: %s: the API refused the exchanged token", name)
		return reconnectMessage
	}
	Logger.Printf("MCP hosted: %s failed", name)
	var mcpErr *mcperrors.MCPError
	if !errors.As(err, &mcpErr) {
		mcpErr = mcperrors.ParseHTTPError(name, err, "")
	}
	return mcpErr.Error()
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
