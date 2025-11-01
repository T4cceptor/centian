package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CentianAI/centian-cli/internal/logging"
	urfavecli "github.com/urfave/cli/v3"
)

// TestHandleLogsCommandOutputsEntries verifies that the logs command correctly
// reads and displays log entries from a JSONL file.
//
// Given: a logs directory with one test log entry containing session "sess-123"
// When: handleLogsCommand is executed
// Then: output contains the log directory path and session ID, with no errors
func TestHandleLogsCommandOutputsEntries(t *testing.T) {
	// Given: a temporary logs directory
	tempDir := t.TempDir()
	defer func() {
		os.Unsetenv("CENTIAN_LOG_DIR")
	}()
	os.Setenv("CENTIAN_LOG_DIR", tempDir)

	// Given: a log file with a log entry
	writeTestLogFile(t, filepath.Join(tempDir, "requests_2025-01-05.jsonl"), []logging.LogEntry{
		{
			Timestamp:   time.Date(2025, 1, 5, 10, 0, 0, 0, time.UTC),
			Direction:   "request",
			MessageType: "request",
			Command:     "npx",
			Args:        []string{"@server"},
			SessionID:   "sess-123",
			Message:     "ping",
			Success:     true,
		},
	})

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}

	// Given: a urfavecli.Command to be run with handleLogsCommand
	cmd := &urfavecli.Command{
		Writer:    outBuf,
		ErrWriter: errBuf,
		Flags: []urfavecli.Flag{
			&urfavecli.IntFlag{
				Name:  "limit",
				Value: defaultLogDisplayLimit,
			},
		},
	}

	// When: running handleLogsCommand
	if err := handleLogsCommand(context.Background(), cmd); err != nil {
		t.Fatalf("handleLogsCommand returned error: %v", err)
	}

	output := outBuf.String()
	// Then: stdout contains log directory path and session ID
	if !strings.Contains(output, "Log directory") {
		t.Fatalf("expected log directory header in output, got: %s", output)
	}
	if !strings.Contains(output, "sess-123") {
		t.Fatalf("expected session ID in output, got: %s", output)
	}
	// Then: stderr is empty (no errors)
	if errBuf.Len() != 0 {
		t.Fatalf("expected no error output, got: %s", errBuf.String())
	}
}

// TestHandleLogsCommandNoDirectory verifies that the logs command handles
// missing log directories gracefully with a helpful error message.
//
// Given: CENTIAN_LOG_DIR points to a non-existent directory
// When: handleLogsCommand is executed
// Then: command succeeds but writes "No logs found" message to stderr
func TestHandleLogsCommandNoDirectory(t *testing.T) {
	// Given: a non-existent logs directory path
	tempDir := filepath.Join(t.TempDir(), "missing")
	defer func() {
		os.Unsetenv("CENTIAN_LOG_DIR")
	}()
	os.Setenv("CENTIAN_LOG_DIR", tempDir)

	// Given: output buffers to capture command output
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}

	// Given: a CLI command configured to use the buffers
	cmd := &urfavecli.Command{
		Writer:    outBuf,
		ErrWriter: errBuf,
		Flags: []urfavecli.Flag{
			&urfavecli.IntFlag{
				Name:  "limit",
				Value: defaultLogDisplayLimit,
			},
		},
	}

	// When: running handleLogsCommand
	if err := handleLogsCommand(context.Background(), cmd); err != nil {
		t.Fatalf("handleLogsCommand returned error: %v", err)
	}

	// Then: stderr contains a helpful message about missing logs
	if errBuf.Len() == 0 {
		t.Fatal("expected helpful message when logs directory is missing")
	}

	if !strings.Contains(errBuf.String(), "No logs found") {
		t.Fatalf("expected missing logs message, got: %s", errBuf.String())
	}
}

// writeTestLogFile creates a JSONL log file at the specified path with the given entries.
// Each entry is encoded as a single JSON line, matching the format used by the logging system.
// The function creates parent directories if needed and truncates any existing file.
func writeTestLogFile(t *testing.T, path string, entries []logging.LogEntry) {
	t.Helper()

	// Create parent directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create log directory: %v", err)
	}

	// Create or truncate the log file
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to open log file: %v", err)
	}
	defer file.Close()

	// Write each entry as a JSON line
	encoder := json.NewEncoder(file)
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			t.Fatalf("failed to encode log entry: %v", err)
		}
	}
}
