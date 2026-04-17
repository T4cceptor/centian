package benchmarks

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/agentrunner"
	"github.com/T4cceptor/centian/internal/common"
)

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
	case agentrunner.AgentClaude:
		metadata, err := loadClaudeAgentMetadata(path)
		return metadata, nil, err
	case agentrunner.AgentCodex, agentrunner.AgentCodexOllama:
		metadata, err := loadCodexAgentMetadata(path)
		return metadata, nil, err
	// TODO: missing gemini?
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
		if common.StringValue(payload["type"]) != "result" {
			continue
		}
		return &AgentMetadata{
			Format:               "claude_result",
			LogPath:              path,
			SessionID:            common.StringValue(payload["session_id"]),
			NumTurns:             common.IntPtrFromAny(payload["num_turns"]),
			DurationMilliseconds: common.Int64PtrFromAny(payload["duration_ms"]),
			TotalCostUSD:         common.Float64PtrFromAny(payload["total_cost_usd"]),
			Usage:                parseAgentUsageMap(common.AnyMap(payload["usage"])),
			ModelUsage:           parseClaudeModelUsage(common.AnyMap(payload["modelUsage"])),
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
		switch common.StringValue(payload["type"]) {
		case "thread.started":
			metadata.ThreadID = common.StringValue(payload["thread_id"])
		case "turn.completed":
			metadata.Usage = parseCodexUsageMap(common.AnyMap(payload["usage"]))
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
		InputTokens:              common.Int64PtrFromAny(payload["input_tokens"]),
		OutputTokens:             common.Int64PtrFromAny(payload["output_tokens"]),
		CachedInputTokens:        common.Int64PtrFromAny(payload["cached_input_tokens"]),
		CacheCreationInputTokens: common.Int64PtrFromAny(payload["cache_creation_input_tokens"]),
		CacheReadInputTokens:     common.Int64PtrFromAny(payload["cache_read_input_tokens"]),
	}
}

// parseCodexUsageMap normalizes Codex usage payload fields.
func parseCodexUsageMap(payload map[string]any) AgentUsageMetadata {
	return AgentUsageMetadata{
		InputTokens:       common.Int64PtrFromAny(payload["input_tokens"]),
		OutputTokens:      common.Int64PtrFromAny(payload["output_tokens"]),
		CachedInputTokens: common.Int64PtrFromAny(payload["cached_input_tokens"]),
	}
}

// parseClaudeModelUsage normalizes Claude's per-model usage map when present.
func parseClaudeModelUsage(payload map[string]any) map[string]AgentModelUsage {
	if len(payload) == 0 {
		return nil
	}
	result := make(map[string]AgentModelUsage, len(payload))
	for modelName, raw := range payload {
		fields := common.AnyMap(raw)
		result[modelName] = AgentModelUsage{
			InputTokens:              common.Int64PtrFromAny(fields["inputTokens"]),
			OutputTokens:             common.Int64PtrFromAny(fields["outputTokens"]),
			CacheReadInputTokens:     common.Int64PtrFromAny(fields["cacheReadInputTokens"]),
			CacheCreationInputTokens: common.Int64PtrFromAny(fields["cacheCreationInputTokens"]),
			CostUSD:                  common.Float64PtrFromAny(fields["costUSD"]),
		}
	}
	return result
}
