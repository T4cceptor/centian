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
	"github.com/T4cceptor/centian/internal/persistence"
	tv "github.com/T4cceptor/centian/internal/taskverification"
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

type taskToolExpectation struct {
	ToolName        string
	Phase           string
	CurrentNodeKind string
	NextNodePath    string
	ApprovalBlocked *bool
}

type taskVerificationScenario struct {
	Name                     string
	PromptPath               string
	ProjectFile              string
	TestFile                 string
	ExpectProjectContains    []string
	ExpectTestContains       []string
	PytestCommand            string
	AdditionalVerifications  []verificationCommand
	RequireProjectAssertions bool
	RequirePytestAndLint     bool
	ExpectedToolSubsequence  []string
	ExpectedTaskToolResults  []taskToolExpectation
	ExpectedTaskEventTypes   []tv.TaskEventType
	ExpectBlockedToolPrefix  string
}

type loggedToolResult struct {
	IsError           bool           `json:"isError"`
	StructuredContent map[string]any `json:"structuredContent"`
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
	falseValue := false
	trueValue := true
	fullFlowTools := []string{
		"centian.task_list_templates",
		"centian.task_register",
		"centian.task_complete_onboarding",
		"centian.task_complete_planning",
		"centian.task_start_step",
		"centian.task_complete_step",
		"centian.task_start_step",
		"centian.task_complete_step",
	}
	fullFlowExpectations := []taskToolExpectation{
		{
			ToolName:        "centian.task_register",
			Phase:           "onboarding",
			CurrentNodeKind: string(tv.WorkflowNodeKindOnboarding),
			NextNodePath:    "planning",
			ApprovalBlocked: &falseValue,
		},
		{
			ToolName:        "centian.task_complete_onboarding",
			Phase:           "planning",
			CurrentNodeKind: string(tv.WorkflowNodeKindPlanning),
			NextNodePath:    "execution.establish_failing_baseline",
			ApprovalBlocked: &falseValue,
		},
		{
			ToolName:        "centian.task_complete_planning",
			Phase:           "execution.establish_failing_baseline",
			CurrentNodeKind: string(tv.WorkflowNodeKindExecution),
			NextNodePath:    "execution.implement_solution",
			ApprovalBlocked: &falseValue,
		},
	}

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
			RequireProjectAssertions: true,
			RequirePytestAndLint:     true,
			ExpectedToolSubsequence:  fullFlowTools,
			ExpectedTaskToolResults:  fullFlowExpectations,
			ExpectedTaskEventTypes: []tv.TaskEventType{
				tv.TaskEventTypeRegistered,
				tv.TaskEventTypeOnboardingCompleted,
				tv.TaskEventTypePlanningCompleted,
				tv.TaskEventTypeStepStarted,
				tv.TaskEventTypeStepCompleted,
				tv.TaskEventTypeStepStarted,
				tv.TaskEventTypeStepCompleted,
			},
		},
		{
			Name:                     "existing_bug",
			PromptPath:               "/agent/prompts/existing_bug_mathlib.md",
			ProjectFile:              "mathlib.py",
			TestFile:                 filepath.Join("tests", "test_mathlib.py"),
			ExpectProjectContains:    []string{"return a + b"},
			ExpectTestContains:       []string{"test_add_two_numbers", "assert add(1, 2) == 3"},
			PytestCommand:            "cd /workspace/project && python -m pytest -q tests/test_mathlib.py",
			RequireProjectAssertions: true,
			RequirePytestAndLint:     true,
			ExpectedToolSubsequence:  fullFlowTools,
			ExpectedTaskToolResults:  fullFlowExpectations,
			ExpectedTaskEventTypes: []tv.TaskEventType{
				tv.TaskEventTypeRegistered,
				tv.TaskEventTypeOnboardingCompleted,
				tv.TaskEventTypePlanningCompleted,
				tv.TaskEventTypeStepStarted,
				tv.TaskEventTypeStepCompleted,
				tv.TaskEventTypeStepStarted,
				tv.TaskEventTypeStepCompleted,
			},
		},
		{
			Name:       "approval_wait",
			PromptPath: "/agent/prompts/approval_wait_mathlib.md",
			ExpectedToolSubsequence: []string{
				"centian.task_list_templates",
				"centian.task_register",
				"centian.task_complete_onboarding",
				"centian.task_complete_planning",
			},
			ExpectedTaskToolResults: []taskToolExpectation{
				{
					ToolName:        "centian.task_register",
					Phase:           "onboarding",
					CurrentNodeKind: string(tv.WorkflowNodeKindOnboarding),
					NextNodePath:    "planning",
					ApprovalBlocked: &falseValue,
				},
				{
					ToolName:        "centian.task_complete_onboarding",
					Phase:           "planning",
					CurrentNodeKind: string(tv.WorkflowNodeKindPlanning),
					NextNodePath:    "waiting_for_approval.review_plan",
					ApprovalBlocked: &falseValue,
				},
				{
					ToolName:        "centian.task_complete_planning",
					Phase:           "waiting_for_approval.review_plan",
					CurrentNodeKind: string(tv.WorkflowNodeKindWaitingForApproval),
					NextNodePath:    "execution.establish_failing_baseline",
					ApprovalBlocked: &trueValue,
				},
			},
			ExpectedTaskEventTypes: []tv.TaskEventType{
				tv.TaskEventTypeRegistered,
				tv.TaskEventTypeOnboardingCompleted,
				tv.TaskEventTypePlanningCompleted,
				tv.TaskEventTypeApprovalWaitEntered,
			},
			ExpectBlockedToolPrefix: "shell___",
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

	if scenario.RequirePytestAndLint {
		assertComposeCommandPasses(t, demoDir, scenario.PytestCommand)
		assertComposeCommandPasses(t, demoDir, "cd /workspace/project && python -m ruff check .")
	}

	for idx := range scenario.AdditionalVerifications {
		verification := scenario.AdditionalVerifications[idx]
		output := assertComposeCommandPasses(t, demoDir, verification.Command)
		if verification.ExpectContains != "" && !strings.Contains(output, verification.ExpectContains) {
			t.Fatalf("verification output %q does not contain %q", output, verification.ExpectContains)
		}
	}

	if scenario.RequireProjectAssertions {
		assertFileContains(t, filepath.Join(projectDir, scenario.ProjectFile), scenario.ExpectProjectContains)
		assertFileContains(t, filepath.Join(projectDir, scenario.TestFile), scenario.ExpectTestContains)
	}

	assertArtifactsExist(t, artifactsDir, "final_message.txt", "codex-events.jsonl", "codex.stderr.log")

	logDir := filepath.Join(artifactsDir, "logs")
	entries := readRequestLogEntries(t, logDir)
	assertStructuredToolFlow(t, entries, scenario)
	assertPersistedEventStore(t, logDir, entries, scenario)
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

func assertStructuredToolFlow(t *testing.T, entries []common.LogEntry, scenario *taskVerificationScenario) {
	t.Helper()

	toolNames := collectToolCallNames(entries)
	assertSubsequence(t, toolNames, scenario.ExpectedToolSubsequence)
	assert.Assert(t, containsToolPrefix(toolNames, "filesystem___"))
	if scenario.ExpectBlockedToolPrefix != "" {
		assert.Assert(t, containsToolPrefix(toolNames, scenario.ExpectBlockedToolPrefix))
	} else {
		assert.Assert(t, containsToolPrefix(toolNames, "shell___"))
	}
	assert.Assert(t, !slices.Contains(toolNames, "centian.task_fail"))

	assertTaskToolResults(t, entries, scenario.ExpectedTaskToolResults)
	if scenario.ExpectBlockedToolPrefix != "" {
		assertBlockedProxiedTool(t, entries, scenario.ExpectBlockedToolPrefix)
	}
}

func assertPersistedEventStore(t *testing.T, logDir string, entries []common.LogEntry, scenario *taskVerificationScenario) {
	t.Helper()

	dbPath := filepath.Join(logDir, "events.sqlite")
	_, err := os.Stat(dbPath)
	assert.NilError(t, err)

	store, err := persistence.NewSQLiteStore(dbPath)
	assert.NilError(t, err)
	defer func() {
		assert.NilError(t, store.Close())
	}()

	taskEvents := store.TaskEvents()
	actionEvents := store.ActionEvents()
	contexts := store.ActionEventTaskContexts()
	taskRunID := taskRunIDFromEntries(t, entries, scenario.ExpectedTaskToolResults)

	assert.Assert(t, len(taskEvents) > 0)
	assert.Assert(t, len(actionEvents) > 0)
	assert.Assert(t, len(contexts) > 0)

	filteredTaskEvents := filterTaskEventsByRunID(taskEvents, taskRunID)
	filteredContexts := filterActionEventTaskContextsByRunID(contexts, taskRunID)

	assert.Assert(t, len(filteredTaskEvents) > 0)
	assert.Assert(t, len(filteredContexts) > 0)
	assertTaskEventSubsequence(t, filteredTaskEvents, scenario.ExpectedTaskEventTypes)

	actionIDs := make(map[string]struct{}, len(actionEvents))
	for idx := range actionEvents {
		actionIDs[actionEvents[idx].ID] = struct{}{}
	}
	for idx := range filteredTaskEvents {
		if filteredTaskEvents[idx].RelatedActionEventID == "" {
			continue
		}
		_, exists := actionIDs[filteredTaskEvents[idx].RelatedActionEventID]
		assert.Assert(t, exists)
	}
	for idx := range filteredContexts {
		_, exists := actionIDs[filteredContexts[idx].ActionEventID]
		assert.Assert(t, exists)
	}

	if scenario.ExpectBlockedToolPrefix != "" {
		foundBlocked := false
		for idx := range actionEvents {
			event := actionEvents[idx]
			if strings.HasPrefix(event.OriginalToolName, scenario.ExpectBlockedToolPrefix) && event.IsError {
				foundBlocked = true
				break
			}
		}
		assert.Assert(t, foundBlocked)
	}
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
		if entries[idx].ToolCall == nil {
			continue
		}
		if entries[idx].ToolCall.OriginalName != "" {
			names = append(names, entries[idx].ToolCall.OriginalName)
			continue
		}
		if entries[idx].ToolCall.Name != "" {
			names = append(names, entries[idx].ToolCall.Name)
		}
	}
	return names
}

func assertTaskToolResults(t *testing.T, entries []common.LogEntry, expectations []taskToolExpectation) {
	t.Helper()

	var taskRunID string
	for idx := range expectations {
		expectation := expectations[idx]
		entry := findToolEntry(t, entries, expectation.ToolName)
		result := decodeLoggedToolResult(t, &entry)
		structured := result.StructuredContent

		assert.Assert(t, structured != nil)
		assert.Equal(t, structured["phase"], expectation.Phase)
		assert.Equal(t, structured["currentNodeKind"], expectation.CurrentNodeKind)
		if expectation.NextNodePath != "" {
			assert.Equal(t, structured["nextNodePath"], expectation.NextNodePath)
		}
		if expectation.ApprovalBlocked != nil {
			assert.Equal(t, structured["approvalBlocked"], *expectation.ApprovalBlocked)
		}
		runID, ok := structured["taskRunId"].(string)
		assert.Assert(t, ok)
		assert.Assert(t, runID != "")
		if taskRunID == "" {
			taskRunID = runID
		} else {
			assert.Equal(t, runID, taskRunID)
		}
		allowedTools, ok := structured["allowedTools"].([]any)
		assert.Assert(t, ok)
		assert.Assert(t, len(allowedTools) > 0)
	}
}

func assertBlockedProxiedTool(t *testing.T, entries []common.LogEntry, prefix string) {
	t.Helper()

	found := false
	for idx := range entries {
		entry := entries[idx]
		if entry.ToolCall == nil {
			continue
		}
		name := entry.ToolCall.OriginalName
		if name == "" {
			name = entry.ToolCall.Name
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		result := decodeLoggedToolResult(t, &entry)
		if !result.IsError {
			continue
		}
		if result.StructuredContent["reason"] == "waiting_for_approval" {
			found = true
			break
		}
	}
	assert.Assert(t, found)
}

func assertTaskEventSubsequence(t *testing.T, events []tv.TaskEvent, expected []tv.TaskEventType) {
	t.Helper()

	index := 0
	for idx := range events {
		if index < len(expected) && events[idx].EventType == expected[index] {
			index++
		}
	}
	if index != len(expected) {
		t.Fatalf("task event sequence %v does not contain subsequence %v", collectTaskEventTypes(events), expected)
	}
}

func collectTaskEventTypes(events []tv.TaskEvent) []tv.TaskEventType {
	result := make([]tv.TaskEventType, 0, len(events))
	for idx := range events {
		result = append(result, events[idx].EventType)
	}
	return result
}

func findToolEntry(t *testing.T, entries []common.LogEntry, toolName string) common.LogEntry {
	t.Helper()

	for idx := range entries {
		entry := entries[idx]
		if entry.ToolCall == nil {
			continue
		}
		if entry.ToolCall.Name == toolName || entry.ToolCall.OriginalName == toolName {
			return entry
		}
	}
	t.Fatalf("tool log entry for %s not found", toolName)
	return common.LogEntry{}
}

func decodeLoggedToolResult(t *testing.T, entry *common.LogEntry) loggedToolResult {
	t.Helper()

	if entry == nil {
		t.Fatal("tool result missing: log entry is nil")
	}
	if entry.ToolCall == nil || len(entry.ToolCall.Result) == 0 {
		t.Fatalf("tool result missing for %s", entry.ToolCall.Name)
	}
	var result loggedToolResult
	assert.NilError(t, json.Unmarshal(entry.ToolCall.Result, &result))
	return result
}

func taskRunIDFromEntries(t *testing.T, entries []common.LogEntry, expectations []taskToolExpectation) string {
	t.Helper()

	for idx := range expectations {
		entry := findToolEntry(t, entries, expectations[idx].ToolName)
		result := decodeLoggedToolResult(t, &entry)
		runID, ok := result.StructuredContent["taskRunId"].(string)
		if ok && runID != "" {
			return runID
		}
	}
	t.Fatal("no taskRunId found in logged task tool results")
	return ""
}

func filterTaskEventsByRunID(events []tv.TaskEvent, taskRunID string) []tv.TaskEvent {
	if taskRunID == "" {
		return nil
	}
	filtered := make([]tv.TaskEvent, 0, len(events))
	for idx := range events {
		if events[idx].TaskRunID == taskRunID {
			filtered = append(filtered, events[idx])
		}
	}
	return filtered
}

func filterActionEventTaskContextsByRunID(contexts []tv.ActionEventTaskContext, taskRunID string) []tv.ActionEventTaskContext {
	if taskRunID == "" {
		return nil
	}
	filtered := make([]tv.ActionEventTaskContext, 0, len(contexts))
	for idx := range contexts {
		if contexts[idx].TaskRunID == taskRunID {
			filtered = append(filtered, contexts[idx])
		}
	}
	return filtered
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
