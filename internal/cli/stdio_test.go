package cli

import (
	"testing"

	urfavecli "github.com/urfave/cli/v3"
)

// TestStdioCommandStructure tests the StdioCommand CLI structure.
func TestStdioCommandStructure(t *testing.T) {
	// Given: the StdioCommand.

	// Then: verify command is properly configured.
	if StdioCommand == nil {
		t.Fatal("StdioCommand is nil")
	}

	if StdioCommand.Name != "stdio" {
		t.Errorf("Expected command name 'stdio', got '%s'", StdioCommand.Name)
	}

	if StdioCommand.Usage == "" {
		t.Error("StdioCommand should have usage text")
	}

	if StdioCommand.Description == "" {
		t.Error("StdioCommand should have description")
	}

	if StdioCommand.Action == nil {
		t.Error("StdioCommand should have action function")
	}

	// Then: verify flags exist.
	expectedFlags := map[string]string{
		"cmd":         "string",
		"config-path": "string",
	}

	for _, flag := range StdioCommand.Flags {
		if sf, ok := flag.(*urfavecli.StringFlag); ok {
			if expectedType, exists := expectedFlags[sf.Name]; exists {
				if expectedType != "string" {
					t.Errorf("Flag '%s' has unexpected type", sf.Name)
				}
				delete(expectedFlags, sf.Name)
			}
		}
	}

	// Verify all expected flags were found.
	for flagName := range expectedFlags {
		t.Errorf("Expected flag '%s' not found in StdioCommand", flagName)
	}
}

// TestStdioCommandFlagDefaults tests default values for stdio command flags.
func TestStdioCommandFlagDefaults(t *testing.T) {
	// Given: the StdioCommand flags.

	// Then: verify cmd flag has correct default.
	var cmdFlagFound bool
	var cmdFlagDefault string

	for _, flag := range StdioCommand.Flags {
		if sf, ok := flag.(*urfavecli.StringFlag); ok && sf.Name == "cmd" {
			cmdFlagFound = true
			cmdFlagDefault = sf.Value
			break
		}
	}

	if !cmdFlagFound {
		t.Fatal("cmd flag not found in StdioCommand")
	}

	if cmdFlagDefault != "npx" {
		t.Errorf("Expected cmd flag default 'npx', got '%s'", cmdFlagDefault)
	}
}

// TestStdioCommandUsageExamples verifies the command has helpful examples.
func TestStdioCommandUsageExamples(t *testing.T) {
	// Given: the StdioCommand.

	// Then: verify description contains examples.
	description := StdioCommand.Description

	expectedExamples := []string{
		"centian stdio",
		"@modelcontextprotocol",
		"--cmd npx",
		"--cmd python",
	}

	for _, example := range expectedExamples {
		if !contains(description, example) {
			t.Errorf("Expected description to contain example '%s', but it didn't", example)
		}
	}

	// Then: verify description mentions the '--' separator.
	if !contains(description, "--") {
		t.Error("Expected description to mention '--' separator for arguments")
	}
}

// TestStdioCommandShortOptionHandling verifies short option handling is enabled.
func TestStdioCommandShortOptionHandling(t *testing.T) {
	// Given: the StdioCommand.

	// Then: verify UseShortOptionHandling is enabled.
	if !StdioCommand.UseShortOptionHandling {
		t.Error("StdioCommand should have UseShortOptionHandling enabled")
	}
}

// Helper function to check if a string contains a substring.
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
