package taskverification

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/assert"
)

func TestStartAndCompleteStepHappyPath(t *testing.T) {
	dir := t.TempDir()
	template := Template{
		Version: "0.1",
		Task: Task{
			ID:          "task",
			Name:        "Task",
			Description: "desc",
		},
		Steps: []Step{
			{
				ID: "step_one",
				Checks: []Check{
					{
						ID:      "check_one",
						Command: "printf 'ready'",
						PreConditions: []Condition{
							{Type: "stdout_contains", Value: "ready"},
						},
						PostConditions: []Condition{
							{Type: "stdout_contains", Value: "ready"},
						},
					},
				},
				Invariants: []Invariant{
					{ID: "stable", Command: "printf 'same'"},
				},
			},
		},
	}
	service := NewService(dir, dir)
	run := &RunState{
		TemplateID:        template.Task.ID,
		SelectedTemplate:  template,
		DraftParameters:   map[string]string{},
		Status:            TaskStatusActive,
		Phase:             TaskPhaseExecution,
		ExecutionReady:    true,
		ExecutionTemplate: &template,
		Steps: []StepState{
			{ID: "step_one", Status: StepStatusPending, InvariantBaselines: map[string]string{}},
		},
	}

	start, err := service.StartStep(run, 1)
	assert.NilError(t, err)
	assert.Assert(t, start.Passed)
	assert.Equal(t, run.Steps[0].Status, StepStatusActive)

	complete, err := service.CompleteStep(run, 1)
	assert.NilError(t, err)
	assert.Assert(t, complete.Passed)
	assert.Equal(t, run.Status, TaskStatusCompleted)
	assert.Equal(t, run.Phase, TaskPhaseExecution)
	assert.Equal(t, run.Steps[0].Status, StepStatusPassed)
}

func TestStartStepFailsPreconditions(t *testing.T) {
	service, run := newRuntimeTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
steps:
  - id: "step_one"
    checks:
      - id: "check_one"
        command: "printf 'ok'"
        pre_conditions:
          - type: stdout_contains
            value: "missing"
`)

	result, err := service.StartStep(run, 1)
	assert.NilError(t, err)
	assert.Assert(t, !result.Passed)
	assert.Equal(t, run.Steps[0].Status, StepStatusPending)
}

func TestStartStepRequiresExecutionPhase(t *testing.T) {
	service, run := newRuntimeShellTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
steps:
  - id: "step_one"
    checks:
      - id: "check_one"
        command: "printf 'ok'"
`)

	_, err := service.StartStep(run, 1)
	assert.ErrorContains(t, err, "step execution is only allowed in execution phase")
}

func TestCompleteStepRequiresExecutionPhase(t *testing.T) {
	service, run := newRuntimeShellTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
steps:
  - id: "step_one"
    checks:
      - id: "check_one"
        command: "printf 'ok'"
`)

	_, err := service.CompleteStep(run, 1)
	assert.ErrorContains(t, err, "step execution is only allowed in execution phase")
}

func TestStartStepFailsInPlanningAfterOnboardingCompletion(t *testing.T) {
	service, run := newRuntimeShellTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
steps:
  - id: "step_one"
    checks:
      - id: "check_one"
        command: "printf 'ok'"
`)

	err := service.StartOnboarding(run)
	assert.NilError(t, err)
	err = service.CompleteOnboarding(run, OnboardingArtifact{ProjectSummary: "ready to plan"})
	assert.NilError(t, err)

	_, err = service.StartStep(run, 1)
	assert.ErrorContains(t, err, "step execution is only allowed in execution phase")
}

func TestCompleteStepFailsInvariantMismatch(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.txt")
	err := os.WriteFile(stateFile, []byte("before"), 0o644)
	assert.NilError(t, err)

	template := Template{
		Version: "0.1",
		Task: Task{
			ID:          "task",
			Name:        "Task",
			Description: "desc",
		},
		Steps: []Step{
			{
				ID: "step_one",
				Checks: []Check{
					{
						ID:      "check_one",
						Command: "printf 'ok'",
						PreConditions: []Condition{
							{Type: "stdout_contains", Value: "ok"},
						},
						PostConditions: []Condition{
							{Type: "stdout_contains", Value: "ok"},
						},
					},
				},
				Invariants: []Invariant{
					{ID: "stable_file", Command: "cat state.txt"},
				},
			},
		},
	}
	service := NewService(dir, dir)
	run := &RunState{
		TemplateID:        template.Task.ID,
		SelectedTemplate:  template,
		DraftParameters:   map[string]string{},
		Status:            TaskStatusActive,
		Phase:             TaskPhaseExecution,
		ExecutionReady:    true,
		ExecutionTemplate: &template,
		Steps: []StepState{
			{ID: "step_one", Status: StepStatusPending, InvariantBaselines: map[string]string{}},
		},
	}

	_, err = service.StartStep(run, 1)
	assert.NilError(t, err)
	err = os.WriteFile(stateFile, []byte("after"), 0o644)
	assert.NilError(t, err)

	result, err := service.CompleteStep(run, 1)
	assert.NilError(t, err)
	assert.Assert(t, !result.Passed)
	assert.Equal(t, run.Steps[0].Status, StepStatusActive)
	assert.Assert(t, result.Message != "")
}

func TestStartStepImplicitlyCompletesPreviousStep(t *testing.T) {
	service, run := newRuntimeTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
steps:
  - id: "step_one"
    checks:
      - id: "check_one"
        command: "printf 'one'"
        pre_conditions:
          - type: stdout_contains
            value: "one"
        post_conditions:
          - type: stdout_contains
            value: "one"
  - id: "step_two"
    checks:
      - id: "check_two"
        command: "printf 'two'"
        pre_conditions:
          - type: stdout_contains
            value: "two"
        post_conditions:
          - type: stdout_contains
            value: "two"
`)

	_, err := service.StartStep(run, 1)
	assert.NilError(t, err)

	result, err := service.StartStep(run, 2)
	assert.NilError(t, err)
	assert.Assert(t, result.Passed)
	assert.Equal(t, run.Steps[0].Status, StepStatusPassed)
	assert.Equal(t, run.Steps[1].Status, StepStatusActive)
}

func TestRestartAndFailTask(t *testing.T) {
	service, run := newRuntimeTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
steps:
  - id: "step_one"
    checks:
      - id: "check_one"
        command: "printf 'ok'"
`)

	err := service.FailTask(run, "stuck")
	assert.NilError(t, err)
	assert.Equal(t, run.Status, TaskStatusFailed)

	err = service.StartOnboarding(run)
	assert.ErrorContains(t, err, "task is failed")

	err = service.RestartTask(run)
	assert.NilError(t, err)
	assert.Equal(t, run.Status, TaskStatusActive)
	assert.Equal(t, run.Phase, TaskPhaseRegistered)
	assert.Assert(t, !run.ExecutionReady)
	assert.Assert(t, run.ExecutionTemplate == nil)
	assert.Equal(t, len(run.Steps), 0)
}

func newRuntimeTestService(t *testing.T, content string) (*Service, *RunState) {
	t.Helper()

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "task.yaml"), []byte(content), 0o644)
	assert.NilError(t, err)

	service := NewService(dir, dir)
	run, err := service.RegisterTask("task", map[string]string{})
	assert.NilError(t, err)
	err = service.PrepareExecution(run)
	assert.NilError(t, err)
	return service, run
}

func newRuntimeShellTestService(t *testing.T, content string) (*Service, *RunState) {
	t.Helper()

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "task.yaml"), []byte(content), 0o644)
	assert.NilError(t, err)

	service := NewService(dir, dir)
	run, err := service.RegisterTask("task", map[string]string{})
	assert.NilError(t, err)
	return service, run
}
