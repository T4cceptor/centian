package benchmarks

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/common"
)

const scoreVersion = "v1"

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
	RawStatus        string              `json:"rawStatus"`
	LatestTaskRunID  string              `json:"latestTaskRunId,omitempty"`
	LinkedTaskRunIDs []string            `json:"linkedTaskRunIds,omitempty"`
	Outcome          ScorecardOutcome    `json:"outcome"`
	Process          ScorecardProcess    `json:"process"`
	Efficiency       ScorecardEfficiency `json:"efficiency"`
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
	AgentMetadata             *AgentMetadata `json:"agentMetadata,omitempty"`
	Warnings                  []string       `json:"warnings,omitempty"`
	Errors                    []string       `json:"errors,omitempty"`
}

// AggregateSummary stores aggregate comparison metrics for one grouping key.
type AggregateSummary struct {
	Key                             string  `json:"key"`
	SessionPath                     string  `json:"sessionPath,omitempty"`
	CaseID                          string  `json:"caseId,omitempty"`
	Agent                           string  `json:"agent,omitempty"`
	TemplateVariant                 string  `json:"templateVariant,omitempty"`
	RunCount                        int     `json:"runCount"`
	ScoredRunCount                  int     `json:"scoredRunCount"`
	SuccessRate                     float64 `json:"successRate"`
	FirstPassSuccessRate            float64 `json:"firstPassSuccessRate"`
	FinalVerificationPassRate       float64 `json:"finalVerificationPassRate"`
	InvariantViolationRate          float64 `json:"invariantViolationRate"`
	RestartFailTimeoutRate          float64 `json:"restartFailTimeoutRate"`
	MedianWallClockSeconds          float64 `json:"medianWallClockSeconds"`
	MedianTotalToolCalls            float64 `json:"medianTotalToolCalls"`
	MedianInputTokens               float64 `json:"medianInputTokens"`
	MedianOutputTokens              float64 `json:"medianOutputTokens"`
	MedianFailedTaskToolCalls       float64 `json:"medianFailedTaskToolCalls"`
	MedianFailedDownstreamToolCalls float64 `json:"medianFailedDownstreamToolCalls"`
	MedianEditedFilesCount          float64 `json:"medianEditedFilesCount"`
	TotalTaskToolCalls              int     `json:"totalTaskToolCalls"`
	TotalDownstreamToolCalls        int     `json:"totalDownstreamToolCalls"`
}

// scoreRunContext holds the resolved case definition and fixture root for scoring.
type scoreRunContext struct {
	caseDef  *CaseDefinition
	caseRoot string
}

// loadSessionManifest reads and minimally validates a preserved benchmark session manifest.
func loadSessionManifest(sessionDir string) (*SessionManifest, error) {
	var session SessionManifest
	if err := common.ReadJSONFile(filepath.Join(sessionDir, sessionFileName), &session); err != nil {
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
			Key:                       key,
			SessionPath:               keys[key].SessionPath,
			CaseID:                    keys[key].CaseID,
			Agent:                     keys[key].Agent,
			TemplateVariant:           keys[key].TemplateVariant,
			RunCount:                  len(group),
			ScoredRunCount:            len(scoredGroup),
			SuccessRate:               rate(scoredGroup, func(row RunSummaryRow) bool { return row.CompletedSuccessfully }),
			FirstPassSuccessRate:      rate(scoredGroup, func(row RunSummaryRow) bool { return row.FirstPassSuccess }),
			FinalVerificationPassRate: rate(scoredGroup, func(row RunSummaryRow) bool { return row.FinalVerificationPassed }),
			InvariantViolationRate:    rate(scoredGroup, func(row RunSummaryRow) bool { return row.InvariantViolation }),
			RestartFailTimeoutRate:    rate(scoredGroup, func(row RunSummaryRow) bool { return row.RestartOccurred || row.FailOccurred || row.TimeoutOccurred }),
			MedianWallClockSeconds: common.MedianFloat(collectFloat64(scoredGroup, func(row RunSummaryRow) (float64, bool) {
				return row.WallClockSeconds, true
			})),
			MedianTotalToolCalls: common.MedianFloat(collectFloat64(scoredGroup, func(row RunSummaryRow) (float64, bool) {
				return float64(row.TotalToolCalls), true
			})),
			MedianInputTokens: common.MedianFloat(collectFloat64(scoredGroup, func(row RunSummaryRow) (float64, bool) {
				if row.InputTokens == nil {
					return 0, false
				}
				return float64(*row.InputTokens), true
			})),
			MedianOutputTokens: common.MedianFloat(collectFloat64(scoredGroup, func(row RunSummaryRow) (float64, bool) {
				if row.OutputTokens == nil {
					return 0, false
				}
				return float64(*row.OutputTokens), true
			})),
			MedianFailedTaskToolCalls: common.MedianFloat(collectFloat64(scoredGroup, func(row RunSummaryRow) (float64, bool) {
				return float64(row.FailedTaskToolCalls), true
			})),
			MedianFailedDownstreamToolCalls: common.MedianFloat(collectFloat64(scoredGroup, func(row RunSummaryRow) (float64, bool) {
				return float64(row.FailedDownstreamToolCalls), true
			})),
			MedianEditedFilesCount: common.MedianFloat(collectFloat64(scoredGroup, func(row RunSummaryRow) (float64, bool) {
				return float64(row.EditedFilesCount), true
			})),
			TotalTaskToolCalls:       sumIntRows(scoredGroup, func(row RunSummaryRow) int { return row.TotalTaskToolCalls }),
			TotalDownstreamToolCalls: sumIntRows(scoredGroup, func(row RunSummaryRow) int { return row.TotalDownstreamToolCalls }),
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
	return float64(common.CountBy(rows, predicate)) / float64(len(rows))
}

// collectFloat64 projects rows into float64 values while allowing callers to skip entries.
func collectFloat64[T any](rows []T, project func(T) (float64, bool)) []float64 {
	values := make([]float64, 0, len(rows))
	for _, row := range rows {
		value, ok := project(row)
		if !ok {
			continue
		}
		values = append(values, value)
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
