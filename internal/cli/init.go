package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
	"github.com/CentianAI/centian-cli/internal/config"
	"github.com/CentianAI/centian-cli/internal/discovery"
	"github.com/urfave/cli/v3"
)

// InitCommand initializes a new centian setup with default configuration
var InitCommand = &cli.Command{
	Name:        "init",
	Usage:       "Initialize centian with default configuration",
	Description: "Creates ~/.centian/config.jsonc with default settings and guides initial setup",
	Action:      initCentian,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "Overwrite existing configuration if it exists",
		},
		&cli.BoolFlag{
			Name:    "no-discovery",
			Aliases: []string{"n"},
			Usage:   "Skip auto-discovery of existing MCP configurations",
		},
	},
}

// initCentian initializes the centian configuration and provides setup guidance.
// This is the main entry point for new users to get started with centian.
func initCentian(ctx context.Context, cmd *cli.Command) error {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("failed to determine config path: %w", err)
	}

	// Check if config already exists
	if !cmd.Bool("force") {
		if _, err := config.LoadConfig(); err == nil {
			fmt.Printf("✅ Configuration already exists at %s\n", configPath)
			fmt.Printf("💡 Use 'centian config show' to view current configuration\n")
			fmt.Printf("💡 Use 'centian init --force' to overwrite existing configuration\n")
			return nil
		}
	}

	// Create default config
	cfg := config.DefaultConfig()

	// Run auto-discovery unless disabled
	var imported int
	if !cmd.Bool("no-discovery") {
		imported = runAutoDiscovery(cfg)
	}

	// Save config (either default or with discovered servers)
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	fmt.Printf("🎉 Centian initialized successfully!\n\n")
	fmt.Printf("📁 Configuration created at: %s\n\n", configPath)

	fmt.Printf("📋 Next steps:\n")
	if imported == 0 {
		fmt.Printf("  * Add an MCP server:\n")
		fmt.Printf("     centian config server add --name \"my-server\" --command \"npx\" --args \"-y,@upstash/context7-mcp,--api-key,YOUR_KEY\"\n\n")
	}
	fmt.Printf("  * List your servers:\n")
	fmt.Printf("     centian config server list\n\n")
	fmt.Printf("  * Start the proxy:\n")
	fmt.Printf("     centian start\n\n")

	fmt.Printf("💡 Use 'centian config --help' for more configuration options\n")

	// Offer to set up shell completion
	if err := SetupShellCompletion(); err != nil {
		fmt.Printf("⚠️  Shell completion setup failed: %v\n", err)
		fmt.Printf("   You can set it up manually later using: centian completion <shell>\n")
	}

	return nil
}

// runAutoDiscovery performs MCP server auto-discovery and handles user interaction
func runAutoDiscovery(cfg *config.GlobalConfig) int {
	common.StreamPrint(10, "🔍 Scanning for existing MCP configurations...\n")
	time.Sleep(1 * time.Second)

	// Create discovery manager and run discovery
	dm := discovery.NewDiscoveryManager()
	result := dm.DiscoverAll()

	// Show results and get user consent
	ui := discovery.NewDiscoveryUI()
	selectedServers, err := ui.ShowDiscoveryResults(result)
	if err != nil {
		fmt.Printf("⚠️  Error during discovery UI: %v", err)
		return 0
	}

	if len(selectedServers) == 0 {
		return 0
	}

	// Import selected servers
	imported := discovery.ImportServers(selectedServers, cfg)

	// Show import summary
	discovery.ShowImportSummary(imported, len(selectedServers))

	return imported
}
