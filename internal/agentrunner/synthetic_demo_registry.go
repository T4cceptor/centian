package agentrunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/identifiers"
	"github.com/T4cceptor/centian/internal/persistence"
	"github.com/T4cceptor/centian/internal/taskverification"
)

const itOpsSyntheticDemoAsset = "it_ops_synthetic_demo.json"

// ErrSyntheticDemoNotFound indicates that a requested bundled demo is unknown.
var ErrSyntheticDemoNotFound = errors.New("synthetic demo not found")

// SyntheticDemoDefinition describes one UI-playable synthetic demo.
type SyntheticDemoDefinition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	DurationMS  int    `json:"durationMs"`
}

// SyntheticDemoRun describes a started synthetic demo replay.
type SyntheticDemoRun struct {
	RunID      string `json:"runId"`
	DemoID     string `json:"demoId"`
	DurationMS int    `json:"durationMs"`
}

var syntheticDemoDefinitions = []SyntheticDemoDefinition{
	{
		ID:          "it_ops",
		Name:        "IT Ops Incident",
		Description: "Replay a governed platform incident with prompt-injection redaction, tool allowlist denial, precondition failure, and frozen verification.",
		DurationMS:  72_000,
	},
}

// ListSyntheticDemos returns the UI-playable synthetic demos bundled with Centian.
func ListSyntheticDemos() []SyntheticDemoDefinition {
	result := make([]SyntheticDemoDefinition, len(syntheticDemoDefinitions))
	copy(result, syntheticDemoDefinitions)
	return result
}

// StartSyntheticDemoRun starts one bundled synthetic demo replay against the provided store.
func StartSyntheticDemoRun(ctx context.Context, store *persistence.Store, demoID string) (*SyntheticDemoRun, error) {
	if store == nil {
		return nil, fmt.Errorf("event store is required")
	}
	definition, scenario, err := loadRegisteredSyntheticDemo(demoID)
	if err != nil {
		return nil, err
	}

	replayer := newSyntheticDemoReplayer()
	state := replayer.newReplayState(scenario)
	run := &SyntheticDemoRun{
		RunID:      state.defaults.RunID,
		DemoID:     definition.ID,
		DurationMS: scenario.DurationMS,
	}

	if len(scenario.Timeline) > 0 && scenario.Timeline[0].OffsetMS == 0 {
		item := scenario.Timeline[0]
		if err := applySyntheticDemoItem(ctx, store, state.defaults, state.start, item, state.seenContexts, state.requestIDs); err != nil {
			return nil, fmt.Errorf("start demo replay: %w", err)
		}
		state.previousOffset = item.OffsetMS
		state.nextIndex = 1
	}

	go func() {
		if err := replayer.replayToStoreFromState(context.Background(), store, scenario, state); err != nil {
			common.LogWarn("Synthetic demo %s failed for run %s: %v", definition.ID, run.RunID, err)
			recordSyntheticDemoFailure(store, state.defaults, err)
		}
	}()

	return run, nil
}

func loadRegisteredSyntheticDemo(demoID string) (*SyntheticDemoDefinition, *syntheticDemoScenario, error) {
	for idx := range syntheticDemoDefinitions {
		definition := &syntheticDemoDefinitions[idx]
		if definition.ID != demoID {
			continue
		}
		scenario, err := loadEmbeddedSyntheticDemoScenario(itOpsSyntheticDemoAsset)
		if err != nil {
			return nil, nil, err
		}
		return definition, scenario, nil
	}
	return nil, nil, fmt.Errorf("%w: %s", ErrSyntheticDemoNotFound, demoID)
}

func loadEmbeddedSyntheticDemoScenario(assetName string) (*syntheticDemoScenario, error) {
	content, err := asset(assetName)
	if err != nil {
		return nil, err
	}
	var scenario syntheticDemoScenario
	if err := json.Unmarshal([]byte(content), &scenario); err != nil {
		return nil, fmt.Errorf("parse demo file: %w", err)
	}
	if err := validateSyntheticDemoScenario(&scenario); err != nil {
		return nil, err
	}
	return &scenario, nil
}

func recordSyntheticDemoFailure(store *persistence.Store, defaults *syntheticDemoDefaults, cause error) {
	if store == nil || defaults == nil || cause == nil {
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"status": "failed",
		"error":  cause.Error(),
	})
	_ = store.AppendTaskEvent(&taskverification.TaskEvent{
		ID:                 identifiers.New(identifiers.KindTaskEvent),
		SchemaVersion:      1,
		CreatedAtUnixMilli: time.Now().UTC().UnixMilli(),
		TaskRunID:          defaults.RunID,
		SessionID:          defaults.SessionID,
		TemplateID:         defaults.TemplateID,
		PrincipalID:        defaults.PrincipalID,
		ClientName:         defaults.ClientName,
		ClientVersion:      defaults.ClientVersion,
		PhasePath:          taskverification.TaskPhase("demo.synthetic_replay"),
		ResultingPhasePath: taskverification.TaskPhase("demo.synthetic_replay"),
		EventType:          taskverification.TaskEventTypeFailed,
		Outcome:            taskverification.TaskEventOutcomeFailed,
		Payload:            payload,
	})
}
