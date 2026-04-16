package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/agentrunner"
	"github.com/T4cceptor/centian/internal/benchmarks"
	"github.com/T4cceptor/centian/internal/common"
	"github.com/urfave/cli/v3"
)

// BenchmarkCommand groups local benchmark subcommands.
var BenchmarkCommand = &cli.Command{
	Name:  "benchmark",
	Usage: "Run local taskverification benchmarks",
	Commands: []*cli.Command{
		BenchmarkRunCommand,
	},
}

// BenchmarkRunCommand executes one benchmark suite locally and preserves raw artifacts.
var BenchmarkRunCommand = &cli.Command{
	Name:  "run",
	Usage: "Run a benchmark suite locally",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "suite",
			Usage:    "Path to the benchmark suite root",
			Required: true,
		},
		&cli.StringSliceFlag{
			Name:  "case",
			Usage: "Benchmark case ids to execute (repeat or comma-separate)",
		},
		&cli.StringSliceFlag{
			Name:     "agent",
			Usage:    "Agents to execute (repeat or comma-separate)",
			Required: true,
		},
		&cli.IntFlag{
			Name:  "repeat",
			Usage: "Number of attempts per matrix cell",
			Value: 1,
		},
		&cli.StringSliceFlag{
			Name:  "template-dir",
			Usage: "Template variant in name=path form (repeatable)",
		},
		&cli.DurationFlag{
			Name:  "timeout",
			Usage: "Per-run timeout",
			Value: 15 * time.Minute,
		},
		&cli.StringFlag{
			Name:  "output-root",
			Usage: "Output root for preserved benchmark artifacts",
		},
		&cli.StringFlag{
			Name:    "model",
			Aliases: []string{"m"},
			Usage:   singleModelFlagUsage(),
		},
		&cli.StringFlag{
			Name:  "profile",
			Usage: "Codex Ollama profile for single-agent codex-ollama runs (" + codexOllamaProfileHelp + ")",
		},
		&cli.StringFlag{
			Name:  "claude-model",
			Usage: "Override Claude model (" + claudeModelHelp + ")",
		},
		&cli.StringFlag{
			Name:  "gemini-model",
			Usage: "Override Gemini model (" + geminiModelHelp + ")",
		},
		&cli.StringFlag{
			Name:  "codex-model",
			Usage: "Override Codex model (" + codexModelHelp + ")",
		},
		&cli.StringFlag{
			Name:  "codex-config",
			Usage: "Base Codex config to copy and patch for codex or codex-ollama runs",
		},
		&cli.StringFlag{
			Name:  "centian-config",
			Usage: "Base Centian config to copy and patch for benchmark runs",
		},
		&cli.BoolFlag{
			Name:  "keep-centian-running",
			Usage: "Print the benchmark UI URL and prompt whether to shut down the Centian server after the agent finishes",
		},
	},
	Action: handleBenchmarkRunCommand,
}

func handleBenchmarkRunCommand(ctx context.Context, cmd *cli.Command) error {
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current centian executable: %w", err)
	}
	options, err := buildBenchmarkRunOptions(cmd, binaryPath)
	if err != nil {
		return err
	}

	options.OnCentianReady = func(run *benchmarks.RunManifest) {
		if run == nil || strings.TrimSpace(run.UIPublicURL) == "" {
			return
		}
		fmt.Printf(
			"Benchmark UI (%s/%s/%s attempt %03d): %s\n",
			run.TemplateVariant.Name,
			run.AgentID,
			run.CaseID,
			run.Attempt,
			run.UIPublicURL,
		)
	}
	if cmd.Bool("keep-centian-running") {
		options.AfterRun = func(run *benchmarks.RunManifest) error {
			if run == nil || run.CentianPID <= 0 {
				return nil
			}
			fmt.Printf("Agent run finished. UI: %s\n", run.UIPublicURL)
			return promptDemoShutdown(os.Stdin, os.Stdout, &agentrunner.DemoResult{
				PID:         run.CentianPID,
				UIPublicURL: run.UIPublicURL,
			})
		}
	}

	runner := benchmarks.NewRunner()
	session, err := runner.RunSuite(ctx, options)
	if session != nil {
		fmt.Printf("Benchmark session: %s\n", session.InvocationDir)
		fmt.Printf("Status: %s\n", session.Status)
	}
	return err
}

func buildBenchmarkRunOptions(cmd *cli.Command, binaryPath string) (*benchmarks.RunOptions, error) {
	suitePath, err := resolveBenchmarkSuitePath(cmd)
	if err != nil {
		return nil, err
	}
	repeat, err := resolveBenchmarkRepeat(cmd)
	if err != nil {
		return nil, err
	}
	agents := common.NormalizeCSVList(cmd.StringSlice("agent"))
	if len(agents) == 0 {
		return nil, fmt.Errorf("at least one agent is required")
	}
	caseIDs := common.NormalizeCSVList(cmd.StringSlice("case"))
	startPath := defaultResolutionStart(suitePath)
	outputRoot, err := resolveBenchmarkOutputRoot(cmd, startPath)
	if err != nil {
		return nil, err
	}
	executions, _, centianConfigPath, err := resolveBenchmarkModelConfigOptions(cmd, agents)
	if err != nil {
		return nil, err
	}
	templateVariants, err := resolveBenchmarkTemplateVariants(cmd, startPath, centianConfigPath)
	if err != nil {
		return nil, err
	}
	if err := validateCodexOllamaOptions(executions); err != nil {
		return nil, err
	}

	return &benchmarks.RunOptions{
		SuitePath:         suitePath,
		CaseIDs:           caseIDs,
		Executions:        executions,
		Repeat:            repeat,
		TemplateVariants:  templateVariants,
		OutputRoot:        outputRoot,
		Timeout:           cmd.Duration("timeout"),
		CentianBinaryPath: binaryPath,
		CentianConfigPath: centianConfigPath,
		SessionLabel:      defaultBenchmarkSessionLabel(templateVariants, executions),
	}, nil
}

func resolveBenchmarkSuitePath(cmd *cli.Command) (string, error) {
	suiteFlag := strings.TrimSpace(cmd.String("suite"))
	if suiteFlag == "" {
		return "", fmt.Errorf("suite path is required")
	}
	suitePath, err := filepath.Abs(suiteFlag)
	if err != nil {
		return "", fmt.Errorf("resolve suite path: %w", err)
	}
	return suitePath, nil
}

func resolveBenchmarkRepeat(cmd *cli.Command) (int, error) {
	repeat := cmd.Int("repeat")
	if !cmd.IsSet("repeat") && repeat == 0 {
		repeat = 1
	}
	if repeat <= 0 {
		return 0, fmt.Errorf("repeat must be greater than zero")
	}
	return repeat, nil
}

func resolveBenchmarkOutputRoot(cmd *cli.Command, startPath string) (string, error) {
	outputRoot := strings.TrimSpace(cmd.String("output-root"))
	if outputRoot == "" {
		return benchmarks.ResolveDefaultOutputRoot(startPath)
	}
	resolved, err := filepath.Abs(outputRoot)
	if err != nil {
		return "", fmt.Errorf("resolve output root: %w", err)
	}
	return resolved, nil
}

func resolveBenchmarkModelConfigOptions(cmd *cli.Command, agents []string) ([]agentrunner.AgentExecutionOptions, string, string, error) {
	codexConfigPath, err := resolveOptionalPath(cmd.String("codex-config"))
	if err != nil {
		return nil, "", "", err
	}
	executions, err := benchmarkExecutionsFromFlags(
		cmd.String("model"),
		cmd.String("profile"),
		agents,
		cmd.String("claude-model"),
		cmd.String("gemini-model"),
		cmd.String("codex-model"),
		codexConfigPath,
	)
	if err != nil {
		return nil, "", "", err
	}
	centianConfigPath, err := resolveOptionalPath(cmd.String("centian-config"))
	if err != nil {
		return nil, "", "", err
	}
	return executions, codexConfigPath, centianConfigPath, nil
}

func resolveBenchmarkTemplateVariants(cmd *cli.Command, startPath, centianConfigPath string) ([]benchmarks.TemplateVariant, error) {
	templateVariants, err := parseTemplateVariants(cmd.StringSlice("template-dir"))
	if err != nil {
		return nil, err
	}
	if len(templateVariants) == 0 && centianConfigPath != "" {
		templateVariants, err = benchmarks.ResolveTemplateVariantsFromCentianConfig(centianConfigPath)
		if err != nil {
			return nil, err
		}
	}
	if len(templateVariants) == 0 {
		return benchmarks.ResolveDefaultTemplateVariants(startPath)
	}
	return templateVariants, nil
}

func validateCodexOllamaOptions(executions []agentrunner.AgentExecutionOptions) error {
	for _, exec := range executions {
		if !strings.EqualFold(exec.Agent, agentrunner.AgentCodexOllama) {
			continue
		}
		if strings.TrimSpace(exec.CodexConfigPath) == "" {
			return fmt.Errorf("codex-ollama requires --codex-config")
		}
		if strings.TrimSpace(exec.Profile) == "" {
			return fmt.Errorf("codex-ollama requires an explicit profile; use --profile")
		}
	}
	return nil
}

func parseTemplateVariants(values []string) ([]benchmarks.TemplateVariant, error) {
	variants := make([]benchmarks.TemplateVariant, 0, len(values))
	for _, raw := range common.NormalizeCSVList(values) {
		name, path, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, fmt.Errorf("template-dir %q must use name=path format", raw)
		}
		name = strings.TrimSpace(name)
		path = strings.TrimSpace(path)
		if name == "" || path == "" {
			return nil, fmt.Errorf("template-dir %q must use name=path format", raw)
		}
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve template-dir %q: %w", raw, err)
		}
		variants = append(variants, benchmarks.TemplateVariant{
			Name:      name,
			SourceDir: absolutePath,
		})
	}
	return variants, nil
}

func defaultResolutionStart(suitePath string) string {
	cwd, err := os.Getwd()
	if err == nil {
		return cwd
	}
	return suitePath
}

func defaultBenchmarkSessionLabel(variants []benchmarks.TemplateVariant, executions []agentrunner.AgentExecutionOptions) string {
	if len(variants) != 1 || len(executions) != 1 {
		return ""
	}
	return variants[0].Name + "_" + executions[0].Agent + "_run"
}
