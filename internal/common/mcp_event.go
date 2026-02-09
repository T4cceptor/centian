package common

import (
	"encoding/json"
	"time"
)

// TODO: rework all of this code based on CallContext

// MCPEvent is a unified event type for all MCP transports.
// It provides a transport-agnostic structure that can represent events from
// HTTP, stdio, SDK-based proxies, or any future transport mechanism.
type MCPEvent struct {
	BaseMcpEvent

	// Routing context (always present)
	Routing RoutingLog `json:"routing"`

	// Tool call context (optional - only for tool call events)
	ToolCall *ToolCallLog `json:"tool_call,omitempty"`
}

// RoutingLog captures where the request is going.
type RoutingLog struct {
	// Transport describes the used transport for this connection (http or stdio)
	Transport McpTransportType `json:"transport,omitempty"`

	// Gateway is the logical grouping of MCP servers
	Gateway string `json:"gateway,omitempty"`

	// ServerName identifies the specific MCP server
	ServerName string `json:"server_name,omitempty"`

	// Endpoint is the HTTP path or identifier for this proxy
	Endpoint string `json:"endpoint,omitempty"`

	// DownstreamURL is the target MCP server URL being proxied to
	DownstreamURL string `json:"downstream_url,omitempty"`

	// DownstreamCommand is the target MCP server command being proxied to
	DownstreamCommand string `json:"downstream_cmd,omitempty"`

	// Args is the target MCP server command args being used
	Args []string `json:"args,omitempty"`

	// TODO: add headers as well - Challenge: requires redaction of sensitive headers like auth tokens when logging
}

// ToolCallLog captures tool call specific details.
type ToolCallLog struct {
	// Name is the tool name being called
	Name string `json:"name"`

	// OriginalName is the tool name before any namespace transformations
	OriginalName string `json:"original_name,omitempty"`

	// Arguments contains the tool call arguments as raw JSON
	Arguments json.RawMessage `json:"arguments,omitempty"`

	// Result contains the tool call result as raw JSON (for responses)
	Result json.RawMessage `json:"result,omitempty"`

	// IsError indicates if the tool call resulted in an error
	IsError bool `json:"is_error,omitempty"`
}

// ============================================================================
// Constructors
// ============================================================================

// NewMCPEvent creates a new MCPEvent with required fields initialized.
func NewMCPEvent(
	transport string,
	direction McpEventDirection,
	messageType McpMessageType,
) *MCPEvent {
	return &MCPEvent{
		BaseMcpEvent: BaseMcpEvent{
			Timestamp:        time.Now(),
			Transport:        transport,
			RequestID:        "", // Should be set by caller
			Direction:        direction,
			MessageType:      messageType,
			Success:          true,
			ProcessingErrors: make(map[string]error),
			Metadata:         make(map[string]string),
		},
		Routing: RoutingLog{},
	}
}

// NewMCPRequestEvent creates an MCPEvent for a request (client → server).
func NewMCPRequestEvent(transport string) *MCPEvent {
	return NewMCPEvent(transport, DirectionClientToServer, MessageTypeRequest)
}

// ============================================================================
// Builder methods for fluent construction
// ============================================================================

// WithRequestID sets the request ID.
func (e *MCPEvent) WithRequestID(id string) *MCPEvent {
	e.RequestID = id
	return e
}

// WithSessionID sets the session ID.
func (e *MCPEvent) WithSessionID(id string) *MCPEvent {
	e.SessionID = id
	return e
}

// WithServerID sets the server ID.
func (e *MCPEvent) WithServerID(id string) *MCPEvent {
	e.ServerID = id
	return e
}

func (e *MCPEvent) WithToolRequest(name, originalName string, args json.RawMessage) *MCPEvent {
	if e.ToolCall == nil {
		e.ToolCall = &ToolCallLog{}
	}
	e.ToolCall.Name = name
	e.ToolCall.OriginalName = originalName
	e.ToolCall.Arguments = args
	return e
}

func (e *MCPEvent) WithToolResult(result json.RawMessage, isError bool) *MCPEvent {
	if e.ToolCall == nil {
		e.ToolCall = &ToolCallLog{}
	}
	e.ToolCall.IsError = isError
	e.ToolCall.Result = result
	return e
}

// ============================================================================
// McpEventInterface implementation
// ============================================================================

// GetBaseEvent returns the BaseMcpEvent.
func (e *MCPEvent) GetBaseEvent() BaseMcpEvent {
	return e.BaseMcpEvent
}

// SetStatus sets the status code for this event.
func (e *MCPEvent) SetStatus(status int) {
	e.Status = status
}
