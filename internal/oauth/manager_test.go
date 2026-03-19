package oauth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gotest.tools/assert"
)

func TestEnsurePendingReplacesOlderFlowForBinding(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	manager, err := NewManager("http://127.0.0.1:8080", nil, nil)
	assert.NilError(t, err)

	binding := Binding{PrincipalID: "principal-1", Gateway: "gw", Server: "srv"}
	metadata := &ResolvedMetadata{
		Resource:              "https://resource.example/mcp",
		Scopes:                []string{"tool:echo"},
		Issuer:                "https://issuer.example",
		AuthorizationEndpoint: "https://issuer.example/authorize",
		TokenEndpoint:         "https://issuer.example/token",
		ClientAuthMethod:      "client_secret_post",
	}

	first, err := manager.CreatePending(binding, "client-id", "client-secret", metadata, "verifier-1")
	assert.NilError(t, err)
	first.Status = PendingStatusFailed

	second, reused, err := manager.EnsurePending(binding, "client-id", "client-secret", metadata)
	assert.NilError(t, err)
	assert.Assert(t, !reused)
	assert.Assert(t, second != nil)
	assert.Assert(t, second.ID != first.ID)
	assert.Assert(t, manager.pending.getByID(first.ID) == nil)
	assert.Equal(t, len(manager.pending.list()), 1)

	current := manager.PendingForBinding(binding)
	assert.Assert(t, current != nil)
	assert.Equal(t, current.ID, second.ID)
}

func TestManagerURLsTransportAndTokenWrappers(t *testing.T) {
	manager := newTestManager(t, "http://127.0.0.1:8080/")
	binding := Binding{PrincipalID: "principal-1", Gateway: "gw", Server: "srv"}
	token := &StoredToken{
		AccessToken: "access-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
	}

	assert.Equal(t, trimTrailingSlash("http://127.0.0.1:8080/"), "http://127.0.0.1:8080")
	assert.Equal(t, manager.StartURL("abc"), "http://127.0.0.1:8080/oauth/start?id=abc")
	assert.Equal(t, manager.StatusURL("abc"), "http://127.0.0.1:8080/oauth/status?id=abc")

	transport := manager.NewTransport(nil, binding, "https://resource.example/mcp", nil, nil)
	assert.Assert(t, transport != nil)

	assert.NilError(t, manager.SaveToken(binding, token))
	loaded, err := manager.LoadToken(binding)
	assert.NilError(t, err)
	assert.DeepEqual(t, loaded, token)
	assert.NilError(t, manager.DeleteToken(binding))
	loaded, err = manager.LoadToken(binding)
	assert.Assert(t, errors.Is(err, errTokenNotFound))
	assert.Assert(t, loaded == nil)
}

func TestManagerHandleStartAndStatus(t *testing.T) {
	manager := newTestManager(t, "http://127.0.0.1:8080")
	metadata := &ResolvedMetadata{
		Resource:              "https://resource.example/mcp",
		Scopes:                []string{"tool:echo"},
		Issuer:                "https://issuer.example",
		AuthorizationEndpoint: "https://issuer.example/authorize",
		TokenEndpoint:         "https://issuer.example/token",
		ClientAuthMethod:      "client_secret_post",
	}
	pending, err := manager.CreatePending(Binding{PrincipalID: "principal-1", Gateway: "gw", Server: "srv"}, "client-id", "client-secret", metadata, "verifier")
	assert.NilError(t, err)
	authURL, err := url.Parse(pending.AuthURL)
	assert.NilError(t, err)
	assert.Equal(t, authURL.Query().Get("code_challenge_method"), "S256")

	mux := http.NewServeMux()
	manager.RegisterRoutes(mux)

	statusListReq := httptest.NewRequest(http.MethodGet, "/oauth/status", http.NoBody)
	statusListRec := httptest.NewRecorder()
	mux.ServeHTTP(statusListRec, statusListReq)
	assert.Equal(t, statusListRec.Code, http.StatusOK)
	assert.Assert(t, strings.Contains(statusListRec.Body.String(), "Pending Downstream OAuth Flows"))

	startReq := httptest.NewRequest(http.MethodGet, manager.StartURL(pending.ID), http.NoBody)
	startRec := httptest.NewRecorder()
	mux.ServeHTTP(startRec, startReq)
	assert.Equal(t, startRec.Code, http.StatusFound)
	assert.Equal(t, startRec.Header().Get("Location"), pending.AuthURL)
	assert.Equal(t, pending.Status, PendingStatusInProgress)

	statusReq := httptest.NewRequest(http.MethodGet, manager.StatusURL(pending.ID), http.NoBody)
	statusRec := httptest.NewRecorder()
	mux.ServeHTTP(statusRec, statusReq)
	assert.Equal(t, statusRec.Code, http.StatusOK)
	assert.Assert(t, strings.Contains(statusRec.Body.String(), "gw/srv"))
	assert.Assert(t, strings.Contains(statusRec.Body.String(), string(PendingStatusInProgress)))
}

func TestManagerHandleCallbackSuccessAndErrors(t *testing.T) {
	authorized := make(chan Binding, 1)
	t.Setenv("HOME", t.TempDir())
	manager, err := NewManager("http://127.0.0.1:8080", func(binding Binding) {
		authorized <- binding
	}, nil)
	assert.NilError(t, err)
	manager.httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, req.URL.String(), "https://issuer.example/token")
		return newJSONResponse(`{"access_token":"callback-access","token_type":"Bearer","refresh_token":"callback-refresh","expires_in":300}`), nil
	})}

	metadata := &ResolvedMetadata{
		Resource:              "https://resource.example/mcp",
		Scopes:                []string{"tool:echo"},
		Issuer:                "https://issuer.example",
		AuthorizationEndpoint: "https://issuer.example/authorize",
		TokenEndpoint:         "https://issuer.example/token",
		ClientAuthMethod:      "client_secret_post",
	}

	mux := http.NewServeMux()
	manager.RegisterRoutes(mux)

	missingReq := httptest.NewRequest(http.MethodGet, "/oauth/callback?state=missing", http.NoBody)
	missingRec := httptest.NewRecorder()
	mux.ServeHTTP(missingRec, missingReq)
	assert.Equal(t, missingRec.Code, http.StatusNotFound)

	failedPending, err := manager.CreatePending(Binding{PrincipalID: "principal-1", Gateway: "gw", Server: "srv"}, "client-id", "client-secret", metadata, "verifier-1")
	assert.NilError(t, err)
	errorReq := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+failedPending.State+"&error=denied", http.NoBody)
	errorRec := httptest.NewRecorder()
	mux.ServeHTTP(errorRec, errorReq)
	assert.Equal(t, errorRec.Code, http.StatusBadRequest)
	assert.Equal(t, failedPending.Status, PendingStatusFailed)
	assert.Equal(t, failedPending.LastError, "denied")

	successPending, err := manager.CreatePending(Binding{PrincipalID: "principal-1", Gateway: "gw", Server: "srv"}, "client-id", "client-secret", metadata, "verifier-2")
	assert.NilError(t, err)
	successReq := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+successPending.State+"&code=auth-code", http.NoBody)
	successRec := httptest.NewRecorder()
	mux.ServeHTTP(successRec, successReq)
	assert.Equal(t, successRec.Code, http.StatusOK)
	assert.Equal(t, successPending.Status, PendingStatusCompleted)
	assert.Equal(t, successPending.LastError, "")
	assert.Assert(t, strings.Contains(successRec.Body.String(), "Authorization complete"))

	select {
	case binding := <-authorized:
		assert.Equal(t, binding.Gateway, "gw")
		assert.Equal(t, binding.Server, "srv")
	case <-time.After(2 * time.Second):
		t.Fatal("expected onAuthorized callback")
	}

	loaded, err := manager.LoadToken(successPending.Binding)
	assert.NilError(t, err)
	assert.Equal(t, loaded.AccessToken, "callback-access")
	assert.Equal(t, loaded.RefreshToken, "callback-refresh")
}
