package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/T4cceptor/centian/internal/common"
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
	request *mcp.CallToolRequest

	// Response (set by SendRequest, mutable by response processors)
	result         *mcp.CallToolResult
	originalResult *mcp.CallToolResult // unmodified result from downstream

	// State
	event *common.MCPEvent

	// Routing context (reuses common.RoutingContext)
	routingContext *common.RoutingLog

	// Handlers
	handlers   map[string]CallContextHandler
	logHandler LogHandler
}

// NewToolCallContext creates a new ToolCallContext.
// Returns CallContext interface to allow implementation swapping.
func NewToolCallContext(
	ctx context.Context,
	proxy *MCPProxy,
	session *CentianProxySession,
	serverName string,
	req *mcp.CallToolRequest,
) (CallContext, error) {
	// nil check
	if proxy.server == nil {
		return nil, fmt.Errorf("server attached to the proxy component is nil (%v)", proxy)
	}
	// Build routing context
	routingCtx := buildRoutingContext(proxy, session, serverName)
	// TODO: get headers from ctx

	conn, err := session.GetConnectionByName(serverName)
	transport := common.UnknownTransport
	if err != nil {
		fmt.Printf("unable to get connection for '%s'", serverName)
	} else {
		transport = conn.GetConfig().GetTransport()
	}

	event := common.NewMCPRequestEvent(string(transport)).
		WithRequestID(getNewUUIDV7()).
		WithSessionID(session.id).
		WithServerID(proxy.server.ServerID) // nil check was done above

	toolCallCtx := &ToolCallContext{
		proxy:              proxy,
		session:            session,
		originalServerName: serverName,
		originalRequest:    deepCloneRequest(req), // Immutable clone
		request:            req,                   // Mutable, will be modified by handlers
		routingContext:     routingCtx,
		event:              event,
	}

	// Register handlers
	toolCallCtx.SetHandler("payload", &DefaultPayloadHandler{})
	toolCallCtx.SetHandler("meta", &DefaultMetaHandler{})
	toolCallCtx.SetHandler("routing", &DefaultRoutingHandler{})

	// Set default log handler - proxy.server nil check was done above
	if proxy.server.Logger == nil {
		return nil, fmt.Errorf("unable to get logger from centian server")
	}
	toolCallCtx.SetLogHandler(NewDefaultLogHandler(proxy.server.Logger))
	return toolCallCtx, nil
}

// buildRoutingContext creates a RoutingContext from proxy and session info.
func buildRoutingContext(proxy *MCPProxy, session *CentianProxySession, serverName string) *common.RoutingLog {
	// TODO: double check if this is actually required
	// ideally we would combine this somehow with MCPevent data struct
	rc := &common.RoutingLog{
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

// SendRequest executes the downstream call using current request state.
// Note: if processors returned a status >= 400 this will NOT trigger a
// downstream call, instead an immediate error response will be returned.
func (c *ToolCallContext) SendRequest(ctx context.Context) error {
	// Resolve connection based on (potentially modified) serverName
	serverName := c.GetRoutingContext().ServerName
	conn, ok := c.session.downstreamConns[serverName]
	if !ok {
		return fmt.Errorf("server %s not found (original: %s)",
			serverName, c.originalServerName)
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
		return err
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
	// TODO: add aggregated logic here! -> this makes it really convenient to call this then!
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
		c.routingContext = buildRoutingContext(c.proxy, c.session, c.GetServerName())
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
	// here we check if we have an aggregated server, then change the name accordingly.
	toolName := c.request.Params.Name
	if c.proxy.isAggregatedProxy {
		parts := strings.SplitN(toolName, NamespaceSeparator, 2)
		if len(parts) < 2 {
			fmt.Printf("failed to recreate original tool name from: %s", toolName)
			return ""
		}
		toolName = strings.Join(parts[1:], "")
	}
	return toolName
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
	if c.session == nil {
		return ""
	}
	return c.session.id
}

// Routing context

// GetRoutingContext returns the attached RoutingLog.
func (c *ToolCallContext) GetRoutingContext() *common.RoutingLog {
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
// Automatically creates the handlers slice if its nil.
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

// SetLogHandler sets the provided longer handler.
func (c *ToolCallContext) SetLogHandler(l LogHandler) {
	c.logHandler = l
}

// Compile-time interface check.
var _ CallContext = (*ToolCallContext)(nil)
