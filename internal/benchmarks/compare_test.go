package benchmarks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/persistence"
	"github.com/T4cceptor/centian/internal/taskruns"
	"gotest.tools/assert"
)

func TestCompareSuiteBuildsCrossSessionComparison(t *testing.T) {
	root := t.TempDir()
	suiteRoot := filepath.Join(root, "simple_tdd_v1")
	sessionOne := filepath.Join(suiteRoot, "20260404210000_run")
	sessionTwo := filepath.Join(suiteRoot, "20260404220000_run")

	writeSyntheticScoringSessionAt(t, sessionOne, syntheticSessionOptions{})
	writeSyntheticScoringSessionAt(t, sessionTwo, syntheticSessionOptions{includeInvariantViolation: true})
	store, err := persistence.NewSQLiteStore(filepath.Join(root, "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	persistSyntheticBenchmarkArtifacts(t, store, sessionOne)
	persistSyntheticBenchmarkArtifacts(t, store, sessionTwo)
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
	t.Setenv("CENTIAN_LOG_DIR", root)

	comparer := &Comparer{Now: func() time.Time { return time.Date(2026, 4, 4, 23, 0, 0, 0, time.UTC) }}
	comparison, outputPath, err := comparer.CompareSuite(context.Background(), &CompareOptions{
		RootPath: root,
		SuiteID:  "simple_tdd_v1",
	})
	assert.NilError(t, err)
	assert.Equal(t, outputPath, "")
	assert.Equal(t, comparison.SessionCount, 2)
	assert.Equal(t, comparison.RunCount, 4)
	assert.Equal(t, len(comparison.Aggregates.BySession), 2)
	assert.Equal(t, len(comparison.Aggregates.ByCase), 2)
	assert.Equal(t, len(comparison.Aggregates.ByAgent), 2)
	assert.Equal(t, len(comparison.Aggregates.ByTemplateVariant), 1)
	assert.Equal(t, len(comparison.Aggregates.ByCaseAgentVariant), 2)
	assert.Equal(t, comparison.Runs[0].SessionPath, sessionTwo)
	assert.Equal(t, comparison.Runs[2].SessionPath, sessionOne)
}

func TestCompareSuiteAppliesFilters(t *testing.T) {
	root := t.TempDir()
	suiteRoot := filepath.Join(root, "simple_tdd_v1")
	sessionOne := filepath.Join(suiteRoot, "20260404210000_run")
	sessionTwo := filepath.Join(suiteRoot, "20260404220000_run")

	writeSyntheticScoringSessionAt(t, sessionOne, syntheticSessionOptions{})
	writeSyntheticScoringSessionAt(t, sessionTwo, syntheticSessionOptions{})
	store, err := persistence.NewSQLiteStore(filepath.Join(root, "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	persistSyntheticBenchmarkArtifacts(t, store, sessionOne)
	persistSyntheticBenchmarkArtifacts(t, store, sessionTwo)
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
	t.Setenv("CENTIAN_LOG_DIR", root)

	comparer := NewComparer()
	comparison, _, err := comparer.CompareSuite(context.Background(), &CompareOptions{
		RootPath: root,
		SuiteID:  "simple_tdd_v1",
		Agents:   []string{"codex"},
		CaseIDs:  []string{"compile_failure_red"},
	})
	assert.NilError(t, err)
	assert.Equal(t, comparison.RunCount, 2)
	assert.Equal(t, len(comparison.Aggregates.BySession), 2)
	assert.Equal(t, len(comparison.Aggregates.ByAgent), 1)
	assert.Equal(t, comparison.Aggregates.ByAgent[0].Agent, "codex")
}

func persistSyntheticBenchmarkArtifacts(t *testing.T, store *persistence.Store, sessionDir string) {
	t.Helper()
	session, err := loadSessionManifest(sessionDir)
	assert.NilError(t, err)
	record, err := buildSessionArtifactRecord(session)
	assert.NilError(t, err)
	assert.NilError(t, store.UpsertBenchmarkArtifact(context.Background(), record))
	for idx := range session.Runs {
		run, loadErr := loadRunManifest(filepath.Join(sessionDir, session.Runs[idx].RelativeRunDir, runFileName))
		assert.NilError(t, loadErr)
		runRecord, recordErr := buildRunArtifactRecord(run)
		assert.NilError(t, recordErr)
		assert.NilError(t, store.UpsertBenchmarkArtifact(context.Background(), runRecord))
	}
}
