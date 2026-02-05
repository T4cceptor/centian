package proxy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

// MockEventProcessor is a test double for EventProcessorInterface.
// It captures processed events and can modify them according to test needs.
type MockEventProcessor struct {
	ProcessedEvents []*common.MCPEvent

	// RequestModifier is called for request events; if non-nil, modifies the event
	RequestModifier func(event *common.MCPEvent)

	// ResponseModifier is called for response events; if non-nil, modifies the event
	ResponseModifier func(event *common.MCPEvent)
}

func (m *MockEventProcessor) Process(event common.McpEventInterface) error {
	mcpEvent, ok := event.(*common.MCPEvent)
	if !ok {
		return nil
	}
	m.ProcessedEvents = append(m.ProcessedEvents, mcpEvent)

	if mcpEvent.IsRequest() && m.RequestModifier != nil {
		m.RequestModifier(mcpEvent)
	}
	if mcpEvent.IsResponse() && m.ResponseModifier != nil {
		m.ResponseModifier(mcpEvent)
	}
	return nil
}

// MockDownstreamConnection is a test double for DownstreamConnectionInterface.
// It captures tool calls and returns configurable results.
type MockDownstreamConnection struct {
	connected bool
	tools     []*mcp.Tool
	cfg       *config.MCPServerConfig

	// Captured call data
	CapturedToolName string
	CapturedArgs     map[string]any

	// Result to return
	ResultToReturn *mcp.CallToolResult
	ErrorToReturn  error
}

func (m *MockDownstreamConnection) CallTool(ctx context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	m.CapturedToolName = toolName
	m.CapturedArgs = args
	return m.ResultToReturn, m.ErrorToReturn
}

func (m *MockDownstreamConnection) IsConnected() bool {
	return m.connected
}

func (m *MockDownstreamConnection) Tools() []*mcp.Tool {
	return m.tools
}

func (m *MockDownstreamConnection) Close() error {
	return nil
}

func (m *MockDownstreamConnection) GetConfig() *config.MCPServerConfig {
	return m.cfg
}

// TestHandleToolCall_ProcessorModifiesRequest verifies that when the event processor
// modifies the request, the modified content is sent to the downstream connection.
func TestHandleToolCall_ProcessorModifiesRequest(t *testing.T) {
	// Given: a mock processor that modifies request arguments
	mockProcessor := &MockEventProcessor{
		RequestModifier: func(event *common.MCPEvent) {
			// Parse current payload
			var payload map[string]any
			if err := json.Unmarshal([]byte(event.GetRawMessage()), &payload); err != nil {
				return
			}
			// Modify the params.arguments to add injected field
			if params, ok := payload["params"].(map[string]any); ok {
				if args, ok := params["arguments"].(map[string]any); ok {
					args["injected"] = "test new request content"
					params["arguments"] = args
					payload["params"] = params
				}
			}
			// Update raw message
			modified, _ := json.Marshal(payload)
			event.SetRawMessage(string(modified))
			event.SetModified(true)
		},
	}

	// And: a mock downstream that captures what it receives
	mockDownstream := &MockDownstreamConnection{
		connected: true,
		cfg:       &config.MCPServerConfig{URL: "http://test"},
		ResultToReturn: &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "original response"},
			},
		},
	}

	// And: a proxy with the mock processor and session with mock downstream
	proxy := &MCPProxy{
		name:           "test-gateway",
		endpoint:       "/mcp/test",
		eventProcessor: mockProcessor,
	}
	session := &CentianProxySession{
		id: "test-session",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"test-server": mockDownstream,
		},
	}

	// And: a tool call request
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "test-tool",
			Arguments: json.RawMessage(`{"original": "value"}`),
		},
	}

	// When: handleToolCall is invoked
	_, err := proxy.handleToolCall(context.Background(), session, "test-server", req)

	// Then: no error occurs
	assert.NilError(t, err)

	// And: the downstream received the modified arguments with injected field
	assert.Equal(t, mockDownstream.CapturedArgs["injected"], "test new request content")
	assert.Equal(t, mockDownstream.CapturedArgs["original"], "value")
}

// TestHandleToolCall_ProcessorModifiesResponse verifies that when the event processor
// modifies the response, the modified content is returned from handleToolCall.
func TestHandleToolCall_ProcessorModifiesResponse(t *testing.T) {
	// Given: a mock processor that modifies response content
	mockProcessor := &MockEventProcessor{
		ResponseModifier: func(event *common.MCPEvent) {
			// Parse current payload
			var payload map[string]any
			if err := json.Unmarshal([]byte(event.GetRawMessage()), &payload); err != nil {
				return
			}
			// Replace the result content
			payload["result"] = map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "test new response content"},
				},
			}
			// Update raw message
			modified, _ := json.Marshal(payload)
			event.SetRawMessage(string(modified))
			event.SetModified(true)
		},
	}

	// And: a mock downstream that returns original content
	mockDownstream := &MockDownstreamConnection{
		connected: true,
		cfg:       &config.MCPServerConfig{URL: "http://test"},
		ResultToReturn: &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "original response"},
			},
		},
	}

	// And: a proxy with the mock processor and session with mock downstream
	proxy := &MCPProxy{
		name:           "test-gateway",
		endpoint:       "/mcp/test",
		eventProcessor: mockProcessor,
	}
	session := &CentianProxySession{
		id: "test-session",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"test-server": mockDownstream,
		},
	}

	// And: a tool call request
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "test-tool",
			Arguments: json.RawMessage(`{"arg": "value"}`),
		},
	}

	// When: handleToolCall is invoked
	result, err := proxy.handleToolCall(context.Background(), session, "test-server", req)

	// Then: no error occurs
	assert.NilError(t, err)

	// And: the returned result contains the modified response content
	assert.Assert(t, len(result.Content) > 0)
	textContent, ok := result.Content[0].(*mcp.TextContent)
	assert.Assert(t, ok, "expected TextContent")
	assert.Equal(t, textContent.Text, "test new response content")
}
