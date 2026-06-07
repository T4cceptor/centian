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
	lastCtx   context.Context
}

type testRunStoreContextKey struct{}

func (s *captureRunStore) UpsertTaskRunSnapshot(ctx context.Context, snapshot *taskruns.PersistedRunSnapshot) error {
	s.lastCtx = ctx
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

// LoadTaskRunSnapshot returns the most recently captured snapshot for runID.
func (s *captureRunStore) LoadTaskRunSnapshot(_ context.Context, runID string) (*taskruns.PersistedRunSnapshot, error) {
	for i := len(s.snapshots) - 1; i >= 0; i-- {
		if s.snapshots[i].RunID == runID {
			return s.snapshots[i], nil
		}
	}
	//nolint:nilnil // A missing run is represented as an absent snapshot.
	return nil, nil
}

// FindOpenRunForPrincipal returns the most recent active/timed-out snapshot owned by principalID.
func (s *captureRunStore) FindOpenRunForPrincipal(_ context.Context, principalID string) (*taskruns.PersistedRunSnapshot, error) {
	if principalID == "" {
		//nolint:nilnil // An empty principal owns no attributable run.
		return nil, nil
	}
	for i := len(s.snapshots) - 1; i >= 0; i-- {
		snapshot := s.snapshots[i]
		if snapshot.OwnerPrincipalID != principalID {
			continue
		}
		if snapshot.Status == string(TaskStatusActive) || snapshot.Status == string(TaskStatusTimedOut) {
			return snapshot, nil
		}
	}
	//nolint:nilnil // No open run for this principal.
	return nil, nil
}

func TestRegisterTaskPassesContextToRunStore(t *testing.T) {
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

	key := testRunStoreContextKey{}
	ctx := context.WithValue(context.Background(), key, "request-scope")
	run, err := service.RegisterTaskWithDescription(ctx, "simple_tdd", "Resolve payment incident", "")
	assert.NilError(t, err)
	assert.Equal(t, run.TemplateID, "simple_tdd")
	assert.Equal(t, run.TaskDescription, "Resolve payment incident")
	assert.Equal(t, store.latest().TaskDescription, "Resolve payment incident")
	assert.Assert(t, store.lastCtx != nil)
	assert.Equal(t, store.lastCtx.Value(key), "request-scope")
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

	run, err := service.RegisterTask(context.Background(), "simple_tdd")
	assert.NilError(t, err)
	assert.Equal(t, len(store.snapshots), 1)
	assert.Equal(t, store.latest().TemplateName, "Simple TDD Task")
	assert.Equal(t, store.latest().SelectedTemplate.Task.Name, "Simple TDD Task")
	assert.Assert(t, store.latest().RunnableTemplate == nil)

	err = service.CompleteOnboarding(context.Background(), run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)
	assert.Equal(t, store.latest().Onboarding.TaskSummary, "ready")

	err = service.CompletePlanning(context.Background(), run, &PlanningArtifact{PlanSummary: "freeze"})
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

	run, err := service.RegisterTask(context.Background(), "simple_tdd")
	assert.NilError(t, err)

	err = service.CompleteOnboarding(context.Background(), run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)
	err = service.CompletePlanning(context.Background(), run, &PlanningArtifact{PlanSummary: "freeze"})
	assert.NilError(t, err)

	err = service.TimeoutTask(context.Background(), run)
	assert.NilError(t, err)
	assert.Equal(t, store.latest().Status, string(TaskStatusTimedOut))

	err = service.ResumeTask(context.Background(), run)
	assert.NilError(t, err)
	assert.Equal(t, store.latest().Status, string(TaskStatusActive))

	err = service.FailTask(context.Background(), run, "stuck")
	assert.NilError(t, err)
	assert.Equal(t, store.latest().Status, string(TaskStatusFailed))
	assert.Equal(t, store.latest().ExplicitFailReason, "stuck")

	err = service.RestartTask(context.Background(), run)
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
	assert.Equal(t, store.latest().Steps[0].Status, string(StepStatusPending))
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
