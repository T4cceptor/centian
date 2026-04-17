package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/T4cceptor/centian/internal/agentrunner"
	"github.com/T4cceptor/centian/internal/common"
	urfavecli "github.com/urfave/cli/v3"
)

func TestDemoCommandStructure(t *testing.T) {
	if DemoCommand == nil {
		t.Fatal("DemoCommand is nil")
	}
	if DemoCommand.Name != "demo" {
		t.Fatalf("expected demo command name, got %q", DemoCommand.Name)
	}
	if DemoCommand.Action == nil {
		t.Fatal("DemoCommand.Action is nil")
	}
	flagNames := map[string]bool{}
	for _, flag := range DemoCommand.Flags {
		if typed, ok := flag.(*urfavecli.StringFlag); ok {
			flagNames[typed.Name] = true
		}
	}
	for _, expected := range []string{"agent", "model", "profile", "path", "codex-config"} {
		if !flagNames[expected] {
			t.Fatalf("expected %q flag on DemoCommand", expected)
		}
	}
}

func TestResolveDemoRootDefault(t *testing.T) {
	cwd := t.TempDir()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(original)
	}()

	root, err := resolveDemoRoot("")
	if err != nil {
		t.Fatalf("resolveDemoRoot: %v", err)
	}
	expected, err := filepath.Abs(filepath.Join(cwd, ".centian", "demo"))
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	if normalizeDarwinPath(root) != normalizeDarwinPath(expected) {
		t.Fatalf("expected %s, got %s", expected, root)
	}
}

func normalizeDarwinPath(value string) string {
	return strings.TrimPrefix(value, "/private")
}

func TestHandleDemoCommandRequiresAgent(t *testing.T) {
	cmd := &urfavecli.Command{}
	err := handleDemoCommand(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
}

func TestShouldShutdownDemo(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "empty defaults to yes", input: "\n", expected: true},
		{name: "explicit yes", input: "y\n", expected: true},
		{name: "explicit no", input: "n\n", expected: false},
		{name: "full no", input: "no\n", expected: false},
		{name: "unexpected value treated as no", input: "later\n", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if actual := shouldShutdownDemo(strings.NewReader(tt.input)); actual != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, actual)
			}
		})
	}
}

func TestDemoExecutionFromFlagsAppliesModelAndConfig(t *testing.T) {
	cmd := newTestCLICommand(DemoCommand.Flags)
	cmd.Set("agent", "codex")
	cmd.Set("model", "gpt5.4-mini")
	cmd.Set("codex-config", "/tmp/codex.toml")

	exec, err := demoExecutionFromFlags(cmd)
	if err != nil {
		t.Fatalf("demoExecutionFromFlags: %v", err)
	}
	if exec.Agent != agentrunner.AgentCodex {
		t.Fatalf("expected codex agent, got %q", exec.Agent)
	}
	if exec.Model != common.ModelCodexGPT54Mini {
		t.Fatalf("expected normalized model, got %q", exec.Model)
	}
	if exec.CodexConfigPath == "" {
		t.Fatal("expected codex config path to be resolved")
	}
}

func TestDemoExecutionFromFlagsRejectsProfileWithoutCodexOllama(t *testing.T) {
	cmd := newTestCLICommand(DemoCommand.Flags)
	cmd.Set("agent", "codex")
	cmd.Set("profile", "local-oss")

	_, err := demoExecutionFromFlags(cmd)
	if err == nil || err.Error() != "--profile can only be used with --agent codex-ollama" {
		t.Fatalf("expected profile error, got %v", err)
	}
}

func TestDemoExecutionFromFlagsRequiresCodexOllamaProfileAndConfig(t *testing.T) {
	cmd := newTestCLICommand(DemoCommand.Flags)
	cmd.Set("agent", "codex-ollama")

	_, err := demoExecutionFromFlags(cmd)
	if err == nil || err.Error() != "codex-ollama requires --codex-config" {
		t.Fatalf("expected missing codex config error, got %v", err)
	}

	cmd = newTestCLICommand(DemoCommand.Flags)
	cmd.Set("agent", "codex-ollama")
	cmd.Set("codex-config", "/tmp/codex.toml")

	_, err = demoExecutionFromFlags(cmd)
	if err == nil || err.Error() != "codex-ollama requires --profile" {
		t.Fatalf("expected missing profile error, got %v", err)
	}
}
