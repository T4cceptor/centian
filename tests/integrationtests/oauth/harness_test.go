package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/proxy"
	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const integrationTimeout = 20 * time.Second

type fakeOAuthServer struct {
	mu                    sync.Mutex
	baseURL               string
	codes                 map[string]string
	refreshTokens         map[string]string
	validAccessTokens     map[string]struct{}
	issueExpiredCodeToken bool
	refreshCount          int
}

func newFakeOAuthServer(issueExpiredCodeToken bool) *fakeOAuthServer {
	return &fakeOAuthServer{
		codes:                 make(map[string]string),
		refreshTokens:         make(map[string]string),
		validAccessTokens:     make(map[string]struct{}),
		issueExpiredCodeToken: issueExpiredCodeToken,
	}
}

func (s *fakeOAuthServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-authorization-server", s.handleMetadata)
	mux.HandleFunc("/authorize", s.handleAuthorize)
	mux.HandleFunc("/token", s.handleToken)
	return mux
}

func (s *fakeOAuthServer) handleMetadata(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                s.baseURL,
		"authorization_endpoint":                s.baseURL + "/authorize",
		"token_endpoint":                        s.baseURL + "/token",
		"token_endpoint_auth_methods_supported": []string{"client_secret_post"},
		"code_challenge_methods_supported":      []string{"S256"},
	})
}

func (s *fakeOAuthServer) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	code := fmt.Sprintf("code-%d", time.Now().UnixNano())
	s.mu.Lock()
	s.codes[code] = r.URL.Query().Get("code_challenge")
	s.mu.Unlock()

	redirectURI := r.URL.Query().Get("redirect_uri")
	state := r.URL.Query().Get("state")
	http.Redirect(w, r, redirectURI+"?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(state), http.StatusFound)
}

func (s *fakeOAuthServer) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		code := r.Form.Get("code")
		s.mu.Lock()
		_, ok := s.codes[code]
		delete(s.codes, code)
		s.mu.Unlock()
		if !ok {
			http.Error(w, "unknown code", http.StatusBadRequest)
			return
		}
		accessToken := "access-token"
		expiry := 300
		if s.issueExpiredCodeToken {
			accessToken = "expired-access-token"
			expiry = -10
		}
		s.mu.Lock()
		if !s.issueExpiredCodeToken {
			s.validAccessTokens[accessToken] = struct{}{}
		}
		s.refreshTokens["refresh-token"] = "refreshed-access-token"
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  accessToken,
			"token_type":    "Bearer",
			"refresh_token": "refresh-token",
			"expires_in":    expiry,
		})
	case "refresh_token":
		s.mu.Lock()
		s.refreshCount++
		accessToken := s.refreshTokens[r.Form.Get("refresh_token")]
		s.validAccessTokens[accessToken] = struct{}{}
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  accessToken,
			"token_type":    "Bearer",
			"refresh_token": r.Form.Get("refresh_token"),
			"expires_in":    300,
		})
	default:
		http.Error(w, "unsupported grant", http.StatusBadRequest)
	}
}

func (s *fakeOAuthServer) acceptsToken(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.validAccessTokens[token]
	return ok
}

func (s *fakeOAuthServer) RefreshCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshCount
}

type fakeProtectedServer struct {
	authServer *fakeOAuthServer
	baseURL    string
}

func (s *fakeProtectedServer) handler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "oauth-downstream", Version: "1.0.0"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "echo",
		Description: "Echo text",
		InputSchema: map[string]any{"type": "object"},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(req.Params.Arguments)}}}, nil
	})
	streamable := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server { return server }, nil)

	protected := sdkauth.RequireBearerToken(func(_ context.Context, token string, _ *http.Request) (*sdkauth.TokenInfo, error) {
		if !s.authServer.acceptsToken(token) {
			return nil, sdkauth.ErrInvalidToken
		}
		return &sdkauth.TokenInfo{Expiration: time.Now().Add(time.Hour)}, nil
	}, &sdkauth.RequireBearerTokenOptions{
		ResourceMetadataURL: s.baseURL + "/.well-known/oauth-protected-resource",
	})

	mux := http.NewServeMux()
	mux.Handle("/mcp", protected(streamable))
	mux.Handle("/.well-known/oauth-protected-resource", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resource":              s.baseURL + "/mcp",
			"authorization_servers": []string{s.authServer.baseURL},
			"scopes_supported":      []string{"tool:echo"},
		})
	}))
	return mux
}

type oauthHarness struct {
	downstreamURL string
	proxyURL      string
	authServer    *fakeOAuthServer
}

func newOAuthHarness(t *testing.T, issueExpiredCodeToken bool) *oauthHarness {
	t.Helper()
	t.Setenv("CENTIAN_LOG_DIR", t.TempDir())

	authServer := newFakeOAuthServer(issueExpiredCodeToken)
	authBaseURL := startLocalHTTPServer(t, authServer.handler())
	authServer.baseURL = authBaseURL

	protected := &fakeProtectedServer{authServer: authServer}
	protectedBaseURL := startLocalHTTPServer(t, protected.handler())
	protected.baseURL = protectedBaseURL

	proxyBaseURL := startCentianOAuthProxy(t, authBaseURL, protectedBaseURL+"/mcp")
	return &oauthHarness{
		downstreamURL: protectedBaseURL + "/mcp",
		proxyURL:      proxyBaseURL + "/mcp/oauth-gateway/protected",
		authServer:    authServer,
	}
}

func startCentianOAuthProxy(t *testing.T, issuerURL, downstreamURL string) string {
	t.Helper()

	authDisabled := false
	port := allocateFreePort(t)
	publicBaseURL := "http://127.0.0.1:" + port

	globalConfig := &config.GlobalConfig{
		Name:        "OAuth Integration Proxy",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Host:    "127.0.0.1",
			Port:    port,
			Timeout: int(integrationTimeout.Seconds()),
			Web: &config.ProxyWebSettings{
				PublicBaseURL: publicBaseURL,
			},
		},
		Gateways: map[string]*config.GatewayConfig{
			"oauth-gateway": {
				MCPServers: map[string]*config.MCPServerConfig{
					"protected": {
						URL: downstreamURL,
						OAuth: &config.OAuthConfig{
							Enabled:          true,
							ClientID:         "test-client",
							ClientSecret:     "test-secret",
							ClientAuthMethod: "client_secret_post",
							Scopes:           []string{"tool:echo"},
							Resource:         downstreamURL,
							Issuer:           issuerURL,
						},
					},
				},
			},
		},
	}

	server, err := proxy.NewCentianServer(globalConfig)
	if err != nil {
		t.Fatalf("failed to create centian server: %v", err)
	}
	if err := server.Setup(); err != nil {
		t.Fatalf("failed to setup centian server: %v", err)
	}

	serveErr := make(chan error, 1)
	go func() {
		if err := server.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	waitForProxyListener(t, server.Server.Addr)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("failed to shutdown centian proxy: %v", err)
		}
		select {
		case err := <-serveErr:
			if err != nil {
				t.Fatalf("proxy serve error: %v", err)
			}
		default:
		}
	})
	return publicBaseURL
}

func allocateFreePort(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate free port: %v", err)
	}
	defer listener.Close()

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address type: %T", listener.Addr())
	}
	return strconv.Itoa(tcpAddr.Port)
}

func waitForProxyListener(t *testing.T, address string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("proxy listener %s did not become ready in time", address)
}

func startLocalHTTPServer(t *testing.T, handler http.Handler) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("test server error: %v", err)
		}
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})
	return "http://" + listener.Addr().String()
}

type oauthClientRecorder struct {
	mu              sync.Mutex
	logs            []string
	toolListChanged int
}

func (r *oauthClientRecorder) addLog(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, message)
}

func (r *oauthClientRecorder) hasLogSubstring(substr string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.logs {
		if strings.Contains(entry, substr) {
			return true
		}
	}
	return false
}

func (r *oauthClientRecorder) incrementToolListChanged() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.toolListChanged++
}

func (r *oauthClientRecorder) toolListChangedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.toolListChanged
}
