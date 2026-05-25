package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/T4cceptor/centian/internal/agentrunner"
	"github.com/T4cceptor/centian/internal/persistence"
)

type demoRunStarter interface {
	startSyntheticDemoRun(ctx context.Context, demoID string) (*agentrunner.SyntheticDemoRun, error)
}

type storeDemoRunStarter struct {
	store *persistence.Store
}

func (s storeDemoRunStarter) startSyntheticDemoRun(ctx context.Context, demoID string) (*agentrunner.SyntheticDemoRun, error) {
	return agentrunner.StartSyntheticDemoRun(ctx, s.store, demoID)
}

// DemoHandler serves UI-playable synthetic demo APIs.
type DemoHandler struct {
	starter demoRunStarter
}

// NewDemoHandler builds a synthetic demo API handler bound to a persistence store.
func NewDemoHandler(store *persistence.Store) *DemoHandler {
	return &DemoHandler{starter: storeDemoRunStarter{store: store}}
}

func newDemoHandlerWithStarter(starter demoRunStarter) *DemoHandler {
	return &DemoHandler{starter: starter}
}

// RegisterRoutesWithMiddleware registers synthetic demo API routes and wraps each
// endpoint with the provided middleware when present.
func (h *DemoHandler) RegisterRoutesWithMiddleware(mux *http.ServeMux, middleware func(http.Handler) http.Handler) {
	if h == nil || h.starter == nil || mux == nil {
		return
	}

	register := func(pattern string, handler http.HandlerFunc) {
		var wrapped http.Handler = handler
		if middleware != nil {
			wrapped = middleware(wrapped)
		}
		mux.Handle(pattern, wrapped)
	}

	register("GET /api/demos", h.handleListDemos)
	register("POST /api/demos/{demoID}/runs", h.handleStartDemoRun)
}

func (h *DemoHandler) handleListDemos(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, agentrunner.ListSyntheticDemos())
}

func (h *DemoHandler) handleStartDemoRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.starter.startSyntheticDemoRun(r.Context(), r.PathValue("demoID"))
	if err != nil {
		if errors.Is(err, agentrunner.ErrSyntheticDemoNotFound) {
			writeError(w, http.StatusNotFound, "demo not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to start demo")
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}
