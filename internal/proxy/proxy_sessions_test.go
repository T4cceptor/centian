package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/identifiers"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestDeriveDownstreamClientState(t *testing.T) {
	state := deriveDownstreamClientState(nil)
	assert.Assert(t, state != nil)
	assert.Equal(t, state.ProtocolVersion, "")
	assert.Assert(t, state.ClientCapabilities == nil)
	assert.Assert(t, state.Roots == nil)
	assert.Equal(t, state.CapabilitiesFingerprint, "")
	assert.Equal(t, state.RootsFingerprint, "")

	captured := &capturedUpstreamClientState{
		protocolVersion: "2025-06-18",
		capabilities: &mcp.ClientCapabilities{
			RootsV2: &mcp.RootCapabilities{ListChanged: true},
		},
		roots: []*mcp.Root{{
			Name: "workspace",
			URI:  "file:///tmp/workspace",
		}},
	}

	assert.DeepEqual(t, deriveDownstreamClientState(captured), buildDownstreamClientState(
		captured.protocolVersion,
		captured.capabilities,
		captured.roots,
	))
}

func TestGetSessionIDGeneratesCanonicalSessionIDsForNewPosts(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway/server-a", http.NoBody)

	sessionID := getSessionID(request)

	assert.Assert(t, identifiers.IsKind(sessionID, identifiers.KindSession))
}

func TestGetOrCreateServerForRequest_SessionAndIdentityGuards(t *testing.T) {
	newEndpoint := func(withAuth bool) *CentianEndpoint {
		endpoint := NewSingleEndpoint("server-a", "/mcp/gateway/server-a", &config.GatewayConfig{
			MCPServers: map[string]*config.MCPServerConfig{
				"server-a": {Command: "node"},
			},
		})
		endpoint.server = &CentianServer{
			Config: &config.GlobalConfig{Version: "1.0.0"},
		}
		if withAuth {
			endpoint.server.APIKeys = createTestAPIKeyStore(t)
		}
		return endpoint
	}

	sharedEndpoint := newEndpoint(false)
	assert.Assert(t, sharedEndpoint.GetOrCreateServerForRequest(nil) == nil)

	getRequest := httptest.NewRequest(http.MethodGet, "http://example.com/mcp/gateway/server-a", http.NoBody)
	assert.Assert(t, sharedEndpoint.GetOrCreateServerForRequest(getRequest) == nil)

	createRequest := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway/server-a", http.NoBody)
	server := sharedEndpoint.GetOrCreateServerForRequest(createRequest)
	assert.Assert(t, server != nil)

	sessionID := findOnlyUpstreamSessionIDForTest(t, sharedEndpoint)
	reuseRequest := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway/server-a", http.NoBody)
	reuseRequest.Header.Set("Mcp-Session-Id", sessionID)
	assert.Assert(t, sharedEndpoint.GetOrCreateServerForRequest(reuseRequest) == server)

	authEndpoint := newEndpoint(true)
	missingIdentity := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway/server-a", http.NoBody)
	assert.Assert(t, authEndpoint.GetOrCreateServerForRequest(missingIdentity) == nil)

	firstRequest := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway/server-a", http.NoBody)
	firstRequest = firstRequest.WithContext(withRequestIdentity(context.Background(), "auth:key_1"))
	firstServer := authEndpoint.GetOrCreateServerForRequest(firstRequest)
	assert.Assert(t, firstServer != nil)

	authSessionID := findOnlyUpstreamSessionIDForTest(t, authEndpoint)
	mismatchRequest := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway/server-a", http.NoBody)
	mismatchRequest.Header.Set("Mcp-Session-Id", authSessionID)
	mismatchRequest = mismatchRequest.WithContext(withRequestIdentity(context.Background(), "auth:key_2"))
	assert.Assert(t, authEndpoint.GetOrCreateServerForRequest(mismatchRequest) == nil)
}

func TestSessionNeedsInitialRootsBootstrap(t *testing.T) {
	proxy := &CentianEndpoint{
		upstreamSessions: make(map[string]*UpstreamSession),
	}
	session := &UpstreamSession{
		id:                 "session-1",
		clientCapabilities: &mcp.ClientCapabilities{RootsV2: &mcp.RootCapabilities{ListChanged: true}},
		rootsDirty:         true,
	}
	proxy.upstreamSessions[session.id] = session

	assert.Assert(t, proxy.sessionNeedsInitialRootsBootstrap(session.id))

	session.rootsDirty = false
	assert.Assert(t, !proxy.sessionNeedsInitialRootsBootstrap(session.id))

	session.rootsDirty = true
	session.clientCapabilities = &mcp.ClientCapabilities{}
	assert.Assert(t, !proxy.sessionNeedsInitialRootsBootstrap(session.id))
}

func TestApplyClientStateLockedPropagatesResolvedRoots(t *testing.T) {
	proxy := NewSingleEndpoint("server-a", "/mcp/server-a", &config.GatewayConfig{
		MCPServers: map[string]*config.MCPServerConfig{
			"server-a": {Command: "node"},
		},
	})
	request, err := http.NewRequest(http.MethodPost, "http://example.com/mcp/server-a", http.NoBody)
	assert.NilError(t, err)

	session := proxy.createUpstreamSession("session-1", request, sharedLocalIdentity)
	proxy.upstreamSessions[session.id] = session

	mockConn := &MockDownstreamConnection{serverName: "server-a"}
	proxy.connectionFactory = func(string, *config.MCPServerConfig) DownstreamConnectionInterface {
		return mockConn
	}

	initialState := buildDownstreamClientState("2025-06-18", &mcp.ClientCapabilities{
		RootsV2: &mcp.RootCapabilities{ListChanged: true},
	}, nil)

	proxy.mu.Lock()
	initial := proxy.applyClientStateLocked(session, initialState, true)
	proxy.mu.Unlock()
	assert.Assert(t, initial.pool != nil)
	assert.Assert(t, proxy.sessionNeedsInitialRootsBootstrap(session.id))

	resolvedRoots := []*mcp.Root{{
		Name: "workspace",
		URI:  "file:///tmp/workspace",
	}}
	resolvedState := buildDownstreamClientState("2025-06-18", &mcp.ClientCapabilities{
		RootsV2: &mcp.RootCapabilities{ListChanged: true},
	}, resolvedRoots)

	proxy.mu.Lock()
	update := proxy.applyClientStateLocked(session, resolvedState, false)
	proxy.mu.Unlock()
	proxy.finalizeDownstreamPoolUpdate(context.Background(), session, update)

	assert.Assert(t, update.pool != nil)
	assert.Assert(t, session.downstreamSessionKey != "")
	assert.Assert(t, !proxy.sessionNeedsInitialRootsBootstrap(session.id))

	waitForCondition(t, 3*time.Second, func() bool {
		mockConn.mu.RLock()
		defer mockConn.mu.RUnlock()

		for _, captured := range mockConn.CapturedConnects {
			if reflect.DeepEqual(normalizeRoots(captured.ClientState.Roots), normalizeRoots(resolvedRoots)) {
				return true
			}
		}
		return false
	})
}

func TestMarkUpstreamSessionRootsDirty(t *testing.T) {
	proxy := &CentianEndpoint{
		upstreamSessions: map[string]*UpstreamSession{
			"session-1": {id: "session-1", rootsDirty: false},
		},
	}

	proxy.markUpstreamSessionRootsDirty("session-1")
	proxy.markUpstreamSessionRootsDirty("missing")

	assert.Assert(t, proxy.upstreamSessions["session-1"].rootsDirty)
}

func TestApplyClientStateLockedPoolTransitions(t *testing.T) {
	proxy := NewSingleEndpoint("server-a", "/mcp/server-a", &config.GatewayConfig{
		MCPServers: map[string]*config.MCPServerConfig{
			"server-a": {Command: "node"},
		},
	})
	proxy.connectionFactory = func(serverName string, cfg *config.MCPServerConfig) DownstreamConnectionInterface {
		return &MockDownstreamConnection{
			serverName: serverName,
			cfg:        cfg,
			Status:     StatusConnected,
		}
	}

	newRequest := func(t *testing.T, auth string) *http.Request {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, "http://example.com/mcp/server-a", http.NoBody)
		assert.NilError(t, err)
		request.Header.Set("Authorization", auth)
		return request
	}

	stateA := buildDownstreamClientState("2025-06-18", &mcp.ClientCapabilities{}, []*mcp.Root{{Name: "a", URI: "file:///a"}})
	stateB := buildDownstreamClientState("2025-06-18", &mcp.ClientCapabilities{}, []*mcp.Root{{Name: "b", URI: "file:///b"}})

	sessionA := proxy.createUpstreamSession("session-a", newRequest(t, "Bearer shared"), sharedLocalIdentity)
	sessionB := proxy.createUpstreamSession("session-b", newRequest(t, "Bearer shared"), sharedLocalIdentity)
	proxy.upstreamSessions[sessionA.id] = sessionA
	proxy.upstreamSessions[sessionB.id] = sessionB

	proxy.mu.Lock()
	initialA := proxy.applyClientStateLocked(sessionA, stateA, false)
	proxy.mu.Unlock()
	proxy.finalizeDownstreamPoolUpdate(context.Background(), sessionA, initialA)

	proxy.mu.Lock()
	initialB := proxy.applyClientStateLocked(sessionB, stateB, false)
	proxy.mu.Unlock()
	proxy.finalizeDownstreamPoolUpdate(context.Background(), sessionB, initialB)

	originalKeyA := sessionA.downstreamSessionKey
	originalPoolA := proxy.downstreamPools[originalKeyA]
	targetKey := sessionB.downstreamSessionKey
	targetPool := proxy.downstreamPools[targetKey]
	assert.Assert(t, originalPoolA != nil)
	assert.Assert(t, targetPool != nil)
	assert.Assert(t, originalPoolA != targetPool)

	proxy.mu.Lock()
	moveToExisting := proxy.applyClientStateLocked(sessionA, stateB, false)
	proxy.mu.Unlock()

	assert.Assert(t, moveToExisting.pool == targetPool)
	assert.Assert(t, moveToExisting.reused)
	assert.Assert(t, moveToExisting.closePool == originalPoolA)
	assert.Assert(t, !moveToExisting.syncPool)
	assert.Assert(t, !moveToExisting.waitForReady)
	assert.Equal(t, sessionA.downstreamSessionKey, targetKey)
	assert.Assert(t, sessionA.downstreamConns["server-a"] == targetPool.downstreamConns["server-a"])
	assert.Equal(t, len(originalPoolA.upstreamSessions), 0)

	sharedState := buildDownstreamClientState("2025-06-18", &mcp.ClientCapabilities{}, []*mcp.Root{{Name: "shared", URI: "file:///shared"}})
	sessionC := proxy.createUpstreamSession("session-c", newRequest(t, "Bearer pooled"), sharedLocalIdentity)
	sessionD := proxy.createUpstreamSession("session-d", newRequest(t, "Bearer pooled"), sharedLocalIdentity)
	proxy.upstreamSessions[sessionC.id] = sessionC
	proxy.upstreamSessions[sessionD.id] = sessionD

	proxy.mu.Lock()
	initialC := proxy.applyClientStateLocked(sessionC, sharedState, false)
	proxy.mu.Unlock()
	proxy.finalizeDownstreamPoolUpdate(context.Background(), sessionC, initialC)

	proxy.mu.Lock()
	initialD := proxy.applyClientStateLocked(sessionD, sharedState, false)
	proxy.mu.Unlock()
	proxy.finalizeDownstreamPoolUpdate(context.Background(), sessionD, initialD)

	sharedOldKey := sessionC.downstreamSessionKey
	sharedPool := proxy.downstreamPools[sharedOldKey]
	assert.Assert(t, sharedPool != nil)
	assert.Equal(t, len(sharedPool.upstreamSessions), 2)

	changedState := buildDownstreamClientState("2025-06-18", &mcp.ClientCapabilities{}, []*mcp.Root{{Name: "changed", URI: "file:///changed"}})
	proxy.mu.Lock()
	migrateShared := proxy.applyClientStateLocked(sessionC, changedState, false)
	proxy.mu.Unlock()

	assert.Assert(t, migrateShared.pool != nil)
	assert.Assert(t, migrateShared.pool != sharedPool)
	assert.Assert(t, !migrateShared.reused)
	assert.Assert(t, migrateShared.waitForReady)
	assert.Assert(t, migrateShared.closePool == nil)
	assert.Equal(t, len(sharedPool.upstreamSessions), 1)
	assert.Assert(t, proxy.downstreamPools[sharedOldKey] == sharedPool)
	assert.Assert(t, sessionC.downstreamConns["server-a"] == migrateShared.pool.downstreamConns["server-a"])

	stableState := buildDownstreamClientState("2025-06-18", &mcp.ClientCapabilities{}, []*mcp.Root{{Name: "stable", URI: "file:///stable"}})
	sessionE := proxy.createUpstreamSession("session-e", newRequest(t, "Bearer stable"), sharedLocalIdentity)
	proxy.upstreamSessions[sessionE.id] = sessionE

	proxy.mu.Lock()
	initialE := proxy.applyClientStateLocked(sessionE, stableState, false)
	proxy.mu.Unlock()
	proxy.finalizeDownstreamPoolUpdate(context.Background(), sessionE, initialE)

	stablePool := proxy.downstreamPools[sessionE.downstreamSessionKey]
	assert.Assert(t, stablePool != nil)

	proxy.mu.Lock()
	syncUpdate := proxy.applyClientStateLocked(sessionE, stableState, false)
	proxy.mu.Unlock()

	assert.Assert(t, syncUpdate.pool == stablePool)
	assert.Assert(t, syncUpdate.reused)
	assert.Assert(t, syncUpdate.syncPool)

	proxy.mu.Lock()
	noSyncWhenDirty := proxy.applyClientStateLocked(sessionE, stableState, true)
	proxy.mu.Unlock()

	assert.Assert(t, noSyncWhenDirty.pool == stablePool)
	assert.Assert(t, !noSyncWhenDirty.syncPool)
}
