package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestHeaderRoundTripperRoundTrip(t *testing.T) {
	// Given: a test server and header round tripper
	received := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rt := HeaderRoundTripper{
		Headers: map[string]string{"X-Test": "value"},
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	assert.NilError(t, err)

	// When: performing the round trip
	resp, err := rt.RoundTrip(request)

	// Then: header is added and request succeeds
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	resp.Body.Close()

	headers := <-received
	assert.Equal(t, headers.Get("X-Test"), "value")
}

func TestCreateTransport_HTTP(t *testing.T) {
	// Given: a downstream connection configured with HTTP URL
	cfg := &config.MCPServerConfig{
		URL: "https://example.com/mcp",
		Headers: map[string]string{
			"Authorization": "Bearer config",
		},
	}
	dc := NewDownstreamConnection("server", cfg)
	authHeaders := map[string]string{"Authorization": "Bearer auth", "X-Extra": "1"}

	// When: creating the transport
	transport, err := dc.createTransport(authHeaders)

	// Then: it returns a StreamableClientTransport with merged headers
	assert.NilError(t, err)
	streamable, ok := transport.(*mcp.StreamableClientTransport)
	assert.Assert(t, ok)
	assert.Equal(t, streamable.Endpoint, cfg.URL)

	roundTripper, ok := streamable.HTTPClient.Transport.(HeaderRoundTripper)
	assert.Assert(t, ok)
	assert.Equal(t, roundTripper.Headers["Authorization"], "Bearer auth")
	assert.Equal(t, roundTripper.Headers["X-Extra"], "1")
}

func TestCreateTransport_Stdio(t *testing.T) {
	// Given: a downstream connection configured with stdio command
	cfg := &config.MCPServerConfig{
		Command: "echo",
		Args:    []string{"hello"},
		Env:     map[string]string{"A": "B"},
	}
	dc := NewDownstreamConnection("server", cfg)

	// When: creating the transport
	transport, err := dc.createTransport(nil)

	// Then: it returns a CommandTransport with env set
	assert.NilError(t, err)
	cmdTransport, ok := transport.(*mcp.CommandTransport)
	assert.Assert(t, ok)
	assert.Assert(t, strings.HasPrefix(filepath.Base(cmdTransport.Command.Path), "echo"))
	assert.Assert(t, containsEnv(cmdTransport.Command.Env, "A=B"))
}

func TestCreateTransport_InvalidConfigs(t *testing.T) {
	// Given: a config with both URL and Command
	cfg := &config.MCPServerConfig{URL: "https://example.com", Command: "echo"}
	dc := NewDownstreamConnection("server", cfg)

	// When: creating the transport
	_, err := dc.createTransport(nil)

	// Then: error is returned
	assert.Assert(t, err != nil)

	// Given: a config with neither URL nor Command
	cfg = &config.MCPServerConfig{}
	dc = NewDownstreamConnection("server", cfg)

	// When: creating the transport
	_, err = dc.createTransport(nil)

	// Then: error is returned
	assert.Assert(t, err != nil)
}

func TestDownstreamConnectionDefaults(t *testing.T) {
	// Given: a new downstream connection
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})

	// Then: it starts disconnected with no tools
	assert.Assert(t, !dc.IsConnected())
	assert.Assert(t, dc.Tools() == nil)

	// When: closing without a session
	err := dc.Close()

	// Then: no error is returned
	assert.NilError(t, err)
}

func TestDownstreamConnectionGetError(t *testing.T) {
	// Given: a connection with no recorded connection error.
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	assert.Assert(t, dc.GetError() == nil)

	// When: a connection error is recorded.
	expectedErr := errors.New("connection failed")
	dc.connError = expectedErr

	// Then: accessor returns the recorded error.
	assert.Equal(t, dc.GetError(), expectedErr)
}

func TestDownstreamConnectionConnectedAt(t *testing.T) {
	// Given: a connection that has not connected yet.
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	assert.Assert(t, dc.ConnectedAt().IsZero())

	// When: connectedAt is set.
	expected := time.Unix(1700000000, 0).UTC()
	dc.connectedAt = expected

	// Then: accessor returns the stored timestamp.
	assert.Equal(t, dc.ConnectedAt(), expected)
}

func TestDownstreamConnectionServerName(t *testing.T) {
	// Given: a named downstream connection.
	dc := NewDownstreamConnection("my-server", &config.MCPServerConfig{})

	// Then: accessor returns the configured server name.
	assert.Equal(t, dc.ServerName(), "my-server")
}

func TestDownstreamConnectionGetConfigAndStatus(t *testing.T) {
	cfg := &config.MCPServerConfig{URL: "https://example.com/mcp"}
	dc := NewDownstreamConnection("server", cfg)
	dc.status = StatusFailed

	assert.Assert(t, dc.GetConfig() == cfg)
	assert.Equal(t, dc.GetStatus(), StatusFailed)
}

func TestDownstreamConnectionDiscoverTools(t *testing.T) {
	clientSession, cleanup := connectTestClientSession(t, func(server *mcp.Server) {
		mcp.AddTool(server, &mcp.Tool{Name: "ping", Description: "ping"}, func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "pong"}},
			}, nil, nil
		})
	})
	defer cleanup()

	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	dc.session = clientSession

	err := dc.discoverTools(context.Background())

	assert.NilError(t, err)
	assert.Equal(t, len(dc.tools), 1)
	assert.Equal(t, dc.tools[0].Name, "ping")
}

func TestBuildClientOptions_IncludesLoggingHandler(t *testing.T) {
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	called := false

	options := dc.buildClientOptions(&DownstreamConnectOptions{
		LoggingHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			called = req != nil && req.Params != nil && req.Params.Level == "info"
		},
	})

	assert.Assert(t, options.LoggingMessageHandler != nil)
	options.LoggingMessageHandler(context.Background(), &mcp.LoggingMessageRequest{
		Params: &mcp.LoggingMessageParams{Level: "info"},
	})
	assert.Assert(t, called)
}

func TestBuildClientOptions_IncludesResourceHandlers(t *testing.T) {
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	listChangedCalled := false
	resourceUpdatedCalled := false

	options := dc.buildClientOptions(&DownstreamConnectOptions{
		ResourceListChangedHandler: func(_ context.Context, req *mcp.ResourceListChangedRequest) {
			listChangedCalled = req != nil
		},
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			resourceUpdatedCalled = req != nil && req.Params != nil && req.Params.URI == "file:///resource"
		},
	})

	assert.Assert(t, options.ResourceListChangedHandler != nil)
	assert.Assert(t, options.ResourceUpdatedHandler != nil)
	options.ResourceListChangedHandler(context.Background(), &mcp.ResourceListChangedRequest{
		Params: &mcp.ResourceListChangedParams{},
	})
	options.ResourceUpdatedHandler(context.Background(), &mcp.ResourceUpdatedNotificationRequest{
		Params: &mcp.ResourceUpdatedNotificationParams{URI: "file:///resource"},
	})
	assert.Assert(t, listChangedCalled)
	assert.Assert(t, resourceUpdatedCalled)
}

func TestDownstreamConnection_ForwardsLoggingNotifications(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "server", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "log", Description: "log"}, func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		if err := req.Session.Log(ctx, &mcp.LoggingMessageParams{Level: "info", Data: "hello"}); err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		}, nil, nil
	})

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	assert.NilError(t, err)
	defer func() { _ = serverSession.Close() }()

	received := make(chan *mcp.LoggingMessageParams, 1)
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	dc.client = mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1.0.0"}, dc.buildClientOptions(&DownstreamConnectOptions{
		LoggingHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			received <- req.Params
		},
	}))
	clientSession, err := dc.client.Connect(ctx, clientTransport, nil)
	assert.NilError(t, err)
	defer func() { _ = clientSession.Close() }()
	dc.session = clientSession
	dc.status = StatusConnected

	assert.NilError(t, dc.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "info"}))
	_, err = dc.CallTool(ctx, "log", map[string]any{})
	assert.NilError(t, err)

	select {
	case params := <-received:
		assert.Equal(t, params.Data.(string), "hello")
	case <-time.After(time.Second):
		t.Fatal("expected downstream log notification")
	}
}

func TestDownstreamConnectionCallTool_NotConnected(t *testing.T) {
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})

	result, err := dc.CallTool(context.Background(), "ping", map[string]any{"k": "v"})

	assert.Assert(t, result == nil)
	assert.ErrorContains(t, err, "not connected to server")
}

func TestDownstreamConnectionDiscoverResourceTemplates(t *testing.T) {
	clientSession, cleanup := connectTestClientSession(t, func(server *mcp.Server) {
		server.AddResourceTemplate(&mcp.ResourceTemplate{
			URITemplate: "file:///items/{id}",
			Name:        "item-template",
		}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{}, nil
		})
	})
	defer cleanup()

	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	dc.session = clientSession

	err := dc.DiscoverResourceTemplates(context.Background())

	assert.NilError(t, err)
	assert.Equal(t, len(dc.ResourceTemplates()), 1)
	assert.Equal(t, dc.ResourceTemplates()[0].URITemplate, "file:///items/{id}")
}

func TestDownstreamConnectionCallTool(t *testing.T) {
	clientSession, cleanup := connectTestClientSession(t, func(server *mcp.Server) {
		mcp.AddTool(server, &mcp.Tool{Name: "ping", Description: "ping"}, func(_ context.Context, _ *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: args["message"].(string)}},
			}, nil, nil
		})
	})
	defer cleanup()

	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	dc.session = clientSession
	dc.status = StatusConnected

	result, err := dc.CallTool(context.Background(), "ping", map[string]any{"message": "pong"})

	assert.NilError(t, err)
	assert.Assert(t, result != nil)
	assert.Equal(t, result.Content[0].(*mcp.TextContent).Text, "pong")
}

func containsEnv(env []string, entry string) bool {
	for _, value := range env {
		if value == entry {
			return true
		}
	}
	return false
}

func connectTestClientSession(t *testing.T, register func(server *mcp.Server)) (*mcp.ClientSession, func()) {
	t.Helper()

	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "server", Version: "1.0.0"}, nil)
	if register != nil {
		register(server)
	}

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	assert.NilError(t, err)

	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	assert.NilError(t, err)

	cleanup := func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	}

	return clientSession, cleanup
}

func TestDownstreamConnectionConnect_EarlyReturn(t *testing.T) {
	// Given: an already connected downstream
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	dc.status = StatusConnected

	// When: connecting
	err := dc.Connect(context.Background(), &DownstreamConnectOptions{})

	// Then: it returns without error
	assert.NilError(t, err)
}

func TestCreateTransport_HTTPHeaderSubstitution(t *testing.T) {
	// Given: config headers with env substitution
	os.Setenv("TEST_HEADER", "value")
	t.Cleanup(func() { os.Unsetenv("TEST_HEADER") })

	cfg := &config.MCPServerConfig{
		URL: "https://example.com/mcp",
		Headers: map[string]string{
			"X-Test": "${TEST_HEADER}",
		},
	}
	dc := NewDownstreamConnection("server", cfg)

	// When: creating the transport
	transport, err := dc.createTransport(nil)

	// Then: substituted header is used
	assert.NilError(t, err)
	streamable, ok := transport.(*mcp.StreamableClientTransport)
	assert.Assert(t, ok)
	roundTripper, ok := streamable.HTTPClient.Transport.(HeaderRoundTripper)
	assert.Assert(t, ok)
	assert.Equal(t, roundTripper.Headers["X-Test"], "value")
}
