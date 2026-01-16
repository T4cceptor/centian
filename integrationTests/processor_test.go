package integrationtests

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/CentianAI/centian-cli/internal/config"
	"github.com/CentianAI/centian-cli/internal/processor"
)

// TestPassthroughProcessor tests that the passthrough processor returns the input unchanged.
func TestPassthroughProcessor(t *testing.T) {
	// Given: a passthrough processor
	processorConfig := createProcessorConfig("passthrough", "processors/passthrough.py")
	input := loadTestInput(t, "testdata/request_normal.json")

	// When: executing the processor
	output, err := executeProcessor(t, processorConfig, input)

	// Then: execution succeeds with status 200
	if err != nil {
		t.Fatalf("Processor execution failed: %v", err)
	}

	// And: status is 200 (success)
	if output.Status != 200 {
		t.Errorf("Expected status 200, got %d", output.Status)
	}

	// And: error is nil
	if output.Error != nil && *output.Error != "" {
		t.Errorf("Expected no error, got: %s", *output.Error)
	}

	// And: payload is unchanged
	if !jsonEqual(t, input.Payload, output.Payload) {
		t.Errorf("Payload was modified by passthrough processor")
	}

	// And: metadata includes processor name
	if output.Metadata == nil {
		t.Fatal("Expected metadata to be present")
	}
	processorName, ok := output.Metadata["processor_name"].(string)
	if !ok || processorName != "passthrough" {
		t.Errorf("Expected processor_name to be 'passthrough', got: %v", output.Metadata["processor_name"])
	}
}

// TestSecurityValidatorAllowsNormalRequests tests that normal requests pass through.
func TestSecurityValidatorAllowsNormalRequests(t *testing.T) {
	// Given: a security validator processor
	processorConfig := createProcessorConfig("security_validator", "processors/security_validator.py")
	input := loadTestInput(t, "testdata/request_normal.json")

	// When: executing with a normal request
	output, err := executeProcessor(t, processorConfig, input)

	// Then: execution succeeds
	if err != nil {
		t.Fatalf("Processor execution failed: %v", err)
	}

	// And: status is 200 (allowed)
	if output.Status != 200 {
		t.Errorf("Expected status 200 for normal request, got %d", output.Status)
	}

	// And: no error is returned
	if output.Error != nil && *output.Error != "" {
		t.Errorf("Expected no error for normal request, got: %s", *output.Error)
	}
}

// TestSecurityValidatorBlocksDeleteRequests tests that delete operations are rejected.
func TestSecurityValidatorBlocksDeleteRequests(t *testing.T) {
	// Given: a security validator processor
	processorConfig := createProcessorConfig("security_validator", "processors/security_validator.py")
	input := loadTestInput(t, "testdata/request_delete.json")

	// When: executing with a delete request
	output, err := executeProcessor(t, processorConfig, input)

	// Then: execution succeeds (processor ran successfully)
	if err != nil {
		t.Fatalf("Processor execution failed: %v", err)
	}

	// And: status is 403 (forbidden)
	if output.Status != 403 {
		t.Errorf("Expected status 403 for delete request, got %d", output.Status)
	}

	// And: error message is present
	if output.Error == nil || *output.Error == "" {
		t.Error("Expected error message for rejected delete request")
	}

	// And: error mentions deletion is not allowed
	expectedError := "Delete operations not allowed"
	if output.Error == nil || *output.Error != expectedError {
		var gotError string
		if output.Error != nil {
			gotError = *output.Error
		}
		t.Errorf("Expected error '%s', got '%s'", expectedError, gotError)
	}

	// And: payload is empty (rejection)
	if len(output.Payload) != 0 {
		t.Errorf("Expected empty payload for rejected request, got: %v", output.Payload)
	}
}

// TestRequestLoggerPassesThrough tests that the logger processor passes requests through.
func TestRequestLoggerPassesThrough(t *testing.T) {
	// Given: a request logger processor
	processorConfig := createProcessorConfig("request_logger", "processors/request_logger.py")
	input := loadTestInput(t, "testdata/request_normal.json")

	// When: executing the processor
	output, err := executeProcessor(t, processorConfig, input)

	// Then: execution succeeds
	if err != nil {
		t.Fatalf("Processor execution failed: %v", err)
	}

	// And: status is 200 (success)
	if output.Status != 200 {
		t.Errorf("Expected status 200, got %d", output.Status)
	}

	// And: payload is unchanged (logging is side effect only)
	if !jsonEqual(t, input.Payload, output.Payload) {
		t.Errorf("Payload was modified by logger processor")
	}
}

// TestPayloadTransformerModifiesRequest tests that the transformer adds custom headers.
func TestPayloadTransformerModifiesRequest(t *testing.T) {
	// Given: a payload transformer processor
	processorConfig := createProcessorConfig("payload_transformer", "processors/payload_transformer.py")
	input := loadTestInput(t, "testdata/request_normal.json")

	// When: executing the processor
	output, err := executeProcessor(t, processorConfig, input)

	// Then: execution succeeds
	if err != nil {
		t.Fatalf("Processor execution failed: %v", err)
	}

	// And: status is 200 (success)
	if output.Status != 200 {
		t.Errorf("Expected status 200, got %d", output.Status)
	}

	// And: payload has been modified
	payload := output.Payload

	params, ok := payload["params"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected payload.params to be a map")
	}

	arguments, ok := params["arguments"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected payload.params.arguments to be a map")
	}

	// And: custom header was added
	xProcessor, ok := arguments["x-processor"].(string)
	if !ok {
		t.Fatal("Expected x-processor header to be added")
	}

	if xProcessor != "payload_transformer" {
		t.Errorf("Expected x-processor to be 'payload_transformer', got '%s'", xProcessor)
	}

	// And: modifications are tracked in metadata
	if output.Metadata == nil {
		t.Fatal("Expected metadata to be present")
	}
}

// TestProcessorWithResponseData tests that processors can handle response messages.
func TestProcessorWithResponseData(t *testing.T) {
	// Given: a passthrough processor
	processorConfig := createProcessorConfig("passthrough", "processors/passthrough.py")
	input := loadTestInput(t, "testdata/response_success.json")

	// When: executing with a response message
	output, err := executeProcessor(t, processorConfig, input)

	// Then: execution succeeds
	if err != nil {
		t.Fatalf("Processor execution failed: %v", err)
	}

	// And: status is 200
	if output.Status != 200 {
		t.Errorf("Expected status 200, got %d", output.Status)
	}

	// And: response payload is preserved
	if !jsonEqual(t, input.Payload, output.Payload) {
		t.Errorf("Response payload was modified")
	}
}

// TestProcessorChain tests that multiple processors can be chained together.
func TestProcessorChain(t *testing.T) {
	// Given: a chain of processors (logger -> validator)
	loggerConfig := createProcessorConfig("request_logger", "processors/request_logger.py")
	validatorConfig := createProcessorConfig("security_validator", "processors/security_validator.py")
	input := loadTestInput(t, "testdata/request_normal.json")

	// When: executing the chain
	// First processor: logger
	output1, err := executeProcessor(t, loggerConfig, input)
	if err != nil {
		t.Fatalf("Logger processor failed: %v", err)
	}

	// Update metadata to track processor chain
	if input.Metadata.ProcessorChain == nil {
		input.Metadata.ProcessorChain = []string{}
	}
	input.Metadata.ProcessorChain = append(input.Metadata.ProcessorChain, "request_logger")

	// Second processor: validator (receives output from logger)
	input.Payload = output1.Payload
	output2, err := executeProcessor(t, validatorConfig, input)
	if err != nil {
		t.Fatalf("Validator processor failed: %v", err)
	}

	// Then: final status is 200 (both processors passed)
	if output2.Status != 200 {
		t.Errorf("Expected final status 200, got %d", output2.Status)
	}
}

// TestProcessorChainWithRejection tests that a rejection stops the chain.
func TestProcessorChainWithRejection(t *testing.T) {
	// Given: a chain where the second processor rejects
	passthroughConfig := createProcessorConfig("passthrough", "processors/passthrough.py")
	validatorConfig := createProcessorConfig("security_validator", "processors/security_validator.py")
	input := loadTestInput(t, "testdata/request_delete.json")

	// When: executing the chain
	// First processor: passthrough
	output1, err := executeProcessor(t, passthroughConfig, input)
	if err != nil {
		t.Fatalf("Passthrough processor failed: %v", err)
	}

	// Second processor: validator (should reject)
	input.Payload = output1.Payload
	output2, err := executeProcessor(t, validatorConfig, input)
	if err != nil {
		t.Fatalf("Validator processor failed: %v", err)
	}

	// Then: chain stops with 403 rejection
	if output2.Status != 403 {
		t.Errorf("Expected status 403 (rejection), got %d", output2.Status)
	}

	// And: subsequent processors should not be executed (tested implicitly)
}

// Helper: createProcessorConfig creates a processor configuration for testing.
func createProcessorConfig(name, scriptPath string) *config.ProcessorConfig {
	// Get absolute path to processor script
	absPath, _ := filepath.Abs(scriptPath)

	return &config.ProcessorConfig{
		Name:    name,
		Type:    "cli",
		Enabled: true,
		Timeout: 15, // 15 seconds
		Config: map[string]interface{}{
			"command": "python3",
			"args":    []interface{}{absPath},
		},
	}
}

// Helper: loadTestInput loads a test input JSON file.
func loadTestInput(t *testing.T, filename string) *config.ProcessorInput {
	t.Helper()

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read test input file %s: %v", filename, err)
	}

	var input config.ProcessorInput
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatalf("Failed to parse test input JSON: %v", err)
	}

	return &input
}

// Helper: executeProcessor executes a processor and returns the output.
func executeProcessor(t *testing.T, processorConfig *config.ProcessorConfig, input *config.ProcessorInput) (*config.ProcessorOutput, error) {
	t.Helper()

	executor, err := processor.NewExecutor()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	return executor.Execute(processorConfig, input)
}

// Helper: jsonEqual compares two JSON objects for equality.
func jsonEqual(t *testing.T, a, b interface{}) bool {
	t.Helper()

	aJSON, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("Failed to marshal first object: %v", err)
	}

	bJSON, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("Failed to marshal second object: %v", err)
	}

	return bytes.Equal(aJSON, bJSON)
}
