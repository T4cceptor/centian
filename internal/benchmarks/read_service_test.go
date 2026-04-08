package benchmarks

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/T4cceptor/centian/internal/persistence"
	"github.com/T4cceptor/centian/internal/taskruns"
	"gotest.tools/assert"
)

func TestReadServiceListsSuitesSessionsRunsAndComparison(t *testing.T) {
	root := t.TempDir()
	suiteRoot := filepath.Join(root, "simple_tdd_v1")
	sessionOne := filepath.Join(suiteRoot, "20260404210000_run")
	sessionTwo := filepath.Join(suiteRoot, "20260404220000_run")

	writeSyntheticScoringSessionAt(t, sessionOne, syntheticSessionOptions{})
	writeSyntheticScoringSessionAt(t, sessionTwo, syntheticSessionOptions{
		includeInvariantViolation: true,
		codexSelectedModel:        "gpt-5.4",
	})

	store, err := persistence.NewSQLiteStore(filepath.Join(root, "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	sessionOneManifest, err := loadSessionManifest(sessionOne)
	assert.NilError(t, err)
	sessionOneRecord, err := buildSessionRecord(sessionOneManifest)
	assert.NilError(t, err)
	assert.NilError(t, store.UpsertBenchmarkSession(context.Background(), sessionOneRecord))
	for idx := range sessionOneManifest.Runs {
		run, loadErr := loadRunManifest(filepath.Join(sessionOne, sessionOneManifest.Runs[idx].RelativeRunDir, runFileName))
		assert.NilError(t, loadErr)
		record, recordErr := buildRunRecord(run)
		assert.NilError(t, recordErr)
		assert.NilError(t, store.UpsertBenchmarkRun(context.Background(), record))
	}
	sessionTwoManifest, err := loadSessionManifest(sessionTwo)
	assert.NilError(t, err)
	sessionTwoRecord, err := buildSessionRecord(sessionTwoManifest)
	assert.NilError(t, err)
	assert.NilError(t, store.UpsertBenchmarkSession(context.Background(), sessionTwoRecord))
	for idx := range sessionTwoManifest.Runs {
		run, loadErr := loadRunManifest(filepath.Join(sessionTwo, sessionTwoManifest.Runs[idx].RelativeRunDir, runFileName))
		assert.NilError(t, loadErr)
		record, recordErr := buildRunRecord(run)
		assert.NilError(t, recordErr)
		assert.NilError(t, store.UpsertBenchmarkRun(context.Background(), record))
	}
	assert.NilError(t, store.UpsertTaskRunSnapshot(&taskruns.PersistedRunSnapshot{
		RunID:        "tr_compile",
		TemplateID:   "simple_tdd",
		TemplateName: "Simple TDD Current",
		Status:       "completed",
		Phase:        "execution.refactor_while_green",
		SelectedTemplate: taskruns.PersistedTemplateSnapshot{
			Task: taskruns.PersistedTaskSnapshot{ID: "simple_tdd", Name: "Simple TDD Current"},
		},
	}))
	assert.NilError(t, store.UpsertTaskRunSnapshot(&taskruns.PersistedRunSnapshot{
		RunID:        "tr_assert",
		TemplateID:   "simple_tdd",
		TemplateName: "Simple TDD Current",
		Status:       "completed",
		Phase:        "execution.implement_fix",
		SelectedTemplate: taskruns.PersistedTemplateSnapshot{
			Task: taskruns.PersistedTaskSnapshot{ID: "simple_tdd", Name: "Simple TDD Current"},
		},
	}))

	service := NewReadService(store)

	suites, err := service.ListSuites(context.Background(), BenchmarkRunFilters{})
	assert.NilError(t, err)
	assert.Equal(t, len(suites), 1)
	assert.Equal(t, suites[0].SuiteID, "simple_tdd_v1")
	assert.Equal(t, suites[0].SuiteName, "Simple TDD Benchmark Suite v1")
	assert.Equal(t, suites[0].TemplateID, "simple_tdd")
	assert.Equal(t, suites[0].TemplateName, "Simple TDD Current")
	assert.Equal(t, suites[0].SessionCount, 2)
	assert.Equal(t, suites[0].RunCount, 4)

	sessions, err := service.ListSessions(context.Background(), "simple_tdd_v1", BenchmarkRunFilters{})
	assert.NilError(t, err)
	assert.Equal(t, len(sessions), 2)
	assert.Equal(t, sessions[0].SuiteID, "simple_tdd_v1")
	assert.Equal(t, sessions[0].SuiteName, "Simple TDD Benchmark Suite v1")
	assert.Equal(t, len(sessions[0].Runs), 0)

	sessionDetail, err := service.GetSession(context.Background(), "simple_tdd_v1", sessions[0].SessionID)
	assert.NilError(t, err)
	assert.Assert(t, sessionDetail != nil)
	assert.Equal(t, len(sessionDetail.Runs), 2)
	assert.Assert(t, sessionDetail.Runs[0].ScorecardID != "")
	assert.Equal(t, sessionDetail.TemplateName, "Simple TDD Current")

	runs, err := service.ListRuns(context.Background(), "simple_tdd_v1", BenchmarkRunFilters{Agent: "codex"})
	assert.NilError(t, err)
	assert.Equal(t, len(runs), 2)
	assert.Equal(t, runs[0].Agent, "codex")
	codexRunModels := map[string]bool{}
	for _, run := range runs {
		codexRunModels[run.SelectedModel] = true
	}
	assert.Assert(t, codexRunModels["gpt-5.4-mini"])
	assert.Assert(t, codexRunModels["gpt-5.4"])
	assert.Equal(t, runs[0].TemplateName, "Simple TDD Current")

	runDetail, err := service.GetRun(context.Background(), "simple_tdd_v1", runs[0].ScorecardID)
	assert.NilError(t, err)
	assert.Assert(t, runDetail != nil)
	assert.Equal(t, runDetail.Scorecard.Agent, "codex")
	assert.Assert(t, codexRunModels[runDetail.Scorecard.SelectedModel])
	assert.Equal(t, runDetail.TemplateName, "Simple TDD Current")

	comparison, err := service.GetComparison(context.Background(), "simple_tdd_v1", BenchmarkRunFilters{Agent: "codex"})
	assert.NilError(t, err)
	assert.Assert(t, comparison != nil)
	assert.Equal(t, comparison.SuiteID, "simple_tdd_v1")
	assert.Equal(t, comparison.SuiteName, "Simple TDD Benchmark Suite v1")
	assert.Equal(t, comparison.TemplateName, "Simple TDD Current")
	assert.Equal(t, comparison.RunCount, 2)
	assert.Equal(t, len(comparison.Aggregates.ByAgent), 1)
	assert.Equal(t, comparison.Aggregates.ByAgent[0].Agent, "codex")

	scorecards, err := service.ListTemplateScorecards(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, len(scorecards), 1)
	assert.Equal(t, scorecards[0].TemplateID, "simple_tdd")
	assert.Equal(t, scorecards[0].TemplateName, "Simple TDD Current")
	assert.Equal(t, scorecards[0].RunCount, 2)
	assert.Equal(t, scorecards[0].SuccessRate, 1.0)

	agentScorecards, err := service.ListAgentScorecards(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, len(agentScorecards), 3)
	agentScorecardKeys := map[string]bool{}
	for _, scorecard := range agentScorecards {
		agentScorecardKeys[scorecard.Agent+"::"+scorecard.Model] = true
		assert.Equal(t, scorecard.SuccessRate, 1.0)
		if scorecard.Model != "" {
			assert.Equal(t, len(scorecard.Models), 1)
			assert.Equal(t, scorecard.Models[0], scorecard.Model)
		}
	}
	assert.Assert(t, agentScorecardKeys["codex::gpt-5.4-mini"])
	assert.Assert(t, agentScorecardKeys["codex::gpt-5.4"])
	assert.Assert(t, agentScorecardKeys["claude::sonnet"])
}

func TestReadServiceReturnsNilForMissingResources(t *testing.T) {
	store, err := persistence.NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	service := NewReadService(store)
	session, err := service.GetSession(context.Background(), "simple_tdd_v1", "missing")
	assert.Assert(t, errors.Is(err, ErrBenchmarkSessionNotFound))
	assert.Assert(t, session == nil)

	run, err := service.GetRun(context.Background(), "simple_tdd_v1", "missing")
	assert.Assert(t, errors.Is(err, ErrBenchmarkRunNotFound))
	assert.Assert(t, run == nil)

	comparison, err := service.GetComparison(context.Background(), "simple_tdd_v1", BenchmarkRunFilters{})
	assert.Assert(t, errors.Is(err, ErrBenchmarkComparisonNotFound))
	assert.Assert(t, comparison == nil)
}
