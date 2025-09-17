package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// Test the current updateConfigFile function logic
func testCurrentLogic(filePath, sourceType string) error {
	fmt.Printf("Testing current logic on %s (type: %s)\n", filePath, sourceType)

	// Read current file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	fmt.Printf("Original content:\n%s\n\n", string(data))

	// Parse JSON
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Store original MCP servers for comparison
	var originalMCPServers map[string]interface{}
	switch sourceType {
	case "claude-desktop":
		if mcpServers, ok := config["mcpServers"].(map[string]interface{}); ok {
			originalMCPServers = mcpServers
		}
	case "vscode-mcp":
		if servers, ok := config["servers"].(map[string]interface{}); ok {
			originalMCPServers = servers
		}
	case "vscode-settings":
		if mcpSection, ok := config["mcp.servers"].(map[string]interface{}); ok {
			originalMCPServers = mcpSection
		}
	}

	fmt.Printf("Original MCP servers found: %v\n", getServerNames(originalMCPServers))

	// Prepare centian server config with current config file path
	centianConfig := map[string]interface{}{
		"command": "centian",
		"args":    []string{"start", "--path", filePath},
	}

	// Apply current logic
	switch sourceType {
	case "claude-desktop":
		config["mcpServers"] = map[string]interface{}{
			"centian": centianConfig,
		}
	case "vscode-mcp":
		if config["servers"] == nil {
			config["servers"] = make(map[string]interface{})
		}
		servers := config["servers"].(map[string]interface{})
		// Clear existing servers and add centian
		for key := range servers {
			delete(servers, key)
		}
		servers["centian"] = centianConfig
	case "vscode-settings":
		if config["mcp.servers"] == nil {
			config["mcp.servers"] = make(map[string]interface{})
		}
		mcpServers := config["mcp.servers"].(map[string]interface{})
		// Clear existing servers and add centian
		for key := range mcpServers {
			delete(mcpServers, key)
		}
		mcpServers["centian"] = centianConfig
	}

	// Write result
	newData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	outputPath := filePath + ".result"
	if err := os.WriteFile(outputPath, newData, 0644); err != nil {
		return fmt.Errorf("failed to write result: %w", err)
	}

	fmt.Printf("Result written to: %s\n", outputPath)
	fmt.Printf("Result content:\n%s\n\n", string(newData))

	return nil
}

func getServerNames(servers map[string]interface{}) []string {
	var names []string
	for name := range servers {
		names = append(names, name)
	}
	return names
}

func TestReplacement(t *testing.T) {
	// Test with Claude Desktop config
	if err := testCurrentLogic("test_configs/claude_desktop_config.json", "claude-desktop"); err != nil {
		fmt.Printf("Error testing Claude Desktop config: %v\n", err)
	}

	fmt.Println("================================================================================")

	// Test with VS Code mcp.json
	if err := testCurrentLogic("test_configs/vscode_mcp.json", "vscode-mcp"); err != nil {
		fmt.Printf("Error testing VS Code mcp.json: %v\n", err)
	}

	fmt.Println("================================================================================")

	// Test with VS Code settings
	if err := testCurrentLogic("test_configs/vscode_settings.json", "vscode-settings"); err != nil {
		fmt.Printf("Error testing VS Code settings: %v\n", err)
	}
}
