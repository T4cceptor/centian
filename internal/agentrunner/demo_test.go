package agentrunner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeAdapter struct {
	agentName string
	commandFn func(*demoLayout, string) ([]string, error)
}

func (f fakeAdapter) name() string                { return f.agentName }
func (fakeAdapter) isAvailable() error            { return nil }
func (fakeAdapter) writeConfig(*demoLayout) error { return nil }
func (fakeAdapter) cleanup(*demoLayout) error     { return nil }
func (f fakeAdapter) command(layout *demoLayout, prompt string) ([]string, error) {
	return f.commandFn(layout, prompt)
}
func (fakeAdapter) env(*demoLayout) []string { return nil }

func TestPrepareLayoutRejectsNonDemoNonEmptyDir(t *testing.T) {
	allocateFreePortFunc = func() (string, error) { return "40123", nil }
	processExistsFunc = func(int) bool { return false }
	defer func() {
		allocateFreePortFunc = allocateFreePort
		processExistsFunc = processExists
	}()

	root := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := prepareLayout(&DemoOptions{RootPath: root})
	if err == nil || !strings.Contains(err.Error(), "is not a Centian demo root") {
		t.Fatalf("expected non-demo dir error, got %v", err)
	}
}

func TestPrepareLayoutAllowsEmptyExistingDir(t *testing.T) {
	allocateFreePortFunc = func() (string, error) { return "40123", nil }
	processExistsFunc = func(int) bool { return false }
	defer func() {
		allocateFreePortFunc = allocateFreePort
		processExistsFunc = processExists
	}()

	root := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	layout, err := prepareLayout(&DemoOptions{RootPath: root})
	if err != nil {
		t.Fatalf("prepareLayout: %v", err)
	}
	if layout.RootPath != root {
		t.Fatalf("unexpected root path: %s", layout.RootPath)
	}
	for _, dir := range []string{layout.WorkspacePath, layout.TemplatesPath, layout.LogsPath} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("expected dir %s: %v", dir, err)
		}
	}
	if layout.AgentStdoutPath == "" || layout.AgentStderrPath == "" {
		t.Fatal("expected agent log paths to be initialized")
	}
	if filepath.Base(layout.GeminiConfig) != "settings.json" {
		t.Fatalf("unexpected gemini config path: %s", layout.GeminiConfig)
	}
}

func TestPrepareLayoutReusesDemoRootAndPreservesHistory(t *testing.T) {
	allocateFreePortFunc = func() (string, error) { return "40123", nil }
	processExistsFunc = func(int) bool { return false }
	defer func() {
		allocateFreePortFunc = allocateFreePort
		processExistsFunc = processExists
	}()

	root := filepath.Join(t.TempDir(), "demo")
	for _, dir := range []string{
		filepath.Join(root, "logs"),
		filepath.Join(root, "workspace"),
		filepath.Join(root, "templates"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	for path, contents := range map[string]string{
		filepath.Join(root, "logs", "events.sqlite"):     "events",
		filepath.Join(root, "logs", "internal.log"):      "internal",
		filepath.Join(root, "agent.stdout.log"):          "stdout-history",
		filepath.Join(root, "agent.stderr.log"):          "stderr-history",
		filepath.Join(root, "workspace", "old.txt"):      "stale-workspace",
		filepath.Join(root, "workspace", ".gemini", "x"): "stale-gemini",
		filepath.Join(root, "templates", "old.yaml"):     "stale-template",
		filepath.Join(root, "config.json"):               "stale-config",
		filepath.Join(root, "prompt.md"):                 "stale-prompt",
		filepath.Join(root, "claude_mcp_config.json"):    "stale-agent-config",
		filepath.Join(root, "centian.pid"):               "12345\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir parent for %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	layout, err := prepareLayout(&DemoOptions{RootPath: root})
	if err != nil {
		t.Fatalf("prepareLayout: %v", err)
	}
	for _, preserved := range []string{
		layout.EventStorePath,
		layout.InternalLogPath,
		layout.AgentStdoutPath,
		layout.AgentStderrPath,
	} {
		if _, err := os.Stat(preserved); err != nil {
			t.Fatalf("expected preserved file %s: %v", preserved, err)
		}
	}
	assertFileContains(t, layout.EventStorePath, "events")
	assertFileContains(t, layout.InternalLogPath, "internal")
	assertFileContains(t, layout.AgentStdoutPath, "stdout-history")
	assertFileContains(t, layout.AgentStderrPath, "stderr-history")
	for _, removed := range []string{
		filepath.Join(root, "workspace", "old.txt"),
		filepath.Join(root, "workspace", ".gemini", "x"),
		filepath.Join(root, "templates", "old.yaml"),
		filepath.Join(root, "config.json"),
		filepath.Join(root, "prompt.md"),
		filepath.Join(root, "claude_mcp_config.json"),
		filepath.Join(root, "centian.pid"),
	} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, got %v", removed, err)
		}
	}
	for _, dir := range []string{layout.WorkspacePath, layout.TemplatesPath, layout.LogsPath} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("expected dir %s: %v", dir, err)
		}
	}

	if err := renderAssets(layout); err != nil {
		t.Fatalf("renderAssets: %v", err)
	}
	if err := (claudeAdapter{model: "sonnet"}).writeConfig(layout); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
	for _, regenerated := range []string{
		layout.ConfigPath,
		layout.PromptPath,
		filepath.Join(layout.TemplatesPath, TaskTemplateFile),
		layout.ClaudeConfig,
	} {
		if _, err := os.Stat(regenerated); err != nil {
			t.Fatalf("expected regenerated artifact %s: %v", regenerated, err)
		}
	}
}

func TestPrepareLayoutBlocksLivePID(t *testing.T) {
	allocateFreePortFunc = func() (string, error) { return "40123", nil }
	processExistsFunc = func(int) bool { return true }
	defer func() {
		allocateFreePortFunc = allocateFreePort
		processExistsFunc = processExists
	}()

	root := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "centian.pid"), []byte("999\n"), 0o644); err != nil {
		t.Fatalf("write pid: %v", err)
	}

	_, err := prepareLayout(&DemoOptions{RootPath: root})
	if err == nil || !strings.Contains(err.Error(), "running Centian server") {
		t.Fatalf("expected live pid error, got %v", err)
	}
}

func TestPrepareLayoutRemovesStalePID(t *testing.T) {
	allocateFreePortFunc = func() (string, error) { return "40123", nil }
	processExistsFunc = func(int) bool { return false }
	defer func() {
		allocateFreePortFunc = allocateFreePort
		processExistsFunc = processExists
	}()

	root := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "centian.pid"), []byte("999\n"), 0o644); err != nil {
		t.Fatalf("write pid: %v", err)
	}

	layout, err := prepareLayout(&DemoOptions{RootPath: root})
	if err != nil {
		t.Fatalf("prepareLayout: %v", err)
	}
	if _, err := os.Stat(layout.PIDPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale pid file to be removed, got %v", err)
	}
}

func TestRenderAssetsWritesExpectedFiles(t *testing.T) {
	allocateFreePortFunc = func() (string, error) { return "40123", nil }
	defer func() { allocateFreePortFunc = allocateFreePort }()

	root := filepath.Join(t.TempDir(), "demo")
	layout, err := prepareLayout(&DemoOptions{RootPath: root})
	if err != nil {
		t.Fatalf("prepareLayout: %v", err)
	}

	if err := renderAssets(layout); err != nil {
		t.Fatalf("renderAssets: %v", err)
	}

	assertFileContains(t, layout.ConfigPath, `"port": "`+layout.Port+`"`)
	assertFileContains(t, layout.ConfigPath, layout.WorkspacePath)
	assertFileContains(t, layout.PromptPath, "scoreParentheses")
	assertFileContains(t, filepath.Join(layout.TemplatesPath, TaskTemplateFile), "Guided TDD Workflow")
	if filepath.Base(layout.AgentStdoutPath) != "agent.stdout.log" {
		t.Fatalf("unexpected agent stdout path: %s", layout.AgentStdoutPath)
	}
	if filepath.Base(layout.AgentStderrPath) != "agent.stderr.log" {
		t.Fatalf("unexpected agent stderr path: %s", layout.AgentStderrPath)
	}
}

func TestClaudeCommandConstruction(t *testing.T) {
	layout := &demoLayout{
		ClaudeConfig:  "/tmp/demo/claude_mcp_config.json",
		WorkspacePath: "/tmp/demo/workspace",
	}
	command, err := claudeAdapter{model: "sonnet"}.command(layout, "prompt")
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	joined := strings.Join(command, " ")
	for _, expected := range []string{
		"claude",
		"-p",
		"--output-format json",
		"--permission-mode default",
		"--allowedTools mcp__centian__*",
		"--mcp-config /tmp/demo/claude_mcp_config.json",
		"--strict-mcp-config",
		"--tools ",
		"--model sonnet",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected %q in command %q", expected, joined)
		}
	}
	for _, forbidden := range []string{"bypassPermissions", "dangerously-skip-permissions"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("unexpected unsafe flag %q in %q", forbidden, joined)
		}
	}
}

func TestGeminiCommandConstruction(t *testing.T) {
	layout := &demoLayout{
		GeminiConfig:  "/tmp/demo/workspace/.gemini/settings.json",
		WorkspacePath: "/tmp/demo/workspace",
	}
	command, err := geminiAdapter{model: "flash"}.command(layout, "solve the task")
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	joined := strings.Join(command, " ")
	for _, expected := range []string{
		"gemini",
		"-p solve the task",
		"--output-format json",
		"--sandbox",
		"--approval-mode default",
		"--allowed-mcp-server-names centian",
		"--model flash",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected %q in command %q", expected, joined)
		}
	}
	for _, forbidden := range []string{"yolo", "auto_edit"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("unexpected unsafe or broad approval mode %q in %q", forbidden, joined)
		}
	}
}

func TestGeminiWriteConfig(t *testing.T) {
	allocateFreePortFunc = func() (string, error) { return "40123", nil }
	defer func() { allocateFreePortFunc = allocateFreePort }()

	root := filepath.Join(t.TempDir(), "demo")
	layout, err := prepareLayout(&DemoOptions{RootPath: root})
	if err != nil {
		t.Fatalf("prepareLayout: %v", err)
	}
	if err := (geminiAdapter{}).writeConfig(layout); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
	assertFileContains(t, layout.GeminiConfig, `"httpUrl": "`+layout.MCPURL+`"`)
	assertFileContains(t, layout.GeminiConfig, `"allowed": [`)
	assertFileContains(t, layout.GeminiConfig, `"core": []`)
}

func TestRunAgentAppendsLogs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	layout := &demoLayout{
		WorkspacePath:   root,
		PromptPath:      filepath.Join(root, "prompt.md"),
		AgentStdoutPath: filepath.Join(root, "agent.stdout.log"),
		AgentStderrPath: filepath.Join(root, "agent.stderr.log"),
	}
	if err := os.MkdirAll(layout.WorkspacePath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(layout.PromptPath, []byte("prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	adapter := fakeAdapter{
		agentName: AgentClaude,
		commandFn: func(*demoLayout, string) ([]string, error) {
			return []string{"sh", "-c", "printf 'stdout-line\\n'; printf 'stderr-line\\n' >&2"}, nil
		},
	}

	if err := runAgent(context.Background(), adapter, layout, &DemoOptions{Timeout: time.Second}); err != nil {
		t.Fatalf("runAgent first run: %v", err)
	}
	if err := runAgent(context.Background(), adapter, layout, &DemoOptions{Timeout: time.Second}); err != nil {
		t.Fatalf("runAgent second run: %v", err)
	}

	stdoutData, err := os.ReadFile(layout.AgentStdoutPath)
	if err != nil {
		t.Fatalf("read stdout log: %v", err)
	}
	stderrData, err := os.ReadFile(layout.AgentStderrPath)
	if err != nil {
		t.Fatalf("read stderr log: %v", err)
	}
	if got := strings.Count(string(stdoutData), "stdout-line"); got != 2 {
		t.Fatalf("expected stdout log to append two runs, got %d entries", got)
	}
	if got := strings.Count(string(stderrData), "stderr-line"); got != 2 {
		t.Fatalf("expected stderr log to append two runs, got %d entries", got)
	}
	if got := strings.Count(string(stdoutData), "===== Demo run "); got != 2 {
		t.Fatalf("expected stdout separators for both runs, got %d", got)
	}
	if got := strings.Count(string(stderrData), "===== Demo run "); got != 2 {
		t.Fatalf("expected stderr separators for both runs, got %d", got)
	}
}

func TestWritePIDAndStopHint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "centian.pid")
	if err := writePID(path, 12345); err != nil {
		t.Fatalf("writePID: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pid: %v", err)
	}
	if strings.TrimSpace(string(data)) != "12345" {
		t.Fatalf("unexpected pid file contents: %q", string(data))
	}
	if got := shellQuote(path); !strings.HasPrefix(got, "'") || !strings.HasSuffix(got, "'") {
		t.Fatalf("shellQuote should single-quote the path, got %q", got)
	}
}

func TestRunDemoUnsupportedAgent(t *testing.T) {
	allocateFreePortFunc = func() (string, error) { return "40123", nil }
	defer func() { allocateFreePortFunc = allocateFreePort }()

	_, err := (DemoRunner{}).RunDemo(context.Background(), &DemoOptions{
		Agent:             "unsupported-agent",
		RootPath:          filepath.Join(t.TempDir(), "demo"),
		CentianBinaryPath: "/tmp/centian",
		Timeout:           time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported agent") {
		t.Fatalf("expected unsupported agent error, got %v", err)
	}
}

func assertFileContains(t *testing.T, path, fragment string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), fragment) {
		t.Fatalf("expected %q in %s", fragment, path)
	}
}

func TestCodexCommandConstruction(t *testing.T) {
	// Given: a Codex adapter with a demo layout
	layout := &demoLayout{
		WorkspacePath: "/tmp/demo/workspace",
		RootPath:      "/tmp/demo",
		CodexConfig:   "/tmp/demo/codex-home/config.toml",
	}

	// When: constructing the command
	command, err := codexAdapter{}.command(layout, "prompt")
	if err != nil {
		t.Fatalf("command: %v", err)
	}

	// Then: the command includes expected Codex flags
	joined := strings.Join(command, " ")
	for _, expected := range []string{
		"codex",
		"exec",
		"--skip-git-repo-check",
		"--json",
		"-C /tmp/demo/workspace",
		"-o /tmp/demo/codex_output.txt",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected %q in command %q", expected, joined)
		}
	}

	// Then: no dangerous bypass flags are present
	for _, forbidden := range []string{"--full-auto", "--dangerously-bypass-approvals-and-sandbox"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("unexpected unsafe flag %q in %q", forbidden, joined)
		}
	}
}

func TestCodexOllamaCommandConstruction(t *testing.T) {
	layout := &demoLayout{CodexConfig: "/tmp/demo/codex-home/config.toml"}

	command, err := codexOllamaAdapter{model: "gemma4"}.command(layout, "prompt")
	if err != nil {
		t.Fatalf("command: %v", err)
	}

	joined := strings.Join(command, " ")
	for _, expected := range []string{
		"codex",
		"--oss",
		"--profile gemma4-local",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected %q in command %q", expected, joined)
		}
	}
}

func TestCodexEnvSetsCODEXHOME(t *testing.T) {
	// Given: a Codex adapter with a layout
	layout := &demoLayout{
		CodexConfig: "/tmp/demo/codex-home/config.toml",
	}

	// When: requesting env vars
	envVars := codexAdapter{}.env(layout)

	// Then: CODEX_HOME points to the parent of the config file
	if len(envVars) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(envVars))
	}
	if envVars[0] != "CODEX_HOME=/tmp/demo/codex-home" {
		t.Fatalf("expected CODEX_HOME=/tmp/demo/codex-home, got %q", envVars[0])
	}
}

func TestCodexWriteConfig(t *testing.T) {
	// Given: a Codex adapter and a temporary layout
	tmpDir := t.TempDir()
	layout := &demoLayout{
		MCPURL:        "http://127.0.0.1:12345/mcp/taskverification",
		WorkspacePath: "/tmp/demo/workspace",
		CodexConfig:   filepath.Join(tmpDir, "codex-home", "config.toml"),
	}

	// When: writing the config
	err := codexAdapter{model: "o3"}.writeConfig(layout)
	if err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	// Then: the config file is written with placeholders replaced
	assertFileContains(t, layout.CodexConfig, `url = "http://127.0.0.1:12345/mcp/taskverification"`)
	assertFileContains(t, layout.CodexConfig, `model = "o3"`)
	assertFileContains(t, layout.CodexConfig, "/tmp/demo/workspace")
	assertFileContains(t, layout.CodexConfig, `sandbox_mode = "read-only"`)
	assertFileContains(t, layout.CodexConfig, `default_tools_approval_mode = "auto"`)
	assertFileContains(t, layout.CodexConfig, `destructive_enabled = false`)
	assertFileContains(t, layout.CodexConfig, `open_world_enabled = false`)
	data, err := os.ReadFile(layout.CodexConfig)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(data), "__MODEL_BLOCK__") {
		t.Fatalf("expected placeholder removed, got:\n%s", string(data))
	}
}

func TestCodexOllamaWriteDefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()
	layout := &demoLayout{
		MCPURL:        "http://127.0.0.1:12345/mcp/taskverification",
		WorkspacePath: "/tmp/demo/workspace",
		CodexConfig:   filepath.Join(tmpDir, "codex-home", "config.toml"),
	}

	if err := (codexOllamaAdapter{}).writeConfig(layout); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	assertFileContains(t, layout.CodexConfig, `oss_provider = "ollama-custom"`)
	assertFileContains(t, layout.CodexConfig, `[model_providers.ollama-custom]`)
	assertFileContains(t, layout.CodexConfig, `[profiles.gemma4-local]`)
	assertFileContains(t, layout.CodexConfig, `[profiles.qwen3.5-local]`)
	assertFileContains(t, layout.CodexConfig, `model_provider = "ollama-custom"`)
	assertFileContains(t, layout.CodexConfig, `url = "http://127.0.0.1:12345/mcp/taskverification"`)
	assertFileContains(t, layout.CodexConfig, `[projects."/tmp/demo/workspace"]`)
}

func TestCodexWriteConfigPatchesCustomBase(t *testing.T) {
	tmpDir := t.TempDir()
	baseConfigPath := filepath.Join(tmpDir, "base.toml")
	baseConfig := `
model = "ignored"
oss_provider = "ollama"

[mcp_servers.centian]
url = "http://old"
enabled = true

[profiles.custom]
model_provider = "ollama"
model = "custom:1b"
`
	if err := os.WriteFile(baseConfigPath, []byte(baseConfig), 0o600); err != nil {
		t.Fatalf("write base config: %v", err)
	}

	layout := &demoLayout{
		MCPURL:        "http://127.0.0.1:12345/mcp/taskverification",
		WorkspacePath: "/tmp/demo/workspace",
		CodexConfig:   filepath.Join(tmpDir, "codex-home", "config.toml"),
	}

	if err := (codexOllamaAdapter{baseConfigPath: baseConfigPath}).writeConfig(layout); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	data, err := os.ReadFile(layout.CodexConfig)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)
	if strings.Contains(content, `url = "http://old"`) {
		t.Fatalf("expected old centian block removed, got:\n%s", content)
	}
	assertFileContains(t, layout.CodexConfig, `[profiles.custom]`)
	assertFileContains(t, layout.CodexConfig, `url = "http://127.0.0.1:12345/mcp/taskverification"`)
	assertFileContains(t, layout.CodexConfig, `[projects."/tmp/demo/workspace"]`)
}

func TestCodexWriteConfigNoModel(t *testing.T) {
	// Given: a Codex adapter without a model
	tmpDir := t.TempDir()
	layout := &demoLayout{
		MCPURL:        "http://127.0.0.1:12345/mcp/taskverification",
		WorkspacePath: "/tmp/demo/workspace",
		CodexConfig:   filepath.Join(tmpDir, "codex-home", "config.toml"),
	}

	// When: writing the config with empty model
	err := codexAdapter{model: ""}.writeConfig(layout)
	if err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	// Then: no model line appears
	data, err := os.ReadFile(layout.CodexConfig)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(data), "model =") {
		t.Fatalf("expected no model line, got:\n%s", string(data))
	}
	if strings.Contains(string(data), "__MODEL_BLOCK__") {
		t.Fatalf("expected placeholder removed, got:\n%s", string(data))
	}
}

func TestCodexWriteConfigCopiesAuthJSON(t *testing.T) {
	// Given: a user Codex home with auth.json and a temporary demo layout
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	authPath := filepath.Join(homeDir, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(authPath), 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	if err := os.WriteFile(authPath, []byte(`{"access_token":"test-token"}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	t.Setenv("HOME", homeDir)

	layout := &demoLayout{
		MCPURL:        "http://127.0.0.1:12345/mcp/taskverification",
		WorkspacePath: "/tmp/demo/workspace",
		CodexConfig:   filepath.Join(tmpDir, "codex-home", "config.toml"),
	}

	// When: writing the demo Codex config
	if err := (codexAdapter{model: "o3"}).writeConfig(layout); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	// Then: auth.json is copied into the isolated CODEX_HOME
	copiedAuthPath := filepath.Join(filepath.Dir(layout.CodexConfig), "auth.json")
	data, err := os.ReadFile(copiedAuthPath)
	if err != nil {
		t.Fatalf("read copied auth.json: %v", err)
	}
	if string(data) != `{"access_token":"test-token"}` {
		t.Fatalf("unexpected auth.json contents: %s", string(data))
	}
}

func TestCodexCleanupRemovesDemoHome(t *testing.T) {
	// Given: a demo CODEX_HOME containing auth material and config
	tmpDir := t.TempDir()
	codexHome := filepath.Join(tmpDir, "codex-home")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	for path, contents := range map[string]string{
		filepath.Join(codexHome, "auth.json"):   `{"access_token":"test-token"}`,
		filepath.Join(codexHome, "config.toml"): `model = "o3"`,
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	layout := &demoLayout{CodexConfig: filepath.Join(codexHome, "config.toml")}

	// When: cleaning up after the Codex run
	if err := (codexAdapter{}).cleanup(layout); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	// Then: the isolated demo home is removed
	if _, err := os.Stat(codexHome); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, got %v", codexHome, err)
	}
}

func TestSelectAdapterCodex(t *testing.T) {
	// Given: demo options with agent "codex"
	opts := &DemoOptions{Agent: "codex", CodexModel: "o3"}

	// When: selecting the adapter
	adapter, err := selectAdapter(opts)

	// Then: a codex adapter is returned
	if err != nil {
		t.Fatalf("selectAdapter: %v", err)
	}
	if adapter.name() != AgentCodex {
		t.Fatalf("expected codex adapter, got %q", adapter.name())
	}
}

func TestSelectAdapterCodexOllama(t *testing.T) {
	opts := &DemoOptions{Agent: AgentCodexOllama, CodexOllamaModel: "gemma4"}

	adapter, err := selectAdapter(opts)
	if err != nil {
		t.Fatalf("selectAdapter: %v", err)
	}
	if adapter.name() != AgentCodexOllama {
		t.Fatalf("expected codex-ollama adapter, got %q", adapter.name())
	}
}
