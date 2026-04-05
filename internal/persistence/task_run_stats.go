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

func createTaskRunStatsTables(ctx context.Context, db bun.IDB) error {
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

func recreateTaskRunStatsTables(ctx context.Context, db bun.IDB) error {
	stmts := []string{
		`DROP INDEX IF EXISTS idx_task_run_stats_started_at`,
		`DROP INDEX IF EXISTS idx_task_run_stats_updated_at`,
		`ALTER TABLE task_run_stats RENAME TO task_run_stats_old`,
		`CREATE TABLE task_run_stats (
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
		`INSERT INTO task_run_stats (
			run_id,
			schema_version,
			created_at_unix_milli,
			updated_at_unix_milli,
			started_at_unix_milli,
			ended_at_unix_milli,
			duration_millis,
			task_tool_call_count,
			downstream_tool_call_count,
			task_tool_error_count,
			downstream_tool_error_count,
			restart_count,
			fail_count,
			timeout_count
		)
		SELECT
			stats.run_id,
			stats.schema_version,
			stats.created_at_unix_milli,
			stats.updated_at_unix_milli,
			stats.started_at_unix_milli,
			stats.ended_at_unix_milli,
			stats.duration_millis,
			stats.task_tool_call_count,
			stats.downstream_tool_call_count,
			stats.task_tool_error_count,
			stats.downstream_tool_error_count,
			stats.restart_count,
			stats.fail_count,
			stats.timeout_count
		FROM task_run_stats_old stats
		JOIN task_runs runs ON runs.run_id = stats.run_id`,
		`DROP TABLE task_run_stats_old`,
		`CREATE INDEX idx_task_run_stats_started_at ON task_run_stats(started_at_unix_milli DESC)`,
		`CREATE INDEX idx_task_run_stats_updated_at ON task_run_stats(updated_at_unix_milli DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to recreate task run stats schema: %w", err)
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
