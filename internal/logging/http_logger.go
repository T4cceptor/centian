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
	"net/http"
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

// StdioLogger wraps Logger with stdio-specific immutable context
type HttpLogger struct {
	*Logger // Note: this is the same Logger instance for ALL HttpLoggers of a single server

	// Immutable context per endpoint
	sessionID     string
	port          string
	gatewayName   string
	serverName    string
	endpoint      string
	downstreamURL string
}

// NewStdioLogger creates a logger with stdio-specific immutable context
func NewHttpLogger(URL string, Headers map[string]string) (*HttpLogger, error) {
	baseLogger, err := NewLogger()
	if err != nil {
		return nil, err
	}

	timestamp := time.Now().UnixNano()
	sessionID := fmt.Sprintf("session_%d", timestamp)

	return &HttpLogger{
		Logger:        baseLogger,
		downstreamURL: URL,
		sessionID:     sessionID,
	}, nil
}

// Getters for immutable context
func (sl *HttpLogger) SessionID() string { return sl.sessionID }
func (sl *HttpLogger) Url() string       { return sl.downstreamURL }

func (sl *HttpLogger) CreateLogEntry(requestID, message, errorMsg string, direction McpEventDirection, messagetype McpMessageType, success bool, metadata map[string]string) *HttpLogEntry {
	baseLogEntry := BaseLogEntry{
		Timestamp:   time.Now(),
		RequestID:   requestID,
		SessionID:   sl.sessionID,
		Direction:   direction,
		Transport:   "http",
		RawMessage:  message,
		MessageType: messagetype,
		Success:     success,
		Metadata:    metadata,
	}
	return &HttpLogEntry{
		BaseLogEntry: baseLogEntry,
		Url:          sl.downstreamURL,
	}
}

// LogRequest logs an MCP request (uses immutable context)
func (sl *HttpLogger) LogRequest(r http.Request, requestID, message string, metadata map[string]string) error {
	logEntry := sl.CreateLogEntry(
		requestID,
		message,
		"", // errorMsg
		DirectionClientToServer,
		MessageTypeRequest,
		true, // Success
		metadata,
	)
	// TODO: transform raw http request "r" into JSON and attach to logEntry
	return sl.logEntry(logEntry)
}

// LogResponse logs an MCP response (uses immutable context)
func (sl *HttpLogger) LogResponse(r http.Response, requestID, message string, success bool, errorMsg string, metadata map[string]string) error {
	logEntry := sl.CreateLogEntry(
		requestID,
		message,
		errorMsg, // errorMsg
		DirectionServerToClient,
		MessageTypeResponse,
		success, // Success
		metadata,
	)
	if !success {
		logEntry.MessageType = "error"
	}
	// TODO: transform raw http response "r" into JSON and attach to logEntry
	return sl.logEntry(logEntry)
}

// LogProxyStart logs when the stdio proxy starts (uses immutable context)
func (sl *HttpLogger) LogProxyStart() error {
	requestID := fmt.Sprintf("start_%d", time.Now().UnixNano())
	logEntry := sl.CreateLogEntry(
		requestID,
		"Proxy started", // TODO: should we log all servers/endpoints we are proxying here?
		"",              // errorMsg
		DirectionSystem,
		MessageTypeSystem,
		true, // Success
		nil,
	)
	return sl.logEntry(logEntry)
}

// LogProxyStop logs when the stdio proxy stops (uses immutable context)
func (sl *HttpLogger) LogProxyStop(success bool, errorMsg string) error {
	requestID := fmt.Sprintf("stop_%d", time.Now().UnixNano())
	logEntry := sl.CreateLogEntry(
		requestID,
		"Proxy stopped",
		"", // errorMsg
		DirectionSystem,
		MessageTypeSystem,
		true, // Success
		nil,
	)
	return sl.logEntry(logEntry)
}
