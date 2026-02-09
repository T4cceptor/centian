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

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
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

	printNextSteps(out, lang, name, outputFile, addToConfig)
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
		return "    # Passthrough: return context unchanged"
	case typeValidator:
		return `    # Example: Block delete operations
    payload = ctx.payload or PayloadPart()
    request = payload.request
    params = request.params if request else None
    tool_name = (params.name or "") if params else ""

    if "delete" in tool_name.lower():
        event = ctx.event or {}
        event["status"] = 403
        event["error"] = "Delete operations not allowed"
        event["success"] = False
        ctx.event = event
        payload.result = CallToolResult(
            content=[{"type": "text", "text": "Delete operations not allowed"}],
            is_error=True,
        )
        ctx.payload = payload`
	case typeTransformer:
		return `    # Example: Add custom argument to tool request
    payload = ctx.payload or PayloadPart()
    request = payload.request
    params = request.params if request else None
    arguments = params.arguments if params else None

    if isinstance(arguments, dict):
        arguments["x-processor"] = "PROCESSOR_NAME"
        params.arguments = arguments
        request.params = params
        payload.request = request
        ctx.payload = payload`
	case typeLogger:
		return `    # Example: Log to file
    import os
    from datetime import datetime

    payload = ctx.payload or PayloadPart()
    request = payload.request
    params = request.params if request else None
    tool_name = (params.name or "unknown") if params else "unknown"

    log_entry = {
        "timestamp": datetime.now().isoformat(),
        "tool_name": tool_name
    }

    log_file = os.path.expanduser("~/centian/logs/processor.log")
    os.makedirs(os.path.dirname(log_file), exist_ok=True)
    with open(log_file, "a", encoding="utf-8") as f:
        f.write(json.dumps(log_entry) + "\n")`
	case typeCustom:
		return `    # TODO: Add your custom logic here
    # Example:
    # if some_condition:
    #     event = ctx.event or {}
    #     event["status"] = 403
    #     event["error"] = "Condition failed"
    #     event["success"] = False
    #     ctx.event = event`
	default:
		return ""
	}
}

func javascriptLogic(procType scaffoldType) string {
	switch procType {
	case typePassthrough:
		return "  // Passthrough: return context unchanged"
	case typeValidator:
		return `  // Example: Block delete operations
  const payload = ctx.payload || {};
  const request = payload.request || {};
  const params = request.Params || {};
  const toolName = (params.name || "");

  if (toolName.toLowerCase().includes("delete")) {
    ctx.event = {
      ...(ctx.event || {}),
      status: 403,
      error: "Delete operations not allowed",
      success: false
    };
    payload.result = {
      content: [{ type: "text", text: "Delete operations not allowed" }],
      isError: true
    };
    ctx.payload = payload;
  }`
	case typeTransformer:
		return `  // Example: Add custom argument to tool request
  const payload = ctx.payload || {};
  const request = payload.request || {};
  const params = request.Params || {};
  const argumentsObject = params.arguments;

  if (argumentsObject && typeof argumentsObject === "object" && !Array.isArray(argumentsObject)) {
    argumentsObject["x-processor"] = "PROCESSOR_NAME";
    params.arguments = argumentsObject;
    request.Params = params;
    payload.request = request;
    ctx.payload = payload;
  }`
	case typeLogger:
		return `  // Example: Log to file
  const fs = require("fs");
  const os = require("os");
  const path = require("path");

  const payload = ctx.payload || {};
  const request = payload.request || {};
  const params = request.Params || {};

  const logEntry = {
    timestamp: new Date().toISOString(),
    direction: ctx.direction || "unknown",
    tool_name: params.name || "unknown"
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
		return "  // Passthrough: return context unchanged"
	case typeValidator:
		return `  // Example: Block delete operations
  const payload = ctx.payload || {};
  const request = payload.request || {};
  const params = request.Params || {};
  const toolName = (params.name || "");

  if (toolName.toLowerCase().includes("delete")) {
    ctx.event = {
      ...(ctx.event || {}),
      status: 403,
      error: "Delete operations not allowed",
      success: false
    };
    payload.result = {
      content: [{ type: "text", text: "Delete operations not allowed" }],
      isError: true
    };
    ctx.payload = payload;
  }`
	case typeTransformer:
		return `  // Example: Add custom argument to tool request
  const payload = ctx.payload || {};
  const request = payload.request || {};
  const params = request.Params || {};
  const argumentsObject = params.arguments;

  if (argumentsObject && typeof argumentsObject === "object" && !Array.isArray(argumentsObject)) {
    argumentsObject["x-processor"] = "PROCESSOR_NAME";
    params.arguments = argumentsObject;
    request.Params = params;
    payload.request = request;
    ctx.payload = payload;
  }`
	case typeLogger:
		return `  // Example: Log to file
  const fs = require("fs");
  const os = require("os");
  const path = require("path");

  const payload = ctx.payload || {};
  const request = payload.request || {};
  const params = request.Params || {};

  const logEntry = {
    timestamp: new Date().toISOString(),
    direction: ctx.direction || "unknown",
    tool_name: params.name || "unknown"
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
		return "# Passthrough: return context unchanged"
	case typeValidator:
		return `# Example: Block delete operations
TOOL_NAME=$(echo "$CTX" | jq -r '.payload.request.Params.name // empty')
if echo "$TOOL_NAME" | grep -iq "delete"; then
  CTX=$(echo "$CTX" | jq '
    .event = ((.event // {}) + {
      "status": 403,
      "error": "Delete operations not allowed",
      "success": false
    })
    | .payload = (.payload // {})
    | .payload.result = {
      "content": [{"type": "text", "text": "Delete operations not allowed"}],
      "isError": true
    }')
fi`
	case typeTransformer:
		return `# Example: Add custom argument to tool request
CTX=$(echo "$CTX" | jq '
  .payload = (.payload // {})
  | .payload.request = (.payload.request // {})
  | .payload.request.Params = (.payload.request.Params // {})
  | .payload.request.Params.arguments = (.payload.request.Params.arguments // {})
  | .payload.request.Params.arguments["x-processor"] = "PROCESSOR_NAME"')`
	case typeLogger:
		return `# Example: Log to file
LOG_FILE="$HOME/centian/logs/processor.log"
mkdir -p "$(dirname "$LOG_FILE")"
DIRECTION=$(echo "$CTX" | jq -r '.direction // "unknown"')
TOOL_NAME=$(echo "$CTX" | jq -r '.payload.request.Params.name // "unknown"')
echo "{\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"direction\":\"$DIRECTION\",\"tool_name\":\"$TOOL_NAME\"}" >> "$LOG_FILE"`
	case typeCustom:
		return "# TODO: Add your custom logic here"
	default:
		return ""
	}
}

func writeTestInput(path string) error {
	const testPayload = `{
  "version": "1.0",
  "event": {
    "status": 200,
    "success": true,
    "message_type": "request",
    "direction": "CLIENT_TO_SERVER"
  },
  "payload": {
    "request": {
      "Params": {
        "name": "test_tool",
        "arguments": {
          "test_key": "test_value"
        }
      }
    }
  },
  "routing": {
    "server_name": "test",
    "tool_name": "test_tool",
    "original_server_name": "test",
    "original_tool_name": "test_tool"
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
		Required: false,
	})
	return config.SaveConfig(cfg)
}

func printNextSteps(out io.Writer, lang scaffoldLanguage, name, outputFile string, addedToConfig bool) {
	fmt.Fprintln(out)
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
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional

def compact_dict(value: Dict[str, Any]) -> Dict[str, Any]:
    return {key: item for key, item in value.items() if item is not None}


@dataclass
class CallToolParamsRaw:
    name: str = ""
    arguments: Any = None
    meta: Optional[Dict[str, Any]] = None

    @staticmethod
    def from_dict(data: Optional[Dict[str, Any]]) -> "CallToolParamsRaw":
        source = data or {}
        return CallToolParamsRaw(
            name=source.get("name", ""),
            arguments=source.get("arguments"),
            meta=source.get("_meta"),
        )

    def to_dict(self) -> Dict[str, Any]:
        return compact_dict({
            "name": self.name,
            "arguments": self.arguments,
            "_meta": self.meta,
        })


@dataclass
class CallToolRequest:
    params: Optional[CallToolParamsRaw] = None

    @staticmethod
    def from_dict(data: Optional[Dict[str, Any]]) -> "CallToolRequest":
        source = data or {}
        params_source = source.get("Params")
        if params_source is None:
            params_source = source.get("params")

        params = None
        if isinstance(params_source, dict):
            params = CallToolParamsRaw.from_dict(params_source)
        return CallToolRequest(params=params)

    def to_dict(self) -> Dict[str, Any]:
        return compact_dict({
            "Params": self.params.to_dict() if self.params else None,
        })


@dataclass
class CallToolResult:
    content: List[Any] = field(default_factory=list)
    structured_content: Any = None
    is_error: bool = False
    meta: Optional[Dict[str, Any]] = None

    @staticmethod
    def from_dict(data: Optional[Dict[str, Any]]) -> "CallToolResult":
        source = data or {}
        raw_content = source.get("content")
        content = raw_content if isinstance(raw_content, list) else []
        return CallToolResult(
            content=content,
            structured_content=source.get("structuredContent"),
            is_error=bool(source.get("isError", False)),
            meta=source.get("_meta"),
        )

    def to_dict(self) -> Dict[str, Any]:
        return compact_dict({
            "content": self.content,
            "structuredContent": self.structured_content,
            "isError": True if self.is_error else None,
            "_meta": self.meta,
        })


@dataclass
class PayloadPart:
    request: Optional[CallToolRequest] = None
    original_request: Optional[CallToolRequest] = None
    result: Optional[CallToolResult] = None
    original_result: Optional[CallToolResult] = None

    @staticmethod
    def from_dict(data: Optional[Dict[str, Any]]) -> "PayloadPart":
        source = data or {}
        return PayloadPart(
            request=CallToolRequest.from_dict(source["request"]) if isinstance(source.get("request"), dict) else None,
            original_request=CallToolRequest.from_dict(source["original_request"]) if isinstance(source.get("original_request"), dict) else None,
            result=CallToolResult.from_dict(source["result"]) if isinstance(source.get("result"), dict) else None,
            original_result=CallToolResult.from_dict(source["original_result"]) if isinstance(source.get("original_result"), dict) else None,
        )

    def to_dict(self) -> Dict[str, Any]:
        return compact_dict({
            "request": self.request.to_dict() if self.request else None,
            "original_request": self.original_request.to_dict() if self.original_request else None,
            "result": self.result.to_dict() if self.result else None,
            "original_result": self.original_result.to_dict() if self.original_result else None,
        })


@dataclass
class RoutingPart:
    server_name: str = ""
    tool_name: str = ""
    original_server_name: str = ""
    original_tool_name: str = ""

    @staticmethod
    def from_dict(data: Optional[Dict[str, Any]]) -> "RoutingPart":
        source = data or {}
        return RoutingPart(
            server_name=source.get("server_name", ""),
            tool_name=source.get("tool_name", ""),
            original_server_name=source.get("original_server_name", ""),
            original_tool_name=source.get("original_tool_name", ""),
        )

    def to_dict(self) -> Dict[str, Any]:
        return compact_dict({
            "server_name": self.server_name,
            "tool_name": self.tool_name,
            "original_server_name": self.original_server_name,
            "original_tool_name": self.original_tool_name,
        })


@dataclass
class ProcessorContext:
    version: str = ""
    event: Optional[Dict[str, Any]] = None
    payload: Optional[PayloadPart] = None
    routing: Optional[RoutingPart] = None

    @staticmethod
    def from_dict(data: Dict[str, Any]) -> "ProcessorContext":
        payload_source = data.get("payload")
        routing_source = data.get("routing")
        return ProcessorContext(
            version=data.get("version", ""),
            event=data.get("event"),
            payload=PayloadPart.from_dict(payload_source) if isinstance(payload_source, dict) else None,
            routing=RoutingPart.from_dict(routing_source) if isinstance(routing_source, dict) else None,
        )

    def to_dict(self) -> Dict[str, Any]:
        return compact_dict({
            "version": self.version,
            "event": self.event,
            "payload": self.payload.to_dict() if self.payload else None,
            "routing": self.routing.to_dict() if self.routing else None,
        })


def process(ctx: ProcessorContext) -> ProcessorContext:

PROCESSOR_LOGIC

    return ctx

def main():
    try:
        input_data = json.load(sys.stdin)
        ctx = ProcessorContext.from_dict(input_data)

        result = process(ctx)
        print(json.dumps(result.to_dict()))
        sys.exit(0)

    except Exception as e:
        fallback = ProcessorContext(
            event={
                "status": 500,
                "error": str(e),
                "success": False
            }
        )
        print(json.dumps(fallback.to_dict()))
        sys.exit(0)

if __name__ == "__main__":
    main()
`

const javascriptTemplate = `#!/usr/bin/env node
/**
 * Centian Processor: PROCESSOR_NAME
 * Type: PROCESSOR_TYPE
 * Generated: TIMESTAMP
 */

function processContext(ctx) {

PROCESSOR_LOGIC

  return ctx;
}

function main() {
  let input = '';

  process.stdin.on('data', chunk => {
    input += chunk;
  });

  process.stdin.on('end', () => {
    try {
      const event = JSON.parse(input);
      const result = processContext(event);
      console.log(JSON.stringify(result));
      process.exit(0);
    } catch (err) {
      const result = {
        event: {
          status: 500,
          error: err.message,
          success: false
        }
      };
      console.log(JSON.stringify(result))
      process.exit(0);
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

interface CallToolParamsRaw {
  name?: string;
  arguments?: Record<string, unknown>;
  _meta?: Record<string, unknown>;
}

interface CallToolRequest {
  Params?: CallToolParamsRaw;
}

interface CallToolResult {
  content?: Array<Record<string, unknown>>;
  structuredContent?: unknown;
  isError?: boolean;
  _meta?: Record<string, unknown>;
}

interface PayloadPart {
  request?: CallToolRequest;
  original_request?: CallToolRequest;
  result?: CallToolResult;
  original_result?: CallToolResult;
}

interface RoutingPart {
  server_name?: string;
  tool_name?: string;
  original_server_name?: string;
  original_tool_name?: string;
}

interface ProcessorContext {
  version?: string;
  direction?: string;
  event?: Record<string, unknown>;
  payload?: PayloadPart;
  routing?: RoutingPart;
}

function processContext(ctx: ProcessorContext): ProcessorContext {

PROCESSOR_LOGIC

  return ctx;
}

function main(): void {
  let input = '';

  process.stdin.on('data', (chunk) => {
    input += chunk;
  });

  process.stdin.on('end', () => {
    try {
      const event: ProcessorContext = JSON.parse(input);
      const result = processContext(event);
      console.log(JSON.stringify(result));
      process.exit(0);
    } catch (err) {
      const result: ProcessorContext = {
        event: {
          status: 500,
          error: (err as Error).message,
          success: false
        }
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

set -euo pipefail

# Read input context from stdin
INPUT=$(cat)
CTX="$INPUT"

if ! echo "$CTX" | jq -e '.' >/dev/null 2>&1; then
  echo '{"event":{"status":500,"error":"Invalid JSON input","success":false}}'
  exit 0
fi

PROCESSOR_LOGIC

# Return processed context
echo "$CTX"
exit 0
`
