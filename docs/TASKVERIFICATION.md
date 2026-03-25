# Taskverification

Centian taskverification adds a workflow-driven task runtime on top of a normal MCP proxy endpoint.
It lets an agent register a task from a template, move through onboarding and planning, execute stepwise verification, and observe lifecycle and action data through the same Centian endpoint.

## Purpose

Taskverification exists to make agent work more explicit, inspectable, and enforceable than a plain "call tools until it works" loop.

It is intended to solve four practical problems:

1. Give the agent a structured lifecycle instead of leaving all process control to prompt text.
2. Freeze execution expectations before implementation starts, so execution reads from a contract instead of ad hoc mutable context.
3. Gate downstream MCP tool usage by workflow node, so onboarding, planning, execution, and approval waits can have different permissions.
4. Produce lifecycle and action history that can later support replay, auditing, and visualization.

At a high level, taskverification turns Centian into:

- an MCP proxy
- a task runtime
- a workflow policy layer
- an observability source for task-oriented agent runs

## Quickstart

Taskverification tools are disabled by default. You must opt in explicitly.

### 1. Enable the feature

Add `proxy.featureFlags.taskVerification` to your Centian config:

```json
{
  "proxy": {
    "host": "127.0.0.1",
    "port": "8080",
    "timeout": 30,
    "taskTemplatesPath": "/absolute/path/to/task-templates",
    "featureFlags": {
      "taskVerification": true
    },
    "eventStorage": {
      "enabled": true,
      "driver": "sqlite"
    }
  }
}
```

Notes:

- `taskVerification` controls whether `centian.task_*` tools are exposed.
- `taskTemplatesPath` overrides where Centian looks for task templates.
- `eventStorage` is separate from `featureFlags` because it configures persistence, not just a capability toggle.
- SQLite is the only implemented runtime backend today.

### 2. Ensure templates exist

By default, taskverification loads templates from:

```text
<current-working-directory>/task-templates
```

If `proxy.taskTemplatesPath` is set, Centian uses that directory instead.

That means you can either:

- start Centian in a directory that contains `task-templates/`
- or point `taskTemplatesPath` at a different template directory explicitly

This repository already includes example templates under [task-templates](/Users/brb/_devspace/centian-cli/task-templates).

### 3. Expose downstream tools through a normal gateway

Taskverification does not replace downstream MCP tools. It orchestrates and governs them.

A typical setup exposes project access through a gateway such as:

- `filesystem`
- `shell`

The agent then talks to one Centian endpoint and gets both:

- normal proxied MCP tools
- `centian.task_*` tools

### 4. Start Centian

```bash
centian start
```

### 5. Use the task lifecycle

The intended lifecycle is:

1. `centian.task_list_templates`
2. `centian.task_register`
3. `centian.task_complete_onboarding`
4. `centian.task_complete_planning`
5. `centian.task_start_step`
6. `centian.task_complete_step`

Approval-wait templates may pause after planning or after step completion. In those nodes, meaningful downstream tool calls are blocked until the workflow leaves the wait node.

## Core Concepts

### Task run

A task run is one active agent task session created from one template.

Current state is held in memory in a `RunState`. This includes:

- template id
- workflow phase/path
- onboarding artifact
- planning artifact
- frozen execution contract
- per-step runtime state

Task runs are not durably persisted yet.

### Workflow path

Taskverification uses workflow paths instead of a coarse lifecycle enum.

Examples:

- `onboarding`
- `planning`
- `execution.step_one`
- `waiting_for_approval.review_plan`

The path tells you where the run is.
The node kind tells you what that path means.

### Node kind

Current workflow node kinds are:

- `onboarding`
- `planning`
- `execution`
- `waiting_for_approval`

### Template

A template defines:

- task identity
- required parameters
- onboarding node
- planning node
- execution and approval-wait nodes
- tool allowlists per node
- planning output requirements
- verification checks and invariants

Templates are authored as hierarchical workflow YAML and compiled into normalized runtime nodes when loaded.

### Frozen execution contract

Planning produces a `PlanningArtifact`, and planning completion freezes execution from:

- the selected template
- the draft parameters
- the planning artifact

Execution then reads from that frozen contract rather than from mutable shell state.

## Runtime Flow

### 1. Register

`centian.task_register`:

- validates template parameters
- creates a task run
- places the run directly into `onboarding`

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
- initializes execution step state
- advances to the configured next node

That next node may be:

- the first execution node
- a `waiting_for_approval.*` node

### 4. Start and complete steps

`centian.task_start_step`:

- validates the current active execution node
- runs preconditions
- captures invariant baselines

`centian.task_complete_step`:

- runs postconditions
- verifies invariants
- advances to the next workflow node
- marks the task completed if there is no next node

### 5. Restart and fail

`centian.task_restart`:

- currently resets the run to onboarding
- clears planning and execution state
- keeps the same task run identity

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

## Observability and Persistence

Taskverification uses a split model:

- `task_runs`
  - current in-memory state only
- `task_events`
  - lifecycle history
- `action_events`
  - MCP/proxy action history
- `action_event_task_context`
  - bridge between action history and active task context

### Request logs

Centian still writes JSONL request logs for MCP activity.

### SQLite event store

When `proxy.eventStorage.enabled` is on, Centian also persists:

- `task_events`
- `action_events`
- `action_event_task_context`

Default storage today is SQLite.

The event DB is useful for:

- querying task timelines
- correlating lifecycle events with tool calls
- preparing later visualization work

### Current persistence boundary

Persisted today:

- lifecycle events
- action events
- action-to-task correlation

Not persisted today:

- full mutable task run state
- execution state recovery from durable storage

## Configuration Notes

### Feature flags

Taskverification is behind:

```json
"proxy": {
  "featureFlags": {
    "taskVerification": true
  }
}
```

Other proxy-owned toggles live in the same block, for example:

```json
"featureFlags": {
  "enableTestTools": false,
  "taskVerification": true
}
```

### Event storage

Event storage remains a dedicated config block:

```json
"eventStorage": {
  "enabled": true,
  "driver": "sqlite",
  "path": "/path/to/events.sqlite"
}
```

### Demo

This repository includes a taskverification demo in [demo/taskverification](/Users/brb/_devspace/centian-cli/demo/taskverification).

The demo covers:

- onboarding
- planning
- execution
- approval waits
- request logs
- persisted SQLite event timelines

## Known Gaps and Issues

Taskverification is usable, but it is still a v1 feature set with deliberate gaps.

### Runtime and lifecycle gaps

- Task runs are in-memory only.
- Restart always resets to onboarding.
- Checkpoint hints exist in the schema, but runtime checkpoint creation and restore are not implemented yet.
- Approval waits can block execution, but there is no dedicated approve/resume mechanism yet.

### Governance gaps

- Governance is tool-level, not semantic.
- Centian does not yet distinguish read vs write shell commands.
- Centian does not yet distinguish read vs write filesystem sub-operations inside a single tool.
- Deeper evidence/evaluation/policy reasoning is not implemented yet.

### Persistence gaps

- SQLite is the only implemented runtime backend.
- The schema is designed to stay portable to Postgres later, but Postgres support is not implemented yet.
- Event persistence is durable, but mutable task state persistence is not.

### Replay and visualization gaps

- Event data is sufficient groundwork for future replay/visualization work, but full replay is not implemented.
- There is no built-in UI or CLI explorer for task timelines yet.

### Operational caveats

- Templates are loaded relative to the process working directory.
- A misaligned working directory can make task templates invisible.
- The taskverification demo intentionally exposes unsafe shell access for the PoC; that setup is not production-safe.

## Recommended Usage

Taskverification is a good fit when you want:

- a structured workflow for agent coding or investigation tasks
- explicit planning before execution
- node-level tool gating
- lifecycle plus tool-call observability

It is not yet a full replacement for:

- durable workflow orchestration systems
- semantic policy engines
- replay systems
- production-grade approval workflows

## Related Files

- Demo: [demo/taskverification/README.md](/Users/brb/_devspace/centian-cli/demo/taskverification/README.md)
- Templates: [task-templates](/Users/brb/_devspace/centian-cli/task-templates)
- Runtime: [internal/taskverification](/Users/brb/_devspace/centian-cli/internal/taskverification)
- Proxy tool surface: [proxy_taskverification_tools.go](/Users/brb/_devspace/centian-cli/internal/proxy/proxy_taskverification_tools.go)
