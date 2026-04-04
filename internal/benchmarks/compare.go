package benchmarks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/persistence"
)

const comparisonFileName = "comparison.json"

// CompareOptions configures cross-session comparison for one benchmark suite.
type CompareOptions struct {
	RootPath         string
	SuiteID          string
	Agents           []string
	CaseIDs          []string
	TemplateVariants []string
}

// ComparisonSummary stores cross-session comparison data for one suite.
type ComparisonSummary struct {
	ScoreVersion string               `json:"scoreVersion"`
	RootPath     string               `json:"rootPath"`
	SuiteID      string               `json:"suiteId"`
	GeneratedAt  time.Time            `json:"generatedAt"`
	SessionCount int                  `json:"sessionCount"`
	RunCount     int                  `json:"runCount"`
	Sessions     []ComparisonSession  `json:"sessions"`
	Runs         []RunSummaryRow      `json:"runs"`
	Aggregates   ComparisonAggregates `json:"aggregates"`
}

// ComparisonAggregates contains grouped aggregates across scored sessions.
type ComparisonAggregates struct {
	BySession          []AggregateSummary `json:"bySession"`
	ByCase             []AggregateSummary `json:"byCase"`
	ByAgent            []AggregateSummary `json:"byAgent"`
	ByTemplateVariant  []AggregateSummary `json:"byTemplateVariant"`
	ByCaseAgentVariant []AggregateSummary `json:"byCaseAgentVariant"`
}

// ComparisonSession is the compact per-session row preserved in comparison output.
type ComparisonSession struct {
	SessionPath        string    `json:"sessionPath"`
	GeneratedAt        time.Time `json:"generatedAt"`
	RunCount           int       `json:"runCount"`
	ScoredRunCount     int       `json:"scoredRunCount"`
	FailedToScoreCount int       `json:"failedToScoreCount"`
}

// Comparer builds cross-session comparison summaries from preserved session summaries.
type Comparer struct {
	Now             func() time.Time
	PersistArtifact func(context.Context, string, *persistence.BenchmarkArtifactRecord) error
}

// NewComparer returns a comparer with default local behavior.
func NewComparer() *Comparer {
	return &Comparer{Now: time.Now, PersistArtifact: persistBenchmarkArtifact}
}

// CompareSuite loads scored sessions for one suite and writes comparison.json.
func (c *Comparer) CompareSuite(_ context.Context, opts *CompareOptions) (*ComparisonSummary, string, error) {
	c = c.withDefaults()
	if opts == nil {
		return nil, "", fmt.Errorf("compare options are required")
	}
	rootPath := strings.TrimSpace(opts.RootPath)
	if rootPath == "" {
		return nil, "", fmt.Errorf("root path is required")
	}
	suiteID := strings.TrimSpace(opts.SuiteID)
	if suiteID == "" {
		return nil, "", fmt.Errorf("suite id is required")
	}
	rootPath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, "", fmt.Errorf("resolve root path: %w", err)
	}
	suiteRoot := filepath.Join(rootPath, suiteID)
	info, err := os.Stat(suiteRoot)
	if err != nil {
		return nil, "", fmt.Errorf("stat suite root: %w", err)
	}
	if !info.IsDir() {
		return nil, "", fmt.Errorf("suite root %q must be a directory", suiteRoot)
	}

	filter := comparisonFilter{
		agents:           makeSet(opts.Agents),
		caseIDs:          makeSet(opts.CaseIDs),
		templateVariants: makeSet(opts.TemplateVariants),
	}

	entries, err := os.ReadDir(suiteRoot)
	if err != nil {
		return nil, "", fmt.Errorf("read suite root: %w", err)
	}

	sessions := make([]ComparisonSession, 0)
	rows := make([]RunSummaryRow, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionDir := filepath.Join(suiteRoot, entry.Name())
		summaryPath := filepath.Join(sessionDir, summaryFileName)
		if _, err := os.Stat(summaryPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, "", fmt.Errorf("stat summary for %q: %w", sessionDir, err)
		}
		var summary SessionSummary
		if err := readJSONFile(summaryPath, &summary); err != nil {
			return nil, "", fmt.Errorf("load summary for %q: %w", sessionDir, err)
		}
		sessions = append(sessions, ComparisonSession{
			SessionPath:        summary.SessionPath,
			GeneratedAt:        summary.GeneratedAt,
			RunCount:           summary.RunCount,
			ScoredRunCount:     summary.ScoredRunCount,
			FailedToScoreCount: summary.FailedToScoreCount,
		})
		for _, row := range summary.Runs {
			if strings.TrimSpace(row.SessionPath) == "" {
				row.SessionPath = summary.SessionPath
			}
			if filter.matches(row) {
				rows = append(rows, row)
			}
		}
	}
	if len(sessions) == 0 {
		return nil, "", fmt.Errorf("no scored benchmark sessions found under %q", suiteRoot)
	}

	sort.Slice(sessions, func(i, j int) bool { return sessions[i].SessionPath < sessions[j].SessionPath })
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].SessionPath != rows[j].SessionPath {
			return rows[i].SessionPath < rows[j].SessionPath
		}
		return compareRunRows(rows[i], rows[j])
	})

	scoredRows := make([]RunSummaryRow, 0, len(rows))
	for _, row := range rows {
		if row.Scored {
			scoredRows = append(scoredRows, row)
		}
	}
	sessionAggregates := buildAggregates(scoredRows)
	comparison := &ComparisonSummary{
		ScoreVersion: scoreVersion,
		RootPath:     rootPath,
		SuiteID:      suiteID,
		GeneratedAt:  c.Now(),
		SessionCount: len(sessions),
		RunCount:     len(rows),
		Sessions:     sessions,
		Runs:         rows,
		Aggregates: ComparisonAggregates{
			BySession: aggregateRows(scoredRows, func(row RunSummaryRow) aggregateKey {
				return aggregateKey{Key: row.SessionPath, SessionPath: row.SessionPath}
			}),
			ByCase:             sessionAggregates.ByCase,
			ByAgent:            sessionAggregates.ByAgent,
			ByTemplateVariant:  sessionAggregates.ByTemplateVariant,
			ByCaseAgentVariant: sessionAggregates.ByCaseAgentVariant,
		},
	}
	outputPath := filepath.Join(suiteRoot, comparisonFileName)
	if err := writeJSONFile(outputPath, comparison); err != nil {
		return nil, "", fmt.Errorf("write comparison summary: %w", err)
	}
	record, err := buildComparisonArtifactRecord(comparison)
	if err != nil {
		return nil, "", err
	}
	storePath, err := config.ResolveEventStorePath(nil)
	if err != nil {
		return nil, "", err
	}
	if err := c.PersistArtifact(context.Background(), storePath, record); err != nil {
		return nil, "", err
	}
	return comparison, outputPath, nil
}

func (c *Comparer) withDefaults() *Comparer {
	if c == nil {
		return NewComparer()
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.PersistArtifact == nil {
		c.PersistArtifact = persistBenchmarkArtifact
	}
	return c
}

type comparisonFilter struct {
	agents           map[string]struct{}
	caseIDs          map[string]struct{}
	templateVariants map[string]struct{}
}

func (f comparisonFilter) matches(row RunSummaryRow) bool {
	return matchesSet(f.agents, row.Agent) &&
		matchesSet(f.caseIDs, row.CaseID) &&
		matchesSet(f.templateVariants, row.TemplateVariant)
}

func makeSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func matchesSet(set map[string]struct{}, value string) bool {
	if len(set) == 0 {
		return true
	}
	_, ok := set[value]
	return ok
}
