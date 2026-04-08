# Benchmarking

Centian benchmarking makes taskverification changes measurable.

Instead of judging one transcript or one demo run, benchmarking runs the same case repeatedly across agents, models, template variants, and attempts, then derives scorecards from persisted benchmark artifacts plus normal Centian task/action persistence.

## What Benchmarking Adds

Benchmarking adds four practical things:

- repeatable local benchmark suites
- preserved per-run artifacts
- derived scorecards for sessions and cross-session comparison
- read-only benchmark API and UI views over persisted history

Use it when you want to answer questions like:

- did this template change improve success rate?
- did this agent/model combination reduce retries or timeouts?
- did one template variant use fewer downstream MCP calls for same result?
- did a workflow change preserve invariants more reliably?

## Prerequisites

Benchmarking depends on normal taskverification plus persistence.

Recommended Centian config:

```json
{
  "proxy": {
    "capabilities": {
      "taskVerification": {
        "enabled": true
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

- `taskVerification.enabled` needed because benchmark runs execute through Centian task lifecycle.
- `eventStorage.enabled` needed because benchmark scoring reads persisted task/action history.
- `ui.enabled` needed only if you want embedded benchmark pages.
- default SQLite path is `~/.centian/logs/events.sqlite` when `eventStorage.path` not set.

## Suite Layout

Repo-tracked benchmark definitions live under:

```text
tests/integrationtests/taskverification/benchmarks/
```

Each suite uses:

- `suite.yaml`
- `cases/<case-id>/case.yaml`
- `cases/<case-id>/prompt.yaml`
- `cases/<case-id>/fixture/`

Checked-in suites today include:

- `simple_tdd_v1`
- `centian_demo_v1`

`centian_demo_v1` turns `centian demo` into a benchmark scenario using `guided_tdd_workflow`.

## Running Benchmarks

Run one suite/case:

```bash
./build/centian benchmark run \
  --suite tests/integrationtests/taskverification/benchmarks/simple_tdd_v1 \
  --agent codex \
  --model gpt-5.4-mini \
  --case assertion_failure_red
```

Run demo-derived case:

```bash
./build/centian benchmark run \
  --suite tests/integrationtests/taskverification/benchmarks/centian_demo_v1 \
  --agent codex \
  --case score_parentheses_js
```

Score one preserved session:

```bash
./build/centian benchmark score \
  --session tests/integrationtests/taskverification/.tmp/benchmarks/<suite-id>/<timestamp>_run
```

Compare persisted sessions:

```bash
./build/centian benchmark compare \
  --root tests/integrationtests/taskverification/.tmp/benchmarks \
  --suite simple_tdd_v1
```

Agent/model selection:

- use one or more `--agent`
- for single-agent runs use `--model` / `-m`
- for multi-agent runs use `--codex-model`, `--claude-model`, `--gemini-model`

Supported model shorthands:

- Codex: `gpt-5.4`, `gpt-5.4-mini`
- Claude: `haiku`, `sonnet`, `opus`
- Gemini: `pro`, `flash`, `2.5-flash`

## Preserved Artifacts

One `benchmark run` invocation writes preserved outputs under:

```text
tests/integrationtests/taskverification/.tmp/benchmarks/<suite-id>/<timestamp>_<label>/
```

Important files:

- `session.json`: whole benchmark invocation manifest
- `runs/.../run.json`: one concrete run manifest
- `runs/.../project/`: post-run project tree
- `runs/.../logs/requests_*.jsonl`: Centian request log
- `runs/.../agent/agent.stdout.log`: agent log used for metadata extraction
- `runs/.../manual_score.json`: optional reviewer score input

Each `run.json` also records resolved shared event-store path in `artifactPaths.eventStorePath`.

## Persistence Model

Benchmarking uses two persistence layers:

1. Preserved benchmark artifacts on disk
2. SQLite read models in Centian event storage

SQLite stores:

- benchmark sessions
- benchmark runs
- task run snapshots
- task/action events
- derived task-run stats

Benchmark run metadata includes selected model plus persisted agent metadata JSON. That lets benchmark UI keep showing model info even when raw logs are gone.

## Benchmark API

Read-only benchmark API routes:

- `GET /api/benchmarks/suites`
- `GET /api/benchmarks/template-scorecards`
- `GET /api/benchmarks/agent-scorecards`
- `GET /api/benchmarks/suites/{suiteID}/sessions`
- `GET /api/benchmarks/suites/{suiteID}/sessions/{sessionID}`
- `GET /api/benchmarks/suites/{suiteID}/runs`
- `GET /api/benchmarks/suites/{suiteID}/runs/{scorecardID}`
- `GET /api/benchmarks/suites/{suiteID}/comparison`

Agent scorecards are split by `agent + model`, not only by agent. One agent using two models shows as two rows.

## Embedded UI

When `ui.enabled` and persistence are on, benchmark views appear in embedded SPA served under `/ui`.

Main benchmark pages:

- `/ui/benchmarks`
- `/ui/benchmarks/:suiteID`
- `/ui/benchmarks/:suiteID/sessions/:sessionID`
- `/ui/benchmarks/:suiteID/runs/:scorecardID`

Current overview scorecards include:

- runs
- total and median MCP event counts split as `Centian / MCP`
- total and median error counts split as `Centian / MCP`
- median time
- success rate
- first-pass rate

## Metric Semantics

Important scorecard meanings:

- Success Rate: completed runs / total runs
- First Pass: completed runs with no restart, fail, or timeout
- MCP Events: total and median tool-call counts split into Centian lifecycle tools vs downstream MCP tools
- Errors: total and median error counts split into Centian lifecycle tool errors vs downstream MCP tool errors

Benchmarking is process-aware. It measures more than final pass/fail.

## Troubleshooting

If a run still appears in benchmark UI after manual cleanup, check `benchmark_runs` first. Benchmark overview pages are driven from benchmark run rows, not only from `task_runs`.

Useful SQLite checks:

```sql
SELECT benchmark_run_id, latest_task_run_id, linked_task_run_ids_json
FROM benchmark_runs
ORDER BY started_at_unix_milli DESC
LIMIT 20;
```

Delete one timed-out task from benchmark overview:

```sql
DELETE FROM benchmark_runs
WHERE latest_task_run_id = 'tr_xxx'
   OR CAST(linked_task_run_ids_json AS TEXT) LIKE '%tr_xxx%';
```

If you expect parent/child cleanup to cascade, enable foreign keys before delete:

```sql
PRAGMA foreign_keys = ON;
```

## Related Files

- Deep benchmark fixture and CLI reference: [tests/integrationtests/taskverification/benchmarks/README.md](../tests/integrationtests/taskverification/benchmarks/README.md)
- Taskverification runtime: [TASKVERIFICATION.md](./TASKVERIFICATION.md)
- Template authoring: [TASK_TEMPLATE_AUTHORING.md](./TASK_TEMPLATE_AUTHORING.md)
