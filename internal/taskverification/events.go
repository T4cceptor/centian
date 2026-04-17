package taskverification

import (
	"sync"
)

// EventStore records task lifecycle events and action-to-task links.
type EventStore interface {
	AppendTaskEvent(*TaskEvent) error
	AppendActionEventTaskContext(ActionEventTaskContext) error
	TaskEvents() ([]TaskEvent, error)
	ActionEventTaskContexts() ([]ActionEventTaskContext, error)
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
func (s *InMemoryEventStore) TaskEvents() ([]TaskEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]TaskEvent(nil), s.taskEvents...), nil
}

// ActionEventTaskContexts returns a copy of all stored action-to-task links.
func (s *InMemoryEventStore) ActionEventTaskContexts() ([]ActionEventTaskContext, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]ActionEventTaskContext(nil), s.actionTaskContexts...), nil
}
