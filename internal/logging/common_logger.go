// Package logging provides utility and helper functions and structs related to logging activity
// for both internal logs (regarding the centian proxy) and MCP requests/respoonses.
package logging

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/T4cceptor/centian/internal/common"
)

// Logger handles log file I/O operations (base logger for all transports).
type Logger struct {
	logFile          *os.File
	logPath          string
	actionEventStore ActionEventStore
	mu               sync.Mutex // Protect concurrent writes
}

// NewLogger creates a new base logger instance.
func NewLogger() (*Logger, error) {
	// Resolve logs directory location.
	logsDir, err := GetLogsDirectory()
	if err != nil {
		return nil, err
	}
	return NewLoggerInDir(logsDir)
}

// NewLoggerInDir creates a logger that writes request JSONL files in logDir.
func NewLoggerInDir(logsDir string) (*Logger, error) {
	if err := os.MkdirAll(logsDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Create log file with current date.
	logFileName := fmt.Sprintf("requests_%s.jsonl", time.Now().Format("2006-01-02"))
	logPath := filepath.Join(logsDir, logFileName)

	// Open log file in append mode.
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

// LogEntry writes any log entry to the JSONL file (base Logger method).
func (l *Logger) LogEntry(entry interface{}) error {
	if l.logFile == nil {
		return fmt.Errorf("logger not initialized")
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	// Write JSON line.
	if _, err := l.logFile.Write(data); err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	// Write newline.
	if _, err := l.logFile.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	// Sync to disk.
	return l.logFile.Sync()
}

// SetActionEventStore configures an optional SQL/event sink for MCP action events.
func (l *Logger) SetActionEventStore(store ActionEventStore) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.actionEventStore = store
}

// Close closes the logger.
func (l *Logger) Close() error {
	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
}

// GetLogPath returns the absolute path to the current log file.
// This method can be used by external callers to:.
//   - Display log location to users for debugging.
//   - Access logs programmatically for analysis or monitoring.
//   - Integrate with external log aggregation tools.
//   - Provide log file paths in status/diagnostic outputs.
func (l *Logger) GetLogPath() string {
	return l.logPath
}

// LogMcpEvent logs the provided stdio/http MCP event.
func (l *Logger) LogMcpEvent(event *common.LogEntry) error {
	var errs []error
	if err := l.LogEntry(event); err != nil {
		errs = append(errs, err)
	}

	l.mu.Lock()
	store := l.actionEventStore
	l.mu.Unlock()
	if store != nil {
		if err := store.AppendActionEvent(event); err != nil {
			errs = append(errs, fmt.Errorf("failed to persist action event: %w", err))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
