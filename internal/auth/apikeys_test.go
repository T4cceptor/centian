package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

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
	if !store.Validate("key-1") || !store.Validate("key-2") {
		t.Fatalf("expected keys to be present")
	}
	if store.Validate("missing") {
		t.Fatalf("expected missing key to be invalid")
	}
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
	if !store.Validate(plain) {
		t.Fatalf("expected key to validate")
	}
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
