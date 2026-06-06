# HTTP Proxy Server Setup

The `centian start` command launches an HTTP proxy server that forwards requests to configured MCP servers and, optionally, exposes Centian-owned capabilities such as taskverification and the embedded UI.

This guide focuses on practical setup and deployment. For the full config surface, use [configuration_reference.md](./configuration_reference.md).

## Quick Start

### 1. Create a configuration file

Create `~/.centian/config.json` with at least one gateway and one active MCP server:

```json
{
  "name": "My Centian Server",
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
    "production": {
      "mcpServers": {
        "github": {
          "url": "https://api.githubcopilot.com/mcp/",
          "headers": {
            "Authorization": "Bearer ${GITHUB_PAT}"
          },
          "enabled": true,
          "description": "GitHub MCP server"
        }
      }
    }
  }
}
```

### 2. Create API keys when `auth=true`

Generate a key:

```bash
centian auth new-key
```

This prints the API key once and stores the hashed credential in the configured
auth backend. By default this is the **SQLite principals database** at
`~/.centian/principals.sqlite`.

The printed key has the form `sk-<credentialId>.<secret>`, where `<credentialId>`
lets the proxy look the credential up in O(1) before verifying the secret. Only the
secret is bcrypt-hashed; the store never holds the plaintext key. Each credential
resolves to a stable `principal_id` (`pr_...`), and authorization grants
(gateways/projects) are stored per principal.

#### Choosing a backend

Principals are global, so the backend is configured once at the top level of
`config.json`:

```json
{
  "authBackend": {
    "type": "sqlite",
    "store": "~/.centian/principals.sqlite"
  }
}
```

- `type`: `sqlite` (default) or `file`.
- `store`: backend location (SQLite db path, or `api_keys.json` path for `file`).
  Omit it to use the default path for the chosen type.

`centian auth new-key` writes to the backend defined by your config's `authBackend`
block, so the command always stays in sync with the server that reads the keys. It
uses `~/.centian/config.json` by default; point at another config with `--config`:

```bash
# Restrict a key to specific projects, name its principal
centian auth new-key --name "ci bot" --projects research

# Store the key in the backend defined by a specific config file
centian auth new-key --config /etc/centian/config.json
```

> **Upgrading from the file backend:** the default backend is now SQLite. To keep
> using `~/.centian/api_keys.json`, set `"authBackend": {"type": "file"}` in your
> config; otherwise recreate your keys with `centian auth new-key` (there is no
> automatic import). Keys created before the token-format change are no longer
> valid and must be regenerated regardless of backend.

The `file` backend stores entries as JSON:

```json
{
  "keys": [
    {
      "id": "key_0123456789abcdef",
      "hash": "$2a$10$...",
      "principal_id": "pr_1712050200000_abcdefghij",
      "created_at": "2026-04-02T10:30:00Z"
    }
  ]
}
```

If you disable auth for local testing, set `"auth": false` in your config instead.

Clients must include the proxy auth header in requests:

```text
X-Centian-Auth: <your-api-key>
```

This header is reserved for proxy auth and is not forwarded to downstream servers.

### 3. Set environment variables

If your configuration uses environment variable substitution:

```bash
export GITHUB_PAT=your_github_personal_access_token
```

Centian currently expands both `${VAR}` and `$VAR` inside HTTP header values. Expansion uses the current process environment, so unset variables become empty strings rather than causing config load to fail.

### 4. Start the server

```bash
# Using default config path (~/.centian/config.json)
centian start

# Or specify a custom config path
centian start --config-path ./my-config.json
```

### 5. Access your MCP servers

Centian exposes two route styles:

- aggregated gateway route:
  - `http://localhost:{port}/mcp/{gateway}`
- single-server route:
  - `http://localhost:{port}/mcp/{gateway}/{server}`

Examples:

- config path `gateways.production.mcpServers.github`
- aggregated endpoint `http://localhost:9666/mcp/production`
- single-server endpoint `http://localhost:9666/mcp/production/github`

The aggregated route is usually the better MCP client target because Centian namespaces downstream tools to avoid collisions.

### 6. Smoke-test the endpoint

Once Centian is running, confirm the proxy responds before wiring it into an MCP client:

```bash
curl -i \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'X-Centian-Auth: <your-api-key>' \
  http://127.0.0.1:9666/mcp/production
```

For most setups, the more useful real test is from the MCP client itself: point the client at the aggregated route first, confirm tool discovery works, then narrow to a single-server route only if you need to isolate one downstream.

## Practical Setup Patterns

### Single local stdio server

```json
{
  "name": "Simple Setup",
  "version": "1.0.0",
  "auth": true,
  "proxy": {
    "host": "127.0.0.1",
    "port": "9666",
    "timeout": 30
  },
  "gateways": {
    "main": {
      "mcpServers": {
        "memory": {
          "command": "npx",
          "args": ["-y", "@modelcontextprotocol/server-memory"],
          "enabled": true
        }
      }
    }
  }
}
```

### Aggregated gateway with multiple downstreams

```json
{
  "name": "Workbench Setup",
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
          "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
        },
        "remote-docs": {
          "url": "https://example.com/mcp",
          "headers": {
            "Authorization": "Bearer ${REMOTE_DOCS_TOKEN}"
          }
        }
      }
    }
  }
}
```

### Taskverification-enabled setup

```json
{
  "name": "Task Setup",
  "version": "1.0.0",
  "auth": true,
  "proxy": {
    "host": "127.0.0.1",
    "port": "9666",
    "timeout": 30,
    "capabilities": {
      "taskVerification": {
        "enabled": true,
        "templatesPath": "task-templates",
        "idleTimeoutSeconds": 900
      },
      "eventStorage": {
        "enabled": true,
        "driver": "sqlite"
      },
      "ui": {
        "enabled": true
      }
    }
  },
  "gateways": {
    "default": {
      "mcpServers": {
        "filesystem": {
          "command": "npx",
          "args": ["-y", "@modelcontextprotocol/server-filesystem", "."]
        }
      }
    }
  }
}
```

## Current Auth Behavior

- `auth` defaults to `true`
- if auth is enabled, clients must send a valid API key in the configured auth header
- if auth is disabled, Centian allows unauthenticated access to the proxy
- binding to `0.0.0.0` requires `auth` to be set explicitly in config

## Current Capability Notes

### Taskverification

- `taskVerification.enabled` exposes `centian.task_*` tools
- runtime disk templates load from `task-templates/` by default
- embedded templates also load from `task-templates/integrated/`
- taskverification command execution uses Centian's startup working directory

### Event storage and UI

- `eventStorage.enabled` persists task and action events to SQLite
- the task run API depends on event storage
- the embedded UI depends on event storage plus `ui.enabled`

## Downstream OAuth

Centian supports downstream OAuth for HTTP MCP servers. When any downstream server enables OAuth:

- `proxy.web.publicBaseUrl` becomes required
- it must be a valid `http://` or `https://` URL
- the downstream server must be configured with `url`, not `command`

Minimal shape:

```json
{
  "proxy": {
    "web": {
      "publicBaseUrl": "http://127.0.0.1:9666"
    }
  },
  "gateways": {
    "default": {
      "mcpServers": {
        "protected-server": {
          "url": "https://example.com/mcp",
          "oauth": {
            "enabled": true,
            "clientId": "${OAUTH_CLIENT_ID}",
            "clientSecret": "${OAUTH_CLIENT_SECRET}",
            "resource": "https://example.com/mcp",
            "issuer": "https://issuer.example"
          }
        }
      }
    }
  }
}
```

## Operational Notes

- gateway and server names must be URL-safe
- each server must define exactly one transport: `command` or `url`
- HTTP server headers support `${VAR}` and `$VAR` environment substitution
- Centian always registers the aggregated gateway route and the single-server routes for active servers

## Graceful Shutdown

Centian handles `SIGINT` and `SIGTERM` and attempts a graceful HTTP shutdown with a 10 second timeout.

Normal local flow:

- start with `centian start`
- stop with `Ctrl+C`
- wait for the shutdown log line before assuming the listener is gone

This only shuts down the Centian HTTP server gracefully. Any downstream behavior still depends on the specific stdio or HTTP server being proxied.

## Troubleshooting

`401 unauthorized`

- confirm `auth` is enabled or intentionally disabled
- confirm the client sends the configured auth header name
- confirm the plaintext key you copied is the one printed by `centian auth new-key`

`api key auth enabled but key file not found`

- run `centian auth new-key`
- confirm `~/.centian/api_keys.json` exists and is readable by the current user

`missing downstream auth header`

- confirm the relevant environment variables are exported in the same shell that starts Centian
- remember that unset variables expand to empty strings in HTTP headers

`404` on `/mcp/...`

- confirm the gateway and server names match the config exactly
- confirm the server is `enabled: true`
- prefer testing the aggregated `/mcp/{gateway}` route first

`downstream OAuth does not start`

- confirm the downstream server uses `url`, not `command`
- confirm `proxy.web.publicBaseUrl` is set to the externally reachable proxy base URL
- confirm the OAuth-enabled downstream block includes the expected client and issuer settings

## Recommended Reads

- [getting_started.md](./getting_started.md)
- [configuration_reference.md](./configuration_reference.md)
- [TASKVERIFICATION.md](./TASKVERIFICATION.md)
- [mcp_proxy_best_practices.md](./mcp_proxy_best_practices.md)
