# Demo

This folder provides two local demos with a single Centian config and container:

- Logging gateway (`/mcp/logging-demo`): emits OpenTelemetry tool-call spans to Jaeger.
- Redaction gateway (`/mcp/modification-demo`): masks sensitive patterns in MCP response text.

It also uses the built-in prompt injection guard processor, which demonstrates blocking obvious prompt injection markers in tool requests and results. The guard is compiled into Centian and configured with `type: "builtin"`.

## Structure

Run commands from `demo/processors`.

- `configs/demo_config.json`: Centian config with 2 demo gateways and gateway-level processors.
- `src/`: Python processors and shared helpers.
- `scripts/setup.sh`: local setup helper.
- `scripts/smoke_demo.sh`: smoke checks for both demo modes.
- `docker-compose.yml`: shared infrastructure and demo services.

Both demos run with `auth: false` for convenience. This is intentionally insecure and only safe for local testing.
Published ports are bound to `127.0.0.1` by default.

Both gateways are served by the same Centian instance on `localhost:8576`.

For the general processor contract and configuration model, see [../../docs/processor_development_guide.md](../../docs/processor_development_guide.md).

## Quickstart

1. Setup, start, and smoke test:

```bash
cd demo/processors
make setup
make demo-up
make demo-test
```

2. Add both gateways to your MCP config:

```json
{
  "mcpServers": {
    "centian-demo-logging": {
      "url": "http://localhost:8576/mcp/logging-demo"
    },
    "centian-demo-modification": {
      "url": "http://localhost:8576/mcp/modification-demo"
    }
  }
}
```

3. Logging gateway checks:

- Ask your agent to: `Use logging-demo-db___query and query for all employees data.`
- Inspect traces in Jaeger: `http://localhost:16686`

Expected:

- MCP tool calls succeed through Centian.
- Jaeger shows spans produced by `demo_otel_span_logger`.
- Tools are namespaced (for example `logging-demo-db___query`).

4. Redaction gateway checks:

- Ask your agent to: `Use modification-demo-db___query and query for all data on table sample_data_1.`

Expected redaction behavior:

- Emails become `[REDACTED_EMAIL]`
- SSNs become `[REDACTED_SSN]`
- Bearer tokens become `Bearer [REDACTED_TOKEN]`
- AWS access keys become `[REDACTED_AWS_ACCESS_KEY]`

Stop all demo services:

```bash
make demo-down
```

## Commands

- `make demo-up` builds the demo image locally and starts Postgres, Jaeger, and the unified demo container.
- `make demo-test` runs smoke checks for both gateways.
- `make demo-down` stops all demo services.
- `make clean` removes local Python cache artifacts.
