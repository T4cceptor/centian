package benchmarks

import (
	"context"
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
	writeSyntheticScoringSessionAt(t, sessionTwo, syntheticSessionOptions{includeInvariantViolation: true})

	store, err := persistence.NewSQLiteStore(filepath.Join(root, "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	scorer := &Scorer{
		PersistArtifact: func(ctx context.Context, _ string, record *persistence.BenchmarkArtifactRecord) error {
			return store.UpsertBenchmarkArtifact(ctx, record)
		},
	}
	_, err = scorer.ScoreSession(context.Background(), &ScoreOptions{SessionPath: sessionOne})
	assert.NilError(t, err)
	_, err = scorer.ScoreSession(context.Background(), &ScoreOptions{SessionPath: sessionTwo})
	assert.NilError(t, err)
	sessionOneManifest, err := loadSessionManifest(sessionOne)
	assert.NilError(t, err)
	sessionOneRecord, err := buildSessionArtifactRecord(sessionOneManifest, filepath.Join(root, "events.sqlite"))
	assert.NilError(t, err)
	assert.NilError(t, store.UpsertBenchmarkArtifact(context.Background(), sessionOneRecord))
	sessionTwoManifest, err := loadSessionManifest(sessionTwo)
	assert.NilError(t, err)
	sessionTwoRecord, err := buildSessionArtifactRecord(sessionTwoManifest, filepath.Join(root, "events.sqlite"))
	assert.NilError(t, err)
	assert.NilError(t, store.UpsertBenchmarkArtifact(context.Background(), sessionTwoRecord))
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
	assert.Equal(t, runs[0].TemplateName, "Simple TDD Current")

	runDetail, err := service.GetRun(context.Background(), "simple_tdd_v1", runs[0].ScorecardID)
	assert.NilError(t, err)
	assert.Assert(t, runDetail != nil)
	assert.Equal(t, runDetail.Scorecard.Agent, "codex")
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
}

func TestReadServiceReturnsNilForMissingResources(t *testing.T) {
	store, err := persistence.NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	service := NewReadService(store)
	session, err := service.GetSession(context.Background(), "simple_tdd_v1", "missing")
	assert.NilError(t, err)
	assert.Assert(t, session == nil)

	run, err := service.GetRun(context.Background(), "simple_tdd_v1", "missing")
	assert.NilError(t, err)
	assert.Assert(t, run == nil)

	comparison, err := service.GetComparison(context.Background(), "simple_tdd_v1", BenchmarkRunFilters{})
	assert.NilError(t, err)
	assert.Assert(t, comparison == nil)
}
