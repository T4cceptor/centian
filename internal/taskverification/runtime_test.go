package taskverification

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gotest.tools/assert"
)

func TestStartAndCompleteStepHappyPath(t *testing.T) {
	dir := t.TempDir()
	template := mustCompileRuntimeTemplate(t, &Template{
		Version: "0.1",
		Task: Task{
			ID:          "task",
			Name:        "Task",
			Description: "desc",
		},
		Workflow: &Workflow{
			Onboarding: &LifecycleNodeSpec{},
			Planning:   &PlanningNodeSpec{},
			Execution: []ExecutionNodeSpec{
				{
					ID: "step_one",
					Checks: []Check{{
						ID:      "check_one",
						Command: "printf 'ready'",
						PreConditions: []Condition{
							{Type: "stdout_contains", Value: "ready"},
						},
						PostConditions: []Condition{
							{Type: "stdout_contains", Value: "ready"},
						},
					}},
					Invariants: []Invariant{
						{ID: "stable", Command: "printf 'same'"},
					},
				},
			},
		},
	})
	service := NewService(dir, dir)
	run := newWorkflowReadyRun(&template)

	start, err := service.StartStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, start.Passed)
	assert.Equal(t, start.FailureKind, StepFailureKind(""))
	assert.Equal(t, start.StdoutSnippet, "")
	assert.Equal(t, run.Steps[0].Status, StepStatusActive)

	complete, err := service.CompleteStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, complete.Passed)
	assert.Equal(t, complete.FailureKind, StepFailureKind(""))
	assert.Equal(t, complete.StdoutSnippet, "")
	assert.Equal(t, run.Status, TaskStatusCompleted)
	assert.Equal(t, run.Phase, TaskPhase("execution.step_one"))
	assert.Equal(t, run.Steps[0].Status, StepStatusPassed)
}

func TestStartAndCompleteStepWithoutChecksOrInvariants(t *testing.T) {
	dir := t.TempDir()
	template := mustCompileRuntimeTemplate(t, &Template{
		Version: "0.1",
		Task: Task{
			ID:          "task",
			Name:        "Task",
			Description: "desc",
		},
		Workflow: &Workflow{
			Onboarding: &LifecycleNodeSpec{},
			Planning:   &PlanningNodeSpec{},
			Execution: []ExecutionNodeSpec{
				{
					ID: "step_one",
				},
			},
		},
	})
	service := NewService(dir, dir)
	run := newWorkflowReadyRun(&template)

	start, err := service.StartStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, start.Passed)
	assert.Equal(t, run.Steps[0].Status, StepStatusActive)

	complete, err := service.CompleteStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, complete.Passed)
	assert.Equal(t, run.Status, TaskStatusCompleted)
	assert.Equal(t, run.Steps[0].Status, StepStatusPassed)
}

func TestStepWithoutChecksStillVerifiesInvariantDrift(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.txt")
	err := os.WriteFile(stateFile, []byte("before"), 0o644)
	assert.NilError(t, err)

	template := mustCompileRuntimeTemplate(t, &Template{
		Version: "0.1",
		Task: Task{
			ID:          "task",
			Name:        "Task",
			Description: "desc",
		},
		Workflow: &Workflow{
			Onboarding: &LifecycleNodeSpec{},
			Planning:   &PlanningNodeSpec{},
			Execution: []ExecutionNodeSpec{
				{
					ID: "step_one",
					Invariants: []Invariant{
						{ID: "stable_file", Command: "cat state.txt"},
					},
				},
			},
		},
	})
	service := NewService(dir, dir)
	run := newWorkflowReadyRun(&template)

	_, err = service.StartStep(context.Background(), run, 1)
	assert.NilError(t, err)
	err = os.WriteFile(stateFile, []byte("after"), 0o644)
	assert.NilError(t, err)

	result, err := service.CompleteStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, !result.Passed)
	assert.Equal(t, result.FailureKind, StepFailureKindInvariant)
	assert.Equal(t, result.FailurePhase, StepFailurePhaseInvariantVerify)
	assert.Equal(t, result.FailedInvariantID, "stable_file")
	assert.Assert(t, strings.Contains(result.StdoutSnippet, "after"))
	assert.Equal(t, run.Steps[0].Status, StepStatusActive)
}

func TestStartStepFailsPreconditions(t *testing.T) {
	service, run := newRuntimeTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "step_one"
      checks:
        - id: "check_one"
          command: "printf 'ok'"
          pre_conditions:
            - type: stdout_contains
              value: "missing"
`)

	result, err := service.StartStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, !result.Passed)
	assert.Equal(t, result.FailureKind, StepFailureKindCheck)
	assert.Equal(t, result.FailurePhase, StepFailurePhasePrecondition)
	assert.Equal(t, result.FailedCheckID, "check_one")
	assert.Equal(t, result.Summary, result.Message)
	assert.Assert(t, strings.Contains(result.StdoutSnippet, "ok"))
	assert.Assert(t, result.ExitCode != nil)
	assert.Equal(t, run.Steps[0].Status, StepStatusPending)
}

func TestScaffoldingStepTransitionsIntoExecution(t *testing.T) {
	service, run := newRuntimeTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  scaffolding:
    - id: "setup_test_file"
      checks:
        - id: "file_created"
          command: "printf 'scaffold'"
          pre_conditions:
            - type: file_not_exists
              path: "generated_test.py"
          post_conditions:
            - type: file_exists
              path: "generated_test.py"
  execution:
    - id: "implement_solution"
      checks:
        - id: "selected_test_passes"
          command: "printf 'ok'"
`)

	assert.Equal(t, run.Phase, TaskPhase("scaffolding.setup_test_file"))

	start, err := service.StartStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, start.Passed)
	assert.Equal(t, start.Phase, TaskPhase("scaffolding.setup_test_file"))

	err = os.WriteFile(filepath.Join(service.WorkingDir, "generated_test.py"), []byte("def test_ok(): pass\n"), 0o644)
	assert.NilError(t, err)

	complete, err := service.CompleteStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, complete.Passed)
	assert.Equal(t, run.Phase, TaskPhase("execution.implement_solution"))
}

func TestStartStepRequiresExecutionNode(t *testing.T) {
	service, run := newRuntimeShellTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "step_one"
      checks:
        - id: "check_one"
          command: "printf 'ok'"
`)

	_, err := service.StartStep(context.Background(), run, 1)
	assert.ErrorContains(t, err, "step execution is only allowed in scaffolding or execution nodes")
}

func TestCompleteStepRequiresExecutionNode(t *testing.T) {
	service, run := newRuntimeShellTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "step_one"
      checks:
        - id: "check_one"
          command: "printf 'ok'"
`)

	_, err := service.CompleteStep(context.Background(), run, 1)
	assert.ErrorContains(t, err, "step execution is only allowed in scaffolding or execution nodes")
}

func TestStartStepFailsInPlanningAfterOnboardingCompletion(t *testing.T) {
	service, run := newRuntimeShellTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "step_one"
      checks:
        - id: "check_one"
          command: "printf 'ok'"
`)

	err := service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready to plan"})
	assert.NilError(t, err)

	_, err = service.StartStep(context.Background(), run, 1)
	assert.ErrorContains(t, err, "step execution is only allowed in scaffolding or execution nodes")
}

func TestCompleteStepFailsInvariantMismatch(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.txt")
	err := os.WriteFile(stateFile, []byte("before"), 0o644)
	assert.NilError(t, err)

	template := mustCompileRuntimeTemplate(t, &Template{
		Version: "0.1",
		Task: Task{
			ID:          "task",
			Name:        "Task",
			Description: "desc",
		},
		Workflow: &Workflow{
			Onboarding: &LifecycleNodeSpec{},
			Planning:   &PlanningNodeSpec{},
			Execution: []ExecutionNodeSpec{
				{
					ID: "step_one",
					Checks: []Check{{
						ID:      "check_one",
						Command: "printf 'ok'",
						PreConditions: []Condition{
							{Type: "stdout_contains", Value: "ok"},
						},
						PostConditions: []Condition{
							{Type: "stdout_contains", Value: "ok"},
						},
					}},
					Invariants: []Invariant{
						{ID: "stable_file", Command: "cat state.txt"},
					},
				},
			},
		},
	})
	service := NewService(dir, dir)
	run := newWorkflowReadyRun(&template)

	_, err = service.StartStep(context.Background(), run, 1)
	assert.NilError(t, err)
	err = os.WriteFile(stateFile, []byte("after"), 0o644)
	assert.NilError(t, err)

	result, err := service.CompleteStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, !result.Passed)
	assert.Equal(t, result.FailureKind, StepFailureKindInvariant)
	assert.Equal(t, result.FailurePhase, StepFailurePhaseInvariantVerify)
	assert.Equal(t, result.FailedInvariantID, "stable_file")
	assert.Assert(t, strings.Contains(result.StdoutSnippet, "after"))
	assert.Equal(t, run.Steps[0].Status, StepStatusActive)
	assert.Assert(t, result.Message != "")
}

func TestCompleteStepFailsPostconditionsWithSnippets(t *testing.T) {
	service, run := newRuntimeTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "step_one"
      checks:
        - id: "check_one"
          command: "printf 'unexpected output for verification'"
          pre_conditions:
            - type: stdout_contains
              value: "unexpected"
          post_conditions:
            - type: stdout_contains
              value: "missing"
`)

	_, err := service.StartStep(context.Background(), run, 1)
	assert.NilError(t, err)

	result, err := service.CompleteStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, !result.Passed)
	assert.Equal(t, result.FailureKind, StepFailureKindCheck)
	assert.Equal(t, result.FailurePhase, StepFailurePhasePostcondition)
	assert.Equal(t, result.FailedCheckID, "check_one")
	assert.Assert(t, strings.Contains(result.StdoutSnippet, "unexpected output"))
}

func TestStartStepFailsInvariantCaptureWithInvariantMetadata(t *testing.T) {
	service, run := newRuntimeTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "step_one"
      checks:
        - id: "check_one"
          command: "printf 'ok'"
          pre_conditions:
            - type: stdout_contains
              value: "ok"
      invariants:
        - id: "stable"
          command: "sh -c 'printf boom >&2; exit 4'"
`)

	result, err := service.StartStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, !result.Passed)
	assert.Equal(t, result.FailureKind, StepFailureKindInvariant)
	assert.Equal(t, result.FailurePhase, StepFailurePhaseInvariantCapture)
	assert.Equal(t, result.FailedInvariantID, "stable")
	assert.Assert(t, result.ExitCode != nil)
	assert.Equal(t, *result.ExitCode, 4)
	assert.Assert(t, strings.Contains(result.StderrSnippet, "boom"))
	assert.Equal(t, run.Steps[0].Status, StepStatusFailed)
}

func TestStartStepReturnsCommandExecutionFailure(t *testing.T) {
	service, run := newRuntimeTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "step_one"
      checks:
        - id: "check_one"
          command: "printf 'ok'"
          pre_conditions:
            - type: stdout_contains
              value: "ok"
`)
	service.WorkingDir = filepath.Join(t.TempDir(), "missing")

	result, err := service.StartStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, !result.Passed)
	assert.Equal(t, result.FailureKind, StepFailureKindCommandExecution)
	assert.Equal(t, result.FailurePhase, StepFailurePhaseCommandExecution)
	assert.Equal(t, result.FailedCheckID, "check_one")
	assert.Equal(t, run.Steps[0].Status, StepStatusFailed)
}

func TestStartStepImplicitlyCompletesPreviousStepAndAdvancesPhase(t *testing.T) {
	service, run := newRuntimeTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
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

	_, err := service.StartStep(context.Background(), run, 1)
	assert.NilError(t, err)

	result, err := service.StartStep(context.Background(), run, 2)
	assert.NilError(t, err)
	assert.Assert(t, result.Passed)
	assert.Equal(t, run.Steps[0].Status, StepStatusPassed)
	assert.Equal(t, run.Steps[1].Status, StepStatusActive)
	assert.Equal(t, run.Phase, TaskPhase("execution.step_two"))
}

func TestRestartAndFailTask(t *testing.T) {
	service, run := newRuntimeTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "step_one"
      checks:
        - id: "check_one"
          command: "printf 'ok'"
`)

	err := service.FailTask(run, "stuck")
	assert.NilError(t, err)
	assert.Equal(t, run.Status, TaskStatusFailed)

	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.ErrorContains(t, err, "task is failed")

	err = service.RestartTask(run)
	assert.NilError(t, err)
	assert.Equal(t, run.Status, TaskStatusActive)
	assert.Equal(t, run.Phase, TaskPhaseOnboarding)
	assert.Assert(t, !run.WorkflowReady)
	assert.Assert(t, run.RunnableTemplate == nil)
	assert.Equal(t, len(run.Steps), 0)
}

func TestCompleteStepAdvancesIntoWaitingNodeWhenConfigured(t *testing.T) {
	service, run := newRuntimeTestService(t, `
version: "0.1"
task:
  id: "task"
  name: "Task"
  description: "desc"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "step_one"
      next: "waiting_for_approval.review"
      checks:
        - id: "check_one"
          command: "printf 'ok'"
          pre_conditions:
            - type: stdout_contains
              value: "ok"
          post_conditions:
            - type: stdout_contains
              value: "ok"
    - id: "review"
      kind: "waiting_for_approval"
      next: "execution.step_two"
    - id: "step_two"
      checks:
        - id: "check_two"
          command: "printf 'ok'"
`)

	_, err := service.StartStep(context.Background(), run, 1)
	assert.NilError(t, err)
	result, err := service.CompleteStep(context.Background(), run, 1)
	assert.NilError(t, err)
	assert.Assert(t, result.Passed)
	assert.Equal(t, run.Phase, TaskPhase("waiting_for_approval.review"))

	_, err = service.StartStep(context.Background(), run, 2)
	assert.ErrorContains(t, err, "step execution is only allowed in scaffolding or execution nodes")
}

func newRuntimeTestService(t *testing.T, content string) (*Service, *RunState) {
	t.Helper()

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "task.yaml"), []byte(content), 0o644)
	assert.NilError(t, err)

	service := NewService(dir, dir)
	run, err := service.RegisterTask("task")
	assert.NilError(t, err)
	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)
	err = service.CompletePlanning(run, &PlanningArtifact{PlanSummary: "Freeze the runtime test plan before execution."})
	assert.NilError(t, err)
	return service, run
}

func newRuntimeShellTestService(t *testing.T, content string) (*Service, *RunState) {
	t.Helper()

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "task.yaml"), []byte(content), 0o644)
	assert.NilError(t, err)

	service := NewService(dir, dir)
	run, err := service.RegisterTask("task")
	assert.NilError(t, err)
	return service, run
}

func mustCompileRuntimeTemplate(t *testing.T, template *Template) Template {
	t.Helper()
	err := template.Validate()
	assert.NilError(t, err)
	return *template
}

func TestCommandTimeout(t *testing.T) {
	// Given: a service with a very short command timeout and a step whose check sleeps
	dir := t.TempDir()
	template := mustCompileRuntimeTemplate(t, &Template{
		Version: "0.1",
		Task: Task{
			ID:          "task",
			Name:        "Task",
			Description: "desc",
		},
		Workflow: &Workflow{
			Onboarding: &LifecycleNodeSpec{},
			Planning:   &PlanningNodeSpec{},
			Execution: []ExecutionNodeSpec{
				{
					ID: "step_one",
					Checks: []Check{{
						ID:      "slow_check",
						Command: "sleep 60",
						PreConditions: []Condition{
							{Type: "exit_code", Value: 0},
						},
					}},
				},
			},
		},
	})
	service := NewService(dir, dir)
	service.CommandTimeout = 200 * time.Millisecond
	run := newWorkflowReadyRun(&template)

	// When: starting the step (which runs the slow check)
	start := time.Now()
	result, err := service.StartStep(context.Background(), run, 1)
	elapsed := time.Since(start)

	// Then: the step fails due to timeout rather than hanging
	assert.NilError(t, err)
	assert.Assert(t, !result.Passed, "expected step to fail due to timeout")
	assert.Assert(t, elapsed < 5*time.Second, "command should have been killed by timeout, took %v", elapsed)
}

func newWorkflowReadyRun(template *Template) *RunState {
	return &RunState{
		RunID:            newTaskRunID(),
		TemplateID:       template.Task.ID,
		SelectedTemplate: *template,
		Status:           TaskStatusActive,
		Phase:            template.CompiledWorkflow.FirstExecutablePath,
		WorkflowReady:    true,
		RunnableTemplate: template,
		Steps:            newWorkflowStepStates(template.CompiledWorkflow),
	}
}
