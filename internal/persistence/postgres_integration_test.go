package persistence

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/sqldb"
	"github.com/T4cceptor/centian/internal/taskruns"
	"github.com/T4cceptor/centian/internal/taskverification"
	"gotest.tools/assert"
)

// postgresTables lists every table the event store bootstraps, newest-first so
// dropping respects foreign keys. Used to start each integration run clean.
var postgresTables = []string{
	"benchmark_run_scores",
	"benchmark_run_task_runs",
	"benchmark_runs",
	"benchmark_sessions",
	"event_annotations",
	"action_event_task_context",
	"action_events",
	"task_run_stats",
	"task_runs",
	"task_events",
	"event_store_schema",
}

// newPostgresTestStore opens a fresh event store against the Postgres DSN in
// CENTIAN_TEST_POSTGRES_DSN, skipping the test when it is unset. It drops all
// known tables first so bootstrap runs against an empty database — this also
// proves every CREATE TABLE statement is valid Postgres DDL.
func newPostgresTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("CENTIAN_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set CENTIAN_TEST_POSTGRES_DSN to run Postgres integration tests")
	}

	db, err := sqldb.Open(sqldb.Postgres, dsn)
	assert.NilError(t, err)
	for _, table := range postgresTables {
		_, err := db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table+" CASCADE")
		assert.NilError(t, err)
	}
	assert.NilError(t, db.Close())

	store, err := NewStore(sqldb.Postgres, dsn)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestPostgres_BootstrapAndTaskEventJSONRoundTrip(t *testing.T) {
	// Given: a freshly bootstrapped Postgres event store with a registered task run
	// (real usage upserts the run snapshot at registration before any task events;
	// Postgres enforces the task_run_stats -> task_runs foreign key, unlike SQLite).
	store := newPostgresTestStore(t)
	ctx := context.Background()
	assert.NilError(t, store.UpsertTaskRunSnapshot(ctx, &taskruns.PersistedRunSnapshot{
		RunID:        "pg-run-1",
		TemplateID:   "template-1",
		TemplateName: "Template One",
		Status:       "active",
		Phase:        "planning",
	}))
	payload := json.RawMessage(`{"testTarget":"pytest -q","nested":{"k":1}}`)

	// When: a task event with a JSON payload is appended and read back
	err := store.AppendTaskEvent(&taskverification.TaskEvent{
		ID:                 "pg-task-event-1",
		SchemaVersion:      1,
		CreatedAtUnixMilli: time.Now().UTC().UnixMilli(),
		TaskRunID:          "pg-run-1",
		SessionID:          "pg-session-1",
		TemplateID:         "template-1",
		PrincipalID:        "principal-1",
		PhasePath:          taskverification.TaskPhasePlanning,
		NodeKind:           taskverification.WorkflowNodeKindPlanning,
		ResultingPhasePath: taskverification.TaskPhase("execution.step_one"),
		ResultingNodeKind:  taskverification.WorkflowNodeKindExecution,
		EventType:          taskverification.TaskEventTypePlanningCompleted,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            payload,
	})
	assert.NilError(t, err)

	rows, err := store.TaskEventRowsByTaskRunID("pg-run-1")
	assert.NilError(t, err)
	assert.Equal(t, len(rows), 1)
	assert.Equal(t, rows[0].PrincipalID, "principal-1")

	// Then: the JSONB payload round-trips with the same semantic content. jsonb
	// canonicalizes key order/whitespace, so compare unmarshaled values, not bytes.
	assertJSONEqual(t, payload, rows[0].PayloadJSON)
}

func TestPostgres_TaskRunSnapshotRoundTrip(t *testing.T) {
	// Given: a freshly bootstrapped Postgres event store
	store := newPostgresTestStore(t)
	ctx := context.Background()

	// When: a task-run snapshot is upserted (exercises ON CONFLICT upsert + JSONB)
	snapshot := &taskruns.PersistedRunSnapshot{
		RunID:        "pg-run-1",
		TemplateID:   "template-1",
		TemplateName: "Template One",
		Status:       "active",
		Phase:        "planning",
	}
	assert.NilError(t, store.UpsertTaskRunSnapshot(ctx, snapshot))

	// And: upserted again to confirm DO UPDATE works on Postgres
	snapshot.Status = "completed"
	assert.NilError(t, store.UpsertTaskRunSnapshot(ctx, snapshot))

	// Then: the latest snapshot is returned
	got, err := store.GetTaskRunSnapshot(ctx, "pg-run-1")
	assert.NilError(t, err)
	assert.Equal(t, got.Status, "completed")
	assert.Equal(t, got.Phase, "planning")
	assert.Equal(t, got.TemplateID, "template-1")
}

func assertJSONEqual(t *testing.T, want, got json.RawMessage) {
	t.Helper()
	var wantVal, gotVal interface{}
	assert.NilError(t, json.Unmarshal(want, &wantVal))
	assert.NilError(t, json.Unmarshal(got, &gotVal))
	if !reflect.DeepEqual(wantVal, gotVal) {
		t.Fatalf("JSON payload mismatch:\n want %s\n got  %s", want, got)
	}
}
