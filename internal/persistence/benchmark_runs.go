package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
)

const benchmarkRunSchemaVersion = 1

var (
	errBenchmarkRunRecordRequired = errors.New("benchmark run record is required")
	errBenchmarkRunRowRequired    = errors.New("benchmark run row is required")
)

type benchmarkSessionRow struct {
	bun.BaseModel      `bun:"table:benchmark_sessions,alias:benchmark_sessions"`
	SessionID          string `bun:"session_id,pk"`
	SchemaVersion      int
	SuiteID            string
	SuitePath          string
	SessionPath        string
	OutputRoot         string
	TemplateID         string
	StartedAtUnixMilli int64
	EndedAtUnixMilli   sql.NullInt64
	Status             string
	RepeatCount        int
}

type benchmarkRunRow struct {
	bun.BaseModel        `bun:"table:benchmark_runs,alias:benchmark_runs"`
	BenchmarkRunID       string `bun:"benchmark_run_id,pk"`
	SchemaVersion        int
	SessionID            string
	CaseID               string
	Agent                string
	TemplateVariant      string
	Attempt              int
	TemplateID           string
	SelectedModel        string
	StartedAtUnixMilli   int64
	EndedAtUnixMilli     sql.NullInt64
	Status               string
	LatestTaskRunID      string
	LatestTaskRunStatus  string
	LinkedTaskRunIDsJSON json.RawMessage `bun:"linked_task_run_ids_json"`
	RunDir               string
	ProjectDir           string
	LogsDir              string
	AgentDir             string
	ConfigPath           string
	EventStoreMode       string
	EventStorePath       string
	RequestLogPath       string
	SelectedTemplatePath string `bun:"selected_template_path"`
	ErrorSummary         string
}

// BenchmarkSessionRecord stores relational benchmark session metadata.
type BenchmarkSessionRecord struct {
	SessionID          string `json:"sessionId"`
	SuiteID            string `json:"suiteId"`
	SuitePath          string `json:"suitePath"`
	SessionPath        string `json:"sessionPath"`
	OutputRoot         string `json:"outputRoot"`
	TemplateID         string `json:"templateId"`
	StartedAtUnixMilli int64  `json:"startedAtUnixMilli"`
	EndedAtUnixMilli   *int64 `json:"endedAtUnixMilli,omitempty"`
	Status             string `json:"status"`
	RepeatCount        int    `json:"repeatCount"`
}

// BenchmarkRunRecord stores relational benchmark run metadata.
type BenchmarkRunRecord struct {
	BenchmarkRunID       string   `json:"benchmarkRunId"`
	SessionID            string   `json:"sessionId"`
	CaseID               string   `json:"caseId"`
	Agent                string   `json:"agent"`
	TemplateVariant      string   `json:"templateVariant"`
	Attempt              int      `json:"attempt"`
	TemplateID           string   `json:"templateId"`
	SelectedModel        string   `json:"selectedModel,omitempty"`
	StartedAtUnixMilli   int64    `json:"startedAtUnixMilli"`
	EndedAtUnixMilli     *int64   `json:"endedAtUnixMilli,omitempty"`
	Status               string   `json:"status"`
	LatestTaskRunID      string   `json:"latestTaskRunId,omitempty"`
	LatestTaskRunStatus  string   `json:"latestTaskRunStatus,omitempty"`
	LinkedTaskRunIDs     []string `json:"linkedTaskRunIds,omitempty"`
	RunDir               string   `json:"runDir"`
	ProjectDir           string   `json:"projectDir"`
	LogsDir              string   `json:"logsDir"`
	AgentDir             string   `json:"agentDir"`
	ConfigPath           string   `json:"configPath"`
	EventStoreMode       string   `json:"eventStoreMode,omitempty"`
	EventStorePath       string   `json:"eventStorePath,omitempty"`
	RequestLogPath       string   `json:"requestLogPath,omitempty"`
	SelectedTemplatePath string   `json:"selectedTemplatePath,omitempty"`
	ErrorSummary         string   `json:"errorSummary,omitempty"`
}

// BenchmarkSessionFilter restricts benchmark session listing.
type BenchmarkSessionFilter struct {
	SuiteID     string
	SessionID   string
	SessionPath string
}

// BenchmarkRunFilter restricts benchmark run listing.
type BenchmarkRunFilter struct {
	SuiteID         string
	SessionID       string
	CaseID          string
	Agent           string
	TemplateVariant string
}

func createBenchmarkRunTables(ctx context.Context, db bun.IDB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS benchmark_sessions (
			session_id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL,
			suite_id TEXT NOT NULL,
			suite_path TEXT NOT NULL,
			session_path TEXT NOT NULL,
			output_root TEXT NOT NULL,
			template_id TEXT NOT NULL,
			started_at_unix_milli INTEGER NOT NULL,
			ended_at_unix_milli INTEGER,
			status TEXT NOT NULL,
			repeat_count INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_benchmark_sessions_suite_started ON benchmark_sessions(suite_id, started_at_unix_milli DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_benchmark_sessions_path ON benchmark_sessions(session_path)`,
		`CREATE TABLE IF NOT EXISTS benchmark_runs (
			benchmark_run_id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL,
			session_id TEXT NOT NULL,
			case_id TEXT NOT NULL,
			agent TEXT NOT NULL,
			template_variant TEXT NOT NULL,
			attempt INTEGER NOT NULL,
			template_id TEXT NOT NULL,
			selected_model TEXT,
			started_at_unix_milli INTEGER NOT NULL,
			ended_at_unix_milli INTEGER,
			status TEXT NOT NULL,
			latest_task_run_id TEXT,
			latest_task_run_status TEXT,
			linked_task_run_ids_json BLOB NOT NULL,
			run_dir TEXT NOT NULL,
			project_dir TEXT NOT NULL,
			logs_dir TEXT NOT NULL,
			agent_dir TEXT NOT NULL,
			config_path TEXT NOT NULL,
			event_store_mode TEXT,
			event_store_path TEXT,
			request_log_path TEXT,
			selected_template_path TEXT,
			error_summary TEXT,
			FOREIGN KEY(session_id) REFERENCES benchmark_sessions(session_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_benchmark_runs_session_case_agent_variant ON benchmark_runs(session_id, case_id, agent, template_variant, attempt)`,
		`CREATE INDEX IF NOT EXISTS idx_benchmark_runs_suite_lookup ON benchmark_runs(template_id, agent, template_variant)`,
		`CREATE INDEX IF NOT EXISTS idx_benchmark_runs_latest_task_run ON benchmark_runs(latest_task_run_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to bootstrap benchmark run schema: %w", err)
		}
	}
	return nil
}

func benchmarkSessionRowFromRecord(record *BenchmarkSessionRecord) *benchmarkSessionRow {
	if record == nil {
		return nil
	}
	row := &benchmarkSessionRow{
		SessionID:          record.SessionID,
		SchemaVersion:      benchmarkRunSchemaVersion,
		SuiteID:            record.SuiteID,
		SuitePath:          record.SuitePath,
		SessionPath:        record.SessionPath,
		OutputRoot:         record.OutputRoot,
		TemplateID:         record.TemplateID,
		StartedAtUnixMilli: record.StartedAtUnixMilli,
		Status:             record.Status,
		RepeatCount:        record.RepeatCount,
	}
	if record.EndedAtUnixMilli != nil {
		row.EndedAtUnixMilli = sql.NullInt64{Int64: *record.EndedAtUnixMilli, Valid: true}
	}
	return row
}

func (row *benchmarkSessionRow) toRecord() *BenchmarkSessionRecord {
	if row == nil {
		return nil
	}
	return &BenchmarkSessionRecord{
		SessionID:          row.SessionID,
		SuiteID:            row.SuiteID,
		SuitePath:          row.SuitePath,
		SessionPath:        row.SessionPath,
		OutputRoot:         row.OutputRoot,
		TemplateID:         row.TemplateID,
		StartedAtUnixMilli: row.StartedAtUnixMilli,
		EndedAtUnixMilli:   nullInt64Pointer(row.EndedAtUnixMilli),
		Status:             row.Status,
		RepeatCount:        row.RepeatCount,
	}
}

func benchmarkRunRowFromRecord(record *BenchmarkRunRecord) (*benchmarkRunRow, error) {
	if record == nil {
		return nil, errBenchmarkRunRecordRequired
	}
	linkedTaskRunIDsJSON, err := json.Marshal(record.LinkedTaskRunIDs)
	if err != nil {
		return nil, fmt.Errorf("marshal linked task run ids: %w", err)
	}
	row := &benchmarkRunRow{
		BenchmarkRunID:       record.BenchmarkRunID,
		SchemaVersion:        benchmarkRunSchemaVersion,
		SessionID:            record.SessionID,
		CaseID:               record.CaseID,
		Agent:                record.Agent,
		TemplateVariant:      record.TemplateVariant,
		Attempt:              record.Attempt,
		TemplateID:           record.TemplateID,
		SelectedModel:        record.SelectedModel,
		StartedAtUnixMilli:   record.StartedAtUnixMilli,
		Status:               record.Status,
		LatestTaskRunID:      record.LatestTaskRunID,
		LatestTaskRunStatus:  record.LatestTaskRunStatus,
		LinkedTaskRunIDsJSON: linkedTaskRunIDsJSON,
		RunDir:               record.RunDir,
		ProjectDir:           record.ProjectDir,
		LogsDir:              record.LogsDir,
		AgentDir:             record.AgentDir,
		ConfigPath:           record.ConfigPath,
		EventStoreMode:       record.EventStoreMode,
		EventStorePath:       record.EventStorePath,
		RequestLogPath:       record.RequestLogPath,
		SelectedTemplatePath: record.SelectedTemplatePath,
		ErrorSummary:         record.ErrorSummary,
	}
	if record.EndedAtUnixMilli != nil {
		row.EndedAtUnixMilli = sql.NullInt64{Int64: *record.EndedAtUnixMilli, Valid: true}
	}
	return row, nil
}

func (row *benchmarkRunRow) toRecord() (*BenchmarkRunRecord, error) {
	if row == nil {
		return nil, errBenchmarkRunRowRequired
	}
	record := &BenchmarkRunRecord{
		BenchmarkRunID:       row.BenchmarkRunID,
		SessionID:            row.SessionID,
		CaseID:               row.CaseID,
		Agent:                row.Agent,
		TemplateVariant:      row.TemplateVariant,
		Attempt:              row.Attempt,
		TemplateID:           row.TemplateID,
		SelectedModel:        row.SelectedModel,
		StartedAtUnixMilli:   row.StartedAtUnixMilli,
		EndedAtUnixMilli:     nullInt64Pointer(row.EndedAtUnixMilli),
		Status:               row.Status,
		LatestTaskRunID:      row.LatestTaskRunID,
		LatestTaskRunStatus:  row.LatestTaskRunStatus,
		RunDir:               row.RunDir,
		ProjectDir:           row.ProjectDir,
		LogsDir:              row.LogsDir,
		AgentDir:             row.AgentDir,
		ConfigPath:           row.ConfigPath,
		EventStoreMode:       row.EventStoreMode,
		EventStorePath:       row.EventStorePath,
		RequestLogPath:       row.RequestLogPath,
		SelectedTemplatePath: row.SelectedTemplatePath,
		ErrorSummary:         row.ErrorSummary,
	}
	if len(row.LinkedTaskRunIDsJSON) > 0 {
		if err := json.Unmarshal(row.LinkedTaskRunIDsJSON, &record.LinkedTaskRunIDs); err != nil {
			return nil, fmt.Errorf("unmarshal linked task run ids: %w", err)
		}
	}
	return record, nil
}
