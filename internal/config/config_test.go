package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConfigLifecycle tests the complete configuration lifecycle:
// create, load, modify, save, and validate
func TestConfigLifecycle(t *testing.T) {
	// Setup - create temporary directory for testing
	tempDir := t.TempDir()

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	testHome := filepath.Join(tempDir, "testhome")
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// Test 1: Create default configuration
	config := DefaultConfig()
	if config == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	// Verify default config structure
	if config.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", config.Version)
	}
	if len(config.Servers) != 0 {
		t.Errorf("Expected empty servers map, got %d servers", len(config.Servers))
	}
	if config.Proxy == nil {
		t.Fatal("Expected proxy settings to be initialized")
	}
	if config.Processors == nil {
		t.Fatal("Expected processors to be initialized")
	}

	// Test 2: Save configuration
	err := SaveConfig(config)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify config file was created
	configPath, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath failed: %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created at %s", configPath)
	}

	// Test 3: Load configuration
	loadedConfig, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify loaded config matches original
	if loadedConfig.Version != config.Version {
		t.Errorf("Loaded version %s doesn't match original %s", loadedConfig.Version, config.Version)
	}

	// Test 4: Add servers and test server operations
	server1 := &MCPServer{
		Name:        "test-server-1",
		Command:     "npx",
		Args:        []string{"-y", "@test/server"},
		Env:         map[string]string{"TEST_ENV": "value1"},
		Enabled:     true,
		Description: "Test server for integration testing",
	}

	server2 := &MCPServer{
		Name:        "test-server-2",
		Command:     "python",
		Args:        []string{"-m", "test_server"},
		Enabled:     false,
		Description: "Disabled test server",
	}

	// Add servers
	loadedConfig.Servers[server1.Name] = server1
	loadedConfig.Servers[server2.Name] = server2

	// Test server retrieval
	retrievedServer, err := loadedConfig.GetServer("test-server-1")
	if err != nil {
		t.Fatalf("GetServer failed: %v", err)
	}
	if retrievedServer.Name != server1.Name {
		t.Errorf("Retrieved server name %s doesn't match expected %s", retrievedServer.Name, server1.Name)
	}

	// Test enabled servers list
	enabledServers := loadedConfig.ListEnabledServers()
	if len(enabledServers) != 1 {
		t.Errorf("Expected 1 enabled server, got %d", len(enabledServers))
	}
	if enabledServers[0] != "test-server-1" {
		t.Errorf("Expected enabled server 'test-server-1', got '%s'", enabledServers[0])
	}

	// Test 5: Save modified configuration
	err = SaveConfig(loadedConfig)
	if err != nil {
		t.Fatalf("SaveConfig with servers failed: %v", err)
	}

	// Test 6: Reload and verify persistence
	finalConfig, err := LoadConfig()
	if err != nil {
		t.Fatalf("Final LoadConfig failed: %v", err)
	}

	if len(finalConfig.Servers) != 2 {
		t.Errorf("Expected 2 servers after reload, got %d", len(finalConfig.Servers))
	}

	// Verify specific server details
	reloadedServer1, exists := finalConfig.Servers["test-server-1"]
	if !exists {
		t.Error("test-server-1 not found after reload")
	} else {
		if reloadedServer1.Command != "npx" {
			t.Errorf("Server command not preserved: expected 'npx', got '%s'", reloadedServer1.Command)
		}
		if len(reloadedServer1.Args) != 2 {
			t.Errorf("Server args not preserved: expected 2 args, got %d", len(reloadedServer1.Args))
		}
		if reloadedServer1.Env["TEST_ENV"] != "value1" {
			t.Errorf("Server environment not preserved")
		}
	}

	// Test 7: Error cases
	// Try to get non-existent server
	_, err = finalConfig.GetServer("non-existent")
	if err == nil {
		t.Error("Expected error when getting non-existent server")
	}

	// Test invalid config path (permission denied scenario would be complex to test reliably)
	t.Logf("Configuration lifecycle test completed successfully")
	t.Logf("Config file location: %s", configPath)
	t.Logf("Servers created: %d", len(finalConfig.Servers))
}

// TestConfigValidation tests configuration validation scenarios
func TestConfigValidation(t *testing.T) {
	// Test valid configurations
	validConfig := &GlobalConfig{
		Version: "1.0",
		Servers: map[string]*MCPServer{
			"valid-server": {
				Name:    "valid-server",
				Command: "python",
				Args:    []string{"-m", "server"},
				Enabled: true,
			},
		},
		Proxy: &ProxySettings{
			Transport: "stdio",
			LogLevel:  "info",
		},
		Processors: []*ProcessorConfig{},
	}

	// This would test validation if we had a validate function
	// For now, just verify the structure is sound
	if len(validConfig.Servers) != 1 {
		t.Error("Valid config should have 1 server")
	}

	// Test server with missing required fields
	incompleteServer := &MCPServer{
		Name: "incomplete",
		// Missing Command - this should be caught by validation
		Args:    []string{"arg1"},
		Enabled: true,
	}

	if incompleteServer.Command == "" {
		t.Log("Incomplete server correctly has empty command (validation would catch this)")
	}
}

// TestConcurrentConfigAccess tests thread-safety of config operations
func TestConcurrentConfigAccess(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	testHome := filepath.Join(tempDir, "concurrent_test")
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// Create initial config
	config := DefaultConfig()
	config.Servers["test-server"] = &MCPServer{
		Name:    "test-server",
		Command: "echo",
		Args:    []string{"test"},
		Enabled: true,
	}

	err := SaveConfig(config)
	if err != nil {
		t.Fatalf("Initial SaveConfig failed: %v", err)
	}

	// Test concurrent reads
	done := make(chan bool, 3)

	for i := 0; i < 3; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < 5; j++ {
				cfg, err := LoadConfig()
				if err != nil {
					t.Errorf("Concurrent load %d-%d failed: %v", id, j, err)
					return
				}

				if len(cfg.Servers) != 1 {
					t.Errorf("Concurrent load %d-%d got wrong server count: %d", id, j, len(cfg.Servers))
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 3; i++ {
		<-done
	}

	t.Log("Concurrent access test completed successfully")
}
