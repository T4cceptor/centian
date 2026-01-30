package common

import (
	"encoding/json"
	"testing"
	"time"

	"gotest.tools/assert"
)

// ========================================.
// McpEventDirection Tests.
// ========================================.

func TestMcpEventDirection_MarshalJSON_ValidDirections(t *testing.T) {
	testCases := []struct {
		direction McpEventDirection
		expected  string
	}{
		{DirectionClientToServer, `"[CLIENT -\u003e SERVER]"`}, // JSON escapes >
		{DirectionServerToClient, `"[SERVER -\u003e CLIENT]"`},
		{DirectionCentianToClient, `"[CENTIAN -\u003e CLIENT]"`},
		{DirectionSystem, `"[SYSTEM]"`},
	}

	for _, tc := range testCases {
		t.Run(string(tc.direction), func(t *testing.T) {
			// Given: a valid direction.
			direction := tc.direction

			// When: marshaling to JSON.
			result, err := json.Marshal(direction)

			// Then: should serialize correctly.
			assert.NilError(t, err)
			assert.Equal(t, tc.expected, string(result))
		})
	}
}

func TestMcpEventDirection_MarshalJSON_UnknownDirection(t *testing.T) {
	// Given: an invalid/unknown direction.
	direction := McpEventDirection("INVALID")

	// When: marshaling to JSON.
	result, err := json.Marshal(direction)

	// Then: should serialize as UNKNOWN.
	assert.NilError(t, err)
	assert.Equal(t, `"[UNKNOWN]"`, string(result))
}

func TestMcpEventDirection_UnmarshalJSON_ValidDirections(t *testing.T) {
	testCases := []struct {
		json     string
		expected McpEventDirection
	}{
		{`"[CLIENT -> SERVER]"`, DirectionClientToServer},
		{`"[SERVER -> CLIENT]"`, DirectionServerToClient},
		{`"[CENTIAN -> CLIENT]"`, DirectionCentianToClient},
		{`"[SYSTEM]"`, DirectionSystem},
	}

	for _, tc := range testCases {
		t.Run(string(tc.expected), func(t *testing.T) {
			// Given: a JSON string with valid direction.
			var direction McpEventDirection

			// When: unmarshaling from JSON.
			err := json.Unmarshal([]byte(tc.json), &direction)

			// Then: should deserialize correctly.
			assert.NilError(t, err)
			assert.Equal(t, tc.expected, direction)
		})
	}
}

func TestMcpEventDirection_UnmarshalJSON_UnknownDirection(t *testing.T) {
	// Given: a JSON string with unknown direction.
	var direction McpEventDirection

	// When: unmarshaling from JSON.
	err := json.Unmarshal([]byte(`"INVALID"`), &direction)

	// Then: should default to UNKNOWN.
	assert.NilError(t, err)
	assert.Equal(t, DirectionUnknown, direction)
}

// ========================================.
// McpMessageType Tests.
// ========================================.

func TestMcpMessageType_MarshalJSON_ValidTypes(t *testing.T) {
	testCases := []struct {
		messageType McpMessageType
		expected    string
	}{
		{MessageTypeRequest, `"request"`},
		{MessageTypeResponse, `"response"`},
		{MessageTypeSystem, `"system"`},
	}

	for _, tc := range testCases {
		t.Run(string(tc.messageType), func(t *testing.T) {
			// Given: a valid message type.
			msgType := tc.messageType

			// When: marshaling to JSON.
			result, err := json.Marshal(msgType)

			// Then: should serialize correctly.
			assert.NilError(t, err)
			assert.Equal(t, tc.expected, string(result))
		})
	}
}

func TestMcpMessageType_MarshalJSON_UnknownType(t *testing.T) {
	// Given: an invalid/unknown message type.
	msgType := McpMessageType("INVALID")

	// When: marshaling to JSON.
	result, err := json.Marshal(msgType)

	// Then: should serialize as unknown.
	assert.NilError(t, err)
	assert.Equal(t, `"unknown"`, string(result))
}

func TestMcpMessageType_UnmarshalJSON_ValidTypes(t *testing.T) {
	testCases := []struct {
		json     string
		expected McpMessageType
	}{
		{`"request"`, MessageTypeRequest},
		{`"response"`, MessageTypeResponse},
		{`"system"`, MessageTypeSystem},
	}

	for _, tc := range testCases {
		t.Run(string(tc.expected), func(t *testing.T) {
			// Given: a JSON string with valid message type.
			var msgType McpMessageType

			// When: unmarshaling from JSON.
			err := json.Unmarshal([]byte(tc.json), &msgType)

			// Then: should deserialize correctly.
			assert.NilError(t, err)
			assert.Equal(t, tc.expected, msgType)
		})
	}
}

func TestMcpMessageType_UnmarshalJSON_UnknownType(t *testing.T) {
	// Given: a JSON string with unknown message type.
	var msgType McpMessageType

	// When: unmarshaling from JSON.
	err := json.Unmarshal([]byte(`"INVALID"`), &msgType)

	// Then: should default to unknown.
	assert.NilError(t, err)
	assert.Equal(t, MessageTypeUnknown, msgType)
}

// ========================================.
// StdioMcpEvent.RawMessage Tests.
// ========================================.

func TestStdioMcpEvent_RawMessage(t *testing.T) {
	// Given: a StdioMcpEvent with message.
	event := MCPEvent{}
	event.SetRawMessage("stdio test message")

	// When: calling RawMessage.
	result := event.GetRawMessage()

	// Then: should return message.
	assert.Equal(t, "stdio test message", result)
}

func TestStdioMcpEvent_RawMessage_EmptyMessage(t *testing.T) {
	// Given: a StdioMcpEvent with empty message.
	event := MCPEvent{}
	event.SetRawMessage("")

	// When: calling RawMessage.
	result := event.GetRawMessage()

	// Then: should return empty string.
	assert.Equal(t, "", result)
}

// ========================================.
// StdioMcpEvent.MarshalJSON Tests.
// ========================================.

func TestStdioMcpEvent_MarshalJSON_Complete(t *testing.T) {
	// Given: a complete StdioMcpEvent.
	event := MCPEvent{
		BaseMcpEvent: BaseMcpEvent{
			Timestamp:   time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Transport:   "stdio",
			RequestID:   "stdio-req-123",
			Direction:   DirectionClientToServer,
			MessageType: MessageTypeRequest,
			Success:     true,
		},
		Routing: RoutingContext{
			DownstreamCommand: "npx",
			Args:              []string{"--version"},
		},
	}
	event.SetRawMessage("stdio message content")

	// When: marshaling to JSON.
	data, err := json.Marshal(event)

	// Then: should serialize successfully with raw_message field.
	assert.NilError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NilError(t, err)

	assert.Equal(t, "stdio message content", result["raw_message"])
	assert.Equal(t, "stdio", result["transport"])
	assert.Equal(t, "npx", result["routing"].(map[string]any)["downstream_cmd"])
}

// ========================================.
// Integration Tests.
// ========================================.

func TestStdioMcpEvent_RoundTripJSON(t *testing.T) {
	// Given: a complete StdioMcpEvent.
	original := MCPEvent{
		BaseMcpEvent: BaseMcpEvent{
			Timestamp:   time.Date(2025, 1, 7, 12, 0, 0, 0, time.UTC),
			Transport:   "stdio",
			RequestID:   "stdio-rt-123",
			Direction:   DirectionClientToServer,
			MessageType: MessageTypeRequest,
			Success:     true,
		},
		Routing: RoutingContext{
			DownstreamCommand: "npx",
			Args:              []string{"-v"},
		},
	}
	original.SetRawMessage("stdio round trip")

	// When: marshaling and unmarshaling.
	data, err := json.Marshal(original)
	assert.NilError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	assert.NilError(t, err)

	// Then: should preserve all fields including raw_message.
	assert.Equal(t, "stdio round trip", decoded["raw_message"])
	assert.Equal(t, "stdio", decoded["transport"])
	assert.Equal(t, "npx", decoded["routing"].(map[string]any)["downstream_cmd"])
}

// ========================================.
// Edge Cases.
// ========================================.

func TestGetBaseEvent_Works(t *testing.T) {
	// Given: some MCP Events.
	baseMcpEvent := BaseMcpEvent{
		Transport: "my-test-transport",
	}
	mcpEvents := []McpEventInterface{
		&MCPEvent{
			BaseMcpEvent: baseMcpEvent,
		},
	}

	for _, event := range mcpEvents {
		// When: calling IsRequest and IsResponse.
		baseEvent := event.GetBaseEvent()

		// Then: the values map to the MessageType property on the base event.
		assert.Equal(t, baseEvent.Transport, baseMcpEvent.Transport)
	}
}

func TestSetStatus_Works(t *testing.T) {
	// Given: some MCP Events.
	mcpEvents := []McpEventInterface{
		&MCPEvent{},
	}

	for _, event := range mcpEvents {
		// When: calling IsRequest and IsResponse.
		event.SetStatus(123)

		// Then: the values map to the MessageType property on the base event.
		assert.Equal(t, event.GetBaseEvent().Status, 123)
	}
}
