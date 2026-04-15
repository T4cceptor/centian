# Benchmark Artifacts

This directory contains repo-tracked benchmark definitions for Centian taskverification.

Benchmarks live beside the existing taskverification integration fixtures because later benchmark runners are expected to reuse the same host-native black-box infrastructure.

## Purpose

The benchmark command exists to make taskverification changes measurable.

Instead of judging a template, prompt, or workflow change from one transcript or one successful demo, the benchmark system gives you a repeatable way to ask:

- does this change improve success rate?
- does it reduce retries, failures, or restarts?
- does it preserve important invariants more reliably?
- does it reduce time and tool activity for the same task outcome?

The goal is to make Centian process refinement empirical. A benchmark run produces preserved artifacts, raw run metadata, and derived scorecards so process changes can be compared across:

- multiple attempts
- multiple agents
- multiple template variants
- later, multiple execution modes such as Centian-backed vs agent-only

## Why Use The Benchmark Command

Use the benchmark command when you want to evaluate process quality.

Typical uses:

- validate a task template change before treating it as an improvement
- compare two template variants on the same benchmark case
- compare different agents on the same workflow
- inspect where a workflow fails, not just whether it eventually succeeds
- build a historical baseline for later UI and persistence-backed comparisons

In practice, the benchmark command is useful because it gives you:

- preserved run artifacts instead of ephemeral console output
- structured scorecards instead of subjective impressions
- repeatable local execution on the same benchmark cases
- a path toward A/B testing process changes over time

## What The Benchmark Evaluates

The current benchmark system evaluates how well an agent completes a benchmark case through Centian's taskverification flow.

Today that means each preserved run is evaluated from:

- the benchmark fixture and case contract
- the copied project state before and after the run
- Centian task runs and task-run events
- request logs and action/tool activity
- the shared SQLite event store Centian normally uses for durable observability
- optional reviewer-supplied `manual_score.json`

For the current `simple_tdd_v1` suite, the benchmark is trying to answer:

- did the agent get the authored-red baseline to green?
- did it do that without changing the locked baseline test file?
- did it avoid restarts, failures, or timeouts?
- how many task-tool and downstream-tool failures happened on the way?
- how much time, tool activity, and file editing did the run require?

This is intentionally process-aware. It does not only ask "did the final code work?" It also captures whether the workflow was efficient, stable, and compliant with the case contract.

The `centian_demo_v1` suite converts the local `centian demo` JavaScript score-parentheses task into the same benchmark format. It uses the `guided_tdd_workflow` template and starts without an implementation or test file, matching the demo flow where the agent must scaffold the red baseline first.

## How To Run A Benchmark

### Recommended make target

The simplest way to run one local benchmark case is:

```bash
make benchmark-simple-tdd
```

That target:

- builds `./build/centian`
- runs `centian benchmark run`
- prints the newest preserved session path for follow-up UI/API inspection
- prints the live Centian UI URL for each run as soon as the server is ready

### Direct CLI commands

Run a benchmark session:

```bash
./build/centian benchmark run \
  --suite tests/integrationtests/taskverification/benchmarks/simple_tdd_v1 \
  --agent codex \
  --model gpt-5.4-mini \
  --case assertion_failure_red

./build/centian benchmark run \
  --suite tests/integrationtests/taskverification/benchmarks/simple_tdd_v1 \
  --agent codex-ollama \
  --codex-config ~/.codex/config.toml \
  --profile local-oss \
  --case assertion_failure_red
```

Run the demo-derived scenario:

```bash
./build/centian benchmark run \
  --suite tests/integrationtests/taskverification/benchmarks/centian_demo_v1 \
  --agent codex \
  --case score_parentheses_js
```

Keep the Centian server alive after the agent finishes and prompt for shutdown:

```bash
./build/centian benchmark run \
  --suite tests/integrationtests/taskverification/benchmarks/simple_tdd_v1 \
  --agent codex \
  --case assertion_failure_red \
  --keep-centian-running
```

Inspect an existing session:

- UI: `/ui/benchmarks/simple_tdd_v1/sessions/<session-id>`
- API: `GET /api/benchmarks/suites/simple_tdd_v1/sessions/<session-id>`

Compare multiple sessions for one suite:

- API: `GET /api/benchmarks/suites/simple_tdd_v1/comparison`

## How To Use Different Agents

The benchmark runner accepts one or more `--agent` flags. The runner executes the same benchmark case(s) once per agent, per template variant, per attempt.
For a single-agent run, use `--model` / `-m` to select that agent's model. `codex-ollama` is the exception: it requires `--profile` plus `--codex-config`, because the model is selected by the named profile in the supplied Codex config. For multi-agent runs, keep using `--profile` for `codex-ollama`, plus `--codex-model`, `--claude-model`, and `--gemini-model` for the other agents.
`codex-ollama` requires a user-managed base Codex config; Centian copies that config per run and patches only the run-local MCP URL and trusted project path.

Examples:

```bash
./build/centian benchmark run \
  --suite tests/integrationtests/taskverification/benchmarks/simple_tdd_v1 \
  --agent codex \
  --agent claude
```

Or via `make`:

```bash
make benchmark-simple-tdd BENCH_AGENT=codex # default
make benchmark-simple-tdd BENCH_AGENT=claude
```

The current runner supports:

- `codex`
- `codex-ollama`
- `claude`
- `gemini`

The agent must be installed and available on `PATH` for the run to work.

## Artifact Layout

Each suite uses the same generic structure:

- `suite.yaml`: suite metadata, targeted template, and case references
- `cases/<case-id>/case.yaml`: one benchmark case contract
- `cases/<case-id>/prompt.yaml`: the user-style prompt for the case
- `cases/<case-id>/fixture/`: the seeded authored-red starting state

The checked-in files under this directory are inputs. They define what should be benchmarked, not the artifacts of one concrete run.

## Preserved Local Outputs

The benchmark runner preserves local outputs under a local Centian workspace. One benchmark CLI invocation creates an invocation directory:

```text
.centian/benchmarks/<suite-id>/<timestamp>_<label>/
```

That directory contains:

- `session.json`
- `runs/<template-variant>_<agent>_<case-id>_attempt_001/`

Each preserved run contains:

- `project/`
- `selected-template.yaml`
- `logs/`
- `agent/`
- `run.json`
- `manual_score.json` optionally, for reviewer-supplied error-actionability input

`logs/` stores Centian-side runtime artifacts such as request logs and internal logs. The live durable event stream is written to Centian's configured shared event store, which by default resolves to `~/.centian/logs/events.sqlite`. The resolved shared path is recorded in `run.json`. `agent/` stores agent logs and agent-specific artifacts for the run.

## How To Inspect Output

The default preserved output root is:

- [tests/integrationtests/taskverification/.tmp/benchmarks](/Users/brb/_devspace/centian-cli/tests/integrationtests/taskverification/.tmp/benchmarks)

For the current suite, sessions are written under:

- [tests/integrationtests/taskverification/.tmp/benchmarks/simple_tdd_v1](/Users/brb/_devspace/centian-cli/tests/integrationtests/taskverification/.tmp/benchmarks/simple_tdd_v1)

The most useful files after a run are:

- `session.json`
  One manifest describing the full benchmark invocation.
- `run.json`
  Raw metadata for one concrete run.
- `logs/requests_*.jsonl`
  The exact request log seen by Centian.
- `run.json -> artifactPaths.eventStorePath`
  The shared SQLite event store used by the run, normally `~/.centian/logs/events.sqlite`.

Benchmark UI and API views derive their output live from:

- persisted benchmark `session` / `run` artifacts
- persisted `task_runs`
- persisted `task_run_stats`

## Score Meaning

### Outcome metrics

- `completedSuccessfully`
  The overall run finished successfully and the latest linked task run ended in `completed`.
- `finalVerificationPassed`
  The latest linked task run finished with `completed`.
- `firstPassSuccess`
  The run eventually completed and there were no `task_restarted`, `task_failed`, or `task_timed_out` events.
- `restartOccurred`
  A task restart happened during the run.
- `failOccurred`
  The task was explicitly failed during the run.
- `timeoutOccurred`
  The task hit an inactivity timeout during the run.
- `invariantViolation`
  One or more locked paths defined by the benchmark case changed relative to the seed fixture.

### Process metrics

- `failedTaskToolCalls`
  Count of failed `centian.task_*` tool calls.
- `failedDownstreamToolCalls`
  Count of failed non-task tool calls, such as filesystem or shell-related actions.
- `totalTaskToolCalls`
  Count of all `centian.task_*` tool calls.
- `totalDownstreamToolCalls`
  Count of all non-task tool calls.
- `restartCount`
  Number of task restarts.
- `failCount`
  Number of explicit task failures.
- `timeoutCount`
  Number of task timeouts.

### Efficiency metrics

- `wallClockSeconds`
  Total elapsed run time from `run.json`.
- `totalToolCalls`
  Sum of task-tool and downstream-tool calls.
- `editedFilesCount`
  Number of added, modified, or deleted files relative to the case seed fixture.
- `editedFiles`
  The concrete paths that changed relative to the case seed fixture.
### Manual metrics

- `errorActionabilityScore`
  Optional reviewer-supplied `0..3` rubric score for how actionable failures were.
- `errorActionabilityNotes`
  Optional reviewer notes stored in `manual_score.json`.

## How To Know If You Are Improving

Benchmark results are most useful when compared across repeated runs, agents, or template variants. A single run can be noisy. Improvement usually means:

- higher `completedSuccessfully` rate
- higher `firstPassSuccess` rate
- lower `invariantViolation` rate
- lower `restartOccurred`, `failOccurred`, and `timeoutOccurred` rates
- lower `failedTaskToolCalls` and `failedDownstreamToolCalls`
- lower `wallClockSeconds`
- lower `totalToolCalls`
- lower `editedFilesCount` for the same task outcome

For `simple_tdd`, the strongest signals are usually:

1. success rate
2. first-pass success rate
3. invariant violation rate
4. median wall-clock time on successful runs
5. median failed tool calls on successful runs

In practice:

- if success goes up and time/tokens/tool activity stay roughly flat, that is a clear improvement
- if success stays the same but restarts, failures, and time/tool cost go down, that is probably still an improvement
- if time goes down but invariant violations go up, that is not an improvement
- if one agent regresses while another improves, compare by agent rather than averaging too early

Session detail views already group runs inside one benchmark invocation by:

- case
- agent
- template variant
- case + agent + template variant

For comparisons across multiple sessions of the same suite, use `GET /api/benchmarks/suites/{suiteID}/comparison`. That read model groups cross-session aggregates by:

- session
- case
- agent
- template variant
- case + agent + template variant

## Parameter Reference

### `centian benchmark run`

- `--suite`
  Required. Path to the benchmark suite root, for example `tests/integrationtests/taskverification/benchmarks/simple_tdd_v1`.
- `--case`
  Optional and repeatable. One or more case ids to run. If omitted, all suite cases run.
- `--agent`
  Required and repeatable. One or more agent ids to execute.
- `--model`, `-m`
  Optional. Model for a single selected agent. Shorthand values: Codex `gpt-5.4`, `gpt-5.4-mini`; Claude `haiku`, `sonnet`, `opus`; Gemini `pro` (`gemini-3.1-pro-preview`), `flash` (`gemini-3-flash-preview`), `2.5-flash` (`gemini-2.5-flash`). `codex-ollama` does not support `--model`; use `--profile`.
- `--profile`
  Optional for single-agent runs and only valid with `--agent codex-ollama`. Selects the Codex profile name from the supplied `--codex-config`.
- `--repeat`
  Optional. Number of attempts per matrix cell. Default: `1`.
- `--template-dir`
  Optional and repeatable. Template variant in `name=path` form. If omitted, the runner first tries `current=<working-dir>/task-templates/integrated`, then falls back to the repo-root path for backwards compatibility.
- `--timeout`
  Optional. Per-run timeout. Default: `15m`.
- `--output-root`
  Optional. Root directory for preserved benchmark artifacts. Default: `.centian/benchmarks` under the current working directory.
- `--centian-config`
  Optional. Base Centian config to copy and patch for benchmark runs. Placeholders such as `__EVENT_STORE_PATH__` and `__TEMPLATES_DIR__` are only resolved if present; otherwise hardcoded paths are preserved.
- `--claude-model`
  Optional. Claude model override for multi-agent runs.
- `--gemini-model`
  Optional. Gemini model override for multi-agent runs.
- `--codex-model`
  Optional. Codex model override for multi-agent runs.
- `--codex-config`
  Optional for `codex`, required for `codex-ollama`. Base Codex config to copy and patch for run-local MCP settings.
- `--keep-centian-running`
  Optional. Print the live UI URL and prompt whether to shut down the run-local Centian server after the agent finishes. This is useful for interactive inspection, but not recommended for unattended matrix runs.

### Session and comparison reads

- Session UI: `/ui/benchmarks/{suiteID}/sessions/{sessionID}`
- Session API: `GET /api/benchmarks/suites/{suiteID}/sessions/{sessionID}`
- Comparison API: `GET /api/benchmarks/suites/{suiteID}/comparison`
- Comparison filters use query params from the benchmark read API, such as `sessionId`, `caseId`, `agent`, `templateVariant`, and `templateId`.

### `make benchmark-simple-tdd`

These variables can be overridden:

- `BENCH_SUITE`
  Suite path used by the make target. Default: `tests/integrationtests/taskverification/benchmarks/simple_tdd_v1`.
- `BENCH_CASE`
  Case id used by the make target. Default: `assertion_failure_red`.
- `BENCH_AGENT`
  Agent used by the make target. Default: `codex`.
- `BENCH_REPEAT`
  Repeat count per matrix cell. Default: `1`.
- `BENCH_OUTPUT_ROOT`
  Preserved benchmark output root. Default: `.centian/benchmarks`.
- `BENCH_TIMEOUT`
  Per-run timeout. Default: `15m`.

Example:

```bash
make benchmark-simple-tdd \
  BENCH_AGENT=claude \
  BENCH_CASE=compile_failure_red \
  BENCH_REPEAT=2 \
  BENCH_TIMEOUT=20m
```

## Scope Of This Directory Today

The current checked-in suite is structural plus runner-ready:

- suite, case, and prompt files are versioned and validated
- fixture directories are real and loadable
- local benchmark execution preserves raw artifacts and run manifests
- local benchmark scoring writes derived scorecards and one session summary
- persistence, API exposure, and UI views are deferred to later tickets
