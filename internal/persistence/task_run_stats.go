package persistence

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/uptrace/bun"
)

const taskRunStatsSchemaVersion = 1

type taskRunStatsRow struct {
	bun.BaseModel            `bun:"table:task_run_stats"`
	RunID                    string `bun:",pk"`
	SchemaVersion            int
	CreatedAtUnixMilli       int64
	UpdatedAtUnixMilli       int64
	StartedAtUnixMilli       int64
	EndedAtUnixMilli         sql.NullInt64
	DurationMillis           sql.NullInt64
	TaskToolCallCount        int
	DownstreamToolCallCount  int
	TaskToolErrorCount       int
	DownstreamToolErrorCount int
	RestartCount             int
	FailCount                int
	TimeoutCount             int
}

// TaskRunStatsRecord stores generic per-run operational counters derived from persisted task and action events.
type TaskRunStatsRecord struct {
	RunID                    string `json:"runId"`
	CreatedAtUnixMilli       int64  `json:"createdAtUnixMilli"`
	UpdatedAtUnixMilli       int64  `json:"updatedAtUnixMilli"`
	StartedAtUnixMilli       int64  `json:"startedAtUnixMilli"`
	EndedAtUnixMilli         *int64 `json:"endedAtUnixMilli,omitempty"`
	DurationMillis           *int64 `json:"durationMillis,omitempty"`
	TaskToolCallCount        int    `json:"taskToolCallCount"`
	DownstreamToolCallCount  int    `json:"downstreamToolCallCount"`
	TaskToolErrorCount       int    `json:"taskToolErrorCount"`
	DownstreamToolErrorCount int    `json:"downstreamToolErrorCount"`
	RestartCount             int    `json:"restartCount"`
	FailCount                int    `json:"failCount"`
	TimeoutCount             int    `json:"timeoutCount"`
}

func createTaskRunStatsTables(ctx context.Context, db sqlExecutor) error {
	// TODO: move to migration
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS task_run_stats (
			run_id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL,
			created_at_unix_milli INTEGER NOT NULL,
			updated_at_unix_milli INTEGER NOT NULL,
			started_at_unix_milli INTEGER NOT NULL,
			ended_at_unix_milli INTEGER,
			duration_millis INTEGER,
			task_tool_call_count INTEGER NOT NULL,
			downstream_tool_call_count INTEGER NOT NULL,
			task_tool_error_count INTEGER NOT NULL,
			downstream_tool_error_count INTEGER NOT NULL,
			restart_count INTEGER NOT NULL,
			fail_count INTEGER NOT NULL,
			timeout_count INTEGER NOT NULL,
			FOREIGN KEY(run_id) REFERENCES task_runs(run_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_run_stats_started_at ON task_run_stats(started_at_unix_milli DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_task_run_stats_updated_at ON task_run_stats(updated_at_unix_milli DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to bootstrap task run stats schema: %w", err)
		}
	}
	return nil
}

func (row *taskRunStatsRow) toRecord() *TaskRunStatsRecord {
	if row == nil {
		return nil
	}
	return &TaskRunStatsRecord{
		RunID:                    row.RunID,
		CreatedAtUnixMilli:       row.CreatedAtUnixMilli,
		UpdatedAtUnixMilli:       row.UpdatedAtUnixMilli,
		StartedAtUnixMilli:       row.StartedAtUnixMilli,
		EndedAtUnixMilli:         nullInt64Pointer(row.EndedAtUnixMilli),
		DurationMillis:           nullInt64Pointer(row.DurationMillis),
		TaskToolCallCount:        row.TaskToolCallCount,
		DownstreamToolCallCount:  row.DownstreamToolCallCount,
		TaskToolErrorCount:       row.TaskToolErrorCount,
		DownstreamToolErrorCount: row.DownstreamToolErrorCount,
		RestartCount:             row.RestartCount,
		FailCount:                row.FailCount,
		TimeoutCount:             row.TimeoutCount,
	}
}

func nullInt64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
