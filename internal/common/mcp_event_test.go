package common

import (
	"encoding/json"
	"testing"
	"time"

	"gotest.tools/assert"
)

func hasDefaultValues(event *MCPEvent) bool {
	is_now := event.Timestamp.Truncate(time.Duration(1000 * 1000)).Equal(time.Now().Truncate(time.Duration(1000 * 1000)))
	is_success := event.Success
	has_processing_error_map := event.ProcessingErrors != nil
	has_metadata_map := event.Metadata != nil
	return is_now && is_success && has_processing_error_map && has_metadata_map
}

func TestNewMCPEvent(t *testing.T) {
	// Given: NewMCPEvent method
	// When: creating a new event using NewMCPEvent
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)

	// Then: the created event is as expected
	assert.Assert(t, event.Transport == "test")
	assert.Assert(t, event.Direction == DirectionSystem)
	assert.Assert(t, event.MessageType == MessageTypeSystem)
	assert.Assert(t, hasDefaultValues(event))
}

func TestNewMCPRequestEvent(t *testing.T) {
	// Given: NewMCPEvent method
	// When: creating a new event using NewMCPEvent
	event := NewMCPRequestEvent("test")

	// Then: the created event is as expected
	assert.Assert(t, event.Transport == "test")
	assert.Assert(t, event.Direction == DirectionClientToServer) // CLIENT -> SERVER
	assert.Assert(t, event.MessageType == MessageTypeRequest)    // request
	assert.Assert(t, hasDefaultValues(event))
}

func TestNewMCPResponseEvent(t *testing.T) {
	// Given: NewMCPEvent method
	// When: creating a new event using NewMCPEvent
	event := NewMCPResponseEvent("test")

	// Then: the created event is as expected
	assert.Assert(t, event.Transport == "test")
	assert.Assert(t, event.Direction == DirectionServerToClient) // SERVER -> CLIENT
	assert.Assert(t, event.MessageType == MessageTypeResponse)   // response
	assert.Assert(t, hasDefaultValues(event))
}

func TestNewMCPSystemEvent(t *testing.T) {
	// Given: NewMCPEvent method
	// When: creating a new event using NewMCPEvent
	event := NewMCPSystemEvent("test")

	// Then: the created event is as expected
	assert.Assert(t, event.Transport == "test")
	assert.Assert(t, event.Direction == DirectionSystem)     // SYSTEM
	assert.Assert(t, event.MessageType == MessageTypeSystem) // system
	assert.Assert(t, hasDefaultValues(event))
}

func TestWithRequestID(t *testing.T) {
	// Given: a MCPEvent and a requestID
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)
	event.RequestID = "old_id"
	requestID := "test_req_id"

	// When: calling WithRequestID
	new_event := event.WithRequestID(requestID)

	// Then:
	assert.Equal(t, new_event, event)                  // the returned event is the same
	assert.Assert(t, event.RequestID == "test_req_id") // the request ID was set
}

func TestWithSessionID(t *testing.T) {
	// Given: a MCPEvent and a sessionID
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)
	event.SessionID = "old_session_id"
	sessionID := "test_session_id"

	// When: calling WithSessionID
	new_event := event.WithSessionID(sessionID)

	// Then:
	assert.Equal(t, new_event, event)                      // the returned event is the same
	assert.Assert(t, event.SessionID == "test_session_id") // the request ID was set
}

func TestWithServerID(t *testing.T) {
	// Given: a MCPEvent and a serverID
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)
	event.ServerID = "old_server_id"
	serverID := "test_server_id"

	// When: calling WithServerID
	new_event := event.WithServerID(serverID)

	// Then:
	assert.Equal(t, new_event, event)                    // the returned event is the same
	assert.Assert(t, event.ServerID == "test_server_id") // the server ID was set
}

func TestWithToolCall(t *testing.T) {
	// Given: a MCPEvent and a tool call payload
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)
	arguments := json.RawMessage(`{"input":"value"}`)

	// When: calling WithToolCall
	new_event := event.WithToolCall("my_tool", arguments)

	// Then:
	assert.Equal(t, new_event, event)
	assert.Assert(t, event.ToolCall != nil)
	assert.Equal(t, event.ToolCall.Name, "my_tool")
	assert.DeepEqual(t, event.ToolCall.Arguments, arguments)
}

func TestWithToolResult(t *testing.T) {
	// Given: a MCPEvent and a tool result
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)
	result := json.RawMessage(`{"ok":true}`)

	// When: calling WithToolResult
	new_event := event.WithToolResult(result, true)

	// Then:
	assert.Equal(t, new_event, event)
	assert.Assert(t, event.ToolCall != nil)
	assert.DeepEqual(t, event.ToolCall.Result, result)
	assert.Assert(t, event.ToolCall.IsError)
}

func TestWithHTTPContext(t *testing.T) {
	// Given: a MCPEvent and an HTTP context
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)
	context := &HTTPContext{
		Method:     "POST",
		URL:        "https://example.com/mcp",
		StatusCode: 200,
	}

	// When: calling WithHTTPContext
	new_event := event.WithHTTPContext(context)

	// Then:
	assert.Equal(t, new_event, event)
	assert.Equal(t, event.HTTP, context)
}

func TestWithRawMessage(t *testing.T) {
	// Given: a MCPEvent and a raw message
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)
	message := `{"jsonrpc":"2.0"}`

	// When: calling WithRawMessage
	new_event := event.WithRawMessage(message)

	// Then:
	assert.Equal(t, new_event, event)
	assert.Equal(t, event.RawMessage, message)
}

func TestSetModified(t *testing.T) {
	// Given: a MCPEvent
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)

	// When: marking modified
	event.SetModified(true)

	// Then: the modified flag is set
	assert.Assert(t, event.Modified)

	// When: clearing modified
	event.SetModified(false)

	// Then: the modified flag is cleared
	assert.Assert(t, !event.Modified)
}

func TestHasContent(t *testing.T) {
	// Given: a MCPEvent without content
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)

	// Then: HasContent is false
	assert.Assert(t, !event.HasContent())

	// When: adding content
	event.RawMessage = "{}"

	// Then: HasContent is true
	assert.Assert(t, event.HasContent())
}

func TestGetBaseEvent(t *testing.T) {
	// Given: a MCPEvent with a request ID
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)
	event.RequestID = "req-123"

	// When: calling GetBaseEvent
	base := event.GetBaseEvent()

	// Then: base event should match
	assert.Equal(t, base.RequestID, "req-123")
}

func TestIsRequestIsResponse(t *testing.T) {
	// Given: request, response, and system MCP events
	requestEvent := NewMCPRequestEvent("test")
	responseEvent := NewMCPResponseEvent("test")
	systemEvent := NewMCPSystemEvent("test")

	// Then: request/response should be correctly identified
	assert.Assert(t, requestEvent.IsRequest())
	assert.Assert(t, !requestEvent.IsResponse())
	assert.Assert(t, responseEvent.IsResponse())
	assert.Assert(t, !responseEvent.IsRequest())
	assert.Assert(t, !systemEvent.IsRequest())
	assert.Assert(t, !systemEvent.IsResponse())
}

func TestSetStatus(t *testing.T) {
	// Given: a MCPEvent
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)

	// When: setting a status code
	event.SetStatus(204)

	// Then: status is updated
	assert.Equal(t, event.Status, 204)
}
