package logging

import (
	"encoding/json"
	"testing"

	"gotest.tools/assert"
)

// ========================================
// McpEventDirection Serialization Tests
// ========================================

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
			// Given: a valid direction
			direction := tc.direction

			// When: marshaling to JSON
			result, err := json.Marshal(direction)

			// Then: should serialize correctly
			assert.NilError(t, err)
			assert.Equal(t, tc.expected, string(result))
		})
	}
}

func TestMcpEventDirection_MarshalJSON_InvalidDirection(t *testing.T) {
	// Given: an invalid direction
	direction := McpEventDirection("INVALID_DIRECTION")

	// When: marshaling to JSON
	result, err := json.Marshal(direction)

	// Then: should serialize as UNKNOWN
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
			// Given: a JSON string with valid direction
			var direction McpEventDirection

			// When: unmarshaling from JSON
			err := json.Unmarshal([]byte(tc.json), &direction)

			// Then: should deserialize correctly
			assert.NilError(t, err)
			assert.Equal(t, tc.expected, direction)
		})
	}
}

func TestMcpEventDirection_UnmarshalJSON_InvalidDirection(t *testing.T) {
	// Given: a JSON string with invalid direction
	var direction McpEventDirection

	// When: unmarshaling from JSON
	err := json.Unmarshal([]byte(`"INVALID_DIRECTION"`), &direction)

	// Then: should default to UNKNOWN
	assert.NilError(t, err)
	assert.Equal(t, DirectionUnknown, direction)
}

func TestMcpEventDirection_UnmarshalJSON_MalformedJSON(t *testing.T) {
	// Given: malformed JSON
	var direction McpEventDirection

	// When: unmarshaling from JSON
	err := json.Unmarshal([]byte(`not valid json`), &direction)

	// Then: should return error
	assert.Assert(t, err != nil)
}

func TestMcpEventDirection_RoundTrip(t *testing.T) {
	testCases := []McpEventDirection{
		DirectionClientToServer,
		DirectionServerToClient,
		DirectionCentianToClient,
		DirectionSystem,
	}

	for _, original := range testCases {
		t.Run(string(original), func(t *testing.T) {
			// Given: a direction

			// When: marshaling and unmarshaling
			data, err := json.Marshal(original)
			assert.NilError(t, err)

			var decoded McpEventDirection
			err = json.Unmarshal(data, &decoded)
			assert.NilError(t, err)

			// Then: should preserve the value
			assert.Equal(t, original, decoded)
		})
	}
}

// ========================================
// McpMessageType Serialization Tests
// ========================================

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
			// Given: a valid message type
			msgType := tc.messageType

			// When: marshaling to JSON
			result, err := json.Marshal(msgType)

			// Then: should serialize correctly
			assert.NilError(t, err)
			assert.Equal(t, tc.expected, string(result))
		})
	}
}

func TestMcpMessageType_MarshalJSON_InvalidType(t *testing.T) {
	// Given: an invalid message type
	msgType := McpMessageType("INVALID_TYPE")

	// When: marshaling to JSON
	result, err := json.Marshal(msgType)

	// Then: should serialize as unknown
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
			// Given: a JSON string with valid message type
			var msgType McpMessageType

			// When: unmarshaling from JSON
			err := json.Unmarshal([]byte(tc.json), &msgType)

			// Then: should deserialize correctly
			assert.NilError(t, err)
			assert.Equal(t, tc.expected, msgType)
		})
	}
}

func TestMcpMessageType_UnmarshalJSON_InvalidType(t *testing.T) {
	// Given: a JSON string with invalid message type
	var msgType McpMessageType

	// When: unmarshaling from JSON
	err := json.Unmarshal([]byte(`"INVALID_TYPE"`), &msgType)

	// Then: should default to unknown
	assert.NilError(t, err)
	assert.Equal(t, MessageTypeUnknown, msgType)
}

func TestMcpMessageType_UnmarshalJSON_MalformedJSON(t *testing.T) {
	// Given: malformed JSON
	var msgType McpMessageType

	// When: unmarshaling from JSON
	err := json.Unmarshal([]byte(`{not valid json`), &msgType)

	// Then: should return error
	assert.Assert(t, err != nil)
}

func TestMcpMessageType_RoundTrip(t *testing.T) {
	testCases := []McpMessageType{
		MessageTypeRequest,
		MessageTypeResponse,
		MessageTypeSystem,
	}

	for _, original := range testCases {
		t.Run(string(original), func(t *testing.T) {
			// Given: a message type

			// When: marshaling and unmarshaling
			data, err := json.Marshal(original)
			assert.NilError(t, err)

			var decoded McpMessageType
			err = json.Unmarshal(data, &decoded)
			assert.NilError(t, err)

			// Then: should preserve the value
			assert.Equal(t, original, decoded)
		})
	}
}

// ========================================
// Edge Cases and Integration Tests
// ========================================

func TestMcpEventDirection_InStruct(t *testing.T) {
	type TestStruct struct {
		Direction McpEventDirection `json:"direction"`
	}

	// Given: a struct with direction
	original := TestStruct{Direction: DirectionClientToServer}

	// When: marshaling and unmarshaling
	data, err := json.Marshal(original)
	assert.NilError(t, err)

	var decoded TestStruct
	err = json.Unmarshal(data, &decoded)
	assert.NilError(t, err)

	// Then: should preserve the direction
	assert.Equal(t, original.Direction, decoded.Direction)
}

func TestMcpMessageType_InStruct(t *testing.T) {
	type TestStruct struct {
		Type McpMessageType `json:"type"`
	}

	// Given: a struct with message type
	original := TestStruct{Type: MessageTypeRequest}

	// When: marshaling and unmarshaling
	data, err := json.Marshal(original)
	assert.NilError(t, err)

	var decoded TestStruct
	err = json.Unmarshal(data, &decoded)
	assert.NilError(t, err)

	// Then: should preserve the message type
	assert.Equal(t, original.Type, decoded.Type)
}

func TestMcpEventDirection_EmptyStringUnmarshal(t *testing.T) {
	// Given: an empty string JSON
	var direction McpEventDirection

	// When: unmarshaling from JSON
	err := json.Unmarshal([]byte(`""`), &direction)

	// Then: should default to UNKNOWN
	assert.NilError(t, err)
	assert.Equal(t, DirectionUnknown, direction)
}

func TestMcpMessageType_EmptyStringUnmarshal(t *testing.T) {
	// Given: an empty string JSON
	var msgType McpMessageType

	// When: unmarshaling from JSON
	err := json.Unmarshal([]byte(`""`), &msgType)

	// Then: should default to unknown
	assert.NilError(t, err)
	assert.Equal(t, MessageTypeUnknown, msgType)
}
