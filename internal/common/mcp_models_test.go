package common

import (
	"encoding/json"
	"testing"

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
// Edge Cases.
// ========================================.

func TestGetBaseEvent_Works(t *testing.T) {
	// Given: some MCP Events.
	baseMcpEvent := BaseMcpEvent{
		Transport: "my-test-transport",
	}
	mcpEvents := []MCPEvent{{BaseMcpEvent: baseMcpEvent}}

	for _, event := range mcpEvents {
		// When: calling IsRequest and IsResponse.
		baseEvent := event.GetBaseEvent()

		// Then: the values map to the MessageType property on the base event.
		assert.Equal(t, baseEvent.Transport, baseMcpEvent.Transport)
	}
}

func TestSetStatus_Works(t *testing.T) {
	// Given: some MCP Events.
	mcpEvents := []MCPEvent{{}}

	for _, event := range mcpEvents {
		// When: calling IsRequest and IsResponse.
		event.SetStatus(123)

		// Then: the values map to the MessageType property on the base event.
		assert.Equal(t, event.GetBaseEvent().Status, 123)
	}
}
