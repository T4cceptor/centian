package agentrunner

import (
	"context"
	"fmt"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/identifiers"
	"github.com/T4cceptor/centian/internal/persistence"
	"github.com/T4cceptor/centian/internal/taskruns"
	"github.com/T4cceptor/centian/internal/taskverification"
)

const syntheticDemoScenarioVersion = 1

type syntheticDemoScenario struct {
	Version    int                         `json:"version"`
	DurationMS int                         `json:"durationMs,omitempty"`
	Defaults   syntheticDemoDefaults       `json:"defaults,omitempty"`
	Timeline   []syntheticDemoTimelineItem `json:"timeline"`
}

type syntheticDemoDefaults struct {
	RunID         string `json:"runId,omitempty"`
	SessionID     string `json:"sessionId,omitempty"`
	TemplateID    string `json:"templateId,omitempty"`
	TemplateName  string `json:"templateName,omitempty"`
	PrincipalID   string `json:"principalId,omitempty"`
	ClientName    string `json:"clientName,omitempty"`
	ClientVersion string `json:"clientVersion,omitempty"`
}

type syntheticDemoTimelineItem struct {
	OffsetMS      int64                                    `json:"offsetMs"`
	Snapshot      *taskruns.PersistedRunSnapshot           `json:"snapshot,omitempty"`
	TaskEvent     *taskverification.TaskEvent              `json:"taskEvent,omitempty"`
	ActionEvent   *common.LogEntry                         `json:"actionEvent,omitempty"`
	ActionContext *taskverification.ActionEventTaskContext `json:"actionContext,omitempty"`
}

type syntheticDemoReplayer struct {
	now func() time.Time
}

type syntheticDemoReplayState struct {
	defaults     *syntheticDemoDefaults
	start        time.Time
	seenContexts map[string]struct{}
	requestIDs   map[string]string
}

func newSyntheticDemoReplayer() syntheticDemoReplayer {
	return syntheticDemoReplayer{
		now: time.Now,
	}
}

func validateSyntheticDemoScenario(scenario *syntheticDemoScenario) error {
	if scenario == nil {
		return fmt.Errorf("demo scenario is required")
	}
	if scenario.Version != syntheticDemoScenarioVersion {
		return fmt.Errorf("unsupported demo scenario version %d", scenario.Version)
	}
	if len(scenario.Timeline) == 0 {
		return fmt.Errorf("demo scenario timeline is required")
	}

	return validateSyntheticDemoTimeline(scenario)
}

func validateSyntheticDemoTimeline(scenario *syntheticDemoScenario) error {
	seenSnapshotOrTaskEvent := false
	seenActionContexts := make(map[string]struct{})
	var previousOffset int64
	for idx := range scenario.Timeline {
		item := scenario.Timeline[idx]
		hasSnapshotOrTaskEvent, err := validateSyntheticDemoTimelineItem(item, idx, previousOffset, scenario.DurationMS, seenActionContexts)
		if err != nil {
			return err
		}
		previousOffset = item.OffsetMS
		if hasSnapshotOrTaskEvent {
			seenSnapshotOrTaskEvent = true
		}
	}
	if !seenSnapshotOrTaskEvent {
		return fmt.Errorf("demo scenario must include at least one task snapshot or task event")
	}
	return nil
}

func validateSyntheticDemoTimelineItem(
	item syntheticDemoTimelineItem,
	idx int,
	previousOffset int64,
	durationMS int,
	seenActionContexts map[string]struct{},
) (bool, error) {
	if item.OffsetMS < 0 {
		return false, fmt.Errorf("demo scenario item %d has negative offset", idx)
	}
	if idx > 0 && item.OffsetMS < previousOffset {
		return false, fmt.Errorf("demo scenario item %d offset is before the previous item", idx)
	}
	if durationMS > 0 && item.OffsetMS > int64(durationMS) {
		return false, fmt.Errorf("demo scenario item %d exceeds duration", idx)
	}
	if item.Snapshot == nil && item.TaskEvent == nil && item.ActionEvent == nil && item.ActionContext == nil {
		return false, fmt.Errorf("demo scenario item %d has no operation", idx)
	}
	if item.ActionContext != nil && item.ActionContext.RequestID != "" {
		if _, ok := seenActionContexts[item.ActionContext.RequestID]; ok {
			return false, fmt.Errorf("demo scenario action context duplicates request id %q", item.ActionContext.RequestID)
		}
		seenActionContexts[item.ActionContext.RequestID] = struct{}{}
	}
	return item.Snapshot != nil || item.TaskEvent != nil, nil
}

func (r syntheticDemoReplayer) replay(ctx context.Context, layout *demoLayout, scenario *syntheticDemoScenario) error {
	if layout == nil {
		return fmt.Errorf("demo layout is required")
	}
	if scenario == nil {
		return fmt.Errorf("demo scenario is required")
	}
	if r.now == nil {
		r.now = time.Now
	}

	store, err := persistence.NewSQLiteStore(layout.EventStorePath)
	if err != nil {
		return fmt.Errorf("open demo event store: %w", err)
	}
	defer func() {
		_ = store.Close()
	}()

	return r.replayToStore(ctx, store, scenario)
}

func (r syntheticDemoReplayer) replayToStore(ctx context.Context, store *persistence.Store, scenario *syntheticDemoScenario) error {
	if store == nil {
		return fmt.Errorf("demo event store is required")
	}
	if scenario == nil {
		return fmt.Errorf("demo scenario is required")
	}
	if r.now == nil {
		r.now = time.Now
	}

	state := r.newReplayState(scenario)
	return r.replayToStoreFromState(ctx, store, scenario, state)
}

func (r syntheticDemoReplayer) newReplayState(scenario *syntheticDemoScenario) syntheticDemoReplayState {
	return syntheticDemoReplayState{
		defaults:     resolveSyntheticDemoDefaults(&scenario.Defaults),
		start:        r.now().UTC(),
		seenContexts: make(map[string]struct{}),
		requestIDs:   make(map[string]string),
	}
}

func (r syntheticDemoReplayer) replayToStoreFromState(ctx context.Context, store *persistence.Store, scenario *syntheticDemoScenario, state syntheticDemoReplayState) error {
	for idx := range scenario.Timeline {
		item := scenario.Timeline[idx]
		if err := applySyntheticDemoItem(ctx, store, state.defaults, state.start.Add(time.Duration(item.OffsetMS)*time.Millisecond), item, state.seenContexts, state.requestIDs); err != nil {
			return fmt.Errorf("replay demo item %d: %w", idx, err)
		}
	}

	return nil
}

func resolveSyntheticDemoDefaults(defaults *syntheticDemoDefaults) *syntheticDemoDefaults {
	if defaults == nil {
		defaults = &syntheticDemoDefaults{}
	}
	if defaults.RunID == "" {
		defaults.RunID = identifiers.New(identifiers.KindTaskRun)
	}
	if defaults.SessionID == "" {
		defaults.SessionID = identifiers.New(identifiers.KindSession)
	}
	if defaults.TemplateID == "" {
		defaults.TemplateID = "guided_tdd_workflow"
	}
	if defaults.TemplateName == "" {
		defaults.TemplateName = "Guided TDD Workflow"
	}
	if defaults.PrincipalID == "" {
		defaults.PrincipalID = "demo-user"
	}
	if defaults.ClientName == "" {
		defaults.ClientName = "centian-demo"
	}
	if defaults.ClientVersion == "" {
		defaults.ClientVersion = "synthetic"
	}
	return defaults
}

func applySyntheticDemoItem(
	ctx context.Context,
	store *persistence.Store,
	defaults *syntheticDemoDefaults,
	timestamp time.Time,
	item syntheticDemoTimelineItem,
	seenContexts map[string]struct{},
	requestIDs map[string]string,
) error {
	if item.Snapshot != nil {
		snapshot := *item.Snapshot
		applySnapshotDefaults(&snapshot, defaults, timestamp)
		if err := store.UpsertTaskRunSnapshot(ctx, &snapshot); err != nil {
			return err
		}
	}
	if item.TaskEvent != nil {
		event := *item.TaskEvent
		applyTaskEventDefaults(&event, defaults, timestamp)
		if event.RelatedActionRequestID != "" {
			event.RelatedActionRequestID = resolveSyntheticDemoRequestID(requestIDs, event.RelatedActionRequestID)
		}
		if err := store.AppendTaskEvent(&event); err != nil {
			return err
		}
	}
	resolvedActionEventRequestID := ""
	if item.ActionEvent != nil {
		event := *item.ActionEvent
		logicalRequestID := event.RequestID
		applyActionEventDefaults(&event, defaults, timestamp)
		if logicalRequestID != "" {
			event.RequestID = resolveSyntheticDemoRequestID(requestIDs, logicalRequestID)
		}
		resolvedActionEventRequestID = event.RequestID
		if err := store.AppendActionEvent(&event); err != nil {
			return err
		}
	}
	if item.ActionContext != nil {
		actionContext := *item.ActionContext
		logicalRequestID := actionContext.RequestID
		applyActionContextDefaults(&actionContext, defaults, timestamp)
		if logicalRequestID != "" {
			actionContext.RequestID = resolveSyntheticDemoRequestID(requestIDs, logicalRequestID)
		} else if resolvedActionEventRequestID != "" {
			actionContext.RequestID = resolvedActionEventRequestID
		}
		if _, ok := seenContexts[actionContext.RequestID]; ok {
			return fmt.Errorf("duplicate action context request id %q", actionContext.RequestID)
		}
		seenContexts[actionContext.RequestID] = struct{}{}
		if err := store.AppendActionEventTaskContext(actionContext); err != nil {
			return err
		}
	}
	return nil
}

func resolveSyntheticDemoRequestID(requestIDs map[string]string, logicalID string) string {
	if logicalID == "" {
		return identifiers.New(identifiers.KindRequest)
	}
	if mapped, ok := requestIDs[logicalID]; ok {
		return mapped
	}
	mapped := identifiers.New(identifiers.KindRequest)
	requestIDs[logicalID] = mapped
	return mapped
}

func applySnapshotDefaults(snapshot *taskruns.PersistedRunSnapshot, defaults *syntheticDemoDefaults, timestamp time.Time) {
	if snapshot.RunID == "" {
		snapshot.RunID = defaults.RunID
	}
	if snapshot.TemplateID == "" {
		snapshot.TemplateID = defaults.TemplateID
	}
	if snapshot.TemplateName == "" {
		snapshot.TemplateName = defaults.TemplateName
	}
	if snapshot.Status == "" {
		snapshot.Status = string(taskverification.TaskStatusActive)
	}
	if snapshot.Phase == "" {
		snapshot.Phase = string(taskverification.TaskPhaseOnboarding)
	}
	if snapshot.LastActivityAtUnixMilli == 0 {
		snapshot.LastActivityAtUnixMilli = timestamp.UnixMilli()
	}
	if snapshot.SelectedTemplate.Version == "" {
		snapshot.SelectedTemplate.Version = "0.1"
	}
	if snapshot.SelectedTemplate.Task.ID == "" {
		snapshot.SelectedTemplate.Task.ID = snapshot.TemplateID
	}
	if snapshot.SelectedTemplate.Task.Name == "" {
		snapshot.SelectedTemplate.Task.Name = snapshot.TemplateName
	}
	if snapshot.SelectedTemplate.Task.Description == "" {
		snapshot.SelectedTemplate.Task.Description = "Synthetic Centian demo task."
	}
}

func applyTaskEventDefaults(event *taskverification.TaskEvent, defaults *syntheticDemoDefaults, timestamp time.Time) {
	if event.ID == "" {
		event.ID = identifiers.New(identifiers.KindTaskEvent)
	}
	if event.SchemaVersion == 0 {
		event.SchemaVersion = 1
	}
	if event.CreatedAtUnixMilli == 0 {
		event.CreatedAtUnixMilli = timestamp.UnixMilli()
	}
	if event.TaskRunID == "" {
		event.TaskRunID = defaults.RunID
	}
	if event.SessionID == "" {
		event.SessionID = defaults.SessionID
	}
	if event.TemplateID == "" {
		event.TemplateID = defaults.TemplateID
	}
	if event.PrincipalID == "" {
		event.PrincipalID = defaults.PrincipalID
	}
	if event.ClientName == "" {
		event.ClientName = defaults.ClientName
	}
	if event.ClientVersion == "" {
		event.ClientVersion = defaults.ClientVersion
	}
	if event.PhasePath == "" {
		event.PhasePath = taskverification.TaskPhaseOnboarding
	}
	if event.ResultingPhasePath == "" {
		event.ResultingPhasePath = event.PhasePath
	}
	if event.EventType == "" {
		event.EventType = taskverification.TaskEventTypeRegistered
	}
	if event.Outcome == "" {
		event.Outcome = taskverification.TaskEventOutcomeSucceeded
	}
}

func applyActionEventDefaults(event *common.LogEntry, defaults *syntheticDemoDefaults, timestamp time.Time) {
	if event.Timestamp.IsZero() {
		event.Timestamp = timestamp
	}
	if event.RequestID == "" {
		event.RequestID = identifiers.New(identifiers.KindRequest)
	}
	if event.SessionID == "" {
		event.SessionID = defaults.SessionID
	}
	if event.Transport == "" {
		event.Transport = string(common.HTTPTransport)
	}
	if event.Direction == "" {
		event.Direction = common.DirectionClientToServer
	}
	if event.MessageType == "" {
		event.MessageType = common.MessageTypeRequest
	}
	if event.Metadata == nil {
		event.Metadata = map[string]string{}
	}
	if event.Metadata["principal_id"] == "" {
		event.Metadata["principal_id"] = defaults.PrincipalID
	}
	if event.Routing.Gateway == "" {
		event.Routing.Gateway = "taskverification"
	}
	if event.Routing.Endpoint == "" {
		event.Routing.Endpoint = "/mcp/taskverification"
	}
}

func applyActionContextDefaults(ctx *taskverification.ActionEventTaskContext, defaults *syntheticDemoDefaults, timestamp time.Time) {
	if ctx.RequestID == "" {
		ctx.RequestID = identifiers.New(identifiers.KindRequest)
	}
	if ctx.TaskRunID == "" {
		ctx.TaskRunID = defaults.RunID
	}
	if ctx.InvocationPhasePath == "" {
		ctx.InvocationPhasePath = taskverification.TaskPhaseOnboarding
	}
	if ctx.CreatedAtUnixMilli == 0 {
		ctx.CreatedAtUnixMilli = timestamp.UnixMilli()
	}
}
