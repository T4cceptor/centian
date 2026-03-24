package logging

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/common"
)

type failingActionEventStore struct{}

func (f *failingActionEventStore) AppendActionEvent(_ *common.LogEntry) error {
	return errors.New("boom")
}

func getBaseMcpEvent() common.BaseMcpEvent {
	requestID := "req_123"
	sessionID := "session_456"
	serverID := "server_789"

	return common.BaseMcpEvent{
		Timestamp:        time.Now(),
		RequestID:        requestID,
		SessionID:        sessionID,
		Direction:        common.DirectionClientToServer,
		ServerID:         serverID,
		MessageType:      common.MessageTypeRequest,
		Success:          true,
		Error:            "",
		Metadata:         nil,
		Transport:        "internal",
		ProcessingErrors: nil,
	}
}

func TestLogMcpEvent(t *testing.T) {
	// Setup: create temporary directory.
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: a logger and a BaseMcpEvent.
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	requestID := "req_123"
	sessionID := "session_456"
	command := "npx"
	args := []string{
		"-y",
		"@modelcontextprotocol/server-sequential-thinking",
	}

	baseMcpEvent := getBaseMcpEvent()
	mcpEvent := common.LogEntry{
		BaseMcpEvent: baseMcpEvent,
		Routing: common.RoutingContext{
			DownstreamCommand: command,
			Args:              args,
		},
	}
	mcpEvent.WithToolRequest("test-tool", "", json.RawMessage(`{"method":"ping"}`))

	// When: logging a request.
	err = logger.LogMcpEvent(&mcpEvent)

	// Then: the log should be written successfully.
	if err != nil {
		t.Errorf("Failed to log request: %v", err)
	}

	// Read and verify log content (date-based naming).
	logsDir := filepath.Join(tempDir, ".centian", "logs")
	logFileName := fmt.Sprintf("requests_%s.jsonl", time.Now().Format("2006-01-02"))
	logFile := filepath.Join(logsDir, logFileName)
	logContent, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Parse the log line as JSON.
	var logEntry common.LogEntry
	err = json.Unmarshal(logContent, &logEntry)
	if err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	// Verify log entry structure.
	if logEntry.RequestID != requestID {
		t.Errorf("Expected request_id '%s', got '%v'", requestID, logEntry.RequestID)
	}

	if logEntry.SessionID != sessionID {
		t.Errorf("Expected session_id '%s', got '%v'", sessionID, logEntry.SessionID)
	}

	expectedDirection := "[CLIENT -> SERVER]"
	if string(logEntry.Direction) != expectedDirection {
		t.Errorf("Expected direction %v', got '%v'", expectedDirection, logEntry.Direction)
	}

	fmt.Printf("logEntry: %#v", logEntry.ToolCall)

	// Parse tool arguments as JSON.
	if logEntry.ToolCall == nil {
		t.Fatal("Expected tool_call to be present")
	}
	var messageData map[string]interface{}
	err = json.Unmarshal([]byte(logEntry.ToolCall.Arguments), &messageData)
	if err != nil {
		t.Fatalf("Failed to parse tool arguments: %v", err)
	}

	// Check the actual message content.
	if messageData["method"] != "ping" {
		t.Errorf("Expected method 'ping', got '%v'", messageData["method"])
	}

	if logEntry.Routing.DownstreamCommand != command {
		t.Errorf("Expected command '%s', got '%v'", command, logEntry.Routing.DownstreamCommand)
	}

	if !slices.Equal(args, logEntry.Routing.Args) {
		t.Errorf("Expected args '%v', got '%v'", args, logEntry.Routing.Args)
	}
}

// ========================================.
// GetLogPath Tests.
// ========================================.

func TestGetLogPath_ReturnsCorrectPath(t *testing.T) {
	// Setup: create temporary directory.
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: a logger.
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// When: getting the log path.
	logPath := logger.GetLogPath()

	// Then: should return a non-empty path.
	if logPath == "" {
		t.Error("Expected non-empty log path")
	}

	// And: path should contain .centian/logs directory.
	expectedDir := filepath.Join(".centian", "logs")
	if !strings.Contains(logPath, expectedDir) {
		t.Errorf("Expected log path to contain '%s', got: %s", expectedDir, logPath)
	}

	// And: path should end with requests_<date>.jsonl format.
	fileName := filepath.Base(logPath)
	if !strings.HasPrefix(fileName, "requests_") || !strings.HasSuffix(fileName, ".jsonl") {
		t.Errorf("Expected log file name format 'requests_YYYY-MM-DD.jsonl', got: %s", fileName)
	}
}

func TestLogMcpEvent_BestEffortWhenActionStoreFails(t *testing.T) {
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()
	logger.SetActionEventStore(&failingActionEventStore{})

	entry := common.LogEntry{
		BaseMcpEvent: getBaseMcpEvent(),
		Routing: common.RoutingContext{
			Gateway:    "gw",
			ServerName: "server-a",
		},
	}
	entry.WithToolRequest("tool-a", "", json.RawMessage(`{"ping":true}`))

	err = logger.LogMcpEvent(&entry)
	if err == nil {
		t.Fatal("expected best-effort log error when action store fails")
	}

	logContent, readErr := os.ReadFile(logger.GetLogPath())
	if readErr != nil {
		t.Fatalf("Failed to read log file: %v", readErr)
	}
	if !strings.Contains(string(logContent), "\"name\":\"tool-a\"") {
		t.Fatalf("expected JSONL log to be written even when action store fails, got %s", string(logContent))
	}
}
