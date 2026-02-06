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

	// Direction (processors need to know request vs response phase)
	GetDirection() Direction
	SetDirection(Direction)

	// Result access
	GetResult() *mcp.CallToolResult // Returns the CallToolResult, can be nil if request was not sent yet OR resulted in error
	SetResult(*mcp.CallToolResult)  // Sets the result for this call context

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
	GetStatus() int      // Returns current status code (0 = not set, 200 = ok, 4xx/5xx = error)
	SetStatus(int)       // Sets status code
	GetError() string    // Returns error message if status >= 400
	SetError(string)     // Sets error message

	// Session and request identification
	GetRequestID() string // Returns unique request ID
	GetSessionID() string // Returns session ID

	// Routing context (reuses common.RoutingContext)
	GetRoutingContext() *common.RoutingContext

	// Handler access
	GetHandler(part string) CallContextHandler // Returns handler for given part (payload, meta, routing, etc.)
	GetLogHandler() LogHandler                 // Returns the log handler for this context

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

// GetCallContext retrieves CallContext from context.Context.
// Returns nil if not present.
func GetCallContext(ctx context.Context) CallContext {
	cc, _ := ctx.Value(callContextKey{}).(CallContext)
	return cc
}

// MustGetCallContext retrieves CallContext from context.Context.
// Panics if not present.
func MustGetCallContext(ctx context.Context) CallContext {
	cc := GetCallContext(ctx)
	if cc == nil {
		panic("CallContext not found in context")
	}
	return cc
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
