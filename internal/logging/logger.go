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
)

// LogEntry represents a single log entry for MCP proxy operations.
// Each entry captures comprehensive information about requests, responses,
// and proxy lifecycle events. Entries are serialized to JSON format for
// structured logging and analysis.
type LogEntry struct {
	// Timestamp is the exact time when the log entry was created
	Timestamp time.Time `json:"timestamp"`

	// RequestID uniquely identifies a single request/response pair
	RequestID string `json:"request_id"`

	// SessionID groups multiple requests within the same proxy session
	SessionID string `json:"session_id,omitempty"`

	// Direction indicates the communication flow perspective:
	// "request" (client→server),
	// "response" (server→client), or
	// "system" (proxy lifecycle events).
	// This field remains stable regardless of success/failure status.
	Direction string `json:"direction"`

	// Command is the executable being proxied (e.g., "npx", "python")
	Command string `json:"command"`

	// Args are the command-line arguments passed to the command
	Args []string `json:"args"`

	// ProjectPath is the working directory where the command executes
	ProjectPath string `json:"project_path"`

	// ConfigSource indicates where configuration originated: "global", "project", or "profile"
	ConfigSource string `json:"config_source"`

	// ServerID uniquely identifies the MCP server instance handling this request
	ServerID string `json:"server_id,omitempty"`

	// Message contains the actual MCP protocol message content
	Message string `json:"message"`

	// MessageType categorizes the content/outcome: "request", "response", "error", or "system".
	// Unlike Direction, this changes to "error" for failed responses, enabling filtering
	// by operational status (e.g., "all errors" vs "all responses regardless of success").
	// This orthogonal design supports both flow analysis (Direction) and status monitoring (MessageType).
	MessageType string `json:"message_type"`

	// Success indicates whether the operation completed successfully
	Success bool `json:"success"`

	// Error contains error details if Success is false
	Error string `json:"error,omitempty"`

	// Metadata holds additional context-specific key-value pairs
	Metadata map[string]string `json:"metadata,omitempty"`
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
	return l.logEntry(&LogEntry{
		Timestamp:   time.Now(),
		RequestID:   requestID,
		SessionID:   sessionID,
		Direction:   "request",
		Command:     command,
		Args:        args,
		ProjectPath: getCurrentWorkingDir(),
		ServerID:    serverID,
		Message:     message,
		MessageType: "request",
		Success:     true,
	})
}

// LogResponse logs an MCP response
func (l *Logger) LogResponse(requestID, sessionID, command string, args []string, serverID, message string, success bool, errorMsg string) error {
	entry := LogEntry{
		Timestamp:   time.Now(),
		RequestID:   requestID,
		SessionID:   sessionID,
		Direction:   "response",
		Command:     command,
		Args:        args,
		ProjectPath: getCurrentWorkingDir(),
		ServerID:    serverID,
		Message:     message,
		MessageType: "response",
		Success:     success,
	}

	if !success {
		entry.Error = errorMsg
		entry.MessageType = "error"
	}

	return l.logEntry(&entry)
}

// LogProxyStart logs when a proxy starts
func (l *Logger) LogProxyStart(sessionID, command string, args []string, serverID string) error {
	return l.logEntry(&LogEntry{
		Timestamp:   time.Now(),
		RequestID:   fmt.Sprintf("start_%d", time.Now().UnixNano()),
		SessionID:   sessionID,
		Direction:   "system",
		Command:     command,
		Args:        args,
		ProjectPath: getCurrentWorkingDir(),
		ServerID:    serverID,
		Message:     "Proxy started",
		MessageType: "system",
		Success:     true,
	})
}

// LogProxyStop logs when a proxy stops
func (l *Logger) LogProxyStop(sessionID, command string, args []string, serverID string, success bool, errorMsg string) error {
	entry := LogEntry{
		Timestamp:   time.Now(),
		RequestID:   fmt.Sprintf("stop_%d", time.Now().UnixNano()),
		SessionID:   sessionID,
		Direction:   "system",
		Command:     command,
		Args:        args,
		ProjectPath: getCurrentWorkingDir(),
		ServerID:    serverID,
		Message:     "Proxy stopped",
		MessageType: "system",
		Success:     success,
	}

	if !success {
		entry.Error = errorMsg
	}

	return l.logEntry(&entry)
}

// logEntry writes a log entry to the file
func (l *Logger) logEntry(entry *LogEntry) error {
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

// getCurrentWorkingDir gets the current working directory
func getCurrentWorkingDir() string {
	pwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return pwd
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
