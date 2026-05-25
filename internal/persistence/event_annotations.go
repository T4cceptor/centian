package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/identifiers"
	"github.com/uptrace/bun"
)

const legacyProcessorAnnotationsMetadataKey = "processor_annotations"

type eventAnnotationRow struct {
	bun.BaseModel      `bun:"table:event_annotations"`
	ID                 string `bun:",pk"`
	SchemaVersion      int
	ActionEventID      string
	RequestID          string
	CreatedAtUnixMilli int64

	Type      string
	Processor string // Suggestion: move this into "metadata" field
	Action    string // leave as it is
	Category  string
	Severity  string          // leave as it is
	Message   string          // leave as it is, make this nullable tho
	Rule      string          // not sure what that is
	Path      string          // not sure what that is
	RawJSON   json.RawMessage // leave as it is
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func createEventAnnotationTables(ctx context.Context, exec sqlExecutor) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS event_annotations (
			id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL,
			action_event_id TEXT NOT NULL,
			request_id TEXT NOT NULL,
			created_at_unix_milli INTEGER NOT NULL,
			type TEXT,
			processor TEXT,
			action TEXT,
			category TEXT,
			severity TEXT,
			message TEXT,
			rule TEXT,
			path TEXT,
			raw_json BLOB,
			FOREIGN KEY(action_event_id) REFERENCES action_events(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_event_annotations_action_event_id ON event_annotations(action_event_id)`,
		`CREATE INDEX IF NOT EXISTS idx_event_annotations_request_id ON event_annotations(request_id)`,
		`CREATE INDEX IF NOT EXISTS idx_event_annotations_created_at ON event_annotations(created_at_unix_milli)`,
	}
	for _, stmt := range stmts {
		if _, err := exec.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to bootstrap event annotation schema: %w", err)
		}
	}
	return nil
}

func newEventAnnotationRowID() string {
	return identifiers.New(identifiers.KindAnnotation)
}

func eventAnnotationRowsFromReports(
	actionEventID string,
	requestID string,
	createdAtUnixMilli int64,
	reports []common.EventAnnotation,
) ([]eventAnnotationRow, error) {
	rows := make([]eventAnnotationRow, 0, len(reports))
	for idx := range reports {
		report := &reports[idx]
		if len(report.Findings) == 0 {
			row, err := eventAnnotationRowFromReport(actionEventID, requestID, createdAtUnixMilli, report, nil)
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
			continue
		}
		for idx := range report.Findings {
			row, err := eventAnnotationRowFromReport(actionEventID, requestID, createdAtUnixMilli, report, &report.Findings[idx])
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func eventAnnotationRowFromReport(
	actionEventID string,
	requestID string,
	createdAtUnixMilli int64,
	report *common.EventAnnotation,
	finding *common.EventAnnotationFinding,
) (eventAnnotationRow, error) {
	rowReport := *report
	rule := ""
	path := ""
	if finding != nil {
		rowReport.Findings = []common.EventAnnotationFinding{*finding}
		rule = finding.Rule
		path = finding.Path
	}

	raw, err := json.Marshal(rowReport)
	if err != nil {
		return eventAnnotationRow{}, fmt.Errorf("marshal event annotation: %w", err)
	}

	return eventAnnotationRow{
		ID:                 newEventAnnotationRowID(),
		SchemaVersion:      schemaVersion,
		ActionEventID:      actionEventID,
		RequestID:          requestID,
		CreatedAtUnixMilli: createdAtUnixMilli,
		Type:               report.Type,
		Processor:          report.Processor,
		Action:             report.Action,
		Category:           report.Category,
		Severity:           report.Severity,
		Message:            report.Message,
		Rule:               rule,
		Path:               path,
		RawJSON:            raw,
	}, nil
}

func eventAnnotationFromRow(row *eventAnnotationRow) common.EventAnnotation {
	annotation := common.EventAnnotation{}
	if len(row.RawJSON) > 0 && json.Unmarshal(row.RawJSON, &annotation) == nil {
		return annotation
	}
	annotation = common.EventAnnotation{
		Type:      row.Type,
		Processor: row.Processor,
		Action:    row.Action,
		Category:  row.Category,
		Severity:  row.Severity,
		Message:   row.Message,
	}
	if row.Rule != "" || row.Path != "" {
		annotation.Findings = []common.EventAnnotationFinding{{
			Rule: row.Rule,
			Path: row.Path,
		}}
	}
	return annotation
}

func (s *Store) annotationsByActionEventID(ctx context.Context, actionEventIDs []string) (map[string][]common.EventAnnotation, error) {
	if len(actionEventIDs) == 0 {
		return map[string][]common.EventAnnotation{}, nil
	}
	uniqueIDs := orderedUniqueStrings(actionEventIDs)
	rows := make([]eventAnnotationRow, 0)
	if err := s.db.NewSelect().
		Model(&rows).
		Where("action_event_id IN (?)", bun.List(uniqueIDs)).
		Order("created_at_unix_milli ASC", "id ASC").
		Scan(ctx); err != nil {
		return nil, err
	}

	annotations := make(map[string][]common.EventAnnotation, len(uniqueIDs))
	for idx := range rows {
		row := &rows[idx]
		annotations[row.ActionEventID] = append(annotations[row.ActionEventID], eventAnnotationFromRow(row))
	}
	return annotations, nil
}

func sortedActionEventIDsFromTaskEvents(events []TaskRunEvent) []string {
	ids := make([]string, 0, len(events))
	for idx := range events {
		event := &events[idx]
		if event.Source == TaskRunEventSourceAction && event.ID != "" {
			ids = append(ids, event.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func sortedActionEventIDsFromListItems(items []EventListItem) []string {
	ids := make([]string, 0, len(items))
	for idx := range items {
		item := &items[idx]
		if item.ID != "" {
			ids = append(ids, item.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func marshalActionEventPayload(entry *common.LogEntry) ([]byte, error) {
	sanitized := *entry
	sanitized.Annotations = nil
	sanitized.Metadata = sanitizedActionEventMetadata(entry.Metadata)
	return json.Marshal(&sanitized)
}

func sanitizedActionEventMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	sanitized := make(map[string]string, len(metadata))
	for key, value := range metadata {
		if key == legacyProcessorAnnotationsMetadataKey {
			continue
		}
		sanitized[key] = value
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}
