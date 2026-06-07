package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/taskruns"
	"github.com/T4cceptor/centian/internal/taskverification"
	"github.com/uptrace/bun"
)

// UpsertTaskRunSnapshot persists or replaces the latest snapshot for one task run.
func (s *Store) UpsertTaskRunSnapshot(ctx context.Context, snapshot *taskruns.PersistedRunSnapshot) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("task run store is not initialized")
	}
	if snapshot == nil {
		return fmt.Errorf("task run snapshot is required")
	}
	if strings.TrimSpace(snapshot.RunID) == "" {
		return fmt.Errorf("task run snapshot run id is required")
	}
	if strings.TrimSpace(snapshot.TemplateID) == "" {
		return fmt.Errorf("task run snapshot template id is required")
	}
	if strings.TrimSpace(snapshot.TemplateName) == "" {
		return fmt.Errorf("task run snapshot template name is required")
	}
	if strings.TrimSpace(snapshot.Status) == "" {
		return fmt.Errorf("task run snapshot status is required")
	}
	if strings.TrimSpace(snapshot.Phase) == "" {
		return fmt.Errorf("task run snapshot phase is required")
	}

	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal task run snapshot: %w", err)
	}
	now := time.Now().UTC().UnixMilli()
	row := taskRunSnapshotRow{
		RunID:              snapshot.RunID,
		SchemaVersion:      taskRunSnapshotSchemaVersion,
		CreatedAtUnixMilli: now,
		UpdatedAtUnixMilli: now,
		TemplateID:         snapshot.TemplateID,
		TemplateName:       snapshot.TemplateName,
		OwnerPrincipalID:   snapshot.OwnerPrincipalID,
		Status:             snapshot.Status,
		Phase:              snapshot.Phase,
		PayloadJSON:        payload,
	}
	if _, err = s.db.NewInsert().
		Model(&row).
		On("CONFLICT (run_id) DO UPDATE").
		Set("schema_version = EXCLUDED.schema_version").
		Set("updated_at_unix_milli = EXCLUDED.updated_at_unix_milli").
		Set("template_id = EXCLUDED.template_id").
		Set("template_name = EXCLUDED.template_name").
		Set("owner_principal_id = EXCLUDED.owner_principal_id").
		Set("status = EXCLUDED.status").
		Set("phase = EXCLUDED.phase").
		Set("payload_json = EXCLUDED.payload_json").
		Exec(ctx); err != nil {
		return err
	}
	return s.refreshTaskRunStatsForSnapshot(ctx, snapshot)
}

// GetTaskRunSnapshot returns one persisted run snapshot by id.
func (s *Store) GetTaskRunSnapshot(ctx context.Context, runID string) (*TaskRunSnapshotRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("task run store is not initialized")
	}
	if strings.TrimSpace(runID) == "" {
		return nil, fmt.Errorf("task run id is required")
	}
	row := &taskRunSnapshotRow{}
	if err := s.db.NewSelect().Model(row).Where("run_id = ?", runID).Scan(ctx); err != nil {
		return nil, err
	}
	return row.toRecord()
}

// LoadTaskRunSnapshot returns the persisted snapshot payload for runID, or (nil, nil) when none exists.
func (s *Store) LoadTaskRunSnapshot(ctx context.Context, runID string) (*taskruns.PersistedRunSnapshot, error) {
	record, err := s.GetTaskRunSnapshot(ctx, runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			//nolint:nilnil // A missing run is represented as an absent snapshot.
			return nil, nil
		}
		return nil, err
	}
	if record == nil {
		//nolint:nilnil // A missing run is represented as an absent snapshot.
		return nil, nil
	}
	return record.Payload, nil
}

// FindOpenRunForPrincipal returns the most recent active or timed-out run owned by
// principalID, or (nil, nil) when the principal has no open run.
func (s *Store) FindOpenRunForPrincipal(ctx context.Context, principalID string) (*taskruns.PersistedRunSnapshot, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("task run store is not initialized")
	}
	if strings.TrimSpace(principalID) == "" {
		//nolint:nilnil // An empty principal owns no attributable run.
		return nil, nil
	}
	row := &taskRunSnapshotRow{}
	err := s.db.NewSelect().
		Model(row).
		Where("owner_principal_id = ?", principalID).
		Where("status IN (?)", bun.In([]string{string(taskverification.TaskStatusActive), string(taskverification.TaskStatusTimedOut)})).
		OrderExpr("updated_at_unix_milli DESC, run_id ASC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			//nolint:nilnil // No open run for this principal.
			return nil, nil
		}
		return nil, err
	}
	record, err := row.toRecord()
	if err != nil {
		return nil, err
	}
	return record.Payload, nil
}

// ListTaskRunSnapshots returns persisted run snapshots ordered by update time descending.
func (s *Store) ListTaskRunSnapshots(ctx context.Context) ([]TaskRunSnapshotRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("task run store is not initialized")
	}
	rows := make([]taskRunSnapshotRow, 0)
	if err := s.db.NewSelect().Model(&rows).OrderExpr("updated_at_unix_milli DESC, run_id ASC").Scan(ctx); err != nil {
		return nil, err
	}
	records := make([]TaskRunSnapshotRecord, 0, len(rows))
	for idx := range rows {
		record, err := rows[idx].toRecord()
		if err != nil {
			return nil, err
		}
		records = append(records, *record)
	}
	return records, nil
}
