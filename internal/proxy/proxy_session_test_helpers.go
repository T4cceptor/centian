package proxy

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func attachInitializedSessionForTest(
	t *testing.T,
	proxy *CentianEndpoint,
	sessionID string,
	capabilities *mcp.ClientCapabilities,
) {
	t.Helper()

	proxy.mu.RLock()
	session := proxy.upstreamSessions[sessionID]
	proxy.mu.RUnlock()

	assert.Assert(t, session != nil)

	state := buildDownstreamClientState("2025-06-18", capabilities, nil)

	proxy.mu.Lock()
	update := proxy.applyClientStateLocked(session, state, false)
	proxy.mu.Unlock()

	proxy.finalizeDownstreamPoolUpdate(context.Background(), session, update)
}

func findOnlyUpstreamSessionIDForTest(t *testing.T, proxy *CentianEndpoint) string {
	t.Helper()

	proxy.mu.RLock()
	defer proxy.mu.RUnlock()

	assert.Equal(t, len(proxy.upstreamSessions), 1)
	for sessionID := range proxy.upstreamSessions {
		return sessionID
	}
	t.Fatal("expected one upstream session")
	return ""
}
