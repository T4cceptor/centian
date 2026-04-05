package benchmarks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/persistence"
)

func persistBenchmarkSession(ctx context.Context, storePath string, record *persistence.BenchmarkSessionRecord) error {
	store, err := openBenchmarkStore(storePath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return store.UpsertBenchmarkSession(ctx, record)
}

func persistBenchmarkRun(ctx context.Context, storePath string, record *persistence.BenchmarkRunRecord) error {
	store, err := openBenchmarkStore(storePath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return store.UpsertBenchmarkRun(ctx, record)
}

func openBenchmarkStore(storePath string) (*persistence.Store, error) {
	if strings.TrimSpace(storePath) == "" {
		return nil, fmt.Errorf("benchmark store path is required")
	}
	return persistence.NewSQLiteStore(storePath)
}

func buildSessionRecord(session *SessionManifest) (*persistence.BenchmarkSessionRecord, error) {
	if session == nil {
		return nil, fmt.Errorf("session manifest is required")
	}
	sessionPath := filepath.Clean(strings.TrimSpace(session.InvocationDir))
	return &persistence.BenchmarkSessionRecord{
		SessionID:          benchmarkSessionID(session.SuiteID, sessionPath),
		SuiteID:            session.SuiteID,
		SuitePath:          session.SuitePath,
		SessionPath:        sessionPath,
		OutputRoot:         session.OutputRoot,
		TemplateID:         session.TemplateID,
		StartedAtUnixMilli: bestTime(session.StartedAt, session.EndedAt).UnixMilli(),
		EndedAtUnixMilli:   timePointerMillis(session.EndedAt),
		Status:             session.Status,
		RepeatCount:        session.Repeat,
	}, nil
}

func buildRunRecord(run *RunManifest) (*persistence.BenchmarkRunRecord, error) {
	if run == nil {
		return nil, fmt.Errorf("run manifest is required")
	}
	sessionPath := sessionPathFromRun(run)
	return &persistence.BenchmarkRunRecord{
		BenchmarkRunID:       benchmarkRunID(run, sessionPath),
		SessionID:            benchmarkSessionID(run.SuiteID, sessionPath),
		CaseID:               run.CaseID,
		Agent:                run.AgentID,
		TemplateVariant:      run.TemplateVariant.Name,
		Attempt:              run.Attempt,
		TemplateID:           run.TemplateID,
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
	}, nil
}

func benchmarkSessionID(suiteID, sessionPath string) string {
	return benchmarkStableID("session", suiteID, filepath.Clean(sessionPath))
}

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

func benchmarkStableID(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return "bm_" + hex.EncodeToString(hash[:16])
}

func sessionPathFromRun(run *RunManifest) string {
	runDir := filepath.Clean(strings.TrimSpace(run.ArtifactPaths.RunDir))
	if runDir == "" {
		return ""
	}
	sessionPath := runDir
	for i := 0; i < 5; i++ {
		sessionPath = filepath.Dir(sessionPath)
	}
	return sessionPath
}

func bestTime(primary, fallback time.Time) time.Time {
	if !primary.IsZero() {
		return primary
	}
	return fallback
}

func timePointerMillis(value time.Time) *int64 {
	if value.IsZero() {
		return nil
	}
	millis := value.UnixMilli()
	return &millis
}
