# Benchmark Artifacts

This directory contains repo-tracked benchmark definitions for Centian taskverification.

Benchmarks live beside the existing taskverification integration fixtures because later benchmark runners are expected to reuse the same host-native black-box infrastructure.

## Artifact Layout

Each suite uses the same generic structure:

- `suite.yaml`: suite metadata, targeted template, and case references
- `cases/<case-id>/case.yaml`: one benchmark case contract
- `cases/<case-id>/prompt.yaml`: the user-style prompt for the case
- `cases/<case-id>/fixture/`: the seeded authored-red starting state

The checked-in files under this directory are inputs. They define what should be benchmarked, not the artifacts of one concrete run.

## Preserved Local Outputs

The benchmark runner preserves local outputs under the existing taskverification `.tmp` tree. One benchmark CLI invocation creates an invocation directory:

```text
tests/integrationtests/taskverification/.tmp/benchmarks/<suite-id>/<timestamp>_<label>/
```

That directory contains:

- `session.json`
- `runs/<template-variant>/<agent>/<case-id>/attempt-001/`

Each preserved run contains:

- `project/`
- `templates/`
- `logs/`
- `agent/`
- `run.json`

`logs/` stores Centian-side runtime artifacts such as the event store, request logs, and task-run snapshots. `agent/` stores agent logs and agent-specific artifacts for the run. Scorecards are intentionally deferred to later tickets.

## Scope Of This Directory Today

The current checked-in suite is structural plus runner-ready:

- suite, case, and prompt files are versioned and validated
- fixture directories are real and loadable
- local benchmark execution preserves raw artifacts and run manifests
- scoring, persistence, API exposure, and UI views are deferred to later tickets
