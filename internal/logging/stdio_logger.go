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

	"github.com/CentianAI/centian-cli/internal/common"
)

// StdioLogEntry represents a single log entry for MCP proxy operations.
// Each entry captures comprehensive information about requests, responses,
// and proxy lifecycle events. Entries are serialized to JSON format for
// structured logging and analysis.
type StdioLogEntry struct {
	BaseLogEntry

	// Command is the executable being proxied (e.g., "npx", "python")
	Command string `json:"command"`

	// Args are the command-line arguments passed to the command
	Args []string `json:"args"`

	// ProjectPath is the working directory where the command executes
	ProjectPath string `json:"project_path"`

	// ConfigSource indicates where configuration originated: "global", "project", or "profile"
	ConfigSource string `json:"config_source"`
}

// StdioLogger wraps Logger with stdio-specific immutable context
type StdioLogger struct {
	*Logger

	// Immutable context (set at initialization)
	command     string
	args        []string
	sessionID   string
	serverID    string
	projectPath string
}

// NewStdioLogger creates a logger with stdio-specific immutable context
func NewStdioLogger(command string, args []string) (*StdioLogger, error) {
	baseLogger, err := NewLogger()
	if err != nil {
		return nil, err
	}

	timestamp := time.Now().UnixNano()
	sessionID := fmt.Sprintf("session_%d", timestamp)
	serverID := fmt.Sprintf("stdio_%s_%d", command, timestamp)

	return &StdioLogger{
		Logger:      baseLogger,
		command:     command,
		args:        args,
		sessionID:   sessionID,
		serverID:    serverID,
		projectPath: common.GetCurrentWorkingDir(),
	}, nil
}

// Getters for immutable context
func (sl *StdioLogger) SessionID() string { return sl.sessionID }
func (sl *StdioLogger) ServerID() string  { return sl.serverID }
func (sl *StdioLogger) Command() string   { return sl.command }
func (sl *StdioLogger) Args() []string    { return sl.args }

func (sl *StdioLogger) CreateLogEntry(requestID, message, errorMsg string, direction McpEventDirection, messagetype McpMessageType, success bool, metadata map[string]string) *StdioLogEntry {
	baseLogEntry := BaseLogEntry{
		Timestamp:   time.Now(),
		RequestID:   requestID,
		SessionID:   sl.sessionID,
		Direction:   direction,
		ServerID:    sl.serverID,
		Transport:   "stdio",
		RawMessage:  message,
		MessageType: messagetype,
		Success:     success,
		Metadata:    metadata,
	}
	return &StdioLogEntry{
		BaseLogEntry: baseLogEntry,
		Command:      sl.command,
		Args:         sl.args,
		ProjectPath:  sl.projectPath,
	}
}

// LogRequest logs an MCP request (uses immutable context)
func (sl *StdioLogger) LogRequest(requestID, message string, metadata map[string]string) error {
	logEntry := sl.CreateLogEntry(
		requestID,
		message,
		"", // errorMsg
		DirectionClientToServer,
		MessageTypeRequest,
		true, // Success
		metadata,
	)
	return sl.logEntry(logEntry)
}

// LogResponse logs an MCP response (uses immutable context)
func (sl *StdioLogger) LogResponse(requestID, message string, success bool, errorMsg string, metadata map[string]string) error {
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
	return sl.logEntry(logEntry)
}

// LogProxyStart logs when the stdio proxy starts (uses immutable context)
func (sl *StdioLogger) LogProxyStart(metadata map[string]string) error {
	requestID := fmt.Sprintf("start_%d", time.Now().UnixNano())
	logEntry := sl.CreateLogEntry(
		requestID,
		"Proxy started",
		"", // errorMsg
		DirectionSystem,
		MessageTypeSystem,
		true, // Success
		nil,
	)
	return sl.logEntry(logEntry)
}

// LogProxyStop logs when the stdio proxy stops (uses immutable context)
func (sl *StdioLogger) LogProxyStop(success bool, errorMsg string, metadata map[string]string) error {
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
