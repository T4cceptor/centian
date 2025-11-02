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
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// DaemonClient represents a client for communicating with the daemon
type DaemonClient struct {
	port    int
	timeout time.Duration
}

// NewDaemonClient creates a new daemon client
// Uses the default daemon port (can be made configurable in the future)
func NewDaemonClient() (*DaemonClient, error) {
	return &DaemonClient{
		port:    DefaultDaemonPort, // Use fixed port from daemon.go
		timeout: 30 * time.Second,
	}, nil
}

// IsDaemonRunning checks if the daemon is running by attempting to connect
func IsDaemonRunning() bool {
	// Try to connect to the daemon port
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", DefaultDaemonPort), 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// SendRequest sends a request to the daemon
func (c *DaemonClient) SendRequest(req *Request) (*Response, error) {
	// Connect to daemon
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", c.port), c.timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer conn.Close()

	// Set timeout
	conn.SetDeadline(time.Now().Add(c.timeout))

	// Send request
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	decoder := json.NewDecoder(conn)
	var response Response
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return &response, nil
}

// StartStdioProxy starts a stdio proxy via the daemon
func (c *DaemonClient) StartStdioProxy(command string, args []string) (*Response, error) {
	req := &Request{
		Type:    "stdio",
		Command: command,
		Args:    args,
		ID:      fmt.Sprintf("stdio_%d", time.Now().UnixNano()),
	}

	return c.SendRequest(req)
}

// Status gets the daemon status
func (c *DaemonClient) Status() (*Response, error) {
	req := &Request{
		Type: "status",
		ID:   fmt.Sprintf("status_%d", time.Now().UnixNano()),
	}

	return c.SendRequest(req)
}

// Stop stops the daemon
func (c *DaemonClient) Stop() (*Response, error) {
	req := &Request{
		Type: "stop",
		ID:   fmt.Sprintf("stop_%d", time.Now().UnixNano()),
	}

	return c.SendRequest(req)
}
