# Getting Started

This guide walks from a fresh install to a working Centian endpoint that an MCP client can use.

## 1. Install Centian

Install the latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/T4cceptor/centian/main/scripts/install.sh | bash
```

Or build from source in this repository:

```bash
go build -o build/centian ./cmd/main.go
```

## 2. Create Config

The fastest path is:

```bash
centian init -q
```

`centian init` also has an interactive mode if you want to choose settings manually.

The quick-start flow creates:

- `~/.centian/config.json`
- at least one gateway and downstream MCP server entry
- an API key for proxy authentication
- an MCP client snippet you can paste into your agent or MCP client

If you need another key later:

```bash
centian auth new-key
```

## 3. Minimal Config Example

This is the smallest practical config for a local Centian instance that fronts one stdio MCP server:

```json
{
  "name": "Centian Server",
  "version": "1.0.0",
  "auth": true,
  "authHeader": "X-Centian-Auth",
  "proxy": {
    "host": "127.0.0.1",
    "port": "9666",
    "timeout": 30,
    "logLevel": "info",
    "logOutput": "file"
  },
  "gateways": {
    "default": {
      "mcpServers": {
        "sequential-thinking": {
          "command": "npx",
          "args": [
            "-y",
            "@modelcontextprotocol/server-sequential-thinking"
          ]
        }
      }
    }
  }
}
```

## 4. Gateway Example

Gateways let you expose multiple downstream servers behind one Centian endpoint while still keeping single-server routes available.

```json
{
  "name": "Centian Server",
  "version": "1.0.0",
  "auth": true,
  "proxy": {
    "host": "127.0.0.1",
    "port": "9666",
    "timeout": 30
  },
  "gateways": {
    "workbench": {
      "mcpServers": {
        "filesystem": {
          "command": "npx",
          "args": [
            "-y",
            "@modelcontextprotocol/server-filesystem",
            "/tmp"
          ]
        },
        "memory": {
          "command": "npx",
          "args": [
            "-y",
            "@modelcontextprotocol/server-memory"
          ]
        }
      }
    }
  }
}
```

With this config, Centian serves:

- the aggregated gateway at `/mcp/workbench`
- single-server endpoints at `/mcp/workbench/filesystem` and `/mcp/workbench/memory`

The aggregated gateway is the usual choice for agents because Centian namespaces the tools it exposes from multiple downstream servers.

## 5. Start the Proxy

```bash
centian start
```

Useful checks:

```bash
centian config validate
centian server list
```

At startup Centian logs:

- the config path
- the bind address
- auth status
- enabled optional capabilities
- the taskverification working directory when taskverification is enabled

## 6. Connect an MCP Client

For the aggregated gateway:

```json
{
  "mcpServers": {
    "centian-workbench": {
      "url": "http://127.0.0.1:9666/mcp/workbench",
      "headers": {
        "X-Centian-Auth": "<your-api-key>"
      }
    }
  }
}
```

If you disabled auth for local testing, omit the header.

## 7. Optional Capabilities

Centian can expose additional proxy-owned behavior through capabilities:

- `taskVerification` enables `centian.task_*` MCP tools.
- `eventStorage` persists task and action events to SQLite.
- `ui` serves the embedded read-only UI under `/ui`.
- `testTools` exposes Centian-owned test/debug tools.

In the flat layout, capabilities go under `proxy.capabilities`. In the project-based layout, they go on each project's `capabilities` field.

Taskverification is usually paired with `eventStorage`, and the UI only appears when both persistence and `ui.enabled` are active.

## 8. Project-Based Isolation (Optional)

For workloads that need separate databases, feature flags, or route prefixes, use the project-based config layout instead of the flat layout:

```json
{
  "name": "Centian Server",
  "version": "1.0.0",
  "proxy": {
    "host": "127.0.0.1",
    "port": "9666",
    "timeout": 30
  },
  "projects": {
    "default": {
      "auth": true,
      "capabilities": {
        "eventStorage": { "enabled": true }
      },
      "gateways": {
        "workbench": {
          "mcpServers": {
            "filesystem": {
              "command": "npx",
              "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
            }
          }
        }
      }
    }
  }
}
```

Each project gets:

- Its own route prefix (`/<project_slug>/mcp/<gateway>`) — the `"default"` project keeps unprefixed routes for backwards compatibility
- Its own SQLite database (`~/.centian/projects/<slug>/events.sqlite`)
- Its own capabilities, auth settings, and processors

You can start with the flat layout and migrate to the project-based layout later. Flat configs are auto-migrated to a `"default"` project at runtime.

## 9. Next Reads

- [Configuration Reference](configuration_reference.md)
- [Processor Development Guide](processor_development_guide.md)
- [Taskverification Runtime](TASKVERIFICATION.md)
- [Task Template Authoring](TASK_TEMPLATE_AUTHORING.md)
