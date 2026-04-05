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

type taskRunStatsReader interface {
	GetTaskRunStats(context.Context, string) (*persistence.TaskRunStatsRecord, error)
}

// ArtifactReader lists persisted benchmark artifact blobs for the read API.
type ArtifactReader interface {
	ListBenchmarkArtifacts(context.Context, persistence.BenchmarkArtifactFilter) ([]persistence.BenchmarkArtifactRecord, error)
}

type taskRunSnapshotReader interface {
	GetTaskRunSnapshot(context.Context, string) (*persistence.TaskRunSnapshotRecord, error)
}

type benchmarkReadStore interface {
	ArtifactReader
	taskRunSnapshotReader
	taskRunStatsReader
}

type persistedSession struct {
	record  persistence.BenchmarkArtifactRecord
	session SessionManifest
}

type persistedRun struct {
	record persistence.BenchmarkArtifactRecord
	run    RunManifest
}

type suiteContext struct {
	suite    *SuiteDefinition
	caseDefs map[string]scoreRunContext
}

type liveQueryService struct {
	now   func() time.Time
	store benchmarkReadStore
}

func newLiveQueryService(store benchmarkReadStore, now func() time.Time) *liveQueryService {
	if now == nil {
		now = time.Now
	}
	return &liveQueryService{now: now, store: store}
}

func (s *liveQueryService) scoreSessionManifest(ctx context.Context, session *SessionManifest) (*SessionSummary, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("benchmark live query service is not initialized")
	}
	if session == nil {
		return nil, fmt.Errorf("session manifest is required")
	}
	sessionDir := filepath.Clean(strings.TrimSpace(session.InvocationDir))
	if sessionDir == "" {
		return nil, fmt.Errorf("session manifest invocationDir is required")
	}
	suiteCtx, err := loadSuiteContext(session.SuitePath)
	if err != nil {
		return nil, err
	}

	summary := &SessionSummary{
		ScoreVersion: scoreVersion,
		SessionPath:  sessionDir,
		SuiteID:      session.SuiteID,
		GeneratedAt:  s.now(),
		RunCount:     len(session.Runs),
		Runs:         make([]RunSummaryRow, 0, len(session.Runs)),
	}

	scoredRows := make([]RunSummaryRow, 0, len(session.Runs))
	scoreFailures := 0
	for idx := range session.Runs {
		entry := session.Runs[idx]
		run, err := loadRunManifest(filepath.Join(sessionDir, entry.RelativeRunDir, runFileName))
		if err != nil {
			summary.Runs = append(summary.Runs, buildRunSummaryRow(entry, nil, nil, []string{err.Error()}, nil))
			scoreFailures++
			continue
		}
		scorecard, err := s.scoreRun(ctx, sessionDir, suiteCtx, run)
		if err != nil {
			summary.Runs = append(summary.Runs, buildRunSummaryRow(entry, run, nil, []string{err.Error()}, nil))
			scoreFailures++
			continue
		}
		row := buildRunSummaryRow(entry, run, scorecard, nil, scorecard.Warnings)
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

func (s *liveQueryService) scorePersistedRun(ctx context.Context, session *persistedSession, item *persistedRun) (*RunScorecard, error) {
	if session == nil || item == nil {
		return nil, fmt.Errorf("persisted session and run are required")
	}
	suiteCtx, err := loadSuiteContext(session.session.SuitePath)
	if err != nil {
		return nil, err
	}
	return s.scoreRun(ctx, session.record.SessionPath, suiteCtx, &item.run)
}

func (s *liveQueryService) scoreRun(ctx context.Context, sessionDir string, suiteCtx *suiteContext, run *RunManifest) (*RunScorecard, error) {
	if run == nil {
		return nil, fmt.Errorf("run manifest is required")
	}
	if suiteCtx == nil || suiteCtx.suite == nil {
		return nil, fmt.Errorf("suite context is required")
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
		return nil, fmt.Errorf("run %q is missing linked task runs", run.ArtifactPaths.RunDir)
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

	manualPath := filepath.Join(run.ArtifactPaths.RunDir, manualScoreFileName)
	manual, manualPathValue, err := loadManualScore(manualPath)
	if err != nil {
		return nil, err
	}
	agentStdoutPath := filepath.Join(run.ArtifactPaths.AgentDir, "agent.stdout.log")
	agentStderrPath := filepath.Join(run.ArtifactPaths.AgentDir, "agent.stderr.log")
	agentMetadata, warnings, err := loadAgentMetadata(agentStdoutPath, run.AgentID)
	if err != nil {
		return nil, err
	}

	templateID := firstNonEmpty(run.TemplateID, snapshot.TemplateID)
	templateName := templateNameFromSnapshot(snapshot)
	if templateName == "" {
		templateName = templateID
	}
	caseName := caseCtx.caseDef.Case.Name
	suiteName := suiteCtx.suite.Suite.Name

	invariantViolation, err := detectInvariantViolation(
		filepath.Join(caseCtx.caseRoot, caseCtx.caseDef.Fixture.SeedPath),
		run.ArtifactPaths.ProjectDir,
		caseCtx.caseDef.Constraints.LockedPaths,
	)
	if err != nil {
		return nil, err
	}
	editedFiles, err := collectEditedFiles(filepath.Join(caseCtx.caseRoot, caseCtx.caseDef.Fixture.SeedPath), run.ArtifactPaths.ProjectDir)
	if err != nil {
		return nil, err
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
		WallClockSeconds:     durationSeconds(stats.DurationMillis, run.StartedAt, run.EndedAt),
		TotalToolCalls:       stats.TaskToolCallCount + stats.DownstreamToolCallCount,
		InputTokens:          agentUsageInputTokens(agentMetadata),
		OutputTokens:         agentUsageOutputTokens(agentMetadata),
		EditedFilesCount:     len(editedFiles),
		EditedFiles:          editedFiles,
		ObservedCommandCalls: 0,
	}

	return &RunScorecard{
		SuiteID:         run.SuiteID,
		SuiteName:       suiteName,
		CaseID:          run.CaseID,
		CaseName:        caseName,
		TemplateID:      templateID,
		TemplateName:    templateName,
		TemplateVariant: run.TemplateVariant.Name,
		Agent:           run.AgentID,
		Attempt:         run.Attempt,
		RunManifestPath: filepath.Join(run.ArtifactPaths.RunDir, runFileName),
		SessionPath:     sessionDir,
		EventStoreMode:  run.ArtifactPaths.EventStoreMode,
		EventStorePath:  run.ArtifactPaths.EventStorePath,
		RequestLogPath:  run.ArtifactPaths.RequestLogPath,
		AgentStdoutPath: agentStdoutPath,
		AgentStderrPath: agentStderrPath,
		ManualScorePath: manualPathValue,
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

func loadSuiteContext(suiteRoot string) (*suiteContext, error) {
	suite, err := LoadSuite(suiteRoot)
	if err != nil {
		return nil, fmt.Errorf("load suite for benchmark scoring: %w", err)
	}
	caseDefs, err := loadCaseContexts(suiteRoot, suite)
	if err != nil {
		return nil, err
	}
	return &suiteContext{suite: suite, caseDefs: caseDefs}, nil
}

func loadRunManifest(path string) (*RunManifest, error) {
	var run RunManifest
	if err := readJSONFile(path, &run); err != nil {
		return nil, fmt.Errorf("load run manifest %q: %w", path, err)
	}
	return &run, nil
}

func latestLinkedTaskRunID(runIDs []string) string {
	for idx := len(runIDs) - 1; idx >= 0; idx-- {
		if trimmed := strings.TrimSpace(runIDs[idx]); trimmed != "" {
			return trimmed
		}
	}
	return ""
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

func durationSeconds(durationMillis *int64, start time.Time, end time.Time) float64 {
	if durationMillis != nil && *durationMillis > 0 {
		return float64(*durationMillis) / 1000.0
	}
	return secondsBetween(start, end)
}

func loadPersistedSessions(ctx context.Context, store ArtifactReader, suiteID string) ([]persistedSession, error) {
	filter := persistence.BenchmarkArtifactFilter{ArtifactKind: persistence.BenchmarkArtifactKindSession}
	if strings.TrimSpace(suiteID) != "" {
		filter.SuiteID = suiteID
	}
	records, err := store.ListBenchmarkArtifacts(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := make([]persistedSession, 0, len(records))
	for idx := range records {
		var session SessionManifest
		if err := json.Unmarshal(records[idx].PayloadJSON, &session); err != nil {
			return nil, fmt.Errorf("decode session artifact %s: %w", records[idx].ID, err)
		}
		result = append(result, persistedSession{record: records[idx], session: session})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].record.CreatedAtUnixMilli != result[j].record.CreatedAtUnixMilli {
			return result[i].record.CreatedAtUnixMilli > result[j].record.CreatedAtUnixMilli
		}
		return result[i].record.ID < result[j].record.ID
	})
	return result, nil
}

func loadPersistedRuns(ctx context.Context, store ArtifactReader, suiteID string, filters BenchmarkRunFilters) ([]persistedRun, error) {
	filter := persistence.BenchmarkArtifactFilter{ArtifactKind: persistence.BenchmarkArtifactKindRun}
	if strings.TrimSpace(suiteID) != "" {
		filter.SuiteID = suiteID
	}
	filter.SessionID = strings.TrimSpace(filters.SessionID)
	filter.CaseID = strings.TrimSpace(filters.CaseID)
	filter.Agent = strings.TrimSpace(filters.Agent)
	filter.TemplateVariant = strings.TrimSpace(filters.TemplateVariant)

	records, err := store.ListBenchmarkArtifacts(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := make([]persistedRun, 0, len(records))
	for idx := range records {
		var run RunManifest
		if err := json.Unmarshal(records[idx].PayloadJSON, &run); err != nil {
			return nil, fmt.Errorf("decode run artifact %s: %w", records[idx].ID, err)
		}
		if filters.TemplateID != "" && run.TemplateID != filters.TemplateID {
			continue
		}
		result = append(result, persistedRun{record: records[idx], run: run})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].record.SessionPath != result[j].record.SessionPath {
			return result[i].record.SessionPath > result[j].record.SessionPath
		}
		return compareRunRows(
			RunSummaryRow{CaseID: result[i].run.CaseID, Agent: result[i].run.AgentID, TemplateVariant: result[i].run.TemplateVariant.Name, Attempt: result[i].run.Attempt},
			RunSummaryRow{CaseID: result[j].run.CaseID, Agent: result[j].run.AgentID, TemplateVariant: result[j].run.TemplateVariant.Name, Attempt: result[j].run.Attempt},
		)
	})
	return result, nil
}
