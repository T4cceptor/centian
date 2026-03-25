package persistence

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/taskverification"
	"github.com/uptrace/bun/driver/sqliteshim"
	"gotest.tools/assert"
)

func TestNewSQLiteStoreBootstrapsAndPersistsRows(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
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

	taskEvents := store.TaskEventRowsByTaskRunID("run-1")
	assert.Equal(t, len(taskEvents), 1)
	assert.Equal(t, taskEvents[0].PrincipalID, "principal-1")
	assert.Equal(t, taskEvents[0].ResultingPhasePath, "execution.step_one")
	assert.Equal(t, taskEvents[0].ResultingNodeKind, string(taskverification.WorkflowNodeKindExecution))

	contexts := store.ActionEventRowsByTaskRunID("run-1")
	assert.Equal(t, len(contexts), 1)
	assert.Equal(t, contexts[0].RequestID, "action-1")
	assert.Equal(t, contexts[0].InvocationPhasePath, string(taskverification.TaskPhasePlanning))

	actionEvents := store.ActionEventsByRequestID("action-1")
	assert.Equal(t, len(actionEvents), 2)
	assert.Equal(t, actionEvents[0].ToolName, "shell__exec")
	assert.Equal(t, actionEvents[0].PrincipalID, "principal-1")
	assert.Assert(t, actionEvents[0].ID != actionEvents[1].ID)
	assert.Equal(t, actionEvents[0].Direction, string(common.DirectionClientToServer))
	assert.Equal(t, actionEvents[0].MessageType, string(common.MessageTypeRequest))
	assert.Equal(t, actionEvents[1].Direction, string(common.DirectionServerToClient))
	assert.Equal(t, actionEvents[1].MessageType, string(common.MessageTypeResponse))
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

func TestNewSQLiteStoreResetsOldSchema(t *testing.T) {
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
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})

	requestEntry := &common.LogEntry{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:   time.Now().UTC(),
			RequestID:   "action-2",
			SessionID:   "session-2",
			Transport:   "http",
			MessageType: common.MessageTypeRequest,
			Direction:   common.DirectionClientToServer,
			Success:     true,
		},
	}
	responseEntry := &common.LogEntry{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:   time.Now().UTC(),
			RequestID:   "action-2",
			SessionID:   "session-2",
			Transport:   "http",
			MessageType: common.MessageTypeResponse,
			Direction:   common.DirectionServerToClient,
			Success:     true,
		},
	}
	err = store.AppendActionEvent(requestEntry)
	assert.NilError(t, err)
	err = store.AppendActionEvent(responseEntry)
	assert.NilError(t, err)
	assert.Equal(t, len(store.ActionEventsByRequestID("action-2")), 2)
}
