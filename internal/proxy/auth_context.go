package proxy

import (
	"context"
	"net/http"

	"github.com/T4cceptor/centian/internal/auth"
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
