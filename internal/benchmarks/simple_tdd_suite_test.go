package benchmarks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"gotest.tools/assert"
)

func TestSimpleTDDSuiteFixturesStartRed(t *testing.T) {
	suiteRoot := checkedInSimpleTDDSuiteRoot(t)
	suite, err := LoadSuite(suiteRoot)
	assert.NilError(t, err)

	for _, ref := range suite.Cases {
		t.Run(ref.ID, func(t *testing.T) {
			caseDef, prompt, caseRoot := loadCheckedInCase(t, suiteRoot, ref)
			projectDir := filepath.Join(t.TempDir(), "project")
			fixtureRoot := filepath.Join(caseRoot, caseDef.Fixture.SeedPath)

			assert.NilError(t, common.CopyDir(fixtureRoot, projectDir))
			lockedPath := filepath.Join(projectDir, caseDef.Constraints.LockedPaths[0])
			_, err := os.Stat(lockedPath)
			assert.NilError(t, err)

			commandParts := strings.Fields(caseDef.Expectations.SelectedCommand)
			if len(commandParts) == 0 {
				t.Fatalf("selected command is empty")
			}
			cmd := exec.Command(commandParts[0], commandParts[1:]...)
			cmd.Dir = projectDir
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("expected %q to fail in red state", caseDef.Expectations.SelectedCommand)
			}
			assert.Assert(t, strings.Contains(string(output), caseDef.Expectations.RedSignal.Value))

			promptText := strings.TrimSpace(prompt.Prompt)
			assert.Assert(t, strings.Contains(promptText, "`simple_tdd`"))
			assert.Assert(t, strings.Contains(strings.ToLower(promptText), "selected command"))
			assert.Assert(t, strings.Contains(promptText, caseDef.Constraints.LockedPaths[0]))
			assert.Assert(t, strings.Contains(strings.ToLower(promptText), "authored red baseline"))
		})
	}
}

func TestGreenThenRefactorPromptRequestsCleanup(t *testing.T) {
	suiteRoot := checkedInSimpleTDDSuiteRoot(t)
	caseDef, prompt, _ := loadCheckedInCase(t, suiteRoot, SuiteCaseRef{
		ID:   "green_then_refactor",
		Path: "cases/green_then_refactor",
	})

	assert.Equal(t, caseDef.Case.ID, "green_then_refactor")
	assert.Assert(t, strings.Contains(strings.ToLower(prompt.Prompt), "refactor"))
	assert.Assert(t, strings.Contains(strings.ToLower(prompt.Prompt), "cleanup"))
}

func TestCentianDemoSuiteScaffoldedBaselineFails(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required to validate the demo benchmark fixture")
	}
	suiteRoot := checkedInBenchmarkSuiteRoot(t, "centian_demo_v1")
	caseDef, prompt, caseRoot := loadCheckedInCase(t, suiteRoot, SuiteCaseRef{
		ID:   "score_parentheses_js",
		Path: "cases/score_parentheses_js",
	})
	projectDir := filepath.Join(t.TempDir(), "project")
	assert.NilError(t, common.CopyDir(filepath.Join(caseRoot, caseDef.Fixture.SeedPath), projectDir))
	assert.NilError(t, os.WriteFile(filepath.Join(projectDir, "package.json"), []byte(`{"type":"module"}`), 0o644))
	assert.NilError(t, os.WriteFile(filepath.Join(projectDir, "scoreParentheses.js"), []byte("export default function scoreParentheses() {\n  return 0;\n}\n"), 0o644))
	assert.NilError(t, os.WriteFile(filepath.Join(projectDir, "scoreParentheses.test.js"), []byte(`import test from "node:test";
import assert from "node:assert/strict";
import scoreParentheses from "./scoreParentheses.js";

test("scoreParentheses scores nested pairs", () => {
  assert.equal(scoreParentheses("(())"), 2);
});
`), 0o644))

	commandParts := strings.Fields(caseDef.Expectations.SelectedCommand)
	cmd := exec.Command(commandParts[0], commandParts[1:]...)
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected %q to fail in red state", caseDef.Expectations.SelectedCommand)
	}
	assert.Assert(t, strings.Contains(string(output), caseDef.Expectations.RedSignal.Value))
	assert.Assert(t, strings.Contains(prompt.Prompt, "guided_tdd_workflow"))
	assert.Assert(t, strings.Contains(prompt.Prompt, "package.json"))
}

func checkedInSimpleTDDSuiteRoot(t *testing.T) string {
	t.Helper()

	return checkedInBenchmarkSuiteRoot(t, "simple_tdd_v1")
}

func checkedInBenchmarkSuiteRoot(t *testing.T, suiteID string) string {
	t.Helper()

	repoRoot, err := FindRepoRoot(".")
	assert.NilError(t, err)
	return filepath.Join(repoRoot, "tests", "integrationtests", "taskverification", "benchmarks", suiteID)
}

func loadCheckedInCase(t *testing.T, suiteRoot string, ref SuiteCaseRef) (*CaseDefinition, *PromptDefinition, string) {
	t.Helper()

	caseDef, err := LoadCase(suiteRoot, ref)
	assert.NilError(t, err)
	caseRoot := filepath.Join(suiteRoot, ref.Path)
	prompt, err := LoadPrompt(caseRoot, caseDef.PromptFile)
	assert.NilError(t, err)
	return caseDef, prompt, caseRoot
}
