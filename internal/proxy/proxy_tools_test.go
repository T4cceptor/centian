package proxy

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/T4cceptor/centian/internal/persistence"
	"github.com/T4cceptor/centian/internal/processor"
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

func newToolSurfaceTestProxy(t *testing.T, surfaceProcessor *ToolSurfaceProcessingController) (*CentianEndpoint, *UpstreamSession, *MockDownstreamConnection) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	logger, err := logging.NewLogger()
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = logger.Close()
	})

	proxy := &CentianEndpoint{
		name:            "gateway",
		endpoint:        "/mcp/gateway",
		downstreamPools: make(map[string]*DownstreamSessionPool),
		server: &CentianServer{
			ServerID: "server-1",
			Config:   &config.GlobalConfig{Version: "1.0.0"},
			Logger:   logger,
		},
		config:           &config.GatewayConfig{},
		surfaceProcessor: surfaceProcessor,
	}
	downstream := &MockDownstreamConnection{
		serverName: "server-1",
		Status:     StatusConnected,
		cfg:        &config.MCPServerConfig{Command: "node"},
		tools: []*mcp.Tool{
			{Name: "read_file", Description: "Read a file", InputSchema: map[string]any{"type": "object"}},
			{Name: "write_file", Description: "Write a file", InputSchema: map[string]any{"type": "object"}},
		},
		ResultToReturn: &mcp.CallToolResult{},
	}
	session := &UpstreamSession{
		id:                         "session-1",
		registeredTools:            make(map[string]struct{}),
		registeredToolFingerprints: make(map[string]string),
		toolRoutes:                 make(map[string]toolRoute),
		downstreamConns:            map[string]DownstreamConnectionInterface{"server-1": downstream},
		downstreamSessionKey:       "pool-1",
		authData:                   &AuthData{},
	}
	session.upstreamServer = proxy.newUpstreamServer(session)
	return proxy, session, downstream
}

func surfaceMockProcessor(fn func(input *processor.DataContext) (*processor.DataContext, error)) *mockProcessor {
	return &mockProcessor{
		cfg: &config.ProcessorConfig{
			Name:    "surface",
			Type:    "cli",
			Enabled: true,
			Parts:   []string{"tool_surface", "annotations"},
		},
		processFn: fn,
	}
}

func TestToolSurfaceProcessorRenamesAndRoutesOriginalTool(t *testing.T) {
	description := "Read from the approved filesystem surface."
	surface := surfaceMockProcessor(func(input *processor.DataContext) (*processor.DataContext, error) {
		assert.Assert(t, input.ToolSurface != nil)
		return &processor.DataContext{
			ToolSurface: &processor.ToolSurfacePart{
				Decisions: []processor.ToolSurfaceDecision{
					{
						ToolName:    "read_file",
						Action:      "modify",
						ExposedName: "fs_read",
						Description: &description,
					},
				},
			},
		}, nil
	})
	proxy, session, downstream := newToolSurfaceTestProxy(t, &ToolSurfaceProcessingController{
		processors: []processor.ProcessorInterface{surface},
	})

	assert.NilError(t, proxy.syncAvailableToolsChecked(session))
	clientSession, cleanup := connectAuthToolClient(t, session, nil)
	defer cleanup()

	tools := listToolsByName(t, clientSession)
	assert.Assert(t, tools["fs_read"] != nil)
	assert.Equal(t, tools["fs_read"].Description, description)
	assert.Assert(t, tools["read_file"] == nil)

	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "fs_read"})
	assert.NilError(t, err)
	assert.Equal(t, downstream.CapturedToolName, "read_file")
}

func TestToolSurfaceProcessorHidesAndRemovesStaleTool(t *testing.T) {
	surface := surfaceMockProcessor(func(_ *processor.DataContext) (*processor.DataContext, error) {
		return &processor.DataContext{
			ToolSurface: &processor.ToolSurfacePart{
				Decisions: []processor.ToolSurfaceDecision{{ToolName: "write_file", Action: "hide"}},
			},
		}, nil
	})
	proxy, session, _ := newToolSurfaceTestProxy(t, &ToolSurfaceProcessingController{
		processors: []processor.ProcessorInterface{surface},
	})

	assert.NilError(t, proxy.syncAvailableToolsChecked(session))
	_, exists := session.registeredTools["write_file"]
	assert.Assert(t, !exists)
	_, exists = session.toolRoutes["write_file"]
	assert.Assert(t, !exists)

	clientSession, cleanup := connectAuthToolClient(t, session, nil)
	defer cleanup()
	tools := listToolsByName(t, clientSession)
	assert.Assert(t, tools["write_file"] == nil)
}

func TestToolSurfaceProcessorReregistersWhenDefinitionChanges(t *testing.T) {
	description := "first"
	surface := surfaceMockProcessor(func(_ *processor.DataContext) (*processor.DataContext, error) {
		return &processor.DataContext{
			ToolSurface: &processor.ToolSurfacePart{
				Decisions: []processor.ToolSurfaceDecision{
					{ToolName: "read_file", Action: "modify", Description: &description},
				},
			},
		}, nil
	})
	proxy, session, _ := newToolSurfaceTestProxy(t, &ToolSurfaceProcessingController{
		processors: []processor.ProcessorInterface{surface},
	})

	assert.NilError(t, proxy.syncAvailableToolsChecked(session))
	firstFingerprint := session.registeredToolFingerprints["read_file"]
	description = "second"
	assert.NilError(t, proxy.syncAvailableToolsChecked(session))

	assert.Assert(t, firstFingerprint != session.registeredToolFingerprints["read_file"])
	clientSession, cleanup := connectAuthToolClient(t, session, nil)
	defer cleanup()
	tools := listToolsByName(t, clientSession)
	assert.Equal(t, tools["read_file"].Description, "second")
}

func TestToolSurfaceOptionalProcessorDuplicateFallsBackToDefault(t *testing.T) {
	surface := surfaceMockProcessor(func(_ *processor.DataContext) (*processor.DataContext, error) {
		return &processor.DataContext{
			ToolSurface: &processor.ToolSurfacePart{
				Decisions: []processor.ToolSurfaceDecision{
					{ToolName: "read_file", Action: "modify", ExposedName: "same"},
					{ToolName: "write_file", Action: "modify", ExposedName: "same"},
				},
			},
		}, nil
	})
	surface.cfg.Required = false
	proxy, session, _ := newToolSurfaceTestProxy(t, &ToolSurfaceProcessingController{
		processors: []processor.ProcessorInterface{surface},
	})

	assert.NilError(t, proxy.syncAvailableToolsChecked(session))
	clientSession, cleanup := connectAuthToolClient(t, session, nil)
	defer cleanup()
	tools := listToolsByName(t, clientSession)
	assert.Assert(t, tools["read_file"] != nil)
	assert.Assert(t, tools["write_file"] != nil)
	assert.Assert(t, tools["same"] == nil)
}

func TestToolSurfaceRequiredProcessorDuplicateFailsSync(t *testing.T) {
	surface := surfaceMockProcessor(func(_ *processor.DataContext) (*processor.DataContext, error) {
		return &processor.DataContext{
			ToolSurface: &processor.ToolSurfacePart{
				Decisions: []processor.ToolSurfaceDecision{
					{ToolName: "read_file", Action: "modify", ExposedName: "same"},
					{ToolName: "write_file", Action: "modify", ExposedName: "same"},
				},
			},
		}, nil
	})
	surface.cfg.Required = true
	proxy, session, _ := newToolSurfaceTestProxy(t, &ToolSurfaceProcessingController{
		processors: []processor.ProcessorInterface{surface},
	})

	err := proxy.syncAvailableToolsChecked(session)
	assert.Assert(t, err != nil)
	assert.Equal(t, len(session.registeredTools), 0)
}

func TestToolSurfaceAnnotationsPersistAsSurfaceEvent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	logger, err := logging.NewLogger()
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = logger.Close()
	})
	store, err := persistence.NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})
	logger.SetActionEventStore(store)

	surface := surfaceMockProcessor(func(_ *processor.DataContext) (*processor.DataContext, error) {
		return &processor.DataContext{
			Annotations: &processor.AnnotationPart{
				Reports: []processor.Report{
					{
						Type:      "governance_events",
						Processor: "surface",
						Action:    "modified",
						Category:  "security",
						Severity:  "medium",
						Message:   "Tool description was rewritten.",
					},
				},
			},
		}, nil
	})
	proxy, session, _ := newToolSurfaceTestProxy(t, &ToolSurfaceProcessingController{
		processors: []processor.ProcessorInterface{surface},
	})
	proxy.server.Logger = logger

	assert.NilError(t, proxy.syncAvailableToolsChecked(session))

	events, err := store.ActionEvents()
	assert.NilError(t, err)
	assert.Equal(t, len(events), 1)
	page, err := store.ListEvents(context.Background(), &persistence.EventListFilter{})
	assert.NilError(t, err)
	assert.Equal(t, len(page.Items), 1)
	assert.Equal(t, len(page.Items[0].Annotations), 1)
	assert.Equal(t, page.Items[0].Annotations[0].Message, "Tool description was rewritten.")
}

func TestToolDefinitionFingerprintChangesForSurfaceFields(t *testing.T) {
	base := &mcp.Tool{
		Name:        "read_file",
		Description: "Read a file",
		InputSchema: map[string]any{"type": "object"},
	}
	same := &mcp.Tool{
		Name:        "read_file",
		Description: "Read a file",
		InputSchema: map[string]any{"type": "object"},
	}
	changedDescription := copyToolForRegistration(base)
	changedDescription.Description = "Read a workspace file"
	changedAnnotations := copyToolForRegistration(base)
	changedAnnotations.Annotations = &mcp.ToolAnnotations{ReadOnlyHint: true}
	changedMeta := copyToolForRegistration(base)
	changedMeta.Meta = mcp.Meta{"policy": "strict"}

	baseFingerprint := fingerprintToolDefinition(base)
	assert.Equal(t, baseFingerprint, fingerprintToolDefinition(same))
	assert.Assert(t, baseFingerprint != fingerprintToolDefinition(changedDescription))
	assert.Assert(t, baseFingerprint != fingerprintToolDefinition(changedAnnotations))
	assert.Assert(t, baseFingerprint != fingerprintToolDefinition(changedMeta))
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

func TestCollectDownstreamToolState(t *testing.T) {
	proxy := &CentianEndpoint{}

	tests := []struct {
		name               string
		serverName         string
		poolConnecting     bool
		conn               *MockDownstreamConnection
		wantConnectedCount int
		wantConnecting     int
		wantErrors         []string
	}{
		{
			name:           "pool-level connecting wins",
			serverName:     "server-a",
			poolConnecting: true,
			conn:           &MockDownstreamConnection{serverName: "server-a", Status: StatusFailed, ErrorToReturn: errors.New("ignored")},
			wantConnecting: 1,
		},
		{
			name:               "connected connection increments connected count",
			serverName:         "server-b",
			conn:               &MockDownstreamConnection{serverName: "server-b", Status: StatusConnected},
			wantConnectedCount: 1,
		},
		{
			name:           "connecting connection increments connecting count",
			serverName:     "server-c",
			conn:           &MockDownstreamConnection{serverName: "server-c", Status: StatusConnecting},
			wantConnecting: 1,
		},
		{
			name:           "pending connection increments connecting count",
			serverName:     "server-d",
			conn:           &MockDownstreamConnection{serverName: "server-d", Status: StatusPending},
			wantConnecting: 1,
		},
		{
			name:       "failed connection records error",
			serverName: "server-e",
			conn: &MockDownstreamConnection{
				serverName:    "server-e",
				Status:        StatusFailed,
				ErrorToReturn: errors.New("dial failed"),
			},
			wantErrors: []string{"server-e: dial failed"},
		},
		{
			name:       "failed connection without error is ignored",
			serverName: "server-f",
			conn:       &MockDownstreamConnection{serverName: "server-f", Status: StatusFailed},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := &DownstreamSessionPool{
				connecting: map[string]bool{tt.serverName: tt.poolConnecting},
			}
			summary := downstreamRegistrationSummary{}

			proxy.collectDownstreamToolState(pool, tt.serverName, tt.conn, &summary)

			assert.Equal(t, summary.connectedCount, tt.wantConnectedCount)
			assert.Equal(t, summary.connectingCount, tt.wantConnecting)
			assert.DeepEqual(t, summary.connErrors, tt.wantErrors)
		})
	}
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
