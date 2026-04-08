package benchmarks

import (
	"context"
	"sort"
	"strings"

	"github.com/T4cceptor/centian/internal/persistence"
)

// ReadService exposes benchmark read models for the API/UI.
type ReadService struct {
	query *QueryService
}

// NewReadService builds a read-only benchmark service on top of the shared live query layer.
func NewReadService(store benchmarkQueryStore) *ReadService {
	return &ReadService{query: NewQueryService(store)}
}

// ListSuites returns suite-level benchmark summaries.
func (s *ReadService) ListSuites(ctx context.Context, filters BenchmarkRunFilters) ([]BenchmarkSuiteSummary, error) {
	return s.query.ListSuites(ctx, filters)
}

// ListSessions returns benchmark sessions for one suite.
func (s *ReadService) ListSessions(ctx context.Context, suiteID string, filters BenchmarkRunFilters) ([]BenchmarkSessionDetail, error) {
	return s.query.ListSessions(ctx, suiteID, filters, false)
}

// GetSession returns one session detail for a suite.
func (s *ReadService) GetSession(ctx context.Context, suiteID, sessionID string) (*BenchmarkSessionDetail, error) {
	return s.query.GetSession(ctx, suiteID, sessionID)
}

// ListRuns returns live benchmark runs for one suite.
func (s *ReadService) ListRuns(ctx context.Context, suiteID string, filters BenchmarkRunFilters) ([]BenchmarkRunSummary, error) {
	return s.query.ListRuns(ctx, suiteID, filters)
}

// GetRun returns one live run detail.
func (s *ReadService) GetRun(ctx context.Context, suiteID, scorecardID string) (*BenchmarkRunDetail, error) {
	return s.query.GetRun(ctx, suiteID, scorecardID)
}

// GetComparison derives a filtered comparison view from live benchmark run summaries.
func (s *ReadService) GetComparison(ctx context.Context, suiteID string, filters BenchmarkRunFilters) (*BenchmarkComparisonView, error) {
	return s.query.GetComparison(ctx, suiteID, filters)
}

// ListTemplateScorecards returns generic template scorecards over all persisted task runs.
func (s *ReadService) ListTemplateScorecards(ctx context.Context) ([]TemplateScorecard, error) {
	return s.query.ListTemplateScorecards(ctx)
}

// ListAgentScorecards returns agent scorecards over all persisted benchmark runs.
func (s *ReadService) ListAgentScorecards(ctx context.Context) ([]AgentScorecard, error) {
	return s.query.ListAgentScorecards(ctx)
}

func buildBenchmarkRunSummary(item *persistence.BenchmarkRunRecord, scorecard *RunScorecard) BenchmarkRunSummary {
	return BenchmarkRunSummary{
		ScorecardID:               item.BenchmarkRunID,
		SessionID:                 item.SessionID,
		SessionPath:               scorecard.SessionPath,
		SuiteID:                   scorecard.SuiteID,
		SuiteName:                 scorecard.SuiteName,
		TemplateID:                scorecard.TemplateID,
		TemplateName:              scorecard.TemplateName,
		CaseID:                    scorecard.CaseID,
		CaseName:                  scorecard.CaseName,
		Agent:                     scorecard.Agent,
		TemplateVariant:           scorecard.TemplateVariant,
		Attempt:                   scorecard.Attempt,
		RawStatus:                 scorecard.RawStatus,
		LatestTaskRunID:           scorecard.LatestTaskRunID,
		LinkedTaskRunIDs:          append([]string(nil), scorecard.LinkedTaskRunIDs...),
		CompletedSuccessfully:     scorecard.Outcome.CompletedSuccessfully,
		FinalVerificationPassed:   scorecard.Outcome.FinalVerificationPassed,
		FirstPassSuccess:          scorecard.Outcome.FirstPassSuccess,
		InvariantViolation:        scorecard.Outcome.InvariantViolation,
		RestartOccurred:           scorecard.Outcome.RestartOccurred,
		FailOccurred:              scorecard.Outcome.FailOccurred,
		TimeoutOccurred:           scorecard.Outcome.TimeoutOccurred,
		WallClockSeconds:          scorecard.Efficiency.WallClockSeconds,
		TotalToolCalls:            scorecard.Efficiency.TotalToolCalls,
		TotalTaskToolCalls:        scorecard.Process.TotalTaskToolCalls,
		TotalDownstreamToolCalls:  scorecard.Process.TotalDownstreamToolCalls,
		InputTokens:               scorecard.Efficiency.InputTokens,
		OutputTokens:              scorecard.Efficiency.OutputTokens,
		FailedTaskToolCalls:       scorecard.Process.FailedTaskToolCalls,
		FailedDownstreamToolCalls: scorecard.Process.FailedDownstreamToolCalls,
		EditedFilesCount:          scorecard.Efficiency.EditedFilesCount,
		ErrorActionabilityScore:   scorecard.Manual.ErrorActionabilityScore,
		AgentMetadata:             scorecard.AgentMetadata,
		Warnings:                  append([]string(nil), scorecard.Warnings...),
		Errors:                    append([]string(nil), scorecard.Errors...),
	}
}

func runSummaryFromRecord(session persistence.BenchmarkSessionRecord, item persistence.BenchmarkRunRecord, err error) BenchmarkRunSummary {
	return BenchmarkRunSummary{
		ScorecardID:      item.BenchmarkRunID,
		SessionID:        item.SessionID,
		SessionPath:      session.SessionPath,
		SuiteID:          session.SuiteID,
		SuiteName:        suiteNameFromPath(session.SuitePath),
		TemplateID:       item.TemplateID,
		CaseID:           item.CaseID,
		CaseName:         caseNamesFromSuitePath(session.SuitePath)[item.CaseID],
		Agent:            item.Agent,
		TemplateVariant:  item.TemplateVariant,
		Attempt:          item.Attempt,
		RawStatus:        item.Status,
		LatestTaskRunID:  item.LatestTaskRunID,
		LinkedTaskRunIDs: append([]string(nil), item.LinkedTaskRunIDs...),
		Errors:           []string{err.Error()},
	}
}

func toRunSummaryRow(run BenchmarkRunSummary) RunSummaryRow {
	return RunSummaryRow{
		SessionPath:               run.SessionPath,
		CaseID:                    run.CaseID,
		Agent:                     run.Agent,
		TemplateVariant:           run.TemplateVariant,
		Attempt:                   run.Attempt,
		RawStatus:                 run.RawStatus,
		LatestTaskRunID:           run.LatestTaskRunID,
		LinkedTaskRunIDs:          append([]string(nil), run.LinkedTaskRunIDs...),
		Scored:                    len(run.Errors) == 0,
		CompletedSuccessfully:     run.CompletedSuccessfully,
		FinalVerificationPassed:   run.FinalVerificationPassed,
		FirstPassSuccess:          run.FirstPassSuccess,
		InvariantViolation:        run.InvariantViolation,
		RestartOccurred:           run.RestartOccurred,
		FailOccurred:              run.FailOccurred,
		TimeoutOccurred:           run.TimeoutOccurred,
		WallClockSeconds:          run.WallClockSeconds,
		TotalToolCalls:            run.TotalToolCalls,
		InputTokens:               run.InputTokens,
		OutputTokens:              run.OutputTokens,
		FailedTaskToolCalls:       run.FailedTaskToolCalls,
		FailedDownstreamToolCalls: run.FailedDownstreamToolCalls,
		EditedFilesCount:          run.EditedFilesCount,
		ErrorActionabilityScore:   run.ErrorActionabilityScore,
		AgentMetadata:             run.AgentMetadata,
		Warnings:                  append([]string(nil), run.Warnings...),
		Errors:                    append([]string(nil), run.Errors...),
	}
}

func sortedSetValues(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func hasRunScopedFilter(filters BenchmarkRunFilters) bool {
	return filters.SessionID != "" || filters.CaseID != "" || filters.Agent != "" || filters.TemplateVariant != "" || filters.TemplateID != ""
}
