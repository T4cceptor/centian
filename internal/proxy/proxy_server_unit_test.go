package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestWriteUnauthorized(t *testing.T) {
	// Given: a response recorder
	recorder := httptest.NewRecorder()

	// When: writing unauthorized for Authorization header
	writeUnauthorized(recorder, "Authorization")

	// Then: status and headers are set
	result := recorder.Result()
	assert.Equal(t, result.StatusCode, http.StatusUnauthorized)
	assert.Equal(t, result.Header.Get("WWW-Authenticate"), "Bearer")
	assert.Equal(t, result.Header.Get("Content-Type"), "application/json")
}

func TestWriteUnauthorized_CustomHeader(t *testing.T) {
	// Given: a response recorder
	recorder := httptest.NewRecorder()

	// When: writing unauthorized for custom header
	writeUnauthorized(recorder, "X-API-Key")

	// Then: www-authenticate is not set
	result := recorder.Result()
	assert.Equal(t, result.StatusCode, http.StatusUnauthorized)
	assert.Equal(t, result.Header.Get("WWW-Authenticate"), "")
}

func TestAPIKeyMiddlewareWithHeader_NoStore(t *testing.T) {
	// Given: a handler and nil API key store
	called := false
	handler := apiKeyMiddlewareWithHeader(nil, "Authorization", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// When: calling the handler
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	handler.ServeHTTP(recorder, request)

	// Then: request passes through
	assert.Assert(t, called)
	assert.Equal(t, recorder.Result().StatusCode, http.StatusOK)
}

func TestAPIKeyMiddlewareWithHeader_WithStore(t *testing.T) {
	// Given: an API key store with one key
	store := createTestAPIKeyStore(t)

	called := false
	handler := apiKeyMiddlewareWithHeader(store, "Authorization", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// When: request is missing token
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	handler.ServeHTTP(recorder, request)

	// Then: unauthorized and handler not called
	assert.Equal(t, recorder.Result().StatusCode, http.StatusUnauthorized)
	assert.Assert(t, !called)

	// When: request has invalid token
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	request.Header.Set("Authorization", "Bearer bad")
	handler.ServeHTTP(recorder, request)

	// Then: unauthorized and handler not called
	assert.Equal(t, recorder.Result().StatusCode, http.StatusUnauthorized)
	assert.Assert(t, !called)

	// When: request has valid token
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	request.Header.Set("Authorization", "Bearer plain-key")
	handler.ServeHTTP(recorder, request)

	// Then: handler is called
	assert.Equal(t, recorder.Result().StatusCode, http.StatusOK)
	assert.Assert(t, called)
}

func TestAPIKeyMiddlewareWithHeader_AttachesIdentityToContext(t *testing.T) {
	// Given: an API key store with one key
	store := createTestAPIKeyStore(t)

	var identity string
	handler := apiKeyMiddlewareWithHeader(store, "Authorization", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, _ = requestIdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// When: request has valid token
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	request.Header.Set("Authorization", "Bearer plain-key")
	handler.ServeHTTP(recorder, request)

	// Then: identity is attached to the request context
	assert.Equal(t, recorder.Result().StatusCode, http.StatusOK)
	assert.Assert(t, identity != "")
}

func TestRegisterHandler_WithAuthMiddleware(t *testing.T) {
	// Given: a proxy with API key auth
	store := createTestAPIKeyStore(t)
	proxy := &MCPProxy{
		name:     "gateway",
		endpoint: "/mcp/gateway",
		server: &CentianProxy{
			APIKeys:    store,
			AuthHeader: "Authorization",
		},
	}
	mux := http.NewServeMux()

	// When: registering handler and calling without auth
	RegisterEndpoint("/mcp/gateway", proxy, mux, nil)
	request := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	recorder := httptest.NewRecorder()
	handler, _ := mux.Handler(request)
	handler.ServeHTTP(recorder, request)

	// Then: unauthorized response is returned
	assert.Equal(t, recorder.Result().StatusCode, http.StatusUnauthorized)
}

func TestNewCentianProxy_RequiresAuthWhenBindingAllInterfaces(t *testing.T) {
	// Given: proxy settings that bind to all interfaces with auth unset
	globalConfig := &config.GlobalConfig{
		Name:    "Test Proxy Server",
		Version: "1.0.0",
		Proxy: &config.ProxySettings{
			Host:    "0.0.0.0",
			Port:    "8080",
			Timeout: 5,
		},
	}

	// When: creating the proxy
	proxy, err := NewCentianProxy(globalConfig)

	// Then: an error is returned and no proxy is created
	assert.ErrorContains(t, err, "auth must be explicitly set when binding to 0.0.0.0")
	assert.Assert(t, proxy == nil)
}

func TestGetServerForRequest_ReusesPooledDownstreamForSameIdentity(t *testing.T) {
	// Given: a proxy with a custom downstream connection factory
	var created []*MockDownstreamConnection
	proxy := &MCPProxy{
		name:              "gateway",
		endpoint:          "/mcp/gateway",
		downstreams:       map[string]*DownstreamConnection{"server1": NewDownstreamConnection("server1", &config.MCPServerConfig{Command: "node"})},
		sessions:          make(map[string]*CentianProxySession),
		pooledDownstreams: make(map[string]*downstreamPoolEntry),
		connectionFactory: func(_ string, cfg *config.MCPServerConfig) DownstreamConnectionInterface {
			conn := &MockDownstreamConnection{
				cfg: cfg,
				tools: []*mcp.Tool{
					{Name: "ping", Description: "ping", InputSchema: map[string]any{"type": "object"}},
				},
			}
			created = append(created, conn)
			return conn
		},
		server: &CentianProxy{
			APIKeys:    createTestAPIKeyStore(t),
			AuthHeader: "Authorization",
			Config:     &config.GlobalConfig{Version: "1.0.0"},
		},
	}

	request1 := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request1 = request1.WithContext(withRequestIdentity(context.Background(), "auth:key_1"))
	request2 := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request2 = request2.WithContext(withRequestIdentity(context.Background(), "auth:key_1"))

	// When: two different upstream sessions use the same identity
	server1 := proxy.GetServerForRequest(request1)
	request2.Header.Set("Mcp-Session-Id", "session-2")
	server2 := proxy.GetServerForRequest(request2)

	// Then: only one downstream connection is created and reused
	assert.Assert(t, server1 != nil)
	assert.Assert(t, server2 != nil)
	assert.Equal(t, len(created), 1)
	assert.Equal(t, len(proxy.pooledDownstreams), 1)
}

func TestGetServerForRequest_UsesSeparatePoolsForDifferentAuthIdentities(t *testing.T) {
	// Given: a proxy with a custom downstream connection factory
	var created []*MockDownstreamConnection
	proxy := &MCPProxy{
		name:              "gateway",
		endpoint:          "/mcp/gateway",
		downstreams:       map[string]*DownstreamConnection{"server1": NewDownstreamConnection("server1", &config.MCPServerConfig{Command: "node"})},
		sessions:          make(map[string]*CentianProxySession),
		pooledDownstreams: make(map[string]*downstreamPoolEntry),
		connectionFactory: func(_ string, cfg *config.MCPServerConfig) DownstreamConnectionInterface {
			conn := &MockDownstreamConnection{cfg: cfg, tools: []*mcp.Tool{{Name: "ping", Description: "ping", InputSchema: map[string]any{"type": "object"}}}}
			created = append(created, conn)
			return conn
		},
		server: &CentianProxy{
			APIKeys:    createTestAPIKeyStore(t),
			AuthHeader: "Authorization",
			Config:     &config.GlobalConfig{Version: "1.0.0"},
		},
	}

	request1 := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request1 = request1.WithContext(withRequestIdentity(context.Background(), "auth:key_1"))
	request2 := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request2.Header.Set("Mcp-Session-Id", "session-2")
	request2 = request2.WithContext(withRequestIdentity(context.Background(), "auth:key_2"))

	// When: requests use different auth identities
	server1 := proxy.GetServerForRequest(request1)
	server2 := proxy.GetServerForRequest(request2)

	// Then: separate pooled downstream entries are created
	assert.Assert(t, server1 != nil)
	assert.Assert(t, server2 != nil)
	assert.Equal(t, len(created), 2)
	assert.Equal(t, len(proxy.pooledDownstreams), 2)
}

func TestGetServerForRequest_UsesSharedPoolWhenAuthDisabled(t *testing.T) {
	// Given: a proxy without auth and with a custom downstream connection factory
	var created []*MockDownstreamConnection
	proxy := &MCPProxy{
		name:              "gateway",
		endpoint:          "/mcp/gateway",
		downstreams:       map[string]*DownstreamConnection{"server1": NewDownstreamConnection("server1", &config.MCPServerConfig{Command: "node"})},
		sessions:          make(map[string]*CentianProxySession),
		pooledDownstreams: make(map[string]*downstreamPoolEntry),
		connectionFactory: func(_ string, cfg *config.MCPServerConfig) DownstreamConnectionInterface {
			conn := &MockDownstreamConnection{cfg: cfg, tools: []*mcp.Tool{{Name: "ping", Description: "ping", InputSchema: map[string]any{"type": "object"}}}}
			created = append(created, conn)
			return conn
		},
		server: &CentianProxy{
			AuthHeader: "Authorization",
			Config:     &config.GlobalConfig{Version: "1.0.0"},
		},
	}

	request1 := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request2 := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request2.Header.Set("Mcp-Session-Id", "session-2")

	// When: two upstream sessions hit the same endpoint with auth disabled
	server1 := proxy.GetServerForRequest(request1)
	server2 := proxy.GetServerForRequest(request2)

	// Then: they share one pooled downstream entry
	assert.Assert(t, server1 != nil)
	assert.Assert(t, server2 != nil)
	assert.Equal(t, len(created), 1)
	assert.Equal(t, len(proxy.pooledDownstreams), 1)
}

func createTestAPIKeyStore(t *testing.T) *auth.APIKeyStore {
	t.Helper()
	entry, err := auth.NewAPIKeyEntry("plain-key")
	assert.NilError(t, err)
	path := filepath.Join(t.TempDir(), "api_keys.json")
	assert.NilError(t, auth.WriteAPIKeyFile(path, &auth.APIKeyFile{Keys: []auth.APIKeyEntry{entry}}))
	store, err := auth.LoadAPIKeys(path)
	assert.NilError(t, err)
	return store
}
