package proxy

import (
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestRefreshAvailableToolsRemovesStaleTools(t *testing.T) {
	proxy := &MCPProxy{
		name:            "gateway",
		endpoint:        "/mcp/gateway",
		downstreamPools: make(map[string]*DownstreamSessionPool),
		server:          &CentianProxy{Config: &config.GlobalConfig{Version: "1.0.0"}},
	}

	downstream := &MockDownstreamConnection{
		serverName: "server-1",
		Status:     StatusConnected,
		tools: []*mcp.Tool{
			{Name: "tool-a", Description: "a", InputSchema: map[string]any{"type": "object"}},
		},
	}

	session := &UpstreamSession{
		id:                   "session-1",
		upstreamServer:       proxy.newUpstreamServer("session-1"),
		registeredTools:      make(map[string]struct{}),
		downstreamConns:      map[string]DownstreamConnectionInterface{"server-1": downstream},
		downstreamSessionKey: "pool-1",
	}

	proxy.refreshAvailableTools(session)
	assert.Equal(t, len(session.registeredTools), 1)

	downstream.tools = nil
	proxy.refreshAvailableTools(session)

	assert.Equal(t, len(session.registeredTools), 0)
}
