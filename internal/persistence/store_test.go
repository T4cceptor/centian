package persistence

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/taskverification"
	"gotest.tools/assert"
)

func TestNewSQLiteStoreBootstrapsAndPersistsRows(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})

	err = store.AppendTaskEvent(taskverification.TaskEvent{
		ID:                 "task-event-1",
		SchemaVersion:      1,
		CreatedAtUnixMilli: time.Now().UTC().UnixMilli(),
		TaskRunID:          "run-1",
		SessionID:          "session-1",
		TemplateID:         "template-1",
		PrincipalID:        "principal-1",
		PhasePath:          taskverification.TaskPhasePlanning,
		NodeKind:           taskverification.WorkflowNodeKindPlanning,
		EventType:          taskverification.TaskEventTypePlanningCompleted,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
		Payload:            json.RawMessage(`{"testTarget":"pytest -q"}`),
	})
	assert.NilError(t, err)

	err = store.AppendActionEventTaskContext(taskverification.ActionEventTaskContext{
		ActionEventID:      "action-1",
		TaskRunID:          "run-1",
		PhasePath:          taskverification.TaskPhasePlanning,
		NodeKind:           taskverification.WorkflowNodeKindPlanning,
		CreatedAtUnixMilli: time.Now().UTC().UnixMilli(),
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

	taskEvents := store.TaskEventRowsByTaskRunID("run-1")
	assert.Equal(t, len(taskEvents), 1)
	assert.Equal(t, taskEvents[0].PrincipalID, "principal-1")

	contexts := store.ActionEventRowsByTaskRunID("run-1")
	assert.Equal(t, len(contexts), 1)
	assert.Equal(t, contexts[0].ActionEventID, "action-1")

	actionEvents := store.ActionEventsByRequestID("action-1")
	assert.Equal(t, len(actionEvents), 1)
	assert.Equal(t, actionEvents[0].ToolName, "shell__exec")
	assert.Equal(t, actionEvents[0].PrincipalID, "principal-1")
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
