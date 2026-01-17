package common

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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
// NewHTTPEventFromRequest Tests.
// ========================================.

func TestNewHTTPEventFromRequest_BasicRequest(t *testing.T) {
	// Given: a basic HTTP request.
	req := httptest.NewRequest("POST", "https://example.com/api/test?param=value", http.NoBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom", "test-header")
	req.ContentLength = 100
	requestID := "test-req-123"

	// When: creating HTTPEvent from request.
	event := NewHTTPEventFromRequest(req, requestID)

	// Then: should capture request details correctly.
	assert.Equal(t, requestID, event.ReqID)
	assert.Equal(t, "POST", event.Method)
	assert.Equal(t, "https://example.com/api/test?param=value", event.URL)
	assert.Equal(t, "application/json", event.ReqHeaders.Get("Content-Type"))
	assert.Equal(t, "test-header", event.ReqHeaders.Get("X-Custom"))
	assert.Equal(t, int64(100), event.BodySize)
	assert.Equal(t, "application/json", event.ContentType)
	assert.Equal(t, -1, event.RespStatus) // No response yet
	assert.Assert(t, event.RespHeaders == nil)
	assert.Assert(t, event.Body == nil) // Body set during processing
}

func TestNewHTTPEventFromRequest_WithResponse(t *testing.T) {
	// Given: a request that has an associated response.
	req := httptest.NewRequest("GET", "https://example.com/api", http.NoBody)
	req.Response = &http.Response{
		StatusCode: 200,
		Header:     http.Header{"X-Response": []string{"success"}},
	}
	requestID := "test-req-456"

	// When: creating HTTPEvent from request.
	event := NewHTTPEventFromRequest(req, requestID)

	// Then: should capture response details.
	assert.Equal(t, 200, event.RespStatus)
	assert.Equal(t, "success", event.RespHeaders.Get("X-Response"))
}

// ========================================.
// NewHTTPEventFromResponse Tests.
// ========================================.

func TestNewHTTPEventFromResponse_SuccessResponse(t *testing.T) {
	// Given: a successful HTTP response.
	req := httptest.NewRequest("GET", "https://example.com/api/users", http.NoBody)
	req.Header.Set("Authorization", "Bearer token")
	resp := &http.Response{
		StatusCode:    200,
		ContentLength: 250,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Request:       req,
	}
	requestID := "test-resp-789"

	// When: creating HTTPEvent from response.
	event := NewHTTPEventFromResponse(resp, requestID)

	// Then: should capture response and request details.
	assert.Equal(t, requestID, event.ReqID)
	assert.Equal(t, "GET", event.Method)
	assert.Equal(t, "https://example.com/api/users", event.URL)
	assert.Equal(t, "Bearer token", event.ReqHeaders.Get("Authorization"))
	assert.Equal(t, 200, event.RespStatus)
	assert.Equal(t, "application/json", event.RespHeaders.Get("Content-Type"))
	assert.Equal(t, int64(250), event.BodySize)
	assert.Equal(t, "application/json", event.ContentType)
}

func TestNewHTTPEventFromResponse_ErrorResponse(t *testing.T) {
	// Given: an error HTTP response.
	req := httptest.NewRequest("POST", "https://example.com/api/create", http.NoBody)
	resp := &http.Response{
		StatusCode: 500,
		Header:     http.Header{"X-Error": []string{"internal-error"}},
		Request:    req,
	}
	requestID := "test-resp-error"

	// When: creating HTTPEvent from response.
	event := NewHTTPEventFromResponse(resp, requestID)

	// Then: should capture error details.
	assert.Equal(t, 500, event.RespStatus)
	assert.Equal(t, "internal-error", event.RespHeaders.Get("X-Error"))
}

// ========================================.
// HTTPMcpEvent.RawMessage Tests.
// ========================================.

func TestHTTPMcpEvent_RawMessage_WithBody(t *testing.T) {
	// Given: an HTTPMcpEvent with body content.
	event := HTTPMcpEvent{
		HTTPEvent: &HTTPEvent{
			Body: []byte("test message body"),
		},
	}

	// When: calling RawMessage.
	result := event.RawMessage()

	// Then: should return body as string.
	assert.Equal(t, "test message body", result)
}

func TestHTTPMcpEvent_RawMessage_EmptyBody(t *testing.T) {
	// Given: an HTTPMcpEvent with empty body.
	event := HTTPMcpEvent{
		HTTPEvent: &HTTPEvent{
			Body: []byte{},
		},
	}

	// When: calling RawMessage.
	result := event.RawMessage()

	// Then: should return empty string.
	assert.Equal(t, "", result)
}

func TestHTTPMcpEvent_RawMessage_NilBody(t *testing.T) {
	// Given: an HTTPMcpEvent with nil body.
	event := HTTPMcpEvent{
		HTTPEvent: &HTTPEvent{
			Body: nil,
		},
	}

	// When: calling RawMessage.
	result := event.RawMessage()

	// Then: should return empty string.
	assert.Equal(t, "", result)
}

// ========================================.
// HTTPMcpEvent.MarshalJSON Tests.
// ========================================.

func TestHTTPMcpEvent_MarshalJSON_Complete(t *testing.T) {
	// Given: a complete HTTPMcpEvent.
	event := HTTPMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			Timestamp:   time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Transport:   "http",
			RequestID:   "req-123",
			SessionID:   "session-456",
			ServerID:    "server-789",
			Direction:   DirectionClientToServer,
			MessageType: MessageTypeRequest,
			Success:     true,
			Metadata:    map[string]string{"key": "value"},
		},
		HTTPEvent: &HTTPEvent{
			Body: []byte("test body"),
		},
		Gateway:       "test-gateway",
		ServerName:    "test-server",
		Endpoint:      "/mcp/test",
		DownstreamURL: "https://downstream.example.com",
		ProxyPort:     "8080",
	}

	// When: marshaling to JSON.
	data, err := json.Marshal(event)

	// Then: should serialize successfully with raw_message field.
	assert.NilError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NilError(t, err)

	assert.Equal(t, "test body", result["raw_message"])
	assert.Equal(t, "http", result["transport"])
	assert.Equal(t, "req-123", result["request_id"])
	assert.Equal(t, "test-gateway", result["gateway"])
}

// ========================================.
// StdioMcpEvent.RawMessage Tests.
// ========================================.

func TestStdioMcpEvent_RawMessage(t *testing.T) {
	// Given: a StdioMcpEvent with message.
	event := StdioMcpEvent{
		Message: "stdio test message",
	}

	// When: calling RawMessage.
	result := event.RawMessage()

	// Then: should return message.
	assert.Equal(t, "stdio test message", result)
}

func TestStdioMcpEvent_RawMessage_EmptyMessage(t *testing.T) {
	// Given: a StdioMcpEvent with empty message.
	event := StdioMcpEvent{
		Message: "",
	}

	// When: calling RawMessage.
	result := event.RawMessage()

	// Then: should return empty string.
	assert.Equal(t, "", result)
}

// ========================================.
// StdioMcpEvent.MarshalJSON Tests.
// ========================================.

func TestStdioMcpEvent_MarshalJSON_Complete(t *testing.T) {
	// Given: a complete StdioMcpEvent.
	event := StdioMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			Timestamp:   time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Transport:   "stdio",
			RequestID:   "stdio-req-123",
			Direction:   DirectionClientToServer,
			MessageType: MessageTypeRequest,
			Success:     true,
		},
		Command:      "npx",
		Args:         []string{"--version"},
		ProjectPath:  "/project",
		ConfigSource: "global",
		Message:      "stdio message content",
	}

	// When: marshaling to JSON.
	data, err := json.Marshal(event)

	// Then: should serialize successfully with raw_message field.
	assert.NilError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NilError(t, err)

	assert.Equal(t, "stdio message content", result["raw_message"])
	assert.Equal(t, "stdio", result["transport"])
	assert.Equal(t, "npx", result["command"])
}

// ========================================.
// HTTPMcpEvent.DeepClone Tests.
// ========================================.

func TestHTTPMcpEvent_DeepClone_BasicClone(t *testing.T) {
	// Given: an HTTPMcpEvent with various fields.
	original := &HTTPMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			Timestamp:        time.Now(),
			Transport:        "http",
			RequestID:        "req-clone-1",
			SessionID:        "session-1",
			Direction:        DirectionClientToServer,
			MessageType:      MessageTypeRequest,
			Success:          true,
			ProcessingErrors: map[string]error{"err1": fmt.Errorf("error1")},
			Metadata:         map[string]string{"key1": "value1"},
		},
		HTTPEvent: &HTTPEvent{
			ReqID:       "req-1",
			Method:      "POST",
			URL:         "https://example.com",
			ReqHeaders:  http.Header{"X-Test": []string{"original"}},
			RespHeaders: http.Header{"X-Response": []string{"resp-original"}},
			Body:        []byte("original body"),
			BodySize:    100,
		},
		Gateway:       "gateway-1",
		ServerName:    "server-1",
		Endpoint:      "/endpoint",
		DownstreamURL: "https://downstream.com",
		ProxyPort:     "8080",
	}

	// When: creating a deep clone.
	cloned := original.DeepClone()

	// Then: cloned should have same values.
	assert.Equal(t, original.RequestID, cloned.RequestID)
	assert.Equal(t, original.Gateway, cloned.Gateway)
	assert.Equal(t, original.HTTPEvent.Method, cloned.HTTPEvent.Method)
	assert.Equal(t, string(original.HTTPEvent.Body), string(cloned.HTTPEvent.Body))
	assert.Equal(t, original.Metadata["key1"], cloned.Metadata["key1"])
}

func TestHTTPMcpEvent_DeepClone_ModifyClonedDoesNotAffectOriginal(t *testing.T) {
	// Given: an HTTPMcpEvent.
	original := &HTTPMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			ProcessingErrors: map[string]error{},
			Metadata:         map[string]string{"key": "original"},
		},
		HTTPEvent: &HTTPEvent{
			ReqHeaders:  http.Header{"X-Test": []string{"original"}},
			RespHeaders: http.Header{"X-Response": []string{"original"}},
			Body:        []byte("original body"),
		},
	}

	// When: cloning and modifying the clone.
	cloned := original.DeepClone()
	cloned.HTTPEvent.ReqHeaders.Set("X-Test", "modified")
	cloned.HTTPEvent.RespHeaders.Set("X-Response", "modified")
	cloned.HTTPEvent.Body[0] = 'M' // Modify first byte
	cloned.Metadata["key"] = "modified"
	cloned.ProcessingErrors["new_error"] = fmt.Errorf("new")

	// Then: original should remain unchanged.
	assert.Equal(t, "original", original.HTTPEvent.ReqHeaders.Get("X-Test"))
	assert.Equal(t, "original", original.HTTPEvent.RespHeaders.Get("X-Response"))
	assert.Equal(t, "original body", string(original.HTTPEvent.Body)) // Compare as string
	assert.Equal(t, "original", original.Metadata["key"])
	assert.Equal(t, 0, len(original.ProcessingErrors))
}

func TestHTTPMcpEvent_DeepClone_NilHTTPEvent(t *testing.T) {
	// Given: an HTTPMcpEvent with nil HTTPEvent.
	original := &HTTPMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			RequestID:        "req-nil",
			ProcessingErrors: map[string]error{},
			Metadata:         map[string]string{},
		},
		HTTPEvent: nil,
		Gateway:   "gateway-nil",
	}

	// When: cloning.
	cloned := original.DeepClone()

	// Then: cloned should also have nil HTTPEvent.
	assert.Assert(t, cloned.HTTPEvent == nil)
	assert.Equal(t, "gateway-nil", cloned.Gateway)
}

func TestHTTPMcpEvent_DeepClone_EmptyMaps(t *testing.T) {
	// Given: an HTTPMcpEvent with empty maps.
	original := &HTTPMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			ProcessingErrors: map[string]error{},
			Metadata:         map[string]string{},
		},
		HTTPEvent: &HTTPEvent{
			Body: []byte("test"),
		},
	}

	// When: cloning.
	cloned := original.DeepClone()

	// Then: cloned should have new empty maps (not shared).
	cloned.Metadata["new"] = "value"
	assert.Equal(t, 0, len(original.Metadata))
	assert.Equal(t, 1, len(cloned.Metadata))
}

func TestHTTPMcpEvent_DeepClone_NilHeaders(t *testing.T) {
	// Given: an HTTPMcpEvent with nil headers.
	original := &HTTPMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			ProcessingErrors: map[string]error{},
			Metadata:         map[string]string{},
		},
		HTTPEvent: &HTTPEvent{
			ReqHeaders:  nil,
			RespHeaders: nil,
			Body:        []byte("test"),
		},
	}

	// When: cloning.
	cloned := original.DeepClone()

	// Then: should handle nil headers gracefully (Clone() on nil returns nil).
	assert.Assert(t, cloned.HTTPEvent.ReqHeaders == nil)
	assert.Assert(t, cloned.HTTPEvent.RespHeaders == nil)
}

func TestHTTPMcpEvent_DeepClone_LargeBody(t *testing.T) {
	// Given: an HTTPMcpEvent with large body.
	largeBody := make([]byte, 10000)
	for i := range largeBody {
		largeBody[i] = byte(i % 256)
	}

	original := &HTTPMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			ProcessingErrors: map[string]error{},
			Metadata:         map[string]string{},
		},
		HTTPEvent: &HTTPEvent{
			Body: largeBody,
		},
	}

	// When: cloning.
	cloned := original.DeepClone()

	// Then: body should be copied, not shared.
	assert.Equal(t, len(original.HTTPEvent.Body), len(cloned.HTTPEvent.Body))
	cloned.HTTPEvent.Body[0] = 0xFF
	assert.Assert(t, original.HTTPEvent.Body[0] != 0xFF)
}

// ========================================.
// Integration Tests.
// ========================================.

func TestHTTPMcpEvent_RoundTripJSON(t *testing.T) {
	// Given: a complete HTTPMcpEvent.
	original := HTTPMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			Timestamp:   time.Date(2025, 1, 7, 12, 0, 0, 0, time.UTC),
			Transport:   "http",
			RequestID:   "req-rt-123",
			Direction:   DirectionClientToServer,
			MessageType: MessageTypeRequest,
			Success:     true,
			Metadata:    map[string]string{"test": "value"},
		},
		HTTPEvent: &HTTPEvent{
			Body: []byte("round trip test"),
		},
		Gateway:    "test-gateway",
		ServerName: "test-server",
	}

	// When: marshaling and unmarshaling.
	data, err := json.Marshal(original)
	assert.NilError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	assert.NilError(t, err)

	// Then: should preserve all fields including raw_message.
	assert.Equal(t, "round trip test", decoded["raw_message"])
	assert.Equal(t, "http", decoded["transport"])
	assert.Equal(t, "req-rt-123", decoded["request_id"])
	assert.Equal(t, "test-gateway", decoded["gateway"])
}

func TestStdioMcpEvent_RoundTripJSON(t *testing.T) {
	// Given: a complete StdioMcpEvent.
	original := StdioMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			Timestamp:   time.Date(2025, 1, 7, 12, 0, 0, 0, time.UTC),
			Transport:   "stdio",
			RequestID:   "stdio-rt-123",
			Direction:   DirectionClientToServer,
			MessageType: MessageTypeRequest,
			Success:     true,
		},
		Command: "npx",
		Args:    []string{"-v"},
		Message: "stdio round trip",
	}

	// When: marshaling and unmarshaling.
	data, err := json.Marshal(original)
	assert.NilError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	assert.NilError(t, err)

	// Then: should preserve all fields including raw_message.
	assert.Equal(t, "stdio round trip", decoded["raw_message"])
	assert.Equal(t, "stdio", decoded["transport"])
	assert.Equal(t, "npx", decoded["command"])
}

// ========================================.
// Edge Cases.
// ========================================.

func TestNewHTTPEventFromRequest_ComplexURL(t *testing.T) {
	// Given: a request with complex URL including fragments.
	complexURL, _ := url.Parse("https://example.com:8080/path/to/resource?param1=value1&param2=value2#fragment")
	req := &http.Request{
		Method: "GET",
		URL:    complexURL,
		Header: http.Header{},
	}

	// When: creating HTTPEvent.
	event := NewHTTPEventFromRequest(req, "test-id")

	// Then: should preserve full URL.
	assert.Equal(t, complexURL.String(), event.URL)
}

func TestMarshalWithRaw_InvalidJSON(t *testing.T) {
	// Given: a value that cannot be marshaled (e.g., contains channels).
	type InvalidStruct struct {
		Ch chan int
	}

	// When: attempting to marshal with raw.
	_, err := marshalWithRaw("test", InvalidStruct{Ch: make(chan int)})

	// Then: should return error.
	assert.Assert(t, err != nil)
}

func TestMcpEventInterface_SetRawMessage_Works(t *testing.T) {
	// Given: some MCP Events.
	mcpEvents := []McpEventInterface{
		&StdioMcpEvent{},
		&HTTPMcpEvent{},
	}
	newBody := "test 123"

	for _, event := range mcpEvents {
		// sanity check.
		assert.Equal(t, event.RawMessage(), "")

		// When: calling SetModified.
		event.SetRawMessage(newBody)

		assert.Equal(t, event.RawMessage(), newBody)
	}
}

func TestMcpEventInterface_SetModified_Works(t *testing.T) {
	// Given: some MCP Events.
	mcpEvents := []McpEventInterface{
		&StdioMcpEvent{},
		&HTTPMcpEvent{},
	}

	for _, event := range mcpEvents {
		// When: calling SetModified.
		event.SetModified(true)

		// Then: the value is modified as expected.
		assert.Equal(t, event.GetBaseEvent().Modified, true)
	}
}

func TestMcpEventInterface_HasContent_Works(t *testing.T) {
	// Given: some MCP Events.
	mcpEvents := []McpEventInterface{
		&StdioMcpEvent{},
		&HTTPMcpEvent{},
	}

	for _, event := range mcpEvents {
		// When: calling HasContent.
		hasContent := event.HasContent()

		// Then: hasContent is false, as there is no content.
		assert.Equal(t, hasContent, false)

		// When: adding content.
		event.SetRawMessage("test message")
		hasContent = event.HasContent()

		// Then: hasContent is now true.
		assert.Equal(t, hasContent, true)
	}
}

func TestMcpEventInterface_IsRequestIsResponse_Works(t *testing.T) {
	// Given: some MCP Events.
	mcpEvents := []McpEventInterface{
		&StdioMcpEvent{
			BaseMcpEvent: BaseMcpEvent{
				MessageType: MessageTypeRequest,
			},
		},
		&HTTPMcpEvent{
			BaseMcpEvent: BaseMcpEvent{
				MessageType: MessageTypeRequest,
			},
		},
		&StdioMcpEvent{
			BaseMcpEvent: BaseMcpEvent{
				MessageType: MessageTypeResponse,
			},
		},
		&HTTPMcpEvent{
			BaseMcpEvent: BaseMcpEvent{
				MessageType: MessageTypeResponse,
			},
		},
		&StdioMcpEvent{
			BaseMcpEvent: BaseMcpEvent{
				MessageType: MessageTypeSystem,
			},
		},
		&HTTPMcpEvent{
			BaseMcpEvent: BaseMcpEvent{
				MessageType: MessageTypeSystem,
			},
		},
		&StdioMcpEvent{
			BaseMcpEvent: BaseMcpEvent{
				MessageType: MessageTypeUnknown,
			},
		},
		&HTTPMcpEvent{
			BaseMcpEvent: BaseMcpEvent{
				MessageType: MessageTypeUnknown,
			},
		},
	}

	for _, event := range mcpEvents {
		// When: calling IsRequest and IsResponse.
		isRequest := event.IsRequest()
		isResponse := event.IsResponse()

		// Then: the values map to the MessageType property on the base event.
		assert.Equal(t, isRequest, event.GetBaseEvent().MessageType == MessageTypeRequest)
		assert.Equal(t, isResponse, event.GetBaseEvent().MessageType == MessageTypeResponse)
	}
}

func TestGetBaseEvent_Works(t *testing.T) {
	// Given: some MCP Events.
	baseMcpEvent := BaseMcpEvent{
		Transport: "my-test-transport",
	}
	mcpEvents := []McpEventInterface{
		&StdioMcpEvent{
			BaseMcpEvent: baseMcpEvent,
		},
		&HTTPMcpEvent{
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
		&StdioMcpEvent{},
		&HTTPMcpEvent{},
	}

	for _, event := range mcpEvents {
		// When: calling IsRequest and IsResponse.
		event.SetStatus(123)

		// Then: the values map to the MessageType property on the base event.
		assert.Equal(t, event.GetBaseEvent().Status, 123)
	}
}
