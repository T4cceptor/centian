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

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp     time.Time         `json:"timestamp"`
	RequestID     string            `json:"request_id"`
	SessionID     string            `json:"session_id,omitempty"`
	Direction     string            `json:"direction"`     // "request" or "response"
	Command       string            `json:"command"`       // Command being proxied
	Args          []string          `json:"args"`          // Command arguments
	ProjectPath   string            `json:"project_path"`  // Working directory
	ConfigSource  string            `json:"config_source"` // global|project|profile
	ServerID      string            `json:"server_id,omitempty"`
	Message       string            `json:"message"`       // The actual MCP message
	MessageType   string            `json:"message_type"`  // request, response, error
	Success       bool              `json:"success"`
	Error         string            `json:"error,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// Logger handles logging of MCP proxy operations
type Logger struct {
	logFile *os.File
	logPath string
}

// NewLogger creates a new logger instance
func NewLogger() (*Logger, error) {
	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	
	// Create logs directory
	logsDir := filepath.Join(homeDir, ".centian", "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}
	
	// Create log file with current date
	logFileName := fmt.Sprintf("requests_%s.jsonl", time.Now().Format("2006-01-02"))
	logPath := filepath.Join(logsDir, logFileName)
	
	// Open log file in append mode
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
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
	return l.logEntry(LogEntry{
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
	
	return l.logEntry(entry)
}

// LogProxyStart logs when a proxy starts
func (l *Logger) LogProxyStart(sessionID, command string, args []string, serverID string) error {
	return l.logEntry(LogEntry{
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
	
	return l.logEntry(entry)
}

// logEntry writes a log entry to the file
func (l *Logger) logEntry(entry LogEntry) error {
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

// GetLogPath returns the path to the current log file
func (l *Logger) GetLogPath() string {
	return l.logPath
}