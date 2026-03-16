// Package processor provides execution logic for MCP request/response processors.
package processor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
)

// CLIProcessor performs a CLI execution.
type CLIProcessor struct {
	// WorkingDir is the directory where processor commands are executed.
	// Defaults to user's home directory.
	WorkingDir string
	config     *config.ProcessorConfig
}

// NewCLIProcessor creates a new NewCLIProcessor.
func NewCLIProcessor(c *config.ProcessorConfig) (*CLIProcessor, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}
	return &CLIProcessor{
		WorkingDir: homeDir,
		config:     c,
	}, nil
}

// GetConfig returns the attached ProcessorConfig.
func (e *CLIProcessor) GetConfig() *config.ProcessorConfig {
	return e.config
}

// Process runs a processor with the given input and returns the output.
// It handles CLI processor execution with timeout, and error handling.
//
// Note: the Processors responsibility is to execute a specific action, its NOT to serialize back the
// result into the correct data format - this is done in the handler.
func (e *CLIProcessor) Process(input *DataContext) (*DataContext, error) {
	command, args, err := extractCommandAndArgs(e.config)
	if err != nil {
		return nil, err
	}

	// Set up timeout context.
	timeout := time.Duration(e.config.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create command with context for timeout.
	// #nosec G204 -- intentional execution of trusted user-configured processor command.
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = e.WorkingDir

	// Marshal input to JSON for stdin.
	// Note: mcp.CallToolRequest includes transport/runtime fields (e.g. Extra.CloseSSEStream func)
	// that are not JSON-marshallable. We intentionally serialize a reduced request shape.
	inputJSON, err := marshalProcessorInput(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal processor input: %w", err)
	}

	// Set up stdin, stdout, stderr buffers.
	cmd.Stdin = bytes.NewReader(inputJSON)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command.
	common.LogDebug("[PROCESSOR:CLI] '%s': executing command: %s", e.config.Name, command)
	err = cmd.Run()

	// Handle timeout.
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("processor '%s' timed out after %d seconds", e.config.Name, e.config.Timeout)
	}

	// Handle execution error (non-zero exit code).
	if err != nil {
		errorMsg := fmt.Sprintf("processor '%s' execution failed: %v", e.config.Name, err)
		if stderr.Len() > 0 {
			errorMsg = fmt.Sprintf("%s\nstderr: %s", errorMsg, stderr.String())
		}
		return nil, fmt.Errorf("%s", errorMsg)
	}

	// Log any stderr output even on success — processors may write warnings there.
	if stderr.Len() > 0 {
		common.LogWarn("[PROCESSOR:CLI] '%s': stderr output: %s", e.config.Name, stderr.String())
	}

	output, err := decodeProcessorJSONOutput(e.config.Name, stdout.Bytes())
	if err != nil {
		return nil, err
	}

	// Warn when the processor was given a payload but returned none — this usually
	// means the output JSON is in the wrong format, and all payload modifications
	// (request args, result content) will be silently skipped.
	if input != nil && input.Payload != nil && output.Payload == nil {
		common.LogWarn("[PROCESSOR:CLI] '%s': input contained a payload but the output does not; "+
			"request/result modifications will be skipped. Ensure the processor output includes a \"payload\" field.", e.config.Name)
	}

	return output, nil
}

// extractCommandAndArgs extracts command and arguments from processor config.
func extractCommandAndArgs(processorConfig *config.ProcessorConfig) (string, []string, error) {
	// Extract command from config.
	// TODO: we could provide dedicated structs for the different processors as config
	// if we were using interfaces this would likely make things easier here
	command, ok := processorConfig.Config["command"].(string)
	if !ok {
		return "", nil, fmt.Errorf("processor '%s': config.command must be a string", processorConfig.Name)
	}

	// Extract args (optional).
	var args []string
	if argsInterface, exists := processorConfig.Config["args"]; exists {
		argsArray, ok := argsInterface.([]interface{})
		if !ok {
			return "", nil, fmt.Errorf("processor '%s': config.args must be an array", processorConfig.Name)
		}
		// Convert []interface{} to []string.
		for _, arg := range argsArray {
			argStr, ok := arg.(string)
			if !ok {
				return "", nil, fmt.Errorf("processor '%s': config.args must contain only strings", processorConfig.Name)
			}
			args = append(args, argStr)
		}
	}

	return command, args, nil
}
