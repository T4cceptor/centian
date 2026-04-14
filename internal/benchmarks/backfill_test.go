package benchmarks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/T4cceptor/centian/internal/persistence"
	"gotest.tools/assert"
)

func TestBackfillScoresWritesMissingSnapshotsAndRestoresReadViews(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "simple_tdd_v1", "20260404210000_run")
	writeSyntheticScoringSessionAt(t, sessionDir, syntheticSessionOptions{})

	mainStorePath := filepath.Join(root, "events.sqlite")
	mainStore, err := persistence.NewSQLiteStore(mainStorePath)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = mainStore.Close() })
	persistLegacyBenchmarkArtifacts(t, mainStore, sessionDir)

	service := NewBackfillService()
	result, err := service.BackfillScores(context.Background(), &BackfillOptions{
		MainStorePath: mainStorePath,
		SuiteID:       "simple_tdd_v1",
	})
	assert.NilError(t, err)
	assert.Equal(t, result.TotalCandidates, 2)
	assert.Equal(t, result.ScoredCount, 2)
	assert.Equal(t, result.UnscoredCount, 0)
	assert.Equal(t, result.SkippedCount, 0)

	scoreRows, err := mainStore.ListBenchmarkRunScores(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, len(scoreRows), 2)
	for _, row := range scoreRows {
		assert.Equal(t, row.ScoreStatus, benchmarkRunScoreStatusReady)
	}

	readService := NewReadService(mainStore)
	runs, err := readService.ListRuns(context.Background(), "simple_tdd_v1", BenchmarkRunFilters{})
	assert.NilError(t, err)
	assert.Equal(t, len(runs), 2)
	assert.Assert(t, runs[0].Scored)
	sessionDetail, err := readService.GetSession(context.Background(), "simple_tdd_v1", runs[0].SessionID)
	assert.NilError(t, err)
	assert.Assert(t, sessionDetail != nil)
	assert.Equal(t, sessionDetail.ScoredRunCount, 2)
	assert.Equal(t, sessionDetail.FailedToScoreCount, 0)
}

func TestBackfillScoresSkipsExistingSnapshotsAndForceOverwrites(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "simple_tdd_v1", "20260404210000_run")
	writeSyntheticScoringSessionAt(t, sessionDir, syntheticSessionOptions{})

	mainStorePath := filepath.Join(root, "events.sqlite")
	mainStore, err := persistence.NewSQLiteStore(mainStorePath)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = mainStore.Close() })
	persistLegacyBenchmarkArtifacts(t, mainStore, sessionDir)

	service := NewBackfillService()
	first, err := service.BackfillScores(context.Background(), &BackfillOptions{MainStorePath: mainStorePath})
	assert.NilError(t, err)
	assert.Equal(t, first.ScoredCount, 2)

	second, err := service.BackfillScores(context.Background(), &BackfillOptions{MainStorePath: mainStorePath})
	assert.NilError(t, err)
	assert.Equal(t, second.TotalCandidates, 2)
	assert.Equal(t, second.SkippedCount, 2)
	assert.Equal(t, second.ScoredCount, 0)

	forced, err := service.BackfillScores(context.Background(), &BackfillOptions{
		MainStorePath: mainStorePath,
		Force:         true,
	})
	assert.NilError(t, err)
	assert.Equal(t, forced.TotalCandidates, 2)
	assert.Equal(t, forced.ScoredCount, 2)
	assert.Equal(t, forced.OverwrittenCount, 2)
}

func TestBackfillScoresWritesUnscoredSnapshotsWhenArtifactsAreMissing(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "simple_tdd_v1", "20260404210000_run")
	writeSyntheticScoringSessionAt(t, sessionDir, syntheticSessionOptions{})

	mainStorePath := filepath.Join(root, "events.sqlite")
	mainStore, err := persistence.NewSQLiteStore(mainStorePath)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = mainStore.Close() })
	persistLegacyBenchmarkArtifacts(t, mainStore, sessionDir)

	assert.NilError(t, os.Remove(filepath.Join(sessionDir, "shared-events.sqlite")))

	service := NewBackfillService()
	result, err := service.BackfillScores(context.Background(), &BackfillOptions{MainStorePath: mainStorePath})
	assert.NilError(t, err)
	assert.Equal(t, result.TotalCandidates, 2)
	assert.Equal(t, result.ScoredCount, 0)
	assert.Equal(t, result.UnscoredCount, 2)
	assert.Equal(t, len(result.Failures), 2)

	scoreRows, err := mainStore.ListBenchmarkRunScores(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, len(scoreRows), 2)
	for _, row := range scoreRows {
		assert.Equal(t, row.ScoreStatus, benchmarkRunScoreStatusUnscored)
		assert.Assert(t, len(row.ScoreErrors) > 0)
	}
}

func TestBackfillScoresRetriesExistingUnscoredSnapshotsWithoutForce(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "simple_tdd_v1", "20260404210000_run")
	writeSyntheticScoringSessionAt(t, sessionDir, syntheticSessionOptions{})

	mainStorePath := filepath.Join(root, "events.sqlite")
	mainStore, err := persistence.NewSQLiteStore(mainStorePath)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = mainStore.Close() })
	persistLegacyBenchmarkArtifacts(t, mainStore, sessionDir)

	sharedStorePath := filepath.Join(sessionDir, "shared-events.sqlite")
	backupStorePath := filepath.Join(root, "shared-events.backup.sqlite")
	assert.NilError(t, os.Rename(sharedStorePath, backupStorePath))

	service := NewBackfillService()
	first, err := service.BackfillScores(context.Background(), &BackfillOptions{MainStorePath: mainStorePath})
	assert.NilError(t, err)
	assert.Equal(t, first.ScoredCount, 0)
	assert.Equal(t, first.UnscoredCount, 2)

	assert.NilError(t, os.Rename(backupStorePath, sharedStorePath))

	second, err := service.BackfillScores(context.Background(), &BackfillOptions{MainStorePath: mainStorePath})
	assert.NilError(t, err)
	assert.Equal(t, second.TotalCandidates, 2)
	assert.Equal(t, second.SkippedCount, 0)
	assert.Equal(t, second.ScoredCount, 2)
	assert.Equal(t, second.UnscoredCount, 0)
	assert.Equal(t, second.OverwrittenCount, 2)

	scoreRows, err := mainStore.ListBenchmarkRunScores(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, len(scoreRows), 2)
	for _, row := range scoreRows {
		assert.Equal(t, row.ScoreStatus, benchmarkRunScoreStatusReady)
	}
}

func TestBackfillScoresAppliesPathRemapsWithoutMutatingStoredMetadata(t *testing.T) {
	root := t.TempDir()
	oldRoot := filepath.Join(root, "old-root")
	newRoot := filepath.Join(root, "new-root")
	sessionDir := filepath.Join(oldRoot, "simple_tdd_v1", "20260404210000_run")
	writeSyntheticScoringSessionAt(t, sessionDir, syntheticSessionOptions{})

	mainStorePath := filepath.Join(root, "events.sqlite")
	mainStore, err := persistence.NewSQLiteStore(mainStorePath)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = mainStore.Close() })
	persistLegacyBenchmarkArtifacts(t, mainStore, sessionDir)

	assert.NilError(t, os.Rename(oldRoot, newRoot))

	service := NewBackfillService()
	result, err := service.BackfillScores(context.Background(), &BackfillOptions{
		MainStorePath: mainStorePath,
		PathRemaps: []PathRemap{{
			From: oldRoot,
			To:   newRoot,
		}},
	})
	assert.NilError(t, err)
	assert.Equal(t, result.ScoredCount, 2)

	runRows, err := mainStore.ListBenchmarkRuns(context.Background(), &persistence.BenchmarkRunFilter{})
	assert.NilError(t, err)
	assert.Assert(t, len(runRows) > 0)
	assert.Assert(t, filepath.Clean(runRows[0].RunDir) != filepath.Clean(applyPathRemaps(runRows[0].RunDir, result.PathRemaps)))
	assert.Assert(t, pathHasDirPrefix(runRows[0].RunDir, oldRoot))
}

func persistLegacyBenchmarkArtifacts(t *testing.T, store *persistence.Store, sessionDir string) {
	t.Helper()
	session, err := loadSessionManifest(sessionDir)
	assert.NilError(t, err)
	sessionRecord, err := buildSessionRecord(session)
	assert.NilError(t, err)
	assert.NilError(t, store.UpsertBenchmarkSession(context.Background(), sessionRecord))
	for idx := range session.Runs {
		run, loadErr := loadRunManifest(filepath.Join(sessionDir, session.Runs[idx].RelativeRunDir, runFileName))
		assert.NilError(t, loadErr)
		runRecord, recordErr := buildRunRecord(run)
		assert.NilError(t, recordErr)
		assert.NilError(t, store.UpsertBenchmarkRun(context.Background(), runRecord))
	}
}

func pathHasDirPrefix(path, prefix string) bool {
	relative, err := filepath.Rel(filepath.Clean(prefix), filepath.Clean(path))
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
