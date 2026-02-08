// Package processor provides execution logic for MCP request/response processors.
package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/T4cceptor/centian/internal/config"
)

// CLIProcessor performs a CLI execution
type CLIProcessor struct {
	// WorkingDir is the directory where processor commands are executed.
	// Defaults to user's home directory.
	WorkingDir string
	config     *config.ProcessorConfig
}

// NewCLIProcessor creates a new NewCLIProcessor.
func NewCLIProcessor(config *config.ProcessorConfig) (*CLIProcessor, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}
	return &CLIProcessor{
		WorkingDir: homeDir,
		config:     config,
	}, nil
}

func (e *CLIProcessor) GetConfig() *config.ProcessorConfig {
	return e.config
}

// Execute runs a processor with the given input and returns the output.
// It handles CLI processor execution with timeout, and error handling.
//
// Note: the Processors responsibility is to execute a specific action, its NOT to serialize back the
// result into the correct data format - this is done in the handler.
func (e *CLIProcessor) Process(input map[string]any) (map[string]any, error) {
	command, args, err := extractCommandAndArgs(e.config)
	if err != nil {
		return nil, err
	}

	// Set up timeout context.
	timeout := time.Duration(e.config.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create command with context for timeout.
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = e.WorkingDir

	// Marshal input to JSON for stdin.
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal processor input: %w", err)
	}

	// Set up stdin, stdout, stderr buffers.
	cmd.Stdin = bytes.NewReader(inputJSON)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command.
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

	// Parse stdout JSON to ProcessorOutput.
	var output map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		errorMsg := fmt.Sprintf("processor '%s' returned invalid JSON: %v", e.config.Name, err)
		if stdout.Len() > 0 {
			errorMsg = fmt.Sprintf("%s\nstdout: %s", errorMsg, stdout.String())
		}
		return nil, fmt.Errorf("%s", errorMsg)
	}

	// TODO -> create a map from output
	return output, nil
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
