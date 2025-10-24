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

package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/CentianAI/centian-cli/internal/proxy"
)

// Daemon represents the persistent MCP proxy daemon
type Daemon struct {
	listener    net.Listener
	servers     map[string]*proxy.StdioProxy
	serversMu   sync.RWMutex
	port        int
	running     bool
	runningMu   sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// DaemonRequest represents a request to the daemon
type DaemonRequest struct {
	Type      string            `json:"type"`       // "stdio", "status", "stop"
	Command   string            `json:"command"`    // Command to execute
	Args      []string          `json:"args"`       // Command arguments
	ID        string            `json:"id"`         // Unique request ID
	Metadata  map[string]string `json:"metadata"`   // Additional metadata
}

// DaemonResponse represents a response from the daemon
type DaemonResponse struct {
	Success   bool              `json:"success"`
	ServerID  string            `json:"server_id,omitempty"`
	Port      int               `json:"port,omitempty"`
	Error     string            `json:"error,omitempty"`
	Data      map[string]any    `json:"data,omitempty"`
}

// DefaultDaemonPort is the default port for the daemon
// This can be made configurable in the future via global config
const DefaultDaemonPort = 50051

// NewDaemon creates a new daemon instance
func NewDaemon() (*Daemon, error) {
	ctx, cancel := context.WithCancel(context.Background())

	return &Daemon{
		servers:  make(map[string]*proxy.StdioProxy),
		port:     DefaultDaemonPort, // TODO: Make configurable via global config
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// Start starts the daemon
func (d *Daemon) Start() error {
	d.runningMu.Lock()
	defer d.runningMu.Unlock()

	if d.running {
		return fmt.Errorf("daemon already running")
	}

	// ATOMIC: Bind to TCP port - only one process can succeed
	// This prevents race condition by using kernel-level atomic bind()
	addr := fmt.Sprintf("127.0.0.1:%d", d.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		// Check if port is already in use (another daemon running)
		if isAddressInUse(err) {
			return fmt.Errorf("daemon already running on port %d", d.port)
		}
		return fmt.Errorf("failed to bind to %s: %w", addr, err)
	}

	d.listener = listener
	d.running = true

	fmt.Printf("Daemon started on port %d\n", d.port)

	// Start accepting connections
	go d.acceptConnections()

	return nil
}

// isAddressInUse checks if the error indicates address is already in use
func isAddressInUse(err error) bool {
	if err == nil {
		return false
	}
	// Check for both "address already in use" and "bind: address already in use"
	errStr := err.Error()
	return contains(errStr, "address already in use") ||
	       contains(errStr, "bind: address already in use")
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
	       (s == substr || len(s) > len(substr) &&
	        (s[:len(substr)] == substr ||
	         s[len(s)-len(substr):] == substr ||
	         findSubstring(s, substr)))
}

// findSubstring finds a substring in a string
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Stop stops the daemon
func (d *Daemon) Stop() error {
	d.runningMu.Lock()
	defer d.runningMu.Unlock()

	if !d.running {
		return nil
	}

	d.cancel()
	d.running = false

	// Stop all servers
	d.serversMu.Lock()
	for _, server := range d.servers {
		server.Stop()
	}
	d.serversMu.Unlock()

	// Close listener (releases port binding)
	if d.listener != nil {
		d.listener.Close()
	}

	fmt.Println("Daemon stopped")
	return nil
}

// IsRunning returns whether the daemon is running
func (d *Daemon) IsRunning() bool {
	d.runningMu.RLock()
	defer d.runningMu.RUnlock()
	return d.running
}

// GetPort returns the daemon's listening port
func (d *Daemon) GetPort() int {
	return d.port
}

// acceptConnections accepts and handles incoming connections
func (d *Daemon) acceptConnections() {
	for {
		select {
		case <-d.ctx.Done():
			return
		default:
			conn, err := d.listener.Accept()
			if err != nil {
				if d.IsRunning() {
					fmt.Printf("Error accepting connection: %v\n", err)
				}
				continue
			}
			
			go d.handleConnection(conn)
		}
	}
}

// handleConnection handles a single client connection
func (d *Daemon) handleConnection(conn net.Conn) {
	defer conn.Close()
	
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)
	
	var req DaemonRequest
	if err := decoder.Decode(&req); err != nil {
		response := DaemonResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to decode request: %v", err),
		}
		encoder.Encode(response)
		return
	}
	
	response := d.handleRequest(&req)
	encoder.Encode(response)
}

// handleRequest processes a daemon request
func (d *Daemon) handleRequest(req *DaemonRequest) DaemonResponse {
	switch req.Type {
	case "stdio":
		return d.handleStdioRequest(req)
	case "status":
		return d.handleStatusRequest(req)
	case "stop":
		return d.handleStopRequest(req)
	default:
		return DaemonResponse{
			Success: false,
			Error:   fmt.Sprintf("unknown request type: %s", req.Type),
		}
	}
}

// handleStdioRequest handles a stdio proxy request
func (d *Daemon) handleStdioRequest(req *DaemonRequest) DaemonResponse {
	serverID := fmt.Sprintf("stdio_%s_%d", req.Command, time.Now().UnixNano())
	
	// Create stdio proxy
	stdioProxy, err := proxy.NewStdioProxy(d.ctx, req.Command, req.Args)
	if err != nil {
		return DaemonResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to create stdio proxy: %v", err),
		}
	}
	
	// Start the proxy
	if err := stdioProxy.Start(); err != nil {
		return DaemonResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to start stdio proxy: %v", err),
		}
	}
	
	// Store the server
	d.serversMu.Lock()
	d.servers[serverID] = stdioProxy
	d.serversMu.Unlock()
	
	// Monitor the server and clean up when it stops
	go func() {
		stdioProxy.Wait()
		d.serversMu.Lock()
		delete(d.servers, serverID)
		d.serversMu.Unlock()
	}()
	
	return DaemonResponse{
		Success:  true,
		ServerID: serverID,
		Data: map[string]any{
			"command": req.Command,
			"args":    req.Args,
		},
	}
}

// handleStatusRequest handles a status request
func (d *Daemon) handleStatusRequest(req *DaemonRequest) DaemonResponse {
	d.serversMu.RLock()
	serverCount := len(d.servers)
	d.serversMu.RUnlock()
	
	return DaemonResponse{
		Success: true,
		Data: map[string]any{
			"running":      d.IsRunning(),
			"port":         d.port,
			"server_count": serverCount,
			"uptime":       time.Since(time.Unix(0, 0)), // TODO: track actual start time
		},
	}
}

// handleStopRequest handles a stop request
func (d *Daemon) handleStopRequest(req *DaemonRequest) DaemonResponse {
	go func() {
		time.Sleep(100 * time.Millisecond) // Give time to send response
		d.Stop()
	}()
	
	return DaemonResponse{
		Success: true,
		Data: map[string]any{
			"message": "daemon stopping",
		},
	}
}