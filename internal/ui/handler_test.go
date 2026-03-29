package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"gotest.tools/assert"
)

func TestHandler_ServesEmbeddedSPA(t *testing.T) {
	handler := newHandler(
		fstest.MapFS{
			"dist/index.html":        {Data: []byte("<!doctype html><html><body>built ui</body></html>")},
			"dist/assets/app.js":     {Data: []byte("console.log('ui');")},
			"dist/assets/app.css":    {Data: []byte("body{color:#fff;}")},
			"dist/assets/logo.svg":   {Data: []byte("<svg></svg>")},
			"dist/assets/chunk.json": {Data: []byte("{\"ok\":true}")},
		},
		[]byte("<!doctype html><html><body>fallback ui</body></html>"),
	)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	t.Run("serves /ui with index html", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ui", http.NoBody)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)
		assert.Assert(t, strings.Contains(rec.Header().Get("Content-Type"), "text/html"))
		assert.Assert(t, strings.Contains(rec.Body.String(), "built ui"))
	})

	t.Run("serves nested client route with spa fallback", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ui/tasks/tr_1742947200123_0000000001", http.NoBody)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)
		assert.Assert(t, strings.Contains(rec.Header().Get("Content-Type"), "text/html"))
		assert.Assert(t, strings.Contains(rec.Body.String(), "built ui"))
	})

	t.Run("serves embedded assets directly", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ui/assets/app.js", http.NoBody)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)
		assert.Assert(t, !strings.Contains(rec.Header().Get("Content-Type"), "text/html"))
		assert.Assert(t, strings.Contains(rec.Body.String(), "console.log"))
	})

	t.Run("falls back for unknown file-like client paths outside assets", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ui/tasks/state.json", http.NoBody)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK)
		assert.Assert(t, strings.Contains(rec.Header().Get("Content-Type"), "text/html"))
		assert.Assert(t, strings.Contains(rec.Body.String(), "built ui"))
	})

	t.Run("returns not found for missing asset files", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ui/assets/missing.js", http.NoBody)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusNotFound)
	})
}

func TestHandler_UsesFallbackIndexWhenBuiltAssetsAreUnavailable(t *testing.T) {
	handler := newHandler(fstest.MapFS{"dist/.keep": {Data: []byte("keep")}}, []byte("<html>fallback ui</html>"))
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/ui", http.NoBody)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, rec.Code, http.StatusOK)
	assert.Assert(t, strings.Contains(rec.Body.String(), "fallback ui"))
}
