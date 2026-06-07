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
	"strings"
	"syscall"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/proxy"
	"github.com/urfave/cli/v3"
)

// StartCommand starts the Centian proxy server.
var StartCommand = &cli.Command{
	Name:  "start",
	Usage: "Start Centian proxy: centian start [--config-path <path>]",
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
	      "port": "9666",
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
  centian start
  centian start --config-path ./custom-config.json
`,
	Action: handleServerStartCommand,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "config-path",
			Usage: "Path to config file (default: ~/.centian/config.json)",
		},
	},
}

// printServerInfo logs the configured projects, endpoints, and task verification context.
func printServerInfo(globalConfig *config.GlobalConfig, taskWorkingDir string) error {
	// Ensure the config is in project-based layout.
	globalConfig.ResolveProjects()

	serverName := globalConfig.Name
	if serverName == "" {
		serverName = "Centian Proxy Server"
	}

	totalGateways, totalServers := countGatewaysAndServers(globalConfig.Projects)
	if totalGateways == 0 {
		return fmt.Errorf("no gateways configured")
	}
	if totalServers == 0 {
		return fmt.Errorf("no MCP servers configured in gateways")
	}

	host := globalConfig.Proxy.Host
	if host == "" {
		host = config.DefaultProxyHost
	}
	common.LogInfo("%s", serverName)
	common.LogInfo("Starting HTTP proxy server")
	common.LogInfo("Host: %s", host)
	common.LogInfo("Port: %s", globalConfig.Proxy.Port)
	common.LogInfo("Timeout: %ds", globalConfig.Proxy.Timeout)

	common.LogInfo("Projects: %d", len(globalConfig.Projects))
	common.LogInfo("Total gateways: %d", totalGateways)
	common.LogInfo("Total MCP servers: %d", totalServers)
	common.LogInfo("Configured endpoints:")

	for projectSlug, project := range globalConfig.Projects {
		if project == nil {
			continue
		}
		printProjectEndpoints(projectSlug, project, host, globalConfig.Proxy.Port, taskWorkingDir)
	}
	return nil
}

// countGatewaysAndServers totals the configured gateways and MCP servers across projects.
func countGatewaysAndServers(projects map[string]*config.ProjectConfig) (int, int) {
	totalGateways := 0
	totalServers := 0
	for _, project := range projects {
		if project == nil {
			continue
		}
		totalGateways += len(project.Gateways)
		for _, gateway := range project.Gateways {
			if gateway == nil {
				continue
			}
			totalServers += len(gateway.MCPServers)
		}
	}
	return totalGateways, totalServers
}

// printProjectEndpoints logs the externally reachable endpoints for one configured project.
func printProjectEndpoints(slug string, project *config.ProjectConfig, host, port, taskWorkingDir string) {
	routePrefix := ""
	if slug != config.DefaultProjectSlug {
		routePrefix = "/" + slug
	}
	if project.TaskVerificationEnabled() && taskWorkingDir != "" {
		common.LogInfo("  Project '%s': task verification cwd: %s", slug, taskWorkingDir)
	}
	for gatewayName, gateway := range project.Gateways {
		if gateway == nil {
			continue
		}
		for srvName, server := range gateway.MCPServers {
			endpoint := fmt.Sprintf("%s/mcp/%s/%s", routePrefix, gatewayName, srvName)
			if server.URL != "" {
				common.LogInfo("  - http://%s:%s%s -> %s", host, port, endpoint, server.URL)
			}
			if server.Command != "" {
				common.LogInfo("  - http://%s:%s%s -> %s -- %s",
					host, port, endpoint, server.Command, strings.Join(server.Args, " "))
			}
		}
	}
}

// handleServerStartCommand handles the server start command.
func handleServerStartCommand(_ context.Context, cmd *cli.Command) error {
	configPath := cmd.String("config-path")

	// Build a file-backed config provider. This is the seam through which the
	// server fetches per-project config on demand; the file provider preloads and
	// serves from memory, so behavior is unchanged.
	if configPath == "" {
		configPath, _ = config.GetConfigPath()
	}
	provider, err := config.NewFileConfigProvider(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config from %s: %w", configPath, err)
	}

	// Read the global settings for the operational validation, logging, and the
	// startup banner below.
	globalConfig, err := provider.Global(context.Background())
	if err != nil {
		return fmt.Errorf("failed to load config from %s: %w", configPath, err)
	}

	// Validating config
	err = config.ValidateConfig(globalConfig, true)
	if err != nil {
		return fmt.Errorf("config validation failed for %s: %w", configPath, err)
	}

	// Initializing internal logger
	if err := common.InitInternalLogger(common.LoggerOptions{
		Level:    globalConfig.Proxy.LogLevel,
		Output:   globalConfig.Proxy.LogOutput,
		FilePath: globalConfig.Proxy.LogFile,
	}); err != nil {
		return fmt.Errorf("failed to initialize internal logger: %w", err)
	}
	defer func() {
		_ = common.CloseLogger()
	}()
	common.LogInfo("Loaded config from: %s", configPath)

	// Create HTTP proxy server from the config provider.
	server, err := proxy.NewCentianServerWithOptions(provider, proxy.CentianServerOptions{})
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
	if err := printServerInfo(globalConfig, server.TaskVerification.WorkingDir); err != nil {
		return err
	}
	go func() {
		if err := server.Server.ListenAndServe(); err != nil {
			errChan <- fmt.Errorf("HTTP proxy server error: %w", err)
		}
	}()

	common.LogInfo("Centian proxy servers started successfully")
	common.LogInfo("Press Ctrl+C to stop")

	// Wait for either signal or server error.
	select {
	case <-sigChan:
		common.LogInfo("Centian received shutdown signal, stopping server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("error during shutdown: %w", err)
		}
		common.LogInfo("Centian server stopped successfully")
		return nil
	case err := <-errChan:
		return err
	}
}
