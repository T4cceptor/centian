package auth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

	plain := "sk-test-default"
	file := &APIKeyFile{
		Keys: []APIKeyEntry{
			{
				ID:        "key_1",
				Hash:      hashKey(t, plain),
				CreatedAt: "2025-01-01T00:00:00Z",
			},
		},
	}
	assert.NilError(t, WriteAPIKeyFile(path, file))

	// When: loading API keys from the default path
	store, err := LoadDefaultAPIKeys()

	// Then: the key should validate and path should match
	assert.NilError(t, err)
	assert.Equal(t, store.Path(), path)
	assert.Equal(t, store.Count(), 1)
	if _, ok := store.Resolve(plain); !ok {
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
	new_key, err := GenerateAPIKey()

	// Then: no error, and key is as expected
	assert.NilError(t, err)
	sk_prefix := new_key[:3]
	assert.Assert(t, sk_prefix == "sk-")
	assert.Assert(t, len(new_key) == 46)
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
	// Given: a JSON object with hashed keys
	hash1 := hashKey(t, "key-1")
	hash2 := hashKey(t, "key-2")
	path := writeTempFile(t, `{"keys":[{"id":"key_1","hash":"`+hash1+`","created_at":"2025-01-01T00:00:00Z"},{"id":"key_2","hash":"`+hash2+`","created_at":"2025-01-02T00:00:00Z"}]}`)

	// When: loading API keys from the file
	store, err := LoadAPIKeys(path)

	// Then: keys should validate
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.Count() != 2 {
		t.Fatalf("expected 2 keys, got %d", store.Count())
	}
	if _, ok := store.Resolve("key-1"); !ok {
		t.Fatalf("expected key-1 to be present")
	}
	if _, ok := store.Resolve("key-2"); !ok {
		t.Fatalf("expected keys to be present")
	}
	if _, ok := store.Resolve("missing"); ok {
		t.Fatalf("expected missing key to be invalid")
	}
}

func TestAPIKeyStoreLookup(t *testing.T) {
	// Given: a store with one API key
	plain := "lookup-key"
	path := writeTempFile(t, `{"keys":[{"id":"key_lookup","hash":"`+hashKey(t, plain)+`","created_at":"2025-01-01T00:00:00Z"}]}`)
	store, err := LoadAPIKeys(path)
	assert.NilError(t, err)

	// When: looking up the valid key
	entry, ok := store.Lookup(plain)

	// Then: the matching entry is returned
	assert.Assert(t, ok)
	assert.Assert(t, entry != nil)
	assert.Equal(t, entry.ID, "key_lookup")

	// When: looking up an invalid key
	entry, ok = store.Lookup("missing")

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
	// Given: an empty api key file
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "api_keys.json")

	plain := "sk-test-key"
	entry, err := NewAPIKeyEntry(plain)
	if err != nil {
		t.Fatalf("failed to create entry: %v", err)
	}

	// When: appending the key
	if _, err := AppendAPIKey(path, entry); err != nil {
		t.Fatalf("failed to append api key: %v", err)
	}

	// Then: the key should validate
	store, err := LoadAPIKeys(path)
	if err != nil {
		t.Fatalf("failed to load api keys: %v", err)
	}
	if _, ok := store.Resolve(plain); !ok {
		t.Fatalf("expected key to validate")
	}
}

func TestResolve(t *testing.T) {
	plain := "sk-test-resolve"
	path := writeTempFile(t, `{"keys":[{"id":"key_resolve","hash":"`+hashKey(t, plain)+`","created_at":"2025-01-01T00:00:00Z"}]}`)
	store, err := LoadAPIKeys(path)
	assert.NilError(t, err)

	entry, ok := store.Resolve(plain)
	assert.Assert(t, ok)
	assert.Assert(t, entry != nil)
	assert.Equal(t, entry.ID, "key_resolve")

	_, ok = store.Resolve("sk-missing")
	assert.Assert(t, !ok)
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
