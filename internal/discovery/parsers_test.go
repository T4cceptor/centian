package discovery

import (
	"testing"
)

func TestParseVSCodeConfig(t *testing.T) {
	// Test data matching the actual .vscode/mcp.json file
	testData := []byte(`{
  "inputs": [],
  "servers": {
    "mcp-proxy": {
      "headers": {
        "CONTEXT7_API_KEY": "ctx7sk-579d7ca0-4646-4a4d-b8b9-a45b321fc845"
      },
      "type": "http",
      "url": "http://localhost:8001/"
    },
    "mcp-proxy-2": {
      "headers": {
        "centian-url": "https://mcp.deepwiki.com/mcp"
      },
      "type": "http",
      "url": "http://localhost:8001"
    },
    "mcp-proxy-3": {
      "type": "http",
      "url": "https://mcp.deepwiki.com/mcp"
    },
    "mcp_proxy-stdio": {
      "type": "stdio",
      "command": "centian",
      "args": ["start"]
    }
  }
}`)

	filePath := "/test/.vscode/mcp.json"

	servers, err := parseVSCodeConfig(testData, filePath)
	if err != nil {
		t.Fatalf("Failed to parse VS Code config: %v", err)
	}

	// Should find all 4 servers
	expectedCount := 4
	if len(servers) != expectedCount {
		t.Errorf("Expected %d servers, got %d", expectedCount, len(servers))
		for i, server := range servers {
			t.Logf("Server %d: %s (Transport: %s, Command: %s, URL: %s)",
				i+1, server.Name, server.Transport, server.Command, server.URL)
		}
	}

	// Check each server
	serverMap := make(map[string]DiscoveredServer)
	for _, server := range servers {
		serverMap[server.Name] = server
	}

	// Test all servers
	testCases := []struct {
		name       string
		transport  string
		hasURL     bool
		hasHeaders bool
	}{
		{"mcp-proxy", "http", true, true},
		{"mcp-proxy-2", "http", true, true},
		{"mcp-proxy-3", "http", true, false},
		{"mcp_proxy-stdio", "stdio", false, false},
	}

	for _, tc := range testCases {
		server, exists := serverMap[tc.name]
		if !exists {
			t.Errorf("Server %s not found", tc.name)
			continue
		}

		if server.Transport != tc.transport {
			t.Errorf("Server %s: expected transport %s, got %s", tc.name, tc.transport, server.Transport)
		}

		if tc.hasURL && server.URL == "" {
			t.Errorf("Server %s: expected URL but got empty", tc.name)
		}

		if !tc.hasURL && server.URL != "" {
			t.Errorf("Server %s: expected no URL but got %s", tc.name, server.URL)
		}

		if tc.hasHeaders && len(server.Headers) == 0 {
			t.Errorf("Server %s: expected headers but got none", tc.name)
		}

		if !tc.hasHeaders && len(server.Headers) > 0 {
			t.Errorf("Server %s: expected no headers but got %v", tc.name, server.Headers)
		}
	}

	// Verify specific headers
	if server, exists := serverMap["mcp-proxy"]; exists {
		if server.Headers["CONTEXT7_API_KEY"] != "ctx7sk-579d7ca0-4646-4a4d-b8b9-a45b321fc845" {
			t.Errorf("mcp-proxy: wrong CONTEXT7_API_KEY header")
		}
	}

	if server, exists := serverMap["mcp-proxy-2"]; exists {
		if server.Headers["centian-url"] != "https://mcp.deepwiki.com/mcp" {
			t.Errorf("mcp-proxy-2: wrong centian-url header")
		}
	}
}

func TestParseVSCodeConfigSkipsInvalidServers(t *testing.T) {
	// Test data with servers that should be skipped
	testData := []byte(`{
  "servers": {
    "valid-http": {
      "type": "http",
      "url": "http://localhost:8000"
    },
    "valid-stdio": {
      "type": "stdio", 
      "command": "node",
      "args": ["server.js"]
    },
    "invalid-http-no-url": {
      "type": "http"
    },
    "invalid-stdio-no-command": {
      "type": "stdio",
      "args": ["some", "args"]
    },
    "invalid-no-type": {
      "command": "node"
    },
    "invalid-unknown-type": {
      "type": "websocket",
      "url": "ws://localhost:8000"
    }
  }
}`)

	filePath := "/test/.vscode/mcp.json"

	servers, err := parseVSCodeConfig(testData, filePath)
	if err != nil {
		t.Fatalf("Failed to parse VS Code config: %v", err)
	}

	// Should only find 2 valid servers
	expectedCount := 2
	if len(servers) != expectedCount {
		t.Errorf("Expected %d servers, got %d", expectedCount, len(servers))
		for i, server := range servers {
			t.Logf("Server %d: %s (Transport: %s, Command: %s, URL: %s)",
				i+1, server.Name, server.Transport, server.Command, server.URL)
		}
	}

	// Verify only valid servers are included
	validNames := []string{"valid-http", "valid-stdio"}
	for _, server := range servers {
		found := false
		for _, validName := range validNames {
			if server.Name == validName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Invalid server %s was included in results", server.Name)
		}
	}
}
