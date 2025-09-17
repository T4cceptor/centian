package common

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Logger provides basic logging functionality to .centian folder
type Logger struct {
	logFile *os.File
	logger  *log.Logger
}

// NewLogger creates a new logger instance that writes to ~/.centian/centian.log
func NewLogger() (*Logger, error) {
	// Get user home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Create .centian directory if it doesn't exist
	centianDir := filepath.Join(homeDir, ".centian")
	if err := os.MkdirAll(centianDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .centian directory: %w", err)
	}

	// Open log file (create if doesn't exist, append if exists)
	logPath := filepath.Join(centianDir, "centian.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// Create logger with timestamp and prefix
	logger := log.New(logFile, "", log.LstdFlags)

	return &Logger{
		logFile: logFile,
		logger:  logger,
	}, nil
}

// Close closes the log file
func (l *Logger) Close() error {
	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
}

// Info logs an info message
func (l *Logger) Info(message string, args ...interface{}) {
	l.logger.Printf("[INFO] "+message, args...)
}

// Error logs an error message
func (l *Logger) Error(message string, args ...interface{}) {
	l.logger.Printf("[ERROR] "+message, args...)
}

// Debug logs a debug message
func (l *Logger) Debug(message string, args ...interface{}) {
	l.logger.Printf("[DEBUG] "+message, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(message string, args ...interface{}) {
	l.logger.Printf("[WARN] "+message, args...)
}

// LogOperation logs the start and completion of an operation
func (l *Logger) LogOperation(operation string, fn func() error) error {
	l.Info("Starting operation: %s", operation)
	start := time.Now()
	
	err := fn()
	duration := time.Since(start)
	
	if err != nil {
		l.Error("Operation failed: %s (duration: %v) - %v", operation, duration, err)
	} else {
		l.Info("Operation completed: %s (duration: %v)", operation, duration)
	}
	
	return err
}

// Global logger instance
var globalLogger *Logger

// InitializeLogger initializes the global logger
func InitializeLogger() error {
	var err error
	globalLogger, err = NewLogger()
	return err
}

// CloseLogger closes the global logger
func CloseLogger() error {
	if globalLogger != nil {
		return globalLogger.Close()
	}
	return nil
}

// LogInfo logs an info message using the global logger
func LogInfo(message string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.Info(message, args...)
	}
}

// LogError logs an error message using the global logger
func LogError(message string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.Error(message, args...)
	}
}

// LogDebug logs a debug message using the global logger
func LogDebug(message string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.Debug(message, args...)
	}
}

// LogWarn logs a warning message using the global logger
func LogWarn(message string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.Warn(message, args...)
	}
}

// LogOperation logs an operation using the global logger
func LogOperation(operation string, fn func() error) error {
	if globalLogger != nil {
		return globalLogger.LogOperation(operation, fn)
	}
	return fn()
}