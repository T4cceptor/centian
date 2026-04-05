package benchmarks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/persistence"
)

func persistBenchmarkArtifact(ctx context.Context, storePath string, record *persistence.BenchmarkArtifactRecord) error {
	if strings.TrimSpace(storePath) == "" {
		return fmt.Errorf("benchmark artifact store path is required")
	}
	store, err := persistence.NewSQLiteStore(storePath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return store.UpsertBenchmarkArtifact(ctx, record)
}

func buildSessionArtifactRecord(session *SessionManifest) (*persistence.BenchmarkArtifactRecord, error) {
	if session == nil {
		return nil, fmt.Errorf("session manifest is required")
	}
	sessionPath := filepath.Clean(strings.TrimSpace(session.InvocationDir))
	sessionID := benchmarkSessionID(session.SuiteID, sessionPath)
	payload, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("marshal session manifest: %w", err)
	}
	return &persistence.BenchmarkArtifactRecord{
		ID:                 benchmarkArtifactID(string(persistence.BenchmarkArtifactKindSession), session.SuiteID, sessionID, sessionPath, ""),
		ArtifactKind:       persistence.BenchmarkArtifactKindSession,
		SuiteID:            session.SuiteID,
		SessionID:          sessionID,
		SessionPath:        sessionPath,
		CreatedAtUnixMilli: bestTime(session.EndedAt, session.StartedAt).UnixMilli(),
		PayloadJSON:        payload,
	}, nil
}

func buildRunArtifactRecord(run *RunManifest) (*persistence.BenchmarkArtifactRecord, error) {
	if run == nil {
		return nil, fmt.Errorf("run manifest is required")
	}
	sessionPath := sessionPathFromRun(run)
	runPath := filepath.Clean(strings.TrimSpace(run.ArtifactPaths.RunDir))
	payload, err := json.Marshal(run)
	if err != nil {
		return nil, fmt.Errorf("marshal run manifest: %w", err)
	}
	attempt := run.Attempt
	return &persistence.BenchmarkArtifactRecord{
		ID:                 benchmarkArtifactID(string(persistence.BenchmarkArtifactKindRun), run.SuiteID, benchmarkSessionID(run.SuiteID, sessionPath), sessionPath, runPath),
		ArtifactKind:       persistence.BenchmarkArtifactKindRun,
		SuiteID:            run.SuiteID,
		SessionID:          benchmarkSessionID(run.SuiteID, sessionPath),
		SessionPath:        sessionPath,
		RunPath:            runPath,
		CaseID:             run.CaseID,
		Agent:              run.AgentID,
		TemplateVariant:    run.TemplateVariant.Name,
		Attempt:            &attempt,
		CreatedAtUnixMilli: bestTime(run.EndedAt, run.StartedAt).UnixMilli(),
		PayloadJSON:        payload,
	}, nil
}

func benchmarkSessionID(suiteID, sessionPath string) string {
	return benchmarkArtifactID("session-scope", suiteID, filepath.Clean(sessionPath))
}

func benchmarkArtifactID(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return "ba_" + hex.EncodeToString(hash[:16])
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
