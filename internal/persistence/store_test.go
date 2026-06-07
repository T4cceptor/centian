package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/identifiers"
	"github.com/T4cceptor/centian/internal/taskruns"
	"github.com/T4cceptor/centian/internal/taskverification"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
	"gotest.tools/assert"
)

func TestNewSQLiteStoreBootstrapsAndPersistsRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.sqlite")
	store, err := NewSQLiteStore(path)
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})

	err = store.AppendTaskEvent(&taskverification.TaskEvent{
		ID:                 "task-event-1",
		SchemaVersion:      1,
		CreatedAtUnixMilli: time.Now().UTC().UnixMilli(),
		TaskRunID:          "run-1",
		SessionID:          "session-1",
		TemplateID:         "template-1",
		PrincipalID:        "principal-1",
		PhasePath:          taskverification.TaskPhasePlanning,
		NodeKind:           taskverification.WorkflowNodeKindPlanning,
		ResultingPhasePath: taskverification.TaskPhase("execution.step_one"),
		ResultingNodeKind:  taskverification.WorkflowNodeKindExecution,
		EventType:          taskverification.TaskEventTypePlanningCompleted,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            json.RawMessage(`{"testTarget":"pytest -q"}`),
	})
	assert.NilError(t, err)

	err = store.AppendActionEventTaskContext(taskverification.ActionEventTaskContext{
		RequestID:           "action-1",
		TaskRunID:           "run-1",
		InvocationPhasePath: taskverification.TaskPhasePlanning,
		InvocationNodeKind:  taskverification.WorkflowNodeKindPlanning,
		CreatedAtUnixMilli:  time.Now().UTC().UnixMilli(),
	})
	assert.NilError(t, err)

	entry := &common.LogEntry{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:   time.Now().UTC(),
			RequestID:   "action-1",
			SessionID:   "session-1",
			Transport:   "http",
			MessageType: common.MessageTypeRequest,
			Direction:   common.DirectionClientToServer,
			Success:     true,
			Metadata: map[string]string{
				"principal_id": "principal-1",
			},
		},
		Routing: common.RoutingContext{
			Gateway:    "gw",
			ServerName: "server-a",
			Endpoint:   "/mcp/gw",
		},
	}
	entry.WithToolRequest("shell__exec", "shell__exec", json.RawMessage(`{"command":"pwd"}`))
	entry.WithToolResult(json.RawMessage(`{"content":[{"type":"text","text":"ok"}]}`), false)
	err = store.AppendActionEvent(entry)
	assert.NilError(t, err)

	responseEntry := &common.LogEntry{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:   time.Now().UTC(),
			RequestID:   "action-1",
			SessionID:   "session-1",
			Transport:   "http",
			MessageType: common.MessageTypeResponse,
			Direction:   common.DirectionServerToClient,
			Success:     true,
			Metadata: map[string]string{
				"principal_id": "principal-1",
			},
		},
		Routing: common.RoutingContext{
			Gateway:    "gw",
			ServerName: "server-a",
			Endpoint:   "/mcp/gw",
		},
	}
	responseEntry.WithToolRequest("shell__exec", "shell__exec", json.RawMessage(`{"command":"pwd"}`))
	responseEntry.WithToolResult(json.RawMessage(`{"content":[{"type":"text","text":"ok"}]}`), false)
	err = store.AppendActionEvent(responseEntry)
	assert.NilError(t, err)

	taskEvents, err := store.TaskEventRowsByTaskRunID("run-1")
	assert.NilError(t, err)
	assert.Equal(t, len(taskEvents), 1)
	assert.Equal(t, taskEvents[0].PrincipalID, "principal-1")
	assert.Equal(t, taskEvents[0].ResultingPhasePath, "execution.step_one")
	assert.Equal(t, taskEvents[0].ResultingNodeKind, string(taskverification.WorkflowNodeKindExecution))

	contexts, err := store.ActionEventRowsByTaskRunID("run-1")
	assert.NilError(t, err)
	assert.Equal(t, len(contexts), 1)
	assert.Equal(t, contexts[0].RequestID, "action-1")
	assert.Equal(t, contexts[0].InvocationPhasePath, string(taskverification.TaskPhasePlanning))

	actionEvents, err := store.ActionEventsByRequestID("action-1")
	assert.NilError(t, err)
	assert.Equal(t, len(actionEvents), 2)
	assert.Equal(t, actionEvents[0].ToolName, "shell__exec")
	assert.Equal(t, actionEvents[0].PrincipalID, "principal-1")
	assert.Assert(t, actionEvents[0].ID != actionEvents[1].ID)
	assert.Assert(t, identifiers.IsKind(actionEvents[0].ID, identifiers.KindActionEvent))
	assert.Assert(t, identifiers.IsKind(actionEvents[1].ID, identifiers.KindActionEvent))
	assert.Equal(t, actionEvents[0].Direction, string(common.DirectionClientToServer))
	assert.Equal(t, actionEvents[0].MessageType, string(common.MessageTypeRequest))
	assert.Equal(t, actionEvents[1].Direction, string(common.DirectionServerToClient))
	assert.Equal(t, actionEvents[1].MessageType, string(common.MessageTypeResponse))

	db, err := sql.Open(sqliteshim.ShimName, path)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	annotationColumns, err := tableColumns(db, "event_annotations")
	assert.NilError(t, err)
	assert.Assert(t, annotationColumns["action_event_id"])
	assert.Assert(t, annotationColumns["type"])
	assert.Assert(t, annotationColumns["category"])
	assert.Assert(t, annotationColumns["raw_json"])
	assertSQLiteIndexExists(t, db, "idx_event_annotations_action_event_id")
	assertSQLiteIndexExists(t, db, "idx_event_annotations_request_id")
	assertSQLiteIndexExists(t, db, "idx_event_annotations_created_at")
}

func TestNewSQLiteStoreMemoryBootstrapsAndPersistsRows(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})

	err = store.AppendTaskEvent(&taskverification.TaskEvent{
		ID:                 "memory-task-event-1",
		SchemaVersion:      1,
		CreatedAtUnixMilli: time.Now().UTC().UnixMilli(),
		TaskRunID:          "memory-run-1",
		SessionID:          "memory-session-1",
		TemplateID:         "memory-template-1",
		PrincipalID:        "memory-principal-1",
		PhasePath:          taskverification.TaskPhasePlanning,
		NodeKind:           taskverification.WorkflowNodeKindPlanning,
		ResultingPhasePath: taskverification.TaskPhase("execution.memory_step"),
		ResultingNodeKind:  taskverification.WorkflowNodeKindExecution,
		EventType:          taskverification.TaskEventTypePlanningCompleted,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            json.RawMessage(`{"ok":true}`),
	})
	assert.NilError(t, err)

	rows, err := store.TaskEventRowsByTaskRunID("memory-run-1")
	assert.NilError(t, err)
	assert.Equal(t, len(rows), 1)
	assert.Equal(t, rows[0].TemplateID, "memory-template-1")
}

func TestAppendActionEventPersistsAnnotationsSeparately(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})

	err = store.AppendActionEventTaskContext(taskverification.ActionEventTaskContext{
		RequestID:           "annotated-1",
		TaskRunID:           "run-annotated",
		InvocationPhasePath: taskverification.TaskPhaseExecution,
		InvocationNodeKind:  taskverification.WorkflowNodeKindExecution,
		CreatedAtUnixMilli:  1000,
	})
	assert.NilError(t, err)

	entry := &common.LogEntry{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:   time.UnixMilli(1000).UTC(),
			RequestID:   "annotated-1",
			SessionID:   "session-1",
			Transport:   "http",
			MessageType: common.MessageTypeResponse,
			Direction:   common.DirectionServerToClient,
			Success:     true,
			Metadata: map[string]string{
				"principal_id":          "principal-1",
				"processor_annotations": "legacy-value",
			},
		},
		Routing: common.RoutingContext{
			Gateway:    "gw",
			ServerName: "server-a",
			Endpoint:   "/mcp/gw",
		},
		Annotations: []common.EventAnnotation{
			{
				Type:      "governance_events",
				Processor: "prompt_injection_guard",
				Action:    "redacted",
				Category:  "security",
				Severity:  "high",
				Message:   "Suspicious tool result content was redacted.",
				Findings: []common.EventAnnotationFinding{
					{Rule: "role_marker", Path: "payload.result.content[0].text"},
					{Rule: "secret_exfiltration", Path: "payload.result.content[1].text"},
				},
			},
		},
	}
	entry.WithToolRequest("search", "search", json.RawMessage(`{"query":"test"}`))
	entry.WithToolResult(json.RawMessage(`{"content":[{"type":"text","text":"ok"}]}`), false)

	err = store.AppendActionEvent(entry)
	assert.NilError(t, err)

	actionEvents, err := store.ActionEventsByRequestID("annotated-1")
	assert.NilError(t, err)
	assert.Equal(t, len(actionEvents), 1)

	var payload map[string]any
	assert.NilError(t, json.Unmarshal(actionEvents[0].PayloadJSON, &payload))
	_, topLevelAnnotations := payload["annotations"]
	assert.Assert(t, !topLevelAnnotations)
	metadata := payload["metadata"].(map[string]any)
	assert.Equal(t, metadata["principal_id"], "principal-1")
	_, legacyAnnotations := metadata["processor_annotations"]
	assert.Assert(t, !legacyAnnotations)

	rows := make([]eventAnnotationRow, 0)
	err = store.DB().NewSelect().
		Model(&rows).
		Where("action_event_id = ?", actionEvents[0].ID).
		Order("path ASC").
		Scan(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, len(rows), 2)
	assert.Equal(t, rows[0].Processor, "prompt_injection_guard")
	assert.Equal(t, rows[0].Type, "governance_events")
	assert.Equal(t, rows[0].Action, "redacted")
	assert.Equal(t, rows[0].Category, "security")
	assert.Equal(t, rows[0].Severity, "high")
	assert.Assert(t, rows[0].Rule != "")
	assert.Assert(t, rows[0].RawJSON != nil)

	eventPage, err := store.ListEvents(context.Background(), &EventListFilter{RequestID: "annotated-1"})
	assert.NilError(t, err)
	assert.Equal(t, len(eventPage.Items), 1)
	assert.Equal(t, len(eventPage.Items[0].Annotations), 2)
	assert.Equal(t, eventPage.Items[0].Annotations[0].Type, "governance_events")
	assert.Equal(t, eventPage.Items[0].Annotations[0].Category, "security")
	assert.Equal(t, eventPage.Items[0].Annotations[0].Processor, "prompt_injection_guard")
	assert.Equal(t, len(eventPage.Items[0].Annotations[0].Findings), 1)

	taskEvents, err := store.GetTaskRunEvents(context.Background(), "run-annotated")
	assert.NilError(t, err)
	assert.Equal(t, len(taskEvents), 1)
	assert.Equal(t, len(taskEvents[0].Annotations), 2)
	assert.Equal(t, taskEvents[0].Annotations[0].Type, "governance_events")
	assert.Equal(t, taskEvents[0].Annotations[0].Category, "security")
	assert.Equal(t, taskEvents[0].Annotations[0].Action, "redacted")
}

func TestNewSQLiteStoreBootstrapIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.sqlite")

	storeA, err := NewSQLiteStore(path)
	assert.NilError(t, err)
	assert.NilError(t, storeA.Close())

	storeB, err := NewSQLiteStore(path)
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = storeB.Close()
	})

	assert.Assert(t, storeB.DB() != nil)
}

func TestNewSQLiteStoreMigratesMainSchemaV4ToCurrentSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.sqlite")

	db, err := sql.Open(sqliteshim.ShimName, path)
	assert.NilError(t, err)
	seedSchemaVersion4Store(t, db)
	assert.NilError(t, db.Close())

	store, err := NewSQLiteStore(path)
	assert.NilError(t, err)
	assert.NilError(t, store.Close())

	db, err = sql.Open(sqliteshim.ShimName, path)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	columns, err := tableColumns(db, "benchmark_runs")
	assert.NilError(t, err)
	assert.Assert(t, columns["agent_metadata_json"])
	assert.Assert(t, columns["case_name"])
	assert.Assert(t, columns["template_name"])
	sessionColumns, err := tableColumns(db, "benchmark_sessions")
	assert.NilError(t, err)
	assert.Assert(t, sessionColumns["suite_name"])
	assert.Assert(t, sessionColumns["template_name"])
	scoreColumns, err := tableColumns(db, "benchmark_run_scores")
	assert.NilError(t, err)
	assert.Assert(t, scoreColumns["benchmark_run_id"])
	assert.Assert(t, scoreColumns["score_status"])

	var version int
	err = db.QueryRow(`SELECT version FROM event_store_schema WHERE name = 'event_storage'`).Scan(&version)
	assert.NilError(t, err)
	assert.Equal(t, version, schemaVersion)
}

func TestNewSQLiteStoreMigratesBenchmarkLinksV5ToV6(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.sqlite")

	db, err := sql.Open(sqliteshim.ShimName, path)
	assert.NilError(t, err)
	seedSchemaVersion5Store(t, db)
	assert.NilError(t, db.Close())

	store, err := NewSQLiteStore(path)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	runs, err := store.ListBenchmarkRuns(context.Background(), &BenchmarkRunFilter{SuiteID: "simple_tdd_v1"})
	assert.NilError(t, err)
	assert.Equal(t, len(runs), 1)
	assert.DeepEqual(t, runs[0].LinkedTaskRunIDs, []string{"tr_1", "tr_2", "tr_3"})

	links, err := store.ListBenchmarkRunTaskRunLinks(context.Background(), &BenchmarkRunTaskRunLinkFilter{
		BenchmarkRunID: "bm_run_1",
	})
	assert.NilError(t, err)
	assert.Equal(t, len(links), 3)
	assert.Equal(t, links[0].TaskRunID, "tr_1")
	assert.Equal(t, *links[0].LinkOrder, 0)
	assert.Equal(t, links[1].TaskRunID, "tr_2")
	assert.Equal(t, *links[1].LinkOrder, 1)
	assert.Equal(t, links[2].TaskRunID, "tr_3")
	assert.Equal(t, *links[2].LinkOrder, 2)

	score, err := store.GetBenchmarkRunScore(context.Background(), "bm_run_1")
	assert.NilError(t, err)
	assert.Equal(t, score.BenchmarkRunID, "bm_run_1")

	db, err = sql.Open(sqliteshim.ShimName, path)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	columns, err := tableColumns(db, "benchmark_runs")
	assert.NilError(t, err)
	assert.Assert(t, !columns["latest_task_run_id"])
	assert.Assert(t, !columns["latest_task_run_status"])
	assert.Assert(t, !columns["linked_task_run_ids_json"])

	legacyCount, err := sqliteMasterCount(db, "table", "benchmark_runs_v5_legacy")
	assert.NilError(t, err)
	assert.Equal(t, legacyCount, 0)

	linkCount, err := sqliteMasterCount(db, "table", "benchmark_run_task_runs")
	assert.NilError(t, err)
	assert.Equal(t, linkCount, 1)

	var version int
	err = db.QueryRow(`SELECT version FROM event_store_schema WHERE name = 'event_storage'`).Scan(&version)
	assert.NilError(t, err)
	assert.Equal(t, version, schemaVersion)
}

func TestNewSQLiteStoreMigratesAnnotationsV6ToCurrentSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.sqlite")

	db, err := sql.Open(sqliteshim.ShimName, path)
	assert.NilError(t, err)
	seedSchemaVersion6Store(t, db)
	assert.NilError(t, db.Close())

	store, err := NewSQLiteStore(path)
	assert.NilError(t, err)
	assert.NilError(t, store.Close())

	db, err = sql.Open(sqliteshim.ShimName, path)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	columns, err := tableColumns(db, "event_annotations")
	assert.NilError(t, err)
	assert.Assert(t, columns["action_event_id"])
	assert.Assert(t, columns["request_id"])
	assert.Assert(t, columns["type"])
	assert.Assert(t, columns["category"])
	assert.Assert(t, columns["raw_json"])
	assertSQLiteIndexExists(t, db, "idx_event_annotations_action_event_id")
	assertSQLiteIndexExists(t, db, "idx_event_annotations_request_id")
	assertSQLiteIndexExists(t, db, "idx_event_annotations_created_at")

	var version int
	err = db.QueryRow(`SELECT version FROM event_store_schema WHERE name = 'event_storage'`).Scan(&version)
	assert.NilError(t, err)
	assert.Equal(t, version, schemaVersion)
}

func TestNewSQLiteStoreFailsV5ToV6MigrationOnMissingTaskRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.sqlite")

	db, err := sql.Open(sqliteshim.ShimName, path)
	assert.NilError(t, err)
	seedSchemaVersion5Store(t, db)
	_, err = db.Exec(`DELETE FROM task_runs WHERE run_id = 'tr_3'`)
	assert.NilError(t, err)
	assert.NilError(t, db.Close())

	store, err := NewSQLiteStore(path)
	assert.Assert(t, store == nil)
	assert.ErrorContains(t, err, "legacy benchmark run bm_run_1 references missing task run tr_3")
}

func TestNewSQLiteStoreRejectsIntermediateBranchSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.sqlite")

	db, err := sql.Open(sqliteshim.ShimName, path)
	assert.NilError(t, err)
	_, err = db.Exec(`CREATE TABLE event_store_schema (name TEXT PRIMARY KEY, version INTEGER NOT NULL)`)
	assert.NilError(t, err)
	_, err = db.Exec(`INSERT INTO event_store_schema(name, version) VALUES ('event_storage', 9)`)
	assert.NilError(t, err)
	_, err = db.Exec(`CREATE TABLE benchmark_runs (
		benchmark_run_id TEXT PRIMARY KEY,
		schema_version INTEGER NOT NULL,
		session_id TEXT NOT NULL,
		case_id TEXT NOT NULL,
		agent TEXT NOT NULL,
		template_variant TEXT NOT NULL,
		attempt INTEGER NOT NULL,
		template_id TEXT NOT NULL,
		selected_model TEXT,
		started_at_unix_milli INTEGER NOT NULL,
		ended_at_unix_milli INTEGER,
		status TEXT NOT NULL,
		latest_task_run_id TEXT,
		latest_task_run_status TEXT,
		linked_task_run_ids_json BLOB NOT NULL,
		run_dir TEXT NOT NULL,
		project_dir TEXT NOT NULL,
		logs_dir TEXT NOT NULL,
		agent_dir TEXT NOT NULL,
		config_path TEXT NOT NULL,
		event_store_mode TEXT,
		event_store_path TEXT,
		request_log_path TEXT,
		selected_template_path TEXT,
		error_summary TEXT
	)`)
	assert.NilError(t, err)
	assert.NilError(t, db.Close())

	store, err := NewSQLiteStore(path)
	assert.Assert(t, store == nil)
	var migrationErr *SchemaMigrationRequiredError
	assert.Assert(t, errors.As(err, &migrationErr))
	assert.Equal(t, migrationErr.StoredVersion, 9)
	assert.Equal(t, migrationErr.ExpectedVersion, schemaVersion)

	db, err = sql.Open(sqliteshim.ShimName, path)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	columns, err := tableColumns(db, "benchmark_runs")
	assert.NilError(t, err)
	assert.Assert(t, !columns["agent_metadata_json"])
	scoreColumns, err := tableColumns(db, "benchmark_run_scores")
	assert.NilError(t, err)
	assert.Equal(t, len(scoreColumns), 0)

	var version int
	err = db.QueryRow(`SELECT version FROM event_store_schema WHERE name = 'event_storage'`).Scan(&version)
	assert.NilError(t, err)
	assert.Equal(t, version, 9)
}

func TestNewSQLiteStoreRejectsMismatchedSchemaWithoutDroppingData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.sqlite")

	db, err := sql.Open(sqliteshim.ShimName, path)
	assert.NilError(t, err)
	_, err = db.Exec(`CREATE TABLE event_store_schema (name TEXT PRIMARY KEY, version INTEGER NOT NULL)`)
	assert.NilError(t, err)
	_, err = db.Exec(`INSERT INTO event_store_schema(name, version) VALUES ('event_storage', 1)`)
	assert.NilError(t, err)
	_, err = db.Exec(`CREATE TABLE action_events (
		id TEXT PRIMARY KEY,
		schema_version INTEGER NOT NULL,
		created_at_unix_milli INTEGER NOT NULL,
		request_id TEXT NOT NULL,
		session_id TEXT,
		principal_id TEXT,
		transport TEXT,
		gateway TEXT,
		server_name TEXT,
		endpoint TEXT,
		tool_name TEXT,
		original_tool_name TEXT,
		success BOOLEAN NOT NULL,
		is_error BOOLEAN NOT NULL,
		payload_json BLOB
	)`)
	assert.NilError(t, err)
	assert.NilError(t, db.Close())

	store, err := NewSQLiteStore(path)
	assert.Assert(t, store == nil)
	var migrationErr *SchemaMigrationRequiredError
	assert.Assert(t, errors.As(err, &migrationErr))
	assert.Equal(t, migrationErr.StoredVersion, 1)
	assert.Equal(t, migrationErr.ExpectedVersion, schemaVersion)

	db, err = sql.Open(sqliteshim.ShimName, path)
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM action_events`).Scan(&count)
	assert.NilError(t, err)
	assert.Equal(t, count, 0)
}

func TestBenchmarkSessionAndRunUpsertAndList(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	session := &BenchmarkSessionRecord{
		SessionID:          "bm_session_1",
		SuiteID:            "simple_tdd_v1",
		SuitePath:          "/tmp/suite",
		SessionPath:        "/tmp/session",
		OutputRoot:         "/tmp",
		TemplateID:         "simple_tdd",
		StartedAtUnixMilli: 1000,
		Status:             "completed",
		RepeatCount:        1,
	}
	assert.NilError(t, store.UpsertBenchmarkSession(context.Background(), session))

	run := &BenchmarkRunRecord{
		BenchmarkRunID:     "bm_run_1",
		SessionID:          session.SessionID,
		CaseID:             "assertion_failure_red",
		Agent:              "codex",
		TemplateVariant:    "current",
		Attempt:            1,
		TemplateID:         "simple_tdd",
		StartedAtUnixMilli: 1000,
		Status:             "completed",
		LinkedTaskRunIDs:   []string{"tr_1"},
		RunDir:             "/tmp/session/run",
		ProjectDir:         "/tmp/session/run/project",
		LogsDir:            "/tmp/session/run/logs",
		AgentDir:           "/tmp/session/run/agent",
		ConfigPath:         "/tmp/session/run/config.json",
		EventStorePath:     "/tmp/events.sqlite",
		AgentMetadataJSON:  json.RawMessage(`{"format":"codex_jsonl","threadId":"thread_1"}`),
	}
	assert.NilError(t, store.UpsertBenchmarkRun(context.Background(), run))

	run.Status = "failed"
	run.ErrorSummary = "agent failed"
	assert.NilError(t, store.UpsertBenchmarkRun(context.Background(), run))

	sessions, err := store.ListBenchmarkSessions(context.Background(), BenchmarkSessionFilter{SuiteID: "simple_tdd_v1"})
	assert.NilError(t, err)
	assert.Equal(t, len(sessions), 1)
	assert.Equal(t, sessions[0].SessionID, session.SessionID)

	runs, err := store.ListBenchmarkRuns(context.Background(), &BenchmarkRunFilter{SuiteID: "simple_tdd_v1", Agent: "codex"})
	assert.NilError(t, err)
	assert.Equal(t, len(runs), 1)
	assert.Equal(t, runs[0].BenchmarkRunID, run.BenchmarkRunID)
	assert.Equal(t, runs[0].Status, "failed")
	assert.DeepEqual(t, runs[0].LinkedTaskRunIDs, []string{"tr_1"})
	assert.Equal(t, string(runs[0].AgentMetadataJSON), `{"format":"codex_jsonl","threadId":"thread_1"}`)
}

func TestStoreReadMethodsReturnErrorsWhenDatabaseIsClosed(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	assert.NilError(t, store.Close())

	_, err = store.TaskEvents()
	assert.Assert(t, err != nil)

	_, err = store.ActionEventTaskContexts()
	assert.Assert(t, err != nil)

	_, err = store.ActionEvents()
	assert.Assert(t, err != nil)

	_, err = store.ActionEventsByRequestID("request-1")
	assert.Assert(t, err != nil)

	_, err = store.ActionEventRowsByTaskRunID("run-1")
	assert.Assert(t, err != nil)

	_, err = store.TaskEventRowsByTaskRunID("run-1")
	assert.Assert(t, err != nil)
}

func TestListTaskRunsAggregatesSummaries(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})

	// Given: three task runs with active, completed, and failed terminal states.
	seedTaskEvent(t, store, &taskverification.TaskEvent{
		ID:                 "run-active-1",
		SchemaVersion:      1,
		CreatedAtUnixMilli: 1_000,
		TaskRunID:          "run-active",
		SessionID:          "session-active",
		TemplateID:         "template-active",
		PrincipalID:        "principal-active",
		PhasePath:          taskverification.TaskPhaseOnboarding,
		NodeKind:           taskverification.WorkflowNodeKindOnboarding,
		ResultingPhasePath: taskverification.TaskPhasePlanning,
		ResultingNodeKind:  taskverification.WorkflowNodeKindPlanning,
		EventType:          taskverification.TaskEventTypeOnboardingCompleted,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            json.RawMessage(`{"status":"active"}`),
	})
	seedTaskEvent(t, store, &taskverification.TaskEvent{
		ID:                 "run-active-2",
		SchemaVersion:      1,
		CreatedAtUnixMilli: 1_100,
		TaskRunID:          "run-active",
		SessionID:          "session-active",
		TemplateID:         "template-active",
		PrincipalID:        "principal-active",
		PhasePath:          taskverification.TaskPhasePlanning,
		NodeKind:           taskverification.WorkflowNodeKindPlanning,
		ResultingPhasePath: taskverification.TaskPhase("execution.step_one"),
		ResultingNodeKind:  taskverification.WorkflowNodeKindExecution,
		EventType:          taskverification.TaskEventTypePlanningCompleted,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            json.RawMessage(`{"status":"active"}`),
	})

	seedTaskEvent(t, store, &taskverification.TaskEvent{
		ID:                 "run-completed-1",
		SchemaVersion:      1,
		CreatedAtUnixMilli: 2_000,
		TaskRunID:          "run-completed",
		SessionID:          "session-completed",
		TemplateID:         "template-completed",
		PrincipalID:        "principal-completed",
		PhasePath:          taskverification.TaskPhasePlanning,
		NodeKind:           taskverification.WorkflowNodeKindPlanning,
		ResultingPhasePath: taskverification.TaskPhaseExecution,
		ResultingNodeKind:  taskverification.WorkflowNodeKindExecution,
		EventType:          taskverification.TaskEventTypePlanningCompleted,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            json.RawMessage(`{"status":"active"}`),
	})
	seedTaskEvent(t, store, &taskverification.TaskEvent{
		ID:                 "run-completed-2b",
		SchemaVersion:      1,
		CreatedAtUnixMilli: 2_100,
		TaskRunID:          "run-completed",
		SessionID:          "session-completed",
		TemplateID:         "template-completed",
		PrincipalID:        "principal-completed",
		PhasePath:          taskverification.TaskPhaseExecution,
		NodeKind:           taskverification.WorkflowNodeKindExecution,
		ResultingPhasePath: taskverification.TaskPhaseExecution,
		ResultingNodeKind:  taskverification.WorkflowNodeKindExecution,
		EventType:          taskverification.TaskEventTypeStepCompleted,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            json.RawMessage(`{"status":"completed"}`),
	})
	seedTaskEvent(t, store, &taskverification.TaskEvent{
		ID:                 "run-completed-2a",
		SchemaVersion:      1,
		CreatedAtUnixMilli: 2_100,
		TaskRunID:          "run-completed",
		SessionID:          "session-completed",
		TemplateID:         "template-completed",
		PrincipalID:        "principal-completed",
		PhasePath:          taskverification.TaskPhaseExecution,
		NodeKind:           taskverification.WorkflowNodeKindExecution,
		ResultingPhasePath: taskverification.TaskPhase("execution.older"),
		ResultingNodeKind:  taskverification.WorkflowNodeKindExecution,
		EventType:          taskverification.TaskEventTypeStepCompleted,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            json.RawMessage(`{"status":"active"}`),
	})

	seedTaskEvent(t, store, &taskverification.TaskEvent{
		ID:                 "run-failed-1",
		SchemaVersion:      1,
		CreatedAtUnixMilli: 1_500,
		TaskRunID:          "run-failed",
		SessionID:          "session-failed",
		TemplateID:         "template-failed",
		PrincipalID:        "principal-failed",
		PhasePath:          taskverification.TaskPhasePlanning,
		NodeKind:           taskverification.WorkflowNodeKindPlanning,
		ResultingPhasePath: taskverification.TaskPhase("execution.step_one"),
		ResultingNodeKind:  taskverification.WorkflowNodeKindExecution,
		EventType:          taskverification.TaskEventTypePlanningCompleted,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            json.RawMessage(`{"status":"active"}`),
	})
	seedTaskEvent(t, store, &taskverification.TaskEvent{
		ID:                 "run-failed-2",
		SchemaVersion:      1,
		CreatedAtUnixMilli: 1_800,
		TaskRunID:          "run-failed",
		SessionID:          "session-failed",
		TemplateID:         "template-failed",
		PrincipalID:        "principal-failed",
		PhasePath:          taskverification.TaskPhase("execution.step_one"),
		NodeKind:           taskverification.WorkflowNodeKindExecution,
		ResultingPhasePath: taskverification.TaskPhase("execution.step_one"),
		ResultingNodeKind:  taskverification.WorkflowNodeKindExecution,
		EventType:          taskverification.TaskEventTypeFailed,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            json.RawMessage(`{"reason":"stuck"}`),
	})

	seedActionContext(t, store, taskverification.ActionEventTaskContext{
		RequestID:           "request-active",
		TaskRunID:           "run-active",
		InvocationPhasePath: taskverification.TaskPhasePlanning,
		InvocationNodeKind:  taskverification.WorkflowNodeKindPlanning,
		CreatedAtUnixMilli:  1_105,
	})
	seedActionEvent(t, store, &ActionEventRecord{
		ID:                 "action-active-1",
		SchemaVersion:      schemaVersion,
		CreatedAtUnixMilli: 1_106,
		RequestID:          "request-active",
		SessionID:          "session-active",
		PrincipalID:        "principal-active",
		Transport:          "http",
		Direction:          string(common.DirectionClientToServer),
		MessageType:        string(common.MessageTypeRequest),
		Gateway:            "gw",
		ServerName:         "server-a",
		Endpoint:           "/mcp/gw",
		ToolName:           "shell__exec",
		OriginalToolName:   "shell__exec",
		Success:            true,
		IsError:            false,
		PayloadJSON:        json.RawMessage(`{"request":"active"}`),
	})

	seedActionContext(t, store, taskverification.ActionEventTaskContext{
		RequestID:           "request-completed",
		TaskRunID:           "run-completed",
		InvocationPhasePath: taskverification.TaskPhaseExecution,
		InvocationNodeKind:  taskverification.WorkflowNodeKindExecution,
		CreatedAtUnixMilli:  2_050,
	})
	seedActionEvent(t, store, &ActionEventRecord{
		ID:                 "action-completed-1",
		SchemaVersion:      schemaVersion,
		CreatedAtUnixMilli: 2_060,
		RequestID:          "request-completed",
		SessionID:          "session-completed",
		PrincipalID:        "principal-completed",
		Transport:          "http",
		Direction:          string(common.DirectionClientToServer),
		MessageType:        string(common.MessageTypeRequest),
		Gateway:            "gw",
		ServerName:         "server-a",
		Endpoint:           "/mcp/gw",
		ToolName:           "shell__exec",
		OriginalToolName:   "shell__exec",
		Success:            true,
		IsError:            false,
		PayloadJSON:        json.RawMessage(`{"request":"completed"}`),
	})
	seedActionEvent(t, store, &ActionEventRecord{
		ID:                 "action-completed-2",
		SchemaVersion:      schemaVersion,
		CreatedAtUnixMilli: 2_070,
		RequestID:          "request-completed",
		SessionID:          "session-completed",
		PrincipalID:        "principal-completed",
		Transport:          "http",
		Direction:          string(common.DirectionServerToClient),
		MessageType:        string(common.MessageTypeResponse),
		Gateway:            "gw",
		ServerName:         "server-a",
		Endpoint:           "/mcp/gw",
		ToolName:           "shell__exec",
		OriginalToolName:   "shell__exec",
		Success:            true,
		IsError:            false,
		PayloadJSON:        json.RawMessage(`{"response":"completed"}`),
	})
	seedActionContext(t, store, taskverification.ActionEventTaskContext{
		RequestID:           "request-missing-action",
		TaskRunID:           "run-completed",
		InvocationPhasePath: taskverification.TaskPhaseExecution,
		InvocationNodeKind:  taskverification.WorkflowNodeKindExecution,
		CreatedAtUnixMilli:  2_080,
	})

	// When: task run summaries are queried.
	summaries, err := store.ListTaskRuns(context.Background(), TaskRunFilter{})
	assert.NilError(t, err)

	// Then: runs are ordered newest-first with aggregated fields populated.
	assert.Equal(t, len(summaries), 3)
	assert.Equal(t, summaries[0].RunID, "run-completed")
	assert.Equal(t, summaries[1].RunID, "run-failed")
	assert.Equal(t, summaries[2].RunID, "run-active")

	assert.Equal(t, summaries[0].TemplateID, "template-completed")
	assert.Equal(t, summaries[0].Status, string(taskverification.TaskStatusCompleted))
	assert.Equal(t, summaries[0].CurrentPhase, string(taskverification.TaskPhaseExecution))
	assert.Equal(t, summaries[0].CurrentNodeKind, string(taskverification.WorkflowNodeKindExecution))
	assert.Equal(t, summaries[0].TaskEventCount, 3)
	assert.Equal(t, summaries[0].ActionEventCount, 2)
	assert.Equal(t, summaries[0].EventCount, 5)
	assert.Assert(t, summaries[0].EndedAt != nil)
	assert.Equal(t, *summaries[0].EndedAt, int64(2_100))

	assert.Equal(t, summaries[1].Status, string(taskverification.TaskStatusFailed))
	assert.Equal(t, summaries[1].TaskEventCount, 2)
	assert.Equal(t, summaries[1].ActionEventCount, 0)
	assert.Assert(t, summaries[1].EndedAt != nil)
	assert.Equal(t, *summaries[1].EndedAt, int64(1_800))

	assert.Equal(t, summaries[2].Status, string(taskverification.TaskStatusActive))
	assert.Equal(t, summaries[2].CurrentPhase, "execution.step_one")
	assert.Equal(t, summaries[2].TaskEventCount, 2)
	assert.Equal(t, summaries[2].ActionEventCount, 1)
	assert.Equal(t, summaries[2].EventCount, 3)
	assert.Assert(t, summaries[2].EndedAt == nil)
}

func TestListTaskRunsKeepsTimedOutRunsOpen(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})

	seedTaskEvent(t, store, &taskverification.TaskEvent{
		ID:                 "run-timeout-1",
		SchemaVersion:      1,
		CreatedAtUnixMilli: 1_000,
		TaskRunID:          "run-timeout",
		SessionID:          "session-timeout",
		TemplateID:         "template-timeout",
		PrincipalID:        "principal-timeout",
		PhasePath:          taskverification.TaskPhasePlanning,
		NodeKind:           taskverification.WorkflowNodeKindPlanning,
		ResultingPhasePath: taskverification.TaskPhase("execution.step_one"),
		ResultingNodeKind:  taskverification.WorkflowNodeKindExecution,
		EventType:          taskverification.TaskEventTypePlanningCompleted,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            json.RawMessage(`{"status":"active"}`),
	})
	seedTaskEvent(t, store, &taskverification.TaskEvent{
		ID:                 "run-timeout-2",
		SchemaVersion:      1,
		CreatedAtUnixMilli: 2_000,
		TaskRunID:          "run-timeout",
		SessionID:          "session-timeout",
		TemplateID:         "template-timeout",
		PrincipalID:        "principal-timeout",
		PhasePath:          taskverification.TaskPhase("execution.step_one"),
		NodeKind:           taskverification.WorkflowNodeKindExecution,
		ResultingPhasePath: taskverification.TaskPhase("execution.step_one"),
		ResultingNodeKind:  taskverification.WorkflowNodeKindExecution,
		EventType:          taskverification.TaskEventTypeTimedOut,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            json.RawMessage(`{"status":"timed_out"}`),
	})

	summaries, err := store.ListTaskRuns(context.Background(), TaskRunFilter{})
	assert.NilError(t, err)
	assert.Equal(t, len(summaries), 1)
	assert.Equal(t, summaries[0].Status, string(taskverification.TaskStatusTimedOut))
	assert.Equal(t, summaries[0].CurrentPhase, "execution.step_one")
	assert.Assert(t, summaries[0].EndedAt == nil)
}

func TestListTaskRunsPrefersPersistedSnapshotMetadata(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})

	seedTaskEvent(t, store, &taskverification.TaskEvent{
		ID:                 "run-1",
		SchemaVersion:      1,
		CreatedAtUnixMilli: 1_000,
		TaskRunID:          "run-1",
		SessionID:          "session-1",
		TemplateID:         "simple_tdd",
		PrincipalID:        "principal-1",
		PhasePath:          taskverification.TaskPhasePlanning,
		NodeKind:           taskverification.WorkflowNodeKindPlanning,
		ResultingPhasePath: taskverification.TaskPhase("execution.step_one"),
		ResultingNodeKind:  taskverification.WorkflowNodeKindExecution,
		EventType:          taskverification.TaskEventTypePlanningCompleted,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            json.RawMessage(`{"status":"active"}`),
	})

	err = store.UpsertTaskRunSnapshot(context.Background(), &taskruns.PersistedRunSnapshot{
		RunID:        "run-1",
		TemplateID:   "simple_tdd",
		TemplateName: "Simple TDD Task",
		Status:       string(taskverification.TaskStatusCompleted),
		Phase:        "execution.refactor_while_green",
		SelectedTemplate: taskruns.PersistedTemplateSnapshot{
			Version: "0.1",
			Task:    taskruns.PersistedTaskSnapshot{ID: "simple_tdd", Name: "Simple TDD Task", Description: "desc"},
		},
	})
	assert.NilError(t, err)

	summaries, err := store.ListTaskRuns(context.Background(), TaskRunFilter{})
	assert.NilError(t, err)
	assert.Equal(t, len(summaries), 1)
	assert.Equal(t, summaries[0].TemplateName, "Simple TDD Task")
	assert.Equal(t, summaries[0].Status, string(taskverification.TaskStatusCompleted))
	assert.Equal(t, summaries[0].CurrentPhase, "execution.refactor_while_green")
	assert.Assert(t, summaries[0].EndedAt != nil)
}

func TestListTaskRunsIncludesSnapshotOnlyRuns(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})

	err = store.UpsertTaskRunSnapshot(context.Background(), &taskruns.PersistedRunSnapshot{
		RunID:        "run-only-snapshot",
		TemplateID:   "simple_tdd",
		TemplateName: "Simple TDD Task",
		Status:       string(taskverification.TaskStatusActive),
		Phase:        string(taskverification.TaskPhaseOnboarding),
		SelectedTemplate: taskruns.PersistedTemplateSnapshot{
			Version: "0.1",
			Task:    taskruns.PersistedTaskSnapshot{ID: "simple_tdd", Name: "Simple TDD Task", Description: "desc"},
		},
	})
	assert.NilError(t, err)

	summaries, err := store.ListTaskRuns(context.Background(), TaskRunFilter{})
	assert.NilError(t, err)
	assert.Equal(t, len(summaries), 1)
	assert.Equal(t, summaries[0].RunID, "run-only-snapshot")
	assert.Equal(t, summaries[0].TemplateName, "Simple TDD Task")
	assert.Equal(t, summaries[0].Status, string(taskverification.TaskStatusActive))
	assert.Equal(t, summaries[0].CurrentPhase, string(taskverification.TaskPhaseOnboarding))
	assert.Equal(t, summaries[0].EventCount, 0)
	assert.Assert(t, summaries[0].EndedAt == nil)
}

func TestGetTaskRunEventsReturnsUnifiedChronologicalTimeline(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})

	// Given: one run with task events, action events, and tied timestamps.
	seedTaskEvent(t, store, &taskverification.TaskEvent{
		ID:                     "task-1",
		SchemaVersion:          1,
		CreatedAtUnixMilli:     1_000,
		TaskRunID:              "run-1",
		SessionID:              "session-1",
		TemplateID:             "template-1",
		PrincipalID:            "principal-1",
		PhasePath:              taskverification.TaskPhaseOnboarding,
		NodeKind:               taskverification.WorkflowNodeKindOnboarding,
		ResultingPhasePath:     taskverification.TaskPhasePlanning,
		ResultingNodeKind:      taskverification.WorkflowNodeKindPlanning,
		EventType:              taskverification.TaskEventTypeRegistered,
		Outcome:                taskverification.TaskEventOutcomeSucceeded,
		RelatedActionRequestID: "request-1",
		Payload:                json.RawMessage(`{"status":"active"}`),
	})
	seedTaskEvent(t, store, &taskverification.TaskEvent{
		ID:                     "task-2",
		SchemaVersion:          1,
		CreatedAtUnixMilli:     1_050,
		TaskRunID:              "run-1",
		SessionID:              "session-1",
		TemplateID:             "template-1",
		PrincipalID:            "principal-1",
		PhasePath:              taskverification.TaskPhasePlanning,
		NodeKind:               taskverification.WorkflowNodeKindPlanning,
		ResultingPhasePath:     taskverification.TaskPhase("execution.step_one"),
		ResultingNodeKind:      taskverification.WorkflowNodeKindExecution,
		EventType:              taskverification.TaskEventTypePlanningCompleted,
		Outcome:                taskverification.TaskEventOutcomeSucceeded,
		RelatedActionRequestID: "request-2",
		Payload:                json.RawMessage(`{"status":"active"}`),
	})
	seedActionContext(t, store, taskverification.ActionEventTaskContext{
		RequestID:           "request-1",
		TaskRunID:           "run-1",
		InvocationPhasePath: taskverification.TaskPhaseOnboarding,
		InvocationNodeKind:  taskverification.WorkflowNodeKindOnboarding,
		CreatedAtUnixMilli:  1_001,
	})
	seedActionEvent(t, store, &ActionEventRecord{
		ID:                 "action-a",
		SchemaVersion:      schemaVersion,
		CreatedAtUnixMilli: 1_025,
		RequestID:          "request-1",
		SessionID:          "session-1",
		PrincipalID:        "principal-1",
		Transport:          "http",
		Direction:          string(common.DirectionClientToServer),
		MessageType:        string(common.MessageTypeRequest),
		Gateway:            "gw",
		ServerName:         "server-a",
		Endpoint:           "/mcp/gw",
		ToolName:           "shell__exec",
		OriginalToolName:   "shell__exec",
		Success:            true,
		IsError:            false,
		PayloadJSON:        json.RawMessage(`{"arguments":{"command":"pwd"}}`),
	})
	seedActionEvent(t, store, &ActionEventRecord{
		ID:                 "action-b",
		SchemaVersion:      schemaVersion,
		CreatedAtUnixMilli: 1_050,
		RequestID:          "request-1",
		SessionID:          "session-1",
		PrincipalID:        "principal-1",
		Transport:          "http",
		Direction:          string(common.DirectionServerToClient),
		MessageType:        string(common.MessageTypeResponse),
		Gateway:            "gw",
		ServerName:         "server-a",
		Endpoint:           "/mcp/gw",
		ToolName:           "shell__exec",
		OriginalToolName:   "shell__exec",
		Success:            false,
		IsError:            true,
		PayloadJSON:        json.RawMessage(`{"error":"failed"}`),
	})

	// When: the unified timeline is queried.
	events, err := store.GetTaskRunEvents(context.Background(), "run-1")
	assert.NilError(t, err)

	// Then: task and action rows are returned in chronological order with stable tie ordering.
	assert.Equal(t, len(events), 4)
	assert.Equal(t, events[0].Source, TaskRunEventSourceTask)
	assert.Equal(t, events[0].ID, "task-1")
	assert.Equal(t, events[0].EventType, string(taskverification.TaskEventTypeRegistered))
	assert.Equal(t, events[0].RelatedActionRequestID, "request-1")

	assert.Equal(t, events[1].Source, TaskRunEventSourceAction)
	assert.Equal(t, events[1].ID, "action-a")
	assert.Equal(t, events[1].RequestID, "request-1")
	assert.Equal(t, events[1].Direction, string(common.DirectionClientToServer))
	assert.Equal(t, events[1].MessageType, string(common.MessageTypeRequest))
	assert.Assert(t, events[1].Success != nil)
	assert.Equal(t, *events[1].Success, true)

	assert.Equal(t, events[2].ID, "action-b")
	assert.Equal(t, events[3].ID, "task-2")
	assert.Equal(t, events[2].CreatedAtUnixMilli, int64(1_050))
	assert.Equal(t, events[3].CreatedAtUnixMilli, int64(1_050))
	assert.Assert(t, events[2].IsError != nil)
	assert.Equal(t, *events[2].IsError, true)
	assert.Equal(t, events[3].Source, TaskRunEventSourceTask)
	assert.Equal(t, events[3].ResultingPhasePath, "execution.step_one")
}

func TestGetTaskRunEventsReturnsTaskOnlyAndEmptyForUnknownRun(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})

	// Given: one run with task events only.
	seedTaskEvent(t, store, &taskverification.TaskEvent{
		ID:                 "task-only-1",
		SchemaVersion:      1,
		CreatedAtUnixMilli: 1_000,
		TaskRunID:          "task-only",
		SessionID:          "session-1",
		TemplateID:         "template-1",
		PrincipalID:        "principal-1",
		PhasePath:          taskverification.TaskPhaseOnboarding,
		NodeKind:           taskverification.WorkflowNodeKindOnboarding,
		ResultingPhasePath: taskverification.TaskPhasePlanning,
		ResultingNodeKind:  taskverification.WorkflowNodeKindPlanning,
		EventType:          taskverification.TaskEventTypeRegistered,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            json.RawMessage(`{"status":"active"}`),
	})

	// When: events are queried for the known and unknown run ids.
	taskOnlyEvents, err := store.GetTaskRunEvents(context.Background(), "task-only")
	assert.NilError(t, err)
	missingEvents, err := store.GetTaskRunEvents(context.Background(), "missing-run")
	assert.NilError(t, err)

	// Then: the known run returns task events only and the missing run is empty.
	assert.Equal(t, len(taskOnlyEvents), 1)
	assert.Equal(t, taskOnlyEvents[0].Source, TaskRunEventSourceTask)
	assert.Equal(t, len(missingEvents), 0)
}

func TestListEventsAppliesFiltersAndExposesTaskContext(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})

	runID := identifiers.New(identifiers.KindTaskRun)
	seedActionContext(t, store, taskverification.ActionEventTaskContext{
		RequestID:           "req-1",
		TaskRunID:           runID,
		InvocationPhasePath: taskverification.TaskPhasePlanning,
		InvocationNodeKind:  taskverification.WorkflowNodeKindPlanning,
		CreatedAtUnixMilli:  3_000,
	})
	seedActionEvent(t, store, &ActionEventRecord{
		ID:                 identifiers.New(identifiers.KindActionEvent),
		SchemaVersion:      schemaVersion,
		CreatedAtUnixMilli: 3_000,
		RequestID:          "req-1",
		SessionID:          "sid-1",
		Transport:          "http",
		Direction:          string(common.DirectionClientToServer),
		MessageType:        string(common.MessageTypeRequest),
		Gateway:            "gw",
		ServerName:         "server-a",
		Endpoint:           "/mcp/gw",
		ToolName:           "shell__exec",
		OriginalToolName:   "shell__exec",
		Success:            true,
		IsError:            false,
		PayloadJSON:        json.RawMessage(`{"arguments":{"command":"pwd"}}`),
	})
	err = store.AppendTaskEvent(&taskverification.TaskEvent{
		ID:                     "task-event-quality",
		SchemaVersion:          schemaVersion,
		CreatedAtUnixMilli:     3_100,
		TaskRunID:              runID,
		EventType:              taskverification.TaskEventTypeStepCompleted,
		Outcome:                taskverification.TaskEventOutcomeFailed,
		RelatedActionRequestID: "req-1",
		Payload: json.RawMessage(`{
				"status": "active",
				"annotations": [
					{
						"type": "governance_events",
						"processor": "centian",
						"action": "stopped",
						"category": "quality",
						"severity": "medium",
						"message": "Ticket was not updated."
					}
				]
			}`),
	})
	assert.NilError(t, err)
	seedActionEvent(t, store, &ActionEventRecord{
		ID:                 identifiers.New(identifiers.KindActionEvent),
		SchemaVersion:      schemaVersion,
		CreatedAtUnixMilli: 2_000,
		RequestID:          "req-2",
		SessionID:          "sid-2",
		Transport:          "stdio",
		Direction:          string(common.DirectionServerToClient),
		MessageType:        string(common.MessageTypeResponse),
		Gateway:            "gw",
		ServerName:         "server-b",
		Endpoint:           "/mcp/gw",
		ToolName:           "git__search",
		OriginalToolName:   "git__search",
		Success:            false,
		IsError:            true,
		PayloadJSON:        json.RawMessage(`{"error":"boom"}`),
	})

	page, err := store.ListEvents(context.Background(), &EventListFilter{
		Gateway:     "gw",
		ServerName:  "server-a",
		ToolName:    "shell__exec",
		Direction:   string(common.DirectionClientToServer),
		MessageType: string(common.MessageTypeRequest),
		RequestID:   "req-1",
		SessionID:   "sid-1",
		Success:     boolPointer(true),
		Limit:       10,
	})
	assert.NilError(t, err)
	assert.Equal(t, len(page.Items), 1)
	assert.Equal(t, page.Items[0].RequestID, "req-1")
	assert.Equal(t, page.Items[0].TaskRunID != "", true)
	assert.Equal(t, page.Items[0].InvocationPhasePath, string(taskverification.TaskPhasePlanning))
	assert.Equal(t, page.Items[0].InvocationNodeKind, string(taskverification.WorkflowNodeKindPlanning))
	assert.Equal(t, len(page.Items[0].Annotations), 1)
	assert.Equal(t, page.Items[0].Annotations[0].Type, "governance_events")
	assert.Equal(t, page.Items[0].Annotations[0].Category, "quality")
	assert.Equal(t, page.Items[0].Annotations[0].Action, "stopped")

	unfiltered, err := store.ListEvents(context.Background(), &EventListFilter{Limit: 10})
	assert.NilError(t, err)
	assert.Equal(t, len(unfiltered.Items), 2)
	assert.Equal(t, unfiltered.Items[0].RequestID, "req-1")
	assert.Equal(t, unfiltered.Items[1].RequestID, "req-2")
	assert.Equal(t, unfiltered.Items[1].TaskRunID, "")

	governanceOnly, err := store.ListEvents(context.Background(), &EventListFilter{WithGovernanceEvent: true, Limit: 10})
	assert.NilError(t, err)
	assert.Equal(t, len(governanceOnly.Items), 1)
	assert.Equal(t, governanceOnly.Items[0].RequestID, "req-1")
	assert.Equal(t, len(governanceOnly.Items[0].Annotations), 1)
	assert.Equal(t, governanceOnly.Items[0].Annotations[0].Category, "quality")
}

func TestListEventsPaginatesWithStableCursorOrdering(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})

	for _, event := range []*ActionEventRecord{
		{
			ID:                 "ae_0000000003000_0000000003",
			SchemaVersion:      schemaVersion,
			CreatedAtUnixMilli: 3_000,
			RequestID:          "req-3",
			ToolName:           "tool-c",
			Success:            true,
		},
		{
			ID:                 "ae_0000000003000_0000000002",
			SchemaVersion:      schemaVersion,
			CreatedAtUnixMilli: 3_000,
			RequestID:          "req-2",
			ToolName:           "tool-b",
			Success:            true,
		},
		{
			ID:                 "ae_0000000003000_0000000001",
			SchemaVersion:      schemaVersion,
			CreatedAtUnixMilli: 3_000,
			RequestID:          "req-1",
			ToolName:           "tool-a",
			Success:            true,
		},
		{
			ID:                 "ae_0000000002000_0000000001",
			SchemaVersion:      schemaVersion,
			CreatedAtUnixMilli: 2_000,
			RequestID:          "req-0",
			ToolName:           "tool-z",
			Success:            true,
		},
	} {
		seedActionEvent(t, store, event)
	}

	firstPage, err := store.ListEvents(context.Background(), &EventListFilter{Limit: 2})
	assert.NilError(t, err)
	assert.Equal(t, len(firstPage.Items), 2)
	assert.Equal(t, firstPage.Items[0].ID, "ae_0000000003000_0000000003")
	assert.Equal(t, firstPage.Items[1].ID, "ae_0000000003000_0000000002")
	assert.Assert(t, firstPage.NextCursor != "")

	cursor, hasCursor, err := ParseEventCursor(firstPage.NextCursor)
	assert.NilError(t, err)
	assert.Equal(t, hasCursor, true)
	secondPage, err := store.ListEvents(context.Background(), &EventListFilter{
		Limit:  2,
		Cursor: &cursor,
	})
	assert.NilError(t, err)
	assert.Equal(t, len(secondPage.Items), 2)
	assert.Equal(t, secondPage.Items[0].ID, "ae_0000000003000_0000000001")
	assert.Equal(t, secondPage.Items[1].ID, "ae_0000000002000_0000000001")
	assert.Equal(t, secondPage.NextCursor, "")
}

func seedTaskEvent(t *testing.T, store *Store, event *taskverification.TaskEvent) {
	t.Helper()
	assert.NilError(t, store.AppendTaskEvent(event))
}

func boolPointer(value bool) *bool {
	return &value
}

func seedActionContext(t *testing.T, store *Store, ctx taskverification.ActionEventTaskContext) {
	t.Helper()
	assert.NilError(t, store.AppendActionEventTaskContext(ctx))
}

func seedActionEvent(t *testing.T, store *Store, event *ActionEventRecord) {
	t.Helper()
	_, err := store.DB().NewInsert().Model(event).Exec(context.Background())
	assert.NilError(t, err)
}

func seedSchemaVersion4Store(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.Exec(`CREATE TABLE event_store_schema (name TEXT PRIMARY KEY, version INTEGER NOT NULL)`)
	assert.NilError(t, err)
	_, err = db.Exec(`INSERT INTO event_store_schema(name, version) VALUES ('event_storage', 4)`)
	assert.NilError(t, err)

	stmts := []string{
		`CREATE TABLE task_events (
			id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL,
			created_at_unix_milli INTEGER NOT NULL,
			task_run_id TEXT NOT NULL,
			session_id TEXT,
			template_id TEXT NOT NULL,
			principal_id TEXT,
			client_name TEXT,
			client_version TEXT,
			phase_path TEXT NOT NULL,
			node_kind TEXT,
			resulting_phase_path TEXT NOT NULL,
			resulting_node_kind TEXT,
			event_type TEXT NOT NULL,
			outcome TEXT NOT NULL,
			related_action_request_id TEXT,
			payload_json BLOB
		)`,
		`CREATE INDEX idx_task_events_task_run_id ON task_events(task_run_id)`,
		`CREATE INDEX idx_task_events_created_at ON task_events(created_at_unix_milli)`,
		`CREATE TABLE action_events (
			id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL,
			created_at_unix_milli INTEGER NOT NULL,
			request_id TEXT NOT NULL,
			session_id TEXT,
			principal_id TEXT,
			transport TEXT,
			direction TEXT,
			message_type TEXT,
			gateway TEXT,
			server_name TEXT,
			endpoint TEXT,
			tool_name TEXT,
			original_tool_name TEXT,
			success BOOLEAN NOT NULL,
			is_error BOOLEAN NOT NULL,
			payload_json BLOB
		)`,
		`CREATE INDEX idx_action_events_request_id ON action_events(request_id)`,
		`CREATE INDEX idx_action_events_created_at ON action_events(created_at_unix_milli)`,
		`CREATE TABLE action_event_task_context (
			request_id TEXT PRIMARY KEY,
			task_run_id TEXT NOT NULL,
			invocation_phase_path TEXT NOT NULL,
			invocation_node_kind TEXT,
			created_at_unix_milli INTEGER NOT NULL
		)`,
		`CREATE INDEX idx_action_event_task_context_task_run_id ON action_event_task_context(task_run_id)`,
	}
	for _, stmt := range stmts {
		_, err = db.Exec(stmt)
		assert.NilError(t, err)
	}
}

func seedSchemaVersion5Store(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx := context.Background()
	_, err := db.Exec(`CREATE TABLE event_store_schema (name TEXT PRIMARY KEY, version INTEGER NOT NULL)`)
	assert.NilError(t, err)
	_, err = db.Exec(`INSERT INTO event_store_schema(name, version) VALUES ('event_storage', 5)`)
	assert.NilError(t, err)

	bundb := bun.NewDB(db, sqlitedialect.New())
	assert.NilError(t, createLegacyBenchmarkRunTablesV5(ctx, bundb))
	assert.NilError(t, createBenchmarkRunScoreTables(ctx, bundb))
	assert.NilError(t, createTaskRunSnapshotTables(ctx, bundb))

	_, err = db.Exec(`
INSERT INTO benchmark_sessions (
	session_id,
	schema_version,
	suite_id,
	suite_name,
	suite_path,
	session_path,
	output_root,
	template_id,
	template_name,
	started_at_unix_milli,
	status,
	repeat_count
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"bm_session_1",
		benchmarkRunSchemaVersion,
		"simple_tdd_v1",
		"Simple TDD Benchmark Suite v1",
		"/tmp/suite",
		"/tmp/session",
		"/tmp",
		"simple_tdd",
		"Simple TDD Current",
		1000,
		"completed",
		1,
	)
	assert.NilError(t, err)

	insertTaskRunSnapshotRow(t, db, "tr_1", "completed")
	insertTaskRunSnapshotRow(t, db, "tr_2", "failed")
	insertTaskRunSnapshotRow(t, db, "tr_3", "completed")

	linkedJSON, err := json.Marshal([]string{"tr_1", "tr_2", "tr_1"})
	assert.NilError(t, err)
	_, err = db.Exec(`
INSERT INTO benchmark_runs (
	benchmark_run_id,
	schema_version,
	session_id,
	case_id,
	case_name,
	agent,
	template_variant,
	attempt,
	template_id,
	template_name,
	selected_model,
	started_at_unix_milli,
	ended_at_unix_milli,
	status,
	latest_task_run_id,
	latest_task_run_status,
	linked_task_run_ids_json,
	run_dir,
	project_dir,
	logs_dir,
	agent_dir,
	config_path,
	event_store_mode,
	event_store_path,
	request_log_path,
	selected_template_path,
	error_summary,
	agent_metadata_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"bm_run_1",
		benchmarkRunSchemaVersion,
		"bm_session_1",
		"compile_failure_red",
		"Compile Failure Red",
		"codex",
		"current",
		1,
		"simple_tdd",
		"Simple TDD Current",
		"gpt-5.4-mini",
		1000,
		2000,
		"completed",
		"tr_3",
		"completed",
		linkedJSON,
		"/tmp/session/run",
		"/tmp/session/run/project",
		"/tmp/session/run/logs",
		"/tmp/session/run/agent",
		"/tmp/session/run/config.json",
		"configured_shared",
		"/tmp/events.sqlite",
		"/tmp/session/run/logs/requests_0001.jsonl",
		"/tmp/session/run/selected-template.yaml",
		"",
		[]byte(`{"threadId":"thread_1"}`),
	)
	assert.NilError(t, err)

	scoreErrorsJSON, err := json.Marshal([]string{})
	assert.NilError(t, err)
	_, err = db.Exec(`
INSERT INTO benchmark_run_scores (
	benchmark_run_id,
	schema_version,
	score_status,
	score_version,
	generated_at_unix_milli,
	scorecard_json,
	score_errors_json,
	selected_model,
	completed_successfully,
	final_verification_passed,
	first_pass_success,
	restart_occurred,
	fail_occurred,
	timeout_occurred,
	invariant_violation,
	wall_clock_seconds,
	total_tool_calls,
	total_task_tool_calls,
	total_downstream_tool_calls,
	failed_task_tool_calls,
	failed_downstream_tool_calls,
	edited_files_count
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"bm_run_1",
		benchmarkRunScoreSchemaVersion,
		"scored",
		"v1",
		2000,
		[]byte(`{"benchmarkRunId":"bm_run_1"}`),
		scoreErrorsJSON,
		"gpt-5.4-mini",
		true,
		true,
		true,
		false,
		false,
		false,
		false,
		5.0,
		3,
		1,
		2,
		0,
		0,
		1,
	)
	assert.NilError(t, err)
}

func seedSchemaVersion6Store(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.Exec(`CREATE TABLE event_store_schema (name TEXT PRIMARY KEY, version INTEGER NOT NULL)`)
	assert.NilError(t, err)
	_, err = db.Exec(`INSERT INTO event_store_schema(name, version) VALUES ('event_storage', 6)`)
	assert.NilError(t, err)

	stmts := []string{
		`CREATE TABLE action_events (
			id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL,
			created_at_unix_milli INTEGER NOT NULL,
			request_id TEXT NOT NULL,
			session_id TEXT,
			principal_id TEXT,
			transport TEXT,
			direction TEXT,
			message_type TEXT,
			gateway TEXT,
			server_name TEXT,
			endpoint TEXT,
			tool_name TEXT,
			original_tool_name TEXT,
			success BOOLEAN NOT NULL,
			is_error BOOLEAN NOT NULL,
			payload_json BLOB
		)`,
		`CREATE INDEX idx_action_events_request_id ON action_events(request_id)`,
		`CREATE INDEX idx_action_events_created_at ON action_events(created_at_unix_milli)`,
	}
	for _, stmt := range stmts {
		_, err = db.Exec(stmt)
		assert.NilError(t, err)
	}
}

func insertTaskRunSnapshotRow(t *testing.T, db *sql.DB, runID string, status string) {
	t.Helper()

	payload, err := json.Marshal(taskruns.PersistedRunSnapshot{
		RunID:        runID,
		TemplateID:   "simple_tdd",
		TemplateName: "Simple TDD Current",
		Status:       status,
		Phase:        "execution.implement_fix",
		SelectedTemplate: taskruns.PersistedTemplateSnapshot{
			Task: taskruns.PersistedTaskSnapshot{ID: "simple_tdd", Name: "Simple TDD Current"},
		},
	})
	assert.NilError(t, err)

	_, err = db.Exec(`
INSERT INTO task_runs (
	run_id,
	schema_version,
	created_at_unix_milli,
	updated_at_unix_milli,
	template_id,
	template_name,
	status,
	phase,
	payload_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID,
		taskRunSnapshotSchemaVersion,
		1000,
		1000,
		"simple_tdd",
		"Simple TDD Current",
		status,
		"execution.implement_fix",
		payload,
	)
	assert.NilError(t, err)
}

func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func sqliteMasterCount(db *sql.DB, entryType string, name string) (int, error) {
	row := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = ? AND name = ?`, entryType, name)
	var count int
	err := row.Scan(&count)
	return count, err
}

func assertSQLiteIndexExists(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	count, err := sqliteMasterCount(db, "index", name)
	assert.NilError(t, err)
	assert.Equal(t, count, 1)
}
