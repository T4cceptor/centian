package persistence

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/common"
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

	// When: aggregating activity over the window
	summary, err := store.ActivitySummary(context.Background(), &ActivityFilter{Start: start, End: end})
	assert.NilError(t, err)

	// Then: stats, category counts, interventions, and volume reflect the seed data
	assert.Equal(t, summary.RangeStartUnixMilli, start)
	assert.Equal(t, summary.RangeEndUnixMilli, end)

	assert.Equal(t, summary.Stats.Interventions, 3)
	assert.Equal(t, summary.Stats.RequestsInspected, 3)
	assert.Equal(t, summary.Stats.ThreatsNeutralized, 1) // security
	assert.Equal(t, summary.Stats.PIIRedacted, 1)        // redacted action
	assert.Equal(t, summary.Stats.RiskyActionsHeld, 1)   // hold action

	assert.Equal(t, summary.CategoryCounts.Security, 1)
	assert.Equal(t, summary.CategoryCounts.Risk, 1)
	assert.Equal(t, summary.CategoryCounts.Quality, 1)
	assert.Equal(t, summary.CategoryCounts.Policy, 0)
	assert.Equal(t, summary.CategoryCounts.Compliance, 0)

	assert.Equal(t, len(summary.Interventions), 3)
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
