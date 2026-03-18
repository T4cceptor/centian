package oauth

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestManager(t *testing.T, publicBaseURL string) *Manager {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	manager, err := NewManager(publicBaseURL, nil, nil)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return manager
}

func newTextResponse(statusCode int, body string) *http.Response {
	resp := &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	resp.Header.Set("Content-Type", "text/plain; charset=utf-8")
	return resp
}

func newJSONResponse(body string) *http.Response {
	resp := newTextResponse(http.StatusOK, body)
	resp.Header.Set("Content-Type", "application/json")
	return resp
}
