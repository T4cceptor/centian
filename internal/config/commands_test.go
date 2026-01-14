package config

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
	"gotest.tools/assert"
)

// ========================================
// Test Helpers
// ========================================

func setupTestEnv(t *testing.T) func() {
	// Create temp directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")

	// Set HOME to temp directory
	os.Setenv("HOME", tempDir)

	cleanup := func() {
		os.Setenv("HOME", originalHome)
	}

	return cleanup
}

func createTestConfig(t *testing.T) {
	config := DefaultConfig()
	err := SaveConfig(config)
	assert.NilError(t, err)
}

// ========================================
// initConfig Tests
// ========================================

func TestInitConfig_CreatesDefaultConfig(t *testing.T) {
	// Given: a clean environment
	cleanup := setupTestEnv(t)
	defer cleanup()

	ctx := context.Background()
	cmd := &cli.Command{}

	// When: initializing config
	err := initConfig(ctx, cmd)

	// Then: should create config successfully
	assert.NilError(t, err)

	// And: config file should exist
	configPath, _ := GetConfigPath()
	_, err = os.Stat(configPath)
	assert.NilError(t, err)

	// And: should be able to load the config
	config, err := LoadConfig()
	assert.NilError(t, err)
	assert.Assert(t, config != nil)
	assert.Equal(t, "1.0.0", config.Version)
}

func TestInitConfig_FailsIfConfigExists(t *testing.T) {
	// Given: an existing config
	cleanup := setupTestEnv(t)
	defer cleanup()

	createTestConfig(t)

	ctx := context.Background()
	cmd := &cli.Command{}

	// When: trying to initialize config again
	err := initConfig(ctx, cmd)

	// Then: should return error
	assert.ErrorContains(t, err, "configuration already exists")
}

// ========================================
// showConfig Tests
// ========================================

func TestShowConfig_DisplaysTextFormat(t *testing.T) {
	// Given: an existing config
	cleanup := setupTestEnv(t)
	defer cleanup()

	createTestConfig(t)

	ctx := context.Background()
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json"},
		},
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: showing config in text format
	err := showConfig(ctx, cmd)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Then: should succeed
	assert.NilError(t, err)

	// And: should display key information
	assert.Assert(t, strings.Contains(output, "Configuration path:"))
	assert.Assert(t, strings.Contains(output, "Version: 1.0.0"))
	assert.Assert(t, strings.Contains(output, "Gateways:"))
	assert.Assert(t, strings.Contains(output, "Servers:"))
}

func TestShowConfig_DisplaysJSONFormat(t *testing.T) {
	// Given: an existing config
	cleanup := setupTestEnv(t)
	defer cleanup()

	createTestConfig(t)

	ctx := context.Background()
	// Create a mock command that returns true for Bool("json")
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Value: true},
		},
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: showing config in JSON format
	err := showConfig(ctx, cmd)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Then: should succeed
	assert.NilError(t, err)

	// And: should output valid JSON
	assert.Assert(t, strings.Contains(output, "\"version\""))
	assert.Assert(t, strings.Contains(output, "\"proxy\""))
}

func TestShowConfig_FailsIfNoConfig(t *testing.T) {
	// Given: no config file
	cleanup := setupTestEnv(t)
	defer cleanup()

	ctx := context.Background()
	cmd := &cli.Command{}

	// When: trying to show config
	err := showConfig(ctx, cmd)

	// Then: should return error
	assert.ErrorContains(t, err, "failed to load configuration")
}

// ========================================
// validateConfig Tests
// ========================================

func TestValidateConfig_SucceedsForValidConfig(t *testing.T) {
	// Given: a valid config
	cleanup := setupTestEnv(t)
	defer cleanup()

	createTestConfig(t)

	ctx := context.Background()
	cmd := &cli.Command{}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: validating config
	err := validateConfig(ctx, cmd)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Then: should succeed
	assert.NilError(t, err)

	// And: should display success message
	assert.Assert(t, strings.Contains(output, "✅ Configuration is valid"))
}

func TestValidateConfig_FailsIfNoConfig(t *testing.T) {
	// Given: no config file
	cleanup := setupTestEnv(t)
	defer cleanup()

	ctx := context.Background()
	cmd := &cli.Command{}

	// When: validating config
	err := validateConfig(ctx, cmd)

	// Then: should return error
	assert.ErrorContains(t, err, "Configuration validation failed")
}

func TestValidateConfig_FailsForInvalidConfig(t *testing.T) {
	// Given: an invalid config file
	cleanup := setupTestEnv(t)
	defer cleanup()

	// Create .centian directory
	centianDir := filepath.Join(os.Getenv("HOME"), ".centian")
	err := os.MkdirAll(centianDir, 0o755)
	assert.NilError(t, err)

	// Write invalid JSON
	configPath := filepath.Join(centianDir, "config.jsonc")
	err = os.WriteFile(configPath, []byte("{invalid json}"), 0o644)
	assert.NilError(t, err)

	ctx := context.Background()
	cmd := &cli.Command{}

	// When: validating config
	err = validateConfig(ctx, cmd)

	// Then: should return error
	assert.ErrorContains(t, err, "Configuration validation failed")
}

// ========================================
// listServers Tests
// ========================================

func TestListServers_DisplaysAllServers(t *testing.T) {
	// Given: a config with servers
	cleanup := setupTestEnv(t)
	defer cleanup()

	createTestConfig(t)

	// Add a test server
	config, _ := LoadConfig()
	gateway := &GatewayConfig{
		MCPServers: map[string]*MCPServerConfig{
			"test-server": {
				Name:    "test-server",
				Enabled: true,
				Command: "npx",
				Args:    []string{"test"},
			},
			"disabled-server": {
				Name:    "disabled-server",
				Enabled: false,
				Command: "npx",
				Args:    []string{"disabled"},
			},
		},
	}
	if config.Gateways == nil {
		config.Gateways = map[string]*GatewayConfig{}
	}
	config.Gateways["test-gateway"] = gateway
	SaveConfig(config)

	ctx := context.Background()
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "enabled-only"},
		},
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: listing servers
	err := listServers(ctx, cmd)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Then: should succeed
	assert.NilError(t, err)

	// And: should display both servers
	assert.Assert(t, strings.Contains(output, "test-server"))
	assert.Assert(t, strings.Contains(output, "disabled-server"))
}

func TestListServers_DisplaysOnlyEnabledServers(t *testing.T) {
	// Given: a config with enabled and disabled servers
	cleanup := setupTestEnv(t)
	defer cleanup()

	createTestConfig(t)

	config, _ := LoadConfig()
	gateway := &GatewayConfig{
		MCPServers: map[string]*MCPServerConfig{
			"enabled-server": {
				Name:    "enabled-server",
				Enabled: true,
				Command: "npx",
				Args:    []string{"enabled"},
			},
			"disabled-server": {
				Name:    "disabled-server",
				Enabled: false,
				Command: "npx",
				Args:    []string{"disabled"},
			},
		},
	}
	if config.Gateways == nil {
		config.Gateways = make(map[string]*GatewayConfig)
	}
	config.Gateways["test-gateway"] = gateway
	SaveConfig(config)

	ctx := context.Background()
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "enabled-only", Value: true},
		},
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: listing only enabled servers
	err := listServers(ctx, cmd)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Then: should succeed
	assert.NilError(t, err)

	// And: should display only enabled server
	assert.Assert(t, strings.Contains(output, "enabled-server"))
	assert.Assert(t, !strings.Contains(output, "disabled-server"))
}

func TestListServers_FailsIfNoConfig(t *testing.T) {
	// Given: no config file
	cleanup := setupTestEnv(t)
	defer cleanup()

	ctx := context.Background()
	cmd := &cli.Command{}

	// When: listing servers
	err := listServers(ctx, cmd)

	// Then: should return error
	assert.ErrorContains(t, err, "failed to load configuration")
}

// ========================================
// removeConfig Tests
// ========================================

func TestRemoveConfig_RemovesConfigWithForceFlag(t *testing.T) {
	// Given: an existing config
	cleanup := setupTestEnv(t)
	defer cleanup()

	createTestConfig(t)

	configPath, _ := GetConfigPath()
	centianDir := filepath.Dir(configPath)

	// Verify config exists
	_, err := os.Stat(configPath)
	assert.NilError(t, err)

	ctx := context.Background()
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "force", Value: true},
		},
	}

	// When: removing config with force flag
	err = removeConfig(ctx, cmd)

	// Then: should succeed
	assert.NilError(t, err)

	// And: centian directory should be removed
	_, err = os.Stat(centianDir)
	assert.Assert(t, os.IsNotExist(err))
}

func TestRemoveConfig_SucceedsIfNoConfig(t *testing.T) {
	// Given: no config file
	cleanup := setupTestEnv(t)
	defer cleanup()

	ctx := context.Background()
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "force", Value: true},
		},
	}

	// When: trying to remove non-existent config
	err := removeConfig(ctx, cmd)

	// Then: should return error
	assert.NilError(t, err)
}
