// Copyright 2025 CentianCLI Contributors.
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

package logging

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
)

// ErrLogsDirNotFound is returned when the Centian logs directory is missing.
var ErrLogsDirNotFound = errors.New("centian logs directory not found")

// ErrNoLogEntries is returned when log files exist but contain no valid entries.
var ErrNoLogEntries = errors.New("no log entries found")

// AnnotatedLogEntry wraps a generic MCP event with contextual metadata used for display.
type AnnotatedLogEntry struct {
	Event      common.McpEventInterface
	SourceFile string
}

// GetLogsDirectory returns the directory where Centian stores log files.
// Tests can override this path by setting the CENTIAN_LOG_DIR environment variable.
func GetLogsDirectory() (string, error) {
	if custom := os.Getenv("CENTIAN_LOG_DIR"); custom != "" {
		return custom, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".centian", "logs"), nil
}

// LoadRecentLogEntries collects log entries from Centian log files, orders them by
// timestamp descending, and enforces an optional limit. A non-positive limit
// returns all available entries.
func LoadRecentLogEntries(limit int) ([]AnnotatedLogEntry, error) {
	logDir, err := GetLogsDirectory()
	if err != nil {
		return nil, err
	}

	// Read directory contents, returning specific error for missing dir.
	fileInfos, err := os.ReadDir(logDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrLogsDirNotFound
		}
		return nil, fmt.Errorf("failed to read log directory: %w", err)
	}

	if len(fileInfos) == 0 {
		return nil, ErrNoLogEntries
	}

	// Read and annotate entries from all files.
	var entries []AnnotatedLogEntry
	for _, entry := range fileInfos {
		if entry.IsDir() {
			continue
		}
		filePath := filepath.Join(logDir, entry.Name())
		fileEntries, err := readLogFile(filePath)
		if err != nil {
			return nil, err
		}
		for i := range fileEntries {
			entries = append(entries, AnnotatedLogEntry{
				Event:      fileEntries[i],
				SourceFile: filePath,
			})
		}
	}

	if len(entries) == 0 {
		return nil, ErrNoLogEntries
	}

	// Sort by timestamp (newest first) for chronological accuracy across files.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Event.GetBaseEvent().Timestamp.After(entries[j].Event.GetBaseEvent().Timestamp)
	})

	// Apply limit if specified (0 = no limit).
	if limit > 0 && len(entries) > limit {
		return entries[:limit], nil
	}

	return entries, nil
}

// FormatDisplayLine converts an AnnotatedLogEntry into a human-readable summary string.
func FormatDisplayLine(entry *AnnotatedLogEntry) string {
	baseEvent := entry.Event.GetBaseEvent()

	status := "ok"
	if !baseEvent.Success {
		status = "fail"
	}

	// Extract transport-specific details.
	command := ""
	if e, ok := entry.Event.(*common.StdioMcpEvent); ok {
		command = e.Command
		if len(e.Args) > 0 {
			command = fmt.Sprintf("%s %s", command, strings.Join(e.Args, " "))
		}
	}

	detail := entry.Event.RawMessage()
	if baseEvent.Error != "" {
		detail = baseEvent.Error
	}
	detail = truncate(detail, 80)

	sessionInfo := baseEvent.SessionID
	if sessionInfo == "" {
		sessionInfo = "-"
	}

	return fmt.Sprintf("%s | %-8s | %-8s | %-4s | %-36s | %s | %s",
		baseEvent.Timestamp.Format(time.RFC3339),
		baseEvent.Direction,
		baseEvent.MessageType,
		status,
		command,
		sessionInfo,
		detail,
	)
}

// readLogFile reads and parses a JSONL log file, returning all valid entries.
// Returns empty slice (not error) if file doesn't exist. Skips malformed lines.
// Supports log lines up to 10MB.
func readLogFile(path string) ([]common.McpEventInterface, error) {
	cleanPath := filepath.Clean(path)
	file, err := os.Open(cleanPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open log file %s: %w", cleanPath, err)
	}
	defer func() { _ = file.Close() }()

	var entries []common.McpEventInterface

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024) // allow larger log lines

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Detect transport type by peeking at the JSON.
		var peek struct {
			Transport string `json:"transport"`
		}
		if err := json.Unmarshal([]byte(line), &peek); err != nil {
			// Skip malformed lines.
			continue
		}

		// Unmarshal to appropriate type based on transport.
		var event common.McpEventInterface
		switch peek.Transport {
		case "stdio":
			var stdioEvent common.StdioMcpEvent
			if err := json.Unmarshal([]byte(line), &stdioEvent); err != nil {
				continue
			}
			event = &stdioEvent
		default:
			// Skip unknown transport types.
			continue
		}

		entries = append(entries, event)
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("failed scanning log file %s: %w", path, err)
	}

	return entries, nil
}

func truncate(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}

	const ellipsis = "..."
	if limit <= len(ellipsis) {
		return ellipsis
	}

	return s[:limit-len(ellipsis)] + ellipsis
}
