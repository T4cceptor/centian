# Configuration Reference

Centian uses a single JSON config, by default at `~/.centian/config.json`.

This page describes the current user-facing configuration surface as implemented in `internal/config/config.go` and the server startup path.

## Config Layouts

Centian supports two config layouts:

- **Flat layout** (legacy, default from `centian init`): gateways, processors, auth, and capabilities live at the top level of the config. At runtime, these are auto-migrated into a `"default"` project via `ResolveProjects()`.
- **Project-based layout**: one or more named projects under the `projects` field. Each project holds its own gateways, processors, capabilities, auth settings, and metadata. Each project gets its own SQLite database, route prefix, and feature flags.

Both layouts cannot be mixed: the config must use either top-level `gateways` or `projects`, not both.

## Validation Modes

Centian validates config in two different ways:

- Basic validation: used when loading or saving config. Requires `version` and `proxy`, but can tolerate empty `gateways` / `projects`.
- Strict validation: used by `centian start`. Requires at least one gateway and at least one active server per gateway (checked per-project in project-based layout).

## Top-Level Fields

| Field | Type | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `name` | string | No | `"Centian Server"` in default config | Human-readable server name. |
| `version` | string | Yes | none | Must be non-empty. |
| `proxy` | object | Yes | see below | Truly global proxy settings: bind address, port, logging, timeouts. |
| `projects` | object | No | none | Map of project slug to project config. Mutually exclusive with top-level `gateways`. |
| `auth` | boolean | No | `true` | Controls proxy API-key auth (flat layout only; use per-project `auth` in project layout). |
| `authHeader` | string | No | `X-Centian-Auth` | Auth header name (flat layout only). |
| `gateways` | object | Strict mode: yes (flat layout) | `{}` | Map of gateway name to gateway config (flat layout only). |
| `processors` | array | No | `[]` | Global processor chain (flat layout only). |
| `metadata` | object | No | `{}` | Free-form metadata (flat layout only). |

## `projects`

Each project is keyed by its slug. Project slugs must be URL-safe: letters, numbers, `_`, and `-`.

| Field | Type | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `slug` | string | No | derived from map key | URL-safe project identifier. |
| `description` | string | No | none | Human-readable project description. |
| `auth` | boolean | No | `true` | Controls API-key auth for this project. |
| `authHeader` | string | No | `X-Centian-Auth` | Auth header name for this project. |
| `capabilities` | object | No | see capabilities section | Project-scoped feature flags. |
| `web` | object | No | `{}` | Public URL settings for downstream OAuth. |
| `gateways` | object | Strict mode: yes | none | Map of gateway name to gateway config. |
| `processors` | array | No | `[]` | Project-level processor chain. |
| `metadata` | object | No | `{}` | Free-form metadata. |

Runtime notes:

- The `"default"` project slug is special: its routes have no prefix (`/mcp/<gateway>`), matching the flat layout behavior.
- All other projects get a route prefix: `/<project_slug>/mcp/<gateway>`.
- Each project gets its own SQLite database at `~/.centian/projects/<slug>/events.sqlite` (the default project uses the legacy global path `~/.centian/logs/events.sqlite`).
- API keys can be scoped to specific projects via the `projects` field in `~/.centian/api_keys.json`.

## `proxy`

In the project-based layout, `proxy` holds only truly global settings. Capabilities and web settings move to each project.

| Field | Type | Required | Default | Validation / runtime behavior |
| --- | --- | --- | --- | --- |
| `host` | string | No | `127.0.0.1` | Bind address for the HTTP server. |
| `port` | string | No | `"9666"` | HTTP listen port. |
| `timeout` | integer | No | `30` | Used for HTTP read and write timeouts in seconds. |
| `logLevel` | string | No | `info` | Must be `debug`, `info`, `warn`, or `error`. |
| `logOutput` | string | No | `file` | Must be `file`, `console`, or `both`. |
| `logFile` | string | No | `~/.centian/centian.log` when file output is used | Custom path for the internal logger. |
| `capabilities` | object | No | taskverification off, event storage on, test tools off, UI off | Flat layout only. In project layout, use per-project `capabilities`. |
| `web` | object | No | `{}` | Flat layout only. In project layout, use per-project `web`. |

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

## Project-Based Example

```json
{
  "name": "Multi-Project Server",
  "version": "1.0.0",
  "proxy": {
    "host": "127.0.0.1",
    "port": "9666",
    "timeout": 45,
    "logLevel": "info",
    "logOutput": "both"
  },
  "projects": {
    "default": {
      "auth": true,
      "capabilities": {
        "eventStorage": { "enabled": true, "driver": "sqlite" },
        "ui": { "enabled": true }
      },
      "gateways": {
        "workbench": {
          "mcpServers": {
            "filesystem": {
              "command": "npx",
              "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"]
            }
          }
        }
      }
    },
    "research": {
      "description": "Isolated research project with task verification",
      "auth": true,
      "capabilities": {
        "taskVerification": { "enabled": true },
        "eventStorage": { "enabled": true },
        "ui": { "enabled": true }
      },
      "gateways": {
        "tools": {
          "mcpServers": {
            "deepwiki": {
              "url": "https://mcp.deepwiki.com/mcp"
            }
          }
        }
      }
    }
  }
}
```

In this example:

- `default` project routes are unprefixed: `/mcp/workbench`, `/ui`
- `research` project routes are prefixed: `/research/mcp/tools`, `/research/ui`
- Each project has its own SQLite database and capabilities
- API keys can be scoped to specific projects

## Related Guides

- [Getting Started](getting_started.md)
- [Processor Development Guide](processor_development_guide.md)
- [Taskverification Runtime](TASKVERIFICATION.md)
- [Task Template Authoring](TASK_TEMPLATE_AUTHORING.md)
