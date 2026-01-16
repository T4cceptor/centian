package logging

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
)

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

func TestLogBaseMcpEvent(t *testing.T) {
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
	baseMcpEvent := getBaseMcpEvent()
	httpMcpEvent := common.HTTPMcpEvent{
		BaseMcpEvent: baseMcpEvent,
		HTTPEvent: &common.HTTPEvent{
			ReqID:       requestID,
			Method:      "POST",
			URL:         "localhost:9000",
			ReqHeaders:  make(http.Header),
			RespStatus:  -1,
			RespHeaders: nil,
			Body:        []byte("{\"method\":\"ping\"}"),
		},
		Gateway:    "my-test-gateway",
		ServerName: "my-test-server",
		Endpoint:   "/mcp/my-test-gateway/my-test-server",
	}

	// When: logging a request.
	err = logger.LogMcpEvent(&httpMcpEvent)

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
	var logEntry map[string]interface{}
	err = json.Unmarshal(logContent, &logEntry)
	if err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	// Verify log entry structure.
	if logEntry["request_id"] != requestID {
		t.Errorf("Expected request_id '%s', got '%v'", requestID, logEntry["request_id"])
	}

	if logEntry["session_id"] != sessionID {
		t.Errorf("Expected session_id '%s', got '%v'", sessionID, logEntry["session_id"])
	}

	exceptedDirecton := "[CLIENT -> SERVER]"
	if logEntry["direction"] != exceptedDirecton {
		t.Errorf("Expected direction %v', got '%v'", exceptedDirecton, logEntry["direction"])
	}

	// Parse the message field as JSON.
	messageStr, ok := logEntry["raw_message"].(string)
	if !ok {
		t.Fatal("Message field should be a string")
	}

	var messageData map[string]interface{}
	err = json.Unmarshal([]byte(messageStr), &messageData)
	if err != nil {
		t.Fatalf("Failed to parse message content: %v", err)
	}

	// Check the actual message content.
	if messageData["method"] != "ping" {
		t.Errorf("Expected method 'ping', got '%v'", messageData["method"])
	}
}

func TestLogHTTPMcpEvent(t *testing.T) {
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
	gateway := "my-awesome-gateway"
	serverName := "my-awesome-server"
	endpoint := fmt.Sprintf("/mcp/%s/%s", gateway, serverName)
	downstreamURL := "localhost:9000/mcp/something"
	port := "8989"

	baseMcpEvent := getBaseMcpEvent()
	httpMcpEvent := common.HTTPMcpEvent{
		BaseMcpEvent: baseMcpEvent,
		HTTPEvent: &common.HTTPEvent{
			ReqID:       requestID,
			Method:      "POST",
			URL:         "localhost:9000",
			ReqHeaders:  make(http.Header),
			RespStatus:  -1,
			RespHeaders: nil,
			Body:        []byte("{\"method\":\"ping\"}"),
		},
		Gateway:       gateway,
		ServerName:    serverName,
		Endpoint:      endpoint,
		DownstreamURL: downstreamURL,
		ProxyPort:     port,
	}

	// When: logging a request.
	err = logger.LogMcpEvent(&httpMcpEvent)

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
	var logEntry map[string]interface{}
	err = json.Unmarshal(logContent, &logEntry)
	if err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	// Verify log entry structure.
	if logEntry["request_id"] != requestID {
		t.Errorf("Expected request_id '%s', got '%v'", requestID, logEntry["request_id"])
	}

	if logEntry["session_id"] != sessionID {
		t.Errorf("Expected session_id '%s', got '%v'", sessionID, logEntry["session_id"])
	}

	exceptedDirecton := "[CLIENT -> SERVER]"
	if logEntry["direction"] != exceptedDirecton {
		t.Errorf("Expected direction %v', got '%v'", exceptedDirecton, logEntry["direction"])
	}

	// Parse the message field as JSON.
	messageStr, ok := logEntry["raw_message"].(string)
	if !ok {
		t.Fatal("Message field should be a string")
	}

	var messageData map[string]interface{}
	err = json.Unmarshal([]byte(messageStr), &messageData)
	if err != nil {
		t.Fatalf("Failed to parse message content: %v", err)
	}

	// Check the actual message content.
	if messageData["method"] != "ping" {
		t.Errorf("Expected method 'ping', got '%v'", messageData["method"])
	}

	if logEntry["gateway"] != gateway {
		t.Errorf("Expected gateway '%s', got '%v'", gateway, logEntry["gateway"])
	}

	if logEntry["server_name"] != serverName {
		t.Errorf("Expected server_name '%s', got '%v'", serverName, logEntry["server_name"])
	}

	if logEntry["endpoint"] != endpoint {
		t.Errorf("Expected endpoint '%s', got '%v'", endpoint, logEntry["endpoint"])
	}

	if logEntry["downstream_url"] != downstreamURL {
		t.Errorf("Expected downstream_url '%s', got '%v'", downstreamURL, logEntry["downstream_url"])
	}

	if logEntry["proxy_port"] != port {
		t.Errorf("Expected proxy_port '%s', got '%v'", port, logEntry["proxy_port"])
	}
}

func TestLogStdioMcpEvent(t *testing.T) {
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
	projectPath := "/my/path/to/project"
	configSource := "global"

	baseMcpEvent := getBaseMcpEvent()
	httpMcpEvent := common.StdioMcpEvent{
		BaseMcpEvent: baseMcpEvent,
		Command:      command,
		Args:         args,
		ProjectPath:  projectPath,
		ConfigSource: configSource,
		Message:      "{\"method\":\"ping\"}",
	}

	// When: logging a request.
	err = logger.LogMcpEvent(&httpMcpEvent)

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
	var logEntry common.StdioMcpEvent
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

	// Parse the message field as JSON.
	var messageData map[string]interface{}
	err = json.Unmarshal([]byte(logEntry.RawMessage()), &messageData)
	if err != nil {
		t.Fatalf("Failed to parse message content: %v", err)
	}

	// Check the actual message content.
	if messageData["method"] != "ping" {
		t.Errorf("Expected method 'ping', got '%v'", messageData["method"])
	}

	if logEntry.Command != command {
		t.Errorf("Expected command '%s', got '%v'", command, logEntry.Command)
	}

	if !slices.Equal(args, logEntry.Args) {
		t.Errorf("Expected args '%v', got '%v'", args, logEntry.Args)
	}

	if logEntry.ProjectPath != projectPath {
		t.Errorf("Expected project_path '%s', got '%v'", projectPath, logEntry.ProjectPath)
	}

	if logEntry.ConfigSource != configSource {
		t.Errorf("Expected config_source '%s', got '%v'", configSource, logEntry.ConfigSource)
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

func TestGetLogPath_PathExistsAfterLogging(t *testing.T) {
	// Setup: create temporary directory.
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Given: a logger that has written a log entry.
	logger, err := NewLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Log an event to ensure file is created.
	baseMcpEvent := getBaseMcpEvent()
	httpMcpEvent := common.HTTPMcpEvent{
		BaseMcpEvent: baseMcpEvent,
		HTTPEvent: &common.HTTPEvent{
			ReqID:      "test-req",
			Method:     "POST",
			URL:        "http://localhost:8080",
			ReqHeaders: make(http.Header),
			Body:       []byte("{\"test\":\"data\"}"),
		},
		Endpoint:  "/test",
		ProxyPort: "8080",
	}
	err = logger.LogMcpEvent(&httpMcpEvent)
	if err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// When: getting the log path.
	logPath := logger.GetLogPath()

	// Then: the log file should exist.
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Errorf("Log file does not exist at path: %s", logPath)
	}
}
