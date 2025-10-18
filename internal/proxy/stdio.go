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

package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/CentianAI/centian-cli/internal/logging"
)

// StdioProxy represents a proxy for MCP servers using stdio transport
type StdioProxy struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	running bool
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewStdioProxy creates a new stdio proxy for the given command and arguments
func NewStdioProxy(ctx context.Context, command string, args []string) (*StdioProxy, error) {
	proxyCtx, cancel := context.WithCancel(ctx)

	// Create the command
	cmd := exec.CommandContext(proxyCtx, command, args...)

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	return &StdioProxy{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		running: false,
		ctx:     proxyCtx,
		cancel:  cancel,
	}, nil
}

// Start starts the MCP server process and begins proxying
func (p *StdioProxy) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("proxy already running")
	}

	// Start the MCP server process
	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start MCP server: %w", err)
	}

	p.running = true

	// Start goroutines to handle I/O
	go p.handleStdout()
	go p.handleStderr()
	go p.handleStdin()

	return nil
}

// Stop stops the MCP server process and proxy
func (p *StdioProxy) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.cancel()
	p.running = false

	// Close pipes
	if p.stdin != nil {
		p.stdin.Close()
	}

	// Wait for process to finish or kill it
	if p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}

	return nil
}

// handleStdout reads from MCP server stdout and forwards to our stdout
func (p *StdioProxy) handleStdout() {
	defer func() {
		if p.stdout != nil {
			p.stdout.Close()
		}
	}()

	scanner := bufio.NewScanner(p.stdout)
	for scanner.Scan() {
		select {
		case <-p.ctx.Done():
			return
		default:
			line := scanner.Text()

			// Log the MCP response (for now just to stderr for debugging)
			fmt.Fprintf(os.Stderr, "[SERVER->CLIENT] %s\n", line)

			// Forward to client
			fmt.Println(line)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading from MCP server stdout: %v\n", err)
	}
}

// handleStderr reads from MCP server stderr and forwards to our stderr
func (p *StdioProxy) handleStderr() {
	defer func() {
		if p.stderr != nil {
			p.stderr.Close()
		}
	}()

	scanner := bufio.NewScanner(p.stderr)
	for scanner.Scan() {
		select {
		case <-p.ctx.Done():
			return
		default:
			line := scanner.Text()
			fmt.Fprintf(os.Stderr, "[MCP-STDERR] %s\n", line)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading from MCP server stderr: %v\n", err)
	}
}

// handleStdin reads from our stdin and forwards to MCP server
func (p *StdioProxy) handleStdin() {
	defer func() {
		if p.stdin != nil {
			p.stdin.Close()
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		select {
		case <-p.ctx.Done():
			return
		default:
			line := scanner.Text()

			// Log the client request (for now just to stderr for debugging)
			fmt.Fprintf(os.Stderr, "[CLIENT->SERVER] %s\n", line)

			// Forward to MCP server
			if p.stdin != nil {
				if _, err := fmt.Fprintln(p.stdin, line); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing to MCP server stdin: %v\n", err)
					return
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading from stdin: %v\n", err)
	}
}

// IsRunning returns whether the proxy is currently running
func (p *StdioProxy) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

// Wait waits for the MCP server process to finish
func (p *StdioProxy) Wait() error {
	return p.cmd.Wait()
}

// ParseCommand parses a command string with the --cmd flag
func ParseCommand(args []string) (string, []string, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("no arguments provided")
	}

	// Check for --cmd flag
	cmdIndex := -1
	for i, arg := range args {
		if arg == "--cmd" {
			cmdIndex = i
			break
		}
	}

	var command string
	var cmdArgs []string

	if cmdIndex >= 0 {
		// --cmd flag found
		if cmdIndex+1 >= len(args) {
			return "", nil, fmt.Errorf("--cmd flag requires a command argument")
		}

		command = args[cmdIndex+1]

		// Collect remaining args after --cmd <command>
		if cmdIndex+2 < len(args) {
			cmdArgs = args[cmdIndex+2:]
		}
	} else {
		// No --cmd flag, default to npx
		command = "npx"
		cmdArgs = args
	}

	return command, cmdArgs, nil
}
