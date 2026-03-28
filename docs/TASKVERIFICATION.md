# Taskverification

Centian taskverification adds a workflow-driven task runtime on top of a normal MCP proxy endpoint.
It lets an agent register a task from a template, move through workflow phases, and inspect lifecycle plus MCP action history through the same Centian deployment.

## What It Adds

Taskverification turns Centian into four things at once:

- an MCP proxy
- a task runtime
- a workflow policy layer
- an observability source for task-oriented agent runs

The feature exists to solve four practical problems:

1. Give the agent a structured lifecycle instead of leaving all process control to prompt text.
2. Freeze expectations before execution starts, so the agent reads from a contract instead of mutable ad hoc context.
3. Gate downstream MCP tool usage by workflow node, so the different workflow phases can have different permissions.
4. Persist enough lifecycle and action history to inspect task runs after the fact.

## Quickstart

Taskverification tools are disabled by default. You must opt in explicitly.

### 1. Enable the runtime

Add `proxy.capabilities.taskVerification` to your Centian config.
If you also want the embedded UI, enable `proxy.capabilities.ui`.

```json
{
  "proxy": {
    "host": "127.0.0.1",
    "port": "8080",
    "timeout": 30,
    "capabilities": {
      "taskVerification": {
        "enabled": true,
        "templatesPath": "/absolute/path/to/task-templates"
      },
      "eventStorage": {
        "enabled": true,
        "driver": "sqlite",
        "path": "/absolute/path/to/events.sqlite"
      },
      "ui": {
        "enabled": true
      }
    }
  }
}
```

Notes:

- `capabilities.taskVerification.enabled` controls whether `centian.task_*` tools are exposed.
- `capabilities.taskVerification.templatesPath` overrides where Centian looks for task templates.
  - Note: we are planning to move a basic set of templates into the binary itself or potentially retrieve it from github. However this is not yet included in `v0.2`
- `capabilities.eventStorage.enabled` defaults to `true`.
- `capabilities.eventStorage.driver` currently only supports `sqlite`.
- `capabilities.eventStorage.path` is optional. If omitted, Centian uses `~/.centian/logs/events.sqlite`.
- `capabilities.ui.enabled` only exposes the embedded frontend. It does not enable taskverification tools by itself.
  - You can also use both event storage and task verification without the UI.

### 2. Ensure templates exist

By default, taskverification loads templates from:

```text
<current-working-directory>/task-templates
```

If `proxy.capabilities.taskVerification.templatesPath` is set:

- an absolute path is used as-is
- a relative path is resolved from Centian's current working directory

This repository includes example templates under [task-templates](../task-templates).

### 3. Expose downstream tools through a normal gateway

Taskverification does not replace downstream MCP tools. It orchestrates and governs them.

A typical setup exposes project access through downstream tools such as:

- `filesystem`
- `shell`

The agent then talks to one Centian endpoint and receives both:

- normal proxied MCP tools
- `centian.task_*` tools

### 4. Start Centian

```bash
centian start
```

### 5. Use the lifecycle

The normal lifecycle is:

1. `centian.task_list_templates`
2. `centian.task_register`
3. `centian.task_complete_onboarding`
4. `centian.task_complete_planning`
5. `centian.task_start_step`
6. `centian.task_complete_step`

Additional control tools are also available:

- `centian.task_restart`
- `centian.task_fail`

Before a task is registered, only these two task tools are allowed:

- `centian.task_list_templates`
- `centian.task_register`

### 6. Inspect persisted runs

If event storage is enabled, Centian exposes a read-only task run API:

- `GET /api/task-runs`
- `GET /api/task-runs/{runID}/events`

If `proxy.capabilities.ui.enabled` is also enabled, Centian serves the embedded UI under `/ui`.
The frontend routes are:

- `/ui/tasks`
- `/ui/tasks/:runID`

## Capability Overview

Taskverification currently spans three separate capability areas in Centian config:

- `proxy.capabilities.taskVerification`
  Enables agents to be controlled via task templates. Agents can use `centian.task_*` MCP tools for process management and control. Default: `false` (meaning no process management for agents).
- `proxy.capabilities.eventStorage`
  Persists task and action events to SQLite so run summaries and timelines can be queried later. Default: `true`.
- `proxy.capabilities.ui`
  Serves the embedded frontend under `/ui`. Default: `false`.

These capabilities are related, but they are not the same thing:

- The taskverification runtime and tools are controlled by `taskVerification`.
- The historical task run API depends on persistence backing from `eventStorage`.
- The embedded UI depends on both persistence backing and `ui.enabled`.

If event storage is disabled, Centian does not register the task run API or the embedded task run UI.

## Tool Surface

The current taskverification MCP tools are:

- `centian.task_list_templates`
- `centian.task_register`
- `centian.task_complete_onboarding`
- `centian.task_complete_planning`
- `centian.task_start_step`
- `centian.task_complete_step`
- `centian.task_restart`
- `centian.task_fail`

These tools are injected as static proxy-owned tools on the Centian endpoint when `taskVerification.enabled` is true.

## Runtime Model

### Task run

A task run is one active agent task session created from one template.

Mutable run state is held in memory in a `RunState`. This includes:

- template id
- workflow phase/path
- onboarding artifact
- planning artifact
- frozen execution contract
- per-step runtime state
- failure metadata

Current mutable run state is not durably restorable yet.

### Workflow path

Taskverification uses workflow paths instead of a coarse lifecycle enum.

Examples:

- `onboarding`
- `planning`
- `scaffolding.setup_test_file`
- `execution.implement_fix`
- `waiting_for_approval.review_plan`

The path tells you where the run is.
The node kind tells you what that path means.

### Node kind

Current workflow node kinds are:

- `onboarding`
- `planning`
- `scaffolding`
- `execution`
- `waiting_for_approval`

Step execution is only allowed in `scaffolding` and `execution` nodes.

### Template

A template defines:

- task identity
- required parameters
- onboarding requirements
- planning requirements
- scaffolding nodes
- execution nodes
- approval-wait nodes
- tool allowlists per node
- preconditions, postconditions, and invariants

Templates are authored as workflow YAML and compiled into normalized runtime nodes when loaded.

### Frozen execution contract

Planning produces a `PlanningArtifact`, and planning completion freezes execution from:

- the selected template
- the draft parameters
- the planning artifact

Execution then reads from that frozen contract rather than from mutable shell state or prompt drift.

## Runtime Flow

### 1. Register

`centian.task_register`:

- validates template parameters
- creates a task run
- places the run into `onboarding`

### 2. Complete onboarding

`centian.task_complete_onboarding`:

- stores reusable project discovery context
- moves the run into `planning`

Typical onboarding data includes:

- project summary
- artifact map
- common commands
- constraints
- open questions

### 3. Complete planning

`centian.task_complete_planning`:

- validates required planning outputs
- stores the planning artifact
- freezes the execution contract
- initializes step state
- advances to the configured next node

That next node may be:

- a `scaffolding.*` node
- an `execution.*` node
- a `waiting_for_approval.*` node

When planning or step completion enters an approval wait, Centian records an `approval_wait_entered` lifecycle event.

### 4. Start and complete steps

`centian.task_start_step`:

- validates the active workflow node
- runs preconditions
- captures invariant baselines

`centian.task_complete_step`:

- runs postconditions
- verifies invariants
- advances to the next workflow node
- marks the task completed if there is no next node

### 5. Restart and fail

`centian.task_restart`:

- resets the run to onboarding
- clears planning and step state
- keeps the same task run id

`centian.task_fail`:

- marks the task failed
- stores an explicit failure reason

## Governance

Taskverification currently enforces tool governance at the MCP tool boundary.

Each workflow node can declare `tools_allowed`, interpreted as raw glob patterns over tool names.

Current behavior:

- if a node omits `tools_allowed`, proxied downstream tools are denied by default
- `waiting_for_approval` blocks all proxied downstream tools regardless of allowlist
- matching works against both upstream-visible and canonical downstream tool names

This is intentionally tool-level governance, not semantic command analysis.

Examples:

- allow `shell__*`
- allow `filesystem__*`
- block everything in approval wait

## Observability, Persistence, and UI

Taskverification currently persists three concrete record types in SQLite:

- `task_events`
  Append-only lifecycle events such as registration, onboarding completion, planning completion, step start, step completion, restart, fail, and approval-wait entry.
- `action_events`
  Persisted MCP/proxy request and response history.
- `action_event_task_context`
  The bridge that links a proxied action request id back to the task run and invocation phase that produced it.

There is no persisted `task_runs` table today.
Instead, task run summaries are derived from persisted lifecycle and action records.

### What is persisted

Persisted today:

- lifecycle events
- proxied action events
- action-to-task correlation

Not durably restorable today:

- full mutable `RunState`
- step execution recovery from persistence
- resume-from-database task restoration

Available today as read-only query projections:

- aggregated task run summaries
- unified per-run task/action timelines

### Request logs

Centian still writes JSONL request logs for MCP activity.
Those logs are separate from the SQLite projections above.

### SQLite event store

When `proxy.capabilities.eventStorage.enabled` is on, Centian persists taskverification history to SQLite.

The default path is:

```text
~/.centian/logs/events.sqlite
```

The database currently supports:

- task run summary queries
- unified task/action timelines
- correlation of lifecycle events with downstream MCP activity

### Task run API

The persistence-backed API is read-only and currently exposes:

- `GET /api/task-runs`
  Returns aggregated run summaries including template id, current phase, status, and event counts.
- `GET /api/task-runs/{runID}/events`
  Returns one chronological unified event stream combining task lifecycle rows and correlated action rows.

### Embedded UI

When `proxy.capabilities.ui.enabled` is true and persistence is available, Centian serves an embedded SPA under `/ui`.

The current UI provides:

- a task run list view at `/ui/tasks`
- a task run detail view at `/ui/tasks/:runID`
- grouped timeline rendering by phase
- correlated task and MCP exchange views
- per-run status and event summary metadata
- a read-only inspector for the selected timeline item

The UI is currently an observer only.
It does not register tasks, advance workflow steps, or mutate run state.

## Demo

This repository includes a taskverification demo in [demo/taskverification](../demo/taskverification).

The demo covers:

- onboarding
- planning
- scaffolding and execution
- approval waits
- request logs
- persisted SQLite event timelines

## Current Boundaries and Gaps

Taskverification is usable, but it is still a v1 feature set with deliberate gaps.

### Runtime and lifecycle gaps

- Mutable task runs are in-memory only.
- Restart always resets to onboarding.
- Checkpoint hints exist in the schema, but checkpoint creation and restore are not implemented yet.
- Approval waits can block execution, but there is no dedicated approve/resume tool yet.

### Governance gaps

- Governance is tool-level, not semantic.
- Centian does not yet distinguish read vs write shell commands.
- Centian does not yet distinguish read vs write filesystem sub-operations inside a single tool.
- Deeper evidence, evaluation, and policy reasoning is not implemented yet.

### Persistence gaps

- SQLite is the only implemented storage backend.
- The schema is intended to stay portable to Postgres later, but Postgres support is not implemented yet.
- Event persistence is durable, but mutable task state persistence is not.

### Replay and visualization gaps

- Historical inspection exists through the API and embedded UI.
- Full replay is not implemented.
- The UI is read-only and does not yet support task control actions.

### Operational caveats

- Templates are loaded relative to the process working directory unless `templatesPath` overrides that.
- A misaligned working directory can make task templates invisible.
- The taskverification demo intentionally exposes unsafe shell access for the PoC; that setup is not production-safe.

## Recommended Usage

Taskverification is a good fit when you want:

- a structured workflow for agent coding or investigation tasks
- explicit planning before execution
- node-level tool gating
- persisted lifecycle plus tool-call observability
- a built-in read-only run explorer

It is not yet a full replacement for:

- durable workflow orchestration systems
- semantic policy engines
- replay systems
- production-grade approval workflows

## Related Files

- Demo: [demo/taskverification/README.md](../demo/taskverification/README.md)
- Templates: [task-templates](../task-templates)
- Runtime: [internal/taskverification](../internal/taskverification)
- Proxy tool surface: [proxy_taskverification_tools.go](../internal/proxy/proxy_taskverification_tools.go)
- Persistence projections: [store.go](../internal/persistence/store.go)
- Task run API: [handler.go](../internal/api/handler.go)
- Embedded UI handler: [handler.go](../internal/ui/handler.go)
