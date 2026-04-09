package benchmarks

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/persistence"
)

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

// Comparer builds cross-session comparison summaries from live benchmark data.
type Comparer struct {
	Now func() time.Time
}

// NewComparer returns a comparer with default local behavior.
func NewComparer() *Comparer {
	return &Comparer{Now: time.Now}
}

// CompareSuite loads benchmark sessions for one suite and derives a live comparison view.
func (c *Comparer) CompareSuite(ctx context.Context, opts *CompareOptions) (*ComparisonSummary, string, error) {
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

	storePath, err := config.ResolveEventStorePath(nil)
	if err != nil {
		return nil, "", err
	}
	store, err := persistence.NewSQLiteStore(storePath)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = store.Close() }()

	query := NewQueryService(store)
	view, err := query.GetComparison(ctx, suiteID, BenchmarkRunFilters{
		Agent:           firstFilterValue(opts.Agents),
		CaseID:          firstFilterValue(opts.CaseIDs),
		TemplateVariant: firstFilterValue(opts.TemplateVariants),
	})
	if err != nil {
		return nil, "", err
	}
	if view == nil {
		return nil, "", fmt.Errorf("no benchmark sessions found for suite %q", suiteID)
	}

	suiteRoot := filepath.Join(mustAbs(rootPath), suiteID)
	sessions := make([]ComparisonSession, 0, len(view.Sessions))
	for idx := range view.Sessions {
		if !strings.HasPrefix(filepath.Clean(view.Sessions[idx].SessionPath), filepath.Clean(suiteRoot)) {
			continue
		}
		sessions = append(sessions, view.Sessions[idx])
	}
	rows := make([]RunSummaryRow, 0, len(view.Runs))
	for idx := range view.Runs {
		if !strings.HasPrefix(filepath.Clean(view.Runs[idx].SessionPath), filepath.Clean(suiteRoot)) {
			continue
		}
		rows = append(rows, toRunSummaryRow(view.Runs[idx]))
	}
	if len(sessions) == 0 && len(rows) == 0 {
		return nil, "", fmt.Errorf("no benchmark sessions found under %q", suiteRoot)
	}
	aggregates := buildAggregates(rows)
	comparison := &ComparisonSummary{
		ScoreVersion: scoreVersion,
		RootPath:     mustAbs(rootPath),
		SuiteID:      suiteID,
		GeneratedAt:  c.Now(),
		SessionCount: len(sessions),
		RunCount:     len(rows),
		Sessions:     sessions,
		Runs:         rows,
		Aggregates: ComparisonAggregates{
			BySession: aggregateRows(rows, func(row RunSummaryRow) aggregateKey {
				return aggregateKey{Key: row.SessionPath, SessionPath: row.SessionPath}
			}),
			ByCase:             aggregates.ByCase,
			ByAgent:            aggregates.ByAgent,
			ByTemplateVariant:  aggregates.ByTemplateVariant,
			ByCaseAgentVariant: aggregates.ByCaseAgentVariant,
		},
	}
	return comparison, "", nil
}

func (c *Comparer) withDefaults() *Comparer {
	if c == nil {
		return NewComparer()
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

func firstFilterValue(values []string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func mustAbs(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return absolute
}
