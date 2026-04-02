# Centian

**Control plane for AI agents** — enforce structured workflows, govern tool access, and inspect every action your agent takes.

Centian sits between your AI agents and their MCP servers. All tool calls flow through Centian's proxy, giving you a single point of control for aggregation, middleware processing, workflow enforcement, and full observability.

![Centian Demo — AI agent completing a TDD task under Centian governance](docs/images/readme_hq.gif)


---

## The Problem

AI agents calling MCP tools today operate with:

- **No structure.** The agent decides what to do based on prompt text alone. There's no contract, no phased workflow, no verification that it followed a process.
- **No visibility.** You see the final output, but not the 47 tool calls the agent made along the way — or the 3 it shouldn't have made.
- **No enforcement.** You can't restrict which tools an agent uses during which phase of work, or block a tool call that violates policy.
- **Config sprawl.** Every MCP client needs separate configuration for every MCP server. Adding a server means updating every client.

Centian solves all four.

---

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/T4cceptor/centian/main/scripts/install.sh | bash
```

## Quick Start

### Local Demo (requires npx + claude/gemini)
- Claude code (using sonnet):
```bash
centian demo -a claude
```
- Gemini CLI (using gemini-2.5-flash):
```bash
centian demo -a gemini
```
- Codex: coming soon

What this does:
- Setup environment: create a local folder `.centian/demo`, copying required artifacts there (see [here](internal/agentrunner/assets)), adjusting configs.
- Start Centian server locally at an available port (selected automatically).
- Start selected coding agent in headless mode with [prompt](internal/agentrunner/assets/prompt.md).
- The Centian UI is opened in a new browser window showing the task overview page UI - once the agent registers the task at Centian you can check what the agent is doing by clicking on it and observing the MCP events.
- After the agent is done the CLI will prompt you if you want to close the server. Feel free to do so, you can run the demo multiple times, also with different agents - previous runs will be preserved.

**Note:** the demo is intended to showcase Centians capabilities and get a first impression, it is NOT a production-grade setup (e.g. `auth = false`, using `127.0.0.1`). If you want to use Centian do NOT copy-paste or reference the created config, checkout [Configuration](#configuration) for how to setup your own centian proxy.

### Using `init` for basic proxy setup (no task verification)

```bash
# 1. Install
curl -fsSL https://raw.githubusercontent.com/T4cceptor/centian/main/scripts/install.sh | bash

# 2. Initialize with a starter MCP server
centian init -q
# Optional: check created config at ~/.centian/config.json

# 3. Add your own MCP servers
centian server add --name "filesystem" --command "npx" --args "-y,@modelcontextprotocol/server-filesystem,/path/to/project"
centian server add --name "deepwiki" --url "https://mcp.deepwiki.com/mcp"

# 4. Start the proxy
centian start

# 5. Point your MCP client at Centian (use the config shown during init)
```

### With task verification

Add capabilities to your config at `~/.centian/config.json`:

```json
{
  "proxy": {
    "capabilities": {
      "taskVerification": {
        "enabled": true,
        "templatesPath": "/path/to/task-templates"
      },
      "eventStorage": {
        "enabled": true,
        "driver": "sqlite"
      },
      "ui": {
        "enabled": true
      }
    }
  }
}
```

Note: by default task-templates/integrated are automatically integrated in centian, but can/will be overwritten by templates using the same task.id

Start Centian and open the UI:

```bash
centian start
# UI available at http://localhost:9666/ui/tasks
```

The agent now has access to `centian.task_*` tools alongside your normal MCP tools. The task lifecycle:

1. `centian.task_list_templates` — discover available workflow templates
2. `centian.task_register` — start a task run from a template
3. `centian.task_complete_onboarding` — submit project context
4. `centian.task_complete_planning` — freeze the execution contract
5. `centian.task_start_step` / `centian.task_complete_step` — execute with verification
6. `centian.task_fail` / `centian.task_restart` — handle failures

---


## How It Works

### 1. One gateway, all your MCP servers

Configure your MCP servers once in Centian. Point every client at `localhost:8080`. Tool namespacing (`<server>_<tool>`) eliminates collisions automatically.

```json
{
  "gateways": {
    "default": {
      "mcpServers": {
        "filesystem": { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/project"] },
        "github": { "url": "https://api.github.com/mcp", "headers": { "Authorization": "Bearer <token>" } }
      }
    }
  }
}
```

Every client connects to one endpoint:

```json
{
  "mcpServers": {
    "centian": {
      "url": "http://127.0.0.1:8080/mcp/default",
      "headers": { "X-Centian-Auth": "<your-api-key>" }
    }
  }
}
```

### 2. Programmable middleware for tool calls

Processors intercept every tool call before and after execution. They receive the full request/response context, can modify payloads, and can abort the chain.

Use cases:
- Audit logging every tool call to a database
- Rate limiting calls that exceed thresholds
- Stripping secrets or environment variables from tool arguments
- Redacting PII from responses
- Enforcing allow-lists for which tools an agent can call

Scaffold a new processor:

```bash
centian processor new
```

### 3. Structured task verification

![Centian Demo — AI agent making a planning error](docs/images/centian_inspector_view.jpeg)

This is what makes Centian a control plane, not just a proxy.

Task verification lets you define **declarative workflow templates** in YAML. Each template describes a structured lifecycle — onboarding, planning, scaffolding, execution — with preconditions, postconditions, invariants, and per-phase tool permissions.

When an agent registers a task from a template:

1. **Onboarding** — the agent gathers project context and constraints
2. **Planning** — the agent proposes an approach, which gets **frozen into an execution contract**
3. **Execution** — the agent works through defined steps, with Centian verifying correctness at each gate
4. **Completion** — postconditions confirm the task was done right

The frozen execution contract is key: once planning completes, the agent reads from an immutable contract rather than mutable prompt context. You can prove what the agent committed to doing, and verify whether it actually did it.

**Per-phase tool governance:** each workflow node can declare which MCP tools the agent is allowed to call. During an approval-wait phase, all downstream tools are blocked. During scaffolding, you might allow filesystem access but block shell commands.

<!-- TODO: Add screenshot of task run UI showing phases, events, and inspector -->
<!-- ![Task Run Detail](docs/images/task_run_detail.png) -->

### 4. Full observability

Every MCP tool call is captured with timestamps, session IDs, request/response payloads, and — when task verification is active — the workflow context that produced it.

Without task verification, Centian logs events via structured JSONL and a queryable SQLite event store. With task verification enabled, Centian serves an embedded UI that shows agent activity **in the context of what the agent was supposed to be doing**:

- Timeline grouped by workflow phase
- Tool calls correlated to task steps
- Failed postcondition checks with detailed failure metadata
- Full request/response inspection

```bash
# CLI log access
centian logs

# Embedded UI (when task verification + UI are enabled)
# http://localhost:8080/ui/tasks
```

---

## Documentation

The deep documentation lives under [`docs/`](docs/README.md).

- [Getting Started](docs/getting-started.md)
- [Configuration Reference](docs/configuration-reference.md)
- [Processor Development](docs/processor-development.md)
- [Task Template Authoring](docs/task-template-authoring.md)
- [Taskverification Runtime](docs/TASKVERIFICATION.md)
- [MCP Proxy Best Practices](docs/mcp-proxy-best-practices.md)

## Task Templates

Templates are YAML files that define structured agent workflows. Each template specifies:

- **Required parameters** the agent must provide at registration
- **Onboarding requirements** — what context the agent needs to gather
- **Planning requirements** — what the agent must define before execution begins
- **Workflow nodes** — scaffolding, execution, and approval-wait phases
- **Tool allowlists** — which MCP tools are permitted in each phase
- **Preconditions and postconditions** — verification checks at each step boundary
- **Invariants** — conditions that must hold throughout execution

Example templates for TDD workflows are included in the repository under `task-templates/`.

The template schema is documented and designed for extensibility. Community contributions of templates for common workflows are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

---

## Configuration

Centian uses a single JSON config at `~/.centian/config.json`.

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
    "logLevel": "info",
    "capabilities": {
      "taskVerification": { "enabled": false },
      "eventStorage": { "enabled": true, "driver": "sqlite" },
      "ui": { "enabled": false }
    }
  },
  "gateways": {
    "default": {
      "mcpServers": {
        "my-server": {
          "url": "https://example.com/mcp",
          "headers": { "Authorization": "Bearer <token>" },
          "enabled": true
        }
      }
    }
  },
  "processors": []
}
```

### Endpoints

- Aggregated gateway: `http://127.0.0.1:8080/mcp/<gateway>`
- Individual server: `http://127.0.0.1:8080/mcp/<gateway>/<server>`

In aggregated mode, tools are namespaced to avoid collisions.

### Security

Binding to `0.0.0.0` is only allowed if `auth` is explicitly configured. This prevents accidental exposure.

---

## Commands

| Command | Description |
|---------|-------------|
| `centian init` | Initialize config (use `-q` for quickstart) |
| `centian start` | Start the proxy |
| `centian auth new-key` | Generate a new API key |
| `centian server add` | Add an MCP server |
| `centian server ...` | Manage MCP servers |
| `centian config ...` | Manage configuration |
| `centian processor new` | Scaffold a new processor |
| `centian logs` | View recent MCP logs |

---

## Installation - From source

```bash
git clone https://github.com/T4cceptor/centian.git
cd centian
go build -o build/centian ./cmd/main.go
```

---

## Current Status

Centian is usable and actively developed, but it's pre-1.0 with deliberate gaps. We're transparent about what works and what doesn't yet.

**Working today:**
- MCP proxy with gateway aggregation and tool namespacing
- Programmable processor chain (CLI and webhook)
- Task verification with template-based workflows, frozen execution contracts, and per-phase tool governance
- SQLite event persistence with task/action correlation
- Embedded read-only UI for task run inspection
- Structured JSONL request logging
- Auto-discovery of existing MCP configs (`centian init -p <path>`)
- API key authentication

**Known limitations:**
- Task run state is in-memory only (not restorable after restart)
- Governance is tool-level, not semantic (no read vs. write distinction within a tool)
- SQLite is the only storage backend (Postgres planned)
- OAuth support or downstream MCP servers is limited, not all flows are supported yet
- The UI is read-only (no task control actions from the UI yet)
- Approval-wait phases block tools but have no dedicated approve/resume mechanism yet

APIs and data structures may change before v1.0, particularly the processor interface and event schemas.

---

## Development

```bash
make build          # Build to build/centian
make install        # Install to ~/.local/bin/centian
make test-all       # Run unit + integration tests
make test-coverage  # Test coverage report
make lint           # Run linting
make dev            # Clean, fmt, vet, test, build
```

---

## Why "Control Plane"?

MCP proxies route traffic. Centian governs it.

The proxy is the mechanism — it's how Centian sees and controls every tool call. But the point isn't routing. The point is knowing what your agent is doing, constraining what it's allowed to do, and verifying that it did what you asked.

If you're using AI agents in environments where process matters — regulated industries, mission-critical workflows, or anywhere you need to answer "what did the agent do and why?" — that's what Centian is for.

---

## License

Apache-2.0
