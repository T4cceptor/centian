# Prompt Injection Guard Processor

This is a dependency-free built-in processor for demonstrating prompt injection detection in Centian.

It is intentionally small. StackOne Defender uses a richer two-tier design with pattern detection plus a bundled ONNX classifier. This processor implements only lightweight Tier 1-style behavior so it can ship inside the Centian binary with no extra runtime package install or separate processor build.

## Behavior

- Scans response-phase `payload.result.content[*].text`
- Scans response-phase `payload.result.structuredContent`
- Scans request-phase `payload.request.Params.arguments` when no result is present
- Detects obvious prompt injection markers such as:
  - `ignore previous instructions`
  - `SYSTEM:` / `DEVELOPER:` role markers
  - `<system>` and `[INST]` tags
  - requests to reveal hidden instructions, system prompts, secrets, tokens, or API keys
  - simple URL-encoded or Base64-encoded variants
- Supports four actions through `config.mode`:
  - `annotate`: add an annotation only and leave the MCP payload/status unchanged
  - `error`: replace the call with an MCP `isError: true` result
  - `redact`: replace suspicious strings with `[PROMPT_INJECTION_REDACTED]`
  - `remove`: remove suspicious strings or text content items
- Emits governance annotations with evidence count, severity, affected paths, matched rules, source, and flagged text ratio

This is not production-grade prompt injection protection. It is meant to make the Centian processor path easy to evaluate with simple and obvious attacks.

## Configure

Add the processor as a built-in processor. Use `required: true` if a processor failure should stop the tool call.

```json
{
  "processors": [
    {
      "name": "prompt_injection_guard",
      "type": "builtin",
      "enabled": true,
      "required": true,
      "timeout": 5,
      "parts": ["payload", "meta", "annotations"],
      "config": {
        "processor": "prompt_injection_guard",
        "mode": "error"
      }
    }
  ]
}
```

Use `mode: "annotate"` to observe only, or `mode: "redact"` / `mode: "remove"` to keep the tool call flowing with suspicious content modified instead of returning an immediate error.

## Test

From the repository root:

```bash
go test ./internal/processor/prompt_injection_guard ./internal/processor
```
