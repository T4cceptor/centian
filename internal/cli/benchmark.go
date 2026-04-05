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
			Name:  "claude-model",
			Usage: "Override Claude model",
		},
		&cli.StringFlag{
			Name:  "gemini-model",
			Usage: "Override Gemini model",
		},
		&cli.StringFlag{
			Name:  "codex-model",
			Usage: "Override Codex model",
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

func buildBenchmarkRunOptions(cmd *cli.Command, binaryPath string) (*benchmarks.RunOptions, error) {
	suiteFlag := strings.TrimSpace(cmd.String("suite"))
	if suiteFlag == "" {
		return nil, fmt.Errorf("suite path is required")
	}
	suitePath, err := filepath.Abs(suiteFlag)
	if err != nil {
		return nil, fmt.Errorf("resolve suite path: %w", err)
	}
	repeat := cmd.Int("repeat")
	if !cmd.IsSet("repeat") && repeat == 0 {
		repeat = 1
	}
	if repeat <= 0 {
		return nil, fmt.Errorf("repeat must be greater than zero")
	}

	agents := splitCSVValues(cmd.StringSlice("agent"))
	if len(agents) == 0 {
		return nil, fmt.Errorf("at least one agent is required")
	}
	caseIDs := splitCSVValues(cmd.StringSlice("case"))
	templateVariants, err := parseTemplateVariants(cmd.StringSlice("template-dir"))
	if err != nil {
		return nil, err
	}

	startPath := defaultResolutionStart(suitePath)
	if len(templateVariants) == 0 {
		templateVariants, err = benchmarks.ResolveDefaultTemplateVariants(startPath)
		if err != nil {
			return nil, err
		}
	}
	outputRoot := strings.TrimSpace(cmd.String("output-root"))
	if outputRoot == "" {
		outputRoot, err = benchmarks.ResolveDefaultOutputRoot(startPath)
		if err != nil {
			return nil, err
		}
	} else {
		outputRoot, err = filepath.Abs(outputRoot)
		if err != nil {
			return nil, fmt.Errorf("resolve output root: %w", err)
		}
	}

	return &benchmarks.RunOptions{
		SuitePath:         suitePath,
		CaseIDs:           caseIDs,
		Agents:            agents,
		Repeat:            repeat,
		TemplateVariants:  templateVariants,
		OutputRoot:        outputRoot,
		Timeout:           cmd.Duration("timeout"),
		CentianBinaryPath: binaryPath,
		Models: benchmarks.AgentModels{
			Claude: strings.TrimSpace(cmd.String("claude-model")),
			Gemini: strings.TrimSpace(cmd.String("gemini-model")),
			Codex:  strings.TrimSpace(cmd.String("codex-model")),
		},
	}, nil
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

func defaultResolutionStart(suitePath string) string {
	cwd, err := os.Getwd()
	if err == nil {
		if _, rootErr := benchmarks.FindRepoRoot(cwd); rootErr == nil {
			return cwd
		}
	}
	return suitePath
}
