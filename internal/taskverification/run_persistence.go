package taskverification

import (
	"context"
	"fmt"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/taskruns"
)

// RunStore persists the latest snapshot for one task run.
type RunStore interface {
	UpsertTaskRunSnapshot(context.Context, *taskruns.PersistedRunSnapshot) error
}

type noopRunStore struct{}

// UpsertTaskRunSnapshot satisfies RunStore for non-persistent service setups.
func (noopRunStore) UpsertTaskRunSnapshot(context.Context, *taskruns.PersistedRunSnapshot) error {
	return nil
}

func (s *Service) persistRunSnapshot(ctx context.Context, run *RunState) error {
	if s == nil || s.RunStore == nil || run == nil {
		return nil
	}
	snapshot, err := snapshotRunState(run)
	if err != nil {
		return err
	}
	return s.RunStore.UpsertTaskRunSnapshot(ctx, snapshot)
}

func snapshotRunState(run *RunState) (*taskruns.PersistedRunSnapshot, error) {
	if run == nil {
		return nil, fmt.Errorf("task run is required")
	}

	selectedTemplate, err := snapshotTemplate(&run.SelectedTemplate)
	if err != nil {
		return nil, err
	}

	var runnableTemplate *taskruns.PersistedTemplateSnapshot
	if run.RunnableTemplate != nil {
		snapshot, err := snapshotTemplate(run.RunnableTemplate)
		if err != nil {
			return nil, err
		}
		runnableTemplate = snapshot
	}

	onboarding, err := common.ConvertViaJSON[OnboardingArtifact, taskruns.PersistedOnboardingArtifact](run.Onboarding)
	if err != nil {
		return nil, err
	}
	planning, err := common.ConvertViaJSON[PlanningArtifact, taskruns.PersistedPlanningArtifact](run.Planning)
	if err != nil {
		return nil, err
	}

	return &taskruns.PersistedRunSnapshot{
		RunID:                   run.RunID,
		TemplateID:              run.TemplateID,
		TemplateName:            run.SelectedTemplate.Task.Name,
		Status:                  string(run.Status),
		Phase:                   string(run.Phase),
		WorkflowReady:           run.WorkflowReady,
		LastFailureMessage:      run.LastFailureMessage,
		ExplicitFailReason:      run.ExplicitFailReason,
		LastActivityAtUnixMilli: run.LastActivityAt,
		ExpiresAtUnixMilli:      run.ExpiresAt,
		Onboarding:              onboarding,
		Planning:                planning,
		SelectedTemplate:        *selectedTemplate,
		RunnableTemplate:        runnableTemplate,
		Steps:                   snapshotStepStates(run.Steps),
	}, nil
}

func snapshotTemplate(template *Template) (*taskruns.PersistedTemplateSnapshot, error) {
	if template == nil {
		//nolint:nilnil // A missing runnable template is represented as an omitted snapshot.
		return nil, nil
	}
	snapshot, err := common.ConvertViaJSON[Template, taskruns.PersistedTemplateSnapshot](template)
	if err != nil {
		return nil, err
	}
	snapshot.CompiledWorkflow = snapshotCompiledWorkflow(template.CompiledWorkflow)
	return snapshot, nil
}

//nolint:gocritic // Snapshotting copies the compiled workflow graph into the persisted form in one pass.
func snapshotCompiledWorkflow(workflow *CompiledWorkflow) *taskruns.PersistedCompiledWorkflowSnapshot {
	if workflow == nil {
		return nil
	}

	nodes := make(map[string]taskruns.PersistedWorkflowNodeSnapshot, len(workflow.Nodes))
	for path, node := range workflow.Nodes {
		nodes[string(path)] = taskruns.PersistedWorkflowNodeSnapshot{
			Path:                   string(node.Path),
			Kind:                   string(node.Kind),
			ParentPath:             string(node.ParentPath),
			NextPath:               string(node.NextPath),
			StepNumber:             node.StepNumber,
			StepID:                 node.StepID,
			Name:                   node.Name,
			Description:            node.Description,
			Instructions:           node.Instructions,
			AllowedTools:           append([]string(nil), node.AllowedTools...),
			Checkpoint:             snapshotCheckpoint(node.Checkpoint),
			EditableFields:         append([]string(nil), node.EditableFields...),
			RequiredPlanningInputs: append([]string(nil), node.RequiredPlanningInputs...),
		}
	}

	steps := make([]taskruns.PersistedCompiledStepSnapshot, 0, len(workflow.WorkflowSteps))
	for idx := range workflow.WorkflowSteps {
		step := workflow.WorkflowSteps[idx]
		steps = append(steps, taskruns.PersistedCompiledStepSnapshot{
			ID:           step.ID,
			Path:         string(step.Path),
			ParentPath:   string(step.ParentPath),
			NextPath:     string(step.NextPath),
			Name:         step.Name,
			Description:  step.Description,
			Instructions: step.Instructions,
			AllowedTools: append([]string(nil), step.AllowedTools...),
			Checkpoint:   snapshotCheckpoint(step.Checkpoint),
			Checks:       snapshotChecks(step.Checks),
			Invariants:   snapshotInvariants(step.Invariants),
		})
	}

	return &taskruns.PersistedCompiledWorkflowSnapshot{
		Nodes:               nodes,
		OnboardingPath:      string(workflow.OnboardingPath),
		PlanningPath:        string(workflow.PlanningPath),
		FirstExecutablePath: string(workflow.FirstExecutablePath),
		WorkflowSteps:       steps,
	}
}

func snapshotCheckpoint(hint *CheckpointHint) *taskruns.PersistedCheckpointHint {
	if hint == nil {
		return nil
	}
	return &taskruns.PersistedCheckpointHint{Enabled: hint.Enabled}
}

func snapshotChecks(checks []Check) []taskruns.PersistedCheck {
	if len(checks) == 0 {
		return nil
	}
	result := make([]taskruns.PersistedCheck, 0, len(checks))
	for idx := range checks {
		check := checks[idx]
		result = append(result, taskruns.PersistedCheck{
			ID:             check.ID,
			Description:    check.Description,
			Command:        check.Command,
			PreConditions:  snapshotConditions(check.PreConditions),
			PostConditions: snapshotConditions(check.PostConditions),
		})
	}
	return result
}

func snapshotInvariants(invariants []Invariant) []taskruns.PersistedInvariant {
	if len(invariants) == 0 {
		return nil
	}
	result := make([]taskruns.PersistedInvariant, 0, len(invariants))
	for idx := range invariants {
		invariant := invariants[idx]
		result = append(result, taskruns.PersistedInvariant{
			ID:          invariant.ID,
			Description: invariant.Description,
			Command:     invariant.Command,
		})
	}
	return result
}

func snapshotConditions(conditions []Condition) []taskruns.PersistedCondition {
	if len(conditions) == 0 {
		return nil
	}
	result := make([]taskruns.PersistedCondition, 0, len(conditions))
	for idx := range conditions {
		condition := conditions[idx]
		values := append([]any(nil), condition.Values...)
		result = append(result, taskruns.PersistedCondition{
			Type:   condition.Type,
			Value:  condition.Value,
			Values: values,
			Path:   condition.Path,
		})
	}
	return result
}

func snapshotStepStates(steps []StepState) []taskruns.PersistedStepStateSnapshot {
	if len(steps) == 0 {
		return nil
	}
	result := make([]taskruns.PersistedStepStateSnapshot, 0, len(steps))
	for idx := range steps {
		step := steps[idx]
		baselines := make(map[string]string, len(step.InvariantBaselines))
		for key, value := range step.InvariantBaselines {
			baselines[key] = value
		}
		if len(baselines) == 0 {
			baselines = nil
		}
		result = append(result, taskruns.PersistedStepStateSnapshot{
			ID:                 step.ID,
			Path:               string(step.Path),
			Status:             string(step.Status),
			InvariantBaselines: baselines,
		})
	}
	return result
}
