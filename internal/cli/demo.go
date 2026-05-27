package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/agentrunner"
	"github.com/urfave/cli/v3"
)

// DemoCommand launches a self-contained Centian demo workspace.
var DemoCommand = &cli.Command{
	Name:  "demo",
	Usage: "Start a self-contained Centian demo workspace",
	Description: `Create a local Centian demo workspace, seed the bundled IT Ops
demo into the event database, start Centian, and open the task run list for
post-hoc inspection.

The --file and --agent flows are deprecated and will likely be moved or removed
in a future release. They remain available for now for legacy demo runs.

Examples:
  centian demo
Deprecated:
  centian demo --file ./demo_scenario.json
  centian demo --agent claude
  centian demo --agent gemini
  centian demo --agent codex --model gpt-5.4-mini
  centian demo --agent codex-ollama --codex-config ~/.codex/config.toml --profile my-local-oss
  centian demo --agent claude --path ./my-demo
`,
	Action: handleDemoCommand,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "agent",
			Aliases: []string{"a"},
			Usage:   "Deprecated: agent to run instead of the static IT Ops demo (v1 supports: claude, gemini, codex, codex-ollama)",
		},
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "Deprecated: synthetic demo scenario JSON file to seed immediately",
		},
		&cli.StringFlag{
			Name:    "path",
			Aliases: []string{"p"},
			Usage:   "Path where the demo workspace should be created",
		},
		&cli.StringFlag{
			Name:    "model",
			Aliases: []string{"m"},
			Usage:   singleModelFlagUsage(),
		},
		&cli.StringFlag{
			Name:  "profile",
			Usage: "Codex Ollama profile from the supplied Codex config (" + codexOllamaProfileHelp + ")",
		},
		&cli.StringFlag{
			Name:  "codex-config",
			Usage: "Base Codex config to copy and patch for codex or codex-ollama runs",
		},
	},
}

// handleDemoCommand resolves demo inputs and runs one local demo session.
func handleDemoCommand(ctx context.Context, cmd *cli.Command) error {
	rootPath, err := resolveDemoRoot(cmd.String("path"))
	if err != nil {
		return err
	}
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve centian executable: %w", err)
	}
	runner := agentrunner.DemoRunner{}
	options := &agentrunner.DemoOptions{
		RootPath:          rootPath,
		CentianBinaryPath: binaryPath,
		Timeout:           5 * time.Minute,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
	}

	var result *agentrunner.DemoResult
	agent := strings.TrimSpace(cmd.String("agent"))
	if agent == "" {
		scenarioPath, scenarioErr := demoScenarioFileFromFlags(cmd)
		if scenarioErr != nil {
			return scenarioErr
		}
		if scenarioPath == "" {
			result, err = runner.RunStaticDemo(ctx, options)
		} else {
			_, _ = fmt.Fprintln(options.Stderr, "warning: centian demo --file is deprecated and will likely be moved or removed in a future release")
			options.ScenarioFilePath = scenarioPath
			result, err = runner.RunSyntheticDemo(ctx, options)
		}
	} else {
		if strings.TrimSpace(cmd.String("file")) != "" {
			return fmt.Errorf("--file cannot be used with --agent")
		}
		_, _ = fmt.Fprintln(options.Stderr, "warning: centian demo --agent is deprecated and will likely be moved or removed in a future release")
		execution, executionErr := demoExecutionFromFlags(cmd)
		if executionErr != nil {
			return executionErr
		}
		options.Execution = execution
		result, err = runner.RunDemo(ctx, options)
	}
	if result != nil {
		fmt.Printf("Demo ready. UI: %s\n", result.UIPublicURL)
		if agent == "" && strings.TrimSpace(cmd.String("file")) == "" {
			if err != nil {
				return err
			}
			return nil
		}
		shutdownErr := promptDemoShutdown(os.Stdin, os.Stdout, result)
		if err != nil {
			return errors.Join(err, shutdownErr)
		}
		return shutdownErr
	}
	if err != nil {
		return err
	}
	return nil
}

func demoScenarioFileFromFlags(cmd *cli.Command) (string, error) {
	for _, flagName := range []string{"model", "profile", "codex-config"} {
		if strings.TrimSpace(cmd.String(flagName)) != "" {
			return "", fmt.Errorf("--%s can only be used with --agent", flagName)
		}
	}
	return resolveOptionalPath(cmd.String("file"))
}

// demoExecutionFromFlags converts demo agent flags into one normalized execution config.
func demoExecutionFromFlags(cmd *cli.Command) (agentrunner.AgentExecutionOptions, error) {
	agent := strings.TrimSpace(cmd.String("agent"))
	exec := agentrunner.AgentExecutionOptions{
		Agent:   agent,
		Profile: strings.TrimSpace(cmd.String("profile")),
	}
	if model := strings.TrimSpace(cmd.String("model")); model != "" {
		if strings.EqualFold(agent, agentrunner.AgentCodexOllama) {
			return agentrunner.AgentExecutionOptions{}, fmt.Errorf("--model is not supported for codex-ollama; use --profile")
		}
		exec.Model = normalizeCLIModel(agent, model)
	}
	codexConfigPath, err := resolveOptionalPath(cmd.String("codex-config"))
	if err != nil {
		return agentrunner.AgentExecutionOptions{}, err
	}
	exec.CodexConfigPath = codexConfigPath
	if strings.EqualFold(agent, agentrunner.AgentCodexOllama) {
		if codexConfigPath == "" {
			return agentrunner.AgentExecutionOptions{}, fmt.Errorf("codex-ollama requires --codex-config")
		}
		if exec.Profile == "" {
			return agentrunner.AgentExecutionOptions{}, fmt.Errorf("codex-ollama requires --profile")
		}
	} else if exec.Profile != "" {
		return agentrunner.AgentExecutionOptions{}, fmt.Errorf("--profile can only be used with --agent codex-ollama")
	}
	return agentrunner.NormalizeExecutionOptions(exec)
}

// promptDemoShutdown optionally stops the demo Centian process after the agent run ends.
func promptDemoShutdown(input io.Reader, output io.Writer, result *agentrunner.DemoResult) error {
	if result == nil || result.PID <= 0 {
		return nil
	}
	_, _ = fmt.Fprint(output, "Shut down the Centian server now? (Y/n): ")
	if !shouldShutdownDemo(input) {
		return nil
	}
	process, err := os.FindProcess(result.PID)
	if err != nil {
		return fmt.Errorf("resolve centian process: %w", err)
	}
	if err := process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("shut down centian server: %w", err)
	}
	return nil
}

// shouldShutdownDemo interprets an interactive shutdown prompt response.
func shouldShutdownDemo(input io.Reader) bool {
	if input == nil {
		return true
	}
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return true
	}
	switch value := normalizePromptAnswer(line); value {
	case "", "y", "yes":
		return true
	default:
		return false
	}
}

// normalizePromptAnswer trims and lowercases a yes/no prompt response.
func normalizePromptAnswer(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// resolveDemoRoot returns the explicit demo path or the default workspace location.
func resolveDemoRoot(flagValue string) (string, error) {
	if flagValue != "" {
		return filepath.Abs(flagValue)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return filepath.Abs(filepath.Join(cwd, ".centian", "demo"))
}

// resolveOptionalPath absolutizes a non-empty path flag and leaves empty values unset.
func resolveOptionalPath(flagValue string) (string, error) {
	value := strings.TrimSpace(flagValue)
	if value == "" {
		return "", nil
	}
	return filepath.Abs(value)
}
