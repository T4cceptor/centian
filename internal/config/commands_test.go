package config

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/urfave/cli/v3"
	"gotest.tools/assert"
)

// ========================================
// Test Helpers
// ========================================

func setupTestEnv(t *testing.T) func() {
	// Create temp directory.
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")

	// Set HOME to temp directory.
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

// createValidTestConfigWithGateway creates a test config with a valid gateway and server.
// Use this for tests that require a fully valid config (e.g., validation tests).
func createValidTestConfigWithGateway(t *testing.T) {
	config := DefaultConfig()
	config.Gateways = map[string]*GatewayConfig{
		"test-gateway": {
			MCPServers: map[string]*MCPServerConfig{
				"test-server": {
					Name: "test-server",
					URL:  "https://example.com/mcp",
				},
			},
		},
	}
	err := SaveConfig(config)
	assert.NilError(t, err)
}

// ========================================
// initConfig Tests
// ========================================

func TestInitConfig_CreatesDefaultConfig(t *testing.T) {
	// Given: a clean environment.
	cleanup := setupTestEnv(t)
	defer cleanup()

	ctx := context.Background()
	cmd := &cli.Command{}

	// When: initializing config.
	err := initConfig(ctx, cmd)

	// Then: should create config successfully.
	assert.NilError(t, err)

	// And: config file should exist.
	configPath, _ := GetConfigPath()
	_, err = os.Stat(configPath)
	assert.NilError(t, err)

	// And: should be able to load the config.
	config, err := LoadConfig()
	assert.NilError(t, err)
	assert.Assert(t, config != nil)
	assert.Equal(t, "1.0.0", config.Version)
}

func TestInitConfig_FailsIfConfigExists(t *testing.T) {
	// Given: an existing config.
	cleanup := setupTestEnv(t)
	defer cleanup()

	createTestConfig(t)

	ctx := context.Background()
	cmd := &cli.Command{}

	// When: trying to initialize config again.
	err := initConfig(ctx, cmd)

	// Then: should return error.
	assert.ErrorContains(t, err, "configuration already exists")
}

// ========================================
// showConfig Tests
// ========================================

func TestShowConfig_DisplaysTextFormat(t *testing.T) {
	// Given: an existing config.
	cleanup := setupTestEnv(t)
	defer cleanup()

	createTestConfig(t)

	ctx := context.Background()
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json"},
		},
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: showing config in text format.
	err := showConfig(ctx, cmd)

	// Restore stdout.
	w.Close()
	os.Stdout = oldStdout

	// Read output.
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Then: should succeed.
	assert.NilError(t, err)

	// And: should display key information.
	assert.Assert(t, strings.Contains(output, "Configuration path:"))
	assert.Assert(t, strings.Contains(output, "Version: 1.0.0"))
	assert.Assert(t, strings.Contains(output, "Gateways:"))
	assert.Assert(t, strings.Contains(output, "Servers:"))
}

func TestShowConfig_DisplaysJSONFormat(t *testing.T) {
	// Given: an existing config.
	cleanup := setupTestEnv(t)
	defer cleanup()

	createTestConfig(t)

	ctx := context.Background()
	// Create a mock command that returns true for Bool("json").
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Value: true},
		},
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: showing config in JSON format.
	err := showConfig(ctx, cmd)

	// Restore stdout.
	w.Close()
	os.Stdout = oldStdout

	// Read output.
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Then: should succeed.
	assert.NilError(t, err)

	// And: should output valid JSON.
	assert.Assert(t, strings.Contains(output, "\"version\""))
	assert.Assert(t, strings.Contains(output, "\"proxy\""))
}

func TestShowConfig_FailsIfNoConfig(t *testing.T) {
	// Given: no config file.
	cleanup := setupTestEnv(t)
	defer cleanup()

	ctx := context.Background()
	cmd := &cli.Command{}

	// When: trying to show config.
	err := showConfig(ctx, cmd)

	// Then: should return error.
	assert.ErrorContains(t, err, "failed to load configuration")
}

// ========================================
// validateConfig Tests
// ========================================

func TestValidateConfig_SucceedsForValidConfig(t *testing.T) {
	// Given: a valid config with gateway (required for server operation).
	cleanup := setupTestEnv(t)
	defer cleanup()

	createValidTestConfigWithGateway(t)

	ctx := context.Background()
	cmd := &cli.Command{}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: validating config.
	err := validateConfig(ctx, cmd)

	// Restore stdout.
	w.Close()
	os.Stdout = oldStdout

	// Read output.
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Then: should succeed.
	assert.NilError(t, err)

	// And: should display success message.
	assert.Assert(t, strings.Contains(output, "✅ Configuration is valid"))
}

func TestValidateConfig_FailsIfNoConfig(t *testing.T) {
	// Given: no config file.
	cleanup := setupTestEnv(t)
	defer cleanup()

	ctx := context.Background()
	cmd := &cli.Command{}

	// When: validating config.
	err := validateConfig(ctx, cmd)

	// Then: should return error.
	assert.ErrorContains(t, err, "Configuration validation failed")
}

func TestValidateConfig_FailsForInvalidConfig(t *testing.T) {
	// Given: an invalid config file.
	cleanup := setupTestEnv(t)
	defer cleanup()

	// Create .centian directory.
	centianDir := filepath.Join(os.Getenv("HOME"), ".centian")
	err := os.MkdirAll(centianDir, 0o755)
	assert.NilError(t, err)

	// Write invalid JSON.
	configPath := filepath.Join(centianDir, "config.json")
	err = os.WriteFile(configPath, []byte("{invalid json}"), 0o644)
	assert.NilError(t, err)

	ctx := context.Background()
	cmd := &cli.Command{}

	// When: validating config.
	err = validateConfig(ctx, cmd)

	// Then: should return error.
	assert.ErrorContains(t, err, "Configuration validation failed")
}

// ========================================
// listServers Tests
// ========================================

func TestListServers_DisplaysAllServers(t *testing.T) {
	// Given: a config with servers.
	cleanup := setupTestEnv(t)
	defer cleanup()

	createTestConfig(t)

	// Add a test server.
	config, _ := LoadConfig()
	disabled := false
	gateway := &GatewayConfig{
		MCPServers: map[string]*MCPServerConfig{
			"test-server": {
				Name:    "test-server",
				Command: "npx",
				Args:    []string{"test"},
			},
			"disabled-server": {
				Name:    "disabled-server",
				Enabled: &disabled,
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

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: listing servers.
	err := listServers(ctx, cmd)

	// Restore stdout.
	w.Close()
	os.Stdout = oldStdout

	// Read output.
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Then: should succeed.
	assert.NilError(t, err)

	// And: should display both servers.
	assert.Assert(t, strings.Contains(output, "test-server"))
	assert.Assert(t, strings.Contains(output, "disabled-server"))
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = orig

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	_ = r.Close()
	return string(out)
}

func TestListServers_Details(t *testing.T) {
	// Given: no config, context and command.
	// prep no config:.
	configPath, confErr := GetConfigPath()
	tmpConfPath := fmt.Sprintf("/tmp/centian_test_config_%d.json", time.Now().UnixNano())
	_, statErr := os.Stat(configPath)
	if statErr == nil && (configPath != "" || confErr != nil) {
		os.Rename(configPath, tmpConfPath)
	}

	ctx := context.Background()
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "enabled-only"},
		},
	}

	// When: calling listServers.
	failedToLoadConfig1 := listServers(ctx, cmd)

	// Then: err is as expected (due to no config).
	assert.ErrorContains(t, failedToLoadConfig1, "failed to load configuration")

	got := captureStdout(t, func() {
		// When: adding a real config and calling listServers.
		proxySettings := NewDefaultProxySettings()
		newConfig := GlobalConfig{
			Name:    "test config",
			Version: "1.0.0",
			Proxy:   &proxySettings,
		}
		saveError := SaveConfig(&newConfig)
		assert.NilError(t, saveError)
		noGetwayError := listServers(ctx, cmd)
		assert.NilError(t, noGetwayError) // in this case we do NOT return an error
	})
	want := "No gateways configured.\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	gatewayName := fmt.Sprintf("my-gateway-%d", time.Now().UnixNano())
	serverName := fmt.Sprintf("my-server-%d", time.Now().UnixNano())
	got = captureStdout(t, func() {
		// When: adding a real config WITH Gateways and servers and calling listServers.
		proxySettings := NewDefaultProxySettings()
		newConfig := GlobalConfig{
			Name:    "test config",
			Version: "1.0.0",
			Proxy:   &proxySettings,
			Gateways: map[string]*GatewayConfig{
				gatewayName: {
					MCPServers: map[string]*MCPServerConfig{
						serverName: {
							URL: "https://test-url.test123",
						},
					},
				},
			},
		}
		saveError := SaveConfig(&newConfig)
		assert.NilError(t, saveError)
		noGetwayError := listServers(ctx, cmd)
		assert.NilError(t, noGetwayError) // in this case we do NOT return an error
	})
	want = "MCP Servers:\n"
	if !strings.Contains(got, want) {
		t.Fatalf("got %q, which does not include %q", got, want)
	}
	if !strings.Contains(got, gatewayName) {
		t.Fatalf("got %q, which does not include gatewayName %q", got, gatewayName)
	}
	if !strings.Contains(got, serverName) {
		t.Fatalf("got %q, which does not include serverName %q", got, serverName)
	}

	if statErr == nil && configPath != "" || confErr != nil {
		e1 := os.Rename(tmpConfPath, configPath)
		assert.NilError(t, e1) // sanity check
		_, e := LoadConfig()
		assert.NilError(t, e) // sanity check
	}
}

func TestAddServer_Details(t *testing.T) {
	ctx := context.Background()
	serverCmdVal := "my-test-server-2"
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "name", Value: serverCmdVal},
		},
	}

	// existing config prep.
	configPath, confErr := GetConfigPath()
	tmpConfPath := fmt.Sprintf("/tmp/centian_test_config_%d.json", time.Now().UnixNano())
	_, statErr := os.Stat(configPath)
	if statErr == nil && (configPath != "" || confErr != nil) {
		os.Rename(configPath, tmpConfPath)
	}

	// When: calling addServer without an existing config.
	failedToLoadConfig1 := addServer(ctx, cmd)

	// Then: err is as expected (due to no config).
	assert.ErrorContains(t, failedToLoadConfig1, "failed to load configuration")

	// When: calling addServer WITH an existing config and NO gateways.
	proxySettings := NewDefaultProxySettings()
	newConfig := GlobalConfig{
		Name:     "test config",
		Version:  "1.0.0",
		Proxy:    &proxySettings,
		Gateways: map[string]*GatewayConfig{},
	}
	saveError := SaveConfig(&newConfig)
	assert.NilError(t, saveError)

	serverName := fmt.Sprintf("my-server-%d", time.Now().UnixNano())
	cmd = &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "gateway", Value: "default"},
			&cli.StringFlag{Name: "name", Value: serverName},
			&cli.StringFlag{Name: "command", Value: "npx"},
		},
	}
	noError := addServer(ctx, cmd)

	// Then: the command works as expected, and a new server is added under the "default" gateway.
	assert.NilError(t, noError)
	config, err := LoadConfig()
	assert.NilError(t, err)
	assert.Equal(t, len(config.Gateways), 1)
	assert.Equal(t, len(config.Gateways["default"].MCPServers), 1)

	mcpServer, ok := config.Gateways["default"].MCPServers[serverName]
	assert.Equal(t, ok, true)
	assert.Equal(t, mcpServer.Command, "npx")

	// When: adding an existing servername.
	cmd = &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "gateway", Value: "default"},
			&cli.StringFlag{Name: "name", Value: serverName},
		},
	}
	serverExistsErr := addServer(ctx, cmd)

	// Then: there is a an error "server '%s' already exists".
	expectedError := fmt.Sprintf("server '%s' already exists", serverName)
	assert.Error(t, serverExistsErr, expectedError)
}

func TestPromptUserToSelectServer_Details(t *testing.T) {
	server1 := MCPServerConfig{
		Name:    "server1",
		Command: "npx",
		Args:    []string{"1", "2", "3"},
	}
	disabled := false
	server2 := MCPServerConfig{
		Name:        "server2",
		URL:         "https://awesomemcp.test123",
		Headers:     make(map[string]string),
		Enabled:     &disabled,
		Description: "test123",
	}
	results := []ServerSearchResult{
		{
			gatewayName: "gateway1",
			gateway: &GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": &server1,
					"server2": &server2,
				},
			},
			server: &server1,
		},
		{
			gatewayName: "gateway2",
			gateway: &GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": &server1,
					"server2": &server2,
				},
			},
			server: &server1,
		},
		{
			gatewayName: "gateway2",
			gateway: &GatewayConfig{
				MCPServers: map[string]*MCPServerConfig{
					"server1": &server1,
					"server2": &server2,
				},
			},
			server: &server2,
		},
	}
	r, w, _ := os.Pipe()
	orig := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = orig }()
	w.WriteString("1\n")
	w.Close()

	got := captureStdout(t, func() {
		result, err := promptUserToSelectServer(results, "server1")
		// TODO: put stdin, then expect certain result - then check out.
		assert.NilError(t, err)
		assert.Assert(t, result.server.Name == "server1")
		assert.Assert(t, result.gatewayName == "gateway1")
	})
	assert.Assert(t, strings.Contains(got, "Status: ✅ enabled")) //
	assert.Assert(t, strings.Contains(got, "Status: ❌ disabled"))
	assert.Assert(t, strings.Contains(got, "command: npx"))
	assert.Assert(t, strings.Contains(got, "url: https://awesomemcp.test123"))
	assert.Assert(t, strings.Contains(got, "Gateway: gateway1"))
	assert.Assert(t, strings.Contains(got, "Gateway: gateway2"))
	assert.Assert(t, strings.Contains(got, "Transport: stdio"))
	assert.Assert(t, strings.Contains(got, "Transport: http"))
	assert.Assert(t, strings.Contains(got, "Select gateway [1-3] or 'c' to cancel:"))
	assert.Assert(t, strings.Contains(got, "Description: test123"))
}

func TestRemoveServer_Details(t *testing.T) {
	ctx := context.Background()
	serverName := "my-test-server-2"
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "name", Value: serverName},
		},
	}

	// handle existing config.
	configPath, confErr := GetConfigPath()
	tmpConfPath := fmt.Sprintf("/tmp/centian_test_config_%d.json", time.Now().UnixNano())
	_, statErr := os.Stat(configPath)
	if statErr == nil && (configPath != "" || confErr != nil) {
		os.Rename(configPath, tmpConfPath)
	}

	// When: calling removeServer without config.
	noConfigError := removeServer(ctx, cmd)
	// Then: correct error is returned.
	assert.ErrorContains(t, noConfigError, "failed to load configuration")

	// Given: existing config without server.
	proxySettings := NewDefaultProxySettings()
	newConfig := GlobalConfig{
		Name:    "test config",
		Version: "1.0.0",
		Proxy:   &proxySettings,
	}
	saveError := SaveConfig(&newConfig)
	assert.NilError(t, saveError)

	// When: calling removeServer.
	noServerError := removeServer(ctx, cmd)
	// Unable to find server '%s' in config.
	assert.ErrorContains(t, noServerError, "unable to find server 'my-test-server-2' in config")

	server := MCPServerConfig{
		Name:    serverName,
		Command: "npx",
	}
	newConfig.Gateways = map[string]*GatewayConfig{
		"gateway1": {
			MCPServers: map[string]*MCPServerConfig{
				serverName: &server,
				"test123": {
					Name:    "test123",
					Command: "npx",
				},
			},
		},
	}
	saveError = SaveConfig(&newConfig)
	assert.NilError(t, saveError)

	// When: calling remnoveServer.
	noError := removeServer(ctx, cmd)

	// Then: it successfully removes the server.
	assert.NilError(t, noError)
	config, err := LoadConfig()
	assert.NilError(t, err)
	_, ok := config.Gateways["gateway1"].MCPServers[serverName]
	assert.Assert(t, !ok)
}

func TestToggleServer_Details(t *testing.T) {
	serverName := "my-test-server-2"

	// handle existing config.
	configPath, confErr := GetConfigPath()
	tmpConfPath := fmt.Sprintf("/tmp/centian_test_config_%d.json", time.Now().UnixNano())
	_, statErr := os.Stat(configPath)
	if statErr == nil && (configPath != "" || confErr != nil) {
		os.Rename(configPath, tmpConfPath)
	}

	// When: calling removeServer without config.
	noConfigError := toggleServer(serverName, false)
	// Then: correct error is returned.
	assert.ErrorContains(t, noConfigError, "failed to load configuration")

	// Given: existing config without server.
	proxySettings := NewDefaultProxySettings()
	newConfig := GlobalConfig{
		Name:    "test config",
		Version: "1.0.0",
		Proxy:   &proxySettings,
	}
	saveError := SaveConfig(&newConfig)
	assert.NilError(t, saveError)

	// When: calling removeServer.
	noServerError := toggleServer(serverName, false)
	// Unable to find server '%s' in config.
	assert.ErrorContains(t, noServerError, "unable to find server 'my-test-server-2' in config")

	server := MCPServerConfig{
		Name:    serverName,
		Command: "npx",
	}
	newConfig.Gateways = map[string]*GatewayConfig{
		"gateway1": {
			MCPServers: map[string]*MCPServerConfig{
				serverName: &server,
				"test123": {
					Name:    "test123",
					Command: "npx",
				},
			},
		},
	}
	saveError = SaveConfig(&newConfig)
	assert.NilError(t, saveError)

	// When: calling remnoveServer.
	expectedValue := false
	noError := toggleServer(serverName, expectedValue)

	// Then: it successfully removes the server.
	assert.NilError(t, noError)
	config, err := LoadConfig()
	assert.NilError(t, err)
	loadedServer, ok := config.Gateways["gateway1"].MCPServers[serverName]
	assert.Assert(t, ok)
	assert.Assert(t, loadedServer.IsEnabled() == expectedValue)

	// When: calling enableServer.
	ctx := context.Background()
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "name", Value: serverName},
		},
	}
	noError = enableServer(ctx, cmd)

	// Then: server is enabled, and no errors are given.
	assert.NilError(t, noError)
	config, err = LoadConfig()
	assert.NilError(t, err)
	loadedServer, ok = config.Gateways["gateway1"].MCPServers[serverName]
	assert.Assert(t, ok)
	assert.Assert(t, loadedServer.IsEnabled())

	// When: calling disableServer.
	ctx = context.Background()
	cmd = &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "name", Value: serverName},
		},
	}
	noError = disableServer(ctx, cmd)

	// Then: server is enabled, and no errors are given.
	assert.NilError(t, noError)
	config, err = LoadConfig()
	assert.NilError(t, err)
	loadedServer, ok = config.Gateways["gateway1"].MCPServers[serverName]
	assert.Assert(t, ok)
	assert.Assert(t, !loadedServer.IsEnabled())
}

func TestListServers_DisplaysOnlyEnabledServers(t *testing.T) {
	// Given: a config with enabled and disabled servers.
	cleanup := setupTestEnv(t)
	defer cleanup()

	createTestConfig(t)

	config, _ := LoadConfig()
	disabled := false
	gateway := &GatewayConfig{
		MCPServers: map[string]*MCPServerConfig{
			"enabled-server": {
				Name:    "enabled-server",
				Command: "npx",
				Args:    []string{"enabled"},
			},
			"disabled-server": {
				Name:    "disabled-server",
				Enabled: &disabled,
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

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// When: listing only enabled servers.
	err := listServers(ctx, cmd)

	// Restore stdout.
	w.Close()
	os.Stdout = oldStdout

	// Read output.
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Then: should succeed.
	assert.NilError(t, err)

	// And: should display only enabled server.
	assert.Assert(t, strings.Contains(output, "enabled-server"))
	assert.Assert(t, !strings.Contains(output, "disabled-server"))
}

func TestListServers_FailsIfNoConfig(t *testing.T) {
	// Given: no config file.
	cleanup := setupTestEnv(t)
	defer cleanup()

	ctx := context.Background()
	cmd := &cli.Command{}

	// When: listing servers.
	err := listServers(ctx, cmd)

	// Then: should return error.
	assert.ErrorContains(t, err, "failed to load configuration")
}

// ========================================
// removeConfig Tests
// ========================================

func TestRemoveConfig_RemovesConfigWithForceFlag(t *testing.T) {
	// Given: an existing config.
	cleanup := setupTestEnv(t)
	defer cleanup()

	createTestConfig(t)

	configPath, _ := GetConfigPath()
	centianDir := filepath.Dir(configPath)

	// Verify config exists.
	_, err := os.Stat(configPath)
	assert.NilError(t, err)

	ctx := context.Background()
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "force", Value: true},
		},
	}

	// When: removing config with force flag.
	err = removeConfig(ctx, cmd)

	// Then: should succeed.
	assert.NilError(t, err)

	// And: centian directory should be removed.
	_, err = os.Stat(centianDir)
	assert.Assert(t, os.IsNotExist(err))
}

func TestRemoveConfig_SucceedsIfNoConfig(t *testing.T) {
	// Given: no config file.
	cleanup := setupTestEnv(t)
	defer cleanup()

	ctx := context.Background()
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "force", Value: true},
		},
	}

	// When: trying to remove non-existent config.
	err := removeConfig(ctx, cmd)

	// Then: should return error.
	assert.NilError(t, err)
}
