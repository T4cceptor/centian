package realworld

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var fetchManifest = &serverManifest{
	Name:           "fetch",
	GatewayID:      "fetch",
	ServerID:       "fetch",
	CommandEnvVar:  "CENTIAN_FETCH_SERVER_CMD",
	ArgsEnvVar:     "CENTIAN_FETCH_SERVER_ARGS",
	DefaultCommand: "uvx",
	DefaultArgs:    []string{"mcp-server-fetch"},
	ExpectedTools:  []string{"fetch"},
	BuildFixture:   buildFetchFixture,
}

func TestFetchToolCatalogParity(t *testing.T) {
	runServerComparison(t, fetchManifest, "tool_catalog", func(ctx context.Context, t *testing.T, pair *connectionPair, _ *fixtureBundle) {
		assertToolCatalogParity(ctx, t, fetchManifest, pair)
	})
}

func TestFetchMarkdownParity(t *testing.T) {
	runServerComparison(t, fetchManifest, "markdown_html", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		assertToolCallParity(
			ctx,
			t,
			fetchManifest,
			fixture,
			pair,
			"fetch",
			map[string]any{"url": fixture.Shared["html_url"]},
			map[string]any{"url": fixture.Shared["html_url"]},
		)
	})
}

func TestFetchRawParity(t *testing.T) {
	runServerComparison(t, fetchManifest, "raw_text", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		assertToolCallParity(
			ctx,
			t,
			fetchManifest,
			fixture,
			pair,
			"fetch",
			map[string]any{"url": fixture.Shared["raw_url"], "raw": true},
			map[string]any{"url": fixture.Shared["raw_url"], "raw": true},
		)
	})
}

func TestFetchChunkedParity(t *testing.T) {
	runServerComparison(t, fetchManifest, "chunked", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		assertToolCallParity(
			ctx,
			t,
			fetchManifest,
			fixture,
			pair,
			"fetch",
			map[string]any{"url": fixture.Shared["long_url"], "start_index": 40, "max_length": 80},
			map[string]any{"url": fixture.Shared["long_url"], "start_index": 40, "max_length": 80},
		)
	})
}

func TestFetchErrorParity(t *testing.T) {
	runServerComparison(t, fetchManifest, "missing", func(ctx context.Context, t *testing.T, pair *connectionPair, fixture *fixtureBundle) {
		assertToolCallParity(
			ctx,
			t,
			fetchManifest,
			fixture,
			pair,
			"fetch",
			map[string]any{"url": fixture.Shared["missing_url"]},
			map[string]any{"url": fixture.Shared["missing_url"]},
		)
	})
}

func buildFetchFixture(t *testing.T) *fixtureBundle {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/page.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><body><h1>Centian Fetch Fixture</h1><p>This is deterministic HTML.</p></body></html>")
	})
	mux.HandleFunc("/raw.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "raw fixture line 1\nraw fixture line 2\n")
	})
	mux.HandleFunc("/long.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<html><body><p>%s</p></body></html>", strings.Repeat("chunked-content-", 80))
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return &fixtureBundle{
		Shared: map[string]string{
			"html_url":    server.URL + "/page.html",
			"raw_url":     server.URL + "/raw.txt",
			"long_url":    server.URL + "/long.html",
			"missing_url": server.URL + "/missing",
		},
	}
}
