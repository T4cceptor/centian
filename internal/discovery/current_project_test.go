package discovery

import (
	"os"
	"testing"
)

func TestCurrentProjectVSCodeConfig(t *testing.T) {
	// Test using actual config file copied to test_configs
	// This test validates that all 4 servers are discovered correctly
	filePath := "../../test_configs/current_project_mcp.json"

	// Read the test config file
	testData, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read test config file: %v", err)
	}

	servers, err := parseVSCodeConfig(testData, filePath)
	if err != nil {
		t.Fatalf("Failed to parse current project VS Code config: %v", err)
	}

	// Should find all 4 servers
	expectedCount := 4
	if len(servers) != expectedCount {
		t.Errorf("Expected %d servers, got %d", expectedCount, len(servers))
	}

	// Validate transport types
	stdioCount := 0
	httpCount := 0
	for _, server := range servers {
		switch server.Transport {
		case "stdio":
			stdioCount++
		case "http":
			httpCount++
		}
	}

	// Expected: 1 stdio, 3 http
	expectedStdio := 1
	expectedHTTP := 3

	if stdioCount != expectedStdio {
		t.Errorf("Expected %d stdio servers, got %d", expectedStdio, stdioCount)
	}

	if httpCount != expectedHTTP {
		t.Errorf("Expected %d http servers, got %d", expectedHTTP, httpCount)
	}

	// Verify specific servers exist
	serverMap := make(map[string]DiscoveredServer)
	for _, server := range servers {
		serverMap[server.Name] = server
	}

	expectedServers := []string{"mcp-proxy-1", "mcp-proxy-2-2", "mcp-proxy-3-3", "mcp_proxy-stdio-4"}
	for _, expected := range expectedServers {
		if _, exists := serverMap[expected]; !exists {
			t.Errorf("Expected server '%s' not found", expected)
		}
	}
}