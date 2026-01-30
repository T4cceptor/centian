# CLI Processor Development Guide

**Version:** 1.0
**Last Updated:** 2025-12-27

A comprehensive guide to developing custom processors for Centian CLI.

---

## Table of Contents

0. [Getting Started](#getting-started)
1. [What is a Processor?](#what-is-a-processor)
2. [Understanding the Processor Contract](#understanding-the-processor-contract)
3. [Setup & Prerequisites](#setup--prerequisites)
4. [Input Structure](#input-structure)
5. [Output Structure](#output-structure)
6. [Status Codes](#status-codes)
7. [Common Processor Patterns](#common-processor-patterns)
8. [Step-by-Step Development](#step-by-step-development)
9. [Testing Your Processor](#testing-your-processor)
10. [Configuration in Centian](#configuration-in-centian)
11. [Debugging Tips](#debugging-tips)
12. [Performance Considerations](#performance-considerations)
13. [Examples](#examples)

---

## Quick Start

### Prerequisites

Before you start, ensure you have:

1. **Centian CLI installed** - Download from [releases](https://github.com/CentianAI/centian-cli/releases) or build from source
2. **Language runtime available in PATH**:
   - **Python 3.x** (recommended) - `python3 --version`
   - **Node.js** (for JavaScript/TypeScript) - `node --version`
   - **Bash with `jq`** (for shell scripts) - `jq --version`
3. **Text editor** - Any editor (VS Code, vim, nano, etc.)
4. **Command line access** - Terminal or shell

**Optional but recommended:**
- `jq` - JSON validation and formatting tool
- `chmod` - Make scripts executable (pre-installed on Unix-like systems)

### Steps
Quick-start to get a processor running in minutes:

1. Generate a scaffold:
   ```bash
   ./scripts/create-processor.sh
   ```
2. Modify the scaffolding and implement your processor logic.
3. Wire it into config (`~/.centian/config.json`):
   ```json
   {
     "processors": [
       {
         "name": "my_processor",
         "type": "cli",
         "command": "python3",
         "args": ["/Users/you/centian/processors/my_processor.py"],
         "enabled": true
       }
     ]
   }
   ```
4. Test the processor standalone with a sample input and verify JSON output:
   ```bash
   echo '{"type":"request","payload":{"method":"tools/call","params":{"name":"ping"}}}' | ./my_processor.py | jq
   ```
5. Ensure it returns status 200 to continue, or 40x/50x to reject.

## What is a Processor?

A **processor** is a composable unit that intercepts, validates, modifies, rejects or in any other way processes MCP (Model Context Protocol) messages, included but not limited to requests AND responses, as they flow through Centian's proxy layer.

**Potential Capabilities:**
- 🔍 Inspect all MCP requests and responses
- ✏️ Modify message payloads
- 🛡️ Enforce security policies
- 📊 Log and analyze communication
- ⛔ Reject requests based on custom rules

**How it Works:**
```
MCP Client → Centian Proxy → [Processor 1] → [Processor 2] → MCP Server
                                     ↓              ↓
                              Can modify      Can reject
```

- Processors execute sequentially in the order defined in your configuration (see `~/.centian/config.json`).
- Processors receive the message type (request/response), payload, timestamp, and other metadata (see below for more details)
- Each processor can:
  - **Allow**: Return status 200 to continue to the next processor
  - **Reject**: Return status 40x/50x to stop the chain and return an error
  - **Modify**: Change the payload before passing it along
- Status codes are derived from HTTP request status codes:
  - 200 = success
  - 40x for "expected" errors that are returned to the MCP client/AI agent
  - 50x for "unexpected" errors
- Processors can indicate an internal issue by using a none 0 exit code

### Communication Model

Processors are **external executables** that Centian runs as child processes:

- **Input**: JSON via stdin (standard input)
- **Output**: JSON via stdout (standard output)
- **Errors**: stderr is currently ignored (use for debugging) - Note: this might change in a future version!
- **Exit Code**: Indicates processor health
  - `0` = Processor executed successfully (check JSON status for decision)
  - `≠ 0` = Processor crashed (treated as 50x error)
  - Remember: Exit code indicates HEALTH, JSON status indicates DECISION. A processor can successfully execute (exit 0) but still reject a request (status 403). The exit code tells Centian whether the processor ran correctly, while the status code tells Centian what to do with the request.

---

## Setup & Prerequisites

### Language Requirements

Your processor can be written in **any language** that can:
1. Read JSON from stdin
2. Write JSON to stdout
3. Exit with appropriate exit codes

Important: since Centian spawns the processor as a child process, ensure that the command used to call the processor is in your PATH, otherwise the call will fail.

**Recommended Languages:**
- **Python** - Rich JSON support, easy to learn
- **JavaScript/Node.js** - Fast execution, good JSON handling
- **TypeScript** - Type safety with Node.js runtime
- **Go** - High performance, compiled binary
- **Bash** - Simple scripts with `jq` for JSON processing

### Script Setup

For interpreted languages (Python, JavaScript):

1. **Add shebang line** at the top:
   ```python
   #!/usr/bin/env python3
   ```

2. **Make executable**:
   ```bash
   chmod +x your-processor.py
   ```

3. **Test standalone**:
   ```bash
   echo '{"test": "input"}' | ./your-processor.py
   ```

---

## Input Structure

Your processor receives a JSON object via stdin with this structure:

```json
{
  "type": "request",
  "timestamp": "2025-12-14T10:30:00Z",
  "connection": {
    "server_name": "memory",
    "transport": "stdio",
    "session_id": "abc123"
  },
  "payload": {
    "method": "tools/call",
    "params": {
      "name": "query",
      "arguments": {
        "query": "SELECT * FROM users"
      }
    }
  },
  "metadata": {
    "processor_chain": ["processor1", "processor2"],
    "original_payload": {...}
  }
}
```

### Field Descriptions

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Either `"request"` (client→server) or `"response"` (server→client) |
| `timestamp` | string | ISO 8601 timestamp of when the message was received |
| `connection.server_name` | string | Name of the MCP server from config |
| `connection.transport` | string | Transport method (`"stdio"`, `"http"`, etc.) |
| `connection.session_id` | string | Unique session identifier |
| `payload` | object | The actual MCP message (structure varies by method) |
| `metadata.processor_chain` | array | List of processors that have already processed this message |
| `metadata.original_payload` | object | The original unmodified payload (before any processors) |

---

## Output Structure

Your processor must output a JSON object to stdout with this structure:

```json
{
  "status": 200,
  "payload": {
    "method": "tools/call",
    "params": {...}
  },
  "error": null,
  "metadata": {
    "processor_name": "my_processor",
    "modifications": ["sanitized SQL query"]
  }
}
```

### Field Descriptions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `status` | number | ✅ Yes | HTTP-style status code (200, 403, 500, etc.) |
| `payload` | object | ✅ Yes | Modified or original payload |
| `error` | string\|null | ✅ Yes | Error message if status is 40x/50x, otherwise null |
| `metadata.processor_name` | string | ⚠️ Recommended | Your processor's name for logging |
| `metadata.modifications` | array | ⚠️ Recommended | List of changes made (for debugging) |

---

## Status Codes

Use HTTP-style status codes to control flow:

### 200 - Success (Continue)

**Meaning**: Everything is OK, continue to next processor or forward to MCP server

**Use When:**
- Request passes validation
- Payload has been successfully modified
- No issues detected

**Example:**
```json
{
  "status": 200,
  "payload": {...},
  "error": null
}
```

### 40x - Client Error (Reject)

**Meaning**: Request violated a policy or failed validation - stop the chain and return error to client

**Common Codes:**
- `400` - Bad Request (malformed data)
- `401` - Unauthorized (authentication required)
- `403` - Forbidden (policy violation)
- `429` - Too Many Requests (rate limiting)

**Use When:**
- Security policy violated
- Validation failed
- Rate limit exceeded
- Prohibited operation attempted

**Example:**
```json
{
  "status": 403,
  "payload": {
    "error": {
      "code": -32001,
      "message": "Delete operations are not allowed",
      "data": {"policy": "no_deletions"}
    }
  },
  "error": "Delete operations are not allowed",
  "metadata": {
    "processor_name": "security_policy"
  }
}
```

### 50x - Server Error (Internal Failure)

**Meaning**: Processor encountered an internal error - stop the chain and return error to client

**Common Codes:**
- `500` - Internal Server Error (unexpected failure)
- `503` - Service Unavailable (dependency down)
- `504` - Gateway Timeout (external call timeout)

**Use When:**
- Processor logic fails unexpectedly
- External dependency unavailable
- Unable to parse input
- Configuration error

**Example:**
```json
{
  "status": 500,
  "payload": {},
  "error": "Failed to connect to validation service",
  "metadata": {
    "processor_name": "external_validator"
  }
}
```

---


## Common Processor Patterns

### 1. Passthrough (No-Op)

Simply return the input unchanged:

```python
import sys, json
event = json.load(sys.stdin)
print(json.dumps({
    "status": 200,
    "payload": event["payload"],
    "error": None,
    "metadata": {"processor_name": "passthrough"}
}))
```

**Use Case**: Testing, debugging, placeholder

---

### 2. Validator

Check conditions and reject if invalid:

```python
import sys, json

event = json.load(sys.stdin)
payload = event["payload"]

# Check if method contains "delete"
if "delete" in payload.get("method", "").lower():
    result = {
        "status": 403,
        "payload": {},
        "error": "Delete operations not allowed",
        "metadata": {"processor_name": "delete_blocker"}
    }
else:
    result = {
        "status": 200,
        "payload": payload,
        "error": None,
        "metadata": {"processor_name": "delete_blocker"}
    }

print(json.dumps(result))
```

**Use Cases**: Security policies, input validation, rate limiting

---

### 3. Transformer

Modify the payload before forwarding:

```python
import sys, json

event = json.load(sys.stdin)
payload = event["payload"]

# Add a custom header
if "params" in payload:
    if "arguments" not in payload["params"]:
        payload["params"]["arguments"] = {}
    payload["params"]["arguments"]["x-processor"] = "transformer"

print(json.dumps({
    "status": 200,
    "payload": payload,
    "error": None,
    "metadata": {
        "processor_name": "transformer",
        "modifications": ["added x-processor header"]
    }
}))
```

**Use Cases**: Data sanitization, enrichment, normalization

---

### 4. Logger

Record data and pass through:

```python
import sys, json
from datetime import datetime

event = json.load(sys.stdin)

# Log to file (stderr ignored by Centian v1)
with open("/tmp/centian-processor.log", "a") as f:
    f.write(f"{datetime.now()}: {event['type']} - {event['payload'].get('method')}\n")

# Pass through unchanged
print(json.dumps({
    "status": 200,
    "payload": event["payload"],
    "error": None,
    "metadata": {"processor_name": "logger"}
}))
```

**Use Cases**: Audit logging, analytics, monitoring

---

### 5. Request Filter

Only process specific request types:

```python
import sys, json

event = json.load(sys.stdin)
payload = event["payload"]

# Only validate "tools/call" requests
if event["type"] == "request" and payload.get("method") == "tools/call":
    # Perform validation
    if not payload.get("params", {}).get("name"):
        print(json.dumps({
            "status": 400,
            "payload": {},
            "error": "Tool name is required",
            "metadata": {"processor_name": "tool_validator"}
        }))
        sys.exit(0)

# Pass through
print(json.dumps({
    "status": 200,
    "payload": payload,
    "error": None,
    "metadata": {"processor_name": "tool_validator"}
}))
```

**Use Cases**: Method-specific validation, server-specific logic

---

## Testing Your Processor

### Manual Testing

**1. Create Test Cases**

Create multiple test input files for different scenarios:

```bash
# test-success.json - Should pass
# test-blocked.json - Should be rejected
# test-malformed.json - Should handle gracefully
```

**2. Run Tests**

```bash
cat test-success.json | ./my_processor.py | jq
cat test-blocked.json | ./my_processor.py | jq
cat test-malformed.json | ./my_processor.py | jq
```

**3. Verify Output**

Check:
- ✅ Valid JSON structure
- ✅ Correct status codes
- ✅ Appropriate error messages
- ✅ Exit code is 0 (even for rejections)

### Automated Testing

Create a simple test script:

```bash
#!/bin/bash
# test-processor.sh

PROCESSOR="./my_processor.py"
FAILED=0

test_case() {
    local name=$1
    local input=$2
    local expected_status=$3

    result=$(echo "$input" | $PROCESSOR)
    status=$(echo "$result" | jq -r '.status')

    if [ "$status" = "$expected_status" ]; then
        echo "✅ $name"
    else
        echo "❌ $name (expected $expected_status, got $status)"
        FAILED=1
    fi
}

# Test cases
test_case "Allow normal request" '{"type":"request","payload":{"method":"tools/call","params":{"name":"safe_tool"}}}' "200"
test_case "Block dangerous tool" '{"type":"request","payload":{"method":"tools/call","params":{"name":"delete_user"}}}' "403"

if [ $FAILED -eq 0 ]; then
    echo "All tests passed!"
else
    echo "Some tests failed!"
    exit 1
fi
```

---

## Configuration in Centian

### Add to Config File

Edit `~/.centian/config.json`:

```json
{
  "processors": [
    {
      "name": "security_policy",
      "type": "cli",
      "command": "python3",
      "args": ["/Users/yourname/centian/processors/my_processor.py"],
      "enabled": true
    }
  ]
}
```

### Configuration Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | ✅ Yes | Unique processor identifier |
| `type` | string | ✅ Yes | Always `"cli"` for v1 |
| `command` | string | ✅ Yes | Executable to run (`python3`, `node`, etc.) |
| `args` | array | ✅ Yes | Arguments including script path |
| `enabled` | boolean | ✅ Yes | Whether to execute this processor |

### Multiple Processors

Processors execute in order:

```json
{
  "processors": [
    {
      "name": "logger",
      "type": "cli",
      "command": "python3",
      "args": ["~/centian/processors/logger.py"],
      "enabled": true
    },
    {
      "name": "security_check",
      "type": "cli",
      "command": "python3",
      "args": ["~/centian/processors/security.py"],
      "enabled": true
    },
    {
      "name": "sanitizer",
      "type": "cli",
      "command": "node",
      "args": ["~/centian/processors/sanitizer.js"],
      "enabled": true
    }
  ]
}
```

**Execution Flow:**
1. Request arrives at Centian
2. `logger` processes (status 200 → continue)
3. `security_check` processes (status 200 → continue)
4. `sanitizer` processes (status 200 → continue)
5. Request forwarded to MCP server
6. Response received
7. Same chain executes in reverse for response

---

## Debugging Tips

### 1. Use stderr for Debug Logging

```python
import sys

# This won't affect Centian (stderr ignored in v1)
print(f"DEBUG: Processing {event['type']}", file=sys.stderr)
```

### 2. Test in Isolation

Always test your processor standalone before adding to config:

```bash
cat test-input.json | ./processor.py
```

### 3. Validate JSON Output

Use `jq` to validate and format JSON:

```bash
cat test-input.json | ./processor.py | jq
```

If `jq` fails, your output isn't valid JSON.

### 4. Check Exit Codes

```bash
cat test-input.json | ./processor.py
echo "Exit code: $?"  # Should always be 0
```

### 5. Enable Centian Logging

Centian logs processor execution:

```
[INFO] Executing processor: security_check
[INFO] Processor security_check completed: status=200, duration=15ms
```

### 6. Common Issues

**Problem**: `permission denied` error
**Solution**: Make script executable: `chmod +x processor.py`

**Problem**: `command not found`
**Solution**: Use full path to interpreter: `/usr/bin/python3` instead of `python3`

**Problem**: Processor times out
**Solution**: Ensure processor completes within 15 seconds (default timeout)

**Problem**: Invalid JSON error
**Solution**: Check for extra print statements. Only output the result JSON.

---

## Performance Considerations

### Timeout

- **Default**: 15 seconds per processor
- **Future**: Configurable per processor (see Issue #31)
- **Best Practice**: Keep processors fast (<100ms ideal)

### Execution Frequency

**Important**: Processors run on **EVERY** MCP message:
- Every request from client to server
- Every response from server to client
- Includes initialization, tool calls, resource lists, etc.

**Impact**: A slow processor will slow down ALL MCP communication.

### Optimization Tips

**1. Avoid Blocking I/O**

```python
# ❌ Bad: Network call on every request
response = requests.get("https://api.example.com/validate")

# ✅ Good: Use local validation
if validate_locally(payload):
    ...
```

**2. Cache Expensive Operations**

```python
import functools

@functools.lru_cache(maxsize=128)
def load_blocked_list():
    with open("blocked.json") as f:
        return json.load(f)
```

**3. Early Return**

```python
# Skip processing if not relevant
if event["type"] != "request":
    return passthrough(event)

if event["payload"].get("method") != "tools/call":
    return passthrough(event)

# Now do expensive work
```

**4. Use Compiled Languages for Heavy Work**

For high-throughput scenarios, consider Go or Rust:
- Python/Node: ~10-50ms startup overhead
- Go binary: ~1-5ms startup overhead

---

## Examples

### Example 1: Request Logger (Python)

```python
#!/usr/bin/env python3
import sys
import json
from datetime import datetime

event = json.load(sys.stdin)

# Log to file
log_entry = {
    "timestamp": datetime.now().isoformat(),
    "type": event["type"],
    "server": event["connection"]["server_name"],
    "method": event["payload"].get("method", "unknown")
}

with open("/tmp/centian-requests.log", "a") as f:
    f.write(json.dumps(log_entry) + "\n")

# Pass through unchanged
print(json.dumps({
    "status": 200,
    "payload": event["payload"],
    "error": None,
    "metadata": {"processor_name": "request_logger"}
}))
```

### Example 2: SQL Injection Filter (Python)

```python
#!/usr/bin/env python3
import sys
import json
import re

event = json.load(sys.stdin)
payload = event["payload"]

# Only check tool calls
if payload.get("method") != "tools/call":
    print(json.dumps({
        "status": 200,
        "payload": payload,
        "error": None,
        "metadata": {"processor_name": "sql_filter"}
    }))
    sys.exit(0)

# Check for SQL injection patterns
sql_patterns = [
    r";\s*DROP\s+TABLE",
    r"'\s*OR\s+'1'\s*=\s*'1",
    r"--\s*$",
    r"UNION\s+SELECT"
]

args_str = json.dumps(payload.get("params", {}).get("arguments", {}))

for pattern in sql_patterns:
    if re.search(pattern, args_str, re.IGNORECASE):
        print(json.dumps({
            "status": 403,
            "payload": {},
            "error": "Potential SQL injection detected",
            "metadata": {
                "processor_name": "sql_filter",
                "pattern_matched": pattern
            }
        }))
        sys.exit(0)

# Safe - pass through
print(json.dumps({
    "status": 200,
    "payload": payload,
    "error": None,
    "metadata": {"processor_name": "sql_filter"}
}))
```

### Example 3: Rate Limiter (JavaScript)

```javascript
#!/usr/bin/env node
const fs = require('fs');

const RATE_LIMIT_FILE = '/tmp/centian-rate-limit.json';
const MAX_REQUESTS = 10;
const WINDOW_MS = 60000; // 1 minute

// Read stdin
let input = '';
process.stdin.on('data', chunk => input += chunk);
process.stdin.on('end', () => {
  const event = JSON.parse(input);

  // Only rate limit requests
  if (event.type !== 'request') {
    return success(event.payload);
  }

  const serverId = event.connection.server_name;
  const now = Date.now();

  // Load rate limit state
  let state = {};
  if (fs.existsSync(RATE_LIMIT_FILE)) {
    state = JSON.parse(fs.readFileSync(RATE_LIMIT_FILE, 'utf8'));
  }

  // Initialize server entry
  if (!state[serverId]) {
    state[serverId] = { count: 0, windowStart: now };
  }

  // Check if window expired
  if (now - state[serverId].windowStart > WINDOW_MS) {
    state[serverId] = { count: 0, windowStart: now };
  }

  // Check rate limit
  if (state[serverId].count >= MAX_REQUESTS) {
    reject(429, 'Rate limit exceeded');
  } else {
    state[serverId].count++;
    fs.writeFileSync(RATE_LIMIT_FILE, JSON.stringify(state));
    success(event.payload);
  }
});

function success(payload) {
  console.log(JSON.stringify({
    status: 200,
    payload: payload,
    error: null,
    metadata: { processor_name: 'rate_limiter' }
  }));
}

function reject(status, error) {
  console.log(JSON.stringify({
    status: status,
    payload: {},
    error: error,
    metadata: { processor_name: 'rate_limiter' }
  }));
}
```

---

---

## Further Reading

- **MCP Specification**: https://spec.modelcontextprotocol.io/
- **Issue Tracker**: [GitHub Issues](https://github.com/CentianAI/centian-cli/issues)

---

## Contributing

Found a bug or have a feature request? Please [open an issue](https://github.com/CentianAI/centian-cli/issues/new).

Want to contribute a processor example? Submit a PR with your processor in `examples/processors/`.
