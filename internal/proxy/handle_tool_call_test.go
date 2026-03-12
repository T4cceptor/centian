package proxy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

// MockEventProcessor is a test double for EventProcessorInterface.
// It branches on direction and mutates the CallContext based on test needs.
type MockEventProcessor struct {
	ProcessedDirections []common.McpEventDirection
	RequestModifier     func(callCtx CallContext)
	ResponseModifier    func(callCtx CallContext)
}

func (m *MockEventProcessor) Process(callCtx CallContext) error {
	direction := callCtx.GetDirection()
	m.ProcessedDirections = append(m.ProcessedDirections, direction)

	if direction == common.DirectionClientToServer && m.RequestModifier != nil {
		m.RequestModifier(callCtx)
	}
	if direction == common.DirectionServerToClient && m.ResponseModifier != nil {
		m.ResponseModifier(callCtx)
	}
	return nil
}

func createTestProxy(t *testing.T, eventProcessor ProcessingControllerInterface) *MCPProxy {
	t.Helper()

	t.Setenv("CENTIAN_LOG_DIR", t.TempDir())
	logger, err := logging.NewLogger()
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = logger.Close()
	})

	return &MCPProxy{
		name:           "test-gateway",
		endpoint:       "/mcp/test",
		eventProcessor: eventProcessor,
		server: &CentianProxy{
			ServerID: "test-server-id",
			Logger:   logger,
		},
	}
}

// TestHandleToolCall_ProcessorModifiesRequest verifies that when the event processor
// modifies the request, the modified content is sent to the downstream connection.
func TestHandleToolCall_ProcessorModifiesRequest(t *testing.T) {
	// Given: a mock processor that modifies request arguments
	mockProcessor := &MockEventProcessor{
		RequestModifier: func(callCtx CallContext) {
			req := callCtx.GetRequest()
			if req == nil || req.Params == nil {
				return
			}

			var payload map[string]any
			if err := json.Unmarshal(req.Params.Arguments, &payload); err != nil {
				return
			}
			payload["injected"] = "test new request content"
			modified, err := json.Marshal(payload)
			if err != nil {
				return
			}
			req.Params.Arguments = modified
		},
	}

	// And: a mock downstream that captures what it receives
	mockDownstream := &MockDownstreamConnection{
		cfg: &config.MCPServerConfig{URL: "http://test"},
		ResultToReturn: &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "original response"},
			},
		},
	}
	mockDownstream.Status = StatusConnected

	// And: a proxy with the mock processor and session with mock downstream
	proxy := createTestProxy(t, mockProcessor)
	session := &UpstreamSession{
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
	assert.DeepEqual(t, mockProcessor.ProcessedDirections, []common.McpEventDirection{
		common.DirectionClientToServer,
		common.DirectionServerToClient,
	})
}

// TestHandleToolCall_ProcessorModifiesResponse verifies that when the event processor
// modifies the response, the modified content is returned from handleToolCall.
func TestHandleToolCall_ProcessorModifiesResponse(t *testing.T) {
	// Given: a mock processor that modifies response content
	mockProcessor := &MockEventProcessor{
		ResponseModifier: func(callCtx CallContext) {
			callCtx.SetResult(&mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "test new response content"},
				},
			})
		},
	}

	// And: a mock downstream that returns original content
	mockDownstream := &MockDownstreamConnection{
		cfg: &config.MCPServerConfig{URL: "http://test"},
		ResultToReturn: &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "original response"},
			},
		},
	}
	mockDownstream.Status = StatusConnected

	// And: a proxy with the mock processor and session with mock downstream
	proxy := createTestProxy(t, mockProcessor)
	session := &UpstreamSession{
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
	assert.DeepEqual(t, mockProcessor.ProcessedDirections, []common.McpEventDirection{
		common.DirectionClientToServer,
		common.DirectionServerToClient,
	})
}
