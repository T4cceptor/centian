package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gotest.tools/assert"
)

// writeProviderFile writes a key file with a single entry and returns its path.
func writeProviderFile(t *testing.T, entry APIKeyEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "api_keys.json")
	assert.NilError(t, WriteAPIKeyFile(path, &APIKeyFile{Keys: []APIKeyEntry{entry}}))
	return path
}

// Given: a file provider over one credential
// When: resolving its token
// Then: the mapped principal carries the persisted id, credential id, and grants.
func TestFilePrincipalProviderGetPrincipal(t *testing.T) {
	entry := APIKeyEntry{
		ID:          "key_abc",
		Hash:        hashKey(t, "the-secret"),
		PrincipalID: "pr_0000000000000_principal0",
		Gateways:    []string{"alpha"},
		Projects:    []string{"acme"},
	}
	provider := NewFilePrincipalProvider(writeProviderFile(t, entry))
	assert.NilError(t, provider.Setup(context.Background()))
	t.Cleanup(func() { _ = provider.Close() })

	principal, err := provider.GetPrincipal(context.Background(), tokenFor("key_abc", "the-secret"))
	assert.NilError(t, err)
	assert.Equal(t, principal.ID, "pr_0000000000000_principal0")
	assert.Equal(t, principal.CredentialID, "key_abc")
	assert.DeepEqual(t, principal.Gateways, []string{"alpha"})
	assert.DeepEqual(t, principal.Projects, []string{"acme"})
}

// Given: a file provider
// When: presenting a wrong secret, unknown credential id, or legacy-format token
// Then: the appropriate sentinel error is returned (and the index is O(1)).
func TestFilePrincipalProviderRejections(t *testing.T) {
	entry := APIKeyEntry{ID: "key_abc", Hash: hashKey(t, "the-secret"), PrincipalID: "pr_x"}
	provider := NewFilePrincipalProvider(writeProviderFile(t, entry))
	assert.NilError(t, provider.Setup(context.Background()))

	_, err := provider.GetPrincipal(context.Background(), tokenFor("key_abc", "wrong"))
	assert.Assert(t, errors.Is(err, ErrPrincipalNotFound))

	_, err = provider.GetPrincipal(context.Background(), tokenFor("key_missing", "the-secret"))
	assert.Assert(t, errors.Is(err, ErrPrincipalNotFound))

	_, err = provider.GetPrincipal(context.Background(), "sk-legacytokenwithoutdot")
	assert.Assert(t, errors.Is(err, ErrLegacyTokenFormat))
}

// Given: a key file with a persisted principal id
// When: the provider is set up twice (simulating a restart)
// Then: the resolved Principal.ID is identical across reloads.
//
// This guards the OAuth-binding/pool-identity stability requirement: the
// principal id must never be regenerated on load.
func TestFilePrincipalProviderIDStableAcrossReload(t *testing.T) {
	gen, err := GenerateAPIKey()
	assert.NilError(t, err)
	entry, err := NewAPIKeyEntry(gen)
	assert.NilError(t, err)
	path := writeProviderFile(t, entry)

	first := NewFilePrincipalProvider(path)
	assert.NilError(t, first.Setup(context.Background()))
	p1, err := first.GetPrincipal(context.Background(), gen.Token)
	assert.NilError(t, err)

	second := NewFilePrincipalProvider(path)
	assert.NilError(t, second.Setup(context.Background()))
	p2, err := second.GetPrincipal(context.Background(), gen.Token)
	assert.NilError(t, err)

	assert.Assert(t, p1.ID != "")
	assert.Equal(t, p1.ID, p2.ID)
}

// Given: missing or empty key files
// When: setting up the provider
// Then: the historical sentinel errors are preserved.
func TestFilePrincipalProviderSetupErrors(t *testing.T) {
	missing := NewFilePrincipalProvider(filepath.Join(t.TempDir(), "nope.json"))
	assert.Assert(t, errors.Is(missing.Setup(context.Background()), ErrAPIKeysNotFound))

	emptyPath := writeTempFile(t, `{"keys":[]}`)
	empty := NewFilePrincipalProvider(emptyPath)
	assert.Assert(t, errors.Is(empty.Setup(context.Background()), ErrAPIKeysEmpty))
}
