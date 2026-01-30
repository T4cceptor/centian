package common

import (
	"encoding/json"
	"time"
)

// McpTransportType represents a valid MCP transport - either stdio or http.
type McpTransportType string

const (
	// HTTPTransport represents HTTP transport -> "http".
	HTTPTransport McpTransportType = "http"

	// StdioTransport represents stdio transport -> "stdio".
	StdioTransport McpTransportType = "stdio"
)

// McpEventDirection represents the event direction, e.g. CLIENT to SERVER, CENTIAN to CLIENT etc.
type McpEventDirection string

const (
	// DirectionClientToServer represents the direction: CLIENT -> SERVER.
	DirectionClientToServer McpEventDirection = "[CLIENT -> SERVER]"

	// DirectionServerToClient represents the direction: SERVER -> CLIENT.
	DirectionServerToClient McpEventDirection = "[SERVER -> CLIENT]"

	// DirectionCentianToClient represents the direction: CENTIAN -> CLIENT,
	// e.g. when a response is returned early before being forwarded to.
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
	// 50x - unexpected error - proxy ran into unexpected error.
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

	// Direction indicates the communication flow perspective:
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

// McpEventInterface provides a transport-agnostic abstraction for all MCP events.
//
// This interface enables polymorphic handling of MCP events across different transport
// mechanisms (stdio, HTTP, etc.) without requiring type assertions. All event types
// (StdioMcpEvent) implement this interface, allowing unified processing
// of events regardless of their underlying transport implementation.
//
// The interface provides methods for:
//   - Message content access and modification (RawMessage, SetRawMessage)
//   - Event state tracking (SetModified, HasContent)
//   - Metadata access (GetBaseEvent)
//   - Type identification (IsRequest, IsResponse).
type McpEventInterface interface {
	GetRawMessage() string
	SetRawMessage(newMessage string)
	SetModified(b bool)
	HasContent() bool
	GetBaseEvent() BaseMcpEvent
	IsResponse() bool
	IsRequest() bool
	SetStatus(newStatus int)
}
