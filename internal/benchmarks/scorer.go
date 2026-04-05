package benchmarks

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/persistence"
)

const (
	manualScoreFileName = "manual_score.json"
	scoreVersion        = "v1"
)

// ScoreOptions configures scoring for one preserved benchmark session.
type ScoreOptions struct {
	SessionPath string
}

// ManualScoreInput stores optional reviewer-supplied scoring inputs.
type ManualScoreInput struct {
	ErrorActionabilityScore *int   `json:"errorActionabilityScore,omitempty"`
	Notes                   string `json:"notes,omitempty"`
	ReviewedBy              string `json:"reviewedBy,omitempty"`
	ReviewedAt              string `json:"reviewedAt,omitempty"`
}

// AgentMetadata captures parsed usage/session data from one agent run log.
type AgentMetadata struct {
	Format               string                     `json:"format,omitempty"`
	LogPath              string                     `json:"logPath,omitempty"`
	SessionID            string                     `json:"sessionId,omitempty"`
	ThreadID             string                     `json:"threadId,omitempty"`
	NumTurns             *int                       `json:"numTurns,omitempty"`
	DurationMilliseconds *int64                     `json:"durationMilliseconds,omitempty"`
	TotalCostUSD         *float64                   `json:"totalCostUsd,omitempty"`
	Usage                AgentUsageMetadata         `json:"usage,omitempty"`
	ModelUsage           map[string]AgentModelUsage `json:"modelUsage,omitempty"`
}

// AgentUsageMetadata stores normalized token metadata observed in agent logs.
type AgentUsageMetadata struct {
	InputTokens              *int64 `json:"inputTokens,omitempty"`
	OutputTokens             *int64 `json:"outputTokens,omitempty"`
	CachedInputTokens        *int64 `json:"cachedInputTokens,omitempty"`
	CacheCreationInputTokens *int64 `json:"cacheCreationInputTokens,omitempty"`
	CacheReadInputTokens     *int64 `json:"cacheReadInputTokens,omitempty"`
}

// AgentModelUsage stores model-specific usage details when the agent exposes them.
type AgentModelUsage struct {
	InputTokens              *int64   `json:"inputTokens,omitempty"`
	OutputTokens             *int64   `json:"outputTokens,omitempty"`
	CacheReadInputTokens     *int64   `json:"cacheReadInputTokens,omitempty"`
	CacheCreationInputTokens *int64   `json:"cacheCreationInputTokens,omitempty"`
	CostUSD                  *float64 `json:"costUsd,omitempty"`
}

// RunScorecard stores derived metrics for one concrete benchmark run.
type RunScorecard struct {
	SuiteID          string              `json:"suiteId"`
	SuiteName        string              `json:"suiteName,omitempty"`
	CaseID           string              `json:"caseId"`
	CaseName         string              `json:"caseName,omitempty"`
	TemplateID       string              `json:"templateId"`
	TemplateName     string              `json:"templateName,omitempty"`
	TemplateVariant  string              `json:"templateVariant"`
	Agent            string              `json:"agent"`
	Attempt          int                 `json:"attempt"`
	RunManifestPath  string              `json:"runManifestPath"`
	SessionPath      string              `json:"sessionPath"`
	EventStoreMode   string              `json:"eventStoreMode,omitempty"`
	EventStorePath   string              `json:"eventStorePath,omitempty"`
	RequestLogPath   string              `json:"requestLogPath,omitempty"`
	AgentStdoutPath  string              `json:"agentStdoutPath,omitempty"`
	AgentStderrPath  string              `json:"agentStderrPath,omitempty"`
	ManualScorePath  string              `json:"manualScorePath,omitempty"`
	RawStatus        string              `json:"rawStatus"`
	LatestTaskRunID  string              `json:"latestTaskRunId,omitempty"`
	LinkedTaskRunIDs []string            `json:"linkedTaskRunIds,omitempty"`
	Outcome          ScorecardOutcome    `json:"outcome"`
	Process          ScorecardProcess    `json:"process"`
	Efficiency       ScorecardEfficiency `json:"efficiency"`
	Manual           ScorecardManual     `json:"manual"`
	AgentMetadata    *AgentMetadata      `json:"agentMetadata,omitempty"`
	ScoreVersion     string              `json:"scoreVersion"`
	GeneratedAt      time.Time           `json:"generatedAt"`
	Warnings         []string            `json:"warnings,omitempty"`
	Errors           []string            `json:"errors,omitempty"`
}

// ScorecardOutcome contains outcome metrics for one run.
type ScorecardOutcome struct {
	CompletedSuccessfully   bool `json:"completedSuccessfully"`
	FinalVerificationPassed bool `json:"finalVerificationPassed"`
	FirstPassSuccess        bool `json:"firstPassSuccess"`
	RestartOccurred         bool `json:"restartOccurred"`
	FailOccurred            bool `json:"failOccurred"`
	TimeoutOccurred         bool `json:"timeoutOccurred"`
	InvariantViolation      bool `json:"invariantViolation"`
}

// ScorecardProcess contains process metrics for one run.
type ScorecardProcess struct {
	FailedTaskToolCalls       int `json:"failedTaskToolCalls"`
	FailedDownstreamToolCalls int `json:"failedDownstreamToolCalls"`
	TotalTaskToolCalls        int `json:"totalTaskToolCalls"`
	TotalDownstreamToolCalls  int `json:"totalDownstreamToolCalls"`
	RestartCount              int `json:"restartCount"`
	FailCount                 int `json:"failCount"`
	TimeoutCount              int `json:"timeoutCount"`
}

// ScorecardEfficiency contains efficiency metrics for one run.
type ScorecardEfficiency struct {
	WallClockSeconds     float64  `json:"wallClockSeconds"`
	TotalToolCalls       int      `json:"totalToolCalls"`
	InputTokens          *int64   `json:"inputTokens,omitempty"`
	OutputTokens         *int64   `json:"outputTokens,omitempty"`
	EditedFilesCount     int      `json:"editedFilesCount"`
	EditedFiles          []string `json:"editedFiles,omitempty"`
	ObservedCommandCalls int      `json:"observedCommandCalls"`
}

// ScorecardManual contains optional reviewer-supplied metrics.
type ScorecardManual struct {
	ErrorActionabilityScore *int   `json:"errorActionabilityScore,omitempty"`
	ErrorActionabilityNotes string `json:"errorActionabilityNotes,omitempty"`
}

// SessionSummary stores comparison-friendly derived metrics for one scored session.
type SessionSummary struct {
	ScoreVersion       string                   `json:"scoreVersion"`
	SessionPath        string                   `json:"sessionPath"`
	SuiteID            string                   `json:"suiteId"`
	GeneratedAt        time.Time                `json:"generatedAt"`
	RunCount           int                      `json:"runCount"`
	ScoredRunCount     int                      `json:"scoredRunCount"`
	FailedToScoreCount int                      `json:"failedToScoreCount"`
	Runs               []RunSummaryRow          `json:"runs"`
	Aggregates         SessionSummaryAggregates `json:"aggregates"`
}

// SessionSummaryAggregates contains grouped score summaries.
type SessionSummaryAggregates struct {
	ByCase             []AggregateSummary `json:"byCase"`
	ByAgent            []AggregateSummary `json:"byAgent"`
	ByTemplateVariant  []AggregateSummary `json:"byTemplateVariant"`
	ByCaseAgentVariant []AggregateSummary `json:"byCaseAgentVariant"`
}

// RunSummaryRow is the compact per-run row used by the session summary.
type RunSummaryRow struct {
	SessionPath               string         `json:"sessionPath,omitempty"`
	CaseID                    string         `json:"caseId"`
	Agent                     string         `json:"agent"`
	TemplateVariant           string         `json:"templateVariant"`
	Attempt                   int            `json:"attempt"`
	EventStoreMode            string         `json:"eventStoreMode,omitempty"`
	EventStorePath            string         `json:"eventStorePath,omitempty"`
	RawStatus                 string         `json:"rawStatus"`
	LatestTaskRunID           string         `json:"latestTaskRunId,omitempty"`
	LinkedTaskRunIDs          []string       `json:"linkedTaskRunIds,omitempty"`
	Scored                    bool           `json:"scored"`
	CompletedSuccessfully     bool           `json:"completedSuccessfully"`
	FinalVerificationPassed   bool           `json:"finalVerificationPassed"`
	FirstPassSuccess          bool           `json:"firstPassSuccess"`
	InvariantViolation        bool           `json:"invariantViolation"`
	RestartOccurred           bool           `json:"restartOccurred"`
	FailOccurred              bool           `json:"failOccurred"`
	TimeoutOccurred           bool           `json:"timeoutOccurred"`
	WallClockSeconds          float64        `json:"wallClockSeconds"`
	TotalToolCalls            int            `json:"totalToolCalls"`
	InputTokens               *int64         `json:"inputTokens,omitempty"`
	OutputTokens              *int64         `json:"outputTokens,omitempty"`
	FailedTaskToolCalls       int            `json:"failedTaskToolCalls"`
	FailedDownstreamToolCalls int            `json:"failedDownstreamToolCalls"`
	EditedFilesCount          int            `json:"editedFilesCount"`
	ErrorActionabilityScore   *int           `json:"errorActionabilityScore,omitempty"`
	AgentMetadata             *AgentMetadata `json:"agentMetadata,omitempty"`
	Warnings                  []string       `json:"warnings,omitempty"`
	Errors                    []string       `json:"errors,omitempty"`
}

// AggregateSummary stores aggregate comparison metrics for one grouping key.
type AggregateSummary struct {
	Key                             string   `json:"key"`
	SessionPath                     string   `json:"sessionPath,omitempty"`
	CaseID                          string   `json:"caseId,omitempty"`
	Agent                           string   `json:"agent,omitempty"`
	TemplateVariant                 string   `json:"templateVariant,omitempty"`
	RunCount                        int      `json:"runCount"`
	ScoredRunCount                  int      `json:"scoredRunCount"`
	SuccessRate                     float64  `json:"successRate"`
	FirstPassSuccessRate            float64  `json:"firstPassSuccessRate"`
	FinalVerificationPassRate       float64  `json:"finalVerificationPassRate"`
	InvariantViolationRate          float64  `json:"invariantViolationRate"`
	RestartFailTimeoutRate          float64  `json:"restartFailTimeoutRate"`
	MedianWallClockSeconds          float64  `json:"medianWallClockSeconds"`
	MedianTotalToolCalls            float64  `json:"medianTotalToolCalls"`
	MedianInputTokens               float64  `json:"medianInputTokens"`
	MedianOutputTokens              float64  `json:"medianOutputTokens"`
	MedianFailedTaskToolCalls       float64  `json:"medianFailedTaskToolCalls"`
	MedianFailedDownstreamToolCalls float64  `json:"medianFailedDownstreamToolCalls"`
	MedianEditedFilesCount          float64  `json:"medianEditedFilesCount"`
	ManualActionabilityCount        int      `json:"manualActionabilityCount"`
	AverageManualActionabilityScore *float64 `json:"averageManualActionabilityScore,omitempty"`
}

// Scorer computes benchmark scorecards and session summaries from preserved artifacts.
type Scorer struct {
	Now func() time.Time
}

type scoreRunContext struct {
	caseDef  *CaseDefinition
	caseRoot string
}

// NewScorer returns a benchmark scorer with default local behavior.
func NewScorer() *Scorer {
	return &Scorer{Now: time.Now}
}

// ScoreSession computes derived scorecards for one preserved benchmark session.
func (s *Scorer) ScoreSession(_ context.Context, opts *ScoreOptions) (*SessionSummary, error) {
	s = s.withDefaults()
	if opts == nil {
		return nil, fmt.Errorf("score options are required")
	}
	sessionDir := strings.TrimSpace(opts.SessionPath)
	if sessionDir == "" {
		return nil, fmt.Errorf("session path is required")
	}
	sessionDir, err := filepath.Abs(sessionDir)
	if err != nil {
		return nil, fmt.Errorf("resolve session path: %w", err)
	}
	info, err := os.Stat(sessionDir)
	if err != nil {
		return nil, fmt.Errorf("stat session path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("session path %q must be a directory", sessionDir)
	}

	session, err := loadSessionManifest(sessionDir)
	if err != nil {
		return nil, err
	}
	storePath := eventStorePathForSession(session)
	store, err := persistence.NewSQLiteStore(storePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = store.Close() }()

	service := newLiveQueryService(store, s.Now)
	return service.scoreSessionManifest(context.Background(), session)
}

func (s *Scorer) withDefaults() *Scorer {
	if s == nil {
		return NewScorer()
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	return s
}

func loadSessionManifest(sessionDir string) (*SessionManifest, error) {
	var session SessionManifest
	if err := readJSONFile(filepath.Join(sessionDir, sessionFileName), &session); err != nil {
		return nil, fmt.Errorf("load session manifest: %w", err)
	}
	if strings.TrimSpace(session.SuiteID) == "" {
		return nil, fmt.Errorf("session manifest must define suiteId")
	}
	if strings.TrimSpace(session.SuitePath) == "" {
		return nil, fmt.Errorf("session manifest must define suitePath")
	}
	return &session, nil
}

func eventStorePathForSession(session *SessionManifest) string {
	if session != nil {
		for idx := range session.Runs {
			runPath := filepath.Join(session.InvocationDir, session.Runs[idx].RelativeRunDir, runFileName)
			run, err := loadRunManifest(runPath)
			if err != nil {
				continue
			}
			if trimmed := strings.TrimSpace(run.ArtifactPaths.EventStorePath); trimmed != "" {
				return trimmed
			}
		}
	}
	storePath, err := config.ResolveEventStorePath(nil)
	if err != nil {
		return ""
	}
	return storePath
}

func loadCaseContexts(suiteRoot string, suite *SuiteDefinition) (map[string]scoreRunContext, error) {
	result := make(map[string]scoreRunContext, len(suite.Cases))
	for _, ref := range suite.Cases {
		caseDef, err := LoadCase(suiteRoot, ref)
		if err != nil {
			return nil, err
		}
		caseRoot := filepath.Join(suiteRoot, ref.Path)
		result[ref.ID] = scoreRunContext{
			caseDef:  caseDef,
			caseRoot: caseRoot,
		}
	}
	return result, nil
}

func loadManualScore(path string) (*ManualScoreInput, string, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return &ManualScoreInput{}, "", nil
		}
		return nil, "", err
	}
	var manual ManualScoreInput
	if err := readJSONFile(path, &manual); err != nil {
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

func loadAgentMetadata(path string, agentID string) (*AgentMetadata, []string, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, []string{"agent stdout log was not found"}, nil
		}
		return nil, nil, err
	}
	switch agentID {
	case "claude":
		metadata, err := loadClaudeAgentMetadata(path)
		return metadata, nil, err
	case "codex":
		metadata, err := loadCodexAgentMetadata(path)
		return metadata, nil, err
	default:
		return &AgentMetadata{
			Format:  agentID,
			LogPath: path,
		}, []string{fmt.Sprintf("agent metadata parsing is not implemented for %q", agentID)}, nil
	}
}

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
		metadata := &AgentMetadata{
			Format:               "claude_result",
			LogPath:              path,
			SessionID:            stringValue(payload["session_id"]),
			NumTurns:             intPtrFromAny(payload["num_turns"]),
			DurationMilliseconds: int64PtrFromAny(payload["duration_ms"]),
			TotalCostUSD:         float64PtrFromAny(payload["total_cost_usd"]),
			Usage:                parseAgentUsageMap(anyMap(payload["usage"])),
			ModelUsage:           parseClaudeModelUsage(anyMap(payload["modelUsage"])),
		}
		return metadata, nil
	}
	return &AgentMetadata{Format: "claude_result", LogPath: path}, nil
}

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

func parseAgentUsageMap(payload map[string]any) AgentUsageMetadata {
	return AgentUsageMetadata{
		InputTokens:              int64PtrFromAny(payload["input_tokens"]),
		OutputTokens:             int64PtrFromAny(payload["output_tokens"]),
		CachedInputTokens:        int64PtrFromAny(payload["cached_input_tokens"]),
		CacheCreationInputTokens: int64PtrFromAny(payload["cache_creation_input_tokens"]),
		CacheReadInputTokens:     int64PtrFromAny(payload["cache_read_input_tokens"]),
	}
}

func parseCodexUsageMap(payload map[string]any) AgentUsageMetadata {
	return AgentUsageMetadata{
		InputTokens:       int64PtrFromAny(payload["input_tokens"]),
		OutputTokens:      int64PtrFromAny(payload["output_tokens"]),
		CachedInputTokens: int64PtrFromAny(payload["cached_input_tokens"]),
	}
}

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

func secondsBetween(start time.Time, end time.Time) float64 {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start).Seconds()
}

func buildRunSummaryRow(entry SessionRunManifestEntry, run *RunManifest, scorecard *RunScorecard, errors []string, warnings []string) RunSummaryRow {
	row := RunSummaryRow{
		SessionPath:     scorecardSessionPath(run, scorecard),
		CaseID:          entry.CaseID,
		Agent:           entry.AgentID,
		TemplateVariant: entry.TemplateVariant,
		Attempt:         entry.Attempt,
		Errors:          append([]string(nil), errors...),
		Warnings:        append([]string(nil), warnings...),
	}
	if run != nil {
		row.RawStatus = run.Status
		row.EventStoreMode = run.ArtifactPaths.EventStoreMode
		row.EventStorePath = run.ArtifactPaths.EventStorePath
		row.LatestTaskRunID = run.LatestTaskRunID
		row.LinkedTaskRunIDs = append([]string(nil), run.LinkedTaskRunIDs...)
	}
	if scorecard == nil {
		return row
	}
	row.Scored = true
	row.SessionPath = scorecard.SessionPath
	row.EventStoreMode = scorecard.EventStoreMode
	row.EventStorePath = scorecard.EventStorePath
	row.RawStatus = scorecard.RawStatus
	row.LatestTaskRunID = scorecard.LatestTaskRunID
	row.LinkedTaskRunIDs = append([]string(nil), scorecard.LinkedTaskRunIDs...)
	row.CompletedSuccessfully = scorecard.Outcome.CompletedSuccessfully
	row.FinalVerificationPassed = scorecard.Outcome.FinalVerificationPassed
	row.FirstPassSuccess = scorecard.Outcome.FirstPassSuccess
	row.InvariantViolation = scorecard.Outcome.InvariantViolation
	row.RestartOccurred = scorecard.Outcome.RestartOccurred
	row.FailOccurred = scorecard.Outcome.FailOccurred
	row.TimeoutOccurred = scorecard.Outcome.TimeoutOccurred
	row.WallClockSeconds = scorecard.Efficiency.WallClockSeconds
	row.TotalToolCalls = scorecard.Efficiency.TotalToolCalls
	row.InputTokens = scorecard.Efficiency.InputTokens
	row.OutputTokens = scorecard.Efficiency.OutputTokens
	row.FailedTaskToolCalls = scorecard.Process.FailedTaskToolCalls
	row.FailedDownstreamToolCalls = scorecard.Process.FailedDownstreamToolCalls
	row.EditedFilesCount = scorecard.Efficiency.EditedFilesCount
	row.ErrorActionabilityScore = scorecard.Manual.ErrorActionabilityScore
	row.AgentMetadata = scorecard.AgentMetadata
	row.Warnings = append(row.Warnings, scorecard.Warnings...)
	row.Errors = append(row.Errors, scorecard.Errors...)
	return row
}

func scorecardSessionPath(run *RunManifest, scorecard *RunScorecard) string {
	if scorecard != nil {
		return scorecard.SessionPath
	}
	if run == nil || strings.TrimSpace(run.ArtifactPaths.RunDir) == "" {
		return ""
	}
	sessionPath := filepath.Clean(run.ArtifactPaths.RunDir)
	for i := 0; i < 5; i++ {
		sessionPath = filepath.Dir(sessionPath)
	}
	return sessionPath
}

func compareRunRows(a RunSummaryRow, b RunSummaryRow) bool {
	if a.TemplateVariant != b.TemplateVariant {
		return a.TemplateVariant < b.TemplateVariant
	}
	if a.Agent != b.Agent {
		return a.Agent < b.Agent
	}
	if a.CaseID != b.CaseID {
		return a.CaseID < b.CaseID
	}
	return a.Attempt < b.Attempt
}

func buildAggregates(rows []RunSummaryRow) SessionSummaryAggregates {
	return SessionSummaryAggregates{
		ByCase:  aggregateRows(rows, func(row RunSummaryRow) aggregateKey { return aggregateKey{Key: row.CaseID, CaseID: row.CaseID} }),
		ByAgent: aggregateRows(rows, func(row RunSummaryRow) aggregateKey { return aggregateKey{Key: row.Agent, Agent: row.Agent} }),
		ByTemplateVariant: aggregateRows(rows, func(row RunSummaryRow) aggregateKey {
			return aggregateKey{Key: row.TemplateVariant, TemplateVariant: row.TemplateVariant}
		}),
		ByCaseAgentVariant: aggregateRows(rows, func(row RunSummaryRow) aggregateKey {
			return aggregateKey{
				Key:    fmt.Sprintf("%s|%s|%s", row.CaseID, row.Agent, row.TemplateVariant),
				CaseID: row.CaseID, Agent: row.Agent, TemplateVariant: row.TemplateVariant,
			}
		}),
	}
}

type aggregateKey struct {
	SessionPath     string
	Key             string
	CaseID          string
	Agent           string
	TemplateVariant string
}

func aggregateRows(rows []RunSummaryRow, keyFn func(RunSummaryRow) aggregateKey) []AggregateSummary {
	grouped := map[string][]RunSummaryRow{}
	keys := map[string]aggregateKey{}
	for _, row := range rows {
		key := keyFn(row)
		grouped[key.Key] = append(grouped[key.Key], row)
		keys[key.Key] = key
	}
	summaries := make([]AggregateSummary, 0, len(grouped))
	for key, group := range grouped {
		summary := AggregateSummary{
			Key:                             key,
			SessionPath:                     keys[key].SessionPath,
			CaseID:                          keys[key].CaseID,
			Agent:                           keys[key].Agent,
			TemplateVariant:                 keys[key].TemplateVariant,
			RunCount:                        len(group),
			ScoredRunCount:                  len(group),
			SuccessRate:                     rate(group, func(row RunSummaryRow) bool { return row.CompletedSuccessfully }),
			FirstPassSuccessRate:            rate(group, func(row RunSummaryRow) bool { return row.FirstPassSuccess }),
			FinalVerificationPassRate:       rate(group, func(row RunSummaryRow) bool { return row.FinalVerificationPassed }),
			InvariantViolationRate:          rate(group, func(row RunSummaryRow) bool { return row.InvariantViolation }),
			RestartFailTimeoutRate:          rate(group, func(row RunSummaryRow) bool { return row.RestartOccurred || row.FailOccurred || row.TimeoutOccurred }),
			MedianWallClockSeconds:          medianFloat(extractFloat(group, func(row RunSummaryRow) float64 { return row.WallClockSeconds })),
			MedianTotalToolCalls:            medianFloat(extractInt(group, func(row RunSummaryRow) int { return row.TotalToolCalls })),
			MedianInputTokens:               medianFloat(extractOptionalInt64(group, func(row RunSummaryRow) *int64 { return row.InputTokens })),
			MedianOutputTokens:              medianFloat(extractOptionalInt64(group, func(row RunSummaryRow) *int64 { return row.OutputTokens })),
			MedianFailedTaskToolCalls:       medianFloat(extractInt(group, func(row RunSummaryRow) int { return row.FailedTaskToolCalls })),
			MedianFailedDownstreamToolCalls: medianFloat(extractInt(group, func(row RunSummaryRow) int { return row.FailedDownstreamToolCalls })),
			MedianEditedFilesCount:          medianFloat(extractInt(group, func(row RunSummaryRow) int { return row.EditedFilesCount })),
			ManualActionabilityCount:        count(group, func(row RunSummaryRow) bool { return row.ErrorActionabilityScore != nil }),
		}
		if avg, ok := averageManualScore(group); ok {
			summary.AverageManualActionabilityScore = &avg
		}
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Key < summaries[j].Key })
	return summaries
}

func rate(rows []RunSummaryRow, predicate func(RunSummaryRow) bool) float64 {
	if len(rows) == 0 {
		return 0
	}
	return float64(count(rows, predicate)) / float64(len(rows))
}

func count(rows []RunSummaryRow, predicate func(RunSummaryRow) bool) int {
	total := 0
	for _, row := range rows {
		if predicate(row) {
			total++
		}
	}
	return total
}

func extractInt(rows []RunSummaryRow, valueFn func(RunSummaryRow) int) []float64 {
	values := make([]float64, 0, len(rows))
	for _, row := range rows {
		values = append(values, float64(valueFn(row)))
	}
	return values
}

func extractFloat(rows []RunSummaryRow, valueFn func(RunSummaryRow) float64) []float64 {
	values := make([]float64, 0, len(rows))
	for _, row := range rows {
		values = append(values, valueFn(row))
	}
	return values
}

func extractOptionalInt64(rows []RunSummaryRow, valueFn func(RunSummaryRow) *int64) []float64 {
	values := make([]float64, 0, len(rows))
	for _, row := range rows {
		value := valueFn(row)
		if value == nil {
			continue
		}
		values = append(values, float64(*value))
	}
	return values
}

func medianFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}

func averageManualScore(rows []RunSummaryRow) (float64, bool) {
	total := 0
	count := 0
	for _, row := range rows {
		if row.ErrorActionabilityScore == nil {
			continue
		}
		total += *row.ErrorActionabilityScore
		count++
	}
	if count == 0 {
		return 0, false
	}
	return float64(total) / float64(count), true
}

func agentUsageInputTokens(metadata *AgentMetadata) *int64 {
	if metadata == nil {
		return nil
	}
	return metadata.Usage.InputTokens
}

func agentUsageOutputTokens(metadata *AgentMetadata) *int64 {
	if metadata == nil {
		return nil
	}
	return metadata.Usage.OutputTokens
}

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

func stringValue(value any) string {
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return typed
}

func intPtrFromAny(value any) *int {
	if parsed, ok := parseInt64(value); ok {
		result := int(parsed)
		return &result
	}
	return nil
}

func int64PtrFromAny(value any) *int64 {
	if parsed, ok := parseInt64(value); ok {
		return &parsed
	}
	return nil
}

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

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
