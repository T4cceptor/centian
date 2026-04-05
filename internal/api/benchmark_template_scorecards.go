package api

import (
	"net/http"

	"github.com/T4cceptor/centian/internal/benchmarks"
)

func (h *BenchmarkHandler) handleListTemplateScorecards(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListTemplateScorecards(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list template scorecards")
		return
	}
	if items == nil {
		items = []benchmarks.TemplateScorecard{}
	}
	writeJSON(w, http.StatusOK, items)
}
