package processor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CentianAI/centian-cli/internal/config"
)

// TestNewExecutor tests executor initialization
func TestNewExecutor(t *testing.T) {
	// Given: creating a new executor
	executor, err := NewExecutor()

	// Then: executor is created successfully with home directory
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}
	if executor.WorkingDir == "" {
		t.Error("WorkingDir should not be empty")
	}
	if !strings.Contains(executor.WorkingDir, string(os.PathSeparator)) {
		t.Errorf("WorkingDir should be an absolute path, got: %s", executor.WorkingDir)
	}
}

// TestExecute_SuccessfulCLI tests successful processor execution
func TestExecute_SuccessfulCLI(t *testing.T) {
	// Given: a test script that returns valid JSON
	tempDir := t.TempDir()
	scriptPath := createTestScript(t, tempDir, "success.sh", `#!/bin/bash
read input
echo '{"status": 200, "payload": {"result": "success"}, "error": null}'
`)

	executor := &Executor{WorkingDir: tempDir}
	processorConfig := &config.ProcessorConfig{
		Name:    "test-processor",
		Type:    "cli",
		Enabled: true,
		Timeout: 5,
		Config: map[string]interface{}{
			"command": "bash",
			"args":    []interface{}{scriptPath},
		},
	}

	input := &config.ProcessorInput{
		Type:      "request",
		Timestamp: time.Now().Format(time.RFC3339),
		Connection: config.ConnectionContext{
			ServerName: "test-server",
			Transport:  "stdio",
			SessionID:  "session-123",
		},
		Payload: map[string]interface{}{
			"method": "tools/list",
		},
		Metadata: config.ProcessorMetadata{
			ProcessorChain:  []string{},
			OriginalPayload: map[string]interface{}{"method": "tools/list"},
		},
	}

	// When: executing the processor
	output, err := executor.Execute(processorConfig, input)

	// Then: execution succeeds with expected output
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if output.Status != 200 {
		t.Errorf("Expected status 200, got %d", output.Status)
	}
	if result, ok := output.Payload["result"].(string); !ok || result != "success" {
		t.Errorf("Expected payload.result='success', got %v", output.Payload)
	}
	if output.Error != nil {
		t.Errorf("Expected no error, got: %s", *output.Error)
	}
}

// TestExecute_PayloadModification tests processor modifying payload
func TestExecute_PayloadModification(t *testing.T) {
	// Given: a processor that modifies the payload
	tempDir := t.TempDir()
	scriptPath := createTestScript(t, tempDir, "modify.sh", `#!/bin/bash
read input
echo '{"status": 200, "payload": {"method": "tools/list", "modified": true}, "error": null}'
`)

	executor := &Executor{WorkingDir: tempDir}
	processorConfig := &config.ProcessorConfig{
		Name:    "modifier",
		Type:    "cli",
		Enabled: true,
		Timeout: 5,
		Config: map[string]interface{}{
			"command": "bash",
			"args":    []interface{}{scriptPath},
		},
	}

	input := &config.ProcessorInput{
		Type:      "request",
		Timestamp: time.Now().Format(time.RFC3339),
		Connection: config.ConnectionContext{
			ServerName: "test-server",
		},
		Payload: map[string]interface{}{
			"method": "tools/list",
		},
		Metadata: config.ProcessorMetadata{
			ProcessorChain:  []string{},
			OriginalPayload: map[string]interface{}{"method": "tools/list"},
		},
	}

	// When: executing the processor
	output, err := executor.Execute(processorConfig, input)

	// Then: payload is modified as expected
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if output.Status != 200 {
		t.Errorf("Expected status 200, got %d", output.Status)
	}
	modified, ok := output.Payload["modified"].(bool)
	if !ok || !modified {
		t.Errorf("Expected payload.modified=true, got %v", output.Payload)
	}
}

// TestExecute_Timeout tests processor timeout handling
func TestExecute_Timeout(t *testing.T) {
	// Given: a processor that sleeps longer than timeout
	// Use 'sleep' command directly instead of bash script for more reliable timeout
	tempDir := t.TempDir()

	executor := &Executor{WorkingDir: tempDir}
	processorConfig := &config.ProcessorConfig{
		Name:    "slow-processor",
		Type:    "cli",
		Enabled: true,
		Timeout: 1, // 1 second timeout
		Config: map[string]interface{}{
			"command": "sleep",
			"args":    []interface{}{"10"},
		},
	}

	input := &config.ProcessorInput{
		Type:      "request",
		Timestamp: time.Now().Format(time.RFC3339),
		Payload:   map[string]interface{}{"method": "test"},
		Metadata: config.ProcessorMetadata{
			ProcessorChain:  []string{},
			OriginalPayload: map[string]interface{}{"method": "test"},
		},
	}

	// When: executing the processor
	start := time.Now()
	output, err := executor.Execute(processorConfig, input)
	duration := time.Since(start)

	// Then: execution times out with 500 error
	if err != nil {
		t.Fatalf("Execute should not return error for timeout, got: %v", err)
	}
	if output.Status != 500 {
		t.Errorf("Expected status 500 for timeout, got %d", output.Status)
	}
	if output.Error == nil {
		t.Error("Expected error message for timeout")
	} else if !strings.Contains(*output.Error, "timed out") {
		t.Errorf("Expected timeout error, got: %s", *output.Error)
	}
	// Should timeout around 1 second, not wait full 10 seconds
	if duration > 3*time.Second {
		t.Errorf("Timeout took too long: %v (expected ~1s)", duration)
	}
	// Original payload should be preserved
	if output.Payload["method"] != "test" {
		t.Errorf("Expected original payload preserved, got: %v", output.Payload)
	}
}

// TestExecute_NonZeroExit tests non-zero exit code handling
func TestExecute_NonZeroExit(t *testing.T) {
	// Given: a processor that exits with non-zero code
	tempDir := t.TempDir()
	scriptPath := createTestScript(t, tempDir, "fail.sh", `#!/bin/bash
echo "Error occurred" >&2
exit 1
`)

	executor := &Executor{WorkingDir: tempDir}
	processorConfig := &config.ProcessorConfig{
		Name:    "failing-processor",
		Type:    "cli",
		Enabled: true,
		Timeout: 5,
		Config: map[string]interface{}{
			"command": "bash",
			"args":    []interface{}{scriptPath},
		},
	}

	input := &config.ProcessorInput{
		Type:      "request",
		Timestamp: time.Now().Format(time.RFC3339),
		Payload:   map[string]interface{}{"method": "test"},
		Metadata: config.ProcessorMetadata{
			ProcessorChain:  []string{},
			OriginalPayload: map[string]interface{}{"method": "test"},
		},
	}

	// When: executing the processor
	output, err := executor.Execute(processorConfig, input)

	// Then: returns 500 error with stderr captured
	if err != nil {
		t.Fatalf("Execute should not return error for non-zero exit, got: %v", err)
	}
	if output.Status != 500 {
		t.Errorf("Expected status 500, got %d", output.Status)
	}
	if output.Error == nil {
		t.Error("Expected error message")
	} else if !strings.Contains(*output.Error, "failed") {
		t.Errorf("Expected failure error, got: %s", *output.Error)
	}
	// Original payload should be preserved
	if output.Payload["method"] != "test" {
		t.Errorf("Expected original payload preserved, got: %v", output.Payload)
	}
}

// TestExecute_InvalidJSON tests invalid JSON output handling
func TestExecute_InvalidJSON(t *testing.T) {
	// Given: a processor that returns invalid JSON
	tempDir := t.TempDir()
	scriptPath := createTestScript(t, tempDir, "invalid.sh", `#!/bin/bash
echo "This is not JSON"
`)

	executor := &Executor{WorkingDir: tempDir}
	processorConfig := &config.ProcessorConfig{
		Name:    "invalid-processor",
		Type:    "cli",
		Enabled: true,
		Timeout: 5,
		Config: map[string]interface{}{
			"command": "bash",
			"args":    []interface{}{scriptPath},
		},
	}

	input := &config.ProcessorInput{
		Type:      "request",
		Timestamp: time.Now().Format(time.RFC3339),
		Payload:   map[string]interface{}{"method": "test"},
		Metadata: config.ProcessorMetadata{
			ProcessorChain:  []string{},
			OriginalPayload: map[string]interface{}{"method": "test"},
		},
	}

	// When: executing the processor
	output, err := executor.Execute(processorConfig, input)

	// Then: returns 500 error for invalid JSON
	if err != nil {
		t.Fatalf("Execute should not return error for invalid JSON, got: %v", err)
	}
	if output.Status != 500 {
		t.Errorf("Expected status 500, got %d", output.Status)
	}
	if output.Error == nil {
		t.Error("Expected error message")
	} else if !strings.Contains(*output.Error, "invalid JSON") {
		t.Errorf("Expected invalid JSON error, got: %s", *output.Error)
	}
}

// TestExecute_DisabledProcessor tests disabled processor handling
func TestExecute_DisabledProcessor(t *testing.T) {
	// Given: a disabled processor
	executor := &Executor{WorkingDir: os.TempDir()}
	processorConfig := &config.ProcessorConfig{
		Name:    "disabled-processor",
		Type:    "cli",
		Enabled: false, // Disabled
		Timeout: 5,
		Config: map[string]interface{}{
			"command": "echo",
		},
	}

	input := &config.ProcessorInput{
		Type:    "request",
		Payload: map[string]interface{}{},
		Metadata: config.ProcessorMetadata{
			ProcessorChain:  []string{},
			OriginalPayload: map[string]interface{}{},
		},
	}

	// When: attempting to execute
	output, err := executor.Execute(processorConfig, input)

	// Then: returns error for disabled processor
	if err == nil {
		t.Error("Expected error for disabled processor")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("Expected 'disabled' error, got: %v", err)
	}
	if output != nil {
		t.Error("Expected nil output for disabled processor")
	}
}

// TestExecute_InvalidStatusCode tests invalid status code handling
func TestExecute_InvalidStatusCode(t *testing.T) {
	// Given: a processor that returns invalid status code
	tempDir := t.TempDir()
	scriptPath := createTestScript(t, tempDir, "badstatus.sh", `#!/bin/bash
echo '{"status": 999, "payload": {}, "error": null}'
`)

	executor := &Executor{WorkingDir: tempDir}
	processorConfig := &config.ProcessorConfig{
		Name:    "bad-status",
		Type:    "cli",
		Enabled: true,
		Timeout: 5,
		Config: map[string]interface{}{
			"command": "bash",
			"args":    []interface{}{scriptPath},
		},
	}

	input := &config.ProcessorInput{
		Type:    "request",
		Payload: map[string]interface{}{"method": "test"},
		Metadata: config.ProcessorMetadata{
			ProcessorChain:  []string{},
			OriginalPayload: map[string]interface{}{"method": "test"},
		},
	}

	// When: executing the processor
	output, err := executor.Execute(processorConfig, input)

	// Then: returns 500 error for invalid status
	if err != nil {
		t.Fatalf("Execute should not return error, got: %v", err)
	}
	if output.Status != 500 {
		t.Errorf("Expected status 500, got %d", output.Status)
	}
	if output.Error == nil || !strings.Contains(*output.Error, "invalid status code") {
		t.Errorf("Expected invalid status code error, got: %v", output.Error)
	}
}

// TestExecute_ErrorStatusWithoutMessage tests status >= 400 without error message
func TestExecute_ErrorStatusWithoutMessage(t *testing.T) {
	// Given: a processor that returns 400 without error message
	tempDir := t.TempDir()
	scriptPath := createTestScript(t, tempDir, "noerror.sh", `#!/bin/bash
echo '{"status": 400, "payload": {}, "error": null}'
`)

	executor := &Executor{WorkingDir: tempDir}
	processorConfig := &config.ProcessorConfig{
		Name:    "no-error-msg",
		Type:    "cli",
		Enabled: true,
		Timeout: 5,
		Config: map[string]interface{}{
			"command": "bash",
			"args":    []interface{}{scriptPath},
		},
	}

	input := &config.ProcessorInput{
		Type:    "request",
		Payload: map[string]interface{}{},
		Metadata: config.ProcessorMetadata{
			ProcessorChain:  []string{},
			OriginalPayload: map[string]interface{}{},
		},
	}

	// When: executing the processor
	output, err := executor.Execute(processorConfig, input)

	// Then: error message is added automatically
	if err != nil {
		t.Fatalf("Execute should not return error, got: %v", err)
	}
	if output.Status != 400 {
		t.Errorf("Expected status 400, got %d", output.Status)
	}
	if output.Error == nil {
		t.Error("Expected error message to be added for status >= 400")
	}
}

// TestExecute_JSONInputParsing tests that input is properly marshaled to JSON stdin
func TestExecute_JSONInputParsing(t *testing.T) {
	// Given: a processor that reads stdin and returns success
	// This verifies JSON is properly written to stdin without causing processor failure
	tempDir := t.TempDir()
	scriptPath := createTestScript(t, tempDir, "echo.sh", `#!/bin/bash
# Read input (validates JSON is provided to stdin)
read input
# Verify we got some input
if [ -z "$input" ]; then
  echo '{"status": 500, "payload": {}, "error": "No input received"}'
  exit 0
fi
# Return success to indicate input was received
echo '{"status": 200, "payload": {"input_received": true}, "error": null}'
`)

	executor := &Executor{WorkingDir: tempDir}
	processorConfig := &config.ProcessorConfig{
		Name:    "echo-processor",
		Type:    "cli",
		Enabled: true,
		Timeout: 5,
		Config: map[string]interface{}{
			"command": "bash",
			"args":    []interface{}{scriptPath},
		},
	}

	input := &config.ProcessorInput{
		Type:      "request",
		Timestamp: time.Now().Format(time.RFC3339),
		Connection: config.ConnectionContext{
			ServerName: "test-server",
			Transport:  "stdio",
			SessionID:  "test-session",
		},
		Payload: map[string]interface{}{
			"method": "tools/call",
			"params": map[string]interface{}{"name": "test"},
		},
		Metadata: config.ProcessorMetadata{
			ProcessorChain:  []string{"previous-processor"},
			OriginalPayload: map[string]interface{}{"method": "tools/call"},
		},
	}

	// When: executing the processor
	output, err := executor.Execute(processorConfig, input)

	// Then: processor receives input successfully
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if output.Status != 200 {
		t.Errorf("Expected status 200, got %d (error: %v)", output.Status, output.Error)
	}
	inputReceived, ok := output.Payload["input_received"].(bool)
	if !ok || !inputReceived {
		t.Errorf("Expected input_received=true, got %v", output.Payload)
	}
}

// createTestScript creates a test script file with the given content
func createTestScript(t *testing.T, dir, name, content string) string {
	t.Helper()

	scriptPath := filepath.Join(dir, name)
	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}
	return scriptPath
}

// TestExecute_RejectionFlow tests processor rejecting a request
func TestExecute_RejectionFlow(t *testing.T) {
	// Given: a processor that rejects the request
	tempDir := t.TempDir()
	scriptPath := createTestScript(t, tempDir, "reject.sh", `#!/bin/bash
echo '{"status": 403, "payload": {}, "error": "Request denied by security policy"}'
`)

	executor := &Executor{WorkingDir: tempDir}
	processorConfig := &config.ProcessorConfig{
		Name:    "security-processor",
		Type:    "cli",
		Enabled: true,
		Timeout: 5,
		Config: map[string]interface{}{
			"command": "bash",
			"args":    []interface{}{scriptPath},
		},
	}

	input := &config.ProcessorInput{
		Type:    "request",
		Payload: map[string]interface{}{"method": "dangerous/action"},
		Metadata: config.ProcessorMetadata{
			ProcessorChain:  []string{},
			OriginalPayload: map[string]interface{}{"method": "dangerous/action"},
		},
	}

	// When: executing the processor
	output, err := executor.Execute(processorConfig, input)

	// Then: request is rejected with 403
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if output.Status != 403 {
		t.Errorf("Expected status 403, got %d", output.Status)
	}
	if output.Error == nil {
		t.Error("Expected error message")
	} else if !strings.Contains(*output.Error, "denied") {
		t.Errorf("Expected denial error, got: %s", *output.Error)
	}
}

// TestExecute_WithMetadata tests processor returning metadata
func TestExecute_WithMetadata(t *testing.T) {
	// Given: a processor that returns metadata
	tempDir := t.TempDir()
	scriptPath := createTestScript(t, tempDir, "metadata.sh", `#!/bin/bash
echo '{"status": 200, "payload": {}, "error": null, "metadata": {"execution_time_ms": 42, "checks_passed": 3}}'
`)

	executor := &Executor{WorkingDir: tempDir}
	processorConfig := &config.ProcessorConfig{
		Name:    "metadata-processor",
		Type:    "cli",
		Enabled: true,
		Timeout: 5,
		Config: map[string]interface{}{
			"command": "bash",
			"args":    []interface{}{scriptPath},
		},
	}

	input := &config.ProcessorInput{
		Type:    "request",
		Payload: map[string]interface{}{},
		Metadata: config.ProcessorMetadata{
			ProcessorChain:  []string{},
			OriginalPayload: map[string]interface{}{},
		},
	}

	// When: executing the processor
	output, err := executor.Execute(processorConfig, input)

	// Then: metadata is preserved in output
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if output.Metadata == nil {
		t.Fatal("Expected metadata in output")
	}

	// Check metadata values (JSON unmarshal converts numbers to float64)
	execTime, ok := output.Metadata["execution_time_ms"].(float64)
	if !ok || execTime != 42 {
		t.Errorf("Expected execution_time_ms=42, got %v", output.Metadata["execution_time_ms"])
	}

	checksPassed, ok := output.Metadata["checks_passed"].(float64)
	if !ok || checksPassed != 3 {
		t.Errorf("Expected checks_passed=3, got %v", output.Metadata["checks_passed"])
	}
}

// TestExecute_UnsupportedType tests unsupported processor type
func TestExecute_UnsupportedType(t *testing.T) {
	// Given: a processor with unsupported type
	executor := &Executor{WorkingDir: os.TempDir()}
	processorConfig := &config.ProcessorConfig{
		Name:    "http-processor",
		Type:    "http", // Not supported in v1
		Enabled: true,
		Timeout: 5,
		Config: map[string]interface{}{
			"url": "http://example.com",
		},
	}

	input := &config.ProcessorInput{
		Type:    "request",
		Payload: map[string]interface{}{},
		Metadata: config.ProcessorMetadata{
			ProcessorChain:  []string{},
			OriginalPayload: map[string]interface{}{},
		},
	}

	// When: attempting to execute
	output, err := executor.Execute(processorConfig, input)

	// Then: returns error for unsupported type
	if err == nil {
		t.Error("Expected error for unsupported type")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("Expected 'unsupported' error, got: %v", err)
	}
	if output != nil {
		t.Error("Expected nil output for unsupported type")
	}
}

// TestExecute_NullPayloadHandling tests processor not returning payload
func TestExecute_NullPayloadHandling(t *testing.T) {
	// Given: a processor that doesn't return payload
	tempDir := t.TempDir()
	scriptPath := createTestScript(t, tempDir, "nopayload.sh", `#!/bin/bash
echo '{"status": 200, "error": null}'
`)

	executor := &Executor{WorkingDir: tempDir}
	processorConfig := &config.ProcessorConfig{
		Name:    "no-payload",
		Type:    "cli",
		Enabled: true,
		Timeout: 5,
		Config: map[string]interface{}{
			"command": "bash",
			"args":    []interface{}{scriptPath},
		},
	}

	input := &config.ProcessorInput{
		Type:    "request",
		Payload: map[string]interface{}{"method": "test", "id": 123},
		Metadata: config.ProcessorMetadata{
			ProcessorChain:  []string{},
			OriginalPayload: map[string]interface{}{"method": "test", "id": 123},
		},
	}

	// When: executing the processor
	output, err := executor.Execute(processorConfig, input)

	// Then: original payload is used
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if output.Payload == nil {
		t.Fatal("Expected payload to be set")
	}
	if output.Payload["method"] != "test" {
		t.Errorf("Expected original payload preserved, got: %v", output.Payload)
	}

	// Verify id is preserved (could be int or float64 depending on marshaling)
	idValue := output.Payload["id"]
	switch v := idValue.(type) {
	case int:
		if v != 123 {
			t.Errorf("Expected id=123, got %d", v)
		}
	case float64:
		if v != 123 {
			t.Errorf("Expected id=123, got %f", v)
		}
	default:
		t.Errorf("Expected id to be int or float64, got type %T with value %v", idValue, idValue)
	}
}
