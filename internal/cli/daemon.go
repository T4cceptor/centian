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
	"github.com/urfave/cli/v3"
)

// DaemonCommand provides daemon management functionality
var DaemonCommand = &cli.Command{
	Name:  "daemon",
	Usage: "Manage the Centian daemon",
	Description: `Manage the persistent Centian daemon process.

The daemon provides improved performance by avoiding process startup overhead
for each MCP request.`,
	Commands: []*cli.Command{
		{
			Name:   "start",
			Usage:  "Start the daemon",
			Action: handleDaemonStart,
		},
		{
			Name:   "stop",
			Usage:  "Stop the daemon",
			Action: handleDaemonStop,
		},
		{
			Name:   "status",
			Usage:  "Show daemon status",
			Action: handleDaemonStatus,
		},
		{
			Name:   "restart",
			Usage:  "Restart the daemon",
			Action: handleDaemonRestart,
		},
	},
}

// handleDaemonStart starts the daemon
func handleDaemonStart(ctx context.Context, cmd *cli.Command) error {
	// Check if daemon is already running
	if daemon.IsDaemonRunning() {
		return fmt.Errorf("daemon is already running")
	}
	
	// Create and start daemon
	d, err := daemon.NewDaemon()
	if err != nil {
		return fmt.Errorf("failed to create daemon: %w", err)
	}
	
	if err := d.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}
	
	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	fmt.Printf("Daemon started on port %d (PID: %d)\n", d.GetPort(), os.Getpid())
	fmt.Println("Press Ctrl+C to stop the daemon")
	
	// Wait for signal
	<-sigChan
	fmt.Println("\nReceived shutdown signal, stopping daemon...")
	
	return d.Stop()
}

// handleDaemonStop stops the daemon
func handleDaemonStop(ctx context.Context, cmd *cli.Command) error {
	client, err := daemon.NewDaemonClient()
	if err != nil {
		return fmt.Errorf("daemon not running: %w", err)
	}
	
	response, err := client.Stop()
	if err != nil {
		return fmt.Errorf("failed to stop daemon: %w", err)
	}
	
	if !response.Success {
		return fmt.Errorf("daemon stop failed: %s", response.Error)
	}
	
	fmt.Println("Daemon stopped successfully")
	return nil
}

// handleDaemonStatus shows daemon status
func handleDaemonStatus(ctx context.Context, cmd *cli.Command) error {
	client, err := daemon.NewDaemonClient()
	if err != nil {
		fmt.Println("Daemon: Not running")
		return nil
	}
	
	response, err := client.Status()
	if err != nil {
		fmt.Printf("Daemon: Error getting status - %v\n", err)
		return nil
	}
	
	if !response.Success {
		fmt.Printf("Daemon: Error - %s\n", response.Error)
		return nil
	}
	
	fmt.Println("Daemon: Running")
	if data := response.Data; data != nil {
		if port, ok := data["port"].(float64); ok {
			fmt.Printf("Port: %d\n", int(port))
		}
		if serverCount, ok := data["server_count"].(float64); ok {
			fmt.Printf("Active servers: %d\n", int(serverCount))
		}
	}
	
	return nil
}

// handleDaemonRestart restarts the daemon
func handleDaemonRestart(ctx context.Context, cmd *cli.Command) error {
	// Try to stop existing daemon
	if daemon.IsDaemonRunning() {
		fmt.Println("Stopping existing daemon...")
		if err := handleDaemonStop(ctx, cmd); err != nil {
			fmt.Printf("Warning: failed to stop existing daemon: %v\n", err)
		}
	}
	
	// Start new daemon
	fmt.Println("Starting daemon...")
	return handleDaemonStart(ctx, cmd)
}