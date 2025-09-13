package internal

import (
	"bufio"
	"fmt"
	"os"
	"strings"
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
func (ui *DiscoveryUI) ShowDiscoveryResults(result *DiscoveryResult) ([]DiscoveredServer, error) {
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
		return []DiscoveredServer{}, nil
	}

	StreamPrint(10, "🔍 Found %d MCP server(s) in existing configurations:\n\n", len(result.Servers))

	// TODO: if more than X servers are found we should add option to skip print or show all

	// Display discovered servers
	for i, server := range result.Servers {
		fmt.Printf("  %d. %s\n", i+1, server.Name)

		if server.Command != "" {
			fmt.Printf("     Command: %s\n", server.Command)
		}
		if server.URL != "" {
			fmt.Printf("     URL: %s\n", server.URL)
		}
		if len(server.Args) > 0 {
			fmt.Printf("     Args: %v\n", server.Args)
		}
		if server.SourcePath != "" {
			fmt.Printf("     Source: %s\n", server.SourcePath)
		}
		if server.Description != "" {
			fmt.Printf("     Description: %s\n", server.Description)
		}
		if len(server.Env) > 0 {
			fmt.Printf("     Environment: %d variables\n", len(server.Env))
		}
		StreamPrint(1, "  \n")
	}

	// Show any errors
	if len(result.Errors) > 0 {
		fmt.Printf("⚠️  Some locations couldn't be scanned:\n")
		for _, err := range result.Errors {
			fmt.Printf("   - %s\n", err)
		}
		fmt.Printf("\n")
	}

	// Prompt for consent
	return ui.promptForImport(result.Servers)
}

// promptForImport asks the user which servers to import and offers proxy replacement
func (ui *DiscoveryUI) promptForImport(servers []DiscoveredServer) ([]DiscoveredServer, error) {
	fmt.Printf("Import these servers into centian configuration?\n")
	fmt.Printf("Options:\n")
	fmt.Printf("  [a]ll     - Import all servers (default)\n")
	fmt.Printf("  [s]elect  - Choose specific servers to import\n")
	fmt.Printf("  [r]eplace - Replace discovered configs with centian proxy\n")
	fmt.Printf("  [n]one    - Skip import\n")
	fmt.Printf("Choice [a/s/r/n]: ")

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
	case "n", "none":
		return []DiscoveredServer{}, nil
	default:
		fmt.Printf("Invalid choice. Skipping import.\n")
		return []DiscoveredServer{}, nil
	}
}

// selectServers allows user to pick specific servers to import
func (ui *DiscoveryUI) selectServers(servers []DiscoveredServer) ([]DiscoveredServer, error) {
	fmt.Printf("\nSelect servers to import (comma-separated numbers, e.g., 1,3,4):\n")

	for i, server := range servers {
		fmt.Printf("  %d. %s (%s)\n", i+1, server.Name, server.SourcePath)
	}

	fmt.Printf("\nServers to import: ")
	response, err := ui.reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.TrimSpace(response)
	if response == "" {
		return []DiscoveredServer{}, nil
	}

	// Parse selection
	selections := strings.Split(response, ",")
	var selectedServers []DiscoveredServer

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

// promptForReplacement asks user about replacing discovered configs with centian proxy
func (ui *DiscoveryUI) promptForReplacement(servers []DiscoveredServer) ([]DiscoveredServer, error) {
	fmt.Printf("🔄 Configuration Replacement\n")
	fmt.Printf("============================\n")
	fmt.Printf("This will:\n")
	fmt.Printf("  1. Import all discovered servers into centian\n")
	fmt.Printf("  2. Generate replacement configs that use centian proxy\n")
	fmt.Printf("  3. Show you the replacement configs (you'll apply them manually)\n")

	fmt.Printf("⚠️  You will need to manually update your original config files.\n")
	fmt.Printf("💡 This centralizes MCP management through centian.\n")

	fmt.Printf("Proceed with replacement config generation? (y/N): ")
	response, err := ui.reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Printf("Replacement cancelled.\n")
		return []DiscoveredServer{}, nil
	}

	// Mark servers for replacement processing
	for i := range servers {
		servers[i].ReplacementMode = true
	}

	return servers, nil
}

// ImportServers converts discovered servers to MCPServer configs and adds them to the global config
func ImportServers(servers []DiscoveredServer, config *GlobalConfig) int {
	imported := 0
	var replacementConfigs []ReplacementConfig

	for _, discovered := range servers {
		// Skip servers that have neither command nor URL
		if discovered.Command == "" && discovered.URL == "" {
			fmt.Printf("⚠️  Skipping '%s': no command or URL specified\n", discovered.Name)
			continue
		}

		// Check if server already exists
		if _, exists := config.Servers[discovered.Name]; exists {
			fmt.Printf("⚠️  Server '%s' already exists, skipping\n", discovered.Name)
			continue
		}

		// Convert to MCPServer
		mcpServer := &MCPServer{
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

		config.AddServer(discovered.Name, mcpServer)
		imported++

		if discovered.ReplacementMode {
			// Track replacement config
			replacementConfigs = append(replacementConfigs, generateReplacementConfig(discovered))
		}

		fmt.Printf("✅ Imported: %s (from %s)\n", discovered.Name, discovered.SourcePath)
	}

	// Show replacement configs if any were requested
	if len(replacementConfigs) > 0 {
		showReplacementConfigs(replacementConfigs)
	}

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
func generateReplacementConfig(server DiscoveredServer) ReplacementConfig {
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

// showReplacementConfigs displays the replacement configurations to the user
func showReplacementConfigs(configs []ReplacementConfig) {
	fmt.Printf("🔄 Replacement Configuration Instructions\n")
	fmt.Printf("========================================\n")

	fmt.Printf("Your servers have been imported into centian. To complete the setup,\n")
	fmt.Printf("replace your original configurations with the following:\n")

	// Group configs by source file
	configGroups := make(map[string][]ReplacementConfig)
	for _, config := range configs {
		configGroups[config.SourcePath] = append(configGroups[config.SourcePath], config)
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
