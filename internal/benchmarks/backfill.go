package benchmarks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/persistence"
)

const defaultBackfillFailureLimit = 10

// PathRemap rewrites one old artifact path prefix to a new local prefix.
type PathRemap struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// BackfillOptions configures one benchmark score recovery run.
type BackfillOptions struct {
	MainStorePath    string      `json:"mainStorePath"`
	SuiteID          string      `json:"suiteId,omitempty"`
	SessionID        string      `json:"sessionId,omitempty"`
	Agent            string      `json:"agent,omitempty"`
	CaseID           string      `json:"caseId,omitempty"`
	TemplateVariant  string      `json:"templateVariant,omitempty"`
	PathRemaps       []PathRemap `json:"pathRemaps,omitempty"`
	Force            bool        `json:"force,omitempty"`
	MaxFailureReport int         `json:"maxFailureReport,omitempty"`
}

// BackfillFailure stores one per-run recovery failure summary.
type BackfillFailure struct {
	BenchmarkRunID string `json:"benchmarkRunId"`
	SessionID      string `json:"sessionId,omitempty"`
	RunDir         string `json:"runDir,omitempty"`
	Error          string `json:"error"`
}

// BackfillResult summarizes one one-off benchmark score recovery execution.
type BackfillResult struct {
	MainStorePath    string            `json:"mainStorePath"`
	TotalCandidates  int               `json:"totalCandidates"`
	ScoredCount      int               `json:"scoredCount"`
	UnscoredCount    int               `json:"unscoredCount"`
	SkippedCount     int               `json:"skippedCount"`
	OverwrittenCount int               `json:"overwrittenCount"`
	PathRemaps       []PathRemap       `json:"pathRemaps,omitempty"`
	Failures         []BackfillFailure `json:"failures,omitempty"`
}

// BackfillService reconstructs persisted benchmark score snapshots for legacy rows.
type BackfillService struct {
	Now       func() time.Time
	OpenStore func(string) (*persistence.Store, error)
}

// NewBackfillService returns a backfill service with default local behavior.
func NewBackfillService() *BackfillService {
	return &BackfillService{
		Now:       timeNowUTC,
		OpenStore: persistence.NewSQLiteStore,
	}
}

func (s *BackfillService) withDefaults() *BackfillService {
	if s == nil {
		return NewBackfillService()
	}
	if s.Now == nil {
		s.Now = timeNowUTC
	}
	if s.OpenStore == nil {
		s.OpenStore = persistence.NewSQLiteStore
	}
	return s
}

// BackfillScores rescans selected benchmark runs and persists score snapshots into the main DB.
func (s *BackfillService) BackfillScores(ctx context.Context, opts *BackfillOptions) (*BackfillResult, error) {
	s = s.withDefaults()
	if opts == nil {
		return nil, fmt.Errorf("backfill options are required")
	}
	storePath := strings.TrimSpace(opts.MainStorePath)
	if storePath == "" {
		return nil, fmt.Errorf("main store path is required")
	}
	mainStore, err := s.OpenStore(storePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = mainStore.Close() }()

	failureLimit := opts.MaxFailureReport
	if failureLimit <= 0 {
		failureLimit = defaultBackfillFailureLimit
	}
	pathRemaps := normalizePathRemaps(opts.PathRemaps)
	result := &BackfillResult{
		MainStorePath: storePath,
		PathRemaps:    append([]PathRemap(nil), pathRemaps...),
	}

	sessions, err := mainStore.ListBenchmarkSessions(ctx, persistence.BenchmarkSessionFilter{
		SuiteID:   strings.TrimSpace(opts.SuiteID),
		SessionID: strings.TrimSpace(opts.SessionID),
	})
	if err != nil {
		return nil, err
	}
	sessionByID := make(map[string]*persistence.BenchmarkSessionRecord, len(sessions))
	for idx := range sessions {
		session := sessions[idx]
		sessionByID[session.SessionID] = &session
	}

	runs, err := mainStore.ListBenchmarkRuns(ctx, &persistence.BenchmarkRunFilter{
		SuiteID:         strings.TrimSpace(opts.SuiteID),
		SessionID:       strings.TrimSpace(opts.SessionID),
		CaseID:          strings.TrimSpace(opts.CaseID),
		Agent:           strings.TrimSpace(opts.Agent),
		TemplateVariant: strings.TrimSpace(opts.TemplateVariant),
	})
	if err != nil {
		return nil, err
	}
	scoreRows, err := mainStore.ListBenchmarkRunScores(ctx)
	if err != nil {
		return nil, err
	}
	existingScores := make(map[string]*persistence.BenchmarkRunScoreRecord, len(scoreRows))
	for idx := range scoreRows {
		score := scoreRows[idx]
		existingScores[score.BenchmarkRunID] = &score
	}

	for idx := range runs {
		run := runs[idx]
		result.TotalCandidates++
		if !opts.Force {
			if score, exists := existingScores[run.BenchmarkRunID]; exists && !isRetryableUnscoredScore(score) {
				result.SkippedCount++
				continue
			}
		}

		session := sessionByID[run.SessionID]
		scoreRecord := s.backfillRunScore(ctx, storePath, session, &run, pathRemaps)
		if _, existed := existingScores[run.BenchmarkRunID]; existed {
			result.OverwrittenCount++
		}
		if err := mainStore.UpsertBenchmarkRunScore(ctx, scoreRecord); err != nil {
			return nil, err
		}
		if scoreRecord.ScoreStatus == benchmarkRunScoreStatusReady {
			result.ScoredCount++
			continue
		}
		result.UnscoredCount++
		if len(result.Failures) < failureLimit {
			result.Failures = append(result.Failures, BackfillFailure{
				BenchmarkRunID: run.BenchmarkRunID,
				SessionID:      run.SessionID,
				RunDir:         run.RunDir,
				Error:          strings.Join(scoreRecord.ScoreErrors, "; "),
			})
		}
	}

	return result, nil
}

func isRetryableUnscoredScore(score *persistence.BenchmarkRunScoreRecord) bool {
	if score == nil {
		return false
	}
	return strings.TrimSpace(score.ScoreStatus) == benchmarkRunScoreStatusUnscored
}

func (s *BackfillService) backfillRunScore(
	ctx context.Context,
	mainStorePath string,
	session *persistence.BenchmarkSessionRecord,
	run *persistence.BenchmarkRunRecord,
	pathRemaps []PathRemap,
) *persistence.BenchmarkRunScoreRecord {
	if run == nil {
		record, _ := buildRunScoreRecord(s.Now(), nil, nil, []string{"benchmark run record is required"})
		return record
	}
	if session == nil {
		record, _ := buildRunScoreRecord(s.Now(), run, nil, []string{"benchmark session record was not found"})
		return record
	}

	remappedSession := remapBenchmarkSessionRecord(session, pathRemaps)
	remappedRun := remapBenchmarkRunRecord(run, pathRemaps)
	scoreStorePath := firstNonEmpty(strings.TrimSpace(remappedRun.EventStorePath), mainStorePath)

	scoreRecord, err := buildPersistedRunScoreRecord(ctx, scoreStorePath, remappedSession, remappedRun, s.Now)
	if err == nil {
		return scoreRecord
	}
	unscored, unscoredErr := buildRunScoreRecord(s.Now(), run, nil, []string{err.Error()})
	if unscoredErr != nil {
		return &persistence.BenchmarkRunScoreRecord{
			BenchmarkRunID:       run.BenchmarkRunID,
			ScoreStatus:          benchmarkRunScoreStatusUnscored,
			ScoreVersion:         scoreVersion,
			GeneratedAtUnixMilli: s.Now().UnixMilli(),
			ScoreErrors:          []string{err.Error(), unscoredErr.Error()},
			SelectedModel:        run.SelectedModel,
		}
	}
	return unscored
}

func remapBenchmarkSessionRecord(record *persistence.BenchmarkSessionRecord, remaps []PathRemap) *persistence.BenchmarkSessionRecord {
	if record == nil {
		return nil
	}
	cloned := *record
	cloned.SuitePath = applyPathRemaps(record.SuitePath, remaps)
	cloned.SessionPath = applyPathRemaps(record.SessionPath, remaps)
	cloned.OutputRoot = applyPathRemaps(record.OutputRoot, remaps)
	return &cloned
}

func remapBenchmarkRunRecord(record *persistence.BenchmarkRunRecord, remaps []PathRemap) *persistence.BenchmarkRunRecord {
	if record == nil {
		return nil
	}
	cloned := *record
	cloned.LinkedTaskRunIDs = append([]string(nil), record.LinkedTaskRunIDs...)
	cloned.AgentMetadataJSON = append(cloned.AgentMetadataJSON[:0:0], record.AgentMetadataJSON...)
	cloned.RunDir = applyPathRemaps(record.RunDir, remaps)
	cloned.ProjectDir = applyPathRemaps(record.ProjectDir, remaps)
	cloned.LogsDir = applyPathRemaps(record.LogsDir, remaps)
	cloned.AgentDir = applyPathRemaps(record.AgentDir, remaps)
	cloned.ConfigPath = applyPathRemaps(record.ConfigPath, remaps)
	cloned.EventStorePath = applyPathRemaps(record.EventStorePath, remaps)
	cloned.RequestLogPath = applyPathRemaps(record.RequestLogPath, remaps)
	cloned.SelectedTemplatePath = applyPathRemaps(record.SelectedTemplatePath, remaps)
	return &cloned
}

func normalizePathRemaps(remaps []PathRemap) []PathRemap {
	if len(remaps) == 0 {
		return nil
	}
	normalized := make([]PathRemap, 0, len(remaps))
	for _, remap := range remaps {
		from := cleanPathForRemap(remap.From)
		to := cleanPathForRemap(remap.To)
		if from == "" || to == "" {
			continue
		}
		normalized = append(normalized, PathRemap{From: from, To: to})
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return len(normalized[i].From) > len(normalized[j].From)
	})
	return normalized
}

func cleanPathForRemap(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
}

func applyPathRemaps(path string, remaps []PathRemap) string {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" || len(remaps) == 0 {
		return cleaned
	}
	cleaned = filepath.Clean(cleaned)
	for _, remap := range remaps {
		if remap.From == "" || remap.To == "" {
			continue
		}
		if cleaned == remap.From {
			return remap.To
		}
		prefix := remap.From + string(os.PathSeparator)
		if strings.HasPrefix(cleaned, prefix) {
			suffix := strings.TrimPrefix(cleaned, prefix)
			if suffix == "" {
				return remap.To
			}
			return filepath.Join(remap.To, suffix)
		}
	}
	return cleaned
}
