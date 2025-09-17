package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogging(t *testing.T) {
	// Initialize logging
	if err := InitializeLogger(); err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer CloseLogger()

	// Test basic logging functions
	LogInfo("Test info message: %s", "logging system")
	LogError("Test error message: %d", 123)
	LogDebug("Test debug message")
	LogWarn("Test warning message")

	// Test operation logging
	err := LogOperation("test operation", func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Errorf("LogOperation failed: %v", err)
	}

	// Test operation logging with error
	err = LogOperation("failing operation", func() error {
		return os.ErrNotExist
	})
	if err == nil {
		t.Error("Expected error from failing operation")
	}

	// Close logger to flush writes
	CloseLogger()

	// Verify log file was created and contains our messages
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	logPath := filepath.Join(homeDir, ".centian", "centian.log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Fatalf("Log file was not created at %s", logPath)
	}

	// Read log file and verify content
	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logStr := string(logContent)
	expectedMessages := []string{
		"[INFO] Test info message: logging system",
		"[ERROR] Test error message: 123",
		"[DEBUG] Test debug message",
		"[WARN] Test warning message",
		"[INFO] Starting operation: test operation",
		"[INFO] Operation completed: test operation",
		"[INFO] Starting operation: failing operation",
		"[ERROR] Operation failed: failing operation",
	}

	for _, expected := range expectedMessages {
		if !strings.Contains(logStr, expected) {
			t.Errorf("Expected log message not found: %s", expected)
		}
	}

	t.Logf("Log file created at: %s", logPath)
	t.Logf("Log file size: %d bytes", len(logContent))
}


