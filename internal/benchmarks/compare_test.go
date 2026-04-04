package benchmarks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/persistence"
	"gotest.tools/assert"
)

func TestCompareSuiteBuildsCrossSessionComparison(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv("CENTIAN_LOG_DIR", logDir)
	root := t.TempDir()
	suiteRoot := filepath.Join(root, "simple_tdd_v1")
	sessionOne := filepath.Join(suiteRoot, "20260404210000_run")
	sessionTwo := filepath.Join(suiteRoot, "20260404220000_run")

	writeSyntheticScoringSessionAt(t, sessionOne, syntheticSessionOptions{})
	writeSyntheticScoringSessionAt(t, sessionTwo, syntheticSessionOptions{includeInvariantViolation: true})

	scorer := &Scorer{Now: func() time.Time { return time.Date(2026, 4, 4, 22, 30, 0, 0, time.UTC) }}
	_, err := scorer.ScoreSession(context.Background(), &ScoreOptions{SessionPath: sessionOne})
	assert.NilError(t, err)
	_, err = scorer.ScoreSession(context.Background(), &ScoreOptions{SessionPath: sessionTwo})
	assert.NilError(t, err)

	comparer := &Comparer{Now: func() time.Time { return time.Date(2026, 4, 4, 23, 0, 0, 0, time.UTC) }}
	comparison, outputPath, err := comparer.CompareSuite(context.Background(), &CompareOptions{
		RootPath: root,
		SuiteID:  "simple_tdd_v1",
	})
	assert.NilError(t, err)
	assert.Equal(t, outputPath, filepath.Join(suiteRoot, comparisonFileName))
	assert.Equal(t, comparison.SessionCount, 2)
	assert.Equal(t, comparison.RunCount, 4)
	assert.Equal(t, len(comparison.Aggregates.BySession), 2)
	assert.Equal(t, len(comparison.Aggregates.ByCase), 2)
	assert.Equal(t, len(comparison.Aggregates.ByAgent), 2)
	assert.Equal(t, len(comparison.Aggregates.ByTemplateVariant), 1)
	assert.Equal(t, len(comparison.Aggregates.ByCaseAgentVariant), 2)
	assert.Equal(t, comparison.Runs[0].SessionPath, sessionOne)
	assert.Equal(t, comparison.Runs[2].SessionPath, sessionTwo)

	store, err := persistence.NewSQLiteStore(filepath.Join(logDir, "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	comparisons, err := store.ListBenchmarkArtifacts(context.Background(), persistence.BenchmarkArtifactFilter{
		SuiteID:      "simple_tdd_v1",
		ArtifactKind: persistence.BenchmarkArtifactKindComparison,
	})
	assert.NilError(t, err)
	assert.Equal(t, len(comparisons), 1)
}

func TestCompareSuiteAppliesFilters(t *testing.T) {
	t.Setenv("CENTIAN_LOG_DIR", t.TempDir())
	root := t.TempDir()
	suiteRoot := filepath.Join(root, "simple_tdd_v1")
	sessionOne := filepath.Join(suiteRoot, "20260404210000_run")
	sessionTwo := filepath.Join(suiteRoot, "20260404220000_run")

	writeSyntheticScoringSessionAt(t, sessionOne, syntheticSessionOptions{})
	writeSyntheticScoringSessionAt(t, sessionTwo, syntheticSessionOptions{})

	scorer := NewScorer()
	_, err := scorer.ScoreSession(context.Background(), &ScoreOptions{SessionPath: sessionOne})
	assert.NilError(t, err)
	_, err = scorer.ScoreSession(context.Background(), &ScoreOptions{SessionPath: sessionTwo})
	assert.NilError(t, err)

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
