package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// FilePrincipalProvider is the file-based PrincipalProvider. It reads the same
// ~/.centian/api_keys.json store and resolves tokens to principals using an
// in-memory credential-id index (O(1) lookup + a single bcrypt verify).
type FilePrincipalProvider struct {
	path  string
	mu    sync.RWMutex
	index map[string]*APIKeyEntry
}

// NewFilePrincipalProvider creates a provider backed by the given key file path.
func NewFilePrincipalProvider(path string) *FilePrincipalProvider {
	return &FilePrincipalProvider{path: path}
}

// DefaultFilePrincipalProvider creates a provider backed by the default key file.
func DefaultFilePrincipalProvider() (*FilePrincipalProvider, error) {
	path, err := DefaultAPIKeysPath()
	if err != nil {
		return nil, err
	}
	return NewFilePrincipalProvider(path), nil
}

// Setup reads and indexes the key file. It mirrors the historical load
// semantics: a missing file yields ErrAPIKeysNotFound and an empty file yields
// ErrAPIKeysEmpty, so callers can surface the same guidance as before.
func (p *FilePrincipalProvider) Setup(_ context.Context) error {
	file, err := ReadAPIKeyFile(p.path)
	if err != nil {
		return err
	}
	if len(file.Keys) == 0 {
		return fmt.Errorf("%w: %s", ErrAPIKeysEmpty, p.path)
	}

	index := make(map[string]*APIKeyEntry, len(file.Keys))
	for i := range file.Keys {
		entry := &file.Keys[i]
		if strings.TrimSpace(entry.Hash) == "" {
			return fmt.Errorf("%w: empty hash", ErrAPIKeysInvalid)
		}
		index[entry.ID] = entry
	}

	p.mu.Lock()
	p.index = index
	p.mu.Unlock()
	return nil
}

// GetPrincipal resolves a token to its principal. Unknown credential ids or
// secret mismatches return ErrPrincipalNotFound; pre-principal tokens return
// ErrLegacyTokenFormat.
func (p *FilePrincipalProvider) GetPrincipal(_ context.Context, token string) (*Principal, error) {
	credID, secret, err := parseToken(token)
	if err != nil {
		return nil, err
	}

	p.mu.RLock()
	entry, ok := p.index[credID]
	p.mu.RUnlock()
	if !ok {
		return nil, ErrPrincipalNotFound
	}
	if bcrypt.CompareHashAndPassword([]byte(entry.Hash), []byte(secret)) != nil {
		return nil, ErrPrincipalNotFound
	}
	return entry.toPrincipal(), nil
}

// ListPrincipals returns the distinct principals across indexed credentials.
func (p *FilePrincipalProvider) ListPrincipals(_ context.Context) ([]Principal, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	seen := make(map[string]struct{}, len(p.index))
	principals := make([]Principal, 0, len(p.index))
	for _, entry := range p.index {
		principal := entry.toPrincipal()
		if principal == nil || principal.ID == "" {
			continue
		}
		if _, dup := seen[principal.ID]; dup {
			continue
		}
		seen[principal.ID] = struct{}{}
		principals = append(principals, *principal)
	}
	return principals, nil
}

// Count returns the number of indexed credentials (for startup logging).
func (p *FilePrincipalProvider) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.index)
}

// Path returns the key file path the provider reads from.
func (p *FilePrincipalProvider) Path() string {
	return p.path
}

// Close releases provider resources. The file provider holds none.
func (p *FilePrincipalProvider) Close() error {
	return nil
}
