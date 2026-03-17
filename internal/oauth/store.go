package oauth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"golang.org/x/oauth2"
)

const (
	defaultMasterKeyFile = "oauth_master_key"
	defaultTokenFile     = "oauth_tokens.json" //nolint:gosec // Encrypted token store filename, not a credential.
)

var errTokenNotFound = errors.New("oauth token not found")

// Binding identifies one persisted downstream OAuth token.
type Binding struct {
	PrincipalID string
	Gateway     string
	Server      string
}

func (b Binding) storageKey() string {
	return fmt.Sprintf("%s|%s|%s", b.PrincipalID, b.Gateway, b.Server)
}

// StoredToken is the serialized OAuth token record persisted for one binding.
type StoredToken struct {
	AccessToken           string    `json:"accessToken"`
	TokenType             string    `json:"tokenType,omitempty"`
	RefreshToken          string    `json:"refreshToken,omitempty"`
	Expiry                time.Time `json:"expiry,omitempty"`
	Resource              string    `json:"resource,omitempty"`
	Scopes                []string  `json:"scopes,omitempty"`
	Issuer                string    `json:"issuer,omitempty"`
	AuthorizationEndpoint string    `json:"authorizationEndpoint,omitempty"`
	TokenEndpoint         string    `json:"tokenEndpoint,omitempty"`
	ClientAuthMethod      string    `json:"clientAuthMethod,omitempty"`
}

func (s *StoredToken) oauthToken() *oauth2.Token {
	if s == nil {
		return nil
	}
	return &oauth2.Token{
		AccessToken:  s.AccessToken,
		TokenType:    s.TokenType,
		RefreshToken: s.RefreshToken,
		Expiry:       s.Expiry,
	}
}

func tokenFromOAuth(src *oauth2.Token, template *StoredToken) *StoredToken {
	if src == nil {
		return nil
	}
	dst := &StoredToken{
		AccessToken:  src.AccessToken,
		TokenType:    src.TokenType,
		RefreshToken: src.RefreshToken,
		Expiry:       src.Expiry,
	}
	if template != nil {
		dst.Resource = template.Resource
		dst.Scopes = append([]string(nil), template.Scopes...)
		dst.Issuer = template.Issuer
		dst.AuthorizationEndpoint = template.AuthorizationEndpoint
		dst.TokenEndpoint = template.TokenEndpoint
		dst.ClientAuthMethod = template.ClientAuthMethod
		if dst.RefreshToken == "" {
			dst.RefreshToken = template.RefreshToken
		}
	}
	return dst
}

type encryptedTokenFile struct {
	Tokens map[string]encryptedTokenEnvelope `json:"tokens"`
}

type encryptedTokenEnvelope struct {
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
	UpdatedAt  string `json:"updatedAt,omitempty"`
}

type masterKeyManager struct {
	path string
}

func defaultMasterKeyPath() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, defaultMasterKeyFile), nil
}

func newDefaultMasterKeyManager() (*masterKeyManager, error) {
	path, err := defaultMasterKeyPath()
	if err != nil {
		return nil, err
	}
	return &masterKeyManager{path: path}, nil
}

func (m *masterKeyManager) loadOrCreate() ([]byte, error) {
	path := filepath.Clean(m.path)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create oauth key directory: %w", err)
	}
	data, err := os.ReadFile(path)
	if err == nil {
		decoded, decErr := base64.RawStdEncoding.DecodeString(string(data))
		if decErr != nil {
			return nil, fmt.Errorf("decode oauth master key: %w", decErr)
		}
		if len(decoded) != 32 {
			return nil, fmt.Errorf("oauth master key must be 32 bytes")
		}
		return decoded, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read oauth master key: %w", err)
	}

	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate oauth master key: %w", err)
	}
	encoded := base64.RawStdEncoding.EncodeToString(key)
	if err := os.WriteFile(path, []byte(encoded), 0o600); err != nil {
		return nil, fmt.Errorf("write oauth master key: %w", err)
	}
	return key, nil
}

type encryptedTokenStore struct {
	path string
	key  []byte
	mu   sync.Mutex
}

func defaultTokenStorePath() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, defaultTokenFile), nil
}

func newDefaultEncryptedTokenStore(keyManager *masterKeyManager) (*encryptedTokenStore, error) {
	if keyManager == nil {
		return nil, fmt.Errorf("master key manager is required")
	}
	key, err := keyManager.loadOrCreate()
	if err != nil {
		return nil, err
	}
	path, err := defaultTokenStorePath()
	if err != nil {
		return nil, err
	}
	return &encryptedTokenStore{path: path, key: key}, nil
}

func (s *encryptedTokenStore) load(binding Binding) (*StoredToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.readFile()
	if err != nil {
		return nil, err
	}
	entry, ok := file.Tokens[binding.storageKey()]
	if !ok {
		return nil, errTokenNotFound
	}
	return s.decrypt(entry)
}

func (s *encryptedTokenStore) save(binding Binding, token *StoredToken) error {
	if token == nil {
		return fmt.Errorf("token is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.readFile()
	if err != nil {
		return err
	}
	entry, err := s.encrypt(token)
	if err != nil {
		return err
	}
	file.Tokens[binding.storageKey()] = entry
	return s.writeFile(file)
}

func (s *encryptedTokenStore) delete(binding Binding) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.readFile()
	if err != nil {
		return err
	}
	delete(file.Tokens, binding.storageKey())
	return s.writeFile(file)
}

func (s *encryptedTokenStore) readFile() (*encryptedTokenFile, error) {
	data, err := os.ReadFile(filepath.Clean(s.path))
	if err != nil {
		if os.IsNotExist(err) {
			return &encryptedTokenFile{Tokens: map[string]encryptedTokenEnvelope{}}, nil
		}
		return nil, fmt.Errorf("read oauth token file: %w", err)
	}
	var file encryptedTokenFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse oauth token file: %w", err)
	}
	if file.Tokens == nil {
		file.Tokens = map[string]encryptedTokenEnvelope{}
	}
	return &file, nil
}

func (s *encryptedTokenStore) writeFile(file *encryptedTokenFile) error {
	path := filepath.Clean(s.path)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil { //nolint:gosec // Store path comes from the local config directory and is cleaned before use.
		return fmt.Errorf("create oauth token directory: %w", err)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal oauth token file: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil { //nolint:gosec // Store path comes from the local config directory and is cleaned before use.
		return fmt.Errorf("write oauth token file: %w", err)
	}
	return nil
}

func (s *encryptedTokenStore) encrypt(token *StoredToken) (encryptedTokenEnvelope, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return encryptedTokenEnvelope{}, fmt.Errorf("create oauth cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return encryptedTokenEnvelope{}, fmt.Errorf("create oauth gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return encryptedTokenEnvelope{}, fmt.Errorf("generate oauth nonce: %w", err)
	}
	plaintext, err := json.Marshal(token) //nolint:gosec // The token JSON is encrypted immediately and never written in plaintext.
	if err != nil {
		return encryptedTokenEnvelope{}, fmt.Errorf("marshal oauth token: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return encryptedTokenEnvelope{
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *encryptedTokenStore) decrypt(entry encryptedTokenEnvelope) (*StoredToken, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, fmt.Errorf("create oauth cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create oauth gcm: %w", err)
	}
	nonce, err := base64.RawStdEncoding.DecodeString(entry.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode oauth nonce: %w", err)
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(entry.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode oauth ciphertext: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt oauth token: %w", err)
	}
	var token StoredToken
	if err := json.Unmarshal(plaintext, &token); err != nil {
		return nil, fmt.Errorf("unmarshal oauth token: %w", err)
	}
	return &token, nil
}
