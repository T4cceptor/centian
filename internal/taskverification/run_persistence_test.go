package taskverification

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/T4cceptor/centian/internal/taskruns"
	"gotest.tools/assert"
)

type captureRunStore struct {
	snapshots []*taskruns.PersistedRunSnapshot
}

func (s *captureRunStore) UpsertTaskRunSnapshot(snapshot *taskruns.PersistedRunSnapshot) error {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	var cloned taskruns.PersistedRunSnapshot
	if err := json.Unmarshal(payload, &cloned); err != nil {
		return err
	}
	s.snapshots = append(s.snapshots, &cloned)
	return nil
}

func (s *captureRunStore) latest() *taskruns.PersistedRunSnapshot {
	if len(s.snapshots) == 0 {
		return nil
	}
	return s.snapshots[len(s.snapshots)-1]
}

func TestRegisterAndPlanningPersistTaskRunSnapshots(t *testing.T) {
	service := newTemplateTestService(t, `
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
          command: "printf stable"
`, "simple_tdd.yaml")
	store := &captureRunStore{}
	service.RunStore = store

	run, err := service.RegisterTask("simple_tdd")
	assert.NilError(t, err)
	assert.Equal(t, len(store.snapshots), 1)
	assert.Equal(t, store.latest().TemplateName, "Simple TDD Task")
	assert.Equal(t, store.latest().SelectedTemplate.Task.Name, "Simple TDD Task")
	assert.Assert(t, store.latest().RunnableTemplate == nil)

	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)
	assert.Equal(t, store.latest().Onboarding.TaskSummary, "ready")

	err = service.CompletePlanning(run, &PlanningArtifact{PlanSummary: "freeze"})
	assert.NilError(t, err)
	assert.Equal(t, store.latest().Planning.PlanSummary, "freeze")
	assert.Assert(t, store.latest().RunnableTemplate != nil)
	assert.Equal(t, store.latest().RunnableTemplate.Task.Name, "Simple TDD Task")
	assert.Equal(t, len(store.latest().Steps), 1)
	assert.Equal(t, store.latest().Steps[0].Status, string(StepStatusPending))
}

func TestTaskLifecycleMutationsPersistSnapshots(t *testing.T) {
	service := newTemplateTestService(t, `
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
`, "simple_tdd.yaml")
	store := &captureRunStore{}
	service.RunStore = store

	run, err := service.RegisterTask("simple_tdd")
	assert.NilError(t, err)

	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)
	err = service.CompletePlanning(run, &PlanningArtifact{PlanSummary: "freeze"})
	assert.NilError(t, err)

	err = service.TimeoutTask(run)
	assert.NilError(t, err)
	assert.Equal(t, store.latest().Status, string(TaskStatusTimedOut))

	err = service.ResumeTask(run)
	assert.NilError(t, err)
	assert.Equal(t, store.latest().Status, string(TaskStatusActive))

	err = service.FailTask(run, "stuck")
	assert.NilError(t, err)
	assert.Equal(t, store.latest().Status, string(TaskStatusFailed))
	assert.Equal(t, store.latest().ExplicitFailReason, "stuck")

	err = service.RestartTask(run)
	assert.NilError(t, err)
	assert.Equal(t, store.latest().Status, string(TaskStatusActive))
	assert.Assert(t, store.latest().Planning == nil)
	assert.Assert(t, store.latest().RunnableTemplate == nil)
	assert.Equal(t, len(store.latest().Steps), 0)
}

func TestStepStartAndCompletionPersistSnapshots(t *testing.T) {
	dir := t.TempDir()
	template := mustCompileRuntimeTemplate(t, &Template{
		Version: "0.1",
		Task:    Task{ID: "task", Name: "Task", Description: "desc"},
		Workflow: &Workflow{
			Onboarding: &LifecycleNodeSpec{},
			Planning:   &PlanningNodeSpec{},
			Execution: []ExecutionNodeSpec{{
				ID: "step_one",
				Checks: []Check{{
					ID:             "check_one",
					Command:        "printf 'ready'",
					PreConditions:  []Condition{{Type: "stdout_contains", Value: "ready"}},
					PostConditions: []Condition{{Type: "stdout_contains", Value: "ready"}},
				}},
				Invariants: []Invariant{{ID: "stable", Command: "printf 'same'"}},
			}},
		},
	})
	service := NewService(dir, dir)
	store := &captureRunStore{}
	service.RunStore = store
	run := newWorkflowReadyRun(&template)

	start, err := service.StartStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, start.Passed)
	assert.Equal(t, store.latest().Steps[0].Status, string(StepStatusActive))
	assert.Equal(t, store.latest().Steps[0].InvariantBaselines["stable"], "same")

	complete, err := service.CompleteStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, complete.Passed)
	assert.Equal(t, store.latest().Status, string(TaskStatusCompleted))
	assert.Equal(t, store.latest().Steps[0].Status, string(StepStatusPassed))
}

func TestStepFailurePersistsSnapshot(t *testing.T) {
	dir := t.TempDir()
	template := mustCompileRuntimeTemplate(t, &Template{
		Version: "0.1",
		Task:    Task{ID: "task", Name: "Task", Description: "desc"},
		Workflow: &Workflow{
			Onboarding: &LifecycleNodeSpec{},
			Planning:   &PlanningNodeSpec{},
			Execution: []ExecutionNodeSpec{{
				ID: "step_one",
				Checks: []Check{{
					ID:            "check_one",
					Command:       "printf 'nope'",
					PreConditions: []Condition{{Type: "stdout_contains", Value: "ready"}},
				}},
			}},
		},
	})
	service := NewService(dir, dir)
	store := &captureRunStore{}
	service.RunStore = store
	run := newWorkflowReadyRun(&template)

	result, err := service.StartStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, !result.Passed)
	assert.Equal(t, store.latest().Steps[0].Status, string(StepStatusFailed))
	assert.Assert(t, store.latest().LastFailureMessage != "")
}

func TestStartingNextStepPersistsImplicitPreviousCompletion(t *testing.T) {
	dir := t.TempDir()
	template := mustCompileRuntimeTemplate(t, &Template{
		Version: "0.1",
		Task:    Task{ID: "task", Name: "Task", Description: "desc"},
		Workflow: &Workflow{
			Onboarding: &LifecycleNodeSpec{},
			Planning:   &PlanningNodeSpec{},
			Execution: []ExecutionNodeSpec{
				{
					ID: "step_one",
					Checks: []Check{{
						ID:             "check_one",
						Command:        "printf 'ready'",
						PreConditions:  []Condition{{Type: "stdout_contains", Value: "ready"}},
						PostConditions: []Condition{{Type: "stdout_contains", Value: "ready"}},
					}},
				},
				{
					ID: "step_two",
					Checks: []Check{{
						ID:            "check_two",
						Command:       "printf 'steady'",
						PreConditions: []Condition{{Type: "stdout_contains", Value: "steady"}},
					}},
				},
			},
		},
	})
	service := NewService(dir, dir)
	store := &captureRunStore{}
	service.RunStore = store
	run := newWorkflowReadyRun(&template)

	_, err := service.StartStep(context.Background(), run, 1)
	assert.NilError(t, err)

	result, err := service.StartStep(context.Background(), run, 2)
	assert.NilError(t, err)
	assert.Assert(t, result.Passed)
	assert.Equal(t, store.latest().Steps[0].Status, string(StepStatusPassed))
	assert.Equal(t, store.latest().Steps[1].Status, string(StepStatusActive))
}
