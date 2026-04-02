package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	centoauth "github.com/T4cceptor/centian/internal/oauth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func testGatewayConfig(serverNames ...string) *config.GatewayConfig {
	serverConfigs := make(map[string]*config.MCPServerConfig, len(serverNames))
	for _, serverName := range serverNames {
		serverConfigs[serverName] = &config.MCPServerConfig{Command: "node"}
	}
	return &config.GatewayConfig{MCPServers: serverConfigs}
}

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
	assert.Equal(t, result.Header.Get(unauthorizedAuthHeaderHint), "Authorization")
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
	assert.Equal(t, result.Header.Get(unauthorizedAuthHeaderHint), "X-API-Key")
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
	proxy := &CentianEndpoint{
		name:     "gateway",
		endpoint: "/mcp/gateway",
		server: &CentianServer{
			APIKeys:    store,
			AuthHeader: "Authorization",
		},
	}
	mux := http.NewServeMux()

	// When: registering handler and calling without auth
	RegisterEndpoint(proxy, mux, nil)
	request := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	recorder := httptest.NewRecorder()
	handler, _ := mux.Handler(request)
	handler.ServeHTTP(recorder, request)

	// Then: unauthorized response is returned
	assert.Equal(t, recorder.Result().StatusCode, http.StatusUnauthorized)
}

func TestAPIKeyMiddlewareWithHeader_AttachesAuthData(t *testing.T) {
	store := createTestAPIKeyStore(t)
	called := false
	handler := apiKeyMiddlewareWithHeader(store, "Authorization", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		authData := getAuthData(r.Context())
		assert.Assert(t, authData != nil)
		assert.Equal(t, authData.AuthHeaderName, "Authorization")
		assert.Equal(t, authData.Gateway, "gateway-a")
		assert.Assert(t, authData.Headers != nil)
		assert.Assert(t, authData.KeyEntry != nil)
		assert.Equal(t, authData.KeyEntry.ID, "key_test")
		w.WriteHeader(http.StatusOK)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway-a", http.NoBody)
	request.Header.Set("Authorization", "Bearer plain-key")
	handler.ServeHTTP(recorder, request)

	assert.Assert(t, called)
	assert.Equal(t, recorder.Result().StatusCode, http.StatusOK)
}

func TestAPIKeyMiddlewareWithHeader_EnforcesGatewayScope(t *testing.T) {
	store := createScopedAPIKeyStore(t, "plain-key", []string{"gateway-a"})
	called := false
	handler := apiKeyMiddlewareWithHeader(store, "Authorization", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway-b", http.NoBody)
	request.Header.Set("Authorization", "Bearer plain-key")
	handler.ServeHTTP(recorder, request)

	assert.Assert(t, !called)
	assert.Equal(t, recorder.Result().StatusCode, http.StatusUnauthorized)
}

func TestGetGatewayFromPath(t *testing.T) {
	assert.Equal(t, getGatewayFromPath("/mcp/default"), "default")
	assert.Equal(t, getGatewayFromPath("/mcp/default/server"), "default")
	assert.Equal(t, getGatewayFromPath("/other/path"), "")
}

func TestNewCentianServer_RequiresAuthWhenBindingAllInterfaces(t *testing.T) {
	// Given: proxy settings that bind to all interfaces with auth unset
	globalConfig := &config.GlobalConfig{
		Name:    "Test Proxy Server",
		Version: "1.0.0",
		Proxy: &config.ProxySettings{
			Host:    "0.0.0.0",
			Port:    "9666",
			Timeout: 5,
		},
	}

	// When: creating the proxy
	proxy, err := NewCentianServer(globalConfig)

	// Then: an error is returned and no proxy is created
	assert.ErrorContains(t, err, "auth must be explicitly set when binding to 0.0.0.0")
	assert.Assert(t, proxy == nil)
}

func TestGetServerForRequest_ReusesPooledDownstreamForSameIdentity(t *testing.T) {
	// Given: a proxy with a custom downstream connection factory
	var created []*MockDownstreamConnection
	proxy := &CentianEndpoint{
		name:              "gateway",
		endpoint:          "/mcp/gateway",
		config:            testGatewayConfig("server1"),
		isAggregatedProxy: true,
		upstreamSessions:  make(map[string]*UpstreamSession),
		downstreamPools:   make(map[string]*DownstreamConnectionPool),
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
		server: &CentianServer{
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
	server1 := proxy.GetOrCreateServerForRequest(request1)
	firstSessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
	attachInitializedSessionForTest(t, proxy, firstSessionID, &mcp.ClientCapabilities{})
	request2.Header.Set("Mcp-Session-Id", "session-2")
	server2 := proxy.GetOrCreateServerForRequest(request2)
	attachInitializedSessionForTest(t, proxy, "session-2", &mcp.ClientCapabilities{})

	// Then: only one downstream connection is created and reused
	assert.Assert(t, server1 != nil)
	assert.Assert(t, server2 != nil)
	assert.Equal(t, len(created), 1)
	assert.Equal(t, len(proxy.downstreamPools), 1)
}

func TestGetServerForRequest_UsesSeparatePoolsForDifferentAuthIdentities(t *testing.T) {
	// Given: a proxy with a custom downstream connection factory
	var created []*MockDownstreamConnection
	proxy := &CentianEndpoint{
		name:              "gateway",
		endpoint:          "/mcp/gateway",
		config:            testGatewayConfig("server1"),
		isAggregatedProxy: true,
		upstreamSessions:  make(map[string]*UpstreamSession),
		downstreamPools:   make(map[string]*DownstreamConnectionPool),
		connectionFactory: func(_ string, cfg *config.MCPServerConfig) DownstreamConnectionInterface {
			conn := &MockDownstreamConnection{cfg: cfg, tools: []*mcp.Tool{{Name: "ping", Description: "ping", InputSchema: map[string]any{"type": "object"}}}}
			created = append(created, conn)
			return conn
		},
		server: &CentianServer{
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
	server1 := proxy.GetOrCreateServerForRequest(request1)
	firstSessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
	server2 := proxy.GetOrCreateServerForRequest(request2)
	attachInitializedSessionForTest(t, proxy, firstSessionID, &mcp.ClientCapabilities{})
	attachInitializedSessionForTest(t, proxy, "session-2", &mcp.ClientCapabilities{})

	// Then: separate pooled downstream entries are created
	assert.Assert(t, server1 != nil)
	assert.Assert(t, server2 != nil)
	assert.Equal(t, len(created), 2)
	assert.Equal(t, len(proxy.downstreamPools), 2)
}

func TestGetServerForRequest_UsesSharedPoolWhenAuthDisabled(t *testing.T) {
	// Given: a proxy without auth and with a custom downstream connection factory
	var created []*MockDownstreamConnection
	proxy := &CentianEndpoint{
		name:              "gateway",
		endpoint:          "/mcp/gateway",
		config:            testGatewayConfig("server1"),
		isAggregatedProxy: true,
		upstreamSessions:  make(map[string]*UpstreamSession),
		downstreamPools:   make(map[string]*DownstreamConnectionPool),
		connectionFactory: func(_ string, cfg *config.MCPServerConfig) DownstreamConnectionInterface {
			conn := &MockDownstreamConnection{cfg: cfg, tools: []*mcp.Tool{{Name: "ping", Description: "ping", InputSchema: map[string]any{"type": "object"}}}}
			created = append(created, conn)
			return conn
		},
		server: &CentianServer{
			AuthHeader: "Authorization",
			Config:     &config.GlobalConfig{Version: "1.0.0"},
		},
	}

	request1 := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request2 := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request2.Header.Set("Mcp-Session-Id", "session-2")

	// When: two upstream sessions hit the same endpoint with auth disabled
	server1 := proxy.GetOrCreateServerForRequest(request1)
	firstSessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
	server2 := proxy.GetOrCreateServerForRequest(request2)
	attachInitializedSessionForTest(t, proxy, firstSessionID, &mcp.ClientCapabilities{})
	attachInitializedSessionForTest(t, proxy, "session-2", &mcp.ClientCapabilities{})

	// Then: they share one pooled downstream entry
	assert.Assert(t, server1 != nil)
	assert.Assert(t, server2 != nil)
	assert.Equal(t, len(created), 1)
	assert.Equal(t, len(proxy.downstreamPools), 1)
}

func TestGetServerForRequest_DoesNotBlockOnSlowDownstreamConnect(t *testing.T) {
	// Given: a proxy with a downstream connect that does not complete immediately
	releaseConnect := make(chan struct{})
	proxy := &CentianEndpoint{
		name:              "gateway",
		endpoint:          "/mcp/gateway",
		config:            testGatewayConfig("server1"),
		isAggregatedProxy: true,
		upstreamSessions:  make(map[string]*UpstreamSession),
		downstreamPools:   make(map[string]*DownstreamConnectionPool),
		connectionFactory: func(_ string, cfg *config.MCPServerConfig) DownstreamConnectionInterface {
			return &MockDownstreamConnection{
				cfg: cfg,
				tools: []*mcp.Tool{
					{Name: "ping", Description: "ping", InputSchema: map[string]any{"type": "object"}},
				},
				ConnectFunc: func(_ context.Context, _ *DownstreamConnectOptions) error {
					<-releaseConnect
					return nil
				},
			}
		},
		server: &CentianServer{
			Config: &config.GlobalConfig{Version: "1.0.0"},
		},
	}

	request := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	serverResult := make(chan *mcp.Server, 1)

	// When: requesting a server while downstream connection is still pending
	go func() {
		serverResult <- proxy.GetOrCreateServerForRequest(request)
	}()

	// Then: the upstream server should be returned without waiting for the slow connect
	select {
	case server := <-serverResult:
		assert.Assert(t, server != nil)
	case <-time.After(2000 * time.Millisecond):
		close(releaseConnect)
		<-serverResult
		t.Fatal("expected GetServerForRequest to return before downstream connect completed")
	}

	close(releaseConnect)
}

func TestGetServerForRequest_DoesNotReusePoolWhenForwardedAuthChanges(t *testing.T) {
	// Given: a proxy that forwards downstream auth separately from Centian auth
	var created []*MockDownstreamConnection
	proxy := &CentianEndpoint{
		name:              "gateway",
		endpoint:          "/mcp/gateway",
		config:            testGatewayConfig("server1"),
		isAggregatedProxy: true,
		upstreamSessions:  make(map[string]*UpstreamSession),
		downstreamPools:   make(map[string]*DownstreamConnectionPool),
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
		server: &CentianServer{
			APIKeys:    createTestAPIKeyStore(t),
			AuthHeader: "X-Centian-Auth",
			Config:     &config.GlobalConfig{Version: "1.0.0"},
		},
	}

	request1 := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request1.Header.Set("Authorization", "Bearer downstream-token-1")
	request1 = request1.WithContext(withRequestIdentity(context.Background(), "auth:key_1"))

	request2 := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request2.Header.Set("Authorization", "Bearer downstream-token-2")
	request2.Header.Set("Mcp-Session-Id", "session-2")
	request2 = request2.WithContext(withRequestIdentity(context.Background(), "auth:key_1"))

	// When: the same Centian identity reconnects with a different downstream credential
	server1 := proxy.GetOrCreateServerForRequest(request1)
	firstSessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
	server2 := proxy.GetOrCreateServerForRequest(request2)
	attachInitializedSessionForTest(t, proxy, firstSessionID, &mcp.ClientCapabilities{})
	attachInitializedSessionForTest(t, proxy, "session-2", &mcp.ClientCapabilities{})

	// Then: a new downstream pool should be established with the new forwarded auth
	assert.Assert(t, server1 != nil)
	assert.Assert(t, server2 != nil)
	waitForCondition(t, time.Second, func() bool {
		return len(created) == 2 &&
			len(created[0].CapturedConnectAuths) == 1 &&
			len(created[1].CapturedConnectAuths) == 1
	})
	assert.Equal(t, created[0].CapturedConnectAuths[0]["Authorization"], "Bearer downstream-token-1")
	assert.Equal(t, created[1].CapturedConnectAuths[0]["Authorization"], "Bearer downstream-token-2")
}

func TestGetServerForRequest_RetriesFailedDownstreamsForLaterSessions(t *testing.T) {
	// Given: an aggregated proxy where a later session encounters a failed pooled downstream
	createdByServer := make(map[string]int)
	proxy := &CentianEndpoint{
		name:              "gateway",
		endpoint:          "/mcp/gateway",
		config:            testGatewayConfig("server1", "server2"),
		isAggregatedProxy: true,
		upstreamSessions:  make(map[string]*UpstreamSession),
		downstreamPools:   make(map[string]*DownstreamConnectionPool),
		connectionFactory: func(name string, cfg *config.MCPServerConfig) DownstreamConnectionInterface {
			createdByServer[name]++
			conn := &MockDownstreamConnection{
				serverName: name,
				cfg:        cfg,
				Status:     StatusConnected,
				tools: []*mcp.Tool{
					{Name: "ping", Description: "ping", InputSchema: map[string]any{"type": "object"}},
				},
			}
			return conn
		},
		server: &CentianServer{
			APIKeys:    createTestAPIKeyStore(t),
			AuthHeader: "Authorization",
			Config:     &config.GlobalConfig{Version: "1.0.0"},
		},
	}

	request1 := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request1 = request1.WithContext(withRequestIdentity(context.Background(), "auth:key_1"))

	request2 := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request2.Header.Set("Mcp-Session-Id", "session-2")
	request2 = request2.WithContext(withRequestIdentity(context.Background(), "auth:key_1"))

	// When: the first session establishes the pooled downstream set
	server1 := proxy.GetOrCreateServerForRequest(request1)
	firstSessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
	attachInitializedSessionForTest(t, proxy, firstSessionID, &mcp.ClientCapabilities{})
	waitForCondition(t, time.Second, func() bool { return createdByServer["server2"] >= 1 })
	beforeRetry := createdByServer["server2"]

	proxy.mu.Lock()
	firstSession := proxy.upstreamSessions[firstSessionID]
	pool := proxy.downstreamPools[firstSession.downstreamSessionKey]
	pool.downstreamConns["server2"] = &MockDownstreamConnection{
		serverName:    "server2",
		cfg:           proxy.config.MCPServers["server2"],
		Status:        StatusFailed,
		ErrorToReturn: errors.New("terminal failure"),
	}
	delete(pool.connecting, "server2")
	proxy.mu.Unlock()

	// And: a later session with the same identity arrives after that failure settled
	server2 := proxy.GetOrCreateServerForRequest(request2)
	attachInitializedSessionForTest(t, proxy, "session-2", &mcp.ClientCapabilities{})

	// Then: the failed downstream should be retried for the later session
	assert.Assert(t, server1 != nil)
	assert.Assert(t, server2 != nil)
	waitForCondition(t, time.Second, func() bool {
		return createdByServer["server2"] == beforeRetry+1
	})
}

func TestGetServerForRequest_RetriesTransientFailureInBackground(t *testing.T) {
	conn := &MockDownstreamConnection{
		serverName: "server1",
		cfg:        &config.MCPServerConfig{Command: "node"},
		tools: []*mcp.Tool{
			{Name: "ping", Description: "ping", InputSchema: map[string]any{"type": "object"}},
		},
	}
	conn.ConnectFunc = func(context.Context, *DownstreamConnectOptions) error {
		if conn.ConnectCalls == 1 {
			return errors.New("dial failed")
		}
		return nil
	}

	proxy := &CentianEndpoint{
		name:              "gateway",
		endpoint:          "/mcp/gateway",
		config:            testGatewayConfig("server1"),
		isAggregatedProxy: true,
		upstreamSessions:  make(map[string]*UpstreamSession),
		downstreamPools:   make(map[string]*DownstreamConnectionPool),
		connectionFactory: func(string, *config.MCPServerConfig) DownstreamConnectionInterface {
			return conn
		},
		server: &CentianServer{
			APIKeys:    createTestAPIKeyStore(t),
			AuthHeader: "Authorization",
			Config:     &config.GlobalConfig{Version: "1.0.0"},
		},
	}

	request := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request = request.WithContext(withRequestIdentity(context.Background(), "auth:key_1"))

	server := proxy.GetOrCreateServerForRequest(request)
	assert.Assert(t, server != nil)

	sessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
	proxy.mu.RLock()
	session := proxy.upstreamSessions[sessionID]
	proxy.mu.RUnlock()
	assert.Assert(t, session != nil)

	clientSession, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	attachInitializedSessionForTest(t, proxy, sessionID, &mcp.ClientCapabilities{})

	waitForCondition(t, 3*time.Second, func() bool {
		toolsResult, err := clientSession.ListTools(context.Background(), nil)
		return err == nil && len(toolsResult.Tools) == 1
	})
	assert.Equal(t, conn.ConnectCalls, 2)
}

func TestGetServerForRequest_DoesNotRetryPermanentFailures(t *testing.T) {
	conn := &MockDownstreamConnection{
		serverName:    "server1",
		cfg:           &config.MCPServerConfig{Command: "node"},
		ErrorToReturn: errors.New("failed to create transport: no URL or Command configured for server server1"),
		Status:        StatusPending,
	}

	proxy := &CentianEndpoint{
		name:              "gateway",
		endpoint:          "/mcp/gateway",
		config:            testGatewayConfig("server1"),
		isAggregatedProxy: true,
		upstreamSessions:  make(map[string]*UpstreamSession),
		downstreamPools:   make(map[string]*DownstreamConnectionPool),
		connectionFactory: func(string, *config.MCPServerConfig) DownstreamConnectionInterface {
			return conn
		},
		server: &CentianServer{
			APIKeys:    createTestAPIKeyStore(t),
			AuthHeader: "Authorization",
			Config:     &config.GlobalConfig{Version: "1.0.0"},
		},
	}

	request := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request = request.WithContext(withRequestIdentity(context.Background(), "auth:key_1"))

	server := proxy.GetOrCreateServerForRequest(request)
	assert.Assert(t, server != nil)

	sessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
	proxy.mu.RLock()
	session := proxy.upstreamSessions[sessionID]
	proxy.mu.RUnlock()
	assert.Assert(t, session != nil)

	_, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	attachInitializedSessionForTest(t, proxy, sessionID, &mcp.ClientCapabilities{})

	waitForCondition(t, time.Second, func() bool {
		proxy.mu.RLock()
		defer proxy.mu.RUnlock()
		pool := proxy.downstreamPools[session.downstreamSessionKey]
		return pool != nil && !pool.connecting["server1"]
	})
	time.Sleep(350 * time.Millisecond)
	assert.Equal(t, conn.ConnectCalls, 1)
}

func TestGetServerForRequest_DoesNotRetryAuthorizationFailures(t *testing.T) {
	testCases := []struct {
		name   string
		reason string
	}{
		{name: "auth required", reason: centoauth.AuthorizationReasonRequired},
		{name: "refresh failed", reason: centoauth.AuthorizationReasonRefreshFailed},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conn := &MockDownstreamConnection{
				serverName: "server1",
				cfg:        &config.MCPServerConfig{Command: "node"},
				ErrorToReturn: &centoauth.AuthorizationRequiredError{
					Binding: centoauth.Binding{
						PrincipalID: "auth:key_1",
						Gateway:     "gateway",
						Server:      "server1",
					},
					AuthURL: "http://127.0.0.1:9666/oauth/start?id=test",
					Reason:  tc.reason,
				},
			}

			proxy := &CentianEndpoint{
				name:              "gateway",
				endpoint:          "/mcp/gateway",
				config:            testGatewayConfig("server1"),
				isAggregatedProxy: true,
				upstreamSessions:  make(map[string]*UpstreamSession),
				downstreamPools:   make(map[string]*DownstreamConnectionPool),
				connectionFactory: func(string, *config.MCPServerConfig) DownstreamConnectionInterface {
					return conn
				},
				server: &CentianServer{
					APIKeys:    createTestAPIKeyStore(t),
					AuthHeader: "Authorization",
					Config:     &config.GlobalConfig{Version: "1.0.0"},
				},
			}

			request := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
			request = request.WithContext(withRequestIdentity(context.Background(), "auth:key_1"))

			server := proxy.GetOrCreateServerForRequest(request)
			assert.Assert(t, server != nil)

			sessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
			proxy.mu.RLock()
			session := proxy.upstreamSessions[sessionID]
			proxy.mu.RUnlock()
			assert.Assert(t, session != nil)

			_, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
			defer cleanup()

			attachInitializedSessionForTest(t, proxy, sessionID, &mcp.ClientCapabilities{})

			waitForCondition(t, time.Second, func() bool {
				proxy.mu.RLock()
				defer proxy.mu.RUnlock()
				pool := proxy.downstreamPools[session.downstreamSessionKey]
				return pool != nil && !pool.connecting["server1"]
			})
			time.Sleep(350 * time.Millisecond)
			assert.Equal(t, conn.ConnectCalls, 1)
		})
	}
}

func TestInvalidatePooledDownstream_CancelsActiveRetry(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	conn := &MockDownstreamConnection{
		serverName: "server1",
		cfg:        &config.MCPServerConfig{Command: "node"},
	}
	conn.ConnectFunc = func(ctx context.Context, _ *DownstreamConnectOptions) error {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	}

	proxy := &CentianEndpoint{
		name:              "gateway",
		endpoint:          "/mcp/gateway",
		config:            testGatewayConfig("server1"),
		isAggregatedProxy: true,
		upstreamSessions:  make(map[string]*UpstreamSession),
		downstreamPools:   make(map[string]*DownstreamConnectionPool),
		connectionFactory: func(string, *config.MCPServerConfig) DownstreamConnectionInterface {
			return conn
		},
		server: &CentianServer{
			APIKeys:    createTestAPIKeyStore(t),
			AuthHeader: "Authorization",
			Config:     &config.GlobalConfig{Version: "1.0.0"},
		},
	}

	request := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request = request.WithContext(withRequestIdentity(context.Background(), "auth:key_1"))

	server := proxy.GetOrCreateServerForRequest(request)
	assert.Assert(t, server != nil)

	sessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
	proxy.mu.RLock()
	session := proxy.upstreamSessions[sessionID]
	proxy.mu.RUnlock()
	assert.Assert(t, session != nil)

	_, cleanup := connectUpstreamTestClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	attachInitializedSessionForTest(t, proxy, sessionID, &mcp.ClientCapabilities{})

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("connect attempt did not start in time")
	}

	proxy.invalidateDownstreamPool(session.downstreamSessionKey)
	close(release)

	waitForCondition(t, time.Second, func() bool {
		proxy.mu.RLock()
		defer proxy.mu.RUnlock()
		_, ok := proxy.downstreamPools[session.downstreamSessionKey]
		return !ok
	})
	waitForCondition(t, time.Second, func() bool {
		return conn.CloseCalls == 1
	})
	assert.Equal(t, conn.ConnectCalls, 1)
}

func TestGetServerForRequest_DoesNotSpawnDuplicateRetryWorker(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	created := 0

	conn := &MockDownstreamConnection{
		serverName: "server1",
		cfg:        &config.MCPServerConfig{Command: "node"},
		tools: []*mcp.Tool{
			{Name: "ping", Description: "ping", InputSchema: map[string]any{"type": "object"}},
		},
	}
	conn.ConnectFunc = func(ctx context.Context, _ *DownstreamConnectOptions) error {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	}

	proxy := &CentianEndpoint{
		name:              "gateway",
		endpoint:          "/mcp/gateway",
		config:            testGatewayConfig("server1"),
		isAggregatedProxy: true,
		upstreamSessions:  make(map[string]*UpstreamSession),
		downstreamPools:   make(map[string]*DownstreamConnectionPool),
		connectionFactory: func(string, *config.MCPServerConfig) DownstreamConnectionInterface {
			created++
			return conn
		},
		server: &CentianServer{
			APIKeys:    createTestAPIKeyStore(t),
			AuthHeader: "Authorization",
			Config:     &config.GlobalConfig{Version: "1.0.0"},
		},
	}

	request1 := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request1 = request1.WithContext(withRequestIdentity(context.Background(), "auth:key_1"))
	request2 := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	request2.Header.Set("Mcp-Session-Id", "session-2")
	request2 = request2.WithContext(withRequestIdentity(context.Background(), "auth:key_1"))

	server1 := proxy.GetOrCreateServerForRequest(request1)
	assert.Assert(t, server1 != nil)

	firstSessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
	proxy.mu.RLock()
	firstSession := proxy.upstreamSessions[firstSessionID]
	proxy.mu.RUnlock()
	assert.Assert(t, firstSession != nil)

	_, cleanup := connectUpstreamTestClient(t, firstSession, &mcp.ClientOptions{})
	defer cleanup()

	attachInitializedSessionForTest(t, proxy, firstSessionID, &mcp.ClientCapabilities{})

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("connect attempt did not start in time")
	}

	server2 := proxy.GetOrCreateServerForRequest(request2)
	assert.Assert(t, server2 != nil)
	attachInitializedSessionForTest(t, proxy, "session-2", &mcp.ClientCapabilities{})

	assert.Equal(t, created, 1)
	close(release)
}

func TestLogRequestForDebugPreservesRequestBody(t *testing.T) {
	var console bytes.Buffer
	err := common.InitInternalLogger(common.LoggerOptions{
		Level:         "debug",
		Output:        "console",
		ConsoleWriter: &console,
	})
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = common.CloseLogger()
	})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", io.NopCloser(bytes.NewBufferString(`{"hello":"world"}`)))
	req.Header.Set("Authorization", "Bearer secret")

	logRequestForDebug(req)

	body, err := io.ReadAll(req.Body)
	assert.NilError(t, err)
	assert.Equal(t, string(body), `{"hello":"world"}`)
	assert.Assert(t, req.GetBody != nil)

	clonedBody, err := req.GetBody()
	assert.NilError(t, err)
	defer clonedBody.Close()

	clonedData, err := io.ReadAll(clonedBody)
	assert.NilError(t, err)
	assert.Equal(t, string(clonedData), `{"hello":"world"}`)
	assert.Assert(t, bytes.Contains(console.Bytes(), []byte("Received request body")))
	assert.Assert(t, !bytes.Contains(console.Bytes(), []byte("Bearer secret")))
}

func TestInvalidatePooledDownstream(t *testing.T) {
	conn := &MockDownstreamConnection{Status: StatusConnected}
	proxy := &CentianEndpoint{
		name: "gateway",
		downstreamPools: map[string]*DownstreamConnectionPool{
			"pool-1": {
				downstreamConns: map[string]DownstreamConnectionInterface{
					"server1": conn,
				},
			},
		},
	}

	proxy.invalidateDownstreamPool("pool-1")

	assert.Equal(t, conn.CloseCalls, 1)
	assert.Assert(t, proxy.downstreamPools["pool-1"] == nil)
}

func TestClosePoolEntryLocked(t *testing.T) {
	okConn := &MockDownstreamConnection{}
	failingConn := &closeErrorDownstreamConnection{
		MockDownstreamConnection: &MockDownstreamConnection{},
		closeErr:                 errors.New("close failed"),
	}

	errs := (&CentianEndpoint{}).closeDownstreamSessionPool(&DownstreamConnectionPool{
		downstreamConns: map[string]DownstreamConnectionInterface{
			"ok":   okConn,
			"fail": failingConn,
		},
	})

	assert.Equal(t, okConn.CloseCalls, 1)
	assert.Equal(t, failingConn.CloseCalls, 1)
	assert.Equal(t, len(errs), 1)
	assert.ErrorContains(t, errs[0], "close failed")
}

type closeErrorDownstreamConnection struct {
	*MockDownstreamConnection
	closeErr error
}

func (c *closeErrorDownstreamConnection) Close() error {
	c.CloseCalls++
	return c.closeErr
}

func createTestAPIKeyStore(t *testing.T) *auth.APIKeyStore {
	t.Helper()
	entry, err := auth.NewAPIKeyEntry("plain-key")
	assert.NilError(t, err)
	entry.ID = "key_test"
	path := filepath.Join(t.TempDir(), "api_keys.json")
	assert.NilError(t, auth.WriteAPIKeyFile(path, &auth.APIKeyFile{Keys: []auth.APIKeyEntry{entry}}))
	store, err := auth.LoadAPIKeys(path)
	assert.NilError(t, err)
	return store
}

func createScopedAPIKeyStore(t *testing.T, plain string, gateways []string) *auth.APIKeyStore {
	t.Helper()
	entry, err := auth.NewAPIKeyEntry(plain)
	assert.NilError(t, err)
	entry.ID = "key_test"
	entry.Gateways = gateways
	path := filepath.Join(t.TempDir(), "api_keys.json")
	assert.NilError(t, auth.WriteAPIKeyFile(path, &auth.APIKeyFile{Keys: []auth.APIKeyEntry{entry}}))
	store, err := auth.LoadAPIKeys(path)
	assert.NilError(t, err)
	return store
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("condition was not met before timeout")
}
