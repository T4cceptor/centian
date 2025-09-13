package internal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReplacementConfig contains information for replacing a source config with centian proxy
type ReplacementConfig struct {
	SourcePath    string   // Path to original config file
	SourceType    string   // Type: "claude-desktop", "vscode-mcp", "vscode-settings"
	OriginalServers []string // Names of servers being replaced
	ProxyConfig   string   // Replacement config snippet
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

	fmt.Printf("🔍 Found %d MCP server(s) in existing configurations:\n\n", len(result.Servers))

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
		fmt.Println()
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
	fmt.Printf("Import these servers into centian configuration?
")
	fmt.Printf("Options:
")
	fmt.Printf("  [a]ll     - Import all servers (default)
")
	fmt.Printf("  [s]elect  - Choose specific servers to import
")
	fmt.Printf("  [r]eplace - Replace discovered configs with centian proxy
")
	fmt.Printf("  [n]one    - Skip import
")
	fmt.Printf("
Choice [a/s/r/n]: ")

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
		fmt.Printf("Invalid choice. Skipping import.
")
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
	fmt.Printf("
🔄 Configuration Replacement
")
	fmt.Printf("============================
")
	fmt.Printf("This will:
")
	fmt.Printf("  1. Import all discovered servers into centian
")
	fmt.Printf("  2. Generate replacement configs that use centian proxy
")
	fmt.Printf("  3. Show you the replacement configs (you'll apply them manually)

")
	
	fmt.Printf("⚠️  You will need to manually update your original config files.
")
	fmt.Printf("💡 This centralizes MCP management through centian.

")
	
	fmt.Printf("Proceed with replacement config generation? (y/N): ")
	response, err := ui.reader.ReadString('
')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Printf("Replacement cancelled.
")
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
		
		fmt.Printf("✅ Imported: %s (from %s)
", discovered.Name, discovered.SourcePath)
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
	fmt.Printf("     centian config server --help\n\n")
}
