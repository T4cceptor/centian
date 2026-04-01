package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
  centian demo --agent claude --path ./my-demo
`,
	Action: handleDemoCommand,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "agent",
			Aliases:  []string{"a"},
			Usage:    "Agent to run for the demo (v1 supports: claude, gemini)",
			Required: true,
		},
		&cli.StringFlag{
			Name:    "path",
			Aliases: []string{"p"},
			Usage:   "Path where the demo workspace should be created",
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

	runner := agentrunner.DemoRunner{}
	_, err = runner.RunDemo(ctx, &agentrunner.DemoOptions{
		Agent:             cmd.String("agent"),
		RootPath:          rootPath,
		CentianBinaryPath: binaryPath,
		Timeout:           5 * time.Minute,
		ClaudeModel:       agentrunner.DefaultClaudeModel,
		GeminiModel:       agentrunner.DefaultGeminiModel,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
	})
	if err != nil {
		return err
	}
	fmt.Println("Agent run finished. Centian is still running.")
	return nil
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
