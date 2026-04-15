# Benchmark CLI Cleanup Plan — `score` and `compare`

Planning document. **No code changes applied yet.**

Target branch: `claude/review-pr-148-MWNIL` (v0.4 feature work). Backward compatibility is out of scope — these CLI subcommands were introduced on this branch and have no external installed-base.

## TL;DR

- `centian benchmark score` and `centian benchmark compare` are **wired correctly today** (they build, tests pass, flags parse).
- Both are **functionally redundant** with the inline scoring + DB read service + UI/API that landed alongside them.
- Estimated removal footprint: ~560 LOC across production + tests, plus small edits to `Makefile` and two docs.

## Why they are redundant

### `benchmark score`

- Flow: load `session.json` from disk → open session-local SQLite → call `QueryService.ScoreSessionManifest` → print JSON.
- Every value it derives is already persisted inline at run finalization (`internal/benchmarks/runner.go:573` writes `BenchmarkRunScoreRecord` into SQLite).
- The read service and UI page (`/ui/benchmarks/:suiteID/sessions/:sessionID`) already surface exactly the same data live from the DB.
- `QueryService.ScoreSessionManifest` (`internal/benchmarks/live_query.go:61`) has **exactly one caller**: `Scorer.ScoreSession` at `internal/benchmarks/scorer.go:274`. If the CLI goes, the method goes with it.

### `benchmark compare`

- Flow: open main event store via `config.ResolveEventStorePath(nil)` → call `QueryService.GetComparison` → filter by `rootPath` prefix → print JSON.
- Equivalent HTTP endpoint already exists: `GET /api/benchmarks/suites/{suiteID}/comparison`. The UI already uses it.
- The only thing the CLI adds over the API is the `rootPath` directory-prefix filter, which is a pre-DB-era artifact — in the DB model every run already belongs to exactly one suite/session.

## Files to remove entirely

| Path | LOC | Reason |
|---|---|---|
| `internal/benchmarks/compare.go` | 168 | Entire file is CLI-only. All public types (`Comparer`, `NewComparer`, `CompareSuite`, `CompareOptions`, `ComparisonSummary`, `ComparisonAggregates`, `ComparisonSession`) and helpers (`firstFilterValue`, `mustAbs`) have no other callers. |
| `internal/benchmarks/compare_test.go` | 133 | Only tests `Comparer.CompareSuite`. |

## Partial edits inside existing files

### `internal/cli/benchmark.go`

Remove:

- `BenchmarkScoreCommand` var declaration
- `BenchmarkCompareCommand` var declaration
- Their entries in `BenchmarkCommand.Commands` (leaves only `BenchmarkRunCommand`)
- `handleBenchmarkScoreCommand`
- `handleBenchmarkCompareCommand`
- `buildBenchmarkScoreOptions`
- `buildBenchmarkCompareOptions`

Keep: `BenchmarkRunCommand` and all of its helpers (`buildBenchmarkRunOptions`, `resolveBenchmarkSuitePath`, `resolveBenchmarkRepeat`, `resolveBenchmarkOutputRoot`, `resolveBenchmarkModelConfigOptions`, `resolveBenchmarkTemplateVariants`, `applyDefaultCodexOllamaModel`, `splitCSVValues`, `parseTemplateVariants`, `defaultResolutionStart`, `defaultBenchmarkSessionLabel`).

Check that `encoding/json` import is still used after removal (it's only used by the removed handlers) — likely needs to be dropped.

### `internal/cli/benchmark_test.go`

- `TestBenchmarkCommandStructure`: update expected subcommand count from **3 → 1** and drop the `BenchmarkScoreCommand` / `BenchmarkCompareCommand` references.
- Remove `TestBenchmarkScoreCommandStructure`.
- Remove `TestBenchmarkCompareCommandStructure`.
- Remove `TestBuildBenchmarkScoreOptionsRequiresSession`.
- Remove `TestBuildBenchmarkScoreOptionsResolvesAbsolutePath`.
- Remove `TestBuildBenchmarkCompareOptionsRequiresRoot`.
- Remove `TestBuildBenchmarkCompareOptionsRequiresSuite`.
- Remove `TestBuildBenchmarkCompareOptionsResolvesFilters`.

### `internal/benchmarks/scorer.go`

Remove (CLI-only surface):

- `Scorer` struct
- `scoreRunContext` struct (only referenced from removed `Scorer` internals — verify with grep before delete)
- `NewScorer`
- `Scorer.ScoreSession`
- `Scorer.withDefaults`
- `eventStorePathForSession`
- `ScoreOptions` struct

Keep (used by read service / API / UI):

- All scorecard/summary types: `RunScorecard`, `ScorecardOutcome`, `ScorecardProcess`, `ScorecardEfficiency`, `ScorecardManual`
- `SessionSummary`, `SessionSummaryAggregates`, `RunSummaryRow`, `AggregateSummary`
- `AgentMetadata`, `AgentUsageMetadata`, `AgentModelUsage`, `ManualScoreInput`
- Helper functions: `buildRunSummaryRow`, `buildAggregates`, `aggregateRows`, `medianFloat`, `rate`, `count`, `filterScoredRows`, `averageManualScore`
- `scoreRunRecord` (inline per-run scoring at runner finalization)
- `loadSessionManifest` — still referenced by `internal/benchmarks/read_service_test.go:165` (after compare removal, `compare_test.go:118` reference goes too)

### `internal/benchmarks/scorer_test.go`

Remove (they all exercise `Scorer.ScoreSession`):

- `TestScoreSessionBuildsLiveSummary`
- `TestScoreSessionUsesPersistedAgentMetadata`
- `TestScoreSessionUsesSharedTaskRunPersistence`
- `TestScoreSessionIgnoresUnrelatedTaskRunsInSharedSQLiteStore`
- `TestScoreSessionRejectsInvalidManualScore`
- `TestScoreSessionRequiresSessionManifest`

Keep: any other tests in that file that exercise the retained helpers directly. Before committing, re-run a grep — everything that touches `NewScorer` / `ScoreSession` / `ScoreOptions` should be gone; everything that touches `buildAggregates` / `aggregateRows` / scorecard types stays.

### `internal/benchmarks/live_query.go`

Remove:

- `ScoreSessionManifest` method (lines 60–… in current tree). Its only caller is `Scorer.ScoreSession`.

Keep everything else — `ErrBenchmarkSessionNotFound`, `ErrBenchmarkRunNotFound`, `ErrBenchmarkComparisonNotFound`, `QueryService`, `NewQueryService`, `GetComparison`, `scoreRunRecord`, etc. remain required by the read service / API.

## External consumers that need coordinated updates

### `Makefile:130`

The `bench-demo-v1` target currently invokes:

```make
./$(BUILD_DIR)/$(BINARY_NAME) benchmark score --session "$$session_dir"
```

Options:

1. **Drop the invocation** — the UI + API already surface the same data, and the benchmark run itself already persists everything.
2. **Replace with a `curl`** against `/api/benchmarks/suites/{suiteID}/sessions/{sessionID}` if a CLI-output step is desired. Probably overkill.

Recommendation: drop the line; the Makefile target still serves its primary purpose (running the benchmark).

### `tests/integrationtests/taskverification/benchmarks/README.md`

References to prune:

- line 83 — `prints a live 'centian benchmark score' view for the newest preserved session`
- line 126 — `./build/centian benchmark score \\` code sample
- line 133 — `./build/centian benchmark compare \\` code sample
- line 225 — `'centian benchmark score' and 'centian benchmark compare' now derive their JSON output live from:` (whole paragraph)
- line 312 — `'centian benchmark score' is the easiest way to compare runs inside one benchmark invocation because it already groups results by:`
- line 319 — `For comparisons across multiple sessions of the same suite, use 'centian benchmark compare'…`
- line 362 — `### 'centian benchmark score'` section header
- line 368 — `### 'centian benchmark compare'` section header

Replace with pointers to the UI/API (`/ui/benchmarks/:suiteID/sessions/:sessionID` and `GET /api/benchmarks/suites/{suiteID}/comparison`).

### `docs/BENCHMARKING.md`

- Lines 129–135 — `Score one preserved session:` section with `centian benchmark score` code block.
- Lines 137–142 — `Compare persisted sessions:` section with `centian benchmark compare` code block.
- API routes section already lists the equivalent endpoints at lines 224 and 227, so no addition is needed.

## Verification checklist (post-removal)

- [ ] `go build ./...`
- [ ] `go vet ./...`
- [ ] `staticcheck -checks U1000 ./...` — should report no new unused symbols
- [ ] `go test ./internal/cli/... ./internal/benchmarks/...`
- [ ] `grep -n 'benchmark score\|benchmark compare' .` — only planned doc/Makefile edits should remain
- [ ] Confirm `encoding/json` import is still needed (or dropped) in `internal/cli/benchmark.go`
- [ ] Confirm `loadSessionManifest` still has a live caller after compare-test removal (should remain from `read_service_test.go:165`)

## Out of scope for this cleanup

- Any changes to `BenchmarkRunCommand` or its flags.
- Any changes to the inline scoring path in `runner.go` or the DB schema.
- Any API / UI changes — both continue to rely on the retained helpers.
