package taskverification

import (
	"context"
	"errors"
	"testing"

	"gotest.tools/assert"
)

const restoreTestTemplate = `
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD Task"
  description: "Test driven task"
workflow:
  onboarding: {}
  planning: {}
  execution:
    - id: "step_one"
      checks:
        - id: "check_one"
          command: "printf ready"
          pre_conditions:
            - type: stdout_contains
              value: "ready"
          post_conditions:
            - type: stdout_contains
              value: "ready"
      invariants:
        - id: "stable"
          command: "printf same"
`

// restoreTestOwner is the principal that owns runs created via advanceToActiveStep.
const restoreTestOwner = "pr_owner"

// advanceToActiveStep registers a run owned by restoreTestOwner and advances it
// through onboarding, planning, and the start of step one (capturing invariant baselines).
func advanceToActiveStep(t *testing.T, service *Service) *RunState {
	t.Helper()
	ctx := context.Background()
	run, err := service.RegisterTaskWithDescription(ctx, "simple_tdd", "Fix the bug", restoreTestOwner)
	assert.NilError(t, err)
	assert.NilError(t, service.CompleteOnboarding(ctx, run, &OnboardingArtifact{TaskSummary: "ready"}))
	assert.NilError(t, service.CompletePlanning(ctx, run, &PlanningArtifact{PlanSummary: "freeze the plan before execution"}))
	start, err := service.StartStep(ctx, run, 1)
	assert.NilError(t, err)
	assert.Assert(t, start.Passed)
	return run
}

func TestRestoreRunStateRoundTrip(t *testing.T) {
	// Given: a run advanced into an active step, snapshotted to a capture store
	service := newTemplateTestService(t, restoreTestTemplate, "simple_tdd.yaml")
	store := &captureRunStore{}
	service.RunStore = store
	run := advanceToActiveStep(t, service)
	snapshot := store.latest()
	assert.Assert(t, snapshot != nil)

	// When: restoring the persisted snapshot into a fresh RunState
	restored, err := RestoreRunState(snapshot)
	assert.NilError(t, err)

	// Then: identity, phase, ownership, templates, planning and step state survive
	assert.Equal(t, restored.RunID, run.RunID)
	assert.Equal(t, restored.OwnerPrincipalID, "pr_owner")
	assert.Equal(t, restored.Status, TaskStatusActive)
	assert.Equal(t, restored.Phase, run.Phase)
	assert.Equal(t, restored.TaskDescription, "Fix the bug")
	assert.Assert(t, restored.Planning != nil)
	assert.Equal(t, restored.Planning.PlanSummary, "freeze the plan before execution")

	// The compiled workflow graph is rebuilt for both templates.
	assert.Assert(t, restored.SelectedTemplate.CompiledWorkflow != nil)
	assert.Assert(t, restored.RunnableTemplate != nil)
	assert.Assert(t, restored.RunnableTemplate.CompiledWorkflow != nil)
	assert.Equal(t, len(restored.RunnableTemplate.CompiledWorkflow.WorkflowSteps), 1)

	// Step runtime state, including the captured invariant baseline, survives.
	assert.Equal(t, len(restored.Steps), 1)
	assert.Equal(t, restored.Steps[0].Status, StepStatusActive)
	assert.Equal(t, restored.Steps[0].InvariantBaselines["stable"], "same")
}

func TestRestoreTaskRunContinuesFromStore(t *testing.T) {
	// Given: a run advanced into an active step and persisted to a shared store
	store := &captureRunStore{}
	service := newTemplateTestService(t, restoreTestTemplate, "simple_tdd.yaml")
	service.RunStore = store
	run := advanceToActiveStep(t, service)

	// When: a fresh service sharing the same store restores the run by id
	fresh := NewService(service.TemplateDir, service.WorkingDir)
	fresh.RunStore = store
	restored, err := fresh.RestoreTaskRun(context.Background(), run.RunID, "pr_owner")
	assert.NilError(t, err)

	// Then: execution continues and completion persists
	complete, err := fresh.CompleteStep(context.Background(), restored, 1)
	assert.NilError(t, err)
	assert.Assert(t, complete.Passed)
	assert.Equal(t, store.latest().Status, string(TaskStatusCompleted))
}

func TestRestoreTaskRunReactivatesTimedOutRun(t *testing.T) {
	// Given: a timed-out run persisted to the store
	service := newTemplateTestService(t, restoreTestTemplate, "simple_tdd.yaml")
	store := &captureRunStore{}
	service.RunStore = store
	run := advanceToActiveStep(t, service)
	assert.NilError(t, service.TimeoutTask(context.Background(), run))
	assert.Equal(t, store.latest().Status, string(TaskStatusTimedOut))

	// When: restoring the timed-out run
	restored, err := service.RestoreTaskRun(context.Background(), run.RunID, "pr_owner")

	// Then: it becomes active again without losing workflow progress
	assert.NilError(t, err)
	assert.Equal(t, restored.Status, TaskStatusActive)
	assert.Equal(t, restored.Steps[0].Status, StepStatusActive)
	assert.Equal(t, restored.Steps[0].InvariantBaselines["stable"], "same")
}

func TestRestoreTaskRunNegativeCases(t *testing.T) {
	service := newTemplateTestService(t, restoreTestTemplate, "simple_tdd.yaml")
	store := &captureRunStore{}
	service.RunStore = store
	ctx := context.Background()

	// Missing runId is rejected.
	_, err := service.RestoreTaskRun(ctx, "  ", "pr_owner")
	assert.ErrorContains(t, err, "runId is required")

	// Unknown run is rejected.
	_, err = service.RestoreTaskRun(ctx, "tr_missing", "pr_owner")
	assert.ErrorContains(t, err, "not found")

	// A completed run cannot be resumed.
	run := advanceToActiveStep(t, service)
	_, err = service.CompleteStep(ctx, run, 1)
	assert.NilError(t, err)
	assert.Equal(t, store.latest().Status, string(TaskStatusCompleted))
	_, err = service.RestoreTaskRun(ctx, run.RunID, "pr_owner")
	assert.ErrorContains(t, err, "cannot be resumed")
}

func TestRestoreTaskRunOwnershipGuard(t *testing.T) {
	service := newTemplateTestService(t, restoreTestTemplate, "simple_tdd.yaml")
	store := &captureRunStore{}
	service.RunStore = store
	run := advanceToActiveStep(t, service)

	// A different principal cannot restore another principal's run.
	_, err := service.RestoreTaskRun(context.Background(), run.RunID, "pr_intruder")
	assert.ErrorContains(t, err, "owned by another principal")

	// With auth disabled (no requester identity) the guard is skipped.
	restored, err := service.RestoreTaskRun(context.Background(), run.RunID, "")
	assert.NilError(t, err)
	assert.Equal(t, restored.RunID, run.RunID)
}

func TestRegisterRejectsSecondOpenRunForPrincipal(t *testing.T) {
	service := newTemplateTestService(t, restoreTestTemplate, "simple_tdd.yaml")
	store := &captureRunStore{}
	service.RunStore = store
	ctx := context.Background()

	// Given: principal A already has an open run
	first, err := service.RegisterTaskWithDescription(ctx, "simple_tdd", "first", "pr_a")
	assert.NilError(t, err)

	// When: principal A registers again
	_, err = service.RegisterTaskWithDescription(ctx, "simple_tdd", "second", "pr_a")

	// Then: it is rejected with the existing run id surfaced
	var openErr *OpenTaskExistsError
	assert.Assert(t, errors.As(err, &openErr))
	assert.Equal(t, openErr.RunID, first.RunID)
	assert.Equal(t, openErr.Status, TaskStatusActive)

	// A different principal is unaffected.
	_, err = service.RegisterTaskWithDescription(ctx, "simple_tdd", "other", "pr_b")
	assert.NilError(t, err)

	// An empty principal is unaffected (in-session guard applies instead).
	_, err = service.RegisterTaskWithDescription(ctx, "simple_tdd", "anon", "")
	assert.NilError(t, err)
}

func TestRegisterAllowsNewRunAfterPriorRunFailed(t *testing.T) {
	service := newTemplateTestService(t, restoreTestTemplate, "simple_tdd.yaml")
	store := &captureRunStore{}
	service.RunStore = store
	ctx := context.Background()

	first, err := service.RegisterTaskWithDescription(ctx, "simple_tdd", "first", "pr_a")
	assert.NilError(t, err)
	assert.NilError(t, service.FailTask(ctx, first, "abandoned"))

	// Once the prior run is failed it no longer blocks a fresh registration.
	second, err := service.RegisterTaskWithDescription(ctx, "simple_tdd", "second", "pr_a")
	assert.NilError(t, err)
	assert.Assert(t, second.RunID != first.RunID)
}
