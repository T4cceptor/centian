package oauth

import (
	"testing"

	"gotest.tools/assert"
)

func TestEnsurePendingReplacesOlderFlowForBinding(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	manager, err := NewManager("http://127.0.0.1:8080", nil, nil)
	assert.NilError(t, err)

	binding := Binding{PrincipalID: "principal-1", Gateway: "gw", Server: "srv"}
	metadata := &ResolvedMetadata{
		Resource:              "https://resource.example/mcp",
		Scopes:                []string{"tool:echo"},
		Issuer:                "https://issuer.example",
		AuthorizationEndpoint: "https://issuer.example/authorize",
		TokenEndpoint:         "https://issuer.example/token",
		ClientAuthMethod:      "client_secret_post",
	}

	first, err := manager.CreatePending(binding, "client-id", "client-secret", metadata, "verifier-1")
	assert.NilError(t, err)
	first.Status = PendingStatusFailed

	second, reused, err := manager.EnsurePending(binding, "client-id", "client-secret", metadata)
	assert.NilError(t, err)
	assert.Assert(t, !reused)
	assert.Assert(t, second != nil)
	assert.Assert(t, second.ID != first.ID)
	assert.Assert(t, manager.pending.getByID(first.ID) == nil)
	assert.Equal(t, len(manager.pending.list()), 1)

	current := manager.PendingForBinding(binding)
	assert.Assert(t, current != nil)
	assert.Equal(t, current.ID, second.ID)
}
