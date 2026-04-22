# Taskverification

Centian taskverification adds a workflow-driven task runtime on top of a normal MCP proxy endpoint. It lets an agent register a task from a template, move through explicit workflow phases, and inspect lifecycle plus MCP action history through the same Centian deployment.

## What It Adds

Taskverification turns Centian into four things at once:

- an MCP proxy
- a task runtime
- a workflow policy layer
- an observability source for task-oriented agent runs

It exists to solve four practical problems:

1. Give the agent a structured lifecycle instead of leaving all process control to prompt text.
2. Freeze expectations before execution starts, so the agent reads from a contract instead of mutable ad hoc context.
3. Gate downstream MCP tool usage by workflow node, so different phases can have different permissions.
4. Persist enough lifecycle and action history to inspect task runs after the fact.

Before enabling taskverification in a shared or production-adjacent environment, read [Current Boundaries and Gaps](#current-boundaries-and-gaps) below.

## Quickstart

Taskverification tools are disabled by default. You must opt in explicitly.

### 1. Enable the runtime

Add `proxy.capabilities.taskVerification` to your Centian config. If you also want persisted timelines and the embedded UI, enable `eventStorage` and `ui`.

```json
{
  "proxy": {
    "host": "127.0.0.1",
    "port": "9666",
    "timeout": 30,
    "capabilities": {
      "taskVerification": {
        "enabled": true,
        "templatesPath": "/absolute/path/to/task-templates",
        "idleTimeoutSeconds": 900
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

- `taskVerification.enabled` enables the task verification runtime for the project.
- `taskVerification.templatesPath` overrides where Centian looks for runtime disk templates.
- `taskVerification.idleTimeoutSeconds` enables task idle timeout when greater than `0`.
- `eventStorage.enabled` defaults to `true`.
- `eventStorage.driver` currently only supports `sqlite`.
- `eventStorage.path` is optional. If omitted, Centian uses `~/.centian/logs/events.sqlite`.
- `ui.enabled` only exposes the embedded frontend. It does not enable taskverification tools by itself.
- Each gateway can further control task verification participation with `verificationRequirement`:
  - `off`: no `centian.task_*` tools on that gateway; downstream tools still work normally
  - `optional`: `centian.task_*` tools are exposed, but downstream tools can be used before registration
  - `required`: `centian.task_*` tools are exposed and downstream tools are blocked until registration

### 2. Understand template sources

Taskverification loads templates from two places:

- embedded templates compiled into the binary from `task-templates/integrated/`
- runtime disk templates from `task-templates/` by default, or from `taskVerification.templatesPath` if set

If a disk template and an embedded template share the same `task.id`, the disk template wins.

For the schema-focused authoring guide, see [TASK_TEMPLATE_AUTHORING.md](./TASK_TEMPLATE_AUTHORING.md).

### 3. Expose downstream tools through a normal gateway

Taskverification does not replace downstream MCP tools. It orchestrates and governs them.

A typical setup exposes project access through downstream tools such as:

- `filesystem`
- `shell`

The agent then talks to one Centian endpoint and receives both:

- normal proxied MCP tools
- `centian.task_*` tools

If you only want some gateways to participate in task verification, set `verificationRequirement` per gateway instead of disabling the project capability globally.

### 4. Start Centian

```bash
centian start
```

When taskverification is enabled, Centian also logs the working directory used for taskverification command execution.

### 5. Use the lifecycle

The normal lifecycle is:

1. `centian.task_list_templates`
2. `centian.task_register`
3. `centian.task_complete_onboarding`
4. `centian.task_complete_planning`
5. `centian.task_start_step`
6. `centian.task_complete_step`

Additional control tools are also available:

- `centian.task_resume`
- `centian.task_restart`
- `centian.task_fail`

Before a task is registered, only these two task tools are allowed:

- `centian.task_list_templates`
- `centian.task_register`

### 6. Inspect persisted runs

If event storage is enabled, Centian exposes a read-only task run API:

- `GET /api/task-runs`
- `GET /api/task-runs/{runID}/events`

If `ui.enabled` is also enabled and persistence is available, Centian serves the embedded UI under `/ui`.

The frontend routes are:

- `/ui/tasks`
- `/ui/tasks/:runID`

The UI is read-only. It is intended for inspection of persisted runs, not task control.

## Capability Overview

Taskverification currently spans three separate capability areas in Centian config:

- `proxy.capabilities.taskVerification`
  Enables agents to be controlled via task templates. Default: `false`.
- `proxy.capabilities.eventStorage`
  Persists task and action events to SQLite so run summaries and timelines can be queried later. Default: `true`.
- `proxy.capabilities.ui`
  Serves the embedded frontend under `/ui`. Default: `false`.

These capabilities are related, but they are not the same thing:

- the taskverification runtime and tools are controlled by `taskVerification`
- the historical task run API depends on `eventStorage`
- the embedded UI depends on both persistence and `ui.enabled`

If event storage is disabled, Centian does not register the task run API or the embedded task run UI.

## Tool Surface

The current taskverification MCP tools are:

- `centian.task_list_templates`
- `centian.task_register`
- `centian.task_complete_onboarding`
- `centian.task_complete_planning`
- `centian.task_start_step`
- `centian.task_complete_step`
- `centian.task_resume`
- `centian.task_restart`
- `centian.task_fail`

These tools are injected as static proxy-owned tools on the Centian endpoint when `taskVerification.enabled` is true.

Response-shape guidance:

- `centian.task_complete_planning` is the rich execution handoff and returns the frozen execution contract summary plus the next actionable step contract
- lifecycle tools such as register, onboarding completion, resume, restart, and fail return compact workflow state instead of the full task snapshot
- `centian.task_start_step` and `centian.task_complete_step` return step-local context on success and compact diagnostics on failure
- failed step actions are retryable in place unless the response explicitly says restart is required
- `recoveryActions` is the canonical structured recovery field for failed step actions; `nextAction` mirrors the first recovery action summary

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
- failure and timeout metadata

Current mutable run state is not durably restorable.

### Workflow path

Taskverification uses workflow paths instead of a coarse lifecycle enum.

Examples:

- `onboarding`
- `planning`
- `scaffolding.setup_test_file`
- `execution.verify_failing_baseline`
- `execution.implement_green`
- `execution.refactor_while_green`
- `waiting_for_approval.review_plan`

The path tells you where the run is. The node kind tells you what that path means.

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
- the resolved parameters
- the planning artifact

Execution then reads from that frozen contract rather than from mutable shell state or prompt drift.

### Working directory

Taskverification command execution uses Centian's current working directory at server startup. That working directory affects:

- check commands
- invariant commands
- file-based conditions
- default runtime template lookup at `task-templates/`

If `taskVerification.templatesPath` is relative, it is resolved from that same working directory.

## Runtime Flow

### 1. Register

`centian.task_register`:

- validates template selection
- creates a task run
- places the run into `onboarding`

### 2. Complete onboarding

`centian.task_complete_onboarding`:

- stores reusable task and environment context
- moves the run into `planning`

Typical onboarding data includes:

- task summary
- artifact map
- common commands
- constraints
- open questions

### 3. Complete planning

`centian.task_complete_planning`:

- validates required planning outputs
- stores the planning artifact
- resolves required template parameters
- freezes the execution contract
- initializes step state
- advances to the configured next node

That next node may be:

- a `scaffolding.*` node
- an `execution.*` node
- a `waiting_for_approval.*` node

### 4. Start and complete steps

`centian.task_start_step`:

- validates the active workflow node
- runs preconditions
- captures invariant baselines
- leaves the step retryable in place when verification fails before activation

`centian.task_complete_step`:

- runs postconditions
- verifies invariants
- advances to the next workflow node
- marks the task completed if there is no next node
- keeps the step retryable in place when verification fails after activation

### 5. Resume, restart, and fail

`centian.task_resume`:

- reactivates a timed-out run
- preserves workflow progress

`centian.task_restart`:

- resets the run to onboarding
- clears planning and step state
- keeps the same task run id

`centian.task_fail`:

- marks the task failed
- stores an explicit failure reason

When a run enters an approval-wait node, Centian records the lifecycle event type `approval_wait_entered`. That event is persisted like other lifecycle events and shows up in the task run timeline when event storage is enabled.

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
  Append-only lifecycle events such as registration, onboarding completion, planning completion, step start, step completion, timeout, restart, fail, and approval-wait entry.
- `action_events`
  Persisted MCP/proxy request and response history.
- `action_event_task_context`
  The bridge that links a proxied action request id back to the task run and invocation phase that produced it.

There is no persisted `task_runs` table today. Task run summaries are derived from persisted lifecycle and action records.

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

Centian still writes JSONL request logs for MCP activity. Those logs are separate from the SQLite projections above.

### SQLite event store

When `eventStorage.enabled` is on, Centian persists taskverification history to SQLite.

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

When `ui.enabled` is true and persistence is available, Centian serves an embedded SPA under `/ui`.

The current UI provides:

- a task run list view at `/ui/tasks`
- a task run detail view at `/ui/tasks/:runID`
- grouped timeline rendering by phase
- correlated task and MCP exchange views
- per-run status and event summary metadata
- a read-only inspector for the selected timeline item

The UI is currently an observer only. It does not register tasks, advance workflow steps, or mutate run state.

Build note:

- Centian embeds the built frontend from `internal/ui/dist` when those assets are present at build time
- if the dist bundle is absent, the UI handler still serves a minimal embedded fallback page instead of failing route registration entirely

## Current Boundaries and Gaps

Taskverification is usable, but it still has deliberate limits.

### Runtime and lifecycle gaps

- mutable task runs are in-memory only
- restart always resets to onboarding
- checkpoint hints exist in the schema, but checkpoint creation and restore are not implemented yet
- approval waits can block execution, but there is no dedicated approval tool yet

### Governance gaps

- governance is tool-level, not semantic
- Centian does not distinguish read vs write shell commands
- Centian does not distinguish read vs write filesystem sub-operations inside a single tool
- deeper evidence, evaluation, and policy reasoning is not implemented

### Persistence gaps

- SQLite is the only implemented storage backend
- event persistence is durable, but mutable task state persistence is not

### Replay and visualization gaps

- historical inspection exists through the API and embedded UI
- full replay is not implemented
- the UI is read-only and does not support task control actions

### Operational caveats

- templates are loaded relative to the process working directory unless `templatesPath` overrides that
- a misaligned working directory can make runtime disk templates invisible
- a task template can rely on shell/file behavior that is valid only for one working directory layout

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

- Templates: [task-templates](../task-templates)
- Template Authoring: [TASK_TEMPLATE_AUTHORING.md](./TASK_TEMPLATE_AUTHORING.md)
- Benchmark CLI and fixtures: [tests/integrationtests/taskverification/benchmarks](../tests/integrationtests/taskverification/benchmarks)
- Benchmarking guide: [BENCHMARKING.md](./BENCHMARKING.md)
- Runtime: [internal/taskverification](../internal/taskverification)
- Proxy tool surface: [proxy_taskverification_tools.go](../internal/proxy/proxy_taskverification_tools.go)
- Persistence projections: [store.go](../internal/persistence/store.go)
- Task run API: [handler.go](../internal/api/handler.go)
- Embedded UI handler: [handler.go](../internal/ui/handler.go)
