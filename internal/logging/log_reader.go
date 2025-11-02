// Copyright 2025 CentianCLI Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
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
)

// ErrLogsDirNotFound is returned when the Centian logs directory is missing.
var ErrLogsDirNotFound = errors.New("centian logs directory not found")

// ErrNoLogEntries is returned when log files exist but contain no valid entries.
var ErrNoLogEntries = errors.New("no log entries found")

// AnnotatedLogEntry wraps a LogEntry with contextual metadata used for display.
type AnnotatedLogEntry struct {
	LogEntry
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

	// Read directory contents, returning specific error for missing dir
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

	// Read and annotate entries from all files
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
		for _, logEntry := range fileEntries {
			entries = append(entries, AnnotatedLogEntry{
				LogEntry:   logEntry,
				SourceFile: filePath,
			})
		}
	}

	if len(entries) == 0 {
		return nil, ErrNoLogEntries
	}

	// Sort by timestamp (newest first) for chronological accuracy across files
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	// Apply limit if specified (0 = no limit)
	if limit > 0 && len(entries) > limit {
		return entries[:limit], nil
	}

	return entries, nil
}

// FormatDisplayLine converts an AnnotatedLogEntry into a human-readable summary string.
func FormatDisplayLine(entry AnnotatedLogEntry) string {
	status := "ok"
	if !entry.Success {
		status = "fail"
	}

	command := entry.Command
	if len(entry.Args) > 0 {
		command = fmt.Sprintf("%s %s", command, strings.Join(entry.Args, " "))
	}

	detail := entry.Message
	if entry.Error != "" {
		detail = entry.Error
	}
	detail = truncate(detail, 80)

	sessionInfo := entry.SessionID
	if sessionInfo == "" {
		sessionInfo = "-"
	}

	return fmt.Sprintf("%s | %-8s | %-8s | %-4s | %-36s | %s | %s",
		entry.Timestamp.Format(time.RFC3339),
		entry.Direction,
		entry.MessageType,
		status,
		command,
		sessionInfo,
		detail,
	)
}

// readLogFile reads and parses a JSONL log file, returning all valid entries.
// Returns empty slice (not error) if file doesn't exist. Skips malformed lines.
// Supports log lines up to 10MB.
func readLogFile(path string) ([]LogEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open log file %s: %w", path, err)
	}
	defer file.Close()

	var entries []LogEntry

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024) // allow larger log lines

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Skip malformed lines but continue processing the rest of the file.
			continue
		}

		entries = append(entries, entry)
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
