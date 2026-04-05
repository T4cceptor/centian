package benchmarks

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/T4cceptor/centian/internal/persistence"
)

// ArtifactReader lists persisted benchmark artifact blobs for the read API.
type ArtifactReader interface {
	ListBenchmarkArtifacts(context.Context, persistence.BenchmarkArtifactFilter) ([]persistence.BenchmarkArtifactRecord, error)
}

type taskRunSnapshotReader interface {
	GetTaskRunSnapshot(context.Context, string) (*persistence.TaskRunSnapshotRecord, error)
}

// ReadService converts persisted benchmark artifacts into typed read models.
type ReadService struct {
	store ArtifactReader
}

type persistedSummary struct {
	record  persistence.BenchmarkArtifactRecord
	summary SessionSummary
}

type persistedScorecard struct {
	record    persistence.BenchmarkArtifactRecord
	scorecard RunScorecard
}

type sessionRunGroup struct {
	templateID   string
	templateName string
	suiteName    string
	runs         []BenchmarkRunSummary
	agents       map[string]struct{}
	caseIDs      map[string]struct{}
	caseNames    map[string]struct{}
	variants     map[string]struct{}
}

type persistedSession struct {
	record  persistence.BenchmarkArtifactRecord
	session SessionManifest
}

type suiteCatalog struct {
	suiteName string
	caseNames map[string]string
}

// NewReadService builds a read-only benchmark query service.
func NewReadService(store ArtifactReader) *ReadService {
	return &ReadService{store: store}
}

// ListSuites returns suite-level benchmark summaries.
func (s *ReadService) ListSuites(ctx context.Context, filters BenchmarkRunFilters) ([]BenchmarkSuiteSummary, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("benchmark read service is not initialized")
	}
	suiteID := strings.TrimSpace(filters.SuiteID)
	summaries, err := s.loadSummaries(ctx, suiteID)
	if err != nil {
		return nil, err
	}
	sessionManifests, err := s.loadSessions(ctx, suiteID)
	if err != nil {
		return nil, err
	}
	catalogs := loadSuiteCatalogs(sessionManifests)
	scorecards, err := s.loadScorecards(ctx, suiteID, filters)
	if err != nil {
		return nil, err
	}
	applyCatalogToScorecards(scorecards, catalogs)

	type suiteAggregate struct {
		item       BenchmarkSuiteSummary
		sessionIDs map[string]struct{}
		agents     map[string]struct{}
		caseIDs    map[string]struct{}
		variants   map[string]struct{}
	}
	grouped := map[string]*suiteAggregate{}

	for _, item := range summaries {
		group := grouped[item.summary.SuiteID]
		if group == nil {
			group = &suiteAggregate{
				item:       BenchmarkSuiteSummary{SuiteID: item.summary.SuiteID},
				sessionIDs: map[string]struct{}{},
				agents:     map[string]struct{}{},
				caseIDs:    map[string]struct{}{},
				variants:   map[string]struct{}{},
			}
			grouped[item.summary.SuiteID] = group
		}
		if catalog, ok := catalogs[item.summary.SuiteID]; ok {
			group.item.SuiteName = firstNonEmpty(group.item.SuiteName, catalog.suiteName)
			group.item.CaseNames = sortedMapValues(catalog.caseNames)
		}
		group.sessionIDs[item.record.SessionID] = struct{}{}
		if item.summary.GeneratedAt.After(group.item.LatestGeneratedAt) {
			group.item.LatestGeneratedAt = item.summary.GeneratedAt
		}
	}
	for _, item := range scorecards {
		group := grouped[item.scorecard.SuiteID]
		if group == nil {
			group = &suiteAggregate{
				item:       BenchmarkSuiteSummary{SuiteID: item.scorecard.SuiteID},
				sessionIDs: map[string]struct{}{},
				agents:     map[string]struct{}{},
				caseIDs:    map[string]struct{}{},
				variants:   map[string]struct{}{},
			}
			grouped[item.scorecard.SuiteID] = group
		}
		group.item.TemplateID = firstNonEmpty(group.item.TemplateID, item.scorecard.TemplateID)
		group.item.TemplateName = firstNonEmpty(group.item.TemplateName, item.scorecard.TemplateName)
		group.item.RunCount++
		group.sessionIDs[item.record.SessionID] = struct{}{}
		group.agents[item.scorecard.Agent] = struct{}{}
		group.caseIDs[item.scorecard.CaseID] = struct{}{}
		group.variants[item.scorecard.TemplateVariant] = struct{}{}
		if item.scorecard.GeneratedAt.After(group.item.LatestGeneratedAt) {
			group.item.LatestGeneratedAt = item.scorecard.GeneratedAt
		}
	}

	result := make([]BenchmarkSuiteSummary, 0, len(grouped))
	for _, group := range grouped {
		if suiteID != "" && group.item.SuiteID != suiteID {
			continue
		}
		if filters.TemplateID != "" && group.item.TemplateID != filters.TemplateID {
			continue
		}
		group.item.SessionCount = len(group.sessionIDs)
		group.item.Agents = sortedSetValues(group.agents)
		group.item.CaseIDs = sortedSetValues(group.caseIDs)
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
func (s *ReadService) GetSession(ctx context.Context, suiteID string, sessionID string) (*BenchmarkSessionDetail, error) {
	sessions, err := s.listSessions(ctx, suiteID, BenchmarkRunFilters{SessionID: sessionID}, true)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	return &sessions[0], nil
}

// ListRuns returns scorecard-backed benchmark runs for one suite.
func (s *ReadService) ListRuns(ctx context.Context, suiteID string, filters BenchmarkRunFilters) ([]BenchmarkRunSummary, error) {
	sessionManifests, err := s.loadSessions(ctx, suiteID)
	if err != nil {
		return nil, err
	}
	catalogs := loadSuiteCatalogs(sessionManifests)
	scorecards, err := s.loadScorecards(ctx, suiteID, filters)
	if err != nil {
		return nil, err
	}
	applyCatalogToScorecards(scorecards, catalogs)
	result := make([]BenchmarkRunSummary, 0, len(scorecards))
	for _, item := range scorecards {
		result = append(result, buildBenchmarkRunSummary(item))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SessionPath != result[j].SessionPath {
			return result[i].SessionPath > result[j].SessionPath
		}
		return compareRunRows(toRunSummaryRow(result[i]), toRunSummaryRow(result[j]))
	})
	return result, nil
}

// GetRun returns one run scorecard detail.
func (s *ReadService) GetRun(ctx context.Context, suiteID string, scorecardID string) (*BenchmarkRunDetail, error) {
	sessionManifests, err := s.loadSessions(ctx, suiteID)
	if err != nil {
		return nil, err
	}
	catalogs := loadSuiteCatalogs(sessionManifests)
	scorecards, err := s.loadScorecards(ctx, suiteID, BenchmarkRunFilters{})
	if err != nil {
		return nil, err
	}
	applyCatalogToScorecards(scorecards, catalogs)
	for _, item := range scorecards {
		if item.record.ID != scorecardID {
			continue
		}
		return &BenchmarkRunDetail{
			ScorecardID:  item.record.ID,
			SessionID:    item.record.SessionID,
			SessionPath:  item.record.SessionPath,
			SuiteName:    item.scorecard.SuiteName,
			TemplateName: item.scorecard.TemplateName,
			CaseName:     item.scorecard.CaseName,
			Scorecard:    item.scorecard,
		}, nil
	}
	return nil, nil
}

// GetComparison derives a filtered comparison view from persisted summaries and scorecards.
func (s *ReadService) GetComparison(ctx context.Context, suiteID string, filters BenchmarkRunFilters) (*BenchmarkComparisonView, error) {
	summaries, err := s.loadSummaries(ctx, suiteID)
	if err != nil {
		return nil, err
	}
	sessionManifests, err := s.loadSessions(ctx, suiteID)
	if err != nil {
		return nil, err
	}
	catalogs := loadSuiteCatalogs(sessionManifests)
	scorecards, err := s.loadScorecards(ctx, suiteID, filters)
	if err != nil {
		return nil, err
	}
	applyCatalogToScorecards(scorecards, catalogs)
	if len(summaries) == 0 && len(scorecards) == 0 {
		return nil, nil
	}

	templateID := ""
	templateName := ""
	suiteName := ""
	rows := make([]RunSummaryRow, 0, len(scorecards))
	runs := make([]BenchmarkRunSummary, 0, len(scorecards))
	sessionMatches := map[string]int{}
	for _, item := range scorecards {
		templateID = firstNonEmpty(templateID, item.scorecard.TemplateID)
		templateName = firstNonEmpty(templateName, item.scorecard.TemplateName)
		suiteName = firstNonEmpty(suiteName, item.scorecard.SuiteName)
		row := buildRunSummaryRow(SessionRunManifestEntry{
			CaseID:          item.scorecard.CaseID,
			AgentID:         item.scorecard.Agent,
			TemplateVariant: item.scorecard.TemplateVariant,
			Attempt:         item.scorecard.Attempt,
		}, nil, &item.scorecard, nil, nil)
		row.SessionPath = item.record.SessionPath
		rows = append(rows, row)
		runs = append(runs, buildBenchmarkRunSummary(item))
		sessionMatches[item.record.SessionID]++
	}

	sessions := make([]ComparisonSession, 0, len(summaries))
	for _, item := range summaries {
		if hasRunScopedFilter(filters) && sessionMatches[item.record.SessionID] == 0 {
			continue
		}
		sessions = append(sessions, ComparisonSession{
			SessionPath:        item.summary.SessionPath,
			GeneratedAt:        item.summary.GeneratedAt,
			RunCount:           item.summary.RunCount,
			ScoredRunCount:     item.summary.ScoredRunCount,
			FailedToScoreCount: item.summary.FailedToScoreCount,
		})
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].SessionPath > sessions[j].SessionPath })
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].SessionPath != runs[j].SessionPath {
			return runs[i].SessionPath > runs[j].SessionPath
		}
		return compareRunRows(toRunSummaryRow(runs[i]), toRunSummaryRow(runs[j]))
	})

	aggregates := buildAggregates(rows)
	return &BenchmarkComparisonView{
		SuiteID:      suiteID,
		SuiteName:    firstNonEmpty(suiteName, suiteNameForSummary(suiteID, catalogs)),
		TemplateID:   templateID,
		TemplateName: templateName,
		Filters:      filters,
		SessionCount: len(sessions),
		RunCount:     len(runs),
		Sessions:     sessions,
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
	summaries, err := s.loadSummaries(ctx, suiteID)
	if err != nil {
		return nil, err
	}
	sessionManifests, err := s.loadSessions(ctx, suiteID)
	if err != nil {
		return nil, err
	}
	catalogs := loadSuiteCatalogs(sessionManifests)
	scorecards, err := s.loadScorecards(ctx, suiteID, filters)
	if err != nil {
		return nil, err
	}
	applyCatalogToScorecards(scorecards, catalogs)
	runsBySession := map[string]*sessionRunGroup{}
	for _, item := range scorecards {
		group := runsBySession[item.record.SessionID]
		if group == nil {
			group = &sessionRunGroup{
				agents:    map[string]struct{}{},
				caseIDs:   map[string]struct{}{},
				caseNames: map[string]struct{}{},
				variants:  map[string]struct{}{},
			}
			runsBySession[item.record.SessionID] = group
		}
		group.templateID = firstNonEmpty(group.templateID, item.scorecard.TemplateID)
		group.templateName = firstNonEmpty(group.templateName, item.scorecard.TemplateName)
		group.suiteName = firstNonEmpty(group.suiteName, item.scorecard.SuiteName)
		group.agents[item.scorecard.Agent] = struct{}{}
		group.caseIDs[item.scorecard.CaseID] = struct{}{}
		if strings.TrimSpace(item.scorecard.CaseName) != "" {
			group.caseNames[item.scorecard.CaseName] = struct{}{}
		}
		group.variants[item.scorecard.TemplateVariant] = struct{}{}
		group.runs = append(group.runs, buildBenchmarkRunSummary(item))
	}

	result := make([]BenchmarkSessionDetail, 0, len(summaries))
	for _, item := range summaries {
		if filters.SessionID != "" && item.record.SessionID != filters.SessionID {
			continue
		}
		group := runsBySession[item.record.SessionID]
		if hasRunScopedFilter(filters) && group == nil {
			continue
		}
		detail := BenchmarkSessionDetail{
			SessionID:          item.record.SessionID,
			SuiteID:            item.summary.SuiteID,
			SuiteName:          suiteNameForSummary(item.summary.SuiteID, catalogs),
			TemplateID:         sessionTemplateID(group),
			TemplateName:       sessionTemplateName(group),
			SessionPath:        item.summary.SessionPath,
			GeneratedAt:        item.summary.GeneratedAt,
			RunCount:           item.summary.RunCount,
			ScoredRunCount:     item.summary.ScoredRunCount,
			FailedToScoreCount: item.summary.FailedToScoreCount,
			Aggregates:         item.summary.Aggregates,
		}
		if group != nil {
			detail.Agents = sortedSetValues(group.agents)
			detail.CaseIDs = sortedSetValues(group.caseIDs)
			detail.CaseNames = sortedSetValues(group.caseNames)
			detail.TemplateVariants = sortedSetValues(group.variants)
			if includeRuns {
				sort.Slice(group.runs, func(i, j int) bool {
					return compareRunRows(toRunSummaryRow(group.runs[i]), toRunSummaryRow(group.runs[j]))
				})
				detail.Runs = append(detail.Runs, group.runs...)
			}
		} else {
			detail.Agents = collectDistinctSummaryValues(item.summary.Runs, func(row RunSummaryRow) string { return row.Agent })
			detail.CaseIDs = collectDistinctSummaryValues(item.summary.Runs, func(row RunSummaryRow) string { return row.CaseID })
			if catalog, ok := catalogs[item.summary.SuiteID]; ok {
				detail.CaseNames = sortedMapValues(catalog.caseNames)
			}
			detail.TemplateVariants = collectDistinctSummaryValues(item.summary.Runs, func(row RunSummaryRow) string { return row.TemplateVariant })
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

func (s *ReadService) loadSummaries(ctx context.Context, suiteID string) ([]persistedSummary, error) {
	filter := persistence.BenchmarkArtifactFilter{ArtifactKind: persistence.BenchmarkArtifactKindSummary}
	if strings.TrimSpace(suiteID) != "" {
		filter.SuiteID = suiteID
	}
	records, err := s.store.ListBenchmarkArtifacts(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := make([]persistedSummary, 0, len(records))
	for _, record := range records {
		var summary SessionSummary
		if err := json.Unmarshal(record.PayloadJSON, &summary); err != nil {
			return nil, fmt.Errorf("decode summary artifact %s: %w", record.ID, err)
		}
		result = append(result, persistedSummary{record: record, summary: summary})
	}
	return result, nil
}

func (s *ReadService) loadSessions(ctx context.Context, suiteID string) ([]persistedSession, error) {
	filter := persistence.BenchmarkArtifactFilter{ArtifactKind: persistence.BenchmarkArtifactKindSession}
	if strings.TrimSpace(suiteID) != "" {
		filter.SuiteID = suiteID
	}
	records, err := s.store.ListBenchmarkArtifacts(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := make([]persistedSession, 0, len(records))
	for _, record := range records {
		var session SessionManifest
		if err := json.Unmarshal(record.PayloadJSON, &session); err != nil {
			return nil, fmt.Errorf("decode session artifact %s: %w", record.ID, err)
		}
		result = append(result, persistedSession{record: record, session: session})
	}
	return result, nil
}

func (s *ReadService) loadScorecards(ctx context.Context, suiteID string, filters BenchmarkRunFilters) ([]persistedScorecard, error) {
	filter := persistence.BenchmarkArtifactFilter{ArtifactKind: persistence.BenchmarkArtifactKindScorecard}
	if strings.TrimSpace(suiteID) != "" {
		filter.SuiteID = suiteID
	}
	filter.SessionID = strings.TrimSpace(filters.SessionID)
	filter.CaseID = strings.TrimSpace(filters.CaseID)
	filter.Agent = strings.TrimSpace(filters.Agent)
	filter.TemplateVariant = strings.TrimSpace(filters.TemplateVariant)
	records, err := s.store.ListBenchmarkArtifacts(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := make([]persistedScorecard, 0, len(records))
	for _, record := range records {
		var scorecard RunScorecard
		if err := json.Unmarshal(record.PayloadJSON, &scorecard); err != nil {
			return nil, fmt.Errorf("decode scorecard artifact %s: %w", record.ID, err)
		}
		enrichScorecardNames(ctx, s.store, &scorecard)
		if filters.TemplateID != "" && scorecard.TemplateID != filters.TemplateID {
			continue
		}
		result = append(result, persistedScorecard{record: record, scorecard: scorecard})
	}
	return result, nil
}

func buildBenchmarkRunSummary(item persistedScorecard) BenchmarkRunSummary {
	return BenchmarkRunSummary{
		ScorecardID:               item.record.ID,
		SessionID:                 item.record.SessionID,
		SessionPath:               item.record.SessionPath,
		SuiteID:                   item.scorecard.SuiteID,
		SuiteName:                 item.scorecard.SuiteName,
		TemplateID:                item.scorecard.TemplateID,
		TemplateName:              item.scorecard.TemplateName,
		CaseID:                    item.scorecard.CaseID,
		CaseName:                  item.scorecard.CaseName,
		Agent:                     item.scorecard.Agent,
		TemplateVariant:           item.scorecard.TemplateVariant,
		Attempt:                   item.scorecard.Attempt,
		RawStatus:                 item.scorecard.RawStatus,
		LatestTaskRunID:           item.scorecard.LatestTaskRunID,
		LinkedTaskRunIDs:          append([]string(nil), item.scorecard.LinkedTaskRunIDs...),
		CompletedSuccessfully:     item.scorecard.Outcome.CompletedSuccessfully,
		FinalVerificationPassed:   item.scorecard.Outcome.FinalVerificationPassed,
		FirstPassSuccess:          item.scorecard.Outcome.FirstPassSuccess,
		InvariantViolation:        item.scorecard.Outcome.InvariantViolation,
		RestartOccurred:           item.scorecard.Outcome.RestartOccurred,
		FailOccurred:              item.scorecard.Outcome.FailOccurred,
		TimeoutOccurred:           item.scorecard.Outcome.TimeoutOccurred,
		WallClockSeconds:          item.scorecard.Efficiency.WallClockSeconds,
		TotalToolCalls:            item.scorecard.Efficiency.TotalToolCalls,
		TotalTaskToolCalls:        item.scorecard.Process.TotalTaskToolCalls,
		TotalDownstreamToolCalls:  item.scorecard.Process.TotalDownstreamToolCalls,
		InputTokens:               item.scorecard.Efficiency.InputTokens,
		OutputTokens:              item.scorecard.Efficiency.OutputTokens,
		FailedTaskToolCalls:       item.scorecard.Process.FailedTaskToolCalls,
		FailedDownstreamToolCalls: item.scorecard.Process.FailedDownstreamToolCalls,
		EditedFilesCount:          item.scorecard.Efficiency.EditedFilesCount,
		ErrorActionabilityScore:   item.scorecard.Manual.ErrorActionabilityScore,
		AgentMetadata:             item.scorecard.AgentMetadata,
		Warnings:                  append([]string(nil), item.scorecard.Warnings...),
		Errors:                    append([]string(nil), item.scorecard.Errors...),
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
		Scored:                    true,
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

func hasRunScopedFilter(filters BenchmarkRunFilters) bool {
	return strings.TrimSpace(filters.TemplateID) != "" ||
		strings.TrimSpace(filters.SessionID) != "" ||
		strings.TrimSpace(filters.CaseID) != "" ||
		strings.TrimSpace(filters.Agent) != "" ||
		strings.TrimSpace(filters.TemplateVariant) != ""
}

func collectDistinctSummaryValues(rows []RunSummaryRow, valueFn func(RunSummaryRow) string) []string {
	set := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if value := strings.TrimSpace(valueFn(row)); value != "" {
			set[value] = struct{}{}
		}
	}
	return sortedSetValues(set)
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

func sessionTemplateID(group *sessionRunGroup) string {
	if group == nil {
		return ""
	}
	return group.templateID
}

func sessionTemplateName(group *sessionRunGroup) string {
	if group == nil {
		return ""
	}
	return group.templateName
}

func enrichScorecardNames(ctx context.Context, store ArtifactReader, scorecard *RunScorecard) {
	if scorecard == nil {
		return
	}
	if snapshotStore, ok := store.(taskRunSnapshotReader); ok && strings.TrimSpace(scorecard.LatestTaskRunID) != "" {
		if snapshot, err := snapshotStore.GetTaskRunSnapshot(ctx, scorecard.LatestTaskRunID); err == nil && snapshot != nil {
			scorecard.TemplateName = firstNonEmpty(scorecard.TemplateName, snapshot.TemplateName)
		}
	}
}

func loadSuiteCatalogs(sessions []persistedSession) map[string]suiteCatalog {
	result := map[string]suiteCatalog{}
	for _, item := range sessions {
		if _, exists := result[item.session.SuiteID]; exists {
			continue
		}
		suite, err := LoadSuite(item.session.SuitePath)
		if err != nil || suite == nil {
			continue
		}
		catalog := suiteCatalog{
			suiteName: strings.TrimSpace(suite.Suite.Name),
			caseNames: map[string]string{},
		}
		for _, ref := range suite.Cases {
			caseDef, err := LoadCase(item.session.SuitePath, ref)
			if err != nil || caseDef == nil {
				continue
			}
			catalog.caseNames[ref.ID] = strings.TrimSpace(caseDef.Case.Name)
		}
		result[item.session.SuiteID] = catalog
	}
	return result
}

func suiteNameForSummary(suiteID string, catalogs map[string]suiteCatalog) string {
	if catalog, ok := catalogs[suiteID]; ok {
		return catalog.suiteName
	}
	return ""
}

func sortedMapValues(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	set := map[string]struct{}{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			set[value] = struct{}{}
		}
	}
	return sortedSetValues(set)
}

func applyCatalogToScorecards(scorecards []persistedScorecard, catalogs map[string]suiteCatalog) {
	for idx := range scorecards {
		catalog, ok := catalogs[scorecards[idx].scorecard.SuiteID]
		if !ok {
			continue
		}
		scorecards[idx].scorecard.SuiteName = firstNonEmpty(scorecards[idx].scorecard.SuiteName, catalog.suiteName)
		scorecards[idx].scorecard.CaseName = firstNonEmpty(scorecards[idx].scorecard.CaseName, catalog.caseNames[scorecards[idx].scorecard.CaseID])
	}
}
