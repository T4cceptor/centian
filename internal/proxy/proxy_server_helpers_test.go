package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestCreateSession_AuthHeaders(t *testing.T) {
	// Given: a proxy with a configured auth header
	proxy := &MCPProxy{
		name:     "gateway",
		endpoint: "/mcp/gateway",
		server:   &CentianProxy{AuthHeader: "Authorization"},
	}
	request, err := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	assert.NilError(t, err)
	request.Header.Set("Authorization", "Bearer skip")
	request.Header.Set("X-API-Key", "keep")
	request.Header.Set("X-Auth-Token", "keep-too")

	// When: creating a session
	session := proxy.createSession("session-1", request)

	// Then: auth header is excluded and other headers are kept
	assert.Equal(t, session.authHeaders["X-API-Key"], "keep")
	assert.Equal(t, session.authHeaders["X-Auth-Token"], "keep-too")
	_, exists := session.authHeaders["Authorization"]
	assert.Assert(t, !exists)
}

func TestCreateSession_IncludesAuthorizationWhenNotConfigured(t *testing.T) {
	// Given: a proxy without a configured auth header
	proxy := &MCPProxy{}
	request, err := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	assert.NilError(t, err)
	request.Header.Set("Authorization", "Bearer token")

	// When: creating a session
	session := proxy.createSession("session-1", request)

	// Then: authorization is captured
	assert.Equal(t, session.authHeaders["Authorization"], "Bearer token")
}

func TestGetRoutingContext_HTTP(t *testing.T) {
	// Given: a session with an HTTP downstream
	cfg := &config.MCPServerConfig{URL: "https://example.com", Command: ""}
	conn := NewDownstreamConnection("server", cfg)
	session := &CentianProxySession{downstreamConns: map[string]DownstreamConnectionInterface{"server": conn}}
	proxy := &MCPProxy{name: "gateway", endpoint: "/mcp/gateway"}

	// When: building routing context
	routing := getRoutingContext(proxy, session, "server")

	// Then: transport and routing details are set
	assert.Equal(t, routing.Transport, common.HTTPTransport)
	assert.Equal(t, routing.Gateway, "gateway")
	assert.Equal(t, routing.ServerName, "server")
	assert.Equal(t, routing.Endpoint, "/mcp/gateway")
	assert.Equal(t, routing.DownstreamURL, "https://example.com")
}

func TestGetRoutingContext_Stdio(t *testing.T) {
	// Given: a session with a stdio downstream
	cfg := &config.MCPServerConfig{Command: "node", Args: []string{"-v"}}
	conn := NewDownstreamConnection("server", cfg)
	session := &CentianProxySession{downstreamConns: map[string]DownstreamConnectionInterface{"server": conn}}
	proxy := &MCPProxy{name: "gateway", endpoint: "/mcp/gateway"}

	// When: building routing context
	routing := getRoutingContext(proxy, session, "server")

	// Then: stdio transport is used
	assert.Equal(t, routing.Transport, common.StdioTransport)
	assert.Equal(t, routing.DownstreamCommand, "node")
	assert.Equal(t, len(routing.Args), 1)
}

func TestGetRoutingContext_MissingConnection(t *testing.T) {
	// Given: a session without the server connection
	session := &CentianProxySession{downstreamConns: map[string]DownstreamConnectionInterface{}}
	proxy := &MCPProxy{name: "gateway", endpoint: "/mcp/gateway"}

	// When: building routing context
	routing := getRoutingContext(proxy, session, "missing")

	// Then: minimal routing data is returned
	assert.Equal(t, routing.Gateway, "gateway")
	assert.Equal(t, routing.ServerName, "missing")
	assert.Equal(t, routing.Endpoint, "/mcp/gateway")
}

func TestBuildRequestEvent(t *testing.T) {
	// Given: a proxy, session, and tool request
	cfg := &config.MCPServerConfig{URL: "https://example.com"}
	conn := NewDownstreamConnection("server", cfg)
	session := &CentianProxySession{id: "session-1", downstreamConns: map[string]DownstreamConnectionInterface{"server": conn}}
	proxy := &MCPProxy{name: "gateway", endpoint: "/mcp/gateway", server: &CentianProxy{ServerID: "server-1"}}
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "tool", Arguments: json.RawMessage(`{"a":1}`)}}

	// When: building a request event
	event := proxy.buildRequestEvent(session, "server", req)

	// Then: event fields are populated
	assert.Assert(t, event.RequestID != "")
	assert.Equal(t, event.SessionID, "session-1")
	assert.Equal(t, event.ServerID, "server-1")
	assert.Equal(t, event.ToolCall.Name, "tool")
	assert.Equal(t, event.Routing.Gateway, "gateway")
	assert.Equal(t, event.Transport, "http")
	assert.Assert(t, event.HasContent())
}

func TestBuildResponseEvent(t *testing.T) {
	// Given: a proxy, session, request, and result
	cfg := &config.MCPServerConfig{Command: "node"}
	conn := NewDownstreamConnection("server", cfg)
	session := &CentianProxySession{id: "session-1", downstreamConns: map[string]DownstreamConnectionInterface{"server": conn}}
	proxy := &MCPProxy{name: "gateway", endpoint: "/mcp/gateway", server: &CentianProxy{ServerID: "server-1"}}
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "tool", Arguments: json.RawMessage(`{"a":1}`)}}
	result := &mcp.CallToolResult{IsError: true}
	reqEvent := &common.MCPEvent{BaseMcpEvent: common.BaseMcpEvent{RequestID: "req-123"}}

	// When: building a response event
	event := proxy.buildResponseEvent(session, "server", req, result, reqEvent)

	// Then: event fields are populated and success is false
	assert.Equal(t, event.RequestID, "req-123")
	assert.Equal(t, event.SessionID, "session-1")
	assert.Equal(t, event.ServerID, "server-1")
	assert.Equal(t, event.ToolCall.Name, "tool")
	assert.Equal(t, event.Routing.Transport, common.StdioTransport)
	assert.Assert(t, event.Success == false)
	assert.Assert(t, event.HasContent())
}

func TestDeepCloneTool(t *testing.T) {
	// Given: a tool with metadata
	tool := &mcp.Tool{
		Name:        "tool",
		Description: "desc",
		InputSchema: map[string]any{"type": "object"},
	}

	// When: cloning the tool
	clone := deepCloneTool(tool)

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
	proxy := NewAggregatedProxy("gateway", "/mcp/gateway", gatewayConfig)

	// Then: only enabled servers are present
	assert.Assert(t, proxy.isAggregatedProxy)
	assert.Equal(t, len(proxy.downstreams), 1)
	_, ok := proxy.downstreams["enabled"]
	assert.Assert(t, ok)
}

func TestNewSingleProxy(t *testing.T) {
	// Given: a server config
	cfg := &config.MCPServerConfig{Command: "node"}

	// When: creating a single proxy
	proxy := NewSingleProxy("server", "/mcp/gateway/server", cfg)

	// Then: proxy is not aggregated and has one downstream
	assert.Assert(t, !proxy.isAggregatedProxy)
	assert.Equal(t, len(proxy.downstreams), 1)
	_, ok := proxy.downstreams["server"]
	assert.Assert(t, ok)
}
