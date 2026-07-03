package config

import (
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"gotest.tools/assert"
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
		name                    string
		gName                   string
		gateway                 GatewayConfig
		taskVerificationEnabled bool
		wantError               bool
		errorMsg                string
	}{
		{
			name:  "valid gateway with stdio server",
			gName: "my-gateway",
			gateway: GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": {
						Name:    "server1",
						Command: "node",
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
						Name: "server1",
						URL:  "https://api.example.com",
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
			errorMsg:  "must have at least one active MCP server",
		},
		{
			name:  "gateway with nil servers",
			gName: "gateway1",
			gateway: GatewayConfig{
				MCPServers: nil,
			},
			wantError: true,
			errorMsg:  "must have at least one active MCP server",
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
		{
			name:  "gateway with supported off verification requirement",
			gName: "gateway1",
			gateway: GatewayConfig{
				VerificationRequirement: VerificationRequirementOff,
				MCPServers: map[string]*MCPServerConfig{
					"server1": {
						Name:    "server1",
						Command: "node",
					},
				},
			},
			wantError: false,
		},
		{
			name:  "gateway with supported optional verification requirement",
			gName: "gateway1",
			gateway: GatewayConfig{
				VerificationRequirement: VerificationRequirementOptional,
				MCPServers: map[string]*MCPServerConfig{
					"server1": {
						Name:    "server1",
						Command: "node",
					},
				},
			},
			taskVerificationEnabled: true,
			wantError:               false,
		},
		{
			name:  "gateway with supported required verification requirement",
			gName: "gateway1",
			gateway: GatewayConfig{
				VerificationRequirement: VerificationRequirementRequired,
				MCPServers: map[string]*MCPServerConfig{
					"server1": {
						Name:    "server1",
						Command: "node",
					},
				},
			},
			taskVerificationEnabled: true,
			wantError:               false,
		},
		{
			name:  "gateway rejects unsupported verification requirement",
			gName: "gateway1",
			gateway: GatewayConfig{
				VerificationRequirement: "recommended",
				MCPServers: map[string]*MCPServerConfig{
					"server1": {
						Name:    "server1",
						Command: "node",
					},
				},
			},
			taskVerificationEnabled: true,
			wantError:               true,
			errorMsg:                "verificationRequirement",
		},
		{
			name:  "gateway rejects optional verification requirement when project task verification disabled",
			gName: "gateway1",
			gateway: GatewayConfig{
				VerificationRequirement: VerificationRequirementOptional,
				MCPServers: map[string]*MCPServerConfig{
					"server1": {
						Name:    "server1",
						Command: "node",
					},
				},
			},
			wantError: true,
			errorMsg:  "requires project capabilities.taskVerification.enabled=true",
		},
		{
			name:  "gateway rejects required verification requirement when project task verification disabled",
			gName: "gateway1",
			gateway: GatewayConfig{
				VerificationRequirement: VerificationRequirementRequired,
				MCPServers: map[string]*MCPServerConfig{
					"server1": {
						Name:    "server1",
						Command: "node",
					},
				},
			},
			wantError: true,
			errorMsg:  "requires project capabilities.taskVerification.enabled=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a gateway configuration.

			// When: validating the gateway.
			err := validateGateway(tt.gName, tt.gateway, tt.taskVerificationEnabled)

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
			},
			wantError: false,
		},
		{
			name:  "valid http server",
			sName: "my-server",
			server: &MCPServerConfig{
				Name: "my-server",
				URL:  "https://api.example.com",
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
				Name: "server1",
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
			},
			wantError: true,
			errorMsg:  "cannot specify both 'command' and 'url'",
		},
		{
			name:  "invalid http url",
			sName: "server1",
			server: &MCPServerConfig{
				Name: "server1",
				URL:  "not-a-valid-url",
			},
			wantError: true,
			errorMsg:  "invalid URL format",
		},
		{
			name:  "ftp url not allowed",
			sName: "server1",
			server: &MCPServerConfig{
				Name: "server1",
				URL:  "ftp://example.com",
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
				Proxy: &ProxySettings{},
				Gateways: map[string]*GatewayConfig{
					"gateway1": {
						MCPServers: map[string]*MCPServerConfig{
							"server1": {Name: "server1", Command: "node"},
						},
					},
				},
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
			errorMsg:  "must have at least one active MCP server",
		},
		{
			name: "config with processor errors",
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
			err := ValidateConfig(tt.config, true)

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

func TestValidateConfigNonStrict_ValidatesNameConventions(t *testing.T) {
	t.Run("invalid gateway name fails in non-strict mode", func(t *testing.T) {
		cfg := &GlobalConfig{
			Version: "1.0.0",
			Proxy:   &ProxySettings{},
			Gateways: map[string]*GatewayConfig{
				"invalid gateway": {
					MCPServers: map[string]*MCPServerConfig{},
				},
			},
		}

		err := ValidateConfig(cfg, false)
		if err == nil {
			t.Fatal("expected error for invalid gateway name")
		}
		if !contains(err.Error(), "name must be URL-safe") {
			t.Fatalf("expected URL-safe validation error, got: %v", err)
		}
	})

	t.Run("invalid server name fails in non-strict mode", func(t *testing.T) {
		cfg := &GlobalConfig{
			Version: "1.0.0",
			Proxy:   &ProxySettings{},
			Gateways: map[string]*GatewayConfig{
				"gateway1": {
					MCPServers: map[string]*MCPServerConfig{
						"bad/server": {Name: "server1", Command: "node"},
					},
				},
			},
		}

		err := ValidateConfig(cfg, false)
		if err == nil {
			t.Fatal("expected error for invalid server name")
		}
		if !contains(err.Error(), "name must be URL-safe") {
			t.Fatalf("expected URL-safe validation error, got: %v", err)
		}
	})

	t.Run("empty gateways still allowed in non-strict mode", func(t *testing.T) {
		cfg := &GlobalConfig{
			Version:  "1.0.0",
			Proxy:    &ProxySettings{},
			Gateways: map[string]*GatewayConfig{},
		}

		err := ValidateConfig(cfg, false)
		if err != nil {
			t.Fatalf("expected no error for non-strict empty gateways, got: %v", err)
		}
	})
}

func TestValidateProcessorPartsToolSurfaceCombinations(t *testing.T) {
	tests := []struct {
		name      string
		parts     []string
		wantError bool
	}{
		{name: "tool surface only", parts: []string{"tool_surface"}},
		{name: "tool surface with annotations", parts: []string{"tool_surface", "annotations"}},
		{name: "tool surface with payload rejected", parts: []string{"tool_surface", "payload"}, wantError: true},
		{name: "tool surface with meta rejected", parts: []string{"tool_surface", "meta"}, wantError: true},
		{name: "reserved prompt surface rejected", parts: []string{"prompt_surface"}, wantError: true},
		{name: "reserved resource surface rejected", parts: []string{"resource_surface"}, wantError: true},
		{name: "reserved mcp surface rejected", parts: []string{"mcp_surface"}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := &ProcessorConfig{Name: "surface", Parts: tt.parts}
			err := validateProcessorParts(processor)
			if tt.wantError && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("expected no validation error, got %v", err)
			}
		})
	}
}

func TestValidateConfig_ValidatesProxyLoggingSettings(t *testing.T) {
	tests := []struct {
		name      string
		proxy     *ProxySettings
		wantError string
	}{
		{
			name: "defaults log settings when omitted",
			proxy: &ProxySettings{
				Port: "9666",
			},
		},
		{
			name: "accepts supported log output",
			proxy: &ProxySettings{
				Port:      "9666",
				LogLevel:  "DEBUG",
				LogOutput: "BOTH",
			},
		},
		{
			name: "rejects unsupported log level",
			proxy: &ProxySettings{
				Port:     "9666",
				LogLevel: "trace",
			},
			wantError: "proxy.logLevel",
		},
		{
			name: "rejects unsupported log output",
			proxy: &ProxySettings{
				Port:      "9666",
				LogOutput: "syslog",
			},
			wantError: "proxy.logOutput",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &GlobalConfig{
				Version:  "1.0.0",
				Proxy:    tt.proxy,
				Gateways: map[string]*GatewayConfig{},
			}

			err := ValidateConfig(cfg, false)
			if tt.wantError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantError)
				}
				if !contains(err.Error(), tt.wantError) {
					t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if cfg.Proxy.LogLevel == "" {
				t.Fatal("expected log level default to be applied")
			}
			if cfg.Proxy.LogOutput == "" {
				t.Fatal("expected log output default to be applied")
			}
		})
	}
}

func TestValidateConfig_RequiresPublicBaseURLForOAuth(t *testing.T) {
	authDisabled := false
	cfg := &GlobalConfig{
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy:       &ProxySettings{Port: "9666"},
		Gateways: map[string]*GatewayConfig{
			"gw": {
				MCPServers: map[string]*MCPServerConfig{
					"oauth-server": {
						URL: "https://example.com/mcp",
						OAuth: &OAuthConfig{
							Enabled:          true,
							ClientID:         "client-id",
							ClientSecret:     "client-secret",
							ClientAuthMethod: "client_secret_post",
							Resource:         "https://example.com/mcp",
							Issuer:           "https://issuer.example",
						},
					},
				},
			},
		},
	}

	err := ValidateConfig(cfg, false)
	if err == nil || !contains(err.Error(), "proxy.web.publicBaseUrl is required") {
		t.Fatalf("expected public base URL validation error, got %v", err)
	}

	cfg.Proxy.Web = &ProxyWebSettings{PublicBaseURL: "https://centian.example"}
	if err := ValidateConfig(cfg, false); err != nil {
		t.Fatalf("expected valid oauth config, got %v", err)
	}
}

func TestValidateOAuthTransport(t *testing.T) {
	tests := []struct {
		name      string
		server    *MCPServerConfig
		wantError string
	}{
		{
			name:      "rejects stdio server",
			server:    &MCPServerConfig{Command: "node"},
			wantError: "downstream OAuth is only supported for HTTP MCP servers",
		},
		{
			name:      "rejects mixed transport",
			server:    &MCPServerConfig{Command: "node", URL: "https://example.com/mcp"},
			wantError: "downstream OAuth is only supported for HTTP MCP servers",
		},
		{
			name:   "accepts http server",
			server: &MCPServerConfig{URL: "https://example.com/mcp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOAuthTransport("oauth-server", tt.server)
			if tt.wantError == "" {
				assert.NilError(t, err)
				return
			}
			assert.Assert(t, err != nil)
			assert.Assert(t, contains(err.Error(), tt.wantError))
		})
	}
}

func TestValidateOAuthConfig(t *testing.T) {
	tests := []struct {
		name      string
		server    *MCPServerConfig
		wantError string
	}{
		{
			name:   "skips nil oauth",
			server: &MCPServerConfig{URL: "https://example.com/mcp"},
		},
		{
			name: "skips disabled oauth",
			server: &MCPServerConfig{
				URL:   "https://example.com/mcp",
				OAuth: &OAuthConfig{},
			},
		},
		{
			name: "rejects stdio transport",
			server: &MCPServerConfig{
				Command: "node",
				OAuth: &OAuthConfig{
					Enabled:      true,
					ClientID:     "client-id",
					ClientSecret: "client-secret",
					Resource:     "https://example.com/mcp",
					Issuer:       "https://issuer.example",
				},
			},
			wantError: "downstream OAuth is only supported for HTTP MCP servers",
		},
		{
			name: "rejects missing required fields",
			server: &MCPServerConfig{
				URL: "https://example.com/mcp",
				OAuth: &OAuthConfig{
					Enabled: true,
					Issuer:  "https://issuer.example",
				},
			},
			wantError: "oauth.clientId is required",
		},
		{
			name: "rejects invalid auth method",
			server: &MCPServerConfig{
				URL: "https://example.com/mcp",
				OAuth: &OAuthConfig{
					Enabled:          true,
					ClientID:         "client-id",
					ClientSecret:     "client-secret",
					ClientAuthMethod: "private_key_jwt",
					Resource:         "https://example.com/mcp",
					Issuer:           "https://issuer.example",
				},
			},
			wantError: "oauth.clientAuthMethod must be client_secret_basic or client_secret_post",
		},
		{
			name: "accepts valid oauth and normalizes values",
			server: &MCPServerConfig{
				URL: "https://example.com/mcp",
				OAuth: &OAuthConfig{
					Enabled:          true,
					ClientID:         " client-id ",
					ClientSecret:     " client-secret ",
					ClientAuthMethod: " CLIENT_SECRET_POST ",
					Resource:         " https://example.com/mcp ",
					Issuer:           " https://issuer.example ",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOAuthConfig("oauth-server", tt.server)
			if tt.wantError != "" {
				assert.Assert(t, err != nil)
				assert.Assert(t, contains(err.Error(), tt.wantError))
				return
			}

			assert.NilError(t, err)
			if tt.server.OAuth != nil && tt.server.OAuth.Enabled {
				assert.Equal(t, tt.server.OAuth.ClientAuthMethod, "client_secret_post")
				assert.Equal(t, tt.server.OAuth.ClientID, "client-id")
				assert.Equal(t, tt.server.OAuth.ClientSecret, "client-secret")
				assert.Equal(t, tt.server.OAuth.Resource, "https://example.com/mcp")
				assert.Equal(t, tt.server.OAuth.Issuer, "https://issuer.example")
			}
		})
	}
}

func TestNormalizeOAuthConfig(t *testing.T) {
	oauthConfig := &OAuthConfig{
		ClientID:              "  client-id  ",
		ClientSecret:          "  client-secret  ",
		ClientAuthMethod:      "  CLIENT_SECRET_POST  ",
		Resource:              "  https://example.com/mcp  ",
		Issuer:                "  https://issuer.example  ",
		AuthorizationEndpoint: "  https://issuer.example/authorize  ",
		TokenEndpoint:         "  https://issuer.example/token  ",
	}

	normalizeOAuthConfig(oauthConfig)

	assert.Equal(t, oauthConfig.ClientID, "client-id")
	assert.Equal(t, oauthConfig.ClientSecret, "client-secret")
	assert.Equal(t, oauthConfig.ClientAuthMethod, "client_secret_post")
	assert.Equal(t, oauthConfig.Resource, "https://example.com/mcp")
	assert.Equal(t, oauthConfig.Issuer, "https://issuer.example")
	assert.Equal(t, oauthConfig.AuthorizationEndpoint, "https://issuer.example/authorize")
	assert.Equal(t, oauthConfig.TokenEndpoint, "https://issuer.example/token")
}

func TestValidateOAuthRequiredFields(t *testing.T) {
	tests := []struct {
		name      string
		oauth     *OAuthConfig
		wantError string
	}{
		{
			name:      "requires client id",
			oauth:     &OAuthConfig{ClientSecret: "secret", Resource: "https://example.com/mcp"},
			wantError: "oauth.clientId is required",
		},
		{
			name:      "requires client secret",
			oauth:     &OAuthConfig{ClientID: "client-id", Resource: "https://example.com/mcp"},
			wantError: "oauth.clientSecret is required",
		},
		{
			name:      "requires resource",
			oauth:     &OAuthConfig{ClientID: "client-id", ClientSecret: "secret"},
			wantError: "oauth.resource is required",
		},
		{
			name:  "accepts complete config",
			oauth: &OAuthConfig{ClientID: "client-id", ClientSecret: "secret", Resource: "https://example.com/mcp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOAuthRequiredFields("oauth-server", tt.oauth)
			if tt.wantError == "" {
				assert.NilError(t, err)
				return
			}
			assert.Assert(t, err != nil)
			assert.Assert(t, contains(err.Error(), tt.wantError))
		})
	}
}

func TestValidateOAuthClientAuthMethod(t *testing.T) {
	validMethods := []string{"", "client_secret_basic", "client_secret_post"}
	for _, method := range validMethods {
		t.Run("accepts "+method, func(t *testing.T) {
			assert.NilError(t, validateOAuthClientAuthMethod("oauth-server", method))
		})
	}

	err := validateOAuthClientAuthMethod("oauth-server", "private_key_jwt")
	assert.Assert(t, err != nil)
	assert.Assert(t, contains(err.Error(), "oauth.clientAuthMethod must be client_secret_basic or client_secret_post"))
}

func TestValidateOAuthEndpoints(t *testing.T) {
	tests := []struct {
		name      string
		oauth     *OAuthConfig
		wantError string
	}{
		{
			name: "accepts issuer only",
			oauth: &OAuthConfig{
				Issuer: "https://issuer.example",
			},
		},
		{
			name: "accepts explicit endpoints without issuer",
			oauth: &OAuthConfig{
				AuthorizationEndpoint: "https://issuer.example/authorize",
				TokenEndpoint:         "https://issuer.example/token",
			},
		},
		{
			name: "rejects invalid issuer",
			oauth: &OAuthConfig{
				Issuer: "issuer.example",
			},
			wantError: "oauth.issuer must be a valid http:// or https:// URL",
		},
		{
			name: "rejects missing issuer and incomplete endpoints",
			oauth: &OAuthConfig{
				AuthorizationEndpoint: "https://issuer.example/authorize",
			},
			wantError: "oauth.issuer or both oauth.authorizationEndpoint and oauth.tokenEndpoint are required",
		},
		{
			name: "rejects invalid authorization endpoint",
			oauth: &OAuthConfig{
				Issuer:                "https://issuer.example",
				AuthorizationEndpoint: "issuer.example/authorize",
			},
			wantError: "oauth.authorizationEndpoint must be a valid http:// or https:// URL",
		},
		{
			name: "rejects invalid token endpoint",
			oauth: &OAuthConfig{
				Issuer:        "https://issuer.example",
				TokenEndpoint: "issuer.example/token",
			},
			wantError: "oauth.tokenEndpoint must be a valid http:// or https:// URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOAuthEndpoints("oauth-server", tt.oauth)
			if tt.wantError == "" {
				assert.NilError(t, err)
				return
			}
			assert.Assert(t, err != nil)
			assert.Assert(t, contains(err.Error(), tt.wantError))
		})
	}
}

func TestValidateOAuthIssuer(t *testing.T) {
	tests := []struct {
		name      string
		oauth     *OAuthConfig
		wantError string
	}{
		{
			name: "accepts issuer",
			oauth: &OAuthConfig{
				Issuer: "https://issuer.example",
			},
		},
		{
			name: "accepts explicit endpoints",
			oauth: &OAuthConfig{
				AuthorizationEndpoint: "https://issuer.example/authorize",
				TokenEndpoint:         "https://issuer.example/token",
			},
		},
		{
			name: "rejects invalid issuer",
			oauth: &OAuthConfig{
				Issuer: "issuer.example",
			},
			wantError: "oauth.issuer must be a valid http:// or https:// URL",
		},
		{
			name: "rejects missing token endpoint",
			oauth: &OAuthConfig{
				AuthorizationEndpoint: "https://issuer.example/authorize",
			},
			wantError: "oauth.issuer or both oauth.authorizationEndpoint and oauth.tokenEndpoint are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOAuthIssuer("oauth-server", tt.oauth)
			if tt.wantError == "" {
				assert.NilError(t, err)
				return
			}
			assert.Assert(t, err != nil)
			assert.Assert(t, contains(err.Error(), tt.wantError))
		})
	}
}

func TestValidateOptionalOAuthURL(t *testing.T) {
	assert.NilError(t, validateOptionalOAuthURL("oauth-server", "oauth.authorizationEndpoint", ""))
	assert.NilError(t, validateOptionalOAuthURL("oauth-server", "oauth.authorizationEndpoint", "https://issuer.example/authorize"))

	err := validateOptionalOAuthURL("oauth-server", "oauth.authorizationEndpoint", "issuer.example/authorize")
	assert.Assert(t, err != nil)
	assert.Assert(t, contains(err.Error(), "oauth.authorizationEndpoint must be a valid http:// or https:// URL"))
}
