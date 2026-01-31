// Copyright 2025 Centian Contributors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at.
//
//     http://www.apache.org/licenses/LICENSE-2.0.
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/T4cceptor/centian/internal/logging"
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
			_, _ = fmt.Fprintf(errOut, "No logs found. Expected directory: %s\n", logDir)
			return nil
		case errors.Is(err, logging.ErrNoLogEntries):
			_, _ = fmt.Fprintf(errOut, "No log entries recorded yet in %s\n", logDir)
			return nil
		default:
			return err
		}
	}

	_, _ = fmt.Fprintf(out, "Log directory: %s\n", logDir)
	_, _ = fmt.Fprintln(out, "TIMESTAMP            | DIRECTION | TYPE     | STAT | COMMAND                              | SESSION ID | DETAILS")

	for i := range entries {
		_, _ = fmt.Fprintln(out, logging.FormatDisplayLine(&(entries[i])))
	}

	return nil
}
