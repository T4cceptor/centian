package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/benchmarks"
	"gotest.tools/assert"
)

type stubBenchmarkStore struct {
	listSuitesFn             func(context.Context, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSuiteSummary, error)
	listTemplateScorecardsFn func(context.Context) ([]benchmarks.TemplateScorecard, error)
	listAgentScorecardsFn    func(context.Context) ([]benchmarks.AgentScorecard, error)
	listSessionsFn           func(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSessionDetail, error)
	getSessionFn             func(context.Context, string, string) (*benchmarks.BenchmarkSessionDetail, error)
	listRunsFn               func(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkRunSummary, error)
	getRunFn                 func(context.Context, string, string) (*benchmarks.BenchmarkRunDetail, error)
	compareFn                func(context.Context, string, benchmarks.BenchmarkRunFilters) (*benchmarks.BenchmarkComparisonView, error)
}

func (s *stubBenchmarkStore) ListSuites(ctx context.Context, filters benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSuiteSummary, error) {
	return s.listSuitesFn(ctx, filters)
}
func (s *stubBenchmarkStore) ListTemplateScorecards(ctx context.Context) ([]benchmarks.TemplateScorecard, error) {
	return s.listTemplateScorecardsFn(ctx)
}
func (s *stubBenchmarkStore) ListAgentScorecards(ctx context.Context) ([]benchmarks.AgentScorecard, error) {
	if s.listAgentScorecardsFn == nil {
		return nil, nil
	}
	return s.listAgentScorecardsFn(ctx)
}
func (s *stubBenchmarkStore) ListSessions(ctx context.Context, suiteID string, filters benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSessionDetail, error) {
	return s.listSessionsFn(ctx, suiteID, filters)
}
func (s *stubBenchmarkStore) GetSession(ctx context.Context, suiteID string, sessionID string) (*benchmarks.BenchmarkSessionDetail, error) {
	return s.getSessionFn(ctx, suiteID, sessionID)
}
func (s *stubBenchmarkStore) ListRuns(ctx context.Context, suiteID string, filters benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkRunSummary, error) {
	return s.listRunsFn(ctx, suiteID, filters)
}
func (s *stubBenchmarkStore) GetRun(ctx context.Context, suiteID string, scorecardID string) (*benchmarks.BenchmarkRunDetail, error) {
	return s.getRunFn(ctx, suiteID, scorecardID)
}
func (s *stubBenchmarkStore) GetComparison(ctx context.Context, suiteID string, filters benchmarks.BenchmarkRunFilters) (*benchmarks.BenchmarkComparisonView, error) {
	return s.compareFn(ctx, suiteID, filters)
}

func TestBenchmarkHandler_ListSuites(t *testing.T) {
	handler := NewBenchmarkHandler(&stubBenchmarkStore{
		listSuitesFn: func(context.Context, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSuiteSummary, error) {
			return []benchmarks.BenchmarkSuiteSummary{{
				SuiteID:           "simple_tdd_v1",
				TemplateID:        "simple_tdd",
				LatestGeneratedAt: time.Date(2026, 4, 5, 9, 0, 0, 0, time.UTC),
				SessionCount:      2,
				RunCount:          4,
			}}, nil
		},
		listTemplateScorecardsFn: func(context.Context) ([]benchmarks.TemplateScorecard, error) { return nil, nil },
		listSessionsFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSessionDetail, error) {
			return nil, nil
		},
		getSessionFn: func(context.Context, string, string) (*benchmarks.BenchmarkSessionDetail, error) { return nil, nil },
		listRunsFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkRunSummary, error) {
			return nil, nil
		},
		getRunFn: func(context.Context, string, string) (*benchmarks.BenchmarkRunDetail, error) { return nil, nil },
		compareFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) (*benchmarks.BenchmarkComparisonView, error) {
			return nil, nil
		},
	})
	mux := http.NewServeMux()
	handler.RegisterRoutesWithMiddleware(mux, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks/suites", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, rec.Code, http.StatusOK)
	var items []benchmarks.BenchmarkSuiteSummary
	assert.NilError(t, json.Unmarshal(rec.Body.Bytes(), &items))
	assert.Equal(t, len(items), 1)
	assert.Equal(t, items[0].SuiteID, "simple_tdd_v1")
}

func TestBenchmarkHandler_GetSessionNotFound(t *testing.T) {
	handler := NewBenchmarkHandler(&stubBenchmarkStore{
		listSuitesFn: func(context.Context, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSuiteSummary, error) {
			return nil, nil
		},
		listTemplateScorecardsFn: func(context.Context) ([]benchmarks.TemplateScorecard, error) { return nil, nil },
		listSessionsFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSessionDetail, error) {
			return nil, nil
		},
		getSessionFn: func(context.Context, string, string) (*benchmarks.BenchmarkSessionDetail, error) { return nil, nil },
		listRunsFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkRunSummary, error) {
			return nil, nil
		},
		getRunFn: func(context.Context, string, string) (*benchmarks.BenchmarkRunDetail, error) { return nil, nil },
		compareFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) (*benchmarks.BenchmarkComparisonView, error) {
			return nil, nil
		},
	})
	mux := http.NewServeMux()
	handler.RegisterRoutesWithMiddleware(mux, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks/suites/simple_tdd_v1/sessions/unknown", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, rec.Code, http.StatusNotFound)
}

func TestBenchmarkHandler_ListRunsHonorsQueryFilters(t *testing.T) {
	var received benchmarks.BenchmarkRunFilters
	handler := NewBenchmarkHandler(&stubBenchmarkStore{
		listSuitesFn: func(context.Context, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSuiteSummary, error) {
			return nil, nil
		},
		listTemplateScorecardsFn: func(context.Context) ([]benchmarks.TemplateScorecard, error) { return nil, nil },
		listSessionsFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSessionDetail, error) {
			return nil, nil
		},
		getSessionFn: func(context.Context, string, string) (*benchmarks.BenchmarkSessionDetail, error) { return nil, nil },
		listRunsFn: func(_ context.Context, suiteID string, filters benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkRunSummary, error) {
			received = filters
			return []benchmarks.BenchmarkRunSummary{{ScorecardID: "ba_run", SuiteID: suiteID, Agent: "claude"}}, nil
		},
		getRunFn: func(context.Context, string, string) (*benchmarks.BenchmarkRunDetail, error) { return nil, nil },
		compareFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) (*benchmarks.BenchmarkComparisonView, error) {
			return nil, nil
		},
	})
	mux := http.NewServeMux()
	handler.RegisterRoutesWithMiddleware(mux, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks/suites/simple_tdd_v1/runs?agent=claude&case=assertion_failure_red&templateVariant=current", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, rec.Code, http.StatusOK)
	assert.Equal(t, received.Agent, "claude")
	assert.Equal(t, received.CaseID, "assertion_failure_red")
	assert.Equal(t, received.TemplateVariant, "current")
}

func TestBenchmarkHandler_ReturnsJSONErrorOnStoreFailure(t *testing.T) {
	handler := NewBenchmarkHandler(&stubBenchmarkStore{
		listSuitesFn: func(context.Context, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSuiteSummary, error) {
			return nil, errors.New("boom")
		},
		listTemplateScorecardsFn: func(context.Context) ([]benchmarks.TemplateScorecard, error) { return nil, nil },
		listSessionsFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSessionDetail, error) {
			return nil, nil
		},
		getSessionFn: func(context.Context, string, string) (*benchmarks.BenchmarkSessionDetail, error) { return nil, nil },
		listRunsFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkRunSummary, error) {
			return nil, nil
		},
		getRunFn: func(context.Context, string, string) (*benchmarks.BenchmarkRunDetail, error) { return nil, nil },
		compareFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) (*benchmarks.BenchmarkComparisonView, error) {
			return nil, nil
		},
	})
	mux := http.NewServeMux()
	handler.RegisterRoutesWithMiddleware(mux, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks/suites", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, rec.Code, http.StatusInternalServerError)
	var response errorResponse
	assert.NilError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, response.Error, "failed to list benchmark suites")
}

func TestBenchmarkHandler_ListTemplateScorecards(t *testing.T) {
	handler := NewBenchmarkHandler(&stubBenchmarkStore{
		listSuitesFn: func(context.Context, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSuiteSummary, error) {
			return nil, nil
		},
		listTemplateScorecardsFn: func(context.Context) ([]benchmarks.TemplateScorecard, error) {
			return []benchmarks.TemplateScorecard{{
				TemplateID:                 "simple_tdd",
				TemplateName:               "Simple TDD Task",
				RunCount:                   15,
				MedianTaskToolCalls:        45,
				MedianDownstreamToolCalls:  15,
				MedianCentianErrors:        3,
				MedianDownstreamToolErrors: 1,
				MedianDurationMillis:       105000,
				FirstPassRate:              0.89,
			}}, nil
		},
		listSessionsFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSessionDetail, error) {
			return nil, nil
		},
		getSessionFn: func(context.Context, string, string) (*benchmarks.BenchmarkSessionDetail, error) {
			return nil, benchmarks.ErrBenchmarkSessionNotFound
		},
		listRunsFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkRunSummary, error) {
			return nil, nil
		},
		getRunFn: func(context.Context, string, string) (*benchmarks.BenchmarkRunDetail, error) {
			return nil, benchmarks.ErrBenchmarkRunNotFound
		},
		compareFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) (*benchmarks.BenchmarkComparisonView, error) {
			return nil, benchmarks.ErrBenchmarkComparisonNotFound
		},
	})
	mux := http.NewServeMux()
	handler.RegisterRoutesWithMiddleware(mux, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks/template-scorecards", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, rec.Code, http.StatusOK)
	var items []benchmarks.TemplateScorecard
	assert.NilError(t, json.Unmarshal(rec.Body.Bytes(), &items))
	assert.Equal(t, len(items), 1)
	assert.Equal(t, items[0].TemplateName, "Simple TDD Task")
}

func TestBenchmarkHandler_ListAgentScorecards(t *testing.T) {
	handler := NewBenchmarkHandler(&stubBenchmarkStore{
		listSuitesFn: func(context.Context, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSuiteSummary, error) {
			return nil, nil
		},
		listTemplateScorecardsFn: func(context.Context) ([]benchmarks.TemplateScorecard, error) { return nil, nil },
		listAgentScorecardsFn: func(context.Context) ([]benchmarks.AgentScorecard, error) {
			return []benchmarks.AgentScorecard{{
				Agent:                      "codex",
				RunCount:                   8,
				MedianTaskToolCalls:        20,
				MedianDownstreamToolCalls:  10,
				MedianCentianErrors:        1,
				MedianDownstreamToolErrors: 0,
				MedianDurationMillis:       90000,
				FirstPassRate:              0.75,
			}}, nil
		},
		listSessionsFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSessionDetail, error) {
			return nil, nil
		},
		getSessionFn: func(context.Context, string, string) (*benchmarks.BenchmarkSessionDetail, error) {
			return nil, benchmarks.ErrBenchmarkSessionNotFound
		},
		listRunsFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkRunSummary, error) {
			return nil, nil
		},
		getRunFn: func(context.Context, string, string) (*benchmarks.BenchmarkRunDetail, error) {
			return nil, benchmarks.ErrBenchmarkRunNotFound
		},
		compareFn: func(context.Context, string, benchmarks.BenchmarkRunFilters) (*benchmarks.BenchmarkComparisonView, error) {
			return nil, benchmarks.ErrBenchmarkComparisonNotFound
		},
	})
	mux := http.NewServeMux()
	handler.RegisterRoutesWithMiddleware(mux, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/benchmarks/agent-scorecards", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, rec.Code, http.StatusOK)
	var items []benchmarks.AgentScorecard
	assert.NilError(t, json.Unmarshal(rec.Body.Bytes(), &items))
	assert.Equal(t, len(items), 1)
	assert.Equal(t, items[0].Agent, "codex")
}
