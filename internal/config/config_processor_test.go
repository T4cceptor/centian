package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestProcessorValidation tests processor configuration validation.
func TestProcessorValidation(t *testing.T) {
	defaultGateways := map[string]*GatewayConfig{
		"default": &GatewayConfig{
			MCPServers: map[string]*MCPServerConfig{
				"test": {URL: "https://test123.com"},
			},
		},
	}
	tests := []struct {
		name      string
		config    *GlobalConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid cli processor",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "test-processor",
						Type:    "cli",
						Enabled: true,
						Timeout: 20,
						Config: map[string]interface{}{
							"command": "python",
							"args":    []interface{}{"script.py"},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "missing processor name",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Type:    "cli",
						Enabled: true,
						Config: map[string]interface{}{
							"command": "python",
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "name is required",
		},
		{
			name: "missing processor type",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "test-processor",
						Enabled: true,
						Config: map[string]interface{}{
							"command": "python",
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "type is required",
		},
		{
			name: "invalid processor type",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "test-processor",
						Type:    "http", // Not supported in v1
						Enabled: true,
						Config: map[string]interface{}{
							"url": "http://example.com",
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "unsupported type 'http'",
		},
		{
			name: "duplicate processor names",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "duplicate",
						Type:    "cli",
						Enabled: true,
						Config: map[string]interface{}{
							"command": "python",
						},
					},
					{
						Name:    "duplicate",
						Type:    "cli",
						Enabled: false,
						Config: map[string]interface{}{
							"command": "bash",
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "duplicate processor name",
		},
		{
			name: "missing config field",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "no-config",
						Type:    "cli",
						Enabled: true,
						Config:  nil,
					},
				},
			},
			wantError: true,
			errorMsg:  "config is required",
		},
		{
			name: "cli processor missing command",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "no-command",
						Type:    "cli",
						Enabled: true,
						Config: map[string]interface{}{
							"args": []interface{}{"arg1"},
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "config.command is required",
		},
		{
			name: "cli processor command not string",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "bad-command-type",
						Type:    "cli",
						Enabled: true,
						Config: map[string]interface{}{
							"command": 123, // Should be string
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "config.command must be a string",
		},
		{
			name: "cli processor args not array",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "bad-args-type",
						Type:    "cli",
						Enabled: true,
						Config: map[string]interface{}{
							"command": "python",
							"args":    "not-an-array", // Should be array
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "config.args must be an array",
		},
		{
			name: "empty processor list is valid",
			config: &GlobalConfig{
				Version:    "1.0.0",
				Gateways:   defaultGateways,
				Processors: []*ProcessorConfig{},
			},
			wantError: false,
		},
		{
			name: "nil processor list is valid",
			config: &GlobalConfig{
				Version:    "1.0.0",
				Gateways:   defaultGateways,
				Processors: nil,
			},
			wantError: false,
		},
		{
			name: "default timeout applied",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "default-timeout",
						Type:    "cli",
						Enabled: true,
						Timeout: 0, // Should default to 15
						Config: map[string]interface{}{
							"command": "python",
						},
					},
				},
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set proxy if not set (required for validation).
			if tt.config.Proxy == nil {
				tt.config.Proxy = &ProxySettings{}
			}

			err := ValidateConfig(tt.config)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
				} else if tt.errorMsg != "" {
					// Check if error message contains expected substring.
					if !contains(err.Error(), tt.errorMsg) {
						t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}

				// Verify default timeout was applied.
				if tt.name == "default timeout applied" {
					if tt.config.Processors[0].Timeout != 15 {
						t.Errorf("Expected default timeout 15, got %d", tt.config.Processors[0].Timeout)
					}
				}
			}
		})
	}
}

// TestProcessorConfigPersistence tests that processor configuration persists through save/load.
func TestProcessorConfigPersistence(t *testing.T) {
	// Setup.
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	testHome := filepath.Join(tempDir, "processor_test")
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// Create config with processors.
	config := DefaultConfig()
	config.Processors = []*ProcessorConfig{
		{
			Name:    "security-check",
			Type:    "cli",
			Enabled: true,
			Timeout: 20,
			Config: map[string]interface{}{
				"command": "python",
				"args":    []interface{}{"~/processors/security.py"},
			},
		},
		{
			Name:    "logger",
			Type:    "cli",
			Enabled: false,
			Timeout: 10,
			Config: map[string]interface{}{
				"command": "bash",
				"args":    []interface{}{"-c", "echo 'logging'"},
			},
		},
	}

	// Save config.
	err := SaveConfig(config)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Load config.
	loadedConfig, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify processors were persisted.
	if len(loadedConfig.Processors) != 2 {
		t.Fatalf("Expected 2 processors, got %d", len(loadedConfig.Processors))
	}

	// Verify first processor.
	p1 := loadedConfig.Processors[0]
	if p1.Name != "security-check" {
		t.Errorf("Processor 1 name: expected 'security-check', got '%s'", p1.Name)
	}
	if p1.Type != "cli" {
		t.Errorf("Processor 1 type: expected 'cli', got '%s'", p1.Type)
	}
	if !p1.Enabled {
		t.Error("Processor 1 should be enabled")
	}
	if p1.Timeout != 20 {
		t.Errorf("Processor 1 timeout: expected 20, got %d", p1.Timeout)
	}

	// Verify config fields.
	cmd, ok := p1.Config["command"].(string)
	if !ok || cmd != "python" {
		t.Errorf("Processor 1 command: expected 'python', got '%v'", cmd)
	}

	// Verify second processor.
	p2 := loadedConfig.Processors[1]
	if p2.Name != "logger" {
		t.Errorf("Processor 2 name: expected 'logger', got '%s'", p2.Name)
	}
	if p2.Enabled {
		t.Error("Processor 2 should be disabled")
	}
}

// Helper function to check if string contains substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == "" ||
		(s != "" && substr != "" && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
