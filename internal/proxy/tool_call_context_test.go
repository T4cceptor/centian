package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

type noopLogHandler struct{}

func (l *noopLogHandler) ToLogEntry(_ CallContext) *common.MCPEvent {
	return nil
}

func (l *noopLogHandler) Log(_ CallContext) error {
	return nil
}

func TestBuildRoutingContext(t *testing.T) {
	// Given: a shared proxy and session containers.
	proxy := &MCPProxy{name: "gateway-a", endpoint: "/mcp/gateway-a"}

	t.Run("http transport", func(t *testing.T) {
		session := &UpstreamSession{
			downstreamConns: map[string]DownstreamConnectionInterface{
				"srv": &MockDownstreamConnection{
					cfg: &config.MCPServerConfig{URL: "https://example.com/mcp"},
				},
			},
		}

		// When: building routing context.
		rc := buildRoutingContext(proxy, session, "srv")

		// Then: transport and URL are populated.
		assert.Equal(t, rc.Gateway, "gateway-a")
		assert.Equal(t, rc.Endpoint, "/mcp/gateway-a")
		assert.Equal(t, rc.ServerName, "srv")
		assert.Equal(t, rc.Transport, common.HTTPTransport)
		assert.Equal(t, rc.DownstreamURL, "https://example.com/mcp")
	})

	t.Run("stdio transport", func(t *testing.T) {
		session := &UpstreamSession{
			downstreamConns: map[string]DownstreamConnectionInterface{
				"srv": &MockDownstreamConnection{
					cfg: &config.MCPServerConfig{
						Command: "python3",
						Args:    []string{"processor.py"},
					},
				},
			},
		}

		// When: building routing context.
		rc := buildRoutingContext(proxy, session, "srv")

		// Then: transport and command details are populated.
		assert.Equal(t, rc.Transport, common.StdioTransport)
		assert.Equal(t, rc.DownstreamCommand, "python3")
		assert.DeepEqual(t, rc.Args, []string{"processor.py"})
	})
}

func TestNewToolCallContext(t *testing.T) {
	request := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "demo_tool",
			Arguments: json.RawMessage(`{"k":"v"}`),
		},
	}

	t.Run("fails when proxy server is nil", func(t *testing.T) {
		proxy := &MCPProxy{name: "gw", endpoint: "/mcp/gw"}
		session := &UpstreamSession{id: "sess-1", downstreamConns: map[string]DownstreamConnectionInterface{}}

		// When: constructing context without server.
		_, err := NewToolCallContext(context.Background(), proxy, session, "srv", request)

		// Then: construction fails.
		assert.Assert(t, err != nil)
	})

	t.Run("fails when logger is nil", func(t *testing.T) {
		proxy := &MCPProxy{
			name:     "gw",
			endpoint: "/mcp/gw",
			server:   &CentianProxy{ServerID: "server-id", Logger: nil},
		}
		session := &UpstreamSession{id: "sess-1", downstreamConns: map[string]DownstreamConnectionInterface{}}

		// When: constructing context without logger.
		_, err := NewToolCallContext(context.Background(), proxy, session, "srv", request)

		// Then: construction fails.
		assert.Assert(t, err != nil)
	})

	t.Run("initializes fields and handlers", func(t *testing.T) {
		t.Setenv("CENTIAN_LOG_DIR", t.TempDir())
		logger, err := logging.NewLogger()
		assert.NilError(t, err)
		t.Cleanup(func() {
			_ = logger.Close()
		})

		proxy := &MCPProxy{
			name:     "gw",
			endpoint: "/mcp/gw",
			server: &CentianProxy{
				ServerID: "server-id",
				Logger:   logger,
			},
		}
		session := &UpstreamSession{
			id: "sess-1",
			downstreamConns: map[string]DownstreamConnectionInterface{
				"srv": &MockDownstreamConnection{
					cfg: &config.MCPServerConfig{URL: "https://example.com/mcp"},
				},
			},
		}

		// When: constructing a valid tool call context.
		callCtxIface, err := NewToolCallContext(context.Background(), proxy, session, "srv", request)
		assert.NilError(t, err)

		callCtx, ok := callCtxIface.(*ToolCallContext)
		assert.Assert(t, ok)

		// Then: core context data is initialized.
		assert.Equal(t, callCtx.GetOriginalServerName(), "srv")
		assert.Equal(t, callCtx.GetSessionID(), "sess-1")
		assert.Equal(t, callCtx.GetDirection(), common.DirectionClientToServer)
		assert.Equal(t, callCtx.GetMessageType(), common.MessageTypeRequest)
		assert.Assert(t, callCtx.GetRequestID() != "")
		assert.Assert(t, callCtx.GetLogHandler() != nil)

		// And: handlers are registered.
		_, hasPayload := callCtx.GetHandler("payload")
		_, hasMeta := callCtx.GetHandler("meta")
		_, hasRouting := callCtx.GetHandler("routing")
		assert.Assert(t, hasPayload)
		assert.Assert(t, hasMeta)
		assert.Assert(t, hasRouting)

		// And: original request was deep-cloned.
		assert.Assert(t, callCtx.GetOriginalRequest() != callCtx.GetRequest())
		callCtx.GetRequest().Params.Arguments = json.RawMessage(`{"k":"changed"}`)
		assert.Equal(t, string(callCtx.GetOriginalRequest().Params.Arguments), `{"k":"v"}`)
	})
}

func TestToolCallContextSendRequest(t *testing.T) {
	makeContext := func(conn DownstreamConnectionInterface, args json.RawMessage) *ToolCallContext {
		return &ToolCallContext{
			proxy:              &MCPProxy{},
			originalServerName: "orig-srv",
			upstreamSession: &UpstreamSession{
				downstreamConns: map[string]DownstreamConnectionInterface{
					"srv": conn,
				},
			},
			routingContext: &common.RoutingLog{ServerName: "srv"},
			request: &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Name:      "tool_name",
					Arguments: args,
				},
			},
			event: common.NewMCPRequestEvent("stdio"),
		}
	}

	t.Run("fails when server is missing", func(t *testing.T) {
		toolCtx := &ToolCallContext{
			proxy:              &MCPProxy{},
			originalServerName: "orig-srv",
			upstreamSession: &UpstreamSession{
				downstreamConns: map[string]DownstreamConnectionInterface{},
			},
			routingContext: &common.RoutingLog{ServerName: "srv"},
			request: &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{Name: "tool_name", Arguments: json.RawMessage(`{}`)},
			},
			event: common.NewMCPRequestEvent("stdio"),
		}

		err := toolCtx.SendRequest(context.Background())
		assert.Assert(t, err != nil)
	})

	t.Run("fails when connection is not connected", func(t *testing.T) {
		toolCtx := makeContext(&MockDownstreamConnection{
			cfg:       &config.MCPServerConfig{Command: "python3"},
			connected: false,
		}, json.RawMessage(`{}`))

		err := toolCtx.SendRequest(context.Background())
		assert.Assert(t, err != nil)
	})

	t.Run("fails on invalid request arguments", func(t *testing.T) {
		toolCtx := makeContext(&MockDownstreamConnection{
			cfg:       &config.MCPServerConfig{Command: "python3"},
			connected: true,
		}, json.RawMessage(`{invalid}`))

		err := toolCtx.SendRequest(context.Background())
		assert.Assert(t, err != nil)
	})

	t.Run("returns downstream call error", func(t *testing.T) {
		toolCtx := makeContext(&MockDownstreamConnection{
			cfg:           &config.MCPServerConfig{Command: "python3"},
			connected:     true,
			ErrorToReturn: errors.New("downstream failed"),
		}, json.RawMessage(`{"a":1}`))

		err := toolCtx.SendRequest(context.Background())
		assert.Assert(t, err != nil)
	})

	t.Run("successfully calls downstream and sets result", func(t *testing.T) {
		conn := &MockDownstreamConnection{
			cfg:       &config.MCPServerConfig{Command: "python3"},
			connected: true,
			ResultToReturn: &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
			},
		}
		toolCtx := makeContext(conn, json.RawMessage(`{"a":1}`))

		err := toolCtx.SendRequest(context.Background())
		assert.NilError(t, err)
		assert.Equal(t, conn.CapturedToolName, "tool_name")
		assert.Equal(t, conn.CapturedArgs["a"], float64(1))
		assert.Assert(t, toolCtx.HasResult())
		assert.Assert(t, toolCtx.GetResult() != nil)
		assert.Assert(t, toolCtx.GetOriginalResult() == toolCtx.GetResult())
	})
}

func TestToolCallContextAccessors(t *testing.T) {
	event := common.NewMCPRequestEvent("stdio")
	toolCtx := &ToolCallContext{
		proxy: &MCPProxy{
			name:              "gw",
			endpoint:          "/mcp/gw",
			isAggregatedProxy: true,
		},
		upstreamSession: &UpstreamSession{
			id:              "sess-1",
			downstreamConns: map[string]DownstreamConnectionInterface{},
		},
		request: &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Name:      "srv___tool_name",
				Arguments: json.RawMessage(`{"x":1}`),
			},
		},
		originalRequest: &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{Name: "orig_tool"},
		},
		originalServerName: "orig-srv",
		event:              event,
	}

	// Given: no routing context yet.
	assert.Equal(t, toolCtx.GetServerName(), "")

	// When: setting server name.
	toolCtx.SetServerName("new-srv")

	// Then: routing context is created and server name is set.
	assert.Assert(t, toolCtx.GetRoutingContext() != nil)
	assert.Equal(t, toolCtx.GetServerName(), "new-srv")
	assert.Equal(t, toolCtx.GetRoutingContext().Gateway, "gw")

	// And: raw and downstream tool names are both available.
	toolCtx.request.Params.Name = "new-srv___tool_name"
	assert.Equal(t, toolCtx.GetRawToolName(), "new-srv___tool_name")
	assert.Equal(t, toolCtx.GetToolName(), "new-srv___tool_name")
	assert.Equal(t, toolCtx.GetDownstreamToolName(), "tool_name")
	assert.Equal(t, toolCtx.GetOriginalToolName(), "orig_tool")

	// And: malformed aggregated names fall back to the raw tool name.
	toolCtx.request.Params.Name = "malformed"
	assert.Equal(t, toolCtx.GetDownstreamToolName(), "malformed")

	// And: mismatched namespaces are not used for downstream dispatch.
	toolCtx.request.Params.Name = "other___tool_name"
	assert.Equal(t, toolCtx.GetDownstreamToolName(), "other___tool_name")

	// And: nil request returns empty.
	toolCtx.request = nil
	assert.Equal(t, toolCtx.GetRawToolName(), "")
	assert.Equal(t, toolCtx.GetDownstreamToolName(), "")

	// And: status/error accessors round-trip values.
	toolCtx.SetStatus(422)
	toolCtx.SetError("invalid")
	assert.Equal(t, toolCtx.GetStatus(), 422)
	assert.Equal(t, toolCtx.GetError(), "invalid")

	// And: direction and message type can be modified.
	toolCtx.SetDirection(common.DirectionServerToClient)
	toolCtx.SetMessageType(common.MessageTypeResponse)
	assert.Equal(t, toolCtx.GetDirection(), common.DirectionServerToClient)
	assert.Equal(t, toolCtx.GetMessageType(), common.MessageTypeResponse)

	// And: event can be replaced.
	updatedEvent := common.NewMCPRequestEvent("http")
	toolCtx.SetEventInfo(updatedEvent)
	assert.Assert(t, toolCtx.GetEventInfo() == updatedEvent)
}

func TestToolCallContextSetResultAndHandlers(t *testing.T) {
	toolCtx := &ToolCallContext{
		event: common.NewMCPRequestEvent("stdio"),
	}

	// Given: first and second results.
	firstResult := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "first"}}}
	secondResult := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "second"}}}

	// When: setting result multiple times.
	toolCtx.SetResult(firstResult)
	toolCtx.SetResult(secondResult)

	// Then: original result remains first and current result is latest.
	assert.Assert(t, toolCtx.HasResult())
	assert.Assert(t, toolCtx.GetOriginalResult() == firstResult)
	assert.Assert(t, toolCtx.GetResult() == secondResult)

	// And: handler lookup is false before registration.
	_, found := toolCtx.GetHandler("payload")
	assert.Assert(t, !found)

	// When: setting handler and log handler.
	toolCtx.SetHandler("payload", &DefaultPayloadHandler{})
	toolCtx.SetLogHandler(&noopLogHandler{})

	// Then: both can be retrieved.
	handler, found := toolCtx.GetHandler("payload")
	assert.Assert(t, found)
	assert.Assert(t, handler != nil)
	assert.Assert(t, toolCtx.GetLogHandler() != nil)
}

func TestToolCallContextSessionAndOriginalAccessors(t *testing.T) {
	toolCtx := &ToolCallContext{
		event: common.NewMCPRequestEvent("stdio").WithRequestID("req-1"),
	}

	// Given: nil session and nil original request.
	assert.Equal(t, toolCtx.GetSessionID(), "")
	assert.Equal(t, toolCtx.GetOriginalToolName(), "")
	assert.Equal(t, toolCtx.GetRequestID(), "req-1")
	assert.Equal(t, toolCtx.GetOriginalServerName(), "")
}

func TestToolCallContextRewriteToolName(t *testing.T) {
	t.Run("rewrites plain tool names in aggregated mode", func(t *testing.T) {
		toolCtx := &ToolCallContext{
			proxy: &MCPProxy{
				isAggregatedProxy: true,
			},
			routingContext: &common.RoutingLog{ServerName: "srv"},
			request: &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{Name: "srv___tool_name"},
			},
		}

		err := toolCtx.RewriteToolName("tool_updated")

		assert.NilError(t, err)
		assert.Equal(t, toolCtx.GetRawToolName(), "srv___tool_updated")
	})

	t.Run("rejects mismatched namespaced tool names in aggregated mode", func(t *testing.T) {
		toolCtx := &ToolCallContext{
			proxy: &MCPProxy{
				isAggregatedProxy: true,
			},
			routingContext: &common.RoutingLog{ServerName: "srv"},
			request: &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{Name: "srv___tool_name"},
			},
		}

		err := toolCtx.RewriteToolName("other___tool_updated")

		assert.Assert(t, errors.Is(err, ErrUnexpectedToolNamespace))
	})
}
