package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/persistence"
)

// ActivityStore provides the read method required by the activity API.
type ActivityStore interface {
	ActivitySummary(context.Context, *persistence.ActivityFilter) (*persistence.ActivitySummary, error)
}

// ActivityHandler serves the read-only activity ("Intervention Skyline") API.
type ActivityHandler struct {
	store ActivityStore
	now   func() time.Time
}

// NewActivityHandler builds an activity API handler.
func NewActivityHandler(store ActivityStore) *ActivityHandler {
	return &ActivityHandler{store: store, now: time.Now}
}

// RegisterRoutesWithMiddleware registers activity routes under the default /api prefix.
func (h *ActivityHandler) RegisterRoutesWithMiddleware(mux *http.ServeMux, middleware func(http.Handler) http.Handler) {
	h.RegisterRoutesWithPrefix(mux, "/api", middleware)
}

// RegisterRoutesWithPrefix registers activity routes under prefix.
func (h *ActivityHandler) RegisterRoutesWithPrefix(mux *http.ServeMux, prefix string, middleware func(http.Handler) http.Handler) {
	if h == nil || h.store == nil || mux == nil {
		return
	}
	prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
	if prefix == "" {
		prefix = "/api"
	}

	register := func(pattern string, handler http.HandlerFunc) {
		var wrapped http.Handler = handler
		if middleware != nil {
			wrapped = middleware(wrapped)
		}
		mux.Handle(pattern, wrapped)
	}

	register(fmt.Sprintf("GET %s/activity", prefix), h.handleActivity)
}

func (h *ActivityHandler) handleActivity(w http.ResponseWriter, r *http.Request) {
	window, err := activityWindowFromQuery(r, h.now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	summary, err := h.store.ActivitySummary(r.Context(), window)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load activity")
		return
	}
	if summary == nil {
		summary = &persistence.ActivitySummary{
			RangeStartUnixMilli: window.Start,
			RangeEndUnixMilli:   window.End,
		}
	}
	if summary.Interventions == nil {
		summary.Interventions = []persistence.ActivityIntervention{}
	}
	if summary.Volume == nil {
		summary.Volume = []persistence.ActivityVolumePoint{}
	}
	writeJSON(w, http.StatusOK, summary)
}

// activityWindowFromQuery resolves the requested window. An explicit start/end
// pair (unix milli) takes precedence; otherwise the ?range= toggle is turned into
// an absolute [start,end] window ending now. Defaults to 6h to match the UI default.
func activityWindowFromQuery(r *http.Request, now time.Time) (*persistence.ActivityFilter, error) {
	query := r.URL.Query()
	rawStart := strings.TrimSpace(query.Get("start"))
	rawEnd := strings.TrimSpace(query.Get("end"))
	if rawStart != "" || rawEnd != "" {
		start, err := strconv.ParseInt(rawStart, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid start")
		}
		end, err := strconv.ParseInt(rawEnd, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid end")
		}
		if end <= start {
			return nil, fmt.Errorf("end must be after start")
		}
		return &persistence.ActivityFilter{Start: start, End: end}, nil
	}

	rawRange := strings.TrimSpace(query.Get("range"))
	if rawRange == "" {
		rawRange = "6h"
	}
	var duration time.Duration
	switch rawRange {
	case "1h":
		duration = time.Hour
	case "6h":
		duration = 6 * time.Hour
	case "1d":
		duration = 24 * time.Hour
	case "1w":
		duration = 7 * 24 * time.Hour
	default:
		return nil, fmt.Errorf("invalid range")
	}
	end := now.UnixMilli()
	start := now.Add(-duration).UnixMilli()
	return &persistence.ActivityFilter{Start: start, End: end}, nil
}
