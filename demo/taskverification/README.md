# Task Verification Demo

This demo runs a two-container PoC:

- `centian`: hosts Centian, mounts the Python sample project, and proxies filesystem plus shell MCP servers.
- `agent`: runs Codex headlessly against Centian and must complete the task through `centian.task_*` tools.

The sample project is mounted only into the Centian container. The agent does not see the project directly.
Centian is exposed to the host on `127.0.0.1:${CENTIAN_TASKVERIFICATION_PORT:-8678}` and is also reachable from the agent container on `http://centian:8080`.

## Requirements

- Docker with `docker compose`
- `OPENAI_API_KEY`
- Node 22 only if you also run root-level local frontend builds such as `make build`

Optional:

- `CODEX_MODEL` to force a specific model for `codex exec`
- `CENTIAN_TASKVERIFICATION_PORT` to change the localhost port, default `8678`

## Flow

The demo now exercises the current workflow-path runtime explicitly:

1. The agent lists templates and registers a workflow template.
2. The agent completes onboarding with a concise project summary and artifact map.
3. The agent completes planning with the required planning artifact.
4. Execution step tools run only after planning has advanced the task into an execution node.
5. When a template routes into `waiting_for_approval.*`, proxied downstream tools are blocked and the task stops there.

The demo currently exposes three scenarios:

- `run-agent-problem`: full TDD flow using `python_tdd_workflow` from only a problem statement
- `run-agent-existing`: full TDD flow using `python_tdd_workflow` for an existing bug
- `run-agent-approval`: planning routes into `python_tdd_approval_wait` and stops at approval wait

Artifacts are written to:

- request logs: `demo/taskverification/artifacts/logs/requests_*.jsonl`
- persisted event store: `demo/taskverification/artifacts/logs/events.sqlite`

## Commands

```bash
cd demo/taskverification
make reset-project
make build
make demo-up
make run-agent-problem
make run-agent-existing
make run-agent-approval
make demo-down
```

Keep using `make build` for this demo flow so the container image embeds the real frontend assets.

For the explicit end-to-end verification path, the Make targets now call the Go e2e harness instead of shelling the demo flow directly:

```bash
make test-taskverification
```

Scenario-specific Go e2e runs are also available directly from the demo folder:

```bash
cd demo/taskverification
make e2e-problem
make e2e-existing
make e2e-approval
make e2e
```

The Go harness is extensible through scenario registration in [demo_test.go](.demo/taskverification/demo_test.go). New tasks only need a new prompt plus a new scenario entry; the Docker lifecycle, artifact reset, request-log assertions, persisted event-store checks, and final verification stay shared.

## Unsafe PoC Note

The shell MCP server is intentionally exposed without additional safety restrictions for this PoC. That is not an acceptable production setup.
