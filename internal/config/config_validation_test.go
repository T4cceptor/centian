package config

import (
	"testing"

	"github.com/CentianAI/centian-cli/internal/common"
)

// TestIsURLCompliant tests URL-safe name validation.
func TestIsURLCompliant(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid names.
		{"simple alphanumeric", "server123", true},
		{"with dash", "my-server", true},
		{"with underscore", "my_server", true},
		{"mixed valid chars", "server-1_test", true},
		{"starts with letter", "a123", true},
		{"starts with number", "1server", true},
		{"all uppercase", "SERVER", true},
		{"mixed case", "MyServer", true},
		{"long name", "very-long-server-name-with-many-parts_123", true},

		// Invalid names.
		{"empty string", "", false},
		{"starts with dash", "-server", false},
		{"starts with underscore", "_server", false},
		{"contains space", "my server", false},
		{"contains dot", "my.server", false},
		{"contains slash", "my/server", false},
		{"contains special chars", "server@123", false},
		{"contains unicode", "servér", false},
		{"only dashes", "---", false},
		{"only underscores", "___", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a name string.

			// When: checking if it's URL compliant.
			result := common.IsURLCompliant(tt.input)

			// Then: verify the result matches expected.
			if result != tt.expected {
				t.Errorf("isURLCompliant('%s') = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestIsValidHTTPURL tests HTTP/HTTPS URL validation.
func TestIsValidHTTPURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid URLs.
		{"simple http", "http://example.com", true},
		{"simple https", "https://example.com", true},
		{"with port", "http://example.com:8080", true},
		{"with path", "https://example.com/api/v1", true},
		{"with query", "https://example.com?key=value", true},
		{"with fragment", "https://example.com#section", true},
		{"localhost http", "http://localhost", true},
		{"localhost with port", "http://localhost:3000", true},
		{"IP address", "http://192.168.1.1", true},
		{"complex URL", "https://api.example.com:8443/v1/endpoint?param=value#section", true},

		// Invalid URLs.
		{"empty string", "", false},
		{"no scheme", "example.com", false},
		{"ftp scheme", "ftp://example.com", false},
		{"ws scheme", "ws://example.com", false},
		{"wss scheme", "wss://example.com", false},
		{"file scheme", "file:///path/to/file", false},
		{"no host", "http://", false},
		{"only scheme", "https://", false},
		{"malformed", "http:/ /example.com", false},
		{"relative path", "/api/endpoint", false},
		{"just path", "api/endpoint", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a URL string.

			// When: checking if it's a valid HTTP/HTTPS URL.
			result := isValidHTTPURL(tt.input)

			// Then: verify the result matches expected.
			if result != tt.expected {
				t.Errorf("isValidHTTPURL('%s') = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestValidateGateway tests gateway configuration validation.
func TestValidateGateway(t *testing.T) {
	tests := []struct {
		name      string
		gName     string
		gateway   GatewayConfig
		wantError bool
		errorMsg  string
	}{
		{
			name:  "valid gateway with stdio server",
			gName: "my-gateway",
			gateway: GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": {
						Name:    "server1",
						Command: "node",
						Enabled: true,
					},
				},
			},
			wantError: false,
		},
		{
			name:  "valid gateway with http server",
			gName: "my-gateway",
			gateway: GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": {
						Name:    "server1",
						URL:     "https://api.example.com",
						Enabled: true,
					},
				},
			},
			wantError: false,
		},
		{
			name:  "invalid gateway name with space",
			gName: "my gateway",
			gateway: GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": {
						Name:    "server1",
						Command: "node",
					},
				},
			},
			wantError: true,
			errorMsg:  "name must be URL-safe",
		},
		{
			name:  "invalid gateway name starting with dash",
			gName: "-gateway",
			gateway: GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": {
						Name:    "server1",
						Command: "node",
					},
				},
			},
			wantError: true,
			errorMsg:  "name must be URL-safe",
		},
		{
			name:  "gateway with no servers",
			gName: "gateway1",
			gateway: GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{},
			},
			wantError: true,
			errorMsg:  "must have at least one MCP server",
		},
		{
			name:  "gateway with nil servers",
			gName: "gateway1",
			gateway: GatewayConfig{
				MCPServers: nil,
			},
			wantError: true,
			errorMsg:  "must have at least one MCP server",
		},
		{
			name:  "gateway with multiple servers",
			gName: "gateway1",
			gateway: GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": {
						Name:    "server1",
						Command: "node",
					},
					"server2": {
						Name: "server2",
						URL:  "https://api.example.com",
					},
				},
			},
			wantError: false,
		},
		{
			name:  "gateway with valid processors",
			gName: "gateway1",
			gateway: GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": {
						Name:    "server1",
						Command: "node",
					},
				},
				Processors: []*ProcessorConfig{
					{
						Name:    "test-processor",
						Type:    "cli",
						Enabled: true,
						Timeout: 15,
						Config: map[string]interface{}{
							"command": "python",
						},
					},
				},
			},
			wantError: false,
		},
		{
			name:  "gateway with invalid processors",
			gName: "gateway1",
			gateway: GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": {
						Name:    "server1",
						Command: "node",
					},
				},
				Processors: []*ProcessorConfig{
					{
						Name:    "",
						Type:    "cli",
						Enabled: true,
					},
				},
			},
			wantError: true,
			errorMsg:  "name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a gateway configuration.

			// When: validating the gateway.
			err := validateGateway(tt.gName, tt.gateway)

			// Then: verify error expectation.
			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

// TestValidateServer tests server configuration validation.
func TestValidateServer(t *testing.T) {
	tests := []struct {
		name      string
		sName     string
		server    *MCPServerConfig
		wantError bool
		errorMsg  string
	}{
		{
			name:  "valid stdio server",
			sName: "my-server",
			server: &MCPServerConfig{
				Name:    "my-server",
				Command: "node",
				Args:    []string{"index.js"},
				Enabled: true,
			},
			wantError: false,
		},
		{
			name:  "valid http server",
			sName: "my-server",
			server: &MCPServerConfig{
				Name:    "my-server",
				URL:     "https://api.example.com",
				Enabled: true,
			},
			wantError: false,
		},
		{
			name:  "valid http server with headers",
			sName: "my-server",
			server: &MCPServerConfig{
				Name: "my-server",
				URL:  "https://api.example.com",
				Headers: map[string]string{
					"Authorization": "Bearer ${TOKEN}",
					"Content-Type":  "application/json",
				},
				Enabled: true,
			},
			wantError: false,
		},
		{
			name:  "invalid server name with space",
			sName: "my server",
			server: &MCPServerConfig{
				Name:    "my server",
				Command: "node",
			},
			wantError: true,
			errorMsg:  "name must be URL-safe",
		},
		{
			name:  "invalid server name with dot",
			sName: "my.server",
			server: &MCPServerConfig{
				Name:    "my.server",
				Command: "node",
			},
			wantError: true,
			errorMsg:  "name must be URL-safe",
		},
		{
			name:  "missing both command and url",
			sName: "server1",
			server: &MCPServerConfig{
				Name:    "server1",
				Enabled: true,
			},
			wantError: true,
			errorMsg:  "must specify either 'command'",
		},
		{
			name:  "both command and url specified",
			sName: "server1",
			server: &MCPServerConfig{
				Name:    "server1",
				Command: "node",
				URL:     "https://api.example.com",
				Enabled: true,
			},
			wantError: true,
			errorMsg:  "cannot specify both 'command' and 'url'",
		},
		{
			name:  "invalid http url",
			sName: "server1",
			server: &MCPServerConfig{
				Name:    "server1",
				URL:     "not-a-valid-url",
				Enabled: true,
			},
			wantError: true,
			errorMsg:  "invalid URL format",
		},
		{
			name:  "ftp url not allowed",
			sName: "server1",
			server: &MCPServerConfig{
				Name:    "server1",
				URL:     "ftp://example.com",
				Enabled: true,
			},
			wantError: true,
			errorMsg:  "invalid URL format",
		},
		{
			name:  "empty header key",
			sName: "server1",
			server: &MCPServerConfig{
				Name: "server1",
				URL:  "https://api.example.com",
				Headers: map[string]string{
					"": "value",
				},
			},
			wantError: true,
			errorMsg:  "header keys cannot be empty",
		},
		{
			name:  "empty header value",
			sName: "server1",
			server: &MCPServerConfig{
				Name: "server1",
				URL:  "https://api.example.com",
				Headers: map[string]string{
					"Authorization": "",
				},
			},
			wantError: true,
			errorMsg:  "has empty value",
		},
		{
			name:  "valid server with env vars",
			sName: "server1",
			server: &MCPServerConfig{
				Name:    "server1",
				Command: "node",
				Env: map[string]string{
					"NODE_ENV": "production",
					"API_KEY":  "${API_KEY}",
				},
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a server configuration.

			// When: validating the server.
			err := validateServer(tt.sName, tt.server)

			// Then: verify error expectation.
			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

// TestValidatedGateways tests validation of multiple gateways.
func TestValidatedGateways(t *testing.T) {
	tests := []struct {
		name      string
		gateways  map[string]*GatewayConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid multiple gateways",
			gateways: map[string]*GatewayConfig{
				"gateway1": {
					MCPServers: map[string]*MCPServerConfig{
						"server1": {Name: "server1", Command: "node"},
					},
				},
				"gateway2": {
					MCPServers: map[string]*MCPServerConfig{
						"server2": {Name: "server2", URL: "https://api.example.com"},
					},
				},
			},
			wantError: false,
		},
		{
			name:      "empty gateways map is valid",
			gateways:  map[string]*GatewayConfig{},
			wantError: false,
		},
		{
			name:      "nil gateways is valid",
			gateways:  nil,
			wantError: false,
		},
		{
			name: "invalid gateway name",
			gateways: map[string]*GatewayConfig{
				"invalid gateway": {
					MCPServers: map[string]*MCPServerConfig{
						"server1": {Name: "server1", Command: "node"},
					},
				},
			},
			wantError: true,
			errorMsg:  "name must be URL-safe",
		},
		{
			name: "gateway with no servers",
			gateways: map[string]*GatewayConfig{
				"gateway1": {
					MCPServers: map[string]*MCPServerConfig{},
				},
			},
			wantError: true,
			errorMsg:  "must have at least one MCP server",
		},
		{
			name: "invalid server in gateway",
			gateways: map[string]*GatewayConfig{
				"gateway1": {
					MCPServers: map[string]*MCPServerConfig{
						"invalid server": {
							Name:    "invalid server",
							Command: "node",
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "name must be URL-safe",
		},
		{
			name: "one valid gateway and one invalid",
			gateways: map[string]*GatewayConfig{
				"gateway1": {
					MCPServers: map[string]*MCPServerConfig{
						"server1": {Name: "server1", Command: "node"},
					},
				},
				"gateway2": {
					MCPServers: map[string]*MCPServerConfig{},
				},
			},
			wantError: true,
			errorMsg:  "must have at least one MCP server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a map of gateway configurations.

			// When: validating all gateways.
			err := validatedGateways(tt.gateways)

			// Then: verify error expectation.
			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

// TestValidateConfigIntegration tests full config validation with gateways.
func TestValidateConfigIntegration(t *testing.T) {
	tests := []struct {
		name      string
		config    *GlobalConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid complete config",
			config: &GlobalConfig{
				Version: "1.0.0",
				Proxy:   &ProxySettings{},
				Gateways: map[string]*GatewayConfig{
					"gateway1": {
						MCPServers: map[string]*MCPServerConfig{
							"server1": {Name: "server1", Command: "node"},
						},
					},
				},
				Processors: []*ProcessorConfig{},
			},
			wantError: false,
		},
		{
			name: "missing version",
			config: &GlobalConfig{
				Proxy:    &ProxySettings{},
				Gateways: map[string]*GatewayConfig{},
			},
			wantError: true,
			errorMsg:  "version field is required",
		},
		{
			name: "config with gateway errors",
			config: &GlobalConfig{
				Version: "1.0.0",
				Proxy:   &ProxySettings{},
				Gateways: map[string]*GatewayConfig{
					"gateway1": {
						MCPServers: map[string]*MCPServerConfig{},
					},
				},
			},
			wantError: true,
			errorMsg:  "must have at least one MCP server",
		},
		{
			name: "config with processor errors",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Proxy:    &ProxySettings{},
				Gateways: map[string]*GatewayConfig{},
				Processors: []*ProcessorConfig{
					{
						Name:    "",
						Type:    "cli",
						Enabled: true,
					},
				},
			},
			wantError: true,
			errorMsg:  "name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a complete config.

			// When: validating the entire config.
			err := ValidateConfig(tt.config)

			// Then: verify error expectation.
			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}
