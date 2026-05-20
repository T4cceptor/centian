package common

import (
	"encoding/json"
	"time"
)

// MetaContext contains processor-facing event metadata.
type MetaContext struct {
	BaseMcpEvent
	Annotations []EventAnnotation `json:"-"`
}

// LogEntry is the enriched log payload written to the Centian JSONL log.
type LogEntry struct {
	BaseMcpEvent
	Routing     RoutingContext    `json:"routing"`
	ToolCall    *ToolCallLog      `json:"tool_call,omitempty"`
	Annotations []EventAnnotation `json:"annotations,omitempty"`
}

// EventAnnotation describes a processor observation or policy action for an event.
type EventAnnotation struct {
	Processor string                   `json:"processor,omitempty"`
	Action    string                   `json:"action,omitempty"`
	Severity  string                   `json:"severity,omitempty"`
	Message   string                   `json:"message,omitempty"`
	Findings  []EventAnnotationFinding `json:"findings,omitempty"`
	Details   map[string]any           `json:"details,omitempty"`
}

// EventAnnotationFinding identifies a specific annotation finding.
type EventAnnotationFinding struct {
	Rule string `json:"rule,omitempty"`
	Path string `json:"path,omitempty"`
}

// RoutingContext captures where the request is going.
type RoutingContext struct {
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
//
// Note: this is only filled for logging.
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

// NewMetaContext creates a new MetaContext with required fields initialized.
func NewMetaContext(
	transport string,
	direction McpEventDirection,
	messageType McpMessageType,
) *MetaContext {
	return &MetaContext{
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
	}
}

// NewRequestMetaContext creates a MetaContext for a request (client → server).
func NewRequestMetaContext(transport string) *MetaContext {
	return NewMetaContext(transport, DirectionClientToServer, MessageTypeRequest)
}

// ============================================================================
// Builder methods for fluent construction
// ============================================================================

// WithRequestID sets the request ID.
func (e *MetaContext) WithRequestID(id string) *MetaContext {
	e.RequestID = id
	return e
}

// WithSessionID sets the session ID.
func (e *MetaContext) WithSessionID(id string) *MetaContext {
	e.SessionID = id
	return e
}

// WithServerID sets the server ID.
func (e *MetaContext) WithServerID(id string) *MetaContext {
	e.ServerID = id
	return e
}

// WithToolRequest attaches name, originalName and args on ToolCall of this LogEntry.
func (e *LogEntry) WithToolRequest(name, originalName string, args json.RawMessage) *LogEntry {
	if e.ToolCall == nil {
		e.ToolCall = &ToolCallLog{}
	}
	e.ToolCall.Name = name
	e.ToolCall.OriginalName = originalName
	e.ToolCall.Arguments = args
	return e
}

// WithToolResult attaches provided result and isError status on ToolCall of this LogEntry.
func (e *LogEntry) WithToolResult(result json.RawMessage, isError bool) *LogEntry {
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
func (e *MetaContext) GetBaseEvent() BaseMcpEvent {
	return e.BaseMcpEvent
}

// GetBaseEvent returns the BaseMcpEvent.
func (e *LogEntry) GetBaseEvent() BaseMcpEvent {
	return e.BaseMcpEvent
}

// SetStatus sets the status code for this event.
func (e *MetaContext) SetStatus(status int) {
	e.Status = status
}

// SetStatus sets the status code for this log entry.
func (e *LogEntry) SetStatus(status int) {
	e.Status = status
}
