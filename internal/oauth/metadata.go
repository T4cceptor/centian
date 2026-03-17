package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode"

	"github.com/T4cceptor/centian/internal/config"
	"golang.org/x/oauth2"
)

type Challenge struct {
	Scheme string
	Params map[string]string
}

type ProtectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers,omitempty"`
	ScopesSupported      []string `json:"scopes_supported,omitempty"`
}

type AuthorizationServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported,omitempty"`
}

type ResolvedMetadata struct {
	Resource              string
	Scopes                []string
	Issuer                string
	AuthorizationEndpoint string
	TokenEndpoint         string
	ClientAuthMethod      string
}

func resolveMetadata(ctx context.Context, httpClient *http.Client, reqURL string, header http.Header, oauthConfig *config.OAuthConfig) (ResolvedMetadata, error) {
	resolved := ResolvedMetadata{
		Resource:         strings.TrimSpace(oauthConfig.Resource),
		Scopes:           append([]string(nil), oauthConfig.Scopes...),
		Issuer:           strings.TrimSpace(oauthConfig.Issuer),
		ClientAuthMethod: strings.TrimSpace(oauthConfig.ClientAuthMethod),
	}

	challenges, err := parseWWWAuthenticate(header[http.CanonicalHeaderKey("WWW-Authenticate")])
	if err != nil {
		return ResolvedMetadata{}, err
	}
	if scopes := scopesFromChallenges(challenges); len(scopes) > 0 {
		resolved.Scopes = scopes
	}

	if endpoint := strings.TrimSpace(oauthConfig.AuthorizationEndpoint); endpoint != "" {
		resolved.AuthorizationEndpoint = endpoint
	}
	if endpoint := strings.TrimSpace(oauthConfig.TokenEndpoint); endpoint != "" {
		resolved.TokenEndpoint = endpoint
	}
	if resolved.AuthorizationEndpoint != "" && resolved.TokenEndpoint != "" {
		return resolved, nil
	}

	if resolved.Issuer != "" {
		meta, err := fetchAuthorizationServerMetadata(ctx, httpClient, resolved.Issuer)
		if err != nil {
			return ResolvedMetadata{}, err
		}
		mergeAuthorizationServerMetadata(&resolved, meta)
		return resolved, nil
	}

	prm, err := fetchProtectedResourceMetadata(ctx, httpClient, reqURL, header)
	if err != nil {
		return ResolvedMetadata{}, err
	}
	if prm != nil {
		if resolved.Resource == "" {
			resolved.Resource = prm.Resource
		}
		if len(resolved.Scopes) == 0 && len(prm.ScopesSupported) > 0 {
			resolved.Scopes = append([]string(nil), prm.ScopesSupported...)
		}
		if resolved.Issuer == "" && len(prm.AuthorizationServers) > 0 {
			resolved.Issuer = prm.AuthorizationServers[0]
		}
	}
	if resolved.Issuer == "" {
		return ResolvedMetadata{}, fmt.Errorf("unable to determine oauth issuer for downstream")
	}

	meta, err := fetchAuthorizationServerMetadata(ctx, httpClient, resolved.Issuer)
	if err != nil {
		return ResolvedMetadata{}, err
	}
	mergeAuthorizationServerMetadata(&resolved, meta)
	return resolved, nil
}

func mergeAuthorizationServerMetadata(dst *ResolvedMetadata, src *AuthorizationServerMetadata) {
	if dst == nil || src == nil {
		return
	}
	if dst.Issuer == "" {
		dst.Issuer = src.Issuer
	}
	if dst.AuthorizationEndpoint == "" {
		dst.AuthorizationEndpoint = src.AuthorizationEndpoint
	}
	if dst.TokenEndpoint == "" {
		dst.TokenEndpoint = src.TokenEndpoint
	}
	if dst.ClientAuthMethod == "" && len(src.TokenEndpointAuthMethodsSupported) > 0 {
		for _, candidate := range []string{"client_secret_post", "client_secret_basic"} {
			for _, supported := range src.TokenEndpointAuthMethodsSupported {
				if supported == candidate {
					dst.ClientAuthMethod = candidate
					break
				}
			}
			if dst.ClientAuthMethod != "" {
				break
			}
		}
	}
}

func fetchProtectedResourceMetadata(ctx context.Context, httpClient *http.Client, resourceURL string, header http.Header) (*ProtectedResourceMetadata, error) {
	metadataURL := metadataURLFromChallenges(header[http.CanonicalHeaderKey("WWW-Authenticate")])
	if metadataURL == "" {
		metadataURL = defaultProtectedResourceMetadataURL(resourceURL)
	}
	if err := checkHTTPSOrLoopback(metadataURL); err != nil {
		return nil, err
	}
	meta, err := fetchJSON[ProtectedResourceMetadata](ctx, httpClient, metadataURL)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func fetchAuthorizationServerMetadata(ctx context.Context, httpClient *http.Client, issuer string) (*AuthorizationServerMetadata, error) {
	metadataURL, err := defaultAuthorizationServerMetadataURL(issuer)
	if err != nil {
		return nil, err
	}
	if err := checkHTTPSOrLoopback(metadataURL); err != nil {
		return nil, err
	}
	meta, err := fetchJSON[AuthorizationServerMetadata](ctx, httpClient, metadataURL)
	if err != nil {
		return nil, err
	}
	if meta.Issuer != "" && meta.Issuer != issuer {
		return nil, fmt.Errorf("authorization server issuer mismatch: got %q want %q", meta.Issuer, issuer)
	}
	if len(meta.CodeChallengeMethodsSupported) == 0 {
		return nil, fmt.Errorf("authorization server does not advertise PKCE support")
	}
	supportsS256 := false
	for _, method := range meta.CodeChallengeMethodsSupported {
		if method == "S256" {
			supportsS256 = true
			break
		}
	}
	if !supportsS256 {
		return nil, fmt.Errorf("authorization server does not support PKCE S256")
	}
	return meta, nil
}

func fetchJSON[T any](ctx context.Context, httpClient *http.Client, endpoint string) (*T, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, endpoint)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var value T
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func defaultProtectedResourceMetadataURL(resourceURL string) string {
	u, err := url.Parse(resourceURL)
	if err != nil {
		return ""
	}
	u.Path = path.Join("/.well-known/oauth-protected-resource", u.Path)
	return u.String()
}

func defaultAuthorizationServerMetadataURL(issuer string) (string, error) {
	u, err := url.Parse(issuer)
	if err != nil {
		return "", err
	}
	originalPath := strings.TrimRight(u.Path, "/")
	if originalPath == "" {
		u.Path = "/.well-known/oauth-authorization-server"
		return u.String(), nil
	}
	u.Path = "/.well-known/oauth-authorization-server/" + strings.TrimLeft(originalPath, "/")
	return u.String(), nil
}

func metadataURLFromChallenges(headers []string) string {
	challenges, err := parseWWWAuthenticate(headers)
	if err != nil {
		return ""
	}
	for _, challenge := range challenges {
		if challenge.Scheme == "bearer" && challenge.Params["resource_metadata"] != "" {
			return challenge.Params["resource_metadata"]
		}
	}
	return ""
}

func scopesFromChallenges(challenges []Challenge) []string {
	for _, challenge := range challenges {
		if scope := challenge.Params["scope"]; scope != "" {
			return strings.Fields(scope)
		}
	}
	return nil
}

func parseWWWAuthenticate(headers []string) ([]Challenge, error) {
	var challenges []Challenge
	for _, header := range headers {
		parts, err := splitChallenges(header)
		if err != nil {
			return nil, err
		}
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			challenge, err := parseSingleChallenge(part)
			if err != nil {
				return nil, err
			}
			challenges = append(challenges, challenge)
		}
	}
	return challenges, nil
}

func splitChallenges(header string) ([]string, error) {
	var result []string
	inQuotes := false
	start := 0
	for i, r := range header {
		switch {
		case r == '"' && (i == 0 || header[i-1] != '\\'):
			inQuotes = !inQuotes
		case r == ',' && !inQuotes:
			lookahead := strings.TrimSpace(header[i+1:])
			eqPos := strings.Index(lookahead, "=")
			isParam := false
			if eqPos > 0 {
				token := lookahead[:eqPos]
				if strings.IndexFunc(token, unicode.IsSpace) == -1 {
					isParam = true
				}
			}
			if !isParam {
				result = append(result, header[start:i])
				start = i + 1
			}
		}
	}
	result = append(result, header[start:])
	return result, nil
}

func parseSingleChallenge(raw string) (Challenge, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Challenge{}, fmt.Errorf("empty challenge")
	}
	scheme, paramsText, found := strings.Cut(raw, " ")
	challenge := Challenge{Scheme: strings.ToLower(scheme), Params: map[string]string{}}
	if !found {
		return challenge, nil
	}
	paramsText = strings.TrimSpace(paramsText)
	for paramsText != "" {
		keyEnd := strings.Index(paramsText, "=")
		if keyEnd <= 0 {
			return Challenge{}, fmt.Errorf("malformed auth param %q", paramsText)
		}
		key := strings.ToLower(strings.TrimSpace(paramsText[:keyEnd]))
		paramsText = strings.TrimSpace(paramsText[keyEnd+1:])
		var value string
		if strings.HasPrefix(paramsText, "\"") {
			paramsText = paramsText[1:]
			var builder strings.Builder
			i := 0
			for ; i < len(paramsText); i++ {
				if paramsText[i] == '\\' && i+1 < len(paramsText) {
					builder.WriteByte(paramsText[i+1])
					i++
					continue
				}
				if paramsText[i] == '"' {
					break
				}
				builder.WriteByte(paramsText[i])
			}
			if i >= len(paramsText) {
				return Challenge{}, fmt.Errorf("unterminated quoted auth param")
			}
			value = builder.String()
			paramsText = strings.TrimSpace(paramsText[i+1:])
		} else {
			commaPos := strings.Index(paramsText, ",")
			if commaPos == -1 {
				value = strings.TrimSpace(paramsText)
				paramsText = ""
			} else {
				value = strings.TrimSpace(paramsText[:commaPos])
				paramsText = strings.TrimSpace(paramsText[commaPos:])
			}
		}
		if value == "" {
			return Challenge{}, fmt.Errorf("empty auth param value for %q", key)
		}
		challenge.Params[key] = value
		if strings.HasPrefix(paramsText, ",") {
			paramsText = strings.TrimSpace(paramsText[1:])
		}
	}
	return challenge, nil
}

func checkHTTPSOrLoopback(raw string) error {
	if raw == "" {
		return fmt.Errorf("empty URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		host := u.Hostname()
		if host == "localhost" {
			return nil
		}
		ip := net.ParseIP(host)
		if ip != nil && ip.IsLoopback() {
			return nil
		}
	}
	return fmt.Errorf("URL must use https or loopback http")
}

func authMethodToStyle(method string) oauth2.AuthStyle {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "client_secret_post":
		return oauth2.AuthStyleInParams
	case "client_secret_basic":
		return oauth2.AuthStyleInHeader
	default:
		return oauth2.AuthStyleAutoDetect
	}
}

func randomToken(byteCount int) (string, error) {
	buf := make([]byte, byteCount)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
