package taskverification

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

// EventStore records task lifecycle events and action-to-task links.
type EventStore interface {
	AppendTaskEvent(TaskEvent) error
	AppendActionEventTaskContext(ActionEventTaskContext) error
	TaskEvents() []TaskEvent
	ActionEventTaskContexts() []ActionEventTaskContext
}

// InMemoryEventStore stores task events in memory for the current process.
type InMemoryEventStore struct {
	mu                 sync.RWMutex
	taskEvents         []TaskEvent
	actionTaskContexts []ActionEventTaskContext
}

// NewInMemoryEventStore creates an empty in-memory task event store.
func NewInMemoryEventStore() *InMemoryEventStore {
	return &InMemoryEventStore{
		taskEvents:         make([]TaskEvent, 0),
		actionTaskContexts: make([]ActionEventTaskContext, 0),
	}
}

func (s *InMemoryEventStore) AppendTaskEvent(event TaskEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskEvents = append(s.taskEvents, event)
	return nil
}

func (s *InMemoryEventStore) AppendActionEventTaskContext(ctx ActionEventTaskContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actionTaskContexts = append(s.actionTaskContexts, ctx)
	return nil
}

func (s *InMemoryEventStore) TaskEvents() []TaskEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]TaskEvent(nil), s.taskEvents...)
}

func (s *InMemoryEventStore) ActionEventTaskContexts() []ActionEventTaskContext {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]ActionEventTaskContext(nil), s.actionTaskContexts...)
}

func newTaskRunID() string {
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	return "taskrun_" + time.Now().UTC().Format("20060102150405.000000000")
}

func newTaskEventID() string {
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	return "taskevent_" + time.Now().UTC().Format("20060102150405.000000000")
}

func nowUnixMilli() int64 {
	return time.Now().UTC().UnixMilli()
}

func mustMarshalPayload(payload map[string]any) json.RawMessage {
	if len(payload) == 0 {
		return nil
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return encoded
}

// RecordTaskEvent appends one lifecycle event to the configured store.
func (s *Service) RecordTaskEvent(run *RunState, sessionID, principalID string, eventType TaskEventType, outcome TaskEventOutcome, relatedActionEventID string, payload map[string]any) error {
	if s == nil || s.EventStore == nil || run == nil {
		return nil
	}
	nodeKind := WorkflowNodeKind("")
	if node, exists := run.CurrentNode(); exists {
		nodeKind = node.Kind
	}
	return s.EventStore.AppendTaskEvent(TaskEvent{
		ID:                   newTaskEventID(),
		SchemaVersion:        1,
		CreatedAtUnixMilli:   nowUnixMilli(),
		TaskRunID:            run.RunID,
		SessionID:            sessionID,
		TemplateID:           run.TemplateID,
		PrincipalID:          principalID,
		PhasePath:            run.Phase,
		NodeKind:             nodeKind,
		EventType:            eventType,
		Outcome:              outcome,
		RelatedActionEventID: relatedActionEventID,
		Payload:              mustMarshalPayload(payload),
	})
}

// RecordActionEventTaskContext appends one task snapshot for an action event.
func (s *Service) RecordActionEventTaskContext(run *RunState, actionEventID string) error {
	if s == nil || s.EventStore == nil || run == nil || actionEventID == "" {
		return nil
	}
	nodeKind := WorkflowNodeKind("")
	if node, exists := run.CurrentNode(); exists {
		nodeKind = node.Kind
	}
	return s.EventStore.AppendActionEventTaskContext(ActionEventTaskContext{
		ActionEventID:      actionEventID,
		TaskRunID:          run.RunID,
		PhasePath:          run.Phase,
		NodeKind:           nodeKind,
		CreatedAtUnixMilli: nowUnixMilli(),
	})
}

// TaskEvents returns the currently recorded task lifecycle events.
func (s *Service) TaskEvents() []TaskEvent {
	if s == nil || s.EventStore == nil {
		return nil
	}
	return s.EventStore.TaskEvents()
}

// ActionEventTaskContexts returns the currently recorded action-to-task links.
func (s *Service) ActionEventTaskContexts() []ActionEventTaskContext {
	if s == nil || s.EventStore == nil {
		return nil
	}
	return s.EventStore.ActionEventTaskContexts()
}
