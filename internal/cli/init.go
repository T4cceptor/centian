// Package cli provides all CLI commands centian offers,
// including init, stdio, server, logs, config and all of their sub-commands.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/discovery"
	"github.com/urfave/cli/v3"
)

// InitOption represents the user's choice for initialization method.
type InitOption int

const (
	// InitOptionEmpty creates an empty config with no servers.
	InitOptionEmpty InitOption = iota
	// InitOptionQuickstart creates a ready-to-run config with a default MCP server.
	InitOptionQuickstart
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
	fmt.Printf("  [2] Quickstart (sequential-thinking, requires npx)\n")
	// TODO: add this back in once discovery is fixed: fmt.Printf("  [3] Auto-discover existing MCP servers (recommended)\n")
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
	case "2":
		return InitOptionQuickstart, nil
	// TODO: add discovery again (return InitOptionDiscovery, nil) - requires refactoring/fixing of current discovery
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
		&cli.BoolFlag{
			Name:  "quickstart",
			Usage: "Create a ready-to-run config (requires npx)",
		},
	},
}

// handleInteractiveInit prompts the user for initialization method and performs import.
func handleInteractiveInit(cfg *config.GlobalConfig, ui *InitUI) (int, bool, error) {
	option, err := ui.promptInitOption()
	if err != nil {
		fmt.Printf("⚠️  %v. Starting with empty config.\n", err)
		return 0, false, nil
	}

	switch option {
	case InitOptionEmpty:
		return 0, false, nil
	case InitOptionQuickstart:
		if _, err := exec.LookPath("npx"); err != nil {
			return 0, false, fmt.Errorf("quickstart requires npx to be installed and available on PATH")
		}
		applyQuickstartConfig(cfg)
		return 1, true, nil
	case InitOptionDiscovery:
		return runAutoDiscovery(cfg), false, nil
	case InitOptionFromPath:
		path, pathErr := ui.promptConfigPath()
		if pathErr != nil {
			fmt.Printf("⚠️  %v. \n\nStarting with empty config.\n", pathErr)
			return 0, false, nil
		}
		imported, importErr := importFromPath(cfg, path)
		if importErr != nil {
			fmt.Printf("⚠️  %v. \n\nStarting with empty config.\n", importErr)
			return 0, false, nil
		}
		return imported, false, nil
	default:
		return 0, false, nil
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
	quickstart := cmd.Bool("quickstart")
	ui := NewInitUI()

	// Determine initialization mode based on flags or interactive prompt.
	if quickstart {
		if _, err := exec.LookPath("npx"); err != nil {
			return fmt.Errorf("quickstart requires npx to be installed and available on PATH")
		}
		applyQuickstartConfig(cfg)
		imported = 1
	} else if cmd.Bool("no-discovery") {
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
		var usedQuickstart bool
		var interactiveErr error
		imported, usedQuickstart, interactiveErr = handleInteractiveInit(cfg, ui)
		if interactiveErr != nil {
			return interactiveErr
		}
		if usedQuickstart {
			quickstart = true
		}
	}

	// Save config (either default or with imported servers).
	if len(cfg.Gateways) == 0 {
		if err := config.SaveConfigSchema(cfg); err != nil {
			return fmt.Errorf("failed to create configuration: %w", err)
		}
	} else if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	if quickstart {
		apiKey, err := createDefaultAPIKey()
		if err != nil {
			return err
		}
		printQuickstartSummary(configPath, cfg, apiKey)
		return nil
	}

	fmt.Printf("\n🎉 Centian initialized successfully!\n")
	fmt.Printf("📁 Configuration created at: %s\n\n", configPath)

	fmt.Printf("📋 Next steps:\n")
	if imported == 0 {
		fmt.Printf("  1. Add MCP servers to proxy:\n")
		fmt.Printf("     centian config server add --name \"my-server\" --command \"npx\" --args \"-y,@upstash/context7-mcp,--api-key,YOUR_KEY\"\n\n")
	}
	fmt.Printf("  2. Create an API key:\n")
	fmt.Printf("     centian server get-key\n\n")
	fmt.Printf("  3. Start the proxy:\n")
	fmt.Printf("     centian server start\n\n")
	fmt.Printf("  4. Configure your MCP client to use centian:\n")
	fmt.Printf(`
    {
        "mcpServers": {
            "centian": {
                "url": "http://localhost:8080/mcp/default",
                "headers": {
                    "X-Centian-Auth": <your api key - see step 2>
                }
            }
        }
    }

`)

	fmt.Printf("💡 Use 'centian config --help' for more configuration options\n")
	fmt.Printf("Press enter to continue")

	_, _ = ui.reader.ReadString('\n')

	// Offer to set up shell completion.
	if err := SetupShellCompletion(); err != nil {
		fmt.Printf("⚠️  Shell completion setup failed: %v\n", err)
		fmt.Printf("   You can set it up manually later using: centian completion <shell>\n")
	}

	return nil
}

func applyQuickstartConfig(cfg *config.GlobalConfig) {
	enabled := true
	cfg.Gateways = map[string]*config.GatewayConfig{
		"default": {
			AllowDynamic:         false,
			AllowGatewayEndpoint: false,
			MCPServers: map[string]*config.MCPServerConfig{
				"sequential-thinking": {
					Name:        "sequential-thinking",
					Command:     "npx",
					Args:        []string{"-y", "@modelcontextprotocol/server-sequential-thinking"},
					Enabled:     &enabled,
					Description: "Sequential thinking MCP server (via npx)",
				},
			},
			Processors: []*config.ProcessorConfig{},
		},
	}
}

func createDefaultAPIKey() (string, error) {
	key, err := auth.GenerateAPIKey()
	if err != nil {
		return "", fmt.Errorf("failed to generate api key: %w", err)
	}
	entry, err := auth.NewAPIKeyEntry(key)
	if err != nil {
		return "", fmt.Errorf("failed to create api key entry: %w", err)
	}
	path, err := auth.DefaultAPIKeysPath()
	if err != nil {
		return "", fmt.Errorf("failed to resolve api key path: %w", err)
	}
	if _, err := auth.AppendAPIKey(path, entry); err != nil {
		return "", fmt.Errorf("failed to persist api key: %w", err)
	}
	return key, nil
}

func printQuickstartSummary(configPath string, cfg *config.GlobalConfig, apiKey string) {
	host := cfg.Proxy.Host
	if host == "" {
		host = config.DefaultProxyHost
	}
	endpoint := fmt.Sprintf("http://%s:%s/mcp/default", host, cfg.Proxy.Port)
	authHeader := cfg.GetAuthHeader()

	fmt.Printf("\n✅ Quickstart configuration initialized\n")
	fmt.Printf("📁 Configuration created at: %s\n", configPath)
	fmt.Printf("🔑 API key: %s\n\n", apiKey)

	fmt.Println("MCP client config snippets:")
	fmt.Println("Claude Desktop / Cursor / Zed (mcpServers):")
	fmt.Printf(`{
  "mcpServers": {
    "centian": {
      "url": "%s",
      "headers": {
        "%s": "%s"
      }
    }
  }
}
`, endpoint, authHeader, apiKey)
	fmt.Println("\nVS Code (mcp.json):")
	fmt.Printf(`{
  "servers": {
    "centian": {
      "type": "http",
      "url": "%s",
      "headers": {
        "%s": "%s"
      }
    }
  }
}
`, endpoint, authHeader, apiKey)
	fmt.Println("\nCopy the above snippets into your MCP client settings and start centian by running 'centian server start'.")
}

// runAutoDiscovery performs MCP server auto-discovery and handles user interaction.
func runAutoDiscovery(_ *config.GlobalConfig) int {
	// TODO: instead of adding the found servers to the file it
	// should add it to the cfg object, then use existing methods to store that config.
	// TODO: refactor discovery!

	fmt.Printf("🔍 Scanning for existing MCP configurations...\n")
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
