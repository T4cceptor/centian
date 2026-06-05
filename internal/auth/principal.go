package auth

// This file defines the Principal model: the first-class actor identity that
// authentication resolves to and that authorization, routing, and downstream
// pool reuse all key off. A Principal is decoupled from the concrete credential
// mechanism (today an API key, later SQL/external providers) so those backends
// can be added without touching consumers.

// Principal is the resolved actor for an authenticated request.
//
// ID is a stable, persisted identifier (identifiers.KindPrincipal, "pr_..."). It
// must remain stable across restarts for the same credential because it becomes
// the downstream pool identity key and the OAuth binding principal id; a fresh id
// per load would silently orphan persisted OAuth downstream bindings.
//
// Gateways and Projects are direct grants (an inline allowlist) carried straight
// from the credential. Empty list means allow-all; a "*" entry also means
// allow-all. Roles are intentionally not modelled yet; they are an additive
// follow-up behind the Authorizer seam.
type Principal struct {
	ID           string
	DisplayName  string
	CredentialID string
	Gateways     []string
	Projects     []string
}

// Clone returns a deep copy safe to store per-session without aliasing the
// provider's cached grants.
func (p *Principal) Clone() *Principal {
	if p == nil {
		return nil
	}
	clone := *p
	clone.Gateways = append([]string(nil), p.Gateways...)
	clone.Projects = append([]string(nil), p.Projects...)
	return &clone
}
