package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/uptrace/bun"
)

const benchmarkRunScoreSchemaVersion = 1

type benchmarkRunScoreRow struct {
	bun.BaseModel             `bun:"table:benchmark_run_scores,alias:benchmark_run_scores"`
	BenchmarkRunID            string `bun:"benchmark_run_id,pk"`
	SchemaVersion             int
	ScoreStatus               string
	ScoreVersion              string
	GeneratedAtUnixMilli      int64
	ScorecardJSON             json.RawMessage `bun:"scorecard_json"`
	ScoreErrorsJSON           json.RawMessage `bun:"score_errors_json"`
	SelectedModel             string
	CompletedSuccessfully     bool
	FinalVerificationPassed   bool
	FirstPassSuccess          bool
	RestartOccurred           bool
	FailOccurred              bool
	TimeoutOccurred           bool
	InvariantViolation        bool
	WallClockSeconds          float64
	TotalToolCalls            int
	TotalTaskToolCalls        int
	TotalDownstreamToolCalls  int
	FailedTaskToolCalls       int
	FailedDownstreamToolCalls int
	InputTokens               sql.NullInt64
	OutputTokens              sql.NullInt64
	EditedFilesCount          int
	ErrorActionabilityScore   sql.NullInt64
}

// BenchmarkRunScoreRecord stores the persisted DB-first score snapshot for one benchmark run.
type BenchmarkRunScoreRecord struct {
	BenchmarkRunID            string          `json:"benchmarkRunId"`
	ScoreStatus               string          `json:"scoreStatus"`
	ScoreVersion              string          `json:"scoreVersion,omitempty"`
	GeneratedAtUnixMilli      int64           `json:"generatedAtUnixMilli"`
	ScorecardJSON             json.RawMessage `json:"scorecardJson,omitempty"`
	ScoreErrors               []string        `json:"scoreErrors,omitempty"`
	SelectedModel             string          `json:"selectedModel,omitempty"`
	CompletedSuccessfully     bool            `json:"completedSuccessfully"`
	FinalVerificationPassed   bool            `json:"finalVerificationPassed"`
	FirstPassSuccess          bool            `json:"firstPassSuccess"`
	RestartOccurred           bool            `json:"restartOccurred"`
	FailOccurred              bool            `json:"failOccurred"`
	TimeoutOccurred           bool            `json:"timeoutOccurred"`
	InvariantViolation        bool            `json:"invariantViolation"`
	WallClockSeconds          float64         `json:"wallClockSeconds"`
	TotalToolCalls            int             `json:"totalToolCalls"`
	TotalTaskToolCalls        int             `json:"totalTaskToolCalls"`
	TotalDownstreamToolCalls  int             `json:"totalDownstreamToolCalls"`
	FailedTaskToolCalls       int             `json:"failedTaskToolCalls"`
	FailedDownstreamToolCalls int             `json:"failedDownstreamToolCalls"`
	InputTokens               *int64          `json:"inputTokens,omitempty"`
	OutputTokens              *int64          `json:"outputTokens,omitempty"`
	EditedFilesCount          int             `json:"editedFilesCount"`
	ErrorActionabilityScore   *int            `json:"errorActionabilityScore,omitempty"`
}

func createBenchmarkRunScoreTables(ctx context.Context, db bun.IDB) error {
	// TODO: move to migration?
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS benchmark_run_scores (
			benchmark_run_id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL,
			score_status TEXT NOT NULL,
			score_version TEXT,
			generated_at_unix_milli INTEGER NOT NULL,
			scorecard_json BLOB,
			score_errors_json BLOB NOT NULL,
			selected_model TEXT,
			completed_successfully BOOLEAN NOT NULL,
			final_verification_passed BOOLEAN NOT NULL,
			first_pass_success BOOLEAN NOT NULL,
			restart_occurred BOOLEAN NOT NULL,
			fail_occurred BOOLEAN NOT NULL,
			timeout_occurred BOOLEAN NOT NULL,
			invariant_violation BOOLEAN NOT NULL,
			wall_clock_seconds REAL NOT NULL,
			total_tool_calls INTEGER NOT NULL,
			total_task_tool_calls INTEGER NOT NULL,
			total_downstream_tool_calls INTEGER NOT NULL,
			failed_task_tool_calls INTEGER NOT NULL,
			failed_downstream_tool_calls INTEGER NOT NULL,
			input_tokens INTEGER,
			output_tokens INTEGER,
			edited_files_count INTEGER NOT NULL,
			error_actionability_score INTEGER,
			FOREIGN KEY(benchmark_run_id) REFERENCES benchmark_runs(benchmark_run_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_benchmark_run_scores_status ON benchmark_run_scores(score_status)`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to bootstrap benchmark run score schema: %w", err)
		}
	}
	return nil
}

func benchmarkRunScoreRowFromRecord(record *BenchmarkRunScoreRecord) (*benchmarkRunScoreRow, error) {
	if record == nil {
		return nil, fmt.Errorf("benchmark run score record is required")
	}
	scoreErrorsJSON, err := json.Marshal(record.ScoreErrors)
	if err != nil {
		return nil, fmt.Errorf("marshal benchmark run score errors: %w", err)
	}
	row := &benchmarkRunScoreRow{
		BenchmarkRunID:            record.BenchmarkRunID,
		SchemaVersion:             benchmarkRunScoreSchemaVersion,
		ScoreStatus:               record.ScoreStatus,
		ScoreVersion:              record.ScoreVersion,
		GeneratedAtUnixMilli:      record.GeneratedAtUnixMilli,
		ScorecardJSON:             append(json.RawMessage(nil), record.ScorecardJSON...),
		ScoreErrorsJSON:           scoreErrorsJSON,
		SelectedModel:             record.SelectedModel,
		CompletedSuccessfully:     record.CompletedSuccessfully,
		FinalVerificationPassed:   record.FinalVerificationPassed,
		FirstPassSuccess:          record.FirstPassSuccess,
		RestartOccurred:           record.RestartOccurred,
		FailOccurred:              record.FailOccurred,
		TimeoutOccurred:           record.TimeoutOccurred,
		InvariantViolation:        record.InvariantViolation,
		WallClockSeconds:          record.WallClockSeconds,
		TotalToolCalls:            record.TotalToolCalls,
		TotalTaskToolCalls:        record.TotalTaskToolCalls,
		TotalDownstreamToolCalls:  record.TotalDownstreamToolCalls,
		FailedTaskToolCalls:       record.FailedTaskToolCalls,
		FailedDownstreamToolCalls: record.FailedDownstreamToolCalls,
		EditedFilesCount:          record.EditedFilesCount,
	}
	if record.InputTokens != nil {
		row.InputTokens = sql.NullInt64{Int64: *record.InputTokens, Valid: true}
	}
	if record.OutputTokens != nil {
		row.OutputTokens = sql.NullInt64{Int64: *record.OutputTokens, Valid: true}
	}
	if record.ErrorActionabilityScore != nil {
		row.ErrorActionabilityScore = sql.NullInt64{Int64: int64(*record.ErrorActionabilityScore), Valid: true}
	}
	return row, nil
}

func (row *benchmarkRunScoreRow) toRecord() (*BenchmarkRunScoreRecord, error) {
	if row == nil {
		return nil, fmt.Errorf("benchmark run score row is required")
	}
	record := &BenchmarkRunScoreRecord{
		BenchmarkRunID:            row.BenchmarkRunID,
		ScoreStatus:               row.ScoreStatus,
		ScoreVersion:              row.ScoreVersion,
		GeneratedAtUnixMilli:      row.GeneratedAtUnixMilli,
		ScorecardJSON:             append(json.RawMessage(nil), row.ScorecardJSON...),
		SelectedModel:             row.SelectedModel,
		CompletedSuccessfully:     row.CompletedSuccessfully,
		FinalVerificationPassed:   row.FinalVerificationPassed,
		FirstPassSuccess:          row.FirstPassSuccess,
		RestartOccurred:           row.RestartOccurred,
		FailOccurred:              row.FailOccurred,
		TimeoutOccurred:           row.TimeoutOccurred,
		InvariantViolation:        row.InvariantViolation,
		WallClockSeconds:          row.WallClockSeconds,
		TotalToolCalls:            row.TotalToolCalls,
		TotalTaskToolCalls:        row.TotalTaskToolCalls,
		TotalDownstreamToolCalls:  row.TotalDownstreamToolCalls,
		FailedTaskToolCalls:       row.FailedTaskToolCalls,
		FailedDownstreamToolCalls: row.FailedDownstreamToolCalls,
		InputTokens:               nullInt64Pointer(row.InputTokens),
		OutputTokens:              nullInt64Pointer(row.OutputTokens),
		EditedFilesCount:          row.EditedFilesCount,
	}
	if row.ErrorActionabilityScore.Valid {
		value := int(row.ErrorActionabilityScore.Int64)
		record.ErrorActionabilityScore = &value
	}
	if len(row.ScoreErrorsJSON) > 0 {
		if err := json.Unmarshal(row.ScoreErrorsJSON, &record.ScoreErrors); err != nil {
			return nil, fmt.Errorf("unmarshal benchmark run score errors: %w", err)
		}
	}
	return record, nil
}
