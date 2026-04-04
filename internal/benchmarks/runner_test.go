package benchmarks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/agentrunner"
	"github.com/T4cceptor/centian/internal/persistence"
	"gotest.tools/assert"
)

func TestResolveDefaultPathsFromRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n"), 0o644))
	assert.NilError(t, os.MkdirAll(filepath.Join(repoRoot, "task-templates", "integrated"), 0o755))
	assert.NilError(t, os.MkdirAll(filepath.Join(repoRoot, "tests", "integrationtests", "taskverification"), 0o755))

	variants, err := ResolveDefaultTemplateVariants(repoRoot)
	assert.NilError(t, err)
	assert.Equal(t, len(variants), 1)
	assert.Equal(t, variants[0].Name, "current")

	outputRoot, err := ResolveDefaultOutputRoot(repoRoot)
	assert.NilError(t, err)
	assert.Equal(t, outputRoot, filepath.Join(repoRoot, "tests", "integrationtests", "taskverification", ".tmp", "benchmarks"))
}

func TestRunSuiteExpandsMatrixAndWritesManifests(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	templateA := writeTemplateVariant(t, "a")
	templateB := writeTemplateVariant(t, "b")
	outputRoot := t.TempDir()
	logDir := t.TempDir()
	t.Setenv("CENTIAN_LOG_DIR", logDir)
	launches := 0

	runner := &Runner{
		Now:          fixedClock(),
		AllocatePort: func() (string, error) { return "40123", nil },
		StartCentian: fakeStartCentian,
		LaunchAgent: func(_ context.Context, opts *agentrunner.RunOptions) (*agentrunner.RunResult, error) {
			launches++
			return &agentrunner.RunResult{
				Agent:         opts.Agent,
				ArtifactRoot:  opts.ArtifactRoot,
				WorkspacePath: opts.WorkspacePath,
				StdoutPath:    filepath.Join(opts.ArtifactRoot, "agent.stdout.log"),
				StderrPath:    filepath.Join(opts.ArtifactRoot, "agent.stderr.log"),
			}, nil
		},
		FetchTaskRuns: alternatingTaskRuns([]persistence.TaskRunSummary{{
			RunID:      "tr_123",
			TemplateID: "simple_tdd",
			StartedAt:  100,
			Status:     "completed",
		}}),
		FetchTaskRunEvents: func(string, string) ([]persistence.TaskRunEvent, error) {
			return []persistence.TaskRunEvent{{ID: "evt_1"}}, nil
		},
		FindLatestRequestLog: fakeRequestLogLookup,
	}

	session, err := runner.RunSuite(context.Background(), &RunOptions{
		SuitePath:         suiteRoot,
		Agents:            []string{"codex", "claude"},
		Repeat:            2,
		TemplateVariants:  []TemplateVariant{{Name: "a", SourceDir: templateA}, {Name: "b", SourceDir: templateB}},
		OutputRoot:        outputRoot,
		Timeout:           time.Minute,
		CentianBinaryPath: "/tmp/centian",
	})
	assert.NilError(t, err)
	assert.Equal(t, session.Status, "completed")
	assert.Equal(t, len(session.Runs), 24)
	assert.Equal(t, launches, 24)

	runPath := filepath.Join(session.InvocationDir, "runs", "a", "codex", "compile_failure_red", "attempt-001", runFileName)
	data, err := os.ReadFile(runPath)
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(data), `"latestTaskRunId": "tr_123"`))
	assert.Assert(t, strings.Contains(string(data), `"eventStoreMode": "configured_shared"`))
	assert.Assert(t, strings.Contains(string(data), filepath.Join(logDir, "events.sqlite")))

	store, err := persistence.NewSQLiteStore(filepath.Join(logDir, "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	runRecords, err := store.ListBenchmarkArtifacts(context.Background(), persistence.BenchmarkArtifactFilter{
		SuiteID:      "simple_tdd_v1",
		ArtifactKind: persistence.BenchmarkArtifactKindRun,
	})
	assert.NilError(t, err)
	sessionRecords, err := store.ListBenchmarkArtifacts(context.Background(), persistence.BenchmarkArtifactFilter{
		SuiteID:      "simple_tdd_v1",
		ArtifactKind: persistence.BenchmarkArtifactKindSession,
	})
	assert.NilError(t, err)
	assert.Equal(t, len(runRecords), 24)
	assert.Equal(t, len(sessionRecords), 1)
}

func TestRunSuiteContinuesAfterFailure(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	templateDir := writeTemplateVariant(t, "current")
	outputRoot := t.TempDir()
	t.Setenv("CENTIAN_LOG_DIR", t.TempDir())
	launches := 0

	runner := &Runner{
		Now:          fixedClock(),
		AllocatePort: func() (string, error) { return "40123", nil },
		StartCentian: fakeStartCentian,
		LaunchAgent: func(_ context.Context, opts *agentrunner.RunOptions) (*agentrunner.RunResult, error) {
			launches++
			if strings.Contains(opts.ArtifactRoot, "compile_failure_red") {
				return nil, errors.New("agent failed")
			}
			return &agentrunner.RunResult{Agent: opts.Agent}, nil
		},
		FetchTaskRuns: alternatingTaskRuns([]persistence.TaskRunSummary{{
			RunID:      "tr_123",
			TemplateID: "simple_tdd",
			StartedAt:  100,
			Status:     "completed",
		}}),
		FetchTaskRunEvents: func(string, string) ([]persistence.TaskRunEvent, error) {
			return []persistence.TaskRunEvent{{ID: "evt_1"}}, nil
		},
		FindLatestRequestLog: fakeRequestLogLookup,
	}

	session, err := runner.RunSuite(context.Background(), &RunOptions{
		SuitePath:         suiteRoot,
		CaseIDs:           []string{"compile_failure_red", "assertion_failure_red"},
		Agents:            []string{"codex"},
		Repeat:            1,
		TemplateVariants:  []TemplateVariant{{Name: "current", SourceDir: templateDir}},
		OutputRoot:        outputRoot,
		Timeout:           time.Minute,
		CentianBinaryPath: "/tmp/centian",
	})
	assert.Assert(t, err != nil)
	assert.Equal(t, launches, 2)
	assert.Equal(t, session.Status, "failed")

	secondRunPath := filepath.Join(session.InvocationDir, "runs", "current", "codex", "assertion_failure_red", "attempt-001", runFileName)
	_, statErr := os.Stat(secondRunPath)
	assert.NilError(t, statErr)
}

func TestRunSuiteCapturesOnlyTaskRunsCreatedDuringCurrentCell(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	templateDir := writeTemplateVariant(t, "current")
	outputRoot := t.TempDir()
	t.Setenv("CENTIAN_LOG_DIR", t.TempDir())
	fetchCalls := 0

	runner := &Runner{
		Now:          fixedClock(),
		AllocatePort: func() (string, error) { return "40123", nil },
		StartCentian: fakeStartCentian,
		LaunchAgent: func(_ context.Context, opts *agentrunner.RunOptions) (*agentrunner.RunResult, error) {
			return &agentrunner.RunResult{Agent: opts.Agent}, nil
		},
		FetchTaskRuns: func(string) ([]persistence.TaskRunSummary, error) {
			fetchCalls++
			if fetchCalls == 1 {
				return []persistence.TaskRunSummary{{
					RunID:      "tr_old",
					TemplateID: "simple_tdd",
					StartedAt:  100,
					Status:     "completed",
				}}, nil
			}
			return []persistence.TaskRunSummary{
				{
					RunID:      "tr_old",
					TemplateID: "simple_tdd",
					StartedAt:  100,
					Status:     "completed",
				},
				{
					RunID:      "tr_new",
					TemplateID: "simple_tdd",
					StartedAt:  200,
					Status:     "completed",
				},
			}, nil
		},
		FetchTaskRunEvents: func(_ string, runID string) ([]persistence.TaskRunEvent, error) {
			return []persistence.TaskRunEvent{{ID: "evt_" + runID}}, nil
		},
		FindLatestRequestLog: fakeRequestLogLookup,
	}

	session, err := runner.RunSuite(context.Background(), &RunOptions{
		SuitePath:         suiteRoot,
		CaseIDs:           []string{"compile_failure_red"},
		Agents:            []string{"codex"},
		Repeat:            1,
		TemplateVariants:  []TemplateVariant{{Name: "current", SourceDir: templateDir}},
		OutputRoot:        outputRoot,
		Timeout:           time.Minute,
		CentianBinaryPath: "/tmp/centian",
	})
	assert.NilError(t, err)
	assert.Equal(t, fetchCalls, 2)

	runPath := filepath.Join(session.InvocationDir, "runs", "current", "codex", "compile_failure_red", "attempt-001", runFileName)
	var manifest RunManifest
	assert.NilError(t, readJSONFile(runPath, &manifest))
	assert.DeepEqual(t, manifest.LinkedTaskRunIDs, []string{"tr_new"})
	assert.Equal(t, manifest.LatestTaskRunID, "tr_new")
}

func TestRunSuiteRejectsUnknownCase(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	templateDir := writeTemplateVariant(t, "current")
	t.Setenv("CENTIAN_LOG_DIR", t.TempDir())

	runner := NewRunner()
	_, err := runner.RunSuite(context.Background(), &RunOptions{
		SuitePath:         suiteRoot,
		CaseIDs:           []string{"missing"},
		Agents:            []string{"codex"},
		Repeat:            1,
		TemplateVariants:  []TemplateVariant{{Name: "current", SourceDir: templateDir}},
		OutputRoot:        t.TempDir(),
		Timeout:           time.Minute,
		CentianBinaryPath: "/tmp/centian",
	})
	assert.ErrorContains(t, err, `unknown case "missing"`)
}

func TestRunSuiteRejectsUnsupportedResetMode(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	templateDir := writeTemplateVariant(t, "current")
	t.Setenv("CENTIAN_LOG_DIR", t.TempDir())
	casePath := filepath.Join(suiteRoot, "cases", "compile_failure_red", caseFileName)
	mustWriteFile(t, casePath, `
version: "0.1"
case:
  id: "compile_failure_red"
  name: "Compile-failure red baseline"
  description: "Authored red baseline benchmark case."
promptFile: "prompt.yaml"
fixture:
  seedPath: "fixture"
  resetMode: "git_checkout"
  startingRepoStateSummary: "Authored red baseline already exists."
expectations:
  selectedCommand: "go test ./internal/example -run TestExample"
  redSignal:
    type: "output_contains"
    value: "undefined: HealthStatus"
  greenConditionSummary: "Selected command exits 0 without editing the locked baseline file."
constraints:
  lockedPaths:
    - "internal/example/example_test.go"
`)

	runner := &Runner{
		Now:          fixedClock(),
		AllocatePort: func() (string, error) { return "40123", nil },
		StartCentian: fakeStartCentian,
		LaunchAgent: func(context.Context, *agentrunner.RunOptions) (*agentrunner.RunResult, error) {
			return &agentrunner.RunResult{}, nil
		},
		FetchTaskRuns:        func(string) ([]persistence.TaskRunSummary, error) { return nil, nil },
		FetchTaskRunEvents:   func(string, string) ([]persistence.TaskRunEvent, error) { return nil, nil },
		FindLatestRequestLog: fakeRequestLogLookup,
	}

	session, err := runner.RunSuite(context.Background(), &RunOptions{
		SuitePath:         suiteRoot,
		CaseIDs:           []string{"compile_failure_red"},
		Agents:            []string{"codex"},
		Repeat:            1,
		TemplateVariants:  []TemplateVariant{{Name: "current", SourceDir: templateDir}},
		OutputRoot:        t.TempDir(),
		Timeout:           time.Minute,
		CentianBinaryPath: "/tmp/centian",
	})
	assert.Assert(t, err != nil)
	assert.Equal(t, session.Status, "failed")
	assert.Equal(t, session.Runs[0].ErrorSummary, `unsupported fixture reset mode "git_checkout"`)
}

func TestRunSuiteKeepsRunFileWhenBenchmarkPersistenceFails(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	templateDir := writeTemplateVariant(t, "current")
	t.Setenv("CENTIAN_LOG_DIR", t.TempDir())

	runner := &Runner{
		Now:          fixedClock(),
		AllocatePort: func() (string, error) { return "40123", nil },
		StartCentian: fakeStartCentian,
		LaunchAgent: func(_ context.Context, opts *agentrunner.RunOptions) (*agentrunner.RunResult, error) {
			return &agentrunner.RunResult{Agent: opts.Agent}, nil
		},
		FetchTaskRuns: alternatingTaskRuns([]persistence.TaskRunSummary{{
			RunID:      "tr_123",
			TemplateID: "simple_tdd",
			StartedAt:  100,
			Status:     "completed",
		}}),
		FetchTaskRunEvents: func(string, string) ([]persistence.TaskRunEvent, error) {
			return []persistence.TaskRunEvent{{ID: "evt_1"}}, nil
		},
		FindLatestRequestLog: fakeRequestLogLookup,
		PersistArtifact: func(context.Context, string, *persistence.BenchmarkArtifactRecord) error {
			return errors.New("persist failed")
		},
	}

	session, err := runner.RunSuite(context.Background(), &RunOptions{
		SuitePath:         suiteRoot,
		CaseIDs:           []string{"compile_failure_red"},
		Agents:            []string{"codex"},
		Repeat:            1,
		TemplateVariants:  []TemplateVariant{{Name: "current", SourceDir: templateDir}},
		OutputRoot:        t.TempDir(),
		Timeout:           time.Minute,
		CentianBinaryPath: "/tmp/centian",
	})
	assert.Assert(t, err != nil)
	runPath := filepath.Join(session.InvocationDir, "runs", "current", "codex", "compile_failure_red", "attempt-001", runFileName)
	_, statErr := os.Stat(runPath)
	assert.NilError(t, statErr)
}

func fakeStartCentian(_ context.Context, _ StartCentianOptions) (*StartedCentian, error) {
	return &StartedCentian{Stop: func() error { return nil }}, nil
}

func fakeRequestLogLookup(logDir string) (string, error) {
	path := filepath.Join(logDir, "requests_0001.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func fixedClock() func() time.Time {
	current := time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		current = current.Add(time.Second)
		return current
	}
}

func sequentialTaskRuns(responses [][]persistence.TaskRunSummary) func(string) ([]persistence.TaskRunSummary, error) {
	index := 0
	return func(string) ([]persistence.TaskRunSummary, error) {
		if index >= len(responses) {
			return nil, nil
		}
		current := responses[index]
		index++
		return current, nil
	}
}

func alternatingTaskRuns(result []persistence.TaskRunSummary) func(string) ([]persistence.TaskRunSummary, error) {
	call := 0
	return func(string) ([]persistence.TaskRunSummary, error) {
		call++
		if call%2 == 1 {
			return nil, nil
		}
		return result, nil
	}
}

func writeTemplateVariant(t *testing.T, name string) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), name)
	assert.NilError(t, os.MkdirAll(root, 0o755))
	assert.NilError(t, os.WriteFile(filepath.Join(root, "simple_tdd.yaml"), []byte("version: \"0.1\"\n"), 0o644))
	return root
}
