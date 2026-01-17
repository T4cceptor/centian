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

	"github.com/CentianAI/centian-cli/internal/proxy"
	"github.com/urfave/cli/v3"
)

// StdioCommand provides stdio transport proxy functionality.
var StdioCommand = &cli.Command{
	Name:  "stdio",
	Usage: "centian stdio [--cmd <command>] [-- <args...>]",
	Description: `Proxy MCP server using stdio transport.

Examples:
  centian stdio @modelcontextprotocol/server-memory
  centian stdio --cmd npx -- -y @modelcontextprotocol/server-memory
  centian stdio --cmd python -- -m my_mcp_server --config config.json

Note: Use '--' to separate centian flags from command arguments that start with '-'`,
	Action: handleStdioCommand,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "cmd",
			Usage: "Command to execute",
			Value: "npx",
		},
		&cli.StringFlag{
			Name:  "config-path",
			Usage: "Path to config file used for processors",
			// TODO: do we really want to use this as a default?
			// Alternative (current): empty string -> no config is loaded.
			// -> also works, but does not apply processors.
			// Value: "~/.centian/config.jsonc",
		},
	},
	UseShortOptionHandling: true,
}

// handleStdioCommand handles the stdio proxy command.
func handleStdioCommand(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	command := cmd.String("cmd")
	configFile := cmd.String("config-path")

	fmt.Fprintf(os.Stderr, "[CENTIAN] Starting direct MCP proxy: %s %v\n", command, args)

	// Create and start the stdio proxy directly.
	stdioProxy, err := proxy.NewStdioProxy(ctx, command, args, configFile)
	if err != nil {
		return fmt.Errorf("failed to create stdio proxy: %w", err)
	}

	// Set up signal handling for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the proxy.
	if err := stdioProxy.Start(); err != nil {
		return fmt.Errorf("failed to start stdio proxy: %w", err)
	}

	fmt.Fprintf(os.Stderr, "[CENTIAN] MCP proxy started successfully\n")

	// Wait for either signal or proxy to finish.
	go func() {
		<-sigChan
		fmt.Fprintf(os.Stderr, "[CENTIAN] Received shutdown signal, stopping proxy...\n")
		_ = stdioProxy.Stop()
	}()

	// Wait for the proxy to finish.
	err = stdioProxy.Wait()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[CENTIAN] MCP proxy exited with error: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "[CENTIAN] MCP proxy exited successfully\n")
	}

	return err
}
