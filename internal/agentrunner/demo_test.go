package agentrunner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrepareLayoutRejectsNonEmptyDir(t *testing.T) {
	allocateFreePortFunc = func() (string, error) { return "40123", nil }
	defer func() { allocateFreePortFunc = allocateFreePort }()

	root := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := prepareLayout(&DemoOptions{RootPath: root})
	if err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("expected non-empty dir error, got %v", err)
	}
}

func TestPrepareLayoutAllowsEmptyExistingDir(t *testing.T) {
	allocateFreePortFunc = func() (string, error) { return "40123", nil }
	defer func() { allocateFreePortFunc = allocateFreePort }()

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
	assertFileContains(t, layout.PromptPath, "score_parentheses")
	assertFileContains(t, filepath.Join(layout.TemplatesPath, "python_tdd_workflow.yaml"), "Python TDD Workflow")
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
		Agent:             "codex",
		RootPath:          filepath.Join(t.TempDir(), "demo"),
		CentianBinaryPath: "/tmp/centian",
		Timeout:           time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "supports") {
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
