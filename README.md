# Centian - the MCP Proxy

Centian is a lightweight MCP ([Model Context Protocol](https://modelcontextprotocol.io/)) proxy that adds **processing hooks**, **gateway aggregation**, and **structured logging** to MCP server traffic.

## Highlights

- **Programmable MCP traffic processing** – inspect, modify, block, or enrich requests and responses with processor scripts.
- **Unified gateway for multiple servers** – expose many downstream MCP servers through one clean endpoint (DRY config).
- **Structured logging & visibility** – capture MCP events for debugging, auditing, and analysis.
- **Fast setup via auto‑discovery** – import existing MCP configs from common tools to get started quickly.

## Quick Start

1) **Install**

```bash
curl -fsSL https://github.com/T4cceptor/centian/main/scripts/install.sh | bash
```

2) **Initialize config**

```bash
centian init
```

This creates `~/.centian/config.json`.

3) **Create API key (or explicitly disable auth)**

```bash
centian server get-key
```

4) **Start the proxy**

```bash
centian server start
```

Default bind: `127.0.0.1:8080`.

> **Security note**
> Binding to `0.0.0.0` is allowed only if `auth` is explicitly set (true or false). This is enforced to reduce accidental exposure.

5) **Point your MCP client to Centian**

```json
{
  "mcpServers": {
    "centian-default": {
      "url": "http://127.0.0.1:8080/mcp/default",
      "headers": {
        "X-Centian-Auth": "<your-api-key>"
      }
    }
  }
}
```

## Configuration

Centian uses a single JSON config at `~/.centian/config.json`.

Minimal example:

```json
{
  "name": "Centian Server",
  "version": "1.0.0",
  "auth": true,
  "authHeader": "X-Centian-Auth",
  "proxy": {
    "host": "127.0.0.1",
    "port": "8080",
    "timeout": 30,
    "logLevel": "info"
  },
  "gateways": {
    "default": {
      "mcpServers": {
        "my-server": {
          "url": "https://example.com/mcp",
          "headers": {
            "Authorization": "Bearer <token>"
          },
          "enabled": true
        }
      }
    }
  },
  "processors": []
}
```

### Endpoints

- Aggregated gateway endpoint:
  - `http://127.0.0.1:8080/mcp/<gateway>`
- Individual server endpoint:
  - `http://127.0.0.1:8080/mcp/<gateway>/<server>`

In aggregated mode, tools are namespaced to avoid collisions.

## Processors

Processors let you enforce policies or transform MCP traffic (request/response). You can scaffold a processor with:

```bash
centian processor init
```

The scaffold can optionally add the processor to your config automatically.

## Logging

Centian logs MCP activity to `~/.centian/logs/`:

- `requests.jsonl` – MCP requests with timestamps and session IDs
- `proxy_operations.jsonl` – lifecycle events (start/stop)

## Commands (Quick Reference)

- `centian init` – initialize config
- `centian server start` – start the proxy
- `centian server get-key` – generate API key
- `centian config ...` – manage config
- `centian logs` – view recent logs

## Installation (More Options)

### Script (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/T4cceptor/centian/main/scripts/install.sh | bash
```

### Homebrew

Coming soon.

### From source

```bash
git clone https://github.com/T4cceptor/centian.git
cd centian-cli
go build -o build/centian ./cmd/main.go
```

## Development

```bash
make build          # Build to build/centian
make install        # Install to ~/.local/bin/centian
make test-all       # Run unit + integration tests
make test-coverage  # Runs test coverage report
make lint           # Run linting
make dev            # Clean, fmt, vet, test, build
```

## License

Apache-2.0
