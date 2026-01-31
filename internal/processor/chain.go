// Package processor provides execution logic for MCP request/response processors.
package processor

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
)

// Chain executes a sequence of processors on MCP messages.
type Chain struct {
	processors []*config.ProcessorConfig
	executor   *Executor
	serverName string
	sessionID  string
}

// NewChain creates a new processor chain.
func NewChain(processors []*config.ProcessorConfig, serverName, sessionID string) (*Chain, error) {
	executor, err := NewExecutor()
	if err != nil {
		return nil, fmt.Errorf("failed to create executor: %w", err)
	}

	return &Chain{
		processors: processors,
		executor:   executor,
		serverName: serverName,
		sessionID:  sessionID,
	}, nil
}

// HasProcessors returns true if the chain has any enabled processors.
func (c *Chain) HasProcessors() bool {
	if c == nil || len(c.processors) == 0 {
		return false
	}
	// Check if any processor is enabled.
	for _, p := range c.processors {
		if p.Enabled {
			return true
		}
	}
	return false
}

// ChainResult represents the result of executing a processor chain.
type ChainResult struct {
	Status          int                    // Final status code (200, 40x, 50x)
	ModifiedPayload map[string]interface{} // Final payload after all processors
	Error           *string                // Error message if status >= 400
	ProcessorChain  []string               // Names of processors that executed
	Metadata        map[string]interface{} // Aggregated processor metadata
}

// Execute runs all enabled processors sequentially on the payload.
func (c *Chain) Execute(event common.McpEventInterface) (*ChainResult, error) {
	// Parse the JSON payload.
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(event.GetRawMessage()), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse JSON payload: %w", err)
	}

	// Track processor execution.
	processorChain := []string{}
	aggregatedMetadata := make(map[string]interface{})
	originalPayload := make(map[string]interface{})

	// Deep copy original payload.
	for k, v := range payload {
		originalPayload[k] = v
	}

	// Execute each enabled processor.
	for _, processorConfig := range c.processors {
		// Skip disabled processors.
		if !processorConfig.Enabled {
			continue
		}

		// Build processor input.
		input := &config.ProcessorInput{
			Type:      string(event.GetBaseEvent().MessageType),
			Timestamp: time.Now().Format(time.RFC3339),
			Connection: config.ConnectionContext{
				ServerName: c.serverName,
				Transport:  event.GetBaseEvent().Transport, // TODO - this is wrong
				SessionID:  event.GetBaseEvent().SessionID,
			},
			Payload: payload,
			Metadata: config.ProcessorMetadata{
				ProcessorChain:  processorChain,
				OriginalPayload: originalPayload,
			},
		}

		// Execute processor.
		output, err := c.executor.Execute(processorConfig, input)
		if err != nil {
			// Processor failed to execute - treat as 500 error.
			errorMsg := fmt.Sprintf("processor '%s' execution failed: %v", processorConfig.Name, err)
			return &ChainResult{
				Status:          500,
				ModifiedPayload: payload,
				Error:           &errorMsg,
				ProcessorChain:  processorChain,
				Metadata:        aggregatedMetadata,
			}, nil
		}

		// Track processor in chain.
		processorChain = append(processorChain, processorConfig.Name)

		// Aggregate processor metadata.
		if output.Metadata != nil {
			aggregatedMetadata[processorConfig.Name] = output.Metadata
		}

		// Check status code.
		if output.Status >= 400 {
			// Processor rejected (40x) or errored (50x).
			return &ChainResult{
				Status:          output.Status,
				ModifiedPayload: output.Payload,
				Error:           output.Error,
				ProcessorChain:  processorChain,
				Metadata:        aggregatedMetadata,
			}, nil
		}

		// Status 200 - continue with modified payload.
		payload = output.Payload
	}

	// All processors passed - return success.
	return &ChainResult{
		Status:          200,
		ModifiedPayload: payload,
		Error:           nil,
		ProcessorChain:  processorChain,
		Metadata:        aggregatedMetadata,
	}, nil
}

// FormatMCPError formats a processor rejection/error as an MCP-compatible error response.
// Returns a JSON-RPC 2.0 error response with processor details in the data field.
func FormatMCPError(result *ChainResult, requestID interface{}) (string, error) {
	// Determine error code based on status.
	var errorCode int
	var errorMessage string

	switch {
	case result.Status >= 500:
		errorCode = -32603 // Internal error
		errorMessage = "Request processing failed"
	case result.Status >= 400:
		errorCode = -32001 // Server error (custom code for processor rejection)
		errorMessage = "Request rejected by processor"
	default:
		return "", fmt.Errorf("cannot format error for status %d (not an error)", result.Status)
	}

	// Build error response data with processor details.
	errorData := map[string]interface{}{
		"processor_chain": result.ProcessorChain,
		"metadata":        result.Metadata,
	}

	if result.Error != nil {
		errorData["rejection_reason"] = *result.Error
	}

	// Build JSON-RPC 2.0 error response.
	errorResponse := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      requestID,
		"error": map[string]interface{}{
			"code":    errorCode,
			"message": errorMessage,
			"data":    errorData,
		},
	}

	// Marshal to JSON.
	jsonBytes, err := json.Marshal(errorResponse)
	if err != nil {
		return "", fmt.Errorf("failed to marshal error response: %w", err)
	}

	return string(jsonBytes), nil
}
