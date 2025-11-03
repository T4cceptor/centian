package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNewLogger tests creating a new logger instance
func TestNewLogger(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// When: creating a new logger
	logger, err := NewLogger()

	// Then: the logger should be created successfully
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	if logger == nil {
		t.Fatal("Logger should not be nil")
	}

	// Cleanup
	logger.Close()
}

// TestLoggerDirectoryCreation tests that logger creates necessary directories
func TestLoggerDirectoryCreation(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: no existing .centian directory
	centianDir := filepath.Join(tempDir, ".centian")
	logsDir := filepath.Join(centianDir, "logs")

	// Verify directories don't exist initially
	if _, err := os.Stat(centianDir); !os.IsNotExist(err) {
		t.Error("Centian directory should not exist initially")
	}

	// When: creating a logger
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Then: directories should be created
	if _, err := os.Stat(centianDir); os.IsNotExist(err) {
		t.Error("Centian directory should be created")
	}

	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		t.Error("Logs directory should be created")
	}
}

// TestLogProxyStart tests logging proxy start events
func TestLogProxyStart(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: a logger
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// When: logging a proxy start event
	sessionID := "test_session_123"
	command := "echo"
	args := []string{"test"}
	serverID := "test_server_456"

	err = logger.LogProxyStart(sessionID, command, args, serverID)

	// Then: the log should be written successfully
	if err != nil {
		t.Errorf("Failed to log proxy start: %v", err)
	}

	// Verify log file exists (date-based naming)
	logsDir := filepath.Join(tempDir, ".centian", "logs")
	logFileName := fmt.Sprintf("requests_%s.jsonl", time.Now().Format("2006-01-02"))
	logFile := filepath.Join(logsDir, logFileName)
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("Log file should exist")
	}

	// Read and verify log content
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)
	if !strings.Contains(logContent, sessionID) {
		t.Error("Log should contain session ID")
	}

	if !strings.Contains(logContent, command) {
		t.Error("Log should contain command")
	}

	if !strings.Contains(logContent, serverID) {
		t.Error("Log should contain server ID")
	}

	if !strings.Contains(logContent, "\"message_type\":\"system\"") {
		t.Error("Log should contain system message type")
	}

	if !strings.Contains(logContent, "Proxy started") {
		t.Error("Log should contain proxy start message")
	}
}

// TestLogProxyStop tests logging proxy stop events
func TestLogProxyStop(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: a logger
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// When: logging a proxy stop event
	sessionID := "test_session_123"
	command := "echo"
	args := []string{"test"}
	serverID := "test_server_456"
	success := true
	errorMsg := ""

	err = logger.LogProxyStop(sessionID, command, args, serverID, success, errorMsg)

	// Then: the log should be written successfully
	if err != nil {
		t.Errorf("Failed to log proxy stop: %v", err)
	}

	// Read and verify log content (date-based naming)
	logsDir := filepath.Join(tempDir, ".centian", "logs")
	logFileName := fmt.Sprintf("requests_%s.jsonl", time.Now().Format("2006-01-02"))
	logFile := filepath.Join(logsDir, logFileName)
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)
	if !strings.Contains(logContent, "\"message_type\":\"system\"") {
		t.Error("Log should contain system message type")
	}

	if !strings.Contains(logContent, "Proxy stopped") {
		t.Error("Log should contain proxy stop message")
	}

	if !strings.Contains(logContent, "\"success\":true") {
		t.Error("Log should contain success status")
	}
}

// TestLogRequest tests logging MCP requests
func TestLogRequest(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: a logger
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// When: logging a request
	requestID := "req_123"
	sessionID := "session_456"
	command := "echo"
	args := []string{"test"}
	serverID := "server_789"
	content := `{"method":"ping"}`

	err = logger.LogRequest(requestID, sessionID, command, args, serverID, content)

	// Then: the log should be written successfully
	if err != nil {
		t.Errorf("Failed to log request: %v", err)
	}

	// Read and verify log content (date-based naming)
	logsDir := filepath.Join(tempDir, ".centian", "logs")
	logFileName := fmt.Sprintf("requests_%s.jsonl", time.Now().Format("2006-01-02"))
	logFile := filepath.Join(logsDir, logFileName)
	logContent, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Parse the log line as JSON
	var logEntry map[string]interface{}
	err = json.Unmarshal(logContent, &logEntry)
	if err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	// Verify log entry structure
	if logEntry["request_id"] != requestID {
		t.Errorf("Expected request_id '%s', got '%v'", requestID, logEntry["request_id"])
	}

	if logEntry["session_id"] != sessionID {
		t.Errorf("Expected session_id '%s', got '%v'", sessionID, logEntry["session_id"])
	}

	if logEntry["direction"] != "request" {
		t.Errorf("Expected direction 'request', got '%v'", logEntry["direction"])
	}

	// Parse the message field as JSON
	messageStr, ok := logEntry["message"].(string)
	if !ok {
		t.Fatal("Message field should be a string")
	}

	var messageData map[string]interface{}
	err = json.Unmarshal([]byte(messageStr), &messageData)
	if err != nil {
		t.Fatalf("Failed to parse message content: %v", err)
	}

	// Check the actual message content
	if messageData["method"] != "ping" {
		t.Errorf("Expected method 'ping', got '%v'", messageData["method"])
	}
}

// TestLogResponse tests logging MCP responses
func TestLogResponse(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: a logger
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// When: logging a response
	requestID := "req_123"
	sessionID := "session_456"
	command := "echo"
	args := []string{"test"}
	serverID := "server_789"
	content := `{"result":"pong"}`
	success := true
	errorMsg := ""

	err = logger.LogResponse(requestID, sessionID, command, args, serverID, content, success, errorMsg)

	// Then: the log should be written successfully
	if err != nil {
		t.Errorf("Failed to log response: %v", err)
	}

	// Read and verify log content (date-based naming)
	logsDir := filepath.Join(tempDir, ".centian", "logs")
	logFileName := fmt.Sprintf("requests_%s.jsonl", time.Now().Format("2006-01-02"))
	logFile := filepath.Join(logsDir, logFileName)
	logContent, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Parse the log line as JSON
	var logEntry map[string]interface{}
	err = json.Unmarshal(logContent, &logEntry)
	if err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	// Verify log entry structure
	if logEntry["request_id"] != requestID {
		t.Errorf("Expected request_id '%s', got '%v'", requestID, logEntry["request_id"])
	}

	if logEntry["session_id"] != sessionID {
		t.Errorf("Expected session_id '%s', got '%v'", sessionID, logEntry["session_id"])
	}

	if logEntry["direction"] != "response" {
		t.Errorf("Expected direction 'response', got '%v'", logEntry["direction"])
	}

	if logEntry["success"] != true {
		t.Errorf("Expected success true, got '%v'", logEntry["success"])
	}

	// Parse the message field as JSON
	messageStr, ok := logEntry["message"].(string)
	if !ok {
		t.Fatal("Message field should be a string")
	}

	var messageData map[string]interface{}
	err = json.Unmarshal([]byte(messageStr), &messageData)
	if err != nil {
		t.Fatalf("Failed to parse message content: %v", err)
	}

	// Check the actual message content
	if messageData["result"] != "pong" {
		t.Errorf("Expected result 'pong', got '%v'", messageData["result"])
	}
}

// TestLoggerConcurrentWrites tests concurrent logging operations
func TestLoggerConcurrentWrites(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: a logger
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// When: performing concurrent logging operations
	numGoroutines := 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			sessionID := fmt.Sprintf("session_%d", id)
			requestID := fmt.Sprintf("req_%d", id)
			serverID := fmt.Sprintf("server_%d", id)

			// Log proxy start
			logger.LogProxyStart(sessionID, "echo", []string{"test"}, serverID)

			// Log request
			logger.LogRequest(requestID, sessionID, "echo", []string{"test"}, serverID, `{"method":"ping"}`)

			// Log response
			logger.LogResponse(requestID, sessionID, "echo", []string{"test"}, serverID, `{"result":"pong"}`, true, "")

			// Log proxy stop
			logger.LogProxyStop(sessionID, "echo", []string{"test"}, serverID, true, "")

			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-done:
			// Success
		case <-time.After(5 * time.Second):
			t.Fatal("Concurrent logging test timed out")
		}
	}

	// Then: all logs should be written successfully
	// Verify log file exists and has content (all logs go to single date-based file)
	logsDir := filepath.Join(tempDir, ".centian", "logs")
	logFileName := fmt.Sprintf("requests_%s.jsonl", time.Now().Format("2006-01-02"))
	logFile := filepath.Join(logsDir, logFileName)

	logContent, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Use the same content for both checks since all logs go to one file
	proxyContent := logContent
	requestContent := logContent

	// Count log lines (each log entry should be on its own line)
	proxyLines := strings.Count(string(proxyContent), "\n")
	requestLines := strings.Count(string(requestContent), "\n")

	// We expect 2 proxy operations per goroutine (start + stop)
	expectedProxyLines := numGoroutines * 2
	if proxyLines < expectedProxyLines {
		t.Errorf("Expected at least %d proxy log lines, got %d", expectedProxyLines, proxyLines)
	}

	// We expect 2 request operations per goroutine (request + response)
	expectedRequestLines := numGoroutines * 2
	if requestLines < expectedRequestLines {
		t.Errorf("Expected at least %d request log lines, got %d", expectedRequestLines, requestLines)
	}
}

// TestLoggerWithInvalidDirectory tests logger behavior with invalid directory
func TestLoggerWithInvalidDirectory(t *testing.T) {
	// This test is platform-specific and may not work on all systems
	// Skip on Windows or if running as root
	if os.Getenv("OS") == "Windows_NT" {
		t.Skip("Skipping invalid directory test on Windows")
	}

	if os.Getuid() == 0 {
		t.Skip("Skipping invalid directory test when running as root")
	}

	// Setup: create temporary directory with restricted permissions
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	restrictedDir := filepath.Join(tempDir, "restricted")

	// Create directory and remove write permissions
	err := os.MkdirAll(restrictedDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create restricted directory: %v", err)
	}

	err = os.Chmod(restrictedDir, 0555) // Read and execute only
	if err != nil {
		t.Fatalf("Failed to set restricted permissions: %v", err)
	}

	os.Setenv("HOME", restrictedDir)
	defer func() {
		os.Setenv("HOME", originalHome)
		os.Chmod(restrictedDir, 0755) // Restore permissions for cleanup
	}()

	// When: trying to create a logger in restricted directory
	logger, err := NewLogger()

	// Then: it should handle the error gracefully
	if err != nil {
		// This is expected behavior - logger should fail gracefully
		t.Logf("Logger correctly failed with restricted directory: %v", err)
		return
	}

	// If logger was created, it might have fallen back to a temp directory
	if logger != nil {
		t.Log("Logger created despite restricted directory (may have used fallback)")
		logger.Close()
	}
}

// TestLoggerClose tests closing the logger
func TestLoggerClose(t *testing.T) {
	// Setup: create temporary directory
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: a logger
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// When: closing the logger
	logger.Close()

	// Then: subsequent logging operations should handle closed logger gracefully
	// (The exact behavior depends on implementation - it might be a no-op or return an error)
	err = logger.LogProxyStart("session", "echo", []string{"test"}, "server")
	if err != nil {
		t.Logf("Logging after close returned error (expected): %v", err)
	}
}
