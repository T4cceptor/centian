package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/agentrunner"
	"github.com/T4cceptor/centian/internal/benchmarks"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/urfave/cli/v3"
)

// BenchmarkCommand groups local benchmark subcommands.
var BenchmarkCommand = &cli.Command{
	Name:  "benchmark",
	Usage: "Run local taskverification benchmarks",
	Commands: []*cli.Command{
		BenchmarkRunCommand,
		BenchmarkScoreCommand,
		BenchmarkCompareCommand,
		BenchmarkBackfillScoresCommand,
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
			Name:  "codex-ollama-model",
			Usage: "Override Codex Ollama model/profile (" + codexOllamaModelHelp + ")",
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

// BenchmarkScoreCommand scores one preserved benchmark session from disk.
var BenchmarkScoreCommand = &cli.Command{
	Name:  "score",
	Usage: "Score a preserved benchmark session",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "session",
			Usage:    "Path to the preserved benchmark session directory",
			Required: true,
		},
	},
	Action: handleBenchmarkScoreCommand,
}

// BenchmarkCompareCommand compares scored benchmark sessions for one suite.
var BenchmarkCompareCommand = &cli.Command{
	Name:  "compare",
	Usage: "Compare scored benchmark sessions for one suite",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "root",
			Usage:    "Root directory containing benchmark suite session folders",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "suite",
			Usage:    "Benchmark suite id to compare",
			Required: true,
		},
		&cli.StringSliceFlag{
			Name:  "agent",
			Usage: "Limit comparison to specific agents (repeat or comma-separate)",
		},
		&cli.StringSliceFlag{
			Name:  "case",
			Usage: "Limit comparison to specific benchmark cases (repeat or comma-separate)",
		},
		&cli.StringSliceFlag{
			Name:  "template-variant",
			Usage: "Limit comparison to specific template variants (repeat or comma-separate)",
		},
	},
	Action: handleBenchmarkCompareCommand,
}

// BenchmarkBackfillScoresCommand rescans legacy benchmark artifacts and persists DB score snapshots.
var BenchmarkBackfillScoresCommand = &cli.Command{
	Name:  "backfill-scores",
	Usage: "Backfill persisted benchmark run scores from legacy artifacts",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "suite",
			Usage: "Limit backfill to one benchmark suite id",
		},
		&cli.StringFlag{
			Name:  "session",
			Usage: "Limit backfill to one benchmark session id",
		},
		&cli.StringFlag{
			Name:  "agent",
			Usage: "Limit backfill to one agent id",
		},
		&cli.StringFlag{
			Name:  "case",
			Usage: "Limit backfill to one benchmark case id",
		},
		&cli.StringFlag{
			Name:  "template-variant",
			Usage: "Limit backfill to one template variant",
		},
		&cli.StringSliceFlag{
			Name:  "path-remap",
			Usage: "Rewrite one old artifact path prefix using OLD=NEW form (repeatable)",
		},
		&cli.BoolFlag{
			Name:  "force",
			Usage: "Recompute and overwrite existing benchmark run score snapshots",
		},
	},
	Action: handleBenchmarkBackfillScoresCommand,
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

func handleBenchmarkScoreCommand(ctx context.Context, cmd *cli.Command) error {
	options, err := buildBenchmarkScoreOptions(cmd)
	if err != nil {
		return err
	}

	scorer := benchmarks.NewScorer()
	summary, err := scorer.ScoreSession(ctx, options)
	if summary != nil {
		encoded, marshalErr := json.MarshalIndent(summary, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		fmt.Println(string(encoded))
	}
	return err
}

func handleBenchmarkCompareCommand(ctx context.Context, cmd *cli.Command) error {
	options, err := buildBenchmarkCompareOptions(cmd)
	if err != nil {
		return err
	}

	comparer := benchmarks.NewComparer()
	comparison, _, err := comparer.CompareSuite(ctx, options)
	if comparison != nil {
		encoded, marshalErr := json.MarshalIndent(comparison, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		fmt.Println(string(encoded))
	}
	return err
}

func handleBenchmarkBackfillScoresCommand(ctx context.Context, cmd *cli.Command) error {
	storePath, err := config.ResolveEventStorePath(nil)
	if err != nil {
		return err
	}
	options, err := buildBenchmarkBackfillOptions(cmd, storePath)
	if err != nil {
		return err
	}

	service := benchmarks.NewBackfillService()
	result, err := service.BackfillScores(ctx, options)
	if result != nil {
		encoded, marshalErr := json.MarshalIndent(result, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		fmt.Println(string(encoded))
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
	agents := splitCSVValues(cmd.StringSlice("agent"))
	if len(agents) == 0 {
		return nil, fmt.Errorf("at least one agent is required")
	}
	caseIDs := splitCSVValues(cmd.StringSlice("case"))
	startPath := defaultResolutionStart(suitePath)
	outputRoot, err := resolveBenchmarkOutputRoot(cmd, startPath)
	if err != nil {
		return nil, err
	}
	models, codexConfigPath, centianConfigPath, err := resolveBenchmarkModelConfigOptions(cmd, agents)
	if err != nil {
		return nil, err
	}
	templateVariants, err := resolveBenchmarkTemplateVariants(cmd, startPath, centianConfigPath)
	if err != nil {
		return nil, err
	}
	applyDefaultCodexOllamaModel(agents, &models, codexConfigPath)

	return &benchmarks.RunOptions{
		SuitePath:         suitePath,
		CaseIDs:           caseIDs,
		Agents:            agents,
		Repeat:            repeat,
		TemplateVariants:  templateVariants,
		OutputRoot:        outputRoot,
		Timeout:           cmd.Duration("timeout"),
		CentianBinaryPath: binaryPath,
		Models:            models,
		CodexConfigPath:   codexConfigPath,
		CentianConfigPath: centianConfigPath,
		SessionLabel:      defaultBenchmarkSessionLabel(templateVariants, agents),
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

func resolveBenchmarkModelConfigOptions(cmd *cli.Command, agents []string) (benchmarks.AgentModels, string, string, error) {
	models, err := benchmarkAgentModelsFromFlags(
		cmd.String("model"),
		agents,
		cmd.String("claude-model"),
		cmd.String("gemini-model"),
		cmd.String("codex-model"),
		cmd.String("codex-ollama-model"),
	)
	if err != nil {
		return benchmarks.AgentModels{}, "", "", err
	}
	codexConfigPath, err := resolveOptionalPath(cmd.String("codex-config"))
	if err != nil {
		return benchmarks.AgentModels{}, "", "", err
	}
	centianConfigPath, err := resolveOptionalPath(cmd.String("centian-config"))
	if err != nil {
		return benchmarks.AgentModels{}, "", "", err
	}
	return models, codexConfigPath, centianConfigPath, nil
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

func applyDefaultCodexOllamaModel(agents []string, models *benchmarks.AgentModels, codexConfigPath string) {
	if models == nil || codexConfigPath != "" {
		return
	}
	for _, agent := range agents {
		if strings.EqualFold(agent, agentrunner.AgentCodexOllama) && strings.TrimSpace(models.CodexOllama) == "" {
			models.CodexOllama = agentrunner.DefaultCodexOllamaModel
			return
		}
	}
}

func buildBenchmarkScoreOptions(cmd *cli.Command) (*benchmarks.ScoreOptions, error) {
	sessionFlag := strings.TrimSpace(cmd.String("session"))
	if sessionFlag == "" {
		return nil, fmt.Errorf("session path is required")
	}
	sessionPath, err := filepath.Abs(sessionFlag)
	if err != nil {
		return nil, fmt.Errorf("resolve session path: %w", err)
	}
	return &benchmarks.ScoreOptions{SessionPath: sessionPath}, nil
}

func buildBenchmarkCompareOptions(cmd *cli.Command) (*benchmarks.CompareOptions, error) {
	rootFlag := strings.TrimSpace(cmd.String("root"))
	if rootFlag == "" {
		return nil, fmt.Errorf("root path is required")
	}
	rootPath, err := filepath.Abs(rootFlag)
	if err != nil {
		return nil, fmt.Errorf("resolve root path: %w", err)
	}
	suiteID := strings.TrimSpace(cmd.String("suite"))
	if suiteID == "" {
		return nil, fmt.Errorf("suite id is required")
	}
	return &benchmarks.CompareOptions{
		RootPath:         rootPath,
		SuiteID:          suiteID,
		Agents:           splitCSVValues(cmd.StringSlice("agent")),
		CaseIDs:          splitCSVValues(cmd.StringSlice("case")),
		TemplateVariants: splitCSVValues(cmd.StringSlice("template-variant")),
	}, nil
}

func buildBenchmarkBackfillOptions(cmd *cli.Command, storePath string) (*benchmarks.BackfillOptions, error) {
	pathRemaps, err := parsePathRemaps(cmd.StringSlice("path-remap"))
	if err != nil {
		return nil, err
	}
	return &benchmarks.BackfillOptions{
		MainStorePath:   storePath,
		SuiteID:         strings.TrimSpace(cmd.String("suite")),
		SessionID:       strings.TrimSpace(cmd.String("session")),
		Agent:           strings.TrimSpace(cmd.String("agent")),
		CaseID:          strings.TrimSpace(cmd.String("case")),
		TemplateVariant: strings.TrimSpace(cmd.String("template-variant")),
		PathRemaps:      pathRemaps,
		Force:           cmd.Bool("force"),
	}, nil
}

func splitCSVValues(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			value := strings.TrimSpace(part)
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			result = append(result, value)
			seen[value] = struct{}{}
		}
	}
	return result
}

func parseTemplateVariants(values []string) ([]benchmarks.TemplateVariant, error) {
	variants := make([]benchmarks.TemplateVariant, 0, len(values))
	for _, raw := range splitCSVValues(values) {
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

func parsePathRemaps(values []string) ([]benchmarks.PathRemap, error) {
	remaps := make([]benchmarks.PathRemap, 0, len(values))
	for _, raw := range splitCSVValues(values) {
		from, to, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, fmt.Errorf("path-remap %q must use OLD=NEW format", raw)
		}
		from = strings.TrimSpace(from)
		to = strings.TrimSpace(to)
		if from == "" || to == "" {
			return nil, fmt.Errorf("path-remap %q must use OLD=NEW format", raw)
		}
		fromPath, err := filepath.Abs(from)
		if err != nil {
			return nil, fmt.Errorf("resolve path-remap source %q: %w", raw, err)
		}
		toPath, err := filepath.Abs(to)
		if err != nil {
			return nil, fmt.Errorf("resolve path-remap target %q: %w", raw, err)
		}
		remaps = append(remaps, benchmarks.PathRemap{
			From: fromPath,
			To:   toPath,
		})
	}
	return remaps, nil
}

func defaultResolutionStart(suitePath string) string {
	cwd, err := os.Getwd()
	if err == nil {
		return cwd
	}
	return suitePath
}

func defaultBenchmarkSessionLabel(variants []benchmarks.TemplateVariant, agents []string) string {
	if len(variants) != 1 || len(agents) != 1 {
		return ""
	}
	return variants[0].Name + "_" + agents[0] + "_run"
}
