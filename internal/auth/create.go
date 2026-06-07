package auth

import (
	"context"
	"fmt"
)

// This file defines the backend-agnostic key-creation entry point used by the CLI
// (`auth new-key`) and `centian init`. It mints a new api key and persists it
// through whichever backend is configured, so callers never branch on storage.

// CreateAPIKeyParams describes a new api key to mint.
type CreateAPIKeyParams struct {
	Name     string   // Human-friendly principal name (optional)
	Gateways []string // Allowed gateways (empty = allow all)
	Projects []string // Allowed project slugs (empty = allow all)
}

// CreatedAPIKey is the result of minting a new api key. Token is shown once.
type CreatedAPIKey struct {
	Token        string
	PrincipalID  string
	CredentialID string
	BackendType  BackendType
	Store        string
}

// CreateAPIKey mints a new api key and persists it through the configured backend,
// resolving empty type/store to defaults (sqlite at the default principals db).
// The returned Token must be shown to the user immediately; it cannot be recovered.
func CreateAPIKey(ctx context.Context, backendType, store string, params CreateAPIKeyParams) (CreatedAPIKey, error) {
	bt, resolved, err := resolveBackend(backendType, store)
	if err != nil {
		return CreatedAPIKey{}, err
	}

	gen, err := GenerateAPIKey()
	if err != nil {
		return CreatedAPIKey{}, err
	}
	entry, err := NewAPIKeyEntry(gen)
	if err != nil {
		return CreatedAPIKey{}, err
	}
	entry.Name = params.Name
	entry.Gateways = params.Gateways
	entry.Projects = params.Projects

	if driver, ok := sqlDriverForBackend(bt); ok {
		store, err := openSQLPrincipalStoreWithDriver(driver, resolved)
		if err != nil {
			return CreatedAPIKey{}, err
		}
		defer func() { _ = store.Close() }()
		if err := store.createAPIKeyPrincipal(ctx, &entry, params.Name, params.Gateways, params.Projects); err != nil {
			return CreatedAPIKey{}, err
		}
	} else {
		switch bt {
		case BackendFile:
			if _, err := AppendAPIKey(resolved, &entry); err != nil {
				return CreatedAPIKey{}, err
			}
		default:
			return CreatedAPIKey{}, fmt.Errorf("unsupported auth backend type %q", backendType)
		}
	}

	return CreatedAPIKey{
		Token:        gen.Token,
		PrincipalID:  entry.PrincipalID,
		CredentialID: entry.ID,
		BackendType:  bt,
		Store:        resolved,
	}, nil
}
