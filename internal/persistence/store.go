package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	// TODO: refactor this out of here so persistence only deals with database access.
	"github.com/T4cceptor/centian/internal/taskverification"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

const schemaVersion = 3

// TaskRunSummary is the aggregated view of one persisted task run.
type TaskRunSummary struct {
	RunID            string `json:"runId"`
	TemplateID       string `json:"templateId"`
	PrincipalID      string `json:"principalId,omitempty"`
	SessionID        string `json:"sessionId,omitempty"`
	StartedAt        int64  `json:"startedAt"`
	EndedAt          *int64 `json:"endedAt,omitempty"`
	Status           string `json:"status"`
	CurrentPhase     string `json:"currentPhase"`
	CurrentNodeKind  string `json:"currentNodeKind,omitempty"`
	TaskEventCount   int    `json:"taskEventCount"`
	ActionEventCount int    `json:"actionEventCount"`
	EventCount       int    `json:"eventCount"`
}

// TaskRunEventSource identifies where one timeline row originated.
type TaskRunEventSource string

const (
	// TaskRunEventSourceTask marks a task lifecycle event.
	TaskRunEventSourceTask TaskRunEventSource = "task"
	// TaskRunEventSourceAction marks an MCP action event.
	TaskRunEventSourceAction TaskRunEventSource = "action"
)

// TaskRunEvent is the unified task timeline projection used by the UI.
type TaskRunEvent struct {
	Source             TaskRunEventSource `json:"source"`
	ID                 string             `json:"id"`
	CreatedAtUnixMilli int64              `json:"createdAtUnixMilli"`
	PayloadJSON        json.RawMessage    `json:"payloadJson,omitempty"`

	EventType              string `json:"eventType,omitempty"`
	Outcome                string `json:"outcome,omitempty"`
	RelatedActionRequestID string `json:"relatedActionRequestId,omitempty"`
	PhasePath              string `json:"phasePath,omitempty"`
	NodeKind               string `json:"nodeKind,omitempty"`
	ResultingPhasePath     string `json:"resultingPhasePath,omitempty"`
	ResultingNodeKind      string `json:"resultingNodeKind,omitempty"`

	RequestID        string `json:"requestId,omitempty"`
	Direction        string `json:"direction,omitempty"`
	MessageType      string `json:"messageType,omitempty"`
	ToolName         string `json:"toolName,omitempty"`
	OriginalToolName string `json:"originalToolName,omitempty"`
	Success          *bool  `json:"success,omitempty"`
	IsError          *bool  `json:"isError,omitempty"`
	Transport        string `json:"transport,omitempty"`
	Gateway          string `json:"gateway,omitempty"`
	ServerName       string `json:"serverName,omitempty"`
	Endpoint         string `json:"endpoint,omitempty"`
}

type taskEventRow struct {
	bun.BaseModel          `bun:"table:task_events"`
	ID                     string `bun:",pk"`
	SchemaVersion          int
	CreatedAtUnixMilli     int64
	TaskRunID              string
	SessionID              string
	TemplateID             string
	PrincipalID            string
	PhasePath              string
	NodeKind               string
	ResultingPhasePath     string
	ResultingNodeKind      string
	EventType              string
	Outcome                string
	RelatedActionRequestID string
	PayloadJSON            json.RawMessage
}

type actionEventTaskContextRow struct {
	bun.BaseModel       `bun:"table:action_event_task_context"`
	RequestID           string `bun:",pk"`
	TaskRunID           string
	InvocationPhasePath string
	InvocationNodeKind  string
	CreatedAtUnixMilli  int64
}

// ActionEventRecord is the persisted SQL projection of one MCP action log entry.
type ActionEventRecord struct {
	bun.BaseModel      `bun:"table:action_events"`
	ID                 string `bun:",pk"`
	SchemaVersion      int
	CreatedAtUnixMilli int64
	RequestID          string
	SessionID          string
	PrincipalID        string
	Transport          string
	Direction          string
	MessageType        string
	Gateway            string
	ServerName         string
	Endpoint           string
	ToolName           string
	OriginalToolName   string
	Success            bool
	IsError            bool
	PayloadJSON        json.RawMessage
}

type schemaVersionRow struct {
	bun.BaseModel `bun:"table:event_store_schema"`
	Name          string `bun:",pk"`
	Version       int
}

type taskRunSummaryRow struct {
	RunID              string
	TemplateID         string
	PrincipalID        string
	SessionID          string
	StartedAt          int64
	LatestEventAt      int64
	Status             string
	CurrentPhase       string
	CurrentNodeKind    string
	TaskEventCount     int
	ActionEventCount   int
	LatestEventType    string
	LatestEventPayload json.RawMessage
}

type taskRunEventRow struct {
	Source                 string
	ID                     string
	CreatedAtUnixMilli     int64
	PayloadJSON            json.RawMessage
	EventType              sql.NullString
	Outcome                sql.NullString
	RelatedActionRequestID sql.NullString
	PhasePath              sql.NullString
	NodeKind               sql.NullString
	ResultingPhasePath     sql.NullString
	ResultingNodeKind      sql.NullString
	RequestID              sql.NullString
	Direction              sql.NullString
	MessageType            sql.NullString
	ToolName               sql.NullString
	OriginalToolName       sql.NullString
	Success                sql.NullBool
	IsError                sql.NullBool
	Transport              sql.NullString
	Gateway                sql.NullString
	ServerName             sql.NullString
	Endpoint               sql.NullString
}

// Store persists task and action events to SQLite using Bun.
type Store struct {
	db *bun.DB
}

// NewSQLiteStore creates a Bun-backed SQLite store and bootstraps the schema.
func NewSQLiteStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite event store path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("failed to create event store directory: %w", err)
	}

	sqldb, err := sql.Open(sqliteshim.ShimName, path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite event store: %w", err)
	}

	store := &Store{
		db: bun.NewDB(sqldb, sqlitedialect.New()),
	}
	if err := store.bootstrap(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, err
	}
	return store, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) bootstrap(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS event_store_schema (
		name TEXT PRIMARY KEY,
		version INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("failed to bootstrap event store schema: %w", err)
	}

	versionRow := &schemaVersionRow{}
	err := s.db.NewSelect().Model(versionRow).Where("name = ?", "event_storage").Scan(ctx)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if err := s.createTables(ctx); err != nil {
			return err
		}
		if _, err := s.db.NewInsert().Model(&schemaVersionRow{
			Name:    "event_storage",
			Version: schemaVersion,
		}).Exec(ctx); err != nil {
			return fmt.Errorf("failed to initialize event store schema version: %w", err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("failed to inspect event store schema version: %w", err)
	case versionRow.Version != schemaVersion:
		if err := s.resetSchema(ctx); err != nil {
			return err
		}
		versionRow.Version = schemaVersion
		if _, err := s.db.NewUpdate().Model(versionRow).Column("version").WherePK().Exec(ctx); err != nil {
			return fmt.Errorf("failed to update event store schema version: %w", err)
		}
		return nil
	default:
		return s.createTables(ctx)
	}
}

func (s *Store) createTables(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS task_events (
			id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL,
			created_at_unix_milli INTEGER NOT NULL,
			task_run_id TEXT NOT NULL,
			session_id TEXT,
			template_id TEXT NOT NULL,
			principal_id TEXT,
			phase_path TEXT NOT NULL,
			node_kind TEXT,
			resulting_phase_path TEXT NOT NULL,
			resulting_node_kind TEXT,
			event_type TEXT NOT NULL,
			outcome TEXT NOT NULL,
			related_action_request_id TEXT,
			payload_json BLOB
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_events_task_run_id ON task_events(task_run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_task_events_created_at ON task_events(created_at_unix_milli)`,
		`CREATE TABLE IF NOT EXISTS action_events (
			id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL,
			created_at_unix_milli INTEGER NOT NULL,
			request_id TEXT NOT NULL,
			session_id TEXT,
			principal_id TEXT,
			transport TEXT,
			direction TEXT,
			message_type TEXT,
			gateway TEXT,
			server_name TEXT,
			endpoint TEXT,
			tool_name TEXT,
			original_tool_name TEXT,
			success BOOLEAN NOT NULL,
			is_error BOOLEAN NOT NULL,
			payload_json BLOB
		)`,
		`CREATE INDEX IF NOT EXISTS idx_action_events_request_id ON action_events(request_id)`,
		`CREATE INDEX IF NOT EXISTS idx_action_events_created_at ON action_events(created_at_unix_milli)`,
		`CREATE TABLE IF NOT EXISTS action_event_task_context (
			request_id TEXT PRIMARY KEY,
			task_run_id TEXT NOT NULL,
			invocation_phase_path TEXT NOT NULL,
			invocation_node_kind TEXT,
			created_at_unix_milli INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_action_event_task_context_task_run_id ON action_event_task_context(task_run_id)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to bootstrap event store schema: %w", err)
		}
	}
	return nil
}

func (s *Store) resetSchema(ctx context.Context) error {
	stmts := []string{
		`DROP TABLE IF EXISTS task_events`,
		`DROP TABLE IF EXISTS action_events`,
		`DROP TABLE IF EXISTS action_event_task_context`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to reset event store schema: %w", err)
		}
	}
	return s.createTables(ctx)
}

// AppendTaskEvent persists one task lifecycle event.
func (s *Store) AppendTaskEvent(event *taskverification.TaskEvent) error {
	if event == nil {
		return nil
	}
	row := taskEventRow{
		ID:                     event.ID,
		SchemaVersion:          event.SchemaVersion,
		CreatedAtUnixMilli:     event.CreatedAtUnixMilli,
		TaskRunID:              event.TaskRunID,
		SessionID:              event.SessionID,
		TemplateID:             event.TemplateID,
		PrincipalID:            event.PrincipalID,
		PhasePath:              string(event.PhasePath),
		NodeKind:               string(event.NodeKind),
		ResultingPhasePath:     string(event.ResultingPhasePath),
		ResultingNodeKind:      string(event.ResultingNodeKind),
		EventType:              string(event.EventType),
		Outcome:                string(event.Outcome),
		RelatedActionRequestID: event.RelatedActionRequestID,
		PayloadJSON:            event.Payload,
	}
	_, err := s.db.NewInsert().Model(&row).Exec(context.Background())
	return err
}

// AppendActionEventTaskContext persists one action-to-task bridge record.
func (s *Store) AppendActionEventTaskContext(ctx taskverification.ActionEventTaskContext) error {
	row := actionEventTaskContextRow{
		RequestID:           ctx.RequestID,
		TaskRunID:           ctx.TaskRunID,
		InvocationPhasePath: string(ctx.InvocationPhasePath),
		InvocationNodeKind:  string(ctx.InvocationNodeKind),
		CreatedAtUnixMilli:  ctx.CreatedAtUnixMilli,
	}
	_, err := s.db.NewInsert().Model(&row).Exec(context.Background())
	return err
}

// AppendActionEvent persists one action event projected from the MCP request log.
func (s *Store) AppendActionEvent(entry *common.LogEntry) error {
	if entry == nil {
		return nil
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal action event payload: %w", err)
	}
	timestamp := TouchTimestamp(entry.Timestamp)
	row := ActionEventRecord{
		ID:                 newActionEventRowID(),
		SchemaVersion:      schemaVersion,
		CreatedAtUnixMilli: timestamp.UnixMilli(),
		RequestID:          entry.RequestID,
		SessionID:          entry.SessionID,
		PrincipalID:        entry.Metadata["principal_id"],
		Transport:          entry.Transport,
		Direction:          string(entry.Direction),
		MessageType:        string(entry.MessageType),
		Gateway:            entry.Routing.Gateway,
		ServerName:         entry.Routing.ServerName,
		Endpoint:           entry.Routing.Endpoint,
		Success:            entry.Success,
		PayloadJSON:        payload,
	}
	if entry.ToolCall != nil {
		row.ToolName = entry.ToolCall.Name
		row.OriginalToolName = entry.ToolCall.OriginalName
		row.IsError = entry.ToolCall.IsError
	}
	_, err = s.db.NewInsert().Model(&row).Exec(context.Background())
	return err
}

// ListTaskRuns returns aggregated task run summaries ordered by start time descending.
func (s *Store) ListTaskRuns(ctx context.Context) ([]TaskRunSummary, error) {
	rows := make([]taskRunSummaryRow, 0)
	query := `
WITH task_agg AS (
	SELECT
		task_run_id,
		MIN(created_at_unix_milli) AS started_at,
		COUNT(*) AS task_event_count
	FROM task_events
	GROUP BY task_run_id
),
latest_task AS (
	SELECT te.*
	FROM task_events te
	WHERE te.id = (
		SELECT te2.id
		FROM task_events te2
		WHERE te2.task_run_id = te.task_run_id
		ORDER BY te2.created_at_unix_milli DESC, te2.id DESC
		LIMIT 1
	)
),
action_counts AS (
	SELECT
		ctx.task_run_id,
		COUNT(ae.id) AS action_event_count
	FROM action_event_task_context ctx
	JOIN action_events ae ON ae.request_id = ctx.request_id
	GROUP BY ctx.task_run_id
)
SELECT
	agg.task_run_id AS run_id,
	latest.template_id,
	latest.principal_id,
	latest.session_id,
	agg.started_at,
	latest.created_at_unix_milli AS latest_event_at,
	latest.outcome AS status,
	latest.resulting_phase_path AS current_phase,
	latest.resulting_node_kind AS current_node_kind,
	agg.task_event_count,
	COALESCE(action_counts.action_event_count, 0) AS action_event_count,
	latest.event_type AS latest_event_type,
	latest.payload_json AS latest_event_payload
FROM task_agg agg
JOIN latest_task latest ON latest.task_run_id = agg.task_run_id
LEFT JOIN action_counts ON action_counts.task_run_id = agg.task_run_id
ORDER BY agg.started_at DESC, agg.task_run_id DESC
`
	if err := s.db.NewRaw(query).Scan(ctx, &rows); err != nil {
		return nil, err
	}

	summaries := make([]TaskRunSummary, 0, len(rows))
	for idx := range rows {
		row := rows[idx]
		summary := TaskRunSummary{
			RunID:            row.RunID,
			TemplateID:       row.TemplateID,
			PrincipalID:      row.PrincipalID,
			SessionID:        row.SessionID,
			StartedAt:        row.StartedAt,
			Status:           row.Status,
			CurrentPhase:     row.CurrentPhase,
			CurrentNodeKind:  row.CurrentNodeKind,
			TaskEventCount:   row.TaskEventCount,
			ActionEventCount: row.ActionEventCount,
			EventCount:       row.TaskEventCount + row.ActionEventCount,
		}
		if isTerminalTaskEvent(row.LatestEventType, row.LatestEventPayload) {
			endedAt := row.LatestEventAt
			summary.EndedAt = &endedAt
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

// GetTaskRunEvents returns the unified task/action timeline for one run.
func (s *Store) GetTaskRunEvents(ctx context.Context, runID string) ([]TaskRunEvent, error) {
	rows := make([]taskRunEventRow, 0)
	query := `
SELECT
	'task' AS source,
	te.id,
	te.created_at_unix_milli,
	te.payload_json,
	te.event_type,
	te.outcome,
	te.related_action_request_id,
	te.phase_path,
	te.node_kind,
	te.resulting_phase_path,
	te.resulting_node_kind,
	NULL AS request_id,
	NULL AS direction,
	NULL AS message_type,
	NULL AS tool_name,
	NULL AS original_tool_name,
	NULL AS success,
	NULL AS is_error,
	NULL AS transport,
	NULL AS gateway,
	NULL AS server_name,
	NULL AS endpoint
FROM task_events te
WHERE te.task_run_id = ?
UNION ALL
SELECT
	'action' AS source,
	ae.id,
	ae.created_at_unix_milli,
	ae.payload_json,
	NULL AS event_type,
	NULL AS outcome,
	NULL AS related_action_request_id,
	NULL AS phase_path,
	NULL AS node_kind,
	NULL AS resulting_phase_path,
	NULL AS resulting_node_kind,
	ae.request_id,
	ae.direction,
	ae.message_type,
	ae.tool_name,
	ae.original_tool_name,
	ae.success,
	ae.is_error,
	ae.transport,
	ae.gateway,
	ae.server_name,
	ae.endpoint
FROM action_event_task_context ctx
JOIN action_events ae ON ae.request_id = ctx.request_id
WHERE ctx.task_run_id = ?
ORDER BY created_at_unix_milli ASC, id ASC
`
	if err := s.db.NewRaw(query, runID, runID).Scan(ctx, &rows); err != nil {
		return nil, err
	}

	events := make([]TaskRunEvent, 0, len(rows))
	for idx := range rows {
		row := rows[idx]
		events = append(events, TaskRunEvent{
			Source:                 TaskRunEventSource(row.Source),
			ID:                     row.ID,
			CreatedAtUnixMilli:     row.CreatedAtUnixMilli,
			PayloadJSON:            row.PayloadJSON,
			EventType:              nullStringValue(row.EventType),
			Outcome:                nullStringValue(row.Outcome),
			RelatedActionRequestID: nullStringValue(row.RelatedActionRequestID),
			PhasePath:              nullStringValue(row.PhasePath),
			NodeKind:               nullStringValue(row.NodeKind),
			ResultingPhasePath:     nullStringValue(row.ResultingPhasePath),
			ResultingNodeKind:      nullStringValue(row.ResultingNodeKind),
			RequestID:              nullStringValue(row.RequestID),
			Direction:              nullStringValue(row.Direction),
			MessageType:            nullStringValue(row.MessageType),
			ToolName:               nullStringValue(row.ToolName),
			OriginalToolName:       nullStringValue(row.OriginalToolName),
			Success:                nullBoolPointer(row.Success),
			IsError:                nullBoolPointer(row.IsError),
			Transport:              nullStringValue(row.Transport),
			Gateway:                nullStringValue(row.Gateway),
			ServerName:             nullStringValue(row.ServerName),
			Endpoint:               nullStringValue(row.Endpoint),
		})
	}
	return events, nil
}

// TaskEvents returns all persisted task lifecycle events ordered by timestamp.
func (s *Store) TaskEvents() []taskverification.TaskEvent {
	rows := make([]taskEventRow, 0)
	if err := s.db.NewSelect().Model(&rows).Order("created_at_unix_milli ASC").Scan(context.Background()); err != nil {
		return nil
	}
	events := make([]taskverification.TaskEvent, 0, len(rows))
	for idx := range rows {
		row := &rows[idx]
		events = append(events, taskverification.TaskEvent{
			ID:                     row.ID,
			SchemaVersion:          row.SchemaVersion,
			CreatedAtUnixMilli:     row.CreatedAtUnixMilli,
			TaskRunID:              row.TaskRunID,
			SessionID:              row.SessionID,
			TemplateID:             row.TemplateID,
			PrincipalID:            row.PrincipalID,
			PhasePath:              taskverification.TaskPhase(row.PhasePath),
			NodeKind:               taskverification.WorkflowNodeKind(row.NodeKind),
			ResultingPhasePath:     taskverification.TaskPhase(row.ResultingPhasePath),
			ResultingNodeKind:      taskverification.WorkflowNodeKind(row.ResultingNodeKind),
			EventType:              taskverification.TaskEventType(row.EventType),
			Outcome:                taskverification.TaskEventOutcome(row.Outcome),
			RelatedActionRequestID: row.RelatedActionRequestID,
			Payload:                row.PayloadJSON,
		})
	}
	return events
}

// ActionEventTaskContexts returns all persisted action-to-task bridge rows ordered by timestamp.
func (s *Store) ActionEventTaskContexts() []taskverification.ActionEventTaskContext {
	rows := make([]actionEventTaskContextRow, 0)
	if err := s.db.NewSelect().Model(&rows).Order("created_at_unix_milli ASC").Scan(context.Background()); err != nil {
		return nil
	}
	result := make([]taskverification.ActionEventTaskContext, 0, len(rows))
	for idx := range rows {
		row := &rows[idx]
		result = append(result, taskverification.ActionEventTaskContext{
			RequestID:           row.RequestID,
			TaskRunID:           row.TaskRunID,
			InvocationPhasePath: taskverification.TaskPhase(row.InvocationPhasePath),
			InvocationNodeKind:  taskverification.WorkflowNodeKind(row.InvocationNodeKind),
			CreatedAtUnixMilli:  row.CreatedAtUnixMilli,
		})
	}
	return result
}

// ActionEvents returns all persisted action events ordered by timestamp.
func (s *Store) ActionEvents() []ActionEventRecord {
	rows := make([]ActionEventRecord, 0)
	if err := s.db.NewSelect().Model(&rows).Order("created_at_unix_milli ASC").Scan(context.Background()); err != nil {
		return nil
	}
	return rows
}

// ActionEventsByRequestID returns all persisted action events for a request id.
func (s *Store) ActionEventsByRequestID(requestID string) []ActionEventRecord {
	rows := make([]ActionEventRecord, 0)
	if err := s.db.NewSelect().Model(&rows).Where("request_id = ?", requestID).Order("created_at_unix_milli ASC").Scan(context.Background()); err != nil {
		return nil
	}
	return rows
}

// ActionEventRowsByTaskRunID returns bridge rows for one task run.
func (s *Store) ActionEventRowsByTaskRunID(taskRunID string) []actionEventTaskContextRow {
	rows := make([]actionEventTaskContextRow, 0)
	if err := s.db.NewSelect().Model(&rows).Where("task_run_id = ?", taskRunID).Order("created_at_unix_milli ASC").Scan(context.Background()); err != nil {
		return nil
	}
	return rows
}

// TaskEventRowsByTaskRunID returns lifecycle rows for one task run.
func (s *Store) TaskEventRowsByTaskRunID(taskRunID string) []taskEventRow {
	rows := make([]taskEventRow, 0)
	if err := s.db.NewSelect().Model(&rows).Where("task_run_id = ?", taskRunID).Order("created_at_unix_milli ASC").Scan(context.Background()); err != nil {
		return nil
	}
	return rows
}

// DB exposes the Bun DB handle for focused tests.
func (s *Store) DB() *bun.DB {
	return s.db
}

// TouchTimestamp normalizes zero-value timestamps before persistence.
func TouchTimestamp(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Now().UTC()
	}
	return ts.UTC()
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullBoolPointer(value sql.NullBool) *bool {
	if !value.Valid {
		return nil
	}
	result := value.Bool
	return &result
}

func isTerminalTaskEvent(eventType string, payload json.RawMessage) bool {
	if eventType == string(taskverification.TaskEventTypeFailed) {
		return true
	}
	if len(payload) == 0 {
		return false
	}

	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return false
	}
	status, _ := body["status"].(string)
	return status == string(taskverification.TaskStatusCompleted) || status == string(taskverification.TaskStatusFailed)
}

func newActionEventRowID() string {
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	return "actionevent_" + time.Now().UTC().Format("20060102150405.000000000")
}
