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

## Future Preserved Outputs

Later runner work should preserve local benchmark outputs under the existing taskverification `.tmp` tree, for example:

```text
tests/integrationtests/taskverification/.tmp/benchmarks/<suite-id>/<timestamp>_<agent>_<template-variant>/
```

Each preserved case run is expected to reserve subpaths such as:

- `project/`
- `templates/`
- `logs/`
- `agent/`
- `run.json`
- `scorecard.json` (future)

That output layout is documented here so later runner work can reuse a stable artifact convention instead of inventing one ad hoc.

## Scope Of This Directory Today

The current checked-in suite is structural only:

- suite, case, and prompt files are versioned and validated
- fixture directories are real and loadable
- benchmark execution, scoring, persistence, API exposure, and UI views are deferred to later tickets
