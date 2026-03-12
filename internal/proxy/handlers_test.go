package proxy

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/T4cceptor/centian/internal/processor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func newHandlerTestCallContext() *ToolCallContext {
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "tool-current",
			Arguments: json.RawMessage(`{"k":"v"}`),
		},
	}

	return &ToolCallContext{
		proxy:              &MCPProxy{},
		upstreamSession:    &UpstreamSession{id: "sess-1", downstreamConns: map[string]DownstreamConnectionInterface{}},
		originalServerName: "server-original",
		originalRequest:    deepCloneRequest(req),
		request:            req,
		result:             &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "result-current"}}},
		originalResult:     &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "result-original"}}},
		event:              common.NewMCPRequestEvent("stdio").WithRequestID("req-1").WithSessionID("sess-1"),
		routingContext:     &common.RoutingContext{ServerName: "server-current"},
	}
}

func TestDefaultPayloadHandlerAttachPart(t *testing.T) {
	callCtx := newHandlerTestCallContext()
	input := &processor.DataContext{}

	handler := &DefaultPayloadHandler{}
	handler.AttachPart(callCtx, input)

	assert.Assert(t, input.Payload != nil)
	assert.Assert(t, input.Payload.Request == callCtx.GetRequest())
	assert.Assert(t, input.Payload.OriginalRequest == callCtx.GetOriginalRequest())
	assert.Assert(t, input.Payload.Result == callCtx.GetResult())
	assert.Assert(t, input.Payload.OriginalResult == callCtx.GetOriginalResult())
}

func TestDefaultPayloadHandlerApply(t *testing.T) {
	t.Run("no payload does nothing", func(t *testing.T) {
		callCtx := newHandlerTestCallContext()
		originalName := callCtx.GetRequest().Params.Name

		handler := &DefaultPayloadHandler{}
		err := handler.Apply(callCtx, &processor.DataContext{})

		assert.NilError(t, err)
		assert.Equal(t, callCtx.GetRequest().Params.Name, originalName)
	})

	t.Run("updates request params and result", func(t *testing.T) {
		callCtx := newHandlerTestCallContext()
		newResult := &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "result-updated"}},
		}

		output := &processor.DataContext{
			Payload: &processor.PayloadPart{
				Request: &mcp.CallToolRequest{
					Params: &mcp.CallToolParamsRaw{
						Name:      "tool-updated",
						Arguments: json.RawMessage(`{"new":"value"}`),
					},
				},
				Result: newResult,
			},
		}

		handler := &DefaultPayloadHandler{}
		err := handler.Apply(callCtx, output)

		assert.NilError(t, err)
		assert.Equal(t, callCtx.GetRequest().Params.Name, "tool-updated")
		assert.Equal(t, string(callCtx.GetRequest().Params.Arguments), `{"new":"value"}`)
		assert.Assert(t, callCtx.GetResult() == newResult)
	})

	t.Run("nil current request does not fail", func(t *testing.T) {
		callCtx := newHandlerTestCallContext()
		callCtx.request = nil

		output := &processor.DataContext{
			Payload: &processor.PayloadPart{
				Request: &mcp.CallToolRequest{
					Params: &mcp.CallToolParamsRaw{Name: "ignored"},
				},
			},
		}

		handler := &DefaultPayloadHandler{}
		err := handler.Apply(callCtx, output)
		assert.NilError(t, err)
	})
}

func TestDefaultMetaHandlerAttachPartAndApply(t *testing.T) {
	callCtx := newHandlerTestCallContext()
	input := &processor.DataContext{}
	handler := &DefaultMetaHandler{}

	handler.AttachPart(callCtx, input)
	assert.Assert(t, input.Event == callCtx.GetEventInfo())

	originalEvent := callCtx.GetEventInfo()
	err := handler.Apply(callCtx, &processor.DataContext{})
	assert.NilError(t, err)
	assert.Assert(t, callCtx.GetEventInfo() == originalEvent)

	newEvent := common.NewMCPRequestEvent("http")
	err = handler.Apply(callCtx, &processor.DataContext{Event: newEvent})
	assert.NilError(t, err)
	assert.Assert(t, callCtx.GetEventInfo() == newEvent)
}

func TestDefaultRoutingHandlerAttachPart(t *testing.T) {
	handler := &DefaultRoutingHandler{}

	t.Run("nil input or call context is ignored", func(t *testing.T) {
		var nilCallCtx CallContext
		handler.AttachPart(nilCallCtx, &processor.DataContext{})
		handler.AttachPart(newHandlerTestCallContext(), nil)
	})

	t.Run("attaches routing data", func(t *testing.T) {
		callCtx := newHandlerTestCallContext()
		input := &processor.DataContext{}

		handler.AttachPart(callCtx, input)

		assert.Assert(t, input.Routing != nil)
		assert.Equal(t, input.Routing.ServerName, "server-current")
		assert.Equal(t, input.Routing.ToolName, "tool-current")
		assert.Equal(t, input.Routing.OriginalServerName, "server-original")
		assert.Equal(t, input.Routing.OriginalToolname, "tool-current")
	})
}

func TestDefaultRoutingHandlerApply(t *testing.T) {
	handler := &DefaultRoutingHandler{}

	t.Run("nil routing does nothing", func(t *testing.T) {
		callCtx := newHandlerTestCallContext()
		err := handler.Apply(callCtx, &processor.DataContext{})
		assert.NilError(t, err)
		assert.Equal(t, callCtx.GetServerName(), "server-current")
	})

	t.Run("updates server and tool name", func(t *testing.T) {
		callCtx := newHandlerTestCallContext()
		callCtx.request.Params = nil

		output := &processor.DataContext{
			Routing: &processor.RoutingPart{
				ServerName: "server-updated",
				ToolName:   "tool-updated",
			},
		}

		err := handler.Apply(callCtx, output)
		assert.NilError(t, err)
		assert.Equal(t, callCtx.GetServerName(), "server-updated")
		assert.Assert(t, callCtx.GetRequest().Params != nil)
		assert.Equal(t, callCtx.GetRequest().Params.Name, "tool-updated")
	})

	t.Run("fails when tool name is set but request is nil", func(t *testing.T) {
		callCtx := newHandlerTestCallContext()
		callCtx.request = nil

		err := handler.Apply(callCtx, &processor.DataContext{
			Routing: &processor.RoutingPart{
				ToolName: "tool-updated",
			},
		})

		assert.Assert(t, err != nil)
	})
}

func TestDefaultLogHandlerToLogEntry(t *testing.T) {
	handler := &DefaultLogHandler{}

	t.Run("request entry includes routing and tool call arguments", func(t *testing.T) {
		callCtx := newHandlerTestCallContext()
		callCtx.result = nil
		callCtx.routingContext.Transport = common.StdioTransport
		callCtx.SetDirection(common.DirectionClientToServer)
		callCtx.SetMessageType(common.MessageTypeRequest)
		callCtx.SetStatus(200)

		entry := handler.ToLogEntry(callCtx)
		assert.Assert(t, entry != nil)
		assert.Equal(t, entry.Transport, string(common.StdioTransport))
		assert.Equal(t, entry.Direction, common.DirectionClientToServer)
		assert.Equal(t, entry.MessageType, common.MessageTypeRequest)
		assert.Equal(t, entry.RequestID, "req-1")
		assert.Equal(t, entry.SessionID, "sess-1")
		assert.Equal(t, entry.Status, 200)
		assert.Assert(t, entry.Success)
		assert.Assert(t, entry.ToolCall != nil)
		assert.Equal(t, entry.ToolCall.Name, "tool-current")
		assert.Equal(t, entry.ToolCall.OriginalName, "tool-current")
		assert.Equal(t, string(entry.ToolCall.Arguments), `{"k":"v"}`)
		assert.Equal(t, entry.Routing.ServerName, "server-current")
	})

	t.Run("response entry includes tool result and error state", func(t *testing.T) {
		callCtx := newHandlerTestCallContext()
		callCtx.result = &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "failed"}},
		}
		callCtx.SetDirection(common.DirectionServerToClient)
		callCtx.SetMessageType(common.MessageTypeResponse)
		callCtx.SetStatus(500)

		entry := handler.ToLogEntry(callCtx)
		assert.Assert(t, entry != nil)
		assert.Equal(t, entry.Direction, common.DirectionServerToClient)
		assert.Equal(t, entry.MessageType, common.MessageTypeResponse)
		assert.Equal(t, entry.Status, 500)
		assert.Assert(t, !entry.Success)
		assert.Assert(t, entry.ToolCall != nil)
		assert.Assert(t, entry.ToolCall.IsError)
		assert.Assert(t, len(entry.ToolCall.Result) > 0)
	})
}

func TestDefaultLogHandlerGetTransport(t *testing.T) {
	handler := &DefaultLogHandler{}
	callCtx := newHandlerTestCallContext()

	callCtx.routingContext = nil
	assert.Equal(t, handler.getTransport(callCtx), "unknown")

	callCtx.routingContext = &common.RoutingContext{Transport: common.HTTPTransport}
	assert.Equal(t, handler.getTransport(callCtx), string(common.HTTPTransport))
}

func TestDefaultLogHandlerLog(t *testing.T) {
	t.Setenv("CENTIAN_LOG_DIR", t.TempDir())
	logger, err := logging.NewLogger()
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = logger.Close()
	})

	handler := NewDefaultLogHandler(logger)
	callCtx := newHandlerTestCallContext()
	callCtx.SetDirection(common.DirectionClientToServer)
	callCtx.SetMessageType(common.MessageTypeRequest)

	err = handler.Log(callCtx)
	assert.NilError(t, err)

	data, err := os.ReadFile(logger.GetLogPath())
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(data), `"request_id":"req-1"`))
}
