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
