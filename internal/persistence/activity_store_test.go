package persistence

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/taskverification"
	"gotest.tools/assert"
)

// appendRequestEvent stores a simple proxied request event at the given time.
func appendRequestEvent(t *testing.T, store *Store, requestID string, tsMillis int64) {
	t.Helper()
	entry := &common.LogEntry{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:   time.UnixMilli(tsMillis).UTC(),
			RequestID:   requestID,
			MessageType: common.MessageTypeRequest,
			Direction:   common.DirectionClientToServer,
			Success:     true,
		},
		Routing: common.RoutingContext{Gateway: "gw", ServerName: "srv"},
	}
	entry.WithToolRequest("search", "search", json.RawMessage(`{"q":"x"}`))
	assert.NilError(t, store.AppendActionEvent(entry))
}

// appendGovernedEvent stores a governed response event carrying one annotation.
func appendGovernedEvent(t *testing.T, store *Store, requestID string, tsMillis int64, annotation common.EventAnnotation) {
	t.Helper()
	entry := &common.LogEntry{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:   time.UnixMilli(tsMillis).UTC(),
			RequestID:   requestID,
			MessageType: common.MessageTypeResponse,
			Direction:   common.DirectionServerToClient,
			Success:     true,
		},
		Routing:     common.RoutingContext{Gateway: "gw", ServerName: "srv"},
		Annotations: []common.EventAnnotation{annotation},
	}
	entry.WithToolRequest("delete_namespace", "delete_namespace", json.RawMessage(`{}`))
	entry.WithToolResult(json.RawMessage(`{"content":[]}`), false)
	assert.NilError(t, store.AppendActionEvent(entry))
}

// appendRequestEventAs stores a request event under a specific principal.
func appendRequestEventAs(t *testing.T, store *Store, requestID string, tsMillis int64, principal string) {
	t.Helper()
	entry := &common.LogEntry{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:   time.UnixMilli(tsMillis).UTC(),
			RequestID:   requestID,
			MessageType: common.MessageTypeRequest,
			Direction:   common.DirectionClientToServer,
			Success:     true,
			Metadata:    map[string]string{"principal_id": principal},
		},
		Routing: common.RoutingContext{Gateway: "gw", ServerName: "srv"},
	}
	entry.WithToolRequest("search", "search", json.RawMessage(`{"q":"x"}`))
	assert.NilError(t, store.AppendActionEvent(entry))
}

// appendGovernedEventAs stores a governed response event under a specific principal.
func appendGovernedEventAs(t *testing.T, store *Store, requestID string, tsMillis int64, principal string, annotation common.EventAnnotation) {
	t.Helper()
	entry := &common.LogEntry{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:   time.UnixMilli(tsMillis).UTC(),
			RequestID:   requestID,
			MessageType: common.MessageTypeResponse,
			Direction:   common.DirectionServerToClient,
			Success:     true,
			Metadata:    map[string]string{"principal_id": principal},
		},
		Routing:     common.RoutingContext{Gateway: "gw", ServerName: "srv"},
		Annotations: []common.EventAnnotation{annotation},
	}
	entry.WithToolRequest("delete_namespace", "delete_namespace", json.RawMessage(`{}`))
	entry.WithToolResult(json.RawMessage(`{"content":[]}`), false)
	assert.NilError(t, store.AppendActionEvent(entry))
}

func TestActivitySummaryPrincipalFilterAndDistinct(t *testing.T) {
	// Given: events under two principals within the window
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	const start, end = int64(10_000), int64(100_000)

	appendRequestEventAs(t, store, "a-req-1", 12_000, "alice")
	appendRequestEventAs(t, store, "a-req-2", 40_000, "alice")
	appendRequestEventAs(t, store, "b-req-1", 60_000, "bob")

	appendGovernedEventAs(t, store, "a-gov", 20_000, "alice", common.EventAnnotation{
		Type: "governance_events", Processor: "prompt_injection_guard", Action: "redacted",
		Category: "security", Severity: "high", Message: "Redacted a secret.",
		Findings: []common.EventAnnotationFinding{{Rule: "secret_exfiltration"}},
	})
	appendGovernedEventAs(t, store, "b-gov", 70_000, "bob", common.EventAnnotation{
		Type: "governance_events", Processor: "policy_guard", Action: "blocked",
		Category: "policy", Severity: "high", Message: "Blocked a tool.",
		Findings: []common.EventAnnotationFinding{{Rule: "tool_allowlist"}},
	})

	// When/Then: distinct principals over the window lists both
	principals, err := store.DistinctPrincipals(context.Background(), start, end)
	assert.NilError(t, err)
	assert.Equal(t, len(principals), 2)
	assert.Equal(t, principals[0], "alice")
	assert.Equal(t, principals[1], "bob")

	// When/Then: filtering to alice scopes stats and interventions to alice
	alice, err := store.ActivitySummary(context.Background(), &ActivityFilter{Start: start, End: end, Principal: "alice"})
	assert.NilError(t, err)
	assert.Equal(t, alice.Stats.RequestsInspected, 2)
	assert.Equal(t, alice.Stats.Interventions, 1)
	assert.Equal(t, alice.CategoryCounts.Security, 1)
	assert.Equal(t, alice.CategoryCounts.Policy, 0)

	// And: no filter sees everything
	all, err := store.ActivitySummary(context.Background(), &ActivityFilter{Start: start, End: end})
	assert.NilError(t, err)
	assert.Equal(t, all.Stats.RequestsInspected, 3)
	assert.Equal(t, all.Stats.Interventions, 2)
}

func TestActivitySummaryAggregatesInterventionsAndVolume(t *testing.T) {
	// Given: a store seeded with proxied requests and three governed events
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	const start, end = int64(10_000), int64(100_000)

	appendRequestEvent(t, store, "req-1", 12_000)
	appendRequestEvent(t, store, "req-2", 50_000)
	appendRequestEvent(t, store, "req-3", 90_000)

	appendGovernedEvent(t, store, "gov-sec", 20_000, common.EventAnnotation{
		Type: "governance_events", Processor: "prompt_injection_guard", Action: "redacted",
		Category: "security", Severity: "high", Message: "Redacted a secret in the tool result.",
		Findings: []common.EventAnnotationFinding{{Rule: "secret_exfiltration", Path: "payload.result"}},
	})
	appendGovernedEvent(t, store, "gov-risk", 60_000, common.EventAnnotation{
		Type: "governance_events", Processor: "blast_radius_guard", Action: "hold",
		Category: "risk", Severity: "medium", Message: "Held a destructive action for approval.",
		Findings: []common.EventAnnotationFinding{{Rule: "high_blast_radius"}},
	})
	appendGovernedEvent(t, store, "gov-qual", 80_000, common.EventAnnotation{
		Type: "governance_events", Processor: "confidence_floor", Action: "flag",
		Category: "quality", Severity: "low", Message: "Flagged a low-confidence result.",
	})
	appendGovernedEvent(t, store, "gov-pol", 85_000, common.EventAnnotation{
		Type: "governance_events", Processor: "policy_guard", Action: "blocked",
		Category: "policy", Severity: "high", Message: "Blocked a disallowed tool call.",
		Findings: []common.EventAnnotationFinding{{Rule: "tool_allowlist"}},
	})

	// When: aggregating activity over the window
	summary, err := store.ActivitySummary(context.Background(), &ActivityFilter{Start: start, End: end})
	assert.NilError(t, err)

	// Then: stats, category counts, interventions, and volume reflect the seed data
	assert.Equal(t, summary.RangeStartUnixMilli, start)
	assert.Equal(t, summary.RangeEndUnixMilli, end)

	assert.Equal(t, summary.Stats.Interventions, 4)
	assert.Equal(t, summary.Stats.RequestsInspected, 3)
	assert.Equal(t, summary.Stats.ActionsBlocked, 1) // blocked action
	assert.Equal(t, summary.Stats.Redacted, 1)       // redacted action
	// Context chars cover all action-event payloads, split by direction: the seeded
	// requests are inbound and the governed responses are outbound.
	assert.Assert(t, summary.Stats.ContextCharsIn > 0)
	assert.Assert(t, summary.Stats.ContextCharsOut > 0)

	assert.Equal(t, summary.CategoryCounts.Security, 1)
	assert.Equal(t, summary.CategoryCounts.Risk, 1)
	assert.Equal(t, summary.CategoryCounts.Quality, 1)
	assert.Equal(t, summary.CategoryCounts.Policy, 1)
	assert.Equal(t, summary.CategoryCounts.Compliance, 0)

	assert.Equal(t, len(summary.Interventions), 4)
	// Ordered ascending by timestamp; first is the high-severity security intervention.
	first := summary.Interventions[0]
	assert.Equal(t, first.Category, "security")
	assert.Equal(t, first.Severity, 0.8)
	assert.Equal(t, first.TimestampUnixMilli, int64(20_000))
	assert.Equal(t, first.RuleID, "secret_exfiltration")
	assert.Equal(t, first.Label, "Redacted") // labelled because severity >= 0.8
	assert.Assert(t, first.Title != "")
	assert.Assert(t, first.ToolName == "delete_namespace")

	// Volume is bucketed across the fixed number of buckets and totals the requests.
	assert.Equal(t, len(summary.Volume), activityVolumeBuckets)
	total := 0.0
	for _, point := range summary.Volume {
		total += point.Volume
	}
	assert.Equal(t, total, 3.0)
}

func TestActivitySummaryIncludesTaskEventGovernance(t *testing.T) {
	// Given: an action event plus a task event whose payload carries a governance
	// annotation related to that action event (the case the Events view also surfaces)
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	const start, end = int64(10_000), int64(100_000)

	appendRequestEvent(t, store, "req-task", 30_000)
	err = store.AppendTaskEvent(&taskverification.TaskEvent{
		ID:                     "te-1",
		SchemaVersion:          schemaVersion,
		CreatedAtUnixMilli:     30_500,
		TaskRunID:              "run-1",
		EventType:              taskverification.TaskEventTypeStepCompleted,
		Outcome:                taskverification.TaskEventOutcomeFailed,
		RelatedActionRequestID: "req-task",
		Payload: json.RawMessage(`{
			"annotations": [
				{
					"type": "governance_events",
					"processor": "centian",
					"action": "stopped",
					"category": "quality",
					"severity": "medium",
					"message": "Ticket was not updated."
				}
			]
		}`),
	})
	assert.NilError(t, err)

	// When: aggregating activity over the window
	summary, err := store.ActivitySummary(context.Background(), &ActivityFilter{Start: start, End: end})
	assert.NilError(t, err)

	// Then: the task-sourced governance event appears as an intervention, joined to
	// the related action event for tool context.
	assert.Equal(t, summary.Stats.Interventions, 1)
	assert.Equal(t, summary.CategoryCounts.Quality, 1)
	assert.Equal(t, len(summary.Interventions), 1)
	iv := summary.Interventions[0]
	assert.Equal(t, iv.Category, "quality")
	assert.Equal(t, iv.ToolName, "search") // from the related action event
	assert.Equal(t, iv.Severity, 0.55)
}

func TestActivitySummaryEmptyWindow(t *testing.T) {
	// Given: an empty store
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.sqlite"))
	assert.NilError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// When: aggregating activity
	summary, err := store.ActivitySummary(context.Background(), &ActivityFilter{Start: 0, End: 1000})
	assert.NilError(t, err)

	// Then: zeroed stats and a full, empty volume series
	assert.Equal(t, summary.Stats.Interventions, 0)
	assert.Equal(t, summary.Stats.RequestsInspected, 0)
	assert.Equal(t, len(summary.Interventions), 0)
	assert.Equal(t, len(summary.Volume), activityVolumeBuckets)
}
