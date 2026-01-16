// Package processor provides execution logic for MCP request/response processors.
package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/CentianAI/centian-cli/internal/config"
)

// Executor handles processor execution with timeout and error handling.
type Executor struct {
	// WorkingDir is the directory where processor commands are executed.
	// Defaults to user's home directory.
	WorkingDir string
}

// NewExecutor creates a new processor executor.
func NewExecutor() (*Executor, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	return &Executor{
		WorkingDir: homeDir,
	}, nil
}

// Execute runs a processor with the given input and returns the output.
// It handles CLI processor execution with timeout, JSON marshaling, and error handling.
func (e *Executor) Execute(processorConfig *config.ProcessorConfig, input *config.ProcessorInput) (*config.ProcessorOutput, error) {
	// Validate processor is enabled.
	if !processorConfig.Enabled {
		return nil, fmt.Errorf("processor '%s' is disabled", processorConfig.Name)
	}

	// Only CLI processors supported in v1.
	if processorConfig.Type != "cli" {
		return nil, fmt.Errorf("unsupported processor type '%s'", processorConfig.Type)
	}

	return e.executeCLI(processorConfig, input)
}

// executeCLI executes a CLI processor with timeout and JSON I/O handling.
func (e *Executor) executeCLI(processorConfig *config.ProcessorConfig, input *config.ProcessorInput) (*config.ProcessorOutput, error) {
	// Extract command and args from config.
	command, args, err := extractCommandAndArgs(processorConfig)
	if err != nil {
		return nil, err
	}

	// Execute the command with timeout.
	stdout, stderr, err := e.executeCommandWithTimeout(processorConfig, command, args, input)

	// Handle execution result.
	return handleExecutionResult(processorConfig, input, stdout, stderr, err)
}

// extractCommandAndArgs extracts command and arguments from processor config.
func extractCommandAndArgs(processorConfig *config.ProcessorConfig) (string, []string, error) {
	// Extract command from config.
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

// executeCommandWithTimeout executes a command with timeout and returns stdout, stderr, and error.
func (e *Executor) executeCommandWithTimeout(processorConfig *config.ProcessorConfig, command string, args []string, input *config.ProcessorInput) (bytes.Buffer, bytes.Buffer, error) {
	// Set up timeout context.
	timeout := time.Duration(processorConfig.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create command with context for timeout.
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = e.WorkingDir

	// Marshal input to JSON for stdin.
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return bytes.Buffer{}, bytes.Buffer{}, fmt.Errorf("failed to marshal processor input: %w", err)
	}

	// Set up stdin, stdout, stderr buffers.
	cmd.Stdin = bytes.NewReader(inputJSON)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command.
	err = cmd.Run()

	// Return with context error if timeout occurred.
	if ctx.Err() == context.DeadlineExceeded {
		return stdout, stderr, context.DeadlineExceeded
	}

	return stdout, stderr, err
}

// handleExecutionResult processes command execution result and returns ProcessorOutput.
func handleExecutionResult(processorConfig *config.ProcessorConfig, input *config.ProcessorInput, stdout, stderr bytes.Buffer, err error) (*config.ProcessorOutput, error) {
	// Handle timeout.
	if errors.Is(err, context.DeadlineExceeded) {
		errorMsg := fmt.Sprintf("processor '%s' timed out after %d seconds", processorConfig.Name, processorConfig.Timeout)
		return &config.ProcessorOutput{
			Status:  500,
			Payload: input.Payload,
			Error:   &errorMsg,
		}, nil
	}

	// Handle execution error (non-zero exit code).
	if err != nil {
		errorMsg := fmt.Sprintf("processor '%s' execution failed: %v", processorConfig.Name, err)
		if stderr.Len() > 0 {
			errorMsg = fmt.Sprintf("%s\nstderr: %s", errorMsg, stderr.String())
		}
		return &config.ProcessorOutput{
			Status:  500,
			Payload: input.Payload,
			Error:   &errorMsg,
		}, nil
	}

	// Parse and validate output.
	return parseAndValidateOutput(processorConfig, input, stdout)
}

// parseAndValidateOutput parses stdout JSON and validates the output.
func parseAndValidateOutput(processorConfig *config.ProcessorConfig, input *config.ProcessorInput, stdout bytes.Buffer) (*config.ProcessorOutput, error) {
	// Parse stdout JSON to ProcessorOutput.
	var output config.ProcessorOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		errorMsg := fmt.Sprintf("processor '%s' returned invalid JSON: %v", processorConfig.Name, err)
		if stdout.Len() > 0 {
			errorMsg = fmt.Sprintf("%s\nstdout: %s", errorMsg, stdout.String())
		}
		return &config.ProcessorOutput{
			Status:  500,
			Payload: input.Payload,
			Error:   &errorMsg,
		}, nil
	}

	// Validate output status code.
	if output.Status < 100 || output.Status >= 600 {
		errorMsg := fmt.Sprintf("processor '%s' returned invalid status code: %d", processorConfig.Name, output.Status)
		return &config.ProcessorOutput{
			Status:  500,
			Payload: input.Payload,
			Error:   &errorMsg,
		}, nil
	}

	// Ensure payload is set (use input payload if not modified).
	if output.Payload == nil {
		output.Payload = input.Payload
	}

	// Validate error field consistency with status.
	if output.Status >= 400 && output.Error == nil {
		errorMsg := fmt.Sprintf("status %d requires error message", output.Status)
		output.Error = &errorMsg
	}

	return &output, nil
}
