package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConfigLifecycle tests the complete configuration lifecycle:
// create, load, modify, save, and validate.
func TestConfigLifecycle(t *testing.T) {
	// Setup - create temporary directory for testing.
	tempDir := t.TempDir()

	// Override home directory for testing.
	originalHome := os.Getenv("HOME")
	testHome := filepath.Join(tempDir, "testhome")
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// Test 1: Create default configuration.
	config := DefaultConfig()
	if config == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	// Verify default config structure.
	if config != nil && config.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", config.Version)
	}
	if config != nil && config.Proxy == nil {
		t.Fatal("Expected proxy settings to be initialized")
	}
	if config != nil && config.Proxy != nil && config.Proxy.Port != "9666" {
		t.Fatalf("Expected default proxy port 9666, got %s", config.Proxy.Port)
	}
	if config != nil && config.Proxy != nil && config.Proxy.Capabilities == nil {
		t.Fatal("Expected proxy capabilities to be initialized")
	}
	if config != nil && config.Proxy != nil && config.Proxy.Capabilities != nil && config.Proxy.Capabilities.EventStorage == nil {
		t.Fatal("Expected default event storage capability to be initialized")
	}
	if config != nil && config.Processors == nil {
		t.Fatal("Expected processors to be initialized")
	}
	if config != nil && !config.IsAuthEnabled() {
		t.Fatal("Expected auth to be enabled by default")
	}
	if config != nil && config.GetAuthHeader() == "" {
		t.Fatal("Expected default auth header to be set")
	}

	// Test 2: Save configuration.
	err := SaveConfig(config)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify config file was created.
	configPath, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath failed: %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created at %s", configPath)
	}

	// Test 3: Load configuration.
	loadedConfig, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify loaded config matches original.
	if loadedConfig.Version != config.Version {
		t.Errorf("Loaded version %s doesn't match original %s", loadedConfig.Version, config.Version)
	}

	// Test 5: Save modified configuration.
	err = SaveConfig(loadedConfig)
	if err != nil {
		t.Fatalf("SaveConfig with servers failed: %v", err)
	}

	// Test invalid config path (permission denied scenario would be complex to test reliably).
	t.Logf("Configuration lifecycle test completed successfully")
	t.Logf("Config file location: %s", configPath)
}

func TestGatewayConfigForceReadOnlyHintsEnabled(t *testing.T) {
	// Given: nil gateway config
	// Then: returns false
	var nilGateway *GatewayConfig
	if nilGateway.ForceReadOnlyHintsEnabled() {
		t.Fatal("expected false for nil gateway")
	}

	// Given: gateway with nil ForceReadOnlyHints
	// Then: returns false
	gateway := &GatewayConfig{}
	if gateway.ForceReadOnlyHintsEnabled() {
		t.Fatal("expected false for nil ForceReadOnlyHints")
	}

	// Given: gateway with ForceReadOnlyHints=false
	// Then: returns false
	f := false
	gateway.ForceReadOnlyHints = &f
	if gateway.ForceReadOnlyHintsEnabled() {
		t.Fatal("expected false for ForceReadOnlyHints=false")
	}

	// Given: gateway with ForceReadOnlyHints=true
	// Then: returns true
	tr := true
	gateway.ForceReadOnlyHints = &tr
	if !gateway.ForceReadOnlyHintsEnabled() {
		t.Fatal("expected true for ForceReadOnlyHints=true")
	}
}

func TestGatewayConfigForceSafeToolHintsEnabled(t *testing.T) {
	// Given: nil gateway config
	// Then: returns false
	var nilGateway *GatewayConfig
	if nilGateway.ForceSafeToolHintsEnabled() {
		t.Fatal("expected false for nil gateway")
	}

	// Given: gateway with nil ForceSafeToolHints
	// Then: returns false
	gateway := &GatewayConfig{}
	if gateway.ForceSafeToolHintsEnabled() {
		t.Fatal("expected false for nil ForceSafeToolHints")
	}

	// Given: gateway with ForceSafeToolHints=false
	// Then: returns false
	f := false
	gateway.ForceSafeToolHints = &f
	if gateway.ForceSafeToolHintsEnabled() {
		t.Fatal("expected false for ForceSafeToolHints=false")
	}

	// Given: gateway with ForceSafeToolHints=true
	// Then: returns true
	tr := true
	gateway.ForceSafeToolHints = &tr
	if !gateway.ForceSafeToolHintsEnabled() {
		t.Fatal("expected true for ForceSafeToolHints=true")
	}
}
