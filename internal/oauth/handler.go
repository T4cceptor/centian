package oauth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"golang.org/x/oauth2"
)

var errTokenRefreshNotAvailable = errors.New("oauth token refresh not available")

const (
	// AuthorizationReasonRequired marks errors that need an initial browser login.
	AuthorizationReasonRequired = "authorization required"
	// AuthorizationReasonRefreshFailed marks errors caused by a failed token refresh.
	AuthorizationReasonRefreshFailed = "refresh failed"
)

// Transport wraps a downstream HTTP transport with OAuth token loading, refresh, and authorization flow handling.
type Transport struct {
	Base          http.RoundTripper
	Manager       *Manager
	Binding       Binding
	DownstreamURL string
	Config        *config.OAuthConfig
	Headers       map[string]string
}

// RoundTrip adds downstream OAuth headers, retries after refresh, and surfaces interactive authorization requirements.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	bodyBytes, err := cloneRequestBody(req)
	if err != nil {
		return nil, err
	}
	resp, err := t.send(req, bodyBytes, t.baseRoundTripper())
	if err != nil {
		return nil, err
	}
	if !isAuthorizationResponse(resp.StatusCode) {
		return resp, nil
	}
	return t.handleAuthorizationChallenge(req, bodyBytes, resp)
}

func (t *Transport) baseRoundTripper() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

func (t *Transport) send(req *http.Request, bodyBytes []byte, base http.RoundTripper) (*http.Response, error) {
	outgoing := cloneOutgoingRequest(req, bodyBytes)
	for key, value := range t.Headers {
		outgoing.Header.Set(key, value)
	}
	if authHeader, err := t.authorizationHeader(req.Context()); err != nil {
		return nil, err
	} else if authHeader != "" {
		outgoing.Header.Set("Authorization", authHeader)
	}
	return base.RoundTrip(outgoing)
}

func (t *Transport) authorizationHeader(ctx context.Context) (string, error) {
	token, err := t.Manager.LoadToken(t.Binding)
	switch {
	case err == nil:
	case errors.Is(err, errTokenNotFound):
		return "", nil
	default:
		return "", err
	}
	if token == nil {
		return "", nil
	}

	oauthToken := token.oauthToken()
	if oauthToken != nil && oauthToken.Valid() && oauthToken.AccessToken != "" {
		return "Bearer " + oauthToken.AccessToken, nil
	}

	refreshed, err := refreshStoredToken(ctx, t.Manager, t.Binding, t.Config, token, resolvedMetadataFromToken(token))
	if err == nil && refreshed != nil && refreshed.AccessToken != "" {
		return "Bearer " + refreshed.AccessToken, nil
	}
	if err != nil && !errors.Is(err, errTokenRefreshNotAvailable) {
		common.LogWarn("oauth: token refresh failed for %s/%s, will retry after 401: %v", t.Binding.Gateway, t.Binding.Server, err)
	}
	return "", nil
}

func (t *Transport) handleAuthorizationChallenge(req *http.Request, bodyBytes []byte, resp *http.Response) (*http.Response, error) {
	resolved, err := resolveMetadata(req.Context(), t.Manager.httpClient, req.URL.String(), resp.Header, t.Config)
	// Headers are already parsed; free the body regardless of metadata resolution outcome.
	closeBody(resp.Body)
	if err != nil {
		return nil, err
	}

	refreshed, refreshErr := refreshStoredToken(req.Context(), t.Manager, t.Binding, t.Config, nil, &resolved)
	if refreshErr == nil && refreshed != nil && refreshed.AccessToken != "" {
		retryResp, retryErr := t.send(req, bodyBytes, t.baseRoundTripper())
		if retryErr != nil {
			return nil, retryErr
		}
		if !isAuthorizationResponse(retryResp.StatusCode) {
			return retryResp, nil
		}
		// Refreshed token was also rejected — proceed to interactive auth.
		closeBody(retryResp.Body)
	}
	verifier := oauth2.GenerateVerifier()
	pending, err := t.Manager.CreatePending(
		t.Binding,
		strings.TrimSpace(t.Config.ClientID),
		strings.TrimSpace(t.Config.ClientSecret),
		&resolved,
		verifier,
	)
	if err != nil {
		return nil, err
	}
	reason := AuthorizationReasonRequired
	if refreshErr != nil && !errors.Is(refreshErr, errTokenRefreshNotAvailable) {
		reason = AuthorizationReasonRefreshFailed
	}
	return nil, &AuthorizationRequiredError{
		Binding:   t.Binding,
		AuthURL:   t.Manager.StartURL(pending.ID),
		StatusURL: t.Manager.StatusURL(pending.ID),
		PendingID: pending.ID,
		Reason:    reason,
	}
}

func cloneOutgoingRequest(req *http.Request, bodyBytes []byte) *http.Request {
	outgoing := req.Clone(req.Context())
	if bodyBytes == nil {
		return outgoing
	}
	outgoing.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	outgoing.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}
	outgoing.ContentLength = int64(len(bodyBytes))
	return outgoing
}

func cloneRequestBody(req *http.Request) ([]byte, error) {
	if req == nil || req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err == nil {
			defer closeBody(body)
			return io.ReadAll(body)
		}
	}
	// Fall back to reading and restoring the original body.
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	closeBody(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}
	return bodyBytes, nil
}

func refreshStoredToken(
	ctx context.Context,
	manager *Manager,
	binding Binding,
	cfg *config.OAuthConfig,
	stored *StoredToken,
	resolved *ResolvedMetadata,
) (*oauth2.Token, error) {
	if manager == nil || cfg == nil {
		return nil, errTokenRefreshNotAvailable
	}
	if stored == nil {
		var err error
		stored, err = manager.LoadToken(binding)
		if err != nil {
			if errors.Is(err, errTokenNotFound) {
				return nil, errTokenRefreshNotAvailable
			}
			return nil, err
		}
	}
	if stored == nil || stored.RefreshToken == "" {
		return nil, errTokenRefreshNotAvailable
	}

	tokenEndpoint := ""
	if resolved != nil {
		tokenEndpoint = resolved.TokenEndpoint
	}
	if tokenEndpoint == "" {
		tokenEndpoint = stored.TokenEndpoint
	}
	if tokenEndpoint == "" {
		return nil, errTokenRefreshNotAvailable
	}

	authMethod := ""
	if resolved != nil {
		authMethod = resolved.ClientAuthMethod
	}
	if authMethod == "" {
		authMethod = stored.ClientAuthMethod
	}
	oauthCfg := oauth2.Config{
		ClientID:     strings.TrimSpace(cfg.ClientID),
		ClientSecret: strings.TrimSpace(cfg.ClientSecret),
		Endpoint: oauth2.Endpoint{
			TokenURL:  tokenEndpoint,
			AuthStyle: authMethodToStyle(authMethod),
		},
	}
	clientCtx := context.WithValue(ctx, oauth2.HTTPClient, manager.httpClient)
	refreshSeed := &oauth2.Token{
		RefreshToken: stored.RefreshToken,
		Expiry:       time.Unix(0, 0),
	}
	refreshed, err := oauthCfg.TokenSource(clientCtx, refreshSeed).Token()
	if err != nil {
		return nil, err
	}
	record := tokenFromOAuth(refreshed, &StoredToken{
		Resource:              nonEmpty(metadataField(resolved, func(value *ResolvedMetadata) string { return value.Resource }), stored.Resource),
		Scopes:                nonEmptyStrings(metadataSliceField(resolved, func(value *ResolvedMetadata) []string { return value.Scopes }), stored.Scopes),
		Issuer:                nonEmpty(metadataField(resolved, func(value *ResolvedMetadata) string { return value.Issuer }), stored.Issuer),
		AuthorizationEndpoint: nonEmpty(metadataField(resolved, func(value *ResolvedMetadata) string { return value.AuthorizationEndpoint }), stored.AuthorizationEndpoint),
		TokenEndpoint:         tokenEndpoint,
		ClientAuthMethod:      nonEmpty(authMethod, stored.ClientAuthMethod),
	})
	return refreshed, manager.SaveToken(binding, record)
}

func nonEmpty(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func nonEmptyStrings(primary, fallback []string) []string {
	if len(primary) > 0 {
		return append([]string(nil), primary...)
	}
	return append([]string(nil), fallback...)
}

func metadataField(resolved *ResolvedMetadata, get func(*ResolvedMetadata) string) string {
	if resolved == nil {
		return ""
	}
	return get(resolved)
}

func metadataSliceField(resolved *ResolvedMetadata, get func(*ResolvedMetadata) []string) []string {
	if resolved == nil {
		return nil
	}
	return get(resolved)
}

func resolvedMetadataFromToken(token *StoredToken) *ResolvedMetadata {
	if token == nil {
		return nil
	}
	return &ResolvedMetadata{
		Resource:              token.Resource,
		Scopes:                append([]string(nil), token.Scopes...),
		Issuer:                token.Issuer,
		AuthorizationEndpoint: token.AuthorizationEndpoint,
		TokenEndpoint:         token.TokenEndpoint,
		ClientAuthMethod:      token.ClientAuthMethod,
	}
}

func closeBody(body io.Closer) {
	if body != nil {
		_ = body.Close()
	}
}

func isAuthorizationResponse(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden
}

// IsAuthorizationRequired unwraps errors returned when a downstream request must be authorized in the browser.
func IsAuthorizationRequired(err error) (*AuthorizationRequiredError, bool) {
	var authErr *AuthorizationRequiredError
	if errors.As(err, &authErr) {
		return authErr, true
	}
	return nil, false
}
