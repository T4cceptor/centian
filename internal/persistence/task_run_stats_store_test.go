package persistence

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/taskruns"
	"github.com/T4cceptor/centian/internal/taskverification"
	"gotest.tools/assert"
)

func TestTaskRunStatsStoreBootstrapsTable(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	count, err := store.db.NewSelect().TableExpr("sqlite_master").
		Where("type = 'table'").
		Where("name = 'task_run_stats'").
		Count(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, count, 1)
}

func TestTaskRunStatsTableReferencesTaskRuns(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	type foreignKeyRow struct {
		ID       int    `bun:"id"`
		Seq      int    `bun:"seq"`
		Table    string `bun:"table"`
		From     string `bun:"from"`
		To       string `bun:"to"`
		OnUpdate string `bun:"on_update"`
		OnDelete string `bun:"on_delete"`
		Match    string `bun:"match"`
	}

	rows := make([]foreignKeyRow, 0)
	err = store.db.NewRaw(`PRAGMA foreign_key_list('task_run_stats')`).Scan(context.Background(), &rows)
	assert.NilError(t, err)
	assert.Assert(t, len(rows) > 0)
	assert.Equal(t, rows[0].Table, "task_runs")
	assert.Equal(t, rows[0].From, "run_id")
	assert.Equal(t, rows[0].To, "run_id")
}

func TestTaskRunStatsTrackLifecycleAndToolCalls(t *testing.T) {
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
				Description: "Task",
			},
		},
	}
	assert.NilError(t, store.UpsertTaskRunSnapshot(context.Background(), snapshot))

	now := time.Now().UTC()
	assert.NilError(t, store.AppendTaskEvent(&taskverification.TaskEvent{
		ID:                 "task-event-1",
		SchemaVersion:      1,
		CreatedAtUnixMilli: now.UnixMilli(),
		TaskRunID:          "run-1",
		SessionID:          "session-1",
		TemplateID:         "simple_tdd",
		PhasePath:          taskverification.TaskPhasePlanning,
		ResultingPhasePath: taskverification.TaskPhasePlanning,
		EventType:          taskverification.TaskEventTypeRegistered,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
	}))
	assert.NilError(t, store.AppendTaskEvent(&taskverification.TaskEvent{
		ID:                 "task-event-2",
		SchemaVersion:      1,
		CreatedAtUnixMilli: now.Add(2 * time.Second).UnixMilli(),
		TaskRunID:          "run-1",
		SessionID:          "session-1",
		TemplateID:         "simple_tdd",
		PhasePath:          taskverification.TaskPhasePlanning,
		ResultingPhasePath: taskverification.TaskPhaseOnboarding,
		EventType:          taskverification.TaskEventTypeRestarted,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
	}))
	assert.NilError(t, store.AppendTaskEvent(&taskverification.TaskEvent{
		ID:                 "task-event-3",
		SchemaVersion:      1,
		CreatedAtUnixMilli: now.Add(4 * time.Second).UnixMilli(),
		TaskRunID:          "run-1",
		SessionID:          "session-1",
		TemplateID:         "simple_tdd",
		PhasePath:          taskverification.TaskPhasePlanning,
		ResultingPhasePath: taskverification.TaskPhasePlanning,
		EventType:          taskverification.TaskEventTypeTimedOut,
		Outcome:            taskverification.TaskEventOutcomeSucceeded,
	}))

	assert.NilError(t, store.AppendActionEventTaskContext(taskverification.ActionEventTaskContext{
		RequestID:           "req-centian",
		TaskRunID:           "run-1",
		InvocationPhasePath: taskverification.TaskPhasePlanning,
		CreatedAtUnixMilli:  now.UnixMilli(),
	}))
	assert.NilError(t, store.AppendActionEventTaskContext(taskverification.ActionEventTaskContext{
		RequestID:           "req-mcp",
		TaskRunID:           "run-1",
		InvocationPhasePath: taskverification.TaskPhasePlanning,
		CreatedAtUnixMilli:  now.UnixMilli(),
	}))

	assert.NilError(t, store.AppendActionEvent(toolLogEntry(now, "req-centian", common.MessageTypeRequest, common.DirectionClientToServer, "centian.task_start_step", false)))
	assert.NilError(t, store.AppendActionEvent(toolLogEntry(now.Add(time.Second), "req-centian", common.MessageTypeResponse, common.DirectionServerToClient, "centian.task_start_step", false)))
	assert.NilError(t, store.AppendActionEvent(toolLogEntry(now, "req-mcp", common.MessageTypeRequest, common.DirectionClientToServer, "shell__exec", false)))
	assert.NilError(t, store.AppendActionEvent(toolLogEntry(now.Add(time.Second), "req-mcp", common.MessageTypeResponse, common.DirectionServerToClient, "shell__exec", true)))

	stats, err := store.GetTaskRunStats(context.Background(), "run-1")
	assert.NilError(t, err)
	assert.Equal(t, stats.RunID, "run-1")
	assert.Assert(t, stats.StartedAtUnixMilli > 0)
	assert.Assert(t, stats.EndedAtUnixMilli != nil)
	assert.Assert(t, stats.DurationMillis != nil)
	assert.Equal(t, stats.TaskToolCallCount, 1)
	assert.Equal(t, stats.DownstreamToolCallCount, 1)
	assert.Equal(t, stats.TaskToolErrorCount, 0)
	assert.Equal(t, stats.DownstreamToolErrorCount, 1)
	assert.Equal(t, stats.RestartCount, 1)
	assert.Equal(t, stats.FailCount, 0)
	assert.Equal(t, stats.TimeoutCount, 1)
}

func TestTaskRunStatsUpdateWhenActionArrivesBeforeContext(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	assert.NilError(t, store.UpsertTaskRunSnapshot(context.Background(), &taskruns.PersistedRunSnapshot{
		RunID:        "run-1",
		TemplateID:   "simple_tdd",
		TemplateName: "Simple TDD Task",
		Status:       "active",
		Phase:        "planning",
		SelectedTemplate: taskruns.PersistedTemplateSnapshot{
			Version: "0.1",
			Task:    taskruns.PersistedTaskSnapshot{ID: "simple_tdd", Name: "Simple TDD Task", Description: "Task"},
		},
	}))

	now := time.Now().UTC()
	assert.NilError(t, store.AppendActionEvent(toolLogEntry(now, "req-1", common.MessageTypeRequest, common.DirectionClientToServer, "centian.task_complete_step", false)))
	assert.NilError(t, store.AppendActionEventTaskContext(taskverification.ActionEventTaskContext{
		RequestID:           "req-1",
		TaskRunID:           "run-1",
		InvocationPhasePath: taskverification.TaskPhasePlanning,
		CreatedAtUnixMilli:  now.UnixMilli(),
	}))

	stats, err := store.GetTaskRunStats(context.Background(), "run-1")
	assert.NilError(t, err)
	assert.Equal(t, stats.TaskToolCallCount, 1)
	assert.Equal(t, stats.DownstreamToolCallCount, 0)
}

func toolLogEntry(ts time.Time, requestID string, messageType common.McpMessageType, direction common.McpEventDirection, toolName string, isError bool) *common.LogEntry {
	entry := &common.LogEntry{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:   ts,
			RequestID:   requestID,
			SessionID:   "session-1",
			Transport:   "http",
			MessageType: messageType,
			Direction:   direction,
			Success:     !isError,
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
	entry.WithToolRequest(toolName, toolName, nil)
	entry.WithToolResult(nil, isError)
	return entry
}
