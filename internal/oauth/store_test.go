package oauth

import (
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"gotest.tools/assert"
)

func TestDefaultMasterKeyManagerCreatesKeyWithSecurePermissions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	manager, err := newDefaultMasterKeyManager()
	assert.NilError(t, err)

	key, err := manager.loadOrCreate()
	assert.NilError(t, err)
	assert.Equal(t, len(key), 32)

	info, err := os.Stat(manager.path)
	assert.NilError(t, err)
	assert.Equal(t, info.Mode().Perm(), os.FileMode(0o600))
}

func TestEncryptedTokenStoreRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	manager, err := newDefaultMasterKeyManager()
	assert.NilError(t, err)
	store, err := newDefaultEncryptedTokenStore(manager)
	assert.NilError(t, err)

	binding := Binding{PrincipalID: "user-1", Gateway: "gw", Server: "srv"}
	token := &StoredToken{
		AccessToken:           "access-token",
		RefreshToken:          "refresh-token",
		Expiry:                time.Now().UTC().Add(time.Hour).Round(time.Second),
		Resource:              "https://resource.example/mcp",
		Scopes:                []string{"tool:echo"},
		Issuer:                "https://issuer.example",
		AuthorizationEndpoint: "https://issuer.example/authorize",
		TokenEndpoint:         "https://issuer.example/token",
		ClientAuthMethod:      "client_secret_post",
	}

	assert.NilError(t, store.save(binding, token))

	loaded, err := store.load(binding)
	assert.NilError(t, err)
	assert.DeepEqual(t, loaded, token)

	assert.NilError(t, store.delete(binding))
	loaded, err = store.load(binding)
	assert.Assert(t, errors.Is(err, errTokenNotFound))
	assert.Assert(t, loaded == nil)
}

func TestStoredTokenHelpers(t *testing.T) {
	expiry := time.Now().UTC().Add(time.Hour).Round(time.Second)
	stored := &StoredToken{
		AccessToken:           "access-token",
		TokenType:             "Bearer",
		RefreshToken:          "refresh-token",
		Expiry:                expiry,
		Resource:              "https://resource.example/mcp",
		Scopes:                []string{"tool:echo"},
		Issuer:                "https://issuer.example",
		AuthorizationEndpoint: "https://issuer.example/authorize",
		TokenEndpoint:         "https://issuer.example/token",
		ClientAuthMethod:      "client_secret_post",
	}

	oauthToken := stored.oauthToken()
	assert.Assert(t, oauthToken != nil)
	assert.Equal(t, oauthToken.AccessToken, "access-token")
	assert.Equal(t, oauthToken.TokenType, "Bearer")
	assert.Equal(t, oauthToken.RefreshToken, "refresh-token")
	assert.DeepEqual(t, oauthToken.Expiry, expiry)
	assert.Assert(t, (*StoredToken)(nil).oauthToken() == nil)

	token := tokenFromOAuth(&oauth2.Token{
		AccessToken: "new-access",
		TokenType:   "Bearer",
		Expiry:      expiry,
	}, stored)
	assert.Equal(t, token.AccessToken, "new-access")
	assert.Equal(t, token.RefreshToken, "refresh-token")
	assert.Equal(t, token.Resource, stored.Resource)
	assert.DeepEqual(t, token.Scopes, stored.Scopes)
	assert.Assert(t, tokenFromOAuth(nil, stored) == nil)
}
