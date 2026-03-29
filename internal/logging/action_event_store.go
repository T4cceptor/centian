package logging

import "github.com/T4cceptor/centian/internal/common"

// ActionEventStore persists MCP action events in a queryable store.
type ActionEventStore interface {
	AppendActionEvent(entry *common.LogEntry) error
}
