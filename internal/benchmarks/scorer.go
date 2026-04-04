package benchmarks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/persistence"
)

const (
	scorecardFileName   = "scorecard.json"
	summaryFileName     = "summary.json"
	manualScoreFileName = "manual_score.json"
	scoreVersion        = "v1"
)

var commandToolNames = map[string]struct{}{
	"functions.exec_command":         {},
	"serena___execute_shell_command": {},
	"shell__exec":                    {},
}

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

// RunScorecard stores derived metrics for one concrete benchmark run.
type RunScorecard struct {
	SuiteID          string              `json:"suiteId"`
	CaseID           string              `json:"caseId"`
	TemplateID       string              `json:"templateId"`
	TemplateVariant  string              `json:"templateVariant"`
	Agent            string              `json:"agent"`
	Attempt          int                 `json:"attempt"`
	RunManifestPath  string              `json:"runManifestPath"`
	TaskRunsPath     string              `json:"taskRunsSnapshotPath"`
	TaskRunEventsDir string              `json:"taskRunEventsDirPath"`
	RequestLogPath   string              `json:"requestLogPath,omitempty"`
	ManualScorePath  string              `json:"manualScorePath,omitempty"`
	RawStatus        string              `json:"rawStatus"`
	LatestTaskRunID  string              `json:"latestTaskRunId,omitempty"`
	LinkedTaskRunIDs []string            `json:"linkedTaskRunIds,omitempty"`
	Outcome          ScorecardOutcome    `json:"outcome"`
	Process          ScorecardProcess    `json:"process"`
	Efficiency       ScorecardEfficiency `json:"efficiency"`
	Manual           ScorecardManual     `json:"manual"`
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
	FailedTaskToolCalls       int            `json:"failedTaskToolCalls"`
	FailedDownstreamToolCalls int            `json:"failedDownstreamToolCalls"`
	TotalTaskToolCalls        int            `json:"totalTaskToolCalls"`
	TotalDownstreamToolCalls  int            `json:"totalDownstreamToolCalls"`
	RetriesByStep             map[string]int `json:"retriesByStep,omitempty"`
	TotalStepRetries          int            `json:"totalStepRetries"`
	ReplanningCount           int            `json:"replanningCount"`
	RecoveryTimeSeconds       *float64       `json:"recoveryTimeSeconds,omitempty"`
	RecoveryToolCalls         *int           `json:"recoveryToolCalls,omitempty"`
}

// ScorecardEfficiency contains efficiency metrics for one run.
type ScorecardEfficiency struct {
	WallClockSeconds     float64  `json:"wallClockSeconds"`
	TotalToolCalls       int      `json:"totalToolCalls"`
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
	CaseID                    string   `json:"caseId"`
	Agent                     string   `json:"agent"`
	TemplateVariant           string   `json:"templateVariant"`
	Attempt                   int      `json:"attempt"`
	RawStatus                 string   `json:"rawStatus"`
	Scored                    bool     `json:"scored"`
	CompletedSuccessfully     bool     `json:"completedSuccessfully"`
	FinalVerificationPassed   bool     `json:"finalVerificationPassed"`
	FirstPassSuccess          bool     `json:"firstPassSuccess"`
	InvariantViolation        bool     `json:"invariantViolation"`
	RestartOccurred           bool     `json:"restartOccurred"`
	FailOccurred              bool     `json:"failOccurred"`
	TimeoutOccurred           bool     `json:"timeoutOccurred"`
	WallClockSeconds          float64  `json:"wallClockSeconds"`
	TotalToolCalls            int      `json:"totalToolCalls"`
	FailedTaskToolCalls       int      `json:"failedTaskToolCalls"`
	FailedDownstreamToolCalls int      `json:"failedDownstreamToolCalls"`
	EditedFilesCount          int      `json:"editedFilesCount"`
	ErrorActionabilityScore   *int     `json:"errorActionabilityScore,omitempty"`
	Warnings                  []string `json:"warnings,omitempty"`
	Errors                    []string `json:"errors,omitempty"`
}

// AggregateSummary stores aggregate comparison metrics for one grouping key.
type AggregateSummary struct {
	Key                             string   `json:"key"`
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
	MedianFailedTaskToolCalls       float64  `json:"medianFailedTaskToolCalls"`
	MedianFailedDownstreamToolCalls float64  `json:"medianFailedDownstreamToolCalls"`
	MedianEditedFilesCount          float64  `json:"medianEditedFilesCount"`
	ManualActionabilityCount        int      `json:"manualActionabilityCount"`
	AverageManualActionabilityScore *float64 `json:"averageManualActionabilityScore,omitempty"`
}

type Scorer struct {
	Now func() time.Time
}

type scoreRunContext struct {
	sessionDir string
	entry      SessionRunManifestEntry
	runPath    string
	run        *RunManifest
	caseDef    *CaseDefinition
	caseRoot   string
}

type scoreFailure struct {
	row RunSummaryRow
	err error
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
	suite, err := LoadSuite(session.SuitePath)
	if err != nil {
		return nil, fmt.Errorf("load suite for scoring: %w", err)
	}
	caseDefs, err := loadCaseContexts(session.SuitePath, suite)
	if err != nil {
		return nil, err
	}

	summary := &SessionSummary{
		ScoreVersion: scoreVersion,
		SessionPath:  sessionDir,
		SuiteID:      session.SuiteID,
		GeneratedAt:  s.Now(),
		RunCount:     len(session.Runs),
		Runs:         make([]RunSummaryRow, 0, len(session.Runs)),
	}

	scoredRows := make([]RunSummaryRow, 0, len(session.Runs))
	scoreFailures := make([]scoreFailure, 0)
	for _, entry := range session.Runs {
		ctx, err := buildScoreRunContext(sessionDir, entry, caseDefs)
		if err != nil {
			scoreFailures = append(scoreFailures, scoreFailure{
				row: RunSummaryRow{
					CaseID:          entry.CaseID,
					Agent:           entry.AgentID,
					TemplateVariant: entry.TemplateVariant,
					Attempt:         entry.Attempt,
					RawStatus:       entry.Status,
					Scored:          false,
					Errors:          []string{err.Error()},
				},
				err: err,
			})
			continue
		}
		scorecard, err := s.scoreRun(ctx)
		if err != nil {
			scoreFailures = append(scoreFailures, scoreFailure{
				row: buildRunSummaryRow(ctx.entry, ctx.run, nil, []string{err.Error()}, nil),
				err: err,
			})
			continue
		}
		if err := writeJSONFile(filepath.Join(filepath.Dir(ctx.runPath), scorecardFileName), scorecard); err != nil {
			scoreFailures = append(scoreFailures, scoreFailure{
				row: buildRunSummaryRow(ctx.entry, ctx.run, scorecard, []string{err.Error()}, nil),
				err: err,
			})
			continue
		}
		row := buildRunSummaryRow(ctx.entry, ctx.run, scorecard, nil, scorecard.Warnings)
		summary.Runs = append(summary.Runs, row)
		scoredRows = append(scoredRows, row)
	}
	for _, failure := range scoreFailures {
		summary.Runs = append(summary.Runs, failure.row)
	}
	sort.Slice(summary.Runs, func(i, j int) bool {
		return compareRunRows(summary.Runs[i], summary.Runs[j])
	})
	summary.ScoredRunCount = len(scoredRows)
	summary.FailedToScoreCount = summary.RunCount - summary.ScoredRunCount
	summary.Aggregates = buildAggregates(scoredRows)

	if err := writeJSONFile(filepath.Join(sessionDir, summaryFileName), summary); err != nil {
		return nil, err
	}
	if len(scoreFailures) > 0 {
		return summary, fmt.Errorf("failed to score %d benchmark run(s)", len(scoreFailures))
	}
	return summary, nil
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

func buildScoreRunContext(sessionDir string, entry SessionRunManifestEntry, caseDefs map[string]scoreRunContext) (*scoreRunContext, error) {
	caseCtx, ok := caseDefs[entry.CaseID]
	if !ok {
		return nil, fmt.Errorf("missing case definition for %q", entry.CaseID)
	}
	runDir := filepath.Join(sessionDir, entry.RelativeRunDir)
	runPath := filepath.Join(runDir, runFileName)
	var run RunManifest
	if err := readJSONFile(runPath, &run); err != nil {
		return nil, fmt.Errorf("load run manifest for %s/%s/%s attempt %d: %w", entry.TemplateVariant, entry.AgentID, entry.CaseID, entry.Attempt, err)
	}
	return &scoreRunContext{
		sessionDir: sessionDir,
		entry:      entry,
		runPath:    runPath,
		run:        &run,
		caseDef:    caseCtx.caseDef,
		caseRoot:   caseCtx.caseRoot,
	}, nil
}

func (s *Scorer) scoreRun(ctx *scoreRunContext) (*RunScorecard, error) {
	taskRuns, err := loadTaskRunsSnapshot(ctx.run.ArtifactPaths.TaskRunsSnapshot)
	if err != nil {
		return nil, err
	}
	events, err := loadRunEvents(ctx.run, taskRuns)
	if err != nil {
		return nil, err
	}
	manualPath := filepath.Join(filepath.Dir(ctx.runPath), manualScoreFileName)
	manual, manualPathValue, err := loadManualScore(manualPath)
	if err != nil {
		return nil, err
	}

	latest := findLatestLinkedTaskRun(ctx.run, taskRuns)
	outcome, warnings, err := scoreOutcome(ctx, latest, events)
	if err != nil {
		return nil, err
	}
	process := scoreProcess(events, ctx.run.EndedAt)
	editedFiles, err := collectEditedFiles(filepath.Join(ctx.caseRoot, ctx.caseDef.Fixture.SeedPath), ctx.run.ArtifactPaths.ProjectDir)
	if err != nil {
		return nil, err
	}
	efficiency := ScorecardEfficiency{
		WallClockSeconds:     secondsBetween(ctx.run.StartedAt, ctx.run.EndedAt),
		TotalToolCalls:       process.TotalTaskToolCalls + process.TotalDownstreamToolCalls,
		EditedFilesCount:     len(editedFiles),
		EditedFiles:          editedFiles,
		ObservedCommandCalls: countObservedCommandCalls(events),
	}
	scorecard := &RunScorecard{
		SuiteID:          ctx.run.SuiteID,
		CaseID:           ctx.run.CaseID,
		TemplateID:       ctx.run.TemplateID,
		TemplateVariant:  ctx.run.TemplateVariant.Name,
		Agent:            ctx.run.AgentID,
		Attempt:          ctx.entry.Attempt,
		RunManifestPath:  ctx.runPath,
		TaskRunsPath:     ctx.run.ArtifactPaths.TaskRunsSnapshot,
		TaskRunEventsDir: ctx.run.ArtifactPaths.TaskRunEventsDir,
		RequestLogPath:   ctx.run.ArtifactPaths.RequestLogPath,
		ManualScorePath:  manualPathValue,
		RawStatus:        ctx.run.Status,
		LatestTaskRunID:  ctx.run.LatestTaskRunID,
		LinkedTaskRunIDs: append([]string(nil), ctx.run.LinkedTaskRunIDs...),
		Outcome:          outcome,
		Process:          process,
		Efficiency:       efficiency,
		Manual: ScorecardManual{
			ErrorActionabilityScore: manual.ErrorActionabilityScore,
			ErrorActionabilityNotes: manual.Notes,
		},
		ScoreVersion: scoreVersion,
		GeneratedAt:  s.Now(),
		Warnings:     warnings,
	}
	return scorecard, nil
}

func scoreOutcome(ctx *scoreRunContext, latest *persistence.TaskRunSummary, events []persistence.TaskRunEvent) (ScorecardOutcome, []string, error) {
	warnings := make([]string, 0)
	if latest == nil {
		warnings = append(warnings, "no linked task runs were available while scoring")
	}
	restartOccurred := hasTaskEvent(events, "task_restarted")
	failOccurred := hasTaskEvent(events, "task_failed")
	timeoutOccurred := hasTaskEvent(events, "task_timed_out")
	finalVerificationPassed := latest != nil && latest.Status == "completed"
	completedSuccessfully := ctx.run.Status == "completed" && finalVerificationPassed
	invariantViolation, err := detectInvariantViolation(filepath.Join(ctx.caseRoot, ctx.caseDef.Fixture.SeedPath), ctx.run.ArtifactPaths.ProjectDir, ctx.caseDef.Constraints.LockedPaths)
	if err != nil {
		return ScorecardOutcome{}, nil, err
	}
	return ScorecardOutcome{
		CompletedSuccessfully:   completedSuccessfully,
		FinalVerificationPassed: finalVerificationPassed,
		FirstPassSuccess:        finalVerificationPassed && !restartOccurred && !failOccurred && !timeoutOccurred,
		RestartOccurred:         restartOccurred,
		FailOccurred:            failOccurred,
		TimeoutOccurred:         timeoutOccurred,
		InvariantViolation:      invariantViolation,
	}, warnings, nil
}

func scoreProcess(events []persistence.TaskRunEvent, runEndedAt time.Time) ScorecardProcess {
	process := ScorecardProcess{
		RetriesByStep: map[string]int{},
	}
	actionEvents := collectActionEvents(events)
	for _, event := range actionEvents {
		toolName := toolNameForEvent(event)
		if isTaskTool(toolName) {
			process.TotalTaskToolCalls++
			if event.IsError != nil && *event.IsError {
				process.FailedTaskToolCalls++
			}
		} else {
			process.TotalDownstreamToolCalls++
			if event.IsError != nil && *event.IsError {
				process.FailedDownstreamToolCalls++
			}
		}
	}
	stepStarts := map[string]int{}
	planningCompletedCount := 0
	for _, event := range events {
		if event.Source != persistence.TaskRunEventSourceTask {
			continue
		}
		switch event.EventType {
		case "step_started":
			stepID := stepKeyForEvent(event)
			stepStarts[stepID]++
		case "planning_completed":
			planningCompletedCount++
		}
	}
	for stepID, count := range stepStarts {
		retries := count - 1
		if retries < 0 {
			retries = 0
		}
		process.RetriesByStep[stepID] = retries
		process.TotalStepRetries += retries
	}
	if len(process.RetriesByStep) == 0 {
		process.RetriesByStep = nil
	}
	if planningCompletedCount > 1 {
		process.ReplanningCount = planningCompletedCount - 1
	}

	recoveryTime, recoveryCalls := computeRecoveryMetrics(events, runEndedAt)
	process.RecoveryTimeSeconds = recoveryTime
	process.RecoveryToolCalls = recoveryCalls
	return process
}

func loadTaskRunsSnapshot(path string) ([]persistence.TaskRunSummary, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("run manifest is missing task runs snapshot path")
	}
	var runs []persistence.TaskRunSummary
	if err := readJSONFile(path, &runs); err != nil {
		return nil, fmt.Errorf("load task runs snapshot: %w", err)
	}
	return runs, nil
}

func loadRunEvents(run *RunManifest, taskRuns []persistence.TaskRunSummary) ([]persistence.TaskRunEvent, error) {
	if strings.TrimSpace(run.ArtifactPaths.TaskRunEventsDir) == "" {
		return nil, fmt.Errorf("run manifest is missing task run events dir path")
	}
	runIDs := append([]string(nil), run.LinkedTaskRunIDs...)
	if len(runIDs) == 0 {
		runIDs = taskRunIDs(taskRuns)
	}
	all := make([]persistence.TaskRunEvent, 0)
	for _, runID := range runIDs {
		if strings.TrimSpace(runID) == "" {
			continue
		}
		var events []persistence.TaskRunEvent
		if err := readJSONFile(filepath.Join(run.ArtifactPaths.TaskRunEventsDir, runID+".json"), &events); err != nil {
			return nil, fmt.Errorf("load task run events for %s: %w", runID, err)
		}
		all = append(all, events...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAtUnixMilli == all[j].CreatedAtUnixMilli {
			return all[i].ID < all[j].ID
		}
		return all[i].CreatedAtUnixMilli < all[j].CreatedAtUnixMilli
	})
	return all, nil
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

func findLatestLinkedTaskRun(run *RunManifest, runs []persistence.TaskRunSummary) *persistence.TaskRunSummary {
	if run == nil {
		return nil
	}
	if run.LatestTaskRunID != "" {
		for idx := range runs {
			if runs[idx].RunID == run.LatestTaskRunID {
				return &runs[idx]
			}
		}
	}
	return latestTaskRun(runs)
}

func hasTaskEvent(events []persistence.TaskRunEvent, eventType string) bool {
	for _, event := range events {
		if event.Source == persistence.TaskRunEventSourceTask && event.EventType == eventType {
			return true
		}
	}
	return false
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

func collectActionEvents(events []persistence.TaskRunEvent) []persistence.TaskRunEvent {
	result := make([]persistence.TaskRunEvent, 0, len(events))
	for _, event := range events {
		if event.Source == persistence.TaskRunEventSourceAction {
			result = append(result, event)
		}
	}
	return result
}

func stepKeyForEvent(event persistence.TaskRunEvent) string {
	payload := map[string]any{}
	if len(event.PayloadJSON) > 0 && json.Unmarshal(event.PayloadJSON, &payload) == nil {
		if stepID, ok := payload["stepId"].(string); ok && strings.TrimSpace(stepID) != "" {
			return stepID
		}
		if step, ok := payload["step"].(string); ok && strings.TrimSpace(step) != "" {
			return step
		}
		if step, ok := payload["step"].(float64); ok {
			return fmt.Sprintf("%d", int(step))
		}
	}
	if strings.TrimSpace(event.ResultingPhasePath) != "" {
		return event.ResultingPhasePath
	}
	return event.ID
}

func computeRecoveryMetrics(events []persistence.TaskRunEvent, runEndedAt time.Time) (*float64, *int) {
	var failedAction *persistence.TaskRunEvent
	for idx := range events {
		event := events[idx]
		if event.Source != persistence.TaskRunEventSourceAction || event.IsError == nil || !*event.IsError {
			continue
		}
		failedAction = &event
		break
	}
	if failedAction == nil {
		return nil, nil
	}
	recoveryAt := runEndedAt.UTC().UnixMilli()
	foundRecovery := false
	for _, event := range events {
		if event.CreatedAtUnixMilli <= failedAction.CreatedAtUnixMilli || event.Source != persistence.TaskRunEventSourceTask {
			continue
		}
		if isRecoveryTaskEvent(event.EventType) {
			recoveryAt = event.CreatedAtUnixMilli
			foundRecovery = true
			break
		}
	}
	recoveryToolCalls := 0
	for _, event := range events {
		if event.Source != persistence.TaskRunEventSourceAction {
			continue
		}
		if event.CreatedAtUnixMilli > failedAction.CreatedAtUnixMilli && event.CreatedAtUnixMilli <= recoveryAt {
			recoveryToolCalls++
		}
	}
	seconds := float64(recoveryAt-failedAction.CreatedAtUnixMilli) / 1000.0
	if seconds < 0 {
		seconds = 0
	}
	if !foundRecovery && runEndedAt.IsZero() {
		seconds = 0
	}
	return &seconds, &recoveryToolCalls
}

func isRecoveryTaskEvent(eventType string) bool {
	switch eventType {
	case "planning_completed", "step_completed", "task_resumed":
		return true
	default:
		return false
	}
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

func countObservedCommandCalls(events []persistence.TaskRunEvent) int {
	count := 0
	for _, event := range events {
		if event.Source != persistence.TaskRunEventSourceAction {
			continue
		}
		if _, ok := commandToolNames[toolNameForEvent(event)]; ok {
			count++
		}
	}
	return count
}

func toolNameForEvent(event persistence.TaskRunEvent) string {
	if strings.TrimSpace(event.ToolName) != "" {
		return event.ToolName
	}
	return event.OriginalToolName
}

func isTaskTool(toolName string) bool {
	return strings.HasPrefix(toolName, "centian.task_")
}

func secondsBetween(start time.Time, end time.Time) float64 {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start).Seconds()
}

func buildRunSummaryRow(entry SessionRunManifestEntry, run *RunManifest, scorecard *RunScorecard, errors []string, warnings []string) RunSummaryRow {
	row := RunSummaryRow{
		CaseID:          entry.CaseID,
		Agent:           entry.AgentID,
		TemplateVariant: entry.TemplateVariant,
		Attempt:         entry.Attempt,
		Errors:          append([]string(nil), errors...),
		Warnings:        append([]string(nil), warnings...),
	}
	if run != nil {
		row.RawStatus = run.Status
	}
	if scorecard == nil {
		return row
	}
	row.Scored = true
	row.RawStatus = scorecard.RawStatus
	row.CompletedSuccessfully = scorecard.Outcome.CompletedSuccessfully
	row.FinalVerificationPassed = scorecard.Outcome.FinalVerificationPassed
	row.FirstPassSuccess = scorecard.Outcome.FirstPassSuccess
	row.InvariantViolation = scorecard.Outcome.InvariantViolation
	row.RestartOccurred = scorecard.Outcome.RestartOccurred
	row.FailOccurred = scorecard.Outcome.FailOccurred
	row.TimeoutOccurred = scorecard.Outcome.TimeoutOccurred
	row.WallClockSeconds = scorecard.Efficiency.WallClockSeconds
	row.TotalToolCalls = scorecard.Efficiency.TotalToolCalls
	row.FailedTaskToolCalls = scorecard.Process.FailedTaskToolCalls
	row.FailedDownstreamToolCalls = scorecard.Process.FailedDownstreamToolCalls
	row.EditedFilesCount = scorecard.Efficiency.EditedFilesCount
	row.ErrorActionabilityScore = scorecard.Manual.ErrorActionabilityScore
	row.Warnings = append(row.Warnings, scorecard.Warnings...)
	row.Errors = append(row.Errors, scorecard.Errors...)
	return row
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

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
