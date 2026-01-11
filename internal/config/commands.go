package config

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
)

// ConfigCommand provides configuration management subcommands for the centian CLI.
// This is the main entry point for all config-related operations including
// initialization, validation, and server management.
var ConfigCommand = &cli.Command{
	Name:        "config",
	Usage:       "Manage centian configuration",
	Description: "Commands to manage the global centian configuration at ~/.centian/config.jsonc",
	Commands: []*cli.Command{
		configInitCommand,
		configShowCommand,
		configValidateCommand,
		configRemoveCommand,
		configServerCommand,
	},
}

var configInitCommand = &cli.Command{
	Name:        "init",
	Usage:       "Initialize configuration with defaults",
	Description: "Creates ~/.centian/config.jsonc with default settings if it doesn't exist",
	Action:      initConfig,
}

var configShowCommand = &cli.Command{
	Name:        "show",
	Usage:       "Display current configuration",
	Description: "Shows the current configuration from ~/.centian/config.jsonc",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "json",
			Aliases: []string{"j"},
			Usage:   "Output as JSON",
		},
	},
	Action: showConfig,
}

var configValidateCommand = &cli.Command{
	Name:        "validate",
	Usage:       "Validate configuration file",
	Description: "Validates the syntax and content of ~/.centian/config.jsonc",
	Action:      validateConfig,
}

var configRemoveCommand = &cli.Command{
	Name:        "remove",
	Usage:       "Remove configuration file",
	Description: "Removes ~/.centian/config.jsonc and the entire ~/.centian directory",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "Skip confirmation prompt",
		},
	},
	Action: removeConfig,
}

var configServerCommand = &cli.Command{
	Name:        "server",
	Usage:       "Manage MCP servers",
	Description: "Add, remove, and configure MCP servers",
	Commands: []*cli.Command{
		{
			Name:        "list",
			Usage:       "List all configured servers",
			Description: "Display all MCP servers in the configuration",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "enabled-only",
					Aliases: []string{"e"},
					Usage:   "Show only enabled servers",
				},
			},
			Action: listServers,
		},
		{
			Name:        "add",
			Usage:       "Add a new server",
			Description: "Add a new MCP server configuration",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "name",
					Aliases:  []string{"n"},
					Usage:    "Server name",
					Required: true,
				},
				&cli.StringFlag{
					Name:    "gateway",
					Aliases: []string{"gw"},
					Usage:   "Gateway name",
					Value:   "default",
				},
				&cli.StringFlag{
					Name:    "command",
					Aliases: []string{"c"},
					Usage:   "Server command",
				},
				&cli.StringSliceFlag{
					Name:    "args",
					Aliases: []string{"a"},
					Usage:   "Command arguments",
				},
				&cli.StringFlag{
					Name:    "url",
					Aliases: []string{"u"},
					Usage:   "Server URL",
				},
				&cli.StringMapFlag{
					Name:    "headers",
					Aliases: []string{"hd"},
					Usage:   "Server Headers",
				},
				&cli.StringFlag{
					Name:    "description",
					Aliases: []string{"d"},
					Usage:   "Server description",
				},
				&cli.BoolFlag{
					Name:  "enabled",
					Usage: "Enable server",
					Value: true,
				},
			},
			Action: addServer,
		},
		{
			Name:        "remove",
			Usage:       "Remove a server",
			Description: "Remove an MCP server from configuration",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "name",
					Aliases:  []string{"n"},
					Usage:    "Server name to remove",
					Required: true,
				},
			},
			Action: removeServer,
		},
		{
			Name:        "enable",
			Usage:       "Enable a server",
			Description: "Enable an MCP server",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "name",
					Aliases:  []string{"n"},
					Usage:    "Server name to enable",
					Required: true,
				},
			},
			Action: enableServer,
		},
		{
			Name:        "disable",
			Usage:       "Disable a server",
			Description: "Disable an MCP server",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "name",
					Aliases:  []string{"n"},
					Usage:    "Server name to disable",
					Required: true,
				},
			},
			Action: disableServer,
		},
	},
}

// initConfig initializes a new configuration file with default settings.
// Creates ~/.centian/config.jsonc if it doesn't exist, fails if file already exists.
func initConfig(ctx context.Context, cmd *cli.Command) error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("configuration already exists at %s", configPath)
	}

	// Create default config
	config := DefaultConfig()
	if err := SaveConfig(config); err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	fmt.Printf("✅ Configuration initialized at %s\n", configPath)
	return nil
}

// showConfig displays the current configuration either as formatted text
// or JSON based on the --json flag.
func showConfig(ctx context.Context, cmd *cli.Command) error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if cmd.Bool("json") {
		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}
		fmt.Println(string(data))
	} else {
		configPath, _ := GetConfigPath()
		fmt.Printf("Configuration path: %s\n", configPath)
		fmt.Printf("Version: %s\n", config.Version)
		if config.Proxy.LogLevel != "" {
			fmt.Printf("Log Level: %s\n", config.Proxy.LogLevel)
		}
		fmt.Printf("Gateways: %d configured\n", len(config.Gateways))

		allServers := []*MCPServerConfig{}
		for _, gatewayConfig := range config.Gateways {
			allServers = append(allServers, gatewayConfig.ListServers()...)
		}
		fmt.Printf("Servers: %d configured\n", len(allServers))

		enabled := []*MCPServerConfig{}
		for _, serverConfig := range allServers {
			if serverConfig.Enabled {
				enabled = append(enabled, serverConfig)
			}
		}
		fmt.Printf("  - Enabled: %d\n", len(enabled))
		fmt.Printf("  - Disabled: %d\n", len(allServers)-len(enabled))
	}

	return nil
}

func validateConfig(ctx context.Context, cmd *cli.Command) error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("❌ Configuration validation failed: %w", err)
	}

	if err := ValidateConfig(config); err != nil {
		return fmt.Errorf("❌ Configuration validation failed: %w", err)
	}

	configPath, _ := GetConfigPath()
	fmt.Printf("✅ Configuration is valid: %s\n", configPath)
	return nil
}

func listServers(ctx context.Context, cmd *cli.Command) error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	enabledOnly := cmd.Bool("enabled-only")
	gateways := config.Gateways

	if len(gateways) == 0 {
		fmt.Println("No gateways configured.")
		return nil
	}

	fmt.Printf("MCP Servers:\n")
	for gatewayName, gatewayConfig := range gateways {
		fmt.Printf("- %s\n", gatewayName)
		for serverName, server := range gatewayConfig.MCPServers {
			if enabledOnly && !server.Enabled {
				continue
			}
			status := "✅ enabled"
			if !server.Enabled {
				status = "❌ disabled"
			}

			fmt.Printf("  - %s (%s)\n", serverName, status)
			// stdio
			if server.Command != "" {
				fmt.Printf("      Command: '%s'\n", server.Command)
			}
			if len(server.Args) > 0 {
				fmt.Printf("      Args: '%v'\n", server.Args)
			}
			// http
			if server.URL != "" {
				fmt.Printf("      URL: '%s'\n", server.URL)
			}
			if len(server.Headers) > 0 {
				fmt.Printf("      Headers: '%v'\n", server.Headers)
			}

			if server.Source != "" {
				fmt.Printf("      Source: %s\n", server.Source)
			}
			fmt.Println()
		}
	}
	return nil
}

// addServer adds a new MCP server configuration to the global config.
// Validates that the server name doesn't already exist before adding.
func addServer(ctx context.Context, cmd *cli.Command) error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	gatewayName := cmd.String("gateway")
	existingGateway, ok := config.Gateways[gatewayName]
	if !ok {
		// gatewayName does not exist in the config, so we create it
		// Note: if no gatewayName was specified the default is "default"
		config.Gateways[gatewayName] = &GatewayConfig{
			AllowDynamic:         false,
			AllowGatewayEndpoint: false,
			MCPServers:           map[string]*MCPServerConfig{},
			Processors:           make([]*ProcessorConfig, 0),
		}
		existingGateway = config.Gateways[gatewayName]
	}

	name := cmd.String("name")
	if _, exists := existingGateway.MCPServers[name]; exists {
		return fmt.Errorf("server '%s' already exists", name)
	}

	serverConfig := &MCPServerConfig{
		Name:        name,
		Command:     cmd.String("command"),
		Args:        cmd.StringSlice("args"),
		URL:         cmd.String("URL"),
		Headers:     cmd.StringMap("headers"),
		Description: cmd.String("description"),
		Enabled:     cmd.Bool("enabled"),
	}
	existingGateway.AddServer(name, serverConfig)

	if err := SaveConfig(config); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("✅ Added server '%s'\n", name)
	return nil
}

// promptUserToSelectServer displays multiple server matches and prompts user to select one.
// Returns the selected ServerSearchResult or error if cancelled/invalid selection.
func promptUserToSelectServer(foundServers []ServerSearchResult, serverName string) (*ServerSearchResult, error) {
	fmt.Printf("⚠️  Server '%s' found in multiple gateways:\n\n", serverName)

	// Display all matches with context
	for i, result := range foundServers {
		status := "✅ enabled"
		if !result.server.Enabled {
			status = "❌ disabled"
		}

		transport := "stdio"
		transportInfo := fmt.Sprintf("command: %s", result.server.Command)
		if result.server.URL != "" {
			transport = "http"
			transportInfo = fmt.Sprintf("url: %s", result.server.URL)
		}

		fmt.Printf("  [%d] Gateway: %s\n", i+1, result.gatewayName)
		fmt.Printf("      Status: %s\n", status)
		fmt.Printf("      Transport: %s (%s)\n", transport, transportInfo)
		if result.server.Description != "" {
			fmt.Printf("      Description: %s\n", result.server.Description)
		}
		fmt.Println()
	}

	// Prompt for selection
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Select gateway [1-%d] or 'c' to cancel: ", len(foundServers))

	response, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))

	// Handle cancellation
	if response == "c" || response == "cancel" {
		return nil, fmt.Errorf("operation cancelled")
	}

	// Parse selection number
	var selection int
	if _, err := fmt.Sscanf(response, "%d", &selection); err != nil {
		return nil, fmt.Errorf("invalid selection: %s", response)
	}

	// Validate selection range
	if selection < 1 || selection > len(foundServers) {
		return nil, fmt.Errorf("selection out of range: %d (valid: 1-%d)", selection, len(foundServers))
	}

	// Return selected result (convert to 0-based index)
	return &foundServers[selection-1], nil
}

func removeServer(ctx context.Context, cmd *cli.Command) error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	serverName := cmd.String("name")
	foundServers := config.SearchServerByName(serverName)

	switch len(foundServers) {
	case 0:
		return fmt.Errorf("Unable to find server '%s' in config", serverName)
	case 1:
		// expected, "good" case -> we just remove this single server
		result := foundServers[0]
		result.gateway.RemoveServer(serverName)
	default:
		// Multiple matches - prompt user to select
		selected, err := promptUserToSelectServer(foundServers, serverName)
		if err != nil {
			return err
		}
		selected.gateway.RemoveServer(serverName)
	}

	if err := SaveConfig(config); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("✅ Removed server '%s'\n", serverName)
	return nil
}

func enableServer(ctx context.Context, cmd *cli.Command) error {
	return toggleServer(cmd.String("name"), true)
}

func disableServer(ctx context.Context, cmd *cli.Command) error {
	return toggleServer(cmd.String("name"), false)
}

func toggleServer(name string, enabled bool) error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	foundServers := config.SearchServerByName(name)

	switch len(foundServers) {
	case 0:
		return fmt.Errorf("Unable to find server '%s' in config", name)
	case 1:
		// expected, "good" case -> we just toggle this single server
		result := foundServers[0]
		result.server.Enabled = enabled
	default:
		// Multiple matches - prompt user to select
		selected, err := promptUserToSelectServer(foundServers, name)
		if err != nil {
			return err
		}
		selected.server.Enabled = enabled
	}

	if err := SaveConfig(config); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	status := "enabled"
	if !enabled {
		status = "disabled"
	}
	fmt.Printf("✅ Server '%s' %s", name, status)
	return nil
}

// removeConfig removes the entire centian configuration
func removeConfig(ctx context.Context, cmd *cli.Command) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	// Check if config exists
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		fmt.Printf("ℹ️  No configuration found at %s", configDir)
		return nil
	}

	// Skip confirmation if --force is used
	if !cmd.Bool("force") {
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("⚠️  This will permanently remove your centian configuration at:")
		fmt.Printf("   %s", configDir)
		fmt.Printf("\n⚠️ This action cannot be undone. Continue? [y/N]: ")

		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Printf("❌ Operation cancelled")
			return nil
		}
	}

	// Remove the entire config directory
	if err := os.RemoveAll(configDir); err != nil {
		return fmt.Errorf("failed to remove configuration: %w", err)
	}

	fmt.Println("✅ Configuration removed successfully")
	fmt.Println("💡 Run 'centian init' to create a new configuration")

	return nil
}
