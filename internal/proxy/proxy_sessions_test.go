package proxy

import (
	"net/http"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestApplyClientStateLockedDefersInitialPoolUntilRootsResolve(t *testing.T) {
	proxy := NewSingleEndpoint("server-a", "/mcp/server-a", &config.GatewayConfig{
		MCPServers: map[string]*config.MCPServerConfig{
			"server-a": {Command: "node"},
		},
	})
	request, err := http.NewRequest(http.MethodPost, "http://example.com/mcp/server-a", http.NoBody)
	assert.NilError(t, err)

	session := proxy.createUpstreamSession("session-1", request, sharedLocalIdentity)
	proxy.upstreamSessions[session.id] = session

	state := buildDownstreamClientState("2025-06-18", &mcp.ClientCapabilities{
		RootsV2: &mcp.RootCapabilities{ListChanged: true},
	}, nil)

	proxy.mu.Lock()
	update := proxy.applyClientStateLocked(session, state, true)
	proxy.mu.Unlock()

	assert.Assert(t, update.pool == nil)
	assert.Equal(t, session.downstreamSessionKey, "")
	assert.Assert(t, session.rootsDirty)
	assert.Assert(t, clientSupportsRoots(session.clientCapabilities))
	assert.Assert(t, proxy.sessionNeedsInitialRootsBootstrap(session.id))

	proxy.mu.RLock()
	assert.Equal(t, len(proxy.downstreamPools), 0)
	proxy.mu.RUnlock()
}

func TestApplyClientStateLockedConnectsWithResolvedRoots(t *testing.T) {
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
	deferred := proxy.applyClientStateLocked(session, initialState, true)
	proxy.mu.Unlock()
	assert.Assert(t, deferred.pool == nil)

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

	assert.Assert(t, update.pool != nil)
	assert.Assert(t, session.downstreamSessionKey != "")
	assert.Assert(t, !proxy.sessionNeedsInitialRootsBootstrap(session.id))

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mockConn.mu.RLock()
		connected := mockConn.ConnectCalls > 0
		roots := []*mcp.Root(nil)
		if connected {
			roots = normalizeRoots(mockConn.CapturedConnects[0].ClientState.Roots)
		}
		mockConn.mu.RUnlock()

		if connected {
			assert.DeepEqual(t, roots, normalizeRoots(resolvedRoots))
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("expected downstream connect with resolved roots")
}
