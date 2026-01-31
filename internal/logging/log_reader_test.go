package logging

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
)

func TestLoadRecentLogEntriesOrdersByTimestamp(t *testing.T) {
	// Given: two log files with entries having different timestamps.
	tempDir := t.TempDir()
	original := os.Getenv("CENTIAN_LOG_DIR")
	os.Setenv("CENTIAN_LOG_DIR", tempDir)
	defer func() {
		if original == "" {
			os.Unsetenv("CENTIAN_LOG_DIR")
			return
		}
		os.Setenv("CENTIAN_LOG_DIR", original)
	}()

	event1 := common.MCPEvent{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:        time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Transport:        "stdio",
			RequestID:        "req-older",
			Direction:        common.DirectionClientToServer,
			MessageType:      common.MessageTypeRequest,
			Success:          true,
			ProcessingErrors: make(map[string]error),
		},
		Routing: common.RoutingContext{
			DownstreamCommand: "npx",
			Args:              []string{"pkg"},
		},
	}
	event1.SetRawMessage("older")
	event2 := common.MCPEvent{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:        time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC),
			Transport:        "stdio",
			RequestID:        "req-newer",
			Direction:        common.DirectionServerToClient,
			MessageType:      common.MessageTypeResponse,
			Success:          true,
			ProcessingErrors: make(map[string]error),
		},
		Routing: common.RoutingContext{
			DownstreamCommand: "npx",
			Args:              []string{"pkg"},
		},
	}
	event2.SetRawMessage("newer")
	writeLogFile(t, tempDir, "requests_2025-01-01.jsonl", []common.MCPEvent{event1})
	writeLogFile(t, tempDir, "requests_2025-01-02.jsonl", []common.MCPEvent{event2})

	// When: loading all recent log entries with no limit.
	entries, err := LoadRecentLogEntries(0)
	if err != nil {
		t.Fatalf("LoadRecentLogEntries returned error: %v", err)
	}

	// Then: entries are sorted by timestamp with newest first.
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Event.GetBaseEvent().RequestID != "req-newer" {
		t.Errorf("expected newest entry first, got %s", entries[0].Event.GetBaseEvent().RequestID)
	}
}

func TestLoadRecentLogEntriesLimit(t *testing.T) {
	// Given: a log file with 2 entries.
	tempDir := t.TempDir()
	original := os.Getenv("CENTIAN_LOG_DIR")
	os.Setenv("CENTIAN_LOG_DIR", tempDir)
	defer func() {
		if original == "" {
			os.Unsetenv("CENTIAN_LOG_DIR")
			return
		}
		os.Setenv("CENTIAN_LOG_DIR", original)
	}()

	event1 := common.MCPEvent{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:        time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC),
			Transport:        "stdio",
			RequestID:        "req-3",
			Direction:        common.DirectionSystem,
			MessageType:      common.MessageTypeSystem,
			Success:          true,
			ProcessingErrors: make(map[string]error),
		},
		Routing: common.RoutingContext{
			DownstreamCommand: "test",
		},
	}
	event1.SetRawMessage("up")
	event2 := common.MCPEvent{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:        time.Date(2025, 1, 4, 12, 0, 0, 0, time.UTC),
			Transport:        "stdio",
			RequestID:        "req-4",
			Direction:        common.DirectionClientToServer,
			MessageType:      common.MessageTypeRequest,
			Success:          true,
			ProcessingErrors: make(map[string]error),
		},
		Routing: common.RoutingContext{
			DownstreamCommand: "npx",
		},
	}
	event2.SetRawMessage("latest")
	writeLogFile(t, tempDir, "requests_2025-01-03.jsonl", []common.MCPEvent{
		event1,
		event2,
	})

	// When: loading recent entries with limit=1.
	entries, err := LoadRecentLogEntries(1)
	if err != nil {
		t.Fatalf("LoadRecentLogEntries returned error: %v", err)
	}

	// Then: only the most recent entry is returned.
	if len(entries) != 1 {
		t.Fatalf("expected limit to return 1 entry, got %d", len(entries))
	}

	if entries[0].Event.GetBaseEvent().RequestID != "req-4" {
		t.Errorf("expected most recent entry, got %s", entries[0].Event.GetBaseEvent().RequestID)
	}
}

func TestLoadRecentLogEntriesMissingDir(t *testing.T) {
	// Given: a log directory that doesn't exist.
	tempDir := filepath.Join(t.TempDir(), "missing")
	original := os.Getenv("CENTIAN_LOG_DIR")
	os.Setenv("CENTIAN_LOG_DIR", tempDir)
	defer func() {
		if original == "" {
			os.Unsetenv("CENTIAN_LOG_DIR")
			return
		}
		os.Setenv("CENTIAN_LOG_DIR", original)
	}()

	// When: attempting to load log entries.
	_, err := LoadRecentLogEntries(0)

	// Then: ErrLogsDirNotFound is returned.
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
	if !errors.Is(err, ErrLogsDirNotFound) {
		t.Fatalf("expected ErrLogsDirNotFound, got %v", err)
	}
}

func TestFormatDisplayLine(t *testing.T) {
	// Given: an annotated log entry with session ID and command details.
	event := &common.MCPEvent{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:        time.Date(2025, 1, 1, 15, 4, 5, 0, time.UTC),
			Transport:        "stdio",
			Direction:        common.DirectionClientToServer,
			MessageType:      common.MessageTypeRequest,
			SessionID:        "sess-1",
			Success:          true,
			ProcessingErrors: make(map[string]error),
		},
		Routing: common.RoutingContext{
			DownstreamCommand: "npx",
			Args:              []string{"@mcp/server"},
		},
	}
	entry := AnnotatedLogEntry{
		Event:      event,
		SourceFile: "/tmp/log",
	}
	event.SetRawMessage("ping")

	// When: formatting the entry for display.
	line := FormatDisplayLine(&entry)

	// Then: the formatted line contains session ID and command.
	if !strings.Contains(line, "sess-1") {
		t.Fatalf("expected session ID in formatted line: %s", line)
	}
	if !strings.Contains(line, "npx @mcp/server") {
		t.Fatalf("expected command in formatted line: %s", line)
	}
}

func writeLogFile(t *testing.T, dir, name string, entries []common.MCPEvent) {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create directory for logs: %v", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to open log file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for i := range entries {
		if err := encoder.Encode(entries[i]); err != nil {
			t.Fatalf("failed to encode log entry: %v", err)
		}
	}
}

func TestTruncate_Details(t *testing.T) {
	tests := []struct {
		longString string
		limit      int
		expected   string
	}{
		{"short", 500, "short"},
		{"not long, but too long", 2, "..."},
		{"something long", 9, "someth..."},
		{"something", 9, "something"},
	}
	for _, test := range tests {
		// Given: a longer string, and a limit.
		// When: calling truncate providing longString and limit.
		result := truncate(test.longString, test.limit)
		// Then: result is as expected.
		if result != test.expected {
			t.Fatalf("Expected: %s, got: %s", test.expected, result)
		}
	}
}
