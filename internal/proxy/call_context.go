package proxy

import (
	"context"
	"encoding/json"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Direction indicates whether we're processing a request or response.
// TODO: This already exists for the MCPEvent - we should reuse this
// TODO: think about integrating MCPEvent into the CallContext
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
	GetResult() *mcp.CallToolResult // Results the CallToolResult, can be nil if request was not send yet OR resulted in error
	SetResult(*mcp.CallToolResult)  // Sets the result for this call context

	// Original request (immutable deep clone - for auditing/comparison)
	// Returns name of the original server, can differ from GetServerName if routing context was changed
	GetOriginalServerName() string
	// Returns original CallToolRequest, can differ from GetRequest if request context was changed
	GetOriginalRequest() *mcp.CallToolRequest
	// Returns original tool name, can differ from GetToolName if request context was changed
	GetOriginalToolName() string

	// Current request (mutable - handlers modify this)
	GetServerName() string            // Returns current server name
	SetServerName(string)             // Sets current server name, can be used for re-routing
	GetRequest() *mcp.CallToolRequest // Returns current CallToolRequest, can be modified during lifetime of this object
	GetToolName() string              // Returns current tool name
	// TODO: check if "GetArguments" might be viable

	// Config access (for processors/handlers that need it)
	GetGlobalConfig() *config.GlobalConfig   // Returns current global config, used by handler
	GetGatewayConfig() *config.GatewayConfig // Returns current gateway config for this request
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
