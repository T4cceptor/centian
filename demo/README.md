# Demo

This folder contains a lightweight demo for showing what Centian processors can do in practice.

Purpose:
- Demonstrate observability hooks by exporting tool-call spans via OpenTelemetry OTLP.
- Demonstrate response post-processing by redacting common sensitive patterns with regex.

These examples are educational starting points, not production-hardened security or monitoring solutions.

## Files

- `config.demo.json` - demo config template with processor wiring.
- `processors/otel_span_logger.py` - emits span data for MCP tool calls.
- `processors/response_redactor.py` - redacts sensitive patterns in response text.
- `requirements-otel.txt` - Python dependencies for the OTEL demo processor.

## Quick Run

1. Install OTEL demo dependencies:

```bash
python3 -m pip install -r demo/requirements-otel.txt
```

2. Generate a runnable config (replace placeholder path):

```bash
sed "s|__REPO_ROOT__|$(pwd)|g" demo/config.demo.json > /tmp/centian.demo.json
```

3. Set OTLP endpoint (example: local collector):

```bash
export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="http://localhost:4318/v1/traces"
```

4. Start Centian with demo config:

```bash
centian start --config-path /tmp/centian.demo.json
```

5. Point your MCP client to:

- `http://127.0.0.1:8080/mcp/demo`

Notes:
- `response_redactor.py` acts on response payloads (`payload.result`).
- `otel_span_logger.py` is best-effort and will pass through traffic if OTEL export fails.
