package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/T4cceptor/centian/internal/identifiers"
	"github.com/T4cceptor/centian/internal/persistence"
)

// Store provides the persistence-backed projections required by the API.
type Store interface {
	ListTaskRuns(ctx context.Context) ([]persistence.TaskRunSummary, error)
	GetTaskRunEvents(ctx context.Context, runID string) ([]persistence.TaskRunEvent, error)
}

// Handler serves the read-only task run API.
type Handler struct {
	store Store
}

type errorResponse struct {
	Error string `json:"error"`
}

// NewHandler builds an API handler bound to a persistence store.
func NewHandler(store Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes registers the task run API routes on the provided mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	h.RegisterRoutesWithMiddleware(mux, nil)
}

// RegisterRoutesWithMiddleware registers the task run API routes and wraps each
// endpoint with the provided middleware when present.
func (h *Handler) RegisterRoutesWithMiddleware(mux *http.ServeMux, middleware func(http.Handler) http.Handler) {
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

	register("GET /api/task-runs", h.handleListTaskRuns)
	register("GET /api/task-runs/{runID}/events", h.handleGetTaskRunEvents)
}

func (h *Handler) handleListTaskRuns(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.store.ListTaskRuns(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list task runs")
		return
	}
	if summaries == nil {
		summaries = []persistence.TaskRunSummary{}
	}
	writeJSON(w, http.StatusOK, summaries)
}

func (h *Handler) handleGetTaskRunEvents(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	if !identifiers.IsKind(runID, identifiers.KindTaskRun) {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}

	events, err := h.store.GetTaskRunEvents(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get task run events")
		return
	}
	if len(events) == 0 {
		writeError(w, http.StatusNotFound, "task run not found")
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
