package common

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultInternalLogFileName = "centian.log"
	defaultLogLevel            = "info"
	logOutputFile              = "file"
	logOutputConsole           = "console"
	logOutputBoth              = "both"
	defaultLogOutput           = logOutputFile
)

// LogLevel defines the minimum severity that is written by the internal logger.
type LogLevel int

const (
	// logLevelDebug enables all internal proxy log messages.
	logLevelDebug LogLevel = iota
	logLevelInfo
	logLevelWarn
	logLevelError
)

// LoggerOptions configures the global internal logger.
type LoggerOptions struct {
	Level         string
	Output        string
	FilePath      string
	ConsoleWriter io.Writer
}

// InternalLogger provides basic logging functionality to .centian folder.
type InternalLogger struct {
	logFile *os.File
	logger  *log.Logger
	level   LogLevel
}

// newInternalLogger creates a new logger instance with configurable output targets.
func newInternalLogger(options LoggerOptions) (*InternalLogger, error) {
	level, err := parseLogLevel(options.Level)
	if err != nil {
		return nil, err
	}

	output, err := parseLogOutput(options.Output)
	if err != nil {
		return nil, err
	}

	var (
		logFile *os.File
		writers []io.Writer
	)

	if output == logOutputFile || output == logOutputBoth {
		logPath, resolveErr := resolveLogFilePath(options.FilePath)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
		//nolint:gosec // We are writing a file without sensitive data.
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		writers = append(writers, logFile)
	}

	if output == logOutputConsole || output == logOutputBoth {
		consoleWriter := options.ConsoleWriter
		if consoleWriter == nil {
			consoleWriter = os.Stderr
		}
		writers = append(writers, consoleWriter)
	}

	logger := log.New(io.MultiWriter(writers...), "", log.LstdFlags)

	return &InternalLogger{
		logFile: logFile,
		logger:  logger,
		level:   level,
	}, nil
}

func parseLogLevel(value string) (LogLevel, error) {
	switch strings.ToLower(strings.TrimSpace(defaultIfEmpty(value, defaultLogLevel))) {
	case "debug":
		return logLevelDebug, nil
	case "info":
		return logLevelInfo, nil
	case "warn":
		return logLevelWarn, nil
	case "error":
		return logLevelError, nil
	default:
		return logLevelInfo, fmt.Errorf("invalid log level %q: expected debug, info, warn, or error", value)
	}
}

func parseLogOutput(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(defaultIfEmpty(value, defaultLogOutput))) {
	case logOutputFile, logOutputConsole, logOutputBoth:
		return strings.ToLower(strings.TrimSpace(defaultIfEmpty(value, defaultLogOutput))), nil
	default:
		return "", fmt.Errorf("invalid log output %q: expected file, console, or both", value)
	}
}

func resolveLogFilePath(configuredPath string) (string, error) {
	path := strings.TrimSpace(configuredPath)
	if path == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		return filepath.Join(homeDir, ".centian", defaultInternalLogFileName), nil
	}

	if path == "~" || strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		if path == "~" {
			return homeDir, nil
		}
		path = filepath.Join(homeDir, strings.TrimPrefix(path, "~/"))
	}

	return filepath.Clean(path), nil
}

func defaultIfEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// Close closes the log file.
func (l *InternalLogger) Close() error {
	if l.logFile != nil {
		err := l.logFile.Close()
		l.logFile = nil
		return err
	}
	return nil
}

// Info logs an info message.
func (l *InternalLogger) Info(message string, args ...interface{}) {
	l.log(logLevelInfo, "INFO", message, args...)
}

// Error logs an error message.
func (l *InternalLogger) Error(message string, args ...interface{}) {
	l.log(logLevelError, "ERROR", message, args...)
}

// Debug logs a debug message.
func (l *InternalLogger) Debug(message string, args ...interface{}) {
	l.log(logLevelDebug, "DEBUG", message, args...)
}

// Warn logs a warning message.
func (l *InternalLogger) Warn(message string, args ...interface{}) {
	l.log(logLevelWarn, "WARN", message, args...)
}

func (l *InternalLogger) log(level LogLevel, prefix, message string, args ...interface{}) {
	if l == nil || l.logger == nil || level < l.level {
		return
	}
	logArgs := append([]interface{}{prefix}, args...)
	l.logger.Printf("[%s] "+message, logArgs...)
}

// Global logger instance.
var globalLogger *InternalLogger

// InitInternalLogger initializes or replaces the global logger.
func InitInternalLogger(options LoggerOptions) error {
	logger, err := newInternalLogger(options)
	if err != nil {
		return err
	}
	if globalLogger != nil {
		_ = globalLogger.Close()
	}
	globalLogger = logger
	return nil
}

// CloseLogger closes the global logger.
func CloseLogger() error {
	if globalLogger != nil {
		err := globalLogger.Close()
		globalLogger = nil
		return err
	}
	return nil
}

// LogInfo logs an info message using the global logger.
func LogInfo(message string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.Info(message, args...)
	}
}

// LogError logs an error message using the global logger.
func LogError(message string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.Error(message, args...)
	}
}

// LogDebug logs a debug message using the global logger.
func LogDebug(message string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.Debug(message, args...)
	}
}

// LogWarn logs a warning message using the global logger.
func LogWarn(message string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.Warn(message, args...)
	}
}

// DebugLoggingEnabled returns true when the global logger will emit debug logs.
func DebugLoggingEnabled() bool {
	return globalLogger != nil && globalLogger.level <= logLevelDebug
}
