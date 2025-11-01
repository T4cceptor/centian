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

func TestHandleLogsCommandOutputsEntries(t *testing.T) {
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
	// Then: the file containing expected data
	if !strings.Contains(output, "Log directory") {
		t.Fatalf("expected log directory header in output, got: %s", output)
	}
	if !strings.Contains(output, "sess-123") {
		t.Fatalf("expected session ID in output, got: %s", output)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("expected no error output, got: %s", errBuf.String())
	}
}

func TestHandleLogsCommandNoDirectory(t *testing.T) {
	tempDir := filepath.Join(t.TempDir(), "missing")
	defer func() {
		os.Unsetenv("CENTIAN_LOG_DIR")
	}()
	os.Setenv("CENTIAN_LOG_DIR", tempDir)

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}

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

	if err := handleLogsCommand(context.Background(), cmd); err != nil {
		t.Fatalf("handleLogsCommand returned error: %v", err)
	}

	if errBuf.Len() == 0 {
		t.Fatal("expected helpful message when logs directory is missing")
	}

	if !strings.Contains(errBuf.String(), "No logs found") {
		t.Fatalf("expected missing logs message, got: %s", errBuf.String())
	}
}

func writeTestLogFile(t *testing.T, path string, entries []logging.LogEntry) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create log directory: %v", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to open log file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			t.Fatalf("failed to encode log entry: %v", err)
		}
	}
}
