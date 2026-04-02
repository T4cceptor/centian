package oauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"gotest.tools/assert"
)

func TestTransportRoundTripInjectsStoredTokenAndPreservesBody(t *testing.T) {
	manager := newTestManager(t, "http://127.0.0.1:9666")
	binding := Binding{PrincipalID: "principal-1", Gateway: "gw", Server: "srv"}
	assert.NilError(t, manager.SaveToken(binding, &StoredToken{
		AccessToken: "access-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
	}))

	var (
		seenAuth   string
		seenHeader string
		seenBody   string
	)
	transport := manager.NewTransport(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		seenAuth = req.Header.Get("Authorization")
		seenHeader = req.Header.Get("X-Test")
		data, err := io.ReadAll(req.Body)
		assert.NilError(t, err)
		seenBody = string(data)
		return newTextResponse(http.StatusOK, "ok"), nil
	}), binding, "https://resource.example/mcp", &config.OAuthConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}, map[string]string{"X-Test": "configured"}).(*Transport)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://resource.example/mcp", strings.NewReader("payload"))
	assert.NilError(t, err)

	resp, err := transport.RoundTrip(req)
	assert.NilError(t, err)
	defer closeBody(resp.Body)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	assert.Equal(t, seenAuth, "Bearer access-token")
	assert.Equal(t, seenHeader, "configured")
	assert.Equal(t, seenBody, "payload")

	originalBody, err := io.ReadAll(req.Body)
	assert.NilError(t, err)
	assert.Equal(t, string(originalBody), "payload")
}

func TestTransportRoundTripRefreshesExpiredStoredToken(t *testing.T) {
	manager := newTestManager(t, "http://127.0.0.1:9666")
	manager.httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, req.URL.String(), "https://issuer.example/token")
		assert.Equal(t, req.Method, http.MethodPost)

		body, err := io.ReadAll(req.Body)
		assert.NilError(t, err)
		payload := string(body)
		assert.Assert(t, strings.Contains(payload, "grant_type=refresh_token"))
		assert.Assert(t, strings.Contains(payload, "refresh_token=refresh-token"))
		return newJSONResponse(`{"access_token":"fresh-access","token_type":"Bearer","refresh_token":"fresh-refresh","expires_in":300}`), nil
	})}

	binding := Binding{PrincipalID: "principal-1", Gateway: "gw", Server: "srv"}
	assert.NilError(t, manager.SaveToken(binding, &StoredToken{
		AccessToken:      "expired-access",
		RefreshToken:     "refresh-token",
		Expiry:           time.Now().Add(-time.Minute),
		TokenEndpoint:    "https://issuer.example/token",
		ClientAuthMethod: "client_secret_post",
		Resource:         "https://resource.example/mcp",
	}))

	var seenAuth string
	transport := manager.NewTransport(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		seenAuth = req.Header.Get("Authorization")
		return newTextResponse(http.StatusOK, "ok"), nil
	}), binding, "https://resource.example/mcp", &config.OAuthConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}, nil).(*Transport)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://resource.example/mcp", http.NoBody)
	assert.NilError(t, err)

	resp, err := transport.RoundTrip(req)
	assert.NilError(t, err)
	defer closeBody(resp.Body)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	assert.Equal(t, seenAuth, "Bearer fresh-access")

	loaded, err := manager.LoadToken(binding)
	assert.NilError(t, err)
	assert.Equal(t, loaded.AccessToken, "fresh-access")
	assert.Equal(t, loaded.RefreshToken, "fresh-refresh")
	assert.Equal(t, loaded.TokenEndpoint, "https://issuer.example/token")
}

func TestTransportRoundTripCreatesPendingAuthorizationOnChallenge(t *testing.T) {
	manager := newTestManager(t, "http://127.0.0.1:9666")
	binding := Binding{PrincipalID: "principal-1", Gateway: "gw", Server: "srv"}

	transport := manager.NewTransport(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return newTextResponse(http.StatusUnauthorized, "unauthorized"), nil
	}), binding, "https://resource.example/mcp", &config.OAuthConfig{
		ClientID:              "client-id",
		ClientSecret:          "client-secret",
		Resource:              "https://resource.example/mcp",
		AuthorizationEndpoint: "https://issuer.example/authorize",
		TokenEndpoint:         "https://issuer.example/token",
		ClientAuthMethod:      "client_secret_post",
	}, nil).(*Transport)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://resource.example/mcp", http.NoBody)
	assert.NilError(t, err)

	resp, err := transport.RoundTrip(req)
	if resp != nil {
		defer closeBody(resp.Body)
	}
	assert.Assert(t, resp == nil)
	authErr, ok := IsAuthorizationRequired(err)
	assert.Assert(t, ok)
	assert.Equal(t, authErr.Reason, AuthorizationReasonRequired)

	pending := manager.PendingForBinding(binding)
	assert.Assert(t, pending != nil)
	assert.Equal(t, authErr.PendingID, pending.ID)
	assert.Equal(t, authErr.AuthURL, manager.StartURL(pending.ID))
}

func TestTransportRoundTripReportsRefreshFailureAsAuthorizationRequired(t *testing.T) {
	manager := newTestManager(t, "http://127.0.0.1:9666")
	manager.httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return newTextResponse(http.StatusBadGateway, "refresh failed"), nil
	})}

	binding := Binding{PrincipalID: "principal-1", Gateway: "gw", Server: "srv"}
	assert.NilError(t, manager.SaveToken(binding, &StoredToken{
		AccessToken:      "expired-access",
		RefreshToken:     "refresh-token",
		Expiry:           time.Now().Add(-time.Minute),
		TokenEndpoint:    "https://issuer.example/token",
		ClientAuthMethod: "client_secret_post",
		Resource:         "https://resource.example/mcp",
	}))

	transport := manager.NewTransport(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return newTextResponse(http.StatusUnauthorized, "unauthorized"), nil
	}), binding, "https://resource.example/mcp", &config.OAuthConfig{
		ClientID:              "client-id",
		ClientSecret:          "client-secret",
		Resource:              "https://resource.example/mcp",
		AuthorizationEndpoint: "https://issuer.example/authorize",
		TokenEndpoint:         "https://issuer.example/token",
		ClientAuthMethod:      "client_secret_post",
	}, nil).(*Transport)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://resource.example/mcp", http.NoBody)
	assert.NilError(t, err)

	resp, err := transport.RoundTrip(req)
	if resp != nil {
		defer closeBody(resp.Body)
	}
	authErr, ok := IsAuthorizationRequired(err)
	assert.Assert(t, ok)
	assert.Equal(t, authErr.Reason, AuthorizationReasonRefreshFailed)
}

func TestHandlerHelpers(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://resource.example/mcp", strings.NewReader("payload"))
	assert.NilError(t, err)

	clonedBody, err := cloneRequestBody(req)
	assert.NilError(t, err)
	assert.Equal(t, string(clonedBody), "payload")

	outgoing := cloneOutgoingRequest(req, clonedBody)
	outgoingBody, err := io.ReadAll(outgoing.Body)
	assert.NilError(t, err)
	assert.Equal(t, string(outgoingBody), "payload")

	assert.Equal(t, nonEmpty("primary", "fallback"), "primary")
	assert.Equal(t, nonEmpty("  ", "fallback"), "fallback")
	assert.DeepEqual(t, nonEmptyStrings([]string{"a"}, []string{"b"}), []string{"a"})
	assert.DeepEqual(t, nonEmptyStrings(nil, []string{"b"}), []string{"b"})

	resolved := &ResolvedMetadata{Issuer: "https://issuer.example", Scopes: []string{"scope.one"}}
	assert.Equal(t, metadataField(resolved, func(value *ResolvedMetadata) string { return value.Issuer }), "https://issuer.example")
	assert.DeepEqual(t, metadataSliceField(resolved, func(value *ResolvedMetadata) []string { return value.Scopes }), []string{"scope.one"})
	assert.Equal(t, metadataField(nil, func(value *ResolvedMetadata) string { return value.Issuer }), "")
	assert.DeepEqual(t, metadataSliceField(nil, func(value *ResolvedMetadata) []string { return value.Scopes }), []string(nil))

	token := &StoredToken{
		Resource:              "https://resource.example/mcp",
		Scopes:                []string{"scope.one"},
		Issuer:                "https://issuer.example",
		AuthorizationEndpoint: "https://issuer.example/authorize",
		TokenEndpoint:         "https://issuer.example/token",
		ClientAuthMethod:      "client_secret_post",
	}
	assert.DeepEqual(t, resolvedMetadataFromToken(token), &ResolvedMetadata{
		Resource:              token.Resource,
		Scopes:                token.Scopes,
		Issuer:                token.Issuer,
		AuthorizationEndpoint: token.AuthorizationEndpoint,
		TokenEndpoint:         token.TokenEndpoint,
		ClientAuthMethod:      token.ClientAuthMethod,
	})
	assert.Assert(t, resolvedMetadataFromToken(nil) == nil)

	assert.Assert(t, isAuthorizationResponse(http.StatusUnauthorized))
	assert.Assert(t, isAuthorizationResponse(http.StatusForbidden))
	assert.Assert(t, !isAuthorizationResponse(http.StatusOK))

	authErr := &AuthorizationRequiredError{Binding: Binding{Gateway: "gw", Server: "srv"}, AuthURL: "http://127.0.0.1:9666/oauth/start?id=test"}
	assert.Assert(t, strings.Contains(authErr.Error(), "gw/srv"))
	unwrapped, ok := IsAuthorizationRequired(authErr)
	assert.Assert(t, ok)
	assert.Assert(t, unwrapped == authErr)
	_, ok = IsAuthorizationRequired(errors.New("other"))
	assert.Assert(t, !ok)

	closed := false
	closeBody(closeTracker{onClose: func() {
		closed = true
	}})
	assert.Assert(t, closed)
	closeBody(nil)
}

type closeTracker struct {
	onClose func()
}

func (c closeTracker) Read(_ []byte) (int, error) {
	return 0, io.EOF
}

func (c closeTracker) Close() error {
	if c.onClose != nil {
		c.onClose()
	}
	return nil
}
