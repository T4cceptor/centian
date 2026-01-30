// Package cli provides all CLI commands centian offers,
// including init, stdio, server, logs, config and all of their sub-commands.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
	"github.com/CentianAI/centian-cli/internal/config"
	"github.com/CentianAI/centian-cli/internal/discovery"
	"github.com/urfave/cli/v3"
)

// InitOption represents the user's choice for initialization method.
type InitOption int

const (
	// InitOptionEmpty creates an empty config with no servers.
	InitOptionEmpty InitOption = iota
	// InitOptionDiscovery auto-discovers existing MCP servers.
	InitOptionDiscovery
	// InitOptionFromPath imports servers from a specific config file.
	InitOptionFromPath
)

// InitUI provides user interface functions for the init command.
type InitUI struct {
	reader *bufio.Reader
}

// NewInitUI creates a new init UI interface.
func NewInitUI() *InitUI {
	return &InitUI{
		reader: bufio.NewReader(os.Stdin),
	}
}

// promptInitOption asks the user how they want to initialize centian.
func (ui *InitUI) promptInitOption() (InitOption, error) {
	fmt.Printf("\n🎉 Welcome to Centian!\n\n")
	fmt.Printf("How would you like to initialize your configuration?\n\n")
	fmt.Printf("  [1] Start fresh (empty config)\n")
	fmt.Printf("  [2] Auto-discover existing MCP servers (recommended)\n")
	fmt.Printf("  [3] Import from a specific config file\n\n")
	fmt.Printf("Choice [1/2/3]: ")

	response, err := ui.reader.ReadString('\n')
	if err != nil {
		return InitOptionEmpty, fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.TrimSpace(response)

	switch response {
	case "1":
		return InitOptionEmpty, nil
	case "2", "":
		return InitOptionDiscovery, nil
	case "3":
		return InitOptionFromPath, nil
	default:
		fmt.Printf("Invalid choice '%s'. Using empty config.\n", response)
		return InitOptionEmpty, nil
	}
}

// promptConfigPath asks the user for a config file path.
func (ui *InitUI) promptConfigPath() (string, error) {
	fmt.Printf("\nEnter the path to your MCP config file: ")

	response, err := ui.reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	path := strings.TrimSpace(response)
	if path == "" {
		return "", fmt.Errorf("no path provided")
	}

	// Validate file exists.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("file does not exist: %s", path)
	}

	return path, nil
}

// importFromPath imports servers from a specific config file path.
// Note: cfg parameter is currently unused as discovery.ImportServers doesn't
// add servers to cfg yet (see TODO in runAutoDiscovery).
//
//nolint:gosec // G304: path is user-provided intentionally for config import
func importFromPath(_ *config.GlobalConfig, path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("failed to read file: %w", err)
	}

	servers, err := discovery.ParseConfigFile(data, path)
	if err != nil {
		return 0, fmt.Errorf("failed to parse config: %w", err)
	}

	if len(servers) == 0 {
		fmt.Printf("⚠️  No servers found in %s\n", path)
		return 0, nil
	}

	fmt.Printf("📦 Found %d server(s) in %s\n", len(servers), path)

	// Import servers using existing discovery import logic.
	imported := discovery.ImportServers(servers)
	discovery.ShowImportSummary(imported)

	return imported, nil
}

// InitCommand initializes a new centian setup with default configuration.
var InitCommand = &cli.Command{
	Name:        "init",
	Usage:       "Initialize centian with default configuration",
	Description: "Creates ~/.centian/config.json with default settings and guides initial setup",
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
			Usage:   "Skip auto-discovery and start with empty configuration",
		},
		&cli.StringFlag{
			Name:    "from-path",
			Aliases: []string{"p"},
			Usage:   "Import servers from a specific MCP config file path",
		},
	},
}

// handleInteractiveInit prompts the user for initialization method and performs import.
func handleInteractiveInit(cfg *config.GlobalConfig, ui *InitUI) int {
	option, err := ui.promptInitOption()
	if err != nil {
		fmt.Printf("⚠️  %v. Starting with empty config.\n", err)
		return 0
	}

	switch option {
	case InitOptionEmpty:
		return 0
	case InitOptionDiscovery:
		return runAutoDiscovery(cfg)
	case InitOptionFromPath:
		path, pathErr := ui.promptConfigPath()
		if pathErr != nil {
			fmt.Printf("⚠️  %v. Starting with empty config.\n", pathErr)
			return 0
		}
		imported, importErr := importFromPath(cfg, path)
		if importErr != nil {
			fmt.Printf("⚠️  %v. Starting with empty config.\n", importErr)
			return 0
		}
		return imported
	default:
		return 0
	}
}

// initCentian initializes the centian configuration and provides setup guidance.
// This is the main entry point for new users to get started with centian.
func initCentian(_ context.Context, cmd *cli.Command) error {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("failed to determine config path: %w", err)
	}

	// Check if config already exists.
	if !cmd.Bool("force") {
		if _, err := config.LoadConfig(); err == nil {
			fmt.Printf("✅ Configuration already exists at %s\n", configPath)
			fmt.Printf("💡 Use 'centian config show' to view current configuration\n")
			fmt.Printf("💡 Use 'centian init --force' to overwrite existing configuration\n")
			return nil
		}
	}

	// Create default config.
	cfg := config.DefaultConfig()

	var imported int
	ui := NewInitUI()

	// Determine initialization mode based on flags or interactive prompt.
	if cmd.Bool("no-discovery") {
		// Flag: empty config.
		imported = 0
	} else if fromPath := cmd.String("from-path"); fromPath != "" {
		// Flag: import from specific path.
		var importErr error
		imported, importErr = importFromPath(cfg, fromPath)
		if importErr != nil {
			return fmt.Errorf("failed to import from path: %w", importErr)
		}
	} else {
		// Interactive mode: prompt user.
		imported = handleInteractiveInit(cfg, ui)
	}

	// Save config (either default or with imported servers).
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	fmt.Printf("\n🎉 Centian initialized successfully!\n\n")
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

	// Offer to set up shell completion.
	if err := SetupShellCompletion(); err != nil {
		fmt.Printf("⚠️  Shell completion setup failed: %v\n", err)
		fmt.Printf("   You can set it up manually later using: centian completion <shell>\n")
	}

	return nil
}

// runAutoDiscovery performs MCP server auto-discovery and handles user interaction.
func runAutoDiscovery(_ *config.GlobalConfig) int {
	// TODO: instead of adding the found servers to the file it
	// should add it to the cfg object, then use existing methods to store that config.

	common.StreamPrint(10, "🔍 Scanning for existing MCP configurations...\n")
	time.Sleep(1 * time.Second)

	// Create discovery manager and run discovery.
	dm := discovery.NewDiscoveryManager()
	result := dm.DiscoverAll()

	// Show results and get user consent.
	ui := discovery.NewDiscoveryUI()
	selectedServers, err := ui.ShowDiscoveryResults(result)
	if err != nil {
		fmt.Printf("⚠️  Error during discovery UI: %v", err)
		return 0
	}

	if len(selectedServers) == 0 {
		return 0
	}

	// Import selected servers.
	imported := discovery.ImportServers(selectedServers)

	// Show import summary.
	discovery.ShowImportSummary(imported)

	return imported
}
