# How To Create a Taskverification Template

**Version:** 1.0
**Last Updated:** 2026-03-29

This guide explains how to create a new taskverification template for Centian.
The template itself is a YAML file. This document is the Markdown authoring guide for that YAML file.

Use this guide together with [TASKVERIFICATION.md](./TASKVERIFICATION.md).

## What a Template Does

A taskverification template defines:

- the task identity shown to the agent
- the parameters the agent can provide
- the onboarding and planning phases
- the executable workflow steps
- the checks and invariants Centian uses to verify each step
- the tool allowlist for each workflow node

At runtime, Centian:

1. Loads the YAML file from the configured templates directory.
2. Validates and compiles the workflow.
3. Registers a task run from that template.
4. Freezes a runnable version of the template when planning completes.
5. Uses the compiled checks and invariants to gate step execution.

## Where to Put the File

Create the template as a `.yaml` or `.yml` file in the task template directory:

- default: `<current-working-directory>/task-templates`
- override: `proxy.capabilities.taskVerification.templatesPath`

Template IDs must be unique across every YAML file in that directory.

## Authoring Flow

Create templates in this order:

1. Define top-level metadata: `version`, `task`, `workflow`.
2. Add `parameters` for every placeholder you want to substitute.
3. Define `workflow.onboarding` and `workflow.planning`.
4. Add optional `workflow.scaffolding` steps.
5. Add required `workflow.execution` steps.
6. Add `checks` for every executable step.
7. Add `invariants` only where output must remain stable between start and complete.
8. Add explicit `next` edges only when the default linear order is not what you want.
9. Load the template through Centian and fix any validation errors before using it.

## Minimal Working Template

```yaml
version: "0.1"
task:
  id: "go_bugfix"
  name: "Go Bugfix Task"
  description: "Drive a focused bugfix with an explicit failing baseline and implementation step."
  instructions: |
    Register the task first, finish onboarding and planning, then follow the workflow steps in order.

parameters:
  - name: "testCommand"
    description: "Command prefix used to run the targeted test."
  - name: "testTarget"
    description: "Concrete test target."
  - name: "expectedError"
    description: "Stable failing output expected before the fix."
  - name: "sourceFile"
    description: "Implementation file expected to change."

workflow:
  onboarding:
    instructions: |
      Identify the relevant package, test target, and implementation file.
    tools_allowed:
      - "shell__*"
      - "filesystem__*"

  planning:
    tools_allowed:
      - "shell__*"
      - "filesystem__*"
    editable_fields:
      - "parameters.testCommand"
      - "parameters.testTarget"
      - "parameters.expectedError"
      - "parameters.sourceFile"
    required_inputs:
      - "selectedFiles"
      - "testTarget"
      - "expectedError"
      - "implementationTarget"

  execution:
    - id: "establish_failing_baseline"
      name: "Establish failing baseline"
      tools_allowed:
        - "shell__*"
        - "filesystem__*"
      checks:
        - id: "selected_test_fails"
          command: "${testCommand} ${testTarget}"
          post_conditions:
            - type: exit_code
              value: 1
            - type: stdout_contains
              value: "${expectedError}"

    - id: "implement_fix"
      name: "Implement fix"
      tools_allowed:
        - "shell__*"
        - "filesystem__*"
      checks:
        - id: "selected_test_passes"
          command: "${testCommand} ${testTarget}"
          pre_conditions:
            - type: exit_code
              value: 1
          post_conditions:
            - type: exit_code
              value: 0
      invariants:
        - id: "test_target_stable"
          command: "printf '%s' '${testTarget}'"
```

## Template Schema

### Top-Level Fields

Every template must include:

- `version`
- `task.id`
- `task.name`
- `task.description`
- `workflow`
- `workflow.onboarding`
- `workflow.planning`
- `workflow.execution`

`workflow.execution` must contain at least one executable node.

### `task`

`task` is the human-facing identity:

```yaml
task:
  id: "go_bugfix"
  name: "Go Bugfix Task"
  description: "Drive a focused bugfix."
  instructions: |
    Optional long-form instructions shown to the agent.
```

Rules:

- `task.id` is required and must be unique across all loaded templates.
- `task.name` is required.
- `task.description` is required.
- `task.instructions` is optional.

## Parameters and Placeholders

Use `parameters` when you want a value substituted into the YAML before execution:

```yaml
parameters:
  - name: "testCommand"
    description: "Command prefix used to run the selected test."
```

Reference parameters with `${name}` inside any string field:

```yaml
command: "${testCommand} ${testTarget}"
value: "${expectedError}"
path: "${relativePath}"
```

Rules:

- Placeholder names must match `${[a-zA-Z0-9_]+}`.
- Parameter names must be unique.
- If `parameters` is present, every defined parameter must be used by at least one placeholder.
- If a placeholder is used, it must be declared in `parameters`.
- Unknown parameters passed during task registration are rejected.
- Missing parameter values are tolerated at registration time, but planning completion fails if placeholder substitution still leaves unresolved `${...}` values.

Practical guidance:

- Keep parameters focused on values that vary per task run.
- Do not create parameters for static strings that belong in the template itself.
- Prefer relative file paths in parameters when they are later used by `file_*` conditions.

## Workflow Structure

The workflow has four authoring areas:

- `onboarding`
- `planning`
- `scaffolding` optional
- `execution` required

### Onboarding

Use onboarding for task and environment discovery context.

```yaml
workflow:
  onboarding:
    instructions: |
      Discover the target package, test command, and relevant files.
    tools_allowed:
      - "shell__*"
      - "filesystem__*"
    checkpoint:
      enabled: true
```

Supported fields:

- `instructions`
- `tools_allowed`
- `checkpoint.enabled`

### Planning

Use planning to freeze the execution contract.

```yaml
planning:
  instructions: |
    Confirm the concrete target files and expected failure mode.
  tools_allowed:
    - "shell__*"
    - "filesystem__*"
  editable_fields:
    - "parameters.testCommand"
    - "parameters.testTarget"
  required_inputs:
    - "selectedFiles"
    - "testTarget"
  next: "execution.establish_failing_baseline"
```

Supported fields:

- `instructions`
- `tools_allowed`
- `checkpoint.enabled`
- `editable_fields`
- `required_inputs`
- `next`

Rules for `editable_fields`:

- Each value must be unique.
- Only `parameters.<name>` is supported.
- `<name>` must refer to a declared or placeholder-derived parameter.

Rules for `required_inputs`:

- Each value must be unique.
- Only these outputs are supported:
  - `selectedFiles`
  - `testTarget`
  - `lintCommand`
  - `expectedError`
  - `implementationTarget`
  - `invariants`

These outputs map to fields on the planning artifact sent to `centian.task_complete_planning`:

- `selectedFiles`: non-empty `[]string`, each entry trimmed and unique
- `testTarget`: non-empty string
- `lintCommand`: non-empty string
- `expectedError`: non-empty string
- `implementationTarget`: non-empty string
- `invariants`: non-empty `[]string`, each entry trimmed and unique

If `planning.next` is omitted, Centian automatically advances to the first executable step after planning.

### Scaffolding

Use scaffolding for additive setup before the main execution path. This is optional.

```yaml
scaffolding:
  - id: "create_test_file"
    name: "Create test file"
    checks:
      - id: "test_file_created"
        command: "printf 'created'"
        pre_conditions:
          - type: file_not_exists
            path: "tests/new_test.go"
        post_conditions:
          - type: file_exists
            path: "tests/new_test.go"
```

Scaffolding nodes are executable steps, just like execution nodes.

### Execution

Use execution for the steps that define task success.

```yaml
execution:
  - id: "establish_failing_baseline"
    checks:
      - id: "selected_test_fails"
        command: "${testCommand} ${testTarget}"
        post_conditions:
          - type: exit_code
            value: 1

  - id: "implement_fix"
    checks:
      - id: "selected_test_passes"
        command: "${testCommand} ${testTarget}"
        post_conditions:
          - type: exit_code
            value: 0
```

Executable leaf nodes may omit `checks` entirely.
This is useful for free-form templates that are intended to track and monitor a task run without enforcing verification at each step.

## Execution Nodes

Scaffolding and execution lists use the same node schema:

```yaml
- id: "implement_fix"
  kind: "execution"
  name: "Implement fix"
  description: "Make the selected test pass."
  instructions: |
    Start the step before editing code.
  tools_allowed:
    - "shell__*"
    - "filesystem__*"
  checkpoint:
    enabled: true
  checks:
    - id: "selected_test_passes"
      command: "${testCommand} ${testTarget}"
  invariants:
    - id: "test_file_stable"
      command: "cat ${testFile}"
  next: "waiting_for_approval.review_plan"
```

Supported fields:

- `id`
- `kind`
- `name`
- `description`
- `instructions`
- `tools_allowed`
- `checkpoint.enabled`
- `checks`
- `invariants`
- `next`
- `sub_steps`

Rules:

- `id` is required.
- Node IDs must be unique across the whole workflow, not just inside one list.
- `kind` defaults to the containing list kind:
  - nodes under `scaffolding` default to `scaffolding`
  - nodes under `execution` default to `execution`
- The only non-default `kind` currently allowed is `waiting_for_approval`.
- `checks` are optional. If provided, they must be valid.
- `invariants` are optional. If provided, they must be valid.

## Checks

Checks are the actual verification units run by Centian.

```yaml
checks:
  - id: "selected_test_fails"
    command: "${testCommand} ${testTarget}"
    pre_conditions:
      - type: exit_code_in
        values: [0, 1]
    post_conditions:
      - type: exit_code
        value: 1
      - type: stdout_contains
        value: "${expectedError}"
```

Rules:

- Each check needs `id`.
- Check IDs must be unique within the step.
- Each check needs `command`.
- `pre_conditions` are evaluated when `centian.task_start_step` runs.
- `post_conditions` are evaluated when `centian.task_complete_step` runs.

### Supported Condition Types

`exit_code`

- required field: `value`
- `value` must be numeric

`exit_code_in`

- required field: `values`
- `values` must be a non-empty numeric list

`stdout_contains`

- required field: `value`
- `value` must be a non-empty string

`stdout_not_contains`

- required field: `value`
- `value` must be a non-empty string

`file_exists`

- required field: `path`
- `path` must be relative to the service working directory

`file_not_exists`

- required field: `path`
- `path` must be relative to the service working directory

`file_contains`

- required fields: `path`, `value`
- `path` must be relative to the service working directory
- `value` must be a non-empty string

`file_not_contains`

- required fields: `path`, `value`
- `path` must be relative to the service working directory
- `value` must be a non-empty string

Path safety rules for all `file_*` conditions:

- absolute paths are rejected
- paths that escape the working directory through `..` are rejected

## Invariants

Invariants capture stdout at step start and require the same stdout at step completion.

```yaml
invariants:
  - id: "test_file_stable"
    command: "cat ${testFile}"
```

Rules:

- Each invariant needs `id`.
- Invariant IDs must be unique within the step.
- Each invariant needs `command`.
- Invariant commands must exit with code `0` during capture and verification.
- Verification compares stdout exactly.

Use invariants when a file, selector, or other derived output must remain unchanged for the duration of a step.

## Nested Steps with `sub_steps`

You can group steps hierarchically with `sub_steps`:

```yaml
execution:
  - id: "implement"
    sub_steps:
      - id: "update_tests"
        checks:
          - id: "tests_changed"
            command: "printf 'ok'"
      - id: "update_code"
        checks:
          - id: "code_changed"
            command: "printf 'ok'"
```

Behavior:

- Centian compiles only leaf nodes into executable steps.
- Leaf workflow paths are built from IDs:
  - `execution.implement.update_tests`
  - `execution.implement.update_code`
- Step order follows leaf discovery order.

Rules for nodes with `sub_steps`:

- they cannot define `checks`
- they cannot define `invariants`
- they cannot define `next`
- they cannot use `kind: waiting_for_approval`

## Approval Wait Nodes

You can insert manual approval pauses by using `kind: waiting_for_approval` inside `execution` or `scaffolding`.

```yaml
execution:
  - id: "review_plan"
    kind: "waiting_for_approval"
    instructions: |
      Wait for approval before implementation starts.
    next: "execution.implement_fix"

  - id: "implement_fix"
    checks:
      - id: "selected_test_passes"
        command: "${testCommand} ${testTarget}"
```

Rules:

- waiting nodes cannot define `checks`
- waiting nodes cannot define `invariants`
- a waiting node cannot be terminal; it must define or inherit a valid next step
- active workflow paths for these nodes are under `waiting_for_approval.<id>`

Step execution is not allowed while the run is parked in a waiting-for-approval node.

## Transition Rules

Centian wires transitions like this:

- `onboarding` always moves to `planning`
- `planning` moves to `planning.next` if set, otherwise to the first executable leaf node
- each executable leaf node moves to its explicit `next` if set
- otherwise each executable leaf node moves to the next compiled leaf node in order
- the final executable leaf node completes the task if it has no next node

Validation rules:

- `next` must target an existing compiled node
- `next` may target only `scaffolding`, `execution`, or `waiting_for_approval` nodes
- a node cannot target itself
- the workflow cannot contain cycles
- every compiled node must be reachable from onboarding

## Command Execution Model

Template commands are executed as shell commands:

- runtime shell: `/bin/sh -c`
- working directory: the taskverification service working directory
- default timeout per command: `30s`

Important behavior:

- a non-zero process exit does not automatically fail the check
- Centian records the exit code and lets conditions decide whether that result passes
- command execution failures are reserved for OS-level failures and timeouts

Design commands and conditions together. If failure is expected, declare it with conditions instead of assuming the runtime will reject the command automatically.

## Common Validation Failures

These are the errors template authors hit most often:

- missing required fields such as `task.id` or `workflow.execution`
- duplicate template IDs across files
- duplicate parameter names
- parameter declared but never referenced by a placeholder
- placeholder used but missing from `parameters`
- unsupported `editable_fields` entry
- unsupported `required_inputs` value
- executable step with no checks
- duplicate check or invariant IDs within a step
- unsupported condition type
- absolute or escaping paths in `file_*` conditions
- `waiting_for_approval` node with no valid next target
- explicit `next` pointing at an unknown node
- workflow cycles or unreachable nodes

## Authoring Recommendations

- Keep step scope narrow. One verification intent per step is easier to debug.
- Prefer `post_conditions` for outcome verification and `pre_conditions` for baseline verification.
- Use `scaffolding` only for additive setup work.
- Use `invariants` sparingly and only when the exact stdout comparison is meaningful.
- Keep file-based conditions relative to the project working directory.
- Use stable substrings in `stdout_contains` checks instead of brittle full-output matches.
- If planning is expected to refine task inputs, list those specific parameter fields in `editable_fields`.
- If a workflow needs a pause for human review, model it explicitly with `waiting_for_approval`.

## A Larger Example

```yaml
version: "0.1"
task:
  id: "go_tdd_with_review"
  name: "Go TDD With Review"
  description: "Create a failing Go test, implement the fix, then pause for review."
  instructions: |
    Use the task runtime as the authoritative workflow.

parameters:
  - name: "testCommand"
    description: "Go test command prefix, for example `go test ./... -run`."
  - name: "testTarget"
    description: "Concrete Go test selector."
  - name: "testFile"
    description: "Relative path to the Go test file."
  - name: "expectedError"
    description: "Stable failing output before the fix."
  - name: "implementationTarget"
    description: "Relative path to the implementation file."

workflow:
  onboarding:
    instructions: |
      Identify the package under test, test file, and implementation target.
    tools_allowed:
      - "shell__*"
      - "filesystem__*"

  planning:
    tools_allowed:
      - "shell__*"
      - "filesystem__*"
    editable_fields:
      - "parameters.testCommand"
      - "parameters.testTarget"
      - "parameters.testFile"
      - "parameters.expectedError"
      - "parameters.implementationTarget"
    required_inputs:
      - "selectedFiles"
      - "testTarget"
      - "expectedError"
      - "implementationTarget"

  scaffolding:
    - id: "create_test_file"
      name: "Create test file"
      checks:
        - id: "test_file_created"
          command: "printf 'test-file-created'"
          pre_conditions:
            - type: file_not_exists
              path: "${testFile}"
          post_conditions:
            - type: file_exists
              path: "${testFile}"

  execution:
    - id: "establish_failing_baseline"
      name: "Establish failing baseline"
      checks:
        - id: "target_fails"
          command: "${testCommand} ${testTarget}"
          post_conditions:
            - type: exit_code
              value: 1
            - type: stdout_contains
              value: "${expectedError}"

    - id: "implement_fix"
      name: "Implement fix"
      next: "waiting_for_approval.review_changes"
      checks:
        - id: "target_passes"
          command: "${testCommand} ${testTarget}"
          pre_conditions:
            - type: exit_code
              value: 1
          post_conditions:
            - type: exit_code
              value: 0
      invariants:
        - id: "test_file_stable"
          command: "cat ${testFile}"

    - id: "review_changes"
      kind: "waiting_for_approval"
      instructions: |
        Wait for review approval before any follow-up work.
      next: "execution.final_verification"

    - id: "final_verification"
      name: "Final verification"
      checks:
        - id: "target_still_passes"
          command: "${testCommand} ${testTarget}"
          post_conditions:
            - type: exit_code
              value: 0
        - id: "implementation_file_exists"
          command: "printf 'check-file'"
          post_conditions:
            - type: file_exists
              path: "${implementationTarget}"
```

## Validation Checklist

Before using a new template, confirm:

- the file is in the configured template directory
- the file extension is `.yaml` or `.yml`
- `task.id` is unique
- every placeholder has a matching parameter definition
- every declared parameter is actually used
- planning outputs match what `centian.task_complete_planning` will provide
- every executable leaf node has at least one check
- every `file_*` condition path is relative
- every `next` target exists and is reachable
- no `waiting_for_approval` node is terminal

If the template loads successfully through `centian.task_list_templates`, the structural validation pass has succeeded.
