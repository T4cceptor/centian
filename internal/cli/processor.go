package cli

import (
	"context"
	"os"

	"github.com/CentianAI/centian-cli/internal/processor"
	"github.com/urfave/cli/v3"
)

// ProcessorCommand provides processor management functionality.
var ProcessorCommand = &cli.Command{
	Name:  "processor",
	Usage: "Manage Centian processors",
	Commands: []*cli.Command{
		ProcessorInitCommand,
	},
}

// ProcessorInitCommand scaffolds a new processor.
var ProcessorInitCommand = &cli.Command{
	Name:        "init",
	Usage:       "centian processor init",
	Description: "Interactively scaffold a new processor.",
	Action:      handleProcessorInit,
}

func handleProcessorInit(_ context.Context, _ *cli.Command) error {
	return processor.RunScaffoldInteractive(os.Stdin, os.Stdout)
}
