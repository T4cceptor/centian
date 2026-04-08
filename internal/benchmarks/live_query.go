package benchmarks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/persistence"
)

var (
	// ErrBenchmarkSessionNotFound indicates that the requested benchmark session does not exist.
	ErrBenchmarkSessionNotFound = errors.New("benchmark session not found")
	// ErrBenchmarkRunNotFound indicates that the requested benchmark run does not exist.
	ErrBenchmarkRunNotFound = errors.New("benchmark run not found")
	// ErrBenchmarkComparisonNotFound indicates that no benchmark data matched the requested comparison scope.
	ErrBenchmarkComparisonNotFound = errors.New("benchmark comparison not found")
)

type benchmarkQueryStore interface {
	ListBenchmarkSessions(context.Context, persistence.BenchmarkSessionFilter) ([]persistence.BenchmarkSessionRecord, error)
	ListBenchmarkRuns(context.Context, *persistence.BenchmarkRunFilter) ([]persistence.BenchmarkRunRecord, error)
	GetBenchmarkRun(context.Context, string) (*persistence.BenchmarkRunRecord, error)
	ListTaskRunSnapshots(context.Context) ([]persistence.TaskRunSnapshotRecord, error)
	GetTaskRunSnapshot(context.Context, string) (*persistence.TaskRunSnapshotRecord, error)
	ListTaskRunStats(context.Context) ([]persistence.TaskRunStatsRecord, error)
	GetTaskRunStats(context.Context, string) (*persistence.TaskRunStatsRecord, error)
}

// QueryService is the single live benchmark query/scoring service used by CLI and API.
type QueryService struct {
	now   func() time.Time
	store benchmarkQueryStore
}

// NewQueryService builds a live benchmark query service.
func NewQueryService(store benchmarkQueryStore) *QueryService {
	return &QueryService{now: timeNowUTC, store: store}
}

func (s *QueryService) withDefaults() *QueryService {
	if s == nil {
		return &QueryService{now: timeNowUTC}
	}
	if s.now == nil {
		s.now = timeNowUTC
	}
	return s
}

// ScoreSessionManifest computes a live session score summary from benchmark manifests and persisted task-run data.
func (s *QueryService) ScoreSessionManifest(ctx context.Context, session *SessionManifest) (*SessionSummary, error) {
	s = s.withDefaults()
	if s.store == nil {
		return nil, fmt.Errorf("benchmark query service is not initialized")
	}
	if session == nil {
		return nil, fmt.Errorf("session manifest is required")
	}
	sessionPath := filepath.Clean(strings.TrimSpace(session.InvocationDir))
	sessionID := benchmarkSessionID(session.SuiteID, sessionPath)
	runs, err := s.store.ListBenchmarkRuns(ctx, &persistence.BenchmarkRunFilter{SessionID: sessionID})
	if err != nil {
		return nil, err
	}
	runByDir := make(map[string]*persistence.BenchmarkRunRecord, len(runs))
	for idx := range runs {
		run := runs[idx]
		runByDir[filepath.Clean(run.RunDir)] = &run
	}

	summary := &SessionSummary{
		ScoreVersion: scoreVersion,
		SessionPath:  sessionPath,
		SuiteID:      session.SuiteID,
		GeneratedAt:  s.now(),
		RunCount:     len(session.Runs),
		Runs:         make([]RunSummaryRow, 0, len(session.Runs)),
	}

	scoredRows := make([]RunSummaryRow, 0, len(session.Runs))
	scoreFailures := 0
	for idx := range session.Runs {
		entry := session.Runs[idx]
		runDir := filepath.Clean(filepath.Join(sessionPath, entry.RelativeRunDir))
		run := runByDir[runDir]
		if run == nil {
			summary.Runs = append(summary.Runs, buildRunSummaryRow(entry, nil, nil, []string{"benchmark run record was not found"}, nil))
			scoreFailures++
			continue
		}
		sessionRecord := &persistence.BenchmarkSessionRecord{
			SessionID:          sessionID,
			SuiteID:            session.SuiteID,
			SuitePath:          session.SuitePath,
			SessionPath:        sessionPath,
			OutputRoot:         session.OutputRoot,
			TemplateID:         session.TemplateID,
			StartedAtUnixMilli: session.StartedAt.UnixMilli(),
			EndedAtUnixMilli:   timePointerMillis(session.EndedAt),
			Status:             session.Status,
			RepeatCount:        session.Repeat,
		}
		scorecard, err := s.scoreRunRecord(ctx, sessionRecord, run)
		if err != nil {
			summary.Runs = append(summary.Runs, buildRunSummaryRow(entry, nil, nil, []string{err.Error()}, nil))
			scoreFailures++
			continue
		}
		row := buildRunSummaryRow(entry, runRecordToManifest(run), scorecard, nil, scorecard.Warnings)
		summary.Runs = append(summary.Runs, row)
		scoredRows = append(scoredRows, row)
	}

	sort.Slice(summary.Runs, func(i, j int) bool { return compareRunRows(summary.Runs[i], summary.Runs[j]) })
	summary.ScoredRunCount = len(scoredRows)
	summary.FailedToScoreCount = scoreFailures
	summary.Aggregates = buildAggregates(scoredRows)
	if scoreFailures > 0 {
		return summary, fmt.Errorf("failed to score %d benchmark run(s)", scoreFailures)
	}
	return summary, nil
}

// ListSuites returns benchmark suite summaries derived from persisted benchmark sessions and runs.
func (s *QueryService) ListSuites(ctx context.Context, filters BenchmarkRunFilters) ([]BenchmarkSuiteSummary, error) {
	s = s.withDefaults()
	if s.store == nil {
		return nil, fmt.Errorf("benchmark query service is not initialized")
	}
	sessions, err := s.store.ListBenchmarkSessions(ctx, persistence.BenchmarkSessionFilter{SuiteID: filters.SuiteID})
	if err != nil {
		return nil, err
	}
	runs, err := s.store.ListBenchmarkRuns(ctx, &persistence.BenchmarkRunFilter{SuiteID: filters.SuiteID})
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
		group := grouped[session.SuiteID]
		if group == nil {
			group = &suiteAggregate{
				item:       BenchmarkSuiteSummary{SuiteID: session.SuiteID},
				sessionIDs: map[string]struct{}{},
				agents:     map[string]struct{}{},
				caseIDs:    map[string]struct{}{},
				caseNames:  map[string]struct{}{},
				variants:   map[string]struct{}{},
			}
			grouped[session.SuiteID] = group
		}
		group.item.TemplateID = firstNonEmpty(group.item.TemplateID, session.TemplateID)
		group.item.SuiteName = firstNonEmpty(group.item.SuiteName, suiteNameFromPath(session.SuitePath))
		group.item.LatestGeneratedAt = latestTime(group.item.LatestGeneratedAt, timeFromMillis(session.EndedAtUnixMilli, session.StartedAtUnixMilli))
		group.sessionIDs[session.SessionID] = struct{}{}
		for caseID, caseName := range caseNamesFromSuitePath(session.SuitePath) {
			group.caseIDs[caseID] = struct{}{}
			if caseName != "" {
				group.caseNames[caseName] = struct{}{}
			}
		}
	}
	for idx := range runs {
		run := runs[idx]
		session := findSessionByID(sessions, run.SessionID)
		if session == nil {
			continue
		}
		if filters.TemplateID != "" && run.TemplateID != filters.TemplateID {
			continue
		}
		group := grouped[session.SuiteID]
		if group == nil {
			continue
		}
		group.item.RunCount++
		group.item.TemplateName = firstNonEmpty(group.item.TemplateName, templateNameForRun(ctx, s.store, &run))
		group.item.LatestGeneratedAt = latestTime(group.item.LatestGeneratedAt, timeFromMillis(run.EndedAtUnixMilli, run.StartedAtUnixMilli))
		group.agents[run.Agent] = struct{}{}
		group.caseIDs[run.CaseID] = struct{}{}
		if caseName := caseNamesFromSuitePath(session.SuitePath)[run.CaseID]; caseName != "" {
			group.caseNames[caseName] = struct{}{}
		}
		group.variants[run.TemplateVariant] = struct{}{}
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

// ListSessions returns suite sessions, optionally including their scored run summaries.
func (s *QueryService) ListSessions(ctx context.Context, suiteID string, filters BenchmarkRunFilters, includeRuns bool) ([]BenchmarkSessionDetail, error) {
	s = s.withDefaults()
	if s.store == nil {
		return nil, fmt.Errorf("benchmark query service is not initialized")
	}
	sessions, err := s.store.ListBenchmarkSessions(ctx, persistence.BenchmarkSessionFilter{SuiteID: suiteID, SessionID: filters.SessionID})
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
		sessionRuns := runsBySession[session.SessionID]
		if hasRunScopedFilter(filters) && len(sessionRuns) == 0 {
			continue
		}
		rows := make([]RunSummaryRow, 0, len(sessionRuns))
		agents := map[string]struct{}{}
		caseIDs := map[string]struct{}{}
		caseNames := map[string]struct{}{}
		variants := map[string]struct{}{}
		templateName := ""
		for runIdx := range sessionRuns {
			rows = append(rows, toRunSummaryRow(sessionRuns[runIdx]))
			agents[sessionRuns[runIdx].Agent] = struct{}{}
			caseIDs[sessionRuns[runIdx].CaseID] = struct{}{}
			if sessionRuns[runIdx].CaseName != "" {
				caseNames[sessionRuns[runIdx].CaseName] = struct{}{}
			}
			variants[sessionRuns[runIdx].TemplateVariant] = struct{}{}
			templateName = firstNonEmpty(templateName, sessionRuns[runIdx].TemplateName)
		}
		detail := BenchmarkSessionDetail{
			SessionID:          session.SessionID,
			SuiteID:            session.SuiteID,
			SuiteName:          suiteNameFromPath(session.SuitePath),
			TemplateID:         session.TemplateID,
			TemplateName:       templateName,
			SessionPath:        session.SessionPath,
			GeneratedAt:        timeFromMillis(session.EndedAtUnixMilli, session.StartedAtUnixMilli),
			RunCount:           len(sessionRuns),
			ScoredRunCount:     len(sessionRuns),
			FailedToScoreCount: 0,
			Agents:             sortedSetValues(agents),
			CaseIDs:            sortedSetValues(caseIDs),
			CaseNames:          sortedSetValues(caseNames),
			TemplateVariants:   sortedSetValues(variants),
			Aggregates:         buildAggregates(rows),
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

// GetSession returns one scored benchmark session by suite and session id.
func (s *QueryService) GetSession(ctx context.Context, suiteID, sessionID string) (*BenchmarkSessionDetail, error) {
	sessions, err := s.ListSessions(ctx, suiteID, BenchmarkRunFilters{SessionID: sessionID}, true)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, ErrBenchmarkSessionNotFound
	}
	return &sessions[0], nil
}

// ListRuns returns scored benchmark runs for one suite, filtered by the provided dimensions.
func (s *QueryService) ListRuns(ctx context.Context, suiteID string, filters BenchmarkRunFilters) ([]BenchmarkRunSummary, error) {
	s = s.withDefaults()
	if s.store == nil {
		return nil, fmt.Errorf("benchmark query service is not initialized")
	}
	sessions, err := s.store.ListBenchmarkSessions(ctx, persistence.BenchmarkSessionFilter{SuiteID: suiteID})
	if err != nil {
		return nil, err
	}
	sessionByID := map[string]*persistence.BenchmarkSessionRecord{}
	for idx := range sessions {
		session := sessions[idx]
		sessionByID[session.SessionID] = &session
	}
	runRecords, err := s.store.ListBenchmarkRuns(ctx, &persistence.BenchmarkRunFilter{
		SuiteID:         suiteID,
		SessionID:       filters.SessionID,
		CaseID:          filters.CaseID,
		Agent:           filters.Agent,
		TemplateVariant: filters.TemplateVariant,
	})
	if err != nil {
		return nil, err
	}
	result := make([]BenchmarkRunSummary, 0, len(runRecords))
	for idx := range runRecords {
		run := runRecords[idx]
		if filters.TemplateID != "" && run.TemplateID != filters.TemplateID {
			continue
		}
		session := sessionByID[run.SessionID]
		if session == nil {
			continue
		}
		scorecard, err := s.scoreRunRecord(ctx, session, &run)
		if err != nil {
			result = append(result, runSummaryFromRecord(*session, run, err))
			continue
		}
		result = append(result, buildBenchmarkRunSummary(&run, scorecard))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SessionPath != result[j].SessionPath {
			return result[i].SessionPath > result[j].SessionPath
		}
		return compareRunRows(toRunSummaryRow(result[i]), toRunSummaryRow(result[j]))
	})
	return result, nil
}

// GetRun returns one scored benchmark run detail by suite and run id.
func (s *QueryService) GetRun(ctx context.Context, suiteID, scorecardID string) (*BenchmarkRunDetail, error) {
	s = s.withDefaults()
	run, err := s.store.GetBenchmarkRun(ctx, scorecardID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBenchmarkRunNotFound
		}
		return nil, err
	}
	if run == nil {
		return nil, ErrBenchmarkRunNotFound
	}
	sessions, err := s.store.ListBenchmarkSessions(ctx, persistence.BenchmarkSessionFilter{SuiteID: suiteID, SessionID: run.SessionID})
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, ErrBenchmarkRunNotFound
	}
	scorecard, err := s.scoreRunRecord(ctx, &sessions[0], run)
	if err != nil {
		return nil, err
	}
	return &BenchmarkRunDetail{
		ScorecardID:  run.BenchmarkRunID,
		SessionID:    run.SessionID,
		SessionPath:  sessions[0].SessionPath,
		SuiteName:    scorecard.SuiteName,
		TemplateName: scorecard.TemplateName,
		CaseName:     scorecard.CaseName,
		Scorecard:    *scorecard,
	}, nil
}

// GetComparison returns a live comparison view for one suite and filter set.
func (s *QueryService) GetComparison(ctx context.Context, suiteID string, filters BenchmarkRunFilters) (*BenchmarkComparisonView, error) {
	sessions, err := s.ListSessions(ctx, suiteID, filters, false)
	if err != nil {
		return nil, err
	}
	runs, err := s.ListRuns(ctx, suiteID, filters)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 && len(runs) == 0 {
		return nil, ErrBenchmarkComparisonNotFound
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

func (s *QueryService) scoreRunRecord(ctx context.Context, session *persistence.BenchmarkSessionRecord, run *persistence.BenchmarkRunRecord) (*RunScorecard, error) {
	if session == nil || run == nil {
		return nil, fmt.Errorf("benchmark session and run are required")
	}
	suiteCtx, err := loadSuiteContext(session.SuitePath)
	if err != nil {
		return nil, err
	}
	caseCtx, ok := suiteCtx.caseDefs[run.CaseID]
	if !ok {
		return nil, fmt.Errorf("missing case definition for %q", run.CaseID)
	}
	latestTaskRunID := strings.TrimSpace(run.LatestTaskRunID)
	if latestTaskRunID == "" {
		latestTaskRunID = latestLinkedTaskRunID(run.LinkedTaskRunIDs)
	}
	if latestTaskRunID == "" {
		return nil, fmt.Errorf("benchmark run %q is missing linked task runs", run.RunDir)
	}
	snapshot, err := s.store.GetTaskRunSnapshot(ctx, latestTaskRunID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("task run snapshot %q was not found", latestTaskRunID)
		}
		return nil, err
	}
	stats, err := s.store.GetTaskRunStats(ctx, latestTaskRunID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("task run stats %q were not found", latestTaskRunID)
		}
		return nil, err
	}
	manual, manualPath, err := loadManualScore(filepath.Join(run.RunDir, manualScoreFileName))
	if err != nil {
		return nil, err
	}
	agentStdoutPath := filepath.Join(run.AgentDir, "agent.stdout.log")
	agentStderrPath := filepath.Join(run.AgentDir, "agent.stderr.log")
	agentMetadata, warnings, err := runAgentMetadata(run, agentStdoutPath)
	if err != nil {
		return nil, err
	}
	invariantViolation, err := detectInvariantViolation(
		filepath.Join(caseCtx.caseRoot, caseCtx.caseDef.Fixture.SeedPath),
		run.ProjectDir,
		caseCtx.caseDef.Constraints.LockedPaths,
	)
	if err != nil {
		return nil, err
	}
	editedFiles, err := collectEditedFiles(filepath.Join(caseCtx.caseRoot, caseCtx.caseDef.Fixture.SeedPath), run.ProjectDir)
	if err != nil {
		return nil, err
	}
	templateID := firstNonEmpty(run.TemplateID, snapshot.TemplateID)
	templateName := templateNameFromSnapshot(snapshot)
	if templateName == "" {
		templateName = templateID
	}
	finalVerificationPassed := strings.TrimSpace(snapshot.Status) == runStatusCompleted
	restartCount := stats.RestartCount
	failCount := stats.FailCount
	timeoutCount := stats.TimeoutCount
	outcome := ScorecardOutcome{
		CompletedSuccessfully:   run.Status == runStatusCompleted && finalVerificationPassed,
		FinalVerificationPassed: finalVerificationPassed,
		FirstPassSuccess:        finalVerificationPassed && restartCount == 0 && failCount == 0 && timeoutCount == 0,
		RestartOccurred:         restartCount > 0,
		FailOccurred:            failCount > 0,
		TimeoutOccurred:         timeoutCount > 0,
		InvariantViolation:      invariantViolation,
	}
	process := ScorecardProcess{
		FailedTaskToolCalls:       stats.TaskToolErrorCount,
		FailedDownstreamToolCalls: stats.DownstreamToolErrorCount,
		TotalTaskToolCalls:        stats.TaskToolCallCount,
		TotalDownstreamToolCalls:  stats.DownstreamToolCallCount,
		RestartCount:              restartCount,
		FailCount:                 failCount,
		TimeoutCount:              timeoutCount,
	}
	efficiency := ScorecardEfficiency{
		WallClockSeconds:     durationSeconds(stats.DurationMillis, timeFromUnixMillis(run.StartedAtUnixMilli), timeFromMillis(run.EndedAtUnixMilli, run.StartedAtUnixMilli)),
		TotalToolCalls:       stats.TaskToolCallCount + stats.DownstreamToolCallCount,
		InputTokens:          agentUsageInputTokens(agentMetadata),
		OutputTokens:         agentUsageOutputTokens(agentMetadata),
		EditedFilesCount:     len(editedFiles),
		EditedFiles:          editedFiles,
		ObservedCommandCalls: 0,
	}
	return &RunScorecard{
		SuiteID:         session.SuiteID,
		SuiteName:       suiteCtx.suite.Suite.Name,
		CaseID:          run.CaseID,
		CaseName:        caseCtx.caseDef.Case.Name,
		TemplateID:      templateID,
		TemplateName:    templateName,
		TemplateVariant: run.TemplateVariant,
		Agent:           run.Agent,
		Attempt:         run.Attempt,
		RunManifestPath: filepath.Join(run.RunDir, runFileName),
		SessionPath:     session.SessionPath,
		EventStoreMode:  run.EventStoreMode,
		EventStorePath:  run.EventStorePath,
		RequestLogPath:  run.RequestLogPath,
		AgentStdoutPath: agentStdoutPath,
		AgentStderrPath: agentStderrPath,
		ManualScorePath: manualPath,
		RawStatus:       run.Status,
		LatestTaskRunID: latestTaskRunID,
		LinkedTaskRunIDs: append([]string(nil),
			run.LinkedTaskRunIDs...,
		),
		Outcome:    outcome,
		Process:    process,
		Efficiency: efficiency,
		Manual: ScorecardManual{
			ErrorActionabilityScore: manual.ErrorActionabilityScore,
			ErrorActionabilityNotes: manual.Notes,
		},
		AgentMetadata: agentMetadata,
		ScoreVersion:  scoreVersion,
		GeneratedAt:   s.now(),
		Warnings:      warnings,
	}, nil
}

func runAgentMetadata(run *persistence.BenchmarkRunRecord, agentStdoutPath string) (*AgentMetadata, []string, error) {
	if run == nil {
		return nil, nil, fmt.Errorf("benchmark run is required")
	}
	if len(run.AgentMetadataJSON) > 0 {
		var metadata AgentMetadata
		if err := json.Unmarshal(run.AgentMetadataJSON, &metadata); err != nil {
			return nil, nil, fmt.Errorf("unmarshal benchmark agent metadata: %w", err)
		}
		return &metadata, nil, nil
	}
	return loadAgentMetadata(agentStdoutPath, run.Agent)
}

func findSessionByID(sessions []persistence.BenchmarkSessionRecord, sessionID string) *persistence.BenchmarkSessionRecord {
	for idx := range sessions {
		if sessions[idx].SessionID == sessionID {
			return &sessions[idx]
		}
	}
	return nil
}

func templateNameForRun(ctx context.Context, store benchmarkQueryStore, run *persistence.BenchmarkRunRecord) string {
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

func caseNamesFromSuitePath(suitePath string) map[string]string {
	result := map[string]string{}
	suiteCtx, err := loadSuiteContext(suitePath)
	if err != nil {
		return result
	}
	for caseID, caseCtx := range suiteCtx.caseDefs {
		result[caseID] = caseCtx.caseDef.Case.Name
	}
	return result
}

func suiteNameFromPath(suitePath string) string {
	suiteCtx, err := loadSuiteContext(suitePath)
	if err != nil || suiteCtx == nil || suiteCtx.suite == nil {
		return ""
	}
	return suiteCtx.suite.Suite.Name
}

func templateNameFromSnapshot(snapshot *persistence.TaskRunSnapshotRecord) string {
	if snapshot == nil || snapshot.Payload == nil {
		return ""
	}
	runnableTemplateName := ""
	if snapshot.Payload.RunnableTemplate != nil {
		runnableTemplateName = snapshot.Payload.RunnableTemplate.Task.Name
	}
	return firstNonEmpty(
		snapshot.TemplateName,
		runnableTemplateName,
		snapshot.Payload.SelectedTemplate.Task.Name,
	)
}

func latestLinkedTaskRunID(runIDs []string) string {
	for idx := len(runIDs) - 1; idx >= 0; idx-- {
		if trimmed := strings.TrimSpace(runIDs[idx]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func durationSeconds(durationMillis *int64, start, end time.Time) float64 {
	if durationMillis != nil && *durationMillis > 0 {
		return float64(*durationMillis) / 1000.0
	}
	return secondsBetween(start, end)
}

func timeNowUTC() time.Time {
	return time.Now().UTC()
}

func timeFromMillis(primary *int64, fallback int64) time.Time {
	if primary != nil && *primary > 0 {
		return time.UnixMilli(*primary).UTC()
	}
	return time.UnixMilli(fallback).UTC()
}

func timeFromUnixMillis(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}

func latestTime(current, candidate time.Time) time.Time {
	if current.IsZero() {
		return candidate
	}
	if candidate.After(current) {
		return candidate
	}
	return current
}

func runRecordToManifest(run *persistence.BenchmarkRunRecord) *RunManifest {
	if run == nil {
		return nil
	}
	return &RunManifest{
		SuiteID:             "",
		CaseID:              run.CaseID,
		TemplateID:          run.TemplateID,
		TemplateVariant:     TemplateVariant{Name: run.TemplateVariant},
		AgentID:             run.Agent,
		Attempt:             run.Attempt,
		SelectedModel:       run.SelectedModel,
		StartedAt:           timeFromUnixMillis(run.StartedAtUnixMilli),
		EndedAt:             timeFromMillis(run.EndedAtUnixMilli, run.StartedAtUnixMilli),
		Status:              run.Status,
		LatestTaskRunID:     run.LatestTaskRunID,
		LatestTaskRunStatus: run.LatestTaskRunStatus,
		LinkedTaskRunIDs:    append([]string(nil), run.LinkedTaskRunIDs...),
		ArtifactPaths: RunArtifactPaths{
			RunDir:               run.RunDir,
			ProjectDir:           run.ProjectDir,
			LogsDir:              run.LogsDir,
			AgentDir:             run.AgentDir,
			ConfigPath:           run.ConfigPath,
			EventStoreMode:       run.EventStoreMode,
			EventStorePath:       run.EventStorePath,
			RequestLogPath:       run.RequestLogPath,
			SelectedTemplatePath: run.SelectedTemplatePath,
		},
		ErrorSummary: run.ErrorSummary,
	}
}
