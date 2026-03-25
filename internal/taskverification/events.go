package taskverification

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

// EventStore records task lifecycle events and action-to-task links.
type EventStore interface {
	AppendTaskEvent(*TaskEvent) error
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

// AppendTaskEvent stores one task lifecycle event in memory.
func (s *InMemoryEventStore) AppendTaskEvent(event *TaskEvent) error {
	if event == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskEvents = append(s.taskEvents, *event)
	return nil
}

// AppendActionEventTaskContext stores one action-to-task link in memory.
func (s *InMemoryEventStore) AppendActionEventTaskContext(ctx ActionEventTaskContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actionTaskContexts = append(s.actionTaskContexts, ctx)
	return nil
}

// TaskEvents returns a copy of all stored lifecycle events.
func (s *InMemoryEventStore) TaskEvents() []TaskEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]TaskEvent(nil), s.taskEvents...)
}

// ActionEventTaskContexts returns a copy of all stored action-to-task links.
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
func (s *Service) RecordTaskEvent(
	run *RunState,
	sessionID,
	principalID string,
	sourcePhase TaskPhase,
	sourceNodeKind WorkflowNodeKind,
	resultingPhase TaskPhase,
	resultingNodeKind WorkflowNodeKind,
	eventType TaskEventType,
	outcome TaskEventOutcome,
	relatedActionRequestID string,
	payload map[string]any,
) error {
	if s == nil || s.EventStore == nil || run == nil {
		return nil
	}
	event := &TaskEvent{
		ID:                     newTaskEventID(),
		SchemaVersion:          1,
		CreatedAtUnixMilli:     nowUnixMilli(),
		TaskRunID:              run.RunID,
		SessionID:              sessionID,
		TemplateID:             run.TemplateID,
		PrincipalID:            principalID,
		PhasePath:              sourcePhase,
		NodeKind:               sourceNodeKind,
		ResultingPhasePath:     resultingPhase,
		ResultingNodeKind:      resultingNodeKind,
		EventType:              eventType,
		Outcome:                outcome,
		RelatedActionRequestID: relatedActionRequestID,
		Payload:                mustMarshalPayload(payload),
	}
	return s.EventStore.AppendTaskEvent(event)
}

// RecordActionEventTaskContext appends one task snapshot for an action event.
func (s *Service) RecordActionEventTaskContext(run *RunState, requestID string, invocationPhase TaskPhase, invocationNodeKind WorkflowNodeKind) error {
	if s == nil || s.EventStore == nil || run == nil || requestID == "" {
		return nil
	}
	return s.EventStore.AppendActionEventTaskContext(ActionEventTaskContext{
		RequestID:           requestID,
		TaskRunID:           run.RunID,
		InvocationPhasePath: invocationPhase,
		InvocationNodeKind:  invocationNodeKind,
		CreatedAtUnixMilli:  nowUnixMilli(),
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
