package discovery

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/CentianAI/centian-cli/internal/common"
	"github.com/CentianAI/centian-cli/internal/config"
)

// ReplacementConfig contains information for replacing a source config with centian proxy
type ReplacementConfig struct {
	SourcePath      string   // Path to original config file
	SourceType      string   // Type: "claude-desktop", "vscode-mcp", "vscode-settings"
	OriginalServers []string // Names of servers being replaced
	ProxyConfig     string   // Replacement config snippet
}

// DiscoveryUI provides user interface functions for the discovery system
type DiscoveryUI struct {
	reader *bufio.Reader
}

// NewDiscoveryUI creates a new discovery UI interface
func NewDiscoveryUI() *DiscoveryUI {
	return &DiscoveryUI{
		reader: bufio.NewReader(os.Stdin),
	}
}

// ShowDiscoveryResults displays discovered servers and prompts for user consent
func (ui *DiscoveryUI) ShowDiscoveryResults(result *Result) ([]Server, error) {
	if len(result.Servers) == 0 {
		if len(result.Errors) > 0 {
			fmt.Printf("🔍 Searched for existing MCP configurations but found none.\n")
			fmt.Printf("⚠️  Some locations couldn't be scanned:\n")
			for _, err := range result.Errors {
				fmt.Printf("   - %s\n", err)
			}
		} else {
			fmt.Printf("🔍 No existing MCP configurations found.\n")
		}
		fmt.Printf("💡 You'll need to add servers manually using 'centian config server add'\n\n")
		return []Server{}, nil
	}

	// Group the results by source file
	grouped := GroupDiscoveryResults(result)

	common.StreamPrint(10, "🔍 Found MCP configurations in %d file(s):\n\n", len(grouped.Groups))

	// Display grouped servers
	for _, group := range grouped.Groups {
		fmt.Printf("📁 %s\n", group.SourcePath)
		fmt.Printf("   📊 %d servers", group.TotalCount)

		if group.StdioCount > 0 || group.HTTPCount > 0 {
			fmt.Printf(" (stdio: %d, http: %d)", group.StdioCount, group.HTTPCount)
		}

		if group.DuplicatesFound > 0 {
			plural := ""
			if group.DuplicatesFound > 1 {
				plural = "s"
			}
			fmt.Printf(" [🔄 %d duplicate%s merged]", group.DuplicatesFound, plural)
		}

		fmt.Printf("\n\n")
	}

	// Show any errors
	if len(grouped.Errors) > 0 {
		fmt.Printf("⚠️  Some locations couldn't be scanned:\n")
		for _, err := range grouped.Errors {
			fmt.Printf("   - %s\n", err)
		}
		fmt.Printf("\n")
	}

	// Add option to show detailed view
	fmt.Printf("💡 To see individual servers, run: centian discovery --details\n\n")

	// Prompt for consent
	return ui.promptForImport(result.Servers)
}

// promptForImport asks the user which servers to import and offers proxy replacement
func (ui *DiscoveryUI) promptForImport(servers []Server) ([]Server, error) {
	fmt.Printf("Import these servers into centian configuration?\n")
	fmt.Printf("Options:\n")
	fmt.Printf("  [a]ll      - Import all servers (default)\n")
	fmt.Printf("  [s]elect   - Choose specific servers to import\n")
	fmt.Printf("  [r]eplace  - Replace all discovered configs with centian proxy (creates backup)\n")
	fmt.Printf("  [sr]       - Select configs to replace with centian proxy\n")
	fmt.Printf("  [n]one     - Skip import\n")
	fmt.Printf("Choice [a/s/r/sr/n]: ")

	response, err := ui.reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))

	switch response {
	case "", "a", "all":
		return servers, nil
	case "s", "select":
		return ui.selectServers(servers)
	case "r", "replace":
		return ui.promptForReplacement(servers)
	case "sr":
		return ui.selectAndReplace(servers)
	case "n", "none":
		return []Server{}, nil
	default:
		fmt.Printf("Invalid choice. Skipping import.\n")
		return []Server{}, nil
	}
}

// selectServers allows user to pick specific servers to import
func (ui *DiscoveryUI) selectServers(servers []Server) ([]Server, error) {
	fmt.Printf("\nSelect servers to import (comma-separated numbers, e.g., 1,3,4):\n")

	for i := range servers {
		fmt.Printf("  %d. %s (%s)\n", i+1, servers[i].Name, servers[i].SourcePath)
	}

	fmt.Printf("\nServers to import: ")
	response, err := ui.reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.TrimSpace(response)
	if response == "" {
		return []Server{}, nil
	}

	// Parse selection
	selections := strings.Split(response, ",")
	var selectedServers []Server

	for _, sel := range selections {
		sel = strings.TrimSpace(sel)
		var index int
		if _, err := fmt.Sscanf(sel, "%d", &index); err != nil {
			fmt.Printf("⚠️  Invalid selection: %s\n", sel)
			continue
		}

		if index < 1 || index > len(servers) {
			fmt.Printf("⚠️  Selection out of range: %d\n", index)
			continue
		}

		selectedServers = append(selectedServers, servers[index-1])
	}

	if len(selectedServers) > 0 {
		fmt.Printf("\n✅ Selected %d server(s) for import\n", len(selectedServers))
	} else {
		fmt.Printf("\n⚠️  No valid servers selected\n")
	}

	return selectedServers, nil
}

// selectAndReplace allows user to pick specific configs to replace with centian proxy
func (ui *DiscoveryUI) selectAndReplace(servers []Server) ([]Server, error) {
	// Group servers by source file for better display
	configGroups := make(map[string][]Server)
	for i := range servers {
		configGroups[servers[i].SourcePath] = append(configGroups[servers[i].SourcePath], servers[i])
	}

	fmt.Printf("🔄 Select Configuration Files to Replace\n")
	fmt.Printf("========================================\n")
	fmt.Printf("Choose which config files to replace with centian proxy:\n")

	// Display grouped configs with indices
	var configOptions []string
	var configServers [][]Server
	index := 1

	for sourcePath, groupServers := range configGroups {
		configOptions = append(configOptions, sourcePath)
		configServers = append(configServers, groupServers)

		fmt.Printf("  %d. %s\n", index, sourcePath)
		fmt.Printf("     Contains %d server(s): ", len(groupServers))
		var serverNames []string
		for i := range groupServers {
			serverNames = append(serverNames, groupServers[i].Name)
		}
		fmt.Printf("%s\n", strings.Join(serverNames, ", "))
		fmt.Printf("\n")
		index++
	}

	fmt.Printf("Select config files to replace (comma-separated numbers, e.g., 1,3):\n")
	fmt.Printf("Config files to replace: ")

	response, err := ui.reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.TrimSpace(response)
	if response == "" {
		return []Server{}, nil
	}

	// Parse selection
	selections := strings.Split(response, ",")
	var allSelectedServers []Server

	for _, sel := range selections {
		sel = strings.TrimSpace(sel)
		var selectedIndex int
		if _, err := fmt.Sscanf(sel, "%d", &selectedIndex); err != nil {
			fmt.Printf("⚠️  Invalid selection: %s\n", sel)
			continue
		}

		if selectedIndex < 1 || selectedIndex > len(configOptions) {
			fmt.Printf("⚠️  Selection out of range: %d\n", selectedIndex)
			continue
		}

		// Mark all servers from this config for replacement
		selectedConfigServers := configServers[selectedIndex-1]
		for i := range selectedConfigServers {
			selectedConfigServers[i].ReplacementMode = true
		}

		allSelectedServers = append(allSelectedServers, selectedConfigServers...)
		fmt.Printf("✅ Selected for replacement: %s\n", configOptions[selectedIndex-1])
	}

	if len(allSelectedServers) > 0 {
		fmt.Printf("🔄 Configuration Replacement Preview\n")
		fmt.Printf("====================================\n")
		fmt.Printf("This will:\n")
		fmt.Printf("  1. Import all selected servers into centian\n")
		fmt.Printf("  2. Replace selected config files with centian proxy\n")
		fmt.Printf("  3. Create backup files (.centian-backup)\n")

		fmt.Printf("Proceed with replacement? (y/N): ")
		confirmResponse, err := ui.reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read input: %w", err)
		}

		confirmResponse = strings.TrimSpace(strings.ToLower(confirmResponse))
		if confirmResponse != "y" && confirmResponse != "yes" {
			fmt.Printf("Replacement cancelled.\n")
			// Remove replacement mode and return servers for regular import
			for i := range allSelectedServers {
				allSelectedServers[i].ReplacementMode = false
			}
		}
	} else {
		fmt.Printf("⚠️  No valid config files selected\n")
	}

	return allSelectedServers, nil
}

// promptForReplacement asks user about replacing discovered configs with centian proxy
func (ui *DiscoveryUI) promptForReplacement(servers []Server) ([]Server, error) {
	common.StreamPrint(8, "🔄 Configuration Replacement\n")
	common.StreamPrint(10, "============================\n")
	common.StreamPrint(8, "💡 This centralizes MCP management through centian.\n")

	common.StreamPrint(8, "Performed steps:\n")
	common.StreamPrint(7, "  1. Import all discovered servers into centian\n")
	common.StreamPrint(8, "  2. Automatically replace MCP configs with Centian proxy (and create a backup file for the old config just in case)\n")

	common.StreamPrint(10, "Proceed with replacement config generation? (y/N): ")
	response, err := ui.reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Printf("Replacement cancelled.\n")
		return []Server{}, nil
	}

	// Mark servers for replacement processing
	for i := range servers {
		servers[i].ReplacementMode = true
	}

	return servers, nil
}

// ImportServers converts discovered servers to MCPServer configs and adds them to the global config
func ImportServers(servers []Server, globalConfig *config.GlobalConfig) int {
	common.LogInfo("Starting import of %d discovered servers", len(servers))

	imported := 0
	var replacementConfigs []ReplacementConfig

	for i := range servers {
		discovered := servers[i]
		common.LogDebug("Processing server: %s (from %s)", discovered.Name, discovered.SourcePath)

		// Skip servers that have neither command nor URL
		if discovered.Command == "" && discovered.URL == "" {
			common.LogWarn("Skipping server '%s': no command or URL specified", discovered.Name)
			fmt.Printf("⚠️  Skipping '%s': no command or URL specified\n", discovered.Name)
			continue
		}

		// Check if server already exists
		if _, exists := globalConfig.Servers[discovered.Name]; exists {
			common.LogWarn("Server '%s' already exists in config, skipping", discovered.Name)
			fmt.Printf("⚠️  Server '%s' already exists, skipping\n", discovered.Name)
			continue
		}

		// Convert to MCPServer
		mcpServer := &config.MCPServer{
			Name:        discovered.Name,
			Command:     discovered.Command,
			Args:        discovered.Args,
			Env:         discovered.Env,
			URL:         discovered.URL,
			Transport:   discovered.Transport,
			Enabled:     true, // Auto-discovered servers are enabled by default
			Description: discovered.Description,
			Source:      discovered.SourcePath,
		}

		globalConfig.AddServer(discovered.Name, mcpServer)
		imported++
		common.LogInfo("Imported server: %s (transport: %s, source: %s)", discovered.Name, discovered.Transport, discovered.SourcePath)

		if discovered.ReplacementMode {
			// Track replacement config
			replacementConfigs = append(replacementConfigs, generateReplacementConfig(discovered))
			common.LogDebug("Server '%s' marked for config replacement", discovered.Name)
		}

		fmt.Printf("✅ Imported: %s (from %s)\n", discovered.Name, discovered.SourcePath)
	}

	// Apply replacement configs if any were requested
	if len(replacementConfigs) > 0 {
		common.LogInfo("Applying %d replacement configs", len(replacementConfigs))
		applyReplacementConfigs(replacementConfigs)
	}

	common.LogInfo("Import completed: %d servers imported successfully", imported)
	return imported
}

// ShowImportSummary displays the results of the import process
func ShowImportSummary(imported int, total int) {
	if imported == 0 {
		fmt.Printf("\n📋 No servers were imported.\n")
		fmt.Printf("💡 You can add servers manually using:\n")
		fmt.Printf("   centian config server add --name \"my-server\" --command \"npx\" --args \"-y,@upstash/context7-mcp\"\n\n")
		return
	}

	fmt.Printf("\n🎉 Successfully imported %d server(s)!\n", imported)

	fmt.Printf("\n📋 Next steps:\n")
	fmt.Printf("  1. Review imported servers:\n")
	fmt.Printf("     centian config server list\n\n")
	fmt.Printf("  2. Start the proxy:\n")
	fmt.Printf("     centian start\n\n")
	fmt.Printf("  3. Manage servers:\n")
	fmt.Printf("     centian config server --help\n")
}

// generateReplacementConfig creates replacement configuration for a discovered server
func generateReplacementConfig(server Server) ReplacementConfig {
	var sourceType string
	var proxyConfig string

	// Determine source type from path
	if strings.Contains(server.SourcePath, "claude_desktop_config.json") {
		sourceType = "claude-desktop"
		proxyConfig = generateClaudeDesktopReplacement()
	} else if strings.Contains(server.SourcePath, ".vscode/mcp.json") {
		sourceType = "vscode-mcp"
		proxyConfig = generateVSCodeMCPReplacement()
	} else if strings.Contains(server.SourcePath, "settings.json") {
		sourceType = "vscode-settings"
		proxyConfig = generateVSCodeSettingsReplacement()
	} else if strings.Contains(server.SourcePath, ".mcp.json") {
		// Generic .mcp.json files - use mcpServers structure like Claude Desktop
		sourceType = "generic-mcp"
		proxyConfig = generateGenericMCPReplacement()
	} else if sourceType == "" {
		// Default fallback for unknown file types
		sourceType = "generic-mcp"
		proxyConfig = generateGenericMCPReplacement()
	} else {
		// Default fallback for unknown file types
		sourceType = "generic-mcp"
		proxyConfig = generateGenericMCPReplacement()
	}

	return ReplacementConfig{
		SourcePath:      server.SourcePath,
		SourceType:      sourceType,
		OriginalServers: []string{server.Name},
		ProxyConfig:     proxyConfig,
	}
}

// generateClaudeDesktopReplacement creates Claude Desktop config replacement
func generateClaudeDesktopReplacement() string {
	return `{
  "mcpServers": {
    "centian": {
      "command": "centian",
      "args": ["start"]
    }
  }
}`
}

// generateVSCodeMCPReplacement creates VS Code mcp.json replacement
func generateVSCodeMCPReplacement() string {
	return `{
  "servers": {
    "centian": {
      "command": "centian",
      "args": ["start"]
    }
  }
}`
}

// generateVSCodeSettingsReplacement creates VS Code settings.json replacement
func generateVSCodeSettingsReplacement() string {
	return `{
  "servers": {
    "centian": {
      "command": "centian",
      "args": ["start"]
    }
  }
}`
}

// generateGenericMCPReplacement creates generic .mcp.json replacement
func generateGenericMCPReplacement() string {
	return `{
  "mcpServers": {
    "centian": {
      "command": "centian",
      "args": ["start"]
    }
  }
}`
}

// showReplacementConfigs displays the replacement configurations to the user
func applyReplacementConfigs(configs []ReplacementConfig) {
	common.StreamPrint(10, "🔄 Updating Configuration Files\n")
	common.StreamPrint(15, "===============================\n")

	// Group configs by source file
	configGroups := make(map[string][]ReplacementConfig)
	for _, config := range configs {
		configGroups[config.SourcePath] = append(configGroups[config.SourcePath], config)
	}

	successCount := 0
	errorCount := 0

	for sourcePath, groupConfigs := range configGroups {
		fmt.Printf("📁 Updating %s\n", sourcePath)

		// Get all server names being replaced
		var serverNames []string
		for _, config := range groupConfigs {
			serverNames = append(serverNames, config.OriginalServers...)
		}

		// Apply the replacement
		err := updateConfigFile(sourcePath, groupConfigs[0].SourceType)
		if err != nil {
			fmt.Printf("   ❌ Failed: %v\n", err)
			errorCount++
		} else {
			fmt.Printf("   ✅ Replaced %d server(s): %s\n", len(serverNames), strings.Join(serverNames, ", "))
			successCount++
		}
	}

	fmt.Printf("📊 Summary:\n")
	fmt.Printf("   ✅ Updated: %d file(s)\n", successCount)
	if errorCount > 0 {
		fmt.Printf("   ❌ Failed: %d file(s)\n", errorCount)
	}

	if successCount > 0 {
		fmt.Printf("💡 Next steps:\n")
		fmt.Printf("   1. Restart Claude Desktop / VS Code\n")
		fmt.Printf("   2. Run 'centian start' to start the proxy\n")
		fmt.Printf("   3. Test MCP functionality in your applications\n")
	}

	for sourcePath, groupConfigs := range configGroups {
		fmt.Printf("📁 %s\n", sourcePath)
		fmt.Printf("   Replace the entire file content with:\n")

		// Use the first config's proxy config (they should be the same for same source type)
		fmt.Printf("```json%s```\n", groupConfigs[0].ProxyConfig)

		fmt.Printf("   This replaces %d server(s): ", len(groupConfigs))
		var serverNames []string
		for _, config := range groupConfigs {
			serverNames = append(serverNames, config.OriginalServers...)
		}
		fmt.Printf("%s\n", strings.Join(serverNames, ", "))
	}

	fmt.Printf("⚠️  Important:\n")
	fmt.Printf("  - Make sure centian is in your PATH or use full path to binary\n")
	fmt.Printf("  - Restart Claude Desktop / VS Code after making changes\n")
	fmt.Printf("  - Run 'centian start' to test the proxy before restarting applications\n")
}

// updateConfigFile modifies the config file to replace MCP servers with centian proxy
func updateConfigFile(filePath, sourceType string) error {
	common.LogInfo("Updating config file: %s (type: %s)", filePath, sourceType)

	// Read current file
	data, err := os.ReadFile(filePath)
	if err != nil {
		common.LogError("Failed to read config file %s: %v", filePath, err)
		return fmt.Errorf("failed to read file: %w", err)
	}
	common.LogDebug("Read config file %s (%d bytes)", filePath, len(data))

	// Parse JSON
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		common.LogError("Failed to parse JSON in %s: %v", filePath, err)
		return fmt.Errorf("failed to parse JSON: %w", err)
	}
	common.LogDebug("Parsed JSON config successfully")

	// Prepare centian server config with current config file path
	centianConfig := map[string]interface{}{
		"command": "centian",
		"args":    []string{"start", "--path", filePath},
	}

	// Replace MCP servers section based on source type
	var serversReplaced int
	switch sourceType {
	case "claude-desktop", "generic-mcp":
		if existingServers, ok := config["mcpServers"].(map[string]interface{}); ok {
			serversReplaced = len(existingServers)
		}
		config["mcpServers"] = map[string]interface{}{
			"centian": centianConfig,
		}
		common.LogDebug("Replaced mcpServers section (%d servers -> 1 centian proxy)", serversReplaced)
	case "vscode-mcp":
		if config["servers"] == nil {
			config["servers"] = make(map[string]interface{})
		}
		servers := config["servers"].(map[string]interface{})
		serversReplaced = len(servers)
		// Clear existing servers and add centian
		for key := range servers {
			delete(servers, key)
		}
		servers["centian"] = centianConfig
		common.LogDebug("Replaced servers section (%d servers -> 1 centian proxy)", serversReplaced)
	case "vscode-settings":
		if config["mcp.servers"] == nil {
			config["mcp.servers"] = make(map[string]interface{})
		}
		mcpServers := config["mcp.servers"].(map[string]interface{})
		serversReplaced = len(mcpServers)
		// Clear existing servers and add centian
		for key := range mcpServers {
			delete(mcpServers, key)
		}
		mcpServers["centian"] = centianConfig
		common.LogDebug("Replaced mcp.servers section (%d servers -> 1 centian proxy)", serversReplaced)
	default:
		common.LogError("Unsupported source type: %s", sourceType)
		return fmt.Errorf("unsupported source type: %s", sourceType)
	}

	// Write back to file with proper formatting
	newData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		common.LogError("Failed to marshal JSON for %s: %v", filePath, err)
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Create backup
	backupPath := filePath + ".centian-backup"
	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		common.LogError("Failed to create backup %s: %v", backupPath, err)
		return fmt.Errorf("failed to create backup: %w", err)
	}
	common.LogInfo("Created backup: %s", backupPath)

	// Write new config
	if err := os.WriteFile(filePath, newData, 0o644); err != nil {
		common.LogError("Failed to write updated config %s: %v", filePath, err)
		return fmt.Errorf("failed to write updated config: %w", err)
	}

	common.LogInfo("Successfully updated config file %s (replaced %d servers with centian proxy)", filePath, serversReplaced)
	return nil
}
