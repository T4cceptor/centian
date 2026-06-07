package taskverification

import (
	"context"
	"fmt"
	"strings"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/taskruns"
)

// OpenTaskExistsError reports that a principal already has an open (active or
// timed-out) task run and therefore cannot register a new one until the existing
// run is resumed or explicitly failed.
type OpenTaskExistsError struct {
	RunID      string
	TemplateID string
	Status     TaskStatus
}

// Error implements the error interface.
func (e *OpenTaskExistsError) Error() string {
	return fmt.Sprintf("principal already has an open task run %q (%s)", e.RunID, e.Status)
}

// RestoreRunState hydrates a persisted snapshot back into an executable RunState.
//
// The frozen templates are rebuilt from their persisted authored form and then
// recompiled through the normal compilation path, so the in-memory compiled
// workflow graph is reconstructed by the same logic that produced it originally
// rather than by reversing the persisted compiled snapshot.
func RestoreRunState(snapshot *taskruns.PersistedRunSnapshot) (*RunState, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("task run snapshot is required")
	}

	selected, err := restoreTemplate(&snapshot.SelectedTemplate)
	if err != nil {
		return nil, fmt.Errorf("restore selected template: %w", err)
	}
	if selected == nil {
		return nil, fmt.Errorf("restored task run is missing its selected template")
	}

	var runnable *Template
	if snapshot.RunnableTemplate != nil {
		runnable, err = restoreTemplate(snapshot.RunnableTemplate)
		if err != nil {
			return nil, fmt.Errorf("restore runnable template: %w", err)
		}
	}

	onboarding, err := common.ConvertViaJSON[taskruns.PersistedOnboardingArtifact, OnboardingArtifact](snapshot.Onboarding)
	if err != nil {
		return nil, err
	}
	planning, err := common.ConvertViaJSON[taskruns.PersistedPlanningArtifact, PlanningArtifact](snapshot.Planning)
	if err != nil {
		return nil, err
	}

	return &RunState{
		RunID:              snapshot.RunID,
		OwnerPrincipalID:   snapshot.OwnerPrincipalID,
		TemplateID:         snapshot.TemplateID,
		TaskDescription:    snapshot.TaskDescription,
		SelectedTemplate:   *selected,
		Status:             TaskStatus(snapshot.Status),
		Phase:              TaskPhase(snapshot.Phase),
		Onboarding:         onboarding,
		Planning:           planning,
		WorkflowReady:      snapshot.WorkflowReady,
		RunnableTemplate:   runnable,
		Steps:              restoreStepStates(snapshot.Steps),
		LastFailureMessage: snapshot.LastFailureMessage,
		ExplicitFailReason: snapshot.ExplicitFailReason,
		LastActivityAt:     snapshot.LastActivityAtUnixMilli,
		ExpiresAt:          snapshot.ExpiresAtUnixMilli,
	}, nil
}

func restoreTemplate(snapshot *taskruns.PersistedTemplateSnapshot) (*Template, error) {
	if snapshot == nil {
		//nolint:nilnil // A missing runnable template is represented as an absent template.
		return nil, nil
	}
	template, err := common.ConvertViaJSON[taskruns.PersistedTemplateSnapshot, Template](snapshot)
	if err != nil {
		return nil, err
	}
	// Recompile the workflow graph from the restored authored form so the runtime
	// uses the same compiled representation it would have built originally.
	if err := template.validate(false); err != nil {
		return nil, fmt.Errorf("recompile restored template workflow: %w", err)
	}
	return template, nil
}

func restoreStepStates(steps []taskruns.PersistedStepStateSnapshot) []StepState {
	if len(steps) == 0 {
		return nil
	}
	result := make([]StepState, 0, len(steps))
	for idx := range steps {
		step := steps[idx]
		baselines := make(map[string]string, len(step.InvariantBaselines))
		for key, value := range step.InvariantBaselines {
			baselines[key] = value
		}
		result = append(result, StepState{
			ID:                 step.ID,
			Path:               TaskPhase(step.Path),
			Status:             StepStatus(step.Status),
			InvariantBaselines: baselines,
		})
	}
	return result
}

// LoadOwnedRun loads and hydrates a persisted run by id, verifying that the
// requesting principal owns it. It applies no status gate, so callers may use it
// to act on terminal or open runs alike (e.g. failing a stale run).
func (s *Service) LoadOwnedRun(ctx context.Context, runID, requesterPrincipalID string) (*RunState, error) {
	if s == nil || s.RunStore == nil {
		return nil, fmt.Errorf("task run persistence is not configured")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("runId is required")
	}
	snapshot, err := s.RunStore.LoadTaskRunSnapshot(ctx, runID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, fmt.Errorf("task run %q not found", runID)
	}
	run, err := RestoreRunState(snapshot)
	if err != nil {
		return nil, err
	}
	if err := ensurePrincipalOwnsRun(run, requesterPrincipalID); err != nil {
		return nil, err
	}
	return run, nil
}

// RestoreTaskRun loads a persisted run by id and prepares it to continue in a new
// session. Only active or timed-out runs may be restored; a timed-out run is
// reactivated to active. Ownership is enforced when a requester principal is
// supplied and the run records an owner.
func (s *Service) RestoreTaskRun(ctx context.Context, runID, requesterPrincipalID string) (*RunState, error) {
	run, err := s.LoadOwnedRun(ctx, runID, requesterPrincipalID)
	if err != nil {
		return nil, err
	}

	switch run.Status {
	case TaskStatusActive:
		// Already active; continue from the persisted phase.
		return run, nil
	case TaskStatusTimedOut:
		run.Status = TaskStatusActive
		run.LastFailureMessage = ""
		run.ExpiresAt = 0
		if err := s.persistRunSnapshot(ctx, run); err != nil {
			return nil, err
		}
		return run, nil
	default:
		return nil, fmt.Errorf("task %q is %s and cannot be resumed", run.RunID, run.Status)
	}
}

func ensurePrincipalOwnsRun(run *RunState, requesterPrincipalID string) error {
	requester := strings.TrimSpace(requesterPrincipalID)
	// Skip the check when auth is disabled (no requester identity) or the run is
	// a legacy/unowned snapshot that predates ownership tracking.
	if requester == "" || run.OwnerPrincipalID == "" {
		return nil
	}
	if run.OwnerPrincipalID != requester {
		return fmt.Errorf("task run %q is owned by another principal", run.RunID)
	}
	return nil
}
