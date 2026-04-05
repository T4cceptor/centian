package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/T4cceptor/centian/internal/benchmarks"
)

// BenchmarkStore provides the benchmark read methods required by the API.
type BenchmarkStore interface {
	ListSuites(context.Context, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSuiteSummary, error)
	ListSessions(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkSessionDetail, error)
	GetSession(context.Context, string, string) (*benchmarks.BenchmarkSessionDetail, error)
	ListRuns(context.Context, string, benchmarks.BenchmarkRunFilters) ([]benchmarks.BenchmarkRunSummary, error)
	GetRun(context.Context, string, string) (*benchmarks.BenchmarkRunDetail, error)
	GetComparison(context.Context, string, benchmarks.BenchmarkRunFilters) (*benchmarks.BenchmarkComparisonView, error)
}

// BenchmarkHandler serves the read-only benchmark API.
type BenchmarkHandler struct {
	store BenchmarkStore
}

// NewBenchmarkHandler builds a benchmark API handler.
func NewBenchmarkHandler(store BenchmarkStore) *BenchmarkHandler {
	return &BenchmarkHandler{store: store}
}

// RegisterRoutesWithMiddleware registers benchmark routes with optional middleware.
func (h *BenchmarkHandler) RegisterRoutesWithMiddleware(mux *http.ServeMux, middleware func(http.Handler) http.Handler) {
	if h == nil || h.store == nil || mux == nil {
		return
	}

	register := func(pattern string, handler http.HandlerFunc) {
		var wrapped http.Handler = handler
		if middleware != nil {
			wrapped = middleware(wrapped)
		}
		mux.Handle(pattern, wrapped)
	}

	register("GET /api/benchmarks/suites", h.handleListSuites)
	register("GET /api/benchmarks/suites/{suiteID}/sessions", h.handleListSessions)
	register("GET /api/benchmarks/suites/{suiteID}/sessions/{sessionID}", h.handleGetSession)
	register("GET /api/benchmarks/suites/{suiteID}/runs", h.handleListRuns)
	register("GET /api/benchmarks/suites/{suiteID}/runs/{scorecardID}", h.handleGetRun)
	register("GET /api/benchmarks/suites/{suiteID}/comparison", h.handleGetComparison)
}

func (h *BenchmarkHandler) handleListSuites(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListSuites(r.Context(), benchmarkFiltersFromQuery(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list benchmark suites")
		return
	}
	if items == nil {
		items = []benchmarks.BenchmarkSuiteSummary{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *BenchmarkHandler) handleListSessions(w http.ResponseWriter, r *http.Request) {
	suiteID := strings.TrimSpace(r.PathValue("suiteID"))
	if suiteID == "" {
		writeError(w, http.StatusBadRequest, "suite id is required")
		return
	}
	items, err := h.store.ListSessions(r.Context(), suiteID, benchmarkFiltersFromQuery(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list benchmark sessions")
		return
	}
	if items == nil {
		items = []benchmarks.BenchmarkSessionDetail{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *BenchmarkHandler) handleGetSession(w http.ResponseWriter, r *http.Request) {
	suiteID := strings.TrimSpace(r.PathValue("suiteID"))
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if suiteID == "" || sessionID == "" {
		writeError(w, http.StatusBadRequest, "suite id and session id are required")
		return
	}
	item, err := h.store.GetSession(r.Context(), suiteID, sessionID)
	if err != nil {
		if errors.Is(err, benchmarks.ErrBenchmarkSessionNotFound) {
			writeError(w, http.StatusNotFound, "benchmark session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get benchmark session")
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "benchmark session not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *BenchmarkHandler) handleListRuns(w http.ResponseWriter, r *http.Request) {
	suiteID := strings.TrimSpace(r.PathValue("suiteID"))
	if suiteID == "" {
		writeError(w, http.StatusBadRequest, "suite id is required")
		return
	}
	items, err := h.store.ListRuns(r.Context(), suiteID, benchmarkFiltersFromQuery(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list benchmark runs")
		return
	}
	if items == nil {
		items = []benchmarks.BenchmarkRunSummary{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *BenchmarkHandler) handleGetRun(w http.ResponseWriter, r *http.Request) {
	suiteID := strings.TrimSpace(r.PathValue("suiteID"))
	scorecardID := strings.TrimSpace(r.PathValue("scorecardID"))
	if suiteID == "" || scorecardID == "" {
		writeError(w, http.StatusBadRequest, "suite id and scorecard id are required")
		return
	}
	item, err := h.store.GetRun(r.Context(), suiteID, scorecardID)
	if err != nil {
		if errors.Is(err, benchmarks.ErrBenchmarkRunNotFound) {
			writeError(w, http.StatusNotFound, "benchmark run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get benchmark run")
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "benchmark run not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *BenchmarkHandler) handleGetComparison(w http.ResponseWriter, r *http.Request) {
	suiteID := strings.TrimSpace(r.PathValue("suiteID"))
	if suiteID == "" {
		writeError(w, http.StatusBadRequest, "suite id is required")
		return
	}
	item, err := h.store.GetComparison(r.Context(), suiteID, benchmarkFiltersFromQuery(r))
	if err != nil {
		if errors.Is(err, benchmarks.ErrBenchmarkComparisonNotFound) {
			writeError(w, http.StatusNotFound, "benchmark comparison not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get benchmark comparison")
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "benchmark comparison not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func benchmarkFiltersFromQuery(r *http.Request) benchmarks.BenchmarkRunFilters {
	query := r.URL.Query()
	return benchmarks.BenchmarkRunFilters{
		SuiteID:         strings.TrimSpace(query.Get("suite")),
		TemplateID:      strings.TrimSpace(firstQueryValue(query.Get("template"), query.Get("templateId"))),
		SessionID:       strings.TrimSpace(query.Get("sessionID")),
		CaseID:          strings.TrimSpace(firstQueryValue(query.Get("case"), query.Get("caseId"))),
		Agent:           strings.TrimSpace(query.Get("agent")),
		TemplateVariant: strings.TrimSpace(firstQueryValue(query.Get("templateVariant"), query.Get("variant"))),
	}
}

func firstQueryValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
