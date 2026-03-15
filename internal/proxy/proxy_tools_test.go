package proxy

import (
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestSyncAvailableToolsRemovesStaleTools(t *testing.T) {
	proxy := &CentianEndpoint{
		name:            "gateway",
		endpoint:        "/mcp/gateway",
		downstreamPools: make(map[string]*DownstreamSessionPool),
		server:          &CentianServer{Config: &config.GlobalConfig{Version: "1.0.0"}},
	}

	downstream := &MockDownstreamConnection{
		serverName: "server-1",
		Status:     StatusConnected,
		tools: []*mcp.Tool{
			{Name: "tool-a", Description: "a", InputSchema: map[string]any{"type": "object"}},
		},
	}

	session := &UpstreamSession{
		id:              "session-1",
		registeredTools: make(map[string]struct{}),
		downstreamConns: map[string]DownstreamConnectionInterface{"server-1": downstream},
		downstreamSessionKey: "pool-1",
	}
	session.upstreamServer = proxy.newUpstreamServer(session)

	proxy.syncAvailableTools(session)
	assert.Equal(t, len(session.registeredTools), 1)

	downstream.tools = nil
	proxy.syncAvailableTools(session)

	assert.Equal(t, len(session.registeredTools), 0)
}
