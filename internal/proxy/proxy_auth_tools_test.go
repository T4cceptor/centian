package proxy

import (
	"context"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	centoauth "github.com/T4cceptor/centian/internal/oauth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func newOAuthToolTestProxy(t *testing.T, enableTestTools bool) (*CentianEndpoint, *UpstreamSession, centoauth.Binding) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	manager, err := centoauth.NewManager("http://127.0.0.1:8080", nil, nil)
	assert.NilError(t, err)
	logger, err := logging.NewLogger()
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = logger.Close()
	})

	endpoint := &CentianEndpoint{
		name:     "protected",
		endpoint: "/mcp/gateway/protected",
		server: &CentianServer{
			Config: &config.GlobalConfig{
				Version: "1.0.0",
				Proxy:   &config.ProxySettings{EnableTestTools: enableTestTools},
			},
			Logger: logger,
			OAuth:  manager,
		},
		config: &config.GatewayConfig{
			MCPServers: map[string]*config.MCPServerConfig{
				"protected": {
					URL: "http://127.0.0.1:9000/mcp",
					OAuth: &config.OAuthConfig{
						Enabled:          true,
						ClientID:         "client-id",
						ClientSecret:     "client-secret",
						ClientAuthMethod: "client_secret_post",
						Resource:         "http://127.0.0.1:9000/mcp",
						Issuer:           "http://127.0.0.1:9000",
					},
				},
			},
		},
		upstreamSessions: make(map[string]*UpstreamSession),
		downstreamPools:  make(map[string]*DownstreamSessionPool),
	}

	session := &UpstreamSession{
		id:                    "session-1",
		identityKey:           "principal-1",
		downstreamSessionKey:  "pool-1",
		downstreamConns:       make(map[string]DownstreamConnectionInterface),
		registeredTools:       make(map[string]struct{}),
		registeredStaticTools: make(map[string]struct{}),
	}
	session.upstreamServer = endpoint.newUpstreamServer(session)
	endpoint.upstreamSessions[session.id] = session

	binding := centoauth.Binding{
		PrincipalID: session.identityKey,
		Gateway:     "gateway",
		Server:      "protected",
	}
	return endpoint, session, binding
}

func connectAuthToolClient(
	t *testing.T,
	session *UpstreamSession,
	options *mcp.ClientOptions,
) (*mcp.ClientSession, func()) {
	t.Helper()

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := session.upstreamServer.Connect(ctx, serverTransport, nil)
	assert.NilError(t, err)

	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1.0.0"}, options)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	assert.NilError(t, err)

	session.id = ""
	return clientSession, func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	}
}

func listToolNames(t *testing.T, clientSession *mcp.ClientSession) []string {
	t.Helper()

	result, err := clientSession.ListTools(context.Background(), nil)
	assert.NilError(t, err)
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		if tool != nil {
			names = append(names, tool.Name)
		}
	}
	sort.Strings(names)
	return names
}

func TestNewUpstreamServerRegistersAuthStatusTool(t *testing.T) {
	_, session, _ := newOAuthToolTestProxy(t, false)

	clientSession, cleanup := connectAuthToolClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	assert.DeepEqual(t, listToolNames(t, clientSession), []string{authStatusToolName})
}

func TestNewUpstreamServerRegistersTestNotificationsToolWhenEnabled(t *testing.T) {
	_, session, _ := newOAuthToolTestProxy(t, true)

	clientSession, cleanup := connectAuthToolClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	assert.DeepEqual(t, listToolNames(t, clientSession), []string{authStatusToolName, testNotificationsTool})
}

func TestHandleAuthStatusTool(t *testing.T) {
	proxy, session, binding := newOAuthToolTestProxy(t, false)
	pending, err := proxy.server.OAuth.CreatePending(binding, "client-id", "client-secret", &centoauth.ResolvedMetadata{
		Resource:              "http://127.0.0.1:9000/mcp",
		Scopes:                []string{"tool:echo"},
		Issuer:                "http://127.0.0.1:9000",
		AuthorizationEndpoint: "http://127.0.0.1:9000/authorize",
		TokenEndpoint:         "http://127.0.0.1:9000/token",
		ClientAuthMethod:      "client_secret_post",
	}, "verifier")
	assert.NilError(t, err)

	result, err := proxy.handleAuthStatusTool(context.Background(), session, nil)
	assert.NilError(t, err)
	assert.Assert(t, result != nil)
	assert.Equal(t, len(result.Content), 1)

	text, ok := result.Content[0].(*mcp.TextContent)
	assert.Assert(t, ok)
	assert.Equal(t, text.Text, "protected: auth_required via "+loginToolName("protected"))

	structured, ok := result.StructuredContent.(map[string]any)
	assert.Assert(t, ok)
	servers, ok := structured["servers"].([]map[string]any)
	assert.Assert(t, ok)
	assert.Equal(t, len(servers), 1)
	assert.Equal(t, servers[0]["server"], "protected")
	assert.Equal(t, servers[0]["state"], authStateRequired)
	assert.Assert(t, strings.Contains(servers[0]["message"].(string), "requires login"))
	assert.Equal(t, servers[0]["loginTool"], loginToolName("protected"))
	assert.Equal(t, servers[0]["startUrl"], proxy.server.OAuth.StartURL(pending.ID))
	assert.Equal(t, servers[0]["statusUrl"], proxy.server.OAuth.StatusURL(pending.ID))
	assert.Equal(t, servers[0]["lastError"], "")
}

func TestSyncAvailableToolsAddsAndRemovesLoginTool(t *testing.T) {
	proxy, session, binding := newOAuthToolTestProxy(t, false)
	_, err := proxy.server.OAuth.CreatePending(binding, "client-id", "client-secret", &centoauth.ResolvedMetadata{
		Resource:              "http://127.0.0.1:9000/mcp",
		Scopes:                []string{"tool:echo"},
		Issuer:                "http://127.0.0.1:9000",
		AuthorizationEndpoint: "http://127.0.0.1:9000/authorize",
		TokenEndpoint:         "http://127.0.0.1:9000/token",
		ClientAuthMethod:      "client_secret_post",
	}, "verifier")
	assert.NilError(t, err)

	conn := &MockDownstreamConnection{
		serverName: "protected",
		Status:     StatusAuthRequired,
	}
	session.downstreamConns["protected"] = conn
	proxy.downstreamPools[session.downstreamSessionKey] = &DownstreamSessionPool{
		downstreamSessionKey: session.downstreamSessionKey,
		identityKey:          session.identityKey,
		downstreamConns:      map[string]DownstreamConnectionInterface{"protected": conn},
		upstreamSessions:     map[string]*UpstreamSession{"session-1": session},
		connecting:           make(map[string]bool),
	}

	clientSession, cleanup := connectAuthToolClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	proxy.syncAvailableTools(session)
	assert.DeepEqual(t, listToolNames(t, clientSession), []string{authStatusToolName, loginToolName("protected")})

	conn.Status = StatusConnected
	conn.tools = []*mcp.Tool{{Name: "echo", Description: "echo", InputSchema: map[string]any{"type": "object"}}}
	proxy.syncAvailableTools(session)
	assert.DeepEqual(t, listToolNames(t, clientSession), []string{authStatusToolName, "echo"})
}

func TestSyncAvailableToolsSkipsReservedDownstreamToolNames(t *testing.T) {
	proxy, session, _ := newOAuthToolTestProxy(t, false)
	conn := &MockDownstreamConnection{
		serverName: "protected",
		Status:     StatusConnected,
		tools: []*mcp.Tool{
			{Name: authStatusToolName, Description: "reserved", InputSchema: map[string]any{"type": "object"}},
			{Name: "echo", Description: "echo", InputSchema: map[string]any{"type": "object"}},
		},
	}
	session.downstreamConns["protected"] = conn

	clientSession, cleanup := connectAuthToolClient(t, session, &mcp.ClientOptions{})
	defer cleanup()

	proxy.syncAvailableTools(session)
	assert.DeepEqual(t, listToolNames(t, clientSession), []string{authStatusToolName, "echo"})
}

func TestLoginToolUsesURLElicitationWhenSupported(t *testing.T) {
	proxy, session, binding := newOAuthToolTestProxy(t, false)
	pending, err := proxy.server.OAuth.CreatePending(binding, "client-id", "client-secret", &centoauth.ResolvedMetadata{
		Resource:              "http://127.0.0.1:9000/mcp",
		Scopes:                []string{"tool:echo"},
		Issuer:                "http://127.0.0.1:9000",
		AuthorizationEndpoint: "http://127.0.0.1:9000/authorize",
		TokenEndpoint:         "http://127.0.0.1:9000/token",
		ClientAuthMethod:      "client_secret_post",
	}, "verifier")
	assert.NilError(t, err)

	conn := &MockDownstreamConnection{serverName: "protected", Status: StatusAuthRequired}
	session.downstreamConns["protected"] = conn
	proxy.downstreamPools[session.downstreamSessionKey] = &DownstreamSessionPool{
		downstreamSessionKey: session.downstreamSessionKey,
		identityKey:          session.identityKey,
		downstreamConns:      map[string]DownstreamConnectionInterface{"protected": conn},
		upstreamSessions:     map[string]*UpstreamSession{"session-1": session},
		connecting:           make(map[string]bool),
	}
	proxy.syncAvailableTools(session)

	var elicitedURL string
	clientSession, cleanup := connectAuthToolClient(t, session, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{
			Elicitation: &mcp.ElicitationCapabilities{URL: &mcp.URLElicitationCapabilities{}},
		},
		ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			elicitedURL = req.Params.URL
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	})
	defer cleanup()

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{Name: loginToolName("protected"), Arguments: map[string]any{}})
	assert.NilError(t, err)
	assert.Assert(t, result != nil)
	assert.Equal(t, elicitedURL, proxy.server.OAuth.StartURL(pending.ID))
}

func TestHandleToolCallAuthorizationRequiredAddsLoginTool(t *testing.T) {
	proxy, session, binding := newOAuthToolTestProxy(t, false)
	pending, err := proxy.server.OAuth.CreatePending(binding, "client-id", "client-secret", &centoauth.ResolvedMetadata{
		Resource:              "http://127.0.0.1:9000/mcp",
		Scopes:                []string{"tool:echo"},
		Issuer:                "http://127.0.0.1:9000",
		AuthorizationEndpoint: "http://127.0.0.1:9000/authorize",
		TokenEndpoint:         "http://127.0.0.1:9000/token",
		ClientAuthMethod:      "client_secret_post",
	}, "verifier")
	assert.NilError(t, err)

	conn := &MockDownstreamConnection{
		serverName: "protected",
		Status:     StatusConnected,
		cfg:        &config.MCPServerConfig{URL: "http://127.0.0.1:9000/mcp"},
		tools: []*mcp.Tool{
			{Name: "echo", Description: "echo", InputSchema: map[string]any{"type": "object"}},
		},
		ErrorToReturn: &centoauth.AuthorizationRequiredError{
			Binding:   binding,
			AuthURL:   proxy.server.OAuth.StartURL(pending.ID),
			StatusURL: proxy.server.OAuth.StatusURL(pending.ID),
			PendingID: pending.ID,
			Reason:    centoauth.AuthorizationReasonRequired,
		},
	}
	session.downstreamConns["protected"] = conn
	proxy.downstreamPools[session.downstreamSessionKey] = &DownstreamSessionPool{
		downstreamSessionKey: session.downstreamSessionKey,
		identityKey:          session.identityKey,
		downstreamConns:      map[string]DownstreamConnectionInterface{"protected": conn},
		upstreamSessions:     map[string]*UpstreamSession{"session-1": session},
		connecting:           make(map[string]bool),
	}
	proxy.syncAvailableTools(session)

	var elicitedURL string
	clientSession, cleanup := connectAuthToolClient(t, session, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{
			Elicitation: &mcp.ElicitationCapabilities{URL: &mcp.URLElicitationCapabilities{}},
		},
		ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			elicitedURL = req.Params.URL
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	})
	defer cleanup()

	_, err = clientSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "hello"}})
	assert.ErrorContains(t, err, loginToolName("protected"))
	assert.Equal(t, elicitedURL, proxy.server.OAuth.StartURL(pending.ID))
	assert.Assert(t, containsToolName(listToolNames(t, clientSession), loginToolName("protected")))
}

func containsToolName(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

func TestTestNotificationsToolEmitsLogs(t *testing.T) {
	_, session, _ := newOAuthToolTestProxy(t, true)

	var (
		mu       sync.Mutex
		messages []string
	)
	clientSession, cleanup := connectAuthToolClient(t, session, &mcp.ClientOptions{
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			if req == nil || req.Params == nil {
				return
			}
			if data, ok := req.Params.Data.(string); ok {
				mu.Lock()
				messages = append(messages, data)
				mu.Unlock()
			}
		},
	})
	defer cleanup()

	assert.NilError(t, clientSession.SetLoggingLevel(context.Background(), &mcp.SetLoggingLevelParams{Level: "info"}))
	session.logLevel = "info"
	_, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: testNotificationsTool,
		Arguments: map[string]any{
			"intervalSeconds": 0.05,
			"count":           2,
		},
	})
	assert.NilError(t, err)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := len(messages)
		mu.Unlock()
		if count >= 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("expected at least 2 log notifications, got %d (%v)", len(messages), messages)
}

func TestNotifyOAuthAuthorizedIncludesCurrentToolNames(t *testing.T) {
	proxy, session, _ := newOAuthToolTestProxy(t, false)
	session.downstreamConns["protected"] = &MockDownstreamConnection{
		serverName: "protected",
		Status:     StatusConnected,
		tools: []*mcp.Tool{
			{Name: "echo", Description: "echo", InputSchema: map[string]any{"type": "object"}},
		},
	}

	recorder, cleanup := connectLoggingClient(t, session)
	defer cleanup()

	proxy.syncAvailableTools(session)
	proxy.notifyOAuthAuthorized(session, "protected", proxy.currentUpstreamToolNames(session))

	messages := waitForSingleLog(t, recorder)
	data, ok := messages[0].Data.(string)
	assert.Assert(t, ok)
	assert.Assert(t, strings.Contains(data, "OAuth complete for downstream protected."))
	assert.Assert(t, strings.Contains(data, authStatusToolName))
	assert.Assert(t, strings.Contains(data, "echo"))
}
