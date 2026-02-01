// Copyright 2025 Centian Contributors.
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

	"github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/proxy"
	"github.com/urfave/cli/v3"
)

// ServerCommand provides server management functionality.
var ServerCommand = &cli.Command{
	Name:  "server",
	Usage: "Manage Centian proxy server",
	Commands: []*cli.Command{
		ServerStartCommand,
		ServerGetKeyCommand,
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

Configuration is loaded from ~/.centian/config.json by default.

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
			Usage: "Path to config file (default: ~/.centian/config.json)",
		},
	},
}

// ServerGetKeyCommand generates and stores a new API key.
var ServerGetKeyCommand = &cli.Command{
	Name:  "get-key",
	Usage: "centian server get-key",
	Description: `Generate a new API key for the HTTP proxy.

The key is printed once to the console, then hashed with bcrypt and stored in:
  ~/.centian/api_keys.json
`,
	Action: handleServerGetKeyCommand,
}

func printServerInfo(globalConfig *config.GlobalConfig) error {
	serverName := globalConfig.Name
	if serverName == "" {
		serverName = "Centian Proxy Server"
	}
	if len(globalConfig.Gateways) < 1 {
		return fmt.Errorf("no gateways configured")
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
	host := globalConfig.Proxy.Host
	if host == "" {
		host = config.DefaultProxyHost
	}
	fmt.Fprintf(os.Stderr, "[CENTIAN] Host: %s\n", host)
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
			if server.URL != "" {
				fmt.Fprintf(os.Stderr, "  - http://%s:%s%s -> %s\n",
					host, globalConfig.Proxy.Port, endpoint, server.URL)
			}
			if server.Command != "" {
				fmt.Fprintf(os.Stderr, "  - http://%s:%s%s -> %s -- %#v\n",
					host, globalConfig.Proxy.Port, endpoint, server.Command, server.Args)
			}
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
	err = config.ValidateConfig(globalConfig)
	if err != nil {
		return fmt.Errorf("config validation failed for %s: %w", configPath, err)
	}
	fmt.Fprintf(os.Stderr, "[CENTIAN] Loaded config from: %s\n", configPath)

	// Create HTTP proxy server.
	server, err := proxy.NewCentianProxy(globalConfig)
	if err != nil {
		return fmt.Errorf("failed to create centian server: %w", err)
	}
	if setupErr := server.Setup(); setupErr != nil {
		return fmt.Errorf("failed to setup centian server: %w", setupErr)
	}

	// Set up signal handling for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in background.
	errChan := make(chan error, 1)

	// Display server information.
	if err := printServerInfo(globalConfig); err != nil {
		return err
	}
	go func() {
		if err := server.Server.ListenAndServe(); err != nil {
			errChan <- fmt.Errorf("HTTP proxy server error: %w", err)
		}
	}()

	fmt.Fprintf(os.Stderr, "[CENTIAN] Proxy servers started successfully\n")
	fmt.Fprintf(os.Stderr, "[CENTIAN] Press Ctrl+C to stop\n\n")

	// Wait for either signal or server error.
	select {
	case <-sigChan:
		fmt.Fprintf(os.Stderr, "\n[CENTIAN] Received shutdown signal, stopping server...\n")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("error during shutdown: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[CENTIAN] Server stopped successfully\n")
		return nil
	case err := <-errChan:
		return err
	}
}

// handleServerGetKeyCommand generates and stores a new API key.
func handleServerGetKeyCommand(_ context.Context, _ *cli.Command) error {
	path, err := auth.DefaultAPIKeysPath()
	if err != nil {
		return fmt.Errorf("failed to resolve api key path: %w", err)
	}

	key, err := auth.GenerateAPIKey()
	if err != nil {
		return err
	}

	var pErr error
	_, pErr = fmt.Fprintln(os.Stdout, "New API key (store this now, it won't be shown again):")
	if pErr != nil {
		return pErr
	}
	_, pErr = fmt.Fprintln(os.Stdout, key)
	if pErr != nil {
		return pErr
	}

	entry, err := auth.NewAPIKeyEntry(key)
	if err != nil {
		return err
	}

	if _, err := auth.AppendAPIKey(path, entry); err != nil {
		return err
	}

	_, pErr = fmt.Fprintf(os.Stdout, "Stored hashed key in %s\n", path)
	if pErr != nil {
		return pErr
	}
	return nil
}
