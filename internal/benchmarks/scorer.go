package benchmarks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	manualScoreFileName = "manual_score.json"
	scoreVersion        = "v1"
)

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
	SelectedModel        string                     `json:"selectedModel,omitempty"`
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
	SelectedModel    string              `json:"selectedModel,omitempty"`
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
	SelectedModel             string         `json:"selectedModel,omitempty"`
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
	TotalTaskToolCalls        int            `json:"totalTaskToolCalls"`
	TotalDownstreamToolCalls  int            `json:"totalDownstreamToolCalls"`
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
	TotalTaskToolCalls              int      `json:"totalTaskToolCalls"`
	TotalDownstreamToolCalls        int      `json:"totalDownstreamToolCalls"`
	ManualActionabilityCount        int      `json:"manualActionabilityCount"`
	AverageManualActionabilityScore *float64 `json:"averageManualActionabilityScore,omitempty"`
}

// scoreRunContext holds the resolved case definition and fixture root for scoring.
type scoreRunContext struct {
	caseDef  *CaseDefinition
	caseRoot string
}

// loadSessionManifest reads and minimally validates a preserved benchmark session manifest.
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

// loadCaseContexts expands each suite case into the data needed for scoring helpers.
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

// secondsBetween returns elapsed seconds when both timestamps are valid and ordered.
func secondsBetween(start time.Time, end time.Time) float64 {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start).Seconds()
}

// buildRunSummaryRow merges session entry, run manifest, and scorecard data into one aggregate row.
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
		row.SelectedModel = strings.TrimSpace(run.SelectedModel)
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
	row.SelectedModel = scorecard.SelectedModel
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
	row.TotalTaskToolCalls = scorecard.Process.TotalTaskToolCalls
	row.TotalDownstreamToolCalls = scorecard.Process.TotalDownstreamToolCalls
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

// scorecardSessionPath prefers the scored session path and falls back to deriving it from run artifacts.
func scorecardSessionPath(run *RunManifest, scorecard *RunScorecard) string {
	if scorecard != nil {
		return scorecard.SessionPath
	}
	if run == nil || strings.TrimSpace(run.ArtifactPaths.RunDir) == "" {
		return ""
	}
	return sessionPathFromRun(run)
}

// compareRunRows provides stable ordering for session and comparison run tables.
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

// buildAggregates computes the standard aggregate groupings used across benchmark views.
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

// aggregateKey preserves group identity plus dimensions needed in the aggregate output.
type aggregateKey struct {
	SessionPath     string
	Key             string
	CaseID          string
	Agent           string
	TemplateVariant string
}

// aggregateRows groups run rows by key and derives rollup metrics for each group.
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
		scoredGroup := filterScoredRows(group)
		summary := AggregateSummary{
			Key:                             key,
			SessionPath:                     keys[key].SessionPath,
			CaseID:                          keys[key].CaseID,
			Agent:                           keys[key].Agent,
			TemplateVariant:                 keys[key].TemplateVariant,
			RunCount:                        len(group),
			ScoredRunCount:                  len(scoredGroup),
			SuccessRate:                     rate(scoredGroup, func(row RunSummaryRow) bool { return row.CompletedSuccessfully }),
			FirstPassSuccessRate:            rate(scoredGroup, func(row RunSummaryRow) bool { return row.FirstPassSuccess }),
			FinalVerificationPassRate:       rate(scoredGroup, func(row RunSummaryRow) bool { return row.FinalVerificationPassed }),
			InvariantViolationRate:          rate(scoredGroup, func(row RunSummaryRow) bool { return row.InvariantViolation }),
			RestartFailTimeoutRate:          rate(scoredGroup, func(row RunSummaryRow) bool { return row.RestartOccurred || row.FailOccurred || row.TimeoutOccurred }),
			MedianWallClockSeconds:          medianFloat(extractFloat(scoredGroup, func(row RunSummaryRow) float64 { return row.WallClockSeconds })),
			MedianTotalToolCalls:            medianFloat(extractInt(scoredGroup, func(row RunSummaryRow) int { return row.TotalToolCalls })),
			MedianInputTokens:               medianFloat(extractOptionalInt64(scoredGroup, func(row RunSummaryRow) *int64 { return row.InputTokens })),
			MedianOutputTokens:              medianFloat(extractOptionalInt64(scoredGroup, func(row RunSummaryRow) *int64 { return row.OutputTokens })),
			MedianFailedTaskToolCalls:       medianFloat(extractInt(scoredGroup, func(row RunSummaryRow) int { return row.FailedTaskToolCalls })),
			MedianFailedDownstreamToolCalls: medianFloat(extractInt(scoredGroup, func(row RunSummaryRow) int { return row.FailedDownstreamToolCalls })),
			MedianEditedFilesCount:          medianFloat(extractInt(scoredGroup, func(row RunSummaryRow) int { return row.EditedFilesCount })),
			TotalTaskToolCalls:              sumIntRows(scoredGroup, func(row RunSummaryRow) int { return row.TotalTaskToolCalls }),
			TotalDownstreamToolCalls:        sumIntRows(scoredGroup, func(row RunSummaryRow) int { return row.TotalDownstreamToolCalls }),
			ManualActionabilityCount:        count(scoredGroup, func(row RunSummaryRow) bool { return row.ErrorActionabilityScore != nil }),
		}
		if avg, ok := averageManualScore(scoredGroup); ok {
			summary.AverageManualActionabilityScore = &avg
		}
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Key < summaries[j].Key })
	return summaries
}

// filterScoredRows drops runs that never produced a usable score snapshot.
func filterScoredRows(rows []RunSummaryRow) []RunSummaryRow {
	scored := make([]RunSummaryRow, 0, len(rows))
	for _, row := range rows {
		if row.Scored {
			scored = append(scored, row)
		}
	}
	return scored
}

// rate computes the fraction of rows matching predicate.
func rate(rows []RunSummaryRow, predicate func(RunSummaryRow) bool) float64 {
	if len(rows) == 0 {
		return 0
	}
	return float64(count(rows, predicate)) / float64(len(rows))
}

// count returns the number of rows matching predicate.
func count(rows []RunSummaryRow, predicate func(RunSummaryRow) bool) int {
	total := 0
	for _, row := range rows {
		if predicate(row) {
			total++
		}
	}
	return total
}

// extractInt projects integer row values into float64 slices for median math.
func extractInt(rows []RunSummaryRow, valueFn func(RunSummaryRow) int) []float64 {
	values := make([]float64, 0, len(rows))
	for _, row := range rows {
		values = append(values, float64(valueFn(row)))
	}
	return values
}

// extractFloat projects float row values for aggregate math.
func extractFloat(rows []RunSummaryRow, valueFn func(RunSummaryRow) float64) []float64 {
	values := make([]float64, 0, len(rows))
	for _, row := range rows {
		values = append(values, valueFn(row))
	}
	return values
}

// extractOptionalInt64 projects optional int64 values while skipping nils.
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

// sumIntRows sums one integer field across rows.
func sumIntRows(rows []RunSummaryRow, valueFn func(RunSummaryRow) int) int {
	total := 0
	for _, row := range rows {
		total += valueFn(row)
	}
	return total
}

// medianFloat returns the median of values, or zero when the slice is empty.
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

// averageManualScore computes the average reviewer actionability score when present.
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

// agentUsageInputTokens returns normalized input tokens from parsed agent metadata.
func agentUsageInputTokens(metadata *AgentMetadata) *int64 {
	if metadata == nil {
		return nil
	}
	return metadata.Usage.InputTokens
}

// agentUsageOutputTokens returns normalized output tokens from parsed agent metadata.
func agentUsageOutputTokens(metadata *AgentMetadata) *int64 {
	if metadata == nil {
		return nil
	}
	return metadata.Usage.OutputTokens
}

// readJSONFile reads JSON from disk into target.
func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
