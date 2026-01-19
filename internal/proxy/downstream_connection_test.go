package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
	"github.com/CentianAI/centian-cli/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestDownstreamConnect(t *testing.T) {
	// Given: a server config
	downstreamURL := "https://api.githubcopilot.com/mcp/"
	githubPAT := os.Getenv("GITHUB_PAT")
	if githubPAT == "" {
		t.Skip("GITHUB_PAT environment variable not set - skipping local GitHub test")
	}
	testConf := config.MCPServerConfig{
		Name: "my-test-mcp-1",
		URL:  downstreamURL,
		Headers: map[string]string{
			"Authorization": fmt.Sprintf("Bearer %s", githubPAT),
		},
	}

	dsc := NewDownstreamConnection("testServer1", &testConf)
	ctx := context.Background()
	err := dsc.Connect(ctx, map[string]string{})
	assert.NilError(t, err)

	tools := dsc.Tools()
	assert.Assert(t, len(tools) > 0)

	for _, tool := range tools {
		fmt.Printf("tool: %s\n", tool.Name)
	}
}

func TestAggragetedGatewaySimpleMcpRequest(t *testing.T) {
	downstreamURL := "https://api.githubcopilot.com/mcp/"
	githubPAT := os.Getenv("GITHUB_PAT")
	if githubPAT == "" {
		t.Skip("GITHUB_PAT environment variable not set - skipping local GitHub test")
	}
	gatewayName := "my-test-gateway"
	serverName := "my-test-mcp-1"
	serverName2 := "my-test-mcp-2"

	gwConfig := config.GatewayConfig{
		MCPServers: map[string]*config.MCPServerConfig{
			serverName: {
				URL: downstreamURL,
				Headers: map[string]string{
					"Authorization": fmt.Sprintf("Bearer %s", githubPAT),
				},
				Enabled: true,
			},
			serverName2: {
				URL:     "https://mcp.deepwiki.com/mcp",
				Headers: map[string]string{
					//"Authorization": fmt.Sprintf("Bearer %s", githubPAT),
				},
				Enabled: true,
			},
			"local-memory": {
				Command: "npx", // tests cross-transport feature
				Args: []string{
					"-y",
					"@modelcontextprotocol/server-memory",
				},
				Enabled: true,
			},
		},
	}

	globalConfig := &config.GlobalConfig{
		Name:    "Test Proxy Server",
		Version: "1.0.0",
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 30,
		},
		Gateways: map[string]*config.GatewayConfig{
			gatewayName: &gwConfig,
		},
	}
	go func() {
		log.Printf("Creating proxy: %#v", globalConfig)
		server, err := NewCentianProxy(globalConfig)
		if err != nil {
			log.Fatal("Unable to create proxy server:", err)
		}
		server.Setup()
		// gw.RegisterHandler(server.Mux) // registers an /all handler
		if err := server.Server.ListenAndServe(); err != nil {
			log.Fatal("Unable to start proxy server:", err)
		}
	}()

	// Wait for server to start.
	time.Sleep(3 * time.Second)
	ctx := context.Background()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	proxyURL := fmt.Sprintf(
		"http://localhost:%s/mcp/%s", // connecting to /all endpoint
		globalConfig.Proxy.Port,
		gatewayName,
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
	defer func() {
		err := session.Close()
		if err != nil {
			common.LogError(err.Error())
		}
	}()

	log.Println("✅ Connected to proxy server")

	// Test 1: List tools (this request goes through proxy to GitHub).
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
}

func TestMultiRequestsOnIndividualServerOfGateway(t *testing.T) {
	downstreamURL := "https://api.githubcopilot.com/mcp/"
	githubPAT := os.Getenv("GITHUB_PAT")
	if githubPAT == "" {
		t.Skip("GITHUB_PAT environment variable not set - skipping local GitHub test")
	}
	gatewayName := "my-test-gateway"
	serverName := "my-test-mcp-1"
	serverName2 := "my-test-mcp-2"

	gwConfig := config.GatewayConfig{
		MCPServers: map[string]*config.MCPServerConfig{
			serverName: {
				URL: downstreamURL,
				Headers: map[string]string{
					"Authorization": fmt.Sprintf("Bearer %s", githubPAT),
				},
				Enabled: true,
			},
			serverName2: {
				URL:     "https://mcp.deepwiki.com/mcp",
				Headers: map[string]string{
					//"Authorization": fmt.Sprintf("Bearer %s", githubPAT),
				},
				Enabled: true,
			},
			"local-memory": {
				Command: "npx", // tests cross-transport feature
				Args: []string{
					"-y",
					"@modelcontextprotocol/server-memory",
				},
				Enabled: true,
			},
		},
	}

	globalConfig := &config.GlobalConfig{
		Name:    "Test Proxy Server",
		Version: "1.0.0",
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 30,
		},
		Gateways: map[string]*config.GatewayConfig{
			gatewayName: &gwConfig,
		},
	}
	go func() {
		log.Printf("Creating proxy: %#v", globalConfig)
		server, err := NewCentianProxy(globalConfig)
		if err != nil {
			log.Fatal("Unable to create proxy server:", err)
		}
		server.Setup()
		// gw.RegisterHandler(server.Mux) // registers an /all handler
		if err := server.Server.ListenAndServe(); err != nil {
			log.Fatal("Unable to start proxy server:", err)
		}
	}()

	// Wait for server to start.
	time.Sleep(3 * time.Second)

	for serverName, _ := range gwConfig.MCPServers {
		ctx := context.Background()

		client := mcp.NewClient(&mcp.Implementation{
			Name:    "test-client",
			Version: "1.0.0",
		}, nil)

		proxyURL := fmt.Sprintf(
			"http://localhost:%s/mcp/%s/%s", // connecting to /all endpoint
			globalConfig.Proxy.Port,
			gatewayName,
			serverName,
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
		defer func() {
			err := session.Close()
			if err != nil {
				common.LogError(err.Error())
			}
		}()

		log.Printf("✅ Connected to proxy server: %s\n", serverName)

		// Test 1: List tools (this request goes through proxy to GitHub).
		log.Println("\n=== Test 1: List Tools (via proxy) ===")
		toolsResult, err := session.ListTools(ctx, nil)
		if err != nil {
			t.Fatalf("Failed to list tools: %v", err)
		}
		toolCount := len(toolsResult.Tools)
		assert.Assert(t, toolCount > 0)
		log.Printf("✅ Received %d tools from GitHub (via proxy):", toolCount)
	}
}
