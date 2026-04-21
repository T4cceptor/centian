package benchmarks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/persistence"
	"github.com/T4cceptor/centian/internal/taskruns"
	"github.com/T4cceptor/centian/internal/taskverification"
	"gotest.tools/assert"
)

func TestCollectFloat64(t *testing.T) {
	rows := []int{1, 2, 3}
	if got := collectFloat64(rows, func(row int) (float64, bool) { return float64(row) * 1.5, true }); !reflect.DeepEqual(got, []float64{1.5, 3, 4.5}) {
		t.Fatalf("unexpected collected float values: %#v", got)
	}

	type optionalRow struct {
		Value *int64
	}
	valueOne := int64(10)
	valueTwo := int64(30)
	optionalRows := []optionalRow{{Value: &valueOne}, {}, {Value: &valueTwo}}
	if got := collectFloat64(optionalRows, func(row optionalRow) (float64, bool) {
		if row.Value == nil {
			return 0, false
		}
		return float64(*row.Value), true
	}); !reflect.DeepEqual(got, []float64{10, 30}) {
		t.Fatalf("unexpected collected optional float values: %#v", got)
	}
}

type syntheticSessionOptions struct {
	includeInvariantViolation bool
	includeLegacyManualScore  bool
	codexSelectedModel        string
}

func writeSyntheticScoringSessionAt(t *testing.T, sessionDir string, opts syntheticSessionOptions) {
	t.Helper()

	suiteRoot := checkedInSimpleTDDSuiteRoot(t)
	assert.NilError(t, os.MkdirAll(sessionDir, 0o755))
	sharedEventStorePath := filepath.Join(sessionDir, "shared-events.sqlite")

	runs := []SessionRunManifestEntry{
		{
			CaseID:          "compile_failure_red",
			CaseName:        "Compile Failure Red",
			AgentID:         "codex",
			TemplateVariant: "current",
			Attempt:         1,
			RelativeRunDir:  filepath.Join("runs", "current_codex_compile_failure_red_attempt_001"),
			Status:          "completed",
		},
		{
			CaseID:          "assertion_failure_red",
			CaseName:        "Assertion Failure Red",
			AgentID:         "claude",
			TemplateVariant: "current",
			Attempt:         1,
			RelativeRunDir:  filepath.Join("runs", "current_claude_assertion_failure_red_attempt_001"),
			Status:          "completed",
		},
	}

	runManifests := make([]*RunManifest, 0, len(runs))
	for _, entry := range runs {
		runManifests = append(runManifests, writeSyntheticRun(t, sessionDir, suiteRoot, sharedEventStorePath, entry, opts))
	}

	session := &SessionManifest{
		SuiteID:       "simple_tdd_v1",
		SuiteName:     "Simple TDD Benchmark Suite v1",
		TemplateID:    "simple_tdd",
		TemplateName:  "Simple TDD Current",
		SuitePath:     suiteRoot,
		InvocationDir: sessionDir,
		OutputRoot:    filepath.Dir(sessionDir),
		StartedAt:     time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
		EndedAt:       time.Date(2026, 4, 4, 12, 10, 0, 0, time.UTC),
		Status:        "completed",
		Repeat:        1,
		Agents:        []string{"codex", "claude"},
		CaseIDs:       []string{"compile_failure_red", "assertion_failure_red"},
		TemplateVariants: []TemplateVariant{{
			Name: "current", SourceDir: filepath.Join(t.TempDir(), "templates"),
		}},
		Runs: runs,
	}
	assert.NilError(t, common.WriteJSONFile(filepath.Join(sessionDir, sessionFileName), session))
	store, err := persistence.NewSQLiteStore(sharedEventStorePath)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	sessionRecord, err := buildSessionRecord(session)
	assert.NilError(t, err)
	assert.NilError(t, store.UpsertBenchmarkSession(context.Background(), sessionRecord))
	for idx := range runManifests {
		runRecord, recordErr := buildRunRecord(runManifests[idx])
		assert.NilError(t, recordErr)
		assert.NilError(t, store.UpsertBenchmarkRun(context.Background(), runRecord))
		scoreRecord, scoreErr := buildPersistedRunScoreRecord(context.Background(), sharedEventStorePath, sessionRecord, runRecord, common.NowUTC)
		assert.NilError(t, scoreErr)
		assert.Assert(t, scoreRecord.ErrorActionabilityScore == nil)
		assert.NilError(t, store.UpsertBenchmarkRunScore(context.Background(), scoreRecord))
	}
}

func persistSyntheticBenchmarkArtifacts(t *testing.T, store *persistence.Store, sessionDir string) {
	t.Helper()

	session, err := loadSessionManifest(sessionDir)
	assert.NilError(t, err)
	sessionRecord, err := buildSessionRecord(session)
	assert.NilError(t, err)
	assert.NilError(t, store.UpsertBenchmarkSession(context.Background(), sessionRecord))
	for idx := range session.Runs {
		var run RunManifest
		loadErr := common.ReadJSONFile(filepath.Join(sessionDir, session.Runs[idx].RelativeRunDir, runFileName), &run)
		assert.NilError(t, loadErr)
		runRecord, recordErr := buildRunRecord(&run)
		assert.NilError(t, recordErr)
		assert.NilError(t, store.UpsertBenchmarkRun(context.Background(), runRecord))
		scoreRecord, scoreErr := buildPersistedRunScoreRecord(context.Background(), run.ArtifactPaths.EventStorePath, sessionRecord, runRecord, common.NowUTC)
		assert.NilError(t, scoreErr)
		assert.Assert(t, scoreRecord.ErrorActionabilityScore == nil)
		assert.NilError(t, store.UpsertBenchmarkRunScore(context.Background(), scoreRecord))
	}
}

func writeSyntheticRun(t *testing.T, sessionDir string, suiteRoot string, sharedEventStorePath string, entry SessionRunManifestEntry, opts syntheticSessionOptions) *RunManifest {
	t.Helper()

	caseDef, err := LoadCase(suiteRoot, SuiteCaseRef{ID: entry.CaseID, Path: filepath.Join("cases", entry.CaseID)})
	assert.NilError(t, err)
	caseRoot := filepath.Join(suiteRoot, "cases", entry.CaseID)
	fixtureRoot := filepath.Join(caseRoot, caseDef.Fixture.SeedPath)

	runDir := filepath.Join(sessionDir, entry.RelativeRunDir)
	projectDir := filepath.Join(runDir, "project")
	logsDir := filepath.Join(runDir, "logs")
	agentDir := filepath.Join(runDir, "agent")
	assert.NilError(t, os.MkdirAll(logsDir, 0o755))
	assert.NilError(t, os.MkdirAll(agentDir, 0o755))
	assert.NilError(t, common.CopyDir(fixtureRoot, projectDir))
	selectedTemplatePath := filepath.Join(runDir, "selected-template.yaml")
	assert.NilError(t, os.WriteFile(selectedTemplatePath, []byte("version: \"0.1\"\ntask:\n  id: simple_tdd\n  name: Simple TDD Current\nworkflow:\n  onboarding: {}\n  planning: {}\n  execution: []\n"), 0o644))

	switch entry.CaseID {
	case "compile_failure_red":
		assert.NilError(t, os.WriteFile(filepath.Join(projectDir, "internal", "health", "health.go"), []byte("package health\n\nfunc Status() string { return \"ready\" }\n"), 0o644))
	case "assertion_failure_red":
		assert.NilError(t, os.WriteFile(filepath.Join(projectDir, "internal", "health", "health.go"), []byte("package health\n\nfunc Status() string { return \"ready\" }\n"), 0o644))
		if opts.includeInvariantViolation {
			assert.NilError(t, os.WriteFile(filepath.Join(projectDir, "internal", "health", "health_test.go"), []byte("package health\n"), 0o644))
		}
	}

	requestLogPath := filepath.Join(logsDir, "requests_0001.jsonl")
	assert.NilError(t, os.WriteFile(requestLogPath, []byte("{}\n"), 0o644))
	assert.NilError(t, os.WriteFile(filepath.Join(agentDir, "agent.stderr.log"), []byte{}, 0o644))

	taskRunID := map[string]string{
		"compile_failure_red":   "tr_compile",
		"assertion_failure_red": "tr_assert",
	}[entry.CaseID]
	events := syntheticEventsForCase(entry.CaseID)
	assert.NilError(t, writeSyntheticEventStore(sharedEventStorePath, taskRunID, events))
	assert.NilError(t, os.WriteFile(filepath.Join(agentDir, "agent.stdout.log"), []byte(syntheticAgentStdout(entry.AgentID)), 0o644))
	store, err := persistence.NewSQLiteStore(sharedEventStorePath)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	assert.NilError(t, store.UpsertTaskRunSnapshot(context.Background(), &taskruns.PersistedRunSnapshot{
		RunID:        taskRunID,
		TemplateID:   "simple_tdd",
		TemplateName: "Simple TDD Current",
		Status:       "completed",
		Phase:        "execution.implement_fix",
		SelectedTemplate: taskruns.PersistedTemplateSnapshot{
			Task: taskruns.PersistedTaskSnapshot{ID: "simple_tdd", Name: "Simple TDD Current"},
		},
	}))

	if opts.includeLegacyManualScore {
		assert.NilError(t, os.WriteFile(filepath.Join(runDir, "manual_score.json"), []byte(`{"errorActionabilityScore":3,"notes":"Actionable"}`), 0o644))
	}

	run := &RunManifest{
		SuiteID:          "simple_tdd_v1",
		SuiteName:        "Simple TDD Benchmark Suite v1",
		CaseID:           entry.CaseID,
		CaseName:         entry.CaseName,
		TemplateID:       "simple_tdd",
		TemplateName:     "Simple TDD Current",
		TemplateVariant:  TemplateVariant{Name: "current"},
		AgentID:          entry.AgentID,
		SelectedModel:    syntheticSelectedModel(entry.AgentID, opts),
		StartedAt:        time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
		EndedAt:          time.Date(2026, 4, 4, 12, 0, 5, 0, time.UTC),
		Status:           "completed",
		LinkedTaskRunIDs: []string{taskRunID},
		ArtifactPaths: RunArtifactPaths{
			RunDir:               runDir,
			ProjectDir:           projectDir,
			LogsDir:              logsDir,
			AgentDir:             agentDir,
			ConfigPath:           filepath.Join(runDir, "centian.config.json"),
			EventStoreMode:       "configured_shared",
			EventStorePath:       sharedEventStorePath,
			RequestLogPath:       requestLogPath,
			SelectedTemplatePath: selectedTemplatePath,
		},
	}
	assert.NilError(t, common.WriteJSONFile(filepath.Join(runDir, runFileName), run))
	return run
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

func writeSyntheticEventStore(path string, taskRunID string, events []persistence.TaskRunEvent) error {
	store, err := persistence.NewSQLiteStore(path)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	for _, event := range events {
		event.ID = taskRunID + "_" + event.ID
		if event.RequestID != "" {
			event.RequestID = taskRunID + "_" + event.RequestID
		}
		if event.RelatedActionRequestID != "" {
			event.RelatedActionRequestID = taskRunID + "_" + event.RelatedActionRequestID
		}
		switch event.Source {
		case persistence.TaskRunEventSourceTask:
			if err := store.AppendTaskEvent(&taskverification.TaskEvent{
				ID:                 event.ID,
				SchemaVersion:      1,
				CreatedAtUnixMilli: event.CreatedAtUnixMilli,
				TaskRunID:          taskRunID,
				SessionID:          "session-1",
				TemplateID:         "simple_tdd",
				PhasePath:          taskverification.TaskPhase(event.PhasePath),
				NodeKind:           taskverification.WorkflowNodeKind(event.NodeKind),
				ResultingPhasePath: taskverification.TaskPhase(event.ResultingPhasePath),
				ResultingNodeKind:  taskverification.WorkflowNodeKind(event.ResultingNodeKind),
				EventType:          taskverification.TaskEventType(event.EventType),
				Outcome:            taskverification.TaskEventOutcome(event.Outcome),
				Payload:            event.PayloadJSON,
			}); err != nil {
				return err
			}
		case persistence.TaskRunEventSourceAction:
			requestID := event.RequestID
			if requestID == "" {
				requestID = event.ID + "_req"
			}
			if err := store.AppendActionEventTaskContext(taskverification.ActionEventTaskContext{
				RequestID:          requestID,
				TaskRunID:          taskRunID,
				CreatedAtUnixMilli: event.CreatedAtUnixMilli,
			}); err != nil {
				return err
			}
			record := &persistence.ActionEventRecord{
				ID:                 event.ID,
				SchemaVersion:      4,
				CreatedAtUnixMilli: event.CreatedAtUnixMilli,
				RequestID:          requestID,
				SessionID:          "session-1",
				ToolName:           event.ToolName,
				OriginalToolName:   event.OriginalToolName,
				Success:            event.Success != nil && *event.Success,
				IsError:            event.IsError != nil && *event.IsError,
				PayloadJSON:        event.PayloadJSON,
			}
			if _, err := store.DB().NewInsert().Model(record).Exec(context.Background()); err != nil {
				return err
			}
		}
	}
	return nil
}

func syntheticAgentStdout(agent string) string {
	switch agent {
	case "claude":
		return "\n===== Demo run 2026-04-04T21:35:26+02:00 (claude) =====\n" +
			"{\"type\":\"result\",\"session_id\":\"session_claude\",\"num_turns\":7,\"duration_ms\":3210,\"total_cost_usd\":0.12,\"usage\":{\"input_tokens\":123,\"output_tokens\":222,\"cache_creation_input_tokens\":333,\"cache_read_input_tokens\":444},\"modelUsage\":{\"claude-sonnet-4-6\":{\"inputTokens\":123,\"outputTokens\":222,\"cacheReadInputTokens\":444,\"cacheCreationInputTokens\":333,\"costUSD\":0.12}}}\n"
	default:
		return "\n===== Demo run 2026-04-04T21:22:12+02:00 (codex) =====\n" +
			"{\"type\":\"thread.started\",\"thread_id\":\"thread_codex\"}\n" +
			"{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":111,\"cached_input_tokens\":555,\"output_tokens\":666}}\n"
	}
}

func syntheticSelectedModel(agent string, opts syntheticSessionOptions) string {
	switch agent {
	case "claude":
		return common.ModelClaudeSonnet
	default:
		if opts.codexSelectedModel != "" {
			return opts.codexSelectedModel
		}
		return common.ModelCodexGPT54Mini
	}
}

func boolPtr(value bool) *bool {
	return &value
}
