# Processor Development Guide

**Version:** 1.0
**Last Updated:** 2026-04-02

A guide to developing custom processors for Centian.

## Getting Started

Processors are the Centian extension point for proxied `tools/call` traffic. They can inspect, modify, or reject request and result payloads as those tool calls pass through the proxy.

Centian currently supports two processor runtimes:

- **CLI processors**
  Local executables or scripts invoked by Centian.
- **Webhook processors**
  Remote HTTP endpoints that accept a synchronous JSON `POST`.

Quick-start path:

1. choose a runtime
2. for local CLI processors, start with `centian processor new` when you want a scaffold
3. implement the processor logic
4. test it standalone with a sample `DataContext`
5. register it in `~/.centian/config.json`, via `centian processor new`, or via `centian processor add`
6. start Centian and verify behavior through a real proxied tool call

## What a Processor Is

A **processor** is a composable unit that intercepts, validates, modifies, rejects, or otherwise processes proxied `tools/call` traffic as it flows through Centian's proxy layer.

Potential uses:

- inspect proxied tool call requests and downstream tool results
- modify tool call payloads
- enforce security or policy rules
- log or export tool-call telemetry
- inject an error result when a request should be blocked

Processors execute sequentially in configuration order. The full order is:

1. global `processors`
2. gateway-level `gateways.<name>.processors`

Processors are currently invoked for proxied `tools/call` traffic only. They are not run for `initialize`, resources, prompts, completions, or other non-tool protocol surfaces.

If a processor execution fails:

- required processors stop the chain
- non-required processors are skipped and later processors still run

## Processor Transports

Centian supports two processor transports that share the same JSON contract.

### CLI processors

- input: JSON via `stdin`
- output: JSON via `stdout`
- errors: `stderr` is ignored by the processor contract and can be used for diagnostics
- exit code: non-zero means processor execution failed
- working directory: Centian currently executes CLI processors from the user's home directory by default

### Webhook processors

- transport: synchronous HTTP `POST`
- request body: JSON `DataContext`
- response body: JSON `DataContext`
- failure modes: non-2xx responses, invalid JSON, transport failures, and timeouts

Webhook processor constraints:

- synchronous request/response only
- `POST` only
- one HTTP request per invocation
- no retries or backoff
- no streaming or callback workflows
- `http` and `https` are allowed, though `https` is recommended outside local development

## Understanding the Processor Contract

Processors receive a reduced JSON document called `DataContext`.

### Input structure

CLI processors read it from `stdin`. Webhook processors receive it as the HTTP request body.

```json
{
  "version": "1.0",
  "event": {
    "status": 0,
    "timestamp": "2026-04-02T10:30:00Z",
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
  },
  "annotations": {
    "reports": []
  }
}
```

### Field descriptions

| Field | Type | Description |
| --- | --- | --- |
| `version` | string | Data contract version. Current value is `"1.0"`. |
| `event` | object | MCP event metadata when the `meta` part is enabled. |
| `payload.request` | object | Current `tools/call` request payload when the `payload` part is enabled. |
| `payload.original_request` | object | Original upstream request snapshot. |
| `payload.result` | object | Current downstream tool result if one exists. |
| `payload.original_result` | object | Original downstream result snapshot. |
| `routing` | object | Current and original server/tool routing data. |
| `auth` | object | Read-only auth context. |
| `annotations` | object | Processor-supplied reports about the event. Persisted as Centian event annotations, not applied to the MCP request or result payload. |

Notes:

- only configured parts are present
- for `tools/call`, request parameters are serialized under `payload.request.Params`
- `event.direction` is `"[CLIENT -> SERVER]"` for request-phase processing and `"[SERVER -> CLIENT]"` for response-phase processing
- `auth` is informational; it is not a writable control surface

### Output structure

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
  },
  "annotations": {
    "reports": [
      {
        "processor": "security-policy",
        "action": "redacted",
        "severity": "high",
        "message": "Suspicious tool result content was redacted.",
        "findings": [
          {
            "rule": "ignore_previous_instructions",
            "path": "payload.result.content[0].text"
          }
        ]
      }
    ]
  }
}
```

Fields you return are applied back into the current call context according to the configured `parts`.

### Processor annotations

Use the `annotations` part when a processor needs to tell Centian what it found without changing the MCP call itself. Returned annotation reports are persisted as event annotations and exposed in event APIs, while allowing `payload` to remain unchanged for observe-only processors.

## Configuration in Centian

### Scaffold a new local CLI processor

If you want a local processor script to start from, use:

```bash
centian processor new
```

This is an interactive scaffold flow for CLI processors. It currently asks for:

1. language
   Python, JavaScript, TypeScript, or Bash
2. processor type
   passthrough, validator, transformer, logger, or custom
3. processor name
   sanitized to alphanumeric, `_`, and `-`
4. output directory
   defaults to Centian's current working directory
5. whether to add the new processor to `~/.centian/config.json`

What it generates:

- one executable processor file such as `my_processor.py`, `my_processor.js`, `my_processor.ts`, or `my_processor.sh`
- a starter implementation shaped for the selected processor type
- an overwrite prompt if the target file already exists

What it does not currently generate:

- webhook processors
- test fixtures or sample `DataContext` JSON files
- gateway-scoped processor config

If you choose "add to config", Centian adds a global CLI processor entry with:

- `type: "cli"`
- `enabled: true`
- `timeout: 15`
- `required: false`
- an inferred command for the selected language:
  - `python3`
  - `node`
  - `ts-node`
  - `bash`

If you decline the config update, the scaffold prints a next-steps block showing how it expects the processor to be wired. Treat the config schema in this guide as the source of truth.

Use the scaffold when you want a correct contract-shaped starting point quickly. Use `centian processor add` when you already have an existing script or want to register a webhook processor directly.

### Add a processor to config

Edit `~/.centian/config.json`:

```json
{
  "processors": [
    {
      "name": "security-policy",
      "type": "cli",
      "enabled": true,
      "parts": ["payload", "meta"],
      "timeout": 15,
      "required": true,
      "config": {
        "command": "python3",
        "args": ["./processors/security.py"]
      }
    },
    {
      "name": "prompt_injection_guard",
      "type": "builtin",
      "enabled": true,
      "parts": ["payload", "meta", "annotations"],
      "timeout": 15,
      "required": true,
      "config": {
        "processor": "prompt_injection_guard",
        "mode": "redact"
      }
    },
    {
      "name": "audit-webhook",
      "type": "webhook",
      "enabled": true,
      "parts": ["meta", "routing", "auth"],
      "timeout": 10,
      "required": false,
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

### Configuration fields

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | Yes | Unique processor identifier within the processor list. |
| `type` | string | Yes | `"cli"`, `"webhook"`, or `"builtin"`. |
| `enabled` | boolean | Yes | Whether the processor is active. |
| `parts` | array | No | Context parts to provide. Defaults to `["payload","meta"]`. |
| `timeout` | number | No | Per-invocation timeout in seconds. Defaults to `15`. |
| `required` | boolean | No | If `true`, processor failure stops the chain. |
| `config.command` | string | CLI only | Executable to run. |
| `config.args` | array | CLI only | Arguments including script path. |
| `config.url` | string | Webhook only | HTTP(S) endpoint invoked with `POST`. |
| `config.headers` | object | Webhook only | Optional string headers. |
| `config.processor` | string | Built-in only | Built-in processor identifier, for example `prompt_injection_guard`. |
| `config.mode` | string | Built-in optional | Built-in processor mode. The prompt injection guard supports `annotate`, `error`, `redact`, and `remove`. |

Important runtime notes:

- supported processor types are `cli`, `webhook`, and `builtin`
- supported parts are only `payload`, `meta`, `routing`, `auth`, and `annotations`
- webhook `config` only supports `url` and `headers`
- built-in processors are compiled into Centian and do not require a separate executable

### Timeout behavior

- `timeout` is enforced separately for each processor invocation
- a processor can run twice for one MCP tool call:
  - once in the request phase
  - once in the response phase
- if `timeout` is omitted or set to `0`, Centian defaults it to `15`
- for CLI processors, the timeout covers the spawned process execution
- for webhook processors, the timeout covers the synchronous HTTP request/response round trip
- there are no automatic retries after a timeout

### Registration helpers

Interactive scaffold for a new local CLI processor:

```bash
centian processor new
```

CLI registration:

```bash
centian processor add --path ./processors/security.py
```

Webhook registration:

```bash
centian processor add --type webhook --url https://example.com/processors/audit \
  --header "Authorization=Bearer ${TOKEN}" \
  --header "X-Trace=trace-1"
```

Practical guidance:

- use `processor new` to generate a local CLI starting point
- use `processor add --path ...` to register an existing local script
- use `processor add --type webhook --url ...` for remote HTTP processors

## Practical Development Flow

### CLI processor example

```python
#!/usr/bin/env python3
import json
import sys

ctx = json.load(sys.stdin)
payload = ctx.get("payload") or {}
result = payload.get("result") or {}
content = result.get("content") or []

for item in content:
    if item.get("type") == "text":
        item["text"] = item["text"].replace("secret", "[REDACTED]")

json.dump(ctx, sys.stdout)
```

### Webhook processor example

Any language is fine as long as it:

1. accepts a JSON `POST`
2. returns valid JSON `DataContext`
3. completes within the configured timeout

### Common processor patterns

Passthrough:

- read input
- return it unchanged

Validator:

- inspect request or result data
- inject an error result when a policy is violated

Transformer:

- modify tool arguments or result content deterministically

Logger or telemetry exporter:

- record metadata
- pass the context through unchanged

Direction-specific logic:

- only run enforcement on request phase
- only run redaction on response phase

Concrete high-value patterns:

- SQL or shell guard:
  inspect `payload.request.Params.arguments` for obviously dangerous inputs and replace the downstream result with a structured error before forwarding
- result redactor:
  scrub secrets or PII from `payload.result.content[*].text` on the response phase only
- route-aware policy:
  branch on `routing.server_name` or `routing.tool_name` to enforce different logic for different downstreams inside one processor
- auth-aware telemetry:
  include `auth.gateway` and `auth.principal_type` in exported spans or audit events without mutating the call

## Testing Your Processor

### Manual testing

Test the processor standalone before adding it to config.

CLI example:

```bash
echo '{"version":"1.0","payload":{"request":{"Params":{"name":"ping","arguments":{"hello":"world"}}}}}' \
  | python3 ./processor.py | jq
```

Webhook example:

```bash
curl -sS \
  -H 'Content-Type: application/json' \
  -d '{"version":"1.0","routing":{"server_name":"demo","tool_name":"query"}}' \
  http://127.0.0.1:9000/process | jq
```

Check:

- valid JSON structure
- expected `payload.request` or `payload.result` mutations
- no accidental extra stdout output
- correct exit behavior for CLI processors

### What to test before wiring it in

1. request-phase handling
2. response-phase handling
3. invalid JSON handling
4. timeout behavior
5. no-op behavior when required fields are absent

### Lightweight regression harness

For CLI processors, keep a small fixture-based test loop next to the processor code:

1. store sample request-phase and response-phase `DataContext` JSON payloads under a `testdata/` directory
2. invoke the processor executable with each fixture on `stdin`
3. assert the output JSON is valid
4. assert the expected fields changed and unrelated fields remained stable

Even a shell-based harness is useful:

```bash
for f in ./testdata/*.json; do
  python3 ./processor.py < "$f" | jq >/dev/null || exit 1
done
```

That catches the most common failures early: invalid JSON output, accidental stdout logging, and missing `payload` wrappers.

## Debugging Tips

### Use stderr for CLI diagnostics

```python
import sys
print("debug message", file=sys.stderr)
```

Centian does not treat `stderr` as the processor output payload.

### Validate JSON output

```bash
cat test-input.json | ./processor.py | jq
```

If `jq` fails, your output is not valid JSON.

### Check exit behavior

```bash
cat test-input.json | ./processor.py
echo "Exit code: $?"
```

### Common issues

`permission denied`

- make the script executable with `chmod +x`

`command not found`

- ensure the configured command is available in `PATH`, or use an absolute path

`timed out`

- reduce processor latency or increase `timeout`

`invalid JSON`

- ensure only the final JSON document is written to `stdout`

`processor appears to do nothing`

- confirm the processor is attached to the right scope: global `processors` versus `gateways.<name>.processors`
- confirm the selected `parts` include the fields you expect to read or mutate
- remember that processors only run for proxied `tools/call`, not for every MCP method

## Performance Considerations

### Timeout

- default timeout is `15` seconds per invocation
- keep processors fast; sub-100ms work is a good target when possible

### Execution frequency

Processors run on proxied `tools/call` traffic only:

- once before the downstream tool call is sent
- once after a downstream result is returned

A slow processor therefore directly slows the tool-call path.

### Optimization tips

- avoid unnecessary blocking I/O
- cache expensive local reads where practical
- return early when the current direction or part set is irrelevant
- use compiled languages for heavy or high-frequency logic

## Repository Examples

The maintained local processor demo lives in [demo/processors](../demo/processors).

It demonstrates:

- OpenTelemetry span export for MCP tool calls
- response redaction with a gateway-level processor
- a local Docker Compose flow that builds the demo image and bundles the demo processor code

The dependency-free built-in prompt injection guard lives in [internal/processor/prompt_injection_guard](../internal/processor/prompt_injection_guard).

## Further Reading

- MCP Specification: https://spec.modelcontextprotocol.io/
- Processor demo: [demo/processors/README.md](../demo/processors/README.md)
- Config reference: [configuration_reference.md](./configuration_reference.md)
