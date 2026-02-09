package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/processor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

type mockCallContext struct {
	request            *mcp.CallToolRequest
	originalRequest    *mcp.CallToolRequest
	result             *mcp.CallToolResult
	originalResult     *mcp.CallToolResult
	event              *common.MCPEvent
	routing            *common.RoutingLog
	handlers           map[string]CallContextHandler
	logHandler         LogHandler
	serverName         string
	originalServerName string
}

func newMockCallContext() *mockCallContext {
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "test-tool",
			Arguments: json.RawMessage(`{"hello":"world"}`),
		},
	}

	return &mockCallContext{
		request:            req,
		originalRequest:    deepCloneRequest(req),
		event:              common.NewMCPRequestEvent("stdio").WithRequestID("req-1").WithSessionID("sess-1"),
		routing:            &common.RoutingLog{ServerName: "server-a", Transport: common.StdioTransport},
		handlers:           map[string]CallContextHandler{},
		serverName:         "server-a",
		originalServerName: "server-a",
	}
}

func (m *mockCallContext) SendRequest(ctx context.Context) error { return nil }
func (m *mockCallContext) HasResult() bool                       { return m.result != nil }
func (m *mockCallContext) GetResult() *mcp.CallToolResult        { return m.result }
func (m *mockCallContext) GetOriginalResult() *mcp.CallToolResult {
	return m.originalResult
}
func (m *mockCallContext) SetResult(result *mcp.CallToolResult) {
	if m.originalResult == nil {
		m.originalResult = result
	}
	m.result = result
}
func (m *mockCallContext) GetEventInfo() *common.MCPEvent { return m.event }
func (m *mockCallContext) SetEventInfo(event *common.MCPEvent) {
	m.event = event
}
func (m *mockCallContext) GetOriginalServerName() string { return m.originalServerName }
func (m *mockCallContext) GetOriginalRequest() *mcp.CallToolRequest {
	return m.originalRequest
}
func (m *mockCallContext) GetOriginalToolName() string {
	if m.originalRequest == nil || m.originalRequest.Params == nil {
		return ""
	}
	return m.originalRequest.Params.Name
}
func (m *mockCallContext) GetServerName() string { return m.serverName }
func (m *mockCallContext) SetServerName(name string) {
	m.serverName = name
	if m.routing != nil {
		m.routing.ServerName = name
	}
}
func (m *mockCallContext) GetRequest() *mcp.CallToolRequest { return m.request }
func (m *mockCallContext) GetToolName() string {
	if m.request == nil || m.request.Params == nil {
		return ""
	}
	return m.request.Params.Name
}
func (m *mockCallContext) GetStatus() int       { return m.event.Status }
func (m *mockCallContext) SetStatus(status int) { m.event.Status = status }
func (m *mockCallContext) GetError() string     { return m.event.Error }
func (m *mockCallContext) SetError(msg string)  { m.event.Error = msg }
func (m *mockCallContext) GetRequestID() string { return m.event.RequestID }
func (m *mockCallContext) GetSessionID() string { return m.event.SessionID }
func (m *mockCallContext) GetDirection() common.McpEventDirection {
	return m.event.Direction
}
func (m *mockCallContext) SetDirection(direction common.McpEventDirection) {
	m.event.Direction = direction
}
func (m *mockCallContext) GetMessageType() common.McpMessageType { return m.event.MessageType }
func (m *mockCallContext) SetMessageType(msgType common.McpMessageType) {
	m.event.MessageType = msgType
}
func (m *mockCallContext) GetRoutingContext() *common.RoutingLog { return m.routing }
func (m *mockCallContext) GetHandler(part string) (CallContextHandler, bool) {
	handler, ok := m.handlers[part]
	return handler, ok
}
func (m *mockCallContext) SetHandler(part string, handler CallContextHandler) {
	m.handlers[part] = handler
}
func (m *mockCallContext) GetLogHandler() LogHandler { return m.logHandler }
func (m *mockCallContext) SetLogHandler(handler LogHandler) {
	m.logHandler = handler
}

var _ CallContext = (*mockCallContext)(nil)

type mockHandler struct {
	getCalls   int
	applyCalls int
	getFn      func(callCtx CallContext, input *processor.DataContext)
	applyFn    func(callCtx CallContext, output *processor.DataContext) error
}

func (m *mockHandler) AttachPart(callCtx CallContext, input *processor.DataContext) {
	m.getCalls++
	if m.getFn != nil {
		m.getFn(callCtx, input)
	}
}

func (m *mockHandler) Apply(callCtx CallContext, output *processor.DataContext) error {
	m.applyCalls++
	if m.applyFn != nil {
		return m.applyFn(callCtx, output)
	}
	return nil
}

type mockLogHandler struct {
	logCalls int
	logErr   error
}

func (m *mockLogHandler) ToLogEntry(callCtx CallContext) *common.MCPEvent { return nil }
func (m *mockLogHandler) Log(callCtx CallContext) error {
	m.logCalls++
	return m.logErr
}

type mockProcessor struct {
	cfg       *config.ProcessorConfig
	processFn func(input *processor.DataContext) (*processor.DataContext, error)
	callCount int
	lastInput *processor.DataContext
}

func (m *mockProcessor) Process(input *processor.DataContext) (*processor.DataContext, error) {
	m.callCount++
	m.lastInput = input
	if m.processFn != nil {
		return m.processFn(input)
	}
	return &processor.DataContext{}, nil
}

func (m *mockProcessor) GetConfig() *config.ProcessorConfig {
	return m.cfg
}

var _ processor.ProcessorInterface = (*mockProcessor)(nil)

func TestNewProcessingController_SkipsNonRequiredInvalidProcessor(t *testing.T) {
	ep, err := NewProcessingController([]*config.ProcessorConfig{
		{
			Name:     "optional-disabled",
			Type:     "cli",
			Enabled:  false,
			Required: false,
		},
	})

	assert.NilError(t, err)
	assert.Equal(t, 0, len(ep.processors))
}

func TestNewProcessingController_FailsRequiredInvalidProcessor(t *testing.T) {
	_, err := NewProcessingController([]*config.ProcessorConfig{
		{
			Name:     "required-disabled",
			Type:     "cli",
			Enabled:  false,
			Required: true,
		},
	})

	assert.Assert(t, err != nil)
}

func TestGetInput_UsesConfiguredHandlers(t *testing.T) {
	callCtx := newMockCallContext()

	payloadHandler := &mockHandler{
		getFn: func(callCtx CallContext, input *processor.DataContext) {
			input.Payload = &processor.PayloadPart{Request: callCtx.GetRequest()}
		},
	}
	metaHandler := &mockHandler{
		getFn: func(callCtx CallContext, input *processor.DataContext) {
			input.Event = callCtx.GetEventInfo()
		},
	}

	callCtx.SetHandler("payload", payloadHandler)
	callCtx.SetHandler("meta", metaHandler)

	processorConfig := &config.ProcessorConfig{
		Parts: []string{"payload", "meta"},
	}

	input := GetInput(processorConfig, callCtx)

	assert.Assert(t, input.Payload != nil)
	assert.Assert(t, input.Event != nil)
	assert.Equal(t, 1, payloadHandler.getCalls)
	assert.Equal(t, 1, metaHandler.getCalls)
}

func TestGetInput_SkipsMissingHandler(t *testing.T) {
	callCtx := newMockCallContext()
	payloadHandler := &mockHandler{
		getFn: func(callCtx CallContext, input *processor.DataContext) {
			input.Payload = &processor.PayloadPart{}
		},
	}

	callCtx.SetHandler("payload", payloadHandler)

	processorConfig := &config.ProcessorConfig{
		Parts: []string{"payload", "routing"},
	}

	input := GetInput(processorConfig, callCtx)

	assert.Assert(t, input.Payload != nil)
	assert.Assert(t, input.Routing == nil)
	assert.Equal(t, 1, payloadHandler.getCalls)
}

func TestApplyResult_AppliesConfiguredHandlers(t *testing.T) {
	callCtx := newMockCallContext()

	payloadHandler := &mockHandler{
		applyFn: func(callCtx CallContext, output *processor.DataContext) error {
			callCtx.SetStatus(201)
			return nil
		},
	}
	metaHandler := &mockHandler{
		applyFn: func(callCtx CallContext, output *processor.DataContext) error {
			callCtx.SetError("meta-updated")
			return nil
		},
	}
	callCtx.SetHandler("payload", payloadHandler)
	callCtx.SetHandler("meta", metaHandler)

	processorConfig := &config.ProcessorConfig{
		Parts: []string{"payload", "meta"},
	}
	result := &processor.DataContext{
		Payload: &processor.PayloadPart{},
		Event:   common.NewMCPRequestEvent("stdio"),
	}

	err := ApplyResult(processorConfig, result, callCtx)
	assert.NilError(t, err)
	assert.Equal(t, 1, payloadHandler.applyCalls)
	assert.Equal(t, 1, metaHandler.applyCalls)
	assert.Equal(t, 201, callCtx.GetStatus())
	assert.Equal(t, "meta-updated", callCtx.GetError())
}

func TestApplyResult_MissingHandlerReturnsError(t *testing.T) {
	callCtx := newMockCallContext()
	processorConfig := &config.ProcessorConfig{
		Parts: []string{"payload"},
	}

	err := ApplyResult(processorConfig, &processor.DataContext{}, callCtx)
	assert.Assert(t, err != nil)
}

func TestApplyResult_HandlerErrorReturned(t *testing.T) {
	callCtx := newMockCallContext()
	expectedErr := errors.New("apply failed")
	handler := &mockHandler{
		applyFn: func(callCtx CallContext, output *processor.DataContext) error {
			return expectedErr
		},
	}
	callCtx.SetHandler("payload", handler)

	processorConfig := &config.ProcessorConfig{
		Parts: []string{"payload"},
	}

	err := ApplyResult(processorConfig, &processor.DataContext{}, callCtx)
	assert.Equal(t, expectedErr, err)
}

func TestProcess_NoProcessors_LogsBeforeAndAfter(t *testing.T) {
	callCtx := newMockCallContext()
	logHandler := &mockLogHandler{}
	callCtx.SetLogHandler(logHandler)

	ep := &ProcessingController{
		processors:          nil,
		logBeforeProcessing: true,
		logAfterProcessing:  true,
	}

	err := ep.Process(callCtx)
	assert.NilError(t, err)
	assert.Equal(t, 2, logHandler.logCalls)
}

func TestProcess_ExecutesProcessorsInOrder(t *testing.T) {
	callCtx := newMockCallContext()
	callCtx.SetLogHandler(&mockLogHandler{})

	payloadHandler := &mockHandler{
		getFn: func(callCtx CallContext, input *processor.DataContext) {
			input.Payload = &processor.PayloadPart{Request: callCtx.GetRequest()}
		},
		applyFn: func(callCtx CallContext, output *processor.DataContext) error {
			if output.Payload == nil {
				return nil
			}
			if output.Payload.Request != nil {
				callCtx.GetRequest().Params = output.Payload.Request.Params
			}
			if output.Payload.Result != nil {
				callCtx.SetResult(output.Payload.Result)
			}
			return nil
		},
	}
	callCtx.SetHandler("payload", payloadHandler)

	cfg1 := &config.ProcessorConfig{Name: "p1", Parts: []string{"payload"}}
	cfg2 := &config.ProcessorConfig{Name: "p2", Parts: []string{"payload"}}

	p1 := &mockProcessor{
		cfg: cfg1,
		processFn: func(input *processor.DataContext) (*processor.DataContext, error) {
			request := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Name:      input.Payload.Request.Params.Name,
					Arguments: json.RawMessage(`{"step":1}`),
				},
			}
			return &processor.DataContext{
				Payload: &processor.PayloadPart{Request: request},
			}, nil
		},
	}
	p2 := &mockProcessor{
		cfg: cfg2,
		processFn: func(input *processor.DataContext) (*processor.DataContext, error) {
			var payload map[string]any
			_ = json.Unmarshal(input.Payload.Request.Params.Arguments, &payload)
			assert.Equal(t, float64(1), payload["step"])
			return &processor.DataContext{
				Payload: &processor.PayloadPart{
					Result: &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: "done"}},
					},
				},
			}, nil
		},
	}

	ep := &ProcessingController{
		processors:          []processor.ProcessorInterface{p1, p2},
		logBeforeProcessing: false,
		logAfterProcessing:  false,
	}

	err := ep.Process(callCtx)
	assert.NilError(t, err)
	assert.Equal(t, 1, p1.callCount)
	assert.Equal(t, 1, p2.callCount)
	assert.Assert(t, callCtx.HasResult())
}

func TestProcess_ProcessorErrorStopsPipeline(t *testing.T) {
	callCtx := newMockCallContext()
	callCtx.SetLogHandler(&mockLogHandler{})
	callCtx.SetHandler("payload", &mockHandler{
		getFn: func(callCtx CallContext, input *processor.DataContext) {
			input.Payload = &processor.PayloadPart{Request: callCtx.GetRequest()}
		},
	})

	expectedErr := errors.New("processor failed")
	p1 := &mockProcessor{
		cfg: &config.ProcessorConfig{Name: "p1", Parts: []string{"payload"}},
		processFn: func(input *processor.DataContext) (*processor.DataContext, error) {
			return nil, expectedErr
		},
	}
	p2 := &mockProcessor{
		cfg: &config.ProcessorConfig{Name: "p2", Parts: []string{"payload"}},
	}

	ep := &ProcessingController{
		processors:          []processor.ProcessorInterface{p1, p2},
		logBeforeProcessing: false,
		logAfterProcessing:  false,
	}

	err := ep.Process(callCtx)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, 1, p1.callCount)
	assert.Equal(t, 0, p2.callCount)
}

func TestProcess_ApplyErrorStopsPipeline(t *testing.T) {
	callCtx := newMockCallContext()
	callCtx.SetLogHandler(&mockLogHandler{})
	callCtx.SetHandler("payload", &mockHandler{
		getFn: func(callCtx CallContext, input *processor.DataContext) {
			input.Payload = &processor.PayloadPart{}
		},
		applyFn: func(callCtx CallContext, output *processor.DataContext) error {
			return errors.New("apply failed")
		},
	})

	p1 := &mockProcessor{
		cfg: &config.ProcessorConfig{Name: "p1", Parts: []string{"payload"}},
		processFn: func(input *processor.DataContext) (*processor.DataContext, error) {
			return &processor.DataContext{Payload: &processor.PayloadPart{}}, nil
		},
	}

	ep := &ProcessingController{
		processors:          []processor.ProcessorInterface{p1},
		logBeforeProcessing: false,
		logAfterProcessing:  false,
	}

	err := ep.Process(callCtx)
	assert.Assert(t, err != nil)
}
