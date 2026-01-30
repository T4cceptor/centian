package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/CentianAI/centian-cli/internal/config"
)

var (
	// ErrAPIKeysNotFound is returned if no api key file was found.
	ErrAPIKeysNotFound = errors.New("api key file not found")

	// ErrAPIKeysEmpty is returned if the provided api key file does not contain any keys.
	ErrAPIKeysEmpty = errors.New("api key file contains no keys")

	// ErrAPIKeysInvalid is returned when the api key file is malformed.
	ErrAPIKeysInvalid = errors.New("api key file is invalid")
)

// APIKeyFile stores hashed API keys on disk.
type APIKeyFile struct {
	Keys []APIKeyEntry `json:"keys"`
}

// APIKeyEntry represents a stored API key hash and metadata.
type APIKeyEntry struct {
	ID        string `json:"id"`
	Hash      string `json:"hash"`
	CreatedAt string `json:"created_at"`
}

// APIKeyStore stores API keys loaded from disk for quick validation.
type APIKeyStore struct {
	path    string
	entries []APIKeyEntry
}

// DefaultAPIKeysPath returns the default path to the API keys file (~/.centian/api_keys.json).
func DefaultAPIKeysPath() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "api_keys.json"), nil
}

// LoadDefaultAPIKeys loads API keys from the default path.
func LoadDefaultAPIKeys() (*APIKeyStore, error) {
	path, err := DefaultAPIKeysPath()
	if err != nil {
		return nil, err
	}
	return LoadAPIKeys(path)
}

// LoadAPIKeys loads API keys from a JSON file.
func LoadAPIKeys(path string) (*APIKeyStore, error) {
	file, err := ReadAPIKeyFile(path)
	if err != nil {
		return nil, err
	}

	if len(file.Keys) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrAPIKeysEmpty, path)
	}

	for _, entry := range file.Keys {
		if strings.TrimSpace(entry.Hash) == "" {
			return nil, fmt.Errorf("%w: empty hash", ErrAPIKeysInvalid)
		}
	}

	return &APIKeyStore{
		path:    path,
		entries: file.Keys,
	}, nil
}

// Validate returns true if the provided API key exists in the store.
func (s *APIKeyStore) Validate(key string) bool {
	if s == nil {
		return false
	}
	for _, entry := range s.entries {
		if err := bcrypt.CompareHashAndPassword([]byte(entry.Hash), []byte(key)); err == nil {
			return true
		}
	}
	return false
}

// Count returns the number of unique API keys in the store.
func (s *APIKeyStore) Count() int {
	if s == nil {
		return 0
	}
	return len(s.entries)
}

// Path returns the file path the keys were loaded from.
func (s *APIKeyStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// ReadAPIKeyFile loads API key data from disk without validating contents.
func ReadAPIKeyFile(path string) (*APIKeyFile, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrAPIKeysNotFound, path)
		}
		return nil, fmt.Errorf("failed to read api key file: %w", err)
	}

	var file APIKeyFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("failed to parse api key file: %w", err)
	}
	if file.Keys == nil {
		file.Keys = []APIKeyEntry{}
	}
	return &file, nil
}

// WriteAPIKeyFile writes API keys to disk using secure permissions.
func WriteAPIKeyFile(path string, file *APIKeyFile) error {
	if file == nil {
		return fmt.Errorf("%w: nil payload", ErrAPIKeysInvalid)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("failed to create api key directory: %w", err)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal api key file: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write api key file: %w", err)
	}
	return nil
}

// AppendAPIKey appends an entry to the API key file, creating it if needed.
func AppendAPIKey(path string, entry APIKeyEntry) (*APIKeyFile, error) {
	file, err := ReadAPIKeyFile(path)
	if err != nil {
		if errors.Is(err, ErrAPIKeysNotFound) {
			file = &APIKeyFile{Keys: []APIKeyEntry{}}
		} else {
			return nil, err
		}
	}

	file.Keys = append(file.Keys, entry)
	if err := WriteAPIKeyFile(path, file); err != nil {
		return nil, err
	}
	return file, nil
}

// GenerateAPIKey creates a new API key string with a stable prefix.
func GenerateAPIKey() (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate api key: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	return "sk-" + token, nil
}

// NewAPIKeyEntry hashes a plaintext API key and returns a stored entry.
func NewAPIKeyEntry(plainKey string) (APIKeyEntry, error) {
	if strings.TrimSpace(plainKey) == "" {
		return APIKeyEntry{}, fmt.Errorf("%w: empty api key", ErrAPIKeysInvalid)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plainKey), bcrypt.DefaultCost)
	if err != nil {
		return APIKeyEntry{}, fmt.Errorf("failed to hash api key: %w", err)
	}
	id, err := generateKeyID()
	if err != nil {
		return APIKeyEntry{}, err
	}
	return APIKeyEntry{
		ID:        id,
		Hash:      string(hash),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func generateKeyID() (string, error) {
	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		return "", fmt.Errorf("failed to generate api key id: %w", err)
	}
	return "key_" + hex.EncodeToString(idBytes), nil
}
