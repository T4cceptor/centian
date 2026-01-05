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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// Logger handles logging of MCP proxy operations
type Logger struct {
	logFile *os.File
	logPath string
}

// NewLogger creates a new logger instance
func NewLogger() (*Logger, error) {
	// Resolve logs directory location
	logsDir, err := GetLogsDirectory()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Create log file with current date
	logFileName := fmt.Sprintf("requests_%s.jsonl", time.Now().Format("2006-01-02"))
	logPath := filepath.Join(logsDir, logFileName)

	// Open log file in append mode
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return &Logger{
		logFile: logFile,
		logPath: logPath,
	}, nil
}

// Close closes the logger
func (l *Logger) Close() error {
	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
}

// LogRequest logs an MCP request
func (l *Logger) LogRequest(requestID, sessionID, command string, args []string, serverID, message string) error {
	baseLogEntry := BaseLogEntry{
		Timestamp:   time.Now(),
		RequestID:   requestID,
		SessionID:   sessionID,
		Direction:   "request",
		ServerID:    serverID,
		RawMessage:  message,
		MessageType: "request",
		Success:     true,
		// TODO: JsonRpcMessage
	}
	return l.logEntry(&StdioLogEntry{
		BaseLogEntry: baseLogEntry,
		Command:      command,
		Args:         args,
		ProjectPath:  common.GetCurrentWorkingDir(),
	})
}

// LogResponse logs an MCP response
func (l *Logger) LogResponse(requestID, sessionID, command string, args []string, serverID, message string, success bool, errorMsg string) error {
	baseLogEntry := BaseLogEntry{
		Timestamp:   time.Now(),
		RequestID:   requestID,
		SessionID:   sessionID,
		Direction:   "response",
		ServerID:    serverID,
		RawMessage:  message,
		MessageType: "response",
		Success:     success,
		Error:       errorMsg,
		// TODO: JsonRpcMessage
	}
	entry := StdioLogEntry{
		BaseLogEntry: baseLogEntry,
		Command:      command,
		Args:         args,
		ProjectPath:  common.GetCurrentWorkingDir(),
	}

	if !success {
		entry.MessageType = "error"
	}

	return l.logEntry(&entry)
}

// LogProxyStart logs when a proxy starts
func (l *Logger) LogProxyStart(sessionID, command string, args []string, serverID string) error {
	baseLogEntry := BaseLogEntry{
		Timestamp:   time.Now(),
		RequestID:   fmt.Sprintf("start_%d", time.Now().UnixNano()),
		SessionID:   sessionID,
		Direction:   "system",
		ServerID:    serverID,
		RawMessage:  "Proxy started",
		MessageType: "system",
		Success:     true,
	}
	return l.logEntry(&StdioLogEntry{
		BaseLogEntry: baseLogEntry,
		Command:      command,
		Args:         args,
		ProjectPath:  common.GetCurrentWorkingDir(),
	})
}

// LogProxyStop logs when a proxy stops
func (l *Logger) LogProxyStop(sessionID, command string, args []string, serverID string, success bool, errorMsg string) error {
	baseLogEntry := BaseLogEntry{
		Timestamp:   time.Now(),
		RequestID:   fmt.Sprintf("stop_%d", time.Now().UnixNano()),
		SessionID:   sessionID,
		Direction:   "system",
		ServerID:    serverID,
		RawMessage:  "Proxy stopped",
		MessageType: "system",
		Success:     success,
		Error:       errorMsg,
	}
	entry := StdioLogEntry{
		BaseLogEntry: baseLogEntry,
		Command:      command,
		Args:         args,
		ProjectPath:  common.GetCurrentWorkingDir(),
	}

	return l.logEntry(&entry)
}

// logEntry writes a log entry to the file
func (l *Logger) logEntry(entry *StdioLogEntry) error {
	if l.logFile == nil {
		return fmt.Errorf("logger not initialized")
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	// Write JSON line
	if _, err := l.logFile.Write(data); err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	// Write newline
	if _, err := l.logFile.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	// Sync to disk
	return l.logFile.Sync()
}

// GetLogPath returns the absolute path to the current log file.
// This method can be used by external callers to:
//   - Display log location to users for debugging
//   - Access logs programmatically for analysis or monitoring
//   - Integrate with external log aggregation tools
//   - Provide log file paths in status/diagnostic outputs
func (l *Logger) GetLogPath() string {
	return l.logPath
}
