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

// ========================================
// McpEventDirection Tests
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

func TestMcpEventDirection_MarshalJSON_UnknownDirection(t *testing.T) {
	// Given: an invalid/unknown direction
	direction := McpEventDirection("INVALID")

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

func TestMcpEventDirection_UnmarshalJSON_UnknownDirection(t *testing.T) {
	// Given: a JSON string with unknown direction
	var direction McpEventDirection

	// When: unmarshaling from JSON
	err := json.Unmarshal([]byte(`"INVALID"`), &direction)

	// Then: should default to UNKNOWN
	assert.NilError(t, err)
	assert.Equal(t, DirectionUnknown, direction)
}

// ========================================
// McpMessageType Tests
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

func TestMcpMessageType_MarshalJSON_UnknownType(t *testing.T) {
	// Given: an invalid/unknown message type
	msgType := McpMessageType("INVALID")

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

func TestMcpMessageType_UnmarshalJSON_UnknownType(t *testing.T) {
	// Given: a JSON string with unknown message type
	var msgType McpMessageType

	// When: unmarshaling from JSON
	err := json.Unmarshal([]byte(`"INVALID"`), &msgType)

	// Then: should default to unknown
	assert.NilError(t, err)
	assert.Equal(t, MessageTypeUnknown, msgType)
}

// ========================================
// NewHttpEventFromRequest Tests
// ========================================

func TestNewHttpEventFromRequest_BasicRequest(t *testing.T) {
	// Given: a basic HTTP request
	req := httptest.NewRequest("POST", "https://example.com/api/test?param=value", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom", "test-header")
	req.ContentLength = 100
	requestID := "test-req-123"

	// When: creating HttpEvent from request
	event := NewHttpEventFromRequest(req, requestID)

	// Then: should capture request details correctly
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

func TestNewHttpEventFromRequest_WithResponse(t *testing.T) {
	// Given: a request that has an associated response
	req := httptest.NewRequest("GET", "https://example.com/api", nil)
	req.Response = &http.Response{
		StatusCode: 200,
		Header:     http.Header{"X-Response": []string{"success"}},
	}
	requestID := "test-req-456"

	// When: creating HttpEvent from request
	event := NewHttpEventFromRequest(req, requestID)

	// Then: should capture response details
	assert.Equal(t, 200, event.RespStatus)
	assert.Equal(t, "success", event.RespHeaders.Get("X-Response"))
}

// ========================================
// NewHttpEventFromResponse Tests
// ========================================

func TestNewHttpEventFromResponse_SuccessResponse(t *testing.T) {
	// Given: a successful HTTP response
	req := httptest.NewRequest("GET", "https://example.com/api/users", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp := &http.Response{
		StatusCode:    200,
		ContentLength: 250,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Request:       req,
	}
	requestID := "test-resp-789"

	// When: creating HttpEvent from response
	event := NewHttpEventFromResponse(resp, requestID)

	// Then: should capture response and request details
	assert.Equal(t, requestID, event.ReqID)
	assert.Equal(t, "GET", event.Method)
	assert.Equal(t, "https://example.com/api/users", event.URL)
	assert.Equal(t, "Bearer token", event.ReqHeaders.Get("Authorization"))
	assert.Equal(t, 200, event.RespStatus)
	assert.Equal(t, "application/json", event.RespHeaders.Get("Content-Type"))
	assert.Equal(t, int64(250), event.BodySize)
	assert.Equal(t, "application/json", event.ContentType)
}

func TestNewHttpEventFromResponse_ErrorResponse(t *testing.T) {
	// Given: an error HTTP response
	req := httptest.NewRequest("POST", "https://example.com/api/create", nil)
	resp := &http.Response{
		StatusCode: 500,
		Header:     http.Header{"X-Error": []string{"internal-error"}},
		Request:    req,
	}
	requestID := "test-resp-error"

	// When: creating HttpEvent from response
	event := NewHttpEventFromResponse(resp, requestID)

	// Then: should capture error details
	assert.Equal(t, 500, event.RespStatus)
	assert.Equal(t, "internal-error", event.RespHeaders.Get("X-Error"))
}

// ========================================
// HttpMcpEvent.RawMessage Tests
// ========================================

func TestHttpMcpEvent_RawMessage_WithBody(t *testing.T) {
	// Given: an HttpMcpEvent with body content
	event := HttpMcpEvent{
		HttpEvent: &HttpEvent{
			Body: []byte("test message body"),
		},
	}

	// When: calling RawMessage
	result := event.RawMessage()

	// Then: should return body as string
	assert.Equal(t, "test message body", result)
}

func TestHttpMcpEvent_RawMessage_EmptyBody(t *testing.T) {
	// Given: an HttpMcpEvent with empty body
	event := HttpMcpEvent{
		HttpEvent: &HttpEvent{
			Body: []byte{},
		},
	}

	// When: calling RawMessage
	result := event.RawMessage()

	// Then: should return empty string
	assert.Equal(t, "", result)
}

func TestHttpMcpEvent_RawMessage_NilBody(t *testing.T) {
	// Given: an HttpMcpEvent with nil body
	event := HttpMcpEvent{
		HttpEvent: &HttpEvent{
			Body: nil,
		},
	}

	// When: calling RawMessage
	result := event.RawMessage()

	// Then: should return empty string
	assert.Equal(t, "", result)
}

// ========================================
// HttpMcpEvent.MarshalJSON Tests
// ========================================

func TestHttpMcpEvent_MarshalJSON_Complete(t *testing.T) {
	// Given: a complete HttpMcpEvent
	event := HttpMcpEvent{
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
		HttpEvent: &HttpEvent{
			Body: []byte("test body"),
		},
		Gateway:       "test-gateway",
		ServerName:    "test-server",
		Endpoint:      "/mcp/test",
		DownstreamURL: "https://downstream.example.com",
		ProxyPort:     "8080",
	}

	// When: marshaling to JSON
	data, err := json.Marshal(event)

	// Then: should serialize successfully with raw_message field
	assert.NilError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NilError(t, err)

	assert.Equal(t, "test body", result["raw_message"])
	assert.Equal(t, "http", result["transport"])
	assert.Equal(t, "req-123", result["request_id"])
	assert.Equal(t, "test-gateway", result["gateway"])
}

// ========================================
// StdioMcpEvent.RawMessage Tests
// ========================================

func TestStdioMcpEvent_RawMessage(t *testing.T) {
	// Given: a StdioMcpEvent with message
	event := StdioMcpEvent{
		Message: "stdio test message",
	}

	// When: calling RawMessage
	result := event.RawMessage()

	// Then: should return message
	assert.Equal(t, "stdio test message", result)
}

func TestStdioMcpEvent_RawMessage_EmptyMessage(t *testing.T) {
	// Given: a StdioMcpEvent with empty message
	event := StdioMcpEvent{
		Message: "",
	}

	// When: calling RawMessage
	result := event.RawMessage()

	// Then: should return empty string
	assert.Equal(t, "", result)
}

// ========================================
// StdioMcpEvent.MarshalJSON Tests
// ========================================

func TestStdioMcpEvent_MarshalJSON_Complete(t *testing.T) {
	// Given: a complete StdioMcpEvent
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

	// When: marshaling to JSON
	data, err := json.Marshal(event)

	// Then: should serialize successfully with raw_message field
	assert.NilError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NilError(t, err)

	assert.Equal(t, "stdio message content", result["raw_message"])
	assert.Equal(t, "stdio", result["transport"])
	assert.Equal(t, "npx", result["command"])
}

// ========================================
// HttpMcpEvent.DeepClone Tests
// ========================================

func TestHttpMcpEvent_DeepClone_BasicClone(t *testing.T) {
	// Given: an HttpMcpEvent with various fields
	original := &HttpMcpEvent{
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
		HttpEvent: &HttpEvent{
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

	// When: creating a deep clone
	cloned := original.DeepClone()

	// Then: cloned should have same values
	assert.Equal(t, original.RequestID, cloned.RequestID)
	assert.Equal(t, original.Gateway, cloned.Gateway)
	assert.Equal(t, original.HttpEvent.Method, cloned.HttpEvent.Method)
	assert.Equal(t, string(original.HttpEvent.Body), string(cloned.HttpEvent.Body))
	assert.Equal(t, original.Metadata["key1"], cloned.Metadata["key1"])
}

func TestHttpMcpEvent_DeepClone_ModifyClonedDoesNotAffectOriginal(t *testing.T) {
	// Given: an HttpMcpEvent
	original := &HttpMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			ProcessingErrors: map[string]error{},
			Metadata:         map[string]string{"key": "original"},
		},
		HttpEvent: &HttpEvent{
			ReqHeaders:  http.Header{"X-Test": []string{"original"}},
			RespHeaders: http.Header{"X-Response": []string{"original"}},
			Body:        []byte("original body"),
		},
	}

	// When: cloning and modifying the clone
	cloned := original.DeepClone()
	cloned.HttpEvent.ReqHeaders.Set("X-Test", "modified")
	cloned.HttpEvent.RespHeaders.Set("X-Response", "modified")
	cloned.HttpEvent.Body[0] = 'M' // Modify first byte
	cloned.Metadata["key"] = "modified"
	cloned.ProcessingErrors["new_error"] = fmt.Errorf("new")

	// Then: original should remain unchanged
	assert.Equal(t, "original", original.HttpEvent.ReqHeaders.Get("X-Test"))
	assert.Equal(t, "original", original.HttpEvent.RespHeaders.Get("X-Response"))
	assert.Equal(t, "original body", string(original.HttpEvent.Body)) // Compare as string
	assert.Equal(t, "original", original.Metadata["key"])
	assert.Equal(t, 0, len(original.ProcessingErrors))
}

func TestHttpMcpEvent_DeepClone_NilHttpEvent(t *testing.T) {
	// Given: an HttpMcpEvent with nil HttpEvent
	original := &HttpMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			RequestID:        "req-nil",
			ProcessingErrors: map[string]error{},
			Metadata:         map[string]string{},
		},
		HttpEvent: nil,
		Gateway:   "gateway-nil",
	}

	// When: cloning
	cloned := original.DeepClone()

	// Then: cloned should also have nil HttpEvent
	assert.Assert(t, cloned.HttpEvent == nil)
	assert.Equal(t, "gateway-nil", cloned.Gateway)
}

func TestHttpMcpEvent_DeepClone_EmptyMaps(t *testing.T) {
	// Given: an HttpMcpEvent with empty maps
	original := &HttpMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			ProcessingErrors: map[string]error{},
			Metadata:         map[string]string{},
		},
		HttpEvent: &HttpEvent{
			Body: []byte("test"),
		},
	}

	// When: cloning
	cloned := original.DeepClone()

	// Then: cloned should have new empty maps (not shared)
	cloned.Metadata["new"] = "value"
	assert.Equal(t, 0, len(original.Metadata))
	assert.Equal(t, 1, len(cloned.Metadata))
}

func TestHttpMcpEvent_DeepClone_NilHeaders(t *testing.T) {
	// Given: an HttpMcpEvent with nil headers
	original := &HttpMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			ProcessingErrors: map[string]error{},
			Metadata:         map[string]string{},
		},
		HttpEvent: &HttpEvent{
			ReqHeaders:  nil,
			RespHeaders: nil,
			Body:        []byte("test"),
		},
	}

	// When: cloning
	cloned := original.DeepClone()

	// Then: should handle nil headers gracefully (Clone() on nil returns nil)
	assert.Assert(t, cloned.HttpEvent.ReqHeaders == nil)
	assert.Assert(t, cloned.HttpEvent.RespHeaders == nil)
}

func TestHttpMcpEvent_DeepClone_LargeBody(t *testing.T) {
	// Given: an HttpMcpEvent with large body
	largeBody := make([]byte, 10000)
	for i := range largeBody {
		largeBody[i] = byte(i % 256)
	}

	original := &HttpMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			ProcessingErrors: map[string]error{},
			Metadata:         map[string]string{},
		},
		HttpEvent: &HttpEvent{
			Body: largeBody,
		},
	}

	// When: cloning
	cloned := original.DeepClone()

	// Then: body should be copied, not shared
	assert.Equal(t, len(original.HttpEvent.Body), len(cloned.HttpEvent.Body))
	cloned.HttpEvent.Body[0] = 0xFF
	assert.Assert(t, original.HttpEvent.Body[0] != 0xFF)
}

// ========================================
// Integration Tests
// ========================================

func TestHttpMcpEvent_RoundTripJSON(t *testing.T) {
	// Given: a complete HttpMcpEvent
	original := HttpMcpEvent{
		BaseMcpEvent: BaseMcpEvent{
			Timestamp:   time.Date(2025, 1, 7, 12, 0, 0, 0, time.UTC),
			Transport:   "http",
			RequestID:   "req-rt-123",
			Direction:   DirectionClientToServer,
			MessageType: MessageTypeRequest,
			Success:     true,
			Metadata:    map[string]string{"test": "value"},
		},
		HttpEvent: &HttpEvent{
			Body: []byte("round trip test"),
		},
		Gateway:    "test-gateway",
		ServerName: "test-server",
	}

	// When: marshaling and unmarshaling
	data, err := json.Marshal(original)
	assert.NilError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	assert.NilError(t, err)

	// Then: should preserve all fields including raw_message
	assert.Equal(t, "round trip test", decoded["raw_message"])
	assert.Equal(t, "http", decoded["transport"])
	assert.Equal(t, "req-rt-123", decoded["request_id"])
	assert.Equal(t, "test-gateway", decoded["gateway"])
}

func TestStdioMcpEvent_RoundTripJSON(t *testing.T) {
	// Given: a complete StdioMcpEvent
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

	// When: marshaling and unmarshaling
	data, err := json.Marshal(original)
	assert.NilError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	assert.NilError(t, err)

	// Then: should preserve all fields including raw_message
	assert.Equal(t, "stdio round trip", decoded["raw_message"])
	assert.Equal(t, "stdio", decoded["transport"])
	assert.Equal(t, "npx", decoded["command"])
}

// ========================================
// Edge Cases
// ========================================

func TestNewHttpEventFromRequest_ComplexURL(t *testing.T) {
	// Given: a request with complex URL including fragments
	complexURL, _ := url.Parse("https://example.com:8080/path/to/resource?param1=value1&param2=value2#fragment")
	req := &http.Request{
		Method: "GET",
		URL:    complexURL,
		Header: http.Header{},
	}

	// When: creating HttpEvent
	event := NewHttpEventFromRequest(req, "test-id")

	// Then: should preserve full URL
	assert.Equal(t, complexURL.String(), event.URL)
}

func TestMarshalWithRaw_InvalidJSON(t *testing.T) {
	// Given: a value that cannot be marshaled (e.g., contains channels)
	type InvalidStruct struct {
		Ch chan int
	}

	// When: attempting to marshal with raw
	_, err := marshalWithRaw("test", InvalidStruct{Ch: make(chan int)})

	// Then: should return error
	assert.Assert(t, err != nil)
}