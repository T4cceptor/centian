# Configuration Reference

Centian uses a single JSON config, by default at `~/.centian/config.json`.

This page describes the current user-facing configuration surface as implemented in `internal/config/config.go` and the server startup path.

## Validation Modes

Centian validates config in two different ways:

- Basic validation: used when loading or saving config. Requires `version` and `proxy`, but can tolerate empty `gateways`.
- Strict validation: used by `centian start`. Requires at least one gateway and at least one active server per gateway.

## Top-Level Fields

| Field | Type | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `name` | string | No | `"Centian Server"` in default config | Human-readable server name. |
| `version` | string | Yes | none | Must be non-empty. |
| `auth` | boolean | No | `true` | Controls proxy API-key auth. |
| `authHeader` | string | No | `X-Centian-Auth` | Reserved for Centian auth and not forwarded downstream. |
| `proxy` | object | Yes | see below | Proxy bind, logging, timeout, and capability settings. |
| `gateways` | object | Strict mode: yes | `{}` | Map of gateway name to gateway config. |
| `processors` | array | No | `[]` | Global processor chain applied before gateway-level processors. |
| `metadata` | object | No | `{}` | Free-form metadata; not used by core runtime. |

## `proxy`

| Field | Type | Required | Default | Validation / runtime behavior |
| --- | --- | --- | --- | --- |
| `host` | string | No | `127.0.0.1` | Bind address for the HTTP server. |
| `port` | string | No | `"9666"` | HTTP listen port. |
| `timeout` | integer | No | `30` | Used for HTTP read and write timeouts in seconds. |
| `logLevel` | string | No | `info` | Must be `debug`, `info`, `warn`, or `error`. |
| `logOutput` | string | No | `file` | Must be `file`, `console`, or `both`. |
| `logFile` | string | No | `~/.centian/centian.log` when file output is used | Custom path for the internal logger. |
| `capabilities` | object | No | taskverification off, event storage on, test tools off, UI off | Controls proxy-owned optional features. |
| `web` | object | No | `{}` | Public URL settings used for downstream OAuth browser flows. |

### `proxy.web`

| Field | Type | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `publicBaseUrl` | string | Required when any downstream server enables OAuth | none | Must be a valid `http://` or `https://` URL. |

## `proxy.capabilities`

### `taskVerification`

| Field | Type | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `enabled` | boolean | No | `false` | Registers `centian.task_*` tools on Centian endpoints. |
| `templatesPath` | string | No | `{cwd}/task-templates` | Relative values resolve against Centian's current working directory. |
| `idleTimeoutSeconds` | integer | No | `0` | `0` disables task idle timeout. Positive values time out inactive active runs. |

Runtime notes:

- Taskverification command checks and invariants run in Centian's current working directory.
- Centian logs this working directory at startup when taskverification is enabled.
- Templates are loaded from embedded built-ins first and then overlaid with any disk templates from `templatesPath`.

### `eventStorage`

| Field | Type | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `enabled` | boolean | No | `true` | Enables durable event persistence. |
| `driver` | string | No | `sqlite` | Only `sqlite` is currently supported. |
| `path` | string | No | `~/.centian/logs/events.sqlite` | SQLite file path override. |

Runtime notes:

- If event storage is disabled, the task run API is not registered.
- The embedded UI also depends on event storage being available.

### `testTools`

| Field | Type | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `enabled` | boolean | No | `false` | Enables Centian-owned test/debug MCP tools. |

### `ui`

| Field | Type | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `enabled` | boolean | No | `false` | Serves the embedded UI under `/ui` if event storage is also active. |

## `gateways`

Each gateway is keyed by its gateway name. Gateway names must be URL-safe: letters, numbers, `_`, and `-`.

| Field | Type | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `mcpServers` | object | Yes in strict mode | none | Map of server name to MCP server config. |
| `processors` | array | No | `[]` | Gateway-level processor chain appended after global processors. |

Runtime notes:

- Centian always registers an aggregated gateway route at `/mcp/{gateway}`.
- For every active downstream server, Centian also registers `/mcp/{gateway}/{server}`.
- A gateway must have at least one active server in strict validation.

## `mcpServers`

Each server name must also be URL-safe.

| Field | Type | Required | Default | Validation / runtime behavior |
| --- | --- | --- | --- | --- |
| `name` | string | No | none | Human-facing name field; the map key is the route identity. |
| `command` | string | Conditionally required | none | Use for stdio/process transport. Mutually exclusive with `url`. |
| `args` | array of strings | No | none | Command arguments for stdio transport. |
| `env` | object of string:string | No | none | Environment variables for stdio transport. |
| `url` | string | Conditionally required | none | Use for HTTP transport. Must be valid `http://` or `https://`. |
| `headers` | object of string:string | No | none | HTTP headers for downstream requests. Values support `${ENV_VAR}` and `$ENV_VAR` substitution. |
| `oauth` | object | No | disabled | Only supported for HTTP MCP servers. |
| `enabled` | boolean | No | `true` | Disabled servers stay in config but are not activated. |
| `description` | string | No | none | Human-readable description. |
| `source` | string | No | none | Source path for imported or discovered config. |
| `config` | object | No | none | Server-specific metadata; not interpreted by core transport setup. |

Validation rules:

- A server must define exactly one transport: `command` or `url`.
- `url` must parse as a valid HTTP or HTTPS URL.
- Header keys and values must be non-empty strings.

## Downstream OAuth

`oauth` applies to HTTP MCP servers only.

| Field | Type | Required | Default | Validation / runtime behavior |
| --- | --- | --- | --- | --- |
| `enabled` | boolean | No | `false` | OAuth is only active when this is `true`. |
| `clientId` | string | Yes when OAuth enabled | none | Required. |
| `clientSecret` | string | Yes when OAuth enabled | none | Required. |
| `clientAuthMethod` | string | No | empty | Allowed values: empty, `client_secret_basic`, `client_secret_post`. |
| `scopes` | array of strings | No | none | Passed into downstream OAuth flow when used. |
| `resource` | string | Yes when OAuth enabled | none | Required. |
| `issuer` | string | Conditionally required | none | Must be valid `http://` or `https://` when set. |
| `authorizationEndpoint` | string | Conditionally required | none | Required if `issuer` is not set. Must be valid HTTP/HTTPS URL when set. |
| `tokenEndpoint` | string | Conditionally required | none | Required if `issuer` is not set. Must be valid HTTP/HTTPS URL when set. |

Rules:

- OAuth is rejected for stdio servers.
- Centian requires either:
  - `issuer`, or
  - both `authorizationEndpoint` and `tokenEndpoint`
- If any server enables OAuth, `proxy.web.publicBaseUrl` becomes required.

## `processors`

Processor names must be unique within a single processor list.

| Field | Type | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `name` | string | Yes | none | Unique within the list being validated. |
| `type` | string | Yes | none | Must be `cli` or `webhook`. |
| `enabled` | boolean | Yes | none | Disabled processors are skipped. |
| `timeout` | integer | No | `15` | Per-invocation timeout in seconds. |
| `parts` | array of strings | No | `["payload", "meta"]` | Allowed parts: `payload`, `meta`, `routing`, `auth`. |
| `config` | object | Yes | none | Type-specific processor config. |
| `required` | boolean | No | `false` | Required processor failures stop the chain. |

Runtime notes:

- Processors currently run only for proxied `tools/call` traffic.
- Global processors run before gateway-level processors.
- Non-required processor failures are logged and skipped; later processors still run.

### CLI Processor `config`

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `command` | string | Yes | Executable to run. |
| `args` | array of strings | No | Passed directly to the executable. |

CLI runtime behavior:

- Centian writes `DataContext` JSON to `stdin`.
- The processor must write a full `DataContext` JSON document to `stdout`.
- Non-zero exit codes are treated as processor failures.
- CLI processors run in the user's home directory by default.

### Webhook Processor `config`

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `url` | string | Yes | Must be a valid HTTP or HTTPS URL. |
| `headers` | object of string:string | No | Additional headers for the POST request. |

Webhook runtime behavior:

- Centian sends a synchronous HTTP `POST`.
- The body is the current `DataContext` JSON.
- The response must be a valid `DataContext` JSON document.
- Non-2xx responses, invalid JSON, and timeouts are treated as failures.
- `config.url` and `config.headers` are the only supported webhook config keys.

## Minimal Example

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

## Multi-Gateway Example

```json
{
  "name": "Team Proxy",
  "version": "1.0.0",
  "auth": true,
  "proxy": {
    "host": "127.0.0.1",
    "port": "9666",
    "timeout": 45,
    "logLevel": "info",
    "logOutput": "both",
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
    },
    "web": {
      "publicBaseUrl": "http://127.0.0.1:9666"
    }
  },
  "processors": [
    {
      "name": "audit",
      "type": "webhook",
      "enabled": true,
      "timeout": 10,
      "parts": ["meta", "routing", "auth"],
      "required": false,
      "config": {
        "url": "http://127.0.0.1:9000/audit"
      }
    }
  ],
  "gateways": {
    "workbench": {
      "mcpServers": {
        "filesystem": {
          "command": "npx",
          "args": [
            "-y",
            "@modelcontextprotocol/server-filesystem",
            "/workspace"
          ]
        },
        "remote-docs": {
          "url": "https://example.com/mcp",
          "headers": {
            "Authorization": "Bearer ${REMOTE_DOCS_TOKEN}"
          }
        }
      }
    },
    "oauth-demo": {
      "mcpServers": {
        "protected-api": {
          "url": "https://example.com/mcp",
          "oauth": {
            "enabled": true,
            "clientId": "${DEMO_CLIENT_ID}",
            "clientSecret": "${DEMO_CLIENT_SECRET}",
            "resource": "https://example.com/mcp",
            "issuer": "https://issuer.example.com"
          }
        }
      }
    }
  }
}
```

## Related Guides

- [Getting Started](getting_started.md)
- [Processor Development Guide](processor_development_guide.md)
- [Taskverification Runtime](TASKVERIFICATION.md)
- [Task Template Authoring](TASK_TEMPLATE_AUTHORING.md)
