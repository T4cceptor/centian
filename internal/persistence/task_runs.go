package persistence

import (
	"context"
	"encoding/json"

	"github.com/T4cceptor/centian/internal/taskruns"
	"github.com/uptrace/bun"
)

const taskRunSnapshotSchemaVersion = 1

type taskRunSnapshotRow struct {
	bun.BaseModel      `bun:"table:task_runs"`
	RunID              string `bun:",pk"`
	SchemaVersion      int
	CreatedAtUnixMilli int64
	UpdatedAtUnixMilli int64
	TemplateID         string
	TemplateName       string
	Status             string
	Phase              string
	PayloadJSON        json.RawMessage
}

// TaskRunSnapshotRecord is the persisted snapshot row metadata plus payload.
type TaskRunSnapshotRecord struct {
	RunID              string                         `json:"runId"`
	CreatedAtUnixMilli int64                          `json:"createdAtUnixMilli"`
	UpdatedAtUnixMilli int64                          `json:"updatedAtUnixMilli"`
	TemplateID         string                         `json:"templateId"`
	TemplateName       string                         `json:"templateName"`
	Status             string                         `json:"status"`
	Phase              string                         `json:"phase"`
	Payload            *taskruns.PersistedRunSnapshot `json:"payload"`
}

func createTaskRunSnapshotTables(ctx context.Context, db bun.IDB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS task_runs (
			run_id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL,
			created_at_unix_milli INTEGER NOT NULL,
			updated_at_unix_milli INTEGER NOT NULL,
			template_id TEXT NOT NULL,
			template_name TEXT NOT NULL,
			status TEXT NOT NULL,
			phase TEXT NOT NULL,
			payload_json BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_runs_updated_at ON task_runs(updated_at_unix_milli DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_task_runs_template_updated_at ON task_runs(template_id, updated_at_unix_milli DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_task_runs_status_updated_at ON task_runs(status, updated_at_unix_milli DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (row *taskRunSnapshotRow) toRecord() (*TaskRunSnapshotRecord, error) {
	if row == nil {
		return nil, nil
	}
	var payload taskruns.PersistedRunSnapshot
	if err := json.Unmarshal(row.PayloadJSON, &payload); err != nil {
		return nil, err
	}
	return &TaskRunSnapshotRecord{
		RunID:              row.RunID,
		CreatedAtUnixMilli: row.CreatedAtUnixMilli,
		UpdatedAtUnixMilli: row.UpdatedAtUnixMilli,
		TemplateID:         row.TemplateID,
		TemplateName:       row.TemplateName,
		Status:             row.Status,
		Phase:              row.Phase,
		Payload:            &payload,
	}, nil
}
