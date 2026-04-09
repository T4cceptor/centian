package persistence

import (
	"context"
	"fmt"
	"strings"

	"github.com/uptrace/bun"
)

// UpsertBenchmarkSession persists or replaces one benchmark session row.
func (s *Store) UpsertBenchmarkSession(ctx context.Context, record *BenchmarkSessionRecord) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("benchmark session store is not initialized")
	}
	if record == nil {
		return fmt.Errorf("benchmark session record is required")
	}
	if strings.TrimSpace(record.SessionID) == "" {
		return fmt.Errorf("benchmark session id is required")
	}
	if strings.TrimSpace(record.SuiteID) == "" {
		return fmt.Errorf("benchmark suite id is required")
	}
	if strings.TrimSpace(record.SuitePath) == "" {
		return fmt.Errorf("benchmark suite path is required")
	}
	if strings.TrimSpace(record.SessionPath) == "" {
		return fmt.Errorf("benchmark session path is required")
	}
	row := benchmarkSessionRowFromRecord(record)
	_, err := s.db.NewInsert().
		Model(row).
		On("CONFLICT (session_id) DO UPDATE").
		Set("schema_version = EXCLUDED.schema_version").
		Set("suite_id = EXCLUDED.suite_id").
		Set("suite_path = EXCLUDED.suite_path").
		Set("session_path = EXCLUDED.session_path").
		Set("output_root = EXCLUDED.output_root").
		Set("template_id = EXCLUDED.template_id").
		Set("started_at_unix_milli = EXCLUDED.started_at_unix_milli").
		Set("ended_at_unix_milli = EXCLUDED.ended_at_unix_milli").
		Set("status = EXCLUDED.status").
		Set("repeat_count = EXCLUDED.repeat_count").
		Exec(ctx)
	return err
}

// UpsertBenchmarkRun persists or replaces one benchmark run row.
func (s *Store) UpsertBenchmarkRun(ctx context.Context, record *BenchmarkRunRecord) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("benchmark run store is not initialized")
	}
	if record == nil {
		return fmt.Errorf("benchmark run record is required")
	}
	if strings.TrimSpace(record.BenchmarkRunID) == "" {
		return fmt.Errorf("benchmark run id is required")
	}
	if strings.TrimSpace(record.SessionID) == "" {
		return fmt.Errorf("benchmark session id is required")
	}
	if strings.TrimSpace(record.RunDir) == "" {
		return fmt.Errorf("benchmark run dir is required")
	}
	row, err := benchmarkRunRowFromRecord(record)
	if err != nil {
		return err
	}
	_, err = s.db.NewInsert().
		Model(row).
		On("CONFLICT (benchmark_run_id) DO UPDATE").
		Set("schema_version = EXCLUDED.schema_version").
		Set("session_id = EXCLUDED.session_id").
		Set("case_id = EXCLUDED.case_id").
		Set("agent = EXCLUDED.agent").
		Set("template_variant = EXCLUDED.template_variant").
		Set("attempt = EXCLUDED.attempt").
		Set("template_id = EXCLUDED.template_id").
		Set("selected_model = EXCLUDED.selected_model").
		Set("started_at_unix_milli = EXCLUDED.started_at_unix_milli").
		Set("ended_at_unix_milli = EXCLUDED.ended_at_unix_milli").
		Set("status = EXCLUDED.status").
		Set("latest_task_run_id = EXCLUDED.latest_task_run_id").
		Set("latest_task_run_status = EXCLUDED.latest_task_run_status").
		Set("linked_task_run_ids_json = EXCLUDED.linked_task_run_ids_json").
		Set("run_dir = EXCLUDED.run_dir").
		Set("project_dir = EXCLUDED.project_dir").
		Set("logs_dir = EXCLUDED.logs_dir").
		Set("agent_dir = EXCLUDED.agent_dir").
		Set("config_path = EXCLUDED.config_path").
		Set("event_store_mode = EXCLUDED.event_store_mode").
		Set("event_store_path = EXCLUDED.event_store_path").
		Set("request_log_path = EXCLUDED.request_log_path").
		Set("selected_template_path = EXCLUDED.selected_template_path").
		Set("error_summary = EXCLUDED.error_summary").
		Set("agent_metadata_json = EXCLUDED.agent_metadata_json").
		Exec(ctx)
	return err
}

// ListBenchmarkSessions returns relational benchmark sessions filtered by lightweight metadata.
func (s *Store) ListBenchmarkSessions(ctx context.Context, filter BenchmarkSessionFilter) ([]BenchmarkSessionRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("benchmark session store is not initialized")
	}
	rows := make([]benchmarkSessionRow, 0)
	query := s.db.NewSelect().Model(&rows)
	if strings.TrimSpace(filter.SuiteID) != "" {
		query = query.Where("suite_id = ?", filter.SuiteID)
	}
	if strings.TrimSpace(filter.SessionID) != "" {
		query = query.Where("session_id = ?", filter.SessionID)
	}
	if strings.TrimSpace(filter.SessionPath) != "" {
		query = query.Where("session_path = ?", filter.SessionPath)
	}
	query = query.OrderExpr("started_at_unix_milli DESC, session_id ASC")
	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	records := make([]BenchmarkSessionRecord, 0, len(rows))
	for idx := range rows {
		records = append(records, *rows[idx].toRecord())
	}
	return records, nil
}

// ListBenchmarkRuns returns relational benchmark runs filtered by lightweight metadata.
func (s *Store) ListBenchmarkRuns(ctx context.Context, filter *BenchmarkRunFilter) ([]BenchmarkRunRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("benchmark run store is not initialized")
	}
	if filter == nil {
		filter = &BenchmarkRunFilter{}
	}
	rows := make([]benchmarkRunRow, 0)
	query := s.db.NewSelect().Model(&rows)
	if strings.TrimSpace(filter.SessionID) != "" {
		query = query.Where("session_id = ?", filter.SessionID)
	} else if strings.TrimSpace(filter.SuiteID) != "" {
		sessions, err := s.ListBenchmarkSessions(ctx, BenchmarkSessionFilter{SuiteID: filter.SuiteID})
		if err != nil {
			return nil, err
		}
		sessionIDs := make([]string, 0, len(sessions))
		for idx := range sessions {
			sessionIDs = append(sessionIDs, sessions[idx].SessionID)
		}
		if len(sessionIDs) == 0 {
			return nil, nil
		}
		query = query.Where("session_id IN (?)", bun.List(sessionIDs))
	}
	if strings.TrimSpace(filter.CaseID) != "" {
		query = query.Where("case_id = ?", filter.CaseID)
	}
	if strings.TrimSpace(filter.Agent) != "" {
		query = query.Where("agent = ?", filter.Agent)
	}
	if strings.TrimSpace(filter.TemplateVariant) != "" {
		query = query.Where("template_variant = ?", filter.TemplateVariant)
	}
	query = query.OrderExpr("benchmark_runs.started_at_unix_milli DESC, benchmark_runs.benchmark_run_id ASC")
	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	records := make([]BenchmarkRunRecord, 0, len(rows))
	for idx := range rows {
		record, err := rows[idx].toRecord()
		if err != nil {
			return nil, err
		}
		records = append(records, *record)
	}
	return records, nil
}

// GetBenchmarkSession returns one benchmark session by id.
func (s *Store) GetBenchmarkSession(ctx context.Context, sessionID string) (*BenchmarkSessionRecord, error) {
	rows, err := s.ListBenchmarkSessions(ctx, BenchmarkSessionFilter{SessionID: sessionID})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

// GetBenchmarkRun returns one benchmark run by id.
func (s *Store) GetBenchmarkRun(ctx context.Context, benchmarkRunID string) (*BenchmarkRunRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("benchmark run store is not initialized")
	}
	if strings.TrimSpace(benchmarkRunID) == "" {
		return nil, fmt.Errorf("benchmark run id is required")
	}
	row := &benchmarkRunRow{}
	if err := s.db.NewSelect().Model(row).Where("benchmark_run_id = ?", benchmarkRunID).Scan(ctx); err != nil {
		return nil, err
	}
	return row.toRecord()
}
