# Processor Development Guide

**Version:** 1.0
**Last Updated:** 2026-03-16

A guide to developing custom processors for Centian.

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

1. **Centian installed** - Download from [releases](https://github.com/T4cceptor/centian/releases) or build from source
2. **Choose a processor runtime**:
   - **CLI processor** - local executable/script invoked by Centian
   - **Webhook processor** - remote HTTP endpoint that accepts JSON `POST` requests
3. **Language runtime available in PATH** (CLI processors only):
   - **Python 3.x** (recommended) - `python3 --version`
   - **Node.js** (for JavaScript/TypeScript) - `node --version`
   - **Bash with `jq`** (for shell scripts) - `jq --version`
4. **Text editor** - Any editor (VS Code, vim, nano, etc.)
5. **Command line access** - Terminal or shell

**Optional but recommended:**
- `jq` - JSON validation and formatting tool
- `chmod` - Make scripts executable (pre-installed on Unix-like systems)

### Steps
Quick-start to get a processor running in minutes:

1. Choose a runtime:
   - **CLI**: generate a scaffold
   ```bash
   centian processor new
   ```
   - **Webhook**: implement an HTTP handler that accepts and returns `DataContext` JSON.
2. Implement your processor logic.
3. Register it in config (`~/.centian/config.json`) or via `centian processor add`:
   ```json
   {
     "processors": [
       {
          "name": "my_processor",
          "type": "cli",
         "enabled": true,
         "config": {
           "command": "python3",
           "args": ["/Users/you/centian/processors/my_processor.py"]
         }
       },
       {
         "name": "audit-webhook",
         "type": "webhook",
         "enabled": true,
         "config": {
           "url": "https://example.com/processors/audit",
           "headers": {
             "Authorization": "Bearer ${TOKEN}"
           }
         }
       }
     ]
   }
   ```
4. Test the processor with a sample `DataContext` and verify JSON output:
   ```bash
   echo '{"version":"1.0","payload":{"request":{"Params":{"name":"ping","arguments":{"hello":"world"}}}}}' | ./my_processor.py | jq
   ```
5. Ensure it returns valid JSON. Processors modify the current context by returning an updated `DataContext`.

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
- Processors receive a reduced `DataContext` JSON document built from the configured parts (`payload`, `meta`, `routing`, `auth`).
- A processor modifies the current call by returning an updated `DataContext`.
- If a processor execution fails:
  - required processors stop the chain
  - non-required processors are skipped and later processors still run

### Communication Model

Centian supports two processor transports that share the same JSON contract:

- **CLI processors**
  - Input: JSON via `stdin`
  - Output: JSON via `stdout`
  - Errors: `stderr` is ignored by Centian and can be used for debugging
  - Exit code: non-zero means processor execution failed
- **Webhook processors**
  - Transport: synchronous HTTP `POST`
  - Request body: JSON `DataContext`
  - Response body: JSON `DataContext`
  - Non-2xx responses, invalid JSON, transport failures, and timeouts are treated as processor execution failures

Webhook processor v1 constraints:

- synchronous request/response only
- `POST` only
- one HTTP request per invocation
- no retries or backoff
- no streaming or callback workflows
- `http` and `https` are allowed, but `https` is recommended outside local development

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

Processors receive a JSON object with this structure. CLI processors read it from `stdin`; webhook processors receive it as the HTTP request body.

```json
{
  "version": "1.0",
  "event": {
    "status": 0,
    "timestamp": "2025-12-14T10:30:00Z",
    "transport": "http",
    "request_id": "req-abc123",
    "direction": "[CLIENT -> SERVER]",
    "message_type": "request",
    "success": true,
    "modified": false
  },
  "payload": {
    "request": {
      "Params": {
        "name": "query",
        "arguments": {
          "query": "SELECT * FROM users"
        },
        "_meta": {
          "progressToken": "abc123"
        }
      }
    },
    "original_request": {
      "Params": {
        "name": "query",
        "arguments": {
          "query": "SELECT * FROM users"
        }
      }
    }
  },
  "routing": {
    "server_name": "memory",
    "tool_name": "query",
    "original_server_name": "memory",
    "original_tool_name": "query"
  },
  "auth": {
    "authenticated": true,
    "principal_type": "api_key",
    "gateway": "default"
  }
}
```

### Field Descriptions

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Data contract version (e.g. `"1.0"`). Follows major.minor: major is breaking, minor is additive |
| `event` | object | MCP event metadata when the `meta` part is enabled. Key fields: `direction` (`"[CLIENT -> SERVER]"` or `"[SERVER -> CLIENT]"`), `message_type` (`"request"` or `"response"`), `transport`, `timestamp`, `success` |
| `payload.request` | object | Current `tools/call` request payload when the `payload` part is enabled |
| `payload.original_request` | object | Original upstream request snapshot (read-only) |
| `payload.result` | object | Current downstream tool result if one exists |
| `payload.original_result` | object | Original downstream result snapshot (read-only) |
| `routing` | object | Current and original server/tool routing data |
| `auth` | object | Read-only auth context |

Notes:

- Only configured parts are present.
- For `tools/call`, request parameters are serialized under `payload.request.Params` (note capital `P`).
- The `event.direction` value is `"[CLIENT -> SERVER]"` for requests and `"[SERVER -> CLIENT]"` for responses.
- Processors should return the full structures they want Centian to apply. Partial request patching is not supported by the default payload handler.

---

## Output Structure

Processors must return a JSON object with the same `DataContext` shape. CLI processors write it to `stdout`; webhook processors return it as the HTTP response body.

```json
{
  "payload": {
    "request": {
      "Params": {
        "name": "query",
        "arguments": {
          "query": "SELECT id FROM users"
        }
      }
    },
    "result": {
      "content": [
        {
          "type": "text",
          "text": "sanitized"
        }
      ]
    }
  }
}
```

### Field Descriptions

| Field | Required | Description |
|-------|----------|-------------|
| `payload` | No | Updated request/result data to apply |
| `event` | No | Updated event metadata to apply |
| `routing` | No | Updated routing data to apply |
| `auth` | No | Ignored by Centian today; auth is read-only |

---

## Status Codes

The current processor contract does not use a top-level `status` field.

- Successful processors return a valid `DataContext` JSON document.
- Processor execution failure is signaled by runtime or transport failure.
- To block or reshape a tool call, return an updated MCP request or result payload.

### CLI Failure Signals

- non-zero exit code
- invalid JSON on `stdout`

### Webhook Failure Signals

- non-2xx HTTP response
- invalid JSON response body
- timeout or transport error

### Required vs Non-Required

- Required processor failure stops the chain.
- Non-required processor failure is logged and later processors continue.

### Example

```json
{
  "payload": {
    "result": {
      "isError": true,
      "content": [
        {
          "type": "text",
          "text": "Blocked by security policy"
        }
      ]
    }
  }
}
```

---


## Common Processor Patterns

### 1. Passthrough (No-Op)

Simply return the input unchanged:

```python
import sys, json
ctx = json.load(sys.stdin)
print(json.dumps(ctx))
```

**Use Case**: Testing, debugging, placeholder

---

### 2. Validator

Check conditions and inject a blocking result:

```python
import sys, json

ctx = json.load(sys.stdin)
payload = ctx.get("payload") or {}
request = payload.get("request") or {}
params = request.get("Params") or {}
tool_name = params.get("name", "")

if "delete" in tool_name.lower():
    payload["result"] = {
        "content": [{"type": "text", "text": "Delete operations not allowed"}],
        "isError": True
    }
    ctx["payload"] = payload

print(json.dumps(ctx))
```

**Use Cases**: Security policies, input validation, rate limiting

---

### 3. Transformer

Modify the payload before forwarding:

```python
import sys, json

ctx = json.load(sys.stdin)
payload = ctx.get("payload") or {}
request = payload.get("request") or {}
params = request.get("Params") or {}
arguments = params.get("arguments") or {}

if isinstance(arguments, dict):
    arguments["x-processor"] = "transformer"
    params["arguments"] = arguments
    request["Params"] = params
    payload["request"] = request
    ctx["payload"] = payload

print(json.dumps(ctx))
```

**Use Cases**: Data sanitization, enrichment, normalization

---

### 4. Logger

Record data and pass through:

```python
import sys, json
from datetime import datetime

ctx = json.load(sys.stdin)

payload = ctx.get("payload") or {}
request = payload.get("request") or {}
params = request.get("Params") or {}
tool_name = params.get("name", "unknown")
direction = (ctx.get("event") or {}).get("direction", "unknown")

# Log to file (stderr is also available for debug output)
with open("/tmp/centian-processor.log", "a") as f:
    f.write(json.dumps({"timestamp": datetime.now().isoformat(), "direction": direction, "tool": tool_name}) + "\n")

# Pass through unchanged
print(json.dumps(ctx))
```

**Use Cases**: Audit logging, analytics, monitoring

---

### 5. Request Filter

Only apply logic for specific event directions:

```python
import sys, json

ctx = json.load(sys.stdin)
event = ctx.get("event") or {}

# Only act on requests (client → server direction)
if event.get("direction") == "[CLIENT -> SERVER]":
    payload = ctx.get("payload") or {}
    request = payload.get("request") or {}
    params = request.get("Params") or {}
    tool_name = params.get("name", "")

    if not tool_name:
        payload["result"] = {
            "content": [{"type": "text", "text": "Tool name is required"}],
            "isError": True
        }
        ctx["payload"] = payload

print(json.dumps(ctx))
```

**Use Cases**: Direction-specific validation, server-specific logic

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
    local expect_error=$3  # "true" or "false"

    result=$(echo "$input" | $PROCESSOR)
    is_error=$(echo "$result" | jq -r '.payload.result.isError // false')

    if [ "$is_error" = "$expect_error" ]; then
        echo "✅ $name"
    else
        echo "❌ $name (expected isError=$expect_error, got isError=$is_error)"
        FAILED=1
    fi
}

# Test cases: use the DataContext format (version + event + payload + routing + auth)
SAFE='{"version":"1.0","event":{"direction":"[CLIENT -> SERVER]","message_type":"request","success":true},"payload":{"request":{"Params":{"name":"safe_tool","arguments":{}}}}}'
DANGEROUS='{"version":"1.0","event":{"direction":"[CLIENT -> SERVER]","message_type":"request","success":true},"payload":{"request":{"Params":{"name":"delete_user","arguments":{}}}}}'

test_case "Allow normal request" "$SAFE" "false"
test_case "Block dangerous tool" "$DANGEROUS" "true"

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
      "enabled": true,
      "parts": ["payload", "meta"],
      "config": {
        "command": "python3",
        "args": ["/Users/yourname/centian/processors/my_processor.py"]
      }
    },
    {
      "name": "audit_webhook",
      "type": "webhook",
      "enabled": true,
      "parts": ["payload", "routing"],
      "config": {
        "url": "https://example.com/processors/audit",
        "headers": {
          "Authorization": "Bearer ${TOKEN}"
        }
      }
    }
  ]
}
```

### Configuration Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | ✅ Yes | Unique processor identifier |
| `type` | string | ✅ Yes | `"cli"` or `"webhook"` |
| `enabled` | boolean | ✅ Yes | Whether to execute this processor |
| `parts` | array | No | Context parts to provide; defaults to `["payload","meta"]` |
| `timeout` | number | No | Timeout in seconds; defaults to `15` |
| `config.command` | string | CLI only | Executable to run (`python3`, `node`, etc.) |
| `config.args` | array | CLI only | Arguments including script path |
| `config.url` | string | Webhook only | HTTP(S) endpoint invoked with `POST` |
| `config.headers` | object | Webhook only | Optional string headers; supports `${VAR}` and `$VAR` env substitution |

### CLI Registration

```bash
centian processor add --path ./processors/security.py
```

### Webhook Registration

```bash
centian processor add --type webhook --url https://example.com/processors/audit \
  --header "Authorization=Bearer ${TOKEN}" \
  --header "X-Trace=trace-1"
```

### Webhook Constraints

- synchronous `POST` only
- JSON in, JSON out
- no retries
- no streaming or async callbacks

### Multiple Processors

Processors execute in order:

```json
{
  "processors": [
    {
      "name": "logger",
      "type": "cli",
      "enabled": true,
      "config": {
        "command": "python3",
        "args": ["~/centian/processors/logger.py"]
      }
    },
    {
      "name": "security_check",
      "type": "webhook",
      "enabled": true,
      "config": {
        "url": "https://example.com/processors/security"
      }
    },
    {
      "name": "sanitizer",
      "type": "cli",
      "enabled": true,
      "config": {
        "command": "node",
        "args": ["~/centian/processors/sanitizer.js"]
      }
    }
  ]
}
```

**Execution Flow:**
1. Request arrives at Centian
2. `logger` receives the current `DataContext` and can modify it
3. `security_check` receives the updated context next
4. `sanitizer` receives the latest context after that
5. Updated context is forwarded to the MCP server
6. Response is attached to the same context and later processors can modify it

---

## Debugging Tips

### 1. Use stderr for Debug Logging

```python
import sys, json

ctx = json.load(sys.stdin)
event = ctx.get("event") or {}

# This won't affect Centian (stderr ignored by Centian v1)
print(f"DEBUG: direction={event.get('direction')} message_type={event.get('message_type')}", file=sys.stderr)
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
# Skip processing if not relevant (e.g. only act on client→server requests)
event = ctx.get("event") or {}
if event.get("direction") != "[CLIENT -> SERVER]":
    print(json.dumps(ctx))
    sys.exit(0)

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

ctx = json.load(sys.stdin)

event = ctx.get("event") or {}
routing = ctx.get("routing") or {}
payload = ctx.get("payload") or {}
request = payload.get("request") or {}
params = request.get("Params") or {}

# Log to file
log_entry = {
    "timestamp": datetime.now().isoformat(),
    "direction": event.get("direction", "unknown"),
    "server": routing.get("server_name", "unknown"),
    "tool": params.get("name", "unknown")
}

with open("/tmp/centian-requests.log", "a") as f:
    f.write(json.dumps(log_entry) + "\n")

# Pass through unchanged
print(json.dumps(ctx))
```

### Example 2: SQL Injection Filter (Python)

```python
#!/usr/bin/env python3
import sys
import json
import re

ctx = json.load(sys.stdin)

event = ctx.get("event") or {}
# Only check client→server requests
if event.get("direction") != "[CLIENT -> SERVER]":
    print(json.dumps(ctx))
    sys.exit(0)

payload = ctx.get("payload") or {}
request = payload.get("request") or {}
params = request.get("Params") or {}
arguments = params.get("arguments") or {}

# Check for SQL injection patterns
sql_patterns = [
    r";\s*DROP\s+TABLE",
    r"'\s*OR\s+'1'\s*=\s*'1",
    r"--\s*$",
    r"UNION\s+SELECT"
]

args_str = json.dumps(arguments)

for pattern in sql_patterns:
    if re.search(pattern, args_str, re.IGNORECASE):
        payload["result"] = {
            "content": [{"type": "text", "text": "Potential SQL injection detected"}],
            "isError": True
        }
        ctx["payload"] = payload
        print(json.dumps(ctx))
        sys.exit(0)

# Safe - pass through
print(json.dumps(ctx))
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
  const ctx = JSON.parse(input);
  const event = ctx.event || {};
  const routing = ctx.routing || {};

  // Only rate limit client→server requests
  if (event.direction !== '[CLIENT -> SERVER]') {
    console.log(JSON.stringify(ctx));
    return;
  }

  const serverId = routing.server_name || 'unknown';
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
    const payload = ctx.payload || {};
    payload.result = {
      content: [{ type: 'text', text: 'Rate limit exceeded' }],
      isError: true
    };
    ctx.payload = payload;
  } else {
    state[serverId].count++;
    fs.writeFileSync(RATE_LIMIT_FILE, JSON.stringify(state));
  }

  console.log(JSON.stringify(ctx));
});
```

---

---

## Further Reading

- **MCP Specification**: https://spec.modelcontextprotocol.io/
- **Issue Tracker**: [GitHub Issues](https://github.com/T4cceptor/centian/issues)

---

## Contributing

Found a bug or have a feature request? Please [open an issue](https://github.com/T4cceptor/centian/issues/new).

Want to contribute a processor example? Submit a PR with your processor in `examples/processors/`.
