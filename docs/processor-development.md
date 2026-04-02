# Processor Development

Centian processors let you inspect, modify, or reject proxied `tools/call` traffic.

This guide reflects the current processor contract and the current demo under `demo/processors/`.

## What Processors Can Do

Processors are called during proxied `tools/call` handling only. They can:

- inspect requests before they are sent downstream
- inspect results after they come back
- mutate request payloads or result payloads
- attach policy or audit logic around a tool call
- fail the current processor invocation

Centian supports two processor types:

- `cli`: Centian spawns a local command and exchanges JSON through `stdin` and `stdout`
- `webhook`: Centian sends a synchronous HTTP `POST` with JSON and expects JSON back

## Execution Model

Processors run in order.

The order is:

1. global `processors`
2. gateway-level `gateways.<name>.processors`

Failure behavior:

- if `required` is `true`, a processor failure stops the chain
- if `required` is `false`, the failure is logged and later processors still run

## Data Contract

Processors receive a JSON document with this top-level shape:

```json
{
  "version": "1.0",
  "event": {},
  "payload": {},
  "routing": {},
  "auth": {}
}
```

Only the configured `parts` are included.

Current supported parts:

- `payload`
- `meta`
- `routing`
- `auth`

If `parts` is omitted, Centian uses:

```json
["payload", "meta"]
```

### `DataContext`

| Field | Type | Present when | Notes |
| --- | --- | --- | --- |
| `version` | string | always | Current value is `1.0`. |
| `event` | object | `meta` enabled | Metadata about the proxied event. |
| `payload` | object | `payload` enabled | Current and original tool-call request/result payloads. |
| `routing` | object | `routing` enabled | Current and original server/tool routing. |
| `auth` | object | `auth` enabled | Current auth context. |

### `payload`

`payload` can contain:

- `request`
- `original_request`
- `result`
- `original_result`

`original_*` fields are snapshots. The writable fields are `request` and `result`.

### `routing`

`routing` contains:

- `server_name`
- `tool_name`
- `original_server_name`
- `original_tool_name`

### `auth`

`auth` contains the read-only auth context Centian has derived for the request.

## CLI Processors

CLI processors are the easiest way to start.

Config shape:

```json
{
  "name": "redactor",
  "type": "cli",
  "enabled": true,
  "timeout": 15,
  "parts": ["payload", "meta"],
  "required": false,
  "config": {
    "command": "python3",
    "args": ["./processors/redactor.py"]
  }
}
```

Runtime behavior:

- Centian serializes `DataContext` JSON to `stdin`
- your process must emit a full `DataContext` JSON document to `stdout`
- non-zero exit status is treated as failure
- `stderr` is ignored for the contract and may be used for debugging
- the processor command runs in the user's home directory by default

### Minimal Python Example

```python
#!/usr/bin/env python3
import json
import sys

data = json.load(sys.stdin)

payload = data.get("payload") or {}
result = payload.get("result") or {}
content = result.get("content") or []

for item in content:
    if item.get("type") == "text":
        item["text"] = item["text"].replace("secret", "[REDACTED]")

json.dump(data, sys.stdout)
```

## Webhook Processors

Webhook processors are useful when you want the processor logic to run in a separate service.

Config shape:

```json
{
  "name": "audit-webhook",
  "type": "webhook",
  "enabled": true,
  "timeout": 10,
  "parts": ["meta", "routing", "auth"],
  "required": false,
  "config": {
    "url": "http://127.0.0.1:9000/process",
    "headers": {
      "Authorization": "Bearer ${PROCESSOR_TOKEN}"
    }
  }
}
```

Runtime behavior:

- Centian sends a synchronous HTTP `POST`
- request body is the current `DataContext`
- response body must be a full `DataContext`
- non-2xx responses, invalid JSON, and timeouts are failures
- only `url` and `headers` are supported in webhook `config`

## Configuration Checklist

Before enabling a processor in Centian:

1. confirm the processor works standalone
2. confirm it returns valid JSON
3. confirm the returned JSON still includes any parts you expect Centian to apply
4. keep `timeout` short
5. decide whether the processor should be `required`

## Testing Before Wiring It In

Test the processor outside Centian first.

CLI example:

```bash
echo '{"version":"1.0","payload":{"request":{"Params":{"name":"ping","arguments":{"hello":"world"}}}}}' \
  | python3 ./processors/redactor.py
```

Webhook example:

```bash
curl -sS \
  -H 'Content-Type: application/json' \
  -d '{"version":"1.0","routing":{"server_name":"demo","tool_name":"query"}}' \
  http://127.0.0.1:9000/process
```

Then add the processor to config and start Centian.

## Recommended Patterns

- Keep processors narrow. A processor should do one thing well.
- Prefer read-only audit processors unless you truly need mutation.
- Use `parts` to minimize the amount of data a processor receives.
- Make mutation deterministic. The same input should produce the same output.
- Keep non-required processors non-essential. If failure would break correctness, mark them `required`.

## Current Demo

The maintained local processor demo lives in [`demo/processors/`](../demo/processors/README.md).

It demonstrates:

- OpenTelemetry span export for MCP tool calls
- response redaction with a gateway-level processor
- a local Docker Compose flow that builds the demo image and includes the demo processor code

## Related Reads

- [Configuration Reference](configuration-reference.md)
- [MCP Proxy Best Practices](mcp-proxy-best-practices.md)
