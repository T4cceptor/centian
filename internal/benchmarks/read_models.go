package benchmarks

import "time"

// BenchmarkRunFilters restrict benchmark read queries to one run subset.
type BenchmarkRunFilters struct {
	SuiteID         string `json:"suiteId,omitempty"`
	TemplateID      string `json:"templateId,omitempty"`
	SessionID       string `json:"sessionId,omitempty"`
	CaseID          string `json:"caseId,omitempty"`
	Agent           string `json:"agent,omitempty"`
	TemplateVariant string `json:"templateVariant,omitempty"`
}

// BenchmarkSuiteSummary is the suite-level API/UI read model.
type BenchmarkSuiteSummary struct {
	SuiteID           string    `json:"suiteId"`
	SuiteName         string    `json:"suiteName,omitempty"`
	TemplateID        string    `json:"templateId,omitempty"`
	TemplateName      string    `json:"templateName,omitempty"`
	LatestGeneratedAt time.Time `json:"latestGeneratedAt"`
	SessionCount      int       `json:"sessionCount"`
	RunCount          int       `json:"runCount"`
	Agents            []string  `json:"agents,omitempty"`
	CaseIDs           []string  `json:"caseIds,omitempty"`
	CaseNames         []string  `json:"caseNames,omitempty"`
	TemplateVariants  []string  `json:"templateVariants,omitempty"`
}

// BenchmarkRunSummary is the compact scorecard-backed row used in list views.
type BenchmarkRunSummary struct {
	ScorecardID               string         `json:"scorecardId"`
	SessionID                 string         `json:"sessionId"`
	SessionPath               string         `json:"sessionPath"`
	SuiteID                   string         `json:"suiteId"`
	SuiteName                 string         `json:"suiteName,omitempty"`
	TemplateID                string         `json:"templateId"`
	TemplateName              string         `json:"templateName,omitempty"`
	CaseID                    string         `json:"caseId"`
	CaseName                  string         `json:"caseName,omitempty"`
	Agent                     string         `json:"agent"`
	SelectedModel             string         `json:"selectedModel,omitempty"`
	TemplateVariant           string         `json:"templateVariant"`
	Attempt                   int            `json:"attempt"`
	RawStatus                 string         `json:"rawStatus"`
	LatestTaskRunID           string         `json:"latestTaskRunId,omitempty"`
	LinkedTaskRunIDs          []string       `json:"linkedTaskRunIds,omitempty"`
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

// BenchmarkSessionDetail is the session-level API/UI read model.
type BenchmarkSessionDetail struct {
	SessionID          string                   `json:"sessionId"`
	SuiteID            string                   `json:"suiteId"`
	SuiteName          string                   `json:"suiteName,omitempty"`
	TemplateID         string                   `json:"templateId,omitempty"`
	TemplateName       string                   `json:"templateName,omitempty"`
	SessionPath        string                   `json:"sessionPath"`
	GeneratedAt        time.Time                `json:"generatedAt"`
	RunCount           int                      `json:"runCount"`
	ScoredRunCount     int                      `json:"scoredRunCount"`
	FailedToScoreCount int                      `json:"failedToScoreCount"`
	Agents             []string                 `json:"agents,omitempty"`
	CaseIDs            []string                 `json:"caseIds,omitempty"`
	CaseNames          []string                 `json:"caseNames,omitempty"`
	TemplateVariants   []string                 `json:"templateVariants,omitempty"`
	Aggregates         SessionSummaryAggregates `json:"aggregates"`
	Runs               []BenchmarkRunSummary    `json:"runs,omitempty"`
}

// BenchmarkRunDetail returns the full stored scorecard for one run.
type BenchmarkRunDetail struct {
	ScorecardID  string       `json:"scorecardId"`
	SessionID    string       `json:"sessionId"`
	SessionPath  string       `json:"sessionPath"`
	SuiteName    string       `json:"suiteName,omitempty"`
	TemplateName string       `json:"templateName,omitempty"`
	CaseName     string       `json:"caseName,omitempty"`
	Scorecard    RunScorecard `json:"scorecard"`
}

// BenchmarkComparisonView is the read-side comparison result used by the UI.
type BenchmarkComparisonView struct {
	SuiteID      string                `json:"suiteId"`
	SuiteName    string                `json:"suiteName,omitempty"`
	TemplateID   string                `json:"templateId,omitempty"`
	TemplateName string                `json:"templateName,omitempty"`
	Filters      BenchmarkRunFilters   `json:"filters"`
	SessionCount int                   `json:"sessionCount"`
	RunCount     int                   `json:"runCount"`
	Sessions     []ComparisonSession   `json:"sessions"`
	Runs         []BenchmarkRunSummary `json:"runs"`
	Aggregates   ComparisonAggregates  `json:"aggregates"`
}
