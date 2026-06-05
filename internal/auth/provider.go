package auth

import (
	"context"
	"errors"
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
	Close() error
}
