package oauth

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
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

	waitUntil(t, integrationTimeout, func() bool {
		return recorder.authURL() != "" || fetchPendingStartURL(t, harness) != ""
	})
	authURL := recorder.authURL()
	if authURL == "" {
		authURL = fetchPendingStartURL(t, harness)
	}
	if _, err := http.Get(authURL); err != nil {
		t.Fatalf("visit auth url: %v", err)
	}

	waitUntil(t, integrationTimeout, func() bool {
		return recorder.toolListChangedCount() > 0
	})
	waitUntil(t, integrationTimeout, func() bool {
		tools, err := session.ListTools(ctx, nil)
		return err == nil && len(tools.Tools) == 1
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
		return recorder.authURL() != "" || fetchPendingStartURL(t, harness) != ""
	})
	authURL := recorder.authURL()
	if authURL == "" {
		authURL = fetchPendingStartURL(t, harness)
	}
	if _, err := http.Get(authURL); err != nil {
		t.Fatalf("visit auth url: %v", err)
	}

	waitUntil(t, integrationTimeout, func() bool {
		tools, err := session.ListTools(ctx, nil)
		return err == nil && len(tools.Tools) == 1
	})
	if harness.authServer.RefreshCount() == 0 {
		t.Fatalf("expected token refresh during reconnect")
	}
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

func fetchPendingStartURL(t *testing.T, harness *oauthHarness) string {
	t.Helper()

	resp, err := http.Get(strings.TrimSuffix(harness.proxyURL, "/mcp/oauth-gateway/protected") + "/oauth/status")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	matches := regexp.MustCompile(`href="(http://[^"]+/oauth/start\?id=[^"]+)"`).FindStringSubmatch(string(body))
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}
