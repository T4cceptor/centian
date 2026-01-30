package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/CentianAI/centian-cli/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestDeepWikiHTTPProxyWithSDKClient tests the HTTP proxy with DeepWiki's public MCP server.
// This test connects to a real public MCP endpoint and performs actual searches.
func TestDeepWikiHTTPProxyWithSDKClient(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test with external service in short mode")
	}

	downstreamURL := "https://mcp.deepwiki.com/mcp"

	// Given: a GlobalConfig with HTTP proxy settings pointing to DeepWiki.
	authDisabled := false
	globalConfig := &config.GlobalConfig{
		Name:        "Test Proxy Server",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9002",
			Timeout: 30,
		},
		Gateways: map[string]*config.GatewayConfig{
			"public-gateway": {
				MCPServers: map[string]*config.MCPServerConfig{
					"deepwiki": {
						URL: downstreamURL,
						// Headers: map[string]string{.
						// 	"Content-Type": "application/json",
						// },
					},
				},
			},
		},
	}

	// When: starting the proxy server in background.
	server, err := NewCentianProxy(globalConfig)
	if err != nil {
		t.Fatal("Unable to create proxy server:", err)
	}
	if setupErr := server.Setup(); setupErr != nil {
		t.Fatal("failed to setup centian server:", setupErr)
	}
	go func() {
		if err := server.Server.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for server to start.
	time.Sleep(5 * time.Second)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Server.Shutdown(ctx)
	}()

	ctx := context.Background()

	// When: creating an MCP client and connecting through the proxy.
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

	log.Println("✅ Connected to proxy server successfully")

	// Then: listing tools should return DeepWiki's available tools.
	log.Println("\n=== Test 1: List Available Tools ===")
	toolsResult, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	if len(toolsResult.Tools) == 0 {
		t.Fatal("Expected at least one tool from DeepWiki")
	}

	log.Printf("✅ Received %d tools from DeepWiki (via proxy):", len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		log.Printf("  - %s: %s", tool.Name, tool.Description)
	}

	// Then: calling a tool should return valid results.
	log.Println("\n=== Test 2: Ask Question about 'modelcontextprotocol/servers' ===")

	// Use ask_question tool with proper parameters.
	toolName := "ask_question"
	found := false
	for _, tool := range toolsResult.Tools {
		if tool.Name == "ask_question" {
			found = true
			break
		}
	}

	if !found {
		t.Skip("ask_question tool not available from DeepWiki")
	}

	params := &mcp.CallToolParams{
		Name: toolName,
		Arguments: map[string]any{
			"repoName": "modelcontextprotocol/servers",
			"question": "What is the Model Context Protocol?",
		},
	}

	res, err := session.CallTool(ctx, params)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if res.IsError {
		t.Log("Tool returned error response:")
		for _, c := range res.Content {
			t.Logf("  %v", c)
		}
		t.Fatal("Tool call resulted in error")
	}

	log.Println("✅ Tool call successful (via proxy)")

	// Verify we got content back.
	if len(res.Content) == 0 {
		t.Fatal("Expected content in tool response, got none")
	}

	for i, c := range res.Content {
		if textContent, ok := c.(*mcp.TextContent); ok {
			if len(textContent.Text) > 100 {
				log.Printf("  Response[%d] (first 100 chars): %s...", i, textContent.Text[:100])
			} else {
				log.Printf("  Response[%d]: %s", i, textContent.Text)
			}
		}
	}

	// Summary.
	log.Println("\n=== Test Results ===")
	log.Println("✅ Proxy successfully forwarded requests to DeepWiki")
	log.Println("✅ Tools list retrieved via proxy")
	log.Println("✅ Search query executed successfully")
	log.Println("✅ Valid response content received")
	log.Printf("✅ Validated proxy flow with public MCP server (DeepWiki)")
}
