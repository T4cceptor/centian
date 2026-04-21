package persistence

import (
	"context"
	"fmt"
	"sort"
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
		Set("suite_name = EXCLUDED.suite_name").
		Set("suite_path = EXCLUDED.suite_path").
		Set("session_path = EXCLUDED.session_path").
		Set("output_root = EXCLUDED.output_root").
		Set("template_id = EXCLUDED.template_id").
		Set("template_name = EXCLUDED.template_name").
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
	links := orderedBenchmarkRunTaskRunLinks(record)
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().
			Model(row).
			On("CONFLICT (benchmark_run_id) DO UPDATE").
			Set("schema_version = EXCLUDED.schema_version").
			Set("session_id = EXCLUDED.session_id").
			Set("case_id = EXCLUDED.case_id").
			Set("case_name = EXCLUDED.case_name").
			Set("agent = EXCLUDED.agent").
			Set("template_variant = EXCLUDED.template_variant").
			Set("attempt = EXCLUDED.attempt").
			Set("template_id = EXCLUDED.template_id").
			Set("template_name = EXCLUDED.template_name").
			Set("selected_model = EXCLUDED.selected_model").
			Set("started_at_unix_milli = EXCLUDED.started_at_unix_milli").
			Set("ended_at_unix_milli = EXCLUDED.ended_at_unix_milli").
			Set("status = EXCLUDED.status").
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
			Exec(ctx); err != nil {
			return err
		}
		return replaceBenchmarkRunTaskRunLinks(ctx, tx, record.BenchmarkRunID, links)
	})
}

// UpsertBenchmarkRunScore persists or replaces one benchmark run score snapshot row.
func (s *Store) UpsertBenchmarkRunScore(ctx context.Context, record *BenchmarkRunScoreRecord) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("benchmark run score store is not initialized")
	}
	if record == nil {
		return fmt.Errorf("benchmark run score record is required")
	}
	if strings.TrimSpace(record.BenchmarkRunID) == "" {
		return fmt.Errorf("benchmark run id is required")
	}
	row, err := benchmarkRunScoreRowFromRecord(record)
	if err != nil {
		return err
	}
	_, err = s.db.NewInsert().
		Model(row).
		On("CONFLICT (benchmark_run_id) DO UPDATE").
		Set("schema_version = EXCLUDED.schema_version").
		Set("score_status = EXCLUDED.score_status").
		Set("score_version = EXCLUDED.score_version").
		Set("generated_at_unix_milli = EXCLUDED.generated_at_unix_milli").
		Set("scorecard_json = EXCLUDED.scorecard_json").
		Set("score_errors_json = EXCLUDED.score_errors_json").
		Set("selected_model = EXCLUDED.selected_model").
		Set("completed_successfully = EXCLUDED.completed_successfully").
		Set("final_verification_passed = EXCLUDED.final_verification_passed").
		Set("first_pass_success = EXCLUDED.first_pass_success").
		Set("restart_occurred = EXCLUDED.restart_occurred").
		Set("fail_occurred = EXCLUDED.fail_occurred").
		Set("timeout_occurred = EXCLUDED.timeout_occurred").
		Set("invariant_violation = EXCLUDED.invariant_violation").
		Set("wall_clock_seconds = EXCLUDED.wall_clock_seconds").
		Set("total_tool_calls = EXCLUDED.total_tool_calls").
		Set("total_task_tool_calls = EXCLUDED.total_task_tool_calls").
		Set("total_downstream_tool_calls = EXCLUDED.total_downstream_tool_calls").
		Set("failed_task_tool_calls = EXCLUDED.failed_task_tool_calls").
		Set("failed_downstream_tool_calls = EXCLUDED.failed_downstream_tool_calls").
		Set("input_tokens = EXCLUDED.input_tokens").
		Set("output_tokens = EXCLUDED.output_tokens").
		Set("edited_files_count = EXCLUDED.edited_files_count").
		Set("error_actionability_score = EXCLUDED.error_actionability_score").
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
	empty, err := s.applyBenchmarkRunSessionFilter(ctx, query, filter)
	if err != nil {
		return nil, err
	}
	if empty {
		return nil, nil
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
	if err := s.populateLinkedTaskRunIDs(ctx, records); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Store) applyBenchmarkRunSessionFilter(ctx context.Context, query *bun.SelectQuery, filter *BenchmarkRunFilter) (bool, error) {
	if strings.TrimSpace(filter.SessionID) != "" {
		query.Where("session_id = ?", filter.SessionID)
		return false, nil
	}
	if strings.TrimSpace(filter.SuiteID) == "" {
		return false, nil
	}
	sessions, err := s.ListBenchmarkSessions(ctx, BenchmarkSessionFilter{SuiteID: filter.SuiteID})
	if err != nil {
		return false, err
	}
	sessionIDs := make([]string, 0, len(sessions))
	for idx := range sessions {
		sessionIDs = append(sessionIDs, sessions[idx].SessionID)
	}
	if len(sessionIDs) == 0 {
		return true, nil
	}
	query.Where("session_id IN (?)", bun.List(sessionIDs))
	return false, nil
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
	record, err := row.toRecord()
	if err != nil {
		return nil, err
	}
	records := []BenchmarkRunRecord{*record}
	if err := s.populateLinkedTaskRunIDs(ctx, records); err != nil {
		return nil, err
	}
	return &records[0], nil
}

// ListBenchmarkRunScores returns all persisted benchmark run score snapshots.
func (s *Store) ListBenchmarkRunScores(ctx context.Context) ([]BenchmarkRunScoreRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("benchmark run score store is not initialized")
	}
	rows := make([]benchmarkRunScoreRow, 0)
	if err := s.db.NewSelect().Model(&rows).Scan(ctx); err != nil {
		return nil, err
	}
	records := make([]BenchmarkRunScoreRecord, 0, len(rows))
	for idx := range rows {
		record, err := rows[idx].toRecord()
		if err != nil {
			return nil, err
		}
		records = append(records, *record)
	}
	return records, nil
}

// GetBenchmarkRunScore returns one benchmark run score snapshot by run id.
func (s *Store) GetBenchmarkRunScore(ctx context.Context, benchmarkRunID string) (*BenchmarkRunScoreRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("benchmark run score store is not initialized")
	}
	if strings.TrimSpace(benchmarkRunID) == "" {
		return nil, fmt.Errorf("benchmark run id is required")
	}
	row := &benchmarkRunScoreRow{}
	if err := s.db.NewSelect().Model(row).Where("benchmark_run_id = ?", benchmarkRunID).Scan(ctx); err != nil {
		return nil, err
	}
	return row.toRecord()
}

// ListBenchmarkRunTaskRunLinks returns benchmark/task-run links filtered by run or task.
func (s *Store) ListBenchmarkRunTaskRunLinks(ctx context.Context, filter *BenchmarkRunTaskRunLinkFilter) ([]BenchmarkRunTaskRunLinkRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("benchmark run task-run link store is not initialized")
	}
	if filter == nil {
		filter = &BenchmarkRunTaskRunLinkFilter{}
	}
	rows := make([]benchmarkRunTaskRunLinkRow, 0)
	query := s.db.NewSelect().Model(&rows)
	if strings.TrimSpace(filter.BenchmarkRunID) != "" {
		query = query.Where("benchmark_run_id = ?", filter.BenchmarkRunID)
	} else if len(filter.BenchmarkRunIDs) > 0 {
		query = query.Where("benchmark_run_id IN (?)", bun.List(filter.BenchmarkRunIDs))
	}
	if strings.TrimSpace(filter.TaskRunID) != "" {
		query = query.Where("task_run_id = ?", filter.TaskRunID)
	}
	query = query.OrderExpr("benchmark_run_id ASC, COALESCE(link_order, 2147483647) ASC, task_run_id ASC")
	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	records := make([]BenchmarkRunTaskRunLinkRecord, 0, len(rows))
	for idx := range rows {
		records = append(records, rows[idx].toRecord())
	}
	return records, nil
}

func (s *Store) populateLinkedTaskRunIDs(ctx context.Context, runs []BenchmarkRunRecord) error {
	if len(runs) == 0 {
		return nil
	}
	runIDs := make([]string, 0, len(runs))
	indexByRunID := make(map[string]int, len(runs))
	for idx := range runs {
		runIDs = append(runIDs, runs[idx].BenchmarkRunID)
		indexByRunID[runs[idx].BenchmarkRunID] = idx
	}
	links, err := s.ListBenchmarkRunTaskRunLinks(ctx, &BenchmarkRunTaskRunLinkFilter{BenchmarkRunIDs: runIDs})
	if err != nil {
		return err
	}
	for _, link := range links {
		runIdx, ok := indexByRunID[link.BenchmarkRunID]
		if !ok {
			continue
		}
		runs[runIdx].LinkedTaskRunIDs = append(runs[runIdx].LinkedTaskRunIDs, link.TaskRunID)
	}
	return nil
}

func replaceBenchmarkRunTaskRunLinks(ctx context.Context, db bun.IDB, benchmarkRunID string, links []BenchmarkRunTaskRunLinkRecord) error {
	if strings.TrimSpace(benchmarkRunID) == "" {
		return fmt.Errorf("benchmark run id is required")
	}
	if _, err := db.NewDelete().Table("benchmark_run_task_runs").Where("benchmark_run_id = ?", benchmarkRunID).Exec(ctx); err != nil {
		return err
	}
	if len(links) == 0 {
		return nil
	}
	rows := make([]*benchmarkRunTaskRunLinkRow, 0, len(links))
	for _, link := range links {
		row, err := benchmarkRunTaskRunLinkRowFromRecord(link)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	_, err := db.NewInsert().Model(&rows).Exec(ctx)
	return err
}

func orderedBenchmarkRunTaskRunLinks(record *BenchmarkRunRecord) []BenchmarkRunTaskRunLinkRecord {
	if record == nil || len(record.LinkedTaskRunIDs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(record.LinkedTaskRunIDs))
	links := make([]BenchmarkRunTaskRunLinkRecord, 0, len(record.LinkedTaskRunIDs))
	for idx, taskRunID := range record.LinkedTaskRunIDs {
		taskRunID = strings.TrimSpace(taskRunID)
		if taskRunID == "" {
			continue
		}
		if _, exists := seen[taskRunID]; exists {
			continue
		}
		seen[taskRunID] = struct{}{}
		order := idx
		links = append(links, BenchmarkRunTaskRunLinkRecord{
			BenchmarkRunID: record.BenchmarkRunID,
			TaskRunID:      taskRunID,
			LinkOrder:      &order,
		})
	}
	sort.SliceStable(links, func(i, j int) bool {
		left := links[i].LinkOrder
		right := links[j].LinkOrder
		if left == nil || right == nil {
			return links[i].TaskRunID < links[j].TaskRunID
		}
		return *left < *right
	})
	return links
}
