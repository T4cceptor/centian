package proxy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/google/uuid"
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
	status    int    // 0 = not set, 200 = ok, 4xx/5xx = error
	errorMsg  string // Error message if status >= 400

	// Identification
	requestID string

	// Routing context (reuses common.RoutingContext)
	routingContext *common.RoutingContext

	// Handlers
	handlers   map[string]CallContextHandler
	logHandler LogHandler
}

// NewToolCallContext creates a new ToolCallContext.
// Returns CallContext interface to allow implementation swapping.
func NewToolCallContext(
	proxy *MCPProxy,
	session *CentianProxySession,
	serverName string,
	req *mcp.CallToolRequest,
) CallContext {
	// Build routing context
	routingCtx := buildRoutingContext(proxy, session, serverName)

	ctx := &ToolCallContext{
		proxy:              proxy,
		session:            session,
		originalServerName: serverName,
		originalRequest:    deepCloneRequest(req), // Immutable clone
		serverName:         serverName,
		request:            req, // Mutable, will be modified by handlers
		downstreamHeaders:  copyHeaders(session.authHeaders),
		direction:          DirectionRequest,
		status:             0,
		requestID:          uuid.New().String(),
		routingContext:     routingCtx,
	}

	// Register handlers
	ctx.handlers = map[string]CallContextHandler{
		"payload": &PayloadHandler{},
		"meta":    &MetaHandler{},
		"routing": &RoutingHandler{},
	}

	// Set default log handler
	ctx.logHandler = NewDefaultLogHandler()

	return ctx
}

// buildRoutingContext creates a RoutingContext from proxy and session info
func buildRoutingContext(proxy *MCPProxy, session *CentianProxySession, serverName string) *common.RoutingContext {
	rc := &common.RoutingContext{
		Gateway:    proxy.name,
		ServerName: serverName,
		Endpoint:   proxy.endpoint,
	}

	// Try to get connection details
	if conn, ok := session.downstreamConns[serverName]; ok {
		cfg := conn.GetConfig()
		if cfg != nil {
			if cfg.URL != "" {
				rc.Transport = common.HTTPTransport
				rc.DownstreamURL = cfg.URL
			} else if cfg.Command != "" {
				rc.Transport = common.StdioTransport
				rc.DownstreamCommand = cfg.Command
				rc.Args = cfg.Args
			}
		}
	}

	return rc
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

// Status and error handling

func (c *ToolCallContext) GetStatus() int {
	return c.status
}

func (c *ToolCallContext) SetStatus(status int) {
	c.status = status
}

func (c *ToolCallContext) GetError() string {
	return c.errorMsg
}

func (c *ToolCallContext) SetError(msg string) {
	c.errorMsg = msg
}

// Session and request identification

func (c *ToolCallContext) GetRequestID() string {
	return c.requestID
}

func (c *ToolCallContext) GetSessionID() string {
	if c.session == nil {
		return ""
	}
	return c.session.id
}

// Routing context

func (c *ToolCallContext) GetRoutingContext() *common.RoutingContext {
	return c.routingContext
}

// Handler access

func (c *ToolCallContext) GetHandler(part string) CallContextHandler {
	if c.handlers == nil {
		return nil
	}
	return c.handlers[part]
}

func (c *ToolCallContext) GetLogHandler() LogHandler {
	return c.logHandler
}

// Compile-time interface check
var _ CallContext = (*ToolCallContext)(nil)