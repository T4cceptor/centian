package benchmarks

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ReadService converts persisted benchmark run/session artifacts plus task-run persistence into typed read models.
type ReadService struct {
	store benchmarkReadStore
}

// NewReadService builds a read-only benchmark query service.
func NewReadService(store benchmarkReadStore) *ReadService {
	return &ReadService{store: store}
}

// ListSuites returns suite-level benchmark summaries.
func (s *ReadService) ListSuites(ctx context.Context, filters BenchmarkRunFilters) ([]BenchmarkSuiteSummary, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("benchmark read service is not initialized")
	}
	sessions, err := loadPersistedSessions(ctx, s.store, filters.SuiteID)
	if err != nil {
		return nil, err
	}
	runs, err := loadPersistedRuns(ctx, s.store, filters.SuiteID, filters)
	if err != nil {
		return nil, err
	}

	type suiteAggregate struct {
		item       BenchmarkSuiteSummary
		sessionIDs map[string]struct{}
		agents     map[string]struct{}
		caseIDs    map[string]struct{}
		caseNames  map[string]struct{}
		variants   map[string]struct{}
	}

	grouped := map[string]*suiteAggregate{}
	for idx := range sessions {
		session := sessions[idx]
		group := grouped[session.session.SuiteID]
		if group == nil {
			group = &suiteAggregate{
				item:       BenchmarkSuiteSummary{SuiteID: session.session.SuiteID},
				sessionIDs: map[string]struct{}{},
				agents:     map[string]struct{}{},
				caseIDs:    map[string]struct{}{},
				caseNames:  map[string]struct{}{},
				variants:   map[string]struct{}{},
			}
			grouped[session.session.SuiteID] = group
		}
		group.item.SuiteName = firstNonEmpty(group.item.SuiteName, sessionSuiteName(session))
		group.item.TemplateID = firstNonEmpty(group.item.TemplateID, session.session.TemplateID)
		group.item.LatestGeneratedAt = latestTime(group.item.LatestGeneratedAt, bestTime(session.session.EndedAt, session.session.StartedAt))
		group.sessionIDs[session.record.SessionID] = struct{}{}
		for caseID, caseName := range sessionCaseNames(session) {
			group.caseIDs[caseID] = struct{}{}
			if caseName != "" {
				group.caseNames[caseName] = struct{}{}
			}
		}
	}

	for idx := range runs {
		run := runs[idx]
		group := grouped[run.run.SuiteID]
		if group == nil {
			group = &suiteAggregate{
				item:       BenchmarkSuiteSummary{SuiteID: run.run.SuiteID},
				sessionIDs: map[string]struct{}{},
				agents:     map[string]struct{}{},
				caseIDs:    map[string]struct{}{},
				caseNames:  map[string]struct{}{},
				variants:   map[string]struct{}{},
			}
			grouped[run.run.SuiteID] = group
		}
		group.item.TemplateID = firstNonEmpty(group.item.TemplateID, run.run.TemplateID)
		group.item.TemplateName = firstNonEmpty(group.item.TemplateName, runTemplateName(ctx, s.store, &run.run))
		group.item.RunCount++
		group.item.LatestGeneratedAt = latestTime(group.item.LatestGeneratedAt, bestTime(run.run.EndedAt, run.run.StartedAt))
		group.sessionIDs[run.record.SessionID] = struct{}{}
		group.agents[run.run.AgentID] = struct{}{}
		group.caseIDs[run.run.CaseID] = struct{}{}
		group.variants[run.run.TemplateVariant.Name] = struct{}{}
	}

	result := make([]BenchmarkSuiteSummary, 0, len(grouped))
	for _, group := range grouped {
		if filters.TemplateID != "" && group.item.TemplateID != filters.TemplateID {
			continue
		}
		group.item.SessionCount = len(group.sessionIDs)
		group.item.Agents = sortedSetValues(group.agents)
		group.item.CaseIDs = sortedSetValues(group.caseIDs)
		group.item.CaseNames = sortedSetValues(group.caseNames)
		group.item.TemplateVariants = sortedSetValues(group.variants)
		result = append(result, group.item)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].LatestGeneratedAt.Equal(result[j].LatestGeneratedAt) {
			return result[i].LatestGeneratedAt.After(result[j].LatestGeneratedAt)
		}
		return result[i].SuiteID < result[j].SuiteID
	})
	return result, nil
}

// ListSessions returns benchmark sessions for one suite.
func (s *ReadService) ListSessions(ctx context.Context, suiteID string, filters BenchmarkRunFilters) ([]BenchmarkSessionDetail, error) {
	return s.listSessions(ctx, suiteID, filters, false)
}

// GetSession returns one session detail for a suite.
func (s *ReadService) GetSession(ctx context.Context, suiteID, sessionID string) (*BenchmarkSessionDetail, error) {
	sessions, err := s.listSessions(ctx, suiteID, BenchmarkRunFilters{SessionID: sessionID}, true)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		//nolint:nilnil // Missing benchmark session is represented as an absent resource.
		return nil, nil
	}
	return &sessions[0], nil
}

// ListRuns returns live benchmark runs for one suite.
func (s *ReadService) ListRuns(ctx context.Context, suiteID string, filters BenchmarkRunFilters) ([]BenchmarkRunSummary, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("benchmark read service is not initialized")
	}
	sessions, err := loadPersistedSessions(ctx, s.store, suiteID)
	if err != nil {
		return nil, err
	}
	sessionByID := sessionMap(sessions)
	runs, err := loadPersistedRuns(ctx, s.store, suiteID, filters)
	if err != nil {
		return nil, err
	}
	service := newLiveQueryService(s.store, timeNowUTC)

	result := make([]BenchmarkRunSummary, 0, len(runs))
	for idx := range runs {
		session := sessionByID[runs[idx].record.SessionID]
		if session == nil {
			continue
		}
		scorecard, err := service.scorePersistedRun(ctx, session, &runs[idx])
		if err != nil {
			result = append(result, runSummaryFromManifest(*session, runs[idx], err))
			continue
		}
		result = append(result, buildBenchmarkRunSummary(runs[idx], scorecard))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SessionPath != result[j].SessionPath {
			return result[i].SessionPath > result[j].SessionPath
		}
		return compareRunRows(toRunSummaryRow(result[i]), toRunSummaryRow(result[j]))
	})
	return result, nil
}

// GetRun returns one live run detail.
func (s *ReadService) GetRun(ctx context.Context, suiteID, scorecardID string) (*BenchmarkRunDetail, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("benchmark read service is not initialized")
	}
	sessions, err := loadPersistedSessions(ctx, s.store, suiteID)
	if err != nil {
		return nil, err
	}
	sessionByID := sessionMap(sessions)
	runs, err := loadPersistedRuns(ctx, s.store, suiteID, BenchmarkRunFilters{})
	if err != nil {
		return nil, err
	}
	service := newLiveQueryService(s.store, timeNowUTC)
	for idx := range runs {
		if runs[idx].record.ID != scorecardID {
			continue
		}
		session := sessionByID[runs[idx].record.SessionID]
		if session == nil {
			break
		}
		scorecard, err := service.scorePersistedRun(ctx, session, &runs[idx])
		if err != nil {
			return nil, err
		}
		return &BenchmarkRunDetail{
			ScorecardID:  runs[idx].record.ID,
			SessionID:    runs[idx].record.SessionID,
			SessionPath:  runs[idx].record.SessionPath,
			SuiteName:    scorecard.SuiteName,
			TemplateName: scorecard.TemplateName,
			CaseName:     scorecard.CaseName,
			Scorecard:    *scorecard,
		}, nil
	}
	//nolint:nilnil // Missing benchmark run is represented as an absent resource.
	return nil, nil
}

// GetComparison derives a filtered comparison view from live benchmark run summaries.
func (s *ReadService) GetComparison(ctx context.Context, suiteID string, filters BenchmarkRunFilters) (*BenchmarkComparisonView, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("benchmark read service is not initialized")
	}
	sessions, err := s.listSessions(ctx, suiteID, filters, false)
	if err != nil {
		return nil, err
	}
	runs, err := s.ListRuns(ctx, suiteID, filters)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 && len(runs) == 0 {
		//nolint:nilnil // Missing benchmark comparison is represented as an absent resource.
		return nil, nil
	}

	rows := make([]RunSummaryRow, 0, len(runs))
	templateID := ""
	templateName := ""
	suiteName := ""
	for idx := range runs {
		templateID = firstNonEmpty(templateID, runs[idx].TemplateID)
		templateName = firstNonEmpty(templateName, runs[idx].TemplateName)
		suiteName = firstNonEmpty(suiteName, runs[idx].SuiteName)
		rows = append(rows, toRunSummaryRow(runs[idx]))
	}
	aggregates := buildAggregates(rows)
	comparisonSessions := make([]ComparisonSession, 0, len(sessions))
	for idx := range sessions {
		comparisonSessions = append(comparisonSessions, ComparisonSession{
			SessionPath:        sessions[idx].SessionPath,
			GeneratedAt:        sessions[idx].GeneratedAt,
			RunCount:           sessions[idx].RunCount,
			ScoredRunCount:     sessions[idx].ScoredRunCount,
			FailedToScoreCount: sessions[idx].FailedToScoreCount,
		})
	}
	return &BenchmarkComparisonView{
		SuiteID:      suiteID,
		SuiteName:    suiteName,
		TemplateID:   templateID,
		TemplateName: templateName,
		Filters:      filters,
		SessionCount: len(comparisonSessions),
		RunCount:     len(runs),
		Sessions:     comparisonSessions,
		Runs:         runs,
		Aggregates: ComparisonAggregates{
			BySession: aggregateRows(rows, func(row RunSummaryRow) aggregateKey {
				return aggregateKey{Key: row.SessionPath, SessionPath: row.SessionPath}
			}),
			ByCase:             aggregates.ByCase,
			ByAgent:            aggregates.ByAgent,
			ByTemplateVariant:  aggregates.ByTemplateVariant,
			ByCaseAgentVariant: aggregates.ByCaseAgentVariant,
		},
	}, nil
}

func (s *ReadService) listSessions(ctx context.Context, suiteID string, filters BenchmarkRunFilters, includeRuns bool) ([]BenchmarkSessionDetail, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("benchmark read service is not initialized")
	}
	sessions, err := loadPersistedSessions(ctx, s.store, suiteID)
	if err != nil {
		return nil, err
	}
	runs, err := s.ListRuns(ctx, suiteID, filters)
	if err != nil {
		return nil, err
	}
	runsBySession := map[string][]BenchmarkRunSummary{}
	for idx := range runs {
		runsBySession[runs[idx].SessionID] = append(runsBySession[runs[idx].SessionID], runs[idx])
	}

	result := make([]BenchmarkSessionDetail, 0, len(sessions))
	for idx := range sessions {
		session := sessions[idx]
		if filters.SessionID != "" && session.record.SessionID != filters.SessionID {
			continue
		}
		sessionRuns := runsBySession[session.record.SessionID]
		if hasRunScopedFilter(filters) && len(sessionRuns) == 0 {
			continue
		}
		rows := make([]RunSummaryRow, 0, len(sessionRuns))
		agents := map[string]struct{}{}
		caseIDs := map[string]struct{}{}
		caseNames := map[string]struct{}{}
		variants := map[string]struct{}{}
		templateID := ""
		templateName := ""
		suiteName := firstNonEmpty(sessionSuiteName(session))
		for runIdx := range sessionRuns {
			rows = append(rows, toRunSummaryRow(sessionRuns[runIdx]))
			agents[sessionRuns[runIdx].Agent] = struct{}{}
			caseIDs[sessionRuns[runIdx].CaseID] = struct{}{}
			if name := strings.TrimSpace(sessionRuns[runIdx].CaseName); name != "" {
				caseNames[name] = struct{}{}
			}
			variants[sessionRuns[runIdx].TemplateVariant] = struct{}{}
			templateID = firstNonEmpty(templateID, sessionRuns[runIdx].TemplateID)
			templateName = firstNonEmpty(templateName, sessionRuns[runIdx].TemplateName)
			suiteName = firstNonEmpty(suiteName, sessionRuns[runIdx].SuiteName)
		}

		detail := BenchmarkSessionDetail{
			SessionID:          session.record.SessionID,
			SuiteID:            session.session.SuiteID,
			SuiteName:          suiteName,
			TemplateID:         firstNonEmpty(templateID, session.session.TemplateID),
			TemplateName:       templateName,
			SessionPath:        session.record.SessionPath,
			GeneratedAt:        bestTime(session.session.EndedAt, session.session.StartedAt),
			RunCount:           len(session.session.Runs),
			ScoredRunCount:     len(sessionRuns),
			FailedToScoreCount: maxInt(len(session.session.Runs)-len(sessionRuns), 0),
			Agents:             sortedSetValues(agents),
			CaseIDs:            sortedSetValues(caseIDs),
			CaseNames:          sortedSetValues(caseNames),
			TemplateVariants:   sortedSetValues(variants),
			Aggregates:         buildAggregates(rows),
		}
		if len(detail.CaseNames) == 0 {
			for _, name := range sessionCaseNames(session) {
				if strings.TrimSpace(name) != "" {
					caseNames[name] = struct{}{}
				}
			}
			detail.CaseNames = sortedSetValues(caseNames)
		}
		if includeRuns {
			detail.Runs = append(detail.Runs, sessionRuns...)
		}
		result = append(result, detail)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].GeneratedAt.Equal(result[j].GeneratedAt) {
			return result[i].GeneratedAt.After(result[j].GeneratedAt)
		}
		return result[i].SessionPath > result[j].SessionPath
	})
	return result, nil
}

func buildBenchmarkRunSummary(item persistedRun, scorecard *RunScorecard) BenchmarkRunSummary {
	return BenchmarkRunSummary{
		ScorecardID:               item.record.ID,
		SessionID:                 item.record.SessionID,
		SessionPath:               item.record.SessionPath,
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

func runSummaryFromManifest(session persistedSession, item persistedRun, err error) BenchmarkRunSummary {
	return BenchmarkRunSummary{
		ScorecardID:      item.record.ID,
		SessionID:        item.record.SessionID,
		SessionPath:      item.record.SessionPath,
		SuiteID:          item.run.SuiteID,
		SuiteName:        sessionSuiteName(session),
		TemplateID:       item.run.TemplateID,
		CaseID:           item.run.CaseID,
		CaseName:         sessionCaseNames(session)[item.run.CaseID],
		Agent:            item.run.AgentID,
		TemplateVariant:  item.run.TemplateVariant.Name,
		Attempt:          item.run.Attempt,
		RawStatus:        item.run.Status,
		LatestTaskRunID:  item.run.LatestTaskRunID,
		LinkedTaskRunIDs: append([]string(nil), item.run.LinkedTaskRunIDs...),
		Errors:           []string{err.Error()},
	}
}

func sessionMap(sessions []persistedSession) map[string]*persistedSession {
	result := make(map[string]*persistedSession, len(sessions))
	for idx := range sessions {
		result[sessions[idx].record.SessionID] = &sessions[idx]
	}
	return result
}

func runTemplateName(ctx context.Context, store taskRunSnapshotReader, run *RunManifest) string {
	if store == nil || run == nil {
		return ""
	}
	runID := strings.TrimSpace(run.LatestTaskRunID)
	if runID == "" {
		runID = latestLinkedTaskRunID(run.LinkedTaskRunIDs)
	}
	if runID == "" {
		return ""
	}
	snapshot, err := store.GetTaskRunSnapshot(ctx, runID)
	if err != nil || snapshot == nil {
		return ""
	}
	return templateNameFromSnapshot(snapshot)
}

func sessionCaseNames(session persistedSession) map[string]string {
	result := map[string]string{}
	suiteCtx, err := loadSuiteContext(session.session.SuitePath)
	if err != nil {
		return result
	}
	for caseID, caseCtx := range suiteCtx.caseDefs {
		result[caseID] = caseCtx.caseDef.Case.Name
	}
	return result
}

func sessionSuiteName(session persistedSession) string {
	suiteCtx, err := loadSuiteContext(session.session.SuitePath)
	if err != nil || suiteCtx == nil || suiteCtx.suite == nil {
		return ""
	}
	return suiteCtx.suite.Suite.Name
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
	return strings.TrimSpace(filters.TemplateID) != "" ||
		strings.TrimSpace(filters.SessionID) != "" ||
		strings.TrimSpace(filters.CaseID) != "" ||
		strings.TrimSpace(filters.Agent) != "" ||
		strings.TrimSpace(filters.TemplateVariant) != ""
}

func latestTime(current time.Time, candidate time.Time) time.Time {
	if candidate.After(current) {
		return candidate
	}
	return current
}

func maxInt(value int, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
}

func timeNowUTC() time.Time {
	return time.Now().UTC()
}
