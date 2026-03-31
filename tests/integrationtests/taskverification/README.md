# Taskverification Black-Box Integration Test

This directory contains the host-native black-box integration harness for Centian taskverification.

The goal of this test is to run real local coding agents against a real Centian process and assert the result from Centian's own task runs, events, and request logs, not from the agent's prose.

## What This Test Covers

The harness starts a fresh local Centian instance, points a headless agent at that instance over MCP, gives the agent a clean user-style prompt, and then checks whether Centian observed a valid taskverification lifecycle.

Today the suite supports:

- `codex`
- `claude`

The same prompt and task template are used for both agents.

This is intentionally a black-box test:

- the agent sees only the configured Centian MCP server
- the prompt does not teach the agent Centian internals
- success is asserted from Centian task state and project output

## Fixture Files

This directory contains the versioned fixtures used by the harness:

- [blackbox_test.go](/Users/brb/_devspace/centian-cli/tests/integrationtests/taskverification/blackbox_test.go): test harness and strict assertions
- [centian_config.json](/Users/brb/_devspace/centian-cli/tests/integrationtests/taskverification/centian_config.json): Centian config template resolved into a temp run directory
- [prompt.yaml](/Users/brb/_devspace/centian-cli/tests/integrationtests/taskverification/prompt.yaml): shared user-style prompt
- [python_tdd_workflow.yaml](/Users/brb/_devspace/centian-cli/tests/integrationtests/taskverification/python_tdd_workflow.yaml): taskverification template fixture
- [codex_config.toml](/Users/brb/_devspace/centian-cli/tests/integrationtests/taskverification/codex_config.toml): Codex config template
- [claude_mcp_config.json](/Users/brb/_devspace/centian-cli/tests/integrationtests/taskverification/claude_mcp_config.json): Claude MCP config template

The harness creates one preserved run directory per agent under [`.tmp`](/Users/brb/_devspace/centian-cli/tests/integrationtests/taskverification/.tmp), for example:

```text
tests/integrationtests/taskverification/.tmp/20260330145635_run_codex
```

Each run directory contains:

- `project/`: the empty project root the agent must populate
- `templates/`: localized taskverification templates used by Centian
- `logs/`: request logs and SQLite event store
- `centian.config.json`: resolved Centian config for the run
- `centian.stdout.log` and `centian.stderr.log`
- `codex/` or `claude/`: agent-specific config, stdout, stderr, and captured final output

Artifacts are intentionally preserved after the test finishes.

## Runtime Flow

For each selected agent, the harness does the following:

1. Creates a fresh temp run directory under [`.tmp`](/Users/brb/_devspace/centian-cli/tests/integrationtests/taskverification/.tmp).
2. Creates an empty `project/` directory.
3. Copies the versioned task template fixture into `templates/`.
4. Resolves [centian_config.json](/Users/brb/_devspace/centian-cli/tests/integrationtests/taskverification/centian_config.json) into a run-local config.
5. Starts `centian start --config-path <run-config>` with its process working directory set to the run-local project root.
6. Waits for:
   - the MCP endpoint `/mcp/taskverification`
   - the HTTP API `/api/task-runs`
7. Prints the UI URL:
   - `http://127.0.0.1:<port>/ui/tasks`
8. Creates agent-specific config in the run directory and starts the agent headlessly.
9. After the agent exits or times out, fetches:
   - `/api/task-runs`
   - `/api/task-runs/{runID}/events`
   - the latest `requests_*.jsonl`
10. Verifies both Centian lifecycle behavior and the produced Python files in `project/`.

## Strict Success Criteria

The black-box test currently requires all of the following:

- at least one task run was created
- the latest task run finished with `completed`
- the observed task tool call order includes:
  - `centian.task_list_templates`
  - `centian.task_register`
  - `centian.task_complete_onboarding`
  - `centian.task_complete_planning`
  - `centian.task_start_step`
  - `centian.task_complete_step`
- `centian.task_fail` was not used
- no terminal failure or timeout event was recorded
- the agent created:
  - `score_parentheses.py`
  - `test_score_parentheses.py`
- the final project passes:
  - `python test_score_parentheses.py`
  - `python -m py_compile score_parentheses.py test_score_parentheses.py`

## Running The Test

Preferred command:

```bash
make test-taskverification-blackbox
```

Direct `go test` command:

```bash
CENTIAN_RUN_TASKVERIFICATION_BLACKBOX=1 \
go test -v ./tests/integrationtests/taskverification -run TestTaskVerificationBlackBox
```

Useful environment variables:

- `CENTIAN_RUN_TASKVERIFICATION_BLACKBOX=1`: required opt-in gate
- `CENTIAN_TASKVERIFICATION_AGENTS=codex,claude`: agent selection
- `CENTIAN_TASKVERIFICATION_AGENT_TIMEOUT=15m`: per-agent timeout
- `CENTIAN_TASKVERIFICATION_BINARY=/path/to/centian`: Centian binary override
- `CODEX_MODEL=<model>`: optional Codex model override
- `CLAUDE_MODEL=<model>`: optional Claude model override

Prerequisites:

- `centian` available on `PATH`, or set via `CENTIAN_TASKVERIFICATION_BINARY`
- `python`
- `npx`
- `codex` and/or `claude`

The test fixture assumes only standard-library Python is available. It does not require `pytest` or `ruff`.

## Debugging

The printed UI URL can be opened while the test is running to observe task runs in real time.

The most useful artifacts after a failure are:

- `centian.stderr.log`: Centian startup and proxy errors
- `logs/requests_*.jsonl`: exact MCP request log as seen by Centian
- `logs/events.sqlite`: persisted task and action events
- `codex/stdout.jsonl` or `claude/stdout.json`: agent transcript output
- `codex/stderr.log` or `claude/stderr.log`

If an agent created no task run at all, start with:

- the agent stderr log
- the run-local config files
- `centian.stderr.log`

If a task run exists but did not complete, inspect:

- the task events from the UI or API
- `requests_*.jsonl`
- the project files under `project/`

## Known Limitation

This harness is a strong black-box test of Centian taskverification, but it is **not yet a strict MCP-only confinement test**, especially for Codex.

Current limitation:

- Codex can still use native workspace capabilities that are not mediated by Centian.
- Proxied tool annotations are preserved from the downstream tools, so approval behavior is still partly determined outside Centian.
- Changing annotations only for Centian-owned tools is not enough, because proxied tools can still present approval-triggering metadata to the client.

In practice this means:

- a run can succeed through Centian while still involving agent-native file access
- approval or tool-safety behavior may differ across clients even when the Centian task flow is identical

This does not make the test useless, but it does mean the result should be interpreted as:

- "did the agent complete a real task through Centian?"

not as:

- "was every action forced through Centian and only through Centian?"

## Potential Future Mitigation

A likely future mitigation is a **configurable, client-specific tool annotation override** at the gateway level.

High-level idea:

- configure a gateway policy that targets specific clients such as Codex
- when a matching client connects, Centian rewrites annotations for **all tools exposed by that gateway**
- this includes both:
  - Centian-owned task tools
  - proxied downstream tools

The most likely first policy mode is:

- `force_read_only`

That mode would present all tools on that gateway as:

- `readOnlyHint = true`
- `idempotentHint = true`
- `destructiveHint = false`
- `openWorldHint = false`

This would not change actual tool behavior or governance. It would only change how the client interprets tool safety and approval requirements.

That mitigation is intentionally deferred. The current priority of this directory is to keep the black-box taskverification test understandable, inspectable, and stable enough to expose real agent-integration issues.
