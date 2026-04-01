package agentrunner

import (
	"bytes"
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
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// AgentClaude is the supported public agent identifier for the Claude CLI.
	AgentClaude = "claude"
	// AgentGemini is the supported public agent identifier for the Gemini CLI.
	AgentGemini = "gemini"
	// DefaultClaudeModel is the default Claude model alias for demo runs.
	DefaultClaudeModel = "sonnet"
	// DefaultGeminiModel is the default Gemini model alias for demo runs.
	DefaultGeminiModel = "gemini-3.1-pro-preview" // "flash" is an alternative
	// DefaultAgentTimeout is the default maximum runtime for a demo agent invocation.
	DefaultAgentTimeout = 5 * time.Minute
)

var allocateFreePortFunc = allocateFreePort

// DemoOptions configures a single demo run.
type DemoOptions struct {
	Agent             string
	RootPath          string
	CentianBinaryPath string
	Timeout           time.Duration
	ClaudeModel       string
	GeminiModel       string
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

type agentAdapter interface {
	name() string
	isAvailable() error
	writeConfig(*demoLayout) error
	command(*demoLayout, string) ([]string, error)
}

type demoLayout struct {
	RootPath        string
	WorkspacePath   string
	TemplatesPath   string
	LogsPath        string
	ConfigPath      string
	PromptPath      string
	AgentStdoutPath string
	AgentStderrPath string
	InternalLogPath string
	EventStorePath  string
	PIDPath         string
	ClaudeConfig    string
	GeminiConfig    string
	BaseURL         string
	MCPURL          string
	Port            string
}

// RunDemo creates the demo workspace, starts Centian, launches the selected
// agent, and leaves Centian running after the agent exits.
func (DemoRunner) RunDemo(ctx context.Context, opts *DemoOptions) (*DemoResult, error) {
	options := normalizeOptions(opts)
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

	centianCmd, errCh, err := startCentianProcess(layout, options)
	if err != nil {
		return nil, err
	}
	if err := writePID(layout.PIDPath, centianCmd.Process.Pid); err != nil {
		_ = centianCmd.Process.Kill()
		return nil, err
	}

	if err := waitForCentian(layout, errCh); err != nil {
		_ = centianCmd.Process.Kill()
		return nil, err
	}

	result := &DemoResult{
		RootPath:      layout.RootPath,
		WorkspacePath: layout.WorkspacePath,
		ConfigPath:    layout.ConfigPath,
		PromptPath:    layout.PromptPath,
		AgentStdout:   layout.AgentStdoutPath,
		AgentStderr:   layout.AgentStderrPath,
		UIPublicURL:   layout.BaseURL + "/ui/tasks",
		MCPURL:        layout.MCPURL,
		PID:           centianCmd.Process.Pid,
		StopHint:      fmt.Sprintf("kill $(cat %s)", shellQuote(layout.PIDPath)),
	}

	if options.OpenBrowser && runtime.GOOS == "darwin" {
		//nolint:gosec // UI URL is generated locally from the bound loopback port.
		_ = exec.Command("open", result.UIPublicURL).Start()
	}
	printDemoStatus(options.Stdout, result)

	if err := runAgent(ctx, adapter, layout, options); err != nil {
		return result, err
	}
	return result, nil
}

func normalizeOptions(opts *DemoOptions) *DemoOptions {
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultAgentTimeout
	}
	if strings.TrimSpace(opts.ClaudeModel) == "" {
		opts.ClaudeModel = DefaultClaudeModel
	}
	if strings.TrimSpace(opts.GeminiModel) == "" {
		opts.GeminiModel = DefaultGeminiModel
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if !opts.OpenBrowser && runtime.GOOS == "darwin" {
		opts.OpenBrowser = true
	}
	return opts
}

func prepareLayout(opts *DemoOptions) (*demoLayout, error) {
	root := strings.TrimSpace(opts.RootPath)
	if root == "" {
		return nil, fmt.Errorf("demo root path is required")
	}
	if err := ensureEmptyOrNewDir(root); err != nil {
		return nil, err
	}
	layout := &demoLayout{
		RootPath:        root,
		WorkspacePath:   filepath.Join(root, "workspace"),
		TemplatesPath:   filepath.Join(root, "templates"),
		LogsPath:        filepath.Join(root, "logs"),
		ConfigPath:      filepath.Join(root, "config.json"),
		PromptPath:      filepath.Join(root, "prompt.md"),
		AgentStdoutPath: filepath.Join(root, "agent.stdout.log"),
		AgentStderrPath: filepath.Join(root, "agent.stderr.log"),
		InternalLogPath: filepath.Join(root, "logs", "internal.log"),
		EventStorePath:  filepath.Join(root, "logs", "events.sqlite"),
		PIDPath:         filepath.Join(root, "centian.pid"),
		ClaudeConfig:    filepath.Join(root, "claude_mcp_config.json"),
		GeminiConfig:    filepath.Join(root, "workspace", ".gemini", "settings.json"),
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

func ensureEmptyOrNewDir(path string) error {
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
	if len(entries) > 0 {
		return fmt.Errorf("demo path exists and is not empty: %s", path)
	}
	return nil
}

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

	prompt, err := asset("prompt.md")
	if err != nil {
		return err
	}
	if err := os.WriteFile(layout.PromptPath, []byte(prompt), 0o600); err != nil {
		return fmt.Errorf("write prompt.md: %w", err)
	}

	template, err := asset("python_tdd_workflow.yaml")
	if err != nil {
		return err
	}
	templatePath := filepath.Join(layout.TemplatesPath, "python_tdd_workflow.yaml")
	if err := os.WriteFile(templatePath, []byte(template), 0o600); err != nil {
		return fmt.Errorf("write template: %w", err)
	}
	return nil
}

func selectAdapter(opts *DemoOptions) (agentAdapter, error) {
	switch strings.ToLower(strings.TrimSpace(opts.Agent)) {
	case AgentClaude:
		return claudeAdapter{model: opts.ClaudeModel}, nil
	case AgentGemini:
		return geminiAdapter{model: opts.GeminiModel}, nil
	default:
		return nil, fmt.Errorf("unsupported agent %q; v1 supports %q and %q only", opts.Agent, AgentClaude, AgentGemini)
	}
}

func startCentianProcess(layout *demoLayout, opts *DemoOptions) (*exec.Cmd, <-chan error, error) {
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

func writePID(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

func waitForCentian(layout *demoLayout, errCh <-chan error) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			if err == nil {
				return fmt.Errorf("centian exited before becoming ready")
			}
			return fmt.Errorf("centian exited before becoming ready: %w", err)
		default:
		}
		if isEndpointReachable(client, layout.MCPURL) && isJSONEndpointReady(client, layout.BaseURL+"/api/task-runs") {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("centian did not become ready in time; inspect %s", layout.InternalLogPath)
}

func runAgent(ctx context.Context, adapter agentAdapter, layout *demoLayout, opts *DemoOptions) error {
	command, err := adapter.command(layout, loadPrompt(layout.PromptPath))
	if err != nil {
		return err
	}
	stdoutFile, err := os.Create(layout.AgentStdoutPath)
	if err != nil {
		return fmt.Errorf("create agent stdout log: %w", err)
	}
	defer func() {
		_ = stdoutFile.Close()
	}()

	stderrFile, err := os.Create(layout.AgentStderrPath)
	if err != nil {
		return fmt.Errorf("create agent stderr log: %w", err)
	}
	defer func() {
		_ = stderrFile.Close()
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	//nolint:gosec // The command is constructed by the selected internal agent adapter.
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = layout.WorkspacePath
	cmd.Stdout = io.MultiWriter(stdoutFile, &stdout)
	cmd.Stderr = io.MultiWriter(stderrFile, &stderr)
	cmd.Stdin = strings.NewReader(loadPrompt(layout.PromptPath))
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("agent timed out after %s", opts.Timeout)
		}
		return fmt.Errorf("agent failed: %w\nstdout:\n%s\nstderr:\n%s", err, trimOutput(stdout.String()), trimOutput(stderr.String()))
	}
	if opts.Stdout != nil && stdout.Len() > 0 {
		_, _ = io.Copy(opts.Stdout, &stdout)
	}
	if opts.Stderr != nil && stderr.Len() > 0 {
		_, _ = io.Copy(opts.Stderr, &stderr)
	}
	return nil
}

func loadPrompt(path string) string {
	//nolint:gosec // The prompt file path is generated inside the demo workspace layout.
	data, _ := os.ReadFile(path)
	return string(data)
}

func isEndpointReachable(client *http.Client, endpoint string) bool {
	resp, err := client.Get(endpoint)
	if err != nil {
		return false
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return resp.StatusCode < 500
}

func isJSONEndpointReady(client *http.Client, endpoint string) bool {
	resp, err := client.Get(endpoint)
	if err != nil {
		return false
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return resp.StatusCode == http.StatusOK
}

func allocateFreePort() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("allocate port: %w", err)
	}
	defer func() {
		_ = listener.Close()
	}()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return "", fmt.Errorf("split host port: %w", err)
	}
	return port, nil
}

func trimOutput(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 4000 {
		return value[:4000] + "\n...truncated..."
	}
	return value
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

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
