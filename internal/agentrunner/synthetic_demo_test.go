package agentrunner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/persistence"
	"github.com/T4cceptor/centian/internal/taskruns"
	"github.com/T4cceptor/centian/internal/taskverification"
)

func TestLoadSyntheticDemoScenarioEmbedded(t *testing.T) {
	scenario, data, err := loadSyntheticDemoScenario("")
	if err != nil {
		t.Fatalf("loadSyntheticDemoScenario: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected embedded scenario bytes")
	}
	if scenario.Version != syntheticDemoScenarioVersion {
		t.Fatalf("expected version %d, got %d", syntheticDemoScenarioVersion, scenario.Version)
	}
	if scenario.DurationMS < 20_000 || scenario.DurationMS > 30_000 {
		t.Fatalf("expected embedded scenario to last 20-30 seconds, got %dms", scenario.DurationMS)
	}
	if len(scenario.Timeline) == 0 {
		t.Fatal("expected embedded timeline items")
	}
}

func TestLoadSyntheticDemoScenarioFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scenario.json")
	if err := os.WriteFile(path, []byte(`{
		"version": 1,
		"timeline": [
			{"offsetMs": 0, "snapshot": {"templateId": "demo", "templateName": "Demo", "status": "active", "phase": "onboarding", "selectedTemplate": {"version": "0.1", "task": {"id": "demo", "name": "Demo", "description": "Demo"}}}}
		]
	}`), 0o600); err != nil {
		t.Fatalf("write scenario: %v", err)
	}

	scenario, data, err := loadSyntheticDemoScenario(path)
	if err != nil {
		t.Fatalf("loadSyntheticDemoScenario: %v", err)
	}
	if !strings.Contains(string(data), `"version": 1`) {
		t.Fatalf("expected original file bytes, got %s", string(data))
	}
	if len(scenario.Timeline) != 1 {
		t.Fatalf("expected one timeline item, got %d", len(scenario.Timeline))
	}
}

func TestLoadOpsSyntheticDemoScenario(t *testing.T) {
	data, err := asset(itOpsSyntheticDemoAsset)
	if err != nil {
		t.Fatalf("load ops demo asset: %v", err)
	}
	scenario, err := loadEmbeddedSyntheticDemoScenario(itOpsSyntheticDemoAsset)
	if err != nil {
		t.Fatalf("loadEmbeddedSyntheticDemoScenario: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected ops demo scenario bytes")
	}
	if scenario.Defaults.TemplateID != "it_incident_resolution" {
		t.Fatalf("expected it_incident_resolution template, got %q", scenario.Defaults.TemplateID)
	}

	var sawPromptInjectionAnnotation bool
	var sawAllowlistDenial bool
	var sawPreconditionFailure bool
	var sawFrozenChecksComplete bool
	for _, item := range scenario.Timeline {
		if item.ActionEvent != nil {
			event := item.ActionEvent
			for _, annotation := range event.Annotations {
				if annotation.Processor == "prompt_injection_guard" && annotation.Action == "redacted" {
					sawPromptInjectionAnnotation = true
				}
			}
			if event.Error == "restart_service is not permitted in step root_cause_analysis." {
				sawAllowlistDenial = true
			}
			if event.Error == "Cannot start step `resolution`: precondition `rca_documented` not met." {
				sawPreconditionFailure = true
			}
			if event.ToolCall != nil && event.ToolCall.Name == "centian.task_complete_step" &&
				strings.Contains(string(event.ToolCall.Result), "latency_within_target") &&
				strings.Contains(string(event.ToolCall.Result), "service_healthy") {
				sawFrozenChecksComplete = true
			}
		}
	}
	if !sawPromptInjectionAnnotation {
		t.Fatal("expected prompt injection annotation beat")
	}
	if !sawAllowlistDenial {
		t.Fatal("expected allowlist denial beat")
	}
	if !sawPreconditionFailure {
		t.Fatal("expected precondition failure beat")
	}
	if !sawFrozenChecksComplete {
		t.Fatal("expected frozen verification completion beat")
	}

	replayer := syntheticDemoReplayer{
		now: func() time.Time {
			return time.UnixMilli(1_779_318_000_000).UTC()
		},
		sleep: func(context.Context, time.Duration) error {
			return nil
		},
	}
	layout := &demoLayout{EventStorePath: filepath.Join(t.TempDir(), "events.sqlite")}
	if err := replayer.replay(context.Background(), layout, scenario); err != nil {
		t.Fatalf("replay ops scenario: %v", err)
	}
}

func TestLoadRegisteredSyntheticDemo(t *testing.T) {
	definition, scenario, err := loadRegisteredSyntheticDemo("it_ops")
	if err != nil {
		t.Fatalf("loadRegisteredSyntheticDemo: %v", err)
	}
	if definition.ID != "it_ops" {
		t.Fatalf("expected it_ops definition, got %q", definition.ID)
	}
	if scenario.Defaults.TemplateID != "it_incident_resolution" {
		t.Fatalf("expected it_incident_resolution template, got %q", scenario.Defaults.TemplateID)
	}
	if scenario.DurationMS != 72_000 {
		t.Fatalf("expected 72000ms duration, got %d", scenario.DurationMS)
	}
}

func TestValidateSyntheticDemoScenarioRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name     string
		scenario *syntheticDemoScenario
		fragment string
	}{
		{
			name:     "unsupported version",
			scenario: &syntheticDemoScenario{Version: 99, Timeline: []syntheticDemoTimelineItem{{Snapshot: &taskruns.PersistedRunSnapshot{}}}},
			fragment: "unsupported demo scenario version",
		},
		{
			name:     "unordered offsets",
			scenario: &syntheticDemoScenario{Version: 1, Timeline: []syntheticDemoTimelineItem{{OffsetMS: 10, Snapshot: &taskruns.PersistedRunSnapshot{}}, {OffsetMS: 9, TaskEvent: &taskverification.TaskEvent{}}}},
			fragment: "before the previous item",
		},
		{
			name:     "empty operation",
			scenario: &syntheticDemoScenario{Version: 1, Timeline: []syntheticDemoTimelineItem{{OffsetMS: 0}}},
			fragment: "has no operation",
		},
		{
			name: "duplicate action context",
			scenario: &syntheticDemoScenario{Version: 1, Timeline: []syntheticDemoTimelineItem{
				{OffsetMS: 0, Snapshot: &taskruns.PersistedRunSnapshot{}},
				{OffsetMS: 1, ActionContext: &taskverification.ActionEventTaskContext{RequestID: "req-1"}},
				{OffsetMS: 2, ActionContext: &taskverification.ActionEventTaskContext{RequestID: "req-1"}},
			}},
			fragment: "duplicates request id",
		},
		{
			name:     "no task data",
			scenario: &syntheticDemoScenario{Version: 1, Timeline: []syntheticDemoTimelineItem{{OffsetMS: 0, ActionEvent: &common.LogEntry{}}}},
			fragment: "must include at least one task snapshot or task event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSyntheticDemoScenario(tt.scenario)
			if err == nil || !strings.Contains(err.Error(), tt.fragment) {
				t.Fatalf("expected %q error, got %v", tt.fragment, err)
			}
		})
	}
}

func TestSyntheticDemoReplayPersistsTimelineWithoutWaiting(t *testing.T) {
	layout := &demoLayout{EventStorePath: filepath.Join(t.TempDir(), "events.sqlite")}
	scenario := &syntheticDemoScenario{
		Version:    1,
		DurationMS: 300,
		Defaults: syntheticDemoDefaults{
			RunID:        "run-1",
			SessionID:    "session-1",
			TemplateID:   "demo",
			TemplateName: "Demo",
			PrincipalID:  "principal-1",
		},
		Timeline: []syntheticDemoTimelineItem{
			{
				OffsetMS: 0,
				Snapshot: &taskruns.PersistedRunSnapshot{
					Status: "active",
					Phase:  "onboarding",
				},
			},
			{
				OffsetMS: 100,
				TaskEvent: &taskverification.TaskEvent{
					PhasePath:          taskverification.TaskPhaseOnboarding,
					ResultingPhasePath: taskverification.TaskPhasePlanning,
					EventType:          taskverification.TaskEventTypeOnboardingCompleted,
				},
			},
			{
				OffsetMS: 250,
				ActionEvent: &common.LogEntry{
					BaseMcpEvent: common.BaseMcpEvent{
						RequestID:   "request-1",
						Direction:   common.DirectionClientToServer,
						MessageType: common.MessageTypeRequest,
						Success:     true,
					},
					Routing: common.RoutingContext{ServerName: "shell"},
					ToolCall: &common.ToolCallLog{
						Name:      "shell__exec",
						Arguments: []byte(`{"command":"go test ./..."}`),
					},
				},
				ActionContext: &taskverification.ActionEventTaskContext{
					RequestID:           "request-1",
					InvocationPhasePath: taskverification.TaskPhasePlanning,
				},
			},
		},
	}
	var sleeps []time.Duration
	replayer := syntheticDemoReplayer{
		now: func() time.Time {
			return time.UnixMilli(1_742_947_200_000).UTC()
		},
		sleep: func(_ context.Context, duration time.Duration) error {
			sleeps = append(sleeps, duration)
			return nil
		},
	}

	if err := replayer.replay(context.Background(), layout, scenario); err != nil {
		t.Fatalf("replay: %v", err)
	}

	var totalSleep time.Duration
	for _, duration := range sleeps {
		totalSleep += duration
	}
	if totalSleep != 300*time.Millisecond {
		t.Fatalf("expected replay sleeps to span 300ms, got %s from %#v", totalSleep, sleeps)
	}

	store, err := persistence.NewSQLiteStore(layout.EventStorePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	summaries, err := store.ListTaskRuns(context.Background(), persistence.TaskRunFilter{})
	if err != nil {
		t.Fatalf("ListTaskRuns: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one task run, got %d", len(summaries))
	}
	if summaries[0].ActionEventCount != 1 || summaries[0].TaskEventCount != 1 {
		t.Fatalf("unexpected event counts: action=%d task=%d", summaries[0].ActionEventCount, summaries[0].TaskEventCount)
	}

	events, err := store.GetTaskRunEvents(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("GetTaskRunEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected task and action events, got %d", len(events))
	}
	if events[0].CreatedAtUnixMilli != 1_742_947_200_100 {
		t.Fatalf("expected offset timestamp, got %d", events[0].CreatedAtUnixMilli)
	}
	if events[1].ToolName != "shell__exec" {
		t.Fatalf("expected shell action event, got %q", events[1].ToolName)
	}
}

func TestSyntheticDemoReplayRemapsLogicalRequestIDsPerRun(t *testing.T) {
	layout := &demoLayout{EventStorePath: filepath.Join(t.TempDir(), "events.sqlite")}
	scenario := &syntheticDemoScenario{
		Version: 1,
		Defaults: syntheticDemoDefaults{
			RunID:        "run-1",
			TemplateID:   "demo",
			TemplateName: "Demo",
		},
		Timeline: []syntheticDemoTimelineItem{
			{
				OffsetMS: 0,
				Snapshot: &taskruns.PersistedRunSnapshot{
					Status: "active",
					Phase:  "planning",
				},
			},
			{
				OffsetMS: 1,
				ActionEvent: &common.LogEntry{
					BaseMcpEvent: common.BaseMcpEvent{
						RequestID:   "logical-request",
						Direction:   common.DirectionClientToServer,
						MessageType: common.MessageTypeRequest,
						Success:     true,
					},
					ToolCall: &common.ToolCallLog{Name: "centian.task_complete_planning"},
				},
				ActionContext: &taskverification.ActionEventTaskContext{
					RequestID:           "logical-request",
					InvocationPhasePath: taskverification.TaskPhasePlanning,
				},
			},
			{
				OffsetMS: 2,
				ActionEvent: &common.LogEntry{
					BaseMcpEvent: common.BaseMcpEvent{
						RequestID:   "logical-request",
						Direction:   common.DirectionServerToClient,
						MessageType: common.MessageTypeResponse,
						Success:     true,
					},
					ToolCall: &common.ToolCallLog{Name: "centian.task_complete_planning"},
				},
			},
		},
	}
	replayer := syntheticDemoReplayer{
		now: func() time.Time { return time.UnixMilli(1_742_947_200_000).UTC() },
		sleep: func(context.Context, time.Duration) error {
			return nil
		},
	}

	if err := replayer.replay(context.Background(), layout, scenario); err != nil {
		t.Fatalf("first replay: %v", err)
	}
	if err := replayer.replay(context.Background(), layout, scenario); err != nil {
		t.Fatalf("second replay: %v", err)
	}

	store, err := persistence.NewSQLiteStore(layout.EventStorePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	events, err := store.GetTaskRunEvents(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("GetTaskRunEvents: %v", err)
	}
	requestIDs := make(map[string]struct{})
	for _, event := range events {
		if event.RequestID != "" {
			requestIDs[event.RequestID] = struct{}{}
		}
	}
	if len(requestIDs) != 2 {
		t.Fatalf("expected one remapped request id per replay, got %d: %#v", len(requestIDs), requestIDs)
	}
}
