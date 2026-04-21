# Benchmarking

Centian benchmarking makes taskverification changes measurable.

Instead of judging one transcript or one demo run, benchmarking runs the same case repeatedly across agents, models, template variants, and attempts, then derives scorecards from persisted benchmark artifacts plus normal Centian task/action persistence.

The benchmark CLI is designed to be portable. It does not require running inside this repository as long as you provide:

- a `centian` binary on `PATH` or an explicit binary path
- a benchmark suite directory via `--suite`
- a template source via `--template-dir`, `--centian-config`, or a local `task-templates/integrated`

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
- benchmark runs can use a custom base config via `--centian-config`; placeholders such as `__EVENT_STORE_PATH__` and `__TEMPLATES_DIR__` are only resolved if present

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

The suite itself can live anywhere. The checked-in directory above is only where this repository stores its own examples.

## Running Benchmarks

Run one suite/case:

```bash
centian benchmark run \
  --suite /path/to/simple_tdd_v1 \
  --agent codex \
  --model gpt-5.4-mini \
  --case assertion_failure_red
```

Run demo-derived case:

```bash
centian benchmark run \
  --suite /path/to/centian_demo_v1 \
  --agent codex \
  --case score_parentheses_js
```

Run with an explicit Centian benchmark config:

```bash
centian benchmark run \
  --suite /path/to/centian_demo_v1 \
  --agent gemini \
  --model pro \
  --centian-config /path/to/benchmark.centian.json
```

Run with an explicit template variant and output root:

```bash
centian benchmark run \
  --suite /path/to/centian_demo_v1 \
  --agent claude \
  --model sonnet \
  --template-dir current=/path/to/task-templates/integrated \
  --output-root /path/to/benchmark-artifacts
```

Run multiple agents with per-agent model overrides and two attempts per matrix cell:

```bash
centian benchmark run \
  --suite /path/to/simple_tdd_v1 \
  --agent codex \
  --agent claude \
  --repeat 2 \
  --codex-model gpt-5.4-mini \
  --claude-model sonnet
```

Inspect one preserved session in the embedded UI or API:

- UI: `/ui/benchmarks/<suite-id>/sessions/<session-id>`
- API: `GET /api/benchmarks/suites/<suite-id>/sessions/<session-id>`

Compare persisted sessions through the API:

- `GET /api/benchmarks/suites/simple_tdd_v1/comparison`

Agent/model selection:

- use one or more `--agent`
- for single-agent runs use `--model` / `-m`
- `codex-ollama` is the exception: it requires `--codex-config` plus `--profile`, because the model comes from the named Codex profile
- for multi-agent runs use `--profile` for `codex-ollama`, plus `--codex-model`, `--claude-model`, `--gemini-model` as needed
- `--model` cannot be combined with per-agent override flags for the same selected agent
- `--model` cannot be used for multi-agent runs; use the per-agent override flags instead

Supported model shorthands:

- Codex: `gpt-5.4`, `gpt-5.4-mini`
- Claude: `haiku`, `sonnet`, `opus`
- Gemini: `pro`, `flash`, `2.5-flash`

`codex-ollama` does not have built-in defaults. Supply a Codex config that already defines the local OSS profile you want to run.

Important execution flags:

- `--repeat` controls attempts per matrix cell; default is `1`
- `--timeout` sets the per-run timeout; default is `15m`
- `--template-dir name=path` is repeatable, so one invocation can benchmark multiple template variants
- `--keep-centian-running` prints the benchmark UI URL after each run and prompts whether to shut down the temporary Centian process

Template and config resolution:

- precedence is: explicit `--template-dir`, then concrete `taskVerification.templatesPath` from `--centian-config`, then `<cwd>/task-templates/integrated`, then `<repo-root>/task-templates/integrated`
- a `taskVerification.templatesPath` containing the placeholder `__TEMPLATES_DIR__` is treated as unresolved and does not count as an implicit template source
- relative `taskVerification.templatesPath` values from `--centian-config` are resolved relative to the config file directory
- `eventStorage.path` from `--centian-config` can also be used as the shared benchmark event-store path
- runtime placeholder patching still applies when present: `__TEMPLATES_DIR__` and `__EVENT_STORE_PATH__` are replaced in the copied run-local Centian config

Artifact root resolution:

- `--output-root` overrides artifact placement directly
- if omitted, benchmark artifacts go under `<cwd>/.centian/benchmarks`
- this keeps benchmark runs self-contained and independent from repository-specific test fixture trees

## Preserved Artifacts

One `benchmark run` invocation writes preserved outputs under:

```text
<output-root>/<suite-id>/<timestamp>_<label>/
```

Important files:

- `session.json`: whole benchmark invocation manifest
- `runs/<template-variant>_<agent>_<case-id>_attempt_001/run.json`: one concrete run manifest
- `runs/<template-variant>_<agent>_<case-id>_attempt_001/project/`: post-run project tree
- `runs/<template-variant>_<agent>_<case-id>_attempt_001/logs/requests_*.jsonl`: Centian request log
- `runs/<template-variant>_<agent>_<case-id>_attempt_001/agent/agent.stdout.log`: agent log used for metadata extraction

For single-agent single-variant runs, the default session directory label is `<variant>_<agent>_run`, for example:

```text
.centian/benchmarks/centian_demo_v1/20260411103000_current_codex_run/
```

Each `run.json` also records resolved shared event-store path in `artifactPaths.eventStorePath`.

## Persistence Model

Benchmarking uses two persistence layers:

1. Preserved benchmark artifacts on disk
2. SQLite read models in Centian event storage

SQLite stores:

- benchmark sessions
- benchmark runs
- benchmark run score snapshots
- task run snapshots
- task/action events
- derived task-run stats

Benchmark run metadata includes selected model plus persisted agent metadata JSON. Benchmark reads are DB-first:

- normal UI and API reads do not rescore from filesystem artifacts
- per-run score snapshots are persisted in SQLite at run time
- runs that fail to score inline remain visible as unscored instead of contributing synthetic zero metrics

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
- total and median tool-call counts split as `Centian / MCP`
- total and median error counts split as `Centian / MCP`
- median time
- success rate
- first-pass rate

## Metric Semantics

Important scorecard meanings:

- Success Rate: runs that completed and also passed final verification / total runs
- First Pass: runs that passed final verification with no restart, fail, or timeout / total runs
- Final Verification Pass: runs whose final task run finished with completed status, regardless of whether the outer benchmark run itself failed
- Tool Calls: total and median tool-call counts split into Centian lifecycle tools vs downstream MCP tools
- Errors: total and median error counts split into Centian lifecycle tool errors vs downstream MCP tool errors

Benchmarking is process-aware. It measures more than final pass/fail.

## Troubleshooting

Common benchmark-specific failures:

- `codex-ollama requires --codex-config`: pass a Codex config file for any `codex-ollama` run
- `codex-ollama requires an explicit profile; use --profile`: `codex-ollama` has no implicit default profile
- `default template dir was not found`: add `--template-dir`, provide a concrete templates path in `--centian-config`, or create `task-templates/integrated` under the current working directory
- `repeat must be greater than zero`: benchmark attempts must be a positive integer
- unscored runs in UI/API usually mean the run artifact was preserved but inline score derivation failed; the run remains visible, but it does not contribute synthetic zero metrics

If a run still appears in benchmark UI after manual cleanup, check `benchmark_runs` and `benchmark_run_task_runs` first. Benchmark overview pages are driven from benchmark run rows plus the normalized benchmark-to-task-run links, not only from `task_runs`.

Useful SQLite checks:

```sql
SELECT l.benchmark_run_id, l.task_run_id, l.link_order
FROM benchmark_run_task_runs l
JOIN benchmark_runs r ON r.benchmark_run_id = l.benchmark_run_id
ORDER BY r.started_at_unix_milli DESC, l.link_order ASC
LIMIT 20;
```

Delete one timed-out task from benchmark overview:

```sql
DELETE FROM benchmark_run_task_runs
WHERE task_run_id = 'tr_xxx';
```

Delete the now-unlinked benchmark run rows if needed:

```sql
DELETE FROM benchmark_runs
WHERE benchmark_run_id NOT IN (
  SELECT DISTINCT benchmark_run_id
  FROM benchmark_run_task_runs
);
```

If you expect parent/child cleanup to cascade, enable foreign keys before delete:

```sql
PRAGMA foreign_keys = ON;
```

## Related Files

- Deep benchmark fixture and CLI reference: [tests/integrationtests/taskverification/benchmarks/README.md](../tests/integrationtests/taskverification/benchmarks/README.md)
- Taskverification runtime: [TASKVERIFICATION.md](./TASKVERIFICATION.md)
- Template authoring: [TASK_TEMPLATE_AUTHORING.md](./TASK_TEMPLATE_AUTHORING.md)
