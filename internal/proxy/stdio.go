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
	"syscall"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
	"github.com/CentianAI/centian-cli/internal/config"
	"github.com/CentianAI/centian-cli/internal/logging"
	"github.com/CentianAI/centian-cli/internal/processor"
)

// StdioProxy represents a proxy for MCP servers using stdio transport.
// It manages the lifecycle of an MCP server process and handles bidirectional
// communication between the client and server.
type StdioProxy struct {
	config *config.GlobalConfig

	// cmd is the command being executed for the MCP server
	cmd *exec.Cmd

	// stdin is the pipe to write data to the MCP server's stdin
	stdin io.WriteCloser

	// stdout is the pipe to read data from the MCP server's stdout
	stdout io.ReadCloser

	// stderr is the pipe to read data from the MCP server's stderr
	stderr io.ReadCloser

	// running indicates whether the proxy is currently active
	running bool

	// mu provides thread-safe access to the running state
	mu sync.RWMutex

	// wg tracks active I/O handler goroutines to ensure clean shutdown
	wg sync.WaitGroup

	// ctx manages the proxy lifecycle and cancellation
	ctx context.Context

	// cancel stops the proxy by canceling the context
	cancel context.CancelFunc

	// logger records proxy activity for debugging and monitoring
	logger *logging.Logger

	// sessionID is a unique identifier for this proxy session (format: "session_<timestamp>")
	sessionID string

	// serverID is a unique identifier for the MCP server instance (format: "stdio_<command>_<timestamp>")
	serverID string

	// command is the executable name being run (e.g., "npx", "python")
	command string

	// args are the arguments passed to the command
	args []string

	processor *EventProcessor
}

// NewStdioProxy creates a new stdio proxy for the given command and arguments.
// To enable processors, call SetProcessors after creation.
func NewStdioProxy(ctx context.Context, command string, args []string, pathToConfig string) (*StdioProxy, error) {
	proxyCtx, cancel := context.WithCancel(ctx)
	var globalConfig *config.GlobalConfig
	if pathToConfig != "" {
		var err error
		globalConfig, err = config.LoadConfigFromPath(pathToConfig)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to load config: %s - %w", pathToConfig, err)
		}
	}

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

	// Create logger
	logger, err := logging.NewLogger()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	timestamp := time.Now().UnixNano()
	sessionID := fmt.Sprintf("session_%d", timestamp)
	serverID := fmt.Sprintf("stdio_%s_%d", command, timestamp)

	proxy := StdioProxy{
		config:    nil,
		cmd:       cmd,
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
		running:   false,
		ctx:       proxyCtx,
		cancel:    cancel,
		logger:    logger,
		sessionID: sessionID,
		serverID:  serverID,
		command:   command,
		args:      args,
		processor: nil,
	}

	// Create and attach processors
	if globalConfig != nil {
		proxy.config = globalConfig
		processors, err := processor.NewChain(globalConfig.Processors, globalConfig.Name, sessionID)
		if err == nil {
			proxy.processor = NewEventProcessor(logger, processors)
		}
	}

	return &proxy, nil
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

	// Log proxy start
	if p.logger != nil {
		p.logSystemMessage(fmt.Sprintf("Stdio proxy started: %s %v", p.command, p.args))
	}

	// Start goroutines to handle I/O with WaitGroup tracking
	p.wg.Add(3)
	go func() { defer p.wg.Done(); p.handleStdout() }()
	go func() { defer p.wg.Done(); p.handleStderr() }()
	go func() { defer p.wg.Done(); p.handleStdin() }()

	return nil
}

// Stop stops the MCP server process and proxy
func (p *StdioProxy) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	// Cancel context to signal goroutines to stop
	p.cancel()
	p.running = false

	// Close stdin pipe to signal no more input
	if p.stdin != nil {
		_ = p.stdin.Close()
	}

	// Attempt graceful shutdown with SIGTERM first
	if p.cmd.Process != nil {
		// Send SIGTERM for graceful shutdown
		if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			// If SIGTERM fails, process might already be dead
			fmt.Fprintf(os.Stderr, "Failed to send SIGTERM: %v\n", err)
		}

		// Give process time to exit gracefully after SIGTERM
		time.Sleep(5 * time.Second)

		// If still running, escalate to SIGKILL
		// Note: We check the process, not call Wait() to avoid race with monitoring goroutine
		if p.cmd.Process != nil {
			// Attempt SIGKILL if process is still alive
			if err := p.cmd.Process.Signal(syscall.Signal(0)); err == nil {
				// Process is still alive, send SIGKILL
				fmt.Fprintf(os.Stderr, "Process did not exit gracefully, sending SIGKILL\n")
				_ = p.cmd.Process.Kill()
			}
		}
	}

	// Wait for all I/O handler goroutines to finish
	p.wg.Wait()

	// Now safe to close logger after all goroutines have finished
	if p.logger != nil {
		p.logSystemMessage(fmt.Sprintf("Stdio proxy stopped: %s %v", p.command, p.args))
		_ = p.logger.Close()
	}

	return nil
}

func (p *StdioProxy) logSystemMessage(message string) {
	requestID := fmt.Sprintf("system_event_%d", time.Now().UnixNano())
	baseEvent := common.BaseMcpEvent{
		Timestamp:        time.Now(),
		SessionID:        p.sessionID,
		ServerID:         p.serverID,
		Transport:        "stdio",
		RequestID:        requestID,
		Direction:        common.DirectionSystem,
		MessageType:      common.MessageTypeSystem,
		Error:            "",
		Success:          true,
		Metadata:         nil,
		ProcessingErrors: make(map[string]error),
	}
	mcpEvent := &common.StdioMcpEvent{
		BaseMcpEvent: baseEvent,
		Command:      p.command,
		Args:         p.args,
		ProjectPath:  p.cmd.Path,
		ConfigSource: "project",
		Message:      message,
	}
	if err := p.logger.LogMcpEvent(mcpEvent); err != nil {
		common.LogError(err.Error())
	}
}

// GetEvent returns a new StdioMcpEvent for the given parameters
func (p *StdioProxy) GetEvent(
	message, requestID string,
	direction common.McpEventDirection,
	messageType common.McpMessageType,
) common.StdioMcpEvent {
	baseMcpEvent := common.BaseMcpEvent{
		Timestamp:        time.Now(),
		Transport:        "stdio",
		RequestID:        requestID,
		SessionID:        p.sessionID,
		ServerID:         p.serverID,
		Direction:        direction,
		MessageType:      messageType,
		Success:          true, // TODO: this is not necessarily the case - but how do we know if its successful or not?
		Error:            "",
		ProcessingErrors: map[string]error{},
		Metadata:         map[string]string{}, // TODO
		Modified:         false,
	}
	return common.StdioMcpEvent{
		BaseMcpEvent: baseMcpEvent,
		Command:      p.command,
		Args:         p.args,
		ProjectPath:  p.cmd.Path,
		ConfigSource: "project", // TODO
		Message:      message,
	}
}

// handleStdout reads from MCP server stdout and forwards to our stdout
func (p *StdioProxy) handleStdout() {
	defer func() {
		if p.stdout != nil {
			_ = p.stdout.Close()
		}
	}()

	scanner := bufio.NewScanner(p.stdout)
	for scanner.Scan() {
		select {
		case <-p.ctx.Done():
			return
		default:
			line := scanner.Text()
			requestID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
			event := p.GetEvent(line, requestID, common.DirectionServerToClient, common.MessageTypeResponse)
			fmt.Fprintf(os.Stderr, "[SERVER->CLIENT] %s\n", line)

			// Execute processor chain if configured
			if p.processor != nil {
				if err := p.processor.Process(&event); err != nil {
					common.LogError(err.Error())
					event.ProcessingErrors["processing_error"] = err
				}
			}

			// Forward to client
			fmt.Println(event.RawMessage())
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
			_ = p.stderr.Close()
		}
	}()

	scanner := bufio.NewScanner(p.stderr)
	for scanner.Scan() {
		select {
		case <-p.ctx.Done():
			return
		default:
			line := scanner.Text()
			fmt.Fprintf(os.Stderr, "[SERVER-STDERR] %s\n", line)
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
			_ = p.stdin.Close()
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		select {
		case <-p.ctx.Done():
			return
		default:
			line := scanner.Text()
			requestID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
			event := p.GetEvent(line, requestID, common.DirectionClientToServer, common.MessageTypeRequest)
			fmt.Fprintf(os.Stderr, "[CLIENT->SERVER] %s\n", line)

			// Execute processor chain if configured
			if p.processor != nil {
				if err := p.processor.Process(&event); err != nil {
					common.LogError(err.Error())
					event.ProcessingErrors["processing_error"] = err
				}
			}

			// Forward to MCP server
			// TODO: proceed depending on event status!
			if event.Status > 299 {
				// If status indicates an error we return to the client immediately
				// TODO: log this?
				fmt.Println(event.RawMessage())
				return
			}
			if p.stdin != nil {
				if _, err := fmt.Fprintln(p.stdin, event.RawMessage()); err != nil {
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
