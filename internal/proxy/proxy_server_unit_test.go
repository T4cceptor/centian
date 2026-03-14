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
	RegisterEndpoint("/mcp/gateway", proxy, mux, nil)
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
			Port:    "8080",
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
		name:             "gateway",
		endpoint:         "/mcp/gateway",
		downstreams:      map[string]*DownstreamConnection{"server1": NewDownstreamConnection("server1", &config.MCPServerConfig{Command: "node"})},
		upstreamSessions: make(map[string]*UpstreamSession),
		downstreamPools:  make(map[string]*DownstreamConnectionPool),
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
	server1 := proxy.GetServerForRequest(request1)
	firstSessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
	attachInitializedSessionForTest(t, proxy, firstSessionID, &mcp.ClientCapabilities{})
	request2.Header.Set("Mcp-Session-Id", "session-2")
	server2 := proxy.GetServerForRequest(request2)
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
		name:             "gateway",
		endpoint:         "/mcp/gateway",
		downstreams:      map[string]*DownstreamConnection{"server1": NewDownstreamConnection("server1", &config.MCPServerConfig{Command: "node"})},
		upstreamSessions: make(map[string]*UpstreamSession),
		downstreamPools:  make(map[string]*DownstreamConnectionPool),
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
	server1 := proxy.GetServerForRequest(request1)
	firstSessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
	server2 := proxy.GetServerForRequest(request2)
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
		name:             "gateway",
		endpoint:         "/mcp/gateway",
		downstreams:      map[string]*DownstreamConnection{"server1": NewDownstreamConnection("server1", &config.MCPServerConfig{Command: "node"})},
		upstreamSessions: make(map[string]*UpstreamSession),
		downstreamPools:  make(map[string]*DownstreamConnectionPool),
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
	server1 := proxy.GetServerForRequest(request1)
	firstSessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
	server2 := proxy.GetServerForRequest(request2)
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
		name:             "gateway",
		endpoint:         "/mcp/gateway",
		downstreams:      map[string]*DownstreamConnection{"server1": NewDownstreamConnection("server1", &config.MCPServerConfig{Command: "node"})},
		upstreamSessions: make(map[string]*UpstreamSession),
		downstreamPools:  make(map[string]*DownstreamConnectionPool),
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
		serverResult <- proxy.GetServerForRequest(request)
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
		name:             "gateway",
		endpoint:         "/mcp/gateway",
		downstreams:      map[string]*DownstreamConnection{"server1": NewDownstreamConnection("server1", &config.MCPServerConfig{Command: "node"})},
		upstreamSessions: make(map[string]*UpstreamSession),
		downstreamPools:  make(map[string]*DownstreamConnectionPool),
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
	server1 := proxy.GetServerForRequest(request1)
	firstSessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
	server2 := proxy.GetServerForRequest(request2)
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
	// Given: an aggregated proxy where one downstream fails the first time
	createdByServer := make(map[string]int)
	proxy := &CentianEndpoint{
		name:     "gateway",
		endpoint: "/mcp/gateway",
		downstreams: map[string]*DownstreamConnection{
			"server1": NewDownstreamConnection("server1", &config.MCPServerConfig{Command: "node"}),
			"server2": NewDownstreamConnection("server2", &config.MCPServerConfig{Command: "node"}),
		},
		upstreamSessions: make(map[string]*UpstreamSession),
		downstreamPools:  make(map[string]*DownstreamConnectionPool),
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
			if name == "server2" && createdByServer[name] == 1 {
				conn.ErrorToReturn = errors.New("dial failed")
				conn.Status = StatusFailed
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

	// When: the first session creates a partial failure
	server1 := proxy.GetServerForRequest(request1)
	firstSessionID := findOnlyUpstreamSessionIDForTest(t, proxy)
	firstSession := attachInitializedSessionForTest(t, proxy, firstSessionID, &mcp.ClientCapabilities{})
	waitForCondition(t, time.Second, func() bool {
		proxy.mu.RLock()
		defer proxy.mu.RUnlock()
		entry, ok := proxy.downstreamPools[firstSession.downstreamSessionKey]
		if !ok {
			return false
		}
		conn, exists := entry.downstreamConns["server2"]
		return exists && conn.GetStatus() == StatusFailed && !entry.connecting["server2"]
	})

	// And: a later session with the same identity arrives after that failure settled
	server2 := proxy.GetServerForRequest(request2)
	attachInitializedSessionForTest(t, proxy, "session-2", &mcp.ClientCapabilities{})

	// Then: the failed downstream should be retried for the later session
	assert.Assert(t, server1 != nil)
	assert.Assert(t, server2 != nil)
	assert.Equal(t, createdByServer["server2"], 2)
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

	proxy.invalidatePooledDownstream("pool-1")

	assert.Equal(t, conn.CloseCalls, 1)
	assert.Assert(t, proxy.downstreamPools["pool-1"] == nil)
}

func TestClosePoolEntryLocked(t *testing.T) {
	okConn := &MockDownstreamConnection{}
	failingConn := &closeErrorDownstreamConnection{
		MockDownstreamConnection: &MockDownstreamConnection{},
		closeErr:                 errors.New("close failed"),
	}

	errs := (&CentianEndpoint{}).closePoolEntryLocked(&DownstreamConnectionPool{
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
