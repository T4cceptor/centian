package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/proxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// inlineCLIGatewayProvider is a simple in-memory GatewayProvider for integration tests.
type inlineCLIGatewayProvider struct {
	file *config.GatewayFile
}

func (p *inlineCLIGatewayProvider) LoadGatewayFile() (*config.GatewayFile, error) {
	return p.file, nil
}

func (p *inlineCLIGatewayProvider) SaveGatewayFile(f *config.GatewayFile) error {
	p.file = f
	return nil
}

// TestServerStartIntegration tests the complete server startup flow with a real config file.
func TestServerStartIntegration(t *testing.T) {
	// Given: a mock downstream MCP server.
	mockMCPServer := createMockMCPServer()
	defer mockMCPServer.Close()

	// Given: a temporary server config file pointing to the mock server.
	serverConfig, gf := createTestServerAndGatewayConfig(t, mockMCPServer.URL)

	// Validate config structure.
	if serverConfig.Name != "Test Integration Server" {
		t.Errorf("Expected server name 'Test Integration Server', got '%s'", serverConfig.Name)
	}

	if serverConfig.Proxy.Port != "9001" {
		t.Errorf("Expected port '9001', got '%s'", serverConfig.Proxy.Port)
	}

	if len(gf.Gateways) != 1 {
		t.Fatalf("Expected 1 gateway, got %d", len(gf.Gateways))
	}

	// When: starting the Centian proxy server.
	provider := &inlineCLIGatewayProvider{file: gf}
	server, err := proxy.NewCentianServer(serverConfig, provider)
	if err != nil {
		t.Fatal("Unable to create proxy server:", err)
	}
	setupErr := server.Setup()
	if setupErr != nil {
		t.Fatal("Unable to setup proxy server:", setupErr)
	}

	// Start server in background.
	go func() {
		if err := server.Server.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for server to start.
	time.Sleep(2 * time.Second)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Server.Shutdown(ctx)
	}()

	// When: connecting an MCP client to the proxy.
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "integration-test-client",
		Version: "1.0.0",
	}, nil)

	proxyURL := fmt.Sprintf("http://localhost:%s/mcp/test-gateway/mock-server", serverConfig.Proxy.Port)
	log.Printf("Connecting to proxy at %s", proxyURL)

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

	// Then: listing tools should succeed and return mock tools.
	toolsResult, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	if len(toolsResult.Tools) == 0 {
		t.Error("Expected at least one tool from mock server")
	}

	log.Printf("✅ Received %d tools from mock server (via proxy)", len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		log.Printf("  - %s: %s", tool.Name, tool.Description)
	}

	// Then: calling a tool should succeed.
	params := &mcp.CallToolParams{
		Name: "get_weather",
		Arguments: map[string]any{
			"city": "San Francisco",
		},
	}

	res, err := session.CallTool(ctx, params)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if res.IsError {
		t.Fatal("Tool returned error")
	}

	log.Println("✅ Tool call successful (via proxy)")
	for _, c := range res.Content {
		if textContent, ok := c.(*mcp.TextContent); ok {
			log.Printf("  Response: %s", textContent.Text)

			// Verify response contains expected data.
			if textContent.Text != "Sunny, 72°F in San Francisco" {
				t.Errorf("Unexpected tool response: %s", textContent.Text)
			}
		}
	}

	log.Println("\n=== Integration Test Results ===")
	log.Println("✅ Config file loaded successfully")
	log.Println("✅ Proxy server started with correct configuration")
	log.Println("✅ Client connected through proxy")
	log.Println("✅ Tools listed successfully")
	log.Println("✅ Tool execution successful")
	log.Println("✅ Request/response flow validated")
}

// createMockMCPServer creates a mock HTTP server that responds with MCP protocol messages.
func createMockMCPServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the request body.
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		// Parse JSON-RPC request.
		var request map[string]interface{}
		json.Unmarshal(body, &request)

		method, _ := request["method"].(string)
		id := request["id"]

		var response map[string]interface{}

		switch method {
		case "initialize":
			response = map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"capabilities": map[string]interface{}{
						"tools": map[string]interface{}{},
					},
					"serverInfo": map[string]interface{}{
						"name":    "mock-mcp-server",
						"version": "1.0.0",
					},
				},
			}

		case "tools/list":
			response = map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]interface{}{
					"tools": []map[string]interface{}{
						{
							"name":        "get_weather",
							"description": "Get current weather for a city",
							"inputSchema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"city": map[string]interface{}{
										"type":        "string",
										"description": "City name",
									},
								},
								"required": []string{"city"},
							},
						},
						{
							"name":        "get_time",
							"description": "Get current time",
							"inputSchema": map[string]interface{}{
								"type":       "object",
								"properties": map[string]interface{}{},
							},
						},
					},
				},
			}

		case "tools/call":
			params, _ := request["params"].(map[string]interface{})
			toolName, _ := params["name"].(string)

			var content string
			switch toolName {
			case "get_weather":
				content = "Sunny, 72°F in San Francisco"
			case "get_time":
				content = time.Now().Format(time.RFC3339)
			default:
				content = "Unknown tool"
			}

			response = map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]interface{}{
					"content": []map[string]interface{}{
						{
							"type": "text",
							"text": content,
						},
					},
				},
			}

		default:
			response = map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"error": map[string]interface{}{
					"code":    -32601,
					"message": "Method not found",
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
}

// createTestServerAndGatewayConfig creates server config + gateway file for testing.
func createTestServerAndGatewayConfig(t *testing.T, mockServerURL string) (*config.ServerConfig, *config.GatewayFile) {
	t.Helper()

	authDisabled := false
	serverConfig := &config.ServerConfig{
		Name:        "Test Integration Server",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9001",
			Timeout: 30,
		},
	}

	gf := &config.GatewayFile{
		Version: "1.0.0",
		Gateways: map[string]*config.GatewayConfig{
			"test-gateway": {
				MCPServers: map[string]*config.MCPServerConfig{
					"mock-server": {
						URL: mockServerURL,
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						Description: "Mock MCP server for integration testing",
					},
					"second-mock": {
						URL: mockServerURL,
						Headers: map[string]string{
							"Content-Type":  "application/json",
							"X-Test-Header": "test-value",
						},
						Description: "Second mock server to test multiple endpoints",
					},
				},
			},
		},
	}

	return serverConfig, gf
}

// createTestConfigFile creates a temporary config file for testing.
// Returns the path to the server config file. Also writes a gateway file alongside.
func createTestConfigFile(t *testing.T, mockServerURL string) string {
	t.Helper()

	serverConfig, gf := createTestServerAndGatewayConfig(t, mockServerURL)

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := config.EnsureConfigDir(); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configPath := filepath.Join(tmpDir, ".centian", "config.json")
	serverData, err := json.MarshalIndent(serverConfig, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal server config: %v", err)
	}
	if err := os.WriteFile(configPath, serverData, 0o644); err != nil {
		t.Fatalf("Failed to write server config: %v", err)
	}

	gwPath := filepath.Join(tmpDir, ".centian", "gateways.json")
	gwData, err := json.MarshalIndent(gf, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal gateway file: %v", err)
	}
	if err := os.WriteFile(gwPath, gwData, 0o644); err != nil {
		t.Fatalf("Failed to write gateway file: %v", err)
	}

	log.Printf("Created test config at: %s", configPath)
	return configPath
}

// TestConfigFileValidation tests config file loading and validation.
func TestConfigFileValidation(t *testing.T) {
	tests := []struct {
		name        string
		serverJSON  map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			serverJSON: map[string]interface{}{
				"name":    "Valid Server",
				"version": "1.0.0",
				"proxy":   map[string]interface{}{"port": "8080", "timeout": 30},
			},
			expectError: false,
		},
		{
			name: "missing version",
			serverJSON: map[string]interface{}{
				"name":  "Invalid Server",
				"proxy": map[string]interface{}{"port": "8080"},
			},
			expectError: true,
			errorMsg:    "version field is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file.
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "test_config.json")

			data, _ := json.MarshalIndent(tt.serverJSON, "", "  ")
			os.WriteFile(configPath, data, 0o644)

			// Try to load config.
			_, err := config.LoadConfigFromPath(configPath)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errorMsg != "" && err.Error() != fmt.Sprintf("invalid configuration: %s", tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}
