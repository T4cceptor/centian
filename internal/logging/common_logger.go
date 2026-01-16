// Package logging provides utility and helper functions and structs related to logging activity
// for both internal logs (regarding the centian proxy) and MCP requests/respoonses.
package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
)

// McpEventDirection identifies the direction of the MCP event,
// e.g. from client to server, or from proxy to client, etc.
type McpEventDirection string

const (
	// DirectionClientToServer represents the direction from CLIENT to SERVER.
	DirectionClientToServer McpEventDirection = "[CLIENT -> SERVER]"

	// DirectionServerToClient represents the direction from SERVER to CLIENT.
	DirectionServerToClient McpEventDirection = "[SERVER -> CLIENT]"

	// DirectionCentianToClient represents the direction from CENTIAN to CLIENT,
	// e.g. when the event is returned prematurely back to the CLIENT due to processing.
	DirectionCentianToClient McpEventDirection = "[CENTIAN -> CLIENT]"

	// DirectionSystem represents that this is a SYSTEM event
	// - meaning it is not forwarded to either CLIENT or SERVER.
	DirectionSystem McpEventDirection = "[SYSTEM]"

	// DirectionUnknown represents an unknown direction,
	// in case the direction is not one of the above!
	DirectionUnknown McpEventDirection = "[UNKNOWN]"
)

// MarshalJSON returns the JSON encoding for McpEventDirection
// - if m does match any of the allowed values
// it is replaced with DirectionUnknown
func (m McpEventDirection) MarshalJSON() ([]byte, error) {
	switch m {
	case DirectionClientToServer, DirectionServerToClient, DirectionCentianToClient, DirectionSystem:
		return json.Marshal(string(m))
	default:
		return json.Marshal(string(DirectionUnknown))
	}
}

// UnmarshalJSON parses the JSON-encoded data of McpEventDirection
// - if m does match any of the allowed values
// it is replaced with DirectionUnknown
func (m *McpEventDirection) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch McpEventDirection(s) {
	case DirectionClientToServer, DirectionServerToClient, DirectionCentianToClient, DirectionSystem:
		*m = McpEventDirection(s)
		return nil
	default:
		*m = DirectionUnknown
		return nil
	}
}

// McpMessageType identifies the type of a MCP message, e.g. request, response, system, etc.
type McpMessageType string

const (
	// MessageTypeRequest identifies a message of type "request"
	MessageTypeRequest McpMessageType = "request"
	// MessageTypeResponse identifies a message of type "response"
	MessageTypeResponse McpMessageType = "response"
	// MessageTypeSystem identifies a message of type "system"
	MessageTypeSystem McpMessageType = "system"
	// MessageTypeUnknown identifies a message of type "unknown"
	MessageTypeUnknown McpMessageType = "unknown" // fallback in case of error
)

// MarshalJSON returns the JSON encoding for McpMessageType
// - if m does match any of the allowed values
// it is replaced with MessageTypeUnknown
func (m McpMessageType) MarshalJSON() ([]byte, error) {
	switch m {
	case MessageTypeRequest, MessageTypeResponse, MessageTypeSystem:
		return json.Marshal(string(m))
	default:
		return json.Marshal(string(MessageTypeUnknown))
	}
}

// UnmarshalJSON parses the JSON-encoded data of m
// - if m does match any of the allowed values
// it is replaced with MessageTypeUnknown
func (m *McpMessageType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch McpMessageType(s) {
	case MessageTypeRequest, MessageTypeResponse, MessageTypeSystem:
		*m = McpMessageType(s)
		return nil
	default:
		*m = MessageTypeUnknown
		return nil
	}
}

// BaseLogEntry represents the basic fields for any log entry and is always included in all logs
type BaseLogEntry struct {
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
	Direction McpEventDirection `json:"direction"`

	// ServerID uniquely identifies the MCP server instance handling this request
	ServerID string `json:"server_id,omitempty"`

	// RawMessage container the raw input received for the specific message
	RawMessage string `json:"raw_message"`

	// MessageType categorizes the content/outcome: "request", "response", "error", or "system".
	// Unlike Direction, this changes to "error" for failed responses, enabling filtering
	// by operational status (e.g., "all errors" vs "all responses regardless of success").
	// This orthogonal design supports both flow analysis (Direction) and status monitoring (MessageType).
	MessageType McpMessageType `json:"message_type"`

	// Success indicates whether the operation completed successfully
	Success bool `json:"success"`

	// Error contains error details if Success is false
	Error string `json:"error,omitempty"`

	// Metadata holds additional context-specific key-value pairs
	Metadata map[string]string `json:"metadata,omitempty"`

	// Transport identifies the proxy type: "stdio", "http", "websocket"
	Transport string `json:"transport"`
}

// Logger handles log file I/O operations (base logger for all transports)
type Logger struct {
	logFile *os.File
	logPath string
	mu      sync.Mutex // Protect concurrent writes
}

// NewLogger creates a new base logger instance
func NewLogger() (*Logger, error) {
	// Resolve logs directory location
	logsDir, err := GetLogsDirectory()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(logsDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Create log file with current date
	logFileName := fmt.Sprintf("requests_%s.jsonl", time.Now().Format("2006-01-02"))
	logPath := filepath.Join(logsDir, logFileName)

	// Open log file in append mode
	//nolint:gosec // We are writing a file without sensitive data.
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return &Logger{
		logFile: logFile,
		logPath: logPath,
	}, nil
}

// logEntry writes any log entry to the JSONL file (base Logger method)
func (l *Logger) logEntry(entry interface{}) error {
	if l.logFile == nil {
		return fmt.Errorf("logger not initialized")
	}
	l.mu.Lock()
	defer l.mu.Unlock()

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

// Close closes the logger
func (l *Logger) Close() error {
	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
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

// LogMcpEvent logs the provided stdio/http MCP event
func (l *Logger) LogMcpEvent(event common.McpEventInterface) error {
	return l.logEntry(event)
}
