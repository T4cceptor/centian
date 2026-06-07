package proxy

import (
	"context"
	"net/http"

	"github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/common"
)

// AuthData stores raw auth-related references/snapshots for internal use.
//
// This struct intentionally does not match processor contracts. Handlers are
// responsible for mapping AuthData to processor-facing context.
type AuthData struct {
	AuthHeaderName string
	Project        string
	Gateway        string
	Headers        http.Header
	Principal      *auth.Principal
	CredentialID   string
}

// Clone creates a deep copy of AuthData to ensure immutability when attached to contexts.
func (a *AuthData) Clone() *AuthData {
	if a == nil {
		return nil
	}
	var headersCopy http.Header
	if a.Headers != nil {
		headersCopy = a.Headers.Clone()
	}
	return &AuthData{
		AuthHeaderName: a.AuthHeaderName,
		Project:        a.Project,
		Gateway:        a.Gateway,
		Headers:        headersCopy,
		Principal:      a.Principal.Clone(),
		CredentialID:   a.CredentialID,
	}
}

type authDataKey struct{}

func withAuthData(ctx context.Context, authData *AuthData) context.Context {
	if authData == nil {
		return ctx
	}
	return context.WithValue(ctx, authDataKey{}, authData)
}

func getAuthData(ctx context.Context) *AuthData {
	if ctx == nil {
		return nil
	}
	authData, _ := ctx.Value(authDataKey{}).(*AuthData)
	return authData
}

// stampPrincipalMetadata records the resolved principal's id and human name onto
// an event's metadata. The name is captured as a denormalized fallback so it
// survives even if the principal id can no longer be resolved to a live principal.
func stampPrincipalMetadata(entry *common.LogEntry, identityKey string, authData *AuthData) {
	if entry == nil {
		return
	}
	if entry.Metadata == nil {
		entry.Metadata = make(map[string]string)
	}
	if identityKey != "" {
		entry.Metadata["principal_id"] = identityKey
	}
	if authData != nil && authData.Principal != nil && authData.Principal.DisplayName != "" {
		entry.Metadata["principal_name"] = authData.Principal.DisplayName
	}
}
