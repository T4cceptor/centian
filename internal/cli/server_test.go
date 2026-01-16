package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CentianAI/centian-cli/internal/config"
	urfavecli "github.com/urfave/cli/v3"
)

// TestPrintServerInfo tests the server info printing function
func TestPrintServerInfo(t *testing.T) {
	tests := []struct {
		name           string
		config         *config.GlobalConfig
		wantError      bool
		expectInOutput []string
	}{
		{
			name: "valid config with single gateway and server",
			config: &config.GlobalConfig{
				Name:    "Test Server",
				Version: "1.0.0",
				Proxy: &config.ProxySettings{
					Port:    "8080",
					Timeout: 30,
				},
				Gateways: map[string]*config.GatewayConfig{
					"gateway1": {
						MCPServers: map[string]*config.MCPServerConfig{
							"server1": {
								Name: "server1",
								URL:  "https://api.example.com",
							},
						},
					},
				},
			},
			wantError: false,
			expectInOutput: []string{
				"Test Server",
				"Port: 8080",
				"Timeout: 30s",
				"Gateways: 1",
				"Total MCP servers: 1",
				"/mcp/gateway1/server1",
				"https://api.example.com",
			},
		},
		{
			name: "config with multiple gateways and servers",
			config: &config.GlobalConfig{
				Name:    "Multi-Gateway Server",
				Version: "1.0.0",
				Proxy: &config.ProxySettings{
					Port:    "9000",
					Timeout: 60,
				},
				Gateways: map[string]*config.GatewayConfig{
					"gateway1": {
						MCPServers: map[string]*config.MCPServerConfig{
							"server1": {Name: "server1", URL: "https://api1.example.com"},
							"server2": {Name: "server2", URL: "https://api2.example.com"},
						},
					},
					"gateway2": {
						MCPServers: map[string]*config.MCPServerConfig{
							"server3": {Name: "server3", URL: "https://api3.example.com"},
						},
					},
				},
			},
			wantError: false,
			expectInOutput: []string{
				"Multi-Gateway Server",
				"Port: 9000",
				"Timeout: 60s",
				"Gateways: 2",
				"Total MCP servers: 3",
				"/mcp/gateway1/server1",
				"/mcp/gateway1/server2",
				"/mcp/gateway2/server3",
			},
		},
		{
			name: "config without name uses default",
			config: &config.GlobalConfig{
				Version: "1.0.0",
				Proxy: &config.ProxySettings{
					Port:    "8080",
					Timeout: 30,
				},
				Gateways: map[string]*config.GatewayConfig{
					"gateway1": {
						MCPServers: map[string]*config.MCPServerConfig{
							"server1": {Name: "server1", URL: "https://api.example.com"},
						},
					},
				},
			},
			wantError: false,
			expectInOutput: []string{
				"Centian Proxy Server",
			},
		},
		{
			name: "no servers configured error",
			config: &config.GlobalConfig{
				Name:    "Empty Server",
				Version: "1.0.0",
				Proxy: &config.ProxySettings{
					Port:    "8080",
					Timeout: 30,
				},
				Gateways: map[string]*config.GatewayConfig{
					"gateway1": {
						MCPServers: map[string]*config.MCPServerConfig{},
					},
				},
			},
			wantError:      true,
			expectInOutput: []string{},
		},
		{
			name: "empty gateways map error",
			config: &config.GlobalConfig{
				Name:     "No Gateways",
				Version:  "1.0.0",
				Proxy:    &config.ProxySettings{Port: "8080", Timeout: 30},
				Gateways: map[string]*config.GatewayConfig{},
			},
			wantError:      true,
			expectInOutput: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a server configuration
			// Capture stderr output
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			// When: printing server info
			err := printServerInfo(tt.config)

			// Restore stderr and capture output
			w.Close()
			os.Stderr = oldStderr
			var buf bytes.Buffer
			buf.ReadFrom(r)
			output := buf.String()

			// Then: verify error expectation
			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}

				// Then: verify expected strings in output
				for _, expected := range tt.expectInOutput {
					if !strings.Contains(output, expected) {
						t.Errorf("Expected output to contain '%s', but it didn't.\nOutput:\n%s", expected, output)
					}
				}
			}
		})
	}
}

// TestHandleServerStartCommandValidation tests config validation logic
func TestHandleServerStartCommandValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.GlobalConfig
		expectedErr string
	}{
		{
			name:        "missing proxy settings",
			config:      &config.GlobalConfig{Version: "1.0.0", Gateways: map[string]*config.GatewayConfig{"g1": {MCPServers: map[string]*config.MCPServerConfig{"s1": {URL: "http://example.com"}}}}},
			expectedErr: "proxy settings are required",
		},
		{
			name:        "no gateways configured",
			config:      &config.GlobalConfig{Version: "1.0.0", Proxy: &config.ProxySettings{Port: "8080"}, Gateways: map[string]*config.GatewayConfig{}},
			expectedErr: "no gateways configured",
		},
		{
			name:        "nil gateways",
			config:      &config.GlobalConfig{Version: "1.0.0", Proxy: &config.ProxySettings{Port: "8080"}, Gateways: nil},
			expectedErr: "no gateways configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a test config file
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "test_config.json")
			data, _ := json.MarshalIndent(tt.config, "", "  ")
			os.WriteFile(configPath, data, 0o644)

			// Given: a CLI command with config-path flag
			cmd := &urfavecli.Command{
				Flags: []urfavecli.Flag{
					&urfavecli.StringFlag{Name: "config-path"},
				},
			}
			cmd.Set("config-path", configPath)

			// When: running handleServerStartCommand
			err := handleServerStartCommand(context.Background(), cmd)

			// Then: should return validation error
			if err == nil {
				t.Fatalf("Expected error containing '%s', got nil", tt.expectedErr)
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedErr, err.Error())
			}
		})
	}
}

// TestHandleServerStartCommandConfigLoading tests config file loading
func TestHandleServerStartCommandConfigLoading(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func(t *testing.T, dir string) string
		expectedErr string
	}{
		{
			name: "non-existent config file",
			setupConfig: func(_ *testing.T, dir string) string {
				return filepath.Join(dir, "nonexistent.json")
			},
			expectedErr: "failed to load config",
		},
		{
			name: "invalid JSON in config file",
			setupConfig: func(_ *testing.T, dir string) string {
				path := filepath.Join(dir, "invalid.json")
				os.WriteFile(path, []byte("{ invalid json"), 0o644)
				return path
			},
			expectedErr: "failed to load config",
		},
		{
			name: "config with invalid structure",
			setupConfig: func(_ *testing.T, dir string) string {
				path := filepath.Join(dir, "invalid_structure.json")
				// Missing required version field
				invalidConfig := map[string]interface{}{
					"proxy": map[string]interface{}{"port": "8080"},
				}
				data, _ := json.Marshal(invalidConfig)
				os.WriteFile(path, data, 0o644)
				return path
			},
			expectedErr: "failed to load config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a test config setup
			tempDir := t.TempDir()
			configPath := tt.setupConfig(t, tempDir)

			// Given: a CLI command with config-path
			cmd := &urfavecli.Command{
				Flags: []urfavecli.Flag{
					&urfavecli.StringFlag{Name: "config-path"},
				},
			}
			cmd.Set("config-path", configPath)

			// When: running handleServerStartCommand
			err := handleServerStartCommand(context.Background(), cmd)

			// Then: should return config loading error
			if err == nil {
				t.Fatalf("Expected error containing '%s', got nil", tt.expectedErr)
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedErr, err.Error())
			}
		})
	}
}

// TestHandleServerStartCommandWithDefaultPath tests using default config path
func TestHandleServerStartCommandWithDefaultPath(t *testing.T) {
	// Given: a test environment with default config
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	testHome := filepath.Join(tempDir, "testhome")
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// Create valid config at default location
	err := config.EnsureConfigDir()
	if err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	testConfig := &config.GlobalConfig{
		Name:    "Default Path Test",
		Version: "1.0.0",
		Proxy: &config.ProxySettings{
			Port:    "8080",
			Timeout: 30,
		},
		Gateways: map[string]*config.GatewayConfig{
			"test-gateway": {
				MCPServers: map[string]*config.MCPServerConfig{
					"test-server": {
						Name: "test-server",
						URL:  "https://api.example.com",
					},
				},
			},
		},
	}

	err = config.SaveConfig(testConfig)
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// When: verifying config loads from default path
	// Note: We can't easily test the full server startup in a unit test
	// without complex mocking. The integration test covers the full flow.
	// Here we just test that config loading from default path works.

	configPath, _ := config.GetConfigPath()
	loadedConfig, err := config.LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("Config loading from default path failed: %v", err)
	}

	// Then: verify config was loaded correctly
	if loadedConfig.Name != "Default Path Test" {
		t.Errorf("Expected config name 'Default Path Test', got '%s'", loadedConfig.Name)
	}
}

// TestServerCommandStructure tests the ServerCommand CLI structure
func TestServerCommandStructure(t *testing.T) {
	// Given: the ServerCommand

	// Then: verify command is properly configured
	if ServerCommand == nil {
		t.Fatal("ServerCommand is nil")
	}

	if ServerCommand.Name != "server" {
		t.Errorf("Expected command name 'server', got '%s'", ServerCommand.Name)
	}

	if ServerCommand.Usage == "" {
		t.Error("ServerCommand should have usage text")
	}

	// Then: verify subcommands exist
	if len(ServerCommand.Commands) == 0 {
		t.Error("ServerCommand should have subcommands")
	}

	// Verify ServerStartCommand exists
	var hasStartCommand bool
	for _, subcmd := range ServerCommand.Commands {
		if subcmd.Name != "start" {
			continue
		}
		hasStartCommand = true

		// Verify start command structure
		if subcmd.Usage == "" {
			t.Error("ServerStartCommand should have usage text")
		}
		if subcmd.Description == "" {
			t.Error("ServerStartCommand should have description")
		}
		if subcmd.Action == nil {
			t.Error("ServerStartCommand should have action function")
		}

		// Verify flags
		var hasConfigPathFlag bool
		for _, flag := range subcmd.Flags {
			if sf, ok := flag.(*urfavecli.StringFlag); ok && sf.Name == "config-path" {
				hasConfigPathFlag = true
			}
		}
		if !hasConfigPathFlag {
			t.Error("ServerStartCommand should have config-path flag")
		}
		break
	}

	if !hasStartCommand {
		t.Error("ServerCommand should have 'start' subcommand")
	}
}

// TestPrintServerInfoEdgeCases tests edge cases in server info printing
func TestPrintServerInfoEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.GlobalConfig
		expectPanic bool
	}{
		{
			name: "nil gateway in map",
			config: &config.GlobalConfig{
				Name:    "Test",
				Version: "1.0.0",
				Proxy:   &config.ProxySettings{Port: "8080", Timeout: 30},
				Gateways: map[string]*config.GatewayConfig{
					"gateway1": nil,
				},
			},
			expectPanic: true,
		},
		{
			name: "server with empty URL",
			config: &config.GlobalConfig{
				Name:    "Test",
				Version: "1.0.0",
				Proxy:   &config.ProxySettings{Port: "8080", Timeout: 30},
				Gateways: map[string]*config.GatewayConfig{
					"gateway1": {
						MCPServers: map[string]*config.MCPServerConfig{
							"server1": {Name: "server1", URL: ""},
						},
					},
				},
			},
			expectPanic: false,
		},
		{
			name: "gateway with special characters in name",
			config: &config.GlobalConfig{
				Name:    "Test",
				Version: "1.0.0",
				Proxy:   &config.ProxySettings{Port: "8080", Timeout: 30},
				Gateways: map[string]*config.GatewayConfig{
					"gateway-with-dashes": {
						MCPServers: map[string]*config.MCPServerConfig{
							"server_with_underscores": {
								Name: "server_with_underscores",
								URL:  "https://api.example.com",
							},
						},
					},
				},
			},
			expectPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a config with edge cases

			if tt.expectPanic {
				// When: expecting panic
				defer func() {
					if r := recover(); r == nil {
						t.Error("Expected panic but didn't get one")
					}
				}()
			}

			// Capture stderr
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			// When: printing server info
			_ = printServerInfo(tt.config)

			// Restore stderr
			w.Close()
			os.Stderr = oldStderr
			var buf bytes.Buffer
			buf.ReadFrom(r)

			// Then: if we reach here without panic, test passes
			if tt.expectPanic {
				t.Error("Should have panicked but didn't")
			}
		})
	}
}
