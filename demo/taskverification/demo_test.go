package taskverification

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"gotest.tools/assert"
)

const (
	runTaskVerificationIntegrationEnv = "CENTIAN_RUN_TASKVERIFICATION_INTEGRATION"
	taskVerificationScenarioEnv       = "CENTIAN_TASKVERIFICATION_SCENARIO"
	openAIAPIKeyEnv                   = "OPENAI_API_KEY"
	demoHostPort                      = "8678"
	centianServiceName                = "centian"
	agentServiceName                  = "agent"
)

type verificationCommand struct {
	Command        string
	ExpectContains string
}

type taskVerificationScenario struct {
	Name                    string
	PromptPath              string
	ProjectFile             string
	TestFile                string
	ExpectProjectContains   []string
	ExpectTestContains      []string
	PytestCommand           string
	AdditionalVerifications []verificationCommand
}

func TestTaskVerificationE2E(t *testing.T) {
	requireTaskVerificationPrereqs(t)

	scenarios := selectedTaskVerificationScenarios(t, taskVerificationScenarios())
	for idx := range scenarios {
		scenario := &scenarios[idx]
		t.Run(scenario.Name, func(t *testing.T) {
			runTaskVerificationScenario(t, scenario)
		})
	}
}

func taskVerificationScenarios() []taskVerificationScenario {
	return []taskVerificationScenario{
		{
			Name:                  "problem_only",
			PromptPath:            "/agent/prompts/problem_score_parentheses.md",
			ProjectFile:           "score_parentheses.py",
			TestFile:              filepath.Join("tests", "test_score_parentheses.py"),
			ExpectProjectContains: []string{"def score_parentheses"},
			ExpectTestContains:    []string{"score_parentheses"},
			PytestCommand:         "cd /workspace/project && python -m pytest -q tests/test_score_parentheses.py",
			AdditionalVerifications: []verificationCommand{
				{
					Command:        "cd /workspace/project && python - <<'PY'\nfrom score_parentheses import score_parentheses\ncases = {'()': 1, '(())': 2, '()()': 2, '(()(()))': 6, '(()(()(())))': 14}\nfor raw, expected in cases.items():\n    actual = score_parentheses(raw)\n    assert actual == expected, f'{raw}: expected {expected}, got {actual}'\nprint('contract-ok')\nPY",
					ExpectContains: "contract-ok",
				},
			},
		},
		{
			Name:                  "existing_bug",
			PromptPath:            "/agent/prompts/existing_bug_mathlib.md",
			ProjectFile:           "mathlib.py",
			TestFile:              filepath.Join("tests", "test_mathlib.py"),
			ExpectProjectContains: []string{"return a + b"},
			ExpectTestContains:    []string{"test_add_two_numbers", "assert add(1, 2) == 3"},
			PytestCommand:         "cd /workspace/project && python -m pytest -q tests/test_mathlib.py",
		},
	}
}

func requireTaskVerificationPrereqs(t *testing.T) {
	t.Helper()

	if os.Getenv(runTaskVerificationIntegrationEnv) != "1" {
		t.Skipf("set %s=1 to run the Docker task verification integration test", runTaskVerificationIntegrationEnv)
	}
	if strings.TrimSpace(os.Getenv(openAIAPIKeyEnv)) == "" {
		t.Skipf("%s is required for the Docker task verification integration test", openAIAPIKeyEnv)
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker is not available: %v", err)
	}
}

func selectedTaskVerificationScenarios(t *testing.T, scenarios []taskVerificationScenario) []taskVerificationScenario {
	t.Helper()

	selected := strings.TrimSpace(os.Getenv(taskVerificationScenarioEnv))
	if selected == "" || selected == "all" {
		return scenarios
	}

	for idx := range scenarios {
		if scenarios[idx].Name == selected {
			return []taskVerificationScenario{scenarios[idx]}
		}
	}

	names := make([]string, 0, len(scenarios))
	for idx := range scenarios {
		names = append(names, scenarios[idx].Name)
	}
	t.Fatalf("unknown %s value %q, expected one of %v", taskVerificationScenarioEnv, selected, names)
	return nil
}

func runTaskVerificationScenario(t *testing.T, scenario *taskVerificationScenario) {
	t.Helper()

	demoDir := filepath.Clean(filepath.Dir(mustCurrentFile(t)))
	artifactsDir := filepath.Join(demoDir, "artifacts")
	projectDir := filepath.Join(demoDir, "project")

	resetDemoArtifacts(t, artifactsDir)
	resetDemoProject(t, projectDir)
	defer tearDownCompose(t, demoDir)

	buildOutput, err := runCompose(context.Background(), demoDir, "build")
	if err != nil {
		t.Fatalf("docker compose build failed: %v\n%s", err, buildOutput)
	}

	upOutput, err := runCompose(context.Background(), demoDir, "up", "-d", centianServiceName)
	if err != nil {
		t.Fatalf("docker compose up failed: %v\n%s", err, upOutput)
	}

	waitForTCPPort(t, "127.0.0.1:"+demoHostPort, 60*time.Second)

	agentCtx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	agentOutput, err := runCompose(
		agentCtx,
		demoDir,
		"run",
		"--rm",
		"-e",
		"PROMPT_PATH="+scenario.PromptPath,
		agentServiceName,
	)
	if err != nil {
		t.Fatalf("docker compose run agent failed: %v\n%s", err, agentOutput)
	}

	assertComposeCommandPasses(t, demoDir, scenario.PytestCommand)
	assertComposeCommandPasses(t, demoDir, "cd /workspace/project && python -m ruff check .")

	for idx := range scenario.AdditionalVerifications {
		verification := scenario.AdditionalVerifications[idx]
		output := assertComposeCommandPasses(t, demoDir, verification.Command)
		if verification.ExpectContains != "" && !strings.Contains(output, verification.ExpectContains) {
			t.Fatalf("verification output %q does not contain %q", output, verification.ExpectContains)
		}
	}

	assertFileContains(t, filepath.Join(projectDir, scenario.ProjectFile), scenario.ExpectProjectContains)
	assertFileContains(t, filepath.Join(projectDir, scenario.TestFile), scenario.ExpectTestContains)
	assertArtifactsExist(t, artifactsDir, "final_message.txt", "codex-events.jsonl", "codex.stderr.log")
	assertStructuredToolFlow(t, filepath.Join(artifactsDir, "logs"))
}

func mustCurrentFile(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine caller path")
	}
	return file
}

func resetDemoArtifacts(t *testing.T, artifactsDir string) {
	t.Helper()

	assert.NilError(t, os.RemoveAll(artifactsDir))
	assert.NilError(t, os.MkdirAll(filepath.Join(artifactsDir, "logs"), 0o755))
	assert.NilError(t, os.WriteFile(filepath.Join(artifactsDir, ".gitkeep"), []byte{}, 0o644))
}

func resetDemoProject(t *testing.T, projectDir string) {
	t.Helper()

	removeIfPresent(t, filepath.Join(projectDir, "score_parentheses.py"))
	removeIfPresent(t, filepath.Join(projectDir, "tests", "test_score_parentheses.py"))
	assert.NilError(t, os.WriteFile(
		filepath.Join(projectDir, "mathlib.py"),
		[]byte("def add(a: int, b: int) -> int:\n    return a - b\n"),
		0o644,
	))
	assert.NilError(t, os.WriteFile(
		filepath.Join(projectDir, "tests", "test_mathlib.py"),
		[]byte("from mathlib import add\n\n\ndef test_add_two_numbers() -> None:\n    assert add(1, 2) == 3\n"),
		0o644,
	))
}

func removeIfPresent(t *testing.T, path string) {
	t.Helper()

	err := os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return
	}
	assert.NilError(t, err)
}

func tearDownCompose(t *testing.T, demoDir string) {
	t.Helper()

	output, err := runCompose(context.Background(), demoDir, "down", "--remove-orphans")
	if err != nil {
		t.Fatalf("docker compose down failed: %v\n%s", err, output)
	}
}

func assertComposeCommandPasses(t *testing.T, demoDir, shellCommand string) string {
	t.Helper()

	output, err := runCompose(
		context.Background(),
		demoDir,
		"exec",
		"-T",
		centianServiceName,
		"bash",
		"-lc",
		shellCommand,
	)
	if err != nil {
		t.Fatalf("command failed: %s\n%v\n%s", shellCommand, err, output)
	}
	if strings.TrimSpace(output) == "" {
		return output
	}
	return output
}

func assertFileContains(t *testing.T, path string, expected []string) {
	t.Helper()

	content, err := os.ReadFile(path)
	assert.NilError(t, err)
	for _, needle := range expected {
		if !strings.Contains(string(content), needle) {
			t.Fatalf("%s does not contain %q", path, needle)
		}
	}
}

func assertArtifactsExist(t *testing.T, artifactsDir string, names ...string) {
	t.Helper()

	for _, name := range names {
		_, err := os.Stat(filepath.Join(artifactsDir, name))
		assert.NilError(t, err)
	}
}

func runCompose(ctx context.Context, demoDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
	cmd.Dir = demoDir
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func waitForTCPPort(t *testing.T, address string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s: %v", address, err)
		}
		time.Sleep(time.Second)
	}
}

func assertStructuredToolFlow(t *testing.T, logDir string) {
	t.Helper()

	toolNames := collectToolCallNames(readRequestLogEntries(t, logDir))

	assertSubsequence(t, toolNames, []string{
		"centian.task_list_templates",
		"centian.task_register",
		"centian.task_start_step",
		"centian.task_complete_step",
		"centian.task_start_step",
		"centian.task_complete_step",
	})
	assert.Assert(t, containsToolPrefix(toolNames, "filesystem___"))
	assert.Assert(t, containsToolPrefix(toolNames, "shell___"))
	assert.Assert(t, !slices.Contains(toolNames, "centian.task_fail"))
	assert.Assert(t, hasToolPrefixBetween(toolNames, "shell___", "centian.task_start_step", "centian.task_complete_step"))
	assert.Assert(t, hasToolPrefixBetween(toolNames, "filesystem___", "centian.task_complete_step", "centian.task_start_step"))
}

func readRequestLogEntries(t *testing.T, logDir string) []common.LogEntry {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(logDir, "requests_*.jsonl"))
	assert.NilError(t, err)
	if len(matches) == 0 {
		t.Fatalf("no request logs found in %s", logDir)
	}
	slices.Sort(matches)

	entries := make([]common.LogEntry, 0, 32)
	for _, match := range matches {
		data, err := os.ReadFile(match)
		assert.NilError(t, err)

		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var entry common.LogEntry
			assert.NilError(t, json.Unmarshal([]byte(line), &entry))
			entries = append(entries, entry)
		}
	}
	return entries
}

func collectToolCallNames(entries []common.LogEntry) []string {
	names := make([]string, 0, len(entries))
	for idx := range entries {
		if entries[idx].ToolCall != nil && entries[idx].ToolCall.Name != "" {
			names = append(names, entries[idx].ToolCall.Name)
		}
	}
	return names
}

func assertSubsequence(t *testing.T, actual, expected []string) {
	t.Helper()

	index := 0
	for _, item := range actual {
		if index < len(expected) && item == expected[index] {
			index++
		}
	}
	if index != len(expected) {
		t.Fatalf("tool sequence %v does not contain subsequence %v", actual, expected)
	}
}

func containsToolPrefix(names []string, prefix string) bool {
	for _, name := range names {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func hasToolPrefixBetween(names []string, prefix, startName, endName string) bool {
	start := slices.Index(names, startName)
	if start == -1 {
		return false
	}

	for idx := start + 1; idx < len(names); idx++ {
		switch {
		case names[idx] == endName:
			return false
		case strings.HasPrefix(names[idx], prefix):
			return true
		}
	}
	return false
}
