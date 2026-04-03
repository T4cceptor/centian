package taskverification

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/persistence"
	tv "github.com/T4cceptor/centian/internal/taskverification"
	"gopkg.in/yaml.v3"
)

const (
	runBlackboxEnv      = "CENTIAN_RUN_TASKVERIFICATION_BLACKBOX"
	agentsEnv           = "CENTIAN_TASKVERIFICATION_AGENTS"
	agentTimeoutEnv     = "CENTIAN_TASKVERIFICATION_AGENT_TIMEOUT"
	centianBinaryEnv    = "CENTIAN_TASKVERIFICATION_BINARY"
	defaultAgentTimeout = 15 * time.Minute
)

type blackboxHarness struct {
	t              *testing.T
	assetsDir      string
	artifactsDir   string
	projectDir     string
	templatesDir   string
	configPath     string
	baseURL        string
	mcpURL         string
	logDir         string
	internalLog    string
	eventStorePath string
	requestLogDir  string
}

type agentAdapter interface {
	Name() string
	IsAvailable() error
	PrepareConfig(*testing.T, *blackboxHarness) (agentLaunchConfig, error)
	Run(context.Context, *testing.T, *blackboxHarness, *agentLaunchConfig, string) error
}

type agentLaunchConfig struct {
	WorkDir      string
	StdoutPath   string
	StderrPath   string
	FinalPath    string
	Env          []string
	Command      []string
	ConfigPath   string
	ArtifactRoot string
}

type requestLogEntry struct {
	ToolCall *struct {
		Name         string `json:"name"`
		OriginalName string `json:"original_name"`
		IsError      bool   `json:"is_error"`
	} `json:"tool_call"`
}

type promptFile struct {
	Prompt string `yaml:"prompt"`
}

func TestTaskVerificationBlackBox(t *testing.T) {
	if os.Getenv(runBlackboxEnv) != "1" {
		t.Skipf("set %s=1 to run taskverification black-box tests", runBlackboxEnv)
	}

	adapters := selectedAgentAdapters()
	if len(adapters) == 0 {
		t.Fatalf("no agents selected via %s", agentsEnv)
	}
	for _, adapter := range adapters {
		t.Run(adapter.Name(), func(t *testing.T) {
			harness := newBlackboxHarness(t, adapter.Name())
			prompt := loadUserPrompt(t, harness.assetsDir)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			centianCmd := startCentian(t, ctx, harness)
			defer stopCommand(t, centianCmd)

			waitForCentian(t, harness)

			if err := adapter.IsAvailable(); err != nil {
				t.Fatalf("%s adapter is not available: %v", adapter.Name(), err)
			}

			agentTimeout := loadAgentTimeout(t)
			runCtx, runCancel := context.WithTimeout(context.Background(), agentTimeout)
			defer runCancel()

			cfg, err := adapter.PrepareConfig(t, harness)
			if err != nil {
				t.Fatalf("failed to prepare %s config: %v", adapter.Name(), err)
			}

			err = adapter.Run(runCtx, t, harness, &cfg, prompt)
			runs := fetchTaskRuns(t, harness)
			if len(runs) == 0 {
				t.Fatalf("%s created no task run; run error: %v; artifacts: %s", adapter.Name(), err, cfg.ArtifactRoot)
			}

			latest := &runs[0]
			events := fetchTaskRunEvents(t, harness, latest.RunID)
			requests := loadRequestLogs(t, harness)

			assertStrictSuccess(t, adapter.Name(), harness, latest, events, requests)
			if err != nil {
				t.Fatalf("%s run failed despite successful task assertions: %v; artifacts: %s", adapter.Name(), err, cfg.ArtifactRoot)
			}
		})
	}
}

func newBlackboxHarness(t *testing.T, agentName string) *blackboxHarness {
	t.Helper()

	assetsDir := assetDir(t)
	artifactsDir, cleanup := newArtifactDir(t, assetsDir, agentName)
	t.Cleanup(cleanup)

	projectDir := filepath.Join(artifactsDir, "project")
	templatesDir := filepath.Join(artifactsDir, "templates")
	logDir := filepath.Join(artifactsDir, "logs")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatalf("failed to create templates dir: %v", err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("failed to create logs dir: %v", err)
	}

	writeEmptyProject(t, projectDir)
	writeLocalTemplates(t, assetsDir, templatesDir)

	port := allocateFreePort(t)
	baseURL := "http://127.0.0.1:" + port
	configPath := filepath.Join(artifactsDir, "centian.config.json")
	eventStorePath := filepath.Join(logDir, "events.sqlite")
	internalLog := filepath.Join(artifactsDir, "centian_internal.log")
	writeCentianConfig(t, assetsDir, configPath, templatesDir, projectDir, internalLog, eventStorePath, port)

	return &blackboxHarness{
		t:              t,
		assetsDir:      assetsDir,
		artifactsDir:   artifactsDir,
		projectDir:     projectDir,
		templatesDir:   templatesDir,
		configPath:     configPath,
		baseURL:        baseURL,
		mcpURL:         baseURL + "/mcp/taskverification",
		logDir:         logDir,
		internalLog:    internalLog,
		eventStorePath: eventStorePath,
		requestLogDir:  logDir,
	}
}

func assetDir(t *testing.T) string {
	t.Helper()

	path, err := filepath.Abs(filepath.Dir("tests/integrationtests/taskverification/blackbox_test.go"))
	if err == nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return path
		}
	}

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to resolve working directory: %v", err)
	}
	for {
		candidate := filepath.Join(dir, "tests", "integrationtests", "taskverification")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("failed to locate taskverification test asset directory from %s", dir)
		}
		dir = parent
	}
}

func newArtifactDir(t *testing.T, assetsDir, agentName string) (string, func()) {
	t.Helper()

	root := filepath.Join(assetsDir, ".tmp")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("failed to create taskverification tmp dir: %v", err)
	}
	baseName := time.Now().Format("20060102150405") + "_run_" + sanitizeName(agentName)
	dir := filepath.Join(root, baseName)
	for i := 1; ; i++ {
		err := os.Mkdir(dir, 0o755)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			t.Fatalf("failed to create artifact dir: %v", err)
		}
		dir = filepath.Join(root, fmt.Sprintf("%s_%02d", baseName, i))
	}
	return dir, func() {
		t.Logf("preserving taskverification black-box artifacts at %s", dir)
	}
}

func sanitizeName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "agent"
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	result := strings.Trim(b.String(), "_")
	if result == "" {
		return "agent"
	}
	return result
}

func writeEmptyProject(t *testing.T, projectDir string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(projectDir, ".gitkeep"), []byte(""), 0o644); err != nil {
		t.Fatalf("failed to initialize empty project dir: %v", err)
	}
}

func writeLocalTemplates(t *testing.T, assetsDir, templatesDir string) {
	t.Helper()

	data := []byte(loadAsset(t, assetsDir, "guided_tdd_workflow.yaml"))
	var template tv.Template
	if err := yaml.Unmarshal(data, &template); err != nil {
		t.Fatalf("failed to parse guided_tdd_workflow.yaml fixture: %v", err)
	}
	if err := template.Validate(); err != nil {
		t.Fatalf("guided_tdd_workflow.yaml fixture is invalid: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templatesDir, "guided_tdd_workflow.yaml"), data, 0o644); err != nil {
		t.Fatalf("failed to write template fixture: %v", err)
	}
}

func writeCentianConfig(
	t *testing.T,
	assetsDir string,
	configPath, templatesDir, projectDir, internalLog, eventStorePath, port string,
) {
	t.Helper()

	configTemplate := loadAsset(t, assetsDir, "centian_config.json")
	replacer := strings.NewReplacer(
		"__PORT__", port,
		"__TEMPLATES_DIR__", templatesDir,
		"__PROJECT_DIR__", projectDir,
		"__INTERNAL_LOG__", internalLog,
		"__EVENT_STORE_PATH__", eventStorePath,
	)
	resolved := replacer.Replace(configTemplate)
	var payload any
	if err := json.Unmarshal([]byte(resolved), &payload); err != nil {
		t.Fatalf("failed to parse resolved centian config template: %v", err)
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal resolved centian config: %v", err)
	}
	if err := os.WriteFile(configPath, encoded, 0o644); err != nil {
		t.Fatalf("failed to write centian config: %v", err)
	}
}

func loadUserPrompt(t *testing.T, assetsDir string) string {
	t.Helper()

	content := loadAsset(t, assetsDir, "prompt.yaml")
	var file promptFile
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		t.Fatalf("failed to parse prompt.yaml: %v", err)
	}
	if strings.TrimSpace(file.Prompt) == "" {
		t.Fatalf("prompt.yaml does not contain a prompt")
	}
	return strings.TrimSpace(file.Prompt)
}

func selectedAgentAdapters() []agentAdapter {
	selected := strings.TrimSpace(os.Getenv(agentsEnv))
	if selected == "" {
		selected = "codex,claude"
	}

	available := map[string]agentAdapter{
		"codex":  codexAdapter{},
		"claude": claudeAdapter{},
	}
	result := make([]agentAdapter, 0, 2)
	for _, raw := range strings.Split(selected, ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		adapter, ok := available[name]
		if !ok {
			continue
		}
		result = append(result, adapter)
	}
	return result
}

type codexAdapter struct{}

func (codexAdapter) Name() string { return "codex" }

func (codexAdapter) IsAvailable() error {
	_, err := exec.LookPath("codex")
	return err
}

func (codexAdapter) PrepareConfig(t *testing.T, h *blackboxHarness) (agentLaunchConfig, error) {
	t.Helper()

	artifactRoot := filepath.Join(h.artifactsDir, "codex")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		return agentLaunchConfig{}, err
	}
	homeDir := filepath.Join(artifactRoot, "codex-home")
	workDir := filepath.Join(artifactRoot, "workdir")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return agentLaunchConfig{}, err
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return agentLaunchConfig{}, err
	}

	if err := copyIfExists(filepath.Join(os.Getenv("HOME"), ".codex", "auth.json"), filepath.Join(homeDir, "auth.json")); err != nil {
		return agentLaunchConfig{}, err
	}

	configPath := filepath.Join(homeDir, "config.toml")
	content := loadAsset(t, h.assetsDir, "codex_config.toml")
	modelBlock := ""
	if model := strings.TrimSpace(os.Getenv("CODEX_MODEL")); model != "" {
		modelBlock = `model = ` + strconv.Quote(model)
	}
	content = strings.NewReplacer(
		"__MODEL_BLOCK__", modelBlock,
		"__MCP_URL__", h.mcpURL,
		"__WORK_DIR__", workDir,
	).Replace(content)
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		return agentLaunchConfig{}, err
	}

	return agentLaunchConfig{
		WorkDir:      workDir,
		StdoutPath:   filepath.Join(artifactRoot, "stdout.jsonl"),
		StderrPath:   filepath.Join(artifactRoot, "stderr.log"),
		FinalPath:    filepath.Join(artifactRoot, "final.txt"),
		ConfigPath:   configPath,
		ArtifactRoot: artifactRoot,
		Env: append(os.Environ(),
			"CODEX_HOME="+homeDir,
		),
		Command: []string{
			"codex",
			"exec",
			"--skip-git-repo-check",
			"--json",
			// "--full-auto",
			// "--dangerously-bypass-approvals-and-sandbox",
			"-C", workDir,
			"-o", filepath.Join(artifactRoot, "final.txt"),
			"-",
		},
	}, nil
}

func (codexAdapter) Run(ctx context.Context, t *testing.T, _ *blackboxHarness, cfg *agentLaunchConfig, prompt string) error {
	t.Helper()
	return runAgentCommand(ctx, cfg, prompt)
}

type claudeAdapter struct{}

func (claudeAdapter) Name() string { return "claude" }

func (claudeAdapter) IsAvailable() error {
	_, err := exec.LookPath("claude")
	return err
}

func (claudeAdapter) PrepareConfig(t *testing.T, h *blackboxHarness) (agentLaunchConfig, error) {
	t.Helper()

	artifactRoot := filepath.Join(h.artifactsDir, "claude")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		return agentLaunchConfig{}, err
	}
	workDir := filepath.Join(artifactRoot, "workdir")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return agentLaunchConfig{}, err
	}
	configPath := filepath.Join(workDir, ".mcp.json")
	config := strings.ReplaceAll(loadAsset(t, h.assetsDir, "claude_mcp_config.json"), "__MCP_URL__", h.mcpURL)
	var payload any
	if err := json.Unmarshal([]byte(config), &payload); err != nil {
		return agentLaunchConfig{}, err
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return agentLaunchConfig{}, err
	}
	if err := os.WriteFile(configPath, encoded, 0o644); err != nil {
		return agentLaunchConfig{}, err
	}

	command := []string{
		"claude",
		"-p",
		"--output-format", "json",
		"--no-session-persistence",
		"--permission-mode", "bypassPermissions",
		"--mcp-config", configPath,
		"--strict-mcp-config",
	}
	if model := strings.TrimSpace(os.Getenv("CLAUDE_MODEL")); model != "" {
		command = append(command, "--model", model)
	}

	return agentLaunchConfig{
		WorkDir:      workDir,
		StdoutPath:   filepath.Join(artifactRoot, "stdout.json"),
		StderrPath:   filepath.Join(artifactRoot, "stderr.log"),
		FinalPath:    filepath.Join(artifactRoot, "final.txt"),
		ConfigPath:   configPath,
		ArtifactRoot: artifactRoot,
		Env:          os.Environ(),
		Command:      command,
	}, nil
}

func (claudeAdapter) Run(ctx context.Context, t *testing.T, _ *blackboxHarness, cfg *agentLaunchConfig, prompt string) error {
	t.Helper()
	return runAgentCommand(ctx, cfg, prompt)
}

func runAgentCommand(ctx context.Context, cfg *agentLaunchConfig, prompt string) error {
	stdoutFile, err := os.Create(cfg.StdoutPath)
	if err != nil {
		return err
	}
	defer stdoutFile.Close()

	stderrFile, err := os.Create(cfg.StderrPath)
	if err != nil {
		return err
	}
	defer stderrFile.Close()

	cmd := exec.CommandContext(ctx, cfg.Command[0], cfg.Command[1:]...)
	cmd.Dir = cfg.WorkDir
	cmd.Env = cfg.Env
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	cmd.Stdin = strings.NewReader(prompt)

	err = cmd.Run()
	if finalBytes, readErr := os.ReadFile(cfg.StdoutPath); readErr == nil {
		_ = os.WriteFile(cfg.FinalPath, finalBytes, 0o644)
	}
	if ctx.Err() != nil {
		return fmt.Errorf("agent timed out: %w", ctx.Err())
	}
	return err
}

func startCentian(t *testing.T, ctx context.Context, h *blackboxHarness) *exec.Cmd {
	t.Helper()

	binary := strings.TrimSpace(os.Getenv(centianBinaryEnv))
	if binary == "" {
		binary = "centian"
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		t.Fatalf("failed to resolve centian binary %q: %v", binary, err)
	}

	stdoutPath := filepath.Join(h.artifactsDir, "centian.stdout.log")
	stderrPath := filepath.Join(h.artifactsDir, "centian.stderr.log")
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		t.Fatalf("failed to create centian stdout log: %v", err)
	}
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		t.Fatalf("failed to create centian stderr log: %v", err)
	}
	t.Cleanup(func() {
		_ = stdoutFile.Close()
		_ = stderrFile.Close()
	})

	cmd := exec.CommandContext(ctx, resolved, "start", "--config-path", h.configPath)
	cmd.Dir = h.projectDir
	cmd.Env = append(os.Environ(), "CENTIAN_LOG_DIR="+h.logDir)
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start centian: %v", err)
	}
	return cmd
}

func stopCommand(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

func waitForCentian(t *testing.T, h *blackboxHarness) {
	t.Helper()

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(45 * time.Second)
	apiURL := h.baseURL + "/api/task-runs"

	for time.Now().Before(deadline) {
		mcpReady := isEndpointReachable(client, h.mcpURL)
		apiReady := isJSONEndpointReady(client, apiURL)
		if mcpReady && apiReady {
			fmt.Printf("Task UI: %s/ui/tasks\n", h.baseURL)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("centian did not become ready in time; artifacts: %s", h.artifactsDir)
}

func isEndpointReachable(client *http.Client, endpoint string) bool {
	resp, err := client.Get(endpoint)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

func isJSONEndpointReady(client *http.Client, endpoint string) bool {
	resp, err := client.Get(endpoint)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func fetchTaskRuns(t *testing.T, h *blackboxHarness) []persistence.TaskRunSummary {
	t.Helper()

	resp, err := http.Get(h.baseURL + "/api/task-runs")
	if err != nil {
		t.Fatalf("failed to fetch task runs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("task runs endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var runs []persistence.TaskRunSummary
	if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
		t.Fatalf("failed to decode task runs: %v", err)
	}
	return runs
}

func fetchTaskRunEvents(t *testing.T, h *blackboxHarness, runID string) []persistence.TaskRunEvent {
	t.Helper()

	resp, err := http.Get(h.baseURL + "/api/task-runs/" + runID + "/events")
	if err != nil {
		t.Fatalf("failed to fetch task run events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("task run events endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var events []persistence.TaskRunEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatalf("failed to decode task run events: %v", err)
	}
	return events
}

func loadRequestLogs(t *testing.T, h *blackboxHarness) []requestLogEntry {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(h.requestLogDir, "requests_*.jsonl"))
	if err != nil {
		t.Fatalf("failed to glob request logs: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no request logs found in %s", h.requestLogDir)
	}
	slices.Sort(matches)
	path := matches[len(matches)-1]

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open request log %s: %v", path, err)
	}
	defer file.Close()

	result := make([]requestLogEntry, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if strings.TrimSpace(string(line)) == "" {
			continue
		}
		var entry requestLogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("failed to parse request log %s: %v", path, err)
		}
		result = append(result, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("failed to scan request log %s: %v", path, err)
	}
	return result
}

func assertStrictSuccess(
	t *testing.T,
	agentName string,
	h *blackboxHarness,
	latest *persistence.TaskRunSummary,
	events []persistence.TaskRunEvent,
	requests []requestLogEntry,
) {
	t.Helper()

	if latest.Status != string(tv.TaskStatusCompleted) {
		t.Fatalf("%s latest task run did not complete: status=%s phase=%s artifacts=%s", agentName, latest.Status, latest.CurrentPhase, h.artifactsDir)
	}

	taskToolNames := make([]string, 0)
	for _, entry := range requests {
		if entry.ToolCall == nil {
			continue
		}
		taskToolNames = append(taskToolNames, entry.ToolCall.Name)
	}

	if !containsSubsequence(taskToolNames, []string{
		"centian.task_list_templates",
		"centian.task_register",
		"centian.task_complete_onboarding",
		"centian.task_complete_planning",
		"centian.task_start_step",
		"centian.task_complete_step",
	}) {
		t.Fatalf("%s did not produce the expected task tool call order: %v", agentName, taskToolNames)
	}

	taskToolCalls := 0
	for _, name := range taskToolNames {
		if strings.HasPrefix(name, "centian.task_") {
			taskToolCalls++
		}
		if name == "centian.task_fail" {
			t.Fatalf("%s used centian.task_fail unexpectedly", agentName)
		}
	}
	if taskToolCalls < 5 {
		t.Fatalf("%s produced too few centian task tool calls: %d", agentName, taskToolCalls)
	}

	eventTypes := make([]string, 0)
	for idx := range events {
		event := &events[idx]
		if event.Source != persistence.TaskRunEventSourceTask || event.EventType == "" {
			continue
		}
		if event.EventType == string(tv.TaskEventTypeFailed) || event.EventType == string(tv.TaskEventTypeTimedOut) {
			t.Fatalf("%s task run contained terminal failure event %s", agentName, event.EventType)
		}
		eventTypes = append(eventTypes, event.EventType)
	}
	if !containsSubsequence(eventTypes, []string{
		string(tv.TaskEventTypeRegistered),
		string(tv.TaskEventTypeOnboardingCompleted),
		string(tv.TaskEventTypePlanningCompleted),
		string(tv.TaskEventTypeStepStarted),
		string(tv.TaskEventTypeStepCompleted),
	}) {
		t.Fatalf("%s task run lifecycle events are incomplete: %v", agentName, eventTypes)
	}

	scorePath := filepath.Join(h.projectDir, "scoreParentheses.js")
	if _, err := os.Stat(scorePath); err != nil {
		t.Fatalf("%s did not create %s: %v", agentName, scorePath, err)
	}
	testPath := filepath.Join(h.projectDir, "scoreParentheses.test.js")
	if _, err := os.Stat(testPath); err != nil {
		t.Fatalf("%s did not create %s: %v", agentName, testPath, err)
	}

	runProjectCommand(t, h.projectDir, "node", "--test", "scoreParentheses.test.js")
	runProjectCommand(t, h.projectDir, "node", "--check", "scoreParentheses.js")
	runProjectCommand(t, h.projectDir, "node", "--check", "scoreParentheses.test.js")
}

func runProjectCommand(t *testing.T, dir, name string, args ...string) {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %s %s failed in %s: %v\n%s", name, strings.Join(args, " "), dir, err, strings.TrimSpace(string(output)))
	}
}

func containsSubsequence(haystack, needle []string) bool {
	if len(needle) == 0 {
		return true
	}
	index := 0
	for _, item := range haystack {
		if item == needle[index] {
			index++
			if index == len(needle) {
				return true
			}
		}
	}
	return false
}

func loadAgentTimeout(t *testing.T) time.Duration {
	t.Helper()

	raw := strings.TrimSpace(os.Getenv(agentTimeoutEnv))
	if raw == "" {
		return defaultAgentTimeout
	}
	duration, err := time.ParseDuration(raw)
	if err != nil {
		t.Fatalf("failed to parse %s=%q: %v", agentTimeoutEnv, raw, err)
	}
	return duration
}

func loadAsset(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(data)
}

func allocateFreePort(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate free port: %v", err)
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("failed to resolve allocated tcp port")
	}
	return fmt.Sprintf("%d", addr.Port)
}

func copyIfExists(src, dst string) error {
	data, err := os.ReadFile(src)
	switch {
	case err == nil:
		return os.WriteFile(dst, data, 0o600)
	case errors.Is(err, os.ErrNotExist):
		return nil
	default:
		return err
	}
}
