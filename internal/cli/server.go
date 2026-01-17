// Copyright 2025 CentianCLI Contributors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at.
//
//     http://www.apache.org/licenses/LICENSE-2.0.
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CentianAI/centian-cli/internal/config"
	"github.com/CentianAI/centian-cli/internal/proxy"
	"github.com/urfave/cli/v3"
)

// ServerCommand provides server management functionality.
var ServerCommand = &cli.Command{
	Name:  "server",
	Usage: "Manage Centian proxy server",
	Commands: []*cli.Command{
		ServerStartCommand,
	},
}

// ServerStartCommand starts the Centian proxy server.
var ServerStartCommand = &cli.Command{
	Name:  "start",
	Usage: "centian server start [--config-path <path>]",
	Description: `Start Centian proxy server for configured MCP servers.

Currently supports HTTP transport. The HTTP proxy creates endpoints for each
configured HTTP MCP server at:
  /mcp/<gateway_name>/<server_name>

Configuration is loaded from ~/.centian/config.jsonc by default.

Example config structure:
  {
    "version": "1.0.0",
    "name": "My Centian Server",
    "proxy": {
      "port": "8080",
      "timeout": 30
    },
    "gateways": {
      "my-gateway": {
        "mcpServers": {
          "github": {
            "url": "https://api.githubcopilot.com/mcp/",
            "headers": {
              "Authorization": "Bearer ${MY_GH_TOKEN_ENV_VAR}"
            }
          }
        }
      }
    }
  }

Examples:
  centian server start
  centian server start --config-path ./custom-config.json
`,
	Action: handleServerStartCommand,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "config-path",
			Usage: "Path to config file (default: ~/.centian/config.jsonc)",
		},
	},
}

func printServerInfo(globalConfig *config.GlobalConfig) error {
	serverName := globalConfig.Name
	if serverName == "" {
		serverName = "Centian Proxy Server"
	}
	totalServers := 0
	for _, gateway := range globalConfig.Gateways {
		totalServers += len(gateway.MCPServers)
	}

	if totalServers == 0 {
		return fmt.Errorf("no MCP servers configured in gateways")
	}

	fmt.Fprintf(os.Stderr, "[CENTIAN] %s\n", serverName)
	fmt.Fprintf(os.Stderr, "[CENTIAN] Starting HTTP proxy server...\n")
	fmt.Fprintf(os.Stderr, "[CENTIAN] Port: %s\n", globalConfig.Proxy.Port)
	fmt.Fprintf(os.Stderr, "[CENTIAN] Timeout: %ds\n", globalConfig.Proxy.Timeout)
	fmt.Fprintf(os.Stderr, "[CENTIAN] Gateways: %d\n", len(globalConfig.Gateways))
	fmt.Fprintf(os.Stderr, "[CENTIAN] Total MCP servers: %d\n", totalServers)
	fmt.Fprintf(os.Stderr, "\n")

	// Print endpoint information.
	fmt.Fprintf(os.Stderr, "[CENTIAN] Configured endpoints:\n")
	for gatewayName, gateway := range globalConfig.Gateways {
		for serverName, server := range gateway.MCPServers {
			endpoint := fmt.Sprintf("/mcp/%s/%s", gatewayName, serverName)
			fmt.Fprintf(os.Stderr, "  - http://localhost:%s%s -> %s\n",
				globalConfig.Proxy.Port, endpoint, server.URL)
		}
	}
	fmt.Fprintf(os.Stderr, "\n")
	return nil
}

// handleServerStartCommand handles the server start command.
func handleServerStartCommand(_ context.Context, cmd *cli.Command) error {
	configPath := cmd.String("config-path")

	// Load configuration.
	var globalConfig *config.GlobalConfig
	var err error

	if configPath == "" {
		configPath, _ = config.GetConfigPath()
	}
	globalConfig, err = config.LoadConfigFromPath(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config from %s: %w", configPath, err)
	}
	fmt.Fprintf(os.Stderr, "[CENTIAN] Loaded config from: %s\n", configPath)

	// Validate that we have proxy configuration.
	if globalConfig.Proxy == nil {
		return fmt.Errorf("proxy settings are required in config")
	}

	if len(globalConfig.Gateways) == 0 {
		return fmt.Errorf("no gateways configured. Add at least one gateway with HTTP MCP servers in your config")
	}

	// Display server information.
	if err := printServerInfo(globalConfig); err != nil {
		return err
	}

	// Create HTTP proxy server.
	// TODO: handle stdio as well - requires cross-transport support.
	server, err := proxy.NewCentianHTTPProxy(globalConfig)
	if err != nil {
		return fmt.Errorf("failed to create HTTP proxy server: %w", err)
	}

	// Set up signal handling for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in background.
	errChan := make(chan error, 1)
	go func() {
		if err := server.StartCentianServer(); err != nil {
			errChan <- fmt.Errorf("HTTP proxy server error: %w", err)
		}
	}()

	fmt.Fprintf(os.Stderr, "[CENTIAN] HTTP proxy server started successfully\n")
	fmt.Fprintf(os.Stderr, "[CENTIAN] Press Ctrl+C to stop\n\n")

	// Wait for either signal or server error.
	select {
	case <-sigChan:
		fmt.Fprintf(os.Stderr, "\n[CENTIAN] Received shutdown signal, stopping server...\n")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("error during shutdown: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[CENTIAN] Server stopped successfully\n")
		return nil
	case err := <-errChan:
		return err
	}
}
