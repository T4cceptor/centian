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
	Description: `Create a local Centian demo workspace, start Centian, launch a supported
coding agent against it, and print the live UI URL.

Examples:
  centian demo --agent claude
  centian demo --agent gemini
  centian demo --agent codex --model gpt-5.4-mini
  centian demo --agent claude --path ./my-demo
`,
	Action: handleDemoCommand,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "agent",
			Aliases:  []string{"a"},
			Usage:    "Agent to run for the demo (v1 supports: claude, gemini, codex)",
			Required: true,
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
	},
}

func handleDemoCommand(ctx context.Context, cmd *cli.Command) error {
	if cmd.String("agent") == "" {
		return fmt.Errorf("--agent is required")
	}
	rootPath, err := resolveDemoRoot(cmd.String("path"))
	if err != nil {
		return err
	}
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve centian executable: %w", err)
	}
	claudeModel := agentrunner.DefaultClaudeModel
	geminiModel := agentrunner.DefaultGeminiModel
	codexModel := agentrunner.DefaultCodexModel
	if model := strings.TrimSpace(cmd.String("model")); model != "" {
		switch agent := strings.ToLower(strings.TrimSpace(cmd.String("agent"))); agent {
		case agentrunner.AgentClaude:
			claudeModel = normalizeCLIModel(agent, model)
		case agentrunner.AgentGemini:
			geminiModel = normalizeCLIModel(agent, model)
		case agentrunner.AgentCodex:
			codexModel = normalizeCLIModel(agent, model)
		default:
			return fmt.Errorf("unsupported agent %q; cannot apply --model", cmd.String("agent"))
		}
	}

	runner := agentrunner.DemoRunner{}
	result, err := runner.RunDemo(ctx, &agentrunner.DemoOptions{
		Agent:             cmd.String("agent"),
		RootPath:          rootPath,
		CentianBinaryPath: binaryPath,
		Timeout:           5 * time.Minute,
		ClaudeModel:       claudeModel,
		GeminiModel:       geminiModel,
		CodexModel:        codexModel,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
	})
	if result != nil {
		fmt.Printf("Agent run finished. UI: %s\n", result.UIPublicURL)
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

func normalizePromptAnswer(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

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
