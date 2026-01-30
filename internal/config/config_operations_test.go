package config

import (
	"os"
	"testing"
)

// TestSearchServerByName tests the SearchServerByName functionality.
func TestSearchServerByName(t *testing.T) {
	tests := []struct {
		name            string
		config          *GlobalConfig
		searchName      string
		expectedCount   int
		expectedGateway string
	}{
		{
			name: "single server found in one gateway",
			config: &GlobalConfig{
				Gateways: map[string]*GatewayConfig{
					"gateway1": {
						MCPServers: map[string]*MCPServerConfig{
							"server1": {Name: "server1", Command: "node"},
						},
					},
				},
			},
			searchName:      "server1",
			expectedCount:   1,
			expectedGateway: "gateway1",
		},
		{
			name: "server found in multiple gateways",
			config: &GlobalConfig{
				Gateways: map[string]*GatewayConfig{
					"gateway1": {
						MCPServers: map[string]*MCPServerConfig{
							"server1": {Name: "server1", Command: "node"},
						},
					},
					"gateway2": {
						MCPServers: map[string]*MCPServerConfig{
							"server1": {Name: "server1", Command: "python"},
						},
					},
				},
			},
			searchName:    "server1",
			expectedCount: 2,
		},
		{
			name: "server not found",
			config: &GlobalConfig{
				Gateways: map[string]*GatewayConfig{
					"gateway1": {
						MCPServers: map[string]*MCPServerConfig{
							"server1": {Name: "server1", Command: "node"},
						},
					},
				},
			},
			searchName:    "nonexistent",
			expectedCount: 0,
		},
		{
			name: "empty gateways",
			config: &GlobalConfig{
				Gateways: map[string]*GatewayConfig{},
			},
			searchName:    "server1",
			expectedCount: 0,
		},
		{
			name: "nil gateways",
			config: &GlobalConfig{
				Gateways: nil,
			},
			searchName:    "server1",
			expectedCount: 0,
		},
		{
			name: "gateway with no servers",
			config: &GlobalConfig{
				Gateways: map[string]*GatewayConfig{
					"gateway1": {
						MCPServers: map[string]*MCPServerConfig{},
					},
				},
			},
			searchName:    "server1",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a config with specific gateway and server setup.

			// When: searching for a server by name.
			results := tt.config.SearchServerByName(tt.searchName)

			// Then: verify the expected number of results.
			if len(results) != tt.expectedCount {
				t.Errorf("Expected %d results, got %d", tt.expectedCount, len(results))
			}

			// Then: verify gateway name if single result expected.
			if tt.expectedCount == 1 && tt.expectedGateway != "" {
				if results[0].gatewayName != tt.expectedGateway {
					t.Errorf("Expected gateway '%s', got '%s'", tt.expectedGateway, results[0].gatewayName)
				}
			}

			// Then: verify all results have matching server names.
			for _, result := range results {
				if result.server.Name != tt.searchName {
					t.Errorf("Expected server name '%s', got '%s'", tt.searchName, result.server.Name)
				}
			}
		})
	}
}

// TestGetSubstitutedHeaders tests environment variable substitution in headers.
func TestGetSubstitutedHeaders(t *testing.T) {
	// Setup test environment variables.
	os.Setenv("TEST_TOKEN", "secret_token_123")
	os.Setenv("API_KEY", "api_key_456")
	os.Setenv("EMPTY_VAR", "")
	defer func() {
		os.Unsetenv("TEST_TOKEN")
		os.Unsetenv("API_KEY")
		os.Unsetenv("EMPTY_VAR")
	}()

	tests := []struct {
		name     string
		server   *MCPServerConfig
		expected map[string]string
	}{
		{
			name: "substitute single env var with braces",
			server: &MCPServerConfig{
				Headers: map[string]string{
					"Authorization": "Bearer ${TEST_TOKEN}",
				},
			},
			expected: map[string]string{
				"Authorization": "Bearer secret_token_123",
			},
		},
		{
			name: "substitute single env var without braces",
			server: &MCPServerConfig{
				Headers: map[string]string{
					"Authorization": "Bearer $TEST_TOKEN",
				},
			},
			expected: map[string]string{
				"Authorization": "Bearer secret_token_123",
			},
		},
		{
			name: "substitute multiple env vars",
			server: &MCPServerConfig{
				Headers: map[string]string{
					"Authorization": "Bearer ${TEST_TOKEN}",
					"X-API-Key":     "${API_KEY}",
				},
			},
			expected: map[string]string{
				"Authorization": "Bearer secret_token_123",
				"X-API-Key":     "api_key_456",
			},
		},
		{
			name: "non-existent env var becomes empty",
			server: &MCPServerConfig{
				Headers: map[string]string{
					"Authorization": "Bearer ${NONEXISTENT_VAR}",
				},
			},
			expected: map[string]string{
				"Authorization": "Bearer ",
			},
		},
		{
			name: "empty env var substitution",
			server: &MCPServerConfig{
				Headers: map[string]string{
					"Authorization": "Bearer ${EMPTY_VAR}",
				},
			},
			expected: map[string]string{
				"Authorization": "Bearer ",
			},
		},
		{
			name: "no substitution needed",
			server: &MCPServerConfig{
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
			expected: map[string]string{
				"Content-Type": "application/json",
			},
		},
		{
			name: "nil headers",
			server: &MCPServerConfig{
				Headers: nil,
			},
			expected: map[string]string{},
		},
		{
			name: "empty headers",
			server: &MCPServerConfig{
				Headers: map[string]string{},
			},
			expected: map[string]string{},
		},
		{
			name: "mixed substitution and literals",
			server: &MCPServerConfig{
				Headers: map[string]string{
					"Authorization": "Bearer ${TEST_TOKEN}",
					"Content-Type":  "application/json",
					"X-Custom":      "prefix-${API_KEY}-suffix",
				},
			},
			expected: map[string]string{
				"Authorization": "Bearer secret_token_123",
				"Content-Type":  "application/json",
				"X-Custom":      "prefix-api_key_456-suffix",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a server config with headers containing env vars.

			// When: getting substituted headers.
			result := tt.server.GetSubstitutedHeaders()

			// Then: verify all expected headers are present.
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d headers, got %d", len(tt.expected), len(result))
			}

			// Then: verify each header value matches expected.
			for key, expectedValue := range tt.expected {
				if actualValue, ok := result[key]; !ok {
					t.Errorf("Expected header '%s' not found", key)
				} else if actualValue != expectedValue {
					t.Errorf("Header '%s': expected '%s', got '%s'", key, expectedValue, actualValue)
				}
			}
		})
	}
}

// TestGatewayListServers tests the ListServers method on GatewayConfig.
func TestGatewayListServers(t *testing.T) {
	tests := []struct {
		name          string
		gateway       *GatewayConfig
		expectedCount int
		expectedNames []string
	}{
		{
			name: "multiple servers",
			gateway: &GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": {Name: "server1", Command: "node"},
					"server2": {Name: "server2", Command: "python"},
					"server3": {Name: "server3", Command: "go"},
				},
			},
			expectedCount: 3,
			expectedNames: []string{"server1", "server2", "server3"},
		},
		{
			name: "single server",
			gateway: &GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": {Name: "server1", Command: "node"},
				},
			},
			expectedCount: 1,
			expectedNames: []string{"server1"},
		},
		{
			name: "no servers",
			gateway: &GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{},
			},
			expectedCount: 0,
			expectedNames: []string{},
		},
		{
			name: "nil servers map",
			gateway: &GatewayConfig{
				MCPServers: nil,
			},
			expectedCount: 0,
			expectedNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a gateway with specific servers.

			// When: listing all servers.
			result := tt.gateway.ListServers()

			// Then: verify the count matches expected.
			if len(result) != tt.expectedCount {
				t.Errorf("Expected %d servers, got %d", tt.expectedCount, len(result))
			}

			// Then: verify all expected server names are present.
			foundNames := make(map[string]bool)
			for _, server := range result {
				foundNames[server.Name] = true
			}

			for _, expectedName := range tt.expectedNames {
				if !foundNames[expectedName] {
					t.Errorf("Expected server '%s' not found in results", expectedName)
				}
			}
		})
	}
}

// TestGatewayAddServer tests the AddServer method on GatewayConfig.
func TestGatewayAddServer(t *testing.T) {
	disabled := false
	tests := []struct {
		name           string
		initialServers map[string]*MCPServerConfig
		addName        string
		addServer      *MCPServerConfig
		expectedCount  int
	}{
		{
			name:           "add server to empty gateway",
			initialServers: map[string]*MCPServerConfig{},
			addName:        "server1",
			addServer: &MCPServerConfig{
				Name:    "server1",
				Command: "node",
			},
			expectedCount: 1,
		},
		{
			name: "add server to gateway with existing servers",
			initialServers: map[string]*MCPServerConfig{
				"server1": {Name: "server1", Command: "node"},
			},
			addName: "server2",
			addServer: &MCPServerConfig{
				Name:    "server2",
				Command: "python",
			},
			expectedCount: 2,
		},
		{
			name: "overwrite existing server with same name",
			initialServers: map[string]*MCPServerConfig{
				"server1": {Name: "server1", Command: "node"},
			},
			addName: "server1",
			addServer: &MCPServerConfig{
				Name:    "server1",
				Command: "python",
				Enabled: &disabled,
			},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a gateway with initial servers.
			gateway := &GatewayConfig{
				MCPServers: tt.initialServers,
			}

			// When: adding a new server.
			gateway.AddServer(tt.addName, tt.addServer)

			// Then: verify the server count.
			if len(gateway.MCPServers) != tt.expectedCount {
				t.Errorf("Expected %d servers, got %d", tt.expectedCount, len(gateway.MCPServers))
			}

			// Then: verify the server was added correctly.
			addedServer, exists := gateway.MCPServers[tt.addName]
			if !exists {
				t.Errorf("Server '%s' was not added", tt.addName)
			} else {
				if addedServer.Name != tt.addServer.Name {
					t.Errorf("Expected server name '%s', got '%s'", tt.addServer.Name, addedServer.Name)
				}
				if addedServer.Command != tt.addServer.Command {
					t.Errorf("Expected command '%s', got '%s'", tt.addServer.Command, addedServer.Command)
				}
			}
		})
	}
}

// TestGatewayRemoveServer tests the RemoveServer method on GatewayConfig.
func TestGatewayRemoveServer(t *testing.T) {
	tests := []struct {
		name           string
		initialServers map[string]*MCPServerConfig
		removeName     string
		expectedCount  int
		shouldExist    bool
	}{
		{
			name: "remove existing server",
			initialServers: map[string]*MCPServerConfig{
				"server1": {Name: "server1", Command: "node"},
				"server2": {Name: "server2", Command: "python"},
			},
			removeName:    "server1",
			expectedCount: 1,
			shouldExist:   false,
		},
		{
			name: "remove non-existent server (no-op)",
			initialServers: map[string]*MCPServerConfig{
				"server1": {Name: "server1", Command: "node"},
			},
			removeName:    "nonexistent",
			expectedCount: 1,
			shouldExist:   false,
		},
		{
			name: "remove last server",
			initialServers: map[string]*MCPServerConfig{
				"server1": {Name: "server1", Command: "node"},
			},
			removeName:    "server1",
			expectedCount: 0,
			shouldExist:   false,
		},
		{
			name:           "remove from empty gateway",
			initialServers: map[string]*MCPServerConfig{},
			removeName:     "server1",
			expectedCount:  0,
			shouldExist:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a gateway with initial servers.
			gateway := &GatewayConfig{
				MCPServers: tt.initialServers,
			}

			// When: removing a server.
			gateway.RemoveServer(tt.removeName)

			// Then: verify the server count.
			if len(gateway.MCPServers) != tt.expectedCount {
				t.Errorf("Expected %d servers, got %d", tt.expectedCount, len(gateway.MCPServers))
			}

			// Then: verify the server was removed.
			_, exists := gateway.MCPServers[tt.removeName]
			if exists != tt.shouldExist {
				if tt.shouldExist {
					t.Errorf("Expected server '%s' to exist, but it doesn't", tt.removeName)
				} else {
					t.Errorf("Expected server '%s' to be removed, but it still exists", tt.removeName)
				}
			}
		})
	}
}

// TestGatewayHasServer tests the HasServer method on GatewayConfig.
func TestGatewayHasServer(t *testing.T) {
	tests := []struct {
		name       string
		gateway    *GatewayConfig
		serverName string
		expected   bool
	}{
		{
			name: "server exists",
			gateway: &GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": {Name: "server1", Command: "node"},
					"server2": {Name: "server2", Command: "python"},
				},
			},
			serverName: "server1",
			expected:   true,
		},
		{
			name: "server does not exist",
			gateway: &GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": {Name: "server1", Command: "node"},
				},
			},
			serverName: "nonexistent",
			expected:   false,
		},
		{
			name: "empty gateway",
			gateway: &GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{},
			},
			serverName: "server1",
			expected:   false,
		},
		{
			name: "nil servers map",
			gateway: &GatewayConfig{
				MCPServers: nil,
			},
			serverName: "server1",
			expected:   false,
		},
		{
			name: "case sensitive check",
			gateway: &GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"Server1": {Name: "Server1", Command: "node"},
				},
			},
			serverName: "server1",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a gateway with specific servers.

			// When: checking if a server exists.
			result := tt.gateway.HasServer(tt.serverName)

			// Then: verify the result matches expected.
			if result != tt.expected {
				t.Errorf("Expected HasServer('%s') to return %v, got %v", tt.serverName, tt.expected, result)
			}
		})
	}
}

// TestGatewayServerOperationsIntegration tests a complete workflow.
func TestGatewayServerOperationsIntegration(t *testing.T) {
	// Given: an empty gateway.
	gateway := &GatewayConfig{
		MCPServers: make(map[string]*MCPServerConfig),
	}

	// When: adding multiple servers.
	disabled := false
	server1 := &MCPServerConfig{Name: "server1", Command: "node"}
	server2 := &MCPServerConfig{Name: "server2", Command: "python"}
	server3 := &MCPServerConfig{Name: "server3", Command: "go", Enabled: &disabled}

	gateway.AddServer("server1", server1)
	gateway.AddServer("server2", server2)
	gateway.AddServer("server3", server3)

	// Then: verify all servers were added.
	if len(gateway.MCPServers) != 3 {
		t.Fatalf("Expected 3 servers, got %d", len(gateway.MCPServers))
	}

	// When: checking for server existence.
	if !gateway.HasServer("server1") {
		t.Error("Expected server1 to exist")
	}
	if !gateway.HasServer("server2") {
		t.Error("Expected server2 to exist")
	}
	if !gateway.HasServer("server3") {
		t.Error("Expected server3 to exist")
	}
	if gateway.HasServer("nonexistent") {
		t.Error("Expected nonexistent server to not exist")
	}

	// When: listing servers.
	servers := gateway.ListServers()
	if len(servers) != 3 {
		t.Errorf("Expected 3 servers in list, got %d", len(servers))
	}

	// When: removing a server.
	gateway.RemoveServer("server2")

	// Then: verify server was removed.
	if gateway.HasServer("server2") {
		t.Error("Expected server2 to be removed")
	}
	if len(gateway.MCPServers) != 2 {
		t.Errorf("Expected 2 servers after removal, got %d", len(gateway.MCPServers))
	}

	// When: updating an existing server.
	updatedServer1 := &MCPServerConfig{Name: "server1", Command: "deno"}
	gateway.AddServer("server1", updatedServer1)

	// Then: verify server was updated.
	if server := gateway.MCPServers["server1"]; server.Command != "deno" {
		t.Errorf("Expected server1 command to be 'deno', got '%s'", server.Command)
	}
	if len(gateway.MCPServers) != 2 {
		t.Errorf("Expected 2 servers after update, got %d", len(gateway.MCPServers))
	}
}
