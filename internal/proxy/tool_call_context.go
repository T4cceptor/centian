package proxy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolCallContext handles standard tool calls (client → downstream → client).
// Implements the CallContext interface.
type ToolCallContext struct {
	// Infrastructure references
	proxy           *CentianEndpoint // Gateway (has back-ref to server via proxy.server)
	upstreamSession *UpstreamSession // The upstream client session currently executing the tool call.

	// Original request (immutable - deep cloned for auditing/comparison)
	originalServerName string
	originalRequest    *mcp.CallToolRequest

	// Current request (mutable - handlers modify this)
	request *mcp.CallToolRequest

	// Response (set by SendRequest, mutable by response processors)
	result         *mcp.CallToolResult
	originalResult *mcp.CallToolResult // unmodified result from downstream

	// State
	event *common.MCPEvent

	// Routing context (reuses common.RoutingContext)
	routingContext *common.RoutingContext
	authData       *AuthData

	// Handlers
	handlers   map[string]CallContextHandler
	logHandler LogHandler
}

// NewToolCallContext creates a new ToolCallContext.
// Returns CallContext interface to allow implementation swapping.
func NewToolCallContext(
	proxy *CentianEndpoint,
	upstreamSession *UpstreamSession,
	serverName string,
	req *mcp.CallToolRequest,
) (CallContext, error) {
	// nil check
	if proxy.server == nil {
		return nil, fmt.Errorf("server attached to the proxy component is nil (%v)", proxy)
	}
	// Build routing context
	routingCtx := buildRoutingContext(proxy, upstreamSession, serverName)
	// TODO: get headers from ctx

	conn, err := upstreamSession.GetConnectionByServerName(serverName)
	transport := common.UnknownTransport
	if err != nil {
		common.LogWarn("unable to get connection for '%s': %v", serverName, err)
	} else {
		transport = conn.GetConfig().GetTransport()
	}

	event := common.NewMCPRequestEvent(string(transport)).
		WithRequestID(getNewUUIDV7()).
		WithSessionID(upstreamSession.id).
		WithServerID(proxy.server.ServerID) // nil check was done above

	// request holds the current mutable MCP tool name. In aggregated mode we
	// normalize it once before processing so all later code sees the current
	// downstream tool name directly. originalRequest keeps the raw upstream name.
	originalRequest := deepCloneRequest(req) // cloned here because req is being modified
	if proxy.isAggregatedProxy {
		if req == nil || req.Params == nil {
			return nil, fmt.Errorf("aggregated tool call requires request params")
		}
		toolName, err := parseAggregatedToolName(req.Params.Name, serverName)
		if err != nil {
			return nil, err
		}
		req.Params.Name = toolName
	}

	toolCallCtx := &ToolCallContext{
		proxy:              proxy,
		upstreamSession:    upstreamSession,
		originalServerName: serverName,
		originalRequest:    originalRequest, // Immutable clone of the upstream request
		request:            req,             // Mutable, will be modified by handlers
		routingContext:     routingCtx,
		event:              event,
		authData:           upstreamSession.authData.Clone(),
	}

	// Register handlers
	toolCallCtx.SetHandler("payload", &DefaultPayloadHandler{})
	toolCallCtx.SetHandler("meta", &DefaultMetaHandler{})
	toolCallCtx.SetHandler("routing", &DefaultRoutingHandler{})
	toolCallCtx.SetHandler("auth", &DefaultAuthHandler{})

	// Set default log handler - proxy.server nil check was done above
	if proxy.server.Logger == nil {
		return nil, fmt.Errorf("unable to get logger from centian server")
	}
	toolCallCtx.SetLogHandler(NewDefaultLogHandler(proxy.server.Logger))
	return toolCallCtx, nil
}

// buildRoutingContext creates a RoutingContext from proxy and session info.
func buildRoutingContext(proxy *CentianEndpoint, upstreamSession *UpstreamSession, serverName string) *common.RoutingContext {
	// ideally we would combine this somehow with MCPevent data struct
	rc := &common.RoutingContext{
		Gateway:    proxy.name,
		ServerName: serverName,
		Endpoint:   proxy.endpoint,
	}

	// Try to get connection details
	if conn, ok := upstreamSession.downstreamConns[serverName]; ok {
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

// SendRequest executes the downstream call using current request state.
// Note: if processors returned a status >= 400 this will NOT trigger a
// downstream call, instead an immediate error response will be returned.
func (c *ToolCallContext) SendRequest(ctx context.Context) error {
	// Resolve connection based on (potentially modified) serverName
	serverName := c.GetServerName()
	conn, err := c.upstreamSession.GetConnectionByServerName(serverName)
	if err != nil {
		return err
	}
	if !conn.IsConnected() {
		return fmt.Errorf("server %s found (original: %s), but not connected",
			serverName, c.originalServerName)
	}

	// Parse arguments from current request
	var args map[string]any
	if err := json.Unmarshal(c.request.Params.Arguments, &args); err != nil {
		return fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Make the call with current request data
	// Note: per-request headers require CallTool signature change (Phase 3)
	result, err := conn.CallTool(ctx, c.GetToolName(), args)
	if err != nil {
		return normalizeForwardedMethodError(mcpMethodCallTool, err)
	}
	c.SetResult(result)
	return nil
}

// GetEventInfo returns the attached MCPEvent.
func (c *ToolCallContext) GetEventInfo() *common.MCPEvent {
	return c.event
}

// SetEventInfo sets the provided MCPEvent.
func (c *ToolCallContext) SetEventInfo(event *common.MCPEvent) {
	c.event = event
}

// Result methods

// HasResult returns true if a result is available.
func (c *ToolCallContext) HasResult() bool {
	return c.result != nil
}

// GetResult returns the CallToolResult for this CallContext.
func (c *ToolCallContext) GetResult() *mcp.CallToolResult {
	return c.result
}

// SetResult sets the CallToolResult for this CallContext.
func (c *ToolCallContext) SetResult(result *mcp.CallToolResult) {
	if c.originalResult == nil {
		c.originalResult = result
	}
	c.result = result
}

// GetOriginalResult returns the initial, original CallToolResult.
func (c *ToolCallContext) GetOriginalResult() *mcp.CallToolResult {
	return c.originalResult
}

// Direction methods

// GetDirection returns MCPEvent.Direction.
func (c *ToolCallContext) GetDirection() common.McpEventDirection {
	return c.event.Direction
}

// SetDirection sets MCPEvent.Direction.
func (c *ToolCallContext) SetDirection(d common.McpEventDirection) {
	c.event.Direction = d
}

// GetMessageType returns MCPEvent.MessageType.
func (c *ToolCallContext) GetMessageType() common.McpMessageType {
	return c.event.MessageType
}

// SetMessageType sets MCPEvent.MessageType.
func (c *ToolCallContext) SetMessageType(t common.McpMessageType) {
	c.event.MessageType = t
}

// Original request accessors

// GetOriginalServerName returns the initial, original server name.
func (c *ToolCallContext) GetOriginalServerName() string {
	return c.originalServerName
}

// GetOriginalRequest returns a reference to the original CallToolRequest.
func (c *ToolCallContext) GetOriginalRequest() *mcp.CallToolRequest {
	return c.originalRequest
}

// GetOriginalToolName returns the initial tool name.
func (c *ToolCallContext) GetOriginalToolName() string {
	if c.originalRequest == nil || c.originalRequest.Params == nil {
		return ""
	}
	return c.originalRequest.Params.Name
}

// Current request accessors (mutable)

// GetServerName returns the current server name from routing context.
func (c *ToolCallContext) GetServerName() string {
	if c.routingContext == nil {
		return ""
	}
	return c.routingContext.ServerName
}

// SetServerName sets the server name in routing context.
//
// Note: creates routing context if it doesn't exist.
func (c *ToolCallContext) SetServerName(name string) {
	if c.routingContext == nil {
		c.routingContext = buildRoutingContext(c.proxy, c.upstreamSession, c.GetServerName())
	}
	c.routingContext.ServerName = name
}

// GetRequest returns a reference to the current CallToolRequest - allows modifications.
func (c *ToolCallContext) GetRequest() *mcp.CallToolRequest {
	return c.request
}

// GetToolName returns the current tool name.
func (c *ToolCallContext) GetToolName() string {
	if c.request == nil || c.request.Params == nil {
		return ""
	}
	return c.request.Params.Name
}

// Status and error handling

// GetStatus returns ToolCallContext.MCPEvent.Status.
func (c *ToolCallContext) GetStatus() int {
	return c.event.Status
}

// SetStatus sets ToolCallContext.MCPEvent.Status.
func (c *ToolCallContext) SetStatus(status int) {
	c.event.Status = status
}

// GetError returns ToolCallContext.MCPEvent.Error.
func (c *ToolCallContext) GetError() string {
	return c.event.Error
}

// SetError sets the ToolCallContext.MCPEvent.Error.
func (c *ToolCallContext) SetError(msg string) {
	c.event.Error = msg
}

// Session and request identification

// GetRequestID returns the current request ID.
func (c *ToolCallContext) GetRequestID() string {
	return c.event.RequestID
}

// GetSessionID returns the current session ID.
func (c *ToolCallContext) GetSessionID() string {
	if c.upstreamSession == nil {
		return ""
	}
	return c.upstreamSession.id
}

// GetAuthData returns auth mapping data attached to this call.
func (c *ToolCallContext) GetAuthData() *AuthData {
	return c.authData
}

// Routing context

// GetRoutingContext returns the attached RoutingLog.
func (c *ToolCallContext) GetRoutingContext() *common.RoutingContext {
	return c.routingContext
}

// Handler access

// GetHandler returns the handler for the given part.
func (c *ToolCallContext) GetHandler(part string) (CallContextHandler, bool) {
	if c.handlers == nil {
		return nil, false
	}
	res, ok := c.handlers[part]
	return res, ok
}

// SetHandler sets the provided handler for the provided part.
//
// Automatically creates the handlers map if it is nil.
func (c *ToolCallContext) SetHandler(part string, h CallContextHandler) {
	if c.handlers == nil {
		c.handlers = make(map[string]CallContextHandler)
	}
	c.handlers[part] = h
}

// GetLogHandler returns the attached log handler.
func (c *ToolCallContext) GetLogHandler() LogHandler {
	return c.logHandler
}

// SetLogHandler sets the provided log handler.
func (c *ToolCallContext) SetLogHandler(l LogHandler) {
	c.logHandler = l
}

// Compile-time interface check.
var _ CallContext = (*ToolCallContext)(nil)
