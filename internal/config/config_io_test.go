package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfigFromPath tests loading configuration from a custom path
func TestLoadConfigFromPath(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string // Returns path to test file
		wantError bool
		errorMsg  string
		verify    func(t *testing.T, cfg *GlobalConfig)
	}{
		{
			name: "load valid config from custom path",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				configPath := filepath.Join(tempDir, "custom-config.json")

				cfg := DefaultConfig()
				cfg.Gateways = map[string]*GatewayConfig{
					"gateway1": {
						MCPServers: map[string]*MCPServerConfig{
							"server1": {Name: "server1", Command: "node"},
						},
					},
				}

				data, _ := json.MarshalIndent(cfg, "", "  ")
				os.WriteFile(configPath, data, 0o644)

				return configPath
			},
			wantError: false,
			verify: func(t *testing.T, cfg *GlobalConfig) {
				if cfg == nil {
					t.Fatal("Expected config, got nil")
				}
				if len(cfg.Gateways) != 1 {
					t.Errorf("Expected 1 gateway, got %d", len(cfg.Gateways))
				}
			},
		},
		{
			name: "load config with multiple gateways",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				configPath := filepath.Join(tempDir, "multi-gateway.json")

				cfg := DefaultConfig()
				cfg.Gateways = map[string]*GatewayConfig{
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
				}

				data, _ := json.MarshalIndent(cfg, "", "  ")
				os.WriteFile(configPath, data, 0o644)

				return configPath
			},
			wantError: false,
			verify: func(t *testing.T, cfg *GlobalConfig) {
				if len(cfg.Gateways) != 2 {
					t.Errorf("Expected 2 gateways, got %d", len(cfg.Gateways))
				}
			},
		},
		{
			name: "file does not exist",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent.json")
			},
			wantError: true,
			errorMsg:  "failed to read config file",
		},
		{
			name: "malformed JSON",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				configPath := filepath.Join(tempDir, "malformed.json")
				os.WriteFile(configPath, []byte("{ invalid json "), 0o644)
				return configPath
			},
			wantError: true,
			errorMsg:  "failed to parse config",
		},
		{
			name: "invalid config structure",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				configPath := filepath.Join(tempDir, "invalid.json")

				// Config without required version field
				invalidConfig := map[string]interface{}{
					"proxy": map[string]interface{}{},
				}

				data, _ := json.Marshal(invalidConfig)
				os.WriteFile(configPath, data, 0o644)

				return configPath
			},
			wantError: true,
			errorMsg:  "invalid configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a test config file
			path := tt.setup(t)

			// When: loading config from the path
			cfg, err := LoadConfigFromPath(path)

			// Then: verify error expectation
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
				if tt.verify != nil {
					tt.verify(t, cfg)
				}
			}
		})
	}
}

// TestLoadConfigMissingFile tests loading when config file doesn't exist
func TestLoadConfigMissingFile(t *testing.T) {
	// Given: a test environment with no config file
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	testHome := filepath.Join(tempDir, "noconfig")
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// When: attempting to load config
	cfg, err := LoadConfig()

	// Then: should return error about missing file
	if err == nil {
		t.Error("Expected error for missing config file, got nil")
	}
	if cfg != nil {
		t.Error("Expected nil config for missing file")
	}
	if !contains(err.Error(), "configuration file not found") {
		t.Errorf("Expected 'configuration file not found' error, got: %v", err)
	}
}

// TestSaveAndLoadConfigRoundtrip tests saving and loading config
func TestSaveAndLoadConfigRoundtrip(t *testing.T) {
	// Given: a test environment
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	testHome := filepath.Join(tempDir, "roundtrip")
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// Given: a config with specific values
	originalConfig := DefaultConfig()
	originalConfig.Name = "Test Server"
	originalConfig.Gateways = map[string]*GatewayConfig{
		"gateway1": {
			AllowDynamic: true,
			MCPServers: map[string]*MCPServerConfig{
				"server1": {
					Name:    "server1",
					Command: "node",
					Args:    []string{"index.js", "--port", "3000"},
					Enabled: true,
					Env: map[string]string{
						"NODE_ENV": "production",
					},
				},
				"server2": {
					Name: "server2",
					URL:  "https://api.example.com",
					Headers: map[string]string{
						"Authorization": "Bearer ${TOKEN}",
					},
					Enabled: false,
				},
			},
		},
	}

	// When: saving the config
	err := SaveConfig(originalConfig)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Then: verify config file was created
	configPath, _ := GetConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// When: loading the config back
	loadedConfig, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Then: verify all fields match
	if loadedConfig.Name != originalConfig.Name {
		t.Errorf("Name: expected '%s', got '%s'", originalConfig.Name, loadedConfig.Name)
	}
	if loadedConfig.Version != originalConfig.Version {
		t.Errorf("Version: expected '%s', got '%s'", originalConfig.Version, loadedConfig.Version)
	}
	if len(loadedConfig.Gateways) != len(originalConfig.Gateways) {
		t.Errorf("Gateways count: expected %d, got %d", len(originalConfig.Gateways), len(loadedConfig.Gateways))
	}

	// Verify gateway details
	if gw, ok := loadedConfig.Gateways["gateway1"]; ok {
		if !gw.AllowDynamic {
			t.Error("Gateway AllowDynamic should be true")
		}
		if len(gw.MCPServers) != 2 {
			t.Errorf("Expected 2 servers in gateway, got %d", len(gw.MCPServers))
		}

		// Verify server1
		if srv, ok := gw.MCPServers["server1"]; ok {
			if srv.Command != "node" {
				t.Errorf("Server1 command: expected 'node', got '%s'", srv.Command)
			}
			if len(srv.Args) != 3 {
				t.Errorf("Server1 args: expected 3, got %d", len(srv.Args))
			}
			if !srv.Enabled {
				t.Error("Server1 should be enabled")
			}
		} else {
			t.Error("Server1 not found in loaded config")
		}

		// Verify server2
		if srv, ok := gw.MCPServers["server2"]; ok {
			if srv.URL != "https://api.example.com" {
				t.Errorf("Server2 URL: expected 'https://api.example.com', got '%s'", srv.URL)
			}
			if srv.Enabled {
				t.Error("Server2 should be disabled")
			}
		} else {
			t.Error("Server2 not found in loaded config")
		}
	} else {
		t.Error("Gateway1 not found in loaded config")
	}
}

// TestGetConfigDir tests config directory path retrieval
func TestGetConfigDir(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T)
		teardown  func()
		wantError bool
		verify    func(t *testing.T, dir string)
	}{
		{
			name: "normal home directory",
			setup: func(t *testing.T) {
				// Use current HOME
			},
			teardown:  func() {},
			wantError: false,
			verify: func(t *testing.T, dir string) {
				if dir == "" {
					t.Error("Expected non-empty directory path")
				}
				if !contains(dir, ".centian") {
					t.Errorf("Expected path to contain '.centian', got: %s", dir)
				}
			},
		},
		{
			name: "custom home directory",
			setup: func(t *testing.T) {
				os.Setenv("HOME", "/custom/home")
			},
			teardown: func() {
				os.Unsetenv("HOME")
			},
			wantError: false,
			verify: func(t *testing.T, dir string) {
				expected := "/" + filepath.Join("custom", "home", ".centian")
				if dir != expected {
					t.Errorf("Expected '%s', got '%s'", expected, dir)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: test environment setup
			tt.setup(t)
			defer tt.teardown()

			// When: getting config directory
			dir, err := GetConfigDir()

			// Then: verify expectations
			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
				if tt.verify != nil {
					tt.verify(t, dir)
				}
			}
		})
	}
}

// TestGetConfigPath tests config file path retrieval
func TestGetConfigPath(t *testing.T) {
	// Given: a test home directory
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", "/test/home")
	defer os.Setenv("HOME", originalHome)

	// When: getting config path
	path, err := GetConfigPath()

	// Then: verify path is correct
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expected := "/" + filepath.Join("test", "home", ".centian", "config.jsonc")
	if path != expected {
		t.Errorf("Expected path '%s', got '%s'", expected, path)
	}
}

// TestEnsureConfigDir tests config directory creation
func TestEnsureConfigDir(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string // Returns temp home dir
		wantError bool
		verify    func(t *testing.T, homeDir string)
	}{
		{
			name: "create new config directory",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				testHome := filepath.Join(tempDir, "newhome")
				os.Setenv("HOME", testHome)
				return testHome
			},
			wantError: false,
			verify: func(t *testing.T, homeDir string) {
				configDir := filepath.Join(homeDir, ".centian")
				if _, err := os.Stat(configDir); os.IsNotExist(err) {
					t.Error("Config directory was not created")
				}
			},
		},
		{
			name: "directory already exists",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				testHome := filepath.Join(tempDir, "existinghome")
				configDir := filepath.Join(testHome, ".centian")
				os.MkdirAll(configDir, 0o755)
				os.Setenv("HOME", testHome)
				return testHome
			},
			wantError: false,
			verify: func(t *testing.T, homeDir string) {
				configDir := filepath.Join(homeDir, ".centian")
				if _, err := os.Stat(configDir); os.IsNotExist(err) {
					t.Error("Config directory should still exist")
				}
			},
		},
	}

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: test environment setup
			homeDir := tt.setup(t)

			// When: ensuring config directory exists
			err := EnsureConfigDir()

			// Then: verify expectations
			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
				if tt.verify != nil {
					tt.verify(t, homeDir)
				}
			}
		})
	}
}

// TestSaveConfigErrorHandling tests error scenarios in SaveConfig
func TestSaveConfigErrorHandling(t *testing.T) {
	// Given: a test environment
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	testHome := filepath.Join(tempDir, "savetest")
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	tests := []struct {
		name      string
		config    *GlobalConfig
		setup     func(t *testing.T)
		wantError bool
		errorMsg  string
	}{
		{
			name:      "save valid config",
			config:    DefaultConfig(),
			setup:     func(t *testing.T) {},
			wantError: false,
		},
		{
			name: "save config with processors",
			config: &GlobalConfig{
				Version: "1.0.0",
				Proxy:   &ProxySettings{},
				Processors: []*ProcessorConfig{
					{
						Name:    "test",
						Type:    "cli",
						Enabled: true,
						Config: map[string]interface{}{
							"command": "python",
						},
					},
				},
			},
			setup:     func(t *testing.T) {},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: test setup
			tt.setup(t)

			// When: saving config
			err := SaveConfig(tt.config)

			// Then: verify expectations
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

				// Verify file was created and is valid JSON
				configPath, _ := GetConfigPath()
				data, err := os.ReadFile(configPath)
				if err != nil {
					t.Fatalf("Failed to read saved config: %v", err)
				}

				var loaded GlobalConfig
				if err := json.Unmarshal(data, &loaded); err != nil {
					t.Fatalf("Saved config is not valid JSON: %v", err)
				}
			}
		})
	}
}
