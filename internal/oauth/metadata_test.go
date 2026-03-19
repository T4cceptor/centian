package oauth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"golang.org/x/oauth2"
	"gotest.tools/assert"
)

func TestResolveMetadataUsesConfiguredEndpointsAndChallengeScopes(t *testing.T) {
	resolved, err := resolveMetadata(context.Background(), &http.Client{}, "https://resource.example/mcp", http.Header{
		http.CanonicalHeaderKey("WWW-Authenticate"): []string{`Bearer scope="scope.one scope.two"`},
	}, &config.OAuthConfig{
		Resource:              "https://resource.example/mcp",
		Scopes:                []string{"fallback"},
		Issuer:                "https://issuer.example",
		ClientAuthMethod:      "client_secret_post",
		AuthorizationEndpoint: "https://issuer.example/authorize",
		TokenEndpoint:         "https://issuer.example/token",
	})
	assert.NilError(t, err)
	assert.Equal(t, resolved.Resource, "https://resource.example/mcp")
	assert.DeepEqual(t, resolved.Scopes, []string{"scope.one", "scope.two"})
	assert.Equal(t, resolved.AuthorizationEndpoint, "https://issuer.example/authorize")
	assert.Equal(t, resolved.TokenEndpoint, "https://issuer.example/token")
	assert.Equal(t, resolved.ClientAuthMethod, "client_secret_post")
}

func TestResolveMetadataFetchesProtectedResourceAndIssuerMetadata(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "http://127.0.0.1:7777/resource-meta":
			return newJSONResponse(`{"resource":"http://127.0.0.1:7777/mcp","authorization_servers":["http://127.0.0.1:7777/issuer"],"scopes_supported":["tool:echo"]}`), nil
		case "http://127.0.0.1:7777/.well-known/oauth-authorization-server/issuer":
			return newJSONResponse(`{"issuer":"http://127.0.0.1:7777/issuer","authorization_endpoint":"http://127.0.0.1:7777/authorize","token_endpoint":"http://127.0.0.1:7777/token","token_endpoint_auth_methods_supported":["client_secret_basic","client_secret_post"],"code_challenge_methods_supported":["S256"]}`), nil
		default:
			return nil, errors.New("unexpected fetch: " + req.URL.String())
		}
	})}

	resolved, err := resolveMetadata(context.Background(), client, "http://127.0.0.1:7777/mcp", http.Header{
		http.CanonicalHeaderKey("WWW-Authenticate"): []string{`Bearer resource_metadata="http://127.0.0.1:7777/resource-meta"`},
	}, &config.OAuthConfig{})
	assert.NilError(t, err)
	assert.Equal(t, resolved.Resource, "http://127.0.0.1:7777/mcp")
	assert.Equal(t, resolved.Issuer, "http://127.0.0.1:7777/issuer")
	assert.Equal(t, resolved.AuthorizationEndpoint, "http://127.0.0.1:7777/authorize")
	assert.Equal(t, resolved.TokenEndpoint, "http://127.0.0.1:7777/token")
	assert.Equal(t, resolved.ClientAuthMethod, "client_secret_post")
	assert.DeepEqual(t, resolved.Scopes, []string{"tool:echo"})
}

func TestMetadataParsingHelpers(t *testing.T) {
	headers := []string{`Bearer realm="example", scope="scope.one scope.two", resource_metadata="https://issuer.example/meta", error="invalid_token", Basic realm="ignored"`}

	challenges, err := parseWWWAuthenticate(headers)
	assert.NilError(t, err)
	assert.Equal(t, len(challenges), 2)
	assert.Equal(t, challenges[0].Scheme, "bearer")
	assert.Equal(t, metadataURLFromChallenges(headers), "https://issuer.example/meta")
	assert.DeepEqual(t, scopesFromChallenges(challenges), []string{"scope.one", "scope.two"})

	parts := splitChallenges(headers[0])
	assert.Equal(t, len(parts), 2)
	parsed, err := parseSingleChallenge(`Bearer error="invalid_token", scope="scope.one"`)
	assert.NilError(t, err)
	assert.Equal(t, parsed.Params["error"], "invalid_token")
	assert.Equal(t, parsed.Params["scope"], "scope.one")

	_, err = parseSingleChallenge(`Bearer bad`)
	assert.ErrorContains(t, err, "malformed auth param")
}

func TestMetadataURLAndMergeHelpers(t *testing.T) {
	assert.Equal(t, defaultProtectedResourceMetadataURL("https://resource.example/api/mcp"), "https://resource.example/.well-known/oauth-protected-resource/api/mcp")

	url, err := defaultAuthorizationServerMetadataURL("https://issuer.example/path")
	assert.NilError(t, err)
	assert.Equal(t, url, "https://issuer.example/.well-known/oauth-authorization-server/path")

	assert.NilError(t, checkHTTPSOrLoopback("https://issuer.example/meta"))
	assert.NilError(t, checkHTTPSOrLoopback("http://127.0.0.1:7777/meta"))
	assert.NilError(t, checkHTTPSOrLoopback("http://localhost:7777/meta"))
	assert.ErrorContains(t, checkHTTPSOrLoopback("http://issuer.example/meta"), "https or loopback")

	dst := &ResolvedMetadata{}
	mergeProtectedResourceMetadata(dst, &protectedResourceMetadata{
		Resource:             "https://resource.example/mcp",
		AuthorizationServers: []string{"https://issuer.example"},
		ScopesSupported:      []string{"tool:echo"},
	})
	assert.Equal(t, dst.Resource, "https://resource.example/mcp")
	assert.Equal(t, dst.Issuer, "https://issuer.example")
	assert.DeepEqual(t, dst.Scopes, []string{"tool:echo"})

	mergeAuthorizationServerMetadata(dst, &authorizationServerMetadata{
		Issuer:                            "https://issuer.example",
		AuthorizationEndpoint:             "https://issuer.example/authorize",
		TokenEndpoint:                     "https://issuer.example/token",
		TokenEndpointAuthMethodsSupported: []string{"client_secret_basic", "client_secret_post"},
	})
	assert.Equal(t, dst.AuthorizationEndpoint, "https://issuer.example/authorize")
	assert.Equal(t, dst.TokenEndpoint, "https://issuer.example/token")
	assert.Equal(t, dst.ClientAuthMethod, "client_secret_post")
}

func TestFetchJSONAndAuthMethodHelpers(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://issuer.example/success":
			return newJSONResponse(`{"value":"ok"}`), nil
		case "https://issuer.example/bad-json":
			return newJSONResponse(`not-json`), nil
		default:
			return newTextResponse(http.StatusBadGateway, "bad gateway"), nil
		}
	})}

	type payload struct {
		Value string `json:"value"`
	}

	value, err := fetchJSON[payload](context.Background(), client, "https://issuer.example/success")
	assert.NilError(t, err)
	assert.Equal(t, value.Value, "ok")

	_, err = fetchJSON[payload](context.Background(), client, "https://issuer.example/failure")
	assert.ErrorContains(t, err, "unexpected status")

	_, err = fetchJSON[payload](context.Background(), client, "https://issuer.example/bad-json")
	assert.ErrorContains(t, err, "invalid character")

	assert.Equal(t, authMethodToStyle("client_secret_post"), oauth2.AuthStyleInParams)
	assert.Equal(t, authMethodToStyle("client_secret_basic"), oauth2.AuthStyleInHeader)
	assert.Equal(t, authMethodToStyle("other"), oauth2.AuthStyleAutoDetect)

	first, err := randomToken(12)
	assert.NilError(t, err)
	second, err := randomToken(12)
	assert.NilError(t, err)
	assert.Assert(t, first != "")
	assert.Assert(t, second != "")
	assert.Assert(t, first != second)
	assert.Assert(t, !strings.Contains(first, "+"))
}

func TestFetchAuthorizationServerMetadataRequiresPKCES256(t *testing.T) {
	t.Run("missing code challenge methods", func(t *testing.T) {
		client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return newJSONResponse(`{"issuer":"https://issuer.example","authorization_endpoint":"https://issuer.example/authorize","token_endpoint":"https://issuer.example/token"}`), nil
		})}

		_, err := fetchAuthorizationServerMetadata(context.Background(), client, "https://issuer.example")
		assert.Assert(t, err != nil)
		assert.ErrorContains(t, err, pkceS256Requirement)
		assert.ErrorContains(t, err, "code_challenge_methods_supported")
	})

	t.Run("missing S256 support", func(t *testing.T) {
		client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return newJSONResponse(`{"issuer":"https://issuer.example","authorization_endpoint":"https://issuer.example/authorize","token_endpoint":"https://issuer.example/token","code_challenge_methods_supported":["plain"]}`), nil
		})}

		_, err := fetchAuthorizationServerMetadata(context.Background(), client, "https://issuer.example")
		assert.Assert(t, err != nil)
		assert.ErrorContains(t, err, pkceS256Requirement)
		assert.ErrorContains(t, err, "plain")
	})
}
