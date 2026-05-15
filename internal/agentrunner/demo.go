package agentrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/common"
)

const (
	darwinOS = "darwin"

	// AgentClaude is the supported public agent identifier for the Claude CLI.
	AgentClaude = common.AgentClaude
	// AgentGemini is the supported public agent identifier for the Gemini CLI.
	AgentGemini = common.AgentGemini
	// AgentCodex is the supported public agent identifier for the Codex CLI.
	AgentCodex = common.AgentCodex
	// AgentCodexOllama is the supported public agent identifier for Codex OSS mode via Ollama.
	AgentCodexOllama = common.AgentCodexOllama
	// DefaultClaudeModel is the default Claude model alias for demo runs.
	DefaultClaudeModel = common.ModelClaudeSonnet
	// DefaultGeminiModel is the default Gemini model alias for demo runs.
	DefaultGeminiModel = common.ModelGemini25Flash
	// DefaultCodexModel is the default Codex model alias for demo runs (empty uses Codex default).
	DefaultCodexModel = ""
	// DefaultAgentTimeout is the default maximum runtime for a demo agent invocation.
	DefaultAgentTimeout = 5 * time.Minute
	// TaskTemplateFile is the default task template file name for the task.
	TaskTemplateFile = "guided_tdd_workflow.yaml"
)

// allocateFreePortFunc is swappable in tests that need deterministic ports.
var allocateFreePortFunc = common.AllocateFreePort

// processExistsFunc is swappable in tests that simulate stale or live demo PID files.
var processExistsFunc = common.ProcessExists

// disposableDemoPaths are rebuilt on each demo run and are safe to remove when reusing a demo root.
var disposableDemoPaths = []string{
	"workspace",
	"templates",
	"config.json",
	"prompt.yaml",
	"prompt.md",
	"demo_scenario.json",
	"centian.pid",
	"claude_mcp_config.json",
	"codex-home",
	"codex_output.txt",
}

// allowedDemoRootEntries defines the expected top-level files in a reusable demo root.
var allowedDemoRootEntries = map[string]struct{}{
	"workspace":              {},
	"templates":              {},
	"logs":                   {},
	"config.json":            {},
	"prompt.yaml":            {},
	"prompt.md":              {},
	"demo_scenario.json":     {},
	"agent.stdout.log":       {},
	"agent.stderr.log":       {},
	"centian.pid":            {},
	"claude_mcp_config.json": {},
	"codex-home":             {},
	"codex_output.txt":       {},
	".DS_Store":              {},
}

// DemoOptions configures a single demo run.
type DemoOptions struct {
	Execution         AgentExecutionOptions
	RootPath          string
	CentianBinaryPath string
	ScenarioFilePath  string
	Timeout           time.Duration
	OpenBrowser       bool
	Stdout            io.Writer
	Stderr            io.Writer
}

// DemoResult describes the generated demo workspace and running Centian instance.
type DemoResult struct {
	RootPath      string
	WorkspacePath string
	ConfigPath    string
	PromptPath    string
	AgentStdout   string
	AgentStderr   string
	UIPublicURL   string
	MCPURL        string
	PID           int
	StopHint      string
}

// DemoRunner provisions and launches a self-contained Centian demo workspace.
type DemoRunner struct{}

// agentAdapter abstracts the per-agent config, environment, and command construction.
type agentAdapter interface {
	name() string
	isAvailable() error
	writeConfig(*demoLayout) error
	cleanup(*demoLayout) error
	command(*demoLayout, string) ([]string, error)
	env(*demoLayout) []string
}

// demoLayout contains all generated paths and URLs for one demo workspace.
type demoLayout struct {
	RootPath        string
	WorkspacePath   string
	TemplatesPath   string
	LogsPath        string
	ConfigPath      string
	PromptPath      string
	ScenarioPath    string
	AgentStdoutPath string
	AgentStderrPath string
	InternalLogPath string
	EventStorePath  string
	PIDPath         string
	ClaudeConfig    string
	GeminiConfig    string
	CodexConfig     string
	BaseURL         string
	MCPURL          string
	Port            string
}

// RunDemo creates the demo workspace, starts Centian, launches the selected
// agent, and leaves Centian running after the agent exits.
func (DemoRunner) RunDemo(ctx context.Context, opts *DemoOptions) (*DemoResult, error) {
	options, err := normalizeOptions(opts)
	if err != nil {
		return nil, err
	}
	layout, err := prepareLayout(options)
	if err != nil {
		return nil, err
	}

	adapter, err := selectAdapter(options)
	if err != nil {
		return nil, err
	}
	if err := adapter.isAvailable(); err != nil {
		return nil, fmt.Errorf("%s is not available: %w", adapter.name(), err)
	}
	if err := renderAssets(layout); err != nil {
		return nil, err
	}
	if err := adapter.writeConfig(layout); err != nil {
		return nil, err
	}
	defer func() {
		if err := adapter.cleanup(layout); err != nil && options.Stderr != nil {
			_, _ = fmt.Fprintf(options.Stderr, "warning: cleanup %s demo artifacts: %v\n", adapter.name(), err)
		}
	}()

	centianCmd, errCh, err := startCentianProcess(layout, options)
	if err != nil {
		return nil, err
	}
	if err := common.WritePIDFile(layout.PIDPath, centianCmd.Process.Pid); err != nil {
		_ = centianCmd.Process.Kill()
		return nil, err
	}

	if err := waitForCentian(layout, errCh); err != nil {
		_ = centianCmd.Process.Kill()
		return nil, err
	}

	result := demoResultFromLayout(layout, centianCmd.Process.Pid)

	if options.OpenBrowser && runtime.GOOS == darwinOS {
		//nolint:gosec // UI URL is generated locally from the bound loopback port.
		_ = exec.Command("open", result.UIPublicURL).Start()
	}
	printDemoStatus(options.Stdout, result)

	if err := runAgent(ctx, adapter, layout, options); err != nil {
		return result, err
	}
	return result, nil
}

// normalizeOptions fills omitted demo options with platform and agent defaults.
func normalizeOptions(opts *DemoOptions) (*DemoOptions, error) {
	if opts == nil {
		return nil, fmt.Errorf("demo options are required")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultAgentTimeout
	}
	execution, err := NormalizeExecutionOptions(opts.Execution)
	if err != nil {
		return nil, err
	}
	opts.Execution = execution
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if !opts.OpenBrowser && runtime.GOOS == darwinOS {
		opts.OpenBrowser = true
	}
	return opts, nil
}

// normalizeSyntheticOptions fills omitted demo options for the synthetic flow.
func normalizeSyntheticOptions(opts *DemoOptions) (*DemoOptions, error) {
	if opts == nil {
		return nil, fmt.Errorf("demo options are required")
	}
	if strings.TrimSpace(opts.RootPath) == "" {
		return nil, fmt.Errorf("demo root path is required")
	}
	if strings.TrimSpace(opts.CentianBinaryPath) == "" {
		return nil, fmt.Errorf("centian binary path is required")
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if !opts.OpenBrowser && runtime.GOOS == darwinOS {
		opts.OpenBrowser = true
	}
	opts.ScenarioFilePath = strings.TrimSpace(opts.ScenarioFilePath)
	return opts, nil
}

func demoResultFromLayout(layout *demoLayout, pid int) *DemoResult {
	return &DemoResult{
		RootPath:      layout.RootPath,
		WorkspacePath: layout.WorkspacePath,
		ConfigPath:    layout.ConfigPath,
		PromptPath:    layout.PromptPath,
		AgentStdout:   layout.AgentStdoutPath,
		AgentStderr:   layout.AgentStderrPath,
		UIPublicURL:   layout.BaseURL + "/ui/tasks",
		MCPURL:        layout.MCPURL,
		PID:           pid,
		StopHint:      fmt.Sprintf("kill $(cat %s)", common.ShellQuote(layout.PIDPath)),
	}
}

// prepareLayout creates the demo directory layout and allocates a loopback port.
func prepareLayout(opts *DemoOptions) (*demoLayout, error) {
	root := strings.TrimSpace(opts.RootPath)
	if root == "" {
		return nil, fmt.Errorf("demo root path is required")
	}
	if err := prepareDemoRoot(root); err != nil {
		return nil, err
	}
	layout := &demoLayout{
		RootPath:        root,
		WorkspacePath:   filepath.Join(root, "workspace"),
		TemplatesPath:   filepath.Join(root, "templates"),
		LogsPath:        filepath.Join(root, "logs"),
		ConfigPath:      filepath.Join(root, "config.json"),
		PromptPath:      filepath.Join(root, "prompt.yaml"),
		ScenarioPath:    filepath.Join(root, "demo_scenario.json"),
		AgentStdoutPath: filepath.Join(root, "agent.stdout.log"),
		AgentStderrPath: filepath.Join(root, "agent.stderr.log"),
		InternalLogPath: filepath.Join(root, "logs", "internal.log"),
		EventStorePath:  filepath.Join(root, "logs", "events.sqlite"),
		PIDPath:         filepath.Join(root, "centian.pid"),
		ClaudeConfig:    filepath.Join(root, "claude_mcp_config.json"),
		GeminiConfig:    filepath.Join(root, "workspace", ".gemini", "settings.json"),
		CodexConfig:     filepath.Join(root, "codex-home", "config.toml"),
	}
	for _, dir := range []string{layout.RootPath, layout.WorkspacePath, layout.TemplatesPath, layout.LogsPath} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	port, err := allocateFreePortFunc()
	if err != nil {
		return nil, err
	}
	layout.Port = port
	layout.BaseURL = "http://127.0.0.1:" + port
	layout.MCPURL = layout.BaseURL + "/mcp/taskverification"
	return layout, nil
}

// RunSyntheticDemo creates the demo workspace, starts Centian, and replays a
// JSON-defined synthetic timeline without launching an external agent.
func (DemoRunner) RunSyntheticDemo(ctx context.Context, opts *DemoOptions) (*DemoResult, error) {
	options, err := normalizeSyntheticOptions(opts)
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintln(options.Stdout, "Starting synthetic Centian demo...")
	layout, err := prepareLayout(options)
	if err != nil {
		return nil, err
	}
	if err := renderAssets(layout); err != nil {
		return nil, err
	}
	scenario, scenarioBytes, err := loadSyntheticDemoScenario(options.ScenarioFilePath)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(layout.ScenarioPath, scenarioBytes, 0o600); err != nil {
		return nil, fmt.Errorf("write demo scenario: %w", err)
	}

	centianCmd, errCh, err := startCentianProcess(layout, options)
	if err != nil {
		return nil, err
	}
	if err := common.WritePIDFile(layout.PIDPath, centianCmd.Process.Pid); err != nil {
		_ = centianCmd.Process.Kill()
		return nil, err
	}
	if err := waitForCentian(layout, errCh); err != nil {
		_ = centianCmd.Process.Kill()
		return nil, err
	}

	result := demoResultFromLayout(layout, centianCmd.Process.Pid)
	if options.OpenBrowser && runtime.GOOS == darwinOS {
		//nolint:gosec // UI URL is generated locally from the bound loopback port.
		_ = exec.Command("open", result.UIPublicURL).Start()
	}
	printDemoStatus(options.Stdout, result)

	replayer := newSyntheticDemoReplayer()
	_, _ = fmt.Fprintf(options.Stdout, "Replaying synthetic demo timeline from %s...\n", layout.ScenarioPath)
	if err := replayer.replay(ctx, layout, scenario); err != nil {
		return result, err
	}
	return result, nil
}

// prepareDemoRoot validates or initializes the chosen demo root before assets are rendered.
func prepareDemoRoot(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat demo path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("demo path exists and is not a directory: %s", path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("read demo path: %w", err)
	}
	if len(entries) == 0 {
		return nil
	}
	if !looksLikeDemoRoot(entries) {
		return fmt.Errorf("demo path exists and is not a Centian demo root: %s", path)
	}
	if err := ensureNoLiveDemoProcess(filepath.Join(path, "centian.pid")); err != nil {
		return err
	}
	if err := removeDisposableDemoAssets(path); err != nil {
		return err
	}
	return nil
}

// looksLikeDemoRoot reports whether an existing directory only contains expected demo artifacts.
func looksLikeDemoRoot(entries []os.DirEntry) bool {
	hasMarker := false
	for _, entry := range entries {
		name := entry.Name()
		if _, ok := allowedDemoRootEntries[name]; !ok {
			return false
		}
		if name != ".DS_Store" {
			hasMarker = true
		}
	}
	return hasMarker
}

// ensureNoLiveDemoProcess prevents reusing a demo root that still has a live Centian child.
func ensureNoLiveDemoProcess(pidPath string) error {
	//nolint:gosec // pidPath is always the fixed centian.pid file inside the prepared demo root.
	data, err := os.ReadFile(pidPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read centian pid file: %w", err)
	}
	pidText := strings.TrimSpace(string(data))
	if pidText == "" {
		if err := os.Remove(pidPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove empty centian pid file: %w", err)
		}
		return nil
	}
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		return fmt.Errorf("parse centian pid %q: %w", pidText, err)
	}
	if processExistsFunc(pid) {
		return fmt.Errorf("demo path already has a running Centian server (pid %d); stop it before starting a new demo", pid)
	}
	if err := os.Remove(pidPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale centian pid file: %w", err)
	}
	return nil
}

// removeDisposableDemoAssets clears runtime-generated files while preserving durable logs.
func removeDisposableDemoAssets(root string) error {
	for _, relativePath := range disposableDemoPaths {
		target := filepath.Join(root, relativePath)
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove demo asset %s: %w", target, err)
		}
	}
	return nil
}

// renderAssets writes the default config, prompt, and template assets into the demo workspace.
func renderAssets(layout *demoLayout) error {
	configTemplate, err := asset("centian_config.json")
	if err != nil {
		return err
	}
	configResolved := strings.NewReplacer(
		"__PORT__", layout.Port,
		"__TEMPLATES_DIR__", layout.TemplatesPath,
		"__PROJECT_DIR__", layout.WorkspacePath,
		"__INTERNAL_LOG__", layout.InternalLogPath,
		"__EVENT_STORE_PATH__", layout.EventStorePath,
	).Replace(configTemplate)
	var payload any
	if err := json.Unmarshal([]byte(configResolved), &payload); err != nil {
		return fmt.Errorf("parse rendered centian config: %w", err)
	}
	configBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode rendered centian config: %w", err)
	}
	if err := os.WriteFile(layout.ConfigPath, configBytes, 0o600); err != nil {
		return fmt.Errorf("write config.json: %w", err)
	}

	prompt, err := asset("prompt.yaml")
	if err != nil {
		return err
	}
	if err := os.WriteFile(layout.PromptPath, []byte(prompt), 0o600); err != nil {
		return fmt.Errorf("write prompt.yaml: %w", err)
	}

	template, err := asset(TaskTemplateFile)
	if err != nil {
		return err
	}
	templatePath := filepath.Join(layout.TemplatesPath, TaskTemplateFile)
	if err := os.WriteFile(templatePath, []byte(template), 0o600); err != nil {
		return fmt.Errorf("write template: %w", err)
	}
	return nil
}

// selectAdapter chooses the concrete adapter for the requested demo agent.
func selectAdapter(opts *DemoOptions) (agentAdapter, error) {
	return selectAdapterForExecution(opts.Execution)
}

// startCentianProcess launches the demo-local Centian child process and returns a watcher channel.
func startCentianProcess(layout *demoLayout, opts *DemoOptions) (*exec.Cmd, <-chan error, error) {
	// This stays package-local because the demo flow needs its own process watcher
	// and layout-specific wiring around the shared readiness helpers.
	binary := strings.TrimSpace(opts.CentianBinaryPath)
	if binary == "" {
		return nil, nil, fmt.Errorf("centian binary path is required")
	}
	//nolint:gosec // The executable path comes from the current Centian binary or explicit CLI input.
	cmd := exec.Command(binary, "start", "--config-path", layout.ConfigPath)
	cmd.Dir = layout.WorkspacePath
	cmd.Env = append(os.Environ(), "CENTIAN_LOG_DIR="+layout.LogsPath)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = detachedProcessAttrs()
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start centian: %w", err)
	}
	errCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		if err != nil {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	return cmd, errCh, nil
}

// waitForCentian blocks until both the MCP and JSON API endpoints are serving successfully.
func waitForCentian(layout *demoLayout, errCh <-chan error) error {
	client := &http.Client{Timeout: 2 * time.Second}
	err := common.WaitForReadiness(client, 45*time.Second, 500*time.Millisecond, func() error {
		select {
		case err := <-errCh:
			if err == nil {
				return fmt.Errorf("centian exited before becoming ready")
			}
			return fmt.Errorf("centian exited before becoming ready: %w", err)
		default:
			return nil
		}
	}, func(client *http.Client) bool {
		return common.IsEndpointReachable(client, layout.MCPURL) &&
			common.IsJSONEndpointReady(client, layout.BaseURL+"/api/task-runs")
	})
	if errors.Is(err, common.ErrReadinessTimeout) {
		return fmt.Errorf("centian did not become ready in time; inspect %s", layout.InternalLogPath)
	}
	return err
}

// runAgent loads the saved prompt file and executes the selected agent against the demo MCP server.
func runAgent(ctx context.Context, adapter agentAdapter, layout *demoLayout, opts *DemoOptions) error {
	prompt, err := common.LoadPromptDefinition(layout.PromptPath)
	if err != nil {
		return err
	}
	return runAgentPrompt(ctx, adapter, layout, prompt.Prompt, opts.Stdout, opts.Stderr, opts.Timeout)
}

// runAgentPrompt executes one agent command, mirrors logs, and returns trimmed failure output.
func runAgentPrompt(
	ctx context.Context,
	adapter agentAdapter,
	layout *demoLayout,
	prompt string,
	stdoutMirror io.Writer,
	stderrMirror io.Writer,
	timeout time.Duration,
) error {
	command, err := adapter.command(layout, prompt)
	if err != nil {
		return err
	}
	stdoutFile, err := openAgentLog(layout.AgentStdoutPath, adapter.name())
	if err != nil {
		return fmt.Errorf("open agent stdout log: %w", err)
	}
	defer func() {
		_ = stdoutFile.Close()
	}()

	stderrFile, err := openAgentLog(layout.AgentStderrPath, adapter.name())
	if err != nil {
		return fmt.Errorf("open agent stderr log: %w", err)
	}
	defer func() {
		_ = stderrFile.Close()
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	//nolint:gosec // The command is constructed by the selected internal agent adapter.
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = layout.WorkspacePath
	if envVars := adapter.env(layout); len(envVars) > 0 {
		cmd.Env = append(os.Environ(), envVars...)
	}
	cmd.Stdout = io.MultiWriter(stdoutFile, &stdout)
	cmd.Stderr = io.MultiWriter(stderrFile, &stderr)
	cmd.Stdin = strings.NewReader(prompt)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("agent timed out after %s", timeout)
		}
		return fmt.Errorf("agent failed: %w\nstdout:\n%s\nstderr:\n%s", err, common.TrimOutput(stdout.String()), common.TrimOutput(stderr.String()))
	}
	if stdoutMirror != nil && stdout.Len() > 0 {
		_, _ = io.Copy(stdoutMirror, &stdout)
	}
	if stderrMirror != nil && stderr.Len() > 0 {
		_, _ = io.Copy(stderrMirror, &stderr)
	}
	return nil
}

// openAgentLog appends a run separator and opens the log file for one agent stream.
func openAgentLog(path, agentName string) (*os.File, error) {
	//nolint:gosec // path is a fixed log file path generated by the internal demo layout.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	separator := fmt.Sprintf(
		"\n===== Demo run %s (%s) =====\n",
		time.Now().Format(time.RFC3339),
		agentName,
	)
	if _, err := file.WriteString(separator); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

// printDemoStatus emits the key paths and URLs the user needs after the demo is ready.
func printDemoStatus(w io.Writer, result *DemoResult) {
	if w == nil || result == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "Demo root: %s\n", result.RootPath)
	_, _ = fmt.Fprintf(w, "Workspace: %s\n", result.WorkspacePath)
	_, _ = fmt.Fprintf(w, "UI: %s\n", result.UIPublicURL)
	_, _ = fmt.Fprintf(w, "Agent stdout: %s\n", result.AgentStdout)
	_, _ = fmt.Fprintf(w, "Agent stderr: %s\n", result.AgentStderr)
	_, _ = fmt.Fprintf(w, "Centian PID: %d\n", result.PID)
	_, _ = fmt.Fprintf(w, "Stop: %s\n", result.StopHint)
}
