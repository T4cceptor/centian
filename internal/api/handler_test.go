package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/T4cceptor/centian/internal/identifiers"
	"github.com/T4cceptor/centian/internal/persistence"
	"gotest.tools/assert"
)

type stubStore struct {
	listTaskRunsFn     func(context.Context) ([]persistence.TaskRunSummary, error)
	getTaskRunEventsFn func(context.Context, string) ([]persistence.TaskRunEvent, error)
	getTaskRunCalls    int
}

func (s *stubStore) ListTaskRuns(ctx context.Context) ([]persistence.TaskRunSummary, error) {
	if s.listTaskRunsFn == nil {
		return nil, nil
	}
	return s.listTaskRunsFn(ctx)
}

func (s *stubStore) GetTaskRunEvents(ctx context.Context, runID string) ([]persistence.TaskRunEvent, error) {
	s.getTaskRunCalls++
	if s.getTaskRunEventsFn == nil {
		return nil, nil
	}
	return s.getTaskRunEventsFn(ctx, runID)
}

func TestHandler_ListTaskRuns(t *testing.T) {
	t.Run("returns summaries as json", func(t *testing.T) {
		// Given: a handler backed by a store returning task run summaries
		handler := NewHandler(&stubStore{
			listTaskRunsFn: func(context.Context) ([]persistence.TaskRunSummary, error) {
				return []persistence.TaskRunSummary{{
					RunID:        identifiers.New(identifiers.KindTaskRun),
					TemplateID:   "template-a",
					StartedAt:    1234,
					Status:       "succeeded",
					CurrentPhase: "planning",
					EventCount:   2,
				}}, nil
			},
		})
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		// When: the list endpoint is requested
		req := httptest.NewRequest(http.MethodGet, "/api/task-runs", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// Then: the response is a JSON array with status 200
		assert.Equal(t, rec.Code, http.StatusOK)
		assert.Equal(t, rec.Header().Get("Content-Type"), "application/json")

		var summaries []persistence.TaskRunSummary
		err := json.Unmarshal(rec.Body.Bytes(), &summaries)
		assert.NilError(t, err)
		assert.Equal(t, len(summaries), 1)
		assert.Equal(t, summaries[0].TemplateID, "template-a")
	})

	t.Run("returns json error on store failure", func(t *testing.T) {
		// Given: a handler backed by a failing store
		handler := NewHandler(&stubStore{
			listTaskRunsFn: func(context.Context) ([]persistence.TaskRunSummary, error) {
				return nil, errors.New("boom")
			},
		})
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		// When: the list endpoint is requested
		req := httptest.NewRequest(http.MethodGet, "/api/task-runs", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// Then: the handler returns a JSON 500 response
		assert.Equal(t, rec.Code, http.StatusInternalServerError)
		assert.Equal(t, rec.Header().Get("Content-Type"), "application/json")

		var response errorResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NilError(t, err)
		assert.Equal(t, response.Error, "failed to list task runs")
	})
}

func TestHandler_GetTaskRunEvents(t *testing.T) {
	validRunID := identifiers.New(identifiers.KindTaskRun)

	t.Run("returns events as json", func(t *testing.T) {
		// Given: a handler backed by a store returning task run events
		handler := NewHandler(&stubStore{
			getTaskRunEventsFn: func(context.Context, string) ([]persistence.TaskRunEvent, error) {
				return []persistence.TaskRunEvent{{
					Source:             persistence.TaskRunEventSourceTask,
					ID:                 identifiers.New(identifiers.KindTaskEvent),
					CreatedAtUnixMilli: 1234,
					EventType:          "task_started",
				}}, nil
			},
		})
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		// When: the event timeline endpoint is requested
		req := httptest.NewRequest(http.MethodGet, "/api/task-runs/"+validRunID+"/events", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// Then: the response is a JSON array with status 200
		assert.Equal(t, rec.Code, http.StatusOK)
		assert.Equal(t, rec.Header().Get("Content-Type"), "application/json")

		var events []persistence.TaskRunEvent
		err := json.Unmarshal(rec.Body.Bytes(), &events)
		assert.NilError(t, err)
		assert.Equal(t, len(events), 1)
		assert.Equal(t, events[0].EventType, "task_started")
	})

	t.Run("rejects invalid run ids before store lookup", func(t *testing.T) {
		// Given: a handler with a store spy
		store := &stubStore{}
		handler := NewHandler(store)
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		// When: the event timeline endpoint is requested with a malformed run id
		req := httptest.NewRequest(http.MethodGet, "/api/task-runs/not-a-run-id/events", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// Then: the request is rejected as a JSON 400 without hitting the store
		assert.Equal(t, rec.Code, http.StatusBadRequest)
		assert.Equal(t, store.getTaskRunCalls, 0)
	})

	t.Run("returns not found for valid but unknown run ids", func(t *testing.T) {
		// Given: a handler backed by a store returning no events
		handler := NewHandler(&stubStore{
			getTaskRunEventsFn: func(context.Context, string) ([]persistence.TaskRunEvent, error) {
				return []persistence.TaskRunEvent{}, nil
			},
		})
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		// When: the event timeline endpoint is requested for an unknown run
		req := httptest.NewRequest(http.MethodGet, "/api/task-runs/"+validRunID+"/events", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// Then: the handler returns a JSON 404 response
		assert.Equal(t, rec.Code, http.StatusNotFound)
		assert.Equal(t, rec.Header().Get("Content-Type"), "application/json")
	})

	t.Run("returns json error on store failure", func(t *testing.T) {
		// Given: a handler backed by a failing store
		handler := NewHandler(&stubStore{
			getTaskRunEventsFn: func(context.Context, string) ([]persistence.TaskRunEvent, error) {
				return nil, errors.New("boom")
			},
		})
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		// When: the event timeline endpoint is requested
		req := httptest.NewRequest(http.MethodGet, "/api/task-runs/"+validRunID+"/events", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// Then: the handler returns a JSON 500 response
		assert.Equal(t, rec.Code, http.StatusInternalServerError)
		assert.Equal(t, rec.Header().Get("Content-Type"), "application/json")

		var response errorResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NilError(t, err)
		assert.Equal(t, response.Error, "failed to get task run events")
	})
}
