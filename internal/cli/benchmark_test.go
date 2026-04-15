package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	urfavecli "github.com/urfave/cli/v3"
)

func newTestCLICommand(flags []urfavecli.Flag) *urfavecli.Command {
	return &urfavecli.Command{Flags: cloneTestCLIFlags(flags)}
}

func cloneTestCLIFlags(flags []urfavecli.Flag) []urfavecli.Flag {
	cloned := make([]urfavecli.Flag, 0, len(flags))
	for _, flag := range flags {
		switch typed := flag.(type) {
		case *urfavecli.StringFlag:
			clonedFlag := *typed
			cloned = append(cloned, &clonedFlag)
		case *urfavecli.StringSliceFlag:
			clonedFlag := *typed
			cloned = append(cloned, &clonedFlag)
		case *urfavecli.IntFlag:
			clonedFlag := *typed
			cloned = append(cloned, &clonedFlag)
		case *urfavecli.DurationFlag:
			clonedFlag := *typed
			cloned = append(cloned, &clonedFlag)
		case *urfavecli.BoolFlag:
			clonedFlag := *typed
			cloned = append(cloned, &clonedFlag)
		default:
			cloned = append(cloned, flag)
		}
	}
	return cloned
}

func TestBenchmarkCommandStructure(t *testing.T) {
	if BenchmarkCommand == nil {
		t.Fatal("BenchmarkCommand is nil")
	}
	if BenchmarkCommand.Name != "benchmark" {
		t.Fatalf("expected benchmark command name, got %q", BenchmarkCommand.Name)
	}
	if len(BenchmarkCommand.Commands) != 1 {
		t.Fatalf("expected 1 benchmark subcommand, got %d", len(BenchmarkCommand.Commands))
	}
	if BenchmarkCommand.Commands[0] != BenchmarkRunCommand {
		t.Fatal("expected benchmark run subcommand to be registered")
	}
}

func TestBenchmarkRunCommandStructure(t *testing.T) {
	if BenchmarkRunCommand == nil {
		t.Fatal("BenchmarkRunCommand is nil")
	}
	if BenchmarkRunCommand.Name != "run" {
		t.Fatalf("expected benchmark run command name, got %q", BenchmarkRunCommand.Name)
	}
	flagNames := map[string]bool{}
	for _, flag := range BenchmarkRunCommand.Flags {
		switch typed := flag.(type) {
		case *urfavecli.StringFlag:
			flagNames[typed.Name] = true
		case *urfavecli.StringSliceFlag:
			flagNames[typed.Name] = true
		case *urfavecli.IntFlag:
			flagNames[typed.Name] = true
		case *urfavecli.DurationFlag:
			flagNames[typed.Name] = true
		case *urfavecli.BoolFlag:
			flagNames[typed.Name] = true
		}
	}
	for _, expected := range []string{
		"suite", "case", "agent", "repeat", "template-dir", "timeout",
		"output-root", "model", "claude-model", "gemini-model", "codex-model", "codex-ollama-model", "codex-config", "centian-config", "keep-centian-running",
	} {
		if !flagNames[expected] {
			t.Fatalf("expected %q flag on BenchmarkRunCommand", expected)
		}
	}
}

func TestBuildBenchmarkRunOptionsRequiresAgent(t *testing.T) {
	cmd := newTestCLICommand(BenchmarkRunCommand.Flags)
	cmd.Set("suite", t.TempDir())
	cmd.Set("repeat", "1")

	_, err := buildBenchmarkRunOptions(cmd, "/tmp/centian")
	if err == nil || err.Error() != "at least one agent is required" {
		t.Fatalf("expected missing agent error, got %v", err)
	}
}

func TestBuildBenchmarkRunOptionsRejectsInvalidRepeat(t *testing.T) {
	cmd := newTestCLICommand(BenchmarkRunCommand.Flags)
	cmd.Set("suite", t.TempDir())
	cmd.Set("agent", "codex")
	cmd.Set("repeat", "0")

	_, err := buildBenchmarkRunOptions(cmd, "/tmp/centian")
	if err == nil || err.Error() != "repeat must be greater than zero" {
		t.Fatalf("expected invalid repeat error, got %v", err)
	}
}

func TestParseTemplateVariantsRejectsInvalidFormat(t *testing.T) {
	_, err := parseTemplateVariants([]string{"broken"})
	if err == nil || err.Error() != `template-dir "broken" must use name=path format` {
		t.Fatalf("expected invalid template-dir error, got %v", err)
	}
}

func TestBuildBenchmarkRunOptionsResolvesDefaults(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	cmd := newTestCLICommand(BenchmarkRunCommand.Flags)
	cmd.Set("suite", filepath.Join(cwd, "..", "..", "tests", "integrationtests", "taskverification", "benchmarks", "simple_tdd_v1"))
	cmd.Set("agent", "codex,claude")
	cmd.Set("repeat", "1")
	cmd.Set("timeout", "20m")

	opts, err := buildBenchmarkRunOptions(cmd, "/tmp/centian")
	if err != nil {
		t.Fatalf("buildBenchmarkRunOptions: %v", err)
	}
	if len(opts.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(opts.Agents))
	}
	if opts.Timeout != 20*time.Minute {
		t.Fatalf("expected 20m timeout, got %s", opts.Timeout)
	}
	if len(opts.TemplateVariants) != 1 || opts.TemplateVariants[0].Name != "current" {
		t.Fatalf("expected current template variant, got %+v", opts.TemplateVariants)
	}
	if filepath.Base(opts.OutputRoot) != "benchmarks" {
		t.Fatalf("expected benchmark output root, got %s", opts.OutputRoot)
	}
	if opts.SessionLabel != "" {
		t.Fatalf("expected empty session label for multi-agent run, got %q", opts.SessionLabel)
	}
}

func TestBuildBenchmarkRunOptionsUsesTemplateDirFromCentianConfig(t *testing.T) {
	suiteRoot := t.TempDir()
	templateDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "centian.config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "name": "Benchmark Config",
  "version": "1.0.0",
  "auth": false,
  "proxy": {
    "host": "127.0.0.1",
    "port": "__PORT__",
    "capabilities": {
      "taskVerification": {
        "enabled": true,
        "templatesPath": `+`"`+templateDir+`"`+`
      },
      "eventStorage": {
        "enabled": true,
        "driver": "sqlite",
        "path": "__EVENT_STORE_PATH__"
      }
    }
  },
  "gateways": {
    "taskverification": {
      "mcpServers": {}
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := newTestCLICommand(BenchmarkRunCommand.Flags)
	cmd.Set("suite", suiteRoot)
	cmd.Set("agent", "codex")
	cmd.Set("repeat", "1")
	cmd.Set("centian-config", configPath)

	opts, err := buildBenchmarkRunOptions(cmd, "/tmp/centian")
	if err != nil {
		t.Fatalf("buildBenchmarkRunOptions: %v", err)
	}
	if opts.CentianConfigPath != configPath {
		t.Fatalf("expected centian config path %q, got %q", configPath, opts.CentianConfigPath)
	}
	if len(opts.TemplateVariants) != 1 || opts.TemplateVariants[0].Name != "current" || opts.TemplateVariants[0].SourceDir != templateDir {
		t.Fatalf("expected template variant from centian config, got %+v", opts.TemplateVariants)
	}
	if opts.SessionLabel != "current_codex_run" {
		t.Fatalf("expected derived session label, got %q", opts.SessionLabel)
	}
}

func TestBenchmarkAgentModelsFromFlagsAppliesSingleModelFlag(t *testing.T) {
	models, err := benchmarkAgentModelsFromFlags("gpt5.4-mini", []string{"codex"}, "", "", "", "")
	if err != nil {
		t.Fatalf("benchmarkAgentModelsFromFlags: %v", err)
	}
	if models.Codex != "gpt-5.4-mini" {
		t.Fatalf("expected normalized codex model, got %q", models.Codex)
	}
}

func TestBenchmarkAgentModelsFromFlagsRejectsSingleModelWithMultipleAgents(t *testing.T) {
	_, err := benchmarkAgentModelsFromFlags("gpt-5.4-mini", []string{"codex", "claude"}, "", "", "", "")
	if err == nil || err.Error() != "--model can only be used with exactly one agent; use --claude-model, --gemini-model, --codex-model, or --codex-ollama-model for multi-agent runs" {
		t.Fatalf("expected multi-agent model error, got %v", err)
	}
}

func TestBenchmarkAgentModelsFromFlagsRejectsSingleAndAgentModelConflict(t *testing.T) {
	_, err := benchmarkAgentModelsFromFlags("gpt-5.4-mini", []string{"codex"}, "", "", "gpt-5.4", "")
	if err == nil || err.Error() != "--model cannot be combined with --codex-model" {
		t.Fatalf("expected model conflict error, got %v", err)
	}
}

func TestBenchmarkAgentModelsFromFlagsAppliesCodexOllamaSingleModelFlag(t *testing.T) {
	models, err := benchmarkAgentModelsFromFlags("gemma4", []string{"codex-ollama"}, "", "", "", "")
	if err != nil {
		t.Fatalf("benchmarkAgentModelsFromFlags: %v", err)
	}
	if models.CodexOllama != "gemma4" {
		t.Fatalf("expected normalized codex-ollama model, got %q", models.CodexOllama)
	}
}

func TestBuildBenchmarkRunOptionsRequiresSuite(t *testing.T) {
	cmd := newTestCLICommand(BenchmarkRunCommand.Flags)
	cmd.Set("suite", "")
	cmd.Set("agent", "codex")
	cmd.Set("repeat", "1")

	_, err := buildBenchmarkRunOptions(cmd, "/tmp/centian")
	if err == nil || err.Error() != "suite path is required" {
		t.Fatalf("expected missing suite error, got %v", err)
	}
}
