package proxy

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/CentianAI/centian-cli/internal/auth"
	"gotest.tools/assert"
)

func TestWriteUnauthorized(t *testing.T) {
	// Given: a response recorder
	recorder := httptest.NewRecorder()

	// When: writing unauthorized for Authorization header
	writeUnauthorized(recorder, "Authorization")

	// Then: status and headers are set
	result := recorder.Result()
	assert.Equal(t, result.StatusCode, http.StatusUnauthorized)
	assert.Equal(t, result.Header.Get("WWW-Authenticate"), "Bearer")
	assert.Equal(t, result.Header.Get("Content-Type"), "application/json")
}

func TestWriteUnauthorized_CustomHeader(t *testing.T) {
	// Given: a response recorder
	recorder := httptest.NewRecorder()

	// When: writing unauthorized for custom header
	writeUnauthorized(recorder, "X-API-Key")

	// Then: www-authenticate is not set
	result := recorder.Result()
	assert.Equal(t, result.StatusCode, http.StatusUnauthorized)
	assert.Equal(t, result.Header.Get("WWW-Authenticate"), "")
}

func TestAPIKeyMiddlewareWithHeader_NoStore(t *testing.T) {
	// Given: a handler and nil API key store
	called := false
	handler := apiKeyMiddlewareWithHeader(nil, "Authorization", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// When: calling the handler
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	handler.ServeHTTP(recorder, request)

	// Then: request passes through
	assert.Assert(t, called)
	assert.Equal(t, recorder.Result().StatusCode, http.StatusOK)
}

func TestAPIKeyMiddlewareWithHeader_WithStore(t *testing.T) {
	// Given: an API key store with one key
	store := createTestAPIKeyStore(t, "plain-key")

	called := false
	handler := apiKeyMiddlewareWithHeader(store, "Authorization", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// When: request is missing token
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	handler.ServeHTTP(recorder, request)

	// Then: unauthorized and handler not called
	assert.Equal(t, recorder.Result().StatusCode, http.StatusUnauthorized)
	assert.Assert(t, !called)

	// When: request has invalid token
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	request.Header.Set("Authorization", "Bearer bad")
	handler.ServeHTTP(recorder, request)

	// Then: unauthorized and handler not called
	assert.Equal(t, recorder.Result().StatusCode, http.StatusUnauthorized)
	assert.Assert(t, !called)

	// When: request has valid token
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	request.Header.Set("Authorization", "Bearer plain-key")
	handler.ServeHTTP(recorder, request)

	// Then: handler is called
	assert.Equal(t, recorder.Result().StatusCode, http.StatusOK)
	assert.Assert(t, called)
}

func TestRegisterHandler_WithAuthMiddleware(t *testing.T) {
	// Given: a proxy with API key auth
	store := createTestAPIKeyStore(t, "plain-key")
	proxy := &MCPProxy{
		name:     "gateway",
		endpoint: "/mcp/gateway",
		server: &CentianProxy{
			APIKeys:    store,
			AuthHeader: "Authorization",
		},
	}
	mux := http.NewServeMux()

	// When: registering handler and calling without auth
	RegisterHandler("/mcp/gateway", proxy, mux, nil)
	request := httptest.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	recorder := httptest.NewRecorder()
	handler, _ := mux.Handler(request)
	handler.ServeHTTP(recorder, request)

	// Then: unauthorized response is returned
	assert.Equal(t, recorder.Result().StatusCode, http.StatusUnauthorized)
}

func createTestAPIKeyStore(t *testing.T, plain string) *auth.APIKeyStore {
	t.Helper()
	entry, err := auth.NewAPIKeyEntry(plain)
	assert.NilError(t, err)
	path := filepath.Join(t.TempDir(), "api_keys.json")
	assert.NilError(t, auth.WriteAPIKeyFile(path, &auth.APIKeyFile{Keys: []auth.APIKeyEntry{entry}}))
	store, err := auth.LoadAPIKeys(path)
	assert.NilError(t, err)
	return store
}
