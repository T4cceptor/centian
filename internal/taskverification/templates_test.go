package taskverification

import (
	"os"
	"path/filepath"
	"testing"

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
steps:
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
steps:
  - id: "failing_test"
    checks:
      - id: "selected_test_fails"
        command: "printf '%s' '${testName}'"
`, "simple_tdd.yaml")

	run, err := service.RegisterTask("simple_tdd", map[string]string{})
	assert.NilError(t, err)
	assert.Equal(t, run.Status, TaskStatusActive)
	assert.Equal(t, run.Phase, TaskPhaseRegistered)
	assert.Assert(t, !run.ExecutionReady)
	assert.Assert(t, run.ExecutionTemplate == nil)
	assert.Equal(t, len(run.Steps), 0)

	_, err = service.RegisterTask("simple_tdd", map[string]string{
		"testName": "MyTest",
		"unknown":  "value",
	})
	assert.ErrorContains(t, err, `unknown task parameter "unknown"`)
}

func TestPrepareExecutionResolvesDraftParameters(t *testing.T) {
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
steps:
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

	err = service.PrepareExecution(run)
	assert.NilError(t, err)
	assert.Assert(t, run.ExecutionReady)
	assert.Assert(t, run.ExecutionTemplate != nil)
	assert.Equal(t, run.Phase, TaskPhaseExecution)
	assert.Equal(t, run.ExecutionTemplate.Steps[0].Checks[0].Command, "printf '%s' 'TestThing:boom'")
}

func TestInvalidTemplateFailsValidation(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte(`
version: "0.1"
task:
  id: ""
  name: "Broken"
  description: "broken"
steps: []
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
steps:
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

func newTemplateTestService(t *testing.T, content, fileName string) *Service {
	t.Helper()

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, fileName), []byte(content), 0o644)
	assert.NilError(t, err)
	return NewService(dir, dir)
}
