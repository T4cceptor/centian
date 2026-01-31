package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogging(t *testing.T) {
	// Initialize logging.
	if err := initInternalLogger(); err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer CloseLogger()

	// Test basic logging functions.
	LogInfo("Test info message: %s", "logging system")
	LogError("Test error message: %d", 123)
	LogDebug("Test debug message")
	LogWarn("Test warning message")

	// Close logger to flush writes.
	CloseLogger()

	// Verify log file was created and contains our messages.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	logPath := filepath.Join(homeDir, ".centian", "centian.log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Fatalf("Log file was not created at %s", logPath)
	}

	// Read log file and verify content.
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
	}

	for _, expected := range expectedMessages {
		if !strings.Contains(logStr, expected) {
			t.Errorf("Expected log message not found: %s", expected)
		}
	}

	t.Logf("Log file created at: %s", logPath)
	t.Logf("Log file size: %d bytes", len(logContent))
}
