package oauth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"golang.org/x/oauth2"
)

type Transport struct {
	Base          http.RoundTripper
	Manager       *Manager
	Binding       Binding
	DownstreamURL string
	Config        *config.OAuthConfig
	Headers       map[string]string
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	bodyBytes, err := cloneRequestBody(req)
	if err != nil {
		return nil, err
	}

	send := func() (*http.Response, error) {
		outgoing := req.Clone(req.Context())
		if bodyBytes != nil {
			outgoing.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			outgoing.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(bodyBytes)), nil
			}
			outgoing.ContentLength = int64(len(bodyBytes))
		}
		for key, value := range t.Headers {
			outgoing.Header.Set(key, value)
		}
		token, loadErr := t.Manager.LoadToken(t.Binding)
		if loadErr != nil {
			return nil, loadErr
		}
		if token != nil {
			oauthToken := token.OAuthToken()
			if oauthToken != nil && oauthToken.Valid() && oauthToken.AccessToken != "" {
				outgoing.Header.Set("Authorization", "Bearer "+oauthToken.AccessToken)
			} else if oauthToken != nil && token.RefreshToken != "" && token.TokenEndpoint != "" {
				refreshed, refreshErr := refreshStoredToken(req.Context(), t.Manager, t.Binding, t.Config, token, ResolvedMetadata{
					Resource:              token.Resource,
					Scopes:                token.Scopes,
					Issuer:                token.Issuer,
					AuthorizationEndpoint: token.AuthorizationEndpoint,
					TokenEndpoint:         token.TokenEndpoint,
					ClientAuthMethod:      token.ClientAuthMethod,
				})
				if refreshErr == nil && refreshed != nil && refreshed.AccessToken != "" {
					outgoing.Header.Set("Authorization", "Bearer "+refreshed.AccessToken)
				}
			}
		}
		return base.RoundTrip(outgoing)
	}

	resp, err := send()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		return resp, nil
	}

	resolved, err := resolveMetadata(req.Context(), t.Manager.httpClient, req.URL.String(), resp.Header, t.Config)
	if err != nil {
		resp.Body.Close()
		return nil, err
	}

	refreshed, refreshErr := refreshStoredToken(req.Context(), t.Manager, t.Binding, t.Config, nil, resolved)
	if refreshErr == nil && refreshed != nil && refreshed.AccessToken != "" {
		resp.Body.Close()
		return send()
	}

	resp.Body.Close()
	verifier := oauth2.GenerateVerifier()
	pending, err := t.Manager.CreatePending(
		t.Binding,
		strings.TrimSpace(t.Config.ClientID),
		strings.TrimSpace(t.Config.ClientSecret),
		resolved,
		verifier,
	)
	if err != nil {
		return nil, err
	}
	reason := "authorization required"
	if refreshErr != nil {
		reason = "refresh failed"
	}
	return nil, &AuthorizationRequiredError{
		Binding: t.Binding,
		AuthURL: t.Manager.StartURL(pending.ID),
		Reason:  reason,
	}
}

func cloneRequestBody(req *http.Request) ([]byte, error) {
	if req == nil || req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err == nil {
			defer body.Close()
			return io.ReadAll(body)
		}
	}
	// Fall back to reading and restoring the original body.
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body.Close() //nolint:errcheck
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
	resolved ResolvedMetadata,
) (*oauth2.Token, error) {
	if manager == nil || cfg == nil {
		return nil, nil
	}
	if stored == nil {
		var err error
		stored, err = manager.LoadToken(binding)
		if err != nil {
			return nil, err
		}
	}
	if stored == nil || stored.RefreshToken == "" {
		return nil, nil
	}

	tokenEndpoint := resolved.TokenEndpoint
	if tokenEndpoint == "" {
		tokenEndpoint = stored.TokenEndpoint
	}
	if tokenEndpoint == "" {
		return nil, nil
	}

	authMethod := resolved.ClientAuthMethod
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
		Resource:              nonEmpty(resolved.Resource, stored.Resource),
		Scopes:                nonEmptyStrings(resolved.Scopes, stored.Scopes),
		Issuer:                nonEmpty(resolved.Issuer, stored.Issuer),
		AuthorizationEndpoint: nonEmpty(resolved.AuthorizationEndpoint, stored.AuthorizationEndpoint),
		TokenEndpoint:         tokenEndpoint,
		ClientAuthMethod:      nonEmpty(authMethod, stored.ClientAuthMethod),
	})
	return refreshed, manager.SaveAndReturnToken(binding, record)
}

func (m *Manager) SaveAndReturnToken(binding Binding, token *StoredToken) error {
	return m.SaveToken(binding, token)
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

func IsAuthorizationRequired(err error) (*AuthorizationRequiredError, bool) {
	var authErr *AuthorizationRequiredError
	if errors.As(err, &authErr) {
		return authErr, true
	}
	return nil, false
}
