package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/CentianAI/centian-cli/internal/logging"
	"github.com/urfave/cli/v3"
)

const defaultLogDisplayLimit = 50

// LogsCommand displays Centian log entries in descending timestamp order.
var LogsCommand = &cli.Command{
	Name:  "logs",
	Usage: "centian logs [--limit <n>]",
	Description: `Display formatted Centian MCP proxy logs ordered by timestamp (newest first).

Examples:
  centian logs
  centian logs --limit 10`,
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "limit",
			Usage: "Number of log entries to display (0 shows all)",
			Value: defaultLogDisplayLimit,
		},
	},
	Action: handleLogsCommand,
}

func handleLogsCommand(_ context.Context, cmd *cli.Command) error {
	out := cmd.Writer
	if out == nil {
		out = os.Stdout
	}

	errOut := cmd.ErrWriter
	if errOut == nil {
		errOut = os.Stderr
	}

	logDir, err := logging.GetLogsDirectory()
	if err != nil {
		return fmt.Errorf("failed to resolve logs directory: %w", err)
	}

	limit := cmd.Int("limit")
	entries, err := logging.LoadRecentLogEntries(limit)
	if err != nil {
		switch {
		case errors.Is(err, logging.ErrLogsDirNotFound):
			fmt.Fprintf(errOut, "No logs found. Expected directory: %s\n", logDir)
			return nil
		case errors.Is(err, logging.ErrNoLogEntries):
			fmt.Fprintf(errOut, "No log entries recorded yet in %s\n", logDir)
			return nil
		default:
			return err
		}
	}

	fmt.Fprintf(out, "Log directory: %s\n", logDir)
	fmt.Fprintln(out, "TIMESTAMP            | DIRECTION | TYPE     | STAT | COMMAND                              | SESSION ID | DETAILS")

	for _, entry := range entries {
		fmt.Fprintln(out, logging.FormatDisplayLine(entry))
	}

	return nil
}
