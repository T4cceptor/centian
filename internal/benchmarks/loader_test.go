package benchmarks

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/assert"
)

func TestLoadSuiteLoadsValidSuite(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)

	suite, err := LoadSuite(suiteRoot)
	assert.NilError(t, err)
	assert.Equal(t, suite.Suite.ID, "simple_tdd_v1")
	assert.Equal(t, suite.Suite.TemplateID, "simple_tdd")
	assert.Equal(t, len(suite.Cases), 3)

	caseDef, err := LoadCase(suiteRoot, suite.Cases[0])
	assert.NilError(t, err)
	assert.Equal(t, caseDef.Case.ID, "compile_failure_red")

	prompt, err := LoadPrompt(filepath.Join(suiteRoot, suite.Cases[0].Path), caseDef.PromptFile)
	assert.NilError(t, err)
	assert.Assert(t, prompt.Prompt != "")
}

func TestLoadSuiteRejectsDuplicateCaseIDs(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	mustWriteFile(t, filepath.Join(suiteRoot, suiteFileName), `
version: "0.1"
suite:
  id: "simple_tdd_v1"
  name: "Simple TDD Benchmark Suite v1"
  description: "Initial local benchmark suite."
  templateId: "simple_tdd"
  localOnly: true
  scoringSchemaVersion: "v1"
  executionProtocolVersion: "v1"
cases:
  - id: "compile_failure_red"
    path: "cases/compile_failure_red"
  - id: "compile_failure_red"
    path: "cases/assertion_failure_red"
`)

	_, err := LoadSuite(suiteRoot)
	assert.ErrorContains(t, err, `duplicate suite case id "compile_failure_red"`)
}

func TestLoadSuiteRejectsMissingTemplateID(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	mustWriteFile(t, filepath.Join(suiteRoot, suiteFileName), `
version: "0.1"
suite:
  id: "simple_tdd_v1"
  name: "Simple TDD Benchmark Suite v1"
  description: "Initial local benchmark suite."
  templateId: ""
  localOnly: true
  scoringSchemaVersion: "v1"
  executionProtocolVersion: "v1"
cases:
  - id: "compile_failure_red"
    path: "cases/compile_failure_red"
`)

	_, err := LoadSuite(suiteRoot)
	assert.ErrorContains(t, err, "suite templateId is required")
}

func TestLoadSuiteRejectsEmptyPrompt(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	mustWriteFile(t, filepath.Join(suiteRoot, "cases", "compile_failure_red", "prompt.yaml"), `
prompt: ""
`)

	_, err := LoadSuite(suiteRoot)
	assert.ErrorContains(t, err, "must define a non-empty prompt")
}

func TestLoadSuiteRejectsMissingFixtureSeedPath(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	mustWriteFile(t, filepath.Join(suiteRoot, "cases", "compile_failure_red", caseFileName), `
version: "0.1"
case:
  id: "compile_failure_red"
  name: "Compile-failure red baseline"
  description: "Authored red baseline using compiler output."
promptFile: "prompt.yaml"
fixture:
  seedPath: "missing"
  resetMode: "copy_seed"
  startingRepoStateSummary: "Authored red baseline already exists."
expectations:
  selectedCommand: "go test ./internal/health -run TestHealthStatus"
  redSignal:
    type: "output_contains"
    value: "undefined: HealthStatus"
  greenConditionSummary: "Selected command exits 0."
constraints:
  lockedPaths:
    - "internal/health/health_test.go"
`)

	_, err := LoadSuite(suiteRoot)
	assert.ErrorContains(t, err, `fixture seedPath "missing" does not exist`)
}

func TestLoadSuiteRejectsAbsoluteCasePath(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	mustWriteFile(t, filepath.Join(suiteRoot, suiteFileName), `
version: "0.1"
suite:
  id: "simple_tdd_v1"
  name: "Simple TDD Benchmark Suite v1"
  description: "Initial local benchmark suite."
  templateId: "simple_tdd"
  localOnly: true
  scoringSchemaVersion: "v1"
  executionProtocolVersion: "v1"
cases:
  - id: "compile_failure_red"
    path: "/tmp/compile_failure_red"
`)

	_, err := LoadSuite(suiteRoot)
	assert.ErrorContains(t, err, `case path "/tmp/compile_failure_red" must be relative`)
}

func TestLoadSuiteRejectsPathTraversal(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	mustWriteFile(t, filepath.Join(suiteRoot, suiteFileName), `
version: "0.1"
suite:
  id: "simple_tdd_v1"
  name: "Simple TDD Benchmark Suite v1"
  description: "Initial local benchmark suite."
  templateId: "simple_tdd"
  localOnly: true
  scoringSchemaVersion: "v1"
  executionProtocolVersion: "v1"
cases:
  - id: "compile_failure_red"
    path: "../escape"
`)

	_, err := LoadSuite(suiteRoot)
	assert.ErrorContains(t, err, `case path "../escape" must stay within the suite root`)
}

func TestLoadSuiteRejectsAbsoluteLockedPath(t *testing.T) {
	suiteRoot := writeValidSuiteFixture(t)
	mustWriteFile(t, filepath.Join(suiteRoot, "cases", "compile_failure_red", caseFileName), `
version: "0.1"
case:
  id: "compile_failure_red"
  name: "Compile-failure red baseline"
  description: "Authored red baseline using compiler output."
promptFile: "prompt.yaml"
fixture:
  seedPath: "fixture"
  resetMode: "copy_seed"
  startingRepoStateSummary: "Authored red baseline already exists."
expectations:
  selectedCommand: "go test ./internal/health -run TestHealthStatus"
  redSignal:
    type: "output_contains"
    value: "undefined: HealthStatus"
  greenConditionSummary: "Selected command exits 0."
constraints:
  lockedPaths:
    - "/tmp/health_test.go"
`)

	_, err := LoadSuite(suiteRoot)
	assert.ErrorContains(t, err, `locked path "/tmp/health_test.go" must be relative`)
}

func TestLoadCheckedInSimpleTDDSuite(t *testing.T) {
	repoRoot := findRepoRoot(t)
	suiteRoot := filepath.Join(repoRoot, "tests", "integrationtests", "taskverification", "benchmarks", "simple_tdd_v1")

	suite, err := LoadSuite(suiteRoot)
	assert.NilError(t, err)
	assert.Equal(t, suite.Suite.ID, "simple_tdd_v1")
	assert.Equal(t, suite.Suite.TemplateID, "simple_tdd")
	assert.Equal(t, len(suite.Cases), 3)

	for _, ref := range suite.Cases {
		caseDef, err := LoadCase(suiteRoot, ref)
		assert.NilError(t, err)
		assert.Equal(t, caseDef.Case.ID, ref.ID)

		prompt, err := LoadPrompt(filepath.Join(suiteRoot, ref.Path), caseDef.PromptFile)
		assert.NilError(t, err)
		assert.Assert(t, prompt.Prompt != "")
	}
}

func writeValidSuiteFixture(t *testing.T) string {
	t.Helper()

	suiteRoot := t.TempDir()
	mustWriteFile(t, filepath.Join(suiteRoot, suiteFileName), `
version: "0.1"
suite:
  id: "simple_tdd_v1"
  name: "Simple TDD Benchmark Suite v1"
  description: "Initial local benchmark suite."
  templateId: "simple_tdd"
  localOnly: true
  scoringSchemaVersion: "v1"
  executionProtocolVersion: "v1"
cases:
  - id: "compile_failure_red"
    path: "cases/compile_failure_red"
  - id: "assertion_failure_red"
    path: "cases/assertion_failure_red"
  - id: "green_then_refactor"
    path: "cases/green_then_refactor"
`)

	writeValidCase(t, suiteRoot, "compile_failure_red", "Compile-failure red baseline", "undefined: HealthStatus")
	writeValidCase(t, suiteRoot, "assertion_failure_red", "Assertion-failure red baseline", "--- FAIL: TestHealthStatus")
	writeValidCase(t, suiteRoot, "green_then_refactor", "Green then refactor", "--- FAIL: TestScoreParentheses")
	return suiteRoot
}

func writeValidCase(t *testing.T, suiteRoot string, caseID string, caseName string, redSignal string) {
	t.Helper()

	caseRoot := filepath.Join(suiteRoot, "cases", caseID)
	mustWriteFile(t, filepath.Join(caseRoot, caseFileName), `
version: "0.1"
case:
  id: "`+caseID+`"
  name: "`+caseName+`"
  description: "Authored red baseline benchmark case."
promptFile: "prompt.yaml"
fixture:
  seedPath: "fixture"
  resetMode: "copy_seed"
  startingRepoStateSummary: "Authored red baseline already exists."
expectations:
  selectedCommand: "go test ./internal/example -run TestExample"
  redSignal:
    type: "output_contains"
    value: "`+redSignal+`"
  greenConditionSummary: "Selected command exits 0 without editing the locked baseline file."
constraints:
  lockedPaths:
    - "internal/example/example_test.go"
  allowedAdditionalPaths:
    - "internal/example"
`)
	mustWriteFile(t, filepath.Join(caseRoot, "prompt.yaml"), `
prompt: |
  Implement the missing behavior using the selected command as the source of truth.
`)
	mustWriteFile(t, filepath.Join(caseRoot, "fixture", "go.mod"), `
module example.com/fixture

go 1.25.1
`)
	mustWriteFile(t, filepath.Join(caseRoot, "fixture", "internal", "example", "example.go"), `
package example
`)
	mustWriteFile(t, filepath.Join(caseRoot, "fixture", "internal", "example", "example_test.go"), `
package example
`)
}

func mustWriteFile(t *testing.T, path string, contents string) {
	t.Helper()
	assert.NilError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	assert.NilError(t, os.WriteFile(path, []byte(contents), 0o644))
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	assert.NilError(t, err)

	current := wd
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatalf("failed to locate repo root from %q", wd)
		}
		current = parent
	}
}
