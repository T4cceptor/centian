package proxy

import (
	"context"
	"encoding/json"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Direction indicates whether we're processing a request or response.
type Direction string

const (
	DirectionRequest  Direction = "request"
	DirectionResponse Direction = "response"
)

// CallContext represents a single request/response cycle.
// Implementations know how to send themselves and manage their own state.
type CallContext interface {
	// Lifecycle
	SendRequest(ctx context.Context) error // Execute the downstream call

	// Result access
	HasResult() bool // Returns true if a result is available
	// Note: this is used to avoid calling downstream in case processors already provided a response
	// Scenarios include:
	// - error responses from processors -> in this case we specifically want to prevent downstream call, as its unsafe
	// - cached responses
	// - mock responses
	// - processors so powerful they can provide their own responses, think Processor as MCP client itself
	GetResult() *mcp.CallToolResult // Returns the CallToolResult, can be nil if request was not sent yet OR resulted in error
	SetResult(*mcp.CallToolResult)  // Sets the result for this call context

	// TODO: refactor this: ideally we would provide another interface that has access to all of this information!
	// e.g. CallContext.GetEventInfo().GetStatus() -> this is an opportunity to reuse MCPEvent!

	// Original request (immutable deep clone - for auditing/comparison)
	GetOriginalServerName() string            // Returns name of the original server
	GetOriginalRequest() *mcp.CallToolRequest // Returns original CallToolRequest
	GetOriginalToolName() string              // Returns original tool name

	// Current request (mutable - handlers modify this)
	GetServerName() string            // Returns current server name
	SetServerName(string)             // Sets current server name, can be used for re-routing
	GetRequest() *mcp.CallToolRequest // Returns current CallToolRequest
	GetToolName() string              // Returns current tool name

	// Status and error handling
	GetStatus() int   // Returns current status code (0 = not set, 200 = ok, 4xx/5xx = error)
	SetStatus(int)    // Sets status code
	GetError() string // Returns error message if status >= 400
	SetError(string)  // Sets error message

	// Session and request identification
	GetRequestID() string // Returns unique request ID
	GetSessionID() string // Returns session ID

	// Direction (processors need to know request vs response phase)
	GetDirection() Direction
	SetDirection(Direction)

	// --- end of TODO - se above

	// Routing context (reuses common.RoutingContext)
	GetRoutingContext() *common.RoutingContext

	// Handler access
	GetHandler(part string) (CallContextHandler, bool) // Returns handler for given part (payload, meta, routing, etc.)
	SetHandler(part string, h CallContextHandler)      // Sets the provided context handler
	GetLogHandler() LogHandler                         // Returns the log handler for this context
	SetLogHandler(l LogHandler)                        // sets the provided log handler

	// Config access (for processors/handlers that need it)
	GetGlobalConfig() *config.GlobalConfig   // Returns current global config
	GetGatewayConfig() *config.GatewayConfig // Returns current gateway config
}

// callContextKey is the key type for storing CallContext in context.Context.
type callContextKey struct{}

// WithCallContext attaches a CallContext to a context.Context.
func WithCallContext(ctx context.Context, cc CallContext) context.Context {
	return context.WithValue(ctx, callContextKey{}, cc)
}

// deepCloneRequest creates an immutable copy of the request for auditing.
func deepCloneRequest(req *mcp.CallToolRequest) *mcp.CallToolRequest {
	if req == nil || req.Params == nil {
		return nil
	}
	// Deep copy arguments (json.RawMessage is a []byte)
	argsCopy := make(json.RawMessage, len(req.Params.Arguments))
	copy(argsCopy, req.Params.Arguments)

	return &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      req.Params.Name,
			Arguments: argsCopy,
		},
	}
}
