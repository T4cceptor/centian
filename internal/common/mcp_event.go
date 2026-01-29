package common

import (
	"encoding/json"
	"time"
)

// MCPEvent is a unified event type for all MCP transports.
// It provides a transport-agnostic structure that can represent events from
// HTTP, stdio, SDK-based proxies, or any future transport mechanism.
type MCPEvent struct {
	BaseMcpEvent

	// Routing context (always present)
	Routing RoutingContext `json:"routing"`

	// Transport context (optional - populated when HTTP details are available)
	HTTP *HTTPContext `json:"http,omitempty"`

	// Tool call context (optional - only for tool call events)
	ToolCall *ToolCallContext `json:"tool_call,omitempty"`

	// Raw message content (the JSON-RPC payload)
	rawMessage string
}

// RoutingContext captures where the request is going.
type RoutingContext struct {
	// Gateway is the logical grouping of MCP servers
	Gateway string `json:"gateway,omitempty"`

	// ServerName identifies the specific MCP server
	ServerName string `json:"server_name,omitempty"`

	// Endpoint is the HTTP path or identifier for this proxy
	Endpoint string `json:"endpoint,omitempty"`

	// DownstreamURL is the target MCP server URL being proxied to
	DownstreamURL string `json:"downstream_url,omitempty"`
}

// HTTPContext captures HTTP-specific transport details.
type HTTPContext struct {
	// Method is the HTTP method (GET, POST, etc.)
	Method string `json:"method,omitempty"`

	// URL is the request URL
	URL string `json:"url,omitempty"`

	// Headers contains HTTP headers (sanitized - auth headers redacted)
	Headers map[string]string `json:"headers,omitempty"`

	// StatusCode is the HTTP response status code
	StatusCode int `json:"status_code,omitempty"`

	// ContentType is the Content-Type header value
	ContentType string `json:"content_type,omitempty"`

	// ClientIP is the client's IP address
	ClientIP string `json:"client_ip,omitempty"`
}

// ToolCallContext captures tool call specific details.
type ToolCallContext struct {
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
		Routing: RoutingContext{},
	}
}

// NewMCPRequestEvent creates an MCPEvent for a request (client → server).
func NewMCPRequestEvent(transport string) *MCPEvent {
	return NewMCPEvent(transport, DirectionClientToServer, MessageTypeRequest)
}

// NewMCPResponseEvent creates an MCPEvent for a response (server → client).
func NewMCPResponseEvent(transport string) *MCPEvent {
	return NewMCPEvent(transport, DirectionServerToClient, MessageTypeResponse)
}

// NewMCPSystemEvent creates an MCPEvent for a system event.
func NewMCPSystemEvent(transport string) *MCPEvent {
	return NewMCPEvent(transport, DirectionSystem, MessageTypeSystem)
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

// WithRouting sets the routing context.
func (e *MCPEvent) WithRouting(gateway, serverName, endpoint string) *MCPEvent {
	e.Routing = RoutingContext{
		Gateway:    gateway,
		ServerName: serverName,
		Endpoint:   endpoint,
	}
	return e
}

// WithToolCall sets the tool call context.
func (e *MCPEvent) WithToolCall(name string, arguments json.RawMessage) *MCPEvent {
	e.ToolCall = &ToolCallContext{
		Name:      name,
		Arguments: arguments,
	}
	return e
}

// WithToolResult sets the tool call result (for response events).
func (e *MCPEvent) WithToolResult(result json.RawMessage, isError bool) *MCPEvent {
	if e.ToolCall == nil {
		e.ToolCall = &ToolCallContext{}
	}
	e.ToolCall.Result = result
	e.ToolCall.IsError = isError
	return e
}

// WithHTTPContext sets the HTTP context.
func (e *MCPEvent) WithHTTPContext(ctx *HTTPContext) *MCPEvent {
	e.HTTP = ctx
	return e
}

// WithRawMessage sets the raw message content.
func (e *MCPEvent) WithRawMessage(msg string) *MCPEvent {
	e.rawMessage = msg
	return e
}

// ============================================================================
// McpEventInterface implementation
// ============================================================================

// RawMessage returns the JSON-RPC message content.
func (e *MCPEvent) RawMessage() string {
	return e.rawMessage
}

// SetRawMessage overwrites the message content.
func (e *MCPEvent) SetRawMessage(newMessage string) {
	e.rawMessage = newMessage
}

// SetModified sets the modified flag.
func (e *MCPEvent) SetModified(b bool) {
	e.Modified = b
}

// HasContent returns true if the event has message content.
func (e *MCPEvent) HasContent() bool {
	return e.rawMessage != ""
}

// GetBaseEvent returns the BaseMcpEvent.
func (e *MCPEvent) GetBaseEvent() BaseMcpEvent {
	return e.BaseMcpEvent
}

// IsRequest returns true if this is a request event.
func (e *MCPEvent) IsRequest() bool {
	return e.MessageType == MessageTypeRequest
}

// IsResponse returns true if this is a response event.
func (e *MCPEvent) IsResponse() bool {
	return e.MessageType == MessageTypeResponse
}

// SetStatus sets the status code for this event.
func (e *MCPEvent) SetStatus(status int) {
	e.Status = status
}

// ============================================================================
// JSON Marshaling
// ============================================================================

// MarshalJSON implements custom JSON marshaling for MCPEvent.
// It includes the raw_message field in the JSON output.
//
//nolint:gocritic // Intentional value receiver for json.Marshaler compatibility
func (e MCPEvent) MarshalJSON() ([]byte, error) {
	type Alias MCPEvent
	data, err := json.Marshal(Alias(e))
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	m["raw_message"] = e.rawMessage
	return json.Marshal(m)
}