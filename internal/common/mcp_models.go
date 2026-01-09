package common

import (
	"encoding/json"
	"net/http"
	"time"
)

// McpEventDirection
type McpEventDirection string

const (
	DirectionClientToServer  McpEventDirection = "[CLIENT -> SERVER]"
	DirectionServerToClient  McpEventDirection = "[SERVER -> CLIENT]"
	DirectionCentianToClient McpEventDirection = "[CENTIAN -> CLIENT]"
	DirectionSystem          McpEventDirection = "[SYSTEM]"
	DirectionUnknown         McpEventDirection = "[UNKNOWN]" // in case the direction is not one of the above!
)

func (m McpEventDirection) MarshalJSON() ([]byte, error) {
	switch m {
	case DirectionClientToServer, DirectionServerToClient, DirectionCentianToClient, DirectionSystem:
		return json.Marshal(string(m))
	default:
		return json.Marshal(string(DirectionUnknown))
	}
}
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

// McpMessageType
type McpMessageType string

const (
	MessageTypeRequest  McpMessageType = "request"
	MessageTypeResponse McpMessageType = "response"
	MessageTypeSystem   McpMessageType = "system"
	MessageTypeUnknown  McpMessageType = "unknown" // fallback in case of error
)

func (m McpMessageType) MarshalJSON() ([]byte, error) {
	switch m {
	case MessageTypeRequest, MessageTypeResponse, MessageTypeSystem:
		return json.Marshal(string(m))
	default:
		return json.Marshal(string(MessageTypeUnknown))
	}
}
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

func (h HttpMcpEvent) RawMessage() string {
	rawMessage := ""
	if len(h.HttpEvent.Body) > 0 {
		rawMessage = string(h.HttpEvent.Body)
	}
	return rawMessage
}

func (h *HttpMcpEvent) SetRawMessage(newMessage string) {
	h.HttpEvent.Body = []byte(newMessage)
}

func (h *HttpMcpEvent) SetModified(b bool) {
	h.BaseMcpEvent.Modified = b
}

func (h HttpMcpEvent) HasContent() bool {
	return len(h.HttpEvent.Body) > 0
}

// Convert on serialization (in MarshalJSON)
func (e HttpMcpEvent) MarshalJSON() ([]byte, error) {
	type Alias HttpMcpEvent
	return marshalWithRaw(e.RawMessage(), Alias(e))
}

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

func (s StdioMcpEvent) RawMessage() string {
	return s.Message
}

func (s *StdioMcpEvent) SetRawMessage(newMessage string) {
	s.Message = newMessage
}

func (h *StdioMcpEvent) SetModified(b bool) {
	h.BaseMcpEvent.Modified = b
}

func (s StdioMcpEvent) HasContent() bool {
	return len(s.Message) > 0
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
func (s StdioMcpEvent) MarshalJSON() ([]byte, error) {
	type Alias StdioMcpEvent
	return marshalWithRaw(s.RawMessage(), Alias(s))
}

type McpEventInterface interface {
	RawMessage() string
	SetRawMessage(newMessage string)
	SetModified(b bool)
	HasContent() bool
}
