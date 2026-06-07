package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/uptrace/bun"
)

type benchmarkRunTaskRunLinkRow struct {
	bun.BaseModel  `bun:"table:benchmark_run_task_runs,alias:benchmark_run_task_runs"`
	BenchmarkRunID string `bun:"benchmark_run_id,pk"`
	TaskRunID      string `bun:"task_run_id,pk"`
	LinkOrder      sql.NullInt64
}

// BenchmarkRunTaskRunLinkRecord stores one relational link between a benchmark run and a task run.
type BenchmarkRunTaskRunLinkRecord struct {
	BenchmarkRunID string `json:"benchmarkRunId"`
	TaskRunID      string `json:"taskRunId"`
	LinkOrder      *int   `json:"linkOrder,omitempty"`
}

// BenchmarkRunTaskRunLinkFilter restricts benchmark/task link listing.
type BenchmarkRunTaskRunLinkFilter struct {
	BenchmarkRunID  string
	BenchmarkRunIDs []string
	TaskRunID       string
}

func createBenchmarkRunTaskRunLinkTables(ctx context.Context, db sqlExecutor) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS benchmark_run_task_runs (
			benchmark_run_id TEXT NOT NULL,
			task_run_id TEXT NOT NULL,
			link_order INTEGER,
			PRIMARY KEY (benchmark_run_id, task_run_id),
			FOREIGN KEY(benchmark_run_id) REFERENCES benchmark_runs(benchmark_run_id) ON DELETE CASCADE,
			FOREIGN KEY(task_run_id) REFERENCES task_runs(run_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_benchmark_run_task_runs_run_id ON benchmark_run_task_runs(benchmark_run_id, link_order ASC, task_run_id ASC)`,
		`CREATE INDEX IF NOT EXISTS idx_benchmark_run_task_runs_task_run_id ON benchmark_run_task_runs(task_run_id, benchmark_run_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to bootstrap benchmark run task-run link schema: %w", err)
		}
	}
	return nil
}

func benchmarkRunTaskRunLinkRowFromRecord(record BenchmarkRunTaskRunLinkRecord) (*benchmarkRunTaskRunLinkRow, error) {
	if strings.TrimSpace(record.BenchmarkRunID) == "" {
		return nil, fmt.Errorf("benchmark run task-run link benchmark run id is required")
	}
	if strings.TrimSpace(record.TaskRunID) == "" {
		return nil, fmt.Errorf("benchmark run task-run link task run id is required")
	}
	row := &benchmarkRunTaskRunLinkRow{
		BenchmarkRunID: record.BenchmarkRunID,
		TaskRunID:      record.TaskRunID,
	}
	if record.LinkOrder != nil {
		row.LinkOrder = sql.NullInt64{Int64: int64(*record.LinkOrder), Valid: true}
	}
	return row, nil
}

func (row *benchmarkRunTaskRunLinkRow) toRecord() BenchmarkRunTaskRunLinkRecord {
	record := BenchmarkRunTaskRunLinkRecord{
		BenchmarkRunID: row.BenchmarkRunID,
		TaskRunID:      row.TaskRunID,
	}
	if row.LinkOrder.Valid {
		order := int(row.LinkOrder.Int64)
		record.LinkOrder = &order
	}
	return record
}
