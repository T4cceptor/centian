package processor

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
	"github.com/CentianAI/centian-cli/internal/config"
)

// TestNewChain tests processor chain creation.
func TestNewChain(t *testing.T) {
	tests := []struct {
		name       string
		processors []*config.ProcessorConfig
		serverName string
		sessionID  string
		wantError  bool
	}{
		{
			name: "successful chain creation with one processor",
			processors: []*config.ProcessorConfig{
				{Name: "processor1", Type: "cli", Enabled: true},
			},
			serverName: "test-server",
			sessionID:  "session-123",
			wantError:  false,
		},
		{
			name: "successful chain creation with multiple processors",
			processors: []*config.ProcessorConfig{
				{Name: "processor1", Type: "cli", Enabled: true},
				{Name: "processor2", Type: "cli", Enabled: false},
				{Name: "processor3", Type: "cli", Enabled: true},
			},
			serverName: "test-server",
			sessionID:  "session-456",
			wantError:  false,
		},
		{
			name:       "successful chain creation with no processors",
			processors: []*config.ProcessorConfig{},
			serverName: "test-server",
			sessionID:  "session-789",
			wantError:  false,
		},
		{
			name:       "successful chain creation with nil processors",
			processors: nil,
			serverName: "test-server",
			sessionID:  "session-000",
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: processor configurations.

			// When: creating a new chain.
			chain, err := NewChain(tt.processors, tt.serverName, tt.sessionID)

			// Then: verify creation result.
			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				if chain != nil {
					t.Error("Expected nil chain on error")
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got: %v", err)
				}
				if chain == nil {
					t.Fatal("Expected non-nil chain")
				}
				if chain != nil && chain.serverName != tt.serverName {
					t.Errorf("Expected serverName '%s', got '%s'", tt.serverName, chain.serverName)
				}
				if chain != nil && chain.sessionID != tt.sessionID {
					t.Errorf("Expected sessionID '%s', got '%s'", tt.sessionID, chain.sessionID)
				}
				if chain != nil && chain.executor == nil {
					t.Error("Expected non-nil executor")
				}
			}
		})
	}
}

// TestHasProcessors tests processor presence checking.
func TestHasProcessors(t *testing.T) {
	tests := []struct {
		name       string
		chain      *Chain
		processors []*config.ProcessorConfig
		want       bool
	}{
		{
			name:  "nil chain returns false",
			chain: nil,
			want:  false,
		},
		{
			name: "empty processors returns false",
			chain: &Chain{
				processors: []*config.ProcessorConfig{},
			},
			want: false,
		},
		{
			name: "all disabled processors returns false",
			chain: &Chain{
				processors: []*config.ProcessorConfig{
					{Name: "disabled1", Enabled: false},
					{Name: "disabled2", Enabled: false},
				},
			},
			want: false,
		},
		{
			name: "one enabled processor returns true",
			chain: &Chain{
				processors: []*config.ProcessorConfig{
					{Name: "disabled1", Enabled: false},
					{Name: "enabled1", Enabled: true},
					{Name: "disabled2", Enabled: false},
				},
			},
			want: true,
		},
		{
			name: "all enabled processors returns true",
			chain: &Chain{
				processors: []*config.ProcessorConfig{
					{Name: "enabled1", Enabled: true},
					{Name: "enabled2", Enabled: true},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a chain configuration.

			// When: checking if chain has processors.
			got := tt.chain.HasProcessors()

			// Then: verify result.
			if got != tt.want {
				t.Errorf("HasProcessors() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExecute_SingleProcessor tests chain execution with single processor.
func TestExecute_SingleProcessor(t *testing.T) {
	tests := []struct {
		name            string
		processorScript string
		expectedStatus  int
		expectError     bool
		validateResult  func(*testing.T, *ChainResult)
	}{
		{
			name: "successful processor execution",
			processorScript: `#!/bin/bash
read input
echo '{"status": 200, "payload": {"result": "success"}, "error": null}'
`,
			expectedStatus: 200,
			expectError:    false,
			validateResult: func(t *testing.T, result *ChainResult) {
				if result.Error != nil {
					t.Errorf("Expected no error, got: %s", *result.Error)
				}
				if len(result.ProcessorChain) != 1 {
					t.Errorf("Expected 1 processor in chain, got %d", len(result.ProcessorChain))
				}
			},
		},
		{
			name: "processor rejection with 403",
			processorScript: `#!/bin/bash
read input
echo '{"status": 403, "payload": {}, "error": "Access denied"}'
`,
			expectedStatus: 403,
			expectError:    true,
			validateResult: func(t *testing.T, result *ChainResult) {
				if result.Error == nil {
					t.Error("Expected error message")
				} else if !strings.Contains(*result.Error, "denied") {
					t.Errorf("Expected denial error, got: %s", *result.Error)
				}
			},
		},
		{
			name: "processor error with 500",
			processorScript: `#!/bin/bash
read input
echo '{"status": 500, "payload": {}, "error": "Internal processor error"}'
`,
			expectedStatus: 500,
			expectError:    true,
			validateResult: func(t *testing.T, result *ChainResult) {
				if result.Error == nil {
					t.Error("Expected error message")
				}
			},
		},
		{
			name: "processor with metadata",
			processorScript: `#!/bin/bash
read input
echo '{"status": 200, "payload": {}, "error": null, "metadata": {"exec_time": 42}}'
`,
			expectedStatus: 200,
			expectError:    false,
			validateResult: func(t *testing.T, result *ChainResult) {
				if result.Metadata == nil {
					t.Fatal("Expected metadata")
				}
				if _, ok := result.Metadata["test-processor"]; !ok {
					t.Error("Expected processor metadata to be aggregated")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a processor chain with one processor.
			tempDir := t.TempDir()
			scriptPath := createTestScript(t, tempDir, "processor.sh", tt.processorScript)

			chain, err := NewChain([]*config.ProcessorConfig{
				{
					Name:    "test-processor",
					Type:    "cli",
					Enabled: true,
					Timeout: 5,
					Config: map[string]interface{}{
						"command": "bash",
						"args":    []interface{}{scriptPath},
					},
				},
			}, "test-server", "session-123")
			if err != nil {
				t.Fatalf("Failed to create chain: %v", err)
			}
			chain.executor.WorkingDir = tempDir

			event := createTestEvent(`{"method": "tools/list"}`)

			// When: executing the chain.
			result, err := chain.Execute(event)

			// Then: verify execution result.
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			if result.Status != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, result.Status)
			}
			if tt.expectError && result.Error == nil {
				t.Error("Expected error message")
			}
			if !tt.expectError && result.Error != nil {
				t.Errorf("Expected no error, got: %s", *result.Error)
			}
			if tt.validateResult != nil {
				tt.validateResult(t, result)
			}
		})
	}
}

// TestExecute_MultipleProcessors tests chain with multiple processors.
func TestExecute_MultipleProcessors(t *testing.T) {
	// Given: a chain with three processors.
	tempDir := t.TempDir()

	// Processor 1: Adds "processed_by_1" field.
	script1 := createTestScript(t, tempDir, "proc1.sh", `#!/bin/bash
read input
echo '{"status": 200, "payload": {"processed_by_1": true}, "error": null, "metadata": {"processor": "1"}}'
`)

	// Processor 2: Adds "processed_by_2" field.
	script2 := createTestScript(t, tempDir, "proc2.sh", `#!/bin/bash
read input
echo '{"status": 200, "payload": {"processed_by_2": true}, "error": null, "metadata": {"processor": "2"}}'
`)

	// Processor 3: Adds "processed_by_3" field.
	script3 := createTestScript(t, tempDir, "proc3.sh", `#!/bin/bash
read input
echo '{"status": 200, "payload": {"processed_by_3": true}, "error": null, "metadata": {"processor": "3"}}'
`)

	chain, err := NewChain([]*config.ProcessorConfig{
		{
			Name:    "processor1",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config:  map[string]interface{}{"command": "bash", "args": []interface{}{script1}},
		},
		{
			Name:    "processor2",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config:  map[string]interface{}{"command": "bash", "args": []interface{}{script2}},
		},
		{
			Name:    "processor3",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config:  map[string]interface{}{"command": "bash", "args": []interface{}{script3}},
		},
	}, "test-server", "session-123")
	if err != nil {
		t.Fatalf("Failed to create chain: %v", err)
	}
	chain.executor.WorkingDir = tempDir

	event := createTestEvent(`{"method": "tools/list"}`)

	// When: executing the chain.
	result, err := chain.Execute(event)

	// Then: all processors execute successfully.
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Status != 200 {
		t.Errorf("Expected status 200, got %d", result.Status)
	}
	if result.Error != nil {
		t.Errorf("Expected no error, got: %s", *result.Error)
	}

	// Then: all processors are in the chain.
	if len(result.ProcessorChain) != 3 {
		t.Errorf("Expected 3 processors in chain, got %d", len(result.ProcessorChain))
	}
	expectedChain := []string{"processor1", "processor2", "processor3"}
	for i, name := range expectedChain {
		if i >= len(result.ProcessorChain) || result.ProcessorChain[i] != name {
			t.Errorf("Expected processor[%d]='%s', got '%v'", i, name, result.ProcessorChain)
		}
	}

	// Then: metadata from all processors is aggregated.
	if len(result.Metadata) != 3 {
		t.Errorf("Expected metadata from 3 processors, got %d", len(result.Metadata))
	}
}

// TestExecute_EarlyProcessorFailure tests chain stopping on processor failure.
func TestExecute_EarlyProcessorFailure(t *testing.T) {
	// Given: a chain where second processor fails.
	tempDir := t.TempDir()

	script1 := createTestScript(t, tempDir, "proc1.sh", `#!/bin/bash
read input
echo '{"status": 200, "payload": {"step1": true}, "error": null}'
`)

	script2 := createTestScript(t, tempDir, "proc2.sh", `#!/bin/bash
read input
echo '{"status": 403, "payload": {}, "error": "Rejected by processor 2"}'
`)

	script3 := createTestScript(t, tempDir, "proc3.sh", `#!/bin/bash
read input
echo '{"status": 200, "payload": {"step3": true}, "error": null}'
`)

	chain, err := NewChain([]*config.ProcessorConfig{
		{
			Name:    "processor1",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config:  map[string]interface{}{"command": "bash", "args": []interface{}{script1}},
		},
		{
			Name:    "processor2",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config:  map[string]interface{}{"command": "bash", "args": []interface{}{script2}},
		},
		{
			Name:    "processor3",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config:  map[string]interface{}{"command": "bash", "args": []interface{}{script3}},
		},
	}, "test-server", "session-123")
	if err != nil {
		t.Fatalf("Failed to create chain: %v", err)
	}
	chain.executor.WorkingDir = tempDir

	event := createTestEvent(`{"method": "test"}`)

	// When: executing the chain.
	result, err := chain.Execute(event)

	// Then: chain stops at second processor.
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Status != 403 {
		t.Errorf("Expected status 403, got %d", result.Status)
	}
	if result.Error == nil {
		t.Error("Expected error message")
	}

	// Then: only first two processors executed.
	if len(result.ProcessorChain) != 2 {
		t.Errorf("Expected 2 processors in chain, got %d", len(result.ProcessorChain))
	}
	if len(result.ProcessorChain) >= 2 && result.ProcessorChain[1] != "processor2" {
		t.Errorf("Expected processor2 as last in chain, got %v", result.ProcessorChain)
	}
}

// TestExecute_DisabledProcessorsSkipped tests disabled processor handling.
func TestExecute_DisabledProcessorsSkipped(t *testing.T) {
	// Given: a chain with disabled processors.
	tempDir := t.TempDir()

	script1 := createTestScript(t, tempDir, "proc1.sh", `#!/bin/bash
read input
echo '{"status": 200, "payload": {"p1": true}, "error": null}'
`)

	script2 := createTestScript(t, tempDir, "proc2.sh", `#!/bin/bash
read input
echo '{"status": 500, "payload": {}, "error": "Should not execute"}'
`)

	script3 := createTestScript(t, tempDir, "proc3.sh", `#!/bin/bash
read input
echo '{"status": 200, "payload": {"p3": true}, "error": null}'
`)

	chain, err := NewChain([]*config.ProcessorConfig{
		{
			Name:    "processor1",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config:  map[string]interface{}{"command": "bash", "args": []interface{}{script1}},
		},
		{
			Name:    "processor2-disabled",
			Type:    "cli",
			Enabled: false, // Disabled
			Timeout: 5,
			Config:  map[string]interface{}{"command": "bash", "args": []interface{}{script2}},
		},
		{
			Name:    "processor3",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config:  map[string]interface{}{"command": "bash", "args": []interface{}{script3}},
		},
	}, "test-server", "session-123")
	if err != nil {
		t.Fatalf("Failed to create chain: %v", err)
	}
	chain.executor.WorkingDir = tempDir

	event := createTestEvent(`{"method": "test"}`)

	// When: executing the chain.
	result, err := chain.Execute(event)

	// Then: disabled processor is skipped.
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Status != 200 {
		t.Errorf("Expected status 200, got %d", result.Status)
	}

	// Then: only enabled processors in chain.
	if len(result.ProcessorChain) != 2 {
		t.Errorf("Expected 2 processors in chain (skipping disabled), got %d", len(result.ProcessorChain))
	}
	for _, name := range result.ProcessorChain {
		if name == "processor2-disabled" {
			t.Error("Disabled processor should not be in chain")
		}
	}
}

// TestExecute_ChainInvalidJSON tests handling of invalid JSON input in chain.
func TestExecute_ChainInvalidJSON(t *testing.T) {
	// Given: a chain with valid processor.
	tempDir := t.TempDir()
	chain, err := NewChain([]*config.ProcessorConfig{
		{
			Name:    "processor1",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config:  map[string]interface{}{"command": "echo", "args": []interface{}{"test"}},
		},
	}, "test-server", "session-123")
	if err != nil {
		t.Fatalf("Failed to create chain: %v", err)
	}
	chain.executor.WorkingDir = tempDir

	// Invalid JSON event.
	event := createTestEvent("not valid json {{{")

	// When: executing the chain.
	result, err := chain.Execute(event)

	// Then: returns error for invalid JSON.
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "JSON") {
		t.Errorf("Expected JSON error, got: %v", err)
	}
	if result != nil {
		t.Error("Expected nil result on JSON parse error")
	}
}

// TestFormatMCPError tests MCP error response formatting.
func TestFormatMCPError(t *testing.T) {
	tests := []struct {
		name         string
		result       *ChainResult
		requestID    interface{}
		wantError    bool
		validateJSON func(*testing.T, map[string]interface{})
		expectedCode int
		expectedMsg  string
	}{
		{
			name: "format 500 internal error",
			result: &ChainResult{
				Status:         500,
				Error:          strPtr("Processor execution failed"),
				ProcessorChain: []string{"processor1"},
				Metadata:       map[string]interface{}{"detail": "timeout"},
			},
			requestID:    "req-123",
			wantError:    false,
			expectedCode: -32603,
			expectedMsg:  "Request processing failed",
			validateJSON: func(t *testing.T, data map[string]interface{}) {
				if errorObj, ok := data["error"].(map[string]interface{}); ok {
					if errorData, ok := errorObj["data"].(map[string]interface{}); ok {
						if reason, ok := errorData["rejection_reason"].(string); !ok || reason != "Processor execution failed" {
							t.Errorf("Expected rejection_reason='Processor execution failed', got %v", errorData["rejection_reason"])
						}
					}
				}
			},
		},
		{
			name: "format 403 rejection",
			result: &ChainResult{
				Status:         403,
				Error:          strPtr("Access denied"),
				ProcessorChain: []string{"security-processor"},
			},
			requestID:    "req-456",
			wantError:    false,
			expectedCode: -32001,
			expectedMsg:  "Request rejected by processor",
		},
		{
			name: "format error without error message",
			result: &ChainResult{
				Status:         400,
				ProcessorChain: []string{"validator"},
			},
			requestID:    "req-789",
			wantError:    false,
			expectedCode: -32001,
		},
		{
			name: "non-error status should fail",
			result: &ChainResult{
				Status: 200,
			},
			requestID: "req-000",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a chain result.

			// When: formatting as MCP error.
			jsonStr, err := FormatMCPError(tt.result, tt.requestID)

			// Then: verify formatting result.
			if tt.wantError {
				if err == nil {
					t.Error("Expected error for non-error status")
				}
				return
			}

			if err != nil {
				t.Fatalf("FormatMCPError failed: %v", err)
			}

			// Then: parse and validate JSON structure.
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
				t.Fatalf("Failed to parse JSON: %v\nJSON: %s", err, jsonStr)
			}

			// Verify JSON-RPC structure.
			if parsed["jsonrpc"] != "2.0" {
				t.Errorf("Expected jsonrpc='2.0', got %v", parsed["jsonrpc"])
			}
			if parsed["id"] != tt.requestID {
				t.Errorf("Expected id='%v', got %v", tt.requestID, parsed["id"])
			}

			// Verify error object.
			errorObj, ok := parsed["error"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected error object in response")
			}

			// Verify error code (JSON numbers are float64).
			if code, ok := errorObj["code"].(float64); !ok || int(code) != tt.expectedCode {
				t.Errorf("Expected error code %d, got %v", tt.expectedCode, errorObj["code"])
			}

			// Verify error message.
			if tt.expectedMsg != "" {
				if msg, ok := errorObj["message"].(string); !ok || msg != tt.expectedMsg {
					t.Errorf("Expected message='%s', got %v", tt.expectedMsg, errorObj["message"])
				}
			}

			// Verify error data structure.
			errorData, ok := errorObj["data"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected data object in error")
			}

			if _, ok := errorData["processor_chain"]; !ok {
				t.Error("Expected processor_chain in error data")
			}

			if tt.validateJSON != nil {
				tt.validateJSON(t, parsed)
			}
		})
	}
}

// Helper functions.

// createTestEvent creates a test MCPEvent with the given JSON message.
func createTestEvent(jsonMessage string) common.McpEventInterface {
	result := &common.MCPEvent{
		BaseMcpEvent: common.BaseMcpEvent{
			MessageType: common.MessageTypeRequest,
			Transport:   "stdio",
			SessionID:   "test-session",
			Timestamp:   time.Now(),
		},
	}
	result.SetRawMessage(jsonMessage)
	return result
}

// strPtr returns a pointer to a string.
func strPtr(s string) *string {
	return &s
}

// TestExecute_ProcessorExecutionFailure tests processor execution errors.
func TestExecute_ProcessorExecutionFailure(t *testing.T) {
	// Given: a chain with a processor that fails to execute (not timeout, but execution error).
	tempDir := t.TempDir()

	// Use nonexistent command to trigger execution failure.
	chain, err := NewChain([]*config.ProcessorConfig{
		{
			Name:    "failing-processor",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config: map[string]interface{}{
				"command": "nonexistent-command-xyz",
				"args":    []interface{}{"arg1"},
			},
		},
	}, "test-server", "session-123")
	if err != nil {
		t.Fatalf("Failed to create chain: %v", err)
	}
	chain.executor.WorkingDir = tempDir

	event := createTestEvent(`{"method": "test"}`)

	// When: executing the chain.
	result, err := chain.Execute(event)

	// Then: execution returns 500 error.
	if err != nil {
		t.Fatalf("Execute should not return error, got: %v", err)
	}
	if result.Status != 500 {
		t.Errorf("Expected status 500, got %d", result.Status)
	}
	if result.Error == nil {
		t.Error("Expected error message")
	} else if !strings.Contains(*result.Error, "execution failed") {
		t.Errorf("Expected execution failure error, got: %s", *result.Error)
	}
}

// TestExecute_ChainPayloadModification tests payload changes through chain.
func TestExecute_ChainPayloadModification(t *testing.T) {
	// Given: processors that modify payload incrementally.
	tempDir := t.TempDir()

	// Processor 1: Sets field "a" to 1.
	script1 := createTestScript(t, tempDir, "mod1.sh", `#!/bin/bash
read input
echo '{"status": 200, "payload": {"a": 1}, "error": null}'
`)

	// Processor 2: Receives {"a": 1}, adds "b": 2.
	script2 := createTestScript(t, tempDir, "mod2.sh", `#!/bin/bash
read input
# In real scenario, processor would read input and merge, but for test we just set both
echo '{"status": 200, "payload": {"a": 1, "b": 2}, "error": null}'
`)

	// Processor 3: Receives {"a": 1, "b": 2}, adds "c": 3.
	script3 := createTestScript(t, tempDir, "mod3.sh", `#!/bin/bash
read input
echo '{"status": 200, "payload": {"a": 1, "b": 2, "c": 3}, "error": null}'
`)

	chain, err := NewChain([]*config.ProcessorConfig{
		{
			Name:    "mod1",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config:  map[string]interface{}{"command": "bash", "args": []interface{}{script1}},
		},
		{
			Name:    "mod2",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config:  map[string]interface{}{"command": "bash", "args": []interface{}{script2}},
		},
		{
			Name:    "mod3",
			Type:    "cli",
			Enabled: true,
			Timeout: 5,
			Config:  map[string]interface{}{"command": "bash", "args": []interface{}{script3}},
		},
	}, "test-server", "session-123")
	if err != nil {
		t.Fatalf("Failed to create chain: %v", err)
	}
	chain.executor.WorkingDir = tempDir

	event := createTestEvent(`{"initial": "payload"}`)

	// When: executing the chain.
	result, err := chain.Execute(event)

	// Then: final payload has all modifications.
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Status != 200 {
		t.Errorf("Expected status 200, got %d", result.Status)
	}

	// Verify payload has all three fields (JSON numbers come back as float64).
	payload := result.ModifiedPayload
	if a, ok := payload["a"].(float64); !ok || a != 1 {
		t.Errorf("Expected payload.a=1, got %v", payload["a"])
	}
	if b, ok := payload["b"].(float64); !ok || b != 2 {
		t.Errorf("Expected payload.b=2, got %v", payload["b"])
	}
	if c, ok := payload["c"].(float64); !ok || c != 3 {
		t.Errorf("Expected payload.c=3, got %v", payload["c"])
	}
}

// TestFormatMCPError_RequestIDTypes tests different request ID types.
func TestFormatMCPError_RequestIDTypes(t *testing.T) {
	tests := []struct {
		name      string
		requestID interface{}
	}{
		{
			name:      "string request ID",
			requestID: "req-string-123",
		},
		{
			name:      "numeric request ID",
			requestID: 42,
		},
		{
			name:      "nil request ID",
			requestID: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a chain result with error.
			result := &ChainResult{
				Status:         500,
				Error:          strPtr("Test error"),
				ProcessorChain: []string{"test"},
			}

			// When: formatting as MCP error.
			jsonStr, err := FormatMCPError(result, tt.requestID)

			// Then: formatting succeeds and includes request ID.
			if err != nil {
				t.Fatalf("FormatMCPError failed: %v", err)
			}

			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
				t.Fatalf("Failed to parse JSON: %v", err)
			}

			// Verify request ID is preserved (might be different type after JSON marshal/unmarshal).
			idValue := parsed["id"]
			if tt.requestID == nil && idValue != nil {
				t.Errorf("Expected nil id, got %v", idValue)
			}
			// For non-nil values, just verify id field exists.
			if tt.requestID != nil && parsed["id"] == nil {
				t.Error("Expected non-nil id field")
			}
		})
	}
}

// TestExecute_EmptyProcessorChain tests chain with no enabled processors.
func TestExecute_EmptyProcessorChain(t *testing.T) {
	// Given: a chain with no enabled processors.
	chain, err := NewChain([]*config.ProcessorConfig{
		{Name: "disabled1", Type: "cli", Enabled: false},
		{Name: "disabled2", Type: "cli", Enabled: false},
	}, "test-server", "session-123")
	if err != nil {
		t.Fatalf("Failed to create chain: %v", err)
	}

	event := createTestEvent(`{"method": "test", "value": 123}`)

	// When: executing the chain.
	result, err := chain.Execute(event)

	// Then: execution succeeds with original payload.
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Status != 200 {
		t.Errorf("Expected status 200, got %d", result.Status)
	}
	if result.Error != nil {
		t.Errorf("Expected no error, got: %s", *result.Error)
	}
	if len(result.ProcessorChain) != 0 {
		t.Errorf("Expected empty processor chain, got %d processors", len(result.ProcessorChain))
	}

	// Verify original payload preserved (JSON numbers are float64).
	if method, ok := result.ModifiedPayload["method"].(string); !ok || method != "test" {
		t.Errorf("Expected method='test', got %v", result.ModifiedPayload["method"])
	}
	if value, ok := result.ModifiedPayload["value"].(float64); !ok || value != 123 {
		t.Errorf("Expected value=123, got %v", result.ModifiedPayload["value"])
	}
}
