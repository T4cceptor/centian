package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/T4cceptor/centian/internal/sqldb"
)

// This file defines the PrincipalProvider seam. Authentication is expressed as
// "resolve a token to a Principal"; the concrete backend (file-based today, SQL
// or external IdP later) is hidden behind this interface so consumers never
// depend on the credential storage mechanism.

var (
	// ErrPrincipalNotFound is returned when a token does not resolve to any
	// principal (unknown credential id or secret mismatch). Callers map this to
	// an unauthorized response.
	ErrPrincipalNotFound = errors.New("principal not found for token")

	// ErrLegacyTokenFormat is returned when a token uses the pre-principal format
	// (no embedded credential id). Such tokens are no longer supported; the
	// operator must regenerate them with `centian auth new-key`.
	ErrLegacyTokenFormat = errors.New("legacy token format is no longer supported; regenerate with `centian auth new-key`")
)

// PrincipalProvider resolves authentication tokens to principals.
//
// Setup is called once at startup to initialize the backend (e.g. read and index
// the key file, open a DB connection). GetPrincipal resolves one token per
// request and must be safe for concurrent use after Setup. Close releases any
// resources held by the provider.
type PrincipalProvider interface {
	Setup(ctx context.Context) error
	GetPrincipal(ctx context.Context, token string) (*Principal, error)
	// ListPrincipals returns the known principals (id + display name) for labeling
	// purposes. Secrets are never included and order is unspecified.
	ListPrincipals(ctx context.Context) ([]Principal, error)
	Close() error
}

// BackendType identifies a principal storage backend.
type BackendType string

const (
	// BackendSQLite stores principals in a dedicated SQLite database.
	BackendSQLite BackendType = "sqlite"
	// BackendPostgres stores principals in a PostgreSQL database (store = DSN).
	BackendPostgres BackendType = "postgres"
	// BackendFile stores api-key principals in the legacy api_keys.json file.
	BackendFile BackendType = "file"
	// DefaultBackendType is used when no backend type is configured.
	DefaultBackendType = BackendSQLite
)

// sqlDriverForBackend maps a SQL-backed BackendType to its sqldb.Driver. The
// boolean reports whether bt is a SQL backend at all (false for file).
func sqlDriverForBackend(bt BackendType) (sqldb.Driver, bool) {
	switch bt {
	case BackendSQLite:
		return sqldb.SQLite, true
	case BackendPostgres:
		return sqldb.Postgres, true
	default:
		return "", false
	}
}

// resolveBackend normalizes a configured (type, store) pair, filling defaults: an
// empty type becomes sqlite, and an empty store becomes the default location for
// the resolved type. It is the single source of truth shared by the provider
// factory and the key-creation path so the server and CLI agree on defaults.
func resolveBackend(backendType, store string) (BackendType, string, error) {
	bt := BackendType(strings.ToLower(strings.TrimSpace(backendType)))
	if bt == "" {
		bt = DefaultBackendType
	}
	store = strings.TrimSpace(store)
	switch bt {
	case BackendSQLite:
		if store == "" {
			path, err := DefaultPrincipalsDBPath()
			if err != nil {
				return "", "", err
			}
			store = path
		}
	case BackendPostgres:
		// Postgres has no sensible default location; the DSN must be configured.
		if store == "" {
			return "", "", fmt.Errorf("auth backend %q requires a store (postgres DSN)", BackendPostgres)
		}
	case BackendFile:
		if store == "" {
			path, err := DefaultAPIKeysPath()
			if err != nil {
				return "", "", err
			}
			store = path
		}
	default:
		return "", "", fmt.Errorf("unsupported auth backend type %q (want %q, %q or %q)", backendType, BackendSQLite, BackendPostgres, BackendFile)
	}
	return bt, store, nil
}

// NewPrincipalProvider builds a PrincipalProvider for the configured backend,
// resolving empty type/store to defaults (sqlite at the default principals db).
func NewPrincipalProvider(backendType, store string) (PrincipalProvider, error) {
	bt, resolved, err := resolveBackend(backendType, store)
	if err != nil {
		return nil, err
	}
	if driver, ok := sqlDriverForBackend(bt); ok {
		return NewSQLPrincipalProviderWithDriver(driver, resolved), nil
	}
	switch bt {
	case BackendFile:
		return NewFilePrincipalProvider(resolved), nil
	default:
		return nil, fmt.Errorf("unsupported auth backend type %q", backendType)
	}
}
