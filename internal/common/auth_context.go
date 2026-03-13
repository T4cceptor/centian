package common

// AuthContext captures authenticated principal information for a request.
//
// This context is intended for policy, compliance, and processor-level decisions.
// It must never include raw credentials.
type AuthContext struct {
	Authenticated         bool   `json:"authenticated"`
	PrincipalID           string `json:"principal_id,omitempty"`
	PrincipalType         string `json:"principal_type,omitempty"` // e.g. "api_key"
	KeyID                 string `json:"key_id,omitempty"`
	Gateway               string `json:"gateway,omitempty"`
	AuthHeader            string `json:"auth_header,omitempty"`
	CredentialFingerprint string `json:"credential_fingerprint,omitempty"`
	InternalSessionID     string `json:"internal_session_id,omitempty"`
	TransportSessionID    string `json:"transport_session_id,omitempty"`
}
