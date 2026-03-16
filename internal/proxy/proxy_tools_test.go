package proxy

import (
	"context"
	"errors"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func connectUpstreamCapabilityClient(
	t *testing.T,
	session *UpstreamSession,
	options *mcp.ClientOptions,
) func() {
	t.Helper()

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := session.upstreamServer.Connect(ctx, serverTransport, nil)
	assert.NilError(t, err)

	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1.0.0"}, options)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	assert.NilError(t, err)

	// Tests use the existing empty-ID fallback to resolve the live server session.
	session.id = ""

	return func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	}
}

func TestSyncAvailableToolsRemovesStaleTools(t *testing.T) {
	proxy := &CentianEndpoint{
		name:            "gateway",
		endpoint:        "/mcp/gateway",
		downstreamPools: make(map[string]*DownstreamSessionPool),
		server:          &CentianServer{Config: &config.GlobalConfig{Version: "1.0.0"}},
	}

	downstream := &MockDownstreamConnection{
		serverName: "server-1",
		Status:     StatusConnected,
		tools: []*mcp.Tool{
			{Name: "tool-a", Description: "a", InputSchema: map[string]any{"type": "object"}},
		},
	}

	session := &UpstreamSession{
		id:                   "session-1",
		registeredTools:      make(map[string]struct{}),
		downstreamConns:      map[string]DownstreamConnectionInterface{"server-1": downstream},
		downstreamSessionKey: "pool-1",
	}
	session.upstreamServer = proxy.newUpstreamServer(session)

	proxy.syncAvailableTools(session)
	assert.Equal(t, len(session.registeredTools), 1)

	downstream.tools = nil
	proxy.syncAvailableTools(session)

	assert.Equal(t, len(session.registeredTools), 0)
}

func TestGetSyncedSessionReturnsServerSessionFromContext(t *testing.T) {
	proxy := newLoggingTestProxy()
	session := newLoggingTestSession(proxy, "session-1")
	cleanup := connectUpstreamCapabilityClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	ctx := WithCallContext(context.Background(), &ToolCallContext{
		proxy:           proxy,
		upstreamSession: session,
	})

	serverSession, err := proxy.getSyncedSession(ctx)

	assert.NilError(t, err)
	assert.Assert(t, serverSession != nil)
}

func TestForwardSamplingRequestUsesUpstreamSession(t *testing.T) {
	proxy := newLoggingTestProxy()
	session := newLoggingTestSession(proxy, "session-1")
	cleanup := connectUpstreamCapabilityClient(t, session, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{Sampling: &mcp.SamplingCapabilities{}},
		CreateMessageHandler: func(_ context.Context, req *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			assert.Equal(t, req.Params.MaxTokens, int64(16))
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: "sampled"},
				Model:   "test-model",
				Role:    "assistant",
			}, nil
		},
	})
	defer cleanup()

	ctx := WithCallContext(context.Background(), &ToolCallContext{
		proxy:           proxy,
		upstreamSession: session,
	})

	result, err := proxy.forwardSamplingRequest(ctx, &mcp.CreateMessageRequest{
		Params: &mcp.CreateMessageParams{
			MaxTokens: 16,
			Messages: []*mcp.SamplingMessage{
				{Role: "user", Content: &mcp.TextContent{Text: "hello"}},
			},
		},
	})

	assert.NilError(t, err)
	assert.Equal(t, result.Model, "test-model")
	assert.Equal(t, result.Content.(*mcp.TextContent).Text, "sampled")
}

func TestForwardElicitationRequestUsesUpstreamSession(t *testing.T) {
	proxy := newLoggingTestProxy()
	session := newLoggingTestSession(proxy, "session-1")
	cleanup := connectUpstreamCapabilityClient(t, session, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{
			Elicitation: &mcp.ElicitationCapabilities{Form: &mcp.FormElicitationCapabilities{}},
		},
		ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			assert.Equal(t, req.Params.Message, "Proceed?")
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	})
	defer cleanup()

	ctx := WithCallContext(context.Background(), &ToolCallContext{
		proxy:           proxy,
		upstreamSession: session,
	})

	result, err := proxy.forwardElicitationRequest(ctx, &mcp.ElicitRequest{
		Params: &mcp.ElicitParams{
			Mode:            "form",
			Message:         "Proceed?",
			RequestedSchema: map[string]any{"type": "object"},
		},
	})

	assert.NilError(t, err)
	assert.Equal(t, result.Action, "accept")
}

func TestUpstreamSessionFromContextErrors(t *testing.T) {
	proxy := newLoggingTestProxy()

	_, _, err := proxy.upstreamSessionFromContext(context.Background())
	assert.ErrorContains(t, err, "missing call context")

	ctx := WithCallContext(context.Background(), &mockCallContext{})
	_, _, err = proxy.upstreamSessionFromContext(ctx)
	assert.ErrorContains(t, err, "missing upstream session")

	session := &UpstreamSession{}
	ctx = WithCallContext(context.Background(), &ToolCallContext{upstreamSession: session})
	_, _, err = proxy.upstreamSessionFromContext(ctx)
	assert.ErrorContains(t, err, "upstream server session not available")
}

func TestUpstreamSessionFromContextReturnsSessionAndServerSession(t *testing.T) {
	proxy := newLoggingTestProxy()
	session := newLoggingTestSession(proxy, "session-1")
	cleanup := connectUpstreamCapabilityClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	ctx := WithCallContext(context.Background(), &ToolCallContext{
		proxy:           proxy,
		upstreamSession: session,
	})

	gotSession, serverSession, err := proxy.upstreamSessionFromContext(ctx)

	assert.NilError(t, err)
	assert.Assert(t, gotSession == session)
	assert.Assert(t, serverSession != nil)
}

func TestForwardSamplingRequestReturnsMissingContextError(t *testing.T) {
	proxy := newLoggingTestProxy()

	result, err := proxy.forwardSamplingRequest(context.Background(), &mcp.CreateMessageRequest{
		Params: &mcp.CreateMessageParams{},
	})

	assert.Assert(t, result == nil)
	assert.ErrorContains(t, err, "missing call context")
}

func TestForwardElicitationRequestReturnsMissingContextError(t *testing.T) {
	proxy := newLoggingTestProxy()

	result, err := proxy.forwardElicitationRequest(context.Background(), &mcp.ElicitRequest{
		Params: &mcp.ElicitParams{},
	})

	assert.Assert(t, result == nil)
	assert.ErrorContains(t, err, "missing call context")
}

func TestForwardSamplingRequestPropagatesClientError(t *testing.T) {
	proxy := newLoggingTestProxy()
	session := newLoggingTestSession(proxy, "session-1")
	cleanup := connectUpstreamCapabilityClient(t, session, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{Sampling: &mcp.SamplingCapabilities{}},
		CreateMessageHandler: func(_ context.Context, _ *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			return nil, errors.New("client rejected sampling")
		},
	})
	defer cleanup()

	ctx := WithCallContext(context.Background(), &ToolCallContext{
		proxy:           proxy,
		upstreamSession: session,
	})

	result, err := proxy.forwardSamplingRequest(ctx, &mcp.CreateMessageRequest{
		Params: &mcp.CreateMessageParams{},
	})

	assert.Assert(t, result == nil)
	assert.ErrorContains(t, err, "client rejected sampling")
}
