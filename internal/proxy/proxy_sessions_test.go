package proxy

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

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

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mockConn.mu.RLock()
		connected := mockConn.ConnectCalls > 0
		foundResolvedRoots := false
		if connected {
			for _, captured := range mockConn.CapturedConnects {
				if reflect.DeepEqual(normalizeRoots(captured.ClientState.Roots), normalizeRoots(resolvedRoots)) {
					foundResolvedRoots = true
					break
				}
			}
		}
		mockConn.mu.RUnlock()

		if connected {
			assert.Assert(t, foundResolvedRoots)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("expected downstream connect with resolved roots")
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
