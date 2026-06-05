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

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/identifiers"
)

const (
	// apiKeyTokenPrefix is the stable, human-recognizable prefix on every token.
	apiKeyTokenPrefix = "sk-"
	// tokenSeparator splits the public credential id from the secret. It is
	// outside the base64url alphabet so it cannot appear inside the secret.
	tokenSeparator = "."
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
//
// ID is the credential id embedded (public) in the token; Hash is the bcrypt of
// the token's secret portion only. PrincipalID is the stable principal identity
// this credential resolves to and must be persisted (never regenerated on load)
// so downstream pool reuse and OAuth bindings stay stable across restarts.
type APIKeyEntry struct {
	ID          string   `json:"id"`
	Hash        string   `json:"hash"`
	PrincipalID string   `json:"principal_id,omitempty"`
	CreatedAt   string   `json:"created_at"`
	Gateways    []string `json:"gateways,omitempty"`
	Projects    []string `json:"projects,omitempty"` // Allowed project slugs (empty = allow all, "*" = allow all)
}

// toPrincipal maps a stored credential entry to its resolved Principal.
func (e *APIKeyEntry) toPrincipal() *Principal {
	if e == nil {
		return nil
	}
	return &Principal{
		ID:           e.PrincipalID,
		DisplayName:  e.ID,
		CredentialID: e.ID,
		Gateways:     append([]string(nil), e.Gateways...),
		Projects:     append([]string(nil), e.Projects...),
	}
}

// APIKeyStore stores API keys loaded from disk for quick validation.
//
// Deprecated: superseded by the FilePrincipalProvider. Retained until the proxy
// is fully migrated to PrincipalProvider, then removed.
type APIKeyStore struct {
	path    string
	entries []APIKeyEntry
	index   map[string]*APIKeyEntry // keyed by credential id for O(1) lookup
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

	index := make(map[string]*APIKeyEntry, len(file.Keys))
	for i := range file.Keys {
		entry := &file.Keys[i]
		if strings.TrimSpace(entry.Hash) == "" {
			return nil, fmt.Errorf("%w: empty hash", ErrAPIKeysInvalid)
		}
		index[entry.ID] = entry
	}

	return &APIKeyStore{
		path:    path,
		entries: file.Keys,
		index:   index,
	}, nil
}

// Lookup returns the matching API key entry for the provided token.
// It resolves the credential id embedded in the token to a single entry (O(1))
// and verifies the secret with one bcrypt comparison.
func (s *APIKeyStore) Lookup(key string) (*APIKeyEntry, bool) {
	if s == nil {
		return nil, false
	}
	credID, secret, err := parseToken(key)
	if err != nil {
		return nil, false
	}
	entry, ok := s.index[credID]
	if !ok {
		return nil, false
	}
	if bcrypt.CompareHashAndPassword([]byte(entry.Hash), []byte(secret)) != nil {
		return nil, false
	}
	return entry, true
}

// parseToken splits a token of the form "sk-<credId>.<secret>" into its parts.
// Tokens without the embedded credential id (the pre-principal format) are
// rejected with ErrLegacyTokenFormat.
func parseToken(token string) (credID, secret string, err error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(token), apiKeyTokenPrefix)
	sepIndex := strings.Index(trimmed, tokenSeparator)
	if sepIndex < 0 {
		return "", "", ErrLegacyTokenFormat
	}
	credID = trimmed[:sepIndex]
	secret = trimmed[sepIndex+len(tokenSeparator):]
	if credID == "" || secret == "" {
		return "", "", ErrLegacyTokenFormat
	}
	return credID, secret, nil
}

// AllowsGateway checks whether this key is allowed to access the given gateway.
// Empty list or "*" means allow-all (see allowMatch).
func (e *APIKeyEntry) AllowsGateway(gateway string) bool {
	if e == nil {
		return false
	}
	return allowMatch(e.Gateways, gateway)
}

// AllowsProject checks whether this key is allowed to access the given project.
// Empty list or "*" means allow-all (see allowMatch).
func (e *APIKeyEntry) AllowsProject(project string) bool {
	if e == nil {
		return false
	}
	return allowMatch(e.Projects, project)
}

// Validate returns true if the provided API key exists in the store.
func (s *APIKeyStore) Validate(key string) bool {
	_, ok := s.Lookup(key)
	return ok
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
func AppendAPIKey(path string, entry *APIKeyEntry) (*APIKeyFile, error) {
	file, err := ReadAPIKeyFile(path)
	if err != nil {
		if errors.Is(err, ErrAPIKeysNotFound) {
			file = &APIKeyFile{Keys: []APIKeyEntry{}}
		} else {
			return nil, err
		}
	}

	file.Keys = append(file.Keys, *entry)
	if err := WriteAPIKeyFile(path, file); err != nil {
		return nil, err
	}
	return file, nil
}

// GeneratedKey holds the material produced when minting a new API key. Only the
// Token is shown to the user (once); CredID and Secret are used to build the
// stored entry (the credential id is public, the secret is bcrypt-hashed).
type GeneratedKey struct {
	Token  string
	CredID string
	Secret string
}

// GenerateAPIKey mints a new API key token of the form "sk-<credId>.<secret>".
func GenerateAPIKey() (GeneratedKey, error) {
	credID, err := generateKeyID()
	if err != nil {
		return GeneratedKey{}, err
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return GeneratedKey{}, fmt.Errorf("failed to generate api key: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	return GeneratedKey{
		Token:  apiKeyTokenPrefix + credID + tokenSeparator + secret,
		CredID: credID,
		Secret: secret,
	}, nil
}

// NewAPIKeyEntry builds a stored entry from generated key material. The secret
// (not the whole token) is bcrypt-hashed, and a fresh persisted principal id is
// assigned so the credential resolves to a stable principal across restarts.
func NewAPIKeyEntry(gen GeneratedKey) (APIKeyEntry, error) {
	if strings.TrimSpace(gen.Secret) == "" || strings.TrimSpace(gen.CredID) == "" {
		return APIKeyEntry{}, fmt.Errorf("%w: empty api key material", ErrAPIKeysInvalid)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(gen.Secret), bcrypt.DefaultCost)
	if err != nil {
		return APIKeyEntry{}, fmt.Errorf("failed to hash api key: %w", err)
	}
	return APIKeyEntry{
		ID:          gen.CredID,
		Hash:        string(hash),
		PrincipalID: identifiers.New(identifiers.KindPrincipal),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func generateKeyID() (string, error) {
	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		return "", fmt.Errorf("failed to generate api key id: %w", err)
	}
	return "key_" + hex.EncodeToString(idBytes), nil
}
