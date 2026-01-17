package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/CentianAI/centian-cli/internal/config"
	urfavecli "github.com/urfave/cli/v3"
)

// TestInitCommandWorkflow tests the complete init command workflow.
func TestInitCommandWorkflow(t *testing.T) {
	// Setup - create temporary directory for testing.
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	testHome := filepath.Join(tempDir, "cli_test")
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// Test 1: First time init (no existing config).
	ctx := context.Background()

	// Create a test command with the no-discovery flag.
	cmd := &urfavecli.Command{
		Name: "init",
		Flags: []urfavecli.Flag{
			&urfavecli.BoolFlag{Name: "force"},
			&urfavecli.BoolFlag{Name: "no-discovery"},
		},
	}

	// Set the no-discovery flag to avoid complex discovery testing.
	cmd.Set("no-discovery", "true")

	// Run the init command.
	err := initCentian(ctx, cmd)
	if err != nil {
		t.Fatalf("First init failed: %v", err)
	}

	// Verify config was created.
	configPath, err := config.GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath failed: %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created at %s", configPath)
	}

	// Verify config content.
	loadedConfig, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig after init failed: %v", err)
	}

	if loadedConfig.Version != "1.0.0" {
		t.Errorf("Config version incorrect: expected 1.0.0, got %s", loadedConfig.Version)
	}

	t.Log("Init command workflow test completed successfully")
}

// TestRunAutoDiscovery tests the auto-discovery function logic.
func TestRunAutoDiscovery(t *testing.T) {
	// Setup.
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	testHome := filepath.Join(tempDir, "autodiscovery_test")
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// Create a test config.
	cfg := config.DefaultConfig()

	// Test runAutoDiscovery function.
	// Note: This will run actual discovery which may find real MCP configs.
	imported := runAutoDiscovery(cfg)

	// Verify the function completes without error.
	if imported < 0 {
		t.Error("runAutoDiscovery returned negative import count")
	}

	// The number of imported servers depends on what's on the system.
	// but the function should complete without crashing.
	t.Logf("Auto-discovery imported %d servers", imported)

	// Verify config structure is preserved.
	if cfg.Version != "1.0.0" {
		t.Error("Config version changed during auto-discovery")
	}

	if cfg.Gateways == nil {
		t.Error("Config gateway map should not be nil after auto-discovery")
	}

	t.Log("Auto-discovery function test completed successfully")
}

// TestShellDetection tests shell detection functionality.
func TestShellDetection(t *testing.T) {
	// Save original SHELL env var.
	originalShell := os.Getenv("SHELL")
	defer os.Setenv("SHELL", originalShell)

	// Test common shells.
	shells := []string{
		"/bin/bash",
		"/bin/zsh",
		"/usr/bin/fish",
		"/usr/local/bin/bash",
	}

	for _, shell := range shells {
		os.Setenv("SHELL", shell)

		shellInfo, err := DetectShell()
		if err != nil {
			t.Logf("Shell detection failed for %s: %v (this may be expected)", shell, err)
			continue
		}

		if shellInfo.Name == "" {
			t.Errorf("Shell name empty for %s", shell)
		}

		if shellInfo.RCFile == "" && shellInfo.Name != "fish" {
			t.Errorf("RC file empty for non-fish shell %s", shell)
		}

		t.Logf("Detected shell: %s, RC file: %s", shellInfo.Name, shellInfo.RCFile)
	}

	// Test missing SHELL env var.
	os.Setenv("SHELL", "")
	_, err := DetectShell()
	if err == nil {
		t.Error("Expected error when SHELL env var is empty")
	}

	// Test unsupported shell.
	os.Setenv("SHELL", "/bin/unsupported")
	_, err = DetectShell()
	if err == nil {
		t.Error("Expected error for unsupported shell")
	}
}

// TestCompletionFileOperations tests completion file operations.
func TestCompletionFileOperations(t *testing.T) {
	// Setup.
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	testHome := filepath.Join(tempDir, "completion_test")
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// Create test home directory.
	err := os.MkdirAll(testHome, 0o755)
	if err != nil {
		t.Fatalf("Failed to create test home directory: %v", err)
	}

	// Create test RC file with proper escaped string.
	testRCFile := filepath.Join(testHome, ".testrc")
	testContent := "# Test RC file\nexport TEST_VAR=1\n"
	err = os.WriteFile(testRCFile, []byte(testContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test RC file: %v", err)
	}

	// Test completion existence check (should be false).
	completionLine := "source <(centian completion bash)"
	exists, err := completionExists(testRCFile, completionLine)
	if err != nil {
		t.Fatalf("completionExists failed: %v", err)
	}

	if exists {
		t.Error("Completion should not exist in fresh RC file")
	}

	// Add completion line.
	file, err := os.OpenFile(testRCFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("Failed to open RC file for append: %v", err)
	}

	completionBlock := fmt.Sprintf("\n# Centian CLI completion\n%s\n", completionLine)
	_, err = file.WriteString(completionBlock)
	file.Close()
	if err != nil {
		t.Fatalf("Failed to write completion block: %v", err)
	}

	// Test completion existence check (should be true now).
	exists, err = completionExists(testRCFile, completionLine)
	if err != nil {
		t.Fatalf("completionExists check failed: %v", err)
	}

	if !exists {
		t.Error("Completion should exist after adding")
	}

	// Test with non-existent file.
	nonExistentFile := filepath.Join(testHome, ".nonexistent")
	exists, err = completionExists(nonExistentFile, completionLine)
	if err != nil {
		t.Fatalf("completionExists failed for non-existent file: %v", err)
	}

	if exists {
		t.Error("Completion should not exist in non-existent file")
	}

	t.Log("Completion file operations test completed successfully")
}

// TestCLICommandStructure tests the CLI command structure and flags.
func TestCLICommandStructure(t *testing.T) {
	// Test InitCommand structure.
	if InitCommand == nil {
		t.Fatal("InitCommand is nil")
	}

	if InitCommand.Name != "init" {
		t.Errorf("InitCommand name incorrect: expected 'init', got '%s'", InitCommand.Name)
	}

	if InitCommand.Usage == "" {
		t.Error("InitCommand should have usage text")
	}

	if InitCommand.Description == "" {
		t.Error("InitCommand should have description")
	}

	if InitCommand.Action == nil {
		t.Error("InitCommand should have action function")
	}

	// Verify flags exist.
	expectedFlags := []string{"force", "no-discovery"}
	flagNames := make(map[string]bool)

	for _, flag := range InitCommand.Flags {
		if f, ok := flag.(*urfavecli.BoolFlag); ok {
			flagNames[f.Name] = true
		}
	}

	for _, expected := range expectedFlags {
		if !flagNames[expected] {
			t.Errorf("Expected flag '%s' not found in InitCommand", expected)
		}
	}

	t.Log("CLI command structure test completed successfully")
}
