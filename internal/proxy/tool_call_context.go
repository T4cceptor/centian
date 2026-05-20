package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
	meta *common.MetaContext

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

	conn, err := proxy.sessionConnection(upstreamSession, serverName)
	transport := common.UnknownTransport
	if err != nil {
		common.LogWarn("unable to get connection for '%s': %v", serverName, err)
	} else {
		transport = conn.GetConfig().GetTransport()
	}

	meta := common.NewRequestMetaContext(string(transport)).
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
		meta:               meta,
		authData:           upstreamSession.authData.Clone(),
	}

	// Register handlers
	toolCallCtx.SetHandler("payload", &DefaultPayloadHandler{})
	toolCallCtx.SetHandler("meta", &DefaultMetaHandler{})
	toolCallCtx.SetHandler("routing", &DefaultRoutingHandler{})
	toolCallCtx.SetHandler("auth", &DefaultAuthHandler{})
	toolCallCtx.SetHandler("annotations", &DefaultAnnotationHandler{})

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
	conn, err := proxy.sessionConnection(upstreamSession, serverName)
	if err == nil && conn != nil {
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
	conn, err := c.proxy.sessionConnection(c.upstreamSession, serverName)
	if err != nil {
		return err
	}
	if !conn.IsConnected() {
		return fmt.Errorf("server %s found (original: %s), but not connected",
			serverName, c.originalServerName)
	}

	if c.request == nil || c.request.Params == nil {
		return fmt.Errorf("tool call request params are required")
	}

	// Forward the current request object so downstream receives the full tool
	// call shape, including request metadata.
	result, err := conn.CallTool(ctx, c.GetRequest())
	if err != nil {
		return normalizeForwardedMethodError(mcpMethodCallTool, err)
	}
	c.SetResult(result)
	return nil
}

// GetMetaContext returns the attached processor metadata.
func (c *ToolCallContext) GetMetaContext() *common.MetaContext {
	return c.meta
}

// SetMetaContext sets the provided processor metadata.
func (c *ToolCallContext) SetMetaContext(meta *common.MetaContext) {
	c.meta = meta
}

// ToLogEntry returns the current call state as a structured log entry.
func (c *ToolCallContext) ToLogEntry() *common.LogEntry {
	if c.meta == nil {
		c.meta = common.NewMetaContext(string(common.UnknownTransport), common.DirectionUnknown, common.MessageTypeUnknown)
	}

	entry := &common.LogEntry{
		BaseMcpEvent: c.meta.BaseMcpEvent,
		Annotations:  c.meta.Annotations,
	}
	entry.Timestamp = time.Now()
	entry.Success = c.GetStatus() < 400

	if rc := c.GetRoutingContext(); rc != nil {
		entry.Routing = *rc
		if rc.Transport != "" {
			entry.Transport = string(rc.Transport)
		}
	}
	if entry.Transport == "" {
		entry.Transport = string(common.UnknownTransport)
	}
	if entry.Metadata == nil {
		entry.Metadata = make(map[string]string)
	}
	if c.upstreamSession != nil && c.upstreamSession.identityKey != "" {
		entry.Metadata["principal_id"] = c.upstreamSession.identityKey
	}

	req := c.GetRequest()
	if req != nil && req.Params != nil {
		entry.WithToolRequest(c.GetToolName(), c.GetOriginalToolName(), req.Params.Arguments)
	}

	if c.HasResult() {
		result := c.GetResult()
		resultJSON, _ := json.Marshal(result)
		entry.WithToolResult(resultJSON, result.IsError)
	}

	return entry
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

// GetDirection returns MetaContext.Direction.
func (c *ToolCallContext) GetDirection() common.McpEventDirection {
	return c.meta.Direction
}

// SetDirection sets MetaContext.Direction.
func (c *ToolCallContext) SetDirection(d common.McpEventDirection) {
	c.meta.Direction = d
}

// GetMessageType returns MetaContext.MessageType.
func (c *ToolCallContext) GetMessageType() common.McpMessageType {
	return c.meta.MessageType
}

// SetMessageType sets MetaContext.MessageType.
func (c *ToolCallContext) SetMessageType(t common.McpMessageType) {
	c.meta.MessageType = t
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

// GetStatus returns ToolCallContext.MetaContext.Status.
func (c *ToolCallContext) GetStatus() int {
	return c.meta.Status
}

// SetStatus sets ToolCallContext.MetaContext.Status.
func (c *ToolCallContext) SetStatus(status int) {
	c.meta.Status = status
}

// GetError returns ToolCallContext.MetaContext.Error.
func (c *ToolCallContext) GetError() string {
	return c.meta.Error
}

// SetError sets the ToolCallContext.MetaContext.Error.
func (c *ToolCallContext) SetError(msg string) {
	c.meta.Error = msg
}

// Session and request identification

// GetRequestID returns the current request ID.
func (c *ToolCallContext) GetRequestID() string {
	return c.meta.RequestID
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
