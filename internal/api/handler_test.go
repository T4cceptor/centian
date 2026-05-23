package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/identifiers"
	"github.com/T4cceptor/centian/internal/persistence"
	"github.com/T4cceptor/centian/internal/taskruns"
	"gotest.tools/assert"
)

type stubStore struct {
	listTaskRunsFn            func(context.Context, persistence.TaskRunFilter) ([]persistence.TaskRunSummary, error)
	getTaskRunFn              func(context.Context, string) (*persistence.TaskRunSummary, error)
	getTaskRunSnapshotFn      func(context.Context, string) (*persistence.TaskRunSnapshotRecord, error)
	listTaskRunBenchmarkLinks func(context.Context, string) ([]persistence.TaskRunBenchmarkLink, error)
	getTaskRunEventsFn        func(context.Context, string) ([]persistence.TaskRunEvent, error)
	getTaskRunCalls           int
}

func (s *stubStore) ListTaskRuns(ctx context.Context, filter persistence.TaskRunFilter) ([]persistence.TaskRunSummary, error) {
	if s.listTaskRunsFn == nil {
		return nil, nil
	}
	return s.listTaskRunsFn(ctx, filter)
}

func (s *stubStore) GetTaskRun(ctx context.Context, runID string) (*persistence.TaskRunSummary, error) {
	if s.getTaskRunFn == nil {
		return nil, nil
	}
	return s.getTaskRunFn(ctx, runID)
}

func (s *stubStore) GetTaskRunSnapshot(ctx context.Context, runID string) (*persistence.TaskRunSnapshotRecord, error) {
	if s.getTaskRunSnapshotFn == nil {
		return nil, sql.ErrNoRows
	}
	return s.getTaskRunSnapshotFn(ctx, runID)
}

func (s *stubStore) ListTaskRunBenchmarkLinks(ctx context.Context, runID string) ([]persistence.TaskRunBenchmarkLink, error) {
	if s.listTaskRunBenchmarkLinks == nil {
		return nil, nil
	}
	return s.listTaskRunBenchmarkLinks(ctx, runID)
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
			listTaskRunsFn: func(context.Context, persistence.TaskRunFilter) ([]persistence.TaskRunSummary, error) {
				return []persistence.TaskRunSummary{{
					RunID:        identifiers.New(identifiers.KindTaskRun),
					TemplateID:   "template-a",
					TemplateName: "Template A",
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
		assert.Equal(t, summaries[0].TemplateName, "Template A")
	})

	t.Run("returns json error on store failure", func(t *testing.T) {
		// Given: a handler backed by a failing store
		handler := NewHandler(&stubStore{
			listTaskRunsFn: func(context.Context, persistence.TaskRunFilter) ([]persistence.TaskRunSummary, error) {
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

	t.Run("passes benchmark suite filters through query params", func(t *testing.T) {
		var received persistence.TaskRunFilter
		handler := NewHandler(&stubStore{
			listTaskRunsFn: func(_ context.Context, filter persistence.TaskRunFilter) ([]persistence.TaskRunSummary, error) {
				received = filter
				return []persistence.TaskRunSummary{}, nil
			},
		})
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		req := httptest.NewRequest(http.MethodGet, "/api/task-runs?benchmarkSuite=simple_tdd_v1", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)
		assert.Equal(t, received.BenchmarkSuiteID, "simple_tdd_v1")
	})
}

func TestHandler_GetTaskRun(t *testing.T) {
	validRunID := identifiers.New(identifiers.KindTaskRun)

	t.Run("returns detail metadata as json", func(t *testing.T) {
		handler := NewHandler(&stubStore{
			getTaskRunFn: func(context.Context, string) (*persistence.TaskRunSummary, error) {
				return &persistence.TaskRunSummary{RunID: validRunID, TemplateID: "template-a"}, nil
			},
			listTaskRunBenchmarkLinks: func(context.Context, string) ([]persistence.TaskRunBenchmarkLink, error) {
				return []persistence.TaskRunBenchmarkLink{{
					BenchmarkRunID:  "bm_run_1",
					SuiteID:         "simple_tdd_v1",
					SuiteName:       "Simple TDD",
					CaseID:          "compile_failure_red",
					Agent:           "codex",
					TemplateVariant: "current",
					Attempt:         1,
				}}, nil
			},
		})
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		req := httptest.NewRequest(http.MethodGet, "/api/task-runs/"+validRunID, http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)
		var detail persistence.TaskRunDetailMetadata
		err := json.Unmarshal(rec.Body.Bytes(), &detail)
		assert.NilError(t, err)
		assert.Equal(t, detail.RunID, validRunID)
		assert.Assert(t, detail.Summary != nil)
		assert.Equal(t, detail.Summary.TemplateID, "template-a")
		assert.Assert(t, detail.Snapshot == nil)
		assert.Equal(t, len(detail.BenchmarkLinks), 1)
		assert.Equal(t, detail.BenchmarkLinks[0].BenchmarkRunID, "bm_run_1")
	})

	t.Run("returns persisted snapshot when available", func(t *testing.T) {
		handler := NewHandler(&stubStore{
			getTaskRunFn: func(context.Context, string) (*persistence.TaskRunSummary, error) {
				return &persistence.TaskRunSummary{RunID: validRunID, TemplateID: "template-a"}, nil
			},
			getTaskRunSnapshotFn: func(context.Context, string) (*persistence.TaskRunSnapshotRecord, error) {
				return &persistence.TaskRunSnapshotRecord{
					RunID: validRunID,
					Payload: &taskruns.PersistedRunSnapshot{
						RunID:           validRunID,
						TemplateID:      "template-a",
						TemplateName:    "Template A",
						TaskDescription: "Resolve a production incident",
						Status:          "active",
						Phase:           "execution.step_one",
						SelectedTemplate: taskruns.PersistedTemplateSnapshot{
							Version: "0.1",
							Task: taskruns.PersistedTaskSnapshot{
								ID:          "template-a",
								Name:        "Template A",
								Description: "Template description",
							},
						},
						RunnableTemplate: &taskruns.PersistedTemplateSnapshot{
							Version: "0.1",
							Task:    taskruns.PersistedTaskSnapshot{ID: "template-a", Name: "Template A"},
							CompiledWorkflow: &taskruns.PersistedCompiledWorkflowSnapshot{
								WorkflowSteps: []taskruns.PersistedCompiledStepSnapshot{{
									ID:   "step_one",
									Path: "execution.step_one",
								}},
							},
						},
					},
				}, nil
			},
		})
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		req := httptest.NewRequest(http.MethodGet, "/api/task-runs/"+validRunID, http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)
		var detail persistence.TaskRunDetailMetadata
		err := json.Unmarshal(rec.Body.Bytes(), &detail)
		assert.NilError(t, err)
		assert.Assert(t, detail.Summary != nil)
		assert.Equal(t, detail.Summary.RunID, validRunID)
		assert.Assert(t, detail.Snapshot != nil)
		assert.Equal(t, detail.Snapshot.TaskDescription, "Resolve a production incident")
		assert.Equal(t, detail.Snapshot.SelectedTemplate.Task.Name, "Template A")
		assert.Assert(t, detail.Snapshot.RunnableTemplate != nil)
		assert.Equal(t, detail.Snapshot.RunnableTemplate.CompiledWorkflow.WorkflowSteps[0].Path, "execution.step_one")
	})

	t.Run("rejects invalid run ids before store lookup", func(t *testing.T) {
		store := &stubStore{}
		handler := NewHandler(store)
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		req := httptest.NewRequest(http.MethodGet, "/api/task-runs/not-a-run-id", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusBadRequest)
	})

	t.Run("returns not found for unknown run ids", func(t *testing.T) {
		handler := NewHandler(&stubStore{
			getTaskRunFn: func(context.Context, string) (*persistence.TaskRunSummary, error) {
				return nil, sql.ErrNoRows
			},
		})
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		req := httptest.NewRequest(http.MethodGet, "/api/task-runs/"+validRunID, http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusNotFound)
	})

	t.Run("returns json error on snapshot lookup failure", func(t *testing.T) {
		handler := NewHandler(&stubStore{
			getTaskRunFn: func(context.Context, string) (*persistence.TaskRunSummary, error) {
				return &persistence.TaskRunSummary{RunID: validRunID, TemplateID: "template-a"}, nil
			},
			getTaskRunSnapshotFn: func(context.Context, string) (*persistence.TaskRunSnapshotRecord, error) {
				return nil, errors.New("snapshot unavailable")
			},
		})
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		req := httptest.NewRequest(http.MethodGet, "/api/task-runs/"+validRunID, http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusInternalServerError)
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
				}, {
					Source:             persistence.TaskRunEventSourceAction,
					ID:                 identifiers.New(identifiers.KindActionEvent),
					CreatedAtUnixMilli: 1235,
					RequestID:          identifiers.New(identifiers.KindRequest),
					Annotations: []common.EventAnnotation{{
						Processor: "prompt_injection_guard",
						Action:    "redacted",
						Severity:  "high",
					}},
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
		assert.Equal(t, len(events), 2)
		assert.Equal(t, events[0].EventType, "task_started")
		assert.Equal(t, events[1].Annotations[0].Processor, "prompt_injection_guard")
		assert.Equal(t, events[1].Annotations[0].Action, "redacted")
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

	t.Run("returns an empty timeline for known snapshot-only runs", func(t *testing.T) {
		// Given: a handler backed by a store that knows the run but has no lifecycle events yet
		handler := NewHandler(&stubStore{
			getTaskRunEventsFn: func(context.Context, string) ([]persistence.TaskRunEvent, error) {
				return []persistence.TaskRunEvent{}, nil
			},
			getTaskRunFn: func(context.Context, string) (*persistence.TaskRunSummary, error) {
				return &persistence.TaskRunSummary{RunID: validRunID, TemplateID: "it_incident_resolution"}, nil
			},
		})
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		// When: the event timeline endpoint is requested before the first event arrives
		req := httptest.NewRequest(http.MethodGet, "/api/task-runs/"+validRunID+"/events", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// Then: the handler returns an empty JSON array instead of treating the run as missing
		assert.Equal(t, rec.Code, http.StatusOK)
		var events []persistence.TaskRunEvent
		err := json.Unmarshal(rec.Body.Bytes(), &events)
		assert.NilError(t, err)
		assert.Equal(t, len(events), 0)
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
