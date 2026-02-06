package proxy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolCallContext handles standard tool calls (client → downstream → client).
// Implements the CallContext interface.
type ToolCallContext struct {
	// Infrastructure references
	proxy   *MCPProxy            // Gateway (has back-ref to server via proxy.server)
	session *CentianProxySession // Session (downstream connections)

	// Original request (immutable - deep cloned for auditing/comparison)
	originalServerName string
	originalRequest    *mcp.CallToolRequest

	// Current request (mutable - handlers modify this)
	serverName string
	request    *mcp.CallToolRequest

	// Headers (mutable - HeadersHandler can modify, Phase 3)
	downstreamHeaders map[string]string

	// Response (set by SendRequest, mutable by response processors)
	result *mcp.CallToolResult

	// State
	direction Direction
}

// NewToolCallContext creates a new ToolCallContext.
// Returns CallContext interface to allow implementation swapping.
func NewToolCallContext(
	proxy *MCPProxy,
	session *CentianProxySession,
	serverName string,
	req *mcp.CallToolRequest,
) CallContext {
	return &ToolCallContext{
		proxy:              proxy,
		session:            session,
		originalServerName: serverName,
		originalRequest:    deepCloneRequest(req), // Immutable clone
		serverName:         serverName,
		request:            req, // Mutable, will be modified by handlers
		downstreamHeaders:  copyHeaders(session.authHeaders),
		direction:          DirectionRequest,
	}
}

// copyHeaders creates a copy of the headers map.
func copyHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return make(map[string]string)
	}
	result := make(map[string]string, len(headers))
	for k, v := range headers {
		result[k] = v
	}
	return result
}

// SendRequest executes the downstream call using current request state.
func (c *ToolCallContext) SendRequest(ctx context.Context) error {
	// Resolve connection based on (potentially modified) serverName
	conn, ok := c.session.downstreamConns[c.serverName]
	if !ok {
		return fmt.Errorf("server %s not found (original: %s)",
			c.serverName, c.originalServerName)
	}

	// Parse arguments from current request
	var args map[string]any
	if err := json.Unmarshal(c.request.Params.Arguments, &args); err != nil {
		return fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Make the call with current request data
	// Note: per-request headers require CallTool signature change (Phase 3)
	result, err := conn.CallTool(ctx, c.request.Params.Name, args)
	if err != nil {
		return err
	}
	c.result = result
	return nil
}

// Direction methods

func (c *ToolCallContext) GetDirection() Direction {
	return c.direction
}

func (c *ToolCallContext) SetDirection(d Direction) {
	c.direction = d
}

// Result methods

func (c *ToolCallContext) GetResult() *mcp.CallToolResult {
	return c.result
}

func (c *ToolCallContext) SetResult(result *mcp.CallToolResult) {
	c.result = result
}

// Original request accessors (immutable)

func (c *ToolCallContext) GetOriginalServerName() string {
	return c.originalServerName
}

func (c *ToolCallContext) GetOriginalRequest() *mcp.CallToolRequest {
	return c.originalRequest
}

func (c *ToolCallContext) GetOriginalToolName() string {
	if c.originalRequest == nil || c.originalRequest.Params == nil {
		return ""
	}
	return c.originalRequest.Params.Name
}

// Current request accessors (mutable)

func (c *ToolCallContext) GetServerName() string {
	return c.serverName
}

func (c *ToolCallContext) SetServerName(name string) {
	c.serverName = name
}

func (c *ToolCallContext) GetRequest() *mcp.CallToolRequest {
	return c.request
}

func (c *ToolCallContext) GetToolName() string {
	if c.request == nil || c.request.Params == nil {
		return ""
	}
	return c.request.Params.Name
}

// Config accessors

func (c *ToolCallContext) GetGlobalConfig() *config.GlobalConfig {
	if c.proxy == nil || c.proxy.server == nil {
		return nil
	}
	return c.proxy.server.Config
}

func (c *ToolCallContext) GetGatewayConfig() *config.GatewayConfig {
	if c.proxy == nil {
		return nil
	}
	return c.proxy.config
}

// Compile-time interface check
var _ CallContext = (*ToolCallContext)(nil)