# Task Verification Demo

This demo runs a two-container PoC:

- `centian`: hosts Centian, mounts the Python sample project, and proxies filesystem plus shell MCP servers.
- `agent`: runs Codex headlessly against Centian and must complete the task through `centian.task_*` tools.

The sample project is mounted only into the Centian container. The agent does not see the project directly.
Centian is exposed to the host on `127.0.0.1:${CENTIAN_TASKVERIFICATION_PORT:-8678}` and is also reachable from the agent container on `http://centian:8080`.

## Requirements

- Docker with `docker compose`
- `OPENAI_API_KEY`

Optional:

- `CODEX_MODEL` to force a specific model for `codex exec`
- `CENTIAN_TASKVERIFICATION_PORT` to change the localhost port, default `8678`

## Flow

The demo now uses one generic task template:

- `python_tdd_workflow`

The scenario comes from the prompt you choose:

- `run-agent-problem`: harder path, prompt contains only the problem statement
- `run-agent-existing`: simpler path, prompt points at an existing bug in the sample project

The default agent path prefers the harder problem-only scenario.

Default flow:

1. The agent lists templates and registers `python_tdd_workflow`.
2. Step 1 proves the selected or authored pytest case fails.
3. The agent edits or creates the project through Centian-exposed tools only.
4. Step 2 proves the selected test passes and `ruff` is clean.

Request logs are written to `demo/taskverification/artifacts/logs/requests_*.jsonl`.

## Commands

```bash
cd /Users/brb/_devspace/centian-cli/demo/taskverification
make reset-project
make build
make demo-up
make run-agent-problem
make run-agent-existing
make demo-down
```

For the explicit end-to-end verification path, the Make targets now call the Go e2e harness instead of shelling the demo flow directly:

```bash
cd /Users/brb/_devspace/centian-cli
make demo-taskverification-test
make demo-taskverification-test-problem
make demo-taskverification-test-existing
```

Scenario-specific Go e2e runs are also available directly from the demo folder:

```bash
cd /Users/brb/_devspace/centian-cli/demo/taskverification
make e2e-problem
make e2e-existing
make e2e
```

The Go harness is extensible through scenario registration in [demo_test.go](/Users/brb/_devspace/centian-cli/demo/taskverification/demo_test.go). New tasks only need a new prompt plus a new scenario entry; the Docker lifecycle, artifact reset, request-log assertions, and final verification stay shared.

## Unsafe PoC Note

The shell MCP server is intentionally exposed without additional safety restrictions for this PoC. That is not an acceptable production setup.
