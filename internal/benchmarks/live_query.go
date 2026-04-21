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

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/persistence"
)

// Benchmark read/query errors exposed to higher-level API and UI callers.
var (
	// ErrBenchmarkSessionNotFound indicates that the requested benchmark session does not exist.
	ErrBenchmarkSessionNotFound = errors.New("benchmark session not found")
	// ErrBenchmarkRunNotFound indicates that the requested benchmark run does not exist.
	ErrBenchmarkRunNotFound = errors.New("benchmark run not found")
	// ErrBenchmarkComparisonNotFound indicates that no benchmark data matched the requested comparison scope.
	ErrBenchmarkComparisonNotFound = errors.New("benchmark comparison not found")
)

// benchmarkQueryStore captures the persistence reads needed by the live query layer.
type benchmarkQueryStore interface {
	ListBenchmarkSessions(context.Context, persistence.BenchmarkSessionFilter) ([]persistence.BenchmarkSessionRecord, error)
	ListBenchmarkRuns(context.Context, *persistence.BenchmarkRunFilter) ([]persistence.BenchmarkRunRecord, error)
	GetBenchmarkRun(context.Context, string) (*persistence.BenchmarkRunRecord, error)
	ListBenchmarkRunScores(context.Context) ([]persistence.BenchmarkRunScoreRecord, error)
	GetBenchmarkRunScore(context.Context, string) (*persistence.BenchmarkRunScoreRecord, error)
	ListTaskRunSnapshots(context.Context) ([]persistence.TaskRunSnapshotRecord, error)
	GetTaskRunSnapshot(context.Context, string) (*persistence.TaskRunSnapshotRecord, error)
	ListTaskRunStats(context.Context) ([]persistence.TaskRunStatsRecord, error)
	GetTaskRunStats(context.Context, string) (*persistence.TaskRunStatsRecord, error)
	RecomputeTaskRunStats(context.Context, string) error
}

// QueryService is the single live benchmark query/scoring service used by the API and UI.
type QueryService struct {
	now   func() time.Time
	store benchmarkQueryStore
}

// NewQueryService builds a live benchmark query service.
func NewQueryService(store benchmarkQueryStore) *QueryService {
	return &QueryService{now: common.NowUTC, store: store}
}

// withDefaults ensures the query service always has a clock configured.
func (s *QueryService) withDefaults() *QueryService {
	if s == nil {
		return &QueryService{now: common.NowUTC}
	}
	if s.now == nil {
		s.now = common.NowUTC
	}
	return s
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
		group.item.TemplateID = common.FirstNonEmpty(group.item.TemplateID, session.TemplateID)
		group.item.TemplateName = common.FirstNonEmpty(group.item.TemplateName, session.TemplateName)
		group.item.SuiteName = common.FirstNonEmpty(group.item.SuiteName, session.SuiteName)
		group.item.LatestGeneratedAt = common.LaterTime(group.item.LatestGeneratedAt, common.TimeFromUnixMillisOrFallback(session.EndedAtUnixMilli, session.StartedAtUnixMilli))
		group.sessionIDs[session.SessionID] = struct{}{}
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
		group.item.TemplateName = common.FirstNonEmpty(group.item.TemplateName, run.TemplateName, session.TemplateName)
		group.item.LatestGeneratedAt = common.LaterTime(group.item.LatestGeneratedAt, common.TimeFromUnixMillisOrFallback(run.EndedAtUnixMilli, run.StartedAtUnixMilli))
		group.agents[run.Agent] = struct{}{}
		group.caseIDs[run.CaseID] = struct{}{}
		if caseName := strings.TrimSpace(run.CaseName); caseName != "" {
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
		group.item.Agents = common.SortedSetValues(group.agents)
		group.item.CaseIDs = common.SortedSetValues(group.caseIDs)
		group.item.CaseNames = common.SortedSetValues(group.caseNames)
		group.item.TemplateVariants = common.SortedSetValues(group.variants)
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
			templateName = common.FirstNonEmpty(templateName, sessionRuns[runIdx].TemplateName)
		}
		detail := BenchmarkSessionDetail{
			SessionID:          session.SessionID,
			SuiteID:            session.SuiteID,
			SuiteName:          session.SuiteName,
			TemplateID:         session.TemplateID,
			TemplateName:       common.FirstNonEmpty(templateName, session.TemplateName),
			SessionPath:        session.SessionPath,
			GeneratedAt:        common.TimeFromUnixMillisOrFallback(session.EndedAtUnixMilli, session.StartedAtUnixMilli),
			RunCount:           len(sessionRuns),
			ScoredRunCount:     common.CountBy(rows, func(row RunSummaryRow) bool { return row.Scored }),
			FailedToScoreCount: common.CountBy(rows, func(row RunSummaryRow) bool { return !row.Scored }),
			Agents:             common.SortedSetValues(agents),
			CaseIDs:            common.SortedSetValues(caseIDs),
			CaseNames:          common.SortedSetValues(caseNames),
			TemplateVariants:   common.SortedSetValues(variants),
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
	scoreRows, err := s.store.ListBenchmarkRunScores(ctx)
	if err != nil {
		return nil, err
	}
	scoreByRunID := make(map[string]*persistence.BenchmarkRunScoreRecord, len(scoreRows))
	for idx := range scoreRows {
		score := scoreRows[idx]
		scoreByRunID[score.BenchmarkRunID] = &score
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
		result = append(result, buildBenchmarkRunSummary(&run, session, scoreByRunID[run.BenchmarkRunID]))
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
	score, err := s.store.GetBenchmarkRunScore(ctx, scorecardID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		score = nil
	}
	scorecard, scoreErr := scorecardFromSnapshot(score)
	scoreErrors := []string{"benchmark score unavailable"}
	if score != nil && len(score.ScoreErrors) > 0 {
		scoreErrors = append([]string(nil), score.ScoreErrors...)
	}
	if scoreErr != nil && !errors.Is(scoreErr, errBenchmarkScoreUnavailable) {
		scoreErrors = []string{scoreErr.Error()}
	}
	return &BenchmarkRunDetail{
		ScorecardID:  run.BenchmarkRunID,
		SessionID:    run.SessionID,
		SessionPath:  sessions[0].SessionPath,
		SuiteName:    common.FirstNonEmpty(sessions[0].SuiteName),
		TemplateName: common.FirstNonEmpty(run.TemplateName, sessions[0].TemplateName),
		CaseName:     run.CaseName,
		Scored:       scorecard != nil,
		ScoreErrors:  scoreErrorsIfUnscored(scorecard, scoreErrors),
		Scorecard:    scorecard,
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
		templateID = common.FirstNonEmpty(templateID, runs[idx].TemplateID)
		templateName = common.FirstNonEmpty(templateName, runs[idx].TemplateName)
		suiteName = common.FirstNonEmpty(suiteName, runs[idx].SuiteName)
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

// scoreRunRecord derives a fresh run scorecard from persisted benchmark and task-run state.
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
	if len(run.LinkedTaskRunIDs) == 0 {
		return nil, fmt.Errorf("benchmark run %q is missing linked task runs", run.RunDir)
	}
	snapshots, err := s.loadLinkedTaskRunSnapshots(ctx, run.LinkedTaskRunIDs)
	if err != nil {
		return nil, err
	}
	lastSnapshot := snapshots[len(snapshots)-1]
	s.recomputeStatsForLinkedRuns(ctx, run.LinkedTaskRunIDs)
	stats, err := s.aggregateTaskRunStats(ctx, run.LinkedTaskRunIDs)
	if err != nil {
		return nil, err
	}
	agentStdoutPath := filepath.Join(run.AgentDir, "agent.stdout.log")
	agentStderrPath := filepath.Join(run.AgentDir, "agent.stderr.log")
	agentMetadata, warnings, err := runAgentMetadata(run, agentStdoutPath)
	if err != nil {
		return nil, err
	}
	selectedModel := strings.TrimSpace(common.FirstNonEmpty(run.SelectedModel, agentMetadataSelectedModel(agentMetadata)))
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
	templateID := common.FirstNonEmpty(run.TemplateID, session.TemplateID, lastSnapshot.TemplateID)
	templateName := common.FirstNonEmpty(run.TemplateName, session.TemplateName, templateNameFromSnapshot(lastSnapshot))
	if templateName == "" {
		templateName = templateID
	}
	finalVerificationPassed := strings.TrimSpace(lastSnapshot.Status) == runStatusCompleted
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
		WallClockSeconds:     common.DurationSeconds(stats.DurationMillis, common.TimeFromUnixMillis(run.StartedAtUnixMilli), common.TimeFromUnixMillisOrFallback(run.EndedAtUnixMilli, run.StartedAtUnixMilli)),
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
		SelectedModel:   selectedModel,
		Attempt:         run.Attempt,
		RunManifestPath: filepath.Join(run.RunDir, runFileName),
		SessionPath:     session.SessionPath,
		EventStoreMode:  run.EventStoreMode,
		EventStorePath:  run.EventStorePath,
		RequestLogPath:  run.RequestLogPath,
		AgentStdoutPath: agentStdoutPath,
		AgentStderrPath: agentStderrPath,
		RawStatus:       run.Status,
		LinkedTaskRunIDs: append([]string(nil),
			run.LinkedTaskRunIDs...,
		),
		Outcome:       outcome,
		Process:       process,
		Efficiency:    efficiency,
		AgentMetadata: agentMetadata,
		ScoreVersion:  scoreVersion,
		GeneratedAt:   s.now(),
		Warnings:      warnings,
	}, nil
}

// aggregateTaskRunStats sums stats from all linked task run IDs so that errors from
// scaffolding or intermediate runs are not lost.
func (s *QueryService) aggregateTaskRunStats(ctx context.Context, linkedRunIDs []string) (*persistence.TaskRunStatsRecord, error) {
	var aggregate *persistence.TaskRunStatsRecord
	for _, runID := range orderedUniqueRunIDs(linkedRunIDs) {
		stats, err := s.store.GetTaskRunStats(ctx, runID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("task run stats %q were not found", runID)
			}
			return nil, err
		}
		if aggregate == nil {
			statsCopy := *stats
			aggregate = &statsCopy
			continue
		}
		aggregate.TaskToolCallCount += stats.TaskToolCallCount
		aggregate.DownstreamToolCallCount += stats.DownstreamToolCallCount
		aggregate.TaskToolErrorCount += stats.TaskToolErrorCount
		aggregate.DownstreamToolErrorCount += stats.DownstreamToolErrorCount
		aggregate.RestartCount += stats.RestartCount
		aggregate.FailCount += stats.FailCount
		aggregate.TimeoutCount += stats.TimeoutCount
	}
	if aggregate == nil {
		return nil, fmt.Errorf("benchmark run is missing linked task runs")
	}
	return aggregate, nil
}

// recomputeStatsForLinkedRuns refreshes task_run_stats from raw events for all unique run IDs
// associated with a benchmark run. Errors are ignored — stale stats are preferable to a hard failure.
func (s *QueryService) recomputeStatsForLinkedRuns(ctx context.Context, linkedRunIDs []string) {
	seen := make(map[string]struct{})
	for _, runID := range linkedRunIDs {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		if _, already := seen[runID]; already {
			continue
		}
		seen[runID] = struct{}{}
		_ = s.store.RecomputeTaskRunStats(ctx, runID)
	}
}

func (s *QueryService) loadLinkedTaskRunSnapshots(ctx context.Context, linkedRunIDs []string) ([]*persistence.TaskRunSnapshotRecord, error) {
	orderedIDs := orderedUniqueRunIDs(linkedRunIDs)
	if len(orderedIDs) == 0 {
		return nil, fmt.Errorf("benchmark run is missing linked task runs")
	}
	snapshots := make([]*persistence.TaskRunSnapshotRecord, 0, len(orderedIDs))
	for _, runID := range orderedIDs {
		snapshot, err := s.store.GetTaskRunSnapshot(ctx, runID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("task run snapshot %q was not found", runID)
			}
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

// runAgentMetadata prefers persisted agent metadata and falls back to parsing stdout logs.
func runAgentMetadata(run *persistence.BenchmarkRunRecord, agentStdoutPath string) (*AgentMetadata, []string, error) {
	if run == nil {
		return nil, nil, fmt.Errorf("benchmark run is required")
	}
	if len(run.AgentMetadataJSON) > 0 {
		var metadata AgentMetadata
		if err := json.Unmarshal(run.AgentMetadataJSON, &metadata); err != nil {
			return nil, nil, fmt.Errorf("unmarshal benchmark agent metadata: %w", err)
		}
		return enrichAgentMetadata(&metadata, run.SelectedModel), nil, nil
	}
	metadata, warnings, err := loadAgentMetadata(agentStdoutPath, run.Agent)
	return enrichAgentMetadata(metadata, run.SelectedModel), warnings, err
}

// enrichAgentMetadata fills gaps that can be recovered from the run manifest itself.
func enrichAgentMetadata(metadata *AgentMetadata, selectedModel string) *AgentMetadata {
	model := strings.TrimSpace(selectedModel)
	if metadata == nil {
		if model == "" {
			return nil
		}
		return &AgentMetadata{SelectedModel: model}
	}
	if strings.TrimSpace(metadata.SelectedModel) == "" {
		metadata.SelectedModel = model
	}
	return metadata
}

// agentMetadataSelectedModel returns the selected model from parsed agent metadata.
func agentMetadataSelectedModel(metadata *AgentMetadata) string {
	if metadata == nil {
		return ""
	}
	return metadata.SelectedModel
}

// scoreErrorsIfUnscored exposes score errors only when the run still lacks a scorecard.
func scoreErrorsIfUnscored(scorecard *RunScorecard, scoreErrors []string) []string {
	if scorecard != nil {
		return nil
	}
	return append([]string(nil), scoreErrors...)
}

// findSessionByID finds one persisted session row by its stable identifier.
func findSessionByID(sessions []persistence.BenchmarkSessionRecord, sessionID string) *persistence.BenchmarkSessionRecord {
	for idx := range sessions {
		if sessions[idx].SessionID == sessionID {
			return &sessions[idx]
		}
	}
	return nil
}

func orderedUniqueRunIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// templateNameFromSnapshot derives the most specific template name available in the snapshot payload.
func templateNameFromSnapshot(snapshot *persistence.TaskRunSnapshotRecord) string {
	if snapshot == nil || snapshot.Payload == nil {
		return ""
	}
	runnableTemplateName := ""
	if snapshot.Payload.RunnableTemplate != nil {
		runnableTemplateName = snapshot.Payload.RunnableTemplate.Task.Name
	}
	return common.FirstNonEmpty(
		snapshot.TemplateName,
		runnableTemplateName,
		snapshot.Payload.SelectedTemplate.Task.Name,
	)
}
