#!/bin/bash

# Centian Processor Scaffold Generator
# Creates a new processor with language-specific template

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default processor directory
DEFAULT_PROCESSOR_DIR="$HOME/centian/processors"

echo -e "${BLUE}╔════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Centian Processor Scaffold Generator         ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════╝${NC}"
echo ""

# Step 1: Choose language
echo -e "${YELLOW}Step 1: Choose your language${NC}"
echo "1) Python"
echo "2) JavaScript (Node.js)"
echo "3) TypeScript (Node.js)"
echo "4) Bash"
echo ""
read -p "Select language [1-4]: " lang_choice

case $lang_choice in
    1) LANGUAGE="python" ;;
    2) LANGUAGE="javascript" ;;
    3) LANGUAGE="typescript" ;;
    4) LANGUAGE="bash" ;;
    *) echo -e "${RED}Invalid choice. Exiting.${NC}"; exit 1 ;;
esac

echo -e "${GREEN}✓ Selected: $LANGUAGE${NC}"
echo ""

# Step 2: Choose processor type
echo -e "${YELLOW}Step 2: Choose processor type${NC}"
echo "1) Passthrough (no-op, for testing)"
echo "2) Validator (accept/reject based on rules)"
echo "3) Transformer (modify payload)"
echo "4) Logger (record data, pass through)"
echo "5) Custom (minimal template)"
echo ""
read -p "Select type [1-5]: " type_choice

case $type_choice in
    1) PROCESSOR_TYPE="passthrough" ;;
    2) PROCESSOR_TYPE="validator" ;;
    3) PROCESSOR_TYPE="transformer" ;;
    4) PROCESSOR_TYPE="logger" ;;
    5) PROCESSOR_TYPE="custom" ;;
    *) echo -e "${RED}Invalid choice. Exiting.${NC}"; exit 1 ;;
esac

echo -e "${GREEN}✓ Selected: $PROCESSOR_TYPE${NC}"
echo ""

# Step 3: Enter processor name
echo -e "${YELLOW}Step 3: Enter processor name${NC}"
read -p "Processor name (e.g., my_processor): " PROCESSOR_NAME

if [ -z "$PROCESSOR_NAME" ]; then
    echo -e "${RED}Processor name cannot be empty. Exiting.${NC}"
    exit 1
fi

# Sanitize processor name (replace spaces with underscores, remove special chars)
PROCESSOR_NAME=$(echo "$PROCESSOR_NAME" | tr ' ' '_' | tr -cd '[:alnum:]_-')

echo -e "${GREEN}✓ Processor name: $PROCESSOR_NAME${NC}"
echo ""

# Step 4: Choose output directory
echo -e "${YELLOW}Step 4: Choose output directory${NC}"
read -p "Output directory [$DEFAULT_PROCESSOR_DIR]: " OUTPUT_DIR
OUTPUT_DIR=${OUTPUT_DIR:-$DEFAULT_PROCESSOR_DIR}

# Create directory if it doesn't exist
mkdir -p "$OUTPUT_DIR"

# Determine file extension
case $LANGUAGE in
    python) EXT="py" ;;
    javascript) EXT="js" ;;
    typescript) EXT="ts" ;;
    bash) EXT="sh" ;;
esac

OUTPUT_FILE="$OUTPUT_DIR/${PROCESSOR_NAME}.${EXT}"

# Check if file exists
if [ -f "$OUTPUT_FILE" ]; then
    echo -e "${RED}Error: File already exists: $OUTPUT_FILE${NC}"
    read -p "Overwrite? [y/N]: " overwrite
    if [ "$overwrite" != "y" ] && [ "$overwrite" != "Y" ]; then
        echo "Cancelled."
        exit 0
    fi
fi

echo -e "${GREEN}✓ Output: $OUTPUT_FILE${NC}"
echo ""

# Generate processor based on language and type
generate_processor() {
    case $LANGUAGE in
        python)
            generate_python
            ;;
        javascript)
            generate_javascript
            ;;
        typescript)
            generate_typescript
            ;;
        bash)
            generate_bash
            ;;
    esac
}

# Python templates
generate_python() {
    cat > "$OUTPUT_FILE" << 'PYEOF'
#!/usr/bin/env python3
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
PYEOF

    # Replace placeholders based on processor type
    case $PROCESSOR_TYPE in
        passthrough)
            LOGIC='    # Passthrough: return payload unchanged'
            ;;
        validator)
            LOGIC='    # Example: Block delete operations
    if "delete" in payload.get("method", "").lower():
        return {
            "status": 403,
            "payload": {},
            "error": "Delete operations not allowed",
            "metadata": {"processor_name": "PROCESSOR_NAME"}
        }'
            ;;
        transformer)
            LOGIC='    # Example: Add custom header
    if "params" in payload:
        if "arguments" not in payload["params"]:
            payload["params"]["arguments"] = {}
        payload["params"]["arguments"]["x-processor"] = "PROCESSOR_NAME"
        modifications = ["added x-processor header"]
    else:
        modifications = []'
            ;;
        logger)
            LOGIC='    # Example: Log to file
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
        f.write(json.dumps(log_entry) + "\n")'
            ;;
        custom)
            LOGIC='    # TODO: Add your custom logic here
    # Example:
    # if some_condition:
    #     return {
    #         "status": 403,
    #         "payload": {},
    #         "error": "Condition failed",
    #         "metadata": {"processor_name": "PROCESSOR_NAME"}
    #     }'
            ;;
    esac

    # Use perl for multi-line replacements (sed has issues with newlines)
    perl -i -pe "s|PROCESSOR_LOGIC|$LOGIC|g" "$OUTPUT_FILE"
    perl -i -pe "s/PROCESSOR_NAME/$PROCESSOR_NAME/g" "$OUTPUT_FILE"
    perl -i -pe "s/PROCESSOR_TYPE/$PROCESSOR_TYPE/g" "$OUTPUT_FILE"
    perl -i -pe "s/TIMESTAMP/$(date -u +"%Y-%m-%dT%H:%M:%SZ")/g" "$OUTPUT_FILE"
}

# JavaScript templates
generate_javascript() {
    cat > "$OUTPUT_FILE" << 'JSEOF'
#!/usr/bin/env node
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
JSEOF

    case $PROCESSOR_TYPE in
        passthrough)
            LOGIC='  // Passthrough: return payload unchanged'
            ;;
        validator)
            LOGIC='  // Example: Block delete operations
  if ((payload.method || "").toLowerCase().includes("delete")) {
    return {
      status: 403,
      payload: {},
      error: "Delete operations not allowed",
      metadata: { processor_name: "PROCESSOR_NAME" }
    };
  }'
            ;;
        transformer)
            LOGIC='  // Example: Add custom header
  if (payload.params) {
    if (!payload.params.arguments) {
      payload.params.arguments = {};
    }
    payload.params.arguments["x-processor"] = "PROCESSOR_NAME";
  }'
            ;;
        logger)
            LOGIC='  // Example: Log to file
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
  fs.appendFileSync(logFile, JSON.stringify(logEntry) + "\n");'
            ;;
        custom)
            LOGIC='  // TODO: Add your custom logic here'
            ;;
    esac

    # Use perl for multi-line replacements (sed has issues with newlines)
    perl -i -pe "s|PROCESSOR_LOGIC|$LOGIC|g" "$OUTPUT_FILE"
    perl -i -pe "s/PROCESSOR_NAME/$PROCESSOR_NAME/g" "$OUTPUT_FILE"
    perl -i -pe "s/PROCESSOR_TYPE/$PROCESSOR_TYPE/g" "$OUTPUT_FILE"
    perl -i -pe "s/TIMESTAMP/$(date -u +"%Y-%m-%dT%H:%M:%SZ")/g" "$OUTPUT_FILE"
}

# TypeScript templates
generate_typescript() {
    cat > "$OUTPUT_FILE" << 'TSEOF'
#!/usr/bin/env ts-node
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
TSEOF

    case $PROCESSOR_TYPE in
        passthrough)
            LOGIC='  // Passthrough: return payload unchanged'
            ;;
        validator)
            LOGIC='  // Example: Block delete operations
  if ((payload.method || "").toLowerCase().includes("delete")) {
    return {
      status: 403,
      payload: {},
      error: "Delete operations not allowed",
      metadata: { processor_name: "PROCESSOR_NAME" }
    };
  }'
            ;;
        transformer)
            LOGIC='  // Example: Add custom header
  if (payload.params) {
    if (!payload.params.arguments) {
      payload.params.arguments = {};
    }
    payload.params.arguments["x-processor"] = "PROCESSOR_NAME";
  }'
            ;;
        logger)
            LOGIC='  // Example: Log to file
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
  fs.appendFileSync(logFile, JSON.stringify(logEntry) + "\n");'
            ;;
        custom)
            LOGIC='  // TODO: Add your custom logic here'
            ;;
    esac

    # Use perl for multi-line replacements (sed has issues with newlines)
    perl -i -pe "s|PROCESSOR_LOGIC|$LOGIC|g" "$OUTPUT_FILE"
    perl -i -pe "s/PROCESSOR_NAME/$PROCESSOR_NAME/g" "$OUTPUT_FILE"
    perl -i -pe "s/PROCESSOR_TYPE/$PROCESSOR_TYPE/g" "$OUTPUT_FILE"
    perl -i -pe "s/TIMESTAMP/$(date -u +"%Y-%m-%dT%H:%M:%SZ")/g" "$OUTPUT_FILE"
}

# Bash templates
generate_bash() {
    cat > "$OUTPUT_FILE" << 'BASHEOF'
#!/bin/bash
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
BASHEOF

    case $PROCESSOR_TYPE in
        passthrough)
            LOGIC='# Passthrough: return payload unchanged'
            ;;
        validator)
            LOGIC='# Example: Block delete operations
METHOD=$(echo "$PAYLOAD" | jq -r ".method // empty")
if echo "$METHOD" | grep -iq "delete"; then
  jq -n '"'"'{
    status: 403,
    payload: {},
    error: "Delete operations not allowed",
    metadata: { processor_name: "PROCESSOR_NAME" }
  }'"'"'
  exit 0
fi'
            ;;
        transformer)
            LOGIC='# Example: Add custom header
PAYLOAD=$(echo "$PAYLOAD" | jq ".params.arguments[\"x-processor\"] = \"PROCESSOR_NAME\"")'
            ;;
        logger)
            LOGIC='# Example: Log to file
LOG_FILE="$HOME/centian/logs/processor.log"
mkdir -p "$(dirname "$LOG_FILE")"
echo "{\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"type\":\"$TYPE\",\"method\":\"$(echo "$PAYLOAD" | jq -r ".method // \"unknown\"")\"}" >> "$LOG_FILE"'
            ;;
        custom)
            LOGIC='# TODO: Add your custom logic here'
            ;;
    esac

    # Use perl for multi-line replacements (sed has issues with newlines)
    perl -i -pe "s|PROCESSOR_LOGIC|$LOGIC|g" "$OUTPUT_FILE"
    perl -i -pe "s/PROCESSOR_NAME/$PROCESSOR_NAME/g" "$OUTPUT_FILE"
    perl -i -pe "s/PROCESSOR_TYPE/$PROCESSOR_TYPE/g" "$OUTPUT_FILE"
    perl -i -pe "s/TIMESTAMP/$(date -u +"%Y-%m-%dT%H:%M:%SZ")/g" "$OUTPUT_FILE"
}

# Generate the processor
echo -e "${BLUE}Generating processor...${NC}"
generate_processor

# Make executable
chmod +x "$OUTPUT_FILE"

echo -e "${GREEN}✓ Processor created: $OUTPUT_FILE${NC}"
echo ""

# Create test input file
TEST_INPUT="$OUTPUT_DIR/${PROCESSOR_NAME}_test.json"
cat > "$TEST_INPUT" << 'TESTEOF'
{
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
TESTEOF

echo -e "${GREEN}✓ Test input created: $TEST_INPUT${NC}"
echo ""

# Display next steps
echo -e "${BLUE}╔════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Next Steps                                    ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${YELLOW}1. Test your processor:${NC}"

case $LANGUAGE in
    python)
        echo "   cat \"$TEST_INPUT\" | \"$OUTPUT_FILE\" | jq"
        ;;
    javascript|typescript)
        if [ "$LANGUAGE" = "typescript" ]; then
            echo "   cat \"$TEST_INPUT\" | ts-node \"$OUTPUT_FILE\" | jq"
        else
            echo "   cat \"$TEST_INPUT\" | node \"$OUTPUT_FILE\" | jq"
        fi
        ;;
    bash)
        echo "   cat \"$TEST_INPUT\" | \"$OUTPUT_FILE\" | jq"
        ;;
esac

echo ""
echo -e "${YELLOW}2. Add to Centian config (~/.centian/config.jsonc):${NC}"
echo '   {'
echo '     "processors": ['
echo '       {'
echo "         \"name\": \"$PROCESSOR_NAME\","
echo '         "type": "cli",'

case $LANGUAGE in
    python)
        echo '         "command": "python3",'
        ;;
    javascript)
        echo '         "command": "node",'
        ;;
    typescript)
        echo '         "command": "ts-node",'
        ;;
    bash)
        echo '         "command": "bash",'
        ;;
esac

echo "         \"args\": [\"$OUTPUT_FILE\"],"
echo '         "enabled": true'
echo '       }'
echo '     ]'
echo '   }'
echo ""
echo -e "${YELLOW}3. Read the full documentation:${NC}"
echo "   docs/processor_development_guide.md"
echo ""
echo -e "${GREEN}Happy coding! 🚀${NC}"
