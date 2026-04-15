package benchmarks

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/common"
)

const benchmarkAgentClaude = "claude"

// loadManualScore reads optional reviewer input and validates supported fields.
func loadManualScore(path string) (*ManualScoreInput, string, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return &ManualScoreInput{}, "", nil
		}
		return nil, "", err
	}
	var manual ManualScoreInput
	if err := common.ReadJSONFile(path, &manual); err != nil {
		return nil, "", fmt.Errorf("load manual score input: %w", err)
	}
	if manual.ErrorActionabilityScore != nil {
		if *manual.ErrorActionabilityScore < 0 || *manual.ErrorActionabilityScore > 3 {
			return nil, "", fmt.Errorf("manual score errorActionabilityScore must be between 0 and 3")
		}
	}
	if strings.TrimSpace(manual.ReviewedAt) != "" {
		if _, err := time.Parse(time.RFC3339, manual.ReviewedAt); err != nil {
			return nil, "", fmt.Errorf("manual score reviewedAt must be RFC3339: %w", err)
		}
	}
	return &manual, path, nil
}

// loadAgentMetadata dispatches to the agent-specific log parser for one run.
func loadAgentMetadata(path string, agentID string) (*AgentMetadata, []string, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, []string{"agent stdout log was not found"}, nil
		}
		return nil, nil, err
	}
	switch agentID {
	case benchmarkAgentClaude:
		metadata, err := loadClaudeAgentMetadata(path)
		return metadata, nil, err
	case "codex", "codex-ollama":
		metadata, err := loadCodexAgentMetadata(path)
		return metadata, nil, err
	default:
		return &AgentMetadata{
			Format:  agentID,
			LogPath: path,
		}, []string{fmt.Sprintf("agent metadata parsing is not implemented for %q", agentID)}, nil
	}
}

// loadClaudeAgentMetadata reads the last Claude result payload from a JSONL log.
func loadClaudeAgentMetadata(path string) (*AgentMetadata, error) {
	lines, err := readNonEmptyLines(path)
	if err != nil {
		return nil, err
	}
	for idx := len(lines) - 1; idx >= 0; idx-- {
		line := lines[idx]
		var payload map[string]any
		if json.Unmarshal([]byte(line), &payload) != nil {
			continue
		}
		if stringValue(payload["type"]) != "result" {
			continue
		}
		return &AgentMetadata{
			Format:               "claude_result",
			LogPath:              path,
			SessionID:            stringValue(payload["session_id"]),
			NumTurns:             intPtrFromAny(payload["num_turns"]),
			DurationMilliseconds: int64PtrFromAny(payload["duration_ms"]),
			TotalCostUSD:         float64PtrFromAny(payload["total_cost_usd"]),
			Usage:                parseAgentUsageMap(anyMap(payload["usage"])),
			ModelUsage:           parseClaudeModelUsage(anyMap(payload["modelUsage"])),
		}, nil
	}
	return &AgentMetadata{Format: "claude_result", LogPath: path}, nil
}

// loadCodexAgentMetadata extracts thread and token usage data from Codex JSONL output.
func loadCodexAgentMetadata(path string) (*AgentMetadata, error) {
	lines, err := readNonEmptyLines(path)
	if err != nil {
		return nil, err
	}
	metadata := &AgentMetadata{
		Format:  "codex_jsonl",
		LogPath: path,
	}
	for _, line := range lines {
		var payload map[string]any
		if json.Unmarshal([]byte(line), &payload) != nil {
			continue
		}
		switch stringValue(payload["type"]) {
		case "thread.started":
			metadata.ThreadID = stringValue(payload["thread_id"])
		case "turn.completed":
			metadata.Usage = parseCodexUsageMap(anyMap(payload["usage"]))
		}
	}
	return metadata, nil
}

// detectInvariantViolation reports whether any locked fixture path changed during the run.
func detectInvariantViolation(seedRoot string, projectRoot string, lockedPaths []string) (bool, error) {
	for _, lockedPath := range lockedPaths {
		seedBytes, err := os.ReadFile(filepath.Join(seedRoot, lockedPath))
		if err != nil {
			return false, fmt.Errorf("read seed locked path %q: %w", lockedPath, err)
		}
		projectBytes, err := os.ReadFile(filepath.Join(projectRoot, lockedPath))
		if err != nil {
			if os.IsNotExist(err) {
				return true, nil
			}
			return false, fmt.Errorf("read project locked path %q: %w", lockedPath, err)
		}
		if !bytes.Equal(seedBytes, projectBytes) {
			return true, nil
		}
	}
	return false, nil
}

// collectEditedFiles returns the relative file paths that differ from the seed fixture.
func collectEditedFiles(seedRoot string, projectRoot string) ([]string, error) {
	seedFiles, err := collectRelativeFiles(seedRoot)
	if err != nil {
		return nil, err
	}
	projectFiles, err := collectRelativeFiles(projectRoot)
	if err != nil {
		return nil, err
	}
	keys := map[string]struct{}{}
	for path := range seedFiles {
		keys[path] = struct{}{}
	}
	for path := range projectFiles {
		keys[path] = struct{}{}
	}
	edited := make([]string, 0)
	for rel := range keys {
		seedBytes, seedOK := seedFiles[rel]
		projectBytes, projectOK := projectFiles[rel]
		if !seedOK || !projectOK || !bytes.Equal(seedBytes, projectBytes) {
			edited = append(edited, rel)
		}
	}
	sort.Strings(edited)
	return edited, nil
}

// collectRelativeFiles snapshots every file under root keyed by slash-normalized relative path.
func collectRelativeFiles(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	return files, err
}

// readNonEmptyLines returns trimmed log lines while skipping separators and blanks.
func readNonEmptyLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	lines := make([]string, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "=====") {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

// parseAgentUsageMap normalizes Claude-style usage payload fields.
func parseAgentUsageMap(payload map[string]any) AgentUsageMetadata {
	return AgentUsageMetadata{
		InputTokens:              int64PtrFromAny(payload["input_tokens"]),
		OutputTokens:             int64PtrFromAny(payload["output_tokens"]),
		CachedInputTokens:        int64PtrFromAny(payload["cached_input_tokens"]),
		CacheCreationInputTokens: int64PtrFromAny(payload["cache_creation_input_tokens"]),
		CacheReadInputTokens:     int64PtrFromAny(payload["cache_read_input_tokens"]),
	}
}

// parseCodexUsageMap normalizes Codex usage payload fields.
func parseCodexUsageMap(payload map[string]any) AgentUsageMetadata {
	return AgentUsageMetadata{
		InputTokens:       int64PtrFromAny(payload["input_tokens"]),
		OutputTokens:      int64PtrFromAny(payload["output_tokens"]),
		CachedInputTokens: int64PtrFromAny(payload["cached_input_tokens"]),
	}
}

// parseClaudeModelUsage normalizes Claude's per-model usage map when present.
func parseClaudeModelUsage(payload map[string]any) map[string]AgentModelUsage {
	if len(payload) == 0 {
		return nil
	}
	result := make(map[string]AgentModelUsage, len(payload))
	for modelName, raw := range payload {
		fields := anyMap(raw)
		result[modelName] = AgentModelUsage{
			InputTokens:              int64PtrFromAny(fields["inputTokens"]),
			OutputTokens:             int64PtrFromAny(fields["outputTokens"]),
			CacheReadInputTokens:     int64PtrFromAny(fields["cacheReadInputTokens"]),
			CacheCreationInputTokens: int64PtrFromAny(fields["cacheCreationInputTokens"]),
			CostUSD:                  float64PtrFromAny(fields["costUSD"]),
		}
	}
	return result
}

// anyMap returns value when it is already a JSON-like object map.
func anyMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	typed, ok := value.(map[string]any)
	if ok {
		return typed
	}
	return nil
}

// stringValue converts string-like JSON fields without failing hard on other types.
func stringValue(value any) string {
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return typed
}

// intPtrFromAny converts a loosely typed numeric field into *int.
func intPtrFromAny(value any) *int {
	if parsed, ok := parseInt64(value); ok {
		result := int(parsed)
		return &result
	}
	return nil
}

// int64PtrFromAny converts a loosely typed numeric field into *int64.
func int64PtrFromAny(value any) *int64 {
	if parsed, ok := parseInt64(value); ok {
		return &parsed
	}
	return nil
}

// float64PtrFromAny converts a loosely typed numeric field into *float64.
func float64PtrFromAny(value any) *float64 {
	switch typed := value.(type) {
	case float64:
		return &typed
	case float32:
		result := float64(typed)
		return &result
	case int:
		result := float64(typed)
		return &result
	case int64:
		result := float64(typed)
		return &result
	case json.Number:
		parsed, err := typed.Float64()
		if err == nil {
			return &parsed
		}
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return &parsed
		}
	}
	return nil
}

// parseInt64 accepts common JSON number encodings used in agent logs.
func parseInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case float32:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed, true
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}
