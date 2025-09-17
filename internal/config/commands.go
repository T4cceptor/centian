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

// TODO: determine if this is required - it makes sense, but maybe we have better options
// var configCleanCommand = &cli.Command{
// 	Name:        "clean",
// 	Usage:       "Clean invalid servers from configuration",
// 	Description: "Removes servers with empty commands or other validation issues",
// 	Flags: []cli.Flag{
// 		&cli.BoolFlag{
// 			Name:    "dry-run",
// 			Aliases: []string{"n"},
// 			Usage:   "Show what would be cleaned without making changes",
// 		},
// 	},
// 	Action: cleanConfig,
// }

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
					Name:     "command",
					Aliases:  []string{"c"},
					Usage:    "Server command",
					Required: true,
				},
				&cli.StringSliceFlag{
					Name:    "args",
					Aliases: []string{"a"},
					Usage:   "Command arguments",
				},
				&cli.StringFlag{
					Name:    "description",
					Aliases: []string{"d"},
					Usage:   "Server description",
				},
				&cli.StringSliceFlag{
					Name:    "tags",
					Aliases: []string{"t"},
					Usage:   "Server tags",
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
		fmt.Printf("Transport: %s\n", config.Proxy.Transport)
		if config.Proxy.LogLevel != "" {
			fmt.Printf("Log Level: %s\n", config.Proxy.LogLevel)
		}
		fmt.Printf("Servers: %d configured\n", len(config.Servers))

		enabled := len(config.ListEnabledServers())
		fmt.Printf("  - Enabled: %d\n", enabled)
		fmt.Printf("  - Disabled: %d\n", len(config.Servers)-enabled)
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
	servers := config.Servers

	if len(servers) == 0 {
		fmt.Println("No servers configured.")
		return nil
	}

	fmt.Printf("MCP Servers:\n")
	for name, server := range servers {
		if enabledOnly && !server.Enabled {
			continue
		}

		status := "✅ enabled"
		if !server.Enabled {
			status = "❌ disabled"
		}

		fmt.Printf("  %s (%s)\n", name, status)
		if server.Command != "" {
			fmt.Printf("    Command: %s %v\n", server.Command, server.Args)
		}
		if server.URL != "" {
			fmt.Printf("    URL: %s %v\n", server.URL, server.Args)
		}
		if len(server.Args) > 0 {
			fmt.Printf("    Args: %v\n", server.Args)
		}
		if server.Source != "" {
			fmt.Printf("    Source: %s\n", server.Source)
		}
		fmt.Println()
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

	name := cmd.String("name")
	if _, exists := config.Servers[name]; exists {
		return fmt.Errorf("server '%s' already exists", name)
	}

	server := &MCPServer{
		Name:        name,
		Command:     cmd.String("command"),
		Args:        cmd.StringSlice("args"),
		Description: cmd.String("description"),
		Enabled:     cmd.Bool("enabled"),
	}

	config.AddServer(name, server)

	if err := SaveConfig(config); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("✅ Added server '%s'\n", name)
	return nil
}

func removeServer(ctx context.Context, cmd *cli.Command) error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	name := cmd.String("name")
	if _, exists := config.Servers[name]; !exists {
		return fmt.Errorf("server '%s' not found", name)
	}

	config.RemoveServer(name)

	if err := SaveConfig(config); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("✅ Removed server '%s'\n", name)
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

	server, exists := config.Servers[name]
	if !exists {
		return fmt.Errorf("server '%s' not found", name)
	}

	server.Enabled = enabled

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
