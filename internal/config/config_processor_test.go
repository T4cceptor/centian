package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestProcessorValidation tests processor configuration validation.
func TestProcessorValidation(t *testing.T) {
	defaultGateways := map[string]*GatewayConfig{
		"default": {
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
			name: "valid webhook processor",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "webhook-processor",
						Type:    "webhook",
						Enabled: true,
						Timeout: 20,
						Config: map[string]interface{}{
							"url": "https://example.com/processor",
							"headers": map[string]interface{}{
								"Authorization": "Bearer ${TOKEN}",
							},
						},
					},
				},
			},
			wantError: false,
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
			name: "invalid processor part",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "bad-part",
						Type:    "cli",
						Enabled: true,
						Parts:   []string{"payload", "unknown-part"},
						Config: map[string]interface{}{
							"command": "python",
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "unsupported part 'unknown-part'",
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
			name: "webhook processor missing url",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "no-url",
						Type:    "webhook",
						Enabled: true,
						Config:  map[string]interface{}{},
					},
				},
			},
			wantError: true,
			errorMsg:  "config.url is required",
		},
		{
			name: "webhook processor url not string",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "bad-url",
						Type:    "webhook",
						Enabled: true,
						Config: map[string]interface{}{
							"url": 123,
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "config.url must be a string",
		},
		{
			name: "webhook processor invalid headers shape",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "bad-headers",
						Type:    "webhook",
						Enabled: true,
						Config: map[string]interface{}{
							"url":     "http://example.com/processor",
							"headers": "not-an-object",
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "config.headers must be an object with string values",
		},
		{
			name: "webhook processor rejects unsupported method field",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "bad-method",
						Type:    "webhook",
						Enabled: true,
						Config: map[string]interface{}{
							"url":    "http://example.com/processor",
							"method": "POST",
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "config.method is unsupported",
		},
		{
			name: "webhook processor rejects retry config",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "bad-retry",
						Type:    "webhook",
						Enabled: true,
						Config: map[string]interface{}{
							"url": "http://example.com/processor",
							"retry": map[string]interface{}{
								"max_attempts": 3,
							},
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "config.retry is unsupported",
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
			name: "prompt injection guard requires required true",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:    "prompt-injection",
						Type:    "builtin",
						Enabled: true,
						Parts:   []string{"payload", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinPromptInjectionGuard,
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "must set required=true",
		},
		{
			name: "prompt injection guard default parts omit annotations",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "prompt-injection",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Config: map[string]interface{}{
							"processor": BuiltinPromptInjectionGuard,
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "requires part 'annotations'",
		},
		{
			name: "prompt injection guard requires payload part",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "prompt-injection",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinPromptInjectionGuard,
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "requires part 'payload'",
		},
		{
			name: "valid prompt injection guard",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "prompt-injection",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"payload", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinPromptInjectionGuard,
							"mode":      "redact",
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "valid pattern redaction processor",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "custom-redactor",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"payload", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinPatternRedactionProcessor,
							"mode":      "redact",
							"scope":     "both",
							"rules": []interface{}{
								map[string]interface{}{
									"name":        "internal_token",
									"pattern":     `it_[a-z]{20}`,
									"replacement": "[REDACTED_INTERNAL_TOKEN]",
								},
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "pattern redaction processor requires rules",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "custom-redactor",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"payload", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinPatternRedactionProcessor,
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "config.rules must contain at least one rule",
		},
		{
			name: "pattern redaction processor validates regex",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "custom-redactor",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"payload", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinPatternRedactionProcessor,
							"rules": []interface{}{
								map[string]interface{}{
									"name":        "bad",
									"pattern":     `[`,
									"replacement": "[REDACTED]",
								},
							},
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "config.rules[0].pattern is invalid",
		},
		{
			name: "redaction processor validates mode",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "secret-redactor",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"payload", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinSecretTokenRedactor,
							"mode":      "remove",
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "config.mode must be 'redact' or 'annotate'",
		},
		{
			name: "redaction processor validates scope",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "pii-redactor",
						Type:     "builtin",
						Enabled:  true,
						Required: false,
						Parts:    []string{"payload", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinPIIRedactor,
							"scope":     "everything",
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "config.scope must be 'request', 'response', or 'both'",
		},
		{
			name: "redaction processor requires annotations part",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "secret-redactor",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"payload"},
						Config: map[string]interface{}{
							"processor": BuiltinSecretTokenRedactor,
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "requires part 'annotations'",
		},
		{
			name: "secret redactor rejects custom rules",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "secret-redactor",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"payload", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinSecretTokenRedactor,
							"rules": []interface{}{
								map[string]interface{}{
									"name":        "custom",
									"pattern":     `x+`,
									"replacement": "[REDACTED]",
								},
							},
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "config.rules is only supported for pattern_redaction_processor",
		},
		{
			name: "valid tool call guard processor",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "tool-call-guard",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"payload", "routing", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinToolCallGuard,
							"mode":      "block",
							"presets":   []interface{}{BuiltinToolGuardPresetDangerousCommands},
							"rules": []interface{}{
								map[string]interface{}{
									"name":          "block_prod_environment",
									"severity":      "medium",
									"message":       "Production operations are blocked.",
									"tool_patterns": []interface{}{"deploy___*"},
									"argument_rules": []interface{}{
										map[string]interface{}{
											"path":    "environment",
											"pattern": "^prod$",
										},
									},
								},
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "tool call guard validates mode",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "tool-call-guard",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"payload", "routing", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinToolCallGuard,
							"mode":      "redact",
							"presets":   []interface{}{BuiltinToolGuardPresetDangerousCommands},
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "config.mode must be 'block' or 'annotate'",
		},
		{
			name: "tool call guard validates preset",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "tool-call-guard",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"payload", "routing", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinToolCallGuard,
							"presets":   []interface{}{"unknown"},
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "unsupported preset",
		},
		{
			name: "tool call guard validates glob",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "tool-call-guard",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"payload", "routing", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinToolCallGuard,
							"rules": []interface{}{
								map[string]interface{}{
									"name":          "bad_glob",
									"tool_patterns": []interface{}{"["},
								},
							},
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "invalid glob",
		},
		{
			name: "tool call guard validates argument regex",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "tool-call-guard",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"payload", "routing", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinToolCallGuard,
							"rules": []interface{}{
								map[string]interface{}{
									"name": "bad_regex",
									"argument_rules": []interface{}{
										map[string]interface{}{"pattern": "["},
									},
								},
							},
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "pattern is invalid",
		},
		{
			name: "tool call guard validates malformed argument rule",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "tool-call-guard",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"payload", "routing", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinToolCallGuard,
							"rules": []interface{}{
								map[string]interface{}{
									"name": "malformed",
									"argument_rules": []interface{}{
										map[string]interface{}{},
									},
								},
							},
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "requires path or pattern",
		},
		{
			name: "tool call guard requires routing part",
			config: &GlobalConfig{
				Version:  "1.0.0",
				Gateways: defaultGateways,
				Processors: []*ProcessorConfig{
					{
						Name:     "tool-call-guard",
						Type:     "builtin",
						Enabled:  true,
						Required: true,
						Parts:    []string{"payload", "annotations"},
						Config: map[string]interface{}{
							"processor": BuiltinToolCallGuard,
							"presets":   []interface{}{BuiltinToolGuardPresetDangerousCommands},
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "requires part 'routing'",
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

			err := ValidateConfig(tt.config, true)

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
		{
			Name:    "webhook-audit",
			Type:    "webhook",
			Enabled: true,
			Timeout: 25,
			Config: map[string]interface{}{
				"url": "https://example.com/audit",
				"headers": map[string]interface{}{
					"Authorization": "Bearer ${TOKEN}",
				},
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
	if len(loadedConfig.Processors) != 3 {
		t.Fatalf("Expected 3 processors, got %d", len(loadedConfig.Processors))
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

	p3 := loadedConfig.Processors[2]
	if p3.Name != "webhook-audit" {
		t.Errorf("Processor 3 name: expected 'webhook-audit', got '%s'", p3.Name)
	}
	if p3.Type != "webhook" {
		t.Errorf("Processor 3 type: expected 'webhook', got '%s'", p3.Type)
	}
	if p3.Timeout != 25 {
		t.Errorf("Processor 3 timeout: expected 25, got %d", p3.Timeout)
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
