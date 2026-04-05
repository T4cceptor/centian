package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/taskruns"
	"github.com/T4cceptor/centian/internal/taskverification"
)

type taskRunStatsInputs struct {
	TaskStart        sql.NullInt64 `bun:"task_start"`
	TaskEnd          sql.NullInt64 `bun:"task_end"`
	RestartCount     int           `bun:"restart_count"`
	FailCount        int           `bun:"fail_count"`
	TimeoutCount     int           `bun:"timeout_count"`
	TaskCalls        int           `bun:"task_calls"`
	DownstreamCalls  int           `bun:"downstream_calls"`
	TaskErrors       int           `bun:"task_errors"`
	DownstreamErrors int           `bun:"downstream_errors"`
}

type taskRunSnapshotMeta struct {
	CreatedAtUnixMilli int64
	UpdatedAtUnixMilli int64
	Status             string
}

// GetTaskRunStats returns one derived per-run stats row by run id.
func (s *Store) GetTaskRunStats(ctx context.Context, runID string) (*TaskRunStatsRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("task run stats store is not initialized")
	}
	if strings.TrimSpace(runID) == "" {
		return nil, fmt.Errorf("task run id is required")
	}
	row := &taskRunStatsRow{}
	if err := s.db.NewSelect().Model(row).Where("run_id = ?", runID).Scan(ctx); err != nil {
		return nil, err
	}
	return row.toRecord(), nil
}

// ListTaskRunStats returns all derived per-run stats ordered by start time descending.
func (s *Store) ListTaskRunStats(ctx context.Context) ([]TaskRunStatsRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("task run stats store is not initialized")
	}
	rows := make([]taskRunStatsRow, 0)
	if err := s.db.NewSelect().Model(&rows).OrderExpr("started_at_unix_milli DESC, run_id DESC").Scan(ctx); err != nil {
		return nil, err
	}
	records := make([]TaskRunStatsRecord, 0, len(rows))
	for idx := range rows {
		records = append(records, *rows[idx].toRecord())
	}
	return records, nil
}

func (s *Store) refreshTaskRunStatsForSnapshot(snapshot *taskruns.PersistedRunSnapshot) error {
	if snapshot == nil {
		return nil
	}
	return s.recomputeTaskRunStats(context.Background(), snapshot.RunID)
}

func (s *Store) refreshTaskRunStatsForTaskEvent(event *taskverification.TaskEvent) error {
	if event == nil {
		return nil
	}
	return s.recomputeTaskRunStats(context.Background(), event.TaskRunID)
}

func (s *Store) refreshTaskRunStatsForActionContext(link taskverification.ActionEventTaskContext) error {
	if strings.TrimSpace(link.TaskRunID) == "" {
		return nil
	}
	return s.recomputeTaskRunStats(context.Background(), link.TaskRunID)
}

func (s *Store) refreshTaskRunStatsForActionRequest(requestID string) error {
	if s == nil || s.db == nil || strings.TrimSpace(requestID) == "" {
		return nil
	}
	rows := make([]actionEventTaskContextRow, 0)
	if err := s.db.NewSelect().Model(&rows).Where("request_id = ?", requestID).Scan(context.Background()); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(rows))
	for idx := range rows {
		runID := rows[idx].TaskRunID
		if _, exists := seen[runID]; exists {
			continue
		}
		seen[runID] = struct{}{}
		if err := s.recomputeTaskRunStats(context.Background(), runID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) recomputeTaskRunStats(ctx context.Context, runID string) error {
	if s == nil || s.db == nil || strings.TrimSpace(runID) == "" {
		return nil
	}

	meta, err := s.loadTaskRunSnapshotMeta(ctx, runID)
	if err != nil {
		return err
	}
	inputs, err := s.loadTaskRunStatsInputs(ctx, runID)
	if err != nil {
		return err
	}

	snapshotCreatedAt := int64(0)
	snapshotUpdatedAt := int64(0)
	snapshotStatus := ""
	if meta != nil {
		snapshotCreatedAt = meta.CreatedAtUnixMilli
		snapshotUpdatedAt = meta.UpdatedAtUnixMilli
		snapshotStatus = meta.Status
	}

	startedAt := earliestNonZero(snapshotCreatedAt, inputs.TaskStart.Int64)
	if startedAt == 0 {
		return nil
	}

	terminal := isTerminalTaskStatus(snapshotStatus) || inputs.FailCount > 0 || inputs.TimeoutCount > 0
	endedAt := int64(0)
	if terminal {
		endedAt = latestNonZero(snapshotUpdatedAt, inputs.TaskEnd.Int64)
	}

	now := time.Now().UTC().UnixMilli()
	createdAt := now
	existing := &taskRunStatsRow{}
	if err := s.db.NewSelect().Model(existing).Where("run_id = ?", runID).Scan(ctx); err == nil {
		createdAt = existing.CreatedAtUnixMilli
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	row := taskRunStatsRow{
		RunID:                    runID,
		SchemaVersion:            taskRunStatsSchemaVersion,
		CreatedAtUnixMilli:       createdAt,
		UpdatedAtUnixMilli:       now,
		StartedAtUnixMilli:       startedAt,
		TaskToolCallCount:        inputs.TaskCalls,
		DownstreamToolCallCount:  inputs.DownstreamCalls,
		TaskToolErrorCount:       inputs.TaskErrors,
		DownstreamToolErrorCount: inputs.DownstreamErrors,
		RestartCount:             inputs.RestartCount,
		FailCount:                inputs.FailCount,
		TimeoutCount:             inputs.TimeoutCount,
	}
	if endedAt > 0 {
		row.EndedAtUnixMilli = sql.NullInt64{Int64: endedAt, Valid: true}
		row.DurationMillis = sql.NullInt64{Int64: endedAt - startedAt, Valid: endedAt >= startedAt}
	}

	_, err = s.db.NewInsert().
		Model(&row).
		On("CONFLICT (run_id) DO UPDATE").
		Set("schema_version = EXCLUDED.schema_version").
		Set("updated_at_unix_milli = EXCLUDED.updated_at_unix_milli").
		Set("started_at_unix_milli = EXCLUDED.started_at_unix_milli").
		Set("ended_at_unix_milli = EXCLUDED.ended_at_unix_milli").
		Set("duration_millis = EXCLUDED.duration_millis").
		Set("task_tool_call_count = EXCLUDED.task_tool_call_count").
		Set("downstream_tool_call_count = EXCLUDED.downstream_tool_call_count").
		Set("task_tool_error_count = EXCLUDED.task_tool_error_count").
		Set("downstream_tool_error_count = EXCLUDED.downstream_tool_error_count").
		Set("restart_count = EXCLUDED.restart_count").
		Set("fail_count = EXCLUDED.fail_count").
		Set("timeout_count = EXCLUDED.timeout_count").
		Exec(ctx)
	return err
}

func (s *Store) loadTaskRunSnapshotMeta(ctx context.Context, runID string) (*taskRunSnapshotMeta, error) {
	row := &taskRunSnapshotRow{}
	if err := s.db.NewSelect().
		Model(row).
		Column("created_at_unix_milli", "updated_at_unix_milli", "status").
		Where("run_id = ?", runID).
		Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			//nolint:nilnil // Stats can be derived from task/action events even when no snapshot row exists yet.
			return nil, nil
		}
		return nil, err
	}
	return &taskRunSnapshotMeta{
		CreatedAtUnixMilli: row.CreatedAtUnixMilli,
		UpdatedAtUnixMilli: row.UpdatedAtUnixMilli,
		Status:             row.Status,
	}, nil
}

func (s *Store) loadTaskRunStatsInputs(ctx context.Context, runID string) (*taskRunStatsInputs, error) {
	row := &taskRunStatsInputs{}
	query := `
WITH task_agg AS (
	SELECT
		MIN(created_at_unix_milli) AS task_start,
		MAX(created_at_unix_milli) AS task_end,
		SUM(CASE WHEN event_type = 'task_restarted' THEN 1 ELSE 0 END) AS restart_count,
		SUM(CASE WHEN event_type = 'task_failed' THEN 1 ELSE 0 END) AS fail_count,
		SUM(CASE WHEN event_type = 'task_timed_out' THEN 1 ELSE 0 END) AS timeout_count
	FROM task_events
	WHERE task_run_id = ?
),
	call_agg AS (
	SELECT
		SUM(CASE WHEN tool_name LIKE 'centian.task_%' THEN 1 ELSE 0 END) AS task_calls,
		SUM(CASE WHEN tool_name LIKE 'centian.task_%' AND is_error = 1 THEN 1 ELSE 0 END) AS task_errors,
		SUM(CASE WHEN tool_name NOT LIKE 'centian.task_%' THEN 1 ELSE 0 END) AS downstream_calls,
		SUM(CASE WHEN tool_name NOT LIKE 'centian.task_%' AND is_error = 1 THEN 1 ELSE 0 END) AS downstream_errors
	FROM (
		SELECT
			ctx.request_id,
			COALESCE(
				MAX(NULLIF(ae.tool_name, '')),
				MAX(NULLIF(ae.original_tool_name, '')),
				''
			) AS tool_name,
			MAX(CASE WHEN ae.is_error THEN 1 ELSE 0 END) AS is_error
		FROM action_event_task_context ctx
		JOIN action_events ae ON ae.request_id = ctx.request_id
		WHERE ctx.task_run_id = ?
		GROUP BY ctx.request_id
	) calls
	WHERE tool_name <> ''
)
SELECT
	COALESCE((SELECT task_start FROM task_agg), 0) AS task_start,
	(SELECT task_end FROM task_agg) AS task_end,
	COALESCE((SELECT restart_count FROM task_agg), 0) AS restart_count,
	COALESCE((SELECT fail_count FROM task_agg), 0) AS fail_count,
	COALESCE((SELECT timeout_count FROM task_agg), 0) AS timeout_count,
	COALESCE((SELECT task_calls FROM call_agg), 0) AS task_calls,
	COALESCE((SELECT downstream_calls FROM call_agg), 0) AS downstream_calls,
	COALESCE((SELECT task_errors FROM call_agg), 0) AS task_errors,
	COALESCE((SELECT downstream_errors FROM call_agg), 0) AS downstream_errors
`
	if err := s.db.NewRaw(query, runID, runID).Scan(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}

func earliestNonZero(values ...int64) int64 {
	result := int64(0)
	for idx := range values {
		value := values[idx]
		if value == 0 {
			continue
		}
		if result == 0 || value < result {
			result = value
		}
	}
	return result
}

func latestNonZero(values ...int64) int64 {
	result := int64(0)
	for idx := range values {
		value := values[idx]
		if value > result {
			result = value
		}
	}
	return result
}
