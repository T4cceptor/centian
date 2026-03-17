package oauth

import (
	"os"
	"testing"
	"time"

	"gotest.tools/assert"
)

func TestDefaultMasterKeyManagerCreatesKeyWithSecurePermissions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	manager, err := NewDefaultMasterKeyManager()
	assert.NilError(t, err)

	key, err := manager.LoadOrCreate()
	assert.NilError(t, err)
	assert.Equal(t, len(key), 32)

	info, err := os.Stat(manager.path)
	assert.NilError(t, err)
	assert.Equal(t, info.Mode().Perm(), os.FileMode(0o600))
}

func TestEncryptedTokenStoreRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	manager, err := NewDefaultMasterKeyManager()
	assert.NilError(t, err)
	store, err := NewDefaultEncryptedTokenStore(manager)
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

	assert.NilError(t, store.Save(binding, token))

	loaded, err := store.Load(binding)
	assert.NilError(t, err)
	assert.DeepEqual(t, loaded, token)

	assert.NilError(t, store.Delete(binding))
	loaded, err = store.Load(binding)
	assert.NilError(t, err)
	assert.Assert(t, loaded == nil)
}
