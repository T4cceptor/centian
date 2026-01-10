package common

import (
	"encoding/json"
	"net/http"
	"time"
)

// McpEventDirection represents the event direction, e.g. CLIENT to SERVER, CENTIAN to CLIENT etc
type McpEventDirection string

const (
	// DirectionClientToServer represents the direction: CLIENT -> SERVER
	DirectionClientToServer McpEventDirection = "[CLIENT -> SERVER]"

	// DirectionServerToClient represents the direction: SERVER -> CLIENT
	DirectionServerToClient McpEventDirection = "[SERVER -> CLIENT]"

	// DirectionCentianToClient represents the direction: CENTIAN -> CLIENT,
	// e.g. when a response is returned early before being forwarded to
	// the downstream MCP server
	DirectionCentianToClient McpEventDirection = "[CENTIAN -> CLIENT]"

	// DirectionSystem represents a system event, not intended
	// to be forwarded to either CLIENT or SERVER
	DirectionSystem McpEventDirection = "[SYSTEM]"

	// DirectionUnknown represents an unknown direction and is used
	// in case the direction is not one of the above!
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
		*m = McpEventDirection(DirectionUnknown)
		return nil
	}
}

// McpMessageType represents the type if an MCP event, can be request, response, system, or unknown
type McpMessageType string

const (
	// MessageTypeRequest represents a request message type
	MessageTypeRequest McpMessageType = "request"
	// MessageTypeResponse represents a response message type
	MessageTypeResponse McpMessageType = "response"
	// MessageTypeSystem represents a system message type
	MessageTypeSystem McpMessageType = "system"
	// MessageTypeUnknown represents a unknown message type
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

// BaseMcpEvent holds the basic data for any MCP event independent of transport
type BaseMcpEvent struct {
	// Timestamp is the exact time when the log entry was created
	Timestamp time.Time `json:"timestamp"`

	// Transport identifies the proxy type: "stdio", "http", "websocket"
	Transport string `json:"transport"`

	// RequestID uniquely identifies a single request/response pair
	RequestID string `json:"request_id"`

	// SessionID groups multiple requests within the same proxy session
	SessionID string `json:"session_id,omitempty"`

	// ServerID uniquely identifies the MCP server instance handling this request
	ServerID string `json:"server_id,omitempty"`

	// Direction indicates the communication flow perspective:
	// "request" (client→server),
	// "response" (server→client), or
	// "system" (proxy lifecycle events).
	// This field remains stable regardless of success/failure status.
	Direction McpEventDirection `json:"direction"`

	// MessageType categorizes the content/outcome: "request", "response", "error", or "system".
	// Unlike Direction, this changes to "error" for failed responses, enabling filtering
	// by operational status (e.g., "all errors" vs "all responses regardless of success").
	// This orthogonal design supports both flow analysis (Direction) and status monitoring (MessageType).
	MessageType McpMessageType `json:"message_type"`

	// Success indicates whether the operation completed successfully
	Success bool `json:"success"`

	// Error contains error details if Success is false
	Error string `json:"error,omitempty"`

	// ProcessingErrors indicate errors during processing of this event
	ProcessingErrors map[string]error `json:"-"`

	// Metadata holds additional context-specific key-value pairs
	Metadata map[string]string `json:"metadata,omitempty"`

	// Modified indicates that the event has been modified at least once since being received
	Modified bool `json:"modified"`
}

// HttpEvent holds http request/response data - used by HttpMcpEvent
type HttpEvent struct {
	ReqID       string
	Method      string
	URL         string
	ReqHeaders  http.Header
	RespStatus  int
	RespHeaders http.Header
	Body        []byte // captured (non-streaming or limited)
	BodySize    int64
	Truncated   bool
	Streaming   bool
	ContentType string
}

// NewHttpEventFromRequest creates a new HttpEvent given a http.Request
func NewHttpEventFromRequest(r *http.Request, requestID string) *HttpEvent {
	responseStatus := -1
	var responseHeaders http.Header = nil
	if r.Response != nil {
		responseStatus = r.Response.StatusCode
		responseHeaders = r.Response.Header
	}
	return &HttpEvent{
		ReqID:       requestID,
		Method:      r.Method,
		URL:         r.URL.String(),
		ReqHeaders:  r.Header,
		RespStatus:  responseStatus,  // -1 means there is no response yet
		RespHeaders: responseHeaders, // nil means there is no response yet
		Body:        nil,             // will be set during processing
		BodySize:    r.ContentLength,
		Truncated:   false,                        // TODO
		Streaming:   false,                        // TODO
		ContentType: r.Header.Get("Content-Type"), // TODO: check if this is correct and appropriate
	}
}

// NewHttpEventFromResponse creates a new HttpEvent given a http.Response
func NewHttpEventFromResponse(resp *http.Response, requestID string) *HttpEvent {
	return &HttpEvent{
		ReqID:       requestID,
		Method:      resp.Request.Method,
		URL:         resp.Request.URL.String(),
		ReqHeaders:  resp.Request.Header,
		RespStatus:  resp.StatusCode,
		RespHeaders: resp.Header,
		Body:        nil, // Set during processing
		BodySize:    resp.ContentLength,
		ContentType: resp.Header.Get("Content-Type"),
	}
}

// HttpMcpEvent holds data for an HTTP-based MCP event
type HttpMcpEvent struct {
	BaseMcpEvent

	HttpEvent *HttpEvent

	// Gateway is the logical grouping of MCP servers (e.g., "my-gateway")
	Gateway string `json:"gateway"`

	// ServerName identifies the specific MCP server within the gateway
	ServerName string `json:"server_name"`

	// Endpoint is the HTTP path this server is mounted at (e.g., "/mcp/my-gateway/github")
	//
	// Follows the pattern: /mcp/<gateway_name>/<server_name>
	Endpoint string `json:"endpoint"`

	// DownstreamURL is the target MCP server URL being proxied to
	DownstreamURL string `json:"downstream_url"`

	// ProxyPort is the port the proxy server is listening on
	ProxyPort string `json:"proxy_port,omitempty"`
}

// RawMessage returns the message content of the HttpMcpEvent, is based on HttpEvent.Body
//
// If the original HttpEvent has no Body (e.g. GET, DELETE methods) it returns an empty string
func (h HttpMcpEvent) RawMessage() string {
	rawMessage := ""
	if len(h.HttpEvent.Body) > 0 {
		rawMessage = string(h.HttpEvent.Body)
	}
	return rawMessage
}

// SetRawMessage overwrites the mcp event content - used for writting back processed MCP event data
//
// For HttpMcpEvent this overwrites the original HttpEvent.Body
func (h *HttpMcpEvent) SetRawMessage(newMessage string) {
	h.HttpEvent.Body = []byte(newMessage)
}

// SetModified sets the modified flag
func (h *HttpMcpEvent) SetModified(b bool) {
	h.BaseMcpEvent.Modified = b
}

// HasContent returns true if HttpEvent.Body has content, false otherwise
func (h HttpMcpEvent) HasContent() bool {
	return len(h.HttpEvent.Body) > 0
}

// IsRequest returns true if MessageType is MessageTypeRequest
func (h HttpMcpEvent) IsRequest() bool {
	return h.BaseMcpEvent.MessageType == MessageTypeRequest
}

// IsResponse returns true if MessageType is MessageTypeResponse
func (h HttpMcpEvent) IsResponse() bool {
	return h.BaseMcpEvent.MessageType == MessageTypeResponse
}

// GetBaseEvent returns the BaseMcpEvent for this HttpMcpEvent
func (h HttpMcpEvent) GetBaseEvent() BaseMcpEvent {
	return h.BaseMcpEvent
}

// Convert on serialization (in MarshalJSON)
//
//nolint:gocritic // Intentional value receiver to implement json.Marshaler for both value and pointer types
func (e HttpMcpEvent) MarshalJSON() ([]byte, error) {
	type Alias HttpMcpEvent
	return marshalWithRaw(e.RawMessage(), Alias(e))
}

// DeepClone creates a deep copy of the HttpMcpEvent and returns it
func (e *HttpMcpEvent) DeepClone() *HttpMcpEvent {
	// Shallow copy value fields (dereference pointer to get value)
	processedEvent := *e

	// Deep copy ProcessingErrors map
	processedEvent.ProcessingErrors = make(map[string]error)
	for k, v := range e.ProcessingErrors {
		processedEvent.ProcessingErrors[k] = v // Copy original errors
	}
	processedEvent.Metadata = make(map[string]string)
	for k, v := range e.Metadata {
		processedEvent.Metadata[k] = v // Copy original metadata
	}
	if processedEvent.HttpEvent != nil {
		processedEvent.HttpEvent = &HttpEvent{
			ReqID:       e.HttpEvent.ReqID,
			Method:      e.HttpEvent.Method,
			URL:         e.HttpEvent.URL,
			ReqHeaders:  e.HttpEvent.ReqHeaders.Clone(), // ✅ Deep copy
			RespStatus:  e.HttpEvent.RespStatus,
			RespHeaders: e.HttpEvent.RespHeaders.Clone(), // ✅ Deep copy
			Body:        make([]byte, len(e.HttpEvent.Body)),
			BodySize:    e.HttpEvent.BodySize,
			Truncated:   e.HttpEvent.Truncated,
			Streaming:   e.HttpEvent.Streaming,
			ContentType: e.HttpEvent.ContentType,
		}
		copy(processedEvent.HttpEvent.Body, e.HttpEvent.Body)
	} else {
		processedEvent.HttpEvent = nil
	}

	return &processedEvent
}

// HttpMcpEvent holds data for an stdio-based MCP event
type StdioMcpEvent struct {
	BaseMcpEvent

	// Command is the executable being proxied (e.g., "npx", "python")
	Command string `json:"command"`

	// Args are the command-line arguments passed to the command
	Args []string `json:"args"`

	// ProjectPath is the working directory where the command executes
	ProjectPath string `json:"project_path"`

	// ConfigSource indicates where configuration originated: "global", "project", or "profile"
	ConfigSource string `json:"config_source"`

	Message string `json:"message"`
}

// RawMessage returns the message content of the StdioMcpEvent
func (s StdioMcpEvent) RawMessage() string {
	return s.Message
}

// SetRawMessage sets the message content of the StdioMcpEvent
func (s *StdioMcpEvent) SetRawMessage(newMessage string) {
	s.Message = newMessage
}

func (h *StdioMcpEvent) SetModified(b bool) {
	h.BaseMcpEvent.Modified = b
}

func (s StdioMcpEvent) HasContent() bool {
	return s.Message != ""
}

func (s StdioMcpEvent) GetBaseEvent() BaseMcpEvent {
	return s.BaseMcpEvent
}

func (h StdioMcpEvent) IsRequest() bool {
	return h.BaseMcpEvent.MessageType == MessageTypeRequest
}

func (h StdioMcpEvent) IsResponse() bool {
	return h.BaseMcpEvent.MessageType == MessageTypeResponse
}

func marshalWithRaw(raw string, v any) ([]byte, error) {
	// v should be an alias type (so it won't recurse)
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

// Convert on serialization (in MarshalJSON)
//
//nolint:gocritic // Intentional value receiver to implement json.Marshaler for both value and pointer types
func (s StdioMcpEvent) MarshalJSON() ([]byte, error) {
	type Alias StdioMcpEvent
	return marshalWithRaw(s.RawMessage(), Alias(s))
}

// McpEventInterface provides an abstraction of all MCP Events, independent of transport type
type McpEventInterface interface {
	RawMessage() string
	SetRawMessage(newMessage string)
	SetModified(b bool)
	HasContent() bool
	GetBaseEvent() BaseMcpEvent
	IsResponse() bool
	IsRequest() bool
}
