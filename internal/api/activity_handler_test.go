package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/T4cceptor/centian/internal/persistence"
	"gotest.tools/assert"
)

type stubActivityStore struct {
	summaryFn func(context.Context, *persistence.ActivityFilter) (*persistence.ActivitySummary, error)
	calls     int
	filter    *persistence.ActivityFilter
}

func (s *stubActivityStore) ActivitySummary(ctx context.Context, filter *persistence.ActivityFilter) (*persistence.ActivitySummary, error) {
	s.calls++
	s.filter = filter
	if s.summaryFn == nil {
		return nil, nil
	}
	return s.summaryFn(ctx, filter)
}

func TestActivityHandler_Activity(t *testing.T) {
	t.Run("returns empty slices instead of null", func(t *testing.T) {
		handler := NewActivityHandler(&stubActivityStore{})
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/activity", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)

		var summary persistence.ActivitySummary
		assert.NilError(t, json.Unmarshal(rec.Body.Bytes(), &summary))
		assert.Assert(t, summary.Interventions != nil)
		assert.Assert(t, summary.Volume != nil)
	})

	t.Run("translates range into a window ending now", func(t *testing.T) {
		store := &stubActivityStore{
			summaryFn: func(_ context.Context, filter *persistence.ActivityFilter) (*persistence.ActivitySummary, error) {
				return &persistence.ActivitySummary{
					RangeStartUnixMilli: filter.Start,
					RangeEndUnixMilli:   filter.End,
				}, nil
			},
		}
		handler := NewActivityHandler(store)
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/activity?range=1h", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)
		assert.Equal(t, store.calls, 1)
		// A 1h range spans exactly one hour.
		assert.Equal(t, store.filter.End-store.filter.Start, int64(60*60*1000))
	})

	t.Run("uses an explicit start/end window over range", func(t *testing.T) {
		store := &stubActivityStore{
			summaryFn: func(_ context.Context, filter *persistence.ActivityFilter) (*persistence.ActivitySummary, error) {
				return &persistence.ActivitySummary{
					RangeStartUnixMilli: filter.Start,
					RangeEndUnixMilli:   filter.End,
				}, nil
			},
		}
		handler := NewActivityHandler(store)
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/activity?start=1000&end=5000&range=1h", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)
		assert.Equal(t, store.filter.Start, int64(1000))
		assert.Equal(t, store.filter.End, int64(5000))
	})

	t.Run("rejects a custom window where end precedes start", func(t *testing.T) {
		store := &stubActivityStore{}
		handler := NewActivityHandler(store)
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/activity?start=5000&end=1000", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusBadRequest)
		assert.Equal(t, store.calls, 0)
	})

	t.Run("rejects a non-numeric start", func(t *testing.T) {
		store := &stubActivityStore{}
		handler := NewActivityHandler(store)
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/activity?start=abc&end=5000", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusBadRequest)
		assert.Equal(t, store.calls, 0)
	})

	t.Run("rejects an invalid range", func(t *testing.T) {
		store := &stubActivityStore{}
		handler := NewActivityHandler(store)
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/activity?range=bogus", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusBadRequest)
		assert.Equal(t, store.calls, 0)
	})
}
