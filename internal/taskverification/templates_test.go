package taskverification

import (
	"os"
	"path/filepath"
	"strings"
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
    required_outputs: ["testTarget"]
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

func TestRegisterTaskAllowsMissingParametersButRejectsUnknownOnShellRegistration(t *testing.T) {
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
  planning:
    required_outputs: ["testTarget"]
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf '%s' '${testName}'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd", map[string]string{})
	assert.NilError(t, err)
	assert.Equal(t, run.Status, TaskStatusActive)
	assert.Equal(t, run.Phase, TaskPhaseOnboarding)
	assert.Assert(t, !run.WorkflowReady)
	assert.Assert(t, run.RunnableTemplate == nil)
	assert.Equal(t, len(run.Steps), 0)

	_, err = service.RegisterTask("simple_tdd", map[string]string{
		"testName": "MyTest",
		"unknown":  "value",
	})
	assert.ErrorContains(t, err, `unknown task parameter "unknown"`)
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

func TestCompletePlanningResolvesDraftParametersAndEntersConfiguredExecutionNode(t *testing.T) {
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
    required_outputs: ["testTarget"]
    next: "execution.failing_test"
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf '%s' '${testName}:${expectedError}'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd", map[string]string{
		"testName":      "TestThing",
		"expectedError": "boom",
	})
	assert.NilError(t, err)

	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)

	err = service.CompletePlanning(run, &PlanningArtifact{
		TestTarget: "pytest tests/test_thing.py -q",
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

	run, err := service.RegisterTask("free_form", map[string]string{
		"taskName": "Investigate issue",
	})
	assert.NilError(t, err)

	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)

	err = service.CompletePlanning(run, &PlanningArtifact{})
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
    required_outputs: ["testTarget"]
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

	run, err := service.RegisterTask("simple_tdd", map[string]string{
		"testCommand":   "pytest",
		"expectedError": "boom",
	})
	assert.NilError(t, err)

	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)

	err = service.CompletePlanning(run, &PlanningArtifact{TestTarget: "tests/test_thing.py"})
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
  planning:
    required_outputs: ["testTarget"]
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf 'ok'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd", map[string]string{})
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
  planning:
    required_outputs: ["testTarget"]
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf 'ok'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd", map[string]string{})
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
    required_outputs: ["testTarget"]
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf 'ok'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd", map[string]string{})
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
    required_outputs: ["testTarget"]
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf 'ok'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd", map[string]string{})
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
    required_outputs: ["testTarget"]
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf 'ok'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd", map[string]string{})
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
    required_outputs: ["testTarget"]
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf 'ok'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd", map[string]string{})
	assert.NilError(t, err)

	err = service.CompletePlanning(run, &PlanningArtifact{TestTarget: "pytest -q"})
	assert.ErrorContains(t, err, "cannot transition to")
}

func TestCompletePlanningValidatesPlanningArtifact(t *testing.T) {
	service := newTemplateTestService(t, `
version: "0.1"
task:
  id: "simple_tdd"
  name: "Simple TDD"
  description: "Test driven task"
workflow:
  onboarding: {}
  planning:
    required_outputs: ["testTarget", "selectedFiles"]
  execution:
    - id: "failing_test"
      checks:
        - id: "selected_test_fails"
          command: "printf 'ok'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd", map[string]string{})
	assert.NilError(t, err)
	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)

	err = service.CompletePlanning(run, &PlanningArtifact{})
	assert.Assert(t, err != nil)
	assert.Assert(
		t,
		strings.Contains(err.Error(), "planning.testTarget is required") ||
			strings.Contains(err.Error(), "planning.selectedFiles is required"),
	)

	err = service.CompletePlanning(run, &PlanningArtifact{
		TestTarget:    "pytest -q",
		SelectedFiles: []string{"a.go", "a.go"},
	})
	assert.ErrorContains(t, err, `planning.selectedFiles contains duplicate value "a.go"`)
}

func TestValidatePlanningOutput_RequiresScalarFields(t *testing.T) {
	for _, output := range []string{"testTarget", "lintCommand", "expectedFailure", "implementationTarget"} {
		err := validatePlanningOutput(output, &PlanningArtifact{})
		assert.ErrorContains(t, err, "planning."+output+" is required")
	}
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
    required_outputs: ["testTarget"]
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

	run, err := service.RegisterTask("approval_flow", map[string]string{})
	assert.NilError(t, err)
	err = service.CompleteOnboarding(run, &OnboardingArtifact{TaskSummary: "ready"})
	assert.NilError(t, err)
	err = service.CompletePlanning(run, &PlanningArtifact{TestTarget: "pytest -q"})
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
    required_outputs: ["testTarget"]
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
    required_outputs: ["testTarget"]
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
    required_outputs: ["testTarget"]
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
    required_outputs: ["testTarget"]
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
    required_outputs: ["testTarget"]
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
	return NewService(dir, dir)
}
