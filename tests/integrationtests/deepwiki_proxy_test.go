package integrationtests

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/proxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestDeepWikiHTTPProxyWithSDKClient verifies proxy behavior against DeepWiki's
// public MCP endpoint. This is an external integration test and is opt-in
// because it depends on network reachability and third-party availability.
func TestDeepWikiHTTPProxyWithSDKClient(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping external integration test in short mode")
	}
	if os.Getenv("CENTIAN_RUN_EXTERNAL_INTEGRATION") != "1" {
		t.Skip("Set CENTIAN_RUN_EXTERNAL_INTEGRATION=1 to run external integration tests")
	}

	downstreamURL := "https://mcp.deepwiki.com/mcp"

	authDisabled := false
	globalConfig := &config.GlobalConfig{
		Name:        "Test Proxy Server",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9202",
			Timeout: 30,
		},
		Gateways: map[string]*config.GatewayConfig{
			"public-gateway": {
				MCPServers: map[string]*config.MCPServerConfig{
					"deepwiki": {
						URL: downstreamURL,
					},
				},
			},
		},
	}

	server, err := proxy.NewCentianProxy(globalConfig)
	if err != nil {
		t.Fatal("Unable to create proxy server:", err)
	}
	if setupErr := server.Setup(); setupErr != nil {
		t.Fatal("failed to setup centian server:", setupErr)
	}
	go func() {
		if err := server.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Server error: %v", err)
		}
	}()

	time.Sleep(5 * time.Second)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Server.Shutdown(ctx)
	}()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "centian-test-client",
		Version: "1.0.0",
	}, nil)

	proxyURL := fmt.Sprintf(
		"http://localhost:%s/mcp/%s/%s",
		globalConfig.Proxy.Port,
		"public-gateway",
		"deepwiki",
	)
	log.Printf("Connecting to proxy server at %s", proxyURL)

	session, err := client.Connect(
		ctx,
		&mcp.StreamableClientTransport{
			Endpoint:   proxyURL,
			HTTPClient: &http.Client{Timeout: 30 * time.Second},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to connect to proxy: %v", err)
	}
	defer session.Close()

	log.Println("Connected to proxy server successfully")

	log.Println("Waiting for DeepWiki tools to become available")
	toolsWaitStart := time.Now()
	toolsResult, err := waitForTools(ctx, session, 30*time.Second, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to list tools before timeout: %v", err)
	}
	log.Printf("Tools became available after %s", time.Since(toolsWaitStart))

	if len(toolsResult.Tools) == 0 {
		t.Fatal("Expected at least one tool from DeepWiki")
	}

	params := &mcp.CallToolParams{
		Name: "ask_question",
		Arguments: map[string]any{
			"repoName": "modelcontextprotocol/servers",
			"question": "What is the Model Context Protocol?",
		},
	}

	found := false
	for _, tool := range toolsResult.Tools {
		if tool.Name == params.Name {
			found = true
			break
		}
	}
	if !found {
		t.Skip("ask_question tool not available from DeepWiki")
	}

	res, err := session.CallTool(ctx, params)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if res.IsError {
		t.Fatal("Tool call resulted in error")
	}
	if len(res.Content) == 0 {
		t.Fatal("Expected content in tool response, got none")
	}
}

func waitForTools(
	ctx context.Context,
	session *mcp.ClientSession,
	timeout time.Duration,
	interval time.Duration,
) (*mcp.ListToolsResult, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		toolsResult, err := session.ListTools(ctx, nil)
		if err == nil && len(toolsResult.Tools) > 0 {
			return toolsResult, nil
		}
		if err != nil {
			lastErr = err
		}
		time.Sleep(interval)
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return &mcp.ListToolsResult{}, nil
}
