package taskverification

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/T4cceptor/centian/internal/identifiers"
	"gotest.tools/assert"
)

func TestListTemplatesReturnsSummaries(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "Test driven task"
  instructions: "Use Centian step completion as the source of truth."
parameters:
  - name: "testName"
    description: "The test case to target."
workflow:
  onboarding: {}
  planning:
    required_inputs: ["testName"]
  execution:
    - id: "failing_test"
      instructions: "Start the step before changing anything."
      checks:
        - id: "selected_test_fails"
          command: "printf '%s' '${testName}'"
          pre_conditions:
            - type: stdout_contains
              value: "${testName}"
`, "simple_tdd.yaml")

	summaries, err := service.ListTemplates()
	assert.NilError(t, err)
	assert.Equal(t, len(summaries), 1)
	assert.Equal(t, summaries[0].ID, "simple_tdd")
	assert.Equal(t, summaries[0].Instructions, "Use Centian step completion as the source of truth.")
	assert.DeepEqual(t, summaries[0].Parameters, []TemplateParameter{{
		Name:        "testName",
		Description: "The test case to target.",
	}})
	assert.Equal(t, summaries[0].StepCount, 1)
	assert.Equal(t, summaries[0].Steps[0].Instructions, "Start the step before changing anything.")
	assert.Equal(t, summaries[0].Steps[0].Path, TaskPhase("execution.failing_test"))
}

func TestRegisterTaskCreatesShellWithoutPlanningParameters(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "Test driven task"
parameters:
  - name: "testName"
    description: "The test case to target."
workflow:
  onboarding: {}
  planning: {}
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf '%s' '${testName}'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd")
	assert.NilError(t, err)
	assert.Equal(t, run.Status, TaskStatusActive)
	assert.Equal(t, run.Phase, TaskPhaseOnboarding)
	assert.Assert(t, !run.WorkflowReady)
	assert.Assert(t, run.RunnableTemplate == nil)
	assert.Equal(t, len(run.Steps), 0)
}

func TestTimeoutAndResumeTaskPreserveWorkflowState(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "Test driven task"
workflow:
  onboarding: {}
  planning: {}
  execution:
    - id: "step_one"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd")
	assert.NilError(t, err)
	run.Phase = TaskPhase("execution.step_one")
	run.WorkflowReady = true
	run.Steps = []StepState{{
		ID:                 "step_one",
		Path:               TaskPhase("execution.step_one"),
		Status:             StepStatusActive,
		InvariantBaselines: map[string]string{},
	}}
	run.LastActivityAt = 123
	run.ExpiresAt = 456

	err = service.TimeoutTask(run)
	assert.NilError(t, err)
	assert.Equal(t, run.Status, TaskStatusTimedOut)
	assert.Equal(t, run.Phase, TaskPhase("execution.step_one"))
	assert.Equal(t, run.Steps[0].Status, StepStatusActive)

	err = service.ResumeTask(run)
	assert.NilError(t, err)
	assert.Equal(t, run.Status, TaskStatusActive)
	assert.Equal(t, run.Phase, TaskPhase("execution.step_one"))
	assert.Equal(t, run.Steps[0].Status, StepStatusActive)
	assert.Equal(t, run.ExpiresAt, int64(0))
}

func TestListTemplatesIncludesEmbeddedDefaultsWhenDirectoryMissing(t *testing.T) {
	service := NewService(filepath.Join(t.TempDir(), "missing"), t.TempDir())

	summaries, err := service.ListTemplates()
	assert.NilError(t, err)
	assert.Equal(t, len(summaries), 3)
	assert.Equal(t, summaries[0].ID, "guided_tdd_workflow")
	assert.Equal(t, summaries[1].ID, "minimal")
	assert.Equal(t, summaries[2].ID, "simple_tdd")
}

func TestEmbeddedSimpleTDDTemplateExposesRefinedContract(t *testing.T) {
	service := NewService(filepath.Join(t.TempDir(), "missing"), t.TempDir())

	template, err := service.loadTemplateByID("simple_tdd")
	assert.NilError(t, err)

	assert.DeepEqual(t, template.RequiredParameterNames(), []string{
		"expectedError",
		"testCommand",
		"testFile",
		"testTarget",
	})
	assert.Equal(t, len(template.CompiledWorkflow.WorkflowSteps), 3)
	assert.Equal(t, template.CompiledWorkflow.WorkflowSteps[0].ID, "verify_failing_baseline")
	assert.Equal(t, template.CompiledWorkflow.WorkflowSteps[1].ID, "implement_green")
	assert.Equal(t, template.CompiledWorkflow.WorkflowSteps[2].ID, "refactor_while_green")

	resolved, err := template.Resolve(map[string]string{
		"testCommand":   "go test ./pkg/foo -run TestBar",
		"testTarget":    "./pkg/foo -run TestBar",
		"testFile":      "pkg/foo/foo_test.go",
		"expectedError": "undefined: Thing",
	})
	assert.NilError(t, err)

	steps := resolved.CompiledWorkflow.WorkflowSteps
	assert.Equal(t, steps[0].Checks[0].Command, "go test ./pkg/foo -run TestBar")
	assert.Equal(t, steps[1].Checks[0].Command, "go test ./pkg/foo -run TestBar")
	assert.Equal(t, steps[2].Checks[0].Command, "go test ./pkg/foo -run TestBar")
	assert.Equal(t, steps[0].Checks[0].PostConditions[0].Type, "exit_code_in")
	assert.Equal(t, steps[0].Checks[0].PostConditions[1].Type, "output_contains")
	assert.Equal(t, steps[0].Invariants[0].Command, "cat pkg/foo/foo_test.go")
	assert.Equal(t, steps[1].Invariants[0].Command, "cat pkg/foo/foo_test.go")
	assert.Equal(t, steps[2].Invariants[0].Command, "cat pkg/foo/foo_test.go")
}

func TestListTemplatesAllowsDiskOverrideOfEmbeddedTemplate(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "simple_tdd.yaml"), []byte(`
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD Override"
  description: "Override the embedded template."
workflow:
  onboarding: {}
  planning: {}
  execution:
    - id: "override_step"
      name: "Override step"
`), 0o644)
	assert.NilError(t, err)

	service := NewService(dir, dir)
	summaries, err := service.ListTemplates()
	assert.NilError(t, err)
	assert.Equal(t, len(summaries), 3)
	assert.Equal(t, summaries[2].ID, "simple_tdd")
	assert.Equal(t, summaries[2].Name, "Simple TDD Override")
	assert.Equal(t, summaries[2].Steps[0].ID, "override_step")
}

func TestListTemplatesAllowsExecutionStepWithoutChecks(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "free_form"
  name: "Free Form"
  description: "Minimal execution flow without explicit checks."
workflow:
  onboarding: {}
  planning: {}
  execution:
    - id: "step_one"
`, "free_form.yaml")

	summaries, err := service.ListTemplates()
	assert.NilError(t, err)
	assert.Equal(t, len(summaries), 1)
	assert.Equal(t, summaries[0].ID, "free_form")
	assert.Equal(t, summaries[0].StepCount, 1)
	assert.Equal(t, summaries[0].Steps[0].ID, "step_one")
}

func TestListTemplatesAllowsExecutionStepWithoutChecksWhenInvariantsArePresent(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "free_form_with_invariant"
  name: "Free Form With Invariant"
  description: "Execution step uses invariants without checks."
workflow:
  onboarding: {}
  planning: {}
  execution:
    - id: "step_one"
      invariants:
        - id: "stable"
          command: "printf 'same'"
`, "free_form_with_invariant.yaml")

	summaries, err := service.ListTemplates()
	assert.NilError(t, err)
	assert.Equal(t, len(summaries), 1)
	assert.Equal(t, summaries[0].ID, "free_form_with_invariant")
	assert.Equal(t, summaries[0].StepCount, 1)
	assert.Equal(t, summaries[0].Steps[0].ID, "step_one")
}

func TestCompletePlanningResolvesPlanningParametersAndEntersConfiguredExecutionNode(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "Test driven task"
parameters:
  - name: "testName"
    description: "The test case to target."
  - name: "expectedError"
    description: "The failure message to assert."
workflow:
  onboarding: {}
  planning:
    required_inputs: ["expectedError", "testName"]
    next: "execution.failing_test"
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf '%s' '${testName}:${expectedError}'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd")
	assert.NilError(t, err)

	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)

	err = service.CompletePlanning(run, &PlanningArtifact{
		PlanSummary: "Freeze the resolved planning parameters before execution.",
		Parameters: map[string]string{
			"testName":      "TestThing",
			"expectedError": "boom",
		},
	})
	assert.NilError(t, err)

	assert.Assert(t, run.Planning != nil)
	assert.Assert(t, run.WorkflowReady)
	assert.Assert(t, run.RunnableTemplate != nil)
	assert.Equal(t, run.Phase, TaskPhase("execution.failing_test"))
	assert.Equal(t, run.RunnableTemplate.CompiledWorkflow.WorkflowSteps[0].Checks[0].Command, "printf '%s' 'TestThing:boom'")
}

func TestCompletePlanningUsesResolvedExecutionPathForParameterizedStepID(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "free_form"
  name: "Free Form"
  description: "Parameterized step id."
parameters:
  - name: "taskName"
    description: "Human-readable task name."
workflow:
  onboarding: {}
  planning: {}
  execution:
    - id: "Task ${taskName}"
`, "free_form.yaml")

	run, err := service.RegisterTask("free_form")
	assert.NilError(t, err)

	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)

	err = service.CompletePlanning(run, &PlanningArtifact{
		PlanSummary: "Freeze the resolved task-specific execution path.",
		Parameters:  map[string]string{"taskName": "Investigate issue"},
	})
	assert.NilError(t, err)
	assert.Equal(t, run.Phase, TaskPhase("execution.Task Investigate issue"))
}

func TestCompletePlanningAllowsEditableFieldsForResolvedDeclaredParameters(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "Test driven task"
parameters:
  - name: "testCommand"
    description: "Test command."
  - name: "expectedError"
    description: "Expected error."
workflow:
  onboarding: {}
  planning:
    editable_fields: ["parameters.testCommand", "parameters.expectedError"]
    required_inputs: ["expectedError", "testCommand"]
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf '%s' '${testCommand}:${expectedError}'"
          pre_conditions:
            - type: stdout_contains
              value: "pytest:boom"
          post_conditions:
            - type: stdout_contains
              value: "pytest:boom"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd")
	assert.NilError(t, err)

	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)

	err = service.CompletePlanning(run, &PlanningArtifact{
		PlanSummary: "Freeze the editable planning parameters before execution.",
		Parameters: map[string]string{
			"testCommand":   "pytest",
			"expectedError": "boom",
		},
	})
	assert.NilError(t, err)
	assert.Equal(t, run.Phase, TaskPhase("execution.failing_test"))
	assert.Assert(t, run.WorkflowReady)
}

func TestStartAndCompleteOnboarding(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "Test driven task"
workflow:
  onboarding: {}
  planning: {}
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf 'ok'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd")
	assert.NilError(t, err)
	assert.Assert(t, run.Onboarding == nil)

	artifact := OnboardingArtifact{
		TaskSummary: "Small Python task context with one pytest target.",
		ArtifactMap: []OnboardingArtifactRef{
			{Path: "/workspace/project/tests", Kind: "tests", Notes: "Pytest location"},
		},
		CommonCommands: []OnboardingCommand{
			{Command: "python -m pytest -q", Purpose: "Run focused tests"},
		},
		Constraints:   []string{"Use Centian tools only"},
		OpenQuestions: []string{"Which test should planning target?"},
	}

	err = service.CompleteOnboarding(run, &artifact)
	assert.NilError(t, err)
	assert.Equal(t, run.Phase, TaskPhasePlanning)
	assert.Assert(t, run.Onboarding != nil)
	assert.Equal(t, run.Onboarding.TaskSummary, artifact.TaskSummary)
	assert.Equal(t, run.Onboarding.ArtifactMap[0].Path, artifact.ArtifactMap[0].Path)
}

func TestRegisterTaskStartsInOnboarding(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "Test driven task"
workflow:
  onboarding: {}
  planning: {}
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf 'ok'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd")
	assert.NilError(t, err)
	assert.Assert(t, identifiers.IsKind(run.RunID, identifiers.KindTaskRun))
	assert.Equal(t, run.Phase, TaskPhaseOnboarding)
	run.Onboarding = &OnboardingArtifact{TaskSummary: "existing summary"}
	assert.Assert(t, run.Onboarding != nil)
	assert.Equal(t, run.Onboarding.TaskSummary, "existing summary")
}

func TestCompleteOnboardingRequiresOnboardingPhase(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "Test driven task"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf 'ok'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd")
	assert.NilError(t, err)

	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)

	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready again"})
	assert.ErrorContains(t, err, "cannot transition to planning")
}

func TestCompleteOnboardingValidatesTaskSummary(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "Test driven task"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf 'ok'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd")
	assert.NilError(t, err)

	err = service.CompleteOnboarding(run, &OnboardingArtifact{})
	assert.ErrorContains(t, err, "onboarding.taskSummary is required")
	assert.Equal(t, run.Phase, TaskPhaseOnboarding)
	assert.Assert(t, run.Onboarding == nil)
}

func TestRestartPreservesOnboardingState(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "Test driven task"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf 'ok'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd")
	assert.NilError(t, err)
	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "stored summary"})
	assert.NilError(t, err)

	err = service.RestartTask(run)
	assert.NilError(t, err)
	assert.Equal(t, run.Phase, TaskPhaseOnboarding)
	assert.Assert(t, run.Onboarding != nil)
	assert.Equal(t, run.Onboarding.TaskSummary, "stored summary")
	assert.Assert(t, run.Planning == nil)
}

func TestCompletePlanningRequiresPlanningPhase(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "Test driven task"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf 'ok'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd")
	assert.NilError(t, err)

	err = service.CompletePlanning(run, &PlanningArtifact{PlanSummary: "Attempt to complete planning before onboarding should still fail by phase."})
	assert.ErrorContains(t, err, "cannot transition to")
}

func TestCompletePlanningValidatesPlanningArtifact(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "Test driven task"
parameters:
  - name: "testTarget"
    description: "Targeted test command."
workflow:
  onboarding: {}
  planning:
    required_inputs: ["testTarget"]
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf '%s' '${testTarget}'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd")
	assert.NilError(t, err)
	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)

	err = service.CompletePlanning(run, &PlanningArtifact{})
	assert.Assert(t, err != nil)
	assert.ErrorContains(t, err, "planning.planSummary is required")

	err = service.CompletePlanning(run, &PlanningArtifact{
		PlanSummary:   "Freeze the targeted test command and selected files before execution.",
		Parameters:    map[string]string{"testTarget": "pytest -q"},
		SelectedFiles: []string{"a.go", "a.go"},
	})
	assert.ErrorContains(t, err, `planning.selectedFiles contains duplicate value "a.go"`)
}

func TestValidatePlanningParametersRequiresDeclaredInputs(t *testing.T) {
	template := &Template{
		Parameters: []TemplateParameter{
			{Name: "testTarget"},
			{Name: "expectedError"},
		},
		Workflow: &Workflow{
			Scaffolding: []ExecutionNodeSpec{{
				ID: "setup",
				Checks: []Check{{
					ID:      "check",
					Command: "printf '%s' '${testTarget}:${expectedError}'",
				}},
			}},
		},
	}
	err := validatePlanningParameters(template, map[string]string{})
	var validationErr *PlanningValidationError
	assert.Assert(t, errors.As(err, &validationErr))
	assert.DeepEqual(t, validationErr.MissingParameters, []string{"expectedError", "testTarget"})
	assert.DeepEqual(t, validationErr.RequiredParameterNames, []string{"expectedError", "testTarget"})
	assert.ErrorContains(t, err, "planning.parameters is invalid")
}

func TestValidatePlanningParametersReturnsUnknownAndMissingDetails(t *testing.T) {
	template := &Template{
		Parameters: []TemplateParameter{
			{Name: "expectedError"},
			{Name: "testTarget"},
		},
		Workflow: &Workflow{
			Execution: []ExecutionNodeSpec{{
				ID: "step_one",
				Checks: []Check{{
					ID:      "check_one",
					Command: "printf '%s' '${testTarget}:${expectedError}'",
				}},
			}},
		},
	}

	err := validatePlanningParameters(template, map[string]string{
		"expectedError": "boom",
		"unknown":       "value",
	})
	var validationErr *PlanningValidationError
	assert.Assert(t, errors.As(err, &validationErr))
	assert.DeepEqual(t, validationErr.MissingParameters, []string{"testTarget"})
	assert.DeepEqual(t, validationErr.UnknownParameters, []string{"unknown"})
	assert.DeepEqual(t, validationErr.ProvidedParameterNames, []string{"expectedError", "unknown"})
	assert.ErrorContains(t, err, "planning.parameters is invalid")
}

func TestResolvePreservesCheckAndInvariantDescriptions(t *testing.T) {
	template := mustCompileTemplate(t, &Template{
		Version: "0.1",
		Task: Task{
			ID:          "task",
			Name:        "Task",
			Description: "desc",
		},
		Parameters: []TemplateParameter{
			{Name: "testFile"},
		},
		Workflow: &Workflow{
			Onboarding: &LifecycleNodeSpec{},
			Planning:   &PlanningNodeSpec{},
			Execution: []ExecutionNodeSpec{{
				ID: "step_one",
				Checks: []Check{{
					ID:          "check_one",
					Description: "Explains the check",
					Command:     "printf ok",
					PostConditions: []Condition{
						{Type: "file_exists", Path: "${testFile}"},
					},
				}},
				Invariants: []Invariant{{
					ID:          "stable_test_file",
					Description: "Keeps the selected test file stable",
					Command:     "cat ${testFile}",
				}},
			}},
		},
	})

	resolved, err := template.Resolve(map[string]string{"testFile": "test_score_parentheses.py"})
	assert.NilError(t, err)
	assert.Equal(t, resolved.CompiledWorkflow.WorkflowSteps[0].Checks[0].Description, "Explains the check")
	assert.Equal(t, resolved.CompiledWorkflow.WorkflowSteps[0].Invariants[0].Description, "Keeps the selected test file stable")
}

func TestPlanningCanAdvanceToConfiguredWaitingNode(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "approval_flow"
  name: "Approval Flow"
  description: "Task with approval pause"
workflow:
  onboarding: {}
  planning:
    next: "waiting_for_approval.review_plan"
  execution:
    - id: "review_plan"
      kind: "waiting_for_approval"
      next: "execution.implement_fix"
    - id: "implement_fix"
      checks:
        - id: "selected_test_passes"
          command: "printf 'ok'"
`, "approval_flow.yaml")

	run, err := service.RegisterTask("approval_flow")
	assert.NilError(t, err)
	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)
	err = service.CompletePlanning(run, &PlanningArtifact{PlanSummary: "Freeze the plan before waiting for approval."})
	assert.NilError(t, err)
	assert.Equal(t, run.Phase, TaskPhase("waiting_for_approval.review_plan"))
	node, exists := run.CurrentNode()
	assert.Assert(t, exists)
	assert.Equal(t, node.Kind, WorkflowNodeKindWaitingForApproval)
	assert.Equal(t, node.NextPath, TaskPhase("execution.implement_fix"))
}

func TestInvalidTemplateFailsValidation(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte(`
version: "0.1"
task:
  id: ""
  name: "Broken"
  description: "broken"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "step_one"
      checks:
        - id: "check_one"
          command: "printf 'ok'"
`), 0o644)
	assert.NilError(t, err)

	service := NewService(dir, dir)
	_, err = service.ListTemplates()
	assert.ErrorContains(t, err, "invalid task template")
}

func TestTemplateValidationRequiresDeclaredParameterCoverage(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte(`
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "broken"
parameters:
  - name: "unused"
    description: "Unused."
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf '%s' '${testName}'"
`), 0o644)
	assert.NilError(t, err)

	service := NewService(dir, dir)
	_, err = service.ListTemplates()
	assert.ErrorContains(t, err, `parameter "unused" is defined but not used by any placeholder`)
}

func TestTemplateValidationRejectsMissingWorkflow(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte(`
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "broken"
`), 0o644)
	assert.NilError(t, err)

	service := NewService(dir, dir)
	_, err = service.ListTemplates()
	assert.ErrorContains(t, err, "workflow is required")
}

func TestTemplateValidationRejectsInvalidWaitingPlacement(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte(`
version: "0.1"
task:
  id: "approval_flow"
  name: "Approval Flow"
  description: "broken"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
  execution:
    - id: "step_one"
      checks:
        - id: "check_one"
          command: "printf 'ok'"
    - id: "wait_forever"
      kind: "waiting_for_approval"
`), 0o644)
	assert.NilError(t, err)

	service := NewService(dir, dir)
	_, err = service.ListTemplates()
	assert.ErrorContains(t, err, "cannot be a terminal waiting_for_approval node")
}

func TestTemplateValidationRejectsInvalidPlanningEditableField(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte(`
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "broken"
parameters:
  - name: "testName"
    description: "Target"
workflow:
  onboarding: {}
  planning:
    editable_fields: ["parameters.unknown"]
    required_inputs: []
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf '%s' '${testName}'"
`), 0o644)
	assert.NilError(t, err)

	service := NewService(dir, dir)
	_, err = service.ListTemplates()
	assert.ErrorContains(t, err, `references unknown parameter "unknown"`)
}

func TestTemplateValidationRejectsUnreachableWorkflowNode(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte(`
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "broken"
workflow:
  onboarding: {}
  planning:
    required_inputs: []
    next: "execution.second"
  execution:
    - id: "first"
      checks:
        - id: "check_one"
          command: "printf 'ok'"
    - id: "second"
      checks:
        - id: "check_two"
          command: "printf 'ok'"
`), 0o644)
	assert.NilError(t, err)

	service := NewService(dir, dir)
	_, err = service.ListTemplates()
	assert.ErrorContains(t, err, `workflow node "execution.first" is unreachable`)
}

func newTemplateTestService(t *testing.T, content, fileName string) *Service {
	t.Helper()

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, fileName), []byte(content), 0o644)
	assert.NilError(t, err)
	return NewServiceWithOptions(dir, dir, ServiceOptions{})
}

func mustCompileTemplate(t *testing.T, template *Template) *Template {
	t.Helper()
	err := template.Validate()
	assert.NilError(t, err)
	return template
}
