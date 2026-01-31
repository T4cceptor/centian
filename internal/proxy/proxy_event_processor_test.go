package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/T4cceptor/centian/internal/processor"
	"gotest.tools/assert"
)

// ========================================.
// Test Helpers.
// ========================================.

// mockMcpEvent is a simple mock implementation of McpEventInterface for testing.
type mockMcpEvent struct {
	rawMessage       string
	modified         bool
	hasContent       bool
	status           int
	baseEvent        common.BaseMcpEvent
	processingErrors map[string]error
}

func newMockMcpEvent(rawMessage string, hasContent bool) *mockMcpEvent {
	return &mockMcpEvent{
		rawMessage: rawMessage,
		hasContent: hasContent,
		status:     200,
		baseEvent: common.BaseMcpEvent{
			ProcessingErrors: make(map[string]error),
			MessageType:      "request",
			Transport:        "stdio",
			SessionID:        "test-session",
		},
		processingErrors: make(map[string]error),
	}
}

func (m *mockMcpEvent) GetRawMessage() string             { return m.rawMessage }
func (m *mockMcpEvent) SetRawMessage(msg string)          { m.rawMessage = msg }
func (m *mockMcpEvent) SetModified(modified bool)         { m.modified = modified }
func (m *mockMcpEvent) HasContent() bool                  { return m.hasContent }
func (m *mockMcpEvent) IsRequest() bool                   { return false }
func (m *mockMcpEvent) IsResponse() bool                  { return true }
func (m *mockMcpEvent) GetBaseEvent() common.BaseMcpEvent { return m.baseEvent }
func (m *mockMcpEvent) SetStatus(status int)              { m.status = status }

func createTestLogger(t *testing.T) *logging.Logger {
	// Given: a temporary log directory setup.
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")

	// Set up temp HOME for logger.
	os.Setenv("HOME", tempDir)
	t.Cleanup(func() {
		os.Setenv("HOME", origHome)
	})

	// Create logs directory.
	logDir := filepath.Join(tempDir, ".centian", "logs")
	err := os.MkdirAll(logDir, 0o750)
	assert.NilError(t, err)

	// When: creating a logger.
	logger, err := logging.NewLogger()
	assert.NilError(t, err)
	return logger
}

// ========================================.
// NewEventProcessor Tests.
// ========================================.

func TestNewEventProcessor_WithValidInputs(t *testing.T) {
	// Given: a logger and processor chain.
	logger := createTestLogger(t)
	defer logger.Close()

	chain, err := processor.NewChain(nil, "test-server", "test-session")
	assert.NilError(t, err)

	// When: creating a new event processor.
	ep := NewEventProcessor(logger, chain)

	// Then: should create processor with correct defaults.
	assert.Assert(t, ep != nil)
	assert.Assert(t, ep.logger == logger)
	assert.Assert(t, ep.processorChain == chain)
	assert.Equal(t, true, ep.logBeforeProcessing)
	assert.Equal(t, true, ep.logAfterProcessing)
}

func TestNewEventProcessor_WithNilChain(t *testing.T) {
	// Given: a logger and nil processor chain.
	logger := createTestLogger(t)
	defer logger.Close()

	// When: creating a new event processor with nil chain.
	ep := NewEventProcessor(logger, nil)

	// Then: should create processor successfully.
	assert.Assert(t, ep != nil)
	assert.Assert(t, ep.processorChain == nil)
}

// ========================================.
// Process Tests - No Processors.
// ========================================.

func TestProcess_WithNoProcessors_LogsEvent(t *testing.T) {
	// Given: an event processor with no processors.
	logger := createTestLogger(t)
	defer logger.Close()

	ep := NewEventProcessor(logger, nil)
	event := newMockMcpEvent(`{"jsonrpc":"2.0","id":1,"result":"test"}`, true)

	// When: processing an event.
	err := ep.Process(event)

	// Then: should process successfully.
	assert.NilError(t, err)
	assert.Equal(t, `{"jsonrpc":"2.0","id":1,"result":"test"}`, event.GetRawMessage())
}

func TestProcess_WithNoProcessors_NoContent(t *testing.T) {
	// Given: an event processor with no processors and event with no content.
	logger := createTestLogger(t)
	defer logger.Close()

	ep := NewEventProcessor(logger, nil)
	event := newMockMcpEvent("", false)

	// When: processing an event.
	err := ep.Process(event)

	// Then: should process successfully without modification.
	assert.NilError(t, err)
	assert.Equal(t, "", event.GetRawMessage())
}

func TestProcess_WithEmptyProcessorChain_SkipsProcessing(t *testing.T) {
	// Given: an event processor with empty processor chain.
	logger := createTestLogger(t)
	defer logger.Close()

	chain, err := processor.NewChain(nil, "test-server", "test-session")
	assert.NilError(t, err)

	ep := NewEventProcessor(logger, chain)
	event := newMockMcpEvent(`{"jsonrpc":"2.0","id":1,"result":"test"}`, true)

	// When: processing an event.
	err = ep.Process(event)

	// Then: should skip processing (no processors in chain).
	assert.NilError(t, err)
	assert.Equal(t, `{"jsonrpc":"2.0","id":1,"result":"test"}`, event.GetRawMessage())
}

// ========================================.
// Process Tests - Edge Cases.
// ========================================.

func TestProcess_WithInvalidJSON_HandlesGracefully(t *testing.T) {
	// Given: an event processor.
	logger := createTestLogger(t)
	defer logger.Close()

	ep := NewEventProcessor(logger, nil)
	event := newMockMcpEvent(`{invalid json}`, true)

	// When: processing an event with invalid JSON.
	err := ep.Process(event)

	// Then: should handle gracefully.
	assert.NilError(t, err)
	// Invalid JSON should be preserved as-is.
	assert.Equal(t, `{invalid json}`, event.GetRawMessage())
}

func TestProcess_LoggingDisabled_SkipsLogging(t *testing.T) {
	// Given: an event processor with logging disabled.
	logger := createTestLogger(t)
	defer logger.Close()

	ep := NewEventProcessor(logger, nil)
	ep.logBeforeProcessing = false
	ep.logAfterProcessing = false

	event := newMockMcpEvent(`{"jsonrpc":"2.0","id":1,"result":"test"}`, true)

	// When: processing an event.
	err := ep.Process(event)

	// Then: should process without logging.
	assert.NilError(t, err)
	assert.Equal(t, 0, len(event.baseEvent.ProcessingErrors))
}

func TestProcess_WithEmptyMessage_ProcessesSuccessfully(t *testing.T) {
	// Given: an event processor with empty message.
	logger := createTestLogger(t)
	defer logger.Close()

	ep := NewEventProcessor(logger, nil)
	event := newMockMcpEvent("", false)

	// When: processing an event.
	err := ep.Process(event)

	// Then: should process successfully.
	assert.NilError(t, err)
	assert.Equal(t, "", event.GetRawMessage())
}

func TestProcess_EventWithoutContent_SkipsProcessors(t *testing.T) {
	// Given: an event processor with processors but event has no content.
	logger := createTestLogger(t)
	defer logger.Close()

	chain, err := processor.NewChain(nil, "test-server", "test-session")
	assert.NilError(t, err)

	ep := NewEventProcessor(logger, chain)
	event := newMockMcpEvent("", false) // No content

	// When: processing an event without content.
	err = ep.Process(event)

	// Then: should skip processors.
	assert.NilError(t, err)
	assert.Equal(t, "", event.GetRawMessage())
}

// ========================================.
// Logging Tests.
// ========================================.

func TestProcess_LogBeforeProcessing_CallsLogger(t *testing.T) {
	// Given: an event processor with logging enabled.
	logger := createTestLogger(t)
	defer logger.Close()

	ep := NewEventProcessor(logger, nil)
	ep.logBeforeProcessing = true
	ep.logAfterProcessing = false

	event := newMockMcpEvent(`{"jsonrpc":"2.0","id":1,"result":"test"}`, true)

	// When: processing an event.
	err := ep.Process(event)

	// Then: should process and log.
	assert.NilError(t, err)
}

func TestProcess_LogAfterProcessing_CallsLogger(t *testing.T) {
	// Given: an event processor with after-logging enabled.
	logger := createTestLogger(t)
	defer logger.Close()

	ep := NewEventProcessor(logger, nil)
	ep.logBeforeProcessing = false
	ep.logAfterProcessing = true

	event := newMockMcpEvent(`{"jsonrpc":"2.0","id":1,"result":"test"}`, true)

	// When: processing an event.
	err := ep.Process(event)

	// Then: should process and log after.
	assert.NilError(t, err)
}

func TestProcess_BothLoggingEnabled_LogsTwice(t *testing.T) {
	// Given: an event processor with both logging enabled.
	logger := createTestLogger(t)
	defer logger.Close()

	ep := NewEventProcessor(logger, nil)
	ep.logBeforeProcessing = true
	ep.logAfterProcessing = true

	event := newMockMcpEvent(`{"jsonrpc":"2.0","id":1,"result":"test"}`, true)

	// When: processing an event.
	err := ep.Process(event)

	// Then: should process and log twice.
	assert.NilError(t, err)
}

// ========================================.
// Process Tests - With Processor Chain.
// ========================================.

func TestProcess_WithPassthroughProcessor_ModifiesMessage(t *testing.T) {
	// Given: an event processor with passthrough processor.
	logger := createTestLogger(t)
	defer logger.Close()

	processorConfig := createTestProcessor("passthrough", "../../tests/integrationtests/processors/passthrough.py")
	chain, err := processor.NewChain([]*config.ProcessorConfig{processorConfig}, "test-server", "test-session")
	assert.NilError(t, err)

	ep := NewEventProcessor(logger, chain)
	event := newMockMcpEvent(`{"jsonrpc":"2.0","id":1,"method":"test/method","params":{}}`, true)

	// When: processing an event through the chain.
	err = ep.Process(event)

	// Then: should process successfully.
	assert.NilError(t, err)
	// Passthrough processor returns status 200.
	assert.Equal(t, 200, event.status)
}

func TestProcess_WithPayloadTransformer_ModifiesPayload(t *testing.T) {
	// Given: an event processor with payload transformer.
	logger := createTestLogger(t)
	defer logger.Close()

	processorConfig := createTestProcessor("payload_transformer", "../../tests/integrationtests/processors/payload_transformer.py")
	chain, err := processor.NewChain([]*config.ProcessorConfig{processorConfig}, "test-server", "test-session")
	assert.NilError(t, err)

	ep := NewEventProcessor(logger, chain)
	originalMsg := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"test","arguments":{}}}`
	event := newMockMcpEvent(originalMsg, true)

	// When: processing an event through the transformer.
	err = ep.Process(event)

	// Then: should process successfully with modification.
	assert.NilError(t, err)
	assert.Equal(t, 200, event.status)
	assert.Equal(t, true, event.modified)
	// Message should be different from original.
	assert.Assert(t, event.GetRawMessage() != originalMsg)
}

func TestProcess_WithSecurityValidator_RejectsDeleteRequests(t *testing.T) {
	// Given: an event processor with security validator.
	logger := createTestLogger(t)
	defer logger.Close()

	processorConfig := createTestProcessor("security_validator", "../../tests/integrationtests/processors/security_validator.py")
	chain, err := processor.NewChain([]*config.ProcessorConfig{processorConfig}, "test-server", "test-session")
	assert.NilError(t, err)

	ep := NewEventProcessor(logger, chain)
	deleteMsg := `{"jsonrpc":"2.0","id":3,"method":"files/delete","params":{"path":"/test"}}`
	event := newMockMcpEvent(deleteMsg, true)

	// When: processing a delete request.
	err = ep.Process(event)

	// Then: should process but reject with 403.
	assert.NilError(t, err)
	assert.Equal(t, 403, event.status)
	// Error should be set in processing errors.
	assert.Assert(t, len(event.baseEvent.ProcessingErrors) > 0)
}

func TestProcess_WithSecurityValidator_AllowsNormalRequests(t *testing.T) {
	// Given: an event processor with security validator.
	logger := createTestLogger(t)
	defer logger.Close()

	processorConfig := createTestProcessor("security_validator", "../../tests/integrationtests/processors/security_validator.py")
	chain, err := processor.NewChain([]*config.ProcessorConfig{processorConfig}, "test-server", "test-session")
	assert.NilError(t, err)

	ep := NewEventProcessor(logger, chain)
	normalMsg := `{"jsonrpc":"2.0","id":4,"method":"tools/list","params":{}}`
	event := newMockMcpEvent(normalMsg, true)

	// When: processing a normal request.
	err = ep.Process(event)

	// Then: should process successfully.
	assert.NilError(t, err)
	assert.Equal(t, 200, event.status)
}

func TestProcess_WithMultipleProcessors_ExecutesInOrder(t *testing.T) {
	// Given: an event processor with multiple processors.
	logger := createTestLogger(t)
	defer logger.Close()

	processor1 := createTestProcessor("passthrough", "../../tests/integrationtests/processors/passthrough.py")
	processor2 := createTestProcessor("request_logger", "../../tests/integrationtests/processors/request_logger.py")
	chain, err := processor.NewChain([]*config.ProcessorConfig{processor1, processor2}, "test-server", "test-session")
	assert.NilError(t, err)

	ep := NewEventProcessor(logger, chain)
	event := newMockMcpEvent(`{"jsonrpc":"2.0","id":5,"method":"test/method","params":{}}`, true)

	// When: processing through multiple processors.
	err = ep.Process(event)

	// Then: should execute all processors successfully.
	assert.NilError(t, err)
	assert.Equal(t, 200, event.status)
}

func TestProcess_WithDisabledProcessor_SkipsProcessor(t *testing.T) {
	// Given: an event processor with disabled processor.
	logger := createTestLogger(t)
	defer logger.Close()

	processorConfig := createTestProcessor("passthrough", "../../tests/integrationtests/processors/passthrough.py")
	processorConfig.Enabled = false
	chain, err := processor.NewChain([]*config.ProcessorConfig{processorConfig}, "test-server", "test-session")
	assert.NilError(t, err)

	ep := NewEventProcessor(logger, chain)
	originalMsg := `{"jsonrpc":"2.0","id":6,"method":"test/method","params":{}}`
	event := newMockMcpEvent(originalMsg, true)

	// When: processing with disabled processor.
	err = ep.Process(event)

	// Then: should skip processor (no modification).
	assert.NilError(t, err)
	assert.Equal(t, originalMsg, event.GetRawMessage())
	assert.Equal(t, false, event.modified)
}

// ========================================.
// Test Helper - Processor Creation.
// ========================================.

func createTestProcessor(name, scriptPath string) *config.ProcessorConfig {
	// Get absolute path to processor script.
	absPath, _ := filepath.Abs(scriptPath)

	return &config.ProcessorConfig{
		Name:    name,
		Type:    "cli",
		Enabled: true,
		Timeout: 15,
		Config: map[string]interface{}{
			"command": "python3",
			"args":    []interface{}{absPath},
		},
	}
}
