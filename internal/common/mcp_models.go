package common

import (
	"encoding/json"
	"net/http"
	"time"
)

// McpEventDirection represents the event direction, e.g. CLIENT to SERVER, CENTIAN to CLIENT etc.
type McpEventDirection string

const (
	// DirectionClientToServer represents the direction: CLIENT -> SERVER.
	DirectionClientToServer McpEventDirection = "[CLIENT -> SERVER]"

	// DirectionServerToClient represents the direction: SERVER -> CLIENT.
	DirectionServerToClient McpEventDirection = "[SERVER -> CLIENT]"

	// DirectionCentianToClient represents the direction: CENTIAN -> CLIENT,
	// e.g. when a response is returned early before being forwarded to
	// the downstream MCP server.
	DirectionCentianToClient McpEventDirection = "[CENTIAN -> CLIENT]"

	// DirectionSystem represents a system event, not intended
	// to be forwarded to either CLIENT or SERVER.
	DirectionSystem McpEventDirection = "[SYSTEM]"

	// DirectionUnknown represents an unknown direction and is used
	// in case the direction is not one of the above!.
	DirectionUnknown McpEventDirection = "[UNKNOWN]"
)

/*
	MarshalJSON returns the JSON encoding of McpEventDirection.

It maps the value to one of the allowed directions:
  - [CLIENT -> SERVER]
  - [SERVER -> CLIENT]
  - [CENTIAN -> CLIENT]
  - [SYSTEM]
  - [UNKNOWN] - in case none of the above fit
*/
func (m McpEventDirection) MarshalJSON() ([]byte, error) {
	switch m {
	case DirectionClientToServer, DirectionServerToClient, DirectionCentianToClient, DirectionSystem:
		return json.Marshal(string(m))
	default:
		return json.Marshal(string(DirectionUnknown))
	}
}

/*
	UnmarshalJSON parses the JSON-encoded data of McpEventDirection.

It maps the value to one of the allowed directions:
  - [CLIENT -> SERVER]
  - [SERVER -> CLIENT]
  - [CENTIAN -> CLIENT]
  - [SYSTEM]
  - [UNKNOWN] - in case none of the above fit
*/
func (m *McpEventDirection) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch McpEventDirection(s) {
	case DirectionClientToServer, DirectionServerToClient, DirectionCentianToClient, DirectionSystem:
		*m = McpEventDirection(s)
		return nil
	default:
		*m = DirectionUnknown
		return nil
	}
}

// McpMessageType represents the type if an MCP event, can be request, response, system, or unknown.
type McpMessageType string

const (
	// MessageTypeRequest represents a request message type.
	MessageTypeRequest McpMessageType = "request"
	// MessageTypeResponse represents a response message type.
	MessageTypeResponse McpMessageType = "response"
	// MessageTypeSystem represents a system message type.
	MessageTypeSystem McpMessageType = "system"
	// MessageTypeUnknown represents a unknown message type.
	MessageTypeUnknown McpMessageType = "unknown"
)

// MarshalJSON returns the JSON encoding of McpMessageType.
func (m McpMessageType) MarshalJSON() ([]byte, error) {
	switch m {
	case MessageTypeRequest, MessageTypeResponse, MessageTypeSystem:
		return json.Marshal(string(m))
	default:
		return json.Marshal(string(MessageTypeUnknown))
	}
}

// UnmarshalJSON parses the JSON-encoded data of McpMessageType.
func (m *McpMessageType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch McpMessageType(s) {
	case MessageTypeRequest, MessageTypeResponse, MessageTypeSystem:
		*m = McpMessageType(s)
		return nil
	default:
		*m = MessageTypeUnknown
		return nil
	}
}

// BaseMcpEvent holds the core metadata common to all MCP events regardless of transport type.
//
// This struct contains fields for tracking request flow, timing, status, and contextual information
// that apply universally across stdio, HTTP, and other transport mechanisms.
type BaseMcpEvent struct {

	// Indicates the event status - is related to http status codes:
	// 20x - ok
	// 40x - expected error - client might be able to resolve it by rephrasing the query
	// 50x - unexpected error - proxy ran into unexpected error
	Status int `json:"status"`

	// Timestamp is the exact time when the log entry was created.
	Timestamp time.Time `json:"timestamp"`

	// Transport identifies the proxy type: "stdio", "http", "websocket".
	Transport string `json:"transport"`

	// RequestID uniquely identifies a single request/response pair.
	RequestID string `json:"request_id"`

	// SessionID groups multiple requests within the same proxy session.
	SessionID string `json:"session_id,omitempty"`

	// ServerID uniquely identifies the MCP server instance handling this request.
	ServerID string `json:"server_id,omitempty"`

	// Direction indicates the communication flow perspective:.
	// "request" (client→server),
	// "response" (server→client), or
	// "system" (proxy lifecycle events).
	// This field remains stable regardless of success/failure status.
	Direction McpEventDirection `json:"direction"`

	// MessageType categorizes the content/outcome: "request", "response", "error", or "system".
	// Unlike Direction, this changes to "error" for failed responses, enabling filtering.
	// by operational status (e.g., "all errors" vs "all responses regardless of success").
	// This orthogonal design supports both flow analysis (Direction) and status monitoring (MessageType).
	MessageType McpMessageType `json:"message_type"`

	// Success indicates whether the operation completed successfully.
	Success bool `json:"success"`

	// Error contains error details if Success is false.
	Error string `json:"error,omitempty"`

	// ProcessingErrors indicate errors during processing of this event.
	ProcessingErrors map[string]error `json:"-"`

	// Metadata holds additional context-specific key-value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`

	// Modified indicates that the event has been modified at least once since being received.
	Modified bool `json:"modified"`
}

// HTTPEvent captures HTTP request and response data for tracking MCP communication over HTTP transport.
//
// This struct stores both request metadata (method, URL, headers) and response data (status, headers, body)
// to provide complete visibility into HTTP-based MCP interactions. The Body field may be nil or truncated
// for large payloads or streaming responses.
type HTTPEvent struct {
	// ReqID uniquely identifies this HTTP request.
	ReqID string

	// Method is the HTTP method (GET, POST, etc.).
	Method string

	// URL is the full request URL.
	URL string

	// ReqHeaders contains the HTTP request headers.
	ReqHeaders http.Header

	// RespStatus is the HTTP response status code (-1 if no response yet).
	RespStatus int

	// RespHeaders contains the HTTP response headers (nil if no response yet).
	RespHeaders http.Header

	// Body contains the captured request/response body (may be nil, truncated, or empty for streaming).
	Body []byte

	// BodySize is the original content length.
	BodySize int64

	// Truncated indicates if the body was truncated due to size limits.
	Truncated bool

	// Streaming indicates if this is a streaming response.
	Streaming bool

	// ContentType is the Content-Type header value.
	ContentType string
}

// NewHTTPEventFromRequest creates a new HTTPEvent from an HTTP request.
//
// This constructor initializes an HTTPEvent with request metadata and placeholder values
// for response fields. Response status is set to -1 and headers to nil to indicate no
// response has been received yet. The Body field is initially nil and should be set
// during request processing.
func NewHTTPEventFromRequest(r *http.Request, requestID string) *HTTPEvent {
	responseStatus := -1
	var responseHeaders http.Header
	if r.Response != nil {
		responseStatus = r.Response.StatusCode
		responseHeaders = r.Response.Header
	}
	return &HTTPEvent{
		ReqID:       requestID,
		Method:      r.Method,
		URL:         r.URL.String(),
		ReqHeaders:  r.Header,
		RespStatus:  responseStatus,  // -1 means there is no response yet.
		RespHeaders: responseHeaders, // nil means there is no response yet.
		Body:        nil,             // will be set during processing.
		BodySize:    r.ContentLength,
		Truncated:   false,                        // TODO.
		Streaming:   false,                        // TODO.
		ContentType: r.Header.Get("Content-Type"), // TODO: check if this is correct and appropriate.
	}
}

// NewHTTPEventFromResponse creates a new HTTPEvent from an HTTP response.
//
// This constructor initializes an HTTPEvent with both request and response metadata extracted
// from the response object. The request details are obtained from resp.Request, while response
// status and headers are taken directly from the response. The Body field is initially nil
// and should be set during response processing.
func NewHTTPEventFromResponse(resp *http.Response, requestID string) *HTTPEvent {
	return &HTTPEvent{
		ReqID:       requestID,
		Method:      resp.Request.Method,
		URL:         resp.Request.URL.String(),
		ReqHeaders:  resp.Request.Header,
		RespStatus:  resp.StatusCode,
		RespHeaders: resp.Header,
		Body:        nil, // Set during processing.
		BodySize:    resp.ContentLength,
		ContentType: resp.Header.Get("Content-Type"),
	}
}

// HTTPMcpEvent holds data for an HTTP-based MCP event.
type HTTPMcpEvent struct {
	BaseMcpEvent

	HTTPEvent *HTTPEvent

	// Gateway is the logical grouping of MCP servers (e.g., "my-gateway").
	Gateway string `json:"gateway"`

	// ServerName identifies the specific MCP server within the gateway.
	ServerName string `json:"server_name"`

	// Endpoint is the HTTP path this server is mounted at (e.g., "/mcp/my-gateway/github").
	//.
	// Follows the pattern: /mcp/<gateway_name>/<server_name>.
	Endpoint string `json:"endpoint"`

	// DownstreamURL is the target MCP server URL being proxied to.
	DownstreamURL string `json:"downstream_url"`

	// ProxyPort is the port the proxy server is listening on.
	ProxyPort string `json:"proxy_port,omitempty"`
}

// RawMessage returns the message content of the HTTPMcpEvent, is based on HTTPEvent.Body.
//
// If the original HTTPEvent has no Body (e.g. GET, DELETE methods) it returns an empty string.
func (h *HTTPMcpEvent) RawMessage() string {
	rawMessage := ""
	if h.HTTPEvent != nil && len(h.HTTPEvent.Body) > 0 {
		rawMessage = string(h.HTTPEvent.Body)
	}
	return rawMessage
}

// SetRawMessage overwrites the mcp event content - used for writing back processed MCP event data.
//
// For HTTPMcpEvent this overwrites the original HTTPEvent.Body.
func (h *HTTPMcpEvent) SetRawMessage(newMessage string) {
	if h.HTTPEvent == nil {
		// Note: this should never happen - except during testing.
		LogWarn("HTTPEvent is nil, creating new for message: %s", newMessage)
		h.HTTPEvent = &HTTPEvent{} // we create a new HTTPEvent.
	}
	h.HTTPEvent.Body = []byte(newMessage)
}

// SetModified sets the modified flag.
func (h *HTTPMcpEvent) SetModified(b bool) {
	h.Modified = b
}

// HasContent returns true if HTTPEvent.Body has content, false otherwise.
func (h *HTTPMcpEvent) HasContent() bool {
	return h.HTTPEvent != nil && len(h.HTTPEvent.Body) > 0
}

// IsRequest returns true if MessageType is MessageTypeRequest.
func (h *HTTPMcpEvent) IsRequest() bool {
	return h.MessageType == MessageTypeRequest
}

// IsResponse returns true if MessageType is MessageTypeResponse.
func (h *HTTPMcpEvent) IsResponse() bool {
	return h.MessageType == MessageTypeResponse
}

// GetBaseEvent returns the BaseMcpEvent for this HTTPMcpEvent.
func (h *HTTPMcpEvent) GetBaseEvent() BaseMcpEvent {
	return h.BaseMcpEvent
}

// SetStatus sets the status for this MCP event.
func (h *HTTPMcpEvent) SetStatus(status int) {
	h.Status = status
}

// MarshalJSON implements custom JSON marshaling for HTTPMcpEvent.
//
// This method injects the raw message content into the JSON output using an alias type
// to prevent infinite recursion. Uses a value receiver to implement json.Marshaler
// for both value and pointer types, enabling marshaling of HTTPMcpEvent regardless
// of whether it's passed by value or by pointer.
//
//nolint:gocritic // Intentional value receiver to implement json.Marshaler for both value and pointer types.
func (h HTTPMcpEvent) MarshalJSON() ([]byte, error) {
	type Alias HTTPMcpEvent
	return marshalWithRaw(h.RawMessage(), Alias(h))
}

// DeepClone creates a deep copy of the HTTPMcpEvent.
//
// This method performs a complete deep copy of the event including all nested structures
// (ProcessingErrors map, Metadata map, HTTPEvent with headers and body). The returned copy
// is completely independent from the original, allowing safe concurrent modifications
// without affecting the source event.
func (h *HTTPMcpEvent) DeepClone() *HTTPMcpEvent {
	// Shallow copy value fields (dereference pointer to get value).
	processedEvent := *h

	// Deep copy ProcessingErrors map.
	processedEvent.ProcessingErrors = make(map[string]error)
	for k, v := range h.ProcessingErrors {
		processedEvent.ProcessingErrors[k] = v // Copy original errors.
	}
	processedEvent.Metadata = make(map[string]string)
	for k, v := range h.Metadata {
		processedEvent.Metadata[k] = v // Copy original metadata.
	}
	if processedEvent.HTTPEvent != nil {
		processedEvent.HTTPEvent = &HTTPEvent{
			ReqID:       h.HTTPEvent.ReqID,
			Method:      h.HTTPEvent.Method,
			URL:         h.HTTPEvent.URL,
			ReqHeaders:  h.HTTPEvent.ReqHeaders.Clone(), // ✅ Deep copy.
			RespStatus:  h.HTTPEvent.RespStatus,
			RespHeaders: h.HTTPEvent.RespHeaders.Clone(), // ✅ Deep copy.
			Body:        make([]byte, len(h.HTTPEvent.Body)),
			BodySize:    h.HTTPEvent.BodySize,
			Truncated:   h.HTTPEvent.Truncated,
			Streaming:   h.HTTPEvent.Streaming,
			ContentType: h.HTTPEvent.ContentType,
		}
		copy(processedEvent.HTTPEvent.Body, h.HTTPEvent.Body)
	} else {
		processedEvent.HTTPEvent = nil
	}

	return &processedEvent
}

// StdioMcpEvent holds data for a stdio-based MCP event.
//
// This event type represents MCP communication that occurs via standard input/output streams,
// typically with processes spawned via commands like npx or python. It captures both the
// command execution context and the message content exchanged with the MCP server.
type StdioMcpEvent struct {
	BaseMcpEvent

	// Command is the executable being proxied (e.g., "npx", "python").
	Command string `json:"command"`

	// Args are the command-line arguments passed to the command.
	Args []string `json:"args"`

	// ProjectPath is the working directory where the command executes.
	ProjectPath string `json:"project_path"`

	// ConfigSource indicates where configuration originated: "global", "project", or "profile".
	ConfigSource string `json:"config_source"`

	// Message contains the actual MCP message content (request or response payload).
	Message string `json:"message"`
}

// RawMessage returns the message content of the StdioMcpEvent.
//
// For stdio events, this returns the Message field directly which contains
// the MCP request or response payload.
func (s *StdioMcpEvent) RawMessage() string {
	return s.Message
}

// SetRawMessage overwrites the message content - used for writing back processed MCP event data.
//
// For StdioMcpEvent this overwrites the Message field.
func (s *StdioMcpEvent) SetRawMessage(newMessage string) {
	s.Message = newMessage
}

// SetModified sets the modified flag to indicate the event has been altered.
func (s *StdioMcpEvent) SetModified(b bool) {
	s.Modified = b
}

// HasContent returns true if Message field is non-empty, false otherwise.
func (s *StdioMcpEvent) HasContent() bool {
	return s.Message != ""
}

// GetBaseEvent returns the BaseMcpEvent for this StdioMcpEvent.
func (s *StdioMcpEvent) GetBaseEvent() BaseMcpEvent {
	return s.BaseMcpEvent
}

// IsRequest returns true if MessageType is MessageTypeRequest.
func (s *StdioMcpEvent) IsRequest() bool {
	return s.MessageType == MessageTypeRequest
}

// IsResponse returns true if MessageType is MessageTypeResponse.
func (s *StdioMcpEvent) IsResponse() bool {
	return s.MessageType == MessageTypeResponse
}

// SetStatus sets the status for this MCP event.
func (s *StdioMcpEvent) SetStatus(status int) {
	s.Status = status
}

// marshalWithRaw marshals a struct to JSON and adds a "raw_message" field.
//
// This helper function is used by MarshalJSON implementations to inject the raw message
// content into the JSON output without modifying the original struct definition. The parameter v
// must be an alias type to prevent infinite recursion during marshaling.
func marshalWithRaw(raw string, v any) ([]byte, error) {
	// v should be an alias type (so it won't recurse).
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	m["raw_message"] = raw
	return json.Marshal(m)
}

// MarshalJSON implements custom JSON marshaling for StdioMcpEvent.
//
// This method injects the raw message content into the JSON output using an alias type
// to prevent infinite recursion. Uses a value receiver to implement json.Marshaler
// for both value and pointer types, enabling marshaling of StdioMcpEvent regardless
// of whether it's passed by value or by pointer.
//
//nolint:gocritic // Intentional value receiver to implement json.Marshaler for both value and pointer types.
func (s StdioMcpEvent) MarshalJSON() ([]byte, error) {
	type Alias StdioMcpEvent
	return marshalWithRaw(s.RawMessage(), Alias(s))
}

// McpEventInterface provides a transport-agnostic abstraction for all MCP events.
//
// This interface enables polymorphic handling of MCP events across different transport
// mechanisms (stdio, HTTP, etc.) without requiring type assertions. All event types
// (StdioMcpEvent, HTTPMcpEvent) implement this interface, allowing unified processing
// of events regardless of their underlying transport implementation.
//
// The interface provides methods for:
//   - Message content access and modification (RawMessage, SetRawMessage)
//   - Event state tracking (SetModified, HasContent)
//   - Metadata access (GetBaseEvent)
//   - Type identification (IsRequest, IsResponse)
type McpEventInterface interface {
	RawMessage() string
	SetRawMessage(newMessage string)
	SetModified(b bool)
	HasContent() bool
	GetBaseEvent() BaseMcpEvent
	IsResponse() bool
	IsRequest() bool
	SetStatus(newStatus int)
}
