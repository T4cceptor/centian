package auth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"gotest.tools/assert"
)

func TestDefaultAPIKeysPath(t *testing.T) {
	// Given: the DefaultAPIKeysPath method
	// When: calling it
	result, err := DefaultAPIKeysPath()

	// Then:
	assert.NilError(t, err)
	homeDir, _ := os.UserHomeDir()
	expected := fmt.Sprintf("%s/.centian/api_keys.json", homeDir)
	assert.Equal(t, result, expected)
}

func TestLoadDefaultAPIKeys(t *testing.T) {
	// Given: a temp home directory with a default api key file
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	path, err := DefaultAPIKeysPath()
	assert.NilError(t, err)

	secret := "default-secret"
	file := &APIKeyFile{
		Keys: []APIKeyEntry{
			{
				ID:        "key_1",
				Hash:      hashKey(t, secret),
				CreatedAt: "2025-01-01T00:00:00Z",
			},
		},
	}
	assert.NilError(t, WriteAPIKeyFile(path, file))

	// When: loading API keys from the default path
	store, err := LoadDefaultAPIKeys()

	// Then: the key should validate via its token and path should match
	assert.NilError(t, err)
	assert.Equal(t, store.Path(), path)
	assert.Equal(t, store.Count(), 1)
	if _, ok := store.Lookup(tokenFor("key_1", secret)); !ok {
		t.Fatalf("expected key to validate")
	}
}

func TestPath(t *testing.T) {
	// Given: an APIKeyStore
	set_path := "testpath1"
	keystore := APIKeyStore{
		path: set_path,
	}

	// When: calling Path
	path := keystore.Path()

	// Then: the set path is returned
	assert.Equal(t, path, set_path)
}

func TestGenerateAPIKey(t *testing.T) {
	// Given: GenerateAPIKey method
	// When: calling GenerateAPIKey
	gen, err := GenerateAPIKey()

	// Then: token has the sk- prefix, embeds the credential id, and round-trips
	assert.NilError(t, err)
	assert.Assert(t, strings.HasPrefix(gen.Token, "sk-"))
	assert.Assert(t, gen.CredID != "" && gen.Secret != "")
	assert.Equal(t, gen.Token, tokenFor(gen.CredID, gen.Secret))

	credID, secret, perr := parseToken(gen.Token)
	assert.NilError(t, perr)
	assert.Equal(t, credID, gen.CredID)
	assert.Equal(t, secret, gen.Secret)
}

func TestLoadAPIKeys_NotFound(t *testing.T) {
	// Given: a path that does not exist
	path := filepath.Join(t.TempDir(), "missing.json")

	// When: loading API keys from the missing file
	_, err := LoadAPIKeys(path)

	// Then: we should get a not-found error
	if err == nil || !errors.Is(err, ErrAPIKeysNotFound) {
		t.Fatalf("expected ErrAPIKeysNotFound, got %v", err)
	}
}

func TestLoadAPIKeys_ObjectFormat(t *testing.T) {
	// Given: a JSON object with two hashed keys
	path := writeTempFile(t, `{"keys":[`+
		entryJSON(t, "key_1", "secret-1")+`,`+
		entryJSON(t, "key_2", "secret-2")+`]}`)

	// When: loading API keys from the file
	store, err := LoadAPIKeys(path)

	// Then: both tokens should validate, unknown ones should not
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.Count() != 2 {
		t.Fatalf("expected 2 keys, got %d", store.Count())
	}
	if _, ok := store.Lookup(tokenFor("key_1", "secret-1")); !ok {
		t.Fatalf("expected key_1 to be present")
	}
	if _, ok := store.Lookup(tokenFor("key_2", "secret-2")); !ok {
		t.Fatalf("expected key_2 to be present")
	}
	if _, ok := store.Lookup(tokenFor("key_2", "wrong-secret")); ok {
		t.Fatalf("expected wrong secret to be invalid")
	}
	if _, ok := store.Lookup(tokenFor("key_missing", "secret-1")); ok {
		t.Fatalf("expected missing credential id to be invalid")
	}
}

func TestAPIKeyStoreLookup(t *testing.T) {
	// Given: a store with one API key
	secret := "lookup-secret"
	path := writeTempFile(t, `{"keys":[`+entryJSON(t, "key_lookup", secret)+`]}`)
	store, err := LoadAPIKeys(path)
	assert.NilError(t, err)

	// When: looking up the valid token
	entry, ok := store.Lookup(tokenFor("key_lookup", secret))

	// Then: the matching entry is returned
	assert.Assert(t, ok)
	assert.Assert(t, entry != nil)
	assert.Equal(t, entry.ID, "key_lookup")

	// When: looking up a legacy-format token (no embedded credential id)
	entry, ok = store.Lookup("sk-legacyformat")

	// Then: no entry is returned
	assert.Assert(t, !ok)
	assert.Assert(t, entry == nil)
}

func TestLoadAPIKeys_ArrayFormat(t *testing.T) {
	// Given: a JSON array (unsupported format)
	path := writeTempFile(t, `["key-1","key-2"]`)

	// When: loading API keys from the file
	store, err := LoadAPIKeys(path)

	// Then: array format should be rejected
	if err == nil {
		t.Fatalf("expected error, got store with %d keys", store.Count())
	}
}

func TestLoadAPIKeys_Empty(t *testing.T) {
	// Given: a JSON object with an empty keys list
	path := writeTempFile(t, `{"keys":[]}`)

	// When: loading API keys from the file
	_, err := LoadAPIKeys(path)

	// Then: we should get an empty-keys error
	if err == nil || !errors.Is(err, ErrAPIKeysEmpty) {
		t.Fatalf("expected ErrAPIKeysEmpty, got %v", err)
	}
}

func TestAppendAPIKey(t *testing.T) {
	// Given: an empty api key file and freshly minted key material
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "api_keys.json")

	gen, err := GenerateAPIKey()
	assert.NilError(t, err)
	entry, err := NewAPIKeyEntry(gen)
	if err != nil {
		t.Fatalf("failed to create entry: %v", err)
	}

	// When: appending the key
	if _, err := AppendAPIKey(path, &entry); err != nil {
		t.Fatalf("failed to append api key: %v", err)
	}

	// Then: the original token should validate, and a principal id is persisted
	store, err := LoadAPIKeys(path)
	if err != nil {
		t.Fatalf("failed to load api keys: %v", err)
	}
	if _, ok := store.Lookup(gen.Token); !ok {
		t.Fatalf("expected key to validate")
	}
	assert.Assert(t, strings.HasPrefix(entry.PrincipalID, "pr_"))
}

func TestResolve(t *testing.T) {
	secret := "resolve-secret"
	path := writeTempFile(t, `{"keys":[`+entryJSON(t, "key_resolve", secret)+`]}`)
	store, err := LoadAPIKeys(path)
	assert.NilError(t, err)

	entry, ok := store.Lookup(tokenFor("key_resolve", secret))
	assert.Assert(t, ok)
	assert.Assert(t, entry != nil)
	assert.Equal(t, entry.ID, "key_resolve")

	_, ok = store.Lookup(tokenFor("key_resolve", "wrong"))
	assert.Assert(t, !ok)
}

func TestParseTokenRejectsLegacyFormat(t *testing.T) {
	// Given: a pre-principal token without an embedded credential id
	// When/Then: parsing it returns ErrLegacyTokenFormat
	_, _, err := parseToken("sk-justasecretnodot")
	assert.Assert(t, errors.Is(err, ErrLegacyTokenFormat))
}

func TestAPIKeyEntryAllowsGateway(t *testing.T) {
	t.Run("empty gateways allows all", func(t *testing.T) {
		entry := &APIKeyEntry{ID: "key_any"}
		assert.Assert(t, entry.AllowsGateway("default"))
		assert.Assert(t, entry.AllowsGateway("other"))
	})

	t.Run("explicit gateway is enforced", func(t *testing.T) {
		entry := &APIKeyEntry{ID: "key_scope", Gateways: []string{"alpha"}}
		assert.Assert(t, entry.AllowsGateway("alpha"))
		assert.Assert(t, !entry.AllowsGateway("beta"))
	})

	t.Run("wildcard allows all", func(t *testing.T) {
		entry := &APIKeyEntry{ID: "key_star", Gateways: []string{"*"}}
		assert.Assert(t, entry.AllowsGateway("alpha"))
		assert.Assert(t, entry.AllowsGateway("beta"))
	})
}

func writeTempFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "api_keys.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

func hashKey(t *testing.T, plain string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("failed to hash key: %v", err)
	}
	return string(hash)
}

// tokenFor builds a token for the given credential id and secret.
func tokenFor(credID, secret string) string {
	return apiKeyTokenPrefix + credID + tokenSeparator + secret
}

// entryJSON builds a stored-entry JSON fragment whose hash matches the secret.
func entryJSON(t *testing.T, credID, secret string) string {
	t.Helper()
	return `{"id":"` + credID + `","hash":"` + hashKey(t, secret) + `","created_at":"2025-01-01T00:00:00Z"}`
}
