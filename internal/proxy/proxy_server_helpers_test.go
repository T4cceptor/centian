package proxy

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

// DownstreamConnectionPool is a test-only alias kept for readability in existing test setups.
type DownstreamConnectionPool = DownstreamSessionPool

func TestCreateSession_AuthHeaders(t *testing.T) {
	// Given: a proxy with a configured auth header
	proxy := &CentianEndpoint{
		name:     "gateway",
		endpoint: "/mcp/gateway",
		server:   &CentianServer{AuthHeader: "Authorization"},
	}
	request, err := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	assert.NilError(t, err)
	request.Header.Set("Authorization", "Bearer skip")
	request.Header.Set("X-API-Key", "keep")
	request.Header.Set("X-Auth-Token", "keep-too")

	// When: creating a session
	session := proxy.createUpstreamSession("session-1", request, sharedLocalIdentity)

	// Then: auth header is excluded and other headers are kept
	authHeaders := session.GetAuthHeaders(proxy.excludedClientAuthHeader())
	assert.Equal(t, authHeaders["X-API-Key"], "keep")
	assert.Equal(t, authHeaders["X-Auth-Token"], "keep-too")
	_, exists := authHeaders["Authorization"]
	assert.Assert(t, !exists)
}

func TestCreateSession_IncludesAuthorizationWhenNotConfigured(t *testing.T) {
	// Given: a proxy without a configured auth header
	proxy := &CentianEndpoint{}
	request, err := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	assert.NilError(t, err)
	request.Header.Set("Authorization", "Bearer token")

	// When: creating a session
	session := proxy.createUpstreamSession("session-1", request, sharedLocalIdentity)

	// Then: authorization is captured
	assert.Equal(t, session.GetAuthHeaders(proxy.excludedClientAuthHeader())["Authorization"], "Bearer token")
}

func TestCopyToolForRegistration(t *testing.T) {
	// Given: a tool with metadata
	tool := &mcp.Tool{
		Name:        "tool",
		Description: "desc",
		InputSchema: map[string]any{"type": "object"},
	}

	// When: cloning the tool
	clone := copyToolForRegistration(tool)

	// Then: cloned tool matches values
	assert.Assert(t, clone != tool)
	assert.Equal(t, clone.Name, tool.Name)
	assert.Equal(t, clone.Description, tool.Description)

	got := fmt.Sprintf("%#v", clone.InputSchema)
	expected := fmt.Sprintf("%#v", tool.InputSchema)
	assert.Equal(t, got, expected)
}

func TestNewAggregatedProxy(t *testing.T) {
	// Given: a gateway config with enabled and disabled servers
	enabled := true
	disabled := false
	gatewayConfig := &config.GatewayConfig{
		MCPServers: map[string]*config.MCPServerConfig{
			"enabled":  {Command: "node", Enabled: &enabled},
			"disabled": {Command: "node", Enabled: &disabled},
		},
	}

	// When: creating an aggregated proxy
	proxy := NewAggregatedEndpoint("gateway", "/mcp/gateway", gatewayConfig)

	// Then: only enabled servers are present
	assert.Assert(t, proxy.isAggregatedProxy)
	activeConfigs := proxy.GetActiveMCPServerConfigs()
	assert.Equal(t, len(activeConfigs), 1)
	_, ok := activeConfigs["enabled"]
	assert.Assert(t, ok)
}

func TestNewSingleProxy(t *testing.T) {
	// Given: a server config
	gatewayConfig := &config.GatewayConfig{
		MCPServers: map[string]*config.MCPServerConfig{
			"server": {Command: "node"},
		},
	}

	// When: creating a single proxy
	proxy := NewSingleEndpoint("server", "/mcp/gateway/server", gatewayConfig)

	// Then: proxy is not aggregated and has one downstream
	assert.Assert(t, !proxy.isAggregatedProxy)
	activeConfigs := proxy.GetActiveMCPServerConfigs()
	assert.Equal(t, len(activeConfigs), 1)
	_, ok := activeConfigs["server"]
	assert.Assert(t, ok)
}
