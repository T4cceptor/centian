package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestRawProxyWithSDKClient tests the raw proxy using MCP SDK client
func TestConfigurableHTTPProxyWithSDKClient(t *testing.T) {
	downstreamURL := "https://api.githubcopilot.com/mcp/"
	githubPAT := os.Getenv("GITHUB_PAT")
	if githubPAT == "" {
		log.Fatal("GITHUB_PAT environment variable not set")
	}
	proxyConfig := NewDefaultProxyConfig()
	proxyConfig.Port = "9000"
	centianConfig := CentianConfig{
		ProxyConfiguration: proxyConfig,
		Gateways: []GatewayConfig{
			{
				Name: "my-test-gateway",
				McpServers: map[string]HttpMcpServerConfig{
					"my-test-mcp-1": NewHTTPProxyConfig(downstreamURL, map[string]any{
						"Authorization": fmt.Sprintf("Bearer %s", githubPAT),
					}),
				},
			},
		},
	}
	// Start raw proxy server in background
	go func() {
		StartCentianServer(&centianConfig)
	}()

	// Wait for server to start
	time.Sleep(2 * time.Second)

	ctx := context.Background()

	// Create MCP client (this is what Claude Code/Desktop would use)
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	// Connect to proxy server (NOT directly to GitHub!)
	// This demonstrates that our raw proxy works with real MCP SDK clients
	proxyURL := fmt.Sprintf(
		"http://localhost:%s/mcp/%s/%s",
		proxyConfig.Port,
		centianConfig.Gateways[0].Name,
		"my-test-mcp-1",
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

	log.Println("✅ Connected to proxy server")

	// Test 1: List tools (this request goes through proxy to GitHub)
	log.Println("\n=== Test 1: List Tools (via proxy) ===")
	toolsResult, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	log.Printf("✅ Received %d tools from GitHub (via proxy):", len(toolsResult.Tools))
	for i, tool := range toolsResult.Tools {
		if i < 5 { // Show first 5 tools
			log.Printf("  - %s: %s", tool.Name, tool.Description)
		}
	}
	if len(toolsResult.Tools) > 5 {
		log.Printf("  ... and %d more tools", len(toolsResult.Tools)-5)
	}

	// Test 2: Call a tool (get issue #19 via proxy)
	log.Println("\n=== Test 2: Call Tool (get issue #19 via proxy) ===")
	params := &mcp.CallToolParams{
		Name: "issue_read",
		Arguments: map[string]any{
			"method":       "get",
			"owner":        "CentianAI",
			"repo":         "centian-cli",
			"issue_number": 19,
		},
	}

	res, err := session.CallTool(ctx, params)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if res.IsError {
		for _, c := range res.Content {
			log.Println(c)
		}
		t.Fatal("Tool returned error")
	}

	log.Println("✅ Tool call successful (via proxy)")
	for _, c := range res.Content {
		textContent, ok := c.(*mcp.TextContent)
		if ok {
			// Parse the JSON to extract just the title
			var issueData map[string]interface{}
			if err := json.Unmarshal([]byte(textContent.Text), &issueData); err == nil {
				if title, ok := issueData["title"].(string); ok {
					log.Printf("  Issue #19 title: %s", title)
				}
			}
		}
	}

	// Summary
	log.Println("\n=== POC Results ===")
	log.Println("✅ Raw proxy successfully forwarded requests to downstream")
	log.Println("✅ Headers (Authorization) passed through to downstream")
	log.Println("✅ Transparent JSON-RPC forwarding works")
	log.Println("✅ Compatible with MCP SDK clients")
	log.Println("")
	log.Println("KEY FINDING: This validates the 'forwardToDownstream' approach!")
	log.Println("  - We CAN do transparent forwarding")
	log.Println("  - Headers ARE passed through correctly")
	log.Println("  - SDK clients work with raw proxy")
	log.Println("")
	log.Println("NEXT STEP: Integrate SDK session management with this forwarding approach")
}
