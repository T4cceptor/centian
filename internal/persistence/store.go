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
	"github.com/T4cceptor/centian/internal/taskverification"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

const schemaVersion = 3

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

func newActionEventRowID() string {
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	return "actionevent_" + time.Now().UTC().Format("20060102150405.000000000")
}
