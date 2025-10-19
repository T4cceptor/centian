// Copyright 2025 CentianCLI Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
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

	"github.com/CentianAI/centian-cli/internal/daemon"
	"github.com/CentianAI/centian-cli/internal/proxy"
	"github.com/urfave/cli/v3"
)

// StdioCommand provides stdio transport proxy functionality
var StdioCommand = &cli.Command{
	Name:      "stdio",
	Usage:     "centian stdio [--cmd <command>] <args...>",
	Description: `Proxy MCP server using stdio transport.

Examples:
  centian stdio @modelcontextprotocol/server-memory
  centian stdio --cmd npx @modelcontextprotocol/server-memory  
  centian stdio --cmd python -m my_mcp_server --config config.json
  centian stdio --cmd cat`,
	Action: handleStdioCommand,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "cmd",
			Usage:   "Command to execute (defaults to 'npx')",
			Value:   "npx",
		},
	},
}

// handleStdioCommand handles the stdio proxy command
func handleStdioCommand(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	
	// Get command from flag or default
	command := cmd.String("cmd")
	
	// For commands like "cat" that don't need args, this is fine
	// Only require args if no --cmd was explicitly provided AND no args given
	if len(args) == 0 && command == "npx" {
		return fmt.Errorf("no MCP server arguments provided\n\nUsage: %s", cmd.Usage)
	}
	
	// Parse command and arguments
	var cmdArgs []string
	
	// Check if --cmd was explicitly used in the raw arguments
	rawArgs := os.Args[2:] // Skip "centian stdio"
	cmdFlagUsed := false
	
	for i, arg := range rawArgs {
		if arg == "--cmd" {
			cmdFlagUsed = true
			// Skip --cmd and the command itself, use remaining args
			if i+2 < len(rawArgs) {
				cmdArgs = rawArgs[i+2:]
			}
			break
		}
	}
	
	// If --cmd flag wasn't used, all args go to the MCP server
	if !cmdFlagUsed {
		cmdArgs = args
	}
	
	// Check if daemon is running and use it if available
	if daemon.IsDaemonRunning() {
		fmt.Fprintf(os.Stderr, "[CENTIAN] Using daemon for MCP proxy: %s %v\n", command, cmdArgs)
		return useDaemonProxy(ctx, command, cmdArgs)
	}
	
	fmt.Fprintf(os.Stderr, "[CENTIAN] Starting direct MCP proxy: %s %v\n", command, cmdArgs)
	
	// Create and start the stdio proxy directly
	stdioProxy, err := proxy.NewStdioProxy(ctx, command, cmdArgs)
	if err != nil {
		return fmt.Errorf("failed to create stdio proxy: %w", err)
	}
	
	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	// Start the proxy
	if err := stdioProxy.Start(); err != nil {
		return fmt.Errorf("failed to start stdio proxy: %w", err)
	}
	
	fmt.Fprintf(os.Stderr, "[CENTIAN] MCP proxy started successfully\n")
	
	// Wait for either signal or proxy to finish
	go func() {
		<-sigChan
		fmt.Fprintf(os.Stderr, "[CENTIAN] Received shutdown signal, stopping proxy...\n")
		stdioProxy.Stop()
	}()
	
	// Wait for the proxy to finish
	err = stdioProxy.Wait()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[CENTIAN] MCP proxy exited with error: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "[CENTIAN] MCP proxy exited successfully\n")
	}
	
	return err
}

// useDaemonProxy uses the daemon to handle the MCP proxy
func useDaemonProxy(ctx context.Context, command string, args []string) error {
	client, err := daemon.NewDaemonClient()
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}
	
	response, err := client.StartStdioProxy(command, args)
	if err != nil {
		return fmt.Errorf("failed to start stdio proxy via daemon: %w", err)
	}
	
	if !response.Success {
		return fmt.Errorf("daemon failed to start stdio proxy: %s", response.Error)
	}
	
	fmt.Fprintf(os.Stderr, "[CENTIAN] MCP proxy started via daemon (Server ID: %s)\n", response.ServerID)
	
	// For now, just return success. In a full implementation, we would
	// set up bidirectional communication with the daemon-managed server
	return nil
}