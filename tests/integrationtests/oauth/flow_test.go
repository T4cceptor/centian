package oauth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOAuthDiscoveryAuthFlowReSyncsTools(t *testing.T) {
	harness := newOAuthHarness(t, false)
	recorder := &oauthClientRecorder{}
	client := mcp.NewClient(&mcp.Implementation{Name: "oauth-test-client", Version: "1.0.0"}, &mcp.ClientOptions{
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			if req != nil && req.Params != nil {
				recorder.addLog(req.Params.Data.(string))
			}
		},
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {
			recorder.incrementToolListChanged()
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   harness.proxyURL,
		HTTPClient: &http.Client{Timeout: integrationTimeout},
	}, nil)
	if err != nil {
		t.Fatalf("connect proxy: %v", err)
	}
	defer session.Close()

	if err := session.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "info"}); err != nil {
		t.Fatalf("set logging level: %v", err)
	}

	waitUntil(t, integrationTimeout, func() bool {
		return recorder.hasLogSubstring("OAuth required for downstream oauth-gateway/protected.")
	})

	waitUntil(t, integrationTimeout, func() bool {
		tools, err := session.ListTools(ctx, nil)
		return err == nil && hasTool(tools.Tools, "centian.auth_status") && hasTool(tools.Tools, "centian.login.protected")
	})

	loginResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "centian.login.protected"})
	if err != nil {
		t.Fatalf("call login tool: %v", err)
	}
	authURL := structuredString(loginResult, "startUrl")
	if authURL == "" {
		t.Fatalf("login tool did not return startUrl: %#v", loginResult)
	}
	if _, err := http.Get(authURL); err != nil {
		t.Fatalf("visit auth url: %v", err)
	}

	waitUntil(t, integrationTimeout, func() bool {
		return recorder.hasLogSubstring("OAuth complete for downstream protected.")
	})
	waitUntil(t, integrationTimeout, func() bool {
		return recorder.toolListChangedCount() > 0
	})
	waitUntil(t, integrationTimeout, func() bool {
		tools, err := session.ListTools(ctx, nil)
		return err == nil && hasTool(tools.Tools, "echo") && !hasTool(tools.Tools, "centian.login.protected")
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "hello"}})
	if err != nil {
		t.Fatalf("call tool after auth: %v", err)
	}
	if result.IsError || len(result.Content) == 0 {
		t.Fatalf("unexpected tool result: %#v", result)
	}
}

func TestOAuthExpiredTokenRefreshesDuringReconnect(t *testing.T) {
	harness := newOAuthHarness(t, true)
	recorder := &oauthClientRecorder{}
	client := mcp.NewClient(&mcp.Implementation{Name: "oauth-refresh-client", Version: "1.0.0"}, &mcp.ClientOptions{
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			if req != nil && req.Params != nil {
				if message, ok := req.Params.Data.(string); ok {
					recorder.addLog(message)
				}
			}
		},
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {
			recorder.incrementToolListChanged()
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   harness.proxyURL,
		HTTPClient: &http.Client{Timeout: integrationTimeout},
	}, nil)
	if err != nil {
		t.Fatalf("connect proxy: %v", err)
	}
	defer session.Close()

	waitUntil(t, integrationTimeout, func() bool {
		tools, err := session.ListTools(ctx, nil)
		return err == nil && hasTool(tools.Tools, "centian.login.protected")
	})
	loginResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "centian.login.protected"})
	if err != nil {
		t.Fatalf("call login tool: %v", err)
	}
	authURL := structuredString(loginResult, "startUrl")
	if authURL == "" {
		t.Fatalf("login tool did not return startUrl: %#v", loginResult)
	}
	if _, err := http.Get(authURL); err != nil {
		t.Fatalf("visit auth url: %v", err)
	}

	waitUntil(t, integrationTimeout, func() bool {
		tools, err := session.ListTools(ctx, nil)
		return err == nil && hasTool(tools.Tools, "echo") && !hasTool(tools.Tools, "centian.login.protected")
	})
	if harness.authServer.RefreshCount() == 0 {
		t.Fatalf("expected token refresh during reconnect")
	}
}

func TestOAuthLoginToolUsesURLElicitation(t *testing.T) {
	harness := newOAuthHarness(t, false)

	var promptedURL string
	client := mcp.NewClient(&mcp.Implementation{Name: "oauth-elicit-client", Version: "1.0.0"}, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{
			Elicitation: &mcp.ElicitationCapabilities{URL: &mcp.URLElicitationCapabilities{}},
		},
		ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			promptedURL = req.Params.URL
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   harness.proxyURL,
		HTTPClient: &http.Client{Timeout: integrationTimeout},
	}, nil)
	if err != nil {
		t.Fatalf("connect proxy: %v", err)
	}
	defer session.Close()

	waitUntil(t, integrationTimeout, func() bool {
		tools, err := session.ListTools(ctx, nil)
		return err == nil && hasTool(tools.Tools, "centian.login.protected")
	})

	loginResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "centian.login.protected"})
	if err != nil {
		t.Fatalf("call login tool: %v", err)
	}
	startURL := structuredString(loginResult, "startUrl")
	if promptedURL == "" || promptedURL != startURL {
		t.Fatalf("expected elicitation url %q, got %q", startURL, promptedURL)
	}
	if _, err := http.Get(startURL); err != nil {
		t.Fatalf("visit auth url: %v", err)
	}

	waitUntil(t, integrationTimeout, func() bool {
		tools, err := session.ListTools(ctx, nil)
		return err == nil && hasTool(tools.Tools, "echo")
	})
}

func waitUntil(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("condition not satisfied within %s", timeout)
}

func hasTool(tools []*mcp.Tool, name string) bool {
	for _, tool := range tools {
		if tool != nil && tool.Name == name {
			return true
		}
	}
	return false
}

func structuredString(result *mcp.CallToolResult, key string) string {
	if result == nil {
		return ""
	}
	data, ok := result.StructuredContent.(map[string]any)
	if !ok {
		return ""
	}
	value, _ := data[key].(string)
	return value
}
