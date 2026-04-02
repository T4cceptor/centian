# Task Template Authoring

Taskverification templates define how Centian should guide and verify an agentic task.

Templates are YAML files. Centian validates them, compiles them into a workflow graph, and uses them to drive the `centian.task_*` tools.

## Where Templates Live

There are two template locations:

- `task-templates/integrated/`: built into the Centian binary
- `task-templates/`: runtime disk templates loaded from the current working directory unless `proxy.capabilities.taskVerification.templatesPath` overrides it

Load order:

1. embedded templates from `integrated/`
2. runtime disk templates from the configured templates directory

If a disk template uses the same `task.id` as an embedded template, the disk template wins.

The packaging note in [`task-templates/README.md`](../task-templates/README.md) is intentionally short. This document is the canonical authoring guide.

## Template Structure

Every template has the same top-level shape:

```yaml
version: "0.1"
task:
  id: "example_task"
  name: "Example Task"
  description: "A concise summary."
  instructions: |
    Optional long-form template guidance.

parameters:
  - name: "targetFile"
    description: "Project-relative file path."

workflow:
  onboarding:
    instructions: |
      Discover the relevant files and commands.
    tools_allowed:
      - "filesystem__*"
      - "shell__*"

  planning:
    instructions: |
      Freeze the implementation contract.
    tools_allowed:
      - "filesystem__*"
      - "shell__*"
    editable_fields:
      - "parameters.targetFile"
    required_inputs:
      - "targetFile"

  execution:
    - id: "make_change"
      name: "Make change"
      instructions: |
        Implement the change.
      tools_allowed:
        - "filesystem__*"
        - "shell__*"
      checks:
        - id: "tests_pass"
          command: "go test ./..."
          post_conditions:
            - type: "exit_code"
              value: 0
```

## Required Sections

Every template must include:

- `version`
- `task`
- `workflow`
- `workflow.onboarding`
- `workflow.planning`
- `workflow.execution`

`workflow.scaffolding` is optional.

## `task`

`task` is the human-facing identity block.

| Field | Required | Notes |
| --- | --- | --- |
| `id` | Yes | Stable template identifier used by `centian.task_register`. Must be unique across loaded templates. |
| `name` | Yes | Display name for agents and UIs. |
| `description` | Yes | Short summary of the template's purpose. |
| `instructions` | No | Long-form template-level guidance. |

## Parameters and Placeholders

Parameters let you defer task-specific values until planning.

Example:

```yaml
parameters:
  - name: "testCommand"
    description: "Command used to execute the targeted test."
```

You can reference parameters in strings inside execution and scaffolding nodes:

```yaml
command: "${testCommand} ${testTarget}"
path: "${targetFile}"
value: "${expectedError}"
```

Rules:

- parameter names must be unique
- placeholders use `${name}`
- placeholders are not allowed in `task`, `parameters`, `workflow.onboarding`, or `workflow.planning`
- every declared parameter must be used by at least one placeholder
- every placeholder must map to a declared parameter when `parameters` is present
- planning must supply a non-empty value for every required parameter

## Workflow Model

Centian compiles templates into workflow nodes with these kinds:

- `onboarding`
- `planning`
- `scaffolding`
- `execution`
- `waiting_for_approval`

Normal flow:

1. onboarding
2. planning
3. optional scaffolding nodes
4. execution nodes
5. optional approval pauses where authored

### Onboarding

Use onboarding to gather project context and prepare the planning artifact.

Supported fields:

- `instructions`
- `tools_allowed`
- `checkpoint.enabled`

### Planning

Use planning to freeze the execution contract.

Supported fields:

- `instructions`
- `tools_allowed`
- `checkpoint.enabled`
- `editable_fields`
- `required_inputs`
- `next`

Important constraints:

- `editable_fields` must use the form `parameters.<name>`
- `editable_fields` can only reference known parameters
- `required_inputs` must match the template parameter names exactly
- if `next` is omitted, Centian moves to the first executable node automatically

### Scaffolding

`workflow.scaffolding` is optional. Use it for additive setup work that should happen before the main execution sequence.

Common uses:

- create or update test fixtures
- generate starter files
- prepare environment state for the real implementation steps

### Execution

`workflow.execution` is required and must contain at least one executable node.

Each node supports:

- `id`
- `kind`
- `name`
- `description`
- `instructions`
- `tools_allowed`
- `checkpoint`
- `checks`
- `invariants`
- `next`
- `sub_steps`

`kind` defaults to the surrounding phase kind. For authored execution-like nodes, the only non-default override currently supported is:

- `waiting_for_approval`

## Checks

Checks are shell commands plus preconditions and postconditions.

Example:

```yaml
checks:
  - id: "target_test_fails"
    command: "${testCommand} ${testTarget}"
    post_conditions:
      - type: "exit_code"
        value: 1
      - type: "stdout_contains"
        value: "${expectedError}"
```

Each check requires:

- `id`
- `command`

Optional:

- `description`
- `pre_conditions`
- `post_conditions`

## Invariants

Invariants are commands whose output must remain stable between step start and step completion.

Example:

```yaml
invariants:
  - id: "selected_target_stable"
    command: "printf '%s' '${testTarget}'"
```

Use invariants for values that should not drift while the step is in progress.

## Condition Types

Current supported condition types are:

- `exit_code`
- `exit_code_in`
- `stdout_contains`
- `stdout_not_contains`
- `output_contains`
- `output_not_contains`
- `file_exists`
- `file_not_exists`
- `file_contains`
- `file_not_contains`

Notes:

- `stdout_*` checks only stdout
- `output_*` checks combined stdout and stderr
- file conditions resolve paths relative to the taskverification working directory
- file conditions reject path traversal outside the working directory

## Branching and `next`

If you do not specify `next`, Centian wires executable leaf nodes in authored order.

Use `next` when you need:

- a non-linear workflow
- an explicit jump into a later step
- a pause node such as `waiting_for_approval.review_plan`

Rules:

- `next` must point to an existing executable or approval node
- a node cannot point to itself
- workflow cycles are rejected
- a terminal `waiting_for_approval` node is rejected

## Nested `sub_steps`

`sub_steps` lets you group related executable work under one logical parent.

Constraints:

- a node with `sub_steps` cannot also define `checks`
- a node with `sub_steps` cannot also define `invariants`
- a node with `sub_steps` cannot also define `next`
- `waiting_for_approval` nodes cannot define `sub_steps`

## Planning and Onboarding Artifacts

Agents persist runtime artifacts through task tools.

### Onboarding artifact

`centian.task_complete_onboarding` stores:

- `taskSummary`
- `artifactMap`
- `commonCommands`
- `constraints`
- `openQuestions`

`taskSummary` is required.

### Planning artifact

`centian.task_complete_planning` stores:

- `planSummary`
- `selectedFiles`
- `parameters`
- `invariants`

`planSummary` is required.

Planning also enforces:

- all required parameter values are present and non-empty
- `selectedFiles` values are unique after trimming
- planning invariants are unique after trimming

## Working Directory

Template-defined shell commands run in Centian's taskverification working directory, which is the current working directory of the Centian process at startup.

That matters for:

- `checks[].command`
- `invariants[].command`
- file-based conditions
- relative `templatesPath` resolution

Centian logs the taskverification working directory on startup when the capability is enabled.

## Recommended Authoring Pattern

A good authoring sequence is:

1. start from one of the integrated templates
2. define the minimal parameter set
3. keep onboarding and planning instructions explicit
4. add the smallest number of executable steps that still make the workflow useful
5. use checks for observable facts, not for intent
6. use invariants only where drift would invalidate the step
7. test the template from a real task run before moving it into `integrated/`

## Built-In Templates

This repository currently ships these integrated templates:

- `minimal`
- `python_tdd_workflow`
- `simple_tdd`

Use them as examples of:

- parameter design
- planning contracts
- sequencing of scaffolding versus execution
- checks and invariants that are stable enough to automate

## Related Reads

- [Taskverification Runtime](TASKVERIFICATION.md)
- [Configuration Reference](configuration-reference.md)
- [MCP Proxy Best Practices](mcp-proxy-best-practices.md)
