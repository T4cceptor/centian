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

package logging

import (
	"fmt"
	"time"
)

// HttpLogEntry represents a single log entry for HTTP MCP proxy operations.
// Each entry captures comprehensive information about requests, responses,
// and proxy lifecycle events. Entries are serialized to JSON format for
// structured logging and analysis.
type HttpLogEntry struct {
	BaseLogEntry

	// Gateway is the logical grouping of MCP servers (e.g., "my-gateway")
	Gateway string `json:"gateway"`

	// ServerName identifies the specific MCP server within the gateway
	ServerName string `json:"server_name"`

	// Endpoint is the HTTP path this server is mounted at (e.g., "/mcp/my-gateway/github")
	Endpoint string `json:"endpoint"`

	// DownstreamURL is the target MCP server URL being proxied to
	DownstreamURL string `json:"downstream_url"`

	// Transport identifies the protocol (always "http" for this entry type)
	Transport string `json:"transport"`

	// ProxyPort is the port the proxy server is listening on
	ProxyPort string `json:"proxy_port,omitempty"`
}

// HttpLogger wraps Logger with HTTP endpoint-specific immutable context.
// Multiple HttpLoggers can share the same underlying *Logger (file handle)
// while maintaining independent endpoint contexts.
type HttpLogger struct {
	*Logger // Shared file handle across all HttpLoggers from same base

	// Immutable context per endpoint (set at initialization)
	sessionID     string
	transport     string
	port          string
	gatewayName   string
	serverName    string
	endpoint      string
	downstreamURL string
}

// NewHttpLogger creates a logger for a specific HTTP endpoint.
// All HttpLoggers created from the same baseLogger share the file handle.
//
// Parameters:
//   - baseLogger: Shared Logger instance (file handle)
//   - port: Proxy server port (e.g., "8080")
//   - gatewayName: Gateway identifier (e.g., "my-gateway")
//   - serverName: MCP server identifier (e.g., "github")
//   - endpoint: HTTP endpoint path (e.g., "/mcp/my-gateway/github")
//   - downstreamURL: Target MCP server URL (e.g., "https://api.github.com/mcp")
func NewHttpLogger(baseLogger *Logger, port, gatewayName, serverName, endpoint, downstreamURL string) *HttpLogger {
	timestamp := time.Now().UnixNano()
	sessionID := fmt.Sprintf("http_endpoint_%s_%d", endpoint, timestamp)

	return &HttpLogger{
		Logger:        baseLogger,
		sessionID:     sessionID,
		transport:     "http",
		port:          port,
		gatewayName:   gatewayName,
		serverName:    serverName,
		endpoint:      endpoint,
		downstreamURL: downstreamURL,
	}
}

// Getters for immutable context
func (hl *HttpLogger) SessionID() string { return hl.sessionID }
func (hl *HttpLogger) ServerID() string {
	return fmt.Sprintf("http_%s_%s", hl.gatewayName, hl.serverName)
}
func (hl *HttpLogger) Port() string          { return hl.port }
func (hl *HttpLogger) GatewayName() string   { return hl.gatewayName }
func (hl *HttpLogger) ServerName() string    { return hl.serverName }
func (hl *HttpLogger) Endpoint() string      { return hl.endpoint }
func (hl *HttpLogger) DownstreamURL() string { return hl.downstreamURL }

func (hl *HttpLogger) CreateLogEntry(requestID, message, errorMsg string, direction McpEventDirection, messagetype McpMessageType, success bool, metadata map[string]string) *HttpLogEntry {
	baseLogEntry := BaseLogEntry{
		Timestamp:   time.Now(),
		SessionID:   hl.sessionID,
		ServerID:    hl.ServerID(),
		Transport:   "http",
		RequestID:   requestID,
		Direction:   direction,
		RawMessage:  message,
		MessageType: messagetype,
		Error:       errorMsg,
		Success:     success,
		Metadata:    metadata,
	}
	return &HttpLogEntry{
		BaseLogEntry:  baseLogEntry,
		Gateway:       hl.gatewayName,
		ServerName:    hl.serverName,
		Endpoint:      hl.endpoint,
		DownstreamURL: hl.downstreamURL,
		Transport:     hl.transport,
		ProxyPort:     hl.port,
	}
}

// LogRequest logs an HTTP MCP request (uses immutable context)
func (hl *HttpLogger) LogRequest(requestID, message string, metadata map[string]string) error {
	entry := hl.CreateLogEntry(
		requestID,
		message,
		"", // errMsg
		DirectionClientToServer,
		MessageTypeRequest,
		true, // success
		metadata,
	)
	return hl.logEntry(entry)
}

// LogResponse logs an HTTP MCP response (uses immutable context)
func (hl *HttpLogger) LogResponse(requestID, message string, success bool, errorMsg string, metadata map[string]string) error {
	entry := hl.CreateLogEntry(
		requestID,
		message,
		errorMsg, // errMsg
		DirectionServerToClient,
		MessageTypeResponse,
		success, // success
		metadata,
	)
	return hl.logEntry(entry)
}

// LogProxyStart logs when the HTTP endpoint starts (uses immutable context)
func (hl *HttpLogger) LogProxyStart(metadata map[string]string) error {
	// TODO: offer metadata to be provided here
	requestID := fmt.Sprintf("start_%d", time.Now().UnixNano())
	message := fmt.Sprintf("HTTP endpoint started: %s -> %s", hl.endpoint, hl.downstreamURL)
	entry := hl.CreateLogEntry(
		requestID,
		message,
		"", // errMsg
		DirectionSystem,
		MessageTypeSystem,
		true, // success
		nil,
	)
	return hl.logEntry(entry)
}

// LogProxyStop logs when the HTTP endpoint stops (uses immutable context)
func (hl *HttpLogger) LogProxyStop(success bool, errorMsg string, metadata map[string]string) error {
	// TODO: offer metadata to be provided here
	requestID := fmt.Sprintf("start_%d", time.Now().UnixNano())
	message := fmt.Sprintf("HTTP endpoint stopped: %s", hl.endpoint)
	entry := hl.CreateLogEntry(
		requestID,
		message,
		errorMsg, // errMsg
		DirectionSystem,
		MessageTypeSystem,
		success, // success
		nil,
	)
	return hl.logEntry(entry)
}
