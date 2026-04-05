package persistence

import (
	"context"
	"fmt"
	"strings"
)

// UpsertBenchmarkArtifact persists or replaces one benchmark artifact blob.
func (s *Store) UpsertBenchmarkArtifact(ctx context.Context, record *BenchmarkArtifactRecord) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("benchmark artifact store is not initialized")
	}
	if record == nil {
		return fmt.Errorf("benchmark artifact record is required")
	}
	if strings.TrimSpace(record.ID) == "" {
		return fmt.Errorf("benchmark artifact id is required")
	}
	if strings.TrimSpace(string(record.ArtifactKind)) == "" {
		return fmt.Errorf("benchmark artifact kind is required")
	}
	if strings.TrimSpace(record.SuiteID) == "" {
		return fmt.Errorf("benchmark artifact suite id is required")
	}
	if strings.TrimSpace(record.SessionID) == "" {
		return fmt.Errorf("benchmark artifact session id is required")
	}
	if strings.TrimSpace(record.SessionPath) == "" {
		return fmt.Errorf("benchmark artifact session path is required")
	}
	if len(record.PayloadJSON) == 0 {
		return fmt.Errorf("benchmark artifact payload is required")
	}
	row := benchmarkArtifactRowFromRecord(record)
	_, err := s.db.NewInsert().
		Model(row).
		On("CONFLICT (id) DO UPDATE").
		Set("schema_version = EXCLUDED.schema_version").
		Set("artifact_kind = EXCLUDED.artifact_kind").
		Set("suite_id = EXCLUDED.suite_id").
		Set("session_id = EXCLUDED.session_id").
		Set("session_path = EXCLUDED.session_path").
		Set("run_path = EXCLUDED.run_path").
		Set("case_id = EXCLUDED.case_id").
		Set("agent = EXCLUDED.agent").
		Set("template_variant = EXCLUDED.template_variant").
		Set("attempt = EXCLUDED.attempt").
		Set("score_version = EXCLUDED.score_version").
		Set("created_at_unix_milli = EXCLUDED.created_at_unix_milli").
		Set("payload_json = EXCLUDED.payload_json").
		Exec(ctx)
	return err
}

// ListBenchmarkArtifacts returns benchmark artifact blobs filtered by lightweight metadata.
//
//nolint:gocritic // The filter is passed by value so callers can build it inline without shared mutation.
func (s *Store) ListBenchmarkArtifacts(ctx context.Context, filter BenchmarkArtifactFilter) ([]BenchmarkArtifactRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("benchmark artifact store is not initialized")
	}
	rows := make([]benchmarkArtifactRow, 0)
	query := s.db.NewSelect().Model(&rows)
	if strings.TrimSpace(string(filter.ArtifactKind)) != "" {
		query = query.Where("artifact_kind = ?", string(filter.ArtifactKind))
	}
	if strings.TrimSpace(filter.SuiteID) != "" {
		query = query.Where("suite_id = ?", filter.SuiteID)
	}
	if strings.TrimSpace(filter.SessionID) != "" {
		query = query.Where("session_id = ?", filter.SessionID)
	}
	if strings.TrimSpace(filter.SessionPath) != "" {
		query = query.Where("session_path = ?", filter.SessionPath)
	}
	if strings.TrimSpace(filter.RunPath) != "" {
		query = query.Where("run_path = ?", filter.RunPath)
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
	query = query.OrderExpr("created_at_unix_milli DESC, id ASC")
	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	records := make([]BenchmarkArtifactRecord, 0, len(rows))
	for idx := range rows {
		records = append(records, *rows[idx].toRecord())
	}
	return records, nil
}
