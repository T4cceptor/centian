package benchmarks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/persistence"
	"gotest.tools/assert"
)

func TestScoreSessionWritesScorecardsAndSummary(t *testing.T) {
	sessionDir := writeSyntheticScoringSession(t, syntheticSessionOptions{
		includeInvariantViolation: true,
	})

	scorer := &Scorer{Now: func() time.Time { return time.Date(2026, 4, 4, 15, 0, 0, 0, time.UTC) }}
	summary, err := scorer.ScoreSession(context.Background(), &ScoreOptions{SessionPath: sessionDir})
	assert.NilError(t, err)

	assert.Equal(t, summary.RunCount, 2)
	assert.Equal(t, summary.ScoredRunCount, 2)
	assert.Equal(t, summary.FailedToScoreCount, 0)
	assert.Equal(t, len(summary.Aggregates.ByCase), 2)
	assert.Equal(t, len(summary.Aggregates.ByAgent), 1)
	assert.Equal(t, len(summary.Aggregates.ByTemplateVariant), 1)
	assert.Equal(t, len(summary.Aggregates.ByCaseAgentVariant), 2)

	firstRunScorecard := loadScorecard(t, filepath.Join(sessionDir, "runs", "current", "codex", "compile_failure_red", "attempt-001", scorecardFileName))
	assert.Equal(t, firstRunScorecard.Outcome.FirstPassSuccess, true)
	assert.Equal(t, firstRunScorecard.Outcome.InvariantViolation, false)
	assert.Equal(t, firstRunScorecard.Process.FailedTaskToolCalls, 1)
	assert.Equal(t, firstRunScorecard.Process.FailedDownstreamToolCalls, 1)
	assert.Equal(t, firstRunScorecard.Process.TotalStepRetries, 1)
	assert.Equal(t, firstRunScorecard.Process.ReplanningCount, 1)
	assert.Assert(t, firstRunScorecard.Process.RecoveryTimeSeconds != nil)
	assert.Assert(t, *firstRunScorecard.Process.RecoveryTimeSeconds > 0)
	assert.Assert(t, firstRunScorecard.Process.RecoveryToolCalls != nil)
	assert.Equal(t, *firstRunScorecard.Process.RecoveryToolCalls, 1)
	assert.Equal(t, firstRunScorecard.Efficiency.ObservedCommandCalls, 1)
	assert.Equal(t, firstRunScorecard.Efficiency.EditedFilesCount, 1)
	assert.DeepEqual(t, firstRunScorecard.Efficiency.EditedFiles, []string{"internal/health/health.go"})
	assert.Assert(t, firstRunScorecard.Manual.ErrorActionabilityScore != nil)
	assert.Equal(t, *firstRunScorecard.Manual.ErrorActionabilityScore, 3)

	secondRunScorecard := loadScorecard(t, filepath.Join(sessionDir, "runs", "current", "codex", "assertion_failure_red", "attempt-001", scorecardFileName))
	assert.Equal(t, secondRunScorecard.Outcome.FirstPassSuccess, false)
	assert.Equal(t, secondRunScorecard.Outcome.InvariantViolation, true)
	assert.Equal(t, secondRunScorecard.Outcome.RestartOccurred, true)
}

func TestScoreSessionRejectsInvalidManualScoreButStillWritesSummary(t *testing.T) {
	sessionDir := writeSyntheticScoringSession(t, syntheticSessionOptions{
		invalidManualScoreForCase: "assertion_failure_red",
	})

	scorer := &Scorer{Now: func() time.Time { return time.Date(2026, 4, 4, 16, 0, 0, 0, time.UTC) }}
	summary, err := scorer.ScoreSession(context.Background(), &ScoreOptions{SessionPath: sessionDir})
	assert.ErrorContains(t, err, "failed to score 1 benchmark run(s)")

	assert.Equal(t, summary.RunCount, 2)
	assert.Equal(t, summary.ScoredRunCount, 1)
	assert.Equal(t, summary.FailedToScoreCount, 1)

	_, statErr := os.Stat(filepath.Join(sessionDir, "runs", "current", "codex", "compile_failure_red", "attempt-001", scorecardFileName))
	assert.NilError(t, statErr)
	_, statErr = os.Stat(filepath.Join(sessionDir, summaryFileName))
	assert.NilError(t, statErr)
	_, statErr = os.Stat(filepath.Join(sessionDir, "runs", "current", "codex", "assertion_failure_red", "attempt-001", scorecardFileName))
	assert.Assert(t, os.IsNotExist(statErr))
}

func TestScoreSessionRequiresSessionManifest(t *testing.T) {
	scorer := NewScorer()
	_, err := scorer.ScoreSession(context.Background(), &ScoreOptions{SessionPath: t.TempDir()})
	assert.ErrorContains(t, err, "load session manifest")
}

type syntheticSessionOptions struct {
	includeInvariantViolation bool
	invalidManualScoreForCase string
}

func writeSyntheticScoringSession(t *testing.T, opts syntheticSessionOptions) string {
	t.Helper()

	suiteRoot := checkedInSimpleTDDSuiteRoot(t)
	sessionDir := filepath.Join(t.TempDir(), "session")
	assert.NilError(t, os.MkdirAll(sessionDir, 0o755))

	runs := []SessionRunManifestEntry{
		{
			CaseID:          "compile_failure_red",
			AgentID:         "codex",
			TemplateVariant: "current",
			Attempt:         1,
			RelativeRunDir:  filepath.Join("runs", "current", "codex", "compile_failure_red", "attempt-001"),
			Status:          "completed",
			LatestTaskRunID: "tr_compile",
		},
		{
			CaseID:          "assertion_failure_red",
			AgentID:         "codex",
			TemplateVariant: "current",
			Attempt:         1,
			RelativeRunDir:  filepath.Join("runs", "current", "codex", "assertion_failure_red", "attempt-001"),
			Status:          "completed",
			LatestTaskRunID: "tr_assert",
		},
	}

	for _, entry := range runs {
		writeSyntheticRun(t, sessionDir, suiteRoot, entry, opts)
	}

	session := &SessionManifest{
		SuiteID:       "simple_tdd_v1",
		TemplateID:    "simple_tdd",
		SuitePath:     suiteRoot,
		InvocationDir: sessionDir,
		OutputRoot:    filepath.Dir(sessionDir),
		StartedAt:     time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
		EndedAt:       time.Date(2026, 4, 4, 12, 10, 0, 0, time.UTC),
		Status:        "completed",
		Repeat:        1,
		Agents:        []string{"codex"},
		CaseIDs:       []string{"compile_failure_red", "assertion_failure_red"},
		TemplateVariants: []TemplateVariant{{
			Name: "current", SourceDir: filepath.Join(t.TempDir(), "templates"),
		}},
		Runs: runs,
	}
	assert.NilError(t, writeJSONFile(filepath.Join(sessionDir, sessionFileName), session))
	return sessionDir
}

func writeSyntheticRun(t *testing.T, sessionDir string, suiteRoot string, entry SessionRunManifestEntry, opts syntheticSessionOptions) {
	t.Helper()

	caseDef, err := LoadCase(suiteRoot, SuiteCaseRef{ID: entry.CaseID, Path: filepath.Join("cases", entry.CaseID)})
	assert.NilError(t, err)
	caseRoot := filepath.Join(suiteRoot, "cases", entry.CaseID)
	fixtureRoot := filepath.Join(caseRoot, caseDef.Fixture.SeedPath)

	runDir := filepath.Join(sessionDir, entry.RelativeRunDir)
	projectDir := filepath.Join(runDir, "project")
	templatesDir := filepath.Join(runDir, "templates")
	logsDir := filepath.Join(runDir, "logs")
	agentDir := filepath.Join(runDir, "agent")
	assert.NilError(t, os.MkdirAll(templatesDir, 0o755))
	assert.NilError(t, os.MkdirAll(logsDir, 0o755))
	assert.NilError(t, os.MkdirAll(agentDir, 0o755))
	assert.NilError(t, copyDir(fixtureRoot, projectDir))

	switch entry.CaseID {
	case "compile_failure_red":
		assert.NilError(t, os.WriteFile(filepath.Join(projectDir, "internal", "health", "health.go"), []byte("package health\n\nfunc Status() string { return \"ready\" }\n"), 0o644))
	case "assertion_failure_red":
		assert.NilError(t, os.WriteFile(filepath.Join(projectDir, "internal", "health", "health.go"), []byte("package health\n\nfunc Status() string { return \"ready\" }\n"), 0o644))
		if opts.includeInvariantViolation {
			assert.NilError(t, os.WriteFile(filepath.Join(projectDir, "internal", "health", "health_test.go"), []byte("package health\n"), 0o644))
		}
	}

	taskRunsPath := filepath.Join(logsDir, "task_runs.json")
	taskRunEventsDir := filepath.Join(logsDir, "task_run_events")
	assert.NilError(t, os.MkdirAll(taskRunEventsDir, 0o755))
	requestLogPath := filepath.Join(logsDir, "requests_0001.jsonl")
	assert.NilError(t, os.WriteFile(requestLogPath, []byte("{}\n"), 0o644))

	taskRunID := entry.LatestTaskRunID
	taskRuns := []persistence.TaskRunSummary{{
		RunID:      taskRunID,
		TemplateID: "simple_tdd",
		StartedAt:  1_000,
		EndedAt:    int64Ptr(3_000),
		Status:     "completed",
	}}
	assert.NilError(t, writeJSONFile(taskRunsPath, taskRuns))

	events := syntheticEventsForCase(entry.CaseID)
	assert.NilError(t, writeJSONFile(filepath.Join(taskRunEventsDir, taskRunID+".json"), events))

	if entry.CaseID == opts.invalidManualScoreForCase {
		assert.NilError(t, os.WriteFile(filepath.Join(runDir, manualScoreFileName), []byte(`{"errorActionabilityScore":9}`), 0o644))
	} else {
		assert.NilError(t, os.WriteFile(filepath.Join(runDir, manualScoreFileName), []byte(`{"errorActionabilityScore":3,"notes":"Actionable"}`), 0o644))
	}

	run := &RunManifest{
		SuiteID:             "simple_tdd_v1",
		CaseID:              entry.CaseID,
		TemplateID:          "simple_tdd",
		TemplateVariant:     TemplateVariant{Name: "current", SourceDir: templatesDir},
		AgentID:             entry.AgentID,
		StartedAt:           time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
		EndedAt:             time.Date(2026, 4, 4, 12, 0, 5, 0, time.UTC),
		Status:              "completed",
		LatestTaskRunID:     taskRunID,
		LatestTaskRunStatus: "completed",
		LinkedTaskRunIDs:    []string{taskRunID},
		ArtifactPaths: RunArtifactPaths{
			RunDir:           runDir,
			ProjectDir:       projectDir,
			TemplatesDir:     templatesDir,
			LogsDir:          logsDir,
			AgentDir:         agentDir,
			ConfigPath:       filepath.Join(runDir, "centian.config.json"),
			EventStorePath:   filepath.Join(logsDir, "events.sqlite"),
			RequestLogPath:   requestLogPath,
			TaskRunsSnapshot: taskRunsPath,
			TaskRunEventsDir: taskRunEventsDir,
		},
	}
	assert.NilError(t, writeJSONFile(filepath.Join(runDir, runFileName), run))
}

func syntheticEventsForCase(caseID string) []persistence.TaskRunEvent {
	events := []persistence.TaskRunEvent{
		{
			Source:             persistence.TaskRunEventSourceTask,
			ID:                 "task-registered",
			CreatedAtUnixMilli: 1_000,
			EventType:          "task_registered",
		},
		{
			Source:             persistence.TaskRunEventSourceTask,
			ID:                 "task-planning-1",
			CreatedAtUnixMilli: 1_100,
			EventType:          "planning_completed",
			PayloadJSON:        json.RawMessage(`{"status":"active"}`),
		},
		{
			Source:             persistence.TaskRunEventSourceAction,
			ID:                 "action-failed-downstream",
			CreatedAtUnixMilli: 1_200,
			ToolName:           "filesystem___read_text_file",
			IsError:            boolPtr(true),
			Success:            boolPtr(false),
		},
		{
			Source:             persistence.TaskRunEventSourceAction,
			ID:                 "action-recovery",
			CreatedAtUnixMilli: 1_250,
			ToolName:           "filesystem___read_text_file",
			IsError:            boolPtr(false),
			Success:            boolPtr(true),
		},
		{
			Source:             persistence.TaskRunEventSourceTask,
			ID:                 "task-step-1",
			CreatedAtUnixMilli: 1_300,
			EventType:          "step_started",
			PayloadJSON:        json.RawMessage(`{"stepId":"verify_failing_baseline","step":1}`),
		},
		{
			Source:             persistence.TaskRunEventSourceTask,
			ID:                 "task-step-1-retry",
			CreatedAtUnixMilli: 1_350,
			EventType:          "step_started",
			PayloadJSON:        json.RawMessage(`{"stepId":"verify_failing_baseline","step":1}`),
		},
		{
			Source:             persistence.TaskRunEventSourceTask,
			ID:                 "task-step-complete",
			CreatedAtUnixMilli: 1_400,
			EventType:          "step_completed",
			PayloadJSON:        json.RawMessage(`{"stepId":"verify_failing_baseline","status":"active"}`),
		},
		{
			Source:             persistence.TaskRunEventSourceAction,
			ID:                 "action-failed-task-tool",
			CreatedAtUnixMilli: 1_450,
			ToolName:           "centian.task_complete_step",
			IsError:            boolPtr(true),
			Success:            boolPtr(false),
		},
		{
			Source:             persistence.TaskRunEventSourceAction,
			ID:                 "action-command",
			CreatedAtUnixMilli: 1_500,
			ToolName:           "functions.exec_command",
			IsError:            boolPtr(false),
			Success:            boolPtr(true),
		},
		{
			Source:             persistence.TaskRunEventSourceTask,
			ID:                 "task-planning-2",
			CreatedAtUnixMilli: 1_600,
			EventType:          "planning_completed",
			PayloadJSON:        json.RawMessage(`{"status":"active"}`),
		},
		{
			Source:             persistence.TaskRunEventSourceTask,
			ID:                 "task-final-step",
			CreatedAtUnixMilli: 1_700,
			EventType:          "step_completed",
			PayloadJSON:        json.RawMessage(`{"stepId":"implement_green","status":"completed"}`),
		},
	}
	if caseID == "assertion_failure_red" {
		events = append(events, persistence.TaskRunEvent{
			Source:             persistence.TaskRunEventSourceTask,
			ID:                 "task-restarted",
			CreatedAtUnixMilli: 1_550,
			EventType:          "task_restarted",
		})
	}
	return events
}

func loadScorecard(t *testing.T, path string) *RunScorecard {
	t.Helper()
	var scorecard RunScorecard
	assert.NilError(t, readJSONFile(path, &scorecard))
	return &scorecard
}

func boolPtr(value bool) *bool {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}
