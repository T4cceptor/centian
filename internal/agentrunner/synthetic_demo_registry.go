package agentrunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/T4cceptor/centian/internal/identifiers"
	"github.com/T4cceptor/centian/internal/persistence"
)

const itOpsSyntheticDemoAsset = "it_ops_synthetic_demo.json"

// ErrSyntheticDemoNotFound indicates that a requested bundled demo is unknown.
var ErrSyntheticDemoNotFound = errors.New("synthetic demo not found")

// SyntheticDemoDefinition describes one bundled synthetic demo.
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

// SyntheticDemoRunOptions customizes a seeded synthetic demo run.
type SyntheticDemoRunOptions struct {
	RunID string
}

var syntheticDemoDefinitions = []SyntheticDemoDefinition{
	{
		ID:          "it_ops",
		Name:        "IT Ops Incident",
		Description: "Replay a governed platform incident with prompt-injection redaction, tool allowlist denial, precondition failure, and frozen verification.",
		DurationMS:  72_000,
	},
}

// ListSyntheticDemos returns the synthetic demos bundled with Centian.
func ListSyntheticDemos() []SyntheticDemoDefinition {
	result := make([]SyntheticDemoDefinition, len(syntheticDemoDefinitions))
	copy(result, syntheticDemoDefinitions)
	return result
}

// StartSyntheticDemoRun seeds one bundled synthetic demo against the provided store.
func StartSyntheticDemoRun(ctx context.Context, store *persistence.Store, demoID string) (*SyntheticDemoRun, error) {
	return StartSyntheticDemoRunWithOptions(ctx, store, demoID, SyntheticDemoRunOptions{})
}

// StartSyntheticDemoRunWithOptions seeds one bundled synthetic demo against the
// provided store using explicit replay options.
func StartSyntheticDemoRunWithOptions(ctx context.Context, store *persistence.Store, demoID string, options SyntheticDemoRunOptions) (*SyntheticDemoRun, error) {
	if store == nil {
		return nil, fmt.Errorf("event store is required")
	}
	definition, scenario, err := loadRegisteredSyntheticDemo(demoID)
	if err != nil {
		return nil, err
	}

	replayer := newSyntheticDemoReplayer()
	state := replayer.newReplayState(scenario)
	if options.RunID != "" {
		if !identifiers.IsKind(options.RunID, identifiers.KindTaskRun) {
			return nil, fmt.Errorf("invalid synthetic demo run id %q", options.RunID)
		}
		state.defaults.RunID = options.RunID
	}
	run := &SyntheticDemoRun{
		RunID:      state.defaults.RunID,
		DemoID:     definition.ID,
		DurationMS: scenario.DurationMS,
	}

	if err := replayer.replayToStoreFromState(ctx, store, scenario, state); err != nil {
		return nil, fmt.Errorf("seed demo run: %w", err)
	}

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
	scenario, _, err := loadEmbeddedSyntheticDemoScenarioWithBytes(assetName)
	return scenario, err
}

func loadEmbeddedSyntheticDemoScenarioWithBytes(assetName string) (*syntheticDemoScenario, []byte, error) {
	content, err := asset(assetName)
	if err != nil {
		return nil, nil, err
	}
	var scenario syntheticDemoScenario
	if err := json.Unmarshal([]byte(content), &scenario); err != nil {
		return nil, nil, fmt.Errorf("parse demo file: %w", err)
	}
	if err := validateSyntheticDemoScenario(&scenario); err != nil {
		return nil, nil, err
	}
	return &scenario, []byte(content), nil
}
