package common

import (
	"encoding/json"
	"testing"
	"time"

	"gotest.tools/assert"
)

func hasDefaultValues(event *MCPEvent) bool {
	if event.Timestamp.IsZero() {
		return false
	}
	is_recent := time.Since(event.Timestamp) < 2*time.Second
	is_success := event.Success
	has_processing_error_map := event.ProcessingErrors != nil
	has_metadata_map := event.Metadata != nil
	return is_recent && is_success && has_processing_error_map && has_metadata_map
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

func TestGetBaseEvent(t *testing.T) {
	// Given: a MCPEvent with a request ID
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)
	event.RequestID = "req-123"

	// When: calling GetBaseEvent
	base := event.GetBaseEvent()

	// Then: base event should match
	assert.Equal(t, base.RequestID, "req-123")
}

func TestSetStatus(t *testing.T) {
	// Given: a MCPEvent
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)

	// When: setting a status code
	event.SetStatus(204)

	// Then: status is updated
	assert.Equal(t, event.Status, 204)
}

func TestWithToolRequest(t *testing.T) {
	// Given: an event without ToolCall.
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)
	args := json.RawMessage(`{"a":1}`)

	// When: attaching tool request data.
	updated := event.WithToolRequest("tool-a", "original-tool-a", args)

	// Then: event pointer is preserved and fields are set.
	assert.Assert(t, updated == event)
	assert.Assert(t, event.ToolCall != nil)
	assert.Equal(t, event.ToolCall.Name, "tool-a")
	assert.Equal(t, event.ToolCall.OriginalName, "original-tool-a")
	assert.Equal(t, string(event.ToolCall.Arguments), `{"a":1}`)
}

func TestWithToolRequest_PreservesExistingResultFields(t *testing.T) {
	// Given: an event with existing result state.
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)
	event.ToolCall = &ToolCallLog{
		Result:  json.RawMessage(`{"ok":true}`),
		IsError: true,
	}

	// When: setting request details.
	event.WithToolRequest("tool-b", "orig-tool-b", json.RawMessage(`{"k":"v"}`))

	// Then: request fields are updated and result fields are preserved.
	assert.Equal(t, event.ToolCall.Name, "tool-b")
	assert.Equal(t, event.ToolCall.OriginalName, "orig-tool-b")
	assert.Equal(t, string(event.ToolCall.Arguments), `{"k":"v"}`)
	assert.Equal(t, string(event.ToolCall.Result), `{"ok":true}`)
	assert.Assert(t, event.ToolCall.IsError)
}

func TestWithToolResult(t *testing.T) {
	// Given: an event without ToolCall.
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)
	result := json.RawMessage(`{"content":[{"type":"text","text":"ok"}]}`)

	// When: attaching tool result data.
	updated := event.WithToolResult(result, true)

	// Then: event pointer is preserved and fields are set.
	assert.Assert(t, updated == event)
	assert.Assert(t, event.ToolCall != nil)
	assert.Equal(t, string(event.ToolCall.Result), `{"content":[{"type":"text","text":"ok"}]}`)
	assert.Assert(t, event.ToolCall.IsError)
}

func TestWithToolResult_PreservesExistingRequestFields(t *testing.T) {
	// Given: an event with existing request state.
	event := NewMCPEvent("test", DirectionSystem, MessageTypeSystem)
	event.ToolCall = &ToolCallLog{
		Name:         "tool-c",
		OriginalName: "orig-tool-c",
		Arguments:    json.RawMessage(`{"x":1}`),
	}

	// When: setting result details.
	event.WithToolResult(json.RawMessage(`{"done":true}`), false)

	// Then: result fields are updated and request fields are preserved.
	assert.Equal(t, event.ToolCall.Name, "tool-c")
	assert.Equal(t, event.ToolCall.OriginalName, "orig-tool-c")
	assert.Equal(t, string(event.ToolCall.Arguments), `{"x":1}`)
	assert.Equal(t, string(event.ToolCall.Result), `{"done":true}`)
	assert.Assert(t, !event.ToolCall.IsError)
}
