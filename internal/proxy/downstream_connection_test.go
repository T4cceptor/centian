package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	centoauth "github.com/T4cceptor/centian/internal/oauth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestHeaderRoundTripperRoundTrip(t *testing.T) {
	// Given: a round tripper with a capturing base
	received := make(chan http.Header, 1)
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		received <- req.Header.Clone()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       http.NoBody,
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	rt := HeaderRoundTripper{
		Base:    base,
		Headers: map[string]string{"X-Test": "value"},
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com", http.NoBody)
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
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

func TestCreateTransport_HTTPOAuthSkipsForwardedAuthHeaders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	manager, err := centoauth.NewManager("http://127.0.0.1:9666", nil, nil)
	assert.NilError(t, err)

	cfg := &config.MCPServerConfig{
		URL: "https://example.com/mcp",
		OAuth: &config.OAuthConfig{
			Enabled:          true,
			ClientID:         "client-id",
			ClientSecret:     "client-secret",
			ClientAuthMethod: "client_secret_post",
			Resource:         "https://example.com/mcp",
			Issuer:           "https://issuer.example",
		},
		Headers: map[string]string{
			"X-Config": "set",
		},
	}
	dc := NewDownstreamConnection("server", cfg)
	dc.oauthManager = manager
	dc.connectOptions = &DownstreamConnectOptions{
		IdentityKey: "user-1",
		GatewayName: "gateway",
	}

	transport, err := dc.createTransport(map[string]string{"Authorization": "Bearer upstream"})
	assert.NilError(t, err)

	streamable, ok := transport.(*mcp.StreamableClientTransport)
	assert.Assert(t, ok)

	oauthTransport, ok := streamable.HTTPClient.Transport.(*centoauth.Transport)
	assert.Assert(t, ok)
	assert.Equal(t, oauthTransport.Headers["X-Config"], "set")
	_, exists := oauthTransport.Headers["Authorization"]
	assert.Assert(t, !exists)
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

func TestCreateTransport_StdioMergesConfiguredEnvWithOS(t *testing.T) {
	// Given: inherited OS env and configured downstream env.
	t.Setenv("CENTIAN_TEST_INHERITED_ENV", "inherited")

	cfg := &config.MCPServerConfig{
		Command: "echo",
		Args:    []string{"hello"},
		Env: map[string]string{
			"A": "B",
		},
	}
	dc := NewDownstreamConnection("server", cfg)

	// When: creating the stdio transport.
	transport, err := dc.createTransport(nil)

	// Then: configured env is present and inherited env is preserved.
	assert.NilError(t, err)
	cmdTransport, ok := transport.(*mcp.CommandTransport)
	assert.Assert(t, ok)
	assert.Assert(t, containsEnv(cmdTransport.Command.Env, "A=B"))
	assert.Assert(t, containsEnv(cmdTransport.Command.Env, "CENTIAN_TEST_INHERITED_ENV=inherited"))
}

func TestCreateTransport_StdioConfiguredEnvOverridesOS(t *testing.T) {
	// Given: an OS env var with the same key as configured downstream env.
	t.Setenv("CENTIAN_TEST_OVERRIDE_ENV", "os-value")

	cfg := &config.MCPServerConfig{
		Command: "echo",
		Args:    []string{"hello"},
		Env: map[string]string{
			"CENTIAN_TEST_OVERRIDE_ENV": "config-value",
		},
	}
	dc := NewDownstreamConnection("server", cfg)

	// When: creating the stdio transport.
	transport, err := dc.createTransport(nil)

	// Then: configured env wins over the inherited value.
	assert.NilError(t, err)
	cmdTransport, ok := transport.(*mcp.CommandTransport)
	assert.Assert(t, ok)
	assert.Assert(t, containsEnv(cmdTransport.Command.Env, "CENTIAN_TEST_OVERRIDE_ENV=config-value"))
	assert.Assert(t, !containsEnv(cmdTransport.Command.Env, "CENTIAN_TEST_OVERRIDE_ENV=os-value"))
}

func TestCreateTransport_StdioWithoutConfiguredEnvInheritsOS(t *testing.T) {
	// Given: a stdio command with no configured env overrides.
	cfg := &config.MCPServerConfig{
		Command: "echo",
		Args:    []string{"hello"},
	}
	dc := NewDownstreamConnection("server", cfg)

	// When: creating the stdio transport.
	transport, err := dc.createTransport(nil)

	// Then: Go inheritance behavior is preserved by leaving cmd.Env unset.
	assert.NilError(t, err)
	cmdTransport, ok := transport.(*mcp.CommandTransport)
	assert.Assert(t, ok)
	assert.Assert(t, cmdTransport.Command.Env == nil)
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

func TestConnectionStatusHelpers(t *testing.T) {
	assert.Assert(t, StatusPending.IsPending())
	assert.Assert(t, StatusConnecting.IsConnecting())
	assert.Assert(t, StatusConnected.IsConnected())
	assert.Assert(t, StatusFailed.IsFailed())
	assert.Assert(t, StatusDisconnected.IsDisconnected())
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
	assert.Equal(t, dc.GetServerName(), "my-server")
}

func TestDownstreamConnectionGetConfigAndStatus(t *testing.T) {
	cfg := &config.MCPServerConfig{URL: "https://example.com/mcp"}
	dc := NewDownstreamConnection("server", cfg)
	dc.status = StatusFailed

	assert.Assert(t, dc.GetConfig() == cfg)
	assert.Equal(t, dc.GetStatus(), StatusFailed)
}

func TestDownstreamConnectionRecordConnectError(t *testing.T) {
	t.Run("marks generic errors as failed", func(t *testing.T) {
		dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
		err := errors.New("connect failed")

		dc.recordConnectError(err)

		assert.Equal(t, dc.GetStatus(), StatusFailed)
		assert.Equal(t, dc.GetError(), err)
	})

	t.Run("marks auth required errors", func(t *testing.T) {
		dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
		authErr := &centoauth.AuthorizationRequiredError{
			Binding: centoauth.Binding{
				PrincipalID: "principal-1",
				Gateway:     "gw",
				Server:      "server",
			},
			Reason: centoauth.AuthorizationReasonRequired,
		}

		dc.recordConnectError(authErr)

		assert.Equal(t, dc.GetStatus(), StatusAuthRequired)
		assert.Equal(t, dc.GetError(), authErr)
	})

	t.Run("marks refresh failures distinctly", func(t *testing.T) {
		dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
		authErr := &centoauth.AuthorizationRequiredError{
			Binding: centoauth.Binding{
				PrincipalID: "principal-1",
				Gateway:     "gw",
				Server:      "server",
			},
			Reason: centoauth.AuthorizationReasonRefreshFailed,
		}

		dc.recordConnectError(authErr)

		assert.Equal(t, dc.GetStatus(), StatusRefreshFailed)
		assert.Equal(t, dc.GetError(), authErr)
	})
}

func TestDownstreamConnectionStateAccessors(t *testing.T) {
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})

	assert.Assert(t, dc.IsPending())

	dc.status = StatusConnecting
	assert.Assert(t, dc.IsConnecting())

	dc.status = StatusFailed
	assert.Assert(t, dc.IsFailed())

	dc.status = StatusDisconnected
	assert.Assert(t, dc.IsDisconnected())
}

func TestDownstreamConnectionDiscoverTools(t *testing.T) {
	clientSession, cleanup := connectTestClientSession(t, nil, func(server *mcp.Server) {
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

func TestRootsHelpers(t *testing.T) {
	current := []*mcp.Root{
		{Name: "one", URI: "file:///one"},
		{Name: "two", URI: "file:///two"},
	}
	next := []*mcp.Root{
		{Name: "one", URI: "file:///one"},
		{Name: "three", URI: "file:///three"},
		{Name: "two-updated", URI: "file:///two"},
	}

	currentByURI := rootsByURI(current)
	nextByURI := rootsByURI(next)

	assert.Equal(t, len(currentByURI), 2)
	assert.DeepEqual(t, removedRootURIs(currentByURI, nextByURI), []string{})

	added := addedOrUpdatedRoots(currentByURI, nextByURI)
	assert.Equal(t, len(added), 2)
	assert.Equal(t, added[0].URI != "", true)
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
	_, err = dc.CallTool(ctx, &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "log",
			Arguments: json.RawMessage(`{}`),
		},
	})
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

	result, err := dc.CallTool(context.Background(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "ping",
			Arguments: json.RawMessage(`{"k":"v"}`),
		},
	})

	assert.Assert(t, result == nil)
	assert.ErrorContains(t, err, "not connected to server")
}

func TestDownstreamConnectionReadResourceNotConnected(t *testing.T) {
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})

	result, err := dc.ReadResource(context.Background(), "file:///resource")

	assert.Assert(t, result == nil)
	assert.ErrorContains(t, err, "not connected to server")
}

func TestDownstreamConnectionGetPromptNotConnected(t *testing.T) {
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})

	result, err := dc.GetPrompt(context.Background(), "prompt", map[string]string{"path": "x"})

	assert.Assert(t, result == nil)
	assert.ErrorContains(t, err, "not connected to server")
}

func TestDownstreamConnectionCompleteNotConnected(t *testing.T) {
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})

	result, err := dc.Complete(context.Background(), &mcp.CompleteRequest{
		Params: &mcp.CompleteParams{},
	})

	assert.Assert(t, result == nil)
	assert.ErrorContains(t, err, "not connected to server")
}

func TestDownstreamConnectionSubscribeNotConnected(t *testing.T) {
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})

	err := dc.Subscribe(context.Background(), "file:///resource")

	assert.ErrorContains(t, err, "not connected to server")
}

func TestDownstreamConnectionUnsubscribeNotConnected(t *testing.T) {
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})

	err := dc.Unsubscribe(context.Background(), "file:///resource")

	assert.ErrorContains(t, err, "not connected to server")
}

func TestDownstreamConnectionSyncClientStateWithoutClient(t *testing.T) {
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	state := buildDownstreamClientState("2025-06-18", &mcp.ClientCapabilities{
		RootsV2: &mcp.RootCapabilities{ListChanged: true},
	}, []*mcp.Root{{Name: "workspace", URI: "file:///workspace"}})

	err := dc.SyncClientState(context.Background(), state)

	assert.NilError(t, err)
	assert.Equal(t, dc.clientState.ProtocolVersion, "2025-06-18")
	assert.Equal(t, len(dc.clientState.Roots), 1)
}

func TestDownstreamConnectionDiscoverResourceTemplates(t *testing.T) {
	clientSession, cleanup := connectTestClientSession(t, nil, func(server *mcp.Server) {
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

func TestDownstreamConnectionDiscoverResourcesAndRead(t *testing.T) {
	clientSession, cleanup := connectTestClientSession(t, nil, func(server *mcp.Server) {
		server.AddResource(&mcp.Resource{
			URI:  "file:///resource",
			Name: "resource",
		}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{
					{URI: "file:///resource", Text: "hello"},
				},
			}, nil
		})
	})
	defer cleanup()

	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	dc.session = clientSession
	dc.status = StatusConnected

	assert.NilError(t, dc.discoverResources(context.Background()))
	assert.Equal(t, len(dc.Resources()), 1)

	result, err := dc.ReadResource(context.Background(), "file:///resource")
	assert.NilError(t, err)
	assert.Equal(t, len(result.Contents), 1)
}

func TestDownstreamConnectionDiscoverPromptsAndGetPrompt(t *testing.T) {
	clientSession, cleanup := connectTestClientSession(t, nil, func(server *mcp.Server) {
		server.AddPrompt(&mcp.Prompt{Name: "review", Description: "review prompt"}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{
				Description: req.Params.Arguments["path"],
			}, nil
		})
	})
	defer cleanup()

	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	dc.session = clientSession
	dc.status = StatusConnected

	assert.NilError(t, dc.discoverPrompts(context.Background()))
	assert.Equal(t, len(dc.Prompts()), 1)

	result, err := dc.GetPrompt(context.Background(), "review", map[string]string{"path": "/tmp/file.go"})
	assert.NilError(t, err)
	assert.Equal(t, result.Description, "/tmp/file.go")
}

func TestDownstreamConnectionDiscoverPublicMethodsReturnNilWithoutSession(t *testing.T) {
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})

	assert.NilError(t, dc.DiscoverResources(context.Background()))
	assert.NilError(t, dc.DiscoverPrompts(context.Background()))
}

func TestDownstreamConnectionComplete(t *testing.T) {
	clientSession, cleanup := connectTestClientSession(
		t,
		&mcp.ServerOptions{
			CompletionHandler: func(_ context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
				assert.Equal(t, req.Params.Argument.Value, "sr")
				return &mcp.CompleteResult{
					Completion: mcp.CompletionResultDetails{Values: []string{"src", "scripts"}},
				}, nil
			},
		},
		nil,
	)
	defer cleanup()

	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	dc.session = clientSession
	dc.status = StatusConnected

	result, err := dc.Complete(context.Background(), &mcp.CompleteRequest{
		Params: &mcp.CompleteParams{
			Argument: mcp.CompleteParamsArgument{Name: "path", Value: "sr"},
		},
	})

	assert.NilError(t, err)
	assert.DeepEqual(t, result.Completion.Values, []string{"src", "scripts"})
}

func TestDownstreamConnectionSubscribeAndUnsubscribe(t *testing.T) {
	clientSession, cleanup := connectTestClientSession(
		t,
		&mcp.ServerOptions{
			SubscribeHandler: func(_ context.Context, req *mcp.SubscribeRequest) error {
				assert.Equal(t, req.Params.URI, "file:///resource")
				return nil
			},
			UnsubscribeHandler: func(_ context.Context, req *mcp.UnsubscribeRequest) error {
				assert.Equal(t, req.Params.URI, "file:///resource")
				return nil
			},
		},
		func(server *mcp.Server) {
			server.AddResource(&mcp.Resource{URI: "file:///resource", Name: "resource"}, nil)
		},
	)
	defer cleanup()

	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	dc.session = clientSession
	dc.status = StatusConnected

	assert.NilError(t, dc.Subscribe(context.Background(), "file:///resource"))
	assert.NilError(t, dc.Unsubscribe(context.Background(), "file:///resource"))
}

func TestDownstreamConnectionSyncClientStateWithConnectedClient(t *testing.T) {
	client, clientSession, cleanup := connectTestClient(t, nil, func(server *mcp.Server) {
		mcp.AddTool(server, &mcp.Tool{Name: "ping", Description: "ping"}, func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{}, nil, nil
		})
	})
	defer cleanup()

	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	dc.client = client
	dc.session = clientSession
	dc.status = StatusConnected
	dc.clientState = *buildDownstreamClientState("2025-06-18", &mcp.ClientCapabilities{}, []*mcp.Root{
		{Name: "old", URI: "file:///old"},
	})

	nextState := buildDownstreamClientState("2025-06-18", &mcp.ClientCapabilities{}, []*mcp.Root{
		{Name: "new", URI: "file:///new"},
	})

	err := dc.SyncClientState(context.Background(), nextState)

	assert.NilError(t, err)
	assert.Equal(t, len(dc.clientState.Roots), 1)
	assert.Equal(t, dc.clientState.Roots[0].URI, "file:///new")
	assert.Equal(t, len(dc.Tools()), 1)
}

func TestDownstreamConnectionCallTool(t *testing.T) {
	clientSession, cleanup := connectTestClientSession(t, nil, func(server *mcp.Server) {
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

	result, err := dc.CallTool(context.Background(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "ping",
			Arguments: json.RawMessage(`{"message":"pong"}`),
		},
	})

	assert.NilError(t, err)
	assert.Assert(t, result != nil)
	assert.Equal(t, result.Content[0].(*mcp.TextContent).Text, "pong")
}

func TestDownstreamConnectionCallTool_PreservesMeta(t *testing.T) {
	var capturedMeta mcp.Meta

	clientSession, cleanup := connectTestClientSession(t, nil, func(server *mcp.Server) {
		mcp.AddTool(server, &mcp.Tool{Name: "ping", Description: "ping"}, func(_ context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			if req != nil && req.Params != nil {
				capturedMeta = deepCloneMeta(req.Params.Meta)
			}
			return &mcp.CallToolResult{
				StructuredContent: map[string]any{
					"meta": req.Params.Meta,
				},
			}, nil, nil
		})
	})
	defer cleanup()

	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	dc.session = clientSession
	dc.status = StatusConnected

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Meta: mcp.Meta{
				"progressToken": "progress-1",
				"custom":        "value",
			},
			Name:      "ping",
			Arguments: json.RawMessage(`{"message":"pong"}`),
		},
	}

	result, err := dc.CallTool(context.Background(), req)

	assert.NilError(t, err)
	assert.Assert(t, result != nil)
	assert.DeepEqual(t, capturedMeta, req.Params.Meta)

	structured, ok := result.StructuredContent.(map[string]any)
	assert.Assert(t, ok)
	meta, ok := structured["meta"].(map[string]any)
	assert.Assert(t, ok)
	assert.DeepEqual(t, meta, map[string]any{
		"progressToken": "progress-1",
		"custom":        "value",
	})
}

func containsEnv(env []string, entry string) bool {
	for _, value := range env {
		if value == entry {
			return true
		}
	}
	return false
}

func connectTestClientSession(t *testing.T, options *mcp.ServerOptions, register func(server *mcp.Server)) (*mcp.ClientSession, func()) {
	t.Helper()

	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "server", Version: "1.0.0"}, options)
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

func connectTestClient(t *testing.T, options *mcp.ServerOptions, register func(server *mcp.Server)) (*mcp.Client, *mcp.ClientSession, func()) {
	t.Helper()

	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "server", Version: "1.0.0"}, options)
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

	return client, clientSession, cleanup
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
