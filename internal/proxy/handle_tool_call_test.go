package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/T4cceptor/centian/internal/processor"
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

func createTestProxy(t *testing.T, eventProcessor ProcessingControllerInterface) *CentianEndpoint {
	t.Helper()

	t.Setenv("CENTIAN_LOG_DIR", t.TempDir())
	logger, err := logging.NewLogger()
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = logger.Close()
	})

	return &CentianEndpoint{
		name:           "test-gateway",
		endpoint:       "/mcp/test",
		eventProcessor: eventProcessor,
		server: &CentianServer{
			ServerID: "test-server-id",
			Logger:   logger,
		},
	}
}

type passthroughProcessor struct {
	cfg       *config.ProcessorConfig
	lastInput *processor.DataContext
}

func (p *passthroughProcessor) Process(input *processor.DataContext) (*processor.DataContext, error) {
	p.lastInput = input
	return input, nil
}

func (p *passthroughProcessor) GetConfig() *config.ProcessorConfig {
	return p.cfg
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

// TestHandleToolCall_AggregatedPassthroughProcessorKeepsNormalizedToolName verifies
// that a processor receiving payload and routing data and returning them unchanged
// does not reintroduce the aggregated upstream namespace into the current request state.
func TestHandleToolCall_AggregatedPassthroughProcessorKeepsNormalizedToolName(t *testing.T) {
	mockProcessor := &passthroughProcessor{
		cfg: &config.ProcessorConfig{
			Name:  "passthrough",
			Parts: []string{"payload", "routing"},
		},
	}

	mockDownstream := &MockDownstreamConnection{
		cfg: &config.MCPServerConfig{URL: "http://test"},
		ResultToReturn: &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "ok"},
			},
		},
		Status: StatusConnected,
	}

	proxy := createTestProxy(t, &ProcessingController{
		processors: []processor.ProcessorInterface{mockProcessor},
	})
	proxy.isAggregatedProxy = true

	session := &UpstreamSession{
		id: "test-session",
		downstreamConns: map[string]DownstreamConnectionInterface{
			"test-server": mockDownstream,
		},
	}

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "test-server___test-tool",
			Arguments: json.RawMessage(`{"original": "value"}`),
		},
	}

	result, err := proxy.handleToolCall(context.Background(), session, "test-server", req)

	assert.NilError(t, err)
	assert.Assert(t, result != nil)
	assert.Assert(t, mockProcessor.lastInput != nil)
	assert.Assert(t, mockProcessor.lastInput.Payload != nil)
	assert.Assert(t, mockProcessor.lastInput.Payload.Request != nil)
	assert.Assert(t, mockProcessor.lastInput.Payload.OriginalRequest != nil)
	assert.Assert(t, mockProcessor.lastInput.Routing != nil)

	assert.Equal(t, mockProcessor.lastInput.Payload.Request.Params.Name, "test-tool")
	assert.Equal(t, mockProcessor.lastInput.Payload.OriginalRequest.Params.Name, "test-server___test-tool")
	assert.Equal(t, mockProcessor.lastInput.Routing.ToolName, "test-tool")
	assert.Equal(t, mockProcessor.lastInput.Routing.OriginalToolname, "test-server___test-tool")

	assert.Equal(t, req.Params.Name, "test-tool")
	assert.Equal(t, mockDownstream.CapturedToolName, "test-tool")
	assert.Equal(t, mockDownstream.CapturedArgs["original"], "value")
}

// TestHandleToolCall_ProcessorReceivesAuthContextFromMiddlewareSession verifies
// the auth flow end-to-end: the auth middleware annotates the request, session
// creation snapshots that auth data, and the processor receives the sanitized
// auth context during a real tool call.
func TestHandleToolCall_ProcessorReceivesAuthContextFromMiddlewareSession(t *testing.T) {
	mockProcessor := &passthroughProcessor{
		cfg: &config.ProcessorConfig{
			Name:  "auth-passthrough",
			Parts: []string{"auth"},
		},
	}

	mockDownstream := &MockDownstreamConnection{
		serverName: "test-server",
		cfg:        &config.MCPServerConfig{URL: "http://test"},
		tools: []*mcp.Tool{
			{Name: "test-tool", Description: "test tool", InputSchema: map[string]any{"type": "object"}},
		},
		ResultToReturn: &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "ok"},
			},
		},
		Status: StatusConnected,
	}

	proxy := createTestProxy(t, &ProcessingController{
		processors: []processor.ProcessorInterface{mockProcessor},
	})
	proxy.name = "test-server"
	proxy.endpoint = "/mcp/gateway-a/test-server"
	proxy.config = &config.GatewayConfig{
		MCPServers: map[string]*config.MCPServerConfig{
			"test-server": {URL: "http://test"},
		},
	}
	proxy.upstreamSessions = make(map[string]*UpstreamSession)
	proxy.downstreamPools = make(map[string]*DownstreamConnectionPool)
	proxy.connectionFactory = func(_ string, _ *config.MCPServerConfig) DownstreamConnectionInterface {
		return mockDownstream
	}
	proxy.server.APIKeys = createTestAPIKeyStore(t)
	proxy.server.AuthHeader = "Authorization"
	proxy.server.Config = &config.GlobalConfig{Version: "1.0.0"}

	handler := apiKeyMiddlewareWithHeader(proxy.server.APIKeys, proxy.server.AuthHeader, "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server := proxy.GetOrCreateServerForRequest(r)
		assert.Assert(t, server != nil)
		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway-a/test-server", http.NoBody)
	request.Header.Set("Authorization", "Bearer plain-key")
	request.Header.Set("Mcp-Session-Id", "sess-auth")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	assert.Equal(t, recorder.Result().StatusCode, http.StatusOK)

	proxy.mu.RLock()
	session := proxy.upstreamSessions["sess-auth"]
	proxy.mu.RUnlock()

	assert.Assert(t, session != nil)
	attachInitializedSessionForTest(t, proxy, "sess-auth", &mcp.ClientCapabilities{})
	assert.Assert(t, session.authData != nil)
	assert.Equal(t, session.authData.AuthHeaderName, "Authorization")
	assert.Equal(t, session.authData.Gateway, "gateway-a")
	assert.Assert(t, session.authData.KeyEntry != nil)
	assert.Equal(t, session.authData.KeyEntry.ID, "key_test")

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "test-tool",
			Arguments: json.RawMessage(`{"hello":"world"}`),
		},
	}

	result, err := proxy.handleToolCall(context.Background(), session, "test-server", req)

	assert.NilError(t, err)
	assert.Assert(t, result != nil)
	assert.Assert(t, mockProcessor.lastInput != nil)
	assert.Assert(t, mockProcessor.lastInput.Auth != nil)
	assert.Assert(t, mockProcessor.lastInput.Auth.Authenticated)
	assert.Equal(t, mockProcessor.lastInput.Auth.PrincipalType, "api_key")
	assert.Equal(t, mockProcessor.lastInput.Auth.KeyID, "key_test")
	assert.Equal(t, mockProcessor.lastInput.Auth.Gateway, "gateway-a")
	assert.Equal(t, mockProcessor.lastInput.Auth.AuthHeader, "Authorization")
	assert.Equal(t, mockProcessor.lastInput.Auth.InternalSessionID, "sess-auth")
	assert.Equal(t, mockProcessor.lastInput.Auth.TransportSessionID, "sess-auth")
	assert.Equal(t, mockProcessor.lastInput.Auth.PrincipalID, getPrincipalID("key_test", "gateway-a"))
	assert.Equal(t, mockProcessor.lastInput.Auth.CredentialFingerprint, getCredentialFingerprint("plain-key"))
	assert.Equal(t, mockDownstream.CapturedToolName, "test-tool")
	assert.Equal(t, mockDownstream.CapturedArgs["hello"], "world")
}
