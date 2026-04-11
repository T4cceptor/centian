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

func TestResolveDefaultPathsFromWorkingDirectory(t *testing.T) {
	workingDir := t.TempDir()
	assert.NilError(t, os.MkdirAll(filepath.Join(workingDir, "task-templates", "integrated"), 0o755))

	variants, err := ResolveDefaultTemplateVariants(workingDir)
	assert.NilError(t, err)
	assert.Equal(t, len(variants), 1)
	assert.Equal(t, variants[0].Name, "current")
	assert.Equal(t, variants[0].SourceDir, filepath.Join(workingDir, "task-templates", "integrated"))

	outputRoot, err := ResolveDefaultOutputRoot(workingDir)
	assert.NilError(t, err)
	assert.Equal(t, outputRoot, filepath.Join(workingDir, ".centian", "benchmarks"))
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

	runPath := filepath.Join(session.InvocationDir, "runs", "a_codex_compile_failure_red_attempt_001", runFileName)
	data, err := os.ReadFile(runPath)
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(data), `"latestTaskRunId": "tr_123"`))
	assert.Assert(t, strings.Contains(string(data), `"eventStoreMode": "configured_shared"`))
	assert.Assert(t, strings.Contains(string(data), filepath.Join(logDir, "events.sqlite")))

	store, err := persistence.NewSQLiteStore(filepath.Join(logDir, "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	runRecords, err := store.ListBenchmarkRuns(context.Background(), &persistence.BenchmarkRunFilter{
		SuiteID: "simple_tdd_v1",
	})
	assert.NilError(t, err)
	scoreRecords, err := store.ListBenchmarkRunScores(context.Background())
	assert.NilError(t, err)
	sessionRecords, err := store.ListBenchmarkSessions(context.Background(), persistence.BenchmarkSessionFilter{
		SuiteID: "simple_tdd_v1",
	})
	assert.NilError(t, err)
	assert.Equal(t, len(runRecords), 24)
	assert.Equal(t, len(scoreRecords), 24)
	assert.Equal(t, len(sessionRecords), 1)
	_, statErr := os.Stat(filepath.Join(session.InvocationDir, "runs", "a_codex_compile_failure_red_attempt_001", "selected-template.yaml"))
	assert.NilError(t, statErr)
	_, statErr = os.Stat(filepath.Join(session.InvocationDir, "runs", "a_codex_compile_failure_red_attempt_001", "templates"))
	assert.Assert(t, os.IsNotExist(statErr))
	assert.Assert(t, strings.Contains(filepath.Base(session.InvocationDir), "_run"))
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

	secondRunPath := filepath.Join(session.InvocationDir, "runs", "current_codex_assertion_failure_red_attempt_001", runFileName)
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

	runPath := filepath.Join(session.InvocationDir, "runs", "current_codex_compile_failure_red_attempt_001", runFileName)
	var manifest RunManifest
	assert.NilError(t, readJSONFile(runPath, &manifest))
	assert.DeepEqual(t, manifest.LinkedTaskRunIDs, []string{"tr_new"})
	assert.Equal(t, manifest.LatestTaskRunID, "tr_new")
}

func TestRunSuitePersistsCodexOllamaAgentMetadata(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	templateDir := writeTemplateVariant(t, "current")
	outputRoot := t.TempDir()
	t.Setenv("CENTIAN_LOG_DIR", t.TempDir())

	runner := &Runner{
		Now:          fixedClock(),
		AllocatePort: func() (string, error) { return "40123", nil },
		StartCentian: fakeStartCentian,
		LaunchAgent: func(_ context.Context, opts *agentrunner.RunOptions) (*agentrunner.RunResult, error) {
			return &agentrunner.RunResult{
				Agent:         opts.Agent,
				SelectedModel: "gemma4",
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
		CaseIDs:           []string{"compile_failure_red"},
		Agents:            []string{agentrunner.AgentCodexOllama},
		Repeat:            1,
		TemplateVariants:  []TemplateVariant{{Name: "current", SourceDir: templateDir}},
		OutputRoot:        outputRoot,
		Timeout:           time.Minute,
		CentianBinaryPath: "/tmp/centian",
		Models:            AgentModels{CodexOllama: "gemma4"},
	})
	assert.NilError(t, err)

	runPath := filepath.Join(session.InvocationDir, "runs", "current_codex_ollama_compile_failure_red_attempt_001", runFileName)
	var manifest RunManifest
	assert.NilError(t, readJSONFile(runPath, &manifest))
	assert.Equal(t, manifest.AgentID, agentrunner.AgentCodexOllama)
	assert.Equal(t, manifest.SelectedModel, "gemma4")
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
		PersistSession:       func(context.Context, string, *persistence.BenchmarkSessionRecord) error { return nil },
		PersistRun: func(context.Context, string, *persistence.BenchmarkRunRecord) error {
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
	runPath := filepath.Join(session.InvocationDir, "runs", "current_codex_compile_failure_red_attempt_001", runFileName)
	_, statErr := os.Stat(runPath)
	assert.NilError(t, statErr)
}

func TestRunSuitePersistsSessionToConfiguredEventStore(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	templateDir := writeTemplateVariant(t, "current")
	outputRoot := t.TempDir()
	customStorePath := filepath.Join(t.TempDir(), "custom-events.sqlite")
	centianConfigPath := filepath.Join(t.TempDir(), "centian.config.json")
	t.Setenv("CENTIAN_LOG_DIR", t.TempDir())
	mustWriteFile(t, centianConfigPath, `{
  "name": "Benchmark Config",
  "version": "1.0.0",
  "auth": false,
  "proxy": {
    "host": "127.0.0.1",
    "port": "__PORT__",
    "logFile": "__INTERNAL_LOG__",
    "capabilities": {
      "taskVerification": {
        "enabled": true,
        "templatesPath": "__TEMPLATES_DIR__"
      },
      "eventStorage": {
        "enabled": true,
        "driver": "sqlite",
        "path": "`+customStorePath+`"
      }
    }
  },
  "gateways": {
    "taskverification": {
      "mcpServers": {}
    }
  }
}`)

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
		CentianConfigPath: centianConfigPath,
	})
	assert.NilError(t, err)

	store, err := persistence.NewSQLiteStore(customStorePath)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	sessionRecords, err := store.ListBenchmarkSessions(context.Background(), persistence.BenchmarkSessionFilter{
		SuiteID: "simple_tdd_v1",
	})
	assert.NilError(t, err)
	runRecords, err := store.ListBenchmarkRuns(context.Background(), &persistence.BenchmarkRunFilter{
		SuiteID: "simple_tdd_v1",
	})
	assert.NilError(t, err)
	assert.Equal(t, len(sessionRecords), 1)
	assert.Equal(t, sessionRecords[0].SessionID, benchmarkSessionID(session.SuiteID, session.InvocationDir))
	assert.Equal(t, len(runRecords), 1)
	assert.Equal(t, runRecords[0].EventStorePath, customStorePath)
}

func TestRenderCentianConfigPreservesHardcodedPaths(t *testing.T) {
	templateDir := filepath.Join(t.TempDir(), "templates")
	customStorePath := filepath.Join(t.TempDir(), "custom-events.sqlite")
	baseConfigPath := filepath.Join(t.TempDir(), "centian.config.json")
	mustWriteFile(t, baseConfigPath, `{
  "name": "Benchmark Config",
  "version": "1.0.0",
  "auth": false,
  "proxy": {
    "host": "127.0.0.1",
    "port": "__PORT__",
    "logFile": "__INTERNAL_LOG__",
    "capabilities": {
      "taskVerification": {
        "enabled": true,
        "templatesPath": "`+templateDir+`"
      },
      "eventStorage": {
        "enabled": true,
        "driver": "sqlite",
        "path": "`+customStorePath+`"
      }
    }
  },
  "gateways": {
    "taskverification": {
      "mcpServers": {}
    }
  }
}`)

	rendered, err := renderCentianConfig(
		baseConfigPath,
		"/tmp/runtime-templates",
		"/tmp/project",
		"/tmp/internal.log",
		"/tmp/default-events.sqlite",
		"40123",
	)
	assert.NilError(t, err)
	assert.Equal(t, rendered.EffectiveEventStorePath, customStorePath)
	assert.Assert(t, strings.Contains(string(rendered.Content), `"templatesPath": "`+templateDir+`"`))
	assert.Assert(t, strings.Contains(string(rendered.Content), `"path": "`+customStorePath+`"`))
	assert.Assert(t, strings.Contains(string(rendered.Content), `"port": "40123"`))
	assert.Assert(t, strings.Contains(string(rendered.Content), `"logFile": "/tmp/internal.log"`))
	assert.Assert(t, !strings.Contains(string(rendered.Content), "/tmp/runtime-templates"))
	assert.Assert(t, !strings.Contains(string(rendered.Content), "/tmp/default-events.sqlite"))
}

func TestRenderCentianConfigResolvesRelativeEventStorePathAgainstProjectDir(t *testing.T) {
	baseConfigPath := filepath.Join(t.TempDir(), "centian.config.json")
	mustWriteFile(t, baseConfigPath, `{
  "name": "Benchmark Config",
  "version": "1.0.0",
  "auth": false,
  "proxy": {
    "host": "127.0.0.1",
    "port": "__PORT__",
    "capabilities": {
      "taskVerification": {
        "enabled": true,
        "templatesPath": "__TEMPLATES_DIR__"
      },
      "eventStorage": {
        "enabled": true,
        "driver": "sqlite",
        "path": "relative/events.sqlite"
      }
    }
  },
  "gateways": {
    "taskverification": {
      "mcpServers": {}
    }
  }
}`)

	rendered, err := renderCentianConfig(
		baseConfigPath,
		"/tmp/runtime-templates",
		"/tmp/project",
		"/tmp/internal.log",
		"/tmp/default-events.sqlite",
		"40123",
	)
	assert.NilError(t, err)
	expectedStorePath := filepath.Join(filepath.Dir(baseConfigPath), "relative", "events.sqlite")
	assert.Equal(t, rendered.EffectiveEventStorePath, expectedStorePath)
	assert.Assert(t, strings.Contains(string(rendered.Content), `"path": "`+expectedStorePath+`"`))
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
	assert.NilError(t, os.WriteFile(filepath.Join(root, "simple_tdd.yaml"), []byte(`
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD `+name+`"
  description: "Test template"
workflow:
  onboarding:
    instructions: "Collect context"
  planning:
    instructions: "Plan"
    next: "execution.implement_fix"
  execution:
    - id: "implement_fix"
      instructions: "Implement fix"
      next: ""
`), 0o644))
	return root
}
