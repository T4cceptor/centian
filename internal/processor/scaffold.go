//nolint:errcheck // we have A LOT of Fprintln in here and we do not want to check for errors after every single one
package processor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
	"github.com/CentianAI/centian-cli/internal/config"
)

type scaffoldLanguage string

const (
	langPython     scaffoldLanguage = "python"
	langJavaScript scaffoldLanguage = "javascript"
	langTypeScript scaffoldLanguage = "typescript"
	langBash       scaffoldLanguage = "bash"
)

type scaffoldType string

const (
	typePassthrough scaffoldType = "passthrough"
	typeValidator   scaffoldType = "validator"
	typeTransformer scaffoldType = "transformer"
	typeLogger      scaffoldType = "logger"
	typeCustom      scaffoldType = "custom"
)

// RunScaffoldInteractive creates a processor scaffold via interactive prompts.
func RunScaffoldInteractive(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)

	fmt.Fprintln(out, "==============================================")
	fmt.Fprintln(out, "  Centian Processor Scaffold Generator")
	fmt.Fprintln(out, "==============================================")
	fmt.Fprintln(out)

	lang, err := promptLanguage(reader, out)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Selected: %s\n\n", lang)

	procType, err := promptProcessorType(reader, out)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Selected: %s\n\n", procType)

	name, err := promptProcessorName(reader, out)
	if err != nil {
		return err
	}

	outputDir, err := promptOutputDir(reader, out)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	ext := extensionForLanguage(lang)
	outputFile := filepath.Join(outputDir, fmt.Sprintf("%s.%s", name, ext))

	if exists(outputFile) {
		overwrite, err := promptOverwrite(reader, out, outputFile)
		if err != nil {
			return err
		}
		if !overwrite {
			fmt.Fprintln(out, "Cancelled.")
			return nil
		}
	}

	fmt.Fprintf(out, "Output: %s\n\n", outputFile)

	content, err := generateProcessorTemplate(lang, procType, name)
	if err != nil {
		return err
	}

	//nolint:gosec // file is non-sensitive
	if err := os.WriteFile(outputFile, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write processor file: %w", err)
	}
	//nolint:gosec // file is non-sensitive, and execution is required
	if err := os.Chmod(outputFile, 0o755); err != nil {
		return fmt.Errorf("failed to make processor executable: %w", err)
	}

	fmt.Fprintf(out, "Processor created: %s\n\n", outputFile)

	testInput := filepath.Join(outputDir, fmt.Sprintf("%s_test.json", name))
	if err := writeTestInput(testInput); err != nil {
		return err
	}
	fmt.Fprintf(out, "Test input created: %s\n\n", testInput)

	addToConfig, err := promptAddToConfig(reader, out)
	if err != nil {
		return err
	}
	if addToConfig {
		if err := addProcessorToConfig(name, lang, outputFile); err != nil {
			return err
		}
		fmt.Fprint(out, "Processor added to config.\n")
	}

	printNextSteps(out, lang, name, outputFile, testInput, addToConfig)
	return nil
}

func promptLanguage(reader *bufio.Reader, out io.Writer) (scaffoldLanguage, error) {
	fmt.Fprintln(out, "Step 1: Choose your language")
	fmt.Fprintln(out, "1) Python")
	fmt.Fprintln(out, "2) JavaScript (Node.js)")
	fmt.Fprintln(out, "3) TypeScript (Node.js)")
	fmt.Fprintln(out, "4) Bash")
	fmt.Fprintln(out)
	choice, err := prompt(reader, out, "Select language [1-4]: ")
	if err != nil {
		return "", err
	}
	switch choice {
	case "1":
		return langPython, nil
	case "2":
		return langJavaScript, nil
	case "3":
		return langTypeScript, nil
	case "4":
		return langBash, nil
	default:
		return "", fmt.Errorf("invalid choice")
	}
}

func promptProcessorType(reader *bufio.Reader, out io.Writer) (scaffoldType, error) {
	fmt.Fprintln(out, "Step 2: Choose processor type")
	fmt.Fprintln(out, "1) Passthrough (no-op, for testing)")
	fmt.Fprintln(out, "2) Validator (accept/reject based on rules)")
	fmt.Fprintln(out, "3) Transformer (modify payload)")
	fmt.Fprintln(out, "4) Logger (record data, pass through)")
	fmt.Fprintln(out, "5) Custom (minimal template)")
	fmt.Fprintln(out)
	choice, err := prompt(reader, out, "Select type [1-5]: ")
	if err != nil {
		return "", err
	}
	switch choice {
	case "1":
		return typePassthrough, nil
	case "2":
		return typeValidator, nil
	case "3":
		return typeTransformer, nil
	case "4":
		return typeLogger, nil
	case "5":
		return typeCustom, nil
	default:
		return "", fmt.Errorf("invalid choice")
	}
}

func promptProcessorName(reader *bufio.Reader, out io.Writer) (string, error) {
	fmt.Fprintln(out, "Step 3: Enter processor name")
	name, err := prompt(reader, out, "Processor name (e.g., my_processor): ")
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", fmt.Errorf("processor name cannot be empty")
	}
	sanitized := sanitizeName(name)
	if sanitized == "" {
		return "", fmt.Errorf("processor name must contain alphanumeric characters")
	}
	fmt.Fprintf(out, "✓ Processor name: %s\n\n", sanitized)
	return sanitized, nil
}

func promptOutputDir(reader *bufio.Reader, out io.Writer) (string, error) {
	fmt.Fprintln(out, "Step 4: Choose output directory")
	defaultDir, err := defaultProcessorDir()
	if err != nil {
		return "", err
	}
	line, err := prompt(reader, out, fmt.Sprintf("Output directory [%s]: ", defaultDir))
	if err != nil {
		return "", err
	}
	if line == "" {
		return defaultDir, nil
	}
	return line, nil
}

func promptOverwrite(reader *bufio.Reader, out io.Writer, path string) (bool, error) {
	fmt.Fprintf(out, "Error: File already exists: %s\n", path)
	line, err := prompt(reader, out, "Overwrite? [y/N]: ")
	if err != nil {
		return false, err
	}
	line = strings.TrimSpace(line)
	return line == "y" || line == "Y", nil
}

func promptAddToConfig(reader *bufio.Reader, out io.Writer) (bool, error) {
	fmt.Fprintln(out, "Step 5: Add to centian config")
	line, err := prompt(reader, out, "Add this processor to ~/.centian/config.json now? [y/N]: ")
	if err != nil {
		return false, err
	}
	line = strings.TrimSpace(line)
	return line == "y" || line == "Y", nil
}

func prompt(reader *bufio.Reader, out io.Writer, label string) (string, error) {
	fmt.Fprint(out, label)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func sanitizeName(name string) string {
	normalized := strings.ReplaceAll(name, " ", "_")
	var b strings.Builder
	for _, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func defaultProcessorDir() (string, error) {
	workingDir := common.GetCurrentWorkingDir()
	if workingDir == "" {
		return "", fmt.Errorf("failed to resolve working directory")
	}
	return workingDir, nil
}

func extensionForLanguage(lang scaffoldLanguage) string {
	switch lang {
	case langPython:
		return "py"
	case langJavaScript:
		return "js"
	case langTypeScript:
		return "ts"
	case langBash:
		return "sh"
	default:
		return "txt"
	}
}

func commandForLanguage(lang scaffoldLanguage) string {
	switch lang {
	case langPython:
		return "python3"
	case langJavaScript:
		return "node"
	case langTypeScript:
		return "ts-node"
	case langBash:
		return "bash"
	default:
		return ""
	}
}

func generateProcessorTemplate(lang scaffoldLanguage, procType scaffoldType, name string) (string, error) {
	var template string
	var logic string

	switch lang {
	case langPython:
		template = pythonTemplate
		logic = pythonLogic(procType)
	case langJavaScript:
		template = javascriptTemplate
		logic = javascriptLogic(procType)
	case langTypeScript:
		template = typescriptTemplate
		logic = typescriptLogic(procType)
	case langBash:
		template = bashTemplate
		logic = bashLogic(procType)
	default:
		return "", fmt.Errorf("unsupported language")
	}

	content := strings.ReplaceAll(template, "PROCESSOR_LOGIC", logic)
	content = strings.ReplaceAll(content, "PROCESSOR_NAME", name)
	content = strings.ReplaceAll(content, "PROCESSOR_TYPE", string(procType))
	content = strings.ReplaceAll(content, "TIMESTAMP", time.Now().UTC().Format(time.RFC3339))
	return content, nil
}

func pythonLogic(procType scaffoldType) string {
	switch procType {
	case typePassthrough:
		return "    # Passthrough: return payload unchanged"
	case typeValidator:
		return `    # Example: Block delete operations
    if "delete" in payload.get("method", "").lower():
        return {
            "status": 403,
            "payload": {},
            "error": "Delete operations not allowed",
            "metadata": {"processor_name": "PROCESSOR_NAME"}
        }`
	case typeTransformer:
		return `    # Example: Add custom header
    if "params" in payload:
        if "arguments" not in payload["params"]:
            payload["params"]["arguments"] = {}
        payload["params"]["arguments"]["x-processor"] = "PROCESSOR_NAME"
        modifications = ["added x-processor header"]
    else:
        modifications = []`
	case typeLogger:
		return `    # Example: Log to file
    import os
    from datetime import datetime

    log_entry = {
        "timestamp": datetime.now().isoformat(),
        "type": event["type"],
        "method": payload.get("method", "unknown")
    }

    log_file = os.path.expanduser("~/centian/logs/processor.log")
    os.makedirs(os.path.dirname(log_file), exist_ok=True)
    with open(log_file, "a") as f:
        f.write(json.dumps(log_entry) + "\n")`
	case typeCustom:
		return `    # TODO: Add your custom logic here
    # Example:
    # if some_condition:
    #     return {
    #         "status": 403,
    #         "payload": {},
    #         "error": "Condition failed",
    #         "metadata": {"processor_name": "PROCESSOR_NAME"}
    #     }`
	default:
		return ""
	}
}

func javascriptLogic(procType scaffoldType) string {
	switch procType {
	case typePassthrough:
		return "  // Passthrough: return payload unchanged"
	case typeValidator:
		return `  // Example: Block delete operations
  if ((payload.method || "").toLowerCase().includes("delete")) {
    return {
      status: 403,
      payload: {},
      error: "Delete operations not allowed",
      metadata: { processor_name: "PROCESSOR_NAME" }
    };
  }`
	case typeTransformer:
		return `  // Example: Add custom header
  if (payload.params) {
    if (!payload.params.arguments) {
      payload.params.arguments = {};
    }
    payload.params.arguments["x-processor"] = "PROCESSOR_NAME";
  }`
	case typeLogger:
		return `  // Example: Log to file
  const fs = require("fs");
  const os = require("os");
  const path = require("path");

  const logEntry = {
    timestamp: new Date().toISOString(),
    type: event.type,
    method: payload.method || "unknown"
  };

  const logFile = path.join(os.homedir(), "centian", "logs", "processor.log");
  fs.mkdirSync(path.dirname(logFile), { recursive: true });
  fs.appendFileSync(logFile, JSON.stringify(logEntry) + "\n");`
	case typeCustom:
		return "  // TODO: Add your custom logic here"
	default:
		return ""
	}
}

func typescriptLogic(procType scaffoldType) string {
	switch procType {
	case typePassthrough:
		return "  // Passthrough: return payload unchanged"
	case typeValidator:
		return `  // Example: Block delete operations
  if ((payload.method || "").toLowerCase().includes("delete")) {
    return {
      status: 403,
      payload: {},
      error: "Delete operations not allowed",
      metadata: { processor_name: "PROCESSOR_NAME" }
    };
  }`
	case typeTransformer:
		return `  // Example: Add custom header
  if (payload.params) {
    if (!payload.params.arguments) {
      payload.params.arguments = {};
    }
    payload.params.arguments["x-processor"] = "PROCESSOR_NAME";
  }`
	case typeLogger:
		return `  // Example: Log to file
  import * as fs from "fs";
  import * as os from "os";
  import * as path from "path";

  const logEntry = {
    timestamp: new Date().toISOString(),
    type: event.type,
    method: payload.method || "unknown"
  };

  const logFile = path.join(os.homedir(), "centian", "logs", "processor.log");
  fs.mkdirSync(path.dirname(logFile), { recursive: true });
  fs.appendFileSync(logFile, JSON.stringify(logEntry) + "\n");`
	case typeCustom:
		return "  // TODO: Add your custom logic here"
	default:
		return ""
	}
}

func bashLogic(procType scaffoldType) string {
	switch procType {
	case typePassthrough:
		return "# Passthrough: return payload unchanged"
	case typeValidator:
		return `# Example: Block delete operations
METHOD=$(echo "$PAYLOAD" | jq -r ".method // empty")
if echo "$METHOD" | grep -iq "delete"; then
  jq -n '{
    status: 403,
    payload: {},
    error: "Delete operations not allowed",
    metadata: { processor_name: "PROCESSOR_NAME" }
  }'
  exit 0
fi`
	case typeTransformer:
		return `# Example: Add custom header
PAYLOAD=$(echo "$PAYLOAD" | jq ".params.arguments[\"x-processor\"] = \"PROCESSOR_NAME\"")`
	case typeLogger:
		return `# Example: Log to file
LOG_FILE="$HOME/centian/logs/processor.log"
mkdir -p "$(dirname "$LOG_FILE")"
echo "{\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"type\":\"$TYPE\",\"method\":\"$(echo "$PAYLOAD" | jq -r ".method // \"unknown\"")\"}" >> "$LOG_FILE"`
	case typeCustom:
		return "# TODO: Add your custom logic here"
	default:
		return ""
	}
}

func writeTestInput(path string) error {
	const testPayload = `{
  "type": "request",
  "timestamp": "2025-12-27T10:00:00Z",
  "connection": {
    "server_name": "test",
    "transport": "stdio",
    "session_id": "test123"
  },
  "payload": {
    "method": "tools/call",
    "params": {
      "name": "test_tool",
      "arguments": {}
    }
  },
  "metadata": {
    "processor_chain": [],
    "original_payload": {}
  }
}
`
	//nolint:gosec // file is non-sensitive
	if err := os.WriteFile(path, []byte(testPayload), 0o644); err != nil {
		return fmt.Errorf("failed to write test input: %w", err)
	}
	return nil
}

func addProcessorToConfig(name string, lang scaffoldLanguage, outputFile string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg.Processors == nil {
		cfg.Processors = []*config.ProcessorConfig{}
	}
	for _, processor := range cfg.Processors {
		if processor.Name == name {
			return fmt.Errorf("processor '%s' already exists in config", name)
		}
	}
	command := commandForLanguage(lang)
	if command == "" {
		return fmt.Errorf("unsupported language")
	}
	cfg.Processors = append(cfg.Processors, &config.ProcessorConfig{
		Name:    name,
		Type:    string(config.CLIProcessor),
		Enabled: true,
		Timeout: 15,
		Config: map[string]interface{}{
			"command": command,
			"args":    []interface{}{outputFile},
		},
	})
	return config.SaveConfig(cfg)
}

func printNextSteps(out io.Writer, lang scaffoldLanguage, name, outputFile, testInput string, addedToConfig bool) {
	fmt.Fprintln(out)
	step := 1
	if !addedToConfig {
		fmt.Fprintf(out, "Add to Centian config (~/.centian/config.json):\n")
		fmt.Fprintln(out, "   {")
		fmt.Fprintln(out, "     \"processors\": [")
		fmt.Fprintln(out, "       {")
		fmt.Fprintf(out, "         \"name\": \"%s\",\n", name)
		fmt.Fprintln(out, "         \"type\": \"cli\",")
		switch lang {
		case langPython:
			fmt.Fprintln(out, "         \"command\": \"python3\",")
		case langJavaScript:
			fmt.Fprintln(out, "         \"command\": \"node\",")
		case langTypeScript:
			fmt.Fprintln(out, "         \"command\": \"ts-node\",")
		case langBash:
			fmt.Fprintln(out, "         \"command\": \"bash\",")
		}
		fmt.Fprintf(out, "         \"args\": [\"%s\"],\n", outputFile)
		fmt.Fprintln(out, "         \"enabled\": true")
		fmt.Fprintln(out, "       }")
		fmt.Fprintln(out, "     ]")
		fmt.Fprintln(out, "   }")
		fmt.Fprintln(out)
		step++
	}

	fmt.Fprintf(out, "For further information read the full documentation at:\n")
	fmt.Fprintln(out, "   docs/processor_development_guide.md")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Happy coding!")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

const pythonTemplate = `#!/usr/bin/env python3
"""
Centian Processor: PROCESSOR_NAME
Type: PROCESSOR_TYPE
Generated: TIMESTAMP
"""

import sys
import json

def process(event):
    """
    Process an MCP message.

    Args:
        event: Input event with structure:
            - type: "request" or "response"
            - timestamp: ISO 8601 timestamp
            - connection: {server_name, transport, session_id}
            - payload: MCP message payload
            - metadata: {processor_chain, original_payload}

    Returns:
        dict: Output with structure:
            - status: HTTP status code (200, 40x, 50x)
            - payload: Modified or original payload
            - error: Error message or None
            - metadata: {processor_name, modifications}
    """
    payload = event["payload"]

PROCESSOR_LOGIC

    # Return success
    return {
        "status": 200,
        "payload": payload,
        "error": None,
        "metadata": {
            "processor_name": "PROCESSOR_NAME",
            "modifications": []
        }
    }

def main():
    try:
        # Read input from stdin
        event = json.load(sys.stdin)

        # Process the event
        result = process(event)

        # Write result to stdout
        print(json.dumps(result))
        sys.exit(0)

    except Exception as e:
        # Return internal error
        result = {
            "status": 500,
            "payload": {},
            "error": str(e),
            "metadata": {"processor_name": "PROCESSOR_NAME"}
        }
        print(json.dumps(result))
        sys.exit(0)  # Exit 0 even on error

if __name__ == "__main__":
    main()
`

const javascriptTemplate = `#!/usr/bin/env node
/**
 * Centian Processor: PROCESSOR_NAME
 * Type: PROCESSOR_TYPE
 * Generated: TIMESTAMP
 */

function process(event) {
  const payload = event.payload;

PROCESSOR_LOGIC

  // Return success
  return {
    status: 200,
    payload: payload,
    error: null,
    metadata: {
      processor_name: 'PROCESSOR_NAME',
      modifications: []
    }
  };
}

function main() {
  let input = '';

  process.stdin.on('data', chunk => {
    input += chunk;
  });

  process.stdin.on('end', () => {
    try {
      const event = JSON.parse(input);
      const result = process(event);
      console.log(JSON.stringify(result));
      process.exit(0);
    } catch (err) {
      const result = {
        status: 500,
        payload: {},
        error: err.message,
        metadata: { processor_name: 'PROCESSOR_NAME' }
      };
      console.log(JSON.stringify(result));
      process.exit(0); // Exit 0 even on error
    }
  });
}

main();
`

const typescriptTemplate = `#!/usr/bin/env ts-node
/**
 * Centian Processor: PROCESSOR_NAME
 * Type: PROCESSOR_TYPE
 * Generated: TIMESTAMP
 */

interface ProcessorInput {
  type: 'request' | 'response';
  timestamp: string;
  connection: {
    server_name: string;
    transport: string;
    session_id: string;
  };
  payload: any;
  metadata: {
    processor_chain: string[];
    original_payload: any;
  };
}

interface ProcessorOutput {
  status: number;
  payload: any;
  error: string | null;
  metadata: {
    processor_name: string;
    modifications?: string[];
    [key: string]: any;
  };
}

function process(event: ProcessorInput): ProcessorOutput {
  const payload = event.payload;

PROCESSOR_LOGIC

  // Return success
  return {
    status: 200,
    payload: payload,
    error: null,
    metadata: {
      processor_name: 'PROCESSOR_NAME',
      modifications: []
    }
  };
}

function main(): void {
  let input = '';

  process.stdin.on('data', (chunk) => {
    input += chunk;
  });

  process.stdin.on('end', () => {
    try {
      const event: ProcessorInput = JSON.parse(input);
      const result = process(event);
      console.log(JSON.stringify(result));
      process.exit(0);
    } catch (err) {
      const result: ProcessorOutput = {
        status: 500,
        payload: {},
        error: (err as Error).message,
        metadata: { processor_name: 'PROCESSOR_NAME' }
      };
      console.log(JSON.stringify(result));
      process.exit(0);
    }
  });
}

main();
`

const bashTemplate = `#!/bin/bash
# Centian Processor: PROCESSOR_NAME
# Type: PROCESSOR_TYPE
# Generated: TIMESTAMP

# Read input from stdin
INPUT=$(cat)

# Parse JSON using jq
TYPE=$(echo "$INPUT" | jq -r '.type')
PAYLOAD=$(echo "$INPUT" | jq -c '.payload')

PROCESSOR_LOGIC

# Return success
jq -n \
  --argjson payload "$PAYLOAD" \
  '{
    status: 200,
    payload: $payload,
    error: null,
    metadata: {
      processor_name: "PROCESSOR_NAME",
      modifications: []
    }
  }'

exit 0
`
