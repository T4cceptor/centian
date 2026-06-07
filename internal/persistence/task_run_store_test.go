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
			CompiledWorkflow: &taskruns.PersistedCompiledWorkflowSnapshot{
				WorkflowSteps: []taskruns.PersistedCompiledStepSnapshot{{
					ID:           "step_one",
					Path:         "execution.step_one",
					Instructions: "Implement the first step.",
				}},
			},
		},
		RunnableTemplate: &taskruns.PersistedTemplateSnapshot{
			Version: "0.1",
			Task:    taskruns.PersistedTaskSnapshot{ID: "simple_tdd", Name: "Simple TDD Task", Description: "Resolved task"},
			CompiledWorkflow: &taskruns.PersistedCompiledWorkflowSnapshot{
				WorkflowSteps: []taskruns.PersistedCompiledStepSnapshot{{
					ID:           "step_one",
					Path:         "execution.step_one",
					Instructions: "Implement the resolved first step.",
				}},
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
	assert.Equal(t, record.Payload.SelectedTemplate.CompiledWorkflow.WorkflowSteps[0].Instructions, "Implement the first step.")
	assert.Assert(t, record.Payload.RunnableTemplate != nil)
	assert.Equal(t, record.Payload.RunnableTemplate.CompiledWorkflow.WorkflowSteps[0].Instructions, "Implement the resolved first step.")
	assert.Equal(t, record.Payload.Steps[0].InvariantBaselines["stable"], "same")
}

func minimalRunSnapshot(runID, owner, status string) *taskruns.PersistedRunSnapshot {
	return &taskruns.PersistedRunSnapshot{
		RunID:            runID,
		OwnerPrincipalID: owner,
		TemplateID:       "simple_tdd",
		TemplateName:     "Simple TDD Task",
		Status:           status,
		Phase:            "execution.step_one",
		SelectedTemplate: taskruns.PersistedTemplateSnapshot{
			Version: "0.1",
			Task:    taskruns.PersistedTaskSnapshot{ID: "simple_tdd", Name: "Simple TDD Task", Description: "Test task"},
		},
	}
}

func TestTaskRunSnapshotStorePersistsOwnerPrincipal(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	err = store.UpsertTaskRunSnapshot(context.Background(), minimalRunSnapshot("run-1", "pr_owner", "active"))
	assert.NilError(t, err)

	record, err := store.GetTaskRunSnapshot(context.Background(), "run-1")
	assert.NilError(t, err)
	assert.Equal(t, record.OwnerPrincipalID, "pr_owner")
	assert.Equal(t, record.Payload.OwnerPrincipalID, "pr_owner")

	payload, err := store.LoadTaskRunSnapshot(context.Background(), "run-1")
	assert.NilError(t, err)
	assert.Equal(t, payload.OwnerPrincipalID, "pr_owner")

	missing, err := store.LoadTaskRunSnapshot(context.Background(), "run-missing")
	assert.NilError(t, err)
	assert.Assert(t, missing == nil)
}

func TestFindOpenRunForPrincipal(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	// Given: open and terminal runs across two principals
	assert.NilError(t, store.UpsertTaskRunSnapshot(ctx, minimalRunSnapshot("run-a-active", "pr_a", "active")))
	assert.NilError(t, store.UpsertTaskRunSnapshot(ctx, minimalRunSnapshot("run-a-done", "pr_a", "completed")))
	assert.NilError(t, store.UpsertTaskRunSnapshot(ctx, minimalRunSnapshot("run-b-timeout", "pr_b", "timed_out")))
	assert.NilError(t, store.UpsertTaskRunSnapshot(ctx, minimalRunSnapshot("run-c-failed", "pr_c", "failed")))

	// Then: each principal's open run (or absence thereof) is reported correctly
	openA, err := store.FindOpenRunForPrincipal(ctx, "pr_a")
	assert.NilError(t, err)
	assert.Assert(t, openA != nil)
	assert.Equal(t, openA.RunID, "run-a-active")

	openB, err := store.FindOpenRunForPrincipal(ctx, "pr_b")
	assert.NilError(t, err)
	assert.Assert(t, openB != nil)
	assert.Equal(t, openB.RunID, "run-b-timeout")

	openC, err := store.FindOpenRunForPrincipal(ctx, "pr_c")
	assert.NilError(t, err)
	assert.Assert(t, openC == nil)

	none, err := store.FindOpenRunForPrincipal(ctx, "pr_unknown")
	assert.NilError(t, err)
	assert.Assert(t, none == nil)

	// An empty principal owns no attributable run.
	empty, err := store.FindOpenRunForPrincipal(ctx, "")
	assert.NilError(t, err)
	assert.Assert(t, empty == nil)
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
