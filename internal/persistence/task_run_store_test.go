package persistence

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/T4cceptor/centian/internal/taskruns"
	"gotest.tools/assert"
)

func TestTaskRunSnapshotStoreBootstrapsTable(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	count, err := store.db.NewSelect().TableExpr("sqlite_master").
		Where("type = 'table'").
		Where("name = 'task_runs'").
		Count(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, count, 1)
}

func TestTaskRunSnapshotStoreUpsertAndRoundTrip(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	snapshot := &taskruns.PersistedRunSnapshot{
		RunID:        "run-1",
		TemplateID:   "simple_tdd",
		TemplateName: "Simple TDD Task",
		Status:       "active",
		Phase:        "planning",
		SelectedTemplate: taskruns.PersistedTemplateSnapshot{
			Version: "0.1",
			Task: taskruns.PersistedTaskSnapshot{
				ID:          "simple_tdd",
				Name:        "Simple TDD Task",
				Description: "Test task",
			},
		},
		Steps: []taskruns.PersistedStepStateSnapshot{{
			ID:                 "step_one",
			Path:               "execution.step_one",
			Status:             "active",
			InvariantBaselines: map[string]string{"stable": "same"},
		}},
	}

	err = store.UpsertTaskRunSnapshot(context.Background(), snapshot)
	assert.NilError(t, err)

	record, err := store.GetTaskRunSnapshot(context.Background(), "run-1")
	assert.NilError(t, err)
	assert.Equal(t, record.RunID, "run-1")
	assert.Equal(t, record.TemplateName, "Simple TDD Task")
	assert.Equal(t, record.Payload.SelectedTemplate.Task.Name, "Simple TDD Task")
	assert.Equal(t, record.Payload.Steps[0].InvariantBaselines["stable"], "same")
}

func TestTaskRunSnapshotStoreUpsertOverwritesExistingRow(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	first := &taskruns.PersistedRunSnapshot{
		RunID:        "run-1",
		TemplateID:   "simple_tdd",
		TemplateName: "Simple TDD Task",
		Status:       "active",
		Phase:        "planning",
		SelectedTemplate: taskruns.PersistedTemplateSnapshot{
			Version: "0.1",
			Task:    taskruns.PersistedTaskSnapshot{ID: "simple_tdd", Name: "Simple TDD Task", Description: "Task"},
		},
	}
	second := &taskruns.PersistedRunSnapshot{
		RunID:        "run-1",
		TemplateID:   "simple_tdd",
		TemplateName: "Simple TDD Task",
		Status:       "completed",
		Phase:        "execution.step_one",
		SelectedTemplate: taskruns.PersistedTemplateSnapshot{
			Version: "0.1",
			Task:    taskruns.PersistedTaskSnapshot{ID: "simple_tdd", Name: "Simple TDD Task", Description: "Task"},
		},
	}

	err = store.UpsertTaskRunSnapshot(context.Background(), first)
	assert.NilError(t, err)
	err = store.UpsertTaskRunSnapshot(context.Background(), second)
	assert.NilError(t, err)

	records, err := store.ListTaskRunSnapshots(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, len(records), 1)
	assert.Equal(t, records[0].Status, "completed")
	assert.Equal(t, records[0].Phase, "execution.step_one")
}

func TestTaskRunSnapshotStoreUpsertRespectsCanceledContext(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	snapshot := &taskruns.PersistedRunSnapshot{
		RunID:        "run-1",
		TemplateID:   "simple_tdd",
		TemplateName: "Simple TDD Task",
		Status:       "active",
		Phase:        "planning",
		SelectedTemplate: taskruns.PersistedTemplateSnapshot{
			Version: "0.1",
			Task:    taskruns.PersistedTaskSnapshot{ID: "simple_tdd", Name: "Simple TDD Task", Description: "Task"},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = store.UpsertTaskRunSnapshot(ctx, snapshot)
	assert.Assert(t, errors.Is(err, context.Canceled))
}
