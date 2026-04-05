package persistence

import (
	"context"
	"encoding/json"

	"github.com/uptrace/bun"
)

const benchmarkArtifactSchemaVersion = 1

// BenchmarkArtifactKind identifies one persisted benchmark artifact type.
type BenchmarkArtifactKind string

// Supported persisted benchmark artifact kinds.
const (
	BenchmarkArtifactKindSession BenchmarkArtifactKind = "session"
	BenchmarkArtifactKindRun     BenchmarkArtifactKind = "run"
)

type benchmarkArtifactRow struct {
	bun.BaseModel      `bun:"table:benchmark_artifacts"`
	ID                 string `bun:",pk"`
	SchemaVersion      int
	ArtifactKind       string
	SuiteID            string
	SessionID          string
	SessionPath        string
	RunPath            string
	CaseID             string
	Agent              string
	TemplateVariant    string
	Attempt            *int
	ScoreVersion       string
	CreatedAtUnixMilli int64
	PayloadJSON        json.RawMessage
}

// BenchmarkArtifactRecord is the generic persisted representation of one benchmark artifact.
type BenchmarkArtifactRecord struct {
	ID                 string                `json:"id"`
	ArtifactKind       BenchmarkArtifactKind `json:"artifactKind"`
	SuiteID            string                `json:"suiteId"`
	SessionID          string                `json:"sessionId"`
	SessionPath        string                `json:"sessionPath"`
	RunPath            string                `json:"runPath,omitempty"`
	CaseID             string                `json:"caseId,omitempty"`
	Agent              string                `json:"agent,omitempty"`
	TemplateVariant    string                `json:"templateVariant,omitempty"`
	Attempt            *int                  `json:"attempt,omitempty"`
	ScoreVersion       string                `json:"scoreVersion,omitempty"`
	CreatedAtUnixMilli int64                 `json:"createdAtUnixMilli"`
	PayloadJSON        json.RawMessage       `json:"payloadJson"`
}

// BenchmarkArtifactFilter restricts benchmark artifact listing.
type BenchmarkArtifactFilter struct {
	ArtifactKind    BenchmarkArtifactKind
	SuiteID         string
	SessionID       string
	SessionPath     string
	RunPath         string
	CaseID          string
	Agent           string
	TemplateVariant string
}

func createBenchmarkArtifactTables(ctx context.Context, db bun.IDB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS benchmark_artifacts (
			id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL,
			artifact_kind TEXT NOT NULL,
			suite_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			session_path TEXT NOT NULL,
			run_path TEXT,
			case_id TEXT,
			agent TEXT,
			template_variant TEXT,
			attempt INTEGER,
			score_version TEXT,
			created_at_unix_milli INTEGER NOT NULL,
			payload_json BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_benchmark_artifacts_suite_kind_created_at ON benchmark_artifacts(suite_id, artifact_kind, created_at_unix_milli DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_benchmark_artifacts_suite_session_kind ON benchmark_artifacts(suite_id, session_id, artifact_kind)`,
		`CREATE INDEX IF NOT EXISTS idx_benchmark_artifacts_suite_case_agent_variant_kind ON benchmark_artifacts(suite_id, case_id, agent, template_variant, artifact_kind)`,
		`CREATE INDEX IF NOT EXISTS idx_benchmark_artifacts_session_run_kind ON benchmark_artifacts(session_id, run_path, artifact_kind)`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func benchmarkArtifactRowFromRecord(record *BenchmarkArtifactRecord) *benchmarkArtifactRow {
	if record == nil {
		return nil
	}
	return &benchmarkArtifactRow{
		ID:                 record.ID,
		SchemaVersion:      benchmarkArtifactSchemaVersion,
		ArtifactKind:       string(record.ArtifactKind),
		SuiteID:            record.SuiteID,
		SessionID:          record.SessionID,
		SessionPath:        record.SessionPath,
		RunPath:            record.RunPath,
		CaseID:             record.CaseID,
		Agent:              record.Agent,
		TemplateVariant:    record.TemplateVariant,
		Attempt:            record.Attempt,
		ScoreVersion:       record.ScoreVersion,
		CreatedAtUnixMilli: record.CreatedAtUnixMilli,
		PayloadJSON:        record.PayloadJSON,
	}
}

func (row *benchmarkArtifactRow) toRecord() *BenchmarkArtifactRecord {
	if row == nil {
		return nil
	}
	return &BenchmarkArtifactRecord{
		ID:                 row.ID,
		ArtifactKind:       BenchmarkArtifactKind(row.ArtifactKind),
		SuiteID:            row.SuiteID,
		SessionID:          row.SessionID,
		SessionPath:        row.SessionPath,
		RunPath:            row.RunPath,
		CaseID:             row.CaseID,
		Agent:              row.Agent,
		TemplateVariant:    row.TemplateVariant,
		Attempt:            row.Attempt,
		ScoreVersion:       row.ScoreVersion,
		CreatedAtUnixMilli: row.CreatedAtUnixMilli,
		PayloadJSON:        row.PayloadJSON,
	}
}
