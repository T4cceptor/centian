package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/uptrace/bun"
)

type legacyBenchmarkRunLinkRow struct {
	BenchmarkRunID       string
	LatestTaskRunID      sql.NullString
	LinkedTaskRunIDsJSON json.RawMessage
}

func (s *Store) migrateV5ToV6(ctx context.Context) error {
	linksByRunID, err := s.collectLegacyBenchmarkRunLinks(ctx)
	if err != nil {
		return err
	}

	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
			return fmt.Errorf("disable foreign keys for v5->v6 migration: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `ALTER TABLE benchmark_runs RENAME TO benchmark_runs_v5_legacy`); err != nil {
			return fmt.Errorf("rename legacy benchmark_runs table: %w", err)
		}
		for _, stmt := range benchmarkRunTableStatementsV6() {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("create v6 benchmark run schema: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `
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
)
SELECT
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
FROM benchmark_runs_v5_legacy`); err != nil {
			return fmt.Errorf("copy benchmark runs into v6 schema: %w", err)
		}
		if err := createBenchmarkRunTaskRunLinkTables(ctx, tx); err != nil {
			return err
		}
		for benchmarkRunID, links := range linksByRunID {
			if err := replaceBenchmarkRunTaskRunLinks(ctx, tx, benchmarkRunID, links); err != nil {
				return fmt.Errorf("backfill benchmark run task-run links for %s: %w", benchmarkRunID, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `DROP TABLE benchmark_runs_v5_legacy`); err != nil {
			return fmt.Errorf("drop legacy benchmark_runs table: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
			return fmt.Errorf("re-enable foreign keys for v5->v6 migration: %w", err)
		}
		return nil
	})
}

func (s *Store) collectLegacyBenchmarkRunLinks(ctx context.Context) (map[string][]BenchmarkRunTaskRunLinkRecord, error) {
	rows := make([]legacyBenchmarkRunLinkRow, 0)
	if err := s.db.NewRaw(`
SELECT benchmark_run_id, latest_task_run_id, linked_task_run_ids_json
FROM benchmark_runs
ORDER BY benchmark_run_id ASC`).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("read legacy benchmark run links: %w", err)
	}

	linksByRunID := make(map[string][]BenchmarkRunTaskRunLinkRecord, len(rows))
	for _, row := range rows {
		taskRunIDs, err := decodeLegacyTaskRunIDs(row.LinkedTaskRunIDsJSON)
		if err != nil {
			return nil, fmt.Errorf("decode legacy benchmark run links for %s: %w", row.BenchmarkRunID, err)
		}
		if row.LatestTaskRunID.Valid {
			taskRunIDs = append(taskRunIDs, strings.TrimSpace(row.LatestTaskRunID.String))
		}
		taskRunIDs = orderedUniqueStrings(taskRunIDs)

		links := make([]BenchmarkRunTaskRunLinkRecord, 0, len(taskRunIDs))
		for idx, taskRunID := range taskRunIDs {
			if strings.TrimSpace(taskRunID) == "" {
				continue
			}
			exists, err := s.taskRunExists(ctx, taskRunID)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, fmt.Errorf("legacy benchmark run %s references missing task run %s", row.BenchmarkRunID, taskRunID)
			}
			order := idx
			links = append(links, BenchmarkRunTaskRunLinkRecord{
				BenchmarkRunID: row.BenchmarkRunID,
				TaskRunID:      taskRunID,
				LinkOrder:      &order,
			})
		}
		linksByRunID[row.BenchmarkRunID] = links
	}
	return linksByRunID, nil
}

func decodeLegacyTaskRunIDs(payload json.RawMessage) ([]string, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	var taskRunIDs []string
	if err := json.Unmarshal(payload, &taskRunIDs); err != nil {
		return nil, err
	}
	return taskRunIDs, nil
}

func orderedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (s *Store) taskRunExists(ctx context.Context, taskRunID string) (bool, error) {
	count, err := s.db.NewSelect().Table("task_runs").Where("run_id = ?", taskRunID).Count(ctx)
	if err != nil {
		return false, fmt.Errorf("check task run %s referenced by benchmark links: %w", taskRunID, err)
	}
	return count > 0, nil
}
