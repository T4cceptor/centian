package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/T4cceptor/centian/internal/agentrunner"
	"github.com/T4cceptor/centian/internal/identifiers"
	"gotest.tools/assert"
)

type stubDemoStarter struct {
	run *agentrunner.SyntheticDemoRun
	err error
	id  string
}

func (s *stubDemoStarter) StartSyntheticDemoRun(_ context.Context, demoID string) (*agentrunner.SyntheticDemoRun, error) {
	s.id = demoID
	if s.err != nil {
		return nil, s.err
	}
	return s.run, nil
}

func TestDemoHandler_ListDemos(t *testing.T) {
	handler := newDemoHandlerWithStarter(&stubDemoStarter{})
	mux := http.NewServeMux()
	handler.RegisterRoutesWithMiddleware(mux, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/demos", http.NoBody)
	mux.ServeHTTP(rec, req)

	assert.Equal(t, rec.Code, http.StatusOK)
	var demos []agentrunner.SyntheticDemoDefinition
	assert.NilError(t, json.Unmarshal(rec.Body.Bytes(), &demos))
	assert.Assert(t, len(demos) >= 1)
	assert.Equal(t, demos[0].ID, "it_ops")
}

func TestDemoHandler_StartDemoRun(t *testing.T) {
	runID := identifiers.New(identifiers.KindTaskRun)
	starter := &stubDemoStarter{run: &agentrunner.SyntheticDemoRun{RunID: runID, DemoID: "it_ops", DurationMS: 72_000}}
	handler := newDemoHandlerWithStarter(starter)
	mux := http.NewServeMux()
	handler.RegisterRoutesWithMiddleware(mux, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/demos/it_ops/runs", http.NoBody)
	mux.ServeHTTP(rec, req)

	assert.Equal(t, rec.Code, http.StatusAccepted)
	assert.Equal(t, starter.id, "it_ops")
	var response agentrunner.SyntheticDemoRun
	assert.NilError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, response.RunID, runID)
	assert.Equal(t, response.DemoID, "it_ops")
	assert.Equal(t, response.DurationMS, 72_000)
}

func TestDemoHandler_StartUnknownDemo(t *testing.T) {
	starter := &stubDemoStarter{err: fmt.Errorf("%w: missing", agentrunner.ErrSyntheticDemoNotFound)}
	handler := newDemoHandlerWithStarter(starter)
	mux := http.NewServeMux()
	handler.RegisterRoutesWithMiddleware(mux, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/demos/missing/runs", http.NoBody)
	mux.ServeHTTP(rec, req)

	assert.Equal(t, rec.Code, http.StatusNotFound)
}
