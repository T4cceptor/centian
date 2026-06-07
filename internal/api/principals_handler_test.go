package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gotest.tools/assert"
)

type stubPrincipalStore struct {
	ids   []string
	start int64
	end   int64
}

func (s *stubPrincipalStore) DistinctPrincipals(_ context.Context, start, end int64) ([]string, error) {
	s.start = start
	s.end = end
	return s.ids, nil
}

type stubNamer struct {
	names map[string]string
}

func (n stubNamer) PrincipalNames(_ context.Context) (map[string]string, error) {
	return n.names, nil
}

func TestPrincipalsHandler(t *testing.T) {
	t.Run("labels ids with display names and falls back to the id", func(t *testing.T) {
		store := &stubPrincipalStore{ids: []string{"pr_alice", "demo-user"}}
		handler := NewPrincipalsHandler(store, stubNamer{names: map[string]string{"pr_alice": "Alice"}})
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/principals?start=1000&end=2000", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)
		assert.Equal(t, store.start, int64(1000))
		assert.Equal(t, store.end, int64(2000))

		var resp principalsResponse
		assert.NilError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, len(resp.Principals), 2)
		assert.Equal(t, resp.Principals[0], PrincipalRef{ID: "pr_alice", DisplayName: "Alice"})
		assert.Equal(t, resp.Principals[1], PrincipalRef{ID: "demo-user", DisplayName: "demo-user"})
	})

	t.Run("works without a namer and without a window", func(t *testing.T) {
		store := &stubPrincipalStore{ids: []string{"demo-user"}}
		handler := NewPrincipalsHandler(store, nil)
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/principals", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)
		assert.Equal(t, store.start, int64(0))
		assert.Equal(t, store.end, int64(0))

		var resp principalsResponse
		assert.NilError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, len(resp.Principals), 1)
		assert.Equal(t, resp.Principals[0].DisplayName, "demo-user")
	})

	t.Run("rejects a non-numeric start", func(t *testing.T) {
		handler := NewPrincipalsHandler(&stubPrincipalStore{}, nil)
		mux := http.NewServeMux()
		handler.RegisterRoutesWithMiddleware(mux, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/principals?start=abc", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusBadRequest)
	})
}
