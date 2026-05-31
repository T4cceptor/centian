# How To Create a Taskverification Template

**Version:** 1.0
**Last Updated:** 2026-04-02

This guide explains how to create a taskverification template for Centian. The template itself is a YAML file. This document is the Markdown authoring guide for that YAML file.

Use this guide together with [TASKVERIFICATION.md](./TASKVERIFICATION.md).

## What a Template Does

A taskverification template defines:

- the task identity shown to the agent
- the parameters the agent must determine during onboarding and planning
- the onboarding and planning phases
- the executable workflow steps
- the checks and invariants Centian uses to verify each step
- the tool allowlist for each workflow node

At runtime, Centian:

1. Loads embedded templates from `task-templates/integrated/`.
2. Loads runtime disk templates from the configured template directory.
3. Validates and compiles the workflow.
4. Registers a task run from that template.
5. Freezes a runnable version of the template when planning completes.
6. Uses the compiled checks and invariants to gate step execution.

## Where To Put the File

There are two template locations:

- `task-templates/integrated/`
  Templates here are embedded into the Centian binary.
- `task-templates/`
  Templates here are loaded at runtime from disk.

By default, runtime disk templates are loaded from:

```text
<current-working-directory>/task-templates
```

You can override the runtime disk template directory with:

- `proxy.capabilities.taskVerification.templatesPath`

Load order:

1. embedded templates from `integrated/`
2. runtime disk templates from the configured templates directory

If a runtime disk template uses the same `task.id` as an embedded template, the disk template wins.

Template IDs must be unique across the final loaded template set.

## Authoring Flow

Create templates in this order:

1. Define top-level metadata: `version`, `task`, `workflow`.
2. Add `parameters` for every placeholder you want to substitute.
3. Define `workflow.onboarding` and `workflow.planning`.
4. Add optional `workflow.scaffolding` nodes.
5. Add required `workflow.execution` nodes.
6. Add `checks` for any executable node that should be verified.
7. Add `invariants` only where output must remain stable between start and complete.
8. Add explicit `next` edges only when the default linear order is not what you want.
9. Load the template through Centian and fix any validation errors before using it in a real workflow.

## Minimal Working Template

This example matches the current validator and runtime:

```yaml
version: "0.1"
task:
  id: "go_bugfix"
  name: "Go Bugfix Task"
  description: "Drive a focused bugfix with explicit red, green, and refactor steps."
  instructions: |
    Register the task first, finish onboarding and planning, then follow the workflow steps in order.
    Use this shape when the red baseline already exists before execution begins.

parameters:
  - name: "testCommand"
    description: "Full runnable command used to execute the selected baseline."
  - name: "testTarget"
    description: "Human-facing target metadata for the selected baseline."
  - name: "testFile"
    description: "Concrete baseline test file kept invariant during execution."
  - name: "expectedError"
    description: "Stable failing output expected before the fix, including compile failures."

workflow:
  onboarding:
    instructions: |
      Identify the relevant package, the full runnable test command, the human-facing target metadata, the locked baseline test file, and the expected baseline failure.
    tools_allowed:
      - "shell__*"
      - "filesystem__*"

  planning:
    instructions: |
      Freeze the full runnable test command, the human-facing target metadata, the locked baseline test file, and the expected failure output.
    tools_allowed:
      - "shell__*"
      - "filesystem__*"
    editable_fields:
      - "parameters.testCommand"
      - "parameters.testTarget"
      - "parameters.testFile"
      - "parameters.expectedError"
    required_inputs:
      - "expectedError"
      - "testCommand"
      - "testFile"
      - "testTarget"

  execution:
    - id: "verify_failing_baseline"
      name: "Verify failing baseline"
      tools_allowed:
        - "shell__*"
        - "filesystem__*"
      instructions: |
        Verify the selected baseline already exists and is red for the intended reason.
        Do not edit the selected baseline test file in this step.
      checks:
        - id: "selected_test_fails"
          command: "${testCommand}"
          pre_conditions:
            - type: exit_code_in
              values: [1, 2]
            - type: output_contains
              value: "${expectedError}"
          post_conditions:
            - type: exit_code_in
              values: [1, 2]
            - type: output_contains
              value: "${expectedError}"
      invariants:
        - id: "baseline_test_file_stable"
          command: "cat ${testFile}"

    - id: "implement_green"
      name: "Implement green"
      tools_allowed:
        - "shell__*"
        - "filesystem__*"
      checks:
        - id: "selected_test_passes"
          command: "${testCommand}"
          pre_conditions:
            - type: exit_code_in
              values: [1, 2]
            - type: output_contains
              value: "${expectedError}"
          post_conditions:
            - type: exit_code
              value: 0
      invariants:
        - id: "baseline_test_file_stable"
          command: "cat ${testFile}"

    - id: "refactor_while_green"
      name: "Refactor while green"
      tools_allowed:
        - "shell__*"
        - "filesystem__*"
      checks:
        - id: "selected_test_stays_green"
          command: "${testCommand}"
          pre_conditions:
            - type: exit_code
              value: 0
          post_conditions:
            - type: exit_code
              value: 0
      invariants:
        - id: "baseline_test_file_stable"
          command: "cat ${testFile}"
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

`workflow.execution` must contain at least one executable leaf node.

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

Use `parameters` when you want a value substituted into execution or scaffolding strings:

```yaml
parameters:
  - name: "testCommand"
    description: "Full runnable command used to run the selected baseline."
```

Reference parameters with `${name}` inside string fields:

```yaml
command: "${testCommand}"
value: "${expectedError}"
path: "${relativePath}"
```

Rules:

- placeholder names must match `${[a-zA-Z0-9_]+}`
- parameter names must be unique
- if `parameters` is present, every defined parameter must be used by at least one placeholder
- if a placeholder is used, it must be declared in `parameters`
- unknown parameters passed during planning are rejected
- planning completion fails if required parameters are missing or blank

Important:

- placeholders are not allowed in `task`
- placeholders are not allowed in `parameters`
- placeholders are not allowed in `workflow.onboarding`
- placeholders are not allowed in `workflow.planning`

Practical guidance:

- keep parameters focused on values that vary per task run
- do not create parameters for static strings that belong in the template itself
- prefer relative file paths in parameters when they are later used by `file_*` conditions

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

Onboarding completion currently stores this artifact shape:

```json
{
  "taskSummary": "Investigate the flaky OAuth reconnect flow and identify the affected tests.",
  "artifactMap": [
    {
      "path": "internal/oauth/store.go",
      "kind": "source",
      "notes": "Token persistence and refresh handling."
    }
  ],
  "commonCommands": [
    {
      "command": "GOCACHE=/tmp/centian-gocache go test ./tests/integrationtests/oauth",
      "purpose": "Run OAuth integration coverage."
    }
  ],
  "constraints": [
    "Do not change public CLI behavior."
  ],
  "openQuestions": [
    "Is the bug specific to reconnect after refresh expiry?"
  ]
}
```

Rules:

- `taskSummary` is required
- `artifactMap`, `commonCommands`, `constraints`, and `openQuestions` are optional
- onboarding artifacts are stored as task context only; they do not substitute placeholders directly

### Planning

Use planning to freeze the execution contract.

```yaml
planning:
  instructions: |
    Confirm the concrete target values and expected failure mode.
  tools_allowed:
    - "shell__*"
    - "filesystem__*"
  editable_fields:
    - "parameters.testCommand"
    - "parameters.testTarget"
    - "parameters.testFile"
  required_inputs:
    - "testCommand"
    - "testFile"
    - "testTarget"
  next: "execution.verify_failing_baseline"
```

Supported fields:

- `instructions`
- `tools_allowed`
- `checkpoint.enabled`
- `editable_fields`
- `required_inputs`
- `next`

Rules for `editable_fields`:

- each value must be unique
- only `parameters.<name>` is supported
- `<name>` must refer to a declared or placeholder-derived parameter

Rules for `required_inputs`:

- each value must be unique
- the set of values must match the template parameter names exactly

These names refer to entries that must be present in `planning.parameters` when `centian.task_complete_planning` is called.

Every planning artifact must include:

- `planSummary`

Optional planning artifact fields are:

- `selectedFiles`
- `parameters`
- `invariants`

Typical planning completion payload:

```json
{
  "planSummary": "Reproduce the failing target, patch refresh handling, then rerun the focused tests.",
  "selectedFiles": [
    "internal/oauth/store.go",
    "tests/integrationtests/oauth/reconnect_test.go"
  ],
  "parameters": {
    "testCommand": "GOCACHE=/tmp/centian-gocache go test ./tests/integrationtests/oauth -run TestOAuthExpiredTokenRefreshesDuringReconnect",
    "testTarget": "./tests/integrationtests/oauth -run TestOAuthExpiredTokenRefreshesDuringReconnect",
    "testFile": "tests/integrationtests/oauth/oauth_reconnect_test.go",
    "expectedError": "token refresh failed"
  },
  "invariants": [
    "Do not change the reconnect API contract."
  ]
}
```

Important:

- `planning.parameters` is where template parameter values are frozen
- the artifact-level `invariants` list is descriptive contract metadata, separate from step-level executable `invariants`
- `required_inputs` is checked against the template parameter names, not against `selectedFiles` or any other artifact field

If `planning.next` is omitted, Centian automatically advances to the first executable node after planning.

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
  - id: "verify_failing_baseline"
    checks:
      - id: "selected_test_fails"
        command: "${testCommand}"
        post_conditions:
          - type: exit_code_in
            values: [1, 2]
          - type: output_contains
            value: "${expectedError}"

  - id: "implement_green"
    checks:
      - id: "selected_test_passes"
        command: "${testCommand}"
        post_conditions:
          - type: exit_code
            value: 0

  - id: "refactor_while_green"
    checks:
      - id: "selected_test_stays_green"
        command: "${testCommand}"
        post_conditions:
          - type: exit_code
            value: 0
```

Executable leaf nodes may omit `checks` entirely. This is useful for free-form templates that are intended to track a task run without enforcing verification at each step.

## Execution Nodes

Scaffolding and execution lists use the same node schema:

```yaml
- id: "implement_green"
  kind: "execution"
  name: "Implement green"
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
      command: "${testCommand}"
  invariants:
    - id: "baseline_test_file_stable"
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

- `id` is required
- node IDs must be unique across the whole workflow, not just inside one list
- `kind` defaults to the containing list kind
- the only non-default `kind` currently allowed is `waiting_for_approval`
- `checks` are optional; if provided, they must be valid
- `invariants` are optional; if provided, they must be valid

## Checks

Checks are the verification units run by Centian.

```yaml
checks:
  - id: "selected_test_fails"
    description: "The selected regression test must fail for the expected reason."
    command: "${testCommand}"
    pre_conditions:
      - type: exit_code_in
        values: [1, 2]
      - type: output_contains
        value: "${expectedError}"
    post_conditions:
      - type: exit_code_in
        values: [1, 2]
      - type: output_contains
        value: "${expectedError}"
```

Rules:

- each check needs `id`
- check IDs must be unique within the step
- each check needs `command`
- `description` is optional; when present, Centian uses it as the governance annotation message for failed checks
- `pre_conditions` are evaluated when `centian.task_start_step` runs
- `post_conditions` are evaluated when `centian.task_complete_step` runs

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

`output_contains`

- required field: `value`
- `value` must be a non-empty string

`output_not_contains`

- required field: `value`
- `value` must be a non-empty string

Condition semantics:

- `stdout_*` conditions only evaluate captured stdout
- `output_*` conditions evaluate the combined command output, meaning stdout plus stderr when both are present
- use `stdout_*` when the command has a stable stdout contract
- use `output_*` when the relevant failure signal may appear on stderr
- compile-failure red baselines are a common reason to prefer `output_*` conditions

Example compile-failure baseline:

```yaml
parameters:
  testCommand: "go test ./internal/oauth -run TestReconnect"
  testTarget: "./internal/oauth -run TestReconnect"
  testFile: "internal/oauth/reconnect_test.go"
  expectedError: "undefined: PlanSummary"
```

`file_exists`

- required field: `path`
- `path` must be relative to the taskverification working directory

`file_not_exists`

- required field: `path`
- `path` must be relative to the taskverification working directory

`file_contains`

- required fields: `path`, `value`
- `path` must be relative to the taskverification working directory
- `value` must be a non-empty string

`file_not_contains`

- required fields: `path`, `value`
- `path` must be relative to the taskverification working directory
- `value` must be a non-empty string

Path safety rules for all `file_*` conditions:

- absolute paths are rejected
- paths that escape the working directory through `..` are rejected

## Invariants

Invariants capture stdout at step start and require the same stdout at step completion.

```yaml
invariants:
  - id: "baseline_test_file_stable"
    command: "cat ${testFile}"
```

Rules:

- each invariant needs `id`
- invariant IDs must be unique within the step
- each invariant needs `command`
- invariant commands must exit with code `0` during capture and verification
- verification compares stdout exactly

Use invariants when a file, selector, or other derived output must remain unchanged for the duration of a step.

For `simple_tdd`, the selected baseline test file is intentionally frozen once execution starts. That invariant makes step 1 a verification step, not a test-authoring step.

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

- Centian compiles only leaf nodes into executable steps
- leaf workflow paths are built from IDs
- step order follows leaf discovery order

Rules for nodes with `sub_steps`:

- they cannot define `checks`
- they cannot define `invariants`
- they cannot define `next`
- they cannot use `kind: waiting_for_approval`

## Approval Wait Nodes

You can insert manual approval pauses by using `kind: waiting_for_approval` inside `execution` or `scaffolding`.

```yaml
execution:
  - id: "implement_green"
    next: "waiting_for_approval.review_plan"

  - id: "review_plan"
    kind: "waiting_for_approval"
    name: "Review plan"
    instructions: |
      Wait for external review before continuing.
    next: "execution.finalize"
```

Behavior:

- active workflow paths for these nodes are under `waiting_for_approval.<id>`
- all proxied downstream tools are blocked while the run is in an approval-wait node
- approval-wait nodes may target later executable nodes through `next`

Rules:

- `waiting_for_approval` nodes cannot define `checks`
- `waiting_for_approval` nodes cannot define `invariants`
- `waiting_for_approval` nodes cannot define `sub_steps`
- `waiting_for_approval` nodes must not be terminal; they need a valid `next`

## Working Directory Behavior

Task template commands run from Centian's taskverification working directory, which is the current working directory of the Centian process at startup.

That working directory affects:

- `checks[].command`
- `invariants[].command`
- `file_*` conditions
- the default runtime disk template directory

When authoring file-based checks, use project-relative paths and assume the working directory is the task workspace root Centian was started in.

## Canonical Examples

The embedded templates in `task-templates/integrated/` are the best current examples of real templates:

- `minimal`
- `simple_tdd`
- `python_tdd_workflow`

Use them as references for:

- parameter design
- planning contract shape
- scaffolding versus execution sequencing
- checks and invariants that are stable enough to automate

## Validation Checklist

Before promoting a template to real use:

1. load it through Centian and confirm it validates
2. confirm `required_inputs` equals the parameter set
3. confirm each declared parameter is used by a placeholder
4. confirm file paths are relative and safe
5. confirm checks are deterministic enough to pass repeatedly
6. confirm approval nodes are non-terminal

## Related Files

- Runtime guide: [TASKVERIFICATION.md](./TASKVERIFICATION.md)
- Packaging note: [task-templates/README.md](../task-templates/README.md)
- Embedded examples: [task-templates/integrated](../task-templates/integrated)
- Runtime code: [internal/taskverification](../internal/taskverification)
