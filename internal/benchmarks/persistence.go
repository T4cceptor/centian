package benchmarks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/persistence"
)

const (
	benchmarkRunScoreStatusReady    = "ready"
	benchmarkRunScoreStatusUnscored = "unscored"
)

// persistBenchmarkSession upserts one benchmark session record into the event store.
func persistBenchmarkSession(ctx context.Context, storePath string, record *persistence.BenchmarkSessionRecord) error {
	store, err := openBenchmarkStore(storePath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return store.UpsertBenchmarkSession(ctx, record)
}

// persistBenchmarkRun upserts one benchmark run record into the event store.
func persistBenchmarkRun(ctx context.Context, storePath string, record *persistence.BenchmarkRunRecord) error {
	store, err := openBenchmarkStore(storePath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return store.UpsertBenchmarkRun(ctx, record)
}

// persistBenchmarkRunScore upserts one persisted score snapshot into the event store.
func persistBenchmarkRunScore(ctx context.Context, storePath string, record *persistence.BenchmarkRunScoreRecord) error {
	store, err := openBenchmarkStore(storePath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return store.UpsertBenchmarkRunScore(ctx, record)
}

// openBenchmarkStore validates storePath and opens the shared benchmark SQLite store.
func openBenchmarkStore(storePath string) (*persistence.Store, error) {
	if strings.TrimSpace(storePath) == "" {
		return nil, fmt.Errorf("benchmark store path is required")
	}
	return persistence.NewSQLiteStore(storePath)
}

// buildSessionRecord converts an in-memory session manifest into its persisted row shape.
func buildSessionRecord(session *SessionManifest) (*persistence.BenchmarkSessionRecord, error) {
	if session == nil {
		return nil, fmt.Errorf("session manifest is required")
	}
	sessionPath := filepath.Clean(strings.TrimSpace(session.InvocationDir))
	return &persistence.BenchmarkSessionRecord{
		SessionID:          benchmarkSessionID(session.SuiteID, sessionPath),
		SuiteID:            session.SuiteID,
		SuiteName:          session.SuiteName,
		SuitePath:          session.SuitePath,
		SessionPath:        sessionPath,
		OutputRoot:         session.OutputRoot,
		TemplateID:         session.TemplateID,
		TemplateName:       session.TemplateName,
		StartedAtUnixMilli: bestTime(session.StartedAt, session.EndedAt).UnixMilli(),
		EndedAtUnixMilli:   timePointerMillis(session.EndedAt),
		Status:             session.Status,
		RepeatCount:        session.Repeat,
	}, nil
}

// buildRunRecord converts a run manifest into the persisted benchmark run row.
func buildRunRecord(run *RunManifest) (*persistence.BenchmarkRunRecord, error) {
	if run == nil {
		return nil, fmt.Errorf("run manifest is required")
	}
	sessionPath := sessionPathFromRun(run)
	record := &persistence.BenchmarkRunRecord{
		BenchmarkRunID:       benchmarkRunID(run, sessionPath),
		SessionID:            benchmarkSessionID(run.SuiteID, sessionPath),
		CaseID:               run.CaseID,
		CaseName:             run.CaseName,
		Agent:                run.AgentID,
		TemplateVariant:      run.TemplateVariant.Name,
		Attempt:              run.Attempt,
		TemplateID:           run.TemplateID,
		TemplateName:         run.TemplateName,
		SelectedModel:        run.SelectedModel,
		StartedAtUnixMilli:   bestTime(run.StartedAt, run.EndedAt).UnixMilli(),
		EndedAtUnixMilli:     timePointerMillis(run.EndedAt),
		Status:               run.Status,
		LatestTaskRunID:      run.LatestTaskRunID,
		LatestTaskRunStatus:  run.LatestTaskRunStatus,
		LinkedTaskRunIDs:     append([]string(nil), run.LinkedTaskRunIDs...),
		RunDir:               run.ArtifactPaths.RunDir,
		ProjectDir:           run.ArtifactPaths.ProjectDir,
		LogsDir:              run.ArtifactPaths.LogsDir,
		AgentDir:             run.ArtifactPaths.AgentDir,
		ConfigPath:           run.ArtifactPaths.ConfigPath,
		EventStoreMode:       run.ArtifactPaths.EventStoreMode,
		EventStorePath:       run.ArtifactPaths.EventStorePath,
		RequestLogPath:       run.ArtifactPaths.RequestLogPath,
		SelectedTemplatePath: run.ArtifactPaths.SelectedTemplatePath,
		ErrorSummary:         run.ErrorSummary,
	}
	agentMetadataJSON, err := buildAgentMetadataJSON(run)
	if err != nil {
		return nil, err
	}
	record.AgentMetadataJSON = agentMetadataJSON
	return record, nil
}

// buildRunScoreRecord converts one scored run into the persisted score snapshot row.
func buildRunScoreRecord(
	now time.Time,
	run *persistence.BenchmarkRunRecord,
	scorecard *RunScorecard,
	scoreErrors []string,
) (*persistence.BenchmarkRunScoreRecord, error) {
	if run == nil {
		return nil, fmt.Errorf("benchmark run record is required")
	}
	record := &persistence.BenchmarkRunScoreRecord{
		BenchmarkRunID:       run.BenchmarkRunID,
		ScoreStatus:          benchmarkRunScoreStatusUnscored,
		ScoreVersion:         scoreVersion,
		GeneratedAtUnixMilli: now.UnixMilli(),
		ScoreErrors:          append([]string(nil), scoreErrors...),
		SelectedModel:        run.SelectedModel,
	}
	if scorecard == nil {
		if len(record.ScoreErrors) == 0 {
			record.ScoreErrors = []string{"benchmark score unavailable"}
		}
		return record, nil
	}

	payload, err := json.Marshal(scorecard)
	if err != nil {
		return nil, fmt.Errorf("marshal benchmark run scorecard: %w", err)
	}
	record.ScoreStatus = benchmarkRunScoreStatusReady
	record.ScoreVersion = firstNonEmpty(strings.TrimSpace(scorecard.ScoreVersion), scoreVersion)
	record.ScorecardJSON = payload
	record.ScoreErrors = append(record.ScoreErrors, scorecard.Errors...)
	record.SelectedModel = firstNonEmpty(scorecard.SelectedModel, run.SelectedModel)
	record.CompletedSuccessfully = scorecard.Outcome.CompletedSuccessfully
	record.FinalVerificationPassed = scorecard.Outcome.FinalVerificationPassed
	record.FirstPassSuccess = scorecard.Outcome.FirstPassSuccess
	record.RestartOccurred = scorecard.Outcome.RestartOccurred
	record.FailOccurred = scorecard.Outcome.FailOccurred
	record.TimeoutOccurred = scorecard.Outcome.TimeoutOccurred
	record.InvariantViolation = scorecard.Outcome.InvariantViolation
	record.WallClockSeconds = scorecard.Efficiency.WallClockSeconds
	record.TotalToolCalls = scorecard.Efficiency.TotalToolCalls
	record.TotalTaskToolCalls = scorecard.Process.TotalTaskToolCalls
	record.TotalDownstreamToolCalls = scorecard.Process.TotalDownstreamToolCalls
	record.FailedTaskToolCalls = scorecard.Process.FailedTaskToolCalls
	record.FailedDownstreamToolCalls = scorecard.Process.FailedDownstreamToolCalls
	record.InputTokens = scorecard.Efficiency.InputTokens
	record.OutputTokens = scorecard.Efficiency.OutputTokens
	record.EditedFilesCount = scorecard.Efficiency.EditedFilesCount
	record.ErrorActionabilityScore = scorecard.Manual.ErrorActionabilityScore
	return record, nil
}

// buildPersistedRunScoreRecord scores a persisted run against task-run data and snapshots the result.
func buildPersistedRunScoreRecord(
	ctx context.Context,
	storePath string,
	session *persistence.BenchmarkSessionRecord,
	run *persistence.BenchmarkRunRecord,
	now func() time.Time,
) (*persistence.BenchmarkRunScoreRecord, error) {
	if session == nil {
		return nil, fmt.Errorf("benchmark session record is required")
	}
	if run == nil {
		return nil, fmt.Errorf("benchmark run record is required")
	}
	store, err := openBenchmarkStore(storePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = store.Close() }()

	query := NewQueryService(store).withDefaults()
	if now != nil {
		query.now = now
	}
	scorecard, scoreErr := query.scoreRunRecord(ctx, session, run)
	if scoreErr == nil {
		return buildRunScoreRecord(query.now(), run, scorecard, nil)
	}
	scoreErrors := []string{scoreErr.Error()}
	if errors.Is(scoreErr, context.Canceled) || errors.Is(scoreErr, context.DeadlineExceeded) {
		scoreErrors = append(scoreErrors, "benchmark score generation interrupted")
	}
	return buildRunScoreRecord(query.now(), run, nil, scoreErrors)
}

// buildAgentMetadataJSON parses agent stdout and stores the normalized metadata JSON blob.
func buildAgentMetadataJSON(run *RunManifest) (json.RawMessage, error) {
	agentID := strings.TrimSpace(run.AgentID)
	agentDir := strings.TrimSpace(run.ArtifactPaths.AgentDir)
	if agentID == "" || agentDir == "" {
		return nil, nil
	}
	metadata, _, err := loadAgentMetadata(filepath.Join(agentDir, "agent.stdout.log"), agentID)
	if err != nil {
		return nil, fmt.Errorf("load agent metadata: %w", err)
	}
	metadata = enrichAgentMetadata(metadata, run.SelectedModel)
	if metadata == nil {
		return nil, nil
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal agent metadata: %w", err)
	}
	return payload, nil
}

// benchmarkSessionID builds a stable session identifier from suite and path identity.
func benchmarkSessionID(suiteID, sessionPath string) string {
	return benchmarkStableID("session", suiteID, filepath.Clean(sessionPath))
}

// benchmarkRunID builds a stable run identifier from suite, matrix cell, and run directory.
func benchmarkRunID(run *RunManifest, sessionPath string) string {
	return benchmarkStableID(
		"run",
		run.SuiteID,
		benchmarkSessionID(run.SuiteID, sessionPath),
		run.CaseID,
		run.AgentID,
		run.TemplateVariant.Name,
		fmt.Sprintf("%03d", run.Attempt),
		filepath.Clean(run.ArtifactPaths.RunDir),
	)
}

// benchmarkStableID hashes a logical identity tuple into a short persisted benchmark ID.
func benchmarkStableID(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return "bm_" + hex.EncodeToString(hash[:16])
}

// sessionPathFromRun walks up from a run directory to its enclosing session directory.
func sessionPathFromRun(run *RunManifest) string {
	runDir := filepath.Clean(strings.TrimSpace(run.ArtifactPaths.RunDir))
	if runDir == "" {
		return ""
	}
	current := runDir
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		if filepath.Base(parent) == "runs" {
			return filepath.Dir(parent)
		}
		current = parent
	}
}

// bestTime prefers primary when set and otherwise falls back to the alternate timestamp.
func bestTime(primary, fallback time.Time) time.Time {
	if !primary.IsZero() {
		return primary
	}
	return fallback
}

// timePointerMillis converts a non-zero timestamp into a nullable unix-millis pointer.
func timePointerMillis(value time.Time) *int64 {
	if value.IsZero() {
		return nil
	}
	millis := value.UnixMilli()
	return &millis
}
