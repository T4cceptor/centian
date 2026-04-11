package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	urfavecli "github.com/urfave/cli/v3"
)

func TestBenchmarkCommandStructure(t *testing.T) {
	if BenchmarkCommand == nil {
		t.Fatal("BenchmarkCommand is nil")
	}
	if BenchmarkCommand.Name != "benchmark" {
		t.Fatalf("expected benchmark command name, got %q", BenchmarkCommand.Name)
	}
	if len(BenchmarkCommand.Commands) != 4 {
		t.Fatalf("expected 4 benchmark subcommands, got %d", len(BenchmarkCommand.Commands))
	}
	if BenchmarkCommand.Commands[0] != BenchmarkRunCommand || BenchmarkCommand.Commands[1] != BenchmarkScoreCommand || BenchmarkCommand.Commands[2] != BenchmarkCompareCommand || BenchmarkCommand.Commands[3] != BenchmarkBackfillScoresCommand {
		t.Fatal("expected benchmark run, score, compare, and backfill-scores subcommands to be registered")
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

func TestBenchmarkScoreCommandStructure(t *testing.T) {
	if BenchmarkScoreCommand == nil {
		t.Fatal("BenchmarkScoreCommand is nil")
	}
	if BenchmarkScoreCommand.Name != "score" {
		t.Fatalf("expected benchmark score command name, got %q", BenchmarkScoreCommand.Name)
	}
	if len(BenchmarkScoreCommand.Flags) != 1 {
		t.Fatalf("expected one flag on BenchmarkScoreCommand, got %d", len(BenchmarkScoreCommand.Flags))
	}
	flag, ok := BenchmarkScoreCommand.Flags[0].(*urfavecli.StringFlag)
	if !ok || flag.Name != "session" {
		t.Fatalf("expected session string flag, got %#v", BenchmarkScoreCommand.Flags[0])
	}
}

func TestBenchmarkCompareCommandStructure(t *testing.T) {
	if BenchmarkCompareCommand == nil {
		t.Fatal("BenchmarkCompareCommand is nil")
	}
	if BenchmarkCompareCommand.Name != "compare" {
		t.Fatalf("expected benchmark compare command name, got %q", BenchmarkCompareCommand.Name)
	}
	flagNames := map[string]bool{}
	for _, flag := range BenchmarkCompareCommand.Flags {
		switch typed := flag.(type) {
		case *urfavecli.StringFlag:
			flagNames[typed.Name] = true
		case *urfavecli.StringSliceFlag:
			flagNames[typed.Name] = true
		}
	}
	for _, expected := range []string{"root", "suite", "agent", "case", "template-variant"} {
		if !flagNames[expected] {
			t.Fatalf("expected %q flag on BenchmarkCompareCommand", expected)
		}
	}
}

func TestBenchmarkBackfillScoresCommandStructure(t *testing.T) {
	if BenchmarkBackfillScoresCommand == nil {
		t.Fatal("BenchmarkBackfillScoresCommand is nil")
	}
	if BenchmarkBackfillScoresCommand.Name != "backfill-scores" {
		t.Fatalf("expected benchmark backfill command name, got %q", BenchmarkBackfillScoresCommand.Name)
	}
	flagNames := map[string]bool{}
	for _, flag := range BenchmarkBackfillScoresCommand.Flags {
		switch typed := flag.(type) {
		case *urfavecli.StringFlag:
			flagNames[typed.Name] = true
		case *urfavecli.StringSliceFlag:
			flagNames[typed.Name] = true
		case *urfavecli.BoolFlag:
			flagNames[typed.Name] = true
		}
	}
	for _, expected := range []string{"suite", "session", "agent", "case", "template-variant", "path-remap", "force"} {
		if !flagNames[expected] {
			t.Fatalf("expected %q flag on BenchmarkBackfillScoresCommand", expected)
		}
	}
}

func TestBuildBenchmarkRunOptionsRequiresAgent(t *testing.T) {
	cmd := &urfavecli.Command{
		Flags: BenchmarkRunCommand.Flags,
	}
	cmd.Set("suite", t.TempDir())
	cmd.Set("repeat", "1")

	_, err := buildBenchmarkRunOptions(cmd, "/tmp/centian")
	if err == nil || err.Error() != "at least one agent is required" {
		t.Fatalf("expected missing agent error, got %v", err)
	}
}

func TestBuildBenchmarkRunOptionsRejectsInvalidRepeat(t *testing.T) {
	cmd := &urfavecli.Command{
		Flags: BenchmarkRunCommand.Flags,
	}
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
	cmd := &urfavecli.Command{
		Flags: BenchmarkRunCommand.Flags,
	}
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

	cmd := &urfavecli.Command{
		Flags: BenchmarkRunCommand.Flags,
	}
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
	cmd := &urfavecli.Command{
		Flags: BenchmarkRunCommand.Flags,
	}
	cmd.Set("suite", "")
	cmd.Set("agent", "codex")
	cmd.Set("repeat", "1")

	_, err := buildBenchmarkRunOptions(cmd, "/tmp/centian")
	if err == nil || err.Error() != "suite path is required" {
		t.Fatalf("expected missing suite error, got %v", err)
	}
}

func TestBuildBenchmarkScoreOptionsRequiresSession(t *testing.T) {
	cmd := &urfavecli.Command{
		Flags: BenchmarkScoreCommand.Flags,
	}
	cmd.Set("session", "")

	_, err := buildBenchmarkScoreOptions(cmd)
	if err == nil || err.Error() != "session path is required" {
		t.Fatalf("expected missing session error, got %v", err)
	}
}

func TestBuildBenchmarkScoreOptionsResolvesAbsolutePath(t *testing.T) {
	cmd := &urfavecli.Command{
		Flags: BenchmarkScoreCommand.Flags,
	}
	sessionDir := t.TempDir()
	cmd.Set("session", sessionDir)

	opts, err := buildBenchmarkScoreOptions(cmd)
	if err != nil {
		t.Fatalf("buildBenchmarkScoreOptions: %v", err)
	}
	if opts.SessionPath != sessionDir {
		t.Fatalf("expected resolved session path %q, got %q", sessionDir, opts.SessionPath)
	}
}

func TestBuildBenchmarkCompareOptionsRequiresRoot(t *testing.T) {
	cmd := &urfavecli.Command{
		Flags: BenchmarkCompareCommand.Flags,
	}
	cmd.Set("suite", "simple_tdd_v1")

	_, err := buildBenchmarkCompareOptions(cmd)
	if err == nil || err.Error() != "root path is required" {
		t.Fatalf("expected missing root error, got %v", err)
	}
}

func TestBuildBenchmarkCompareOptionsRequiresSuite(t *testing.T) {
	cmd := &urfavecli.Command{
		Flags: BenchmarkCompareCommand.Flags,
	}
	cmd.Set("root", t.TempDir())
	cmd.Set("suite", "")

	_, err := buildBenchmarkCompareOptions(cmd)
	if err == nil || err.Error() != "suite id is required" {
		t.Fatalf("expected missing suite error, got %v", err)
	}
}

func TestParsePathRemapsRejectsInvalidFormat(t *testing.T) {
	_, err := parsePathRemaps([]string{"broken"})
	if err == nil || err.Error() != `path-remap "broken" must use OLD=NEW format` {
		t.Fatalf("expected invalid path-remap error, got %v", err)
	}
}

func TestBuildBenchmarkBackfillOptionsParsesFiltersAndPathRemaps(t *testing.T) {
	cmd := &urfavecli.Command{
		Flags: BenchmarkBackfillScoresCommand.Flags,
	}
	oldRoot := filepath.Join(t.TempDir(), "old")
	newRoot := filepath.Join(t.TempDir(), "new")
	storePath := filepath.Join(t.TempDir(), "events.sqlite")
	cmd.Set("suite", "simple_tdd_v1")
	cmd.Set("session", "session-1")
	cmd.Set("agent", "codex")
	cmd.Set("case", "compile_failure_red")
	cmd.Set("template-variant", "current")
	cmd.Set("path-remap", oldRoot+"="+newRoot)
	cmd.Set("force", "true")

	opts, err := buildBenchmarkBackfillOptions(cmd, storePath)
	if err != nil {
		t.Fatalf("buildBenchmarkBackfillOptions: %v", err)
	}
	if opts.MainStorePath != storePath {
		t.Fatalf("expected main store path %q, got %q", storePath, opts.MainStorePath)
	}
	if opts.SuiteID != "simple_tdd_v1" || opts.SessionID != "session-1" || opts.Agent != "codex" || opts.CaseID != "compile_failure_red" || opts.TemplateVariant != "current" {
		t.Fatalf("unexpected backfill filters: %+v", opts)
	}
	if !opts.Force {
		t.Fatal("expected force to be true")
	}
	if len(opts.PathRemaps) != 1 {
		t.Fatalf("expected one path remap, got %+v", opts.PathRemaps)
	}
	if opts.PathRemaps[0].From != oldRoot || opts.PathRemaps[0].To != newRoot {
		t.Fatalf("unexpected path remap %+v", opts.PathRemaps[0])
	}
}

func TestBuildBenchmarkCompareOptionsResolvesFilters(t *testing.T) {
	cmd := &urfavecli.Command{
		Flags: BenchmarkCompareCommand.Flags,
	}
	root := t.TempDir()
	cmd.Set("root", root)
	cmd.Set("suite", "simple_tdd_v1")
	cmd.Set("agent", "codex,claude")
	cmd.Set("case", "compile_failure_red")
	cmd.Set("template-variant", "current")

	opts, err := buildBenchmarkCompareOptions(cmd)
	if err != nil {
		t.Fatalf("buildBenchmarkCompareOptions: %v", err)
	}
	if opts.RootPath != root {
		t.Fatalf("expected root path %q, got %q", root, opts.RootPath)
	}
	if len(opts.Agents) != 2 || opts.Agents[0] != "codex" || opts.Agents[1] != "claude" {
		t.Fatalf("unexpected agents: %+v", opts.Agents)
	}
	if len(opts.CaseIDs) != 1 || opts.CaseIDs[0] != "compile_failure_red" {
		t.Fatalf("unexpected case ids: %+v", opts.CaseIDs)
	}
	if len(opts.TemplateVariants) != 1 || opts.TemplateVariants[0] != "current" {
		t.Fatalf("unexpected template variants: %+v", opts.TemplateVariants)
	}
}
